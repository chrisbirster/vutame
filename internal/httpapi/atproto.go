package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/atproto"
)

func registerATProtoRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /oauth-client-metadata.json", func(w http.ResponseWriter, _ *http.Request) {
		if !requireATProto(w, options) {
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		writeJSON(w, http.StatusOK, options.ATProto.ClientMetadata())
	})

	mux.HandleFunc("GET /api/v1/atproto/resolve", func(w http.ResponseWriter, r *http.Request) {
		if !requireATProto(w, options) || !allowRate(w, options, "atproto-resolve:"+r.RemoteAddr, 30, time.Hour) {
			return
		}
		identifier := strings.TrimSpace(r.URL.Query().Get("identifier"))
		if identifier == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identifier is required"})
			return
		}
		item, err := options.ATProto.ResolveIdentity(r.Context(), identifier)
		if atprotoError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/me/atproto", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireATProto(w, options) {
			return
		}
		item, err := options.ATProto.Account(r.Context(), user.ID)
		if errors.Is(err, atproto.ErrNotLinked) {
			writeJSON(w, http.StatusOK, map[string]bool{"linked": false})
			return
		}
		if atprotoError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"linked": true, "account": item})
	})

	mux.HandleFunc("POST /api/v1/me/atproto/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireATProto(w, options) || !requireJSON(w, r) || !allowRate(w, options, "atproto-oauth:"+user.ID, 10, time.Hour) {
			return
		}
		var input struct {
			Identifier string `json:"identifier"`
		}
		if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Identifier) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid AT Protocol handle or DID required"})
			return
		}
		item, err := options.ATProto.StartOAuth(r.Context(), user.ID, input.Identifier)
		if atprotoError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/me/atproto/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireATProto(w, options) {
			return
		}
		_, err := options.ATProto.CompleteOAuth(
			r.Context(), user.ID,
			r.URL.Query().Get("state"),
			r.URL.Query().Get("code"),
			r.URL.Query().Get("iss"),
		)
		if err != nil {
			// The callback is a top-level browser navigation. Keep protocol details
			// out of the URL while returning the creator to the settings surface.
			http.Redirect(w, r, "/settings/atproto?error=oauth", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/settings/atproto?linked=1", http.StatusSeeOther)
	})

	mux.HandleFunc("PUT /api/v1/me/atproto", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireATProto(w, options) || !requireJSON(w, r) {
			return
		}
		var input struct {
			ConflictPolicy string `json:"conflict_policy"`
			PublishEnabled bool   `json:"publish_enabled"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.ATProto.UpdateSettings(r.Context(), user.ID, input.ConflictPolicy, input.PublishEnabled)
		if atprotoError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("DELETE /api/v1/me/atproto", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireATProto(w, options) {
			return
		}
		if atprotoError(w, options.ATProto.Unlink(r.Context(), user.ID)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func requireATProto(w http.ResponseWriter, options Options) bool {
	if options.ATProto == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AT Protocol integration unavailable"})
		return false
	}
	return true
}

func atprotoError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, atproto.ErrInvalidIdentity):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or unverified AT Protocol identity"})
	case errors.Is(err, atproto.ErrOAuthState):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired OAuth state"})
	case errors.Is(err, atproto.ErrOAuthResponse):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "AT Protocol authorization server rejected the request"})
	case errors.Is(err, atproto.ErrConflict):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, atproto.ErrNotLinked):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta or link an AT Protocol identity first"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return true
}
