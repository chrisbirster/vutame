package profile

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteEditorClaimProfileAndLinks(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedEditorUser(t, store, "usr_editor", "editor@example.com")
	ctx := context.Background()

	item, err := store.Claim(ctx, "usr_editor", "@Creator-One")
	if err != nil {
		t.Fatal(err)
	}
	if item.Handle != "creator-one" || item.DisplayName != "@creator-one" {
		t.Fatalf("claimed profile = %#v", item)
	}
	if _, err := store.Claim(ctx, "usr_editor", "another-name"); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("second claim err = %v, want ErrProfileExists", err)
	}

	item, err = store.Update(ctx, "usr_editor", UpdateInput{
		DisplayName: "Creator One",
		Bio:         "Building a portable internet profile.",
		AvatarURL:   "https://example.com/avatar.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.DisplayName != "Creator One" || item.Bio == "" {
		t.Fatalf("updated profile = %#v", item)
	}

	first, err := store.CreateLink(ctx, "usr_editor", LinkInput{Label: "First", URL: "https://example.com/first", Kind: "website", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateLink(ctx, "usr_editor", LinkInput{Label: "Second", URL: "https://example.com/second", Kind: "project", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Position != 0 || second.Position != 1 {
		t.Fatalf("positions = %d,%d", first.Position, second.Position)
	}

	second, err = store.UpdateLink(ctx, "usr_editor", second.ID, LinkInput{Label: "Hidden second", URL: second.URL, Kind: second.Kind, IsActive: false})
	if err != nil {
		t.Fatal(err)
	}
	if second.IsActive {
		t.Fatal("updated link should be hidden")
	}
	if err := store.ReorderLinks(ctx, "usr_editor", []string{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}

	owned, err := store.GetOwned(ctx, "usr_editor")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned.Links) != 2 || owned.Links[0].ID != second.ID || owned.Links[0].Position != 0 || owned.Links[1].ID != first.ID || owned.Links[1].Position != 1 {
		t.Fatalf("owned links after reorder = %#v", owned.Links)
	}
	public, err := store.Get("creator-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Links) != 1 || public.Links[0].ID != first.ID {
		t.Fatalf("public links = %#v, want only active link", public.Links)
	}

	if err := store.DeleteLink(ctx, "usr_editor", first.ID); err != nil {
		t.Fatal(err)
	}
	owned, err = store.GetOwned(ctx, "usr_editor")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned.Links) != 1 || owned.Links[0].ID != second.ID || owned.Links[0].Position != 0 {
		t.Fatalf("links after delete = %#v", owned.Links)
	}
}

func TestSQLiteEditorEnforcesOwnershipAndHandleUniqueness(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedEditorUser(t, store, "usr_one", "one@example.com")
	seedEditorUser(t, store, "usr_two", "two@example.com")
	ctx := context.Background()

	if _, err := store.Claim(ctx, "usr_one", "creator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "usr_two", "creator"); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("duplicate handle err = %v, want ErrHandleTaken", err)
	}
	if _, err := store.Claim(ctx, "usr_two", "creator-two"); err != nil {
		t.Fatal(err)
	}
	link, err := store.CreateLink(ctx, "usr_one", LinkInput{Label: "Owned", URL: "https://example.com", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateLink(ctx, "usr_two", link.ID, LinkInput{Label: "Nope", URL: "https://example.com/nope", IsActive: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account update err = %v, want ErrNotFound", err)
	}
	if err := store.DeleteLink(ctx, "usr_two", link.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account delete err = %v, want ErrNotFound", err)
	}
}

func seedEditorUser(t *testing.T, store *SQLiteStore, id, email string) {
	t.Helper()
	if _, err := store.db.Exec(`
		INSERT INTO users (id, email, email_verified, created_at, updated_at)
		VALUES (?, ?, 1, '2026-09-11T00:00:00Z', '2026-09-11T00:00:00Z')
	`, id, email); err != nil {
		t.Fatal(err)
	}
}
