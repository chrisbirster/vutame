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
	"github.com/chrisbirster/vutame/internal/linkpreview"
	"github.com/chrisbirster/vutame/internal/profile"
)

type fakeLinkPreviewer struct {
	metadata linkpreview.Metadata
	err      error
}

func (f fakeLinkPreviewer) Fetch(context.Context, string) (linkpreview.Metadata, error) {
	return f.metadata, f.err
}

func TestLinkPreviewEndpointReturnsSanitizedMetadata(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "preview.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		db.Close()
		t.Fatal(err)
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
	service, err := auth.NewService(authStore, sender, []byte(strings.Repeat("p", 32)), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = authStore.Close()
		_ = profiles.Close()
	})
	cookie := issueEditorSession(t, service, sender, "preview@example.com")
	previewer := fakeLinkPreviewer{metadata: linkpreview.Metadata{
		URL:         "https://github.com/example/project",
		Title:       "Example Project",
		Description: "A useful project.",
		ImageURL:    "https://example.com/card.png",
		Provider:    "github",
	}}
	handler := New(http.NotFoundHandler(), profiles, Options{Auth: service, Editor: profiles, Previewer: previewer, CookieSecure: false})
	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/link-preview", `{"url":"https://github.com/example/project"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	for _, expected := range []string{`"title":"Example Project"`, `"provider":"github"`, `"image_url":"https://example.com/card.png"`} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Fatalf("body missing %q: %s", expected, rec.Body.String())
		}
	}
}

func TestLinkPreviewEndpointRequiresAuthentication(t *testing.T) {
	handler, _, _ := newEditorAPIHarness(t)
	rec := editorRequest(t, handler, nil, http.MethodPost, "/api/v1/me/link-preview", `{"url":"https://example.com"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
