package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const webhookTolerance = 5 * time.Minute

type WebhookResult struct {
	Duplicate bool `json:"duplicate"`
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type stripeCheckoutSession struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	ClientReferenceID string            `json:"client_reference_id"`
	Metadata          map[string]string `json:"metadata"`
}

type stripeSubscription struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	Metadata          map[string]string `json:"metadata"`
	Items             struct {
		Data []struct {
			CurrentPeriodEnd int64 `json:"current_period_end"`
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

func (s *Service) HandleStripeWebhook(ctx context.Context, rawBody []byte, signatureHeader string) (WebhookResult, error) {
	if !s.Configured() {
		return WebhookResult{}, ErrUnavailable
	}
	if len(rawBody) == 0 || len(rawBody) > maxStripeResponse {
		return WebhookResult{}, ErrInvalidWebhook
	}
	if err := s.verifyStripeSignature(rawBody, signatureHeader); err != nil {
		return WebhookResult{}, err
	}
	var event stripeEvent
	if err := json.Unmarshal(rawBody, &event); err != nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" {
		return WebhookResult{}, ErrInvalidEvent
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WebhookResult{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM billing_events WHERE provider_event_id=?`, event.ID).Scan(&exists); err != nil {
		return WebhookResult{}, err
	}
	if exists > 0 {
		return WebhookResult{Duplicate: true}, nil
	}

	switch event.Type {
	case "checkout.session.completed":
		if err := s.processCheckoutCompleted(ctx, tx, event.Data.Object); err != nil {
			return WebhookResult{}, err
		}
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		if err := s.processSubscriptionEvent(ctx, tx, event.Data.Object); err != nil {
			return WebhookResult{}, err
		}
	}

	digest := sha256.Sum256(rawBody)
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO billing_events(provider_event_id,provider,event_type,payload_hash,received_at,processed_at)
		VALUES(?,'stripe',?,?,?,?)
	`, event.ID, event.Type, hex.EncodeToString(digest[:]), now, now); err != nil {
		return WebhookResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return WebhookResult{}, err
	}
	return WebhookResult{}, nil
}

func (s *Service) verifyStripeSignature(body []byte, header string) error {
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				timestamp = parsed
			}
		case "v1":
			signatures = append(signatures, value)
		}
	}
	if timestamp <= 0 || len(signatures) == 0 {
		return ErrInvalidWebhook
	}
	now := s.now().UTC()
	signedAt := time.Unix(timestamp, 0).UTC()
	age := now.Sub(signedAt)
	if age < -webhookTolerance || age > webhookTolerance {
		return ErrInvalidWebhook
	}
	message := strconv.FormatInt(timestamp, 10) + "." + string(body)
	mac := hmac.New(sha256.New, []byte(s.config.StripeWebhookSecret))
	_, _ = mac.Write([]byte(message))
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		candidate, err := hex.DecodeString(strings.TrimSpace(signature))
		if err == nil && len(candidate) == len(expected) && subtle.ConstantTimeCompare(candidate, expected) == 1 {
			return nil
		}
	}
	return ErrInvalidWebhook
}

func (s *Service) processCheckoutCompleted(ctx context.Context, tx *sql.Tx, raw json.RawMessage) error {
	var session stripeCheckoutSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return ErrInvalidEvent
	}
	customerID := strings.TrimSpace(session.Customer)
	if customerID == "" {
		return nil
	}
	userID := strings.TrimSpace(session.ClientReferenceID)
	if userID == "" {
		userID = strings.TrimSpace(session.Metadata["user_id"])
	}
	if userID == "" || !txUserExists(ctx, tx, userID) {
		return nil
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO billing_customers(user_id,provider,customer_id,created_at,updated_at)
		VALUES(?,'stripe',?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET customer_id=excluded.customer_id,updated_at=excluded.updated_at
	`, userID, customerID, now, now)
	return err
}

func (s *Service) processSubscriptionEvent(ctx context.Context, tx *sql.Tx, raw json.RawMessage) error {
	var subscription stripeSubscription
	if err := json.Unmarshal(raw, &subscription); err != nil {
		return ErrInvalidEvent
	}
	subscription.ID = strings.TrimSpace(subscription.ID)
	subscription.Customer = strings.TrimSpace(subscription.Customer)
	if subscription.ID == "" || subscription.Customer == "" {
		return ErrInvalidEvent
	}
	priceID := ""
	periodEnd := subscription.CurrentPeriodEnd
	if len(subscription.Items.Data) > 0 {
		priceID = strings.TrimSpace(subscription.Items.Data[0].Price.ID)
		if periodEnd == 0 {
			periodEnd = subscription.Items.Data[0].CurrentPeriodEnd
		}
	}
	// Ledger unrelated/unknown Stripe products, but never grant Vutame Pro.
	if priceID == "" || priceID != s.config.ProPriceID {
		return nil
	}
	if !validSubscriptionStatus(subscription.Status) {
		return nil
	}

	userID := strings.TrimSpace(subscription.Metadata["user_id"])
	if userID == "" {
		_ = tx.QueryRowContext(ctx, `SELECT user_id FROM billing_customers WHERE customer_id=?`, subscription.Customer).Scan(&userID)
	}
	if userID == "" || !txUserExists(ctx, tx, userID) {
		return nil
	}
	now := s.now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO billing_customers(user_id,provider,customer_id,created_at,updated_at)
		VALUES(?,'stripe',?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET customer_id=excluded.customer_id,updated_at=excluded.updated_at
	`, userID, subscription.Customer, stamp, stamp); err != nil {
		return err
	}
	period := any(nil)
	if periodEnd > 0 {
		period = time.Unix(periodEnd, 0).UTC().Format(time.RFC3339Nano)
	}
	cancel := 0
	if subscription.CancelAtPeriodEnd {
		cancel = 1
	}
	internalID, err := randomID("bsub_", 18)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO billing_subscriptions(
			id,user_id,provider,provider_subscription_id,customer_id,price_id,plan,status,
			current_period_end,cancel_at_period_end,created_at,updated_at
		) VALUES(?,?,'stripe',?,?,?,'pro',?,?,?,?,?)
		ON CONFLICT(provider_subscription_id) DO UPDATE SET
			user_id=excluded.user_id,customer_id=excluded.customer_id,price_id=excluded.price_id,
			plan='pro',status=excluded.status,current_period_end=excluded.current_period_end,
			cancel_at_period_end=excluded.cancel_at_period_end,updated_at=excluded.updated_at
	`, internalID, userID, subscription.ID, subscription.Customer, priceID, subscription.Status, period, cancel, stamp, stamp)
	return err
}

func txUserExists(ctx context.Context, tx *sql.Tx, userID string) bool {
	var count int
	return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=?`, userID).Scan(&count) == nil && count == 1
}

func validSubscriptionStatus(value string) bool {
	switch value {
	case "incomplete", "incomplete_expired", "trialing", "active", "past_due", "canceled", "unpaid", "paused":
		return true
	default:
		return false
	}
}

var _ = errors.Is
var _ = fmt.Sprintf
