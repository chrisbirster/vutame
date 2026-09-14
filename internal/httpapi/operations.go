package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/operations"
)

func registerOperationsRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/me/domains", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) {
			return
		}
		items, err := options.Operations.Domains(r.Context(), user.ID)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusOK, map[string]any{"domains": items})
	})

	mux.HandleFunc("POST /api/v1/me/domains", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) || !requireJSON(w, r) { return }
		var input struct{ Hostname string `json:"hostname"` }
		if decodeJSON(r, &input) != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"invalid request"}); return }
		item, err := options.Operations.AddDomain(r.Context(), user.ID, input.Hostname)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("POST /api/v1/me/domains/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		item, err := options.Operations.VerifyDomain(r.Context(), user.ID, r.PathValue("id"))
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("DELETE /api/v1/me/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		if operationsError(w, options.Operations.DeleteDomain(r.Context(), user.ID, r.PathValue("id"))) { return }
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/me/verification", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		items, err := options.Operations.VerificationRequests(r.Context(), user.ID)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusOK, map[string]any{"requests": items, "policy":"A Vutame verified badge means control of a supported public identity proof, currently a verified custom domain and later an AT Protocol DID."})
	})

	mux.HandleFunc("GET /api/v1/me/export", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		data, err := options.Operations.Export(r.Context(), user.ID)
		if operationsError(w, err) { return }
		w.Header().Set("Content-Disposition", `attachment; filename="vutame-export.json"`)
		writeJSON(w, http.StatusOK, data)
	})

	mux.HandleFunc("GET /api/v1/me/tokens", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		items, err := options.Operations.Tokens(r.Context(), user.ID)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
	})

	mux.HandleFunc("POST /api/v1/me/tokens", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) || !requireJSON(w, r) { return }
		var input struct { Name string `json:"name"`; Scopes []string `json:"scopes"`; ExpiresInDays int `json:"expires_in_days"` }
		if decodeJSON(r, &input) != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"invalid request"}); return }
		item, err := options.Operations.CreateToken(r.Context(), user.ID, input.Name, input.Scopes, input.ExpiresInDays)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("DELETE /api/v1/me/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		if operationsError(w, options.Operations.RevokeToken(r.Context(), user.ID, r.PathValue("id"))) { return }
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/me/webhooks", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		items, err := options.Operations.Webhooks(r.Context(), user.ID)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusOK, map[string]any{"webhooks": items})
	})

	mux.HandleFunc("POST /api/v1/me/webhooks", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) || !requireJSON(w, r) { return }
		var input struct { URL string `json:"url"`; Events []string `json:"events"` }
		if decodeJSON(r, &input) != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"invalid request"}); return }
		item, err := options.Operations.CreateWebhook(r.Context(), user.ID, input.URL, input.Events)
		if operationsError(w, err) { return }
		writeJSON(w, http.StatusCreated, item)
	})

	mux.HandleFunc("POST /api/v1/me/webhooks/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		if operationsError(w, options.Operations.QueueWebhookTest(r.Context(), user.ID, r.PathValue("id"))) { return }
		writeJSON(w, http.StatusAccepted, map[string]string{"status":"queued"})
	})

	mux.HandleFunc("DELETE /api/v1/me/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireOperations(w, options) { return }
		if operationsError(w, options.Operations.DeleteWebhook(r.Context(), user.ID, r.PathValue("id"))) { return }
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/token/profile", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireBearerScope(w, r, options, operations.ScopeProfileRead)
		if !ok { return }
		if options.Editor == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error":"profile unavailable"}); return }
		item, err := options.Editor.GetOwned(r.Context(), userID)
		if err != nil { writeJSON(w, http.StatusNotFound, map[string]string{"error":"profile not found"}); return }
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/token/analytics", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireBearerScope(w, r, options, operations.ScopeAnalyticsRead)
		if !ok { return }
		if options.Analytics == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error":"analytics unavailable"}); return }
		days := analytics.DefaultDashboardDays
		if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
			parsed, err := strconv.Atoi(raw); if err != nil || parsed < 1 || parsed > analytics.MaxDashboardDays { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"days must be between 1 and 90"}); return }; days = parsed
		}
		item, err := options.Analytics.Dashboard(r.Context(), userID, days, time.Now().UTC())
		if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"analytics unavailable"}); return }
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("GET /api/v1/token/contacts", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireBearerScope(w, r, options, operations.ScopeContactsRead)
		if !ok { return }
		if options.Growth == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error":"contacts unavailable"}); return }
		items, err := options.Growth.Contacts(r.Context(), userID, 200)
		if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"contacts unavailable"}); return }
		writeJSON(w, http.StatusOK, map[string]any{"contacts":items})
	})
}

func requireOperations(w http.ResponseWriter, options Options) bool {
	if options.Operations == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error":"creator operations unavailable"})
		return false
	}
	return true
}

func requireBearerScope(w http.ResponseWriter, r *http.Request, options Options, scope string) (string, bool) {
	if !requireOperations(w, options) { return "", false }
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") { writeJSON(w, http.StatusUnauthorized, map[string]string{"error":"bearer token required"}); return "", false }
	userID, err := options.Operations.AuthenticateToken(r.Context(), strings.TrimSpace(header[len("Bearer "):]), scope)
	if err != nil { writeJSON(w, http.StatusUnauthorized, map[string]string{"error":"invalid or insufficient token"}); return "", false }
	return userID, true
}

func operationsError(w http.ResponseWriter, err error) bool {
	if err == nil { return false }
	switch {
	case errors.Is(err, operations.ErrProfileRequired): writeJSON(w, http.StatusConflict, map[string]string{"error":"claim a Vuta first"})
	case errors.Is(err, operations.ErrVerificationPending): writeJSON(w, http.StatusConflict, map[string]string{"error":"DNS verification record not found yet"})
	case errors.Is(err, operations.ErrDomainTaken): writeJSON(w, http.StatusConflict, map[string]string{"error":"domain already claimed"})
	case errors.Is(err, operations.ErrInvalidDomain), errors.Is(err, operations.ErrInvalidScope), errors.Is(err, operations.ErrInvalidWebhook): writeJSON(w, http.StatusBadRequest, map[string]string{"error":err.Error()})
	case errors.Is(err, operations.ErrDomainNotFound), errors.Is(err, operations.ErrTokenUnauthorized): writeJSON(w, http.StatusNotFound, map[string]string{"error":"not found"})
	default: writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"internal server error"})
	}
	return true
}
