package redis

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

const (
	QueueKeyFast  = "postpilot:queue:fast"
	QueueKeyHeavy = "postpilot:queue:heavy"
	RateKeyPrefix = "postpilot:ratelimit:channel:"
)

// ErrQueueEmpty is returned when BRPOP times out without receiving a job.
var ErrQueueEmpty = errors.New("redis queue empty timeout")

// Client provides a Redis connection via standard RESP (Redis Serialization Protocol),
// supporting an in-memory fallback for testing and development without a live Redis server.
type Client struct {
	addr     string
	password string
	db       int

	useMemoryFallback bool
	mu                sync.Mutex
	memLists          map[string][]string
	memKV             map[string]string
}

// NewClient creates a real Redis client. Callers must verify connectivity before serving work.
func NewClient(addr, password string, db int) *Client {
	c := &Client{
		addr:     addr,
		password: password,
		db:       db,
		memLists: make(map[string][]string),
		memKV:    make(map[string]string),
	}
	return c
}

// NewInMemoryClient creates an in-memory Redis client mock for testing.
func NewInMemoryClient() *Client {
	return &Client{
		useMemoryFallback: true,
		memLists:          make(map[string][]string),
		memKV:             make(map[string]string),
	}
}

func (c *Client) IsMemoryFallback() bool {
	return c.useMemoryFallback
}

func (c *Client) Ping(ctx context.Context) error {
	if c.useMemoryFallback {
		return nil
	}
	resp, err := c.execCommand(ctx, 3*time.Second, "PING")
	if err != nil {
		return err
	}
	if str, ok := resp.(string); ok && strings.EqualFold(str, "PONG") {
		return nil
	}
	return fmt.Errorf("unexpected PING response: %v", resp)
}

// EnqueuePublishJob pushes a publishing job payload into either the fast or heavy queue.
func (c *Client) EnqueuePublishJob(ctx context.Context, payload domain.PublishJobPayload) error {
	queueKey := QueueKeyFast
	if payload.QueueTier == domain.QueueTierHeavy {
		queueKey = QueueKeyHeavy
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if c.useMemoryFallback {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.memLists[queueKey] = append(c.memLists[queueKey], string(raw))
		return nil
	}

	_, err = c.execCommand(ctx, 5*time.Second, "LPUSH", queueKey, string(raw))
	return err
}

// DequeuePublishJob pops a job from the queue (FIFO using RPOP / BRPOP).
func (c *Client) DequeuePublishJob(ctx context.Context, tier string, timeoutSec int) (*domain.PublishJobPayload, error) {
	queueKey := QueueKeyFast
	if tier == domain.QueueTierHeavy {
		queueKey = QueueKeyHeavy
	}

	if c.useMemoryFallback {
		c.mu.Lock()
		list := c.memLists[queueKey]
		if len(list) == 0 {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(timeoutSec) * time.Second):
				return nil, ErrQueueEmpty
			}
		}
		item := list[0]
		c.memLists[queueKey] = list[1:]
		c.mu.Unlock()

		var job domain.PublishJobPayload
		if err := json.Unmarshal([]byte(item), &job); err != nil {
			return nil, err
		}
		return &job, nil
	}

	timeout := time.Duration(timeoutSec+5) * time.Second
	resp, err := c.execCommand(ctx, timeout, "BRPOP", queueKey, strconv.Itoa(timeoutSec))
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, ErrQueueEmpty
	}

	arr, ok := resp.([]any)
	if !ok || len(arr) < 2 {
		return nil, ErrQueueEmpty
	}

	valStr, ok := arr[1].(string)
	if !ok {
		return nil, errors.New("invalid BRPOP payload type")
	}

	var job domain.PublishJobPayload
	if err := json.Unmarshal([]byte(valStr), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// QueueDepth returns the number of jobs waiting in the specified queue tier.
func (c *Client) QueueDepth(ctx context.Context, tier string) (int, error) {
	queueKey := QueueKeyFast
	if tier == domain.QueueTierHeavy {
		queueKey = QueueKeyHeavy
	}

	if c.useMemoryFallback {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.memLists[queueKey]), nil
	}

	resp, err := c.execCommand(ctx, 3*time.Second, "LLEN", queueKey)
	if err != nil {
		return 0, err
	}
	if n, ok := resp.(int64); ok {
		return int(n), nil
	}
	return 0, nil
}

// SaveChannelRateLimit caches the rate limit usage for a channel in Redis with a 15-minute TTL.
func (c *Client) SaveChannelRateLimit(ctx context.Context, channelExternalID string, usage domain.RateLimitUsage) error {
	key := RateKeyPrefix + channelExternalID
	raw, _ := json.Marshal(usage)

	if c.useMemoryFallback {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.memKV[key] = string(raw)
		return nil
	}

	_, err := c.execCommand(ctx, 3*time.Second, "SETEX", key, "900", string(raw))
	return err
}

// GetChannelRateLimit reads cached channel rate limit usage from Redis.
func (c *Client) GetChannelRateLimit(ctx context.Context, channelExternalID string) (domain.RateLimitUsage, error) {
	key := RateKeyPrefix + channelExternalID
	var usage domain.RateLimitUsage

	if c.useMemoryFallback {
		c.mu.Lock()
		raw, ok := c.memKV[key]
		c.mu.Unlock()
		if !ok {
			return usage, nil
		}
		_ = json.Unmarshal([]byte(raw), &usage)
		return usage, nil
	}

	resp, err := c.execCommand(ctx, 3*time.Second, "GET", key)
	if err != nil || resp == nil {
		return usage, err
	}
	if raw, ok := resp.(string); ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &usage)
	}
	return usage, nil
}

// DeleteChannelRateLimit deletes the cached rate limit metrics for a channel upon channel deletion.
func (c *Client) DeleteChannelRateLimit(ctx context.Context, channelExternalID string) error {
	key := RateKeyPrefix + channelExternalID
	if c.useMemoryFallback {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.memKV, key)
		return nil
	}
	_, err := c.execCommand(ctx, 3*time.Second, "DEL", key)
	return err
}

// execCommand executes a Redis command complying with the RESP2 protocol over TCP socket.
func (c *Client) execCommand(ctx context.Context, timeout time.Duration, args ...string) (any, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	reader := bufio.NewReader(conn)

	if c.password != "" {
		if err := writeRESPArray(conn, []string{"AUTH", c.password}); err != nil {
			return nil, err
		}
		if _, err := readRESP(reader); err != nil {
			return nil, fmt.Errorf("redis auth: %w", err)
		}
	}

	if c.db > 0 {
		if err := writeRESPArray(conn, []string{"SELECT", strconv.Itoa(c.db)}); err != nil {
			return nil, err
		}
		if _, err := readRESP(reader); err != nil {
			return nil, fmt.Errorf("redis select db: %w", err)
		}
	}

	if err := writeRESPArray(conn, args); err != nil {
		return nil, err
	}
	return readRESP(reader)
}

func writeRESPArray(w io.Writer, args []string) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, arg := range args {
		sb.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	_, err := io.WriteString(w, sb.String())
	return err
}

func readRESP(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(line) == 0 {
		return nil, errors.New("empty RESP line")
	}

	prefix := line[0]
	payload := line[1:]

	switch prefix {
	case '+': // Simple String
		return payload, nil
	case '-': // Error
		return nil, fmt.Errorf("redis error: %s", payload)
	case ':': // Integer
		return strconv.ParseInt(payload, 10, 64)
	case '$': // Bulk String
		length, err := strconv.Atoi(payload)
		if err != nil {
			return nil, err
		}
		if length == -1 {
			return nil, nil
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:length]), nil
	case '*': // Array
		count, err := strconv.Atoi(payload)
		if err != nil {
			return nil, err
		}
		if count == -1 {
			return nil, nil
		}
		items := make([]any, count)
		for i := 0; i < count; i++ {
			val, err := readRESP(r)
			if err != nil {
				return nil, err
			}
			items[i] = val
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix: %c", prefix)
	}
}
