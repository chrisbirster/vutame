package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "turso.tech/database/tursogo"
)

func TestVutameStoresRunOnTursoEngine(t *testing.T) {
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

	profileStore, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open profile store on Turso engine: %v", err)
	}
	if _, err := auth.NewSQLiteStore(db); err != nil {
		t.Fatalf("open auth store on Turso engine: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const userID = "usr_turso_integration"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, email_verified, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
	`, userID, "turso@example.com", now, now); err != nil {
		t.Fatalf("seed Turso user: %v", err)
	}

	claimed, err := profileStore.Claim(ctx, userID, "turso-creator")
	if err != nil {
		t.Fatalf("claim profile: %v", err)
	}
	if claimed.Handle != "turso-creator" || claimed.ID != userID || claimed.Theme != profile.DefaultTheme {
		t.Fatalf("claimed profile = %#v", claimed)
	}

	updated, err := profileStore.Update(ctx, userID, profile.UpdateInput{
		DisplayName: "Turso Creator",
		Bio:         "Running Vutame on the production database engine.",
		AvatarURL:   "https://example.com/avatar.png",
		Theme:       "neon",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.DisplayName != "Turso Creator" || updated.Theme != "neon" {
		t.Fatalf("updated profile = %#v", updated)
	}

	first, err := profileStore.CreateLink(ctx, userID, profile.LinkInput{
		Label:    "First",
		URL:      "https://example.com/first",
		Kind:     "website",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("create first link: %v", err)
	}
	second, err := profileStore.CreateLink(ctx, userID, profile.LinkInput{
		Label:    "Second",
		URL:      "https://example.com/second",
		Kind:     "social",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("create second link: %v", err)
	}

	if err := profileStore.ReorderLinks(ctx, userID, []string{second.ID, first.ID}); err != nil {
		t.Fatalf("reorder links: %v", err)
	}
	if _, err := profileStore.UpdateLink(ctx, userID, first.ID, profile.LinkInput{
		Label:    first.Label,
		URL:      first.URL,
		Kind:     first.Kind,
		IsActive: false,
	}); err != nil {
		t.Fatalf("hide first link: %v", err)
	}

	public, err := profileStore.Get("@TURSO-CREATOR")
	if err != nil {
		t.Fatalf("load public profile: %v", err)
	}
	if public.DisplayName != "Turso Creator" || public.Theme != "neon" {
		t.Fatalf("public profile = %#v", public)
	}
	if len(public.Links) != 1 || public.Links[0].ID != second.ID || public.Links[0].Position != 0 {
		t.Fatalf("public links = %#v, want only reordered active second link", public.Links)
	}

	owned, err := profileStore.GetOwned(ctx, userID)
	if err != nil {
		t.Fatalf("load owned profile: %v", err)
	}
	if owned.Theme != "neon" {
		t.Fatalf("owned theme = %q, want neon", owned.Theme)
	}
	if len(owned.Links) != 2 || owned.Links[0].ID != second.ID || owned.Links[1].ID != first.ID || owned.Links[1].IsActive {
		t.Fatalf("owned links = %#v", owned.Links)
	}

	if err := profileStore.DeleteLink(ctx, userID, first.ID); err != nil {
		t.Fatalf("delete hidden link: %v", err)
	}
	final, err := profileStore.GetOwned(ctx, userID)
	if err != nil {
		t.Fatalf("load final profile: %v", err)
	}
	if len(final.Links) != 1 || final.Links[0].ID != second.ID || final.Links[0].Position != 0 {
		t.Fatalf("final links = %#v", final.Links)
	}
}
