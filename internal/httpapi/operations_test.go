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
	"github.com/chrisbirster/vutame/internal/operations"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

type httpTXTResolver struct{ values map[string][]string }
func (r httpTXTResolver) LookupTXT(_ context.Context, name string) ([]string, error) { return r.values[name], nil }

func TestCreatorOperationsHTTPAndCustomDomain(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "ops-http.db"))
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)
	t.Cleanup(func(){ _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatal(err) }
	profiles, err := profile.NewSQLiteStore(db); if err != nil { t.Fatal(err) }
	authStore, err := auth.NewSQLiteStore(db); if err != nil { t.Fatal(err) }
	sender := &captureCodeSender{}
	secret := []byte(strings.Repeat("o", 32))
	authService, err := auth.NewService(authStore, sender, secret, auth.Config{}); if err != nil { t.Fatal(err) }
	ops, err := operations.NewStore(db, secret); if err != nil { t.Fatal(err) }
	cookie := issueEditorSession(t, authService, sender, "ops-http@example.com")
	user, err := authService.Session(context.Background(), cookie.Value); if err != nil { t.Fatal(err) }
	if _, err := profiles.Claim(context.Background(), user.ID, "ops-http"); err != nil { t.Fatal(err) }
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request){ _, _ = w.Write([]byte(`<html><head><title>Base</title></head><body><div id="root"></div></body></html>`)) })
	handler := New(base, profiles, Options{Auth: authService, Editor: profiles, Operations: ops, CookieSecure: false, ProfileOrigin: "https://vuta.me"})

	createDomain := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/domains", `{"hostname":"creator.example.com"}`)
	if createDomain.Code != http.StatusCreated { t.Fatalf("domain status=%d body=%q", createDomain.Code, createDomain.Body.String()) }
	var domain operations.Domain
	if err := json.Unmarshal(createDomain.Body.Bytes(), &domain); err != nil { t.Fatal(err) }
	ops.SetTXTResolver(httpTXTResolver{values: map[string][]string{domain.DNSName:{domain.DNSValue}}})
	verify := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/domains/"+domain.ID+"/verify", `{}`)
	if verify.Code != http.StatusOK { t.Fatalf("verify status=%d body=%q", verify.Code, verify.Body.String()) }

	custom := httptest.NewRequest(http.MethodGet, "https://creator.example.com/", nil)
	custom.Host = "creator.example.com"
	customRec := httptest.NewRecorder()
	handler.ServeHTTP(customRec, custom)
	if customRec.Code != http.StatusOK || !strings.Contains(customRec.Body.String(), `rel="canonical" href="https://creator.example.com/"`) || !strings.Contains(customRec.Body.String(), "@ops-http") {
		t.Fatalf("custom domain status=%d body=%q", customRec.Code, customRec.Body.String())
	}

	createToken := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/tokens", `{"name":"reader","scopes":["profile:read"],"expires_in_days":30}`)
	if createToken.Code != http.StatusCreated { t.Fatalf("token status=%d body=%q", createToken.Code, createToken.Body.String()) }
	var token operations.CreatedToken
	if err := json.Unmarshal(createToken.Body.Bytes(), &token); err != nil { t.Fatal(err) }
	request := httptest.NewRequest(http.MethodGet, "/api/v1/token/profile", nil)
	request.Header.Set("Authorization", "Bearer "+token.Token)
	response := httptest.NewRecorder(); handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"handle":"ops-http"`) { t.Fatalf("token API status=%d body=%q", response.Code, response.Body.String()) }

	badScope := httptest.NewRequest(http.MethodGet, "/api/v1/token/analytics", nil)
	badScope.Header.Set("Authorization", "Bearer "+token.Token)
	badRec := httptest.NewRecorder(); handler.ServeHTTP(badRec, badScope)
	if badRec.Code != http.StatusUnauthorized { t.Fatalf("scope status=%d", badRec.Code) }

	export := editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/export", "")
	if export.Code != http.StatusOK || !strings.Contains(export.Header().Get("Content-Disposition"), "vutame-export.json") || strings.Contains(export.Body.String(), token.Token) {
		t.Fatalf("export status=%d body=%q", export.Code, export.Body.String())
	}
}
