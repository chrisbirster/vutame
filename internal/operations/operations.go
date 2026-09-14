package operations

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	ErrProfileRequired     = errors.New("creator operations profile required")
	ErrInvalidDomain       = errors.New("invalid custom domain")
	ErrDomainTaken         = errors.New("custom domain already claimed")
	ErrDomainNotFound      = errors.New("custom domain not found")
	ErrVerificationPending = errors.New("custom domain verification pending")
	ErrInvalidScope        = errors.New("invalid API token scope")
	ErrTokenUnauthorized   = errors.New("API token unauthorized")
	ErrInvalidWebhook      = errors.New("invalid webhook")
)

const (
	ScopeProfileRead   = "profile:read"
	ScopeProfileWrite  = "profile:write"
	ScopeAnalyticsRead = "analytics:read"
	ScopeContactsRead  = "contacts:read"
)

var supportedScopes = map[string]struct{}{
	ScopeProfileRead: {}, ScopeProfileWrite: {}, ScopeAnalyticsRead: {}, ScopeContactsRead: {},
}

var supportedWebhookEvents = map[string]struct{}{
	"profile.updated": {}, "link.featured": {}, "webhook.test": {},
}

type TXTResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}

type Domain struct {
	ID                string `json:"id"`
	Hostname          string `json:"hostname"`
	VerificationToken string `json:"verification_token"`
	VerifiedAt        string `json:"verified_at,omitempty"`
	DNSName           string `json:"dns_name"`
	DNSValue          string `json:"dns_value"`
}

type VerificationRequest struct {
	ID        string `json:"id"`
	Method    string `json:"method"`
	Evidence  string `json:"evidence"`
	Status    string `json:"status"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type APIToken struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	LastUsedAt string   `json:"last_used_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

type CreatedToken struct {
	APIToken
	Token string `json:"token"`
}

type Webhook struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type CreatedWebhook struct {
	Webhook
	SigningSecret string `json:"signing_secret"`
}

type Store struct {
	db       *sql.DB
	secret   []byte
	resolver TXTResolver
	now      func() time.Time
}

func NewStore(db *sql.DB, secret []byte) (*Store, error) {
	if db == nil {
		return nil, errors.New("operations: database is required")
	}
	if len(secret) < 32 {
		return nil, errors.New("operations: secret must be at least 32 bytes")
	}
	for _, table := range []string{"profiles", "custom_domains", "verification_requests", "api_tokens", "webhooks", "webhook_deliveries"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify operations table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("operations schema missing table %s; run Atlas schema apply", table)
		}
	}
	return &Store{db: db, secret: append([]byte(nil), secret...), resolver: net.DefaultResolver, now: time.Now}, nil
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

func NormalizeHostname(value string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		parsed, err := url.Parse(host)
		if err != nil || parsed.Hostname() == "" || parsed.Path != "" && parsed.Path != "/" {
			return "", ErrInvalidDomain
		}
		host = strings.ToLower(parsed.Hostname())
	}
	if host == "" || len(host) > 253 || strings.Contains(host, ":") || net.ParseIP(host) != nil {
		return "", ErrInvalidDomain
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "vutame.com" || strings.HasSuffix(host, ".vutame.com") || host == "vuta.me" || strings.HasSuffix(host, ".vuta.me") {
		return "", ErrInvalidDomain
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", ErrInvalidDomain
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidDomain
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return "", ErrInvalidDomain
			}
		}
	}
	return host, nil
}

func NormalizeScopes(scopes []string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		scope := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := supportedScopes[scope]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidScope, scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return nil, ErrInvalidScope
	}
	sort.Strings(out)
	return out, nil
}

func NormalizeWebhookEvents(events []string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(events))
	for _, raw := range events {
		event := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := supportedWebhookEvents[event]; !ok {
			return nil, fmt.Errorf("%w: unsupported event %q", ErrInvalidWebhook, event)
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		out = append(out, event)
	}
	if len(out) == 0 {
		return nil, ErrInvalidWebhook
	}
	sort.Strings(out)
	return out, nil
}

func ValidateWebhookURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrInvalidWebhook
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return "", ErrInvalidWebhook
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", ErrInvalidWebhook
	}
	if address, err := netip.ParseAddr(host); err == nil && !publicAddress(address) {
		return "", ErrInvalidWebhook
	}
	if len(parsed.String()) > 2048 {
		return "", ErrInvalidWebhook
	}
	return parsed.String(), nil
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsGlobalUnicast() && !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsUnspecified()
}

func (s *Store) tokenHash(raw string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("vutame-api-token-v1\x00"))
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Store) webhookSecret(id string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("vutame-webhook-v1\x00"))
	_, _ = mac.Write([]byte(id))
	return mac.Sum(nil)
}

func randomID(prefix string, bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}
