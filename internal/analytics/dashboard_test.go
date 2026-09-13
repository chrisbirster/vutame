package analytics

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestVisitorTokenRotatesDailyAndIsCreatorScoped(t *testing.T) {
	db := openAnalyticsDB(t, "sqlite", "file:visitor-token?mode=memory&cache=shared")
	service, err := NewService(db, []byte(strings.Repeat("v", 32)))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	first := service.VisitorToken("usr_one", "203.0.113.9", at)
	if first == "" {
		t.Fatal("expected visitor token")
	}
	if same := service.VisitorToken("usr_one", "203.0.113.9", at.Add(3*time.Hour)); same != first {
		t.Fatalf("same-day token changed: %q != %q", same, first)
	}
	if nextDay := service.VisitorToken("usr_one", "203.0.113.9", at.Add(24*time.Hour)); nextDay == first {
		t.Fatal("visitor token should rotate across UTC days")
	}
	if otherCreator := service.VisitorToken("usr_two", "203.0.113.9", at); otherCreator == first {
		t.Fatal("visitor token should be creator-scoped")
	}
	if got := ClientIP("198.51.100.7", "10.0.0.2:1234"); got != "198.51.100.7" {
		t.Fatalf("fly client IP = %q", got)
	}
	if got := ClientIP("not-an-ip", "[2001:db8::5]:443"); got != "2001:db8::5" {
		t.Fatalf("remote fallback = %q", got)
	}
}

func TestDashboardAggregatesDailyMetrics(t *testing.T) {
	db := openAnalyticsDB(t, "sqlite", "file:dashboard?mode=memory&cache=shared")
	insertAnalyticsCreator(t, db, "usr_dash", "dash")
	insertAnalyticsLink(t, db, "lnk_one", "usr_dash", "https://example.com/one", true, "", "")
	insertAnalyticsLink(t, db, "lnk_two", "usr_dash", "https://example.com/two", true, "", "")
	if _, err := db.Exec(`UPDATE links SET label = 'One' WHERE id = 'lnk_one'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE links SET label = 'Two' WHERE id = 'lnk_two'`); err != nil {
		t.Fatal(err)
	}

	insertDashboardEvent := func(id, kind, linkID, visitor, referrer, device, createdAt string) {
		t.Helper()
		var link any
		if linkID != "" {
			link = linkID
		}
		if _, err := db.Exec(`
			INSERT INTO analytics_events (id, user_id, link_id, kind, visitor_hash, referrer_host, device_class, created_at)
			VALUES (?, 'usr_dash', ?, ?, ?, ?, ?, ?)
		`, id, link, kind, visitor, referrer, device, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	insertDashboardEvent("e1", KindProfileView, "", "day11-a", "google.com", DeviceDesktop, "2026-09-11T10:00:00Z")
	insertDashboardEvent("e2", KindProfileView, "", "day11-a", "google.com", DeviceDesktop, "2026-09-11T11:00:00Z")
	insertDashboardEvent("e3", KindLinkClick, "lnk_one", "day11-a", "vuta.me", DeviceDesktop, "2026-09-11T11:05:00Z")
	insertDashboardEvent("e4", KindProfileView, "", "day12-a", "", DeviceMobile, "2026-09-12T08:00:00Z")
	insertDashboardEvent("e5", KindProfileView, "", "day12-b", "reddit.com", DeviceMobile, "2026-09-12T09:00:00Z")
	insertDashboardEvent("e6", KindLinkClick, "lnk_one", "day12-a", "vuta.me", DeviceMobile, "2026-09-12T09:05:00Z")
	insertDashboardEvent("e7", KindLinkClick, "lnk_one", "day12-b", "vuta.me", DeviceMobile, "2026-09-12T09:10:00Z")
	insertDashboardEvent("e8", KindLinkClick, "lnk_two", "day12-b", "reddit.com", DeviceMobile, "2026-09-12T09:15:00Z")

	service, err := NewService(db, []byte(strings.Repeat("d", 32)))
	if err != nil {
		t.Fatal(err)
	}
	dashboard, err := service.Dashboard(context.Background(), "usr_dash", 3, time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Summary.ProfileViews != 4 || dashboard.Summary.LinkClicks != 4 || dashboard.Summary.UniqueVisitors != 3 {
		t.Fatalf("summary = %#v", dashboard.Summary)
	}
	if len(dashboard.Series) != 3 || dashboard.Series[0].Date != "2026-09-11" || dashboard.Series[0].UniqueVisitors != 1 || dashboard.Series[1].UniqueVisitors != 2 || dashboard.Series[2].ProfileViews != 0 {
		t.Fatalf("series = %#v", dashboard.Series)
	}
	if len(dashboard.TopLinks) != 2 || dashboard.TopLinks[0].LinkID != "lnk_one" || dashboard.TopLinks[0].Clicks != 3 || dashboard.TopLinks[1].Clicks != 1 {
		t.Fatalf("top links = %#v", dashboard.TopLinks)
	}
	if len(dashboard.Referrers) == 0 || len(dashboard.Devices) == 0 {
		t.Fatalf("breakdowns referrers=%#v devices=%#v", dashboard.Referrers, dashboard.Devices)
	}
	if _, err := service.Dashboard(context.Background(), "missing", 30, time.Now()); err != ErrProfileRequired {
		t.Fatalf("missing profile error = %v", err)
	}
}
