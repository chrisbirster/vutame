package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/activity"
	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/safety"
	"github.com/chrisbirster/vutame/internal/social"
	_ "modernc.org/sqlite"
)

func TestSafetyAPILifecycle(t *testing.T) {
	db, handler, service, sender, profiles, rawActivity := newSafetyAPIHarness(t, nil)
	ctx := context.Background()
	aliceCookie := issueEditorSession(t, service, sender, "alice-safety@example.com")
	aliceUser, err := service.Session(ctx, aliceCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(ctx, aliceUser.ID, "alice-safety"); err != nil {
		t.Fatal(err)
	}
	bobCookie := issueEditorSession(t, service, sender, "bob-safety@example.com")
	bobUser, err := service.Session(ctx, bobCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(ctx, bobUser.ID, "bob-safety"); err != nil {
		t.Fatal(err)
	}

	rec := editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/follows/bob-safety", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("follow status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/blocks/bob-safety", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("block status=%d body=%q", rec.Code, rec.Body.String())
	}
	var follows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM follows WHERE follower_user_id=? AND following_user_id=?`, aliceUser.ID, bobUser.ID).Scan(&follows); err != nil {
		t.Fatal(err)
	}
	if follows != 0 {
		t.Fatalf("follow survived block: %d", follows)
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/creators/bob-safety/social", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blocked social status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/follows/bob-safety", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "follow unavailable") {
		t.Fatalf("blocked follow status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/me/blocks", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"handle":"bob-safety"`) {
		t.Fatalf("blocks status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodDelete, "/api/v1/me/blocks/bob-safety", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unblock status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/follows/bob-safety", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("refollow status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodPut, "/api/v1/me/mutes/bob-safety", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("mute status=%d body=%q", rec.Code, rec.Body.String())
	}
	if err := rawActivity.Record(ctx, bobUser.ID, activity.KindProfileUpdated, "", "Bob Safety"); err != nil {
		t.Fatal(err)
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/feed", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"handle":"bob-safety"`) {
		t.Fatalf("muted feed status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, bobCookie, http.MethodPut, "/api/v1/me/privacy", `{"discoverable":false,"activity_visible":false,"allow_follows":false}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"allow_follows":false`) {
		t.Fatalf("privacy status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, aliceCookie, http.MethodGet, "/api/v1/discovery?q=bob-safety", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"handle":"bob-safety"`) {
		t.Fatalf("private discovery status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, aliceCookie, http.MethodPost, "/api/v1/me/reports/bob-safety", `{"reason":"spam","detail":"repeated unsolicited links"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"status":"open"`) {
		t.Fatalf("report status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPublicDiscoveryRateLimitReturns429(t *testing.T) {
	_, handler, _, _, _, _ := newSafetyAPIHarness(t, denyGate{})
	rec := editorRequest(t, handler, nil, http.MethodGet, "/api/v1/discovery", "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("rate status=%d retry=%q body=%q", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
}

type denyGate struct{}
func (denyGate) Allow(string, int, time.Duration) (bool, time.Duration) { return false, time.Minute }

func newSafetyAPIHarness(t *testing.T, limiter interface{ Allow(string, int, time.Duration) (bool, time.Duration) }) (*sql.DB, http.Handler, *auth.Service, *captureCodeSender, *profile.SQLiteStore, *activity.SQLiteStore) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "safety-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	baseSocial, err := social.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	guardedSocial, err := social.NewGuardedStore(baseSocial, db)
	if err != nil {
		t.Fatal(err)
	}
	safetyStore, err := safety.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	rawActivity, err := activity.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	guardedActivity, err := activity.NewGuardedStore(rawActivity, db)
	if err != nil {
		t.Fatal(err)
	}
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sender := &captureCodeSender{}
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("s", 32)), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	options := Options{Auth: service, Editor: profiles, Social: guardedSocial, Activity: guardedActivity, Safety: safetyStore, CookieSecure: false}
	if limiter != nil {
		options.Limiter = limiter
	}
	return db, New(http.NotFoundHandler(), profiles, options), service, sender, profiles, rawActivity
}
