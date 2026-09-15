package atproto

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrPublishingDisabled = errors.New("AT Protocol publication disabled")

type sessionCredentials struct {
	UserID        string
	DID           string
	PDSURL        string
	TokenEndpoint string
	AccessToken   string
	RefreshToken  string
	DPoPKey       *ecdsa.PrivateKey
	Scope         string
	ExpiresAt     time.Time
}

func (s *Store) accessSession(ctx context.Context, userID string) (sessionCredentials, error) {
	creds, err := s.loadSessionCredentials(ctx, userID)
	if err != nil {
		return sessionCredentials{}, err
	}
	if s.now().UTC().Before(creds.ExpiresAt.Add(-30 * time.Second)) {
		return creds, nil
	}

	// AT Protocol refresh tokens are generally single-use. Serialize refreshes
	// and reload after acquiring the lock so concurrent syncs cannot reuse one.
	s.refresh.Lock()
	defer s.refresh.Unlock()

	creds, err = s.loadSessionCredentials(ctx, userID)
	if err != nil {
		return sessionCredentials{}, err
	}
	if s.now().UTC().Before(creds.ExpiresAt.Add(-30 * time.Second)) {
		return creds, nil
	}
	return s.refreshSession(ctx, creds)
}

func (s *Store) loadSessionCredentials(ctx context.Context, userID string) (sessionCredentials, error) {
	var item sessionCredentials
	var accessEnc, refreshEnc, keyEnc, expiresAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id,did,pds_url,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at
		FROM atproto_accounts WHERE user_id=?
	`, strings.TrimSpace(userID)).Scan(
		&item.UserID, &item.DID, &item.PDSURL, &item.TokenEndpoint,
		&accessEnc, &refreshEnc, &keyEnc, &item.Scope, &expiresAt,
	)
	if err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return sessionCredentials{}, ErrNotLinked
		}
		return sessionCredentials{}, err
	}
	access, err := s.decrypt(accessEnc)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("decrypt AT Protocol access token: %w", err)
	}
	refresh, err := s.decrypt(refreshEnc)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("decrypt AT Protocol refresh token: %w", err)
	}
	keyBytes, err := s.decrypt(keyEnc)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("decrypt AT Protocol DPoP key: %w", err)
	}
	key, err := parseDPoPKey(keyBytes)
	if err != nil {
		return sessionCredentials{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("parse AT Protocol access expiry: %w", err)
	}
	item.AccessToken = string(access)
	item.RefreshToken = string(refresh)
	item.DPoPKey = key
	item.ExpiresAt = expires
	return item, nil
}

func (s *Store) refreshSession(ctx context.Context, current sessionCredentials) (sessionCredentials, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.RefreshToken},
		"client_id":     {s.config.ClientID},
	}
	response, _, err := s.doDPoPForm(ctx, current.TokenEndpoint, form, current.DPoPKey, "")
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("refresh AT Protocol session: %w", err)
	}
	var tokens tokenResponse
	if err := decodeOAuthJSON(response, &tokens); err != nil {
		return sessionCredentials{}, err
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || !scopeContains(tokens.Scope, "atproto") {
		return sessionCredentials{}, fmt.Errorf("%w: refresh response missing token or atproto scope", ErrOAuthResponse)
	}
	if tokens.Sub != "" && tokens.Sub != current.DID {
		return sessionCredentials{}, fmt.Errorf("%w: refresh response changed DID", ErrOAuthResponse)
	}
	if tokens.TokenType != "" && !strings.EqualFold(tokens.TokenType, "DPoP") {
		return sessionCredentials{}, fmt.Errorf("%w: refresh response token type must be DPoP", ErrOAuthResponse)
	}

	accessEnc, err := s.encrypt([]byte(tokens.AccessToken))
	if err != nil {
		return sessionCredentials{}, err
	}
	refreshEnc, err := s.encrypt([]byte(tokens.RefreshToken))
	if err != nil {
		return sessionCredentials{}, err
	}
	now := s.now().UTC()
	expires := now.Add(5 * time.Minute)
	if tokens.ExpiresIn > 0 {
		expires = now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	scope := strings.TrimSpace(tokens.Scope)
	if scope == "" {
		scope = current.Scope
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE atproto_accounts
		SET access_token_enc=?,refresh_token_enc=?,scope=?,expires_at=?,updated_at=?
		WHERE user_id=? AND did=?
	`, accessEnc, refreshEnc, scope, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), current.UserID, current.DID)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("persist refreshed AT Protocol session: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sessionCredentials{}, ErrNotLinked
	}
	current.AccessToken = tokens.AccessToken
	current.RefreshToken = tokens.RefreshToken
	current.Scope = scope
	current.ExpiresAt = expires
	return current, nil
}

func (s *Store) doDPoPJSON(ctx context.Context, method, endpoint string, body any, session sessionCredentials) (*http.Response, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	nonce := ""
	for attempt := 0; attempt < 2; attempt++ {
		proof, err := dpopProof(session.DPoPKey, method, endpoint, nonce, session.AccessToken, s.now().UTC().Unix())
		if err != nil {
			return nil, err
		}
		var reader io.Reader
		if encoded != nil {
			reader = bytes.NewReader(encoded)
		}
		request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return nil, err
		}
		if encoded != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		request.Header.Set("Authorization", "DPoP "+session.AccessToken)
		request.Header.Set("DPoP", proof)
		response, err := s.client.Do(request)
		if err != nil {
			return nil, err
		}
		responseNonce := strings.TrimSpace(response.Header.Get("DPoP-Nonce"))
		if responseNonce == "" {
			response.Body.Close()
			return nil, fmt.Errorf("%w: PDS response missing DPoP nonce", ErrOAuthResponse)
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, nil
		}
		if attempt == 0 && (response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnauthorized) {
			nonce = responseNonce
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
			response.Body.Close()
			continue
		}
		return response, nil
	}
	return nil, ErrOAuthResponse
}

// sqlErrNoRows keeps the session file independent from database/sql imports in
// tests that replace this helper while still comparing the canonical sentinel.
func sqlErrNoRows() error {
	return errors.New("sql: no rows in result set")
}
