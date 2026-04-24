// Package logging configures the application's zerolog-based JSON logger.
//
// All log records are emitted as single-line JSON and carry a small set of
// structural fields by convention: service, run_id, request_id. Callers derive
// per-component loggers via With() rather than constructing their own.
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// Format selects the serialization format of the logger.
type Format string

const (
	// FormatJSON emits structured JSON - required for production and droplet.
	FormatJSON Format = "json"
	// FormatConsole emits human-readable colored output, useful for local dev.
	FormatConsole Format = "console"
)

// Config holds runtime logger configuration. Zero-value Config is valid.
type Config struct {
	Level  string // debug, info, warn, error; default info
	Format Format // json (default) or console
	Output io.Writer
}

// currentLevel supports the /admin/log-level endpoint without restart.
var currentLevel atomic.Int32

// Init configures the global zerolog defaults and returns a root logger.
// Callers must use the returned logger for request/run-scoped derivations;
// the global zerolog.Logger is also set for library code that lacks context.
func Init(cfg Config) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "message"

	lvl := parseLevel(cfg.Level)
	currentLevel.Store(int32(lvl))
	zerolog.SetGlobalLevel(lvl)

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	if cfg.Format == FormatConsole {
		out = zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	}

	logger := zerolog.New(out).With().Timestamp().Logger()
	zerolog.DefaultContextLogger = &logger
	return logger
}

// SetLevel updates the global log level at runtime. Returns the previous level
// as a string so callers can audit the transition.
func SetLevel(name string) (previous string, err error) {
	lvl, perr := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(name)))
	if perr != nil {
		return "", fmt.Errorf("parse level %q: %w", name, perr)
	}
	prevInt := currentLevel.Swap(int32(lvl))
	zerolog.SetGlobalLevel(lvl)
	return levelString(prevInt), nil
}

// Level returns the currently active level as a lowercase string.
func Level() string {
	return levelString(currentLevel.Load())
}

// levelString is a safe converter that avoids the int32->int8 narrowing
// flagged by gosec G115. zerolog levels fit trivially in int8 so the
// conversion is always defined, but we bound-check to keep static analysis
// (and future maintainers) happy.
func levelString(v int32) string {
	if v < -128 || v > 127 {
		return "unknown"
	}
	return zerolog.Level(int8(v)).String()
}

func parseLevel(s string) zerolog.Level {
	if s == "" {
		return zerolog.InfoLevel
	}
	if lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(s))); err == nil {
		return lvl
	}
	return zerolog.InfoLevel
}
