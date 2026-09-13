package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/social"
)

func registerSocialRoutes(mux *http.ServeMux, profiles profile.Store, options Options) {
	mux.HandleFunc("GET /api/v1/discovery", func(w http.ResponseWriter, r *http.Request) {
		if !allowRate(w, options, "search:"+remoteRateIdentity(r), 120, time.Minute) {
			return
		}
		viewerUserID := optionalViewerUserID(w, r, options)
		if options.Social == nil {
			items, err := profiles.Discover()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				return
			}
			query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
			creators := make([]social.Creator, 0, len(items))
			for _, item := range items {
				if query != "" && !strings.Contains(strings.ToLower(item.Handle+" "+item.DisplayName+" "+item.Bio), query) {
					continue
				}
				creators = append(creators, social.Creator{
					Handle: item.Handle, DisplayName: item.DisplayName, Bio: item.Bio,
					AvatarURL: item.AvatarURL, Theme: item.Theme, Verified: item.Verified,
					Interests: []string{},
				})
			}
			writeJSON(w, http.StatusOK, social.CreatorPage{Creators: creators})
			return
		}
		limit := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a number"})
				return
			}
			limit = parsed
		}
		page, err := options.Social.SearchPage(r.Context(), social.SearchInput{
			Query: r.URL.Query().Get("q"), Category: r.URL.Query().Get("category"),
			Interest: r.URL.Query().Get("interest"), Cursor: r.URL.Query().Get("cursor"), Limit: limit,
		}, viewerUserID)
		if errors.Is(err, social.ErrInvalidMetadata) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, social.ErrInvalidCursor) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /api/v1/creators/{handle}/social", func(w http.ResponseWriter, r *http.Request) {
		if !requireSocial(w, options) {
			return
		}
		item, err := options.Social.Creator(r.Context(), r.PathValue("handle"), optionalViewerUserID(w, r, options))
		if errors.Is(err, social.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/creators/{handle}/followers", func(w http.ResponseWriter, r *http.Request) {
		if !requireSocial(w, options) {
			return
		}
		items, err := options.Social.Followers(r.Context(), r.PathValue("handle"), optionalViewerUserID(w, r, options))
		if errors.Is(err, social.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creators": items})
	})

	mux.HandleFunc("GET /api/v1/creators/{handle}/following", func(w http.ResponseWriter, r *http.Request) {
		if !requireSocial(w, options) {
			return
		}
		items, err := options.Social.Following(r.Context(), r.PathValue("handle"), optionalViewerUserID(w, r, options))
		if errors.Is(err, social.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creators": items})
	})

	mux.HandleFunc("PUT /api/v1/me/follows/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSocial(w, options) || !allowRate(w, options, "follow:"+user.ID, 60, time.Hour) {
			return
		}
		err := options.Social.Follow(r.Context(), user.ID, r.PathValue("handle"))
		switch {
		case errors.Is(err, social.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
		case errors.Is(err, social.ErrSelfFollow):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, social.ErrProfileRequired):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before following creators"})
		case errors.Is(err, social.ErrRelationshipBlocked), errors.Is(err, social.ErrFollowsDisabled):
			// Keep block direction and privacy choices private.
			writeJSON(w, http.StatusConflict, map[string]string{"error": "follow unavailable"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	mux.HandleFunc("DELETE /api/v1/me/follows/{handle}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSocial(w, options) || !allowRate(w, options, "follow:"+user.ID, 60, time.Hour) {
			return
		}
		err := options.Social.Unfollow(r.Context(), user.ID, r.PathValue("handle"))
		if errors.Is(err, social.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "creator not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PUT /api/v1/me/discovery-profile", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireSocial(w, options) || !requireJSON(w, r) {
			return
		}
		var input social.MetadataInput
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		item, err := options.Social.UpdateMetadata(r.Context(), user.ID, input)
		if errors.Is(err, social.ErrInvalidMetadata) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, social.ErrProfileRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before editing discovery metadata"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func requireSocial(w http.ResponseWriter, options Options) bool {
	if options.Social == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social graph unavailable"})
		return false
	}
	return true
}

func optionalViewerUserID(w http.ResponseWriter, r *http.Request, options Options) string {
	if options.Auth == nil {
		return ""
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	user, err := options.Auth.Session(r.Context(), cookie.Value)
	if errors.Is(err, auth.ErrSessionNotFound) {
		clearSessionCookie(w, options.CookieSecure)
		return ""
	}
	if err != nil {
		return ""
	}
	return user.ID
}
