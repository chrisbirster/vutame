package atproto

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSyncNowRefreshesAndPublishesProfileAndLinks(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_sync", "portable-vuta")
	now := time.Date(2026, 9, 14, 23, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`
		UPDATE profiles SET display_name='Portable Creator',bio='Portable bio',avatar_url='https://example.com/avatar.png',theme='forest',updated_at=? WHERE user_id='usr_sync'
	`, now.Add(-time.Minute).Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`
		INSERT INTO links(id,user_id,label,url,kind,thumbnail_url,featured,position,is_active,created_at,updated_at)
		VALUES('lnk_portable_1','usr_sync','Project','https://example.com/project','project','',1,0,1,?,?)
	`, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-time.Minute).Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }

	store, err := NewStore(db, []byte(strings.Repeat("s", 32)), Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/callback",
	})
	if err != nil { t.Fatal(err) }
	store.SetNow(func() time.Time { return now })
	seedLinkedATAccount(t, store, "usr_sync", "did:plc:sync", "https://pds.example", "https://auth.example/token", "old-access", "old-refresh", now.Add(-time.Minute), true)

	refreshCalls := 0
	published := make(map[string]map[string]any)
	deleted := make([]string, 0)
	nonceCounter := 0
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://auth.example/token":
			refreshCalls++
			form := readForm(t, request)
			if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "old-refresh" {
				t.Fatalf("unexpected refresh form: %v", form)
			}
			response := jsonResponse(http.StatusOK, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"DPoP","scope":"atproto repo:com.vutame.profile repo:com.vutame.link","sub":"did:plc:sync","expires_in":300}`)
			response.Header.Set("DPoP-Nonce", "auth-nonce")
			return response, nil
		case "https://pds.example/xrpc/com.atproto.repo.putRecord":
			assertPDSAuthorization(t, request, "new-access")
			var body struct {
				Repo string `json:"repo"`
				Collection string `json:"collection"`
				RKey string `json:"rkey"`
				Record map[string]any `json:"record"`
			}
			decodeRequestJSON(t, request, &body)
			if body.Repo != "did:plc:sync" { t.Fatalf("repo = %q", body.Repo) }
			key := body.Collection + "/" + body.RKey
			published[key] = body.Record
			nonceCounter++
			response := jsonResponse(http.StatusOK, `{"uri":"at://did:plc:sync/`+body.Collection+`/`+body.RKey+`","cid":"cid-`+body.RKey+`"}`)
			response.Header.Set("DPoP-Nonce", "pds-nonce-"+string(rune('0'+nonceCounter)))
			return response, nil
		case "https://pds.example/xrpc/com.atproto.repo.deleteRecord":
			assertPDSAuthorization(t, request, "new-access")
			var body struct { Collection string `json:"collection"`; RKey string `json:"rkey"` }
			decodeRequestJSON(t, request, &body)
			deleted = append(deleted, body.Collection+"/"+body.RKey)
			nonceCounter++
			response := jsonResponse(http.StatusOK, `{}`)
			response.Header.Set("DPoP-Nonce", "pds-nonce-delete")
			return response, nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})})

	status, err := store.SyncNow(context.Background(), "usr_sync")
	if err != nil { t.Fatalf("SyncNow: %v", err) }
	if refreshCalls != 1 { t.Fatalf("refresh calls = %d, want 1", refreshCalls) }
	if len(status.Records) != 2 { t.Fatalf("records = %+v", status.Records) }
	profileRecord := published[ProfileCollection+"/self"]
	if profileRecord["$type"] != ProfileCollection || profileRecord["displayName"] != "Portable Creator" || profileRecord["handle"] != "portable-vuta" {
		t.Fatalf("profile record = %#v", profileRecord)
	}
	linkRecord := published[LinkCollection+"/lnk_portable_1"]
	if linkRecord["$type"] != LinkCollection || linkRecord["label"] != "Project" || linkRecord["featured"] != true {
		t.Fatalf("link record = %#v", linkRecord)
	}
	var accessEnc, refreshEnc string
	if err := db.QueryRow(`SELECT access_token_enc,refresh_token_enc FROM atproto_accounts WHERE user_id='usr_sync'`).Scan(&accessEnc, &refreshEnc); err != nil { t.Fatal(err) }
	if accessEnc == "new-access" || refreshEnc == "new-refresh" || accessEnc == "" || refreshEnc == "" {
		t.Fatalf("refreshed credentials were not encrypted")
	}

	if _, err := db.Exec(`UPDATE links SET is_active=0,updated_at=? WHERE id='lnk_portable_1'`, now.Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }
	status, err = store.SyncNow(context.Background(), "usr_sync")
	if err != nil { t.Fatalf("second SyncNow: %v", err) }
	if refreshCalls != 1 { t.Fatalf("second sync unexpectedly refreshed: %d", refreshCalls) }
	if len(status.Records) != 1 || status.Records[0].Collection != ProfileCollection {
		t.Fatalf("post-delete records = %+v", status.Records)
	}
	if len(deleted) != 1 || deleted[0] != LinkCollection+"/lnk_portable_1" {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestPDSNonceChallengeIsRetriedWithNonceAndATH(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_nonce", "nonce-user")
	now := time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC)
	store, err := NewStore(db, []byte(strings.Repeat("n", 32)), Config{ClientID: "https://vutame.example/client", RedirectURI: "https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	store.SetNow(func() time.Time { return now })
	seedLinkedATAccount(t, store, "usr_nonce", "did:plc:nonce", "https://pds.example", "https://auth.example/token", "access-nonce", "refresh-nonce", now.Add(time.Minute), true)

	calls := 0
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://pds.example/xrpc/com.atproto.repo.putRecord" {
			t.Fatalf("unexpected request: %s", request.URL)
		}
		calls++
		claims := dpopClaims(t, request.Header.Get("DPoP"))
		if claims["ath"] != tokenHash("access-nonce") { t.Fatalf("ath = %#v", claims["ath"]) }
		if calls == 1 {
			if _, exists := claims["nonce"]; exists { t.Fatalf("first proof unexpectedly had nonce") }
			response := jsonResponse(http.StatusUnauthorized, `{"error":"use_dpop_nonce","message":"nonce required"}`)
			response.Header.Set("DPoP-Nonce", "challenge-nonce")
			return response, nil
		}
		if claims["nonce"] != "challenge-nonce" { t.Fatalf("retry nonce = %#v", claims["nonce"]) }
		var body struct { Collection string `json:"collection"`; RKey string `json:"rkey"` }
		decodeRequestJSON(t, request, &body)
		response := jsonResponse(http.StatusOK, `{"uri":"at://did:plc:nonce/`+body.Collection+`/`+body.RKey+`","cid":"cid-ok"}`)
		response.Header.Set("DPoP-Nonce", "next-nonce")
		return response, nil
	})})

	if _, err := store.SyncNow(context.Background(), "usr_nonce"); err != nil { t.Fatalf("SyncNow: %v", err) }
	if calls != 2 { t.Fatalf("PDS calls = %d, want 2", calls) }
}

func TestSyncNowRequiresPublicationOptIn(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_disabled", "disabled-user")
	now := time.Now().UTC()
	store, err := NewStore(db, []byte(strings.Repeat("d", 32)), Config{ClientID: "https://vutame.example/client", RedirectURI: "https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	seedLinkedATAccount(t, store, "usr_disabled", "did:plc:disabled", "https://pds.example", "https://auth.example/token", "access", "refresh", now.Add(time.Hour), false)
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("network request made while publication disabled: %s", request.URL)
		return nil, nil
	})})
	if _, err := store.SyncNow(context.Background(), "usr_disabled"); err != ErrPublishingDisabled {
		t.Fatalf("SyncNow error = %v, want ErrPublishingDisabled", err)
	}
}

func seedLinkedATAccount(t *testing.T, store *Store, userID, did, pdsURL, tokenEndpoint, accessToken, refreshToken string, expires time.Time, publish bool) {
	t.Helper()
	key, err := generateDPoPKey(); if err != nil { t.Fatal(err) }
	keyBytes, err := marshalDPoPKey(key); if err != nil { t.Fatal(err) }
	accessEnc, err := store.encrypt([]byte(accessToken)); if err != nil { t.Fatal(err) }
	refreshEnc, err := store.encrypt([]byte(refreshToken)); if err != nil { t.Fatal(err) }
	keyEnc, err := store.encrypt(keyBytes); if err != nil { t.Fatal(err) }
	enabled := 0; if publish { enabled = 1 }
	now := store.now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`
		INSERT INTO atproto_accounts(user_id,did,handle,pds_url,issuer,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at,conflict_policy,publish_enabled,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,? ,?,'vutame_wins',?,?,?)
	`, userID, did, "portable.example", pdsURL, "https://auth.example", tokenEndpoint, accessEnc, refreshEnc, keyEnc, DefaultScope, expires.UTC().Format(time.RFC3339Nano), enabled, now, now); err != nil { t.Fatal(err) }
}

func assertPDSAuthorization(t *testing.T, request *http.Request, accessToken string) {
	t.Helper()
	if request.Header.Get("Authorization") != "DPoP "+accessToken { t.Fatalf("authorization = %q", request.Header.Get("Authorization")) }
	claims := dpopClaims(t, request.Header.Get("DPoP"))
	if claims["ath"] != tokenHash(accessToken) { t.Fatalf("DPoP ath = %#v", claims["ath"]) }
	if claims["jti"] == "" { t.Fatalf("DPoP proof missing jti") }
}

func dpopClaims(t *testing.T, proof string) map[string]any {
	t.Helper()
	parts := strings.Split(proof, ".")
	if len(parts) != 3 { t.Fatalf("invalid DPoP proof: %q", proof) }
	payload, err := base64.RawURLEncoding.DecodeString(parts[1]); if err != nil { t.Fatal(err) }
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil { t.Fatal(err) }
	return claims
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func decodeRequestJSON(t *testing.T, request *http.Request, target any) {
	t.Helper()
	data, err := io.ReadAll(request.Body); if err != nil { t.Fatal(err) }
	if err := json.Unmarshal(data, target); err != nil { t.Fatalf("decode request %q: %v", data, err) }
}
