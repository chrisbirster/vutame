package growth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestGrowthContactConsentAndRetention(t *testing.T) {
	db := openGrowthDB(t)
	seedGrowthProfile(t, db, "usr_growth", "growth")
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	block, err := service.OwnedBlock(ctx, "usr_growth")
	if err != nil {
		t.Fatal(err)
	}
	if block.Enabled || block.ConsentText == "" {
		t.Fatalf("default block = %#v", block)
	}
	block.Enabled = true
	block.Description = "Monthly product updates."
	block.ConsentText = "I agree to receive monthly product updates."
	block, err = service.UpdateBlock(ctx, "usr_growth", block)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitContact(ctx, "growth", "person@example.com", false, "launch", time.Now()); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("missing consent error = %v", err)
	}
	if _, err := service.SubmitContact(ctx, "growth", "not-email", true, "launch", time.Now()); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email error = %v", err)
	}

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	created, err := service.SubmitContact(ctx, "growth", "Person@Example.com", true, "  fall   launch  ", now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "person@example.com" || created.Campaign != "fall launch" {
		t.Fatalf("created = %#v", created)
	}
	block.ConsentText = "A newer consent statement."
	if _, err := service.UpdateBlock(ctx, "usr_growth", block); err != nil {
		t.Fatal(err)
	}
	var storedConsent string
	if err := db.QueryRow(`SELECT consent_text FROM contact_submissions WHERE id = ?`, created.ID).Scan(&storedConsent); err != nil {
		t.Fatal(err)
	}
	if storedConsent != "I agree to receive monthly product updates." {
		t.Fatalf("stored consent = %q", storedConsent)
	}

	old := now.AddDate(0, 0, -60).Format(time.RFC3339Nano)
	recent := now.AddDate(0, 0, -10).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO analytics_events (id, user_id, kind, created_at) VALUES ('old_metric', 'usr_growth', 'profile_view', ?), ('recent_metric', 'usr_growth', 'profile_view', ?)`, old, recent); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO contact_submissions (id, user_id, email, consent_text, created_at) VALUES ('old_contact', 'usr_growth', 'old@example.com', 'consent', ?), ('recent_contact', 'usr_growth', 'recent@example.com', 'consent', ?)`, old, recent); err != nil {
		t.Fatal(err)
	}
	settings, err := service.UpdateSettings(ctx, "usr_growth", DataSettings{AnalyticsRetentionDays: 30, ContactRetentionDays: 30}, now)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AnalyticsRetentionDays != 30 || settings.ContactRetentionDays != 30 {
		t.Fatalf("settings = %#v", settings)
	}
	var oldMetrics, oldContacts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_events WHERE id = 'old_metric'`).Scan(&oldMetrics); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM contact_submissions WHERE id = 'old_contact'`).Scan(&oldContacts); err != nil {
		t.Fatal(err)
	}
	if oldMetrics != 0 || oldContacts != 0 {
		t.Fatalf("expired rows metrics=%d contacts=%d", oldMetrics, oldContacts)
	}
}

func TestGrowthRejectsSuspendedCreatorCapture(t *testing.T) {
	db := openGrowthDB(t)
	seedGrowthProfile(t, db, "usr_suspended", "suspended")
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	block := DefaultBlock()
	block.Enabled = true
	if _, err := service.UpdateBlock(ctx, "usr_suspended", block); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO moderation_profiles (user_id, state, note, updated_at) VALUES ('usr_suspended', 'suspended', '', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublicBlock(ctx, "suspended"); !errors.Is(err, ErrContactDisabled) {
		t.Fatalf("public block error = %v", err)
	}
	if _, err := service.SubmitContact(ctx, "suspended", "person@example.com", true, "", time.Now()); !errors.Is(err, ErrContactDisabled) {
		t.Fatalf("submit error = %v", err)
	}
}

func openGrowthDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "growth.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func seedGrowthProfile(t *testing.T, db *sql.DB, userID, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, ?, ?)`, userID, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}
