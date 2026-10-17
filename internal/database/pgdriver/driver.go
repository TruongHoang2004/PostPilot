package pgdriver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	sql.Register("postgres", &Driver{})
}

// Driver implements database/sql/driver.Driver using PostgreSQL Wire Protocol v3.0.
type Driver struct{}

// Open opens a new connection to PostgreSQL server.
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	cfg, err := parseDSN(dsn)
	if err != nil {
		return nil, err
	}

	netConn, err := net.DialTimeout("tcp", net.JoinHostPort(cfg.Host, cfg.Port), 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("postgres dial %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	conn := &pgConn{
		conn: netConn,
		cfg:  cfg,
	}

	if err := conn.startup(); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	return conn, nil
}

type dsnConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

func parseDSN(dsn string) (*dsnConfig, error) {
	cfg := &dsnConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "postgres",
		Database: "postgres",
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return nil, fmt.Errorf("parse postgres url: %w", err)
		}
		if u.Hostname() != "" {
			cfg.Host = u.Hostname()
		}
		if u.Port() != "" {
			cfg.Port = u.Port()
		}
		if u.User != nil {
			cfg.User = u.User.Username()
			pw, _ := u.User.Password()
			cfg.Password = pw
		}
		dbName := strings.TrimPrefix(u.Path, "/")
		if dbName != "" {
			cfg.Database = dbName
		}
		return cfg, nil
	}

	for _, part := range strings.Fields(dsn) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := strings.TrimSpace(kv[0]), strings.Trim(strings.TrimSpace(kv[1]), "'\"")
		switch k {
		case "host":
			cfg.Host = v
		case "port":
			cfg.Port = v
		case "user":
			cfg.User = v
		case "password":
			cfg.Password = v
		case "dbname":
			cfg.Database = v
		}
	}
	return cfg, nil
}

type pgConn struct {
	conn   net.Conn
	cfg    *dsnConfig
	closed bool
}

func (c *pgConn) startup() error {
	var body bytes.Buffer
	// Protocol version 3.0 (196608)
	_ = binary.Write(&body, binary.BigEndian, int32(196608))
	writeCString(&body, "user")
	writeCString(&body, c.cfg.User)
	writeCString(&body, "database")
	writeCString(&body, c.cfg.Database)
	writeCString(&body, "client_encoding")
	writeCString(&body, "UTF8")
	writeCString(&body, "DateStyle")
	writeCString(&body, "ISO, MDY")
	body.WriteByte(0)

	if err := c.sendPacket(0, body.Bytes()); err != nil {
		return err
	}

	for {
		typ, payload, err := c.readPacket()
		if err != nil {
			return err
		}
		switch typ {
		case 'R': // Authentication request
			if len(payload) < 4 {
				return errors.New("short auth packet")
			}
			authType := int32(binary.BigEndian.Uint32(payload[:4]))
			switch authType {
			case 0: // AuthenticationOk
				// Continue reading until ReadyForQuery ('Z')
			case 3: // CleartextPassword
				var pw bytes.Buffer
				writeCString(&pw, c.cfg.Password)
				if err := c.sendPacket('p', pw.Bytes()); err != nil {
					return err
				}
			case 5: // MD5Password
				if len(payload) < 8 {
					return errors.New("short md5 auth packet")
				}
				salt := payload[4:8]
				h1 := md5.Sum([]byte(c.cfg.Password + c.cfg.User))
				h1Hex := hex.EncodeToString(h1[:])
				h2Input := append([]byte(h1Hex), salt...)
				h2 := md5.Sum(h2Input)
				md5Pass := "md5" + hex.EncodeToString(h2[:])
				var pw bytes.Buffer
				writeCString(&pw, md5Pass)
				if err := c.sendPacket('p', pw.Bytes()); err != nil {
					return err
				}
			case 10: // AuthenticationSASL (SCRAM-SHA-256)
				if err := c.handleSCRAM(payload[4:]); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported postgres auth type: %d", authType)
			}
		case 'E': // ErrorResponse
			return parseErrorResponse(payload)
		case 'Z': // ReadyForQuery
			return nil
		default:
			// Ignore 'S' (ParameterStatus), 'K' (BackendKeyData), 'N' (NoticeResponse)
		}
	}
}

func (c *pgConn) handleSCRAM(mechanismsPayload []byte) error {
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	clientNonce := base64.StdEncoding.EncodeToString(nonceBytes)
	clientFirstBare := "n=,r=" + clientNonce
	clientFirstMsg := "n,," + clientFirstBare

	var saslInit bytes.Buffer
	writeCString(&saslInit, "SCRAM-SHA-256")
	_ = binary.Write(&saslInit, binary.BigEndian, int32(len(clientFirstMsg)))
	saslInit.WriteString(clientFirstMsg)
	if err := c.sendPacket('p', saslInit.Bytes()); err != nil {
		return err
	}

	// Read AuthenticationSASLContinue (type 'R', code 11)
	typ, payload, err := c.readPacket()
	if err != nil {
		return err
	}
	if typ == 'E' {
		return parseErrorResponse(payload)
	}
	if typ != 'R' || len(payload) < 4 || binary.BigEndian.Uint32(payload[:4]) != 11 {
		return fmt.Errorf("expected SASLContinue, got %c", typ)
	}

	serverFirstMsg := string(payload[4:])
	serverParams := parseSCRAMAttrs(serverFirstMsg)
	serverNonce := serverParams["r"]
	saltB64 := serverParams["s"]
	iterStr := serverParams["i"]
	if !strings.HasPrefix(serverNonce, clientNonce) {
		return errors.New("invalid SCRAM server nonce")
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return fmt.Errorf("decode scram salt: %w", err)
	}
	iterations, err := strconv.Atoi(iterStr)
	if err != nil || iterations < 1 {
		return fmt.Errorf("invalid scram iterations: %s", iterStr)
	}

	saltedPassword := pbkdf2SHA256([]byte(c.cfg.Password), salt, iterations)
	clientKey := hmacSHA256(saltedPassword, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)

	clientFinalWithoutProof := "c=biws,r=" + serverNonce
	authMessage := clientFirstBare + "," + serverFirstMsg + "," + clientFinalWithoutProof
	clientSignature := hmacSHA256(storedKey[:], []byte(authMessage))

	clientProof := make([]byte, len(clientKey))
	for i := range clientKey {
		clientProof[i] = clientKey[i] ^ clientSignature[i]
	}
	clientFinalMsg := clientFinalWithoutProof + ",p=" + base64.StdEncoding.EncodeToString(clientProof)

	if err := c.sendPacket('p', []byte(clientFinalMsg)); err != nil {
		return err
	}

	// Read AuthenticationSASLFinal (type 'R', code 12)
	typ, payload, err = c.readPacket()
	if err != nil {
		return err
	}
	if typ == 'E' {
		return parseErrorResponse(payload)
	}
	if typ == 'R' && len(payload) >= 4 && binary.BigEndian.Uint32(payload[:4]) == 12 {
		return nil
	}
	return fmt.Errorf("unexpected response after SASLFinal: %c", typ)
}

func parseSCRAMAttrs(msg string) map[string]string {
	res := make(map[string]string)
	for _, part := range strings.Split(msg, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			res[kv[0]] = kv[1]
		}
	}
	return res
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func pbkdf2SHA256(password, salt []byte, iter int) []byte {
	prf := hmac.New(sha256.New, password)
	prf.Write(salt)
	prf.Write([]byte{0, 0, 0, 1})
	u := prf.Sum(nil)
	out := make([]byte, len(u))
	copy(out, u)
	for i := 2; i <= iter; i++ {
		prf.Reset()
		prf.Write(u)
		u = prf.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}

func (c *pgConn) Prepare(query string) (driver.Stmt, error) {
	return &pgStmt{conn: c, query: query}, nil
}

func (c *pgConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	_ = c.sendPacket('X', nil)
	return c.conn.Close()
}

func (c *pgConn) Begin() (driver.Tx, error) {
	if _, err := c.ExecContext(context.Background(), "BEGIN", nil); err != nil {
		return nil, err
	}
	return &pgTx{conn: c}, nil
}

func (c *pgConn) Ping(ctx context.Context) error {
	_, err := c.ExecContext(ctx, "SELECT 1", nil)
	return err
}

func (c *pgConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	finalQuery, err := InterpolateQuery(query, args)
	if err != nil {
		return nil, err
	}
	_, rowsAffected, err := c.execSimpleQuery(finalQuery)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(rowsAffected), nil
}

func (c *pgConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	finalQuery, err := InterpolateQuery(query, args)
	if err != nil {
		return nil, err
	}
	rows, _, err := c.execSimpleQuery(finalQuery)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *pgConn) execSimpleQuery(sqlText string) (*pgRows, int64, error) {
	var body bytes.Buffer
	writeCString(&body, sqlText)
	if err := c.sendPacket('Q', body.Bytes()); err != nil {
		return nil, 0, driver.ErrBadConn
	}

	resultRows := &pgRows{}
	var rowsAffected int64
	var queryErr error

	for {
		typ, payload, err := c.readPacket()
		if err != nil {
			return nil, 0, driver.ErrBadConn
		}
		switch typ {
		case 'T': // RowDescription
			cols, err := parseRowDescription(payload)
			if err != nil {
				queryErr = err
			} else {
				resultRows.columns = cols
				resultRows.data = nil
			}
		case 'D': // DataRow
			rowVals, err := parseDataRow(payload)
			if err != nil {
				queryErr = err
			} else {
				resultRows.data = append(resultRows.data, rowVals)
			}
		case 'C': // CommandComplete
			tag := strings.TrimRight(string(payload), "\x00")
			parts := strings.Fields(tag)
			if len(parts) > 0 {
				if n, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil {
					rowsAffected = n
				}
			}
		case 'E': // ErrorResponse
			queryErr = parseErrorResponse(payload)
		case 'Z': // ReadyForQuery
			if queryErr != nil {
				return nil, 0, queryErr
			}
			return resultRows, rowsAffected, nil
		default:
			// Ignore 'I' (EmptyQueryResponse), 'N' (NoticeResponse), 'S' (ParameterStatus)
		}
	}
}

func parseRowDescription(payload []byte) ([]string, error) {
	if len(payload) < 2 {
		return nil, errors.New("short RowDescription")
	}
	numCols := int(binary.BigEndian.Uint16(payload[:2]))
	pos := 2
	cols := make([]string, 0, numCols)
	for i := 0; i < numCols; i++ {
		idx := bytes.IndexByte(payload[pos:], 0)
		if idx < 0 {
			return nil, errors.New("malformed RowDescription column name")
		}
		colName := string(payload[pos : pos+idx])
		cols = append(cols, colName)
		pos += idx + 1 + 18 // 18 bytes of tableOID(4)+attrNum(2)+typeOID(4)+typeLen(2)+typeMod(4)+format(2)
	}
	return cols, nil
}

func parseDataRow(payload []byte) ([]driver.Value, error) {
	if len(payload) < 2 {
		return nil, errors.New("short DataRow")
	}
	numCols := int(binary.BigEndian.Uint16(payload[:2]))
	pos := 2
	vals := make([]driver.Value, numCols)
	for i := 0; i < numCols; i++ {
		if pos+4 > len(payload) {
			return nil, errors.New("truncated DataRow")
		}
		colLen := int(int32(binary.BigEndian.Uint32(payload[pos : pos+4])))
		pos += 4
		if colLen == -1 {
			vals[i] = nil
			continue
		}
		if pos+colLen > len(payload) {
			return nil, errors.New("truncated DataRow column value")
		}
		vals[i] = string(payload[pos : pos+colLen])
		pos += colLen
	}
	return vals, nil
}

func parseErrorResponse(payload []byte) error {
	var severity, code, msg, detail string
	pos := 0
	for pos < len(payload) {
		fieldType := payload[pos]
		pos++
		if fieldType == 0 {
			break
		}
		idx := bytes.IndexByte(payload[pos:], 0)
		if idx < 0 {
			break
		}
		val := string(payload[pos : pos+idx])
		pos += idx + 1
		switch fieldType {
		case 'S':
			severity = val
		case 'C':
			code = val
		case 'M':
			msg = val
		case 'D':
			detail = val
		}
	}
	if detail != "" {
		return fmt.Errorf("postgres %s (%s): %s - %s", severity, code, msg, detail)
	}
	return fmt.Errorf("postgres %s (%s): %s", severity, code, msg)
}

func (c *pgConn) sendPacket(typ byte, payload []byte) error {
	var buf bytes.Buffer
	if typ != 0 {
		buf.WriteByte(typ)
	}
	_ = binary.Write(&buf, binary.BigEndian, int32(len(payload)+4))
	buf.Write(payload)
	_, err := c.conn.Write(buf.Bytes())
	return err
}

func (c *pgConn) readPacket() (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(c.conn, header[:]); err != nil {
		return 0, nil, err
	}
	typ := header[0]
	length := int(binary.BigEndian.Uint32(header[1:5])) - 4
	if length < 0 {
		return 0, nil, errors.New("negative packet length")
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return 0, nil, err
		}
	}
	return typ, payload, nil
}

func writeCString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0)
}

var paramRegex = regexp.MustCompile(`\$(\d+)`)

// InterpolateQuery safely substitutes $1, $2, ... placeholders with SQL literals.
func InterpolateQuery(query string, args []driver.NamedValue) (string, error) {
	if len(args) == 0 {
		return query, nil
	}
	argMap := make(map[int]any, len(args))
	for _, a := range args {
		argMap[a.Ordinal] = a.Value
	}

	var replaceErr error
	result := paramRegex.ReplaceAllStringFunc(query, func(match string) string {
		idx, err := strconv.Atoi(match[1:])
		if err != nil {
			return match
		}
		val, ok := argMap[idx]
		if !ok {
			replaceErr = fmt.Errorf("missing parameter $%d", idx)
			return match
		}
		lit, err := formatSQLLiteral(val)
		if err != nil {
			replaceErr = err
			return match
		}
		return lit
	})
	return result, replaceErr
}

func formatSQLLiteral(v any) (string, error) {
	if v == nil {
		return "NULL", nil
	}
	switch val := v.(type) {
	case int64:
		return strconv.FormatInt(val, 10), nil
	case int:
		return strconv.Itoa(val), nil
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case bool:
		if val {
			return "TRUE", nil
		}
		return "FALSE", nil
	case time.Time:
		return "'" + val.UTC().Format(time.RFC3339Nano) + "'", nil
	case []byte:
		return "'" + strings.ReplaceAll(string(val), "'", "''") + "'", nil
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'", nil
	default:
		return "'" + strings.ReplaceAll(fmt.Sprint(val), "'", "''") + "'", nil
	}
}

type pgStmt struct {
	conn  *pgConn
	query string
}

func (s *pgStmt) Close() error  { return nil }
func (s *pgStmt) NumInput() int { return -1 }

func (s *pgStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}

func (s *pgStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

type pgTx struct {
	conn *pgConn
}

func (tx *pgTx) Commit() error {
	_, err := tx.conn.ExecContext(context.Background(), "COMMIT", nil)
	return err
}

func (tx *pgTx) Rollback() error {
	_, err := tx.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

type pgRows struct {
	columns []string
	data    [][]driver.Value
	idx     int
}

func (r *pgRows) Columns() []string { return r.columns }
func (r *pgRows) Close() error      { return nil }

func (r *pgRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.idx]
	r.idx++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}
