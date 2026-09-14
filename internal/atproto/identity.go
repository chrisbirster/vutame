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
	ID      string `json:"id"`
	AlsoKnownAs []string `json:"alsoKnownAs"`
	Service []struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		ServiceEndpoint any    `json:"serviceEndpoint"`
	} `json:"service"`
}

type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

type authorizationServerMetadata struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	PushedAuthorizationRequest       string   `json:"pushed_authorization_request_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	DPoPSigningAlgorithmsSupported   []string `json:"dpop_signing_alg_values_supported"`
	RequirePushedAuthorization       bool     `json:"require_pushed_authorization_requests"`
	ClientIDMetadataDocumentSupport  bool     `json:"client_id_metadata_document_supported"`
}

type oauthDiscovery struct {
	Identity Identity
	Authorization authorizationServerMetadata
}

func (s *Store) ResolveIdentity(ctx context.Context, identifier string) (Identity, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || len(identifier) > 253 {
		return Identity{}, ErrInvalidIdentity
	}
	did := identifier
	handle := ""
	if !strings.HasPrefix(identifier, "did:") {
		handle = strings.ToLower(strings.TrimPrefix(identifier, "@"))
		if strings.ContainsAny(handle, " /?#@") || !strings.Contains(handle, ".") {
			return Identity{}, ErrInvalidIdentity
		}
		resolved, err := s.resolveHandle(ctx, handle)
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
		if service.Type != "AtprotoPersonalDataServer" && !strings.HasSuffix(service.ID, "#atproto_pds") {
			continue
		}
		if endpoint, ok := service.ServiceEndpoint.(string); ok {
			pds = strings.TrimRight(strings.TrimSpace(endpoint), "/")
			break
		}
	}
	if pds == "" || validateATURL(pds, s.config.AllowHTTP) != nil {
		return Identity{}, fmt.Errorf("%w: DID has no safe PDS service", ErrInvalidIdentity)
	}
	if handle == "" {
		for _, alias := range doc.AlsoKnownAs {
			if strings.HasPrefix(alias, "at://") {
				handle = strings.TrimPrefix(alias, "at://")
				break
			}
		}
	}
	return Identity{DID: did, Handle: handle, PDSURL: pds}, nil
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
	endpoint, err := didDocumentURL(did)
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

func didDocumentURL(did string) (string, error) {
	if strings.HasPrefix(did, "did:plc:") {
		return "https://plc.directory/" + url.PathEscape(did), nil
	}
	if strings.HasPrefix(did, "did:web:") {
		methodSpecific := strings.TrimPrefix(did, "did:web:")
		parts := strings.Split(methodSpecific, ":")
		decoded := make([]string, 0, len(parts))
		for _, part := range parts {
			value, err := url.PathUnescape(part)
			if err != nil || value == "" || strings.Contains(value, "/") {
				return "", ErrInvalidIdentity
			}
			decoded = append(decoded, value)
		}
		host := decoded[0]
		if len(decoded) == 1 {
			return "https://" + host + "/.well-known/did.json", nil
		}
		return "https://" + host + "/" + strings.Join(decoded[1:], "/") + "/did.json", nil
	}
	return "", ErrInvalidIdentity
}

func (s *Store) discoverOAuth(ctx context.Context, identity Identity) (oauthDiscovery, error) {
	resourceURL := strings.TrimRight(identity.PDSURL, "/") + "/.well-known/oauth-protected-resource"
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return oauthDiscovery{}, fmt.Errorf("load PDS OAuth metadata: %w", err)
	}
	var resource protectedResourceMetadata
	if err := readJSONResponse(response, &resource); err != nil {
		return oauthDiscovery{}, err
	}
	if len(resource.AuthorizationServers) != 1 {
		return oauthDiscovery{}, fmt.Errorf("%w: PDS must advertise one authorization server", ErrOAuthResponse)
	}
	issuer := strings.TrimRight(resource.AuthorizationServers[0], "/")
	if err := validateATURL(issuer, s.config.AllowHTTP); err != nil {
		return oauthDiscovery{}, err
	}
	metadataURL := issuer + "/.well-known/oauth-authorization-server"
	request, _ = http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	response, err = s.client.Do(request)
	if err != nil {
		return oauthDiscovery{}, fmt.Errorf("load authorization server metadata: %w", err)
	}
	var metadata authorizationServerMetadata
	if err := readJSONResponse(response, &metadata); err != nil {
		return oauthDiscovery{}, err
	}
	if strings.TrimRight(metadata.Issuer, "/") != issuer || !metadata.RequirePushedAuthorization || !metadata.ClientIDMetadataDocumentSupport || !contains(metadata.ResponseTypesSupported, "code") || !contains(metadata.GrantTypesSupported, "authorization_code") || !contains(metadata.GrantTypesSupported, "refresh_token") || !contains(metadata.CodeChallengeMethodsSupported, "S256") || !contains(metadata.ScopesSupported, "atproto") || !contains(metadata.DPoPSigningAlgorithmsSupported, "ES256") {
		return oauthDiscovery{}, fmt.Errorf("%w: authorization server does not satisfy AT Protocol OAuth requirements", ErrOAuthResponse)
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.PushedAuthorizationRequest} {
		if err := validateATURL(endpoint, s.config.AllowHTTP); err != nil {
			return oauthDiscovery{}, err
		}
	}
	return oauthDiscovery{Identity: identity, Authorization: metadata}, nil
}

func contains(values []string, target string) bool {
	for _, value := range values { if value == target { return true } }
	return false
}

var _ = json.Valid
var _ = errors.Is
