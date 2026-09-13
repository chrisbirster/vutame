package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestOpenAllowsMemoryForDevelopment(t *testing.T) {
	runtime, err := Open(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.Backend != "memory" || runtime.DB != nil {
		t.Fatalf("runtime = %#v, want memory backend without DB", runtime)
	}
}

func TestOpenRequiresPersistentStorageWhenConfigured(t *testing.T) {
	_, err := Open(context.Background(), Config{RequirePersistent: true})
	if err == nil || !strings.Contains(err.Error(), "persistent storage is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenRejectsMixedBackends(t *testing.T) {
	_, err := Open(context.Background(), Config{
		SQLiteDSN:      "file:test.db",
		TursoRemoteURL: "https://example.turso.io",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenRejectsPartialTursoConfiguration(t *testing.T) {
	_, err := Open(context.Background(), Config{
		TursoRemoteURL: "https://example.turso.io",
		TursoLocalPath: "replica.db",
	})
	if err == nil || !strings.Contains(err.Error(), "VUTAME_TURSO_AUTH_TOKEN") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenLocalSQLite(t *testing.T) {
	runtime, err := Open(context.Background(), Config{SQLiteDSN: "file:runtime-test?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.Backend != "sqlite" || runtime.DB == nil {
		t.Fatalf("backend = %q DB nil = %v", runtime.Backend, runtime.DB == nil)
	}
	var foreignKeys int
	if err := runtime.DB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestTursoDriverWorksWithoutCGOSpecificApplicationCode(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE smoke (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO smoke (id, value) VALUES (?, ?)`, 1, "ok"); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM smoke WHERE id = ?`, 1).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "ok" {
		t.Fatalf("value = %q, want ok", value)
	}
}
