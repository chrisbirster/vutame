package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/profile"
)

type SQLiteStore struct {
	db *sql.DB
}

type creatorRow struct {
	userID  string
	creator Creator
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("social: database is required")
	}
	store := &SQLiteStore{db: db}
	for _, table := range []string{"profiles", "follows", "creator_metadata", "creator_interests"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify social table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("social schema missing table %s; run Atlas schema apply", table)
		}
	}
	return store, nil
}

func (s *SQLiteStore) Follow(ctx context.Context, followerUserID, handle string) error {
	followerUserID = strings.TrimSpace(followerUserID)
	targetUserID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return err
	}
	if targetUserID == followerUserID {
		return ErrSelfFollow
	}
	var hasProfile int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id = ?`, followerUserID).Scan(&hasProfile); err != nil {
		return fmt.Errorf("check follower profile: %w", err)
	}
	if hasProfile != 1 {
		return ErrProfileRequired
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO follows (follower_user_id, following_user_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(follower_user_id, following_user_id) DO NOTHING
	`, followerUserID, targetUserID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("follow creator: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Unfollow(ctx context.Context, followerUserID, handle string) error {
	targetUserID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM follows WHERE follower_user_id = ? AND following_user_id = ?`, strings.TrimSpace(followerUserID), targetUserID)
	if err != nil {
		return fmt.Errorf("unfollow creator: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Creator(ctx context.Context, handle, viewerUserID string) (Creator, error) {
	row, err := s.creatorByHandle(ctx, handle)
	if err != nil {
		return Creator{}, err
	}
	if err := s.decorateCreator(ctx, &row, viewerUserID); err != nil {
		return Creator{}, err
	}
	return row.creator, nil
}

func (s *SQLiteStore) Followers(ctx context.Context, handle, viewerUserID string) ([]Creator, error) {
	targetUserID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, creatorSelect+`
		JOIN follows f ON f.follower_user_id = p.user_id
		WHERE f.following_user_id = ?
		ORDER BY f.created_at DESC, p.handle
		LIMIT 100
	`, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("list followers: %w", err)
	}
	return s.scanCreators(ctx, rows, viewerUserID)
}

func (s *SQLiteStore) Following(ctx context.Context, handle, viewerUserID string) ([]Creator, error) {
	sourceUserID, err := s.userIDForHandle(ctx, handle)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, creatorSelect+`
		JOIN follows f ON f.following_user_id = p.user_id
		WHERE f.follower_user_id = ?
		ORDER BY f.created_at DESC, p.handle
		LIMIT 100
	`, sourceUserID)
	if err != nil {
		return nil, fmt.Errorf("list following: %w", err)
	}
	return s.scanCreators(ctx, rows, viewerUserID)
}

func (s *SQLiteStore) Search(ctx context.Context, input SearchInput, viewerUserID string) ([]Creator, error) {
	input, err := NormalizeSearch(input)
	if err != nil {
		return nil, err
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
		LIMIT ?
	`, input.Query, like, like, like, like, input.Category, input.Category, input.Interest, input.Interest, input.Limit)
	if err != nil {
		return nil, fmt.Errorf("search creators: %w", err)
	}
	return s.scanCreators(ctx, rows, viewerUserID)
}

func (s *SQLiteStore) UpdateMetadata(ctx context.Context, userID string, input MetadataInput) (Creator, error) {
	input, err := ValidateMetadata(input)
	if err != nil {
		return Creator{}, err
	}
	userID = strings.TrimSpace(userID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Creator{}, fmt.Errorf("begin discovery metadata update: %w", err)
	}
	defer tx.Rollback()
	var handle string
	if err := tx.QueryRowContext(ctx, `SELECT handle FROM profiles WHERE user_id = ?`, userID).Scan(&handle); errors.Is(err, sql.ErrNoRows) {
		return Creator{}, ErrProfileRequired
	} else if err != nil {
		return Creator{}, fmt.Errorf("load discovery profile: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO creator_metadata (user_id, category, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET category = excluded.category, updated_at = excluded.updated_at
	`, userID, input.Category, now); err != nil {
		return Creator{}, fmt.Errorf("update creator category: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM creator_interests WHERE user_id = ?`, userID); err != nil {
		return Creator{}, fmt.Errorf("replace creator interests: %w", err)
	}
	for _, interest := range input.Interests {
		if _, err := tx.ExecContext(ctx, `INSERT INTO creator_interests (user_id, interest) VALUES (?, ?)`, userID, interest); err != nil {
			return Creator{}, fmt.Errorf("add creator interest: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Creator{}, fmt.Errorf("commit discovery metadata update: %w", err)
	}
	return s.Creator(ctx, handle, userID)
}

const creatorSelect = `
	SELECT
		p.user_id,
		p.handle,
		p.display_name,
		p.bio,
		p.avatar_url,
		p.theme,
		p.verified,
		COALESCE(m.category, ''),
		(SELECT COUNT(*) FROM follows followers WHERE followers.following_user_id = p.user_id) AS follower_count,
		(SELECT COUNT(*) FROM follows following WHERE following.follower_user_id = p.user_id) AS following_count
	FROM profiles p
	LEFT JOIN creator_metadata m ON m.user_id = p.user_id
`

func (s *SQLiteStore) creatorByHandle(ctx context.Context, handle string) (creatorRow, error) {
	row := s.db.QueryRowContext(ctx, creatorSelect+` WHERE p.handle = ?`, profile.NormalizeHandle(handle))
	item, err := scanCreator(row)
	if errors.Is(err, sql.ErrNoRows) {
		return creatorRow{}, ErrNotFound
	}
	if err != nil {
		return creatorRow{}, fmt.Errorf("get creator: %w", err)
	}
	return item, nil
}

func (s *SQLiteStore) userIDForHandle(ctx context.Context, handle string) (string, error) {
	var userID string
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE handle = ?`, profile.NormalizeHandle(handle)).Scan(&userID); errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("resolve creator handle: %w", err)
	}
	return userID, nil
}

func (s *SQLiteStore) scanCreators(ctx context.Context, rows *sql.Rows, viewerUserID string) ([]Creator, error) {
	materialized := make([]creatorRow, 0)
	for rows.Next() {
		item, err := scanCreator(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan creator: %w", err)
		}
		materialized = append(materialized, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate creators: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close creator rows: %w", err)
	}

	items := make([]Creator, 0, len(materialized))
	for index := range materialized {
		if err := s.decorateCreator(ctx, &materialized[index], viewerUserID); err != nil {
			return nil, err
		}
		items = append(items, materialized[index].creator)
	}
	return items, nil
}

func scanCreator(scanner interface{ Scan(...any) error }) (creatorRow, error) {
	var item creatorRow
	var verified int
	err := scanner.Scan(
		&item.userID,
		&item.creator.Handle,
		&item.creator.DisplayName,
		&item.creator.Bio,
		&item.creator.AvatarURL,
		&item.creator.Theme,
		&verified,
		&item.creator.Category,
		&item.creator.FollowerCount,
		&item.creator.FollowingCount,
	)
	item.creator.Verified = verified != 0
	return item, err
}

func (s *SQLiteStore) decorateCreator(ctx context.Context, item *creatorRow, viewerUserID string) error {
	interests, err := s.interestsForUser(ctx, item.userID)
	if err != nil {
		return err
	}
	item.creator.Interests = interests
	viewerUserID = strings.TrimSpace(viewerUserID)
	if viewerUserID == "" || viewerUserID == item.userID {
		item.creator.ViewerFollows = false
		return nil
	}
	var follows int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM follows WHERE follower_user_id = ? AND following_user_id = ?)
	`, viewerUserID, item.userID).Scan(&follows); err != nil {
		return fmt.Errorf("check viewer follow state: %w", err)
	}
	item.creator.ViewerFollows = follows != 0
	return nil
}

func (s *SQLiteStore) interestsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT interest FROM creator_interests WHERE user_id = ? ORDER BY interest`, userID)
	if err != nil {
		return nil, fmt.Errorf("list creator interests: %w", err)
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var interest string
		if err := rows.Scan(&interest); err != nil {
			return nil, fmt.Errorf("scan creator interest: %w", err)
		}
		items = append(items, interest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate creator interests: %w", err)
	}
	return items, nil
}
