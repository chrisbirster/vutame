package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/moderation"
)

func registerModerationRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/me/moderation/appeals", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireModerationService(w, options) {
			return
		}
		items, err := options.Moderation.AppealsForUser(r.Context(), user.ID, 100)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"appeals": items})
	})

	mux.HandleFunc("POST /api/v1/me/moderation/appeals", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireModerationService(w, options) || !requireJSON(w, r) || !allowRate(w, options, "moderation-appeal:"+user.ID, 5, 24*time.Hour) {
			return
		}
		var input struct {
			Message string `json:"message"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Moderation.CreateAppeal(r.Context(), user.ID, input.Message)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("GET /api/v1/admin/moderation/session", func(w http.ResponseWriter, r *http.Request) {
		user, role, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": user.ID, "role": role})
	})

	mux.HandleFunc("GET /api/v1/admin/moderation/reports", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		limit, ok := moderationLimit(w, r)
		if !ok {
			return
		}
		items, err := options.Moderation.Queue(r.Context(), user.ID, r.URL.Query().Get("status"), limit)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reports": items})
	})

	mux.HandleFunc("POST /api/v1/admin/moderation/reports/{id}/assign", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		item, err := options.Moderation.Assign(r.Context(), user.ID, r.PathValue("id"))
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/admin/moderation/reports/{id}/notes", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok || !requireJSON(w, r) {
			return
		}
		var input struct {
			Note string `json:"note"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if moderationError(w, options.Moderation.AddNote(r.Context(), user.ID, r.PathValue("id"), input.Note)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/admin/moderation/reports/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok || !requireJSON(w, r) {
			return
		}
		var input struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Moderation.Resolve(r.Context(), user.ID, r.PathValue("id"), input.Status, input.Note)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("PUT /api/v1/admin/moderation/profiles/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok || !requireJSON(w, r) {
			return
		}
		var input struct {
			State string `json:"state"`
			Note  string `json:"note"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if moderationError(w, options.Moderation.SetState(r.Context(), user.ID, r.PathValue("handle"), input.State, input.Note)) {
			return
		}
		policy, err := options.Moderation.Policy(r.Context(), r.PathValue("handle"))
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, policy)
	})

	mux.HandleFunc("GET /api/v1/admin/moderation/actions", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		limit, ok := moderationLimit(w, r)
		if !ok {
			return
		}
		items, err := options.Moderation.Actions(r.Context(), user.ID, limit)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"actions": items})
	})

	mux.HandleFunc("GET /api/v1/admin/moderation/abuse", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		hours := 24
		if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 720 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hours must be between 1 and 720"})
				return
			}
			hours = parsed
		}
		item, err := options.Moderation.Stats(r.Context(), user.ID, time.Now().UTC().Add(-time.Duration(hours)*time.Hour))
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/admin/moderation/appeals", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok {
			return
		}
		limit, ok := moderationLimit(w, r)
		if !ok {
			return
		}
		items, err := options.Moderation.AppealQueue(r.Context(), user.ID, r.URL.Query().Get("status"), limit)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"appeals": items})
	})

	mux.HandleFunc("POST /api/v1/admin/moderation/appeals/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := requireModerator(w, r, options)
		if !ok || !requireJSON(w, r) {
			return
		}
		var input struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Moderation.ReviewAppeal(r.Context(), user.ID, r.PathValue("id"), input.Status, input.Note)
		if moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func requireModerationService(w http.ResponseWriter, options Options) bool {
	if options.Moderation == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "moderation operations unavailable"})
		return false
	}
	return true
}

func requireModerator(w http.ResponseWriter, r *http.Request, options Options) (authUser, moderation.Role, bool) {
	user, ok := requireAuthenticatedUser(w, r, options)
	if !ok {
		return authUser{}, "", false
	}
	if !requireModerationService(w, options) {
		return authUser{}, "", false
	}
	role, err := options.Moderation.Role(r.Context(), user.ID)
	if errors.Is(err, moderation.ErrForbidden) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "moderation admin access required"})
		return authUser{}, "", false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return authUser{}, "", false
	}
	return authUser{ID: user.ID}, role, true
}

type authUser struct {
	ID string
}

func moderationLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 200"})
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func moderationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, moderation.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "moderation admin access required"})
	case errors.Is(err, moderation.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, moderation.ErrProfileRequired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta first"})
	case errors.Is(err, moderation.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid moderation request"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return true
}
