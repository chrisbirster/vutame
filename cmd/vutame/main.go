package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/httpapi"
	"github.com/chrisbirster/vutame/internal/profile"
	webapp "github.com/chrisbirster/vutame/internal/web"
)

func main() {
	port := envString("PORT", "8080")
	dsn := strings.TrimSpace(os.Getenv("VUTAME_DATABASE_DSN"))
	profiles, editor, closeProfiles, backend, err := openProfileStore(dsn)
	if err != nil {
		slog.Error("open profile store", "error", err)
		os.Exit(1)
	}
	defer closeProfiles()

	authService, closeAuth, err := openAuthService(dsn)
	if err != nil {
		slog.Error("open auth service", "error", err)
		os.Exit(1)
	}
	defer closeAuth()

	server := &http.Server{
		Addr: ":" + port,
		Handler: httpapi.New(webapp.Handler(), profiles, httpapi.Options{
			MarketingOrigin: envString("VUTAME_MARKETING_ORIGIN", "https://vutame.com"),
			ProfileOrigin:   envString("VUTAME_PROFILE_ORIGIN", "https://vuta.me"),
			Auth:            authService,
			Editor:          editor,
			CookieSecure:    envString("VUTAME_COOKIE_SECURE", "1") != "0",
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("vutame listening", "addr", server.Addr, "profile_store", backend, "auth_enabled", authService != nil, "editor_enabled", editor != nil)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func openProfileStore(dsn string) (profile.Store, profile.Editor, func(), string, error) {
	if dsn == "" {
		return profile.NewSeedStore(), nil, func() {}, "memory", nil
	}
	store, err := profile.OpenSQLite(dsn)
	if err != nil {
		return nil, nil, func() {}, "sqlite", fmt.Errorf("open VUTAME_DATABASE_DSN: %w", err)
	}
	return store, store, func() { _ = store.Close() }, "sqlite", nil
}

func openAuthService(dsn string) (*auth.Service, func(), error) {
	if dsn == "" {
		return nil, func() {}, nil
	}
	secret := []byte(strings.TrimSpace(os.Getenv("VUTAME_AUTH_SECRET")))
	if len(secret) < 32 {
		return nil, func() {}, errors.New("VUTAME_AUTH_SECRET must be at least 32 bytes when VUTAME_DATABASE_DSN is configured")
	}
	store, err := auth.OpenSQLite(dsn)
	if err != nil {
		return nil, func() {}, err
	}
	var sender auth.Sender = auth.UnavailableSender{}
	if envString("VUTAME_AUTH_LOG_CODES", "0") == "1" {
		sender = auth.LogSender{}
		slog.Warn("development auth code logging is enabled")
	}
	service, err := auth.NewService(store, sender, secret, auth.Config{})
	if err != nil {
		_ = store.Close()
		return nil, func() {}, err
	}
	return service, func() { _ = store.Close() }, nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
