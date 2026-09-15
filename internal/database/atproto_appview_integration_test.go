package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/atproto"
	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "turso.tech/database/tursogo"
)

func TestATProtoAppViewIngestionRunsOnTursoEngine(t *testing.T) {
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

	store, err := atproto.NewStore(db, []byte(strings.Repeat("a", 32)), atproto.Config{
		ClientID: "https://vutame.example/oauth-client-metadata.json", RedirectURI: "https://vutame.example/api/v1/me/atproto/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}

	profileEvent := map[string]any{
		"did": "did:plc:turso-appview", "kind": "commit", "cursor": 9001,
		"commit": map[string]any{
			"operation": "create", "collection": atproto.ProfileCollection, "rkey": "self", "cid": "cid-profile",
			"record": map[string]any{
				"$type": atproto.ProfileCollection, "displayName": "Turso Portable", "handle": "turso-portable.example",
				"bio": "indexed through the Turso engine", "theme": "forest", "verified": true, "updatedAt": "2026-09-15T02:00:00Z",
			},
		},
	}
	payload, err := json.Marshal(profileEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProcessJetstreamEvent(context.Background(), payload); err != nil {
		t.Fatalf("profile ingestion: %v", err)
	}

	linkEvent := map[string]any{
		"did": "did:plc:turso-appview", "kind": "commit", "cursor": 9002,
		"commit": map[string]any{
			"operation": "create", "collection": atproto.LinkCollection, "rkey": "project", "cid": "cid-link",
			"record": map[string]any{
				"$type": atproto.LinkCollection, "label": "Project", "url": "https://example.com/project", "kind": "project",
				"featured": true, "position": 0, "updatedAt": "2026-09-15T02:00:01Z",
			},
		},
	}
	payload, err = json.Marshal(linkEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProcessJetstreamEvent(context.Background(), payload); err != nil {
		t.Fatalf("link ingestion: %v", err)
	}

	profile, err := store.IndexedProfile(context.Background(), "did:plc:turso-appview")
	if err != nil {
		t.Fatalf("IndexedProfile: %v", err)
	}
	if profile.DisplayName != "Turso Portable" || profile.Verified || len(profile.Links) != 1 || profile.Links[0].Label != "Project" {
		t.Fatalf("unexpected portable profile: %+v", profile)
	}

	results, err := store.SearchIndexed(context.Background(), "Turso Portable", 10)
	if err != nil {
		t.Fatalf("SearchIndexed: %v", err)
	}
	if len(results) != 1 || results[0].DID != "did:plc:turso-appview" {
		t.Fatalf("unexpected AppView search results: %+v", results)
	}
}
