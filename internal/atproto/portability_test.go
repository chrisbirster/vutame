package atproto

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPortabilityLifecycleAuthorizePublishIngestRender(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_portable", "portable-vuta")
	now := time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`UPDATE profiles SET display_name='Portable Creator',bio='portable everywhere',updated_at=? WHERE user_id='usr_portable'`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO links(id,user_id,label,url,kind,thumbnail_url,featured,position,is_active,visible_from,visible_until,created_at,updated_at) VALUES('lnk_portable','usr_portable','Project','https://example.com/project','project','',1,0,1,NULL,NULL,?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(db, []byte(strings.Repeat("p", 32)), Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/api/v1/me/atproto/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.SetNow(func() time.Time { return now })
	store.SetTXTResolver(staticTXTResolver{values: map[string][]string{
		"_atproto.portable.example": {"did=did:plc:portable"},
	}})

	var rawState string
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://plc.directory/did:plc:portable":
			return jsonResponse(http.StatusOK, `{"id":"did:plc:portable","alsoKnownAs":["at://portable.example"],"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]}`), nil
		case "https://pds.example/.well-known/oauth-protected-resource":
			return jsonResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, validOAuthMetadataJSON()), nil
		case "https://auth.example/par":
			rawState = readForm(t, request).Get("state")
			response := jsonResponse(http.StatusCreated, `{"request_uri":"urn:test:portable","expires_in":300}`)
			response.Header.Set("DPoP-Nonce", "par-nonce")
			return response, nil
		case "https://auth.example/token":
			response := jsonResponse(http.StatusOK, `{"access_token":"access","refresh_token":"refresh","token_type":"DPoP","scope":"atproto repo:com.vutame.profile repo:com.vutame.link","sub":"did:plc:portable","expires_in":3600}`)
			response.Header.Set("DPoP-Nonce", "token-nonce")
			return response, nil
		default:
			t.Fatalf("unexpected OAuth request %s", request.URL)
			return nil, nil
		}
	})})

	if _, err := store.StartOAuth(context.Background(), "usr_portable", "portable.example"); err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	if rawState == "" {
		t.Fatal("OAuth state not captured")
	}
	if _, err := store.CompleteOAuth(context.Background(), "usr_portable", rawState, "code", "https://auth.example"); err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if _, err := store.UpdateSettings(context.Background(), "usr_portable", "vutame_wins", true); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	pds := newFakePDS(t)
	store.SetHTTPClient(&http.Client{Transport: pds})
	report, err := store.Sync(context.Background(), "usr_portable")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Published != 2 || report.Deleted != 0 || len(report.Conflicts) != 0 {
		t.Fatalf("unexpected sync report: %+v", report)
	}

	profileRecord, ok := pds.record(ProfileCollection, "self")
	if !ok {
		t.Fatal("published profile record missing")
	}
	linkRKey := stableLinkRKey("lnk_portable")
	linkRecord, ok := pds.record(LinkCollection, linkRKey)
	if !ok {
		t.Fatal("published link record missing")
	}

	profileEvent := JetstreamEvent{DID: "did:plc:portable", Kind: "commit", Cursor: 101, Commit: &JetstreamCommit{Operation: "update", Collection: ProfileCollection, RKey: "self", CID: profileRecord.CID, Record: profileRecord.Value}}
	profilePayload, err := json.Marshal(profileEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProcessJetstreamEvent(context.Background(), profilePayload); err != nil {
		t.Fatalf("ingest profile: %v", err)
	}
	linkEvent := JetstreamEvent{DID: "did:plc:portable", Kind: "commit", Cursor: 102, Commit: &JetstreamCommit{Operation: "update", Collection: LinkCollection, RKey: linkRKey, CID: linkRecord.CID, Record: linkRecord.Value}}
	linkPayload, err := json.Marshal(linkEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProcessJetstreamEvent(context.Background(), linkPayload); err != nil {
		t.Fatalf("ingest link: %v", err)
	}

	indexed, err := store.IndexedProfile(context.Background(), "did:plc:portable")
	if err != nil {
		t.Fatalf("IndexedProfile: %v", err)
	}
	if indexed.DID != "did:plc:portable" || indexed.DisplayName != "Portable Creator" || len(indexed.Links) != 1 {
		t.Fatalf("unexpected AppView profile: %+v", indexed)
	}
	if indexed.Links[0].Label != "Project" || indexed.Links[0].URL != "https://example.com/project" {
		t.Fatalf("unexpected AppView link: %+v", indexed.Links[0])
	}
	cursor, err := store.jetstreamCursor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cursor != 102 {
		t.Fatalf("cursor=%d want 102", cursor)
	}
}
