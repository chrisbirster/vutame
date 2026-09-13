package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/auth"
)

const sessionCookieName = "vutame_session"

func registerAuthRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("POST /api/v1/auth/code", func(w http.ResponseWriter, r *http.Request) {
		if options.Auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
			return
		}
		if !requireJSON(w, r) {
			return
		}
		var input struct {
			Email string `json:"email"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		challengeID, err := options.Auth.RequestCode(r.Context(), input.Email)
		if errors.Is(err, auth.ErrInvalidEmail) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
			return
		}
		if errors.Is(err, auth.ErrDeliveryUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "email delivery unavailable"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"challenge_id": challengeID})
	})

	mux.HandleFunc("POST /api/v1/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		if options.Auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
			return
		}
		if !requireJSON(w, r) {
			return
		}
		var input struct {
			ChallengeID string `json:"challenge_id"`
			Email       string `json:"email"`
			Code        string `json:"code"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		result, err := options.Auth.VerifyCode(r.Context(), input.ChallengeID, input.Email, input.Code)
		if errors.Is(err, auth.ErrInvalidCode) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		setSessionCookie(w, result.Token, result.ExpiresAt, options.CookieSecure)
		writeJSON(w, http.StatusOK, map[string]any{"user": result.User})
	})

	mux.HandleFunc("GET /api/v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		if options.Auth == nil {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
			return
		}
		user, err := options.Auth.Session(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrSessionNotFound) {
			clearSessionCookie(w, options.CookieSecure)
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": user})
	})

	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if !requireJSON(w, r) {
			return
		}
		if options.Auth != nil {
			if cookie, err := r.Cookie(sessionCookieName); err == nil {
				_ = options.Auth.Logout(r.Context(), cookie.Value)
			}
		}
		clearSessionCookie(w, options.CookieSecure)
		w.WriteHeader(http.StatusNoContent)
	})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "application/json required"})
		return false
	}
	return true
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
