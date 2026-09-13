package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "modernc.org/sqlite"
)

func TestAnalyticsProfileViewAndTrackedClick(t *testing.T) {
	db, err := sql.Open("sqlite", "file:analytics-http?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES ('usr_creator', 'creator@example.com', 1, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES ('usr_creator', 'creator', 'Creator', 'Building', '', 'midnight', 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, position, is_active, created_at, updated_at)
		VALUES ('lnk_live', 'usr_creator', 'Project', 'https://example.com/project', 'project', '', 0, 0, 1, ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO links (id, user_id, label, url, kind, thumbnail_url, featured, position, is_active, created_at, updated_at)
		VALUES ('lnk_off', 'usr_creator', 'Disabled', 'https://example.com/off', 'website', '', 0, 1, 0, ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	analyticsService, err := analytics.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Vutame</title><meta name="description" content="Vutame"></head><body><div id="root"></div></body></html>`))
	})
	handler := New(web, profiles, Options{Analytics: analyticsService, ProfileOrigin: "https://vuta.me"})

	view := httptest.NewRequest(http.MethodGet, "/@creator", nil)
	view.Header.Set("Referer", "https://www.google.com/search?q=creator")
	view.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit")
	viewRec := httptest.NewRecorder()
	handler.ServeHTTP(viewRec, view)
	if viewRec.Code != http.StatusOK || !strings.Contains(viewRec.Body.String(), `/out/lnk_live`) {
		t.Fatalf("profile status=%d body=%q", viewRec.Code, viewRec.Body.String())
	}

	click := httptest.NewRequest(http.MethodGet, "/out/lnk_live", nil)
	click.Header.Set("Referer", "https://vuta.me/@creator?private=ignored")
	click.Header.Set("User-Agent", "Mozilla/5.0 (iPhone) Mobile")
	clickRec := httptest.NewRecorder()
	handler.ServeHTTP(clickRec, click)
	if clickRec.Code != http.StatusFound || clickRec.Header().Get("Location") != "https://example.com/project" {
		t.Fatalf("click status=%d location=%q body=%q", clickRec.Code, clickRec.Header().Get("Location"), clickRec.Body.String())
	}
	if clickRec.Header().Get("Cache-Control") != "no-store" || clickRec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("click headers = %#v", clickRec.Header())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("event count = %d", count)
	}
	var referrer, device string
	if err := db.QueryRow(`SELECT referrer_host, device_class FROM analytics_events WHERE kind = 'link_click'`).Scan(&referrer, &device); err != nil {
		t.Fatal(err)
	}
	if referrer != "vuta.me" || device != analytics.DeviceMobile {
		t.Fatalf("click metadata referrer=%q device=%q", referrer, device)
	}

	bot := httptest.NewRequest(http.MethodGet, "/@creator", nil)
	bot.Header.Set("User-Agent", "Googlebot/2.1")
	botRec := httptest.NewRecorder()
	handler.ServeHTTP(botRec, bot)
	if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("bot should not add analytics event; count=%d", count)
	}

	disabled := httptest.NewRecorder()
	handler.ServeHTTP(disabled, httptest.NewRequest(http.MethodGet, "/out/lnk_off", nil))
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled redirect status=%d body=%q", disabled.Code, disabled.Body.String())
	}
}
