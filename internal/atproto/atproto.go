package atproto

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ProfileCollection = "com.vutame.profile"
	LinkCollection    = "com.vutame.link"
	DefaultScope      = "atproto repo:com.vutame.profile repo:com.vutame.link"
)

var (
	ErrNotLinked          = errors.New("AT Protocol account not linked")
	ErrInvalidIdentity    = errors.New("invalid AT Protocol identity")
	ErrOAuthState         = errors.New("invalid or expired AT Protocol OAuth state")
	ErrOAuthResponse      = errors.New("invalid AT Protocol OAuth response")
	ErrConflict           = errors.New("AT Protocol record conflict")
	ErrPublishingDisabled = errors.New("AT Protocol publication is disabled")
)

type Config struct {
	ClientID     string
	RedirectURI  string
	Scope        string
	AllowHTTP    bool
	JetstreamURL string
	ClientName   string
	MarketingURL string
}

type Store struct {
	db        *sql.DB
	secret    []byte
	config    Config
	client    *http.Client
	resolver  TXTResolver
	now       func() time.Time
	refresh   sync.Mutex
	pdsNonces sync.Map
}

type TXTResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}

type Account struct {
	DID            string `json:"did"`
	Handle         string `json:"handle,omitempty"`
	PDSURL         string `json:"pds_url"`
	Scope          string `json:"scope"`
	ConflictPolicy string `json:"conflict_policy"`
	PublishEnabled bool   `json:"publish_enabled"`
	ExpiresAt      string `json:"expires_at"`
	UpdatedAt      string `json:"updated_at"`
}

type Identity struct {
	DID    string `json:"did"`
	Handle string `json:"handle,omitempty"`
	PDSURL string `json:"pds_url"`
}

type IndexedProfile struct {
	DID         string        `json:"did"`
	Handle      string        `json:"handle,omitempty"`
	DisplayName string        `json:"display_name,omitempty"`
	Bio         string        `json:"bio,omitempty"`
	AvatarURL   string        `json:"avatar_url,omitempty"`
	Theme       string        `json:"theme"`
	Verified    bool          `json:"verified"`
	Links       []IndexedLink `json:"links"`
	IndexedAt   string        `json:"indexed_at"`
}

type IndexedLink struct {
	RKey         string `json:"rkey"`
	Label        string `json:"label"`
	URL          string `json:"url"`
	Kind         string `json:"kind"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Featured     bool   `json:"featured"`
	Position     int    `json:"position"`
}

type ClientMetadata struct {
	ClientID             string   `json:"client_id"`
	ClientName           string   `json:"client_name,omitempty"`
	ClientURI            string   `json:"client_uri,omitempty"`
	ApplicationType      string   `json:"application_type"`
	GrantTypes           []string `json:"grant_types"`
	ResponseTypes        []string `json:"response_types"`
	RedirectURIs         []string `json:"redirect_uris"`
	Scope                string   `json:"scope"`
	TokenEndpointAuth    string   `json:"token_endpoint_auth_method"`
	DPoPBoundAccessToken bool     `json:"dpop_bound_access_tokens"`
}

func NewStore(db *sql.DB, secret []byte, config Config) (*Store, error) {
	if db == nil {
		return nil, errors.New("atproto: database is required")
	}
	if len(secret) < 32 {
		return nil, errors.New("atproto: secret must be at least 32 bytes")
	}
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.RedirectURI = strings.TrimSpace(config.RedirectURI)
	config.Scope = strings.TrimSpace(config.Scope)
	if config.Scope == "" {
		config.Scope = DefaultScope
	}
	if config.ClientName == "" {
		config.ClientName = "Vutame"
	}
	if config.ClientID == "" || config.RedirectURI == "" {
		return nil, errors.New("atproto: client ID and redirect URI are required")
	}
	for _, table := range []string{"profiles", "links", "atproto_oauth_states", "atproto_accounts", "atproto_records", "atproto_indexed_profiles", "atproto_indexed_links", "atproto_jetstream_state"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify atproto table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("atproto schema missing table %s; run Atlas schema apply", table)
		}
	}
	store := &Store{db: db, secret: append([]byte(nil), secret...), config: config, resolver: net.DefaultResolver, now: time.Now}
	store.client = hardenedClient(config.AllowHTTP)
	return store, nil
}

func (s *Store) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.client = client
	}
}

func (s *Store) SetTXTResolver(resolver TXTResolver) {
	if resolver != nil {
		s.resolver = resolver
	}
}

func (s *Store) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Store) ClientMetadata() ClientMetadata {
	return ClientMetadata{
		ClientID: s.config.ClientID, ClientName: s.config.ClientName, ClientURI: s.config.MarketingURL,
		ApplicationType: "web", GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
		RedirectURIs: []string{s.config.RedirectURI}, Scope: s.config.Scope, TokenEndpointAuth: "none", DPoPBoundAccessToken: true,
	}
}
