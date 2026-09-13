package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chrisbirster/vutame/internal/linkpreview"
)

func registerLinkPreviewRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("POST /api/v1/me/link-preview", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAuthenticatedUser(w, r, options); !ok || !requireJSON(w, r) {
			return
		}
		var input struct {
			URL string `json:"url"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if len(strings.TrimSpace(input.URL)) > 2048 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL is too long"})
			return
		}
		metadata, err := options.Previewer.Fetch(r.Context(), input.URL)
		switch {
		case errors.Is(err, linkpreview.ErrInvalidURL):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "preview URL must be a public HTTPS URL"})
			return
		case errors.Is(err, linkpreview.ErrUnavailable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "preview metadata unavailable"})
			return
		case err != nil:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "preview metadata unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, metadata)
	})
}
