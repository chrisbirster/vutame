package profile

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

var ErrSchemaNotReady = errors.New("database schema is not ready")

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Foreign-key enforcement is connection-local in SQLite. Keeping one
	// connection makes the setting deterministic for this first persistence
	// slice and is adequate for the single-process MVP.
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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

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
	handle = NormalizeHandle(handle)
	var item Profile
	var verified int
	var did sql.NullString
	err := s.db.QueryRow(`
		SELECT user_id, handle, display_name, bio, avatar_url, verified, atproto_did
		FROM profiles
		WHERE handle = ?
	`, handle).Scan(
		&item.ID,
		&item.Handle,
		&item.DisplayName,
		&item.Bio,
		&item.AvatarURL,
		&verified,
		&did,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	item.Verified = verified != 0
	if did.Valid {
		item.ATProtoDID = did.String
	}
	item.Links, err = s.linksForUser(item.ID)
	if err != nil {
		return Profile{}, err
	}
	return publicProfile(item), nil
}

func (s *SQLiteStore) Discover() ([]Profile, error) {
	rows, err := s.db.Query(`
		SELECT user_id, handle, display_name, bio, avatar_url, verified, atproto_did
		FROM profiles
		ORDER BY handle
	`)
	if err != nil {
		return nil, fmt.Errorf("discover profiles: %w", err)
	}
	defer rows.Close()

	items := make([]Profile, 0)
	for rows.Next() {
		var item Profile
		var verified int
		var did sql.NullString
		if err := rows.Scan(&item.ID, &item.Handle, &item.DisplayName, &item.Bio, &item.AvatarURL, &verified, &did); err != nil {
			return nil, fmt.Errorf("scan discovered profile: %w", err)
		}
		item.Verified = verified != 0
		if did.Valid {
			item.ATProtoDID = did.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovered profiles: %w", err)
	}
	// Close before loading links because this initial SQLite configuration uses
	// a single connection to make PRAGMA foreign_keys deterministic.
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close discovered profiles: %w", err)
	}
	for index := range items {
		items[index].Links, err = s.linksForUser(items[index].ID)
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
	handle = NormalizeHandle(handle)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE handle = ?`, handle).Scan(&count); err != nil {
		return false, fmt.Errorf("check handle availability: %w", err)
	}
	return count == 0, nil
}

func (s *SQLiteStore) linksForUser(userID string) ([]Link, error) {
	rows, err := s.db.Query(`
		SELECT id, label, url, kind, position, is_active
		FROM links
		WHERE user_id = ?
		ORDER BY position, id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list profile links: %w", err)
	}
	defer rows.Close()
	links := make([]Link, 0)
	for rows.Next() {
		var link Link
		var active int
		if err := rows.Scan(&link.ID, &link.Label, &link.URL, &link.Kind, &link.Position, &active); err != nil {
			return nil, fmt.Errorf("scan profile link: %w", err)
		}
		link.IsActive = active != 0
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile links: %w", err)
	}
	return links, nil
}
