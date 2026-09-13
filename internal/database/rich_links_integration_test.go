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

func TestRichLinksRunOnTursoEngine(t *testing.T) {
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const userID = "usr_turso_rich_links"
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, "rich-turso@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, userID, "rich-turso"); err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateLink(ctx, userID, profile.LinkInput{
		Label:        "GitHub",
		URL:          "https://github.com/chrisbirster",
		Kind:         "github",
		ThumbnailURL: "https://example.com/github.jpg",
		IsActive:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get("rich-turso")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Links) != 1 || loaded.Links[0].ID != created.ID || loaded.Links[0].Kind != "github" || loaded.Links[0].ThumbnailURL != "https://example.com/github.jpg" {
		t.Fatalf("Turso rich links = %#v", loaded.Links)
	}
}
