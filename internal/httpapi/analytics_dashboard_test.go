package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestAnalyticsDashboardIsOwnedBySession(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "analytics-dashboard.db"))
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
	secret := []byte(strings.Repeat("a", 32))
	authService, err := auth.NewService(authStore, sender, secret, auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := analytics.NewService(db, secret)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(http.NotFoundHandler(), profiles, Options{Auth: authService, Editor: profiles, Analytics: metrics, CookieSecure: false})

	ownerCookie := issueEditorSession(t, authService, sender, "analytics-owner@example.com")
	owner, err := authService.Session(context.Background(), ownerCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(context.Background(), owner.ID, "analytics-owner"); err != nil {
		t.Fatal(err)
	}
	link, err := profiles.CreateLink(context.Background(), owner.ID, profile.LinkInput{
		Label: "Project", URL: "https://example.com/project", Kind: "project", IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := metrics.RecordProfileView(context.Background(), owner.ID, analytics.Metadata{VisitorHash: "owner-day-a", ReferrerHost: "google.com", DeviceClass: analytics.DeviceDesktop}); err != nil {
		t.Fatal(err)
	}
	if err := metrics.RecordLinkClick(context.Background(), owner.ID, link.ID, analytics.Metadata{VisitorHash: "owner-day-a", ReferrerHost: "vuta.me", DeviceClass: analytics.DeviceDesktop}); err != nil {
		t.Fatal(err)
	}

	otherCookie := issueEditorSession(t, authService, sender, "analytics-other@example.com")
	other, err := authService.Session(context.Background(), otherCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(context.Background(), other.ID, "analytics-other"); err != nil {
		t.Fatal(err)
	}
	if err := metrics.RecordProfileView(context.Background(), other.ID, analytics.Metadata{VisitorHash: "other-day-a", DeviceClass: analytics.DeviceMobile}); err != nil {
		t.Fatal(err)
	}

	unauthenticated := editorRequest(t, handler, nil, http.MethodGet, "/api/v1/me/analytics?days=7", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%q", unauthenticated.Code, unauthenticated.Body.String())
	}
	badDays := editorRequest(t, handler, ownerCookie, http.MethodGet, "/api/v1/me/analytics?days=91", "")
	if badDays.Code != http.StatusBadRequest {
		t.Fatalf("bad days status=%d body=%q", badDays.Code, badDays.Body.String())
	}
	response := editorRequest(t, handler, ownerCookie, http.MethodGet, "/api/v1/me/analytics?days=7", "")
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"profile_views":1`) || !strings.Contains(body, `"link_clicks":1`) || !strings.Contains(body, `"unique_visitors":1`) || !strings.Contains(body, `"label":"Project"`) {
		t.Fatalf("dashboard body=%q", body)
	}
	if strings.Contains(body, "analytics-other") || strings.Contains(body, `"profile_views":2`) {
		t.Fatalf("dashboard leaked another creator's data: %q", body)
	}
}
