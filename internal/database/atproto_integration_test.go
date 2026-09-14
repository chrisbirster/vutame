package database_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/atproto"
	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "turso.tech/database/tursogo"
)

type atprotoTXTResolver struct{}

func (atprotoTXTResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if name == "_atproto.turso.example" {
		return []string{"did=did:plc:tursoportable"}, nil
	}
	return nil, &netDNSError{}
}

type netDNSError struct{}
func (*netDNSError) Error() string { return "not found" }

type atprotoRoundTrip func(*http.Request) (*http.Response, error)
func (fn atprotoRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestATProtoOAuthStateRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil { t.Fatal(err) }
	if _, err := db.Exec(dbschema.SQL); err != nil { t.Fatalf("apply schema: %v", err) }

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES('usr_atproto_turso','atproto-turso@example.com',1,?,?)`, now, now); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,created_at,updated_at) VALUES('usr_atproto_turso','atproto-turso','ATProto Turso','','','midnight',0,?,?)`, now, now); err != nil { t.Fatal(err) }

	store, err := atproto.NewStore(db, []byte(strings.Repeat("t", 32)), atproto.Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/api/v1/me/atproto/oauth/callback",
	})
	if err != nil { t.Fatal(err) }
	store.SetTXTResolver(atprotoTXTResolver{})
	store.SetHTTPClient(&http.Client{Transport: atprotoRoundTrip(func(request *http.Request) (*http.Response, error) {
		var status = http.StatusOK
		var body string
		switch request.URL.String() {
		case "https://plc.directory/did:plc:tursoportable":
			body = `{"id":"did:plc:tursoportable","alsoKnownAs":["at://turso.example"],"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]}`
		case "https://pds.example/.well-known/oauth-protected-resource":
			body = `{"authorization_servers":["https://auth.example"]}`
		case "https://auth.example/.well-known/oauth-authorization-server":
			body = `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token","pushed_authorization_request_endpoint":"https://auth.example/par","response_types_supported":["code"],"grant_types_supported":["authorization_code","refresh_token"],"code_challenge_methods_supported":["S256"],"token_endpoint_auth_methods_supported":["none","private_key_jwt"],"token_endpoint_auth_signing_alg_values_supported":["ES256"],"scopes_supported":["atproto"],"authorization_response_iss_parameter_supported":true,"require_pushed_authorization_requests":true,"dpop_signing_alg_values_supported":["ES256"],"client_id_metadata_document_supported":true}`
		case "https://auth.example/par":
			status = http.StatusCreated
			body = `{"request_uri":"urn:turso:par","expires_in":300}`
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
		response := &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		if request.URL.Path == "/par" { response.Header.Set("DPoP-Nonce", "turso-nonce") }
		return response, nil
	})})

	started, err := store.StartOAuth(context.Background(), "usr_atproto_turso", "turso.example")
	if err != nil { t.Fatalf("StartOAuth on Turso engine: %v", err) }
	if started.DID != "did:plc:tursoportable" || started.Handle != "turso.example" || !strings.Contains(started.AuthorizationURL, "request_uri=") {
		t.Fatalf("unexpected OAuth start: %+v", started)
	}
	var count int
	var verifierEnc string
	if err := db.QueryRow(`SELECT COUNT(*),MIN(verifier_enc) FROM atproto_oauth_states WHERE user_id='usr_atproto_turso'`).Scan(&count, &verifierEnc); err != nil { t.Fatal(err) }
	if count != 1 || verifierEnc == "" {
		t.Fatalf("OAuth state not persisted on Turso engine: count=%d verifier=%q", count, verifierEnc)
	}
}
