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

	"github.com/chrisbirster/vutame/internal/activity"
	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/auth"
	datastore "github.com/chrisbirster/vutame/internal/database"
	"github.com/chrisbirster/vutame/internal/httpapi"
	"github.com/chrisbirster/vutame/internal/media"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/safety"
	"github.com/chrisbirster/vutame/internal/social"
	webapp "github.com/chrisbirster/vutame/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := envString("PORT", "8080")
	cookieSecure := envString("VUTAME_COOKIE_SECURE", "1") != "0"

	databaseRuntime, err := openDatabaseRuntime(ctx)
	if err != nil {
		slog.Error("open database runtime", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := databaseRuntime.Close(); err != nil {
			slog.Warn("close database runtime", "error", err)
		}
	}()

	profiles, editor, err := openProfileStore(databaseRuntime)
	if err != nil {
		slog.Error("open profile store", "error", err)
		os.Exit(1)
	}
	authService, err := openAuthService(databaseRuntime, cookieSecure)
	if err != nil {
		slog.Error("open auth service", "error", err)
		os.Exit(1)
	}
	mediaService, err := openMediaService(databaseRuntime)
	if err != nil {
		slog.Error("open media service", "error", err)
		os.Exit(1)
	}
	analyticsService, err := openAnalyticsService(databaseRuntime)
	if err != nil {
		slog.Error("open analytics service", "error", err)
		os.Exit(1)
	}
	safetyStore, err := openSafetyStore(databaseRuntime)
	if err != nil {
		slog.Error("open safety store", "error", err)
		os.Exit(1)
	}
	socialStore, err := openSocialStore(databaseRuntime)
	if err != nil {
		slog.Error("open social graph store", "error", err)
		os.Exit(1)
	}
	activityStore, err := openActivityStore(databaseRuntime)
	if err != nil {
		slog.Error("open activity store", "error", err)
		os.Exit(1)
	}
	editor = activity.NewRecordingEditor(editor, activityStore)

	server := &http.Server{
		Addr: ":" + port,
		Handler: httpapi.New(webapp.Handler(), profiles, httpapi.Options{
			MarketingOrigin: envString("VUTAME_MARKETING_ORIGIN", "https://vutame.com"),
			ProfileOrigin:   envString("VUTAME_PROFILE_ORIGIN", "https://vuta.me"),
			Auth:            authService,
			Editor:          editor,
			Media:           mediaService,
			Social:          socialStore,
			Activity:        activityStore,
			Analytics:       analyticsService,
			Safety:          safetyStore,
			CookieSecure:    cookieSecure,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info(
		"vutame listening",
		"addr", server.Addr,
		"database_backend", databaseRuntime.Backend,
		"auth_enabled", authService != nil,
		"editor_enabled", editor != nil,
		"media_enabled", mediaService != nil,
		"social_enabled", socialStore != nil,
		"activity_enabled", activityStore != nil,
		"analytics_enabled", analyticsService != nil,
		"safety_enabled", safetyStore != nil,
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func openDatabaseRuntime(ctx context.Context) (*datastore.Runtime, error) {
	config, err := databaseConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return datastore.Open(ctx, config)
}

func databaseConfigFromEnv() (datastore.Config, error) {
	environment := strings.ToLower(envString("VUTAME_ENV", "development"))
	requirePersistent := environment == "production" || envString("VUTAME_REQUIRE_DATABASE", "0") == "1"
	syncInterval, err := time.ParseDuration(envString("VUTAME_TURSO_SYNC_INTERVAL", "5s"))
	if err != nil || syncInterval <= 0 {
		return datastore.Config{}, errors.New("VUTAME_TURSO_SYNC_INTERVAL must be a positive duration such as 5s")
	}
	return datastore.Config{
		SQLiteDSN:         strings.TrimSpace(os.Getenv("VUTAME_DATABASE_DSN")),
		RequirePersistent: requirePersistent,
		TursoRemoteURL:    strings.TrimSpace(os.Getenv("VUTAME_TURSO_REMOTE_URL")),
		TursoAuthToken:    strings.TrimSpace(os.Getenv("VUTAME_TURSO_AUTH_TOKEN")),
		TursoLocalPath:    strings.TrimSpace(os.Getenv("VUTAME_TURSO_LOCAL_PATH")),
		SyncInterval:      syncInterval,
	}, nil
}

func openProfileStore(runtime *datastore.Runtime) (profile.Store, profile.Editor, error) {
	if runtime == nil || runtime.DB == nil {
		return profile.NewSeedStore(), nil, nil
	}
	store, err := profile.NewSQLiteStore(runtime.DB)
	if err != nil {
		return nil, nil, err
	}
	return store, store, nil
}

func openSocialStore(runtime *datastore.Runtime) (social.Store, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	base, err := social.NewSQLiteStore(runtime.DB)
	if err != nil {
		return nil, err
	}
	return social.NewGuardedStore(base, runtime.DB)
}

func openActivityStore(runtime *datastore.Runtime) (activity.Store, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	base, err := activity.NewSQLiteStore(runtime.DB)
	if err != nil {
		return nil, err
	}
	return activity.NewGuardedStore(base, runtime.DB)
}

func openAnalyticsService(runtime *datastore.Runtime) (*analytics.Service, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	return analytics.NewService(runtime.DB)
}

func openSafetyStore(runtime *datastore.Runtime) (safety.Store, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	return safety.NewSQLiteStore(runtime.DB)
}

func openAuthService(runtime *datastore.Runtime, cookieSecure bool) (*auth.Service, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	secret := []byte(strings.TrimSpace(os.Getenv("VUTAME_AUTH_SECRET")))
	if len(secret) < 32 {
		return nil, errors.New("VUTAME_AUTH_SECRET must be at least 32 bytes when persistent storage is configured")
	}
	store, err := auth.NewSQLiteStore(runtime.DB)
	if err != nil {
		return nil, err
	}
	sender, err := openAuthSender(cookieSecure)
	if err != nil {
		return nil, err
	}
	service, err := auth.NewService(store, sender, secret, auth.Config{})
	if err != nil {
		return nil, err
	}
	return service, nil
}

func openMediaService(runtime *datastore.Runtime) (*media.Service, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, nil
	}
	root := strings.TrimSpace(os.Getenv("VUTAME_MEDIA_DIR"))
	if root == "" {
		return nil, nil
	}
	blobs, err := media.NewFileStore(root)
	if err != nil {
		return nil, err
	}
	service, err := media.NewService(runtime.DB, blobs)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func openAuthSender(cookieSecure bool) (auth.Sender, error) {
	logCodes := envString("VUTAME_AUTH_LOG_CODES", "0") == "1"
	smtpAddress := strings.TrimSpace(os.Getenv("VUTAME_SMTP_ADDR"))
	smtpUsername := strings.TrimSpace(os.Getenv("VUTAME_SMTP_USERNAME"))
	smtpPassword := os.Getenv("VUTAME_SMTP_PASSWORD")
	from := strings.TrimSpace(os.Getenv("VUTAME_AUTH_EMAIL_FROM"))
	hasSMTPConfig := smtpAddress != "" || smtpUsername != "" || smtpPassword != "" || from != ""

	if logCodes {
		if cookieSecure {
			return nil, errors.New("VUTAME_AUTH_LOG_CODES requires VUTAME_COOKIE_SECURE=0 and is development-only")
		}
		if hasSMTPConfig {
			return nil, errors.New("VUTAME_AUTH_LOG_CODES cannot be combined with SMTP configuration")
		}
		slog.Warn("development auth code logging is enabled")
		return auth.LogSender{}, nil
	}
	if !hasSMTPConfig {
		return auth.UnavailableSender{}, nil
	}
	if smtpAddress == "" {
		return nil, errors.New("VUTAME_SMTP_ADDR is required when SMTP email settings are configured")
	}
	sender, err := auth.NewSMTPSender(auth.SMTPConfig{
		Address:  smtpAddress,
		Username: smtpUsername,
		Password: smtpPassword,
		From:     from,
	})
	if err != nil {
		return nil, fmt.Errorf("configure auth SMTP sender: %w", err)
	}
	return sender, nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
