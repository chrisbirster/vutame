package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/social"
	_ "turso.tech/database/tursogo"
)

func TestSocialGraphRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	store, err := social.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	insertTursoCreator(t, db, "usr_turso_one", "turso-one", "Turso One", "SQLite creator")
	insertTursoCreator(t, db, "usr_turso_two", "turso-two", "Turso Two", "Zig and databases")

	if _, err := store.UpdateMetadata(ctx, "usr_turso_two", social.MetadataInput{Category: "Engineering", Interests: []string{"zig", "databases"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(ctx, "usr_turso_one", "turso-two"); err != nil {
		t.Fatal(err)
	}
	creator, err := store.Creator(ctx, "turso-two", "usr_turso_one")
	if err != nil {
		t.Fatal(err)
	}
	if creator.FollowerCount != 1 || !creator.ViewerFollows || creator.Category != "Engineering" {
		t.Fatalf("creator = %#v", creator)
	}
	results, err := store.Search(ctx, social.SearchInput{Query: "zig"}, "usr_turso_one")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Handle != "turso-two" || !results[0].ViewerFollows {
		t.Fatalf("results = %#v", results)
	}
	if err := store.Unfollow(ctx, "usr_turso_one", "turso-two"); err != nil {
		t.Fatal(err)
	}
}

func insertTursoCreator(t *testing.T, db *sql.DB, id, handle, displayName, bio string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, id, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', 'midnight', 0, NULL, ?, ?)
	`, id, handle, displayName, bio, now, now); err != nil {
		t.Fatal(err)
	}
}
