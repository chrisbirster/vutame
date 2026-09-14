package operations

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

type fakeTXTResolver struct{ values map[string][]string }
func (r fakeTXTResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	values, ok := r.values[name]
	if !ok { return nil, errors.New("not found") }
	return values, nil
}

func openOperationsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)
	t.Cleanup(func(){ _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("schema: %v", err) }
	now := "2026-09-14T12:00:00Z"
	if _, err := db.Exec(`INSERT INTO users (id,email,email_verified,created_at,updated_at) VALUES ('usr_ops','ops@example.com',1,?,?)`, now, now); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO profiles (user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES ('usr_ops','ops','Ops','','','midnight',0,?,?)`, now, now); err != nil { t.Fatal(err) }
	return db
}

func TestDomainTokenWebhookAndExportLifecycle(t *testing.T) {
	db := openOperationsDB(t)
	store, err := NewStore(db, []byte(strings.Repeat("s", 32)))
	if err != nil { t.Fatal(err) }
	fixed := time.Date(2026,9,14,12,0,0,0,time.UTC)
	store.SetNow(func() time.Time { return fixed })

	domain, err := store.AddDomain(context.Background(), "usr_ops", "creator.example.com")
	if err != nil { t.Fatal(err) }
	if domain.DNSName != "_vutame.creator.example.com" || domain.DNSValue == "" { t.Fatalf("domain=%#v", domain) }
	if _, err := store.VerifyDomain(context.Background(), "usr_ops", domain.ID); !errors.Is(err, ErrVerificationPending) { t.Fatalf("pending err=%v", err) }
	store.SetTXTResolver(fakeTXTResolver{values: map[string][]string{domain.DNSName:{domain.DNSValue}}})
	verified, err := store.VerifyDomain(context.Background(), "usr_ops", domain.ID)
	if err != nil { t.Fatal(err) }
	if verified.VerifiedAt == "" { t.Fatal("expected verified timestamp") }
	handle, err := store.HandleForHost(context.Background(), "creator.example.com:443")
	if err != nil || handle != "ops" { t.Fatalf("handle=%q err=%v", handle, err) }
	canonical, err := store.CanonicalDomain(context.Background(), "usr_ops")
	if err != nil || canonical != "creator.example.com" { t.Fatalf("canonical=%q err=%v", canonical, err) }
	var profileVerified int
	if err := db.QueryRow(`SELECT verified FROM profiles WHERE user_id='usr_ops'`).Scan(&profileVerified); err != nil || profileVerified != 1 { t.Fatalf("verified=%d err=%v", profileVerified, err) }

	created, err := store.CreateToken(context.Background(), "usr_ops", "automation", []string{ScopeAnalyticsRead, ScopeProfileRead}, 30)
	if err != nil { t.Fatal(err) }
	if created.Token == "" || !strings.HasPrefix(created.Token, "vut_") { t.Fatalf("token=%#v", created) }
	if _, err := store.AuthenticateToken(context.Background(), created.Token, ScopeAnalyticsRead); err != nil { t.Fatal(err) }
	if _, err := store.AuthenticateToken(context.Background(), created.Token, ScopeContactsRead); !errors.Is(err, ErrTokenUnauthorized) { t.Fatalf("scope err=%v", err) }
	if err := store.RevokeToken(context.Background(), "usr_ops", created.ID); err != nil { t.Fatal(err) }
	if _, err := store.AuthenticateToken(context.Background(), created.Token, ScopeProfileRead); !errors.Is(err, ErrTokenUnauthorized) { t.Fatalf("revoked err=%v", err) }

	webhook, err := store.CreateWebhook(context.Background(), "usr_ops", "https://hooks.example.com/vutame", []string{"profile.updated"})
	if err != nil { t.Fatal(err) }
	if webhook.SigningSecret == "" { t.Fatal("expected one-time signing secret") }
	if err := store.QueueWebhookEvent(context.Background(), "usr_ops", "profile.updated", `{"event":"profile.updated"}`); err != nil { t.Fatal(err) }
	count, err := store.DeliveryCount(context.Background(), "usr_ops")
	if err != nil || count != 1 { t.Fatalf("deliveries=%d err=%v", count, err) }

	export, err := store.Export(context.Background(), "usr_ops")
	if err != nil { t.Fatal(err) }
	if export.Profile["handle"] != "ops" || len(export.Domains) != 1 || len(export.Webhooks) != 1 { t.Fatalf("export=%#v", export) }
}

func TestValidationRejectsUnsafeOperations(t *testing.T) {
	for _, value := range []string{"localhost", "127.0.0.1", "vuta.me", "bad host", "one-label"} {
		if _, err := NormalizeHostname(value); err == nil { t.Fatalf("expected invalid domain %q", value) }
	}
	for _, value := range []string{"http://hooks.example.com", "https://localhost/hook", "https://127.0.0.1/hook", "https://example.com:8443/hook"} {
		if _, err := ValidateWebhookURL(value); err == nil { t.Fatalf("expected invalid webhook %q", value) }
	}
	if _, err := NormalizeScopes([]string{"admin:*"}); !errors.Is(err, ErrInvalidScope) { t.Fatalf("scope err=%v", err) }
}
