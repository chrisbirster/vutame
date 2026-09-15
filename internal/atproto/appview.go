package atproto

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrIndexedNotFound = errors.New("portable Vutame profile not found")

func (s *Store) IndexedProfile(ctx context.Context, did string) (IndexedProfile, error) {
	did = strings.TrimSpace(did)
	var item IndexedProfile
	var verified int
	if err := s.db.QueryRowContext(ctx, `
		SELECT did,handle,display_name,bio,avatar_url,theme,verified,indexed_at
		FROM atproto_indexed_profiles WHERE did=?
	`, did).Scan(&item.DID, &item.Handle, &item.DisplayName, &item.Bio, &item.AvatarURL, &item.Theme, &verified, &item.IndexedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IndexedProfile{}, ErrIndexedNotFound
		}
		return IndexedProfile{}, err
	}
	item.Verified = verified == 1
	links, err := s.indexedLinks(ctx, did)
	if err != nil {
		return IndexedProfile{}, err
	}
	item.Links = links
	return item, nil
}

func (s *Store) SearchIndexed(ctx context.Context, query string, limit int) ([]IndexedProfile, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.did,i.handle,i.display_name,i.bio,i.avatar_url,i.theme,i.verified,i.indexed_at
		FROM atproto_indexed_profiles i
		WHERE NOT EXISTS (SELECT 1 FROM profiles p WHERE p.atproto_did=i.did)
		  AND (?='' OR lower(i.handle) LIKE ? OR lower(i.display_name) LIKE ? OR lower(i.bio) LIKE ?)
		ORDER BY i.indexed_at DESC,i.did
		LIMIT ?
	`, query, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	items := make([]IndexedProfile, 0)
	for rows.Next() {
		var item IndexedProfile
		var verified int
		if err := rows.Scan(&item.DID, &item.Handle, &item.DisplayName, &item.Bio, &item.AvatarURL, &item.Theme, &verified, &item.IndexedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.Verified = verified == 1
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	for index := range items {
		links, err := s.indexedLinks(ctx, items[index].DID)
		if err != nil {
			return nil, err
		}
		items[index].Links = links
	}
	return items, nil
}

func (s *Store) indexedLinks(ctx context.Context, did string) ([]IndexedLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rkey,label,url,kind,thumbnail_url,featured,position
		FROM atproto_indexed_links WHERE did=? ORDER BY featured DESC,position,rkey
	`, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]IndexedLink, 0)
	for rows.Next() {
		var item IndexedLink
		var featured int
		if err := rows.Scan(&item.RKey, &item.Label, &item.URL, &item.Kind, &item.ThumbnailURL, &featured, &item.Position); err != nil {
			return nil, err
		}
		item.Featured = featured == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) indexPortableRecord(ctx context.Context, did, collection, rkey, cid string, record map[string]any, indexedAt time.Time) error {
	did = strings.TrimSpace(did)
	if !strings.HasPrefix(did, "did:") || strings.TrimSpace(rkey) == "" {
		return ErrInvalidIdentity
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	stamp := indexedAt.UTC().Format(time.RFC3339Nano)
	switch collection {
	case ProfileCollection:
		profileRecord, err := validatePortableProfileRecord(record)
		if err != nil {
			return err
		}
		verified := 0
		var trusted int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE atproto_did=? AND verified=1`, did).Scan(&trusted)
		if trusted > 0 {
			verified = 1
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO atproto_indexed_profiles(did,handle,display_name,bio,avatar_url,theme,verified,record_json,indexed_at)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(did) DO UPDATE SET
				handle=excluded.handle,display_name=excluded.display_name,bio=excluded.bio,
				avatar_url=excluded.avatar_url,theme=excluded.theme,verified=excluded.verified,
				record_json=excluded.record_json,indexed_at=excluded.indexed_at
		`, did, profileRecord.Handle, profileRecord.DisplayName, profileRecord.Bio, profileRecord.AvatarURL, profileRecord.Theme, verified, string(encoded), stamp)
		return err
	case LinkCollection:
		link, err := validatePortableLinkRecord(rkey, record)
		if err != nil {
			return err
		}
		// Jetstream can replay a link before its profile record is present in a
		// bounded window. Keep a placeholder parent so the link survives until
		// the profile event arrives and fills in the public fields.
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO atproto_indexed_profiles(did,handle,display_name,bio,avatar_url,theme,verified,record_json,indexed_at)
			VALUES(?,'','','','','midnight',0,'{}',?) ON CONFLICT(did) DO NOTHING
		`, did, stamp)
		if err != nil {
			return err
		}
		featured := 0
		if link.Featured {
			featured = 1
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO atproto_indexed_links(did,rkey,label,url,kind,thumbnail_url,featured,position,record_json,indexed_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(did,rkey) DO UPDATE SET
				label=excluded.label,url=excluded.url,kind=excluded.kind,thumbnail_url=excluded.thumbnail_url,
				featured=excluded.featured,position=excluded.position,record_json=excluded.record_json,indexed_at=excluded.indexed_at
		`, did, rkey, link.Label, link.URL, link.Kind, link.ThumbnailURL, featured, link.Position, string(encoded), stamp)
		return err
	default:
		return fmt.Errorf("unsupported portable collection %q", collection)
	}
}

func (s *Store) deleteIndexedPortableRecord(ctx context.Context, did, collection, rkey string) error {
	switch collection {
	case ProfileCollection:
		_, err := s.db.ExecContext(ctx, `DELETE FROM atproto_indexed_profiles WHERE did=?`, did)
		return err
	case LinkCollection:
		_, err := s.db.ExecContext(ctx, `DELETE FROM atproto_indexed_links WHERE did=? AND rkey=?`, did, rkey)
		return err
	default:
		return nil
	}
}

func (s *Store) purgeIndexedDID(ctx context.Context, did string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM atproto_indexed_profiles WHERE did=?`, strings.TrimSpace(did))
	return err
}

type portableProfileRecord struct {
	DisplayName string
	Handle      string
	Bio         string
	AvatarURL   string
	Theme       string
	UpdatedAt   string
}

func validatePortableProfileRecord(record map[string]any) (portableProfileRecord, error) {
	if recordType(record) != ProfileCollection {
		return portableProfileRecord{}, ErrInvalidIdentity
	}
	item := portableProfileRecord{
		DisplayName: stringField(record, "displayName"),
		Handle:      strings.ToLower(stringField(record, "handle")),
		Bio:         stringField(record, "bio"),
		AvatarURL:   stringField(record, "avatarUrl"),
		Theme:       stringField(record, "theme"),
		UpdatedAt:   stringField(record, "updatedAt"),
	}
	if item.DisplayName == "" || item.Handle == "" || item.UpdatedAt == "" || runeLen(item.DisplayName) > 80 || len(item.Handle) > 253 || runeLen(item.Bio) > 320 {
		return portableProfileRecord{}, ErrInvalidIdentity
	}
	if _, err := time.Parse(time.RFC3339Nano, item.UpdatedAt); err != nil {
		return portableProfileRecord{}, ErrInvalidIdentity
	}
	if item.Theme == "" {
		item.Theme = "midnight"
	}
	if len(item.Theme) > 40 || (item.AvatarURL != "" && !validPortableURL(item.AvatarURL)) {
		return portableProfileRecord{}, ErrInvalidIdentity
	}
	return item, nil
}

func validatePortableLinkRecord(rkey string, record map[string]any) (IndexedLink, error) {
	if recordType(record) != LinkCollection {
		return IndexedLink{}, ErrInvalidIdentity
	}
	position, ok := integerField(record, "position")
	if !ok || position < 0 {
		return IndexedLink{}, ErrInvalidIdentity
	}
	item := IndexedLink{
		RKey:         rkey,
		Label:        stringField(record, "label"),
		URL:          stringField(record, "url"),
		Kind:         stringField(record, "kind"),
		ThumbnailURL: stringField(record, "thumbnailUrl"),
		Featured:     boolField(record, "featured"),
		Position:     position,
	}
	updatedAt := stringField(record, "updatedAt")
	if item.Label == "" || runeLen(item.Label) > 100 || item.Kind == "" || len(item.Kind) > 40 || !validPortableURL(item.URL) {
		return IndexedLink{}, ErrInvalidIdentity
	}
	if item.ThumbnailURL != "" && !validPortableURL(item.ThumbnailURL) {
		return IndexedLink{}, ErrInvalidIdentity
	}
	if _, err := time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return IndexedLink{}, ErrInvalidIdentity
	}
	return item, nil
}

func recordType(record map[string]any) string {
	return stringField(record, "$type")
}

func stringField(record map[string]any, name string) string {
	value, _ := record[name].(string)
	return strings.TrimSpace(value)
}

func boolField(record map[string]any, name string) bool {
	value, _ := record[name].(bool)
	return value
}

func integerField(record map[string]any, name string) (int, bool) {
	switch value := record[name].(type) {
	case float64:
		if value < 0 || value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	case int:
		return value, true
	case json.Number:
		parsed, err := value.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func validPortableURL(value string) bool {
	if len(value) > 2048 {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}
