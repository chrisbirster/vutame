package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestUnconfiguredBillingKeepsCandidateFeaturesUnlocked(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_free", "free@example.com")
	service, err := NewService(db, Config{})
	if err != nil { t.Fatal(err) }
	state, err := service.Plan(context.Background(), "usr_free")
	if err != nil { t.Fatal(err) }
	if state.Plan != "free" || state.BillingConfigured {
		t.Fatalf("unexpected state: %+v", state)
	}
	for _, feature := range []Feature{FeatureCustomDomains, FeatureAdvancedAnalytics} {
		allowed, err := service.Entitled(context.Background(), "usr_free", feature)
		if err != nil || !allowed {
			t.Fatalf("unconfigured feature %s allowed=%v err=%v", feature, allowed, err)
		}
	}
	if _, err := service.Checkout(context.Background(), "usr_free"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("checkout error=%v want ErrUnavailable", err)
	}
}

func TestStripeWebhookActivatesProAndDeduplicates(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_pro", "pro@example.com")
	now := time.Date(2026, 9, 15, 3, 0, 0, 0, time.UTC)
	service := newConfiguredBillingService(t, db, now, nil)

	body := []byte(`{"id":"evt_pro","type":"customer.subscription.updated","data":{"object":{"id":"sub_123","customer":"cus_123","status":"active","cancel_at_period_end":false,"metadata":{"user_id":"usr_pro"},"items":{"data":[{"price":{"id":"price_pro"},"current_period_end":1790000000}]}}}}`)
	header := stripeTestSignature("whsec_test", now, body)
	result, err := service.HandleStripeWebhook(context.Background(), body, header)
	if err != nil { t.Fatalf("HandleStripeWebhook: %v", err) }
	if result.Duplicate { t.Fatal("first event marked duplicate") }

	state, err := service.Plan(context.Background(), "usr_pro")
	if err != nil { t.Fatal(err) }
	if state.Plan != "pro" || state.Subscription == nil || state.Subscription.ProviderSubscriptionID != "sub_123" {
		t.Fatalf("unexpected Pro state: %+v", state)
	}
	if state.Subscription.CurrentPeriodEnd == "" {
		t.Fatal("item-level current_period_end was not persisted")
	}

	result, err = service.HandleStripeWebhook(context.Background(), body, header)
	if err != nil { t.Fatal(err) }
	if !result.Duplicate { t.Fatal("duplicate event was not detected") }
	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_events WHERE provider_event_id='evt_pro'`).Scan(&eventCount); err != nil { t.Fatal(err) }
	if eventCount != 1 { t.Fatalf("event count=%d", eventCount) }
	var payloadHash string
	if err := db.QueryRow(`SELECT payload_hash FROM billing_events WHERE provider_event_id='evt_pro'`).Scan(&payloadHash); err != nil { t.Fatal(err) }
	if payloadHash == "" || strings.Contains(payloadHash, "sub_123") { t.Fatalf("unexpected payload hash %q", payloadHash) }
}

func TestStripeWebhookUnknownPriceFailsClosed(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_other", "other@example.com")
	now := time.Date(2026, 9, 15, 3, 30, 0, 0, time.UTC)
	service := newConfiguredBillingService(t, db, now, nil)
	body := []byte(`{"id":"evt_other","type":"customer.subscription.created","data":{"object":{"id":"sub_other","customer":"cus_other","status":"active","metadata":{"user_id":"usr_other"},"items":{"data":[{"price":{"id":"price_not_vutame"}}]}}}}`)
	if _, err := service.HandleStripeWebhook(context.Background(), body, stripeTestSignature("whsec_test", now, body)); err != nil { t.Fatal(err) }
	state, err := service.Plan(context.Background(), "usr_other")
	if err != nil { t.Fatal(err) }
	if state.Plan != "free" || state.Subscription != nil { t.Fatalf("unknown price granted access: %+v", state) }
	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_events WHERE provider_event_id='evt_other'`).Scan(&eventCount); err != nil { t.Fatal(err) }
	if eventCount != 1 { t.Fatalf("unknown-price event was not ledgered") }
}

func TestStripeWebhookRejectsBadOrStaleSignatures(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_sig", "sig@example.com")
	now := time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)
	service := newConfiguredBillingService(t, db, now, nil)
	body := []byte(`{"id":"evt_sig","type":"noop","data":{"object":{}}}`)
	if _, err := service.HandleStripeWebhook(context.Background(), body, "t=1,v1=deadbeef"); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("bad signature error=%v", err)
	}
	stale := stripeTestSignature("whsec_test", now.Add(-10*time.Minute), body)
	if _, err := service.HandleStripeWebhook(context.Background(), body, stale); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("stale signature error=%v", err)
	}
}

func TestManualEntitlementGrantOverridesFreePlan(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_grant", "grant@example.com")
	now := time.Date(2026, 9, 15, 4, 30, 0, 0, time.UTC)
	service := newConfiguredBillingService(t, db, now, nil)
	if _, err := db.Exec(`INSERT INTO entitlement_grants(user_id,feature,source,expires_at,note,created_at) VALUES(?,?,'migration',NULL,'legacy custom domain',?)`, "usr_grant", string(FeatureCustomDomains), now.Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }
	custom, err := service.Entitled(context.Background(), "usr_grant", FeatureCustomDomains)
	if err != nil || !custom { t.Fatalf("custom grant allowed=%v err=%v", custom, err) }
	advanced, err := service.Entitled(context.Background(), "usr_grant", FeatureAdvancedAnalytics)
	if err != nil { t.Fatal(err) }
	if advanced { t.Fatal("manual custom-domain grant leaked to advanced analytics") }
}

func TestCheckoutAndPortalUseHostedStripeFlows(t *testing.T) {
	db := newBillingTestDB(t)
	insertBillingUser(t, db, "usr_checkout", "checkout@example.com")
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		calls = append(calls, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer sk_test" || r.Header.Get("Idempotency-Key") == "" {
			t.Fatalf("missing Stripe auth/idempotency headers")
		}
		switch r.URL.Path {
		case "/v1/customers":
			if form.Get("email") != "checkout@example.com" || form.Get("metadata[user_id]") != "usr_checkout" { t.Fatalf("customer form=%v", form) }
			writeBillingJSON(t, w, map[string]any{"id":"cus_checkout"})
		case "/v1/checkout/sessions":
			if form.Get("mode") != "subscription" || form.Get("customer") != "cus_checkout" || form.Get("line_items[0][price]") != "price_pro" || form.Get("client_reference_id") != "usr_checkout" { t.Fatalf("checkout form=%v", form) }
			writeBillingJSON(t, w, map[string]any{"id":"cs_1","url":"https://checkout.stripe.example/session"})
		case "/v1/billing_portal/sessions":
			if form.Get("customer") != "cus_checkout" || form.Get("return_url") != "https://vutame.example/billing" { t.Fatalf("portal form=%v", form) }
			writeBillingJSON(t, w, map[string]any{"id":"bps_1","url":"https://billing.stripe.example/session"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := time.Date(2026, 9, 15, 5, 0, 0, 0, time.UTC)
	service := newConfiguredBillingService(t, db, now, server.Client())
	service.config.StripeAPIBase = server.URL
	checkout, err := service.Checkout(context.Background(), "usr_checkout")
	if err != nil { t.Fatal(err) }
	if checkout.URL != "https://checkout.stripe.example/session" { t.Fatalf("checkout URL=%q", checkout.URL) }
	portal, err := service.Portal(context.Background(), "usr_checkout")
	if err != nil { t.Fatal(err) }
	if portal.URL != "https://billing.stripe.example/session" { t.Fatalf("portal URL=%q", portal.URL) }
	if strings.Join(calls, ",") != "/v1/customers,/v1/checkout/sessions,/v1/billing_portal/sessions" { t.Fatalf("calls=%v", calls) }
}

func newBillingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "billing.db"))
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("apply schema: %v", err) }
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertBillingUser(t *testing.T, db *sql.DB, id, email string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES(?,?,1,?,?)`, id, email, now, now); err != nil { t.Fatal(err) }
}

func newConfiguredBillingService(t *testing.T, db *sql.DB, now time.Time, client *http.Client) *Service {
	t.Helper()
	service, err := NewService(db, Config{
		StripeSecretKey: "sk_test", StripeWebhookSecret: "whsec_test", ProPriceID: "price_pro",
		SuccessURL: "https://vutame.example/billing?checkout=success", CancelURL: "https://vutame.example/billing?checkout=cancel",
		PortalReturnURL: "https://vutame.example/billing", HTTPClient: client,
	})
	if err != nil { t.Fatal(err) }
	service.SetNow(func() time.Time { return now })
	return service
}

func stripeTestSignature(secret string, at time.Time, body []byte) string {
	timestamp := strconvFormatInt(at.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	return "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func strconvFormatInt(value int64) string {
	return time.Unix(value, 0).UTC().Format("1136239445")
}

func writeBillingJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil { t.Fatal(err) }
}
