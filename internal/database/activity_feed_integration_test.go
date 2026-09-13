package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/activity"
	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "turso.tech/database/tursogo"
)

func TestActivityFeedRunsOnTursoEngine(t *testing.T) {
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
	store, err := activity.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	insertActivityTursoCreator(t, db, "usr_feed_a", "feed-a")
	insertActivityTursoCreator(t, db, "usr_feed_b", "feed-b")
	if _, err := db.Exec(`INSERT INTO follows (follower_user_id, following_user_id, created_at) VALUES (?, ?, ?)`, "usr_feed_a", "usr_feed_b", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(context.Background(), "usr_feed_b", activity.KindProfileUpdated, "", "Feed B"); err != nil {
		t.Fatal(err)
	}
	page, err := store.Following(context.Background(), "usr_feed_a", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].Handle != "feed-b" {
		t.Fatalf("page = %#v", page)
	}
	trends, err := store.Trending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(trends) < 1 || trends[0].Handle != "feed-b" {
		t.Fatalf("trends = %#v", trends)
	}
}

func insertActivityTursoCreator(t *testing.T, db *sql.DB, userID, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, NULL, ?, ?)`, userID, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}
