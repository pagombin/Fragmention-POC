package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MetricScope identifies what a sample describes (cluster, database,
// collection, member, index). Kept as a narrow string alias so we can use
// database enums later if we ever need to.
type MetricScope string

// Canonical scope names stored in metrics_samples.scope.
const (
	ScopeCluster    MetricScope = "cluster"
	ScopeDatabase   MetricScope = "database"
	ScopeCollection MetricScope = "collection"
	ScopeIndex      MetricScope = "index"
	ScopeMember     MetricScope = "member"
)

// Sample is one time-series data point.
type Sample struct {
	RunID      *string     `json:"run_id,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
	Scope      MetricScope `json:"scope"`
	ScopeID    string      `json:"scope_id"`
	MetricName string      `json:"metric_name"`
	Value      float64     `json:"value"`
}

// Samples is the repository for metrics_samples.
type Samples struct{ db *sql.DB }

// NewSamples constructs the repository.
func NewSamples(s *Store) *Samples { return &Samples{db: s.db} }

// WriteBatch inserts many samples in one transaction. Intended for collector
// ticks where 100+ samples may be produced per tick. Returns the number of
// rows inserted.
func (r *Samples) WriteBatch(ctx context.Context, samples []Sample) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics_samples (run_id, timestamp, scope, scope_id, metric_name, metric_value)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	count := 0
	for _, s := range samples {
		if s.Timestamp.IsZero() {
			s.Timestamp = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx,
			nilString(s.RunID),
			FormatTime(s.Timestamp),
			string(s.Scope),
			s.ScopeID,
			s.MetricName,
			s.Value,
		); err != nil {
			_ = tx.Rollback()
			return count, fmt.Errorf("insert sample: %w", err)
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return count, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}

// Query returns samples matching the supplied filter, ordered by timestamp
// ascending. All filter fields are optional; passing a zero-value struct
// returns every row (up to limit).
type Query struct {
	RunID      string
	Scope      MetricScope
	ScopeID    string
	MetricName string
	Since      time.Time
	Until      time.Time
	Limit      int
}

// Query runs a range-select against metrics_samples and returns samples.
func (r *Samples) Query(ctx context.Context, q Query) ([]Sample, error) {
	stmt := `SELECT run_id, timestamp, scope, scope_id, metric_name, metric_value
	        FROM metrics_samples WHERE 1=1`
	var args []any
	if q.RunID != "" {
		stmt += " AND run_id = ?"
		args = append(args, q.RunID)
	}
	if q.Scope != "" {
		stmt += " AND scope = ?"
		args = append(args, string(q.Scope))
	}
	if q.ScopeID != "" {
		stmt += " AND scope_id = ?"
		args = append(args, q.ScopeID)
	}
	if q.MetricName != "" {
		stmt += " AND metric_name = ?"
		args = append(args, q.MetricName)
	}
	if !q.Since.IsZero() {
		stmt += " AND timestamp >= ?"
		args = append(args, FormatTime(q.Since))
	}
	if !q.Until.IsZero() {
		stmt += " AND timestamp <= ?"
		args = append(args, FormatTime(q.Until))
	}
	stmt += " ORDER BY timestamp ASC"
	if q.Limit > 0 {
		stmt += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	rows, err := r.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("query samples: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Sample
	for rows.Next() {
		var (
			s      Sample
			runSQL sql.NullString
			ts     string
		)
		if err := rows.Scan(&runSQL, &ts, &s.Scope, &s.ScopeID, &s.MetricName, &s.Value); err != nil {
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		if runSQL.Valid {
			v := runSQL.String
			s.RunID = &v
		}
		t, err := ParseTime(ts)
		if err != nil {
			return nil, fmt.Errorf("parse ts: %w", err)
		}
		s.Timestamp = t
		out = append(out, s)
	}
	return out, rows.Err()
}

// Snapshot represents a point-in-time reading labelled for humans.
type Snapshot struct {
	ID        string          `json:"id"`
	RunID     *string         `json:"run_id,omitempty"`
	Label     string          `json:"label"`
	TakenAt   time.Time       `json:"taken_at"`
	Scope     MetricScope     `json:"scope"`
	ScopeID   string          `json:"scope_id"`
	RawStats  json.RawMessage `json:"raw_stats"`
	Note      string          `json:"note,omitempty"`
}

// Snapshots is the repository for the snapshots table.
type Snapshots struct{ db *sql.DB }

// NewSnapshots constructs the repository.
func NewSnapshots(s *Store) *Snapshots { return &Snapshots{db: s.db} }

// Record persists a new snapshot. Assigns an ID if empty.
func (r *Snapshots) Record(ctx context.Context, s Snapshot) (string, error) {
	if s.TakenAt.IsZero() {
		s.TakenAt = time.Now().UTC()
	}
	if s.Label == "" {
		return "", fmt.Errorf("snapshot label required")
	}
	if s.ID == "" {
		return "", fmt.Errorf("snapshot id required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO snapshots (id, run_id, label, taken_at, scope, scope_id, raw_stats_json, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		s.ID, nilString(s.RunID), s.Label, FormatTime(s.TakenAt),
		string(s.Scope), s.ScopeID, string(s.RawStats), nullable(s.Note),
	)
	if err != nil {
		return "", fmt.Errorf("insert snapshot: %w", err)
	}
	return s.ID, nil
}

// Get returns a single snapshot by id.
func (r *Snapshots) Get(ctx context.Context, id string) (*Snapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, run_id, label, taken_at, scope, scope_id, raw_stats_json, COALESCE(note, '')
		FROM snapshots WHERE id = ?
	`, id)
	var (
		snap   Snapshot
		runSQL sql.NullString
		ts     string
		raw    string
	)
	if err := row.Scan(&snap.ID, &runSQL, &snap.Label, &ts, &snap.Scope, &snap.ScopeID, &raw, &snap.Note); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan snapshot: %w", err)
	}
	if runSQL.Valid {
		v := runSQL.String
		snap.RunID = &v
	}
	t, err := ParseTime(ts)
	if err != nil {
		return nil, err
	}
	snap.TakenAt = t
	snap.RawStats = json.RawMessage(raw)
	return &snap, nil
}

// List returns the most-recent `limit` snapshots, filtered by label when set.
func (r *Snapshots) List(ctx context.Context, label string, limit int) ([]Snapshot, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, run_id, label, taken_at, scope, scope_id, raw_stats_json, COALESCE(note, '')
	      FROM snapshots WHERE 1=1`
	args := []any{}
	if label != "" {
		q += " AND label = ?"
		args = append(args, label)
	}
	q += " ORDER BY taken_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Snapshot
	for rows.Next() {
		var (
			s      Snapshot
			runSQL sql.NullString
			ts     string
			raw    string
		)
		if err := rows.Scan(&s.ID, &runSQL, &s.Label, &ts, &s.Scope, &s.ScopeID, &raw, &s.Note); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if runSQL.Valid {
			v := runSQL.String
			s.RunID = &v
		}
		t, err := ParseTime(ts)
		if err != nil {
			return nil, err
		}
		s.TakenAt = t
		s.RawStats = json.RawMessage(raw)
		out = append(out, s)
	}
	return out, rows.Err()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
