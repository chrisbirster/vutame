package profile

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestMemoryStoreResolvePublicLinkHonorsVisibility(t *testing.T) {
	now := time.Now().UTC()
	store := NewMemoryStore([]Profile{{
		ID: "usr_memory", Handle: "memory", Theme: "midnight",
		Links: []Link{
			{ID: "lnk_live", Label: "Live", URL: "https://example.com/live", Kind: "website", IsActive: true},
			{ID: "lnk_off", Label: "Off", URL: "https://example.com/off", Kind: "website", IsActive: false},
			{ID: "lnk_future", Label: "Future", URL: "https://example.com/future", Kind: "website", IsActive: true, VisibleFrom: now.Add(time.Hour).Format(time.RFC3339)},
		},
	}})
	target, err := store.ResolvePublicLink("lnk_live")
	if err != nil {
		t.Fatal(err)
	}
	if target.UserID != "usr_memory" || target.Link.URL != "https://example.com/live" {
		t.Fatalf("target = %#v", target)
	}
	for _, id := range []string{"lnk_off", "lnk_future", "missing"} {
		if _, err := store.ResolvePublicLink(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s error = %v", id, err)
		}
	}
}

func TestSQLiteStoreResolvePublicLinkHonorsVisibility(t *testing.T) {
	db, err := sql.Open("sqlite", "file:public-link?mode=memory&cache=shared")
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
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	created := now.Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES ('usr_sql', 'sql@example.com', 1, ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES ('usr_sql', 'sql', 'SQL', '', '', 'midnight', 0, ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id, target string
		active     int
		from, until any
	}{
		{"lnk_live", "https://example.com/live", 1, nil, nil},
		{"lnk_off", "https://example.com/off", 0, nil, nil},
		{"lnk_future", "https://example.com/future", 1, now.Add(time.Hour).Format(time.RFC3339), nil},
		{"lnk_expired", "https://example.com/expired", 1, nil, now.Add(-time.Hour).Format(time.RFC3339)},
	} {
		if _, err := db.Exec(`
			INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active, created_at, updated_at)
			VALUES (?, 'usr_sql', ?, ?, 'website', '', 0, ?, ?, 0, ?, ?, ?)
		`, item.id, item.id, item.target, item.from, item.until, item.active, created, created); err != nil {
			t.Fatal(err)
		}
	}
	target, err := store.ResolvePublicLink("lnk_live")
	if err != nil {
		t.Fatal(err)
	}
	if target.UserID != "usr_sql" || target.Link.URL != "https://example.com/live" {
		t.Fatalf("target = %#v", target)
	}
	for _, id := range []string{"lnk_off", "lnk_future", "lnk_expired", "missing"} {
		if _, err := store.ResolvePublicLink(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s error = %v", id, err)
		}
	}
}
