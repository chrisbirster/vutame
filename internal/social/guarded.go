package social

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
		return nil, errors.New("social: guarded store requires base store and database")
	}
	return &GuardedStore{base: base, db: db}, nil
}

func (s *GuardedStore) Follow(ctx context.Context, viewerUserID, handle string) error {
	targetUserID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return err
	}
	blocked, err := s.blockedEitherDirection(ctx, viewerUserID, targetUserID)
	if err != nil {
		return err
	}
	if blocked {
		return ErrRelationshipBlocked
	}
	var allowFollows int
	var moderation string
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(priv.allow_follows, 1), COALESCE(mod.state, 'active')
		FROM profiles p
		LEFT JOIN creator_privacy priv ON priv.user_id=p.user_id
		LEFT JOIN moderation_profiles mod ON mod.user_id=p.user_id
		WHERE p.user_id=?
	`, targetUserID).Scan(&allowFollows, &moderation)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check follow policy: %w", err)
	}
	if allowFollows == 0 || moderation == "suspended" {
		return ErrFollowsDisabled
	}
	return s.base.Follow(ctx, viewerUserID, handle)
}

func (s *GuardedStore) Unfollow(ctx context.Context, viewerUserID, handle string) error {
	return s.base.Unfollow(ctx, viewerUserID, handle)
}

func (s *GuardedStore) Creator(ctx context.Context, handle, viewerUserID string) (Creator, error) {
	userID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return Creator{}, err
	}
	visible, err := s.visible(ctx, userID, viewerUserID, false)
	if err != nil {
		return Creator{}, err
	}
	if !visible {
		return Creator{}, ErrNotFound
	}
	return s.base.Creator(ctx, handle, viewerUserID)
}

func (s *GuardedStore) Followers(ctx context.Context, handle, viewerUserID string) ([]Creator, error) {
	items, err := s.base.Followers(ctx, handle, viewerUserID)
	if err != nil {
		return nil, err
	}
	return s.filterCreators(ctx, items, viewerUserID, false)
}

func (s *GuardedStore) Following(ctx context.Context, handle, viewerUserID string) ([]Creator, error) {
	items, err := s.base.Following(ctx, handle, viewerUserID)
	if err != nil {
		return nil, err
	}
	return s.filterCreators(ctx, items, viewerUserID, false)
}

func (s *GuardedStore) Search(ctx context.Context, input SearchInput, viewerUserID string) ([]Creator, error) {
	page, err := s.SearchPage(ctx, input, viewerUserID)
	if err != nil {
		return nil, err
	}
	return page.Creators, nil
}

func (s *GuardedStore) SearchPage(ctx context.Context, input SearchInput, viewerUserID string) (CreatorPage, error) {
	input, err := NormalizeSearch(input)
	if err != nil {
		return CreatorPage{}, err
	}
	wanted := input.Limit
	cursor := input.Cursor
	result := CreatorPage{Creators: make([]Creator, 0, wanted)}
	for len(result.Creators) < wanted {
		remaining := wanted - len(result.Creators)
		request := input
		request.Cursor = cursor
		request.Limit = remaining
		page, err := s.base.SearchPage(ctx, request, viewerUserID)
		if err != nil {
			return CreatorPage{}, err
		}
		visible, err := s.filterCreators(ctx, page.Creators, viewerUserID, true)
		if err != nil {
			return CreatorPage{}, err
		}
		result.Creators = append(result.Creators, visible...)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	result.NextCursor = cursor
	return result, nil
}

func (s *GuardedStore) UpdateMetadata(ctx context.Context, userID string, input MetadataInput) (Creator, error) {
	return s.base.UpdateMetadata(ctx, userID, input)
}

func (s *GuardedStore) filterCreators(ctx context.Context, items []Creator, viewerUserID string, discoverableOnly bool) ([]Creator, error) {
	filtered := make([]Creator, 0, len(items))
	for _, item := range items {
		userID, err := s.userIDForHandle(ctx, item.Handle)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		visible, err := s.visible(ctx, userID, viewerUserID, discoverableOnly)
		if err != nil {
			return nil, err
		}
		if visible {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *GuardedStore) visible(ctx context.Context, targetUserID, viewerUserID string, discoverableOnly bool) (bool, error) {
	var discoverable int
	var moderation string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(priv.discoverable, 1), COALESCE(mod.state, 'active')
		FROM profiles p
		LEFT JOIN creator_privacy priv ON priv.user_id=p.user_id
		LEFT JOIN moderation_profiles mod ON mod.user_id=p.user_id
		WHERE p.user_id=?
	`, targetUserID).Scan(&discoverable, &moderation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check creator visibility: %w", err)
	}
	if moderation == "suspended" || (discoverableOnly && (discoverable == 0 || moderation != "active")) {
		return false, nil
	}
	viewerUserID = strings.TrimSpace(viewerUserID)
	if viewerUserID != "" && viewerUserID != targetUserID {
		blocked, err := s.blockedEitherDirection(ctx, viewerUserID, targetUserID)
		if err != nil {
			return false, err
		}
		if blocked {
			return false, nil
		}
	}
	return true, nil
}

func (s *GuardedStore) blockedEitherDirection(ctx context.Context, a, b string) (bool, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || a == b {
		return false, nil
	}
	var blocked int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM blocks WHERE (blocker_user_id=? AND blocked_user_id=?) OR (blocker_user_id=? AND blocked_user_id=?))`, a, b, b, a).Scan(&blocked); err != nil {
		return false, fmt.Errorf("check block relationship: %w", err)
	}
	return blocked != 0, nil
}

func (s *GuardedStore) userIDForHandle(ctx context.Context, handle string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE handle=?`, strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve guarded creator: %w", err)
	}
	return userID, nil
}
