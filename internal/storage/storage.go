// Package storage wraps the application's SQLite state store. It owns the
// DB handle, applies goose-managed migrations, and exposes repository types
// that encapsulate query construction. All timestamps are stored as RFC3339
// UTC strings for portability and debuggability.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"

	// modernc.org/sqlite registers the "sqlite" driver; pure-Go, no CGO.
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Config holds the minimal parameters for opening the state store.
type Config struct {
	Path        string
	BusyTimeout time.Duration
}

// Store is the application's SQLite state store.
type Store struct {
	db *sql.DB
}

// Open connects to (or creates) the SQLite database at cfg.Path, applies any
// pending migrations, and configures WAL mode with a bounded page cache.
// The directory containing the file is created if missing.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Path == "" {
		return nil, errors.New("storage: path required")
	}
	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = 5 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(cfg.Path), err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)",
		cfg.Path, cfg.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open sqlite: %w", err)
	}
	// Single writer + a handful of readers is the recommended pattern for
	// modernc/sqlite under WAL. We serialize writes by limiting maxOpenConns
	// at the call-site for write paths where necessary.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-20000", // ~20MB
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying DB. Safe to call multiple times.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the raw *sql.DB for repositories in this package.
// Keep external callers away from this method; prefer typed repositories.
func (s *Store) DB() *sql.DB { return s.db }

// Ping verifies the connection is alive. Used by /ready.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("storage: not open")
	}
	return s.db.PingContext(ctx)
}

// Vacuum runs VACUUM; intended for the nightly janitor. Must not be called
// inside a transaction.
func (s *Store) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

func runMigrations(db *sql.DB) error {
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(embeddedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("migrations sub: %w", err)
	}
	goose.SetBaseFS(sub.(fs.ReadDirFS))
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// FormatTime normalizes timestamps to the on-disk format.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseTime reverses FormatTime.
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}
