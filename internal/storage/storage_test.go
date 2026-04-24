package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), Config{Path: filepath.Join(dir, "t.db"), BusyTimeout: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen_RunsMigrations(t *testing.T) {
	s := openTempStore(t)
	require.NoError(t, s.Ping(context.Background()))

	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		tables = append(tables, n)
	}
	require.Contains(t, tables, "runs")
	require.Contains(t, tables, "snapshots")
	require.Contains(t, tables, "metrics_samples")
	require.Contains(t, tables, "operations")
	require.Contains(t, tables, "operation_progress")
	require.Contains(t, tables, "delete_candidates")
	require.Contains(t, tables, "events")
}

func TestEvents_Roundtrip(t *testing.T) {
	s := openTempStore(t)
	repo := NewEvents(s)

	// Insert a run row to satisfy the FK constraint for this roundtrip test.
	runID := "run-123"
	_, err := s.db.Exec(`INSERT INTO runs (id, name, config_json, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		runID, "unit", "{}", "created", FormatTime(time.Now()))
	require.NoError(t, err)

	id, err := repo.Record(context.Background(), Event{
		RunID:    &runID,
		Level:    EventLevelAudit,
		Category: "auth",
		Message:  "login",
		Context:  map[string]any{"user": "alice"},
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	list, err := repo.List(context.Background(), "auth", runID, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "login", list[0].Message)
	require.Equal(t, EventLevelAudit, list[0].Level)
	require.Equal(t, "alice", list[0].Context["user"])
	require.NotNil(t, list[0].RunID)
	require.Equal(t, runID, *list[0].RunID)
}

func TestEvents_UnassociatedEvent(t *testing.T) {
	s := openTempStore(t)
	repo := NewEvents(s)
	id, err := repo.Record(context.Background(), Event{
		Level:    EventLevelInfo,
		Category: "system",
		Message:  "boot",
	})
	require.NoError(t, err)
	require.NotZero(t, id)
}

func TestEvents_RejectsEmptyCategory(t *testing.T) {
	s := openTempStore(t)
	repo := NewEvents(s)
	_, err := repo.Record(context.Background(), Event{Message: "x"})
	require.Error(t, err)
}

func TestVacuum(t *testing.T) {
	s := openTempStore(t)
	require.NoError(t, s.Vacuum(context.Background()))
}

func TestFormatParseTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	s := FormatTime(now)
	got, err := ParseTime(s)
	require.NoError(t, err)
	require.True(t, now.Equal(got))
}
