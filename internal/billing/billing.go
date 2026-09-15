package billing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable    = errors.New("billing provider unavailable")
	ErrNoCustomer     = errors.New("billing customer not found")
	ErrInvalidWebhook = errors.New("invalid billing webhook")
	ErrInvalidEvent   = errors.New("invalid billing event")
)

type Feature string

const (
	FeatureCustomDomains     Feature = "custom_domains"
	FeatureAdvancedAnalytics Feature = "advanced_analytics"
	FeaturePremiumThemes     Feature = "premium_themes"
	FeatureBrandingRemoval   Feature = "branding_removal"
	FeatureAPIIntegrations   Feature = "api_integrations"
)

var proFeatures = []Feature{
	FeatureCustomDomains,
	FeatureAdvancedAnalytics,
	FeaturePremiumThemes,
	FeatureBrandingRemoval,
	FeatureAPIIntegrations,
}

type Config struct {
	StripeSecretKey     string
	StripeWebhookSecret string
	ProPriceID           string
	SuccessURL           string
	CancelURL            string
	PortalReturnURL      string
	StripeAPIBase        string
	HTTPClient           *http.Client
}

type Service struct {
	db         *sql.DB
	config     Config
	client     *http.Client
	now        func() time.Time
	customerMu sync.Mutex
}

type Subscription struct {
	ID                     string `json:"id"`
	ProviderSubscriptionID string `json:"provider_subscription_id"`
	CustomerID             string `json:"customer_id"`
	PriceID                string `json:"price_id"`
	Plan                   string `json:"plan"`
	Status                 string `json:"status"`
	CurrentPeriodEnd       string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd      bool   `json:"cancel_at_period_end"`
	UpdatedAt              string `json:"updated_at"`
}

type PlanState struct {
	Plan              string        `json:"plan"`
	Subscription      *Subscription `json:"subscription,omitempty"`
	Entitlements      []string      `json:"entitlements"`
	BillingConfigured bool          `json:"billing_configured"`
}

func NewService(db *sql.DB, config Config) (*Service, error) {
	if db == nil {
		return nil, errors.New("billing: database is required")
	}
	for _, table := range []string{"users", "billing_customers", "billing_subscriptions", "billing_events", "entitlement_grants"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify billing table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("billing schema missing table %s; run Atlas schema apply", table)
		}
	}

	config.StripeSecretKey = strings.TrimSpace(config.StripeSecretKey)
	config.StripeWebhookSecret = strings.TrimSpace(config.StripeWebhookSecret)
	config.ProPriceID = strings.TrimSpace(config.ProPriceID)
	config.SuccessURL = strings.TrimSpace(config.SuccessURL)
	config.CancelURL = strings.TrimSpace(config.CancelURL)
	config.PortalReturnURL = strings.TrimSpace(config.PortalReturnURL)
	config.StripeAPIBase = strings.TrimRight(strings.TrimSpace(config.StripeAPIBase), "/")
	if config.StripeAPIBase == "" {
		config.StripeAPIBase = "https://api.stripe.com"
	}

	providerValues := []string{config.StripeSecretKey, config.StripeWebhookSecret, config.ProPriceID}
	configured := 0
	for _, value := range providerValues {
		if value != "" {
			configured++
		}
	}
	if configured != 0 && configured != len(providerValues) {
		return nil, errors.New("billing: Stripe secret key, webhook secret, and Pro price ID must be configured together")
	}
	if configured == len(providerValues) && (config.SuccessURL == "" || config.CancelURL == "" || config.PortalReturnURL == "") {
		return nil, errors.New("billing: checkout success/cancel and portal return URLs are required when Stripe is configured")
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Service{db: db, config: config, client: client, now: time.Now}, nil
}

func (s *Service) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Service) Configured() bool {
	return s != nil && s.config.StripeSecretKey != "" && s.config.StripeWebhookSecret != "" && s.config.ProPriceID != ""
}

func (s *Service) Plan(ctx context.Context, userID string) (PlanState, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireUser(ctx, userID); err != nil {
		return PlanState{}, err
	}
	state := PlanState{Plan: "free", Entitlements: []string{}, BillingConfigured: s.Configured()}

	var item Subscription
	var cancel int
	err := s.db.QueryRowContext(ctx, `
		SELECT id,provider_subscription_id,customer_id,price_id,plan,status,
		       COALESCE(current_period_end,''),cancel_at_period_end,updated_at
		FROM billing_subscriptions
		WHERE user_id=?
		ORDER BY CASE WHEN status IN ('active','trialing') THEN 0 ELSE 1 END, updated_at DESC
		LIMIT 1
	`, userID).Scan(
		&item.ID, &item.ProviderSubscriptionID, &item.CustomerID, &item.PriceID, &item.Plan,
		&item.Status, &item.CurrentPeriodEnd, &cancel, &item.UpdatedAt,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PlanState{}, err
	}
	if err == nil {
		item.CancelAtPeriodEnd = cancel == 1
		state.Subscription = &item
		if (item.Status == "active" || item.Status == "trialing") && item.PriceID == s.config.ProPriceID && s.config.ProPriceID != "" {
			state.Plan = "pro"
		}
	}

	entitled := make(map[Feature]bool)
	// When billing is not configured, candidate paid features remain unlocked.
	// This preserves local/self-hosted and pre-M6 behavior rather than turning a
	// missing provider configuration into an accidental feature outage.
	if !s.Configured() || state.Plan == "pro" {
		for _, feature := range proFeatures {
			entitled[feature] = true
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT feature FROM entitlement_grants
		WHERE user_id=? AND (expires_at IS NULL OR expires_at='' OR expires_at>?)
		ORDER BY feature
	`, userID, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return PlanState{}, err
	}
	for rows.Next() {
		var feature string
		if err := rows.Scan(&feature); err != nil {
			_ = rows.Close()
			return PlanState{}, err
		}
		entitled[Feature(feature)] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return PlanState{}, err
	}
	_ = rows.Close()

	for _, feature := range proFeatures {
		if entitled[feature] {
			state.Entitlements = append(state.Entitlements, string(feature))
		}
	}
	return state, nil
}

func (s *Service) Entitled(ctx context.Context, userID string, feature Feature) (bool, error) {
	state, err := s.Plan(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, value := range state.Entitlements {
		if value == string(feature) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) requireUser(ctx context.Context, userID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=?`, strings.TrimSpace(userID)).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func randomID(prefix string, size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}
