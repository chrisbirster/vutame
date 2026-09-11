package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/chrisbirster/vutame/internal/profile"
)

func New(web http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/discover", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"profiles": profile.Discover()})
	})

	mux.HandleFunc("GET /api/v1/profiles/{handle}", func(w http.ResponseWriter, r *http.Request) {
		handle := strings.TrimSpace(r.PathValue("handle"))
		item, err := profile.Get(handle)
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.Handle("/", web)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
