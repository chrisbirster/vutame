package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/media"
	"github.com/chrisbirster/vutame/internal/profile"
)

const httpTestPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Wl9sAAAAASUVORK5CYII="

func TestAvatarUploadServeAndDelete(t *testing.T) {
	handler, service, sender := newMediaAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "media-http@example.com")

	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"media-http"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = avatarUploadRequest(t, handler, cookie, "http://example.com")
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%q", rec.Code, rec.Body.String())
	}
	var asset media.Asset
	if err := json.Unmarshal(rec.Body.Bytes(), &asset); err != nil {
		t.Fatal(err)
	}
	if asset.ContentType != "image/png" || !strings.HasPrefix(asset.URL, "/media/med_") {
		t.Fatalf("asset = %#v", asset)
	}

	publicReq := httptest.NewRequest(http.MethodGet, asset.URL, nil)
	publicRec := httptest.NewRecorder()
	handler.ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK || publicRec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("media get status=%d type=%q", publicRec.Code, publicRec.Header().Get("Content-Type"))
	}
	if publicRec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || publicRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("media headers = %#v", publicRec.Header())
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/profile", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"avatar_url":"`+asset.URL+`"`) {
		t.Fatalf("owned profile after upload status=%d body=%q", rec.Code, rec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/me/avatar", nil)
	deleteReq.Header.Set("Origin", "http://example.com")
	deleteReq.AddCookie(cookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", deleteRec.Code, deleteRec.Body.String())
	}

	goneReq := httptest.NewRequest(http.MethodGet, asset.URL, nil)
	goneRec := httptest.NewRecorder()
	handler.ServeHTTP(goneRec, goneReq)
	if goneRec.Code != http.StatusNotFound {
		t.Fatalf("deleted media status=%d, want 404", goneRec.Code)
	}
}

func TestAvatarUploadRejectsCrossOriginMutation(t *testing.T) {
	handler, service, sender := newMediaAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "media-origin@example.com")
	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"media-origin"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = avatarUploadRequest(t, handler, cookie, "https://evil.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin upload status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func newMediaAPIHarness(t *testing.T) (http.Handler, *auth.Service, *captureCodeSender) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "vutame.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
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
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	blobs, err := media.NewFileStore(t.TempDir())
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	mediaService, err := media.NewService(db, blobs)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	sender := &captureCodeSender{}
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("m", 32)), auth.Config{})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(http.NotFoundHandler(), profiles, Options{Auth: service, Editor: profiles, Media: mediaService, CookieSecure: false}), service, sender
}

func avatarUploadRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(httpTestPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/avatar", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

var _ = context.Background
