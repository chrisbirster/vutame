package media

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Wl9sAAAAASUVORK5CYII="

func TestAvatarLifecycle(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	png := testPNG(t)

	first, err := service.UploadAvatar(ctx, "usr_media", bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentType != "image/png" || first.SizeBytes != int64(len(png)) || !strings.HasPrefix(first.URL, "/media/med_") {
		t.Fatalf("first asset = %#v", first)
	}
	assertProfileAvatar(t, db, first.URL)
	opened, err := service.Open(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(opened.Reader)
	opened.Reader.Close()
	if err != nil || !bytes.Equal(got, png) {
		t.Fatalf("opened first avatar len=%d err=%v", len(got), err)
	}

	second, err := service.UploadAvatar(ctx, "usr_media", bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("replacement reused immutable media id")
	}
	assertProfileAvatar(t, db, second.URL)
	if _, err := service.Open(ctx, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old asset open error = %v, want ErrNotFound", err)
	}

	if err := service.DeleteAvatar(ctx, "usr_media"); err != nil {
		t.Fatal(err)
	}
	assertProfileAvatar(t, db, "")
	if _, err := service.Open(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted asset open error = %v, want ErrNotFound", err)
	}
	if err := service.DeleteAvatar(ctx, "usr_media"); err != nil {
		t.Fatalf("idempotent delete failed: %v", err)
	}
}

func TestUploadAvatarRejectsInvalidAndOversizedFiles(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	if _, err := service.UploadAvatar(ctx, "usr_media", strings.NewReader("not an image")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("invalid image error = %v, want ErrInvalidImage", err)
	}
	if _, err := service.UploadAvatar(ctx, "usr_media", bytes.NewReader(make([]byte, MaxImageBytes+1))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized image error = %v, want ErrTooLarge", err)
	}
}

func TestFileStoreRejectsTraversal(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "../escape.png", []byte("x")); err == nil {
		t.Fatal("expected traversal key to be rejected")
	}
}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES ('usr_media', 'media@example.com', 1, '2026-09-13T00:00:00Z', '2026-09-13T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (user_id, handle, display_name, bio, avatar_url, theme, verified, created_at, updated_at) VALUES ('usr_media', 'media-user', 'Media User', '', '', 'midnight', 0, '2026-09-13T00:00:00Z', '2026-09-13T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	blobs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db, blobs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service, db
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(testPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertProfileAvatar(t *testing.T, db *sql.DB, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT avatar_url FROM profiles WHERE user_id = 'usr_media'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("avatar_url = %q, want %q", got, want)
	}
}
