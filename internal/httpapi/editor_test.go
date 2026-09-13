package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
)

type captureCodeSender struct{ code string }

func (s *captureCodeSender) SendCode(_ context.Context, _ string, code string) error {
	s.code = code
	return nil
}

func TestEditorAPIRequiresAuthentication(t *testing.T) {
	handler, _, _ := newEditorAPIHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/profile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEditorAPILifecycle(t *testing.T) {
	handler, service, sender := newEditorAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "creator@example.com")

	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"Creator-One"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"handle":"creator-one"`) || !strings.Contains(rec.Body.String(), `"theme":"midnight"`) {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPatch, "/api/v1/me/profile", `{"display_name":"Creator One","bio":"A real persisted Vuta.","avatar_url":"https://example.com/avatar.png","theme":"forest"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"display_name":"Creator One"`) || !strings.Contains(rec.Body.String(), `"theme":"forest"`) {
		t.Fatalf("profile update status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPatch, "/api/v1/me/profile", `{"display_name":"Creator One","bio":"A real persisted Vuta.","avatar_url":"https://example.com/avatar.png","theme":"not-a-theme"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unsupported theme") {
		t.Fatalf("invalid theme status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", `{"label":"First","url":"https://example.com/first","kind":"website","is_active":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create first link status=%d body=%q", rec.Code, rec.Body.String())
	}
	var first profile.Link
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", `{"label":"Second","url":"https://example.com/second","kind":"project","is_active":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create second link status=%d body=%q", rec.Code, rec.Body.String())
	}
	var second profile.Link
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}

	rec = editorRequest(t, handler, cookie, http.MethodPatch, "/api/v1/me/links/"+first.ID, `{"label":"First","url":"https://example.com/first","kind":"website","is_active":false}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"is_active":false`) {
		t.Fatalf("hide link status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPut, "/api/v1/me/links/order", `{"ids":["`+second.ID+`","`+first.ID+`"]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reorder status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/profile", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("owned profile status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Index(body, second.ID) < 0 || strings.Index(body, first.ID) < strings.Index(body, second.ID) {
		t.Fatalf("owned profile did not preserve reordered links: %q", body)
	}
	if !strings.Contains(body, `"is_active":false`) {
		t.Fatalf("owned profile should include hidden link: %q", body)
	}
	if !strings.Contains(body, `"theme":"forest"`) {
		t.Fatalf("owned profile should preserve selected theme: %q", body)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/creator-one", nil)
	publicRec := httptest.NewRecorder()
	handler.ServeHTTP(publicRec, req)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("public profile status=%d body=%q", publicRec.Code, publicRec.Body.String())
	}
	if strings.Contains(publicRec.Body.String(), first.ID) || !strings.Contains(publicRec.Body.String(), second.ID) {
		t.Fatalf("public profile visibility incorrect: %q", publicRec.Body.String())
	}
	if !strings.Contains(publicRec.Body.String(), `"theme":"forest"`) {
		t.Fatalf("public profile should expose persisted theme: %q", publicRec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodDelete, "/api/v1/me/links/"+second.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func newEditorAPIHarness(t *testing.T) (http.Handler, *auth.Service, *captureCodeSender) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "vutame.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	profiles, err := profile.OpenSQLite(dsn)
	if err != nil {
		t.Fatal(err)
	}
	authStore, err := auth.OpenSQLite(dsn)
	if err != nil {
		profiles.Close()
		t.Fatal(err)
	}
	sender := &captureCodeSender{}
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("s", 32)), auth.Config{})
	if err != nil {
		profiles.Close()
		authStore.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = authStore.Close()
		_ = profiles.Close()
	})
	return New(http.NotFoundHandler(), profiles, Options{Auth: service, Editor: profiles, CookieSecure: false}), service, sender
}

func issueEditorSession(t *testing.T, service *auth.Service, sender *captureCodeSender, email string) *http.Cookie {
	t.Helper()
	challengeID, err := service.RequestCode(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.VerifyCode(context.Background(), challengeID, email, sender.code)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: result.Token, Path: "/"}
}

func editorRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
