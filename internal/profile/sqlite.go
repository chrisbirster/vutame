package profile

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrSchemaNotReady = errors.New("database schema is not ready")

type SQLiteStore struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

func OpenSQLite(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("profile: sqlite db is required")
	}
	store := &SQLiteStore{db: db}
	if err := store.verifySchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) verifySchema() error {
	for _, table := range []string{"users", "profiles", "links", "sessions"} {
		var count int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count)
		if err != nil {
			return fmt.Errorf("verify schema table %s: %w", table, err)
		}
		if count != 1 {
			return fmt.Errorf("%w: missing table %s; run Atlas schema apply", ErrSchemaNotReady, table)
		}
	}
	return nil
}

func (s *SQLiteStore) Get(handle string) (Profile, error) {
	item, err := scanProfile(s.db.QueryRow(`
		SELECT user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did
		FROM profiles WHERE handle = ?
	`, NormalizeHandle(handle)))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	item.Links, err = s.linksForUserContext(context.Background(), item.ID)
	if err != nil {
		return Profile{}, err
	}
	return publicProfile(item), nil
}

func (s *SQLiteStore) Discover() ([]Profile, error) {
	rows, err := s.db.Query(`
		SELECT user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did
		FROM profiles ORDER BY handle
	`)
	if err != nil {
		return nil, fmt.Errorf("discover profiles: %w", err)
	}
	items := make([]Profile, 0)
	for rows.Next() {
		item, err := scanProfile(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan discovered profile: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate discovered profiles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close discovered profiles: %w", err)
	}
	for index := range items {
		items[index].Links, err = s.linksForUserContext(context.Background(), items[index].ID)
		if err != nil {
			return nil, err
		}
		items[index] = publicProfile(items[index])
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Handle < items[j].Handle })
	return items, nil
}

func (s *SQLiteStore) HandleAvailable(handle string) (bool, error) {
	if err := ValidateHandle(handle); err != nil {
		return false, err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE handle = ?`, NormalizeHandle(handle)).Scan(&count); err != nil {
		return false, fmt.Errorf("check handle availability: %w", err)
	}
	return count == 0, nil
}

func (s *SQLiteStore) GetOwned(ctx context.Context, userID string) (Profile, error) {
	return s.profileForUser(ctx, strings.TrimSpace(userID))
}

func (s *SQLiteStore) Claim(ctx context.Context, userID, handle string) (Profile, error) {
	userID = strings.TrimSpace(userID)
	handle = NormalizeHandle(handle)
	if err := ValidateHandle(handle); err != nil {
		return Profile{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("begin profile claim: %w", err)
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRowContext(ctx, `SELECT handle FROM profiles WHERE user_id = ?`, userID).Scan(&existing)
	if err == nil {
		return Profile{}, fmt.Errorf("%w: @%s", ErrProfileExists, existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Profile{}, fmt.Errorf("check existing profile: %w", err)
	}
	var taken int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE handle = ?`, handle).Scan(&taken); err != nil {
		return Profile{}, fmt.Errorf("check claimed handle: %w", err)
	}
	if taken != 0 {
		return Profile{}, ErrHandleTaken
	}
	var userExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&userExists); err != nil {
		return Profile{}, fmt.Errorf("check profile owner: %w", err)
	}
	if userExists != 1 {
		return Profile{}, ErrNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at)
		VALUES (?, ?, ?, '', '', ?, 0, NULL, ?, ?)
	`, userID, handle, "@"+handle, DefaultTheme, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Profile{}, ErrHandleTaken
		}
		return Profile{}, fmt.Errorf("claim profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, fmt.Errorf("commit profile claim: %w", err)
	}
	return s.profileForUser(ctx, userID)
}

func (s *SQLiteStore) Update(ctx context.Context, userID string, input UpdateInput) (Profile, error) {
	input, err := ValidateUpdateInput(input)
	if err != nil {
		return Profile{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE profiles
		SET display_name = ?, bio = ?, avatar_url = ?, theme = ?, updated_at = ?
		WHERE user_id = ?
	`, input.DisplayName, input.Bio, input.AvatarURL, input.Theme, time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(userID))
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Profile{}, fmt.Errorf("read updated profile count: %w", err)
	}
	if rows != 1 {
		return Profile{}, ErrNotFound
	}
	return s.profileForUser(ctx, userID)
}

func (s *SQLiteStore) CreateLink(ctx context.Context, userID string, input LinkInput) (Link, error) {
	input, err := ValidateLinkInput(input)
	if err != nil {
		return Link{}, err
	}
	userID = strings.TrimSpace(userID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Link{}, fmt.Errorf("begin create link: %w", err)
	}
	defer tx.Rollback()
	var profileExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id = ?`, userID).Scan(&profileExists); err != nil {
		return Link{}, fmt.Errorf("check link owner: %w", err)
	}
	if profileExists != 1 {
		return Link{}, ErrNotFound
	}
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position) + 1, 0) FROM links WHERE user_id = ?`, userID).Scan(&position); err != nil {
		return Link{}, fmt.Errorf("next link position: %w", err)
	}
	id, err := randomOpaqueID("lnk_", 18)
	if err != nil {
		return Link{}, fmt.Errorf("generate link id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, input.Label, input.URL, input.Kind, input.ThumbnailURL, boolInt(input.Featured), nullString(input.VisibleFrom), nullString(input.VisibleUntil), position, boolInt(input.IsActive), now, now)
	if err != nil {
		return Link{}, fmt.Errorf("create link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Link{}, fmt.Errorf("commit create link: %w", err)
	}
	return Link{
		ID: id, Label: input.Label, URL: input.URL, Kind: input.Kind, ThumbnailURL: input.ThumbnailURL,
		Featured: input.Featured, VisibleFrom: input.VisibleFrom, VisibleUntil: input.VisibleUntil,
		Position: position, IsActive: input.IsActive,
	}, nil
}

func (s *SQLiteStore) UpdateLink(ctx context.Context, userID, linkID string, input LinkInput) (Link, error) {
	input, err := ValidateLinkInput(input)
	if err != nil {
		return Link{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE links
		SET label = ?, url = ?, kind = ?, thumbnail_url = ?, featured = ?, visible_from = ?, visible_until = ?, is_active = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, input.Label, input.URL, input.Kind, input.ThumbnailURL, boolInt(input.Featured), nullString(input.VisibleFrom), nullString(input.VisibleUntil), boolInt(input.IsActive), time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(linkID), strings.TrimSpace(userID))
	if err != nil {
		return Link{}, fmt.Errorf("update link: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Link{}, fmt.Errorf("read updated link count: %w", err)
	}
	if rows != 1 {
		return Link{}, ErrNotFound
	}
	item, err := scanLink(s.db.QueryRowContext(ctx, `
		SELECT id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active
		FROM links WHERE id = ? AND user_id = ?
	`, strings.TrimSpace(linkID), strings.TrimSpace(userID)))
	if err != nil {
		return Link{}, fmt.Errorf("load updated link: %w", err)
	}
	return item, nil
}

func (s *SQLiteStore) DeleteLink(ctx context.Context, userID, linkID string) error {
	userID = strings.TrimSpace(userID)
	linkID = strings.TrimSpace(linkID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete link: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM links WHERE id = ? AND user_id = ?`, linkID, userID)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted link count: %w", err)
	}
	if rows != 1 {
		return ErrNotFound
	}
	ids, err := orderedLinkIDs(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := applyLinkOrder(ctx, tx, userID, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete link: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ReorderLinks(ctx context.Context, userID string, ids []string) error {
	userID = strings.TrimSpace(userID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reorder links: %w", err)
	}
	defer tx.Rollback()
	existing, err := orderedLinkIDs(ctx, tx, userID)
	if err != nil {
		return err
	}
	if len(existing) != len(ids) {
		return fmt.Errorf("%w: order must contain every link exactly once", ErrInvalidLink)
	}
	allowed := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		allowed[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ids))
	for index := range ids {
		ids[index] = strings.TrimSpace(ids[index])
		if _, ok := allowed[ids[index]]; !ok {
			return ErrNotFound
		}
		if _, duplicate := seen[ids[index]]; duplicate {
			return fmt.Errorf("%w: duplicate link in order", ErrInvalidLink)
		}
		seen[ids[index]] = struct{}{}
	}
	if err := applyLinkOrder(ctx, tx, userID, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder links: %w", err)
	}
	return nil
}

func (s *SQLiteStore) profileForUser(ctx context.Context, userID string) (Profile, error) {
	item, err := scanProfile(s.db.QueryRowContext(ctx, `
		SELECT user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did
		FROM profiles WHERE user_id = ?
	`, strings.TrimSpace(userID)))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get owned profile: %w", err)
	}
	item.Links, err = s.linksForUserContext(ctx, item.ID)
	if err != nil {
		return Profile{}, err
	}
	return item, nil
}

func (s *SQLiteStore) linksForUserContext(ctx context.Context, userID string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active
		FROM links WHERE user_id = ? ORDER BY position, id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list profile links: %w", err)
	}
	defer rows.Close()
	links := make([]Link, 0)
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan profile link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile links: %w", err)
	}
	return links, nil
}

func scanProfile(row rowScanner) (Profile, error) {
	var item Profile
	var verified int
	var did sql.NullString
	if err := row.Scan(&item.ID, &item.Handle, &item.DisplayName, &item.Bio, &item.AvatarURL, &item.Theme, &verified, &did); err != nil {
		return Profile{}, err
	}
	item.Theme = NormalizeTheme(item.Theme)
	item.Verified = verified != 0
	if did.Valid {
		item.ATProtoDID = did.String
	}
	return item, nil
}

func scanLink(row rowScanner) (Link, error) {
	var item Link
	var featured, active int
	var visibleFrom, visibleUntil sql.NullString
	if err := row.Scan(&item.ID, &item.Label, &item.URL, &item.Kind, &item.ThumbnailURL, &featured, &visibleFrom, &visibleUntil, &item.Position, &active); err != nil {
		return Link{}, err
	}
	item.Kind = NormalizeLinkKind(item.Kind)
	item.Featured = featured != 0
	item.IsActive = active != 0
	if visibleFrom.Valid {
		item.VisibleFrom = visibleFrom.String
	}
	if visibleUntil.Valid {
		item.VisibleUntil = visibleUntil.String
	}
	return item, nil
}

func orderedLinkIDs(ctx context.Context, tx *sql.Tx, userID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM links WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list link order: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan link order: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate link order: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close link order: %w", err)
	}
	return ids, nil
}

func applyLinkOrder(ctx context.Context, tx *sql.Tx, userID string, ids []string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for position, id := range ids {
		result, err := tx.ExecContext(ctx, `UPDATE links SET position = ?, updated_at = ? WHERE id = ? AND user_id = ?`, position, now, id, userID)
		if err != nil {
			return fmt.Errorf("apply link order: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read reordered link count: %w", err)
		}
		if rows != 1 {
			return ErrNotFound
		}
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func randomOpaqueID(prefix string, size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}
