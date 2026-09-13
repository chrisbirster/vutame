package social

import (
	"context"
	"errors"
	"testing"
)

func TestSearchPagePaginatesWithoutDuplicates(t *testing.T) {
	db := openSocialTestDB(t)
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	insertCreator(t, db, "usr_a", "alpha", "Alpha", "builder")
	insertCreator(t, db, "usr_b", "bravo", "Bravo", "builder")
	insertCreator(t, db, "usr_c", "charlie", "Charlie", "builder")
	if err := store.Follow(ctx, "usr_a", "bravo"); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(ctx, "usr_c", "bravo"); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(ctx, "usr_a", "charlie"); err != nil {
		t.Fatal(err)
	}

	first, err := store.SearchPage(ctx, SearchInput{Query: "builder", Limit: 2}, "usr_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Creators) != 2 || first.Creators[0].Handle != "bravo" || first.Creators[1].Handle != "charlie" {
		t.Fatalf("first page = %#v", first)
	}
	if first.NextCursor == "" {
		t.Fatal("expected next cursor")
	}

	second, err := store.SearchPage(ctx, SearchInput{Query: "builder", Limit: 2, Cursor: first.NextCursor}, "usr_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Creators) != 1 || second.Creators[0].Handle != "alpha" || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
}

func TestSearchPageRejectsInvalidCursor(t *testing.T) {
	db := openSocialTestDB(t)
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SearchPage(context.Background(), SearchInput{Cursor: "not-valid-base64!!"}, "")
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor error = %v", err)
	}
}
