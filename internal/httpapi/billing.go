package httpapi

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/chrisbirster/vutame/internal/billing"
)

const maxBillingWebhookBody = 1 << 20

func registerBillingRoutes(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/v1/me/billing", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireBilling(w, options) {
			return
		}
		state, err := options.Billing.Plan(r.Context(), user.ID)
		if billingError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, state)
	})

	mux.HandleFunc("POST /api/v1/me/billing/checkout", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireBilling(w, options) || !allowRate(w, options, "billing-checkout:"+user.ID, 10, time.Hour) {
			return
		}
		item, err := options.Billing.Checkout(r.Context(), user.ID)
		if billingError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/me/billing/portal", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok || !requireBilling(w, options) || !allowRate(w, options, "billing-portal:"+user.ID, 20, time.Hour) {
			return
		}
		item, err := options.Billing.Portal(r.Context(), user.ID)
		if billingError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("POST /api/v1/billing/stripe/webhook", func(w http.ResponseWriter, r *http.Request) {
		if !requireBilling(w, options) {
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBillingWebhookBody+1))
		if err != nil || len(body) == 0 || len(body) > maxBillingWebhookBody {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook body"})
			return
		}
		result, err := options.Billing.HandleStripeWebhook(r.Context(), body, r.Header.Get("Stripe-Signature"))
		if billingError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "duplicate": result.Duplicate})
	})
}

func requireBilling(w http.ResponseWriter, options Options) bool {
	if options.Billing == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "billing unavailable"})
		return false
	}
	return true
}

func billingError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, billing.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "paid upgrades are not configured"})
	case errors.Is(err, billing.ErrNoCustomer):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "start a subscription before opening billing management"})
	case errors.Is(err, billing.ErrInvalidWebhook), errors.Is(err, billing.ErrInvalidEvent):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid billing webhook"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "billing request failed"})
	}
	return true
}
