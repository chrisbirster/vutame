package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestATProtoPublicAppViewProfileAndSearch(t *testing.T) {
	handler, _, _, db := newATProtoAPIHarness(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO atproto_indexed_profiles(did,handle,display_name,bio,avatar_url,theme,verified,record_json,indexed_at)
		VALUES('did:plc:indexed','portable-vuta','Portable Creator','From a PDS','','paper',0,'{}',?)
	`, now); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`
		INSERT INTO atproto_indexed_links(did,rkey,label,url,kind,thumbnail_url,featured,position,record_json,indexed_at)
		VALUES('did:plc:indexed','link-one','Project','https://example.com/project','project','',1,0,'{}',?)
	`, now); err != nil { t.Fatal(err) }

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/atproto/profiles/did:plc:indexed", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"display_name":"Portable Creator"`) || !strings.Contains(rec.Body.String(), `"label":"Project"`) {
		t.Fatalf("portable profile status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/atproto/search?q=portable&limit=10", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"did":"did:plc:indexed"`) {
		t.Fatalf("portable search status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestATProtoSearchDoesNotDuplicateLinkedLocalDID(t *testing.T) {
	handler, service, sender, db := newATProtoAPIHarness(t)
	cookie := issueEditorSession(t, service, sender, "linked-index@example.com")
	rec := editorRequest(t, handler, cookie, http.MethodPost, "/api/v1/me/profile/claim", `{"handle":"linked-index"}`)
	if rec.Code != http.StatusCreated { t.Fatalf("claim status=%d body=%q", rec.Code, rec.Body.String()) }
	if _, err := db.Exec(`UPDATE profiles SET atproto_did='did:plc:local' WHERE handle='linked-index'`); err != nil { t.Fatal(err) }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO atproto_indexed_profiles(did,handle,display_name,bio,avatar_url,theme,verified,record_json,indexed_at) VALUES('did:plc:local','linked-index','Linked Local','','','midnight',1,'{}',?)`, now); err != nil { t.Fatal(err) }

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/atproto/search?q=linked", nil))
	if rec.Code != http.StatusOK { t.Fatalf("search status=%d body=%q", rec.Code, rec.Body.String()) }
	if strings.Contains(rec.Body.String(), `did:plc:local`) { t.Fatalf("linked local DID was duplicated in portable search: %s", rec.Body.String()) }
}
