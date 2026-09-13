package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/chrisbirster/vutame/internal/dbschema"
)

func TestSQLiteStorePublicProfile(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedSQLiteProfile(t, store.db)

	item, err := store.Get("@CHRISDONTMISS")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "usr_01_chrisdontmiss" || item.Handle != "chrisdontmiss" {
		t.Fatalf("profile = %#v", item)
	}
	if item.Theme != DefaultTheme {
		t.Fatalf("theme = %q, want default %q", item.Theme, DefaultTheme)
	}
	if len(item.Links) != 2 {
		t.Fatalf("links = %d, want 2 active links", len(item.Links))
	}
	if item.Links[0].ID != "lnk_first" || item.Links[1].ID != "lnk_second" {
		t.Fatalf("links not ordered: %#v", item.Links)
	}
}

func TestSQLiteStorePersistsTheme(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedSQLiteProfile(t, store.db)

	updated, err := store.Update(context.Background(), "usr_01_chrisdontmiss", UpdateInput{
		DisplayName: "Chris",
		Bio:         "Theme test",
		AvatarURL:   "https://example.com/avatar.png",
		Theme:       "paper",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Theme != "paper" {
		t.Fatalf("updated theme = %q, want paper", updated.Theme)
	}
	public, err := store.Get("chrisdontmiss")
	if err != nil {
		t.Fatal(err)
	}
	if public.Theme != "paper" {
		t.Fatalf("public theme = %q, want paper", public.Theme)
	}
}

func TestSQLiteStoreHandleAvailability(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedSQLiteProfile(t, store.db)

	available, err := store.HandleAvailable("new-person")
	if err != nil || !available {
		t.Fatalf("available = %v err = %v", available, err)
	}
	available, err = store.HandleAvailable("chrisdontmiss")
	if err != nil || available {
		t.Fatalf("available = %v err = %v", available, err)
	}
	if _, err := store.HandleAvailable("admin"); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("err = %v, want ErrInvalidHandle", err)
	}
}

func TestSQLiteStoreRequiresManagedSchema(t *testing.T) {
	db, err := sql.Open("sqlite", "file:missing-schema?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = NewSQLiteStore(db)
	if !errors.Is(err, ErrSchemaNotReady) {
		t.Fatalf("err = %v, want ErrSchemaNotReady", err)
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply test schema: %v", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedSQLiteProfile(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, []any{"usr_01_chrisdontmiss", "chris@example.com", "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"}},
		{`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, verified, created_at, updated_at) VALUES (?, ?, ?, ?, '', 0, ?, ?)`, []any{"usr_01_chrisdontmiss", "chrisdontmiss", "@chrisdontmiss", "Building things on the internet.", "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"}},
		{`INSERT INTO links (id, user_id, label, url, kind, position, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"lnk_second", "usr_01_chrisdontmiss", "Second", "https://example.com/2", "website", 1, 1, "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"}},
		{`INSERT INTO links (id, user_id, label, url, kind, position, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"lnk_hidden", "usr_01_chrisdontmiss", "Hidden", "https://example.com/hidden", "website", 0, 0, "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"}},
		{`INSERT INTO links (id, user_id, label, url, kind, position, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"lnk_first", "usr_01_chrisdontmiss", "First", "https://example.com/1", "website", 0, 1, "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}
