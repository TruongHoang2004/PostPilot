package domain

import (
	"strings"
	"time"
)

// Enums cho định dạng Media của bài đăng (Trừu tượng hóa cho mọi nền tảng xuất bản)
const (
	MediaTypeNone  = "NONE"  // Chỉ đăng chữ (Text-only)
	MediaTypeLink  = "LINK"  // Chữ + Link preview
	MediaTypeImage = "IMAGE" // 1 Ảnh đơn
	MediaTypeAlbum = "ALBUM" // Nhiều ảnh (Multi-photo / Gallery / Carousel)
	MediaTypeVideo = "VIDEO" // Video thường (Long-form / Chunked upload)
	MediaTypeReel  = "REEL"  // Video ngắn dọc 9:16 (Reels / Shorts / TikTok)
)

// NormalizeMediaType chuẩn hóa định dạng Media dựa trên dữ liệu đầu vào của bài đăng.
func NormalizeMediaType(mediaType string, linkURL string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(mediaType))
	if cleaned == "" {
		if strings.TrimSpace(linkURL) != "" {
			return MediaTypeLink
		}
		return MediaTypeNone
	}
	return cleaned
}

// IsHeavyMedia kiểm tra xem định dạng Media có cần xử lý ở hàng đợi nặng (Video/Reel) hay không.
func IsHeavyMedia(mediaType string) bool {
	switch strings.ToUpper(strings.TrimSpace(mediaType)) {
	case MediaTypeVideo, MediaTypeReel:
		return true
	default:
		return false
	}
}

// MediaAttachment là Value Object đại diện cho một tệp đính kèm trong bài viết.
type MediaAttachment struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Position int    `json:"position"`
}

// MediaAsset đại diện cho tài nguyên ảnh/video lưu trên Local / S3 / MinIO / CDN.
type MediaAsset struct {
	ID              string    `json:"id"`
	FileName        string    `json:"file_name"`
	StorageKey      string    `json:"storage_key"`
	PublicURL       string    `json:"public_url"`
	MimeType        string    `json:"mime_type"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	DurationSeconds int       `json:"duration_seconds"`
	CreatedAt       time.Time `json:"created_at"`
}

// IsVideo kiểm tra xem tài nguyên có phải định dạng video hay không.
func (m MediaAsset) IsVideo() bool {
	return strings.HasPrefix(strings.ToLower(m.MimeType), "video/")
}

// IsImage kiểm tra xem tài nguyên có phải định dạng hình ảnh hay không.
func (m MediaAsset) IsImage() bool {
	return strings.HasPrefix(strings.ToLower(m.MimeType), "image/")
}
