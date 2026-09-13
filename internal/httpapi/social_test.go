package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/social"
	_ "modernc.org/sqlite"
)

func TestSocialAPILifecycle(t *testing.T) {
	handler, service, sender, profiles := newSocialAPIHarness(t)
	ctx := context.Background()

	aliceCookie := issueEditorSession(t, service, sender, "alice-social@example.com")
	aliceUser, err := service.Session(ctx, aliceCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(ctx, aliceUser.ID, "alice-social"); err != nil {
		t.Fatal(err)
	}

	bobCookie := issueEditorSession(t, service, sender, "bob-social@example.com")
	bobUser, err := service.Session(ctx, bobCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(ctx, bobUser.ID, "bob-social"); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Update(ctx, bobUser.ID, profile.UpdateInput{
		DisplayName: "Bob Systems", Bio: "Zig compiler tools", Theme: "midnight",
	}); err != nil {
		t.Fatal(err)
	}

	rec := editorRequest(t, handler, bobCookie, http.MethodPut, "/api/v1/me/discovery-profile", `{"category":"Engineering","interests":["zig","compilers"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"category":"Engineering"`) || !strings.Contains(rec.Body.String(), `"zig"`) {
		t.Fatalf("metadata status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/follows/bob-social", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("follow status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/discovery?q=zig", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"handle":"bob-social"`) || !strings.Contains(rec.Body.String(), `"viewer_follows":true`) {
		t.Fatalf("discovery status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/creators/bob-social/social", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"follower_count":1`) || !strings.Contains(rec.Body.String(), `"viewer_follows":true`) {
		t.Fatalf("creator social status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, bobCookie, http.MethodGet, "/api/v1/creators/bob-social/followers", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"handle":"alice-social"`) {
		t.Fatalf("followers status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/creators/alice-social/following", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"handle":"bob-social"`) {
		t.Fatalf("following status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/follows/alice-social", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("self-follow status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodDelete, "/api/v1/me/follows/bob-social", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unfollow status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/creators/bob-social/social", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"follower_count":0`) || !strings.Contains(rec.Body.String(), `"viewer_follows":false`) {
		t.Fatalf("after unfollow status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSocialAPIFollowRequiresAuthentication(t *testing.T) {
	handler, _, _, _ := newSocialAPIHarness(t)
	rec := editorRequest(t, handler, nil, http.MethodPut, "/api/v1/me/follows/someone", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func newSocialAPIHarness(t *testing.T) (http.Handler, *auth.Service, *captureCodeSender, *profile.SQLiteStore) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "social-api.db")
	db, err := sql.Open("sqlite", dsn)
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
	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	socialStore, err := social.NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	sender := &captureCodeSender{}
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("g", 32)), auth.Config{})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(http.NotFoundHandler(), profiles, Options{
		Auth: service, Editor: profiles, Social: socialStore, CookieSecure: false,
	}), service, sender, profiles
}
