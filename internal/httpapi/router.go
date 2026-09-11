package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/profile"
)

type Options struct {
	MarketingOrigin string
	ProfileOrigin   string
	Auth            *auth.Service
	CookieSecure    bool
}

func New(web http.Handler, profiles profile.Store, options Options) http.Handler {
	if profiles == nil {
		panic("httpapi: profile store is required")
	}
	options = normalizeOptions(options)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"product":          "vutame",
			"marketing_origin": options.MarketingOrigin,
			"profile_origin":   options.ProfileOrigin,
		})
	})

	mux.HandleFunc("GET /api/v1/discover", func(w http.ResponseWriter, _ *http.Request) {
		items, err := profiles.Discover()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": items})
	})

	mux.HandleFunc("GET /api/v1/handles/{handle}/availability", func(w http.ResponseWriter, r *http.Request) {
		handle := profile.NormalizeHandle(r.PathValue("handle"))
		available, err := profiles.HandleAvailable(handle)
		if errors.Is(err, profile.ErrInvalidHandle) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"handle": handle, "available": available})
	})

	mux.HandleFunc("GET /api/v1/profiles/{handle}", func(w http.ResponseWriter, r *http.Request) {
		handle := strings.TrimSpace(r.PathValue("handle"))
		item, err := profiles.Get(handle)
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

	registerAuthRoutes(mux, options)
	mux.Handle("/", web)
	return mux
}

func normalizeOptions(options Options) Options {
	options.MarketingOrigin = strings.TrimRight(strings.TrimSpace(options.MarketingOrigin), "/")
	options.ProfileOrigin = strings.TrimRight(strings.TrimSpace(options.ProfileOrigin), "/")
	if options.MarketingOrigin == "" {
		options.MarketingOrigin = "https://vutame.com"
	}
	if options.ProfileOrigin == "" {
		options.ProfileOrigin = "https://vuta.me"
	}
	return options
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
