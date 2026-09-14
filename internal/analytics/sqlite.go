package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrProfileRequired = errors.New("analytics profile required")

type Service struct {
	db     *sql.DB
	secret []byte
}

func NewService(db *sql.DB, secrets ...[]byte) (*Service, error) {
	if db == nil {
		return nil, errors.New("analytics: database is required")
	}
	for _, table := range []string{"profiles", "links", "analytics_events", "creator_data_settings"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify analytics table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("analytics schema missing table %s; run Atlas schema apply", table)
		}
	}
	service := &Service{db: db}
	if len(secrets) > 0 {
		service.secret = append([]byte(nil), secrets[0]...)
	}
	return service, nil
}

func (s *Service) VisitorToken(userID, clientIP string, at time.Time) string {
	userID = strings.TrimSpace(userID)
	clientIP = strings.TrimSpace(clientIP)
	if len(s.secret) < 32 || userID == "" || clientIP == "" {
		return ""
	}
	day := at.UTC().Format("2006-01-02")
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("vutame-visitor-v1\x00"))
	_, _ = mac.Write([]byte(userID))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(day))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(clientIP))
	digest := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(digest[:18])
}

func (s *Service) RecordProfileView(ctx context.Context, userID string, metadata Metadata) error {
	return s.record(ctx, strings.TrimSpace(userID), "", KindProfileView, metadata)
}

func (s *Service) RecordLinkClick(ctx context.Context, userID, linkID string, metadata Metadata) error {
	return s.record(ctx, strings.TrimSpace(userID), strings.TrimSpace(linkID), KindLinkClick, metadata)
}

func (s *Service) record(ctx context.Context, userID, linkID, kind string, metadata Metadata) error {
	if metadata.Bot {
		return nil
	}
	if userID == "" {
		return errors.New("analytics user is required")
	}
	if kind != KindProfileView && kind != KindLinkClick {
		return fmt.Errorf("unsupported analytics event kind %q", kind)
	}
	if kind == KindLinkClick && linkID == "" {
		return errors.New("analytics link is required for click event")
	}
	id, err := analyticsID()
	if err != nil {
		return err
	}
	var nullableLink any
	if linkID != "" {
		nullableLink = linkID
	}
	referrer := strings.ToLower(strings.TrimSpace(metadata.ReferrerHost))
	if len(referrer) > 253 {
		referrer = ""
	}
	device := metadata.DeviceClass
	switch device {
	case DeviceDesktop, DeviceMobile, DeviceTablet, DeviceUnknown:
	default:
		device = DeviceUnknown
	}
	visitorHash := strings.TrimSpace(metadata.VisitorHash)
	if len(visitorHash) > 64 {
		visitorHash = ""
	}
	campaign := NormalizeCampaign(metadata.Campaign)
	now := time.Now().UTC()
	retentionDays, err := s.analyticsRetentionDays(ctx, userID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin analytics record: %w", err)
	}
	defer tx.Rollback()
	cutoff := now.AddDate(0, 0, -retentionDays).Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_events WHERE user_id = ? AND created_at < ?`, userID, cutoff); err != nil {
		return fmt.Errorf("purge expired analytics: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO analytics_events (id, user_id, link_id, kind, visitor_hash, campaign, referrer_host, device_class, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, nullableLink, kind, visitorHash, campaign, referrer, device, now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record analytics event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit analytics event: %w", err)
	}
	return nil
}

func (s *Service) Dashboard(ctx context.Context, userID string, days int, now time.Time) (Dashboard, error) {
	userID = strings.TrimSpace(userID)
	var profileExists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id = ?`, userID).Scan(&profileExists); err != nil {
		return Dashboard{}, fmt.Errorf("check analytics owner profile: %w", err)
	}
	if profileExists != 1 {
		return Dashboard{}, ErrProfileRequired
	}
	days = NormalizeDashboardDays(days)
	retentionDays, err := s.analyticsRetentionDays(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}
	if days > retentionDays {
		days = retentionDays
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -(days - 1))
	cutoff := start.Format(time.RFC3339)

	result := Dashboard{
		Days:      days,
		StartDate: start.Format("2006-01-02"),
		EndDate:   today.Format("2006-01-02"),
		Series:    make([]DailyPoint, days),
		TopLinks:  []LinkMetric{},
		Referrers: []Breakdown{},
		Devices:   []Breakdown{},
		Campaigns: []Breakdown{},
	}
	for index := 0; index < days; index++ {
		result.Series[index].Date = start.AddDate(0, 0, index).Format("2006-01-02")
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN kind = 'profile_view' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = 'link_click' THEN 1 ELSE 0 END), 0)
		FROM analytics_events
		WHERE user_id = ? AND created_at >= ?
	`, userID, cutoff).Scan(&result.Summary.ProfileViews, &result.Summary.LinkClicks); err != nil {
		return Dashboard{}, fmt.Errorf("query analytics summary: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			substr(created_at, 1, 10) AS day,
			SUM(CASE WHEN kind = 'profile_view' THEN 1 ELSE 0 END) AS views,
			SUM(CASE WHEN kind = 'link_click' THEN 1 ELSE 0 END) AS clicks,
			COUNT(DISTINCT CASE WHEN kind = 'profile_view' AND visitor_hash <> '' THEN visitor_hash END) AS uniques
		FROM analytics_events
		WHERE user_id = ? AND created_at >= ?
		GROUP BY substr(created_at, 1, 10)
		ORDER BY day
	`, userID, cutoff)
	if err != nil {
		return Dashboard{}, fmt.Errorf("query analytics series: %w", err)
	}
	seriesByDate := make(map[string]DailyPoint, days)
	for rows.Next() {
		var point DailyPoint
		if err := rows.Scan(&point.Date, &point.ProfileViews, &point.LinkClicks, &point.UniqueVisitors); err != nil {
			_ = rows.Close()
			return Dashboard{}, fmt.Errorf("scan analytics series: %w", err)
		}
		seriesByDate[point.Date] = point
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Dashboard{}, fmt.Errorf("iterate analytics series: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Dashboard{}, fmt.Errorf("close analytics series: %w", err)
	}
	for index := range result.Series {
		if point, ok := seriesByDate[result.Series[index].Date]; ok {
			result.Series[index] = point
		}
		result.Summary.UniqueVisitors += result.Series[index].UniqueVisitors
	}

	result.TopLinks, err = s.topLinks(ctx, userID, cutoff)
	if err != nil {
		return Dashboard{}, err
	}
	result.Referrers, err = s.breakdown(ctx, userID, cutoff, `CASE WHEN referrer_host = '' THEN 'Direct / unknown' ELSE referrer_host END`)
	if err != nil {
		return Dashboard{}, err
	}
	result.Devices, err = s.breakdown(ctx, userID, cutoff, `device_class`)
	if err != nil {
		return Dashboard{}, err
	}
	result.Campaigns, err = s.campaignBreakdown(ctx, userID, cutoff)
	if err != nil {
		return Dashboard{}, err
	}
	return result, nil
}

func (s *Service) topLinks(ctx context.Context, userID, cutoff string) ([]LinkMetric, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(e.link_id, ''), COALESCE(l.label, 'Deleted link'), COUNT(*) AS clicks
		FROM analytics_events e
		LEFT JOIN links l ON l.id = e.link_id
		WHERE e.user_id = ? AND e.kind = 'link_click' AND e.created_at >= ?
		GROUP BY e.link_id, l.label
		ORDER BY clicks DESC, COALESCE(l.label, 'Deleted link')
		LIMIT 10
	`, userID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query top links: %w", err)
	}
	defer rows.Close()
	items := make([]LinkMetric, 0)
	for rows.Next() {
		var item LinkMetric
		if err := rows.Scan(&item.LinkID, &item.Label, &item.Clicks); err != nil {
			return nil, fmt.Errorf("scan top link: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top links: %w", err)
	}
	return items, nil
}

func (s *Service) breakdown(ctx context.Context, userID, cutoff, expression string) ([]Breakdown, error) {
	query := fmt.Sprintf(`
		SELECT %s AS name, COUNT(*) AS count
		FROM analytics_events
		WHERE user_id = ? AND created_at >= ?
		GROUP BY %s
		ORDER BY count DESC, name
		LIMIT 10
	`, expression, expression)
	rows, err := s.db.QueryContext(ctx, query, userID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query analytics breakdown: %w", err)
	}
	defer rows.Close()
	items := make([]Breakdown, 0)
	for rows.Next() {
		var item Breakdown
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, fmt.Errorf("scan analytics breakdown: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics breakdown: %w", err)
	}
	return items, nil
}

func (s *Service) campaignBreakdown(ctx context.Context, userID, cutoff string) ([]Breakdown, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT CASE WHEN campaign = '' THEN 'Unattributed' ELSE campaign END AS name, COUNT(*) AS count
		FROM analytics_events
		WHERE user_id = ? AND kind = 'profile_view' AND created_at >= ?
		GROUP BY CASE WHEN campaign = '' THEN 'Unattributed' ELSE campaign END
		ORDER BY count DESC, name
		LIMIT 10
	`, userID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query campaign breakdown: %w", err)
	}
	defer rows.Close()
	items := make([]Breakdown, 0)
	for rows.Next() {
		var item Breakdown
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, fmt.Errorf("scan campaign breakdown: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaign breakdown: %w", err)
	}
	return items, nil
}

func (s *Service) analyticsRetentionDays(ctx context.Context, userID string) (int, error) {
	var days int
	err := s.db.QueryRowContext(ctx, `SELECT analytics_retention_days FROM creator_data_settings WHERE user_id = ?`, userID).Scan(&days)
	if errors.Is(err, sql.ErrNoRows) {
		return 90, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get analytics retention: %w", err)
	}
	if days != 30 && days != 90 && days != 365 {
		return 90, nil
	}
	return days, nil
}

func analyticsID() (string, error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate analytics id: %w", err)
	}
	return "anl_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}
