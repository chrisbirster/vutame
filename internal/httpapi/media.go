package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/chrisbirster/vutame/internal/media"
)

const mediaMultipartOverhead = 1 << 20

func registerMediaRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /media/{id}", func(w http.ResponseWriter, r *http.Request) {
		if options.Media == nil {
			http.NotFound(w, r)
			return
		}
		asset, err := options.Media.Open(r.Context(), r.PathValue("id"))
		if errors.Is(err, media.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer asset.Reader.Close()
		w.Header().Set("Content-Type", asset.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(asset.SizeBytes, 10))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, err := io.Copy(w, asset.Reader); err != nil {
			return
		}
	})

	mux.HandleFunc("POST /api/v1/me/avatar", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireMedia(w, options) || !requireSameOriginMutation(w, r) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, media.MaxImageBytes+mediaMultipartOverhead)
		if err := r.ParseMultipartForm(media.MaxImageBytes); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload"})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image file is required"})
			return
		}
		defer file.Close()
		if header.Size > media.MaxImageBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("image must be %d MB or smaller", media.MaxImageBytes>>20)})
			return
		}
		asset, err := options.Media.UploadAvatar(r.Context(), user.ID, file)
		switch {
		case errors.Is(err, media.ErrTooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("image must be %d MB or smaller", media.MaxImageBytes>>20)})
			return
		case errors.Is(err, media.ErrInvalidImage):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		case errors.Is(err, media.ErrProfileNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not claimed"})
			return
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusCreated, asset)
	})

	mux.HandleFunc("DELETE /api/v1/me/avatar", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireMedia(w, options) || !requireSameOriginMutation(w, r) {
			return
		}
		err := options.Media.DeleteAvatar(r.Context(), user.ID)
		if errors.Is(err, media.ErrProfileNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not claimed"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func requireMedia(w http.ResponseWriter, options Options) bool {
	if options.Media == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media uploads unavailable"})
		return false
	}
	return true
}

func requireSameOriginMutation(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin mutation denied"})
		return false
	}
	return true
}
