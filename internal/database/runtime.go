package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	turso "turso.tech/database/tursogo"
)

const defaultSyncInterval = 5 * time.Second

type Config struct {
	SQLiteDSN         string
	RequirePersistent bool
	TursoRemoteURL    string
	TursoAuthToken    string
	TursoLocalPath    string
	SyncInterval      time.Duration
}

type Runtime struct {
	DB      *sql.DB
	Backend string

	syncDB       *turso.TursoSyncDb
	syncInterval time.Duration
	cancel       context.CancelFunc
	done         chan struct{}
	closeOnce    sync.Once
}

func Open(ctx context.Context, config Config) (*Runtime, error) {
	config.SQLiteDSN = strings.TrimSpace(config.SQLiteDSN)
	config.TursoRemoteURL = strings.TrimSpace(config.TursoRemoteURL)
	config.TursoAuthToken = strings.TrimSpace(config.TursoAuthToken)
	config.TursoLocalPath = strings.TrimSpace(config.TursoLocalPath)
	if config.SyncInterval <= 0 {
		config.SyncInterval = defaultSyncInterval
	}

	hasSQLite := config.SQLiteDSN != ""
	hasTurso := config.TursoRemoteURL != "" || config.TursoAuthToken != "" || config.TursoLocalPath != ""
	if hasSQLite && hasTurso {
		return nil, errors.New("database: VUTAME_DATABASE_DSN cannot be combined with Turso sync settings")
	}
	if !hasSQLite && !hasTurso {
		if config.RequirePersistent {
			return nil, errors.New("database: persistent storage is required in hosted mode")
		}
		return &Runtime{Backend: "memory"}, nil
	}
	if hasSQLite {
		return openSQLite(config.SQLiteDSN)
	}
	return openTurso(ctx, config)
}

func openSQLite(dsn string) (*Runtime, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := initializeSQLDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Runtime{DB: db, Backend: "sqlite"}, nil
}

func openTurso(ctx context.Context, config Config) (*Runtime, error) {
	if config.TursoRemoteURL == "" {
		return nil, errors.New("database: VUTAME_TURSO_REMOTE_URL is required when Turso sync is configured")
	}
	if config.TursoAuthToken == "" {
		return nil, errors.New("database: VUTAME_TURSO_AUTH_TOKEN is required when Turso sync is configured")
	}
	if config.TursoLocalPath == "" {
		return nil, errors.New("database: VUTAME_TURSO_LOCAL_PATH is required when Turso sync is configured")
	}

	syncDB, err := turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
		Path:            config.TursoLocalPath,
		RemoteUrl:       config.TursoRemoteURL,
		AuthToken:       config.TursoAuthToken,
		ClientName:      "vutame",
		LongPollTimeoutMs: 1000,
		BusyTimeout:     5000,
	})
	if err != nil {
		return nil, fmt.Errorf("database: open Turso sync database: %w", err)
	}
	db, err := syncDB.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("database: connect Turso SQL driver: %w", err)
	}
	// Turso sync operations and SQL statements share the same underlying local
	// replica. A single pooled SQL connection keeps the MVP's write ordering
	// deterministic while the sync layer serializes push/pull operations.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := initializeSQLDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	runtime := &Runtime{
		DB:           db,
		Backend:      "turso-sync",
		syncDB:       syncDB,
		syncInterval: config.SyncInterval,
	}
	runtime.startSync(ctx)
	return runtime, nil
}

func initializeSQLDB(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("database: enable foreign keys: %w", err)
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database: ping: %w", err)
	}
	return nil
}

func (r *Runtime) startSync(parent context.Context) {
	if r.syncDB == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(r.syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := r.Push(flushCtx); err != nil {
					slog.Warn("final Turso push failed", "error", err)
				}
				flushCancel()
				return
			case <-ticker.C:
				syncCtx, syncCancel := context.WithTimeout(ctx, 5*time.Second)
				if err := r.Sync(syncCtx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("Turso background sync failed", "error", err)
				}
				syncCancel()
			}
		}
	}()
}

func (r *Runtime) Push(ctx context.Context) error {
	if r == nil || r.syncDB == nil {
		return nil
	}
	if err := r.syncDB.Push(ctx); err != nil {
		return fmt.Errorf("database: push Turso changes: %w", err)
	}
	return nil
}

func (r *Runtime) Sync(ctx context.Context) error {
	if r == nil || r.syncDB == nil {
		return nil
	}
	if err := r.Push(ctx); err != nil {
		return err
	}
	if _, err := r.syncDB.Pull(ctx); err != nil {
		return fmt.Errorf("database: pull Turso changes: %w", err)
	}
	return nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	r.closeOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		if r.done != nil {
			<-r.done
		}
		if r.DB != nil {
			closeErr = r.DB.Close()
		}
	})
	return closeErr
}
