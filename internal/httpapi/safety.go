package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/chrisbirster/vutame/internal/safety"
)

func registerSafetyRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/me/privacy", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) {
			return
		}
		item, err := options.Safety.Privacy(r.Context(), user.ID)
		if errors.Is(err, safety.ErrProfileRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before editing privacy"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("PUT /api/v1/me/privacy", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !requireJSON(w, r) {
			return
		}
		var input safety.Privacy
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Safety.UpdatePrivacy(r.Context(), user.ID, input)
		if errors.Is(err, safety.ErrProfileRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before editing privacy"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/me/blocks", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) {
			return
		}
		items, err := options.Safety.Blocks(r.Context(), user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creators": items})
	})

	mux.HandleFunc("GET /api/v1/me/mutes", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) {
			return
		}
		items, err := options.Safety.Mutes(r.Context(), user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creators": items})
	})

	mux.HandleFunc("PUT /api/v1/me/blocks/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !allowRate(w, options, "block:"+user.ID, 60, time.Hour) {
			return
		}
		if safetyMutationError(w, options.Safety.Block(r.Context(), user.ID, r.PathValue("handle"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/me/blocks/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !allowRate(w, options, "block:"+user.ID, 60, time.Hour) {
			return
		}
		if safetyMutationError(w, options.Safety.Unblock(r.Context(), user.ID, r.PathValue("handle"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PUT /api/v1/me/mutes/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !allowRate(w, options, "mute:"+user.ID, 60, time.Hour) {
			return
		}
		if safetyMutationError(w, options.Safety.Mute(r.Context(), user.ID, r.PathValue("handle"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/me/mutes/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !allowRate(w, options, "mute:"+user.ID, 60, time.Hour) {
			return
		}
		if safetyMutationError(w, options.Safety.Unmute(r.Context(), user.ID, r.PathValue("handle"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/me/reports/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSafety(w, options) || !requireJSON(w, r) || !allowRate(w, options, "report:"+user.ID, 10, time.Hour) {
			return
		}
		var input safety.ReportInput
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Safety.Report(r.Context(), user.ID, r.PathValue("handle"), input)
		switch {
		case errors.Is(err, safety.ErrInvalidReport):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, safety.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
		case errors.Is(err, safety.ErrSelfAction):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, safety.ErrProfileRequired):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before reporting creators"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		default:
			writeJSON(w, http.StatusCreated, item)
		}
	})
}

func requireSafety(w http.ResponseWriter, options Options) bool {
	if options.Safety == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "safety controls unavailable"})
		return false
	}
	return true
}

func safetyMutationError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, safety.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
	case errors.Is(err, safety.ErrSelfAction):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, safety.ErrProfileRequired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before using safety controls"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	default:
		return false
	}
	return true
}

func allowRate(w http.ResponseWriter, options Options, key string, limit int, window time.Duration) bool {
	allowed, retry := options.Limiter.Allow(key, limit, window)
	if allowed {
		return true
	}
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
	return false
}
