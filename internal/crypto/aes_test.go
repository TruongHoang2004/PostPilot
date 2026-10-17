package crypto

import "testing"

func TestTokenCipher_EncryptDecrypt(t *testing.T) {
	cipher, err := NewTokenCipher("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("unexpected error creating cipher: %v", err)
	}

	originalToken := "EAABsbCS1iHgBO1234567890PermanentFacebookPageAccessTokenXYZ"
	encrypted, err := cipher.Encrypt(originalToken)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if encrypted == originalToken {
		t.Fatalf("encrypted token must not equal plaintext")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if decrypted != originalToken {
		t.Fatalf("expected %q, got %q", originalToken, decrypted)
	}
}

func TestMaskToken(t *testing.T) {
	masked := MaskToken("EAABsbCS1iHgBO9876")
	if masked != "EAABsb...9876" {
		t.Fatalf("unexpected masked output: %s", masked)
	}
}
