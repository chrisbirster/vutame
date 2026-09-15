package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/billing"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestBillingHTTPRequiresAuthenticationAndPreservesFreeAccessWhenUnconfigured(t *testing.T) {
	db := openBillingHTTPTestDB(t)
	sender := &httpCaptureSender{}
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(authStore, sender, []byte("0123456789abcdef0123456789abcdef"), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	billingService, err := billing.NewService(db, billing.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profile.NewSeedStore(), Options{Auth: authService, Billing: billingService})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/me/billing", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated billing status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	cookie := signInBillingHTTPTest(t, handler, sender, "billing@example.com")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/billing", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("billing status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var state billing.PlanState
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Plan != "free" || state.BillingConfigured {
		t.Fatalf("billing state=%+v", state)
	}
	if len(state.Entitlements) == 0 {
		t.Fatal("billing-disabled deployment unexpectedly removed candidate feature access")
	}

	request = jsonRequest(t, http.MethodPost, "/api/v1/me/billing/checkout", struct{}{})
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured checkout status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBillingWebhookHTTPRejectsInvalidSignature(t *testing.T) {
	db := openBillingHTTPTestDB(t)
	billingService, err := billing.NewService(db, billing.Config{
		StripeSecretKey:     "sk_test",
		StripeWebhookSecret: "whsec_test",
		ProPriceID:           "price_pro",
		SuccessURL:           "https://vutame.example/billing?checkout=success",
		CancelURL:            "https://vutame.example/billing?checkout=cancel",
		PortalReturnURL:      "https://vutame.example/billing",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profile.NewSeedStore(), Options{Billing: billingService})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/billing/stripe/webhook", http.NoBody)
	request.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("empty webhook status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/billing/stripe/webhook", strings.NewReader(`{"id":"evt_bad","type":"noop","data":{"object":{}}}`))
	request.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid signature status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func openBillingHTTPTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "billing-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func signInBillingHTTPTest(t *testing.T, handler http.Handler, sender *httpCaptureSender, email string) *http.Cookie {
	t.Helper()
	request := jsonRequest(t, http.MethodPost, "/api/v1/auth/code", map[string]string{"email": email})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("code request status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var issued struct {
		ChallengeID string `json:"challenge_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	request = jsonRequest(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{
		"challenge_id": issued.ChallengeID,
		"email":        email,
		"code":         sender.code,
	})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies=%#v", cookies)
	}
	return cookies[0]
}
