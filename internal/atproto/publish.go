package atproto

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type SyncConflict struct {
	Collection string `json:"collection"`
	RKey       string `json:"rkey"`
	Reason     string `json:"reason"`
}

type SyncReport struct {
	DID        string         `json:"did"`
	Published  int            `json:"published"`
	Deleted    int            `json:"deleted"`
	Conflicts  []SyncConflict `json:"conflicts"`
	SyncedAt   string         `json:"synced_at"`
}

type SyncedRecord struct {
	Collection     string `json:"collection"`
	RKey           string `json:"rkey"`
	CID            string `json:"cid,omitempty"`
	LocalUpdatedAt string `json:"local_updated_at,omitempty"`
	SyncedAt       string `json:"synced_at"`
}

type SyncStatus struct {
	Account Account        `json:"account"`
	Records []SyncedRecord `json:"records"`
}

type localRecord struct {
	Collection     string
	RKey           string
	LocalID        string
	LocalUpdatedAt string
	Record         map[string]any
}

type storedRecord struct {
	CID            string
	LocalUpdatedAt string
	SyncedAt       string
	Payload        string
}

type remoteRecord struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

type putRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

var errRemoteRecordNotFound = errors.New("AT Protocol remote record not found")

func (s *Store) Sync(ctx context.Context, userID string) (SyncReport, error) {
	account, err := s.Account(ctx, userID)
	if err != nil {
		return SyncReport{}, err
	}
	if !account.PublishEnabled {
		return SyncReport{}, ErrPublishingDisabled
	}
	session, err := s.accessSession(ctx, userID)
	if err != nil {
		return SyncReport{}, err
	}
	if !scopeContains(session.Scope, "repo:"+ProfileCollection) || !scopeContains(session.Scope, "repo:"+LinkCollection) {
		return SyncReport{}, fmt.Errorf("%w: linked session does not grant Vutame record scopes", ErrOAuthResponse)
	}
	records, err := s.localPortableRecords(ctx, userID)
	if err != nil {
		return SyncReport{}, err
	}
	report := SyncReport{DID: session.DID, Conflicts: []SyncConflict{}}
	desiredLinks := make(map[string]struct{})
	for _, item := range records {
		if item.Collection == LinkCollection {
			desiredLinks[item.RKey] = struct{}{}
		}
		outcome, conflict, err := s.syncPortableRecord(ctx, account, session, item)
		if err != nil {
			return SyncReport{}, err
		}
		if conflict != nil {
			report.Conflicts = append(report.Conflicts, *conflict)
			continue
		}
		if outcome {
			report.Published++
		}
	}

	deleted, conflicts, err := s.deleteStalePortableLinks(ctx, account, session, desiredLinks)
	if err != nil {
		return SyncReport{}, err
	}
	report.Deleted += deleted
	report.Conflicts = append(report.Conflicts, conflicts...)
	report.SyncedAt = s.now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func (s *Store) Status(ctx context.Context, userID string) (SyncStatus, error) {
	account, err := s.Account(ctx, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT collection,rkey,cid,local_updated_at,synced_at
		FROM atproto_records WHERE user_id=? ORDER BY collection,rkey
	`, strings.TrimSpace(userID))
	if err != nil {
		return SyncStatus{}, err
	}
	defer rows.Close()
	items := make([]SyncedRecord, 0)
	for rows.Next() {
		var item SyncedRecord
		if err := rows.Scan(&item.Collection, &item.RKey, &item.CID, &item.LocalUpdatedAt, &item.SyncedAt); err != nil {
			return SyncStatus{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SyncStatus{}, err
	}
	return SyncStatus{Account: account, Records: items}, nil
}

func (s *Store) localPortableRecords(ctx context.Context, userID string) ([]localRecord, error) {
	var displayName, handle, bio, avatarURL, theme, updatedAt string
	var verified int
	if err := s.db.QueryRowContext(ctx, `
		SELECT display_name,handle,bio,avatar_url,theme,verified,updated_at
		FROM profiles WHERE user_id=?
	`, userID).Scan(&displayName, &handle, &bio, &avatarURL, &theme, &verified, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotLinked
		}
		return nil, err
	}
	profileRecord := map[string]any{
		"$type":       ProfileCollection,
		"displayName": displayName,
		"handle":      handle,
		"theme":       theme,
		"verified":    verified == 1,
		"updatedAt":   normalizeRecordTime(updatedAt, s.now()),
	}
	if bio != "" {
		profileRecord["bio"] = bio
	}
	if avatarURL != "" {
		profileRecord["avatarUrl"] = avatarURL
	}
	items := []localRecord{{Collection: ProfileCollection, RKey: "self", LocalUpdatedAt: updatedAt, Record: profileRecord}}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id,label,url,kind,thumbnail_url,featured,position,is_active,COALESCE(visible_from,''),COALESCE(visible_until,''),updated_at
		FROM links WHERE user_id=? ORDER BY position,id
	`, userID)
	if err != nil {
		return nil, err
	}
	type localLink struct {
		id, label, rawURL, kind, thumbnail, visibleFrom, visibleUntil, updatedAt string
		featured, active                                                        int
		position                                                                int
	}
	links := make([]localLink, 0)
	for rows.Next() {
		var link localLink
		if err := rows.Scan(&link.id, &link.label, &link.rawURL, &link.kind, &link.thumbnail, &link.featured, &link.position, &link.active, &link.visibleFrom, &link.visibleUntil, &link.updatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	now := s.now().UTC()
	for _, link := range links {
		if link.active != 1 || !portableLinkVisible(link.visibleFrom, link.visibleUntil, now) {
			continue
		}
		record := map[string]any{
			"$type":     LinkCollection,
			"label":     link.label,
			"url":       link.rawURL,
			"kind":      link.kind,
			"featured":  link.featured == 1,
			"position":  link.position,
			"updatedAt": normalizeRecordTime(link.updatedAt, now),
		}
		if link.thumbnail != "" {
			record["thumbnailUrl"] = link.thumbnail
		}
		items = append(items, localRecord{
			Collection: LinkCollection,
			RKey: stableLinkRKey(link.id),
			LocalID: link.id,
			LocalUpdatedAt: link.updatedAt,
			Record: record,
		})
	}
	return items, nil
}

func (s *Store) syncPortableRecord(ctx context.Context, account Account, session sessionCredentials, desired localRecord) (bool, *SyncConflict, error) {
	stored, hasStored, err := s.storedPortableRecord(ctx, session.UserID, desired.Collection, desired.RKey)
	if err != nil {
		return false, nil, err
	}
	remote, remoteExists, err := s.getRemoteRecord(ctx, session, desired.Collection, desired.RKey)
	if err != nil {
		return false, nil, err
	}

	if account.ConflictPolicy == "pds_wins" {
		diverged := remoteExists && (!hasStored || remote.CID != stored.CID)
		remoteDeleted := hasStored && !remoteExists && stored.CID != ""
		if diverged || remoteDeleted {
			reason := "remote record changed since the last Vutame sync"
			if remoteDeleted {
				reason = "remote record was deleted since the last Vutame sync"
			}
			if remoteExists {
				payload, _ := json.Marshal(remote.Value)
				if err := s.rememberPortableRecord(ctx, session.UserID, desired.Collection, desired.RKey, remote.CID, desired.LocalUpdatedAt, string(payload)); err != nil {
					return false, nil, err
				}
				_ = s.indexPortableRecord(ctx, session.DID, desired.Collection, desired.RKey, remote.CID, remote.Value, s.now().UTC())
			} else {
				if err := s.forgetPortableRecord(ctx, session.UserID, desired.Collection, desired.RKey); err != nil {
					return false, nil, err
				}
				_ = s.deleteIndexedPortableRecord(ctx, session.DID, desired.Collection, desired.RKey)
			}
			return false, &SyncConflict{Collection: desired.Collection, RKey: desired.RKey, Reason: reason}, nil
		}
	}

	payload, err := canonicalJSON(desired.Record)
	if err != nil {
		return false, nil, err
	}
	if hasStored && remoteExists && remote.CID == stored.CID && stored.LocalUpdatedAt == desired.LocalUpdatedAt && stored.Payload == payload {
		return false, nil, nil
	}

	swapRecord := ""
	if account.ConflictPolicy == "pds_wins" && remoteExists {
		swapRecord = remote.CID
	}
	written, err := s.putRemoteRecord(ctx, session, desired.Collection, desired.RKey, desired.Record, swapRecord)
	if err != nil {
		if account.ConflictPolicy == "pds_wins" && errors.Is(err, ErrConflict) {
			return false, &SyncConflict{Collection: desired.Collection, RKey: desired.RKey, Reason: "remote record changed during sync"}, nil
		}
		return false, nil, err
	}
	if err := s.rememberPortableRecord(ctx, session.UserID, desired.Collection, desired.RKey, written.CID, desired.LocalUpdatedAt, payload); err != nil {
		return false, nil, err
	}
	if err := s.indexPortableRecord(ctx, session.DID, desired.Collection, desired.RKey, written.CID, desired.Record, s.now().UTC()); err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

func (s *Store) deleteStalePortableLinks(ctx context.Context, account Account, session sessionCredentials, desired map[string]struct{}) (int, []SyncConflict, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rkey,cid,local_updated_at,synced_at,payload
		FROM atproto_records WHERE user_id=? AND collection=? ORDER BY rkey
	`, session.UserID, LinkCollection)
	if err != nil {
		return 0, nil, err
	}
	type existing struct {
		rkey string
		storedRecord
	}
	stale := make([]existing, 0)
	for rows.Next() {
		var item existing
		if err := rows.Scan(&item.rkey, &item.CID, &item.LocalUpdatedAt, &item.SyncedAt, &item.Payload); err != nil {
			_ = rows.Close()
			return 0, nil, err
		}
		if _, keep := desired[item.rkey]; !keep {
			stale = append(stale, item)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, nil, err
	}
	_ = rows.Close()

	deleted := 0
	conflicts := make([]SyncConflict, 0)
	for _, item := range stale {
		remote, exists, err := s.getRemoteRecord(ctx, session, LinkCollection, item.rkey)
		if err != nil {
			return deleted, conflicts, err
		}
		if !exists {
			if err := s.forgetPortableRecord(ctx, session.UserID, LinkCollection, item.rkey); err != nil {
				return deleted, conflicts, err
			}
			_ = s.deleteIndexedPortableRecord(ctx, session.DID, LinkCollection, item.rkey)
			continue
		}
		if account.ConflictPolicy == "pds_wins" && remote.CID != item.CID {
			payload, _ := json.Marshal(remote.Value)
			if err := s.rememberPortableRecord(ctx, session.UserID, LinkCollection, item.rkey, remote.CID, item.LocalUpdatedAt, string(payload)); err != nil {
				return deleted, conflicts, err
			}
			_ = s.indexPortableRecord(ctx, session.DID, LinkCollection, item.rkey, remote.CID, remote.Value, s.now().UTC())
			conflicts = append(conflicts, SyncConflict{Collection: LinkCollection, RKey: item.rkey, Reason: "remote link changed; pds_wins preserved it"})
			continue
		}
		swap := ""
		if account.ConflictPolicy == "pds_wins" {
			swap = remote.CID
		}
		if err := s.deleteRemoteRecord(ctx, session, LinkCollection, item.rkey, swap); err != nil {
			if account.ConflictPolicy == "pds_wins" && errors.Is(err, ErrConflict) {
				conflicts = append(conflicts, SyncConflict{Collection: LinkCollection, RKey: item.rkey, Reason: "remote link changed during deletion"})
				continue
			}
			return deleted, conflicts, err
		}
		if err := s.forgetPortableRecord(ctx, session.UserID, LinkCollection, item.rkey); err != nil {
			return deleted, conflicts, err
		}
		_ = s.deleteIndexedPortableRecord(ctx, session.DID, LinkCollection, item.rkey)
		deleted++
	}
	return deleted, conflicts, nil
}

func (s *Store) storedPortableRecord(ctx context.Context, userID, collection, rkey string) (storedRecord, bool, error) {
	var item storedRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT cid,local_updated_at,synced_at,payload FROM atproto_records
		WHERE user_id=? AND collection=? AND rkey=?
	`, userID, collection, rkey).Scan(&item.CID, &item.LocalUpdatedAt, &item.SyncedAt, &item.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return storedRecord{}, false, nil
	}
	if err != nil {
		return storedRecord{}, false, err
	}
	return item, true, nil
}

func (s *Store) rememberPortableRecord(ctx context.Context, userID, collection, rkey, cid, localUpdatedAt, payload string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO atproto_records(user_id,collection,rkey,cid,local_updated_at,synced_at,payload)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id,collection,rkey) DO UPDATE SET
			cid=excluded.cid,local_updated_at=excluded.local_updated_at,synced_at=excluded.synced_at,payload=excluded.payload
	`, userID, collection, rkey, cid, localUpdatedAt, now, payload)
	return err
}

func (s *Store) forgetPortableRecord(ctx context.Context, userID, collection, rkey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM atproto_records WHERE user_id=? AND collection=? AND rkey=?`, userID, collection, rkey)
	return err
}

func (s *Store) getRemoteRecord(ctx context.Context, session sessionCredentials, collection, rkey string) (remoteRecord, bool, error) {
	endpoint := strings.TrimRight(session.PDSURL, "/") + "/xrpc/com.atproto.repo.getRecord"
	query := url.Values{"repo": {session.DID}, "collection": {collection}, "rkey": {rkey}}
	endpoint += "?" + query.Encode()
	response, err := s.doDPoPJSON(ctx, http.MethodGet, endpoint, nil, session)
	if err != nil {
		return remoteRecord{}, false, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxATProtoResponse+1))
	response.Body.Close()
	if err != nil {
		return remoteRecord{}, false, err
	}
	if len(data) > maxATProtoResponse {
		return remoteRecord{}, false, ErrOAuthResponse
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct{ Error string `json:"error"` }
		_ = json.Unmarshal(data, &apiError)
		if response.StatusCode == http.StatusNotFound || apiError.Error == "RecordNotFound" {
			return remoteRecord{}, false, nil
		}
		return remoteRecord{}, false, fmt.Errorf("get AT Protocol record: HTTP %d %s", response.StatusCode, apiError.Error)
	}
	var item remoteRecord
	if err := json.Unmarshal(data, &item); err != nil || item.CID == "" || item.Value == nil {
		return remoteRecord{}, false, ErrOAuthResponse
	}
	return item, true, nil
}

func (s *Store) putRemoteRecord(ctx context.Context, session sessionCredentials, collection, rkey string, record map[string]any, swapRecord string) (putRecordResponse, error) {
	endpoint := strings.TrimRight(session.PDSURL, "/") + "/xrpc/com.atproto.repo.putRecord"
	body := map[string]any{
		"repo": session.DID, "collection": collection, "rkey": rkey, "record": record, "validate": true,
	}
	if swapRecord != "" {
		body["swapRecord"] = swapRecord
	}
	response, err := s.doDPoPJSON(ctx, http.MethodPost, endpoint, body, session)
	if err != nil {
		return putRecordResponse{}, err
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusConflict {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		response.Body.Close()
		if strings.Contains(string(data), "InvalidSwap") || strings.Contains(string(data), "swap") {
			return putRecordResponse{}, ErrConflict
		}
		return putRecordResponse{}, fmt.Errorf("put AT Protocol record: HTTP %d", response.StatusCode)
	}
	var result putRecordResponse
	if err := readJSONResponse(response, &result); err != nil {
		return putRecordResponse{}, err
	}
	if result.CID == "" {
		return putRecordResponse{}, ErrOAuthResponse
	}
	return result, nil
}

func (s *Store) deleteRemoteRecord(ctx context.Context, session sessionCredentials, collection, rkey, swapRecord string) error {
	endpoint := strings.TrimRight(session.PDSURL, "/") + "/xrpc/com.atproto.repo.deleteRecord"
	body := map[string]any{"repo": session.DID, "collection": collection, "rkey": rkey}
	if swapRecord != "" {
		body["swapRecord"] = swapRecord
	}
	response, err := s.doDPoPJSON(ctx, http.MethodPost, endpoint, body, session)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if strings.Contains(string(data), "InvalidSwap") || strings.Contains(string(data), "swap") {
		return ErrConflict
	}
	return fmt.Errorf("delete AT Protocol record: HTTP %d", response.StatusCode)
}

func canonicalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stableLinkRKey(id string) string {
	digest := sha256.Sum256([]byte(id))
	return "v" + hex.EncodeToString(digest[:16])
}

func portableLinkVisible(fromValue, untilValue string, now time.Time) bool {
	if fromValue != "" {
		from, err := time.Parse(time.RFC3339Nano, fromValue)
		if err != nil || now.Before(from) {
			return false
		}
	}
	if untilValue != "" {
		until, err := time.Parse(time.RFC3339Nano, untilValue)
		if err != nil || !now.Before(until) {
			return false
		}
	}
	return true
}

func normalizeRecordTime(value string, fallback time.Time) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return fallback.UTC().Format(time.RFC3339Nano)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var _ = sortedKeys
