package operations

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
)

func (s *Store) CreateToken(ctx context.Context, userID, name string, scopes []string, expiresInDays int) (CreatedToken, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return CreatedToken{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return CreatedToken{}, fmt.Errorf("token name must be 1-80 characters")
	}
	normalizedScopes, err := NormalizeScopes(scopes)
	if err != nil {
		return CreatedToken{}, err
	}
	if expiresInDays < 0 || expiresInDays > 365 {
		return CreatedToken{}, fmt.Errorf("token expiration must be 0-365 days")
	}
	id, err := randomID("tok_", 12)
	if err != nil {
		return CreatedToken{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return CreatedToken{}, err
	}
	raw := "vut_" + base64.RawURLEncoding.EncodeToString(secret)
	prefix := raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	now := s.now().UTC()
	var expires any
	expiresAt := ""
	if expiresInDays > 0 {
		expiresAt = now.AddDate(0, 0, expiresInDays).Format(time.RFC3339Nano)
		expires = expiresAt
	}
	createdAt := now.Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, user_id, name, token_prefix, token_hash, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, name, prefix, s.tokenHash(raw), strings.Join(normalizedScopes, ","), expires, createdAt)
	if err != nil {
		return CreatedToken{}, fmt.Errorf("create API token: %w", err)
	}
	return CreatedToken{APIToken: APIToken{ID: id, Name: name, Prefix: prefix, Scopes: normalizedScopes, ExpiresAt: expiresAt, CreatedAt: createdAt}, Token: raw}, nil
}

func (s *Store) Tokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, token_prefix, scopes, COALESCE(expires_at, ''), COALESCE(last_used_at, ''), created_at
		FROM api_tokens WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC, id DESC
	`, strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	defer rows.Close()
	items := make([]APIToken, 0)
	for rows.Next() {
		var item APIToken
		var scopes string
		if err := rows.Scan(&item.ID, &item.Name, &item.Prefix, &scopes, &item.ExpiresAt, &item.LastUsedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Scopes = splitCSV(scopes)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RevokeToken(ctx context.Context, userID, tokenID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, s.now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(tokenID), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("revoke API token: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrTokenUnauthorized
	}
	return nil
}

func (s *Store) AuthenticateToken(ctx context.Context, raw, requiredScope string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "vut_") || len(raw) < 20 {
		return "", ErrTokenUnauthorized
	}
	var userID, scopes, expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT user_id, scopes, COALESCE(expires_at, '') FROM api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL
	`, s.tokenHash(raw)).Scan(&userID, &scopes, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrTokenUnauthorized
		}
		return "", err
	}
	if expiresAt != "" {
		expires, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil || !s.now().UTC().Before(expires) {
			return "", ErrTokenUnauthorized
		}
	}
	if requiredScope != "" && !containsScope(splitCSV(scopes), requiredScope) {
		return "", ErrTokenUnauthorized
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?`, s.now().UTC().Format(time.RFC3339Nano), s.tokenHash(raw))
	return userID, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	sort.Strings(items)
	return items
}

func containsScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}
