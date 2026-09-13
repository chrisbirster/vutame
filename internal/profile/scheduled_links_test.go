package profile

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidateLinkInputSchedule(t *testing.T) {
	input, err := ValidateLinkInput(LinkInput{
		Label:        "Launch",
		URL:          "https://example.com/launch",
		Kind:         "project",
		Featured:     true,
		VisibleFrom:  "2026-09-13T12:00:00-04:00",
		VisibleUntil: "2026-09-13T18:00:00Z",
		IsActive:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.VisibleFrom != "2026-09-13T16:00:00Z" || input.VisibleUntil != "2026-09-13T18:00:00Z" || !input.Featured {
		t.Fatalf("normalized schedule = %#v", input)
	}

	_, err = ValidateLinkInput(LinkInput{
		Label: "Bad", URL: "https://example.com", Kind: "website",
		VisibleFrom: "2026-09-13T18:00:00Z", VisibleUntil: "2026-09-13T17:00:00Z",
	})
	if !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("reversed schedule error = %v, want ErrInvalidLink", err)
	}

	_, err = ValidateLinkInput(LinkInput{
		Label: "Bad", URL: "https://example.com", Kind: "website", VisibleFrom: "tomorrow",
	})
	if !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("invalid timestamp error = %v, want ErrInvalidLink", err)
	}
}

func TestPublicProfileFiltersScheduleAndPinsFeatured(t *testing.T) {
	now := time.Date(2026, 9, 13, 16, 0, 0, 0, time.UTC)
	item := Profile{
		ID: "usr_schedule", Handle: "schedule", Theme: DefaultTheme,
		Links: []Link{
			{ID: "normal", Label: "Normal", URL: "https://example.com/normal", Kind: "website", Position: 0, IsActive: true},
			{ID: "future", Label: "Future", URL: "https://example.com/future", Kind: "website", Position: 1, IsActive: true, VisibleFrom: "2026-09-13T16:01:00Z"},
			{ID: "featured", Label: "Featured", URL: "https://example.com/featured", Kind: "project", Position: 2, IsActive: true, Featured: true, VisibleFrom: "2026-09-13T15:00:00Z", VisibleUntil: "2026-09-13T17:00:00Z"},
			{ID: "expired", Label: "Expired", URL: "https://example.com/expired", Kind: "website", Position: 3, IsActive: true, VisibleUntil: "2026-09-13T16:00:00Z"},
			{ID: "disabled", Label: "Disabled", URL: "https://example.com/disabled", Kind: "website", Position: 4, IsActive: false},
		},
	}

	public := publicProfileAt(item, now)
	if len(public.Links) != 2 || public.Links[0].ID != "featured" || public.Links[1].ID != "normal" {
		t.Fatalf("public links = %#v", public.Links)
	}
}

func TestSQLiteStorePersistsFeaturedSchedule(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedSQLiteProfile(t, store.db)
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)

	created, err := store.CreateLink(context.Background(), "usr_01_chrisdontmiss", LinkInput{
		Label: "Featured launch", URL: "https://example.com/launch", Kind: "project",
		Featured: true, VisibleFrom: from, VisibleUntil: until, IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Featured || created.VisibleFrom != from || created.VisibleUntil != until {
		t.Fatalf("created link = %#v", created)
	}

	owned, err := store.GetOwned(context.Background(), "usr_01_chrisdontmiss")
	if err != nil {
		t.Fatal(err)
	}
	var found *Link
	for index := range owned.Links {
		if owned.Links[index].ID == created.ID {
			found = &owned.Links[index]
			break
		}
	}
	if found == nil || !found.Featured || found.VisibleFrom != from || found.VisibleUntil != until {
		t.Fatalf("owned scheduled link = %#v", found)
	}

	public, err := store.Get("chrisdontmiss")
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Links) == 0 || public.Links[0].ID != created.ID {
		t.Fatalf("featured link not pinned in public profile: %#v", public.Links)
	}
}
