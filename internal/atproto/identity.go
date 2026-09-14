package atproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type didDocument struct {
	ID           string   `json:"id"`
	AlsoKnownAs  []string `json:"alsoKnownAs"`
	Service      []struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		ServiceEndpoint any    `json:"serviceEndpoint"`
	} `json:"service"`
}

type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

type authorizationServerMetadata struct {
	Issuer                                  string   `json:"issuer"`
	AuthorizationEndpoint                   string   `json:"authorization_endpoint"`
	TokenEndpoint                           string   `json:"token_endpoint"`
	PushedAuthorizationRequest              string   `json:"pushed_authorization_request_endpoint"`
	ResponseTypesSupported                  []string `json:"response_types_supported"`
	GrantTypesSupported                     []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported           []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported       []string `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgsSupported   []string `json:"token_endpoint_auth_signing_alg_values_supported"`
	ScopesSupported                         []string `json:"scopes_supported"`
	DPoPSigningAlgorithmsSupported          []string `json:"dpop_signing_alg_values_supported"`
	AuthorizationResponseISSSupported       bool     `json:"authorization_response_iss_parameter_supported"`
	RequirePushedAuthorization              bool     `json:"require_pushed_authorization_requests"`
	RequireRequestURIRegistration           *bool    `json:"require_request_uri_registration"`
	ClientIDMetadataDocumentSupport         bool     `json:"client_id_metadata_document_supported"`
}

type oauthDiscovery struct {
	Identity      Identity
	Authorization authorizationServerMetadata
}

func (s *Store) ResolveIdentity(ctx context.Context, identifier string) (Identity, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || len(identifier) > 253 {
		return Identity{}, ErrInvalidIdentity
	}

	did := identifier
	requestedHandle := ""
	if !strings.HasPrefix(identifier, "did:") {
		requestedHandle = normalizeATHandle(identifier)
		if !validATHandleCandidate(requestedHandle) {
			return Identity{}, ErrInvalidIdentity
		}
		resolved, err := s.resolveHandle(ctx, requestedHandle)
		if err != nil {
			return Identity{}, err
		}
		did = resolved
	}
	if !strings.HasPrefix(did, "did:plc:") && !strings.HasPrefix(did, "did:web:") {
		return Identity{}, ErrInvalidIdentity
	}

	doc, err := s.resolveDIDDocument(ctx, did)
	if err != nil {
		return Identity{}, err
	}
	if doc.ID != did {
		return Identity{}, fmt.Errorf("%w: DID document id mismatch", ErrInvalidIdentity)
	}

	pds := ""
	for _, service := range doc.Service {
		if service.Type != "AtprotoPersonalDataServer" || !strings.HasSuffix(service.ID, "#atproto_pds") {
			continue
		}
		endpoint, ok := service.ServiceEndpoint.(string)
		if !ok {
			continue
		}
		endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
		if validateATOrigin(endpoint, s.config.AllowHTTP, false) == nil {
			pds = endpoint
			break
		}
	}
	if pds == "" {
		return Identity{}, fmt.Errorf("%w: DID has no safe AT Protocol PDS service", ErrInvalidIdentity)
	}

	if requestedHandle != "" {
		if !didClaimsHandle(doc, requestedHandle) {
			return Identity{}, fmt.Errorf("%w: handle is not claimed by DID document", ErrInvalidIdentity)
		}
		return Identity{DID: did, Handle: requestedHandle, PDSURL: pds}, nil
	}

	// A DID remains a valid canonical identity even when its claimed handle is
	// stale or broken. Only expose the human-facing handle after verifying the
	// handle resolves back to this exact DID.
	handle := firstClaimedHandle(doc)
	if handle != "" {
		resolved, resolveErr := s.resolveHandle(ctx, handle)
		if resolveErr != nil || resolved != did {
			handle = ""
		}
	}
	return Identity{DID: did, Handle: handle, PDSURL: pds}, nil
}

func normalizeATHandle(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}

func validATHandleCandidate(handle string) bool {
	if handle == "" || len(handle) > 253 || strings.ContainsAny(handle, " /?#@:") || !strings.Contains(handle, ".") {
		return false
	}
	for _, label := range strings.Split(handle, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func didClaimsHandle(doc didDocument, handle string) bool {
	for _, alias := range doc.AlsoKnownAs {
		if claimedHandle(alias) == handle {
			return true
		}
	}
	return false
}

func firstClaimedHandle(doc didDocument) string {
	for _, alias := range doc.AlsoKnownAs {
		if handle := claimedHandle(alias); handle != "" {
			return handle
		}
	}
	return ""
}

func claimedHandle(alias string) string {
	parsed, err := url.Parse(strings.TrimSpace(alias))
	if err != nil || parsed.Scheme != "at" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	handle := normalizeATHandle(parsed.Host)
	if !validATHandleCandidate(handle) {
		return ""
	}
	return handle
}

func (s *Store) resolveHandle(ctx context.Context, handle string) (string, error) {
	if values, err := s.resolver.LookupTXT(ctx, "_atproto."+handle); err == nil {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "did=") {
				did := strings.TrimSpace(strings.TrimPrefix(value, "did="))
				if strings.HasPrefix(did, "did:") {
					return did, nil
				}
			}
		}
	}
	endpoint := "https://" + handle + "/.well-known/atproto-did"
	if s.config.AllowHTTP && (strings.HasPrefix(handle, "127.0.0.1") || strings.HasPrefix(handle, "localhost")) {
		endpoint = "http://" + handle + "/.well-known/atproto-did"
	}
	if err := validateATURL(endpoint, s.config.AllowHTTP); err != nil {
		return "", ErrInvalidIdentity
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolve AT Protocol handle: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ErrInvalidIdentity
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return "", err
	}
	did := strings.TrimSpace(string(body))
	if !strings.HasPrefix(did, "did:") {
		return "", ErrInvalidIdentity
	}
	return did, nil
}

func (s *Store) resolveDIDDocument(ctx context.Context, did string) (didDocument, error) {
	endpoint, err := didDocumentURL(did, s.config.AllowHTTP)
	if err != nil {
		return didDocument{}, err
	}
	if err := validateATURL(endpoint, s.config.AllowHTTP); err != nil {
		return didDocument{}, err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return didDocument{}, fmt.Errorf("resolve DID document: %w", err)
	}
	var doc didDocument
	if err := readJSONResponse(response, &doc); err != nil {
		return didDocument{}, err
	}
	return doc, nil
}

func didDocumentURL(did string, allowHTTP bool) (string, error) {
	if strings.HasPrefix(did, "did:plc:") {
		return "https://plc.directory/" + url.PathEscape(did), nil
	}
	if strings.HasPrefix(did, "did:web:") {
		methodSpecific := strings.TrimPrefix(did, "did:web:")
		// AT Protocol deliberately supports hostname-level did:web only. Colons
		// in the method-specific identifier otherwise represent path segments.
		if methodSpecific == "" || strings.Contains(methodSpecific, ":") {
			return "", ErrInvalidIdentity
		}
		host, err := url.PathUnescape(methodSpecific)
		if err != nil || host == "" || strings.ContainsAny(host, "/?#@") {
			return "", ErrInvalidIdentity
		}
		scheme := "https"
		if strings.Contains(host, ":") {
			name, port, splitErr := netSplitHostPortLoose(host)
			if splitErr != nil || name != "localhost" || port == "" || !allowHTTP {
				return "", ErrInvalidIdentity
			}
			scheme = "http"
		} else if host == "localhost" {
			if !allowHTTP {
				return "", ErrInvalidIdentity
			}
			scheme = "http"
		}
		return scheme + "://" + host + "/.well-known/did.json", nil
	}
	return "", ErrInvalidIdentity
}

func netSplitHostPortLoose(host string) (string, string, error) {
	parsed, err := url.Parse("http://" + host)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", "", ErrInvalidIdentity
	}
	return parsed.Hostname(), parsed.Port(), nil
}

func (s *Store) discoverOAuth(ctx context.Context, identity Identity) (oauthDiscovery, error) {
	resourceURL := strings.TrimRight(identity.PDSURL, "/") + "/.well-known/oauth-protected-resource"
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return oauthDiscovery{}, fmt.Errorf("load PDS OAuth metadata: %w", err)
	}
	var resource protectedResourceMetadata
	if err := readOAuthMetadataResponse(response, &resource); err != nil {
		return oauthDiscovery{}, err
	}
	if len(resource.AuthorizationServers) != 1 {
		return oauthDiscovery{}, fmt.Errorf("%w: PDS must advertise one authorization server", ErrOAuthResponse)
	}
	issuer := strings.TrimRight(resource.AuthorizationServers[0], "/")
	if err := validateATOrigin(issuer, s.config.AllowHTTP, true); err != nil {
		return oauthDiscovery{}, err
	}

	metadataURL := issuer + "/.well-known/oauth-authorization-server"
	request, _ = http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	response, err = s.client.Do(request)
	if err != nil {
		return oauthDiscovery{}, fmt.Errorf("load authorization server metadata: %w", err)
	}
	var metadata authorizationServerMetadata
	if err := readOAuthMetadataResponse(response, &metadata); err != nil {
		return oauthDiscovery{}, err
	}
	requestURIRegistrationOK := metadata.RequireRequestURIRegistration == nil || *metadata.RequireRequestURIRegistration
	if strings.TrimRight(metadata.Issuer, "/") != issuer ||
		validateATOrigin(metadata.Issuer, s.config.AllowHTTP, true) != nil ||
		!metadata.AuthorizationResponseISSSupported ||
		!metadata.RequirePushedAuthorization ||
		!requestURIRegistrationOK ||
		!metadata.ClientIDMetadataDocumentSupport ||
		!contains(metadata.ResponseTypesSupported, "code") ||
		!contains(metadata.GrantTypesSupported, "authorization_code") ||
		!contains(metadata.GrantTypesSupported, "refresh_token") ||
		!contains(metadata.CodeChallengeMethodsSupported, "S256") ||
		!contains(metadata.TokenEndpointAuthMethodsSupported, "none") ||
		!contains(metadata.TokenEndpointAuthMethodsSupported, "private_key_jwt") ||
		contains(metadata.TokenEndpointAuthSigningAlgsSupported, "none") ||
		!contains(metadata.TokenEndpointAuthSigningAlgsSupported, "ES256") ||
		!contains(metadata.ScopesSupported, "atproto") ||
		!contains(metadata.DPoPSigningAlgorithmsSupported, "ES256") {
		return oauthDiscovery{}, fmt.Errorf("%w: authorization server does not satisfy AT Protocol OAuth requirements", ErrOAuthResponse)
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.PushedAuthorizationRequest} {
		if err := validateATURL(endpoint, s.config.AllowHTTP); err != nil {
			return oauthDiscovery{}, err
		}
	}
	return oauthDiscovery{Identity: identity, Authorization: metadata}, nil
}

func readOAuthMetadataResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: OAuth metadata HTTP %d", ErrOAuthResponse, response.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" {
		return fmt.Errorf("%w: OAuth metadata must be application/json", ErrOAuthResponse)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxATProtoResponse+1))
	if err != nil {
		return err
	}
	if len(data) > maxATProtoResponse || !json.Valid(data) {
		return ErrOAuthResponse
	}
	if err := json.Unmarshal(data, target); err != nil {
		return ErrOAuthResponse
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var _ = errors.Is
