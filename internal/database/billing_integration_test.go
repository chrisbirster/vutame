package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/billing"
	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "turso.tech/database/tursogo"
)

func TestBillingRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply Vutame schema on Turso engine: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const userID = "usr_turso_billing"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, email_verified, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
	`, userID, "billing-turso@example.com", now, now); err != nil {
		t.Fatalf("seed Turso billing user: %v", err)
	}

	freeService, err := billing.NewService(db, billing.Config{})
	if err != nil {
		t.Fatalf("open billing service on Turso engine: %v", err)
	}
	freeState, err := freeService.Plan(ctx, userID)
	if err != nil {
		t.Fatalf("load free billing state: %v", err)
	}
	if freeState.Plan != "free" || freeState.BillingConfigured {
		t.Fatalf("unexpected free state: %+v", freeState)
	}
	allowed, err := freeService.Entitled(ctx, userID, billing.FeatureCustomDomains)
	if err != nil || !allowed {
		t.Fatalf("billing-disabled entitlement allowed=%v err=%v", allowed, err)
	}

	configuredService, err := billing.NewService(db, billing.Config{
		StripeSecretKey:     "sk_test",
		StripeWebhookSecret: "whsec_test",
		ProPriceID:           "price_pro",
		SuccessURL:           "https://vutame.example/billing?checkout=success",
		CancelURL:            "https://vutame.example/billing?checkout=cancel",
		PortalReturnURL:      "https://vutame.example/billing",
	})
	if err != nil {
		t.Fatalf("open configured billing service on Turso engine: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO billing_customers (user_id, provider, customer_id, created_at, updated_at)
		VALUES (?, 'stripe', 'cus_turso', ?, ?)
	`, userID, now, now); err != nil {
		t.Fatalf("insert Turso billing customer: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO billing_subscriptions (
			id, user_id, provider, provider_subscription_id, customer_id, price_id, plan,
			status, current_period_end, cancel_at_period_end, created_at, updated_at
		) VALUES (?, ?, 'stripe', ?, ?, ?, 'pro', 'active', ?, 0, ?, ?)
	`, "bill_sub_turso", userID, "sub_turso", "cus_turso", "price_pro", now, now, now); err != nil {
		t.Fatalf("insert Turso billing subscription: %v", err)
	}

	proState, err := configuredService.Plan(ctx, userID)
	if err != nil {
		t.Fatalf("load Pro billing state: %v", err)
	}
	if proState.Plan != "pro" || proState.Subscription == nil || proState.Subscription.ProviderSubscriptionID != "sub_turso" {
		t.Fatalf("unexpected Pro state: %+v", proState)
	}
	allowed, err = configuredService.Entitled(ctx, userID, billing.FeatureAdvancedAnalytics)
	if err != nil || !allowed {
		t.Fatalf("Pro analytics entitlement allowed=%v err=%v", allowed, err)
	}
}
