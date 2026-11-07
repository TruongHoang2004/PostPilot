package api

import (
	"encoding/json"
	"net/http"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleMediaUpload handles direct file uploads for images/videos and syncs to S3/MinIO (if configured).
func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	if s.s3 == nil || s.s3.Endpoint == "" {
		writeJSON(w, 503, map[string]any{"error": "object storage is not configured"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid upload or file exceeds 512 MiB"})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "file is required"})
		return
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	publicURL, key, err := s.s3.Upload(r.Context(), header.Filename, mime, file, header.Size)
	if err != nil {
		s.logger.Error("Object storage upload failed", "error", err)
		writeJSON(w, 502, map[string]any{"error": "object storage upload failed; no local fallback was used"})
		return
	}
	asset, err := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{FileName: header.Filename, StorageKey: key, PublicURL: publicURL, MimeType: mime, FileSizeBytes: header.Size})
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "file stored but metadata could not be saved; contact operator"})
		return
	}
	writeJSON(w, 200, map[string]any{"message": "Media uploaded successfully", "public_url": publicURL, "storage_key": key, "file_name": header.Filename, "file_size": header.Size, "mime_type": mime, "media_asset": asset})
}

// handlePresignUpload generates AWS Signature V4 presigned PUT URLs for direct client uploads.
func (s *Server) handlePresignUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileName      string `json:"file_name"`
		MimeType      string `json:"mime_type"`
		FileSizeBytes int64  `json:"file_size_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file_name is required"})
		return
	}

	if s.s3 == nil || s.s3.Endpoint == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "s3 storage is not configured"})
		return
	}

	presigned, err := s.s3.GeneratePresignedPutURL(req.FileName, 900)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	asset, err := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{
		FileName:      req.FileName,
		StorageKey:    presigned.StorageKey,
		PublicURL:     presigned.PublicURL,
		MimeType:      req.MimeType,
		FileSizeBytes: req.FileSizeBytes,
	})

	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "cannot save upload metadata"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"presigned":   presigned,
		"media_asset": asset,
	})
}
