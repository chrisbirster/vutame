package social

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

func TestSQLiteStoreFollowSearchAndMetadata(t *testing.T) {
	db := openSocialTestDB(t)
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	insertCreator(t, db, "usr_alice", "alice", "Alice Builder", "Go and systems")
	insertCreator(t, db, "usr_bob", "bob", "Bob Zig", "Low-level tools")
	insertCreator(t, db, "usr_cara", "cara", "Cara Art", "Illustration and games")
	insertUser(t, db, "usr_noprofile", "noprofile@example.com")

	bob, err := store.UpdateMetadata(ctx, "usr_bob", MetadataInput{Category: "Engineering", Interests: []string{"Zig", "Systems", "zig"}})
	if err != nil {
		t.Fatal(err)
	}
	if bob.Category != "Engineering" || len(bob.Interests) != 2 || bob.Interests[0] != "systems" || bob.Interests[1] != "zig" {
		t.Fatalf("bob metadata = %#v", bob)
	}
	if _, err := store.UpdateMetadata(ctx, "usr_cara", MetadataInput{Category: "Art", Interests: []string{"games", "illustration"}}); err != nil {
		t.Fatal(err)
	}

	if err := store.Follow(ctx, "usr_alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(ctx, "usr_alice", "bob"); err != nil {
		t.Fatalf("duplicate follow should be idempotent: %v", err)
	}
	if err := store.Follow(ctx, "usr_bob", "bob"); !errors.Is(err, ErrSelfFollow) {
		t.Fatalf("self follow error = %v", err)
	}
	if err := store.Follow(ctx, "usr_noprofile", "bob"); !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("unclaimed follower error = %v", err)
	}

	bob, err = store.Creator(ctx, "bob", "usr_alice")
	if err != nil {
		t.Fatal(err)
	}
	if bob.FollowerCount != 1 || bob.FollowingCount != 0 || !bob.ViewerFollows {
		t.Fatalf("bob social state = %#v", bob)
	}

	followers, err := store.Followers(ctx, "bob", "usr_bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(followers) != 1 || followers[0].Handle != "alice" {
		t.Fatalf("followers = %#v", followers)
	}
	following, err := store.Following(ctx, "alice", "usr_alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(following) != 1 || following[0].Handle != "bob" {
		t.Fatalf("following = %#v", following)
	}

	results, err := store.Search(ctx, SearchInput{Query: "zig"}, "usr_alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Handle != "bob" || !results[0].ViewerFollows {
		t.Fatalf("zig search = %#v", results)
	}
	results, err = store.Search(ctx, SearchInput{Category: "art", Interest: "games"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Handle != "cara" {
		t.Fatalf("art search = %#v", results)
	}

	if err := store.Unfollow(ctx, "usr_alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if err := store.Unfollow(ctx, "usr_alice", "bob"); err != nil {
		t.Fatalf("duplicate unfollow should be idempotent: %v", err)
	}
	bob, err = store.Creator(ctx, "bob", "usr_alice")
	if err != nil {
		t.Fatal(err)
	}
	if bob.FollowerCount != 0 || bob.ViewerFollows {
		t.Fatalf("bob after unfollow = %#v", bob)
	}
}

func TestValidateMetadataLimits(t *testing.T) {
	if _, err := ValidateMetadata(MetadataInput{Interests: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("too many interests error = %v", err)
	}
	input, err := ValidateMetadata(MetadataInput{Category: "  Software  ", Interests: []string{" Go ", "go", "Zig"}})
	if err != nil {
		t.Fatal(err)
	}
	if input.Category != "Software" || len(input.Interests) != 2 || input.Interests[0] != "go" || input.Interests[1] != "zig" {
		t.Fatalf("normalized metadata = %#v", input)
	}
}

func openSocialTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "social.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
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

func insertUser(t *testing.T, db *sql.DB, id, email string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, id, email, now, now); err != nil {
		t.Fatal(err)
	}
}

func insertCreator(t *testing.T, db *sql.DB, id, handle, displayName, bio string) {
	t.Helper()
	insertUser(t, db, id, handle+"@example.com")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, atproto_did, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', 'midnight', 0, NULL, ?, ?)
	`, id, handle, displayName, bio, now, now); err != nil {
		t.Fatal(err)
	}
}
