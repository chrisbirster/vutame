package database_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/operations"
	_ "turso.tech/database/tursogo"
)

type tursoTXTResolver struct{ name, value string }
func (r tursoTXTResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if name == r.name { return []string{r.value}, nil }
	return nil, nil
}

func TestCreatorOperationsRunOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("schema: %v", err) }
	now := "2026-09-14T12:00:00Z"
	if _, err := db.Exec(`INSERT INTO users (id,email,email_verified,created_at,updated_at) VALUES ('usr_turso_ops','turso-ops@example.com',1,?,?)`, now, now); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO profiles (user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES ('usr_turso_ops','turso-ops','Turso Ops','','','midnight',0,?,?)`, now, now); err != nil { t.Fatal(err) }
	store, err := operations.NewStore(db, []byte(strings.Repeat("t", 32))); if err != nil { t.Fatal(err) }
	domain, err := store.AddDomain(context.Background(), "usr_turso_ops", "turso.example.com"); if err != nil { t.Fatal(err) }
	store.SetTXTResolver(tursoTXTResolver{name: domain.DNSName, value: domain.DNSValue})
	if _, err := store.VerifyDomain(context.Background(), "usr_turso_ops", domain.ID); err != nil { t.Fatal(err) }
	token, err := store.CreateToken(context.Background(), "usr_turso_ops", "ci", []string{operations.ScopeProfileRead}, 30); if err != nil { t.Fatal(err) }
	if _, err := store.AuthenticateToken(context.Background(), token.Token, operations.ScopeProfileRead); err != nil { t.Fatal(err) }
	webhook, err := store.CreateWebhook(context.Background(), "usr_turso_ops", "https://hooks.example.com/vutame", []string{"profile.updated"}); if err != nil { t.Fatal(err) }
	if webhook.SigningSecret == "" { t.Fatal("missing signing secret") }
	if err := store.QueueWebhookEvent(context.Background(), "usr_turso_ops", "profile.updated", `{"event":"profile.updated"}`); err != nil { t.Fatal(err) }
	count, err := store.DeliveryCount(context.Background(), "usr_turso_ops"); if err != nil || count != 1 { t.Fatalf("deliveries=%d err=%v", count, err) }
	data, err := store.Export(context.Background(), "usr_turso_ops"); if err != nil { t.Fatal(err) }
	if data.Profile["handle"] != "turso-ops" { t.Fatalf("export=%#v", data.Profile) }
}
