package storage

import (
	"net/url"
	"strings"
	"testing"
)

func TestPresignPreservesUnicodeAndReservedFilenames(t *testing.T) {
	service := NewS3Service("https://storage.example.com", "https://cdn.example.com/media", "us-east-1", "media", "test-access", "test-secret")
	for _, name := range []string{"ảnh-cây.png", "photo?#%.png", "photo+copy.png"} {
		signed, err := service.GeneratePresignedPutURL(name, 900)
		if err != nil {
			t.Fatal(err)
		}
		upload, err := url.Parse(signed.UploadURL)
		if err != nil {
			t.Fatal(err)
		}
		public, err := url.Parse(signed.PublicURL)
		if err != nil {
			t.Fatal(err)
		}
		if upload.Fragment != "" || public.Fragment != "" || public.RawQuery != "" {
			t.Fatalf("filename changed URL structure: %s", name)
		}
		if upload.Path != "/media/"+signed.StorageKey || public.Path != "/media/"+signed.StorageKey {
			t.Fatalf("key round trip failed: %+v", signed)
		}
		if upload.Query().Get("X-Amz-Signature") == "" {
			t.Fatal("missing signature")
		}
		if strings.Contains(upload.EscapedPath(), "+") || strings.Contains(upload.EscapedPath(), "ảnh") {
			t.Fatalf("path not consistently URI-encoded: %s", upload.EscapedPath())
		}
	}
}
