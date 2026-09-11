package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chrisbirster/vutame/internal/httpapi"
	"github.com/chrisbirster/vutame/internal/profile"
	webapp "github.com/chrisbirster/vutame/internal/web"
)

func main() {
	port := envString("PORT", "8080")
	profiles, closeProfiles, backend, err := openProfileStore()
	if err != nil {
		slog.Error("open profile store", "error", err)
		os.Exit(1)
	}
	defer closeProfiles()

	server := &http.Server{
		Addr: ":" + port,
		Handler: httpapi.New(webapp.Handler(), profiles, httpapi.Options{
			MarketingOrigin: envString("VUTAME_MARKETING_ORIGIN", "https://vutame.com"),
			ProfileOrigin:   envString("VUTAME_PROFILE_ORIGIN", "https://vuta.me"),
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

	slog.Info("vutame listening", "addr", server.Addr, "profile_store", backend)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func openProfileStore() (profile.Store, func(), string, error) {
	dsn := strings.TrimSpace(os.Getenv("VUTAME_DATABASE_DSN"))
	if dsn == "" {
		// Transitional M1 behavior: preserve the M0 demo when no database is
		// configured. M1 will make durable storage mandatory before completion.
		return profile.NewSeedStore(), func() {}, "memory", nil
	}
	store, err := profile.OpenSQLite(dsn)
	if err != nil {
		return nil, func() {}, "sqlite", fmt.Errorf("open VUTAME_DATABASE_DSN: %w", err)
	}
	return store, func() { _ = store.Close() }, "sqlite", nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
