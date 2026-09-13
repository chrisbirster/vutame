package social

import (
	"context"
	"fmt"
	"strings"
)

func (s *SQLiteStore) SearchPage(ctx context.Context, input SearchInput, viewerUserID string) (CreatorPage, error) {
	input, err := NormalizeSearch(input)
	if err != nil {
		return CreatorPage{}, err
	}
	offset, err := DecodeSearchCursor(input.Cursor)
	if err != nil {
		return CreatorPage{}, err
	}
	like := "%" + strings.ToLower(input.Query) + "%"
	rows, err := s.db.QueryContext(ctx, creatorSelect+`
		WHERE (
			? = '' OR lower(p.handle) LIKE ? OR lower(p.display_name) LIKE ? OR lower(p.bio) LIKE ? OR
			EXISTS (
				SELECT 1 FROM creator_interests search_interest
				WHERE search_interest.user_id = p.user_id AND lower(search_interest.interest) LIKE ?
			)
		)
		AND (? = '' OR lower(COALESCE(m.category, '')) = lower(?))
		AND (? = '' OR EXISTS (
			SELECT 1 FROM creator_interests filter_interest
			WHERE filter_interest.user_id = p.user_id AND lower(filter_interest.interest) = lower(?)
		))
		ORDER BY follower_count DESC, p.handle
		LIMIT ? OFFSET ?
	`, input.Query, like, like, like, like, input.Category, input.Category, input.Interest, input.Interest, input.Limit+1, offset)
	if err != nil {
		return CreatorPage{}, fmt.Errorf("search creator page: %w", err)
	}
	items, err := s.scanCreators(ctx, rows, viewerUserID)
	if err != nil {
		return CreatorPage{}, err
	}
	page := CreatorPage{Creators: items}
	if len(items) > input.Limit {
		page.Creators = items[:input.Limit]
		page.NextCursor = EncodeSearchCursor(offset + input.Limit)
	}
	return page, nil
}
