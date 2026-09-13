package httpapi

import (
	"errors"
	"net/http"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/profile"
)

func registerEditorRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/me/profile", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok {
			return
		}
		if options.Editor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "profile editing unavailable"})
			return
		}
		item, err := options.Editor.GetOwned(r.Context(), user.ID)
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not claimed"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/me/profile/claim", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) || !requireJSON(w, r) {
			return
		}
		var input struct {
			Handle string `json:"handle"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Editor.Claim(r.Context(), user.ID, input.Handle)
		switch {
		case errors.Is(err, profile.ErrInvalidHandle):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		case errors.Is(err, profile.ErrHandleTaken), errors.Is(err, profile.ErrProfileExists):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		case errors.Is(err, profile.ErrNotFound):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "account not found"})
			return
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("PATCH /api/v1/me/profile", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) || !requireJSON(w, r) {
			return
		}
		var input profile.UpdateInput
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Editor.Update(r.Context(), user.ID, input)
		if errors.Is(err, profile.ErrInvalidProfile) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not claimed"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/me/links", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) || !requireJSON(w, r) {
			return
		}
		var input profile.LinkInput
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Editor.CreateLink(r.Context(), user.ID, input)
		if errors.Is(err, profile.ErrInvalidLink) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not claimed"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("PATCH /api/v1/me/links/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) || !requireJSON(w, r) {
			return
		}
		var input profile.LinkInput
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Editor.UpdateLink(r.Context(), user.ID, r.PathValue("id"), input)
		if errors.Is(err, profile.ErrInvalidLink) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("DELETE /api/v1/me/links/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) {
			return
		}
		err := options.Editor.DeleteLink(r.Context(), user.ID, r.PathValue("id"))
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PUT /api/v1/me/links/order", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireEditor(w, options) || !requireJSON(w, r) {
			return
		}
		var input struct {
			IDs []string `json:"ids"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		err := options.Editor.ReorderLinks(r.Context(), user.ID, input.IDs)
		if errors.Is(err, profile.ErrInvalidLink) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func requireEditor(w http.ResponseWriter, options Options) bool {
	if options.Editor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "profile editing unavailable"})
		return false
	}
	return true
}

func requireAuthenticatedUser(w http.ResponseWriter, r *http.Request, options Options) (auth.User, bool) {
	if options.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return auth.User{}, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	user, err := options.Auth.Session(r.Context(), cookie.Value)
	if errors.Is(err, auth.ErrSessionNotFound) {
		clearSessionCookie(w, options.CookieSecure)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return auth.User{}, false
	}
	return user, true
}
