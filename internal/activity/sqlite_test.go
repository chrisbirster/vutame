package activity

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestSQLiteActivityFollowingRecentAndTrending(t *testing.T) {
	db := openActivityDB(t)
	ctx := context.Background()
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	insertActivityCreator(t, db, "usr_alice", "alice")
	insertActivityCreator(t, db, "usr_bob", "bob")
	insertActivityCreator(t, db, "usr_cara", "cara")
	if _, err := db.Exec(`INSERT INTO follows (follower_user_id, following_user_id, created_at) VALUES (?, ?, ?)`, "usr_alice", "usr_bob", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	if err := store.Record(ctx, "usr_bob", KindProfileUpdated, "", "Bob Builder"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := store.Record(ctx, "usr_cara", KindProfileUpdated, "", "Cara"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := store.Record(ctx, "usr_bob", KindLinkFeatured, "", "New project"); err != nil {
		t.Fatal(err)
	}

	feed, err := store.Following(ctx, "usr_alice", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Events) != 2 || feed.Events[0].Handle != "bob" || feed.Events[1].Handle != "bob" {
		t.Fatalf("following feed = %#v", feed)
	}

	first, err := store.Recent(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 1 || first.NextCursor == "" || first.Events[0].Kind != KindLinkFeatured {
		t.Fatalf("first page = %#v", first)
	}
	second, err := store.Recent(ctx, first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) != 1 || second.Events[0].Handle != "cara" {
		t.Fatalf("second page = %#v", second)
	}
	if _, err := store.Recent(ctx, "not-a-cursor", 1); err != ErrInvalidCursor {
		t.Fatalf("invalid cursor error = %v", err)
	}

	trends, err := store.Trending(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(trends) != 3 || trends[0].Handle != "bob" || trends[0].RecentActivity != 2 || trends[0].FollowerCount != 1 {
		t.Fatalf("trends = %#v", trends)
	}
}

func openActivityDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "activity.db"))
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
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertActivityCreator(t *testing.T, db *sql.DB, userID, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, NULL, ?, ?)`, userID, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}
