package domain

import (
	"strings"
	"time"
)

// Post media format constants (abstracted for all social publishing platforms).
const (
	MediaTypeNone  = "NONE"  // Text-only
	MediaTypeLink  = "LINK"  // Text + Link preview
	MediaTypeImage = "IMAGE" // Single photo
	MediaTypeAlbum = "ALBUM" // Multi-photo / Gallery / Carousel
	MediaTypeVideo = "VIDEO" // Standard video (Long-form / Chunked upload)
	MediaTypeReel  = "REEL"  // Short vertical video 9:16 (Reels / Shorts / TikTok)
)

// NormalizeMediaType normalizes the media format based on post input data.
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

// IsHeavyMedia checks if the media format requires processing in the heavy queue (Video/Reel).
func IsHeavyMedia(mediaType string) bool {
	switch strings.ToUpper(strings.TrimSpace(mediaType)) {
	case MediaTypeVideo, MediaTypeReel:
		return true
	default:
		return false
	}
}

// MediaAttachment is a Value Object representing a media file attached to a post.
type MediaAttachment struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Position int    `json:"position"`
}

// MediaAsset represents a media asset stored in Local / S3 / MinIO / CDN storage.
type MediaAsset struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organization_id"`
	FileName        string    `json:"file_name"`
	StorageKey      string    `json:"storage_key"`
	PublicURL       string    `json:"public_url"`
	MimeType        string    `json:"mime_type"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	DurationSeconds int       `json:"duration_seconds"`
	CreatedAt       time.Time `json:"created_at"`
}

// IsVideo checks if the media asset is a video file.
func (m MediaAsset) IsVideo() bool {
	return strings.HasPrefix(strings.ToLower(m.MimeType), "video/")
}

// IsImage checks if the media asset is an image file.
func (m MediaAsset) IsImage() bool {
	return strings.HasPrefix(strings.ToLower(m.MimeType), "image/")
}
