package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Standard JWT & Crypto Errors
var (
	ErrEmptySecret       = errors.New("secret must not be empty")
	ErrEmptyPassword     = errors.New("password cannot be empty")
	ErrInvalidToken      = errors.New("invalid jwt token format")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrTokenExpired      = errors.New("jwt token has expired")
	ErrInvalidOAuthState = errors.New("invalid oauth state")
	ErrOAuthStateExpired = errors.New("oauth state has expired")
)

// JWTClaims holds standard claims for multi-tenant PostPilot authorization.
type JWTClaims struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	OrganizationID string `json:"organization_id"`
	OrgRole        string `json:"org_role"`
	Exp            int64  `json:"exp"`
	Iat            int64  `json:"iat"`
}

// computePasswordHash runs salted iterative HMAC-SHA256 over the plaintext password.
func computePasswordHash(password string, salt []byte) []byte {
	h := hmac.New(sha256.New, salt)
	h.Write([]byte(password))
	digest := h.Sum(nil)
	for i := 0; i < 10000; i++ {
		h = hmac.New(sha256.New, salt)
		h.Write(digest)
		digest = h.Sum(nil)
	}
	return digest
}

// HashPassword creates a secure salted iterative hash using only Go standard library crypto.
// Result format: $postpilot$v1$<saltHex>$<hashHex>
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate random salt: %w", err)
	}

	digest := computePasswordHash(password, salt)
	return fmt.Sprintf("$postpilot$v1$%s$%s", hex.EncodeToString(salt), hex.EncodeToString(digest)), nil
}

// VerifyPassword securely checks password against an encoded hash in constant time.
func VerifyPassword(password, encodedHash string) bool {
	if password == "" || encodedHash == "" {
		return false
	}

	var salt []byte
	var expectedHash []byte

	if strings.HasPrefix(encodedHash, "$postpilot$v1$") {
		parts := strings.Split(encodedHash, "$")
		// parts: ["", "postpilot", "v1", saltHex, hashHex]
		if len(parts) != 5 {
			return false
		}
		var err error
		salt, err = hex.DecodeString(parts[3])
		if err != nil || len(salt) == 0 {
			return false
		}
		expectedHash, err = hex.DecodeString(parts[4])
		if err != nil || len(expectedHash) == 0 {
			return false
		}
	} else if strings.Contains(encodedHash, ":") {
		// Fallback format: saltHex:hashHex
		parts := strings.Split(encodedHash, ":")
		if len(parts) != 2 {
			return false
		}
		var err error
		salt, err = hex.DecodeString(parts[0])
		if err != nil || len(salt) == 0 {
			return false
		}
		expectedHash, err = hex.DecodeString(parts[1])
		if err != nil || len(expectedHash) == 0 {
			return false
		}
	} else {
		return false
	}

	actualHash := computePasswordHash(password, salt)
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

// SignJWT signs the JWT claims using HS256 with 100% Go standard library.
func SignJWT(claims JWTClaims, secret string) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}

	if claims.Iat == 0 {
		claims.Iat = time.Now().Unix()
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	message := headerB64 + "." + payloadB64

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	signature := mac.Sum(nil)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + sigB64, nil
}

// VerifyJWT verifies an HS256 JWT token string against the given secret and returns claims.
func VerifyJWT(tokenStr, secret string) (*JWTClaims, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}

	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	message := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return nil, ErrInvalidSignature
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

// oAuthStatePayload represents the payload embedded into OAuth state parameters.
type oAuthStatePayload struct {
	OrgID     string `json:"org_id"`
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"ts"`
	Nonce     string `json:"nonce"`
}

// SignOAuthState signs orgID and userID with a timestamp and random nonce into a tamper-proof state string.
func SignOAuthState(orgID, userID, secret string) string {
	if secret == "" {
		return ""
	}

	nonceBytes := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, nonceBytes)

	st := oAuthStatePayload{
		OrgID:     orgID,
		UserID:    userID,
		Timestamp: time.Now().Unix(),
		Nonce:     hex.EncodeToString(nonceBytes),
	}

	payloadJSON, err := json.Marshal(st)
	if err != nil {
		return ""
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sigB64
}

// VerifyOAuthState decodes and verifies a signed OAuth state parameter against the secret.
// Ensures state was created within 1 hour and not forged.
func VerifyOAuthState(state, secret string) (orgID, userID string, err error) {
	if secret == "" {
		return "", "", ErrEmptySecret
	}

	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return "", "", ErrInvalidOAuthState
	}

	payloadB64, sigB64 := parts[0], parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return "", "", ErrInvalidOAuthState
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return "", "", ErrInvalidOAuthState
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", "", ErrInvalidOAuthState
	}

	var st oAuthStatePayload
	if err := json.Unmarshal(payloadBytes, &st); err != nil {
		return "", "", ErrInvalidOAuthState
	}

	now := time.Now().Unix()
	// Valid for 1 hour (3600 seconds), allow 5 minutes clock drift tolerance for future timestamps
	if now-st.Timestamp > 3600 || st.Timestamp-now > 300 {
		return "", "", ErrOAuthStateExpired
	}

	return st.OrgID, st.UserID, nil
}
