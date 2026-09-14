package atproto

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestOAuthLinkingPersistsEncryptedCredentialsAndConsumesState(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_alice", "alice-vuta")
	store, err := NewStore(db, []byte(strings.Repeat("a", 32)), Config{
		ClientID:    "https://vutame.example/oauth-client-metadata.json",
		RedirectURI: "https://vutame.example/api/v1/me/atproto/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.SetTXTResolver(staticTXTResolver{values: map[string][]string{
		"_atproto.alice.example": {"did=did:plc:alice"},
	}})

	var rawState string
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://plc.directory/did:plc:alice":
			return jsonResponse(http.StatusOK, `{
				"id":"did:plc:alice",
				"alsoKnownAs":["at://alice.example"],
				"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
			}`), nil
		case "https://pds.example/.well-known/oauth-protected-resource":
			return jsonResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, validOAuthMetadataJSON()), nil
		case "https://auth.example/par":
			form := readForm(t, request)
			rawState = form.Get("state")
			response := jsonResponse(http.StatusCreated, `{"request_uri":"urn:ietf:params:oauth:request_uri:test","expires_in":300}`)
			response.Header.Set("DPoP-Nonce", "par-nonce")
			return response, nil
		case "https://auth.example/token":
			form := readForm(t, request)
			if form.Get("grant_type") != "authorization_code" || form.Get("code_verifier") == "" {
				t.Fatalf("unexpected token form: %v", form)
			}
			response := jsonResponse(http.StatusOK, `{
				"access_token":"access-secret",
				"refresh_token":"refresh-secret",
				"token_type":"DPoP",
				"scope":"atproto repo:com.vutame.profile repo:com.vutame.link",
				"sub":"did:plc:alice",
				"expires_in":300
			}`)
			response.Header.Set("DPoP-Nonce", "token-nonce")
			return response, nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})})

	started, err := store.StartOAuth(context.Background(), "usr_alice", "alice.example")
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	if rawState == "" || !strings.Contains(started.AuthorizationURL, "request_uri=") {
		t.Fatalf("OAuth start missing state/PAR URL: state=%q url=%q", rawState, started.AuthorizationURL)
	}

	var storedStateHash, verifierEnc, keyEnc string
	if err := db.QueryRow(`SELECT state_hash,verifier_enc,dpop_key_enc FROM atproto_oauth_states`).Scan(&storedStateHash, &verifierEnc, &keyEnc); err != nil {
		t.Fatal(err)
	}
	if storedStateHash == rawState || verifierEnc == "" || keyEnc == "" {
		t.Fatalf("OAuth state was not protected at rest")
	}

	account, err := store.CompleteOAuth(context.Background(), "usr_alice", rawState, "authorization-code", "https://auth.example")
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if account.DID != "did:plc:alice" || account.Handle != "alice.example" || account.PublishEnabled {
		t.Fatalf("unexpected account: %+v", account)
	}

	var accessEnc, refreshEnc, persistedKey string
	if err := db.QueryRow(`SELECT access_token_enc,refresh_token_enc,dpop_key_enc FROM atproto_accounts WHERE user_id='usr_alice'`).Scan(&accessEnc, &refreshEnc, &persistedKey); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"access": accessEnc, "refresh": refreshEnc, "key": persistedKey} {
		if value == "" || value == "access-secret" || value == "refresh-secret" {
			t.Fatalf("%s credential not encrypted at rest", name)
		}
	}
	var did string
	var verified int
	if err := db.QueryRow(`SELECT atproto_did,verified FROM profiles WHERE user_id='usr_alice'`).Scan(&did, &verified); err != nil {
		t.Fatal(err)
	}
	if did != "did:plc:alice" || verified != 1 {
		t.Fatalf("profile identity not linked: did=%q verified=%d", did, verified)
	}
	var proofCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM verification_requests WHERE user_id='usr_alice' AND method='atproto' AND status='approved'`).Scan(&proofCount); err != nil {
		t.Fatal(err)
	}
	if proofCount != 1 {
		t.Fatalf("AT identity proof count = %d", proofCount)
	}

	if _, err := store.CompleteOAuth(context.Background(), "usr_alice", rawState, "authorization-code", "https://auth.example"); !errors.Is(err, ErrOAuthState) {
		t.Fatalf("reused state error = %v, want ErrOAuthState", err)
	}
}

func TestOAuthStateIsBoundToIssuerAndUser(t *testing.T) {
	db := newATProtoTestDB(t)
	insertATProtoTestProfile(t, db, "usr_alice", "alice-vuta")
	insertATProtoTestProfile(t, db, "usr_bob", "bob-vuta")
	store, err := NewStore(db, []byte(strings.Repeat("b", 32)), Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/callback",
	})
	if err != nil { t.Fatal(err) }
	store.SetTXTResolver(staticTXTResolver{values: map[string][]string{"_atproto.alice.example": {"did=did:plc:alice"}}})
	var rawState string
	store.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://plc.directory/did:plc:alice":
			return jsonResponse(http.StatusOK, `{"id":"did:plc:alice","alsoKnownAs":["at://alice.example"],"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]}`), nil
		case "https://pds.example/.well-known/oauth-protected-resource":
			return jsonResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, validOAuthMetadataJSON()), nil
		case "https://auth.example/par":
			rawState = readForm(t, request).Get("state")
			response := jsonResponse(http.StatusCreated, `{"request_uri":"urn:test","expires_in":300}`)
			response.Header.Set("DPoP-Nonce", "nonce")
			return response, nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})})
	if _, err := store.StartOAuth(context.Background(), "usr_alice", "alice.example"); err != nil { t.Fatal(err) }
	if _, err := store.CompleteOAuth(context.Background(), "usr_bob", rawState, "code", "https://auth.example"); !errors.Is(err, ErrOAuthState) {
		t.Fatalf("wrong-user state error = %v", err)
	}
	if _, err := store.CompleteOAuth(context.Background(), "usr_alice", rawState, "code", "https://evil.example"); !errors.Is(err, ErrOAuthState) {
		t.Fatalf("wrong-issuer state error = %v", err)
	}
}

func newATProtoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "atproto.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil { t.Fatal(err) }
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("apply schema: %v", err) }
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertATProtoTestProfile(t *testing.T, db *sql.DB, userID, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES(?,?,1,?,?)`, userID, userID+"@example.com", now, now); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES(?,?,?,'','','midnight',0,?,?)`, userID, handle, "@"+handle, now, now); err != nil { t.Fatal(err) }
}

func readForm(t *testing.T, request *http.Request) url.Values {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil { t.Fatal(err) }
	values, err := url.ParseQuery(string(body))
	if err != nil { t.Fatal(err) }
	return values
}

func validOAuthMetadataJSON() string {
	return `{
		"issuer":"https://auth.example",
		"authorization_endpoint":"https://auth.example/authorize",
		"token_endpoint":"https://auth.example/token",
		"pushed_authorization_request_endpoint":"https://auth.example/par",
		"response_types_supported":["code"],
		"grant_types_supported":["authorization_code","refresh_token"],
		"code_challenge_methods_supported":["S256"],
		"token_endpoint_auth_methods_supported":["none","private_key_jwt"],
		"token_endpoint_auth_signing_alg_values_supported":["ES256"],
		"scopes_supported":["atproto","repo:com.vutame.profile","repo:com.vutame.link"],
		"authorization_response_iss_parameter_supported":true,
		"require_pushed_authorization_requests":true,
		"dpop_signing_alg_values_supported":["ES256"],
		"client_id_metadata_document_supported":true
	}`
}
