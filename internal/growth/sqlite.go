package growth

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

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("growth: database is required")
	}
	for _, table := range []string{"profiles", "creator_data_settings", "contact_blocks", "contact_submissions", "analytics_events"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify growth table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("growth schema missing table %s; run Atlas schema apply", table)
		}
	}
	return &Service{db: db}, nil
}

func (s *Service) OwnedBlock(ctx context.Context, userID string) (ContactBlock, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return ContactBlock{}, err
	}
	return s.blockForUser(ctx, userID)
}

func (s *Service) PublicBlock(ctx context.Context, handle string) (ContactBlock, error) {
	var userID string
	var moderationState string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.user_id, COALESCE(m.state, 'active')
		FROM profiles p
		LEFT JOIN moderation_profiles m ON m.user_id = p.user_id
		WHERE p.handle = ?
	`, strings.TrimPrefix(strings.TrimSpace(handle), "@")).Scan(&userID, &moderationState)
	if errors.Is(err, sql.ErrNoRows) {
		return ContactBlock{}, ErrProfileRequired
	}
	if err != nil {
		return ContactBlock{}, fmt.Errorf("resolve contact profile: %w", err)
	}
	if moderationState == "suspended" {
		return ContactBlock{}, ErrContactDisabled
	}
	block, err := s.blockForUser(ctx, userID)
	if err != nil {
		return ContactBlock{}, err
	}
	if !block.Enabled {
		return ContactBlock{}, ErrContactDisabled
	}
	return block, nil
}

func (s *Service) UpdateBlock(ctx context.Context, userID string, input ContactBlock) (ContactBlock, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return ContactBlock{}, err
	}
	input, err := ValidateBlock(input)
	if err != nil {
		return ContactBlock{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO contact_blocks (user_id, enabled, heading, description, consent_text, button_label, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			enabled = excluded.enabled,
			heading = excluded.heading,
			description = excluded.description,
			consent_text = excluded.consent_text,
			button_label = excluded.button_label,
			updated_at = excluded.updated_at
	`, userID, boolInt(input.Enabled), input.Heading, input.Description, input.ConsentText, input.ButtonLabel, now)
	if err != nil {
		return ContactBlock{}, fmt.Errorf("update contact block: %w", err)
	}
	return input, nil
}

func (s *Service) SubmitContact(ctx context.Context, handle, email string, consent bool, campaign string, now time.Time) (ContactSubmission, error) {
	if !consent {
		return ContactSubmission{}, ErrConsentRequired
	}
	cleanEmail, err := NormalizeEmail(email)
	if err != nil {
		return ContactSubmission{}, err
	}
	var userID string
	var enabled int
	var consentText string
	var moderationState string
	err = s.db.QueryRowContext(ctx, `
		SELECT p.user_id, COALESCE(b.enabled, 0), COALESCE(b.consent_text, ?), COALESCE(m.state, 'active')
		FROM profiles p
		LEFT JOIN contact_blocks b ON b.user_id = p.user_id
		LEFT JOIN moderation_profiles m ON m.user_id = p.user_id
		WHERE p.handle = ?
	`, DefaultContactConsent, strings.TrimPrefix(strings.TrimSpace(handle), "@")).Scan(&userID, &enabled, &consentText, &moderationState)
	if errors.Is(err, sql.ErrNoRows) {
		return ContactSubmission{}, ErrProfileRequired
	}
	if err != nil {
		return ContactSubmission{}, fmt.Errorf("resolve contact submission profile: %w", err)
	}
	if enabled != 1 || moderationState == "suspended" {
		return ContactSubmission{}, ErrContactDisabled
	}
	settings, err := s.settingsForUser(ctx, userID)
	if err != nil {
		return ContactSubmission{}, err
	}
	if err := s.purgeContacts(ctx, userID, settings.ContactRetentionDays, now); err != nil {
		return ContactSubmission{}, err
	}
	id, err := growthID("cnt_")
	if err != nil {
		return ContactSubmission{}, err
	}
	campaign = NormalizeCampaign(campaign)
	createdAt := now.UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO contact_submissions (id, user_id, email, consent_text, campaign, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, userID, cleanEmail, consentText, campaign, createdAt)
	if err != nil {
		return ContactSubmission{}, fmt.Errorf("create contact submission: %w", err)
	}
	return ContactSubmission{ID: id, Email: cleanEmail, Campaign: campaign, CreatedAt: createdAt}, nil
}

func (s *Service) Contacts(ctx context.Context, userID string, limit int) ([]ContactSubmission, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	settings, err := s.settingsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.purgeContacts(ctx, userID, settings.ContactRetentionDays, time.Now().UTC()); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email, campaign, created_at
		FROM contact_submissions
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()
	items := make([]ContactSubmission, 0)
	for rows.Next() {
		var item ContactSubmission
		if err := rows.Scan(&item.ID, &item.Email, &item.Campaign, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}
	return items, nil
}

func (s *Service) Settings(ctx context.Context, userID string) (DataSettings, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return DataSettings{}, err
	}
	return s.settingsForUser(ctx, userID)
}

func (s *Service) UpdateSettings(ctx context.Context, userID string, input DataSettings, now time.Time) (DataSettings, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return DataSettings{}, err
	}
	input, err := ValidateSettings(input)
	if err != nil {
		return DataSettings{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DataSettings{}, fmt.Errorf("begin retention update: %w", err)
	}
	defer tx.Rollback()
	stamp := now.UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO creator_data_settings (user_id, analytics_retention_days, contact_retention_days, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			analytics_retention_days = excluded.analytics_retention_days,
			contact_retention_days = excluded.contact_retention_days,
			updated_at = excluded.updated_at
	`, userID, input.AnalyticsRetentionDays, input.ContactRetentionDays, stamp)
	if err != nil {
		return DataSettings{}, fmt.Errorf("update retention settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_events WHERE user_id = ? AND created_at < ?`, userID, RetentionCutoff(input.AnalyticsRetentionDays, now)); err != nil {
		return DataSettings{}, fmt.Errorf("purge analytics retention: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_submissions WHERE user_id = ? AND created_at < ?`, userID, RetentionCutoff(input.ContactRetentionDays, now)); err != nil {
		return DataSettings{}, fmt.Errorf("purge contact retention: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DataSettings{}, fmt.Errorf("commit retention update: %w", err)
	}
	return input, nil
}

func (s *Service) blockForUser(ctx context.Context, userID string) (ContactBlock, error) {
	block := DefaultBlock()
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled, heading, description, consent_text, button_label
		FROM contact_blocks WHERE user_id = ?
	`, userID).Scan(&enabled, &block.Heading, &block.Description, &block.ConsentText, &block.ButtonLabel)
	if errors.Is(err, sql.ErrNoRows) {
		return block, nil
	}
	if err != nil {
		return ContactBlock{}, fmt.Errorf("get contact block: %w", err)
	}
	block.Enabled = enabled == 1
	return block, nil
}

func (s *Service) settingsForUser(ctx context.Context, userID string) (DataSettings, error) {
	settings := DefaultSettings()
	err := s.db.QueryRowContext(ctx, `
		SELECT analytics_retention_days, contact_retention_days
		FROM creator_data_settings WHERE user_id = ?
	`, userID).Scan(&settings.AnalyticsRetentionDays, &settings.ContactRetentionDays)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return DataSettings{}, fmt.Errorf("get retention settings: %w", err)
	}
	return settings, nil
}

func (s *Service) purgeContacts(ctx context.Context, userID string, retentionDays int, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM contact_submissions WHERE user_id = ? AND created_at < ?`, userID, RetentionCutoff(retentionDays, now))
	if err != nil {
		return fmt.Errorf("purge expired contacts: %w", err)
	}
	return nil
}

func (s *Service) requireProfile(ctx context.Context, userID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return fmt.Errorf("check growth profile: %w", err)
	}
	if count != 1 {
		return ErrProfileRequired
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func growthID(prefix string) (string, error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate growth id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
