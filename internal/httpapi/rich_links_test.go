package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestEditorAPIRichLinks(t *testing.T) {
	handler, service, sender := newEditorAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "richlinks@example.com")

	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"rich-links"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", `{"label":"Watch","url":"https://youtube.com/watch?v=1","kind":"youtube","thumbnail_url":"https://example.com/watch.jpg","is_active":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rich link status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"kind":"youtube"`) || !strings.Contains(rec.Body.String(), `"thumbnail_url":"https://example.com/watch.jpg"`) {
		t.Fatalf("rich link response=%q", rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", `{"label":"Nope","url":"https://example.com","kind":"discord","is_active":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unsupported link kind") {
		t.Fatalf("invalid kind status=%d body=%q", rec.Code, rec.Body.String())
	}

	reqBody := `{"label":"Nope","url":"https://example.com","kind":"website","thumbnail_url":"javascript:alert(1)","is_active":true}`
	rec = editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/links", reqBody)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "thumbnail URL") {
		t.Fatalf("invalid thumbnail status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = editorRequest(t, handler, cookie, http.MethodGet, "/api/v1/me/profile", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"thumbnail_url":"https://example.com/watch.jpg"`) {
		t.Fatalf("owned rich profile status=%d body=%q", rec.Code, rec.Body.String())
	}
}
