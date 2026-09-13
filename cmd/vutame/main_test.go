package main

import (
	"context"
	"strings"
	"testing"

	"github.com/chrisbirster/vutame/internal/auth"
)

func clearAuthSenderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"VUTAME_AUTH_LOG_CODES",
		"VUTAME_SMTP_ADDR",
		"VUTAME_SMTP_USERNAME",
		"VUTAME_SMTP_PASSWORD",
		"VUTAME_AUTH_EMAIL_FROM",
	} {
		t.Setenv(key, "")
	}
}

func clearDatabaseEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"VUTAME_ENV",
		"VUTAME_REQUIRE_DATABASE",
		"VUTAME_DATABASE_DSN",
		"VUTAME_TURSO_REMOTE_URL",
		"VUTAME_TURSO_AUTH_TOKEN",
		"VUTAME_TURSO_LOCAL_PATH",
		"VUTAME_TURSO_SYNC_INTERVAL",
	} {
		t.Setenv(key, "")
	}
}

func TestDatabaseConfigDevelopmentAllowsMemory(t *testing.T) {
	clearDatabaseEnv(t)
	config, err := databaseConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.RequirePersistent {
		t.Fatal("development config unexpectedly requires persistence")
	}
}

func TestDatabaseConfigProductionRejectsMemoryFallback(t *testing.T) {
	clearDatabaseEnv(t)
	t.Setenv("VUTAME_ENV", "production")
	_, err := openDatabaseRuntime(context.Background())
	if err == nil || !strings.Contains(err.Error(), "persistent storage is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestDatabaseConfigReadsTursoSettings(t *testing.T) {
	clearDatabaseEnv(t)
	t.Setenv("VUTAME_TURSO_REMOTE_URL", "https://vutame.example.turso.io")
	t.Setenv("VUTAME_TURSO_AUTH_TOKEN", "token")
	t.Setenv("VUTAME_TURSO_LOCAL_PATH", "/tmp/vutame.db")
	t.Setenv("VUTAME_TURSO_SYNC_INTERVAL", "2s")
	config, err := databaseConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.TursoRemoteURL != "https://vutame.example.turso.io" || config.TursoAuthToken != "token" || config.TursoLocalPath != "/tmp/vutame.db" {
		t.Fatalf("config = %#v", config)
	}
	if config.SyncInterval.String() != "2s" {
		t.Fatalf("sync interval = %s", config.SyncInterval)
	}
}

func TestDatabaseConfigRejectsInvalidSyncInterval(t *testing.T) {
	clearDatabaseEnv(t)
	t.Setenv("VUTAME_TURSO_SYNC_INTERVAL", "nope")
	if _, err := databaseConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "positive duration") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenAuthSenderDefaultsUnavailable(t *testing.T) {
	clearAuthSenderEnv(t)
	sender, err := openAuthSender(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(auth.UnavailableSender); !ok {
		t.Fatalf("sender = %T, want auth.UnavailableSender", sender)
	}
}

func TestOpenAuthSenderDevelopmentLoggerRequiresInsecureLocalCookie(t *testing.T) {
	clearAuthSenderEnv(t)
	t.Setenv("VUTAME_AUTH_LOG_CODES", "1")
	if _, err := openAuthSender(true); err == nil || !strings.Contains(err.Error(), "development-only") {
		t.Fatalf("secure-cookie log sender err = %v", err)
	}
	sender, err := openAuthSender(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(auth.LogSender); !ok {
		t.Fatalf("sender = %T, want auth.LogSender", sender)
	}
}

func TestOpenAuthSenderRejectsMixedOrPartialSMTPConfiguration(t *testing.T) {
	clearAuthSenderEnv(t)
	t.Setenv("VUTAME_AUTH_LOG_CODES", "1")
	t.Setenv("VUTAME_SMTP_ADDR", "smtp.example.com:587")
	if _, err := openAuthSender(false); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("mixed sender err = %v", err)
	}

	clearAuthSenderEnv(t)
	t.Setenv("VUTAME_AUTH_EMAIL_FROM", "login@example.com")
	if _, err := openAuthSender(true); err == nil || !strings.Contains(err.Error(), "VUTAME_SMTP_ADDR") {
		t.Fatalf("partial SMTP err = %v", err)
	}
}

func TestOpenAuthSenderBuildsSMTP(t *testing.T) {
	clearAuthSenderEnv(t)
	t.Setenv("VUTAME_SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("VUTAME_SMTP_USERNAME", "smtp-user")
	t.Setenv("VUTAME_SMTP_PASSWORD", "smtp-password")
	t.Setenv("VUTAME_AUTH_EMAIL_FROM", "Vutame <login@example.com>")
	sender, err := openAuthSender(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(*auth.SMTPSender); !ok {
		t.Fatalf("sender = %T, want *auth.SMTPSender", sender)
	}
}
