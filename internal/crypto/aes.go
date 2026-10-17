package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// TokenCipher cung cấp cơ chế mã hóa đối xứng AES-256-GCM chuẩn công nghiệp
// để bảo vệ Facebook Page Access Token khi lưu trong Database.
type TokenCipher struct {
	key []byte
}

// NewTokenCipher khởi tạo bộ mã hóa từ chuỗi key (chấp nhận 64 ký tự hex, 32 ký tự raw, hoặc băm SHA-256).
func NewTokenCipher(rawKey string) (*TokenCipher, error) {
	if rawKey == "" {
		return nil, errors.New("token encryption key must not be empty")
	}

	var keyBytes []byte
	if len(rawKey) == 64 {
		decoded, err := hex.DecodeString(rawKey)
		if err == nil && len(decoded) == 32 {
			keyBytes = decoded
		}
	}
	if keyBytes == nil {
		if len(rawKey) == 32 {
			keyBytes = []byte(rawKey)
		} else {
			sum := sha256.Sum256([]byte(rawKey))
			keyBytes = sum[:]
		}
	}

	return &TokenCipher{key: keyBytes}, nil
}

// Encrypt mã hóa chuỗi plaintext token sang chuỗi Base64 chứa [nonce + ciphertext + GCM tag].
func (tc *TokenCipher) Encrypt(plainText string) (string, error) {
	if plainText == "" {
		return "", errors.New("cannot encrypt empty token")
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt giải mã chuỗi Base64 về lại raw Facebook Access Token.
func (tc *TokenCipher) Decrypt(cipherTextBase64 string) (string, error) {
	if cipherTextBase64 == "" {
		return "", errors.New("empty cipher text")
	}

	data, err := base64.StdEncoding.DecodeString(cipherTextBase64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherBytes := data[:nonceSize], data[nonceSize:]
	plainBytes, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", fmt.Errorf("gcm decrypt failed (invalid key or corrupted token): %w", err)
	}

	return string(plainBytes), nil
}

// MaskToken che giấu phần giữa của token khi hiển thị trên log hoặc UI.
func MaskToken(token string) string {
	if len(token) <= 10 {
		return "********"
	}
	return token[:6] + "..." + token[len(token)-4:]
}
