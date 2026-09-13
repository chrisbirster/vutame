package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chrisbirster/vutame/internal/activity"
)

func registerActivityRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/feed", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireActivity(w, options) {
			return
		}
		limit, ok := activityLimit(w, r)
		if !ok {
			return
		}
		page, err := options.Activity.Following(r.Context(), user.ID, r.URL.Query().Get("cursor"), limit)
		if errors.Is(err, activity.ErrInvalidCursor) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /api/v1/activity/recent", func(w http.ResponseWriter, r *http.Request) {
		if !requireActivity(w, options) {
			return
		}
		limit, ok := activityLimit(w, r)
		if !ok {
			return
		}
		page, err := options.Activity.Recent(r.Context(), r.URL.Query().Get("cursor"), limit)
		if errors.Is(err, activity.ErrInvalidCursor) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /api/v1/discovery/trending", func(w http.ResponseWriter, r *http.Request) {
		if !requireActivity(w, options) {
			return
		}
		limit, ok := activityLimit(w, r)
		if !ok {
			return
		}
		items, err := options.Activity.Trending(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creators": items})
	})
}

func requireActivity(w http.ResponseWriter, options Options) bool {
	if options.Activity == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity unavailable"})
		return false
	}
	return true
}

func activityLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return activity.DefaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive number"})
		return 0, false
	}
	return activity.NormalizeLimit(limit), true
}
