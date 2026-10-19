package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// S3Service hỗ trợ tạo Presigned PUT URL chuẩn AWS Signature V4
// tương thích với cả MinIO (Local Docker), AWS S3 và Cloudflare R2.
type S3Service struct {
	Endpoint      string
	PublicBaseURL string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
}

// NewS3Service khởi tạo dịch vụ lưu trữ Media S3/MinIO.
func NewS3Service(endpoint, publicBaseURL, region, bucket, accessKey, secretKey string) *S3Service {
	return &S3Service{
		Endpoint:      strings.TrimRight(endpoint, "/"),
		PublicBaseURL: strings.TrimRight(publicBaseURL, "/"),
		Region:        region,
		Bucket:        bucket,
		AccessKey:     accessKey,
		SecretKey:     secretKey,
	}
}

// PresignedUploadResult chứa thông tin trả về cho Frontend upload trực tiếp lên S3/MinIO.
type PresignedUploadResult struct {
	StorageKey string `json:"storage_key"`
	UploadURL  string `json:"upload_url"`
	PublicURL  string `json:"public_url"`
	ExpiresIn  int    `json:"expires_in_seconds"`
}

// GeneratePresignedPutURL tạo đường dẫn có chữ ký AWS SigV4 để client upload file trực tiếp.
func (s *S3Service) GeneratePresignedPutURL(fileName string, expiresSeconds int) (*PresignedUploadResult, error) {
	if expiresSeconds <= 0 {
		expiresSeconds = 900 // 15 phút
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	cleanName := path.Base(strings.ReplaceAll(fileName, " ", "_"))
	storageKey := fmt.Sprintf("uploads/%s/%d_%s", dateStamp, now.UnixMilli(), cleanName)

	parsedEndpoint, err := url.Parse(s.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid s3 endpoint: %w", err)
	}

	host := parsedEndpoint.Host
	canonicalURI := fmt.Sprintf("/%s/%s", s.Bucket, storageKey)
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.Region)
	credential := fmt.Sprintf("%s/%s", s.AccessKey, credentialScope)

	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", credential)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", expiresSeconds))
	query.Set("X-Amz-SignedHeaders", "host")

	canonicalQueryString := query.Encode()
	canonicalHeaders := fmt.Sprintf("host:%s\n", host)
	signedHeaders := "host"
	payloadHash := "UNSIGNED-PAYLOAD"

	canonicalRequest := strings.Join([]string{
		"PUT",
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := getSignatureKey(s.SecretKey, dateStamp, s.Region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	uploadURL := fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s", s.Endpoint, canonicalURI, canonicalQueryString, signature)
	publicURL := fmt.Sprintf("%s/%s", s.PublicBaseURL, storageKey)

	return &PresignedUploadResult{
		StorageKey: storageKey,
		UploadURL:  uploadURL,
		PublicURL:  publicURL,
		ExpiresIn:  expiresSeconds,
	}, nil
}

// Upload gửi trực tiếp file stream lên S3/MinIO thông qua Presigned PUT URL.
func (s *S3Service) Upload(ctx context.Context, fileName, contentType string, data io.Reader, size int64) (string, string, error) {
	presigned, err := s.GeneratePresignedPutURL(fileName, 900)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presigned.UploadURL, io.NopCloser(data))
	if err != nil {
		return "", "", err
	}
	if size > 0 {
		req.ContentLength = size
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("s3 put failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", fmt.Errorf("s3 put returned status %d: %s", resp.StatusCode, string(body))
	}

	return presigned.PublicURL, presigned.StorageKey, nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func getSignatureKey(secret, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
