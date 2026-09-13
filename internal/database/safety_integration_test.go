package database_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/activity"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/safety"
	"github.com/chrisbirster/vutame/internal/social"
	_ "turso.tech/database/tursogo"
)

func TestSafetyPolicyRunsOnTursoEngine(t *testing.T) {
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
		t.Fatal(err)
	}
	insertSafetyTursoCreator(t, db, "usr_safe_a", "safe-a")
	insertSafetyTursoCreator(t, db, "usr_safe_b", "safe-b")

	baseSocial, err := social.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	guardedSocial, err := social.NewGuardedStore(baseSocial, db)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := safety.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	baseActivity, err := activity.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	guardedActivity, err := activity.NewGuardedStore(baseActivity, db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := guardedSocial.Follow(ctx, "usr_safe_a", "safe-b"); err != nil {
		t.Fatal(err)
	}
	if err := policies.Block(ctx, "usr_safe_a", "safe-b"); err != nil {
		t.Fatal(err)
	}
	if err := guardedSocial.Follow(ctx, "usr_safe_a", "safe-b"); !errors.Is(err, social.ErrRelationshipBlocked) {
		t.Fatalf("blocked follow error = %v", err)
	}
	page, err := guardedSocial.SearchPage(ctx, social.SearchInput{Limit: 10}, "usr_safe_a")
	if err != nil {
		t.Fatal(err)
	}
	for _, creator := range page.Creators {
		if creator.Handle == "safe-b" {
			t.Fatal("blocked creator leaked through Turso discovery")
		}
	}
	if err := policies.Unblock(ctx, "usr_safe_a", "safe-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := policies.UpdatePrivacy(ctx, "usr_safe_b", safety.Privacy{Discoverable: false, ActivityVisible: false, AllowFollows: false}); err != nil {
		t.Fatal(err)
	}
	if err := guardedSocial.Follow(ctx, "usr_safe_a", "safe-b"); !errors.Is(err, social.ErrFollowsDisabled) {
		t.Fatalf("privacy follow error = %v", err)
	}
	if err := baseActivity.Record(ctx, "usr_safe_b", activity.KindProfileUpdated, "", "Safe B"); err != nil {
		t.Fatal(err)
	}
	recent, err := guardedActivity.RecentForViewer(ctx, "usr_safe_a", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Events) != 0 {
		t.Fatalf("private activity leaked through Turso: %#v", recent)
	}
}

func insertSafetyTursoCreator(t *testing.T, db *sql.DB, id, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id,email,email_verified,created_at,updated_at) VALUES (?, ?, 1, ?, ?)`, id, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, ?, ?)`, id, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}
