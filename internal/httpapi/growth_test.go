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
	"github.com/chrisbirster/vutame/internal/growth"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestGrowthHTTPConsentAndOwnerIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "growth-http.db"))
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
	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	authStore, err := auth.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sender := &captureCodeSender{}
	secret := []byte(strings.Repeat("g", 32))
	authService, err := auth.NewService(authStore, sender, secret, auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	growthService, err := growth.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profiles, Options{Auth: authService, Editor: profiles, Growth: growthService, CookieSecure: false})

	ownerCookie := issueEditorSession(t, authService, sender, "growth-owner@example.com")
	owner, err := authService.Session(context.Background(), ownerCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(context.Background(), owner.ID, "growth-owner"); err != nil {
		t.Fatal(err)
	}

	unauth := editorRequest(t, handler, nil, http.MethodGet, "/api/v1/me/contact-block", "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d body=%q", unauth.Code, unauth.Body.String())
	}
	update := editorRequest(t, handler, ownerCookie, http.MethodPut, "/api/v1/me/contact-block", `{"enabled":true,"heading":"Updates","description":"Monthly notes","consent_text":"I agree to receive creator updates.","button_label":"Join"}`)
	if update.Code != http.StatusOK {
		t.Fatalf("update block status=%d body=%q", update.Code, update.Body.String())
	}
	public := editorRequest(t, handler, nil, http.MethodGet, "/api/v1/profiles/growth-owner/contact-block", "")
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), `"heading":"Updates"`) {
		t.Fatalf("public block status=%d body=%q", public.Code, public.Body.String())
	}
	noConsent := editorRequest(t, handler, nil, http.MethodPost, "/api/v1/profiles/growth-owner/contacts", `{"email":"person@example.com","consent":false}`)
	if noConsent.Code != http.StatusBadRequest {
		t.Fatalf("no consent status=%d body=%q", noConsent.Code, noConsent.Body.String())
	}
	created := editorRequest(t, handler, nil, http.MethodPost, "/api/v1/profiles/growth-owner/contacts", `{"email":"person@example.com","consent":true,"campaign":"launch"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("created status=%d body=%q", created.Code, created.Body.String())
	}
	honeypot := editorRequest(t, handler, nil, http.MethodPost, "/api/v1/profiles/growth-owner/contacts", `{"email":"spam@example.com","consent":true,"website":"https://spam.invalid"}`)
	if honeypot.Code != http.StatusCreated {
		t.Fatalf("honeypot status=%d body=%q", honeypot.Code, honeypot.Body.String())
	}
	contacts := editorRequest(t, handler, ownerCookie, http.MethodGet, "/api/v1/me/contacts", "")
	if contacts.Code != http.StatusOK || !strings.Contains(contacts.Body.String(), "person@example.com") || strings.Contains(contacts.Body.String(), "spam@example.com") {
		t.Fatalf("contacts status=%d body=%q", contacts.Code, contacts.Body.String())
	}

	otherCookie := issueEditorSession(t, authService, sender, "growth-other@example.com")
	other, err := authService.Session(context.Background(), otherCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(context.Background(), other.ID, "growth-other"); err != nil {
		t.Fatal(err)
	}
	otherContacts := editorRequest(t, handler, otherCookie, http.MethodGet, "/api/v1/me/contacts", "")
	if otherContacts.Code != http.StatusOK || strings.Contains(otherContacts.Body.String(), "person@example.com") {
		t.Fatalf("other contacts leaked owner data: %q", otherContacts.Body.String())
	}

	retention := editorRequest(t, handler, ownerCookie, http.MethodPut, "/api/v1/me/data-retention", `{"analytics_retention_days":30,"contact_retention_days":90}`)
	if retention.Code != http.StatusOK || !strings.Contains(retention.Body.String(), `"analytics_retention_days":30`) {
		t.Fatalf("retention status=%d body=%q", retention.Code, retention.Body.String())
	}
}
