package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/profile"
)

func TestProfileHTMLAddsSEOAndNoScriptProfile(t *testing.T) {
	profiles := profile.NewMemoryStore([]profile.Profile{{
		ID:          "usr_html",
		Handle:      "alice",
		DisplayName: "Alice <Maker>",
		Bio:         "Builds & ships things.",
		AvatarURL:   "/media/avatar.png",
		Links: []profile.Link{{
			ID: "lnk_one", Label: "Project & notes", URL: "https://example.com/project", Kind: "project", IsActive: true,
		}},
	}})
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><meta name="description" content="base"><title>Base</title></head><body><div id="root"></div><script src="/app.js"></script></body></html>`))
	})
	handler := profileHTMLHandler(base, profiles, Options{ProfileOrigin: "https://vuta.me"})
	req := httptest.NewRequest(http.MethodGet, "/@alice", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, expected := range []string{
		`<title>Alice &lt;Maker&gt; (@alice) — Vutame</title>`,
		`<link rel="canonical" href="https://vuta.me/@alice">`,
		`<meta property="og:url" content="https://vuta.me/@alice">`,
		`<meta property="og:image" content="https://vuta.me/media/avatar.png">`,
		`<noscript>`,
		`Alice &lt;Maker&gt;`,
		`Builds &amp; ships things.`,
		`Project &amp; notes`,
		`<script src="/app.js"></script>`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, `content="base"`) {
		t.Fatalf("base description was not replaced: %s", body)
	}
}

func TestProfileHTMLFallsBackToSPAForUnknownHandle(t *testing.T) {
	profiles := profile.NewMemoryStore(nil)
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("spa-fallback"))
	})
	handler := profileHTMLHandler(base, profiles, Options{ProfileOrigin: "https://vuta.me"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/@available", nil))
	if rec.Body.String() != "spa-fallback" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestProfileHTMLHeadHasNoBody(t *testing.T) {
	profiles := profile.NewMemoryStore([]profile.Profile{{ID: "usr_head", Handle: "headtest"}})
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Base</title></head><body><div id="root"></div></body></html>`))
	})
	handler := profileHTMLHandler(base, profiles, Options{ProfileOrigin: "https://vuta.me"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/@headtest", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
