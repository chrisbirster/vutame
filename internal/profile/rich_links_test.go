package profile

import (
	"context"
	"errors"
	"testing"
)

func TestValidateLinkInputTypedKindsAndThumbnail(t *testing.T) {
	input, err := ValidateLinkInput(LinkInput{
		Label:        "Project",
		URL:          "https://example.com/project",
		Kind:         " GitHub ",
		ThumbnailURL: "https://example.com/thumb.jpg",
		IsActive:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Kind != "github" || input.ThumbnailURL != "https://example.com/thumb.jpg" {
		t.Fatalf("normalized input = %#v", input)
	}

	if _, err := ValidateLinkInput(LinkInput{Label: "Bad", URL: "https://example.com", Kind: "discord"}); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("unsupported kind error = %v, want ErrInvalidLink", err)
	}
	if _, err := ValidateLinkInput(LinkInput{Label: "Bad", URL: "https://example.com", Kind: "website", ThumbnailURL: "javascript:alert(1)"}); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("bad thumbnail error = %v, want ErrInvalidLink", err)
	}
}

func TestSQLiteStorePersistsRichLinkMetadata(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedSQLiteProfile(t, store.db)

	created, err := store.CreateLink(context.Background(), "usr_01_chrisdontmiss", LinkInput{
		Label:        "Demo",
		URL:          "https://youtube.com/watch?v=demo",
		Kind:         "youtube",
		ThumbnailURL: "https://example.com/demo.jpg",
		IsActive:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "youtube" || created.ThumbnailURL != "https://example.com/demo.jpg" {
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
	if found == nil || found.Kind != "youtube" || found.ThumbnailURL != "https://example.com/demo.jpg" {
		t.Fatalf("owned rich link = %#v", found)
	}

	public, err := store.Get("chrisdontmiss")
	if err != nil {
		t.Fatal(err)
	}
	found = nil
	for index := range public.Links {
		if public.Links[index].ID == created.ID {
			found = &public.Links[index]
			break
		}
	}
	if found == nil || found.Kind != "youtube" || found.ThumbnailURL != "https://example.com/demo.jpg" {
		t.Fatalf("public rich link = %#v", found)
	}
}
