package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Run represents a structured experiment row.
type Run struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	CancelledAt *time.Time      `json:"cancelled_at,omitempty"`
	Notes       string          `json:"notes,omitempty"`
}

// Runs is the repository for the runs table.
type Runs struct{ db *sql.DB }

// NewRuns constructs the repository.
func NewRuns(s *Store) *Runs { return &Runs{db: s.db} }

// Create inserts a new run.
func (r *Runs) Create(ctx context.Context, run Run) error {
	if run.ID == "" {
		return errors.New("runs.Create: id required")
	}
	if run.Status == "" {
		run.Status = "created"
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if len(run.Config) == 0 {
		run.Config = json.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO runs (id, name, config_json, status, created_at, started_at, completed_at, cancelled_at, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.ID, run.Name, string(run.Config), run.Status, FormatTime(run.CreatedAt),
		nilTime(run.StartedAt), nilTime(run.CompletedAt), nilTime(run.CancelledAt),
		nullable(run.Notes),
	)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

// UpdateStatus transitions the run status and optionally records a timestamp.
func (r *Runs) UpdateStatus(ctx context.Context, id, status string) error {
	now := FormatTime(time.Now().UTC())
	q := "UPDATE runs SET status = ?"
	args := []any{status}
	switch status {
	case "running":
		q += ", started_at = COALESCE(started_at, ?)"
		args = append(args, now)
	case "completed", "failed":
		q += ", completed_at = ?"
		args = append(args, now)
	case "cancelled":
		q += ", cancelled_at = ?"
		args = append(args, now)
	}
	q += " WHERE id = ?"
	args = append(args, id)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("run %s not found", id)
	}
	return nil
}

// Get returns one run by id.
func (r *Runs) Get(ctx context.Context, id string) (*Run, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, config_json, status, created_at, started_at, completed_at, cancelled_at, COALESCE(notes, '')
		FROM runs WHERE id = ?
	`, id)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// List returns the most-recent runs.
func (r *Runs) List(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, config_json, status, created_at, started_at, completed_at, cancelled_at, COALESCE(notes, '')
		FROM runs ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func scanRun(sc rowScanner) (*Run, error) {
	var (
		run    Run
		cfg    string
		ts     string
		startSQL, compSQL, cancSQL sql.NullString
	)
	if err := sc.Scan(&run.ID, &run.Name, &cfg, &run.Status, &ts, &startSQL, &compSQL, &cancSQL, &run.Notes); err != nil {
		return nil, err
	}
	run.Config = json.RawMessage(cfg)
	t, err := ParseTime(ts)
	if err != nil {
		return nil, err
	}
	run.CreatedAt = t
	for _, pair := range []struct {
		sq  sql.NullString
		dst **time.Time
	}{{startSQL, &run.StartedAt}, {compSQL, &run.CompletedAt}, {cancSQL, &run.CancelledAt}} {
		if !pair.sq.Valid {
			continue
		}
		t, err := ParseTime(pair.sq.String)
		if err != nil {
			return nil, err
		}
		tt := t
		*pair.dst = &tt
	}
	return &run, nil
}
