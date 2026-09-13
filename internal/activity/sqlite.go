package activity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("activity: database is required")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'activity_events'`).Scan(&count); err != nil {
		return nil, fmt.Errorf("verify activity schema: %w", err)
	}
	if count != 1 {
		return nil, errors.New("activity schema missing table activity_events; run Atlas schema apply")
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Record(ctx context.Context, userID, kind, linkID, label string) error {
	userID = strings.TrimSpace(userID)
	linkID = strings.TrimSpace(linkID)
	label = strings.TrimSpace(label)
	if userID == "" {
		return errors.New("activity user is required")
	}
	if err := ValidateKind(kind); err != nil {
		return err
	}
	id, err := activityID()
	if err != nil {
		return err
	}
	var nullableLink any
	if linkID != "" {
		nullableLink = linkID
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO activity_events (id, user_id, kind, link_id, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, userID, kind, nullableLink, label, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record activity: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Following(ctx context.Context, viewerUserID, cursor string, limit int) (Page, error) {
	viewerUserID = strings.TrimSpace(viewerUserID)
	if viewerUserID == "" {
		return Page{}, errors.New("activity viewer is required")
	}
	createdAt, id, err := DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	return s.queryPage(ctx, `
		SELECT e.id, e.kind, p.handle, p.display_name, p.avatar_url, COALESCE(e.link_id, ''), e.label, e.created_at
		FROM activity_events e
		JOIN profiles p ON p.user_id = e.user_id
		JOIN follows f ON f.following_user_id = e.user_id
		WHERE f.follower_user_id = ?
		  AND (? = '' OR e.created_at < ? OR (e.created_at = ? AND e.id < ?))
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ?
	`, limit, viewerUserID, createdAt, createdAt, createdAt, id)
}

func (s *SQLiteStore) Recent(ctx context.Context, cursor string, limit int) (Page, error) {
	createdAt, id, err := DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	return s.queryPage(ctx, `
		SELECT e.id, e.kind, p.handle, p.display_name, p.avatar_url, COALESCE(e.link_id, ''), e.label, e.created_at
		FROM activity_events e
		JOIN profiles p ON p.user_id = e.user_id
		WHERE (? = '' OR e.created_at < ? OR (e.created_at = ? AND e.id < ?))
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ?
	`, limit, createdAt, createdAt, createdAt, id)
}

func (s *SQLiteStore) Trending(ctx context.Context, limit int) ([]Trend, error) {
	limit = NormalizeLimit(limit)
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			p.handle,
			p.display_name,
			p.avatar_url,
			COUNT(DISTINCT e.id) AS recent_activity,
			(SELECT COUNT(*) FROM follows f WHERE f.following_user_id = p.user_id) AS follower_count
		FROM profiles p
		LEFT JOIN activity_events e ON e.user_id = p.user_id AND e.created_at >= ?
		GROUP BY p.user_id, p.handle, p.display_name, p.avatar_url
		ORDER BY (COUNT(DISTINCT e.id) * 3 + (SELECT COUNT(*) FROM follows f2 WHERE f2.following_user_id = p.user_id)) DESC, p.handle
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("query trending creators: %w", err)
	}
	defer rows.Close()
	items := make([]Trend, 0)
	for rows.Next() {
		var item Trend
		if err := rows.Scan(&item.Handle, &item.DisplayName, &item.AvatarURL, &item.RecentActivity, &item.FollowerCount); err != nil {
			return nil, fmt.Errorf("scan trending creator: %w", err)
		}
		item.Score = item.RecentActivity*3 + item.FollowerCount
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trending creators: %w", err)
	}
	return items, nil
}

func (s *SQLiteStore) queryPage(ctx context.Context, query string, limit int, args ...any) (Page, error) {
	limit = NormalizeLimit(limit)
	queryLimit := limit + 1
	args = append(args, queryLimit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Page{}, fmt.Errorf("query activity page: %w", err)
	}
	defer rows.Close()
	items := make([]Event, 0, queryLimit)
	for rows.Next() {
		var item Event
		if err := rows.Scan(&item.ID, &item.Kind, &item.Handle, &item.DisplayName, &item.AvatarURL, &item.LinkID, &item.Label, &item.CreatedAt); err != nil {
			return Page{}, fmt.Errorf("scan activity event: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate activity page: %w", err)
	}
	page := Page{Events: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Events = items[:limit]
		page.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func activityID() (string, error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate activity id: %w", err)
	}
	return "evt_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}
