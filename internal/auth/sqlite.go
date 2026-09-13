package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open auth sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable auth sqlite foreign keys: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping auth sqlite: %w", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// NewSQLiteStore binds the auth store to an already-open SQLite-compatible
// database/sql pool. The caller owns the database lifetime.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("auth: sqlite-compatible db is required")
	}
	store := &SQLiteStore{db: db}
	if err := store.verifySchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) verifySchema() error {
	for _, table := range []string{"users", "auth_challenges", "sessions"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return fmt.Errorf("verify auth schema table %s: %w", table, err)
		}
		if count != 1 {
			return fmt.Errorf("auth database schema is not ready: missing table %s; run Atlas schema apply", table)
		}
	}
	return nil
}

func (s *SQLiteStore) CreateChallenge(ctx context.Context, challenge Challenge) error {
	now := challenge.CreatedAt.UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM auth_challenges WHERE email = ? OR expires_at <= ?`, challenge.Email, now); err != nil {
		return fmt.Errorf("prune auth challenges: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_challenges (id, email, code_hash, attempts, expires_at, consumed_at, created_at)
		VALUES (?, ?, ?, 0, ?, NULL, ?)
	`, challenge.ID, challenge.Email, challenge.CodeHash, challenge.ExpiresAt.UTC().Format(time.RFC3339Nano), now)
	if err != nil {
		return fmt.Errorf("create auth challenge: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteChallenge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_challenges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete auth challenge: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetChallenge(ctx context.Context, id string) (Challenge, error) {
	var challenge Challenge
	var expiresAt, createdAt string
	var consumedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, code_hash, attempts, expires_at, consumed_at, created_at
		FROM auth_challenges WHERE id = ?
	`, id).Scan(&challenge.ID, &challenge.Email, &challenge.CodeHash, &challenge.Attempts, &expiresAt, &consumedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Challenge{}, ErrInvalidCode
	}
	if err != nil {
		return Challenge{}, fmt.Errorf("get auth challenge: %w", err)
	}
	challenge.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return Challenge{}, fmt.Errorf("parse challenge expiry: %w", err)
	}
	challenge.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Challenge{}, fmt.Errorf("parse challenge created time: %w", err)
	}
	if consumedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, consumedAt.String)
		if parseErr != nil {
			return Challenge{}, fmt.Errorf("parse challenge consumed time: %w", parseErr)
		}
		challenge.ConsumedAt = &value
	}
	return challenge, nil
}

func (s *SQLiteStore) RecordFailedAttempt(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE auth_challenges SET attempts = attempts + 1 WHERE id = ? AND consumed_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("record auth attempt: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ConsumeChallenge(ctx context.Context, challengeID, email, newUserID string, session SessionRecord, now time.Time) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin auth transaction: %w", err)
	}
	defer tx.Rollback()
	nowText := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE auth_challenges
		SET consumed_at = ?
		WHERE id = ? AND email = ? AND consumed_at IS NULL AND expires_at > ?
	`, nowText, challengeID, email, nowText)
	if err != nil {
		return User{}, fmt.Errorf("consume auth challenge: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return User{}, fmt.Errorf("read consumed challenge count: %w", err)
	}
	if rows != 1 {
		return User{}, ErrInvalidCode
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, email, email_verified, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(email) DO UPDATE SET email_verified = 1, updated_at = excluded.updated_at
	`, newUserID, email, nowText, nowText)
	if err != nil {
		return User{}, fmt.Errorf("upsert auth user: %w", err)
	}
	var user User
	var verified int
	if err := tx.QueryRowContext(ctx, `SELECT id, email, email_verified FROM users WHERE email = ?`, email).Scan(&user.ID, &user.Email, &verified); err != nil {
		return User{}, fmt.Errorf("load authenticated user: %w", err)
	}
	user.EmailVerified = verified != 0
	session.UserID = user.ID
	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, session.ID, session.UserID, session.TokenHash, session.ExpiresAt.UTC().Format(time.RFC3339Nano), session.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return User{}, fmt.Errorf("create session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit auth transaction: %w", err)
	}
	return user, nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, tokenHash string, now time.Time) (User, error) {
	var user User
	var verified int
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.email_verified
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?
	`, tokenHash, now.UTC().Format(time.RFC3339Nano)).Scan(&user.ID, &user.Email, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrSessionNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get session: %w", err)
	}
	user.EmailVerified = verified != 0
	return user, nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
