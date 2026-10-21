package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleMediaUpload handles direct file uploads for images/videos and syncs to S3/MinIO (if configured).
func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file too large or invalid multipart form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'file' field in form: " + err.Error()})
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	uploadDir := "./uploads"
	_ = os.MkdirAll(uploadDir, 0755)
	cleanName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), filepath.Base(strings.ReplaceAll(header.Filename, " ", "_")))
	localPath := filepath.Join(uploadDir, cleanName)

	dst, err := os.Create(localPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cannot save file locally: " + err.Error()})
		return
	}
	writtenBytes, copyErr := io.Copy(dst, file)
	_ = dst.Close()
	if copyErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cannot copy file: " + copyErr.Error()})
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	publicURL := fmt.Sprintf("%s://%s/uploads/%s", scheme, r.Host, cleanName)
	storageKey := "uploads/" + cleanName

	if s.s3 != nil && s.s3.Endpoint != "" && !strings.Contains(s.s3.PublicBaseURL, "localhost") && !strings.Contains(s.s3.PublicBaseURL, "127.0.0.1") {
		if localFile, openErr := os.Open(localPath); openErr == nil {
			if pub, key, s3Err := s.s3.Upload(r.Context(), header.Filename, mimeType, localFile, writtenBytes); s3Err == nil {
				publicURL = pub
				storageKey = key
			} else {
				s.logger.Warn("S3 upload failed, using local storage URL", "error", s3Err)
			}
			_ = localFile.Close()
		}
	}

	asset, _ := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{
		FileName:      header.Filename,
		StorageKey:    storageKey,
		PublicURL:     publicURL,
		MimeType:      mimeType,
		FileSizeBytes: writtenBytes,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message":     "Media uploaded successfully",
		"public_url":  publicURL,
		"storage_key": storageKey,
		"file_name":   header.Filename,
		"file_size":   writtenBytes,
		"mime_type":   mimeType,
		"media_asset": asset,
	})
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

	asset, _ := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{
		FileName:      req.FileName,
		StorageKey:    presigned.StorageKey,
		PublicURL:     presigned.PublicURL,
		MimeType:      req.MimeType,
		FileSizeBytes: req.FileSizeBytes,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"presigned":   presigned,
		"media_asset": asset,
	})
}

