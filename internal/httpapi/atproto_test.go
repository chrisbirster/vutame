package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/atproto"
	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestATProtoClientMetadataAndAccountLifecycle(t *testing.T) {
	handler, service, sender, db := newATProtoAPIHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"dpop_bound_access_tokens":true`) || !strings.Contains(rec.Body.String(), `"client_id":"https://vutame.example/oauth-client-metadata.json"`) {
		t.Fatalf("metadata status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/atproto", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated account status=%d body=%q", rec.Code, rec.Body.String())
	}

	cookie := issueEditorSession(t, service, sender, "atproto@example.com")
	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"portable-user"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/atproto", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"linked":false`) {
		t.Fatalf("unlinked account status=%d body=%q", rec.Code, rec.Body.String())
	}

	var userID string
	if err := db.QueryRow(`SELECT user_id FROM profiles WHERE handle='portable-user'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO atproto_accounts(
			user_id,did,handle,pds_url,issuer,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,
			expires_at,conflict_policy,publish_enabled,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`, userID, "did:plc:portable", "portable.example", "https://pds.example", "https://auth.example", "https://auth.example/token", "enc-access", "enc-refresh", "enc-key", "atproto", now.Add(time.Hour).Format(time.RFC3339Nano), "vutame_wins", 0, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/atproto", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"linked":true`) || !strings.Contains(rec.Body.String(), `"did":"did:plc:portable"`) {
		t.Fatalf("linked account status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPut, "/api/v1/me/atproto", `{"conflict_policy":"pds_wins","publish_enabled":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"conflict_policy":"pds_wins"`) || !strings.Contains(rec.Body.String(), `"publish_enabled":true`) {
		t.Fatalf("settings status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodDelete, "/api/v1/me/atproto", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unlink status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/atproto", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"linked":false`) {
		t.Fatalf("post-unlink status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestATProtoSettingsRejectInvalidConflictPolicy(t *testing.T) {
	handler, service, sender, db := newATProtoAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "policy@example.com")
	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"policy-user"}`)
	if rec.Code != http.StatusCreated { t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String()) }
	var userID string
	if err := db.QueryRow(`SELECT user_id FROM profiles WHERE handle='policy-user'`).Scan(&userID); err != nil { t.Fatal(err) }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO atproto_accounts(user_id,did,handle,pds_url,issuer,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at,conflict_policy,publish_enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, userID, "did:plc:policy", "policy.example", "https://pds.example", "https://auth.example", "https://auth.example/token", "a", "b", "c", "atproto", now, "vutame_wins", 0, now, now); err != nil { t.Fatal(err) }

	rec = editorRequest(t, handler, cookie, http.MethodPut, "/api/v1/me/atproto", `{"conflict_policy":"last_write_wins","publish_enabled":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func newATProtoAPIHarness(t *testing.T) (http.Handler, *auth.Service, *captureCodeSender, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "atproto-api.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("apply schema: %v", err) }
	profiles, err := profile.OpenSQLite(dsn)
	if err != nil { t.Fatal(err) }
	authStore, err := auth.OpenSQLite(dsn)
	if err != nil { t.Fatal(err) }
	sender := &captureCodeSender{}
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("s", 32)), auth.Config{})
	if err != nil { t.Fatal(err) }
	atStore, err := atproto.NewStore(db, []byte(strings.Repeat("p", 32)), atproto.Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/api/v1/me/atproto/oauth/callback", MarketingURL: "https://vutame.example",
	})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = authStore.Close(); _ = profiles.Close(); _ = db.Close() })
	return New(http.NotFoundHandler(), profiles, Options{Auth: service, Editor: profiles, ATProto: atStore, CookieSecure: false}), service, sender, db
}
