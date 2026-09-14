package atproto

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type staticTXTResolver struct {
	values map[string][]string
}

func (r staticTXTResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if values, ok := r.values[name]; ok {
		return values, nil
	}
	return nil, errors.New("not found")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testStoreWithHTTP(t *testing.T, resolver TXTResolver, fn roundTripFunc) *Store {
	t.Helper()
	return &Store{
		config:   Config{},
		resolver: resolver,
		client:   &http.Client{Transport: fn},
	}
}

func TestResolveIdentityRequiresBidirectionalHandleClaim(t *testing.T) {
	resolver := staticTXTResolver{values: map[string][]string{
		"_atproto.alice.example": {"did=did:plc:alice"},
	}}
	store := testStoreWithHTTP(t, resolver, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "plc.directory" {
			t.Fatalf("unexpected request: %s", request.URL)
		}
		return jsonResponse(http.StatusOK, `{
			"id":"did:plc:alice",
			"alsoKnownAs":["at://someone-else.example"],
			"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
		}`), nil
	})

	_, err := store.ResolveIdentity(context.Background(), "alice.example")
	if !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("ResolveIdentity error = %v, want ErrInvalidIdentity", err)
	}
}

func TestResolveIdentityAcceptsBidirectionalHandle(t *testing.T) {
	resolver := staticTXTResolver{values: map[string][]string{
		"_atproto.alice.example": {"did=did:plc:alice"},
	}}
	store := testStoreWithHTTP(t, resolver, func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"id":"did:plc:alice",
			"alsoKnownAs":["at://alice.example"],
			"service":[{"id":"did:plc:alice#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
		}`), nil
	})

	identity, err := store.ResolveIdentity(context.Background(), "@Alice.Example")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.DID != "did:plc:alice" || identity.Handle != "alice.example" || identity.PDSURL != "https://pds.example" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestResolveDIDSuppressesUnverifiedClaimedHandle(t *testing.T) {
	resolver := staticTXTResolver{values: map[string][]string{
		"_atproto.alice.example": {"did=did:plc:different"},
	}}
	store := testStoreWithHTTP(t, resolver, func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"id":"did:plc:alice",
			"alsoKnownAs":["at://alice.example"],
			"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
		}`), nil
	})

	identity, err := store.ResolveIdentity(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if identity.Handle != "" {
		t.Fatalf("unverified handle was exposed: %q", identity.Handle)
	}
}

func TestResolveIdentityRequiresMatchingPDSServiceShape(t *testing.T) {
	resolver := staticTXTResolver{values: map[string][]string{
		"_atproto.alice.example": {"did=did:plc:alice"},
	}}
	store := testStoreWithHTTP(t, resolver, func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"id":"did:plc:alice",
			"alsoKnownAs":["at://alice.example"],
			"service":[{"id":"#something_else","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
		}`), nil
	})

	_, err := store.ResolveIdentity(context.Background(), "alice.example")
	if !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("ResolveIdentity error = %v, want ErrInvalidIdentity", err)
	}
}

func TestDidWebOnlyAllowsHostnameLevelIdentifiers(t *testing.T) {
	if _, err := didDocumentURL("did:web:example.com:user", false); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("path-based did:web error = %v, want ErrInvalidIdentity", err)
	}
	url, err := didDocumentURL("did:web:example.com", false)
	if err != nil {
		t.Fatalf("hostname did:web: %v", err)
	}
	if url != "https://example.com/.well-known/did.json" {
		t.Fatalf("did:web URL = %q", url)
	}
}

func TestDiscoverOAuthRequiresCurrentATProtoMetadata(t *testing.T) {
	validAuthorization := `{
		"issuer":"https://auth.example",
		"authorization_endpoint":"https://auth.example/authorize",
		"token_endpoint":"https://auth.example/token",
		"pushed_authorization_request_endpoint":"https://auth.example/par",
		"response_types_supported":["code"],
		"grant_types_supported":["authorization_code","refresh_token"],
		"code_challenge_methods_supported":["S256"],
		"token_endpoint_auth_methods_supported":["none","private_key_jwt"],
		"token_endpoint_auth_signing_alg_values_supported":["ES256"],
		"scopes_supported":["atproto"],
		"authorization_response_iss_parameter_supported":true,
		"require_pushed_authorization_requests":true,
		"dpop_signing_alg_values_supported":["ES256"],
		"client_id_metadata_document_supported":true
	}`
	store := testStoreWithHTTP(t, staticTXTResolver{}, func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://pds.example/.well-known/oauth-protected-resource":
			return jsonResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, validAuthorization), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})

	discovery, err := store.discoverOAuth(context.Background(), Identity{DID: "did:plc:alice", PDSURL: "https://pds.example"})
	if err != nil {
		t.Fatalf("discoverOAuth: %v", err)
	}
	if discovery.Authorization.TokenEndpoint != "https://auth.example/token" {
		t.Fatalf("unexpected discovery: %+v", discovery.Authorization)
	}
}

func TestDiscoverOAuthRejectsFalseRequestURIRegistration(t *testing.T) {
	store := testStoreWithHTTP(t, staticTXTResolver{}, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "pds.example" {
			return jsonResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		}
		return jsonResponse(http.StatusOK, `{
			"issuer":"https://auth.example",
			"authorization_endpoint":"https://auth.example/authorize",
			"token_endpoint":"https://auth.example/token",
			"pushed_authorization_request_endpoint":"https://auth.example/par",
			"response_types_supported":["code"],
			"grant_types_supported":["authorization_code","refresh_token"],
			"code_challenge_methods_supported":["S256"],
			"token_endpoint_auth_methods_supported":["none","private_key_jwt"],
			"token_endpoint_auth_signing_alg_values_supported":["ES256"],
			"scopes_supported":["atproto"],
			"authorization_response_iss_parameter_supported":true,
			"require_pushed_authorization_requests":true,
			"require_request_uri_registration":false,
			"dpop_signing_alg_values_supported":["ES256"],
			"client_id_metadata_document_supported":true
		}`), nil
	})

	_, err := store.discoverOAuth(context.Background(), Identity{DID: "did:plc:alice", PDSURL: "https://pds.example"})
	if !errors.Is(err, ErrOAuthResponse) {
		t.Fatalf("discoverOAuth error = %v, want ErrOAuthResponse", err)
	}
}

func TestValidateATOriginRejectsPathsAndDefaultIssuerPort(t *testing.T) {
	for _, value := range []string{"https://auth.example/path", "https://auth.example?x=1", "https://user@auth.example"} {
		if err := validateATOrigin(value, false, true); !errors.Is(err, ErrInvalidIdentity) {
			t.Fatalf("validateATOrigin(%q) = %v", value, err)
		}
	}
	if err := validateATOrigin("https://auth.example:443", false, true); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("default HTTPS port accepted: %v", err)
	}
	if err := validateATOrigin("https://auth.example:8443", false, true); err != nil {
		t.Fatalf("non-default HTTPS origin rejected: %v", err)
	}
}
