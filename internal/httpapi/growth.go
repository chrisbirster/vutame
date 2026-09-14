package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/growth"
)

func registerGrowthRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/profiles/{handle}/contact-block", func(w http.ResponseWriter, r *http.Request) {
		if !requireGrowth(w, options) {
			return
		}
		block, err := options.Growth.PublicBlock(r.Context(), r.PathValue("handle"))
		if errors.Is(err, growth.ErrProfileRequired) || errors.Is(err, growth.ErrContactDisabled) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contact block not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, block)
	})

	mux.HandleFunc("POST /api/v1/profiles/{handle}/contacts", func(w http.ResponseWriter, r *http.Request) {
		if !requireGrowth(w, options) || !requireJSON(w, r) || !allowRate(w, options, "contact:"+remoteRateIdentity(r), 12, time.Hour) {
			return
		}
		var input struct {
			Email    string `json:"email"`
			Consent  bool   `json:"consent"`
			Campaign string `json:"campaign"`
			Website  string `json:"website"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		// Honeypot submissions are accepted without storing anything so automated
		// form fillers do not receive a useful signal.
		if strings.TrimSpace(input.Website) != "" {
			writeJSON(w, http.StatusCreated, map[string]string{"status": "received"})
			return
		}
		_, err := options.Growth.SubmitContact(r.Context(), r.PathValue("handle"), input.Email, input.Consent, input.Campaign, time.Now().UTC())
		switch {
		case errors.Is(err, growth.ErrProfileRequired), errors.Is(err, growth.ErrContactDisabled):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contact block not found"})
		case errors.Is(err, growth.ErrConsentRequired):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "consent is required"})
		case errors.Is(err, growth.ErrInvalidEmail):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a valid email address"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		default:
			writeJSON(w, http.StatusCreated, map[string]string{"status": "received"})
		}
	})

	mux.HandleFunc("GET /api/v1/me/contact-block", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireGrowth(w, options) {
			return
		}
		block, err := options.Growth.OwnedBlock(r.Context(), user.ID)
		if growthProfileError(w, err, "claim a Vuta before configuring contact capture") {
			return
		}
		writeJSON(w, http.StatusOK, block)
	})

	mux.HandleFunc("PUT /api/v1/me/contact-block", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireGrowth(w, options) || !requireJSON(w, r) {
			return
		}
		var input growth.ContactBlock
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		block, err := options.Growth.UpdateBlock(r.Context(), user.ID, input)
		if errors.Is(err, growth.ErrInvalidBlock) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "contact block text is too long"})
			return
		}
		if growthProfileError(w, err, "claim a Vuta before configuring contact capture") {
			return
		}
		writeJSON(w, http.StatusOK, block)
	})

	mux.HandleFunc("GET /api/v1/me/contacts", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireGrowth(w, options) {
			return
		}
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 200 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 200"})
				return
			}
			limit = parsed
		}
		items, err := options.Growth.Contacts(r.Context(), user.ID, limit)
		if growthProfileError(w, err, "claim a Vuta before viewing contacts") {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"contacts": items})
	})

	mux.HandleFunc("GET /api/v1/me/data-retention", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireGrowth(w, options) {
			return
		}
		settings, err := options.Growth.Settings(r.Context(), user.ID)
		if growthProfileError(w, err, "claim a Vuta before configuring retention") {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	mux.HandleFunc("PUT /api/v1/me/data-retention", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireGrowth(w, options) || !requireJSON(w, r) {
			return
		}
		var input growth.DataSettings
		if err := decodeJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		settings, err := options.Growth.UpdateSettings(r.Context(), user.ID, input, time.Now().UTC())
		if errors.Is(err, growth.ErrInvalidSettings) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "retention must be 30, 90, or 365 days"})
			return
		}
		if growthProfileError(w, err, "claim a Vuta before configuring retention") {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}

func requireGrowth(w http.ResponseWriter, options Options) bool {
	if options.Growth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "growth tools unavailable"})
		return false
	}
	return true
}

func growthProfileError(w http.ResponseWriter, err error, message string) bool {
	if errors.Is(err, growth.ErrProfileRequired) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": message})
		return true
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return true
	}
	return false
}
