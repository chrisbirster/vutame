package database_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/media"
	"github.com/chrisbirster/vutame/internal/profile"
	_ "turso.tech/database/tursogo"
)

const tursoMediaPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Wl9sAAAAASUVORK5CYII="

func TestManagedAvatarRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	profiles, err := profile.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := media.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaService, err := media.NewService(db, blobs)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const userID = "usr_turso_media"
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, userID, "media-turso@example.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Claim(ctx, userID, "media-turso"); err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(tursoMediaPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := mediaService.UploadAvatar(ctx, userID, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := profiles.Get("media-turso")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AvatarURL != asset.URL {
		t.Fatalf("avatar URL = %q, want %q", loaded.AvatarURL, asset.URL)
	}
	if err := mediaService.DeleteAvatar(ctx, userID); err != nil {
		t.Fatal(err)
	}
	loaded, err = profiles.Get("media-turso")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AvatarURL != "" {
		t.Fatalf("avatar URL after delete = %q", loaded.AvatarURL)
	}
}
