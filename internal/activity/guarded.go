package activity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type GuardedStore struct {
	base Store
	db   *sql.DB
}

func NewGuardedStore(base Store, db *sql.DB) (*GuardedStore, error) {
	if base == nil || db == nil {
		return nil, errors.New("activity: guarded store requires base store and database")
	}
	return &GuardedStore{base: base, db: db}, nil
}

func (s *GuardedStore) Record(ctx context.Context, userID, kind, linkID, label string) error {
	return s.base.Record(ctx, userID, kind, linkID, label)
}

func (s *GuardedStore) Following(ctx context.Context, viewerUserID, cursor string, limit int) (Page, error) {
	return s.pageForViewer(ctx, viewerUserID, cursor, limit, func(next string, amount int) (Page, error) {
		return s.base.Following(ctx, viewerUserID, next, amount)
	})
}

func (s *GuardedStore) Recent(ctx context.Context, cursor string, limit int) (Page, error) {
	return s.RecentForViewer(ctx, "", cursor, limit)
}

func (s *GuardedStore) RecentForViewer(ctx context.Context, viewerUserID, cursor string, limit int) (Page, error) {
	return s.pageForViewer(ctx, viewerUserID, cursor, limit, func(next string, amount int) (Page, error) {
		return s.base.Recent(ctx, next, amount)
	})
}

func (s *GuardedStore) Trending(ctx context.Context, limit int) ([]Trend, error) {
	return s.TrendingForViewer(ctx, "", limit)
}

func (s *GuardedStore) TrendingForViewer(ctx context.Context, viewerUserID string, limit int) ([]Trend, error) {
	limit = NormalizeLimit(limit)
	// Ask the base store for a wider candidate set so policy filtering does not
	// make the list artificially tiny while remaining bounded.
	candidates, err := s.base.Trending(ctx, MaxLimit)
	if err != nil {
		return nil, err
	}
	items := make([]Trend, 0, limit)
	for _, item := range candidates {
		userID, err := s.userIDForHandle(ctx, item.Handle)
		if err != nil {
			continue
		}
		visible, err := s.activityVisible(ctx, userID, viewerUserID, true)
		if err != nil {
			return nil, err
		}
		if visible {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
}

func (s *GuardedStore) pageForViewer(ctx context.Context, viewerUserID, cursor string, limit int, fetch func(string, int) (Page, error)) (Page, error) {
	limit = NormalizeLimit(limit)
	result := Page{Events: make([]Event, 0, limit)}
	next := strings.TrimSpace(cursor)
	for len(result.Events) < limit {
		remaining := limit - len(result.Events)
		page, err := fetch(next, remaining)
		if err != nil {
			return Page{}, err
		}
		for _, event := range page.Events {
			userID, err := s.userIDForHandle(ctx, event.Handle)
			if err != nil {
				continue
			}
			visible, err := s.activityVisible(ctx, userID, viewerUserID, false)
			if err != nil {
				return Page{}, err
			}
			if visible {
				result.Events = append(result.Events, event)
			}
		}
		next = page.NextCursor
		if next == "" {
			break
		}
	}
	result.NextCursor = next
	return result, nil
}

func (s *GuardedStore) activityVisible(ctx context.Context, authorUserID, viewerUserID string, requireDiscoverable bool) (bool, error) {
	var discoverable, activityVisible int
	var moderation string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(priv.discoverable, 1), COALESCE(priv.activity_visible, 1), COALESCE(mod.state, 'active')
		FROM profiles p
		LEFT JOIN creator_privacy priv ON priv.user_id=p.user_id
		LEFT JOIN moderation_profiles mod ON mod.user_id=p.user_id
		WHERE p.user_id=?
	`, authorUserID).Scan(&discoverable, &activityVisible, &moderation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check activity visibility: %w", err)
	}
	if activityVisible == 0 || moderation != "active" || (requireDiscoverable && discoverable == 0) {
		return false, nil
	}
	viewerUserID = strings.TrimSpace(viewerUserID)
	if viewerUserID == "" || viewerUserID == authorUserID {
		return true, nil
	}
	var hidden int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM blocks WHERE (blocker_user_id=? AND blocked_user_id=?) OR (blocker_user_id=? AND blocked_user_id=?)
			UNION ALL
			SELECT 1 FROM mutes WHERE muter_user_id=? AND muted_user_id=?
		)
	`, viewerUserID, authorUserID, authorUserID, viewerUserID, viewerUserID, authorUserID).Scan(&hidden); err != nil {
		return false, fmt.Errorf("check activity relationship policy: %w", err)
	}
	return hidden == 0, nil
}

func (s *GuardedStore) userIDForHandle(ctx context.Context, handle string) (string, error) {
	var userID string
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE handle=?`, strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))).Scan(&userID); err != nil {
		return "", err
	}
	return userID, nil
}
