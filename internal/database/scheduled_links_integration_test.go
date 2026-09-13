package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "turso.tech/database/tursogo"
)

func TestFeaturedScheduledLinksRunOnTursoEngine(t *testing.T) {
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
		t.Fatalf("apply schema: %v", err)
	}
	store, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	const userID = "usr_turso_schedule"
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, "schedule-turso@example.com", stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, userID, "schedule-turso"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateLink(ctx, userID, profile.LinkInput{
		Label: "Normal", URL: "https://example.com/normal", Kind: "website", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	featured, err := store.CreateLink(ctx, userID, profile.LinkInput{
		Label: "Featured", URL: "https://example.com/featured", Kind: "project", Featured: true,
		VisibleFrom: now.Add(-time.Hour).Format(time.RFC3339), VisibleUntil: now.Add(time.Hour).Format(time.RFC3339), IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateLink(ctx, userID, profile.LinkInput{
		Label: "Future", URL: "https://example.com/future", Kind: "website",
		VisibleFrom: now.Add(time.Hour).Format(time.RFC3339), IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}

	public, err := store.Get("schedule-turso")
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Links) != 2 || public.Links[0].ID != featured.ID || !public.Links[0].Featured {
		t.Fatalf("public Turso schedule links = %#v", public.Links)
	}
	owned, err := store.GetOwned(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned.Links) != 3 {
		t.Fatalf("owned Turso schedule links = %#v", owned.Links)
	}
}
