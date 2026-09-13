package analytics

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
		return nil, errors.New("analytics: database is required")
	}
	for _, table := range []string{"profiles", "links", "analytics_events"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify analytics table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("analytics schema missing table %s; run Atlas schema apply", table)
		}
	}
	return &Service{db: db}, nil
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
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO analytics_events (id, user_id, link_id, kind, referrer_host, device_class, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, userID, nullableLink, kind, referrer, device, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record analytics event: %w", err)
	}
	return nil
}

func analyticsID() (string, error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate analytics id: %w", err)
	}
	return "anl_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}
