package atproto

import (
	"bytes"
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

var errAccessTokenRejected = errors.New("AT Protocol access token rejected")

type SyncRecord struct {
	Collection     string `json:"collection"`
	RKey           string `json:"rkey"`
	URI            string `json:"uri"`
	CID            string `json:"cid"`
	LocalUpdatedAt string `json:"local_updated_at"`
	SyncedAt       string `json:"synced_at"`
}

type SyncStatus struct {
	DID             string       `json:"did"`
	Handle          string       `json:"handle,omitempty"`
	PDSURL          string       `json:"pds_url"`
	ConflictPolicy  string       `json:"conflict_policy"`
	PublishEnabled  bool         `json:"publish_enabled"`
	LastSyncedAt    string       `json:"last_synced_at,omitempty"`
	Records         []SyncRecord `json:"records"`
}

type accountCredentials struct {
	UserID          string
	DID             string
	Handle          string
	PDSURL          string
	TokenEndpoint   string
	AccessToken     string
	RefreshToken    string
	DPoPKey         *ecdsa.PrivateKey
	Scope           string
	ExpiresAt       time.Time
	ConflictPolicy  string
	PublishEnabled  bool
}

type desiredRecord struct {
	Collection     string
	RKey           string
	LocalUpdatedAt string
	Record         map[string]any
	Payload        string
}

type persistedRecord struct {
	Collection     string
	RKey           string
	CID            string
	LocalUpdatedAt string
	SyncedAt       string
	Payload        string
}

type putRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type pdsError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (s *Store) SyncStatus(ctx context.Context, userID string) (SyncStatus, error) {
	account, err := s.Account(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	records, err := s.persistedRecords(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	status := SyncStatus{
		DID: account.DID, Handle: account.Handle, PDSURL: account.PDSURL,
		ConflictPolicy: account.ConflictPolicy, PublishEnabled: account.PublishEnabled,
		Records: make([]SyncRecord, 0, len(records)),
	}
	for _, item := range records {
		status.Records = append(status.Records, SyncRecord{
			Collection: item.Collection,
			RKey: item.RKey,
			URI: "at://" + account.DID + "/" + item.Collection + "/" + item.RKey,
			CID: item.CID,
			LocalUpdatedAt: item.LocalUpdatedAt,
			SyncedAt: item.SyncedAt,
		})
		if item.SyncedAt > status.LastSyncedAt {
			status.LastSyncedAt = item.SyncedAt
		}
	}
	return status, nil
}

func (s *Store) SyncNow(ctx context.Context, userID string) (SyncStatus, error) {
	account, err := s.Account(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	if !account.PublishEnabled {
		return SyncStatus{}, ErrPublishingDisabled
	}

	desired, err := s.desiredRecords(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	existing, err := s.persistedRecords(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	credentials, err := s.usableCredentials(ctx, userID, false)
	if err != nil {
		return SyncStatus{}, err
	}

	desiredKeys := make(map[string]struct{}, len(desired))
	for _, item := range desired {
		key := item.Collection + "\x00" + item.RKey
		desiredKeys[key] = struct{}{}
		written, err := s.putRecord(ctx, userID, credentials, item)
		if errors.Is(err, errAccessTokenRejected) {
			credentials, err = s.usableCredentials(ctx, userID, true)
			if err == nil {
				written, err = s.putRecord(ctx, userID, credentials, item)
			}
		}
		if err != nil {
			return SyncStatus{}, err
		}
		if err := s.savePublishedRecord(ctx, userID, item, written.CID); err != nil {
			return SyncStatus{}, err
		}
	}

	for _, item := range existing {
		if item.Collection != LinkCollection {
			continue
		}
		key := item.Collection + "\x00" + item.RKey
		if _, keep := desiredKeys[key]; keep {
			continue
		}
		err := s.deleteRecord(ctx, userID, credentials, item.Collection, item.RKey)
		if errors.Is(err, errAccessTokenRejected) {
			credentials, err = s.usableCredentials(ctx, userID, true)
			if err == nil {
				err = s.deleteRecord(ctx, userID, credentials, item.Collection, item.RKey)
			}
		}
		if err != nil {
			return SyncStatus{}, err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM atproto_records WHERE user_id=? AND collection=? AND rkey=?`, userID, item.Collection, item.RKey); err != nil {
			return SyncStatus{}, fmt.Errorf("delete AT Protocol record mapping: %w", err)
		}
	}
	return s.SyncStatus(ctx, userID)
}

func (s *Store) usableCredentials(ctx context.Context, userID string, forceRefresh bool) (accountCredentials, error) {
	credentials, err := s.loadCredentials(ctx, userID)
	if err != nil {
		return accountCredentials{}, err
	}
	if !forceRefresh && credentials.ExpiresAt.After(s.now().UTC().Add(30*time.Second)) {
		return credentials, nil
	}

	// Refresh tokens in the AT Protocol OAuth profile are generally rotating and
	// single-use. Serialize refreshes and reload inside the lock so concurrent
	// syncs never race the same refresh token.
	s.refresh.Lock()
	defer s.refresh.Unlock()
	credentials, err = s.loadCredentials(ctx, userID)
	if err != nil {
		return accountCredentials{}, err
	}
	if !forceRefresh && credentials.ExpiresAt.After(s.now().UTC().Add(30*time.Second)) {
		return credentials, nil
	}
	return s.refreshCredentials(ctx, credentials)
}

func (s *Store) loadCredentials(ctx context.Context, userID string) (accountCredentials, error) {
	var item accountCredentials
	var accessEnc, refreshEnc, keyEnc, expiresAt string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id,did,handle,pds_url,token_endpoint,access_token_enc,refresh_token_enc,dpop_key_enc,scope,expires_at,conflict_policy,publish_enabled
		FROM atproto_accounts WHERE user_id=?
	`, strings.TrimSpace(userID)).Scan(
		&item.UserID, &item.DID, &item.Handle, &item.PDSURL, &item.TokenEndpoint,
		&accessEnc, &refreshEnc, &keyEnc, &item.Scope, &expiresAt, &item.ConflictPolicy, &enabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return accountCredentials{}, ErrNotLinked
	}
	if err != nil {
		return accountCredentials{}, err
	}
	access, err := s.decrypt(accessEnc)
	if err != nil { return accountCredentials{}, err }
	refresh, err := s.decrypt(refreshEnc)
	if err != nil { return accountCredentials{}, err }
	keyBytes, err := s.decrypt(keyEnc)
	if err != nil { return accountCredentials{}, err }
	key, err := parseDPoPKey(keyBytes)
	if err != nil { return accountCredentials{}, err }
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil { return accountCredentials{}, ErrOAuthResponse }
	item.AccessToken = string(access)
	item.RefreshToken = string(refresh)
	item.DPoPKey = key
	item.ExpiresAt = expires
	item.PublishEnabled = enabled == 1
	return item, nil
}

func (s *Store) refreshCredentials(ctx context.Context, current accountCredentials) (accountCredentials, error) {
	if current.RefreshToken == "" {
		return accountCredentials{}, ErrOAuthResponse
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.RefreshToken},
		"client_id":     {s.config.ClientID},
	}
	response, _, err := s.doDPoPForm(ctx, current.TokenEndpoint, form, current.DPoPKey, "")
	if err != nil {
		return accountCredentials{}, fmt.Errorf("refresh AT Protocol OAuth session: %w", err)
	}
	var tokens tokenResponse
	if err := decodeOAuthJSON(response, &tokens); err != nil {
		return accountCredentials{}, err
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || !strings.EqualFold(tokens.TokenType, "DPoP") || !scopeContains(tokens.Scope, "atproto") {
		return accountCredentials{}, fmt.Errorf("%w: refreshed token response incomplete", ErrOAuthResponse)
	}
	if tokens.Sub != "" && tokens.Sub != current.DID {
		return accountCredentials{}, fmt.Errorf("%w: refreshed token identity mismatch", ErrOAuthResponse)
	}
	accessEnc, err := s.encrypt([]byte(tokens.AccessToken)); if err != nil { return accountCredentials{}, err }
	refreshEnc, err := s.encrypt([]byte(tokens.RefreshToken)); if err != nil { return accountCredentials{}, err }
	now := s.now().UTC()
	expires := now.Add(5 * time.Minute)
	if tokens.ExpiresIn > 0 {
		expires = now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE atproto_accounts SET access_token_enc=?,refresh_token_enc=?,scope=?,expires_at=?,updated_at=? WHERE user_id=?
	`, accessEnc, refreshEnc, tokens.Scope, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), current.UserID)
	if err != nil { return accountCredentials{}, fmt.Errorf("persist refreshed AT Protocol tokens: %w", err) }
	count, _ := result.RowsAffected()
	if count != 1 { return accountCredentials{}, ErrNotLinked }
	return s.loadCredentials(ctx, current.UserID)
}

func (s *Store) desiredRecords(ctx context.Context, userID string) ([]desiredRecord, error) {
	var handle, displayName, bio, avatarURL, theme, updatedAt string
	var verified int
	if err := s.db.QueryRowContext(ctx, `
		SELECT handle,display_name,bio,avatar_url,theme,verified,updated_at FROM profiles WHERE user_id=?
	`, userID).Scan(&handle, &displayName, &bio, &avatarURL, &theme, &verified, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return nil, ErrNotLinked }
		return nil, err
	}
	profileRecord := map[string]any{
		"$type": ProfileCollection,
		"displayName": displayName,
		"handle": handle,
		"updatedAt": normalizedRecordTime(updatedAt),
	}
	if bio != "" { profileRecord["bio"] = bio }
	if avatarURL != "" { profileRecord["avatarUrl"] = avatarURL }
	if theme != "" { profileRecord["theme"] = theme }
	if verified != 0 { profileRecord["verified"] = true }
	profilePayload, err := canonicalPayload(profileRecord)
	if err != nil { return nil, err }
	items := []desiredRecord{{Collection: ProfileCollection, RKey: "self", LocalUpdatedAt: updatedAt, Record: profileRecord, Payload: profilePayload}}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id,label,url,kind,thumbnail_url,featured,visible_from,visible_until,position,is_active,updated_at
		FROM links WHERE user_id=? ORDER BY position,id
	`, userID)
	if err != nil { return nil, err }
	type localLink struct {
		id, label, targetURL, kind, thumbnailURL, updatedAt string
		visibleFrom, visibleUntil sql.NullString
		featured, position, active int
	}
	links := make([]localLink, 0)
	for rows.Next() {
		var link localLink
		if err := rows.Scan(&link.id, &link.label, &link.targetURL, &link.kind, &link.thumbnailURL, &link.featured, &link.visibleFrom, &link.visibleUntil, &link.position, &link.active, &link.updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil { rows.Close(); return nil, err }
	if err := rows.Close(); err != nil { return nil, err }

	now := s.now().UTC()
	for _, link := range links {
		if link.active != 1 || !scheduledLinkVisible(now, link.visibleFrom, link.visibleUntil) {
			continue
		}
		if !validRecordKey(link.id) {
			return nil, fmt.Errorf("invalid stable AT Protocol record key for link %s", link.id)
		}
		record := map[string]any{
			"$type": LinkCollection,
			"label": link.label,
			"url": link.targetURL,
			"kind": link.kind,
			"position": link.position,
			"updatedAt": normalizedRecordTime(link.updatedAt),
		}
		if link.thumbnailURL != "" { record["thumbnailUrl"] = link.thumbnailURL }
		if link.featured != 0 { record["featured"] = true }
		payload, err := canonicalPayload(record)
		if err != nil { return nil, err }
		items = append(items, desiredRecord{Collection: LinkCollection, RKey: link.id, LocalUpdatedAt: link.updatedAt, Record: record, Payload: payload})
	}
	return items, nil
}

func scheduledLinkVisible(now time.Time, from, until sql.NullString) bool {
	if from.Valid && strings.TrimSpace(from.String) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, from.String)
		if err != nil || now.Before(parsed) { return false }
	}
	if until.Valid && strings.TrimSpace(until.String) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, until.String)
		if err != nil || !now.Before(parsed) { return false }
	}
	return true
}

func normalizedRecordTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func validRecordKey(value string) bool {
	if value == "" || len(value) > 512 || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || strings.ContainsRune(".-_:~", char) {
			continue
		}
		return false
	}
	return true
}

func canonicalPayload(record map[string]any) (string, error) {
	value, err := json.Marshal(record)
	if err != nil { return "", err }
	return string(value), nil
}

func (s *Store) persistedRecords(ctx context.Context, userID string) ([]persistedRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT collection,rkey,cid,local_updated_at,synced_at,payload FROM atproto_records WHERE user_id=? ORDER BY collection,rkey
	`, userID)
	if err != nil { return nil, err }
	items := make([]persistedRecord, 0)
	for rows.Next() {
		var item persistedRecord
		if err := rows.Scan(&item.Collection, &item.RKey, &item.CID, &item.LocalUpdatedAt, &item.SyncedAt, &item.Payload); err != nil {
			rows.Close(); return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { rows.Close(); return nil, err }
	if err := rows.Close(); err != nil { return nil, err }
	return items, nil
}

func (s *Store) putRecord(ctx context.Context, userID string, credentials accountCredentials, item desiredRecord) (putRecordResponse, error) {
	body := map[string]any{
		"repo": credentials.DID,
		"collection": item.Collection,
		"rkey": item.RKey,
		"record": item.Record,
	}
	var result putRecordResponse
	if err := s.pdsJSON(ctx, userID, credentials, http.MethodPost, "com.atproto.repo.putRecord", nil, body, &result); err != nil {
		return putRecordResponse{}, fmt.Errorf("publish %s/%s: %w", item.Collection, item.RKey, err)
	}
	if result.CID == "" || result.URI != "at://"+credentials.DID+"/"+item.Collection+"/"+item.RKey {
		return putRecordResponse{}, fmt.Errorf("invalid PDS putRecord response")
	}
	return result, nil
}

func (s *Store) deleteRecord(ctx context.Context, userID string, credentials accountCredentials, collection, rkey string) error {
	body := map[string]any{"repo": credentials.DID, "collection": collection, "rkey": rkey}
	var ignored map[string]any
	err := s.pdsJSON(ctx, userID, credentials, http.MethodPost, "com.atproto.repo.deleteRecord", nil, body, &ignored)
	if errors.Is(err, errAccessTokenRejected) { return err }
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "recordnotfound") && !strings.Contains(strings.ToLower(err.Error()), "record not found") {
		return fmt.Errorf("delete %s/%s: %w", collection, rkey, err)
	}
	return nil
}

func (s *Store) savePublishedRecord(ctx context.Context, userID string, item desiredRecord, cid string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO atproto_records(user_id,collection,rkey,cid,local_updated_at,synced_at,payload)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id,collection,rkey) DO UPDATE SET cid=excluded.cid,local_updated_at=excluded.local_updated_at,synced_at=excluded.synced_at,payload=excluded.payload
	`, userID, item.Collection, item.RKey, cid, item.LocalUpdatedAt, now, item.Payload)
	if err != nil { return fmt.Errorf("save AT Protocol record mapping: %w", err) }
	return nil
}

func (s *Store) pdsJSON(ctx context.Context, userID string, credentials accountCredentials, method, nsid string, query url.Values, body any, target any) error {
	endpoint := strings.TrimRight(credentials.PDSURL, "/") + "/xrpc/" + nsid
	if len(query) > 0 { endpoint += "?" + query.Encode() }
	if err := validateATURL(endpoint, s.config.AllowHTTP); err != nil { return err }

	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil { return err }
	}
	nonceKey := strings.TrimRight(credentials.PDSURL, "/")
	nonce := ""
	if cached, ok := s.pdsNonces.Load(nonceKey); ok { nonce, _ = cached.(string) }

	for attempt := 0; attempt < 2; attempt++ {
		proof, err := dpopProof(credentials.DPoPKey, method, endpoint, nonce, credentials.AccessToken, s.now().UTC().Unix())
		if err != nil { return err }
		var reader io.Reader
		if body != nil { reader = bytes.NewReader(encoded) }
		request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil { return err }
		request.Header.Set("DPoP", proof)
		request.Header.Set("Authorization", "DPoP "+credentials.AccessToken)
		if body != nil { request.Header.Set("Content-Type", "application/json") }
		response, err := s.client.Do(request)
		if err != nil { return err }
		responseNonce := strings.TrimSpace(response.Header.Get("DPoP-Nonce"))
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxATProtoResponse+1))
		response.Body.Close()
		if readErr != nil { return readErr }
		if len(data) > maxATProtoResponse { return errors.New("AT Protocol response too large") }
		if responseNonce == "" {
			return fmt.Errorf("%w: PDS response missing DPoP nonce", ErrOAuthResponse)
		}
		s.pdsNonces.Store(nonceKey, responseNonce)
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			if target == nil || len(bytes.TrimSpace(data)) == 0 { return nil }
			if err := json.Unmarshal(data, target); err != nil { return fmt.Errorf("decode PDS response: %w", err) }
			return nil
		}
		var apiErr pdsError
		_ = json.Unmarshal(data, &apiErr)
		name := strings.ToLower(strings.TrimSpace(apiErr.Error))
		message := strings.TrimSpace(apiErr.Message)
		if response.StatusCode == http.StatusUnauthorized && (name == "expiredtoken" || name == "invalidtoken") {
			return errAccessTokenRejected
		}
		if attempt == 0 && responseNonce != nonce && (response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnauthorized) {
			nonce = responseNonce
			continue
		}
		if message == "" { message = strings.TrimSpace(apiErr.Error) }
		if message == "" { message = response.Status }
		return fmt.Errorf("PDS %s failed: %s", nsid, message)
	}
	return ErrOAuthResponse
}
