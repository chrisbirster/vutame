package analytics

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestMetadataFromHeadersIsCoarseAndBotAware(t *testing.T) {
	meta := MetadataFromHeaders("https://www.Reddit.com/r/golang/comments/123?utm_source=secret", "Mozilla/5.0 (iPhone; CPU iPhone OS) Mobile")
	if meta.ReferrerHost != "reddit.com" {
		t.Fatalf("referrer host = %q", meta.ReferrerHost)
	}
	if meta.DeviceClass != DeviceMobile || meta.Bot {
		t.Fatalf("metadata = %#v", meta)
	}
	bot := MetadataFromHeaders("https://example.com/path", "Googlebot/2.1")
	if !bot.Bot {
		t.Fatalf("bot metadata = %#v", bot)
	}
	if MetadataFromHeaders("not a url", "").ReferrerHost != "" {
		t.Fatal("invalid referrer should be discarded")
	}
}

func TestServiceRecordsOnlyCoarseNonBotEvents(t *testing.T) {
	db := openAnalyticsDB(t, "sqlite", "file:analytics?mode=memory&cache=shared")
	insertAnalyticsCreator(t, db, "usr_creator", "creator")
	insertAnalyticsLink(t, db, "lnk_public", "usr_creator", "https://example.com/project", true, "", "")
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	viewMeta := MetadataFromHeaders("https://www.google.com/search?q=vutame", "Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit")
	if err := service.RecordProfileView(ctx, "usr_creator", viewMeta); err != nil {
		t.Fatal(err)
	}
	clickMeta := MetadataFromHeaders("https://vuta.me/@creator?private=path", "Mozilla/5.0 (iPad; CPU OS) AppleWebKit")
	if err := service.RecordLinkClick(ctx, "usr_creator", "lnk_public", clickMeta); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordProfileView(ctx, "usr_creator", MetadataFromHeaders("https://crawler.example/path", "Googlebot/2.1")); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`SELECT kind, COALESCE(link_id, ''), referrer_host, device_class FROM analytics_events ORDER BY kind DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got [][4]string
	for rows.Next() {
		var row [4]string
		if err := rows.Scan(&row[0], &row[1], &row[2], &row[3]); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("events = %#v", got)
	}
	if got[0] != [4]string{KindProfileView, "", "google.com", DeviceDesktop} {
		t.Fatalf("profile event = %#v", got[0])
	}
	if got[1] != [4]string{KindLinkClick, "lnk_public", "vuta.me", DeviceTablet} {
		t.Fatalf("click event = %#v", got[1])
	}
}

func openAnalyticsDB(t *testing.T, driver, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		db.Close()
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertAnalyticsCreator(t *testing.T, db *sql.DB, userID, handle string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, handle+"@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at) VALUES (?, ?, ?, '', '', 'midnight', 0, NULL, ?, ?)`, userID, handle, handle, now, now); err != nil {
		t.Fatal(err)
	}
}

func insertAnalyticsLink(t *testing.T, db *sql.DB, id, userID, target string, active bool, visibleFrom, visibleUntil string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	activeInt := 0
	if active {
		activeInt = 1
	}
	var from any
	if visibleFrom != "" {
		from = visibleFrom
	}
	var until any
	if visibleUntil != "" {
		until = visibleUntil
	}
	if _, err := db.Exec(`
		INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active, created_at, updated_at)
		VALUES (?, ?, 'Project', ?, 'project', '', 0, ?, ?, 0, ?, ?, ?)
	`, id, userID, target, from, until, activeInt, now, now); err != nil {
		t.Fatal(err)
	}
}
