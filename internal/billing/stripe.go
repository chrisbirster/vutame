package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxStripeResponse = 1 << 20

type URLResponse struct {
	URL string `json:"url"`
}

type stripeIDResponse struct {
	ID  string `json:"id"`
	URL string `json:"url,omitempty"`
}

func (s *Service) Checkout(ctx context.Context, userID string) (URLResponse, error) {
	if !s.Configured() {
		return URLResponse{}, ErrUnavailable
	}
	if err := s.requireUser(ctx, userID); err != nil {
		return URLResponse{}, err
	}
	customerID, err := s.customerForUser(ctx, userID)
	if err != nil {
		return URLResponse{}, err
	}
	form := url.Values{
		"mode":                             {"subscription"},
		"customer":                         {customerID},
		"success_url":                      {s.config.SuccessURL},
		"cancel_url":                       {s.config.CancelURL},
		"client_reference_id":              {userID},
		"line_items[0][price]":             {s.config.ProPriceID},
		"line_items[0][quantity]":          {"1"},
		"metadata[user_id]":                {userID},
		"subscription_data[metadata][user_id]": {userID},
	}
	var response stripeIDResponse
	if err := s.stripePostForm(ctx, "/v1/checkout/sessions", form, &response); err != nil {
		return URLResponse{}, err
	}
	if strings.TrimSpace(response.URL) == "" {
		return URLResponse{}, fmt.Errorf("billing: Stripe Checkout response missing URL")
	}
	return URLResponse{URL: response.URL}, nil
}

func (s *Service) Portal(ctx context.Context, userID string) (URLResponse, error) {
	if !s.Configured() {
		return URLResponse{}, ErrUnavailable
	}
	var customerID string
	if err := s.db.QueryRowContext(ctx, `SELECT customer_id FROM billing_customers WHERE user_id=?`, strings.TrimSpace(userID)).Scan(&customerID); err != nil {
		if errors.Is(err, sqlErrNoRows) {
			return URLResponse{}, ErrNoCustomer
		}
		return URLResponse{}, err
	}
	form := url.Values{"customer": {customerID}, "return_url": {s.config.PortalReturnURL}}
	var response stripeIDResponse
	if err := s.stripePostForm(ctx, "/v1/billing_portal/sessions", form, &response); err != nil {
		return URLResponse{}, err
	}
	if strings.TrimSpace(response.URL) == "" {
		return URLResponse{}, fmt.Errorf("billing: Stripe portal response missing URL")
	}
	return URLResponse{URL: response.URL}, nil
}

func (s *Service) customerForUser(ctx context.Context, userID string) (string, error) {
	s.customerMu.Lock()
	defer s.customerMu.Unlock()

	var customerID string
	err := s.db.QueryRowContext(ctx, `SELECT customer_id FROM billing_customers WHERE user_id=?`, userID).Scan(&customerID)
	if err == nil {
		return customerID, nil
	}
	if !errors.Is(err, sqlErrNoRows) {
		return "", err
	}
	var email string
	if err := s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id=?`, userID).Scan(&email); err != nil {
		return "", err
	}
	form := url.Values{"email": {email}, "metadata[user_id]": {userID}}
	var response stripeIDResponse
	if err := s.stripePostForm(ctx, "/v1/customers", form, &response); err != nil {
		return "", err
	}
	customerID = strings.TrimSpace(response.ID)
	if customerID == "" {
		return "", fmt.Errorf("billing: Stripe customer response missing ID")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO billing_customers(user_id,provider,customer_id,created_at,updated_at)
		VALUES(?,'stripe',?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET customer_id=excluded.customer_id,updated_at=excluded.updated_at
	`, userID, customerID, now, now); err != nil {
		return "", err
	}
	return customerID, nil
}

func (s *Service) stripePostForm(ctx context.Context, path string, form url.Values, target any) error {
	if !s.Configured() {
		return ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.StripeAPIBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.config.StripeSecretKey)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if key, err := randomID("req_", 18); err == nil {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("billing: Stripe request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxStripeResponse+1))
	if err != nil {
		return err
	}
	if len(data) > maxStripeResponse {
		return fmt.Errorf("billing: Stripe response too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error struct { Message string `json:"message"` } `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		message := strings.TrimSpace(payload.Error.Message)
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("billing: Stripe request failed: %s", message)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("billing: decode Stripe response: %w", err)
	}
	return nil
}

var sqlErrNoRows = errors.New("sql: no rows in result set")
