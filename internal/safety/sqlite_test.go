package safety

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestSafetyLifecycle(t *testing.T) {
	db := safetyTestDB(t)
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	insertSafetyCreator(t, db, "usr_a", "alpha")
	insertSafetyCreator(t, db, "usr_b", "bravo")

	if _, err := db.Exec(`INSERT INTO follows (follower_user_id, following_user_id, created_at) VALUES ('usr_a','usr_b','2026-01-01T00:00:00Z'), ('usr_b','usr_a','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Block(ctx, "usr_a", "bravo"); err != nil {
		t.Fatal(err)
	}
	var followCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM follows WHERE (follower_user_id='usr_a' AND following_user_id='usr_b') OR (follower_user_id='usr_b' AND following_user_id='usr_a')`).Scan(&followCount); err != nil {
		t.Fatal(err)
	}
	if followCount != 0 {
		t.Fatalf("blocked follow count = %d", followCount)
	}
	blocks, err := store.Blocks(ctx, "usr_a")
	if err != nil || len(blocks) != 1 || blocks[0].Handle != "bravo" {
		t.Fatalf("blocks = %#v err=%v", blocks, err)
	}
	if err := store.Unblock(ctx, "usr_a", "bravo"); err != nil {
		t.Fatal(err)
	}

	if err := store.Mute(ctx, "usr_a", "bravo"); err != nil {
		t.Fatal(err)
	}
	mutes, err := store.Mutes(ctx, "usr_a")
	if err != nil || len(mutes) != 1 || mutes[0].Handle != "bravo" {
		t.Fatalf("mutes = %#v err=%v", mutes, err)
	}

	privacy, err := store.Privacy(ctx, "usr_a")
	if err != nil || !privacy.Discoverable || !privacy.ActivityVisible || !privacy.AllowFollows {
		t.Fatalf("default privacy = %#v err=%v", privacy, err)
	}
	if _, err := db.Exec(`INSERT INTO follows (follower_user_id, following_user_id, created_at) VALUES ('usr_b','usr_a','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdatePrivacy(ctx, "usr_a", Privacy{Discoverable: false, ActivityVisible: false, AllowFollows: false})
	if err != nil || updated.Discoverable || updated.ActivityVisible || updated.AllowFollows {
		t.Fatalf("updated privacy = %#v err=%v", updated, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM follows WHERE following_user_id='usr_a'`).Scan(&followCount); err != nil {
		t.Fatal(err)
	}
	if followCount != 0 {
		t.Fatalf("inbound follows after disabling = %d", followCount)
	}

	report, err := store.Report(ctx, "usr_a", "bravo", ReportInput{Reason: "spam", Detail: "repeated unsolicited links"})
	if err != nil || report.Status != "open" || report.ReportedHandle != "bravo" {
		t.Fatalf("report = %#v err=%v", report, err)
	}
	if _, err := store.Report(ctx, "usr_a", "bravo", ReportInput{Reason: "unknown"}); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("invalid report error = %v", err)
	}
	if err := store.SetModeration(ctx, "bravo", Moderation{State: "suspended", Note: "test"}); err != nil {
		t.Fatal(err)
	}
	moderation, err := store.Moderation(ctx, "bravo")
	if err != nil || moderation.State != "suspended" {
		t.Fatalf("moderation = %#v err=%v", moderation, err)
	}

	if err := store.Block(ctx, "usr_a", "alpha"); !errors.Is(err, ErrSelfAction) {
		t.Fatalf("self block error = %v", err)
	}
}

func safetyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:safety-"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertSafetyCreator(t *testing.T, db *sql.DB, id, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id,email,email_verified,created_at,updated_at) VALUES (?, ?, 1, ?, ?)`, id, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, ?, ?)`, id, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}
