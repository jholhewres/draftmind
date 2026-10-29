package httpapi

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // decoders for the formats a photo may come in
	_ "image/png"
	"io"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/photo"
)

// Photo limits: the browser downscales to about 1600 px, so a larger photo
// is refused rather than decoded.
const (
	maxPhotoBytes = 5 << 20
	maxPhotoSide  = 4096
)

// WithPhotoReader reads drafts from photos; nil leaves POST /api/draft/read unavailable.
func (s *Server) WithPhotoReader(r photo.Reader) *Server {
	s.reader = r
	return s
}

type readResponse struct {
	Patch string `json:"patch"`
	photo.Read
}

// readDraft reads the draft from one uploaded photo of the pick screen
// (ADR-0006). The photo stays in memory: it is never written to disk or
// stored, and is dropped once read.
func (s *Server) readDraft(w http.ResponseWriter, r *http.Request) {
	if s.reader == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no photo reader is configured"})
		return
	}
	data, status, err := photoUpload(w, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the photo must be a JPEG or PNG image"})
		return
	}
	if cfg.Width > maxPhotoSide || cfg.Height > maxPhotoSide {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "the photo is larger than 4096 px on a side"})
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the photo could not be decoded"})
		return
	}
	patch, heroes, err := s.catalog.Heroes(r.Context())
	if err != nil {
		s.log.Error("draft read: heroes", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the heroes"})
		return
	}
	start := time.Now()
	read, err := s.reader.Read(r.Context(), img, heroes)
	if err != nil {
		s.log.Warn("draft read failed", "err", err, "latency", time.Since(start))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the photo could not be read; build the draft by hand"})
		return
	}
	attrs := []any{"reader", read.Reader, "fallback", read.Fallback, "latency", time.Since(start)}
	if read.Cost != nil {
		attrs = append(attrs, "cost_usd", *read.Cost)
	}
	s.log.Info("draft read", attrs...)
	writeJSON(w, http.StatusOK, readResponse{Patch: patch, Read: read})
}

// photoUpload reads the multipart field "image" into memory, at most
// maxPhotoBytes. It streams the parts instead of parsing the form, which
// would spill a large file to a temporary file on disk.
func photoUpload(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("send the photo as multipart/form-data")
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, http.StatusRequestEntityTooLarge, errors.New("the photo is over 5 MB")
			}
			return nil, http.StatusBadRequest, errors.New(`the form has no "image" file`)
		}
		if part.FormName() != "image" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, maxPhotoBytes+1))
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig) || len(data) > maxPhotoBytes:
			return nil, http.StatusRequestEntityTooLarge, errors.New("the photo is over 5 MB")
		case err != nil:
			return nil, http.StatusBadRequest, errors.New("the upload was cut short")
		}
		return data, http.StatusOK, nil
	}
}
