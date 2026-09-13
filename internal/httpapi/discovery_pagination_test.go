package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/social"
	_ "modernc.org/sqlite"
)

func TestDiscoveryPaginationHTTP(t *testing.T) {
	db, err := sql.Open("sqlite", "file:discovery-pagination?mode=memory&cache=shared")
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range []struct{ id, handle string }{{"usr_a", "alpha"}, {"usr_b", "bravo"}, {"usr_c", "charlie"}} {
		if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, item.id, item.handle+"@example.com", now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES (?, ?, ?, 'builder', '', 'midnight', 0, ?, ?)`, item.id, item.handle, item.handle, now, now); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	socialStore, err := social.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profiles, Options{Social: socialStore})

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/discovery?q=builder&limit=2", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	var page social.CreatorPage
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Creators) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %#v", page)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/discovery?q=builder&limit=2&cursor="+page.NextCursor, nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.Code, second.Body.String())
	}
	var next social.CreatorPage
	if err := json.Unmarshal(second.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Creators) != 1 || next.NextCursor != "" {
		t.Fatalf("second page = %#v", next)
	}

	bad := httptest.NewRecorder()
	// URL-safe base64 for "not-a-number": syntactically valid query input,
	// but semantically invalid for the discovery cursor decoder.
	handler.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/v1/discovery?cursor=bm90LWEtbnVtYmVy", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d body=%s", bad.Code, bad.Body.String())
	}
}
