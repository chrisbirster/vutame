package database_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/growth"
	_ "turso.tech/database/tursogo"
)

func TestGrowthToolsRunOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES ('usr_growth_turso', 'growth-turso@example.com', 1, ?, ?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES ('usr_growth_turso', 'growth-turso', 'Growth Turso', '', '', 'midnight', 0, ?, ?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	growthService, err := growth.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	block := growth.DefaultBlock()
	block.Enabled = true
	block.ConsentText = "I agree to creator updates."
	if _, err := growthService.UpdateBlock(context.Background(), "usr_growth_turso", block); err != nil {
		t.Fatal(err)
	}
	contact, err := growthService.SubmitContact(context.Background(), "growth-turso", "person@example.com", true, "spring-launch", now)
	if err != nil {
		t.Fatal(err)
	}
	if contact.Email != "person@example.com" || contact.Campaign != "spring-launch" {
		t.Fatalf("contact = %#v", contact)
	}

	analyticsService, err := analytics.NewService(db, []byte(strings.Repeat("t", 32)))
	if err != nil {
		t.Fatal(err)
	}
	metadata := analytics.Metadata{VisitorHash: "daily-token", Campaign: "spring-launch", DeviceClass: analytics.DeviceMobile}
	if err := analyticsService.RecordProfileView(context.Background(), "usr_growth_turso", metadata); err != nil {
		t.Fatal(err)
	}
	dashboard, err := analyticsService.Dashboard(context.Background(), "usr_growth_turso", 30, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.Campaigns) == 0 || dashboard.Campaigns[0].Name != "spring-launch" || dashboard.Campaigns[0].Count != 1 {
		t.Fatalf("campaigns = %#v", dashboard.Campaigns)
	}
	settings, err := growthService.UpdateSettings(context.Background(), "usr_growth_turso", growth.DataSettings{AnalyticsRetentionDays: 30, ContactRetentionDays: 30}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if settings.AnalyticsRetentionDays != 30 || settings.ContactRetentionDays != 30 {
		t.Fatalf("settings = %#v", settings)
	}
}
