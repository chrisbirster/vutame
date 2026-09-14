package operations

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ExportData struct {
	GeneratedAt     string           `json:"generated_at"`
	Account         map[string]any   `json:"account"`
	Profile         map[string]any   `json:"profile"`
	Links           []map[string]any `json:"links"`
	Interests       []map[string]any `json:"interests"`
	AnalyticsEvents []map[string]any `json:"analytics_events"`
	Contacts        []map[string]any `json:"contacts"`
	Domains         []map[string]any `json:"domains"`
	Verifications   []map[string]any `json:"verifications"`
	APITokens       []map[string]any `json:"api_tokens"`
	Webhooks        []map[string]any `json:"webhooks"`
}

func (s *Store) Export(ctx context.Context, userID string) (ExportData, error) {
	if err := s.requireProfile(ctx, userID); err != nil {
		return ExportData{}, err
	}
	accountRows, err := queryRows(ctx, s.db, `SELECT id, email, email_verified, created_at, updated_at FROM users WHERE id = ?`, userID)
	if err != nil || len(accountRows) != 1 {
		return ExportData{}, fmt.Errorf("export account: %w", err)
	}
	profileRows, err := queryRows(ctx, s.db, `SELECT user_id, handle, display_name, bio, avatar_url, theme, verified, COALESCE(atproto_did, ''), created_at, updated_at FROM profiles WHERE user_id = ?`, userID)
	if err != nil || len(profileRows) != 1 {
		return ExportData{}, fmt.Errorf("export profile: %w", err)
	}
	queries := []struct {
		name string
		sql  string
	}{
		{"links", `SELECT id, label, url, kind, thumbnail_url, featured, COALESCE(visible_from, ''), COALESCE(visible_until, ''), position, is_active, created_at, updated_at FROM links WHERE user_id = ? ORDER BY position, id`},
		{"interests", `SELECT interest FROM creator_interests WHERE user_id = ? ORDER BY interest`},
		{"analytics", `SELECT id, COALESCE(link_id, ''), kind, campaign, referrer_host, device_class, created_at FROM analytics_events WHERE user_id = ? ORDER BY created_at, id`},
		{"contacts", `SELECT id, email, consent_text, campaign, created_at FROM contact_submissions WHERE user_id = ? ORDER BY created_at, id`},
		{"domains", `SELECT id, hostname, COALESCE(verified_at, ''), created_at, updated_at FROM custom_domains WHERE user_id = ? ORDER BY created_at, id`},
		{"verifications", `SELECT id, method, evidence, status, note, created_at, updated_at FROM verification_requests WHERE user_id = ? ORDER BY created_at, id`},
		{"tokens", `SELECT id, name, token_prefix, scopes, COALESCE(expires_at, ''), COALESCE(last_used_at, ''), COALESCE(revoked_at, ''), created_at FROM api_tokens WHERE user_id = ? ORDER BY created_at, id`},
		{"webhooks", `SELECT id, url, events, active, created_at, updated_at FROM webhooks WHERE user_id = ? ORDER BY created_at, id`},
	}
	sets := make(map[string][]map[string]any, len(queries))
	for _, query := range queries {
		rows, err := queryRows(ctx, s.db, query.sql, userID)
		if err != nil {
			return ExportData{}, fmt.Errorf("export %s: %w", query.name, err)
		}
		sets[query.name] = rows
	}
	return ExportData{
		GeneratedAt: s.now().UTC().Format(time.RFC3339Nano),
		Account: accountRows[0], Profile: profileRows[0],
		Links: sets["links"], Interests: sets["interests"], AnalyticsEvents: sets["analytics"],
		Contacts: sets["contacts"], Domains: sets["domains"], Verifications: sets["verifications"],
		APITokens: sets["tokens"], Webhooks: sets["webhooks"],
	}, nil
}

func queryRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for index, column := range columns {
			value := values[index]
			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}
			item[column] = value
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
