package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
)

type httpCaptureSender struct{ code string }

func (s *httpCaptureSender) SendCode(_ context.Context, _ string, code string) error {
	s.code = code
	return nil
}

func TestAuthHTTPFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http-auth.db")
	dsn := "file:" + path
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	authStore, err := auth.OpenSQLite(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer authStore.Close()
	sender := &httpCaptureSender{}
	authService, err := auth.NewService(authStore, sender, []byte("0123456789abcdef0123456789abcdef"), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profile.NewSeedStore(), Options{Auth: authService})

	request := jsonRequest(t, http.MethodPost, "/api/v1/auth/code", map[string]string{"email": "me@example.com"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("code request status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var issued struct{ ChallengeID string `json:"challenge_id"` }
	if err := json.Unmarshal(recorder.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}

	request = jsonRequest(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{
		"challenge_id": issued.ChallengeID,
		"email":        "me@example.com",
		"code":         sender.code,
	})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookies=%#v", cookies)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"authenticated":true`)) {
		t.Fatalf("session status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = jsonRequest(t, http.MethodPost, "/api/v1/auth/logout", struct{}{})
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d", recorder.Code)
	}
}

func TestAuthPostRequiresJSON(t *testing.T) {
	handler := New(http.NotFoundHandler(), profile.NewSeedStore(), Options{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

func jsonRequest(t *testing.T, method, path string, value any) *http.Request {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}
