package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/social"
	_ "turso.tech/database/tursogo"
)

func TestDiscoveryPaginationRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatal(err)
	}
	store, err := social.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	insertTursoCreator(t, db, "usr_page_a", "page-a", "Page A", "pagination")
	insertTursoCreator(t, db, "usr_page_b", "page-b", "Page B", "pagination")
	insertTursoCreator(t, db, "usr_page_c", "page-c", "Page C", "pagination")

	first, err := store.SearchPage(context.Background(), social.SearchInput{Query: "pagination", Limit: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Creators) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second, err := store.SearchPage(context.Background(), social.SearchInput{Query: "pagination", Limit: 2, Cursor: first.NextCursor}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Creators) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
}
