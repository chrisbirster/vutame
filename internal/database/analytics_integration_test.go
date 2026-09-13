package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "turso.tech/database/tursogo"
)

func TestAnalyticsRunsOnTursoEngine(t *testing.T) {
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES ('usr_metrics', 'metrics@example.com', 1, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES ('usr_metrics', 'metrics', 'Metrics', '', '', 'midnight', 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, position, is_active, created_at, updated_at)
		VALUES ('lnk_metrics', 'usr_metrics', 'Metrics Link', 'https://example.com/metrics', 'website', '', 0, 0, 1, ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	profileStore, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	target, err := profileStore.ResolvePublicLink("lnk_metrics")
	if err != nil {
		t.Fatal(err)
	}
	if target.UserID != "usr_metrics" || target.Link.URL != "https://example.com/metrics" {
		t.Fatalf("target = %#v", target)
	}
	service, err := analytics.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := service.RecordProfileView(ctx, "usr_metrics", analytics.MetadataFromHeaders("https://search.example/query", "Mozilla/5.0")); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordLinkClick(ctx, target.UserID, target.Link.ID, analytics.MetadataFromHeaders("https://vuta.me/@metrics", "Mozilla/5.0 (Android) Mobile")); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_events WHERE user_id = 'usr_metrics'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("analytics count = %d", count)
	}
}
