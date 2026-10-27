package crypto

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password := "SecretSuperP@ssw0rd!2026"

	// 1. Valid hash and verify
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	if !strings.HasPrefix(hash, "$postpilot$v1$") {
		t.Fatalf("expected prefix $postpilot$v1$, got: %s", hash)
	}

	if !VerifyPassword(password, hash) {
		t.Fatalf("expected password verification to succeed")
	}

	// 2. Invalid password
	if VerifyPassword("WrongPassword", hash) {
		t.Fatalf("expected verification to fail with wrong password")
	}

	// 3. Empty password or empty hash
	if VerifyPassword("", hash) {
		t.Fatalf("expected empty password to fail verification")
	}
	if VerifyPassword(password, "") {
		t.Fatalf("expected empty hash to fail verification")
	}

	_, err = HashPassword("")
	if err == nil {
		t.Fatalf("expected error when hashing empty password")
	}

	// 4. Malformed hash format
	if VerifyPassword(password, "malformed$hash") {
		t.Fatalf("expected malformed hash to fail verification")
	}

	// 5. Test colon format support
	parts := strings.Split(hash, "$")
	colonFormat := parts[3] + ":" + parts[4]
	if !VerifyPassword(password, colonFormat) {
		t.Fatalf("expected colon format verification to succeed")
	}
}

func TestJWT_SignAndVerify(t *testing.T) {
	secret := "super-secret-key-for-test-32bytes!!"
	claims := JWTClaims{
		UserID:         "user-123",
		Email:          "alice@example.com",
		OrganizationID: "org-456",
		OrgRole:        "ADMIN",
		Exp:            time.Now().Add(1 * time.Hour).Unix(),
		Iat:            time.Now().Unix(),
	}

	// 1. Valid JWT
	token, err := SignJWT(claims, secret)
	if err != nil {
		t.Fatalf("unexpected error signing JWT: %v", err)
	}

	verified, err := VerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("unexpected error verifying JWT: %v", err)
	}

	if verified.UserID != claims.UserID || verified.Email != claims.Email ||
		verified.OrganizationID != claims.OrganizationID || verified.OrgRole != claims.OrgRole {
		t.Fatalf("mismatched claims: %+v vs %+v", verified, claims)
	}

	// 2. Secret validation
	_, err = SignJWT(claims, "")
	if err != ErrEmptySecret {
		t.Fatalf("expected ErrEmptySecret, got: %v", err)
	}

	_, err = VerifyJWT(token, "")
	if err != ErrEmptySecret {
		t.Fatalf("expected ErrEmptySecret, got: %v", err)
	}

	// 3. Invalid signature
	_, err = VerifyJWT(token, "wrong-secret-key-abcdef1234567890")
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}

	// 4. Expired token
	expiredClaims := JWTClaims{
		UserID:         "user-expired",
		Email:          "expired@example.com",
		OrganizationID: "org-expired",
		OrgRole:        "MEMBER",
		Exp:            time.Now().Add(-10 * time.Minute).Unix(),
		Iat:            time.Now().Add(-20 * time.Minute).Unix(),
	}

	expiredToken, err := SignJWT(expiredClaims, secret)
	if err != nil {
		t.Fatalf("unexpected error signing expired JWT: %v", err)
	}

	_, err = VerifyJWT(expiredToken, secret)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}

	// 5. Malformed tokens
	malformedCases := []string{
		"invalid",
		"a.b",
		"a.b.c.d",
		"a.invalid_base64.c",
	}
	for _, tc := range malformedCases {
		if _, err := VerifyJWT(tc, secret); err == nil {
			t.Fatalf("expected error for malformed token %q", tc)
		}
	}
}

func TestOAuthState_SignAndVerify(t *testing.T) {
	secret := "oauth-secret-key-xyz"
	orgID := "org-uuid-1"
	userID := "user-uuid-1"

	// 1. Valid state
	state := SignOAuthState(orgID, userID, secret)
	if state == "" {
		t.Fatalf("expected non-empty state")
	}

	gotOrgID, gotUserID, err := VerifyOAuthState(state, secret)
	if err != nil {
		t.Fatalf("unexpected error verifying oauth state: %v", err)
	}

	if gotOrgID != orgID || gotUserID != userID {
		t.Fatalf("expected org=%s user=%s, got org=%s user=%s", orgID, userID, gotOrgID, gotUserID)
	}

	// 2. Empty secret
	if SignOAuthState(orgID, userID, "") != "" {
		t.Fatalf("expected empty string when signing with empty secret")
	}
	if _, _, err := VerifyOAuthState(state, ""); err != ErrEmptySecret {
		t.Fatalf("expected ErrEmptySecret, got: %v", err)
	}

	// 3. Tampered state
	tampered := state + "extra"
	if _, _, err := VerifyOAuthState(tampered, secret); err == nil {
		t.Fatalf("expected error for tampered state")
	}

	// 4. Wrong secret
	if _, _, err := VerifyOAuthState(state, "wrong-oauth-secret"); err == nil {
		t.Fatalf("expected error for wrong secret")
	}

	// 5. Malformed state string
	if _, _, err := VerifyOAuthState("no_dot_here", secret); err != ErrInvalidOAuthState {
		t.Fatalf("expected ErrInvalidOAuthState, got: %v", err)
	}
}

func TestJWTTamperingAndRoleClaims(t *testing.T) {
	secret := "jwt-secure-signing-key-test-32b"
	roles := []string{"OWNER", "ADMIN", "MEMBER"}

	for _, role := range roles {
		claims := JWTClaims{
			UserID:         "usr-" + role,
			Email:          role + "@test.org",
			OrganizationID: "org-12345",
			OrgRole:        role,
			Exp:            time.Now().Add(30 * time.Minute).Unix(),
			Iat:            time.Now().Unix(),
		}

		token, err := SignJWT(claims, secret)
		if err != nil {
			t.Fatalf("failed to sign JWT for role %s: %v", role, err)
		}

		verified, err := VerifyJWT(token, secret)
		if err != nil {
			t.Fatalf("failed to verify JWT for role %s: %v", role, err)
		}
		if verified.OrgRole != role {
			t.Fatalf("expected role %s, got %s", role, verified.OrgRole)
		}

		// Tampering test: tamper with the payload part (middle segment)
		parts := strings.Split(token, ".")
		if len(parts) == 3 {
			// Tampered payload with escalated role
			tamperedToken := parts[0] + "." + parts[1] + "tamper" + "." + parts[2]
			if _, err := VerifyJWT(tamperedToken, secret); err == nil {
				t.Fatalf("expected verification to fail for tampered token")
			}
		}
	}
}

