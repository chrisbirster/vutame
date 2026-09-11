package main

import (
	"context"
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
	profiles := profile.NewSeedStore()

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

	slog.Info("vutame listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
