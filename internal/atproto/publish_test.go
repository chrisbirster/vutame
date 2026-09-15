package atproto

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccessSessionRefreshesAndRotatesCredentials(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_refresh", "refresh-vuta")
	store, err := NewStore(db, []byte(strings.Repeat("r", 32)), Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/callback",
	})
	if err != nil { t.Fatal(err) }
	now := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	insertLinkedATAccount(t, store, "usr_refresh", "did:plc:refresh", "https://pds.example", "https://auth.example/token", key, "old-access", "old-refresh", now.Add(-time.Minute), "vutame_wins", true)

	var refreshCalls int
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://auth.example/token" { t.Fatalf("unexpected URL %s", request.URL) }
		if request.Header.Get("DPoP") == "" { t.Fatal("refresh missing DPoP proof") }
		form := readForm(t, request)
		if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "old-refresh" { t.Fatalf("unexpected refresh form %v", form) }
		refreshCalls++
		response := jsonResponse(http.StatusOK, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"DPoP","scope":"atproto repo:com.vutame.profile repo:com.vutame.link","sub":"did:plc:refresh","expires_in":600}`)
		response.Header.Set("DPoP-Nonce", "refresh-nonce")
		return response, nil
	})})

	session, err := store.accessSession(context.Background(), "usr_refresh")
	if err != nil { t.Fatalf("accessSession: %v", err) }
	if refreshCalls != 1 || session.AccessToken != "new-access" || session.RefreshToken != "new-refresh" { t.Fatalf("unexpected refresh result calls=%d session=%+v", refreshCalls, session) }
	var accessEnc, refreshEnc string
	if err := db.QueryRow(`SELECT access_token_enc,refresh_token_enc FROM atproto_accounts WHERE user_id='usr_refresh'`).Scan(&accessEnc, &refreshEnc); err != nil { t.Fatal(err) }
	if accessEnc == "new-access" || refreshEnc == "new-refresh" || accessEnc == "" || refreshEnc == "" { t.Fatal("rotated OAuth tokens were not encrypted") }
}

func TestSyncPublishesPublicRecordsAndDeletesStaleLinks(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_sync", "sync-vuta")
	now := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)
	_, err := db.Exec(`UPDATE profiles SET display_name='Sync Creator',bio='portable profile',updated_at=? WHERE user_id='usr_sync'`, now.Format(time.RFC3339Nano))
	if err != nil { t.Fatal(err) }
	_, err = db.Exec(`INSERT INTO links(id,user_id,label,url,kind,thumbnail_url,featured,position,is_active,visible_from,visible_until,created_at,updated_at) VALUES('lnk_public','usr_sync','Public','https://example.com','website','',1,0,1,NULL,NULL,?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil { t.Fatal(err) }
	_, err = db.Exec(`INSERT INTO links(id,user_id,label,url,kind,thumbnail_url,featured,position,is_active,visible_from,visible_until,created_at,updated_at) VALUES('lnk_hidden','usr_sync','Hidden','https://hidden.example','website','',0,1,0,NULL,NULL,?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil { t.Fatal(err) }

	store, err := NewStore(db, []byte(strings.Repeat("s", 32)), Config{ClientID:"https://vutame.example/oauth-client-metadata.json",RedirectURI:"https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	store.SetNow(func() time.Time { return now })
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	insertLinkedATAccount(t, store, "usr_sync", "did:plc:sync", "https://pds.example", "https://auth.example/token", key, "access", "refresh", now.Add(time.Hour), "vutame_wins", true)
	pds := newFakePDS(t)
	store.SetHTTPClient(&http.Client{Transport: pds})

	report, err := store.Sync(context.Background(), "usr_sync")
	if err != nil { t.Fatalf("Sync: %v", err) }
	if report.Published != 2 || report.Deleted != 0 || len(report.Conflicts) != 0 { t.Fatalf("unexpected sync report %+v", report) }
	if _, ok := pds.record(ProfileCollection, "self"); !ok { t.Fatal("profile record not published") }
	publicRKey := stableLinkRKey("lnk_public")
	if _, ok := pds.record(LinkCollection, publicRKey); !ok { t.Fatal("public link not published") }
	if _, ok := pds.record(LinkCollection, stableLinkRKey("lnk_hidden")); ok { t.Fatal("inactive link should not publish") }
	var managed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM atproto_records WHERE user_id='usr_sync'`).Scan(&managed); err != nil { t.Fatal(err) }
	if managed != 2 { t.Fatalf("managed records=%d want 2", managed) }

	if _, err := db.Exec(`UPDATE links SET is_active=0,updated_at=? WHERE id='lnk_public'`, now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }
	now = now.Add(2 * time.Minute)
	report, err = store.Sync(context.Background(), "usr_sync")
	if err != nil { t.Fatalf("second Sync: %v", err) }
	if report.Deleted != 1 { t.Fatalf("deleted=%d want 1 report=%+v", report.Deleted, report) }
	if _, ok := pds.record(LinkCollection, publicRKey); ok { t.Fatal("stale portable link was not deleted") }
}

func TestSyncPDSWinsPreservesRemoteDivergence(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_conflict", "conflict-vuta")
	now := time.Date(2026, 9, 14, 22, 0, 0, 0, time.UTC)
	store, err := NewStore(db, []byte(strings.Repeat("c", 32)), Config{ClientID:"https://vutame.example/oauth-client-metadata.json",RedirectURI:"https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	store.SetNow(func() time.Time { return now })
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	insertLinkedATAccount(t, store, "usr_conflict", "did:plc:conflict", "https://pds.example", "https://auth.example/token", key, "access", "refresh", now.Add(time.Hour), "pds_wins", true)

	oldPayload := `{"$type":"com.vutame.profile","displayName":"Old","handle":"conflict-vuta","theme":"midnight","verified":false,"updatedAt":"2026-09-14T20:00:00Z"}`
	if _, err := db.Exec(`INSERT INTO atproto_records(user_id,collection,rkey,cid,local_updated_at,synced_at,payload) VALUES('usr_conflict',?,'self','cid-old','old','old',?)`, ProfileCollection, oldPayload); err != nil { t.Fatal(err) }
	pds := newFakePDS(t)
	pds.setRecord(ProfileCollection, "self", "cid-remote", map[string]any{"$type":ProfileCollection,"displayName":"Remote","handle":"remote-vuta","theme":"paper","verified":false,"updatedAt":"2026-09-14T21:00:00Z"})
	store.SetHTTPClient(&http.Client{Transport: pds})

	report, err := store.Sync(context.Background(), "usr_conflict")
	if err != nil { t.Fatalf("Sync: %v", err) }
	if report.Published != 0 || len(report.Conflicts) != 1 { t.Fatalf("unexpected conflict report %+v", report) }
	remote, _ := pds.record(ProfileCollection, "self")
	if remote.CID != "cid-remote" || stringField(remote.Value, "displayName") != "Remote" { t.Fatalf("remote record was overwritten: %+v", remote) }
	indexed, err := store.IndexedProfile(context.Background(), "did:plc:conflict")
	if err != nil { t.Fatalf("indexed conflict winner: %v", err) }
	if indexed.DisplayName != "Remote" { t.Fatalf("AppView did not preserve PDS winner: %+v", indexed) }
}

func insertLinkedATAccount(t *testing.T, store *Store, userID, did, pdsURL, tokenEndpoint string, key *ecdsa.PrivateKey, access, refresh string, expires time.Time, policy string, enabled bool) {
	t.Helper()
	accessEnc, err := store.encrypt([]byte(access)); if err != nil { t.Fatal(err) }
	refreshEnc, err := store.encrypt([]byte(refresh)); if err != nil { t.Fatal(err) }
	keyBytes, err := marshalDPoPKey(key); if err != nil { t.Fatal(err) }
	keyEnc, err := store.encrypt(keyBytes); if err != nil { t.Fatal(err) }
	flag := 0; if enabled { flag = 1 }
	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err = store.db.Exec(`INSERT INTO atproto_accounts(user_id,did,handle,pds_url,issuer,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at,conflict_policy,publish_enabled,created_at,updated_at) VALUES(?,?,'alice.example',?,'https://auth.example',?,?,?,?,?,?,?, ?,?,?)`, userID,did,pdsURL,tokenEndpoint,accessEnc,refreshEnc,keyEnc,DefaultScope,expires.Format(time.RFC3339Nano),policy,flag,now,now)
	if err != nil { t.Fatal(err) }
	_, _ = store.db.Exec(`UPDATE profiles SET atproto_did=?,verified=1 WHERE user_id=?`, did, userID)
}

type fakePDSRecord struct {
	CID   string
	Value map[string]any
}

type fakePDS struct {
	t *testing.T
	mu sync.Mutex
	records map[string]fakePDSRecord
	next int
}

func newFakePDS(t *testing.T) *fakePDS { return &fakePDS{t:t,records:map[string]fakePDSRecord{}} }
func (p *fakePDS) key(collection,rkey string) string { return collection+"/"+rkey }
func (p *fakePDS) record(collection,rkey string) (fakePDSRecord,bool) { p.mu.Lock(); defer p.mu.Unlock(); item,ok:=p.records[p.key(collection,rkey)]; return item,ok }
func (p *fakePDS) setRecord(collection,rkey,cid string,value map[string]any) { p.mu.Lock(); defer p.mu.Unlock(); p.records[p.key(collection,rkey)] = fakePDSRecord{CID:cid,Value:value} }

func (p *fakePDS) RoundTrip(request *http.Request) (*http.Response,error) {
	if request.Header.Get("Authorization") == "" || request.Header.Get("DPoP") == "" { p.t.Fatalf("PDS request missing DPoP auth: %s", request.URL) }
	method := request.URL.Path
	switch method {
	case "/xrpc/com.atproto.repo.getRecord":
		collection := request.URL.Query().Get("collection"); rkey := request.URL.Query().Get("rkey")
		item,ok := p.record(collection,rkey)
		if !ok { return nonceJSON(http.StatusBadRequest, `{"error":"RecordNotFound"}`),nil }
		data,_ := json.Marshal(map[string]any{"uri":"at://did:plc:test/"+collection+"/"+rkey,"cid":item.CID,"value":item.Value})
		return nonceJSON(http.StatusOK,string(data)),nil
	case "/xrpc/com.atproto.repo.putRecord":
		var body map[string]any; decodeRequestJSON(p.t,request,&body)
		collection,_ := body["collection"].(string); rkey,_ := body["rkey"].(string); record,_ := body["record"].(map[string]any)
		p.mu.Lock(); defer p.mu.Unlock()
		key := p.key(collection,rkey)
		if swap,ok := body["swapRecord"].(string); ok && swap != "" { if current,exists:=p.records[key]; !exists || current.CID != swap { return nonceJSON(http.StatusBadRequest,`{"error":"InvalidSwap"}`),nil } }
		p.next++; cid := fmt.Sprintf("cid-%d",p.next); p.records[key]=fakePDSRecord{CID:cid,Value:record}
		return nonceJSON(http.StatusOK,fmt.Sprintf(`{"uri":"at://did:plc:test/%s/%s","cid":"%s"}`,collection,rkey,cid)),nil
	case "/xrpc/com.atproto.repo.deleteRecord":
		var body map[string]any; decodeRequestJSON(p.t,request,&body)
		collection,_ := body["collection"].(string); rkey,_ := body["rkey"].(string)
		p.mu.Lock(); defer p.mu.Unlock(); key:=p.key(collection,rkey)
		if swap,ok := body["swapRecord"].(string); ok && swap!="" { if current,exists:=p.records[key]; !exists || current.CID != swap { return nonceJSON(http.StatusBadRequest,`{"error":"InvalidSwap"}`),nil } }
		delete(p.records,key); return nonceJSON(http.StatusOK,`{}`),nil
	default:
		p.t.Fatalf("unexpected PDS request %s", request.URL)
		return nil,nil
	}
}

func nonceJSON(status int,body string) *http.Response { response:=jsonResponse(status,body); response.Header.Set("DPoP-Nonce","pds-nonce"); return response }
func decodeRequestJSON(t *testing.T,request *http.Request,target any) { t.Helper(); data,err:=io.ReadAll(request.Body); if err!=nil { t.Fatal(err) }; if err:=json.Unmarshal(data,target); err!=nil { t.Fatalf("decode request JSON: %v body=%s",err,data) } }

var _ = url.Values{}
