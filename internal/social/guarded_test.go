package social

import (
	"context"
	"errors"
	"testing"

	"github.com/chrisbirster/vutame/internal/safety"
)

func TestGuardedStoreAppliesBlockPrivacyAndModeration(t *testing.T) {
	db := openSocialTestDB(t)
	base, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	guarded, err := NewGuardedStore(base, db)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := safety.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	insertCreator(t, db, "usr_a", "alpha", "Alpha", "builder")
	insertCreator(t, db, "usr_b", "bravo", "Bravo", "builder")
	insertCreator(t, db, "usr_c", "charlie", "Charlie", "builder")

	if _, err := policies.UpdatePrivacy(ctx, "usr_b", safety.Privacy{Discoverable: false, ActivityVisible: true, AllowFollows: true}); err != nil {
		t.Fatal(err)
	}
	page, err := guarded.SearchPage(ctx, SearchInput{Query: "builder", Limit: 10}, "usr_a")
	if err != nil {
		t.Fatal(err)
	}
	for _, creator := range page.Creators {
		if creator.Handle == "bravo" {
			t.Fatal("undiscoverable creator leaked into search")
		}
	}
	if _, err := guarded.Creator(ctx, "bravo", "usr_a"); err != nil {
		t.Fatalf("direct creator lookup should remain available when only discoverability is off: %v", err)
	}

	if err := policies.Block(ctx, "usr_a", "charlie"); err != nil {
		t.Fatal(err)
	}
	if _, err := guarded.Creator(ctx, "charlie", "usr_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("blocked creator lookup error = %v", err)
	}
	if err := guarded.Follow(ctx, "usr_a", "charlie"); !errors.Is(err, ErrRelationshipBlocked) {
		t.Fatalf("blocked follow error = %v", err)
	}

	if _, err := policies.UpdatePrivacy(ctx, "usr_b", safety.Privacy{Discoverable: true, ActivityVisible: true, AllowFollows: false}); err != nil {
		t.Fatal(err)
	}
	if err := guarded.Follow(ctx, "usr_a", "bravo"); !errors.Is(err, ErrFollowsDisabled) {
		t.Fatalf("private follow error = %v", err)
	}

	if err := policies.SetModeration(ctx, "bravo", safety.Moderation{State: "suspended"}); err != nil {
		t.Fatal(err)
	}
	if _, err := guarded.Creator(ctx, "bravo", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("suspended creator lookup error = %v", err)
	}
}
