package httpapi

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/moderation"
	"github.com/chrisbirster/vutame/internal/operations"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestModerationHTTPRequiresDurableRole(t *testing.T) {
	db, handler, sender := newModerationHTTPHarness(t)
	cookie := signInBillingHTTPTest(t, handler, sender, "operator@example.com")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/moderation/reports", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary user admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE email='operator@example.com'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO moderation_admins(user_id,role,created_at,updated_at) VALUES(?, 'moderator', ?, ?)`, userID, now, now); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/moderation/reports", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("moderator admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"reports"`) {
		t.Fatalf("unexpected queue body=%s", recorder.Body.String())
	}
}

func TestModerationTakedownSuppressesPublicAPIDiscoveryAndHTML(t *testing.T) {
	db, handler, sender := newModerationHTTPHarness(t)
	adminCookie := signInBillingHTTPTest(t, handler, sender, "operator@example.com")
	var adminID string
	if err := db.QueryRow(`SELECT id FROM users WHERE email='operator@example.com'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO moderation_admins(user_id,role,created_at,updated_at) VALUES(?, 'admin', ?, ?)`, adminID, now, now); err != nil {
		t.Fatal(err)
	}

	assertModerationStatus(t, handler, http.MethodGet, "/api/v1/profiles/target", "", nil, http.StatusOK)
	assertModerationStatus(t, handler, http.MethodGet, "/@target", "", nil, http.StatusOK)
	assertModerationStatus(t, handler, http.MethodGet, "/", "target.example.com", nil, http.StatusOK)

	request := jsonRequest(t, http.MethodPut, "/api/v1/admin/moderation/profiles/target", map[string]string{
		"state": "takedown",
		"note":  "confirmed impersonation",
	})
	request.AddCookie(adminCookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("takedown status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	assertModerationStatus(t, handler, http.MethodGet, "/api/v1/profiles/target", "", nil, http.StatusNotFound)
	assertModerationStatus(t, handler, http.MethodGet, "/@target", "", nil, http.StatusNotFound)
	assertModerationStatus(t, handler, http.MethodGet, "/", "target.example.com", nil, http.StatusNotFound)

	discover := httptest.NewRecorder()
	handler.ServeHTTP(discover, httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil))
	if discover.Code != http.StatusOK {
		t.Fatalf("discover status=%d body=%s", discover.Code, discover.Body.String())
	}
	if strings.Contains(discover.Body.String(), `"handle":"target"`) {
		t.Fatalf("taken-down profile leaked into discovery: %s", discover.Body.String())
	}
}

func TestModerationAppealRemainsCreatorAccessible(t *testing.T) {
	_, handler, sender := newModerationHTTPHarness(t)
	cookie := signInBillingHTTPTest(t, handler, sender, "target@example.com")
	request := jsonRequest(t, http.MethodPost, "/api/v1/me/moderation/appeals", map[string]string{
		"message": "Please review this moderation decision.",
	})
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("appeal status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"open"`) {
		t.Fatalf("appeal body=%s", recorder.Body.String())
	}
}

func newModerationHTTPHarness(t *testing.T) (*sql.DB, http.Handler, *httpCaptureSender) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "moderation-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES('usr_target','target@example.com',1,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,atproto_did,created_at,updated_at)
		VALUES('usr_target','target','Target','','','midnight',0,'did:plc:target',?,?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO custom_domains(id,user_id,hostname,verification_token,verified_at,created_at,updated_at)
		VALUES('dom_target','usr_target','target.example.com','verify-target',?,?,?)
	`, now, now, now); err != nil {
		t.Fatal(err)
	}

	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sender := &httpCaptureSender{}
	authService, err := auth.NewService(authStore, sender, []byte("0123456789abcdef0123456789abcdef"), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	moderationService, err := moderation.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	operationsStore, err := operations.NewStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>Vutame</title><meta name="description" content="Vutame"></head><body><div id="root"></div></body></html>`)
	})
	handler := New(web, profiles, Options{
		Auth:       authService,
		Moderation: moderationService,
		Operations: operationsStore,
	})
	return db, handler, sender
}

func assertModerationStatus(t *testing.T, handler http.Handler, method, path, host string, body io.Reader, want int) {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	if host != "" {
		request.Host = host
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s host=%q status=%d want=%d body=%s", method, path, host, recorder.Code, want, recorder.Body.String())
	}
}
