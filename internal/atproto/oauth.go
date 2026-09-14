package atproto

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OAuthStart struct {
	AuthorizationURL string `json:"authorization_url"`
	DID              string `json:"did"`
	Handle           string `json:"handle,omitempty"`
}

type oauthState struct {
	UserID        string
	Identifier    string
	ExpectedDID   string
	PDSURL        string
	Issuer        string
	TokenEndpoint string
	Verifier      string
	DPoPKey       *ecdsa.PrivateKey
	ExpiresAt     time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Sub          string `json:"sub"`
	ExpiresIn    int    `json:"expires_in"`
}

type parResponse struct {
	RequestURI string `json:"request_uri"`
	ExpiresIn  int    `json:"expires_in"`
}

func (s *Store) StartOAuth(ctx context.Context, userID, identifier string) (OAuthStart, error) {
	if err := s.requireProfile(ctx, userID); err != nil {
		return OAuthStart{}, err
	}
	identity, err := s.ResolveIdentity(ctx, identifier)
	if err != nil {
		return OAuthStart{}, err
	}
	discovery, err := s.discoverOAuth(ctx, identity)
	if err != nil {
		return OAuthStart{}, err
	}
	state, err := randomToken(24)
	if err != nil { return OAuthStart{}, err }
	verifier, err := randomToken(48)
	if err != nil { return OAuthStart{}, err }
	key, err := generateDPoPKey()
	if err != nil { return OAuthStart{}, err }

	form := url.Values{
		"client_id": {s.config.ClientID},
		"response_type": {"code"},
		"redirect_uri": {s.config.RedirectURI},
		"scope": {s.config.Scope},
		"state": {state},
		"code_challenge": {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"login_hint": {strings.TrimSpace(identifier)},
	}
	response, _, err := s.doDPoPForm(ctx, discovery.Authorization.PushedAuthorizationRequest, form, key, "")
	if err != nil {
		return OAuthStart{}, fmt.Errorf("push AT Protocol authorization request: %w", err)
	}
	var pushed parResponse
	if err := decodeOAuthJSON(response, &pushed); err != nil {
		return OAuthStart{}, err
	}
	if pushed.RequestURI == "" {
		return OAuthStart{}, fmt.Errorf("%w: PAR response missing request_uri", ErrOAuthResponse)
	}

	verifierEnc, err := s.encrypt([]byte(verifier)); if err != nil { return OAuthStart{}, err }
	keyBytes, err := marshalDPoPKey(key); if err != nil { return OAuthStart{}, err }
	keyEnc, err := s.encrypt(keyBytes); if err != nil { return OAuthStart{}, err }
	now := s.now().UTC()
	expires := now.Add(10 * time.Minute)
	if pushed.ExpiresIn > 0 && pushed.ExpiresIn < 600 { expires = now.Add(time.Duration(pushed.ExpiresIn) * time.Second) }
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO atproto_oauth_states (state_hash,user_id,identifier,expected_did,pds_url,issuer,token_endpoint,verifier_enc,dpop_key_enc,expires_at,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
	`, stateHash(state), userID, strings.TrimSpace(identifier), identity.DID, identity.PDSURL, discovery.Authorization.Issuer, discovery.Authorization.TokenEndpoint, verifierEnc, keyEnc, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return OAuthStart{}, fmt.Errorf("persist AT Protocol OAuth state: %w", err)
	}
	authURL, _ := url.Parse(discovery.Authorization.AuthorizationEndpoint)
	query := authURL.Query()
	query.Set("client_id", s.config.ClientID)
	query.Set("request_uri", pushed.RequestURI)
	authURL.RawQuery = query.Encode()
	return OAuthStart{AuthorizationURL: authURL.String(), DID: identity.DID, Handle: identity.Handle}, nil
}

func (s *Store) CompleteOAuth(ctx context.Context, userID, state, code, issuer string) (Account, error) {
	if strings.TrimSpace(state) == "" || strings.TrimSpace(code) == "" || strings.TrimSpace(issuer) == "" {
		return Account{}, ErrOAuthState
	}
	stored, err := s.loadOAuthState(ctx, state)
	if err != nil { return Account{}, err }
	if stored.UserID != userID || s.now().UTC().After(stored.ExpiresAt) || strings.TrimRight(strings.TrimSpace(issuer), "/") != strings.TrimRight(stored.Issuer, "/") {
		return Account{}, ErrOAuthState
	}
	form := url.Values{
		"grant_type": {"authorization_code"},
		"code": {code},
		"client_id": {s.config.ClientID},
		"redirect_uri": {s.config.RedirectURI},
		"code_verifier": {stored.Verifier},
	}
	response, _, err := s.doDPoPForm(ctx, stored.TokenEndpoint, form, stored.DPoPKey, "")
	if err != nil { return Account{}, fmt.Errorf("exchange AT Protocol authorization code: %w", err) }
	var tokens tokenResponse
	if err := decodeOAuthJSON(response, &tokens); err != nil { return Account{}, err }
	if tokens.Sub != stored.ExpectedDID || tokens.AccessToken == "" || tokens.RefreshToken == "" || !scopeContains(tokens.Scope, "atproto") {
		return Account{}, fmt.Errorf("%w: token identity or scope mismatch", ErrOAuthResponse)
	}
	accessEnc, err := s.encrypt([]byte(tokens.AccessToken)); if err != nil { return Account{}, err }
	refreshEnc, err := s.encrypt([]byte(tokens.RefreshToken)); if err != nil { return Account{}, err }
	keyBytes, err := marshalDPoPKey(stored.DPoPKey); if err != nil { return Account{}, err }
	keyEnc, err := s.encrypt(keyBytes); if err != nil { return Account{}, err }
	now := s.now().UTC()
	expires := now.Add(5 * time.Minute)
	if tokens.ExpiresIn > 0 { expires = now.Add(time.Duration(tokens.ExpiresIn) * time.Second) }
	handle := strings.TrimPrefix(strings.TrimSpace(stored.Identifier), "@")
	if strings.HasPrefix(handle, "did:") { handle = "" }

	tx, err := s.db.BeginTx(ctx, nil); if err != nil { return Account{}, err }
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO atproto_accounts (user_id,did,handle,pds_url,issuer,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at,conflict_policy,publish_enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,'vutame_wins',0,?,?)
		ON CONFLICT(user_id) DO UPDATE SET did=excluded.did,handle=excluded.handle,pds_url=excluded.pds_url,issuer=excluded.issuer,token_endpoint=excluded.token_endpoint,access_token_enc=excluded.access_token_enc,refresh_token_enc=excluded.refresh_token_enc,dpop_key_enc=excluded.dpop_key_enc,scope=excluded.scope,expires_at=excluded.expires_at,updated_at=excluded.updated_at
	`, userID, tokens.Sub, handle, stored.PDSURL, stored.Issuer, stored.TokenEndpoint, accessEnc, refreshEnc, keyEnc, tokens.Scope, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Account{}, fmt.Errorf("save AT Protocol account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE profiles SET atproto_did=?, verified=1, updated_at=? WHERE user_id=?`, tokens.Sub, now.Format(time.RFC3339Nano), userID); err != nil {
		return Account{}, err
	}
	requestID, _ := randomToken(12)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO verification_requests (id,user_id,method,evidence,status,note,created_at,updated_at)
		VALUES (?,?,'atproto',?,'approved','AT Protocol OAuth proved control of this DID.',?,?)
	`, "ver_"+requestID, userID, tokens.Sub, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM atproto_oauth_states WHERE state_hash=?`, stateHash(state)); err != nil { return Account{}, err }
	if err := tx.Commit(); err != nil { return Account{}, err }
	return s.Account(ctx, userID)
}

func (s *Store) Account(ctx context.Context, userID string) (Account, error) {
	var item Account
	var enabled int
	if err := s.db.QueryRowContext(ctx, `
		SELECT did,handle,pds_url,scope,conflict_policy,publish_enabled,expires_at,updated_at FROM atproto_accounts WHERE user_id=?
	`, strings.TrimSpace(userID)).Scan(&item.DID,&item.Handle,&item.PDSURL,&item.Scope,&item.ConflictPolicy,&enabled,&item.ExpiresAt,&item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return Account{}, ErrNotLinked }
		return Account{}, err
	}
	item.PublishEnabled = enabled == 1
	return item, nil
}

func (s *Store) UpdateSettings(ctx context.Context, userID, conflictPolicy string, publishEnabled bool) (Account, error) {
	if conflictPolicy != "vutame_wins" && conflictPolicy != "pds_wins" { return Account{}, ErrConflict }
	enabled := 0; if publishEnabled { enabled = 1 }
	result, err := s.db.ExecContext(ctx, `UPDATE atproto_accounts SET conflict_policy=?,publish_enabled=?,updated_at=? WHERE user_id=?`, conflictPolicy, enabled, s.now().UTC().Format(time.RFC3339Nano), userID)
	if err != nil { return Account{}, err }
	count, _ := result.RowsAffected(); if count != 1 { return Account{}, ErrNotLinked }
	return s.Account(ctx, userID)
}

func (s *Store) Unlink(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil); if err != nil { return err }
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM atproto_accounts WHERE user_id=?`, userID); err != nil { return err }
	var customProofs int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_domains WHERE user_id=? AND verified_at IS NOT NULL`, userID).Scan(&customProofs)
	verified := 0; if customProofs > 0 { verified = 1 }
	if _, err := tx.ExecContext(ctx, `UPDATE profiles SET atproto_did=NULL,verified=?,updated_at=? WHERE user_id=?`, verified, s.now().UTC().Format(time.RFC3339Nano), userID); err != nil { return err }
	return tx.Commit()
}

func (s *Store) loadOAuthState(ctx context.Context, rawState string) (oauthState, error) {
	var item oauthState
	var verifierEnc, keyEnc, expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT user_id,identifier,expected_did,pds_url,issuer,token_endpoint,verifier_enc,dpop_key_enc,expires_at
		FROM atproto_oauth_states WHERE state_hash=?
	`, stateHash(rawState)).Scan(&item.UserID,&item.Identifier,&item.ExpectedDID,&item.PDSURL,&item.Issuer,&item.TokenEndpoint,&verifierEnc,&keyEnc,&expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return oauthState{}, ErrOAuthState }
		return oauthState{}, err
	}
	verifier, err := s.decrypt(verifierEnc); if err != nil { return oauthState{}, err }
	keyBytes, err := s.decrypt(keyEnc); if err != nil { return oauthState{}, err }
	key, err := parseDPoPKey(keyBytes); if err != nil { return oauthState{}, err }
	expires, err := time.Parse(time.RFC3339Nano, expiresAt); if err != nil { return oauthState{}, ErrOAuthState }
	item.Verifier = string(verifier); item.DPoPKey = key; item.ExpiresAt = expires
	return item, nil
}

func (s *Store) doDPoPForm(ctx context.Context, endpoint string, form url.Values, key *ecdsa.PrivateKey, accessToken string) (*http.Response, string, error) {
	nonce := ""
	for attempt := 0; attempt < 2; attempt++ {
		proof, err := dpopProof(key, http.MethodPost, endpoint, nonce, accessToken, s.now().UTC().Unix())
		if err != nil { return nil, "", err }
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil { return nil, "", err }
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("DPoP", proof)
		if accessToken != "" { request.Header.Set("Authorization", "DPoP "+accessToken) }
		response, err := s.client.Do(request)
		if err != nil { return nil, "", err }
		responseNonce := strings.TrimSpace(response.Header.Get("DPoP-Nonce"))
		if responseNonce == "" {
			response.Body.Close()
			return nil, "", fmt.Errorf("%w: DPoP response missing nonce", ErrOAuthResponse)
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 { return response, responseNonce, nil }
		if attempt == 0 {
			nonce = responseNonce
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024)); response.Body.Close()
			continue
		}
		return response, responseNonce, nil
	}
	return nil, "", ErrOAuthResponse
}

func decodeOAuthJSON(response *http.Response, target any) error {
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxATProtoResponse+1)); if err != nil { return err }
	if len(data) > maxATProtoResponse { return ErrOAuthResponse }
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload map[string]any; _ = json.Unmarshal(data,&payload)
		return fmt.Errorf("%w: OAuth HTTP %d", ErrOAuthResponse, response.StatusCode)
	}
	if err := json.Unmarshal(data,target); err != nil { return ErrOAuthResponse }
	return nil
}

func scopeContains(scope, value string) bool {
	for _, item := range strings.Fields(scope) { if item == value { return true } }
	return false
}

func (s *Store) requireProfile(ctx context.Context, userID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, strings.TrimSpace(userID)).Scan(&count); err != nil { return err }
	if count != 1 { return ErrNotLinked }
	return nil
}
