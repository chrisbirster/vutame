package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEditorAPIFeaturedScheduledLinks(t *testing.T) {
	handler, service, sender := newEditorAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "schedule@example.com")

	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"schedule-user"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", `{"label":"Normal","url":"https://example.com/normal","kind":"website","is_active":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("normal create status=%d body=%q", rec.Code, rec.Body.String())
	}

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"label":"Featured","url":"https://example.com/featured","kind":"project","featured":true,"visible_from":%q,"visible_until":%q,"is_active":true}`, from, until)
	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", body)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"featured":true`) {
		t.Fatalf("featured create status=%d body=%q", rec.Code, rec.Body.String())
	}

	future := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	body = fmt.Sprintf(`{"label":"Future","url":"https://example.com/future","kind":"website","visible_from":%q,"is_active":true}`, future)
	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("future create status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/profile", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"label":"Future"`) {
		t.Fatalf("owned profile should retain future link: status=%d body=%q", rec.Code, rec.Body.String())
	}

	publicReq := editorRequest(t, handler, nil, http.MethodGet, "/api/v1/profiles/schedule-user", "")
	if publicReq.Code != http.StatusOK {
		t.Fatalf("public profile status=%d body=%q", publicReq.Code, publicReq.Body.String())
	}
	publicBody := publicReq.Body.String()
	if strings.Contains(publicBody, `"label":"Future"`) {
		t.Fatalf("future link leaked into public profile: %q", publicBody)
	}
	featuredIndex := strings.Index(publicBody, `"label":"Featured"`)
	normalIndex := strings.Index(publicBody, `"label":"Normal"`)
	if featuredIndex < 0 || normalIndex < 0 || featuredIndex > normalIndex {
		t.Fatalf("featured link was not pinned: %q", publicBody)
	}

	badBody := `{"label":"Bad","url":"https://example.com/bad","kind":"website","visible_from":"2026-09-13T18:00:00Z","visible_until":"2026-09-13T17:00:00Z","is_active":true}`
	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", badBody)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "visible_until must be after visible_from") {
		t.Fatalf("invalid schedule status=%d body=%q", rec.Code, rec.Body.String())
	}
}
