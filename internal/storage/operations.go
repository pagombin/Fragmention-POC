package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// OperationKind classifies an operation record.
type OperationKind string

// Known operation kinds. The string values are persisted, so renaming any of
// these is a breaking change to the on-disk schema.
const (
	OpKindLoader   OperationKind = "loader"
	OpKindDeleter  OperationKind = "deleter"
	OpKindCompact  OperationKind = "compact"
	OpKindWorkload OperationKind = "workload"
)

// OperationState models the lifecycle: idle → running → (paused ↔ running)
// → stopping → stopped/completed/failed. Orphan rows detected at startup are
// transitioned to `interrupted` (spec § 21.2).
type OperationState string

// Known operation states.
const (
	StateIdle        OperationState = "idle"
	StateRunning     OperationState = "running"
	StatePaused      OperationState = "paused"
	StateStopping    OperationState = "stopping"
	StateStopped     OperationState = "stopped"
	StateCompleted   OperationState = "completed"
	StateFailed      OperationState = "failed"
	StateInterrupted OperationState = "interrupted"
)

// Operation is one persisted operation row.
type Operation struct {
	ID           string          `json:"id"`
	RunID        *string         `json:"run_id,omitempty"`
	Kind         OperationKind   `json:"kind"`
	Target       json.RawMessage `json:"target"`
	Params       json.RawMessage `json:"params"`
	State        OperationState  `json:"state"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	PausedAt     *time.Time      `json:"paused_at,omitempty"`
	ResumedAt    *time.Time      `json:"resumed_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	Stats        json.RawMessage `json:"stats,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

// Progress tracks per-collection progress within one operation.
type Progress struct {
	OperationID    string    `json:"operation_id"`
	CollectionKey  string    `json:"collection_key"` // "db.collection"
	TotalTarget    int64     `json:"total_target"`
	CompletedCount int64     `json:"completed_count"`
	BytesProcessed int64     `json:"bytes_processed"`
	LastUpdated    time.Time `json:"last_updated"`
}

// Operations is the operations repository.
type Operations struct{ db *sql.DB }

// NewOperations constructs the repository.
func NewOperations(s *Store) *Operations { return &Operations{db: s.db} }

// Create inserts a new operation. The caller supplies the id (UUID).
func (r *Operations) Create(ctx context.Context, op Operation) error {
	if op.ID == "" {
		return errors.New("operations.Create: id required")
	}
	if op.Kind == "" {
		return errors.New("operations.Create: kind required")
	}
	if op.State == "" {
		op.State = StateIdle
	}
	if op.Target == nil {
		op.Target = json.RawMessage("{}")
	}
	if op.Params == nil {
		op.Params = json.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO operations (id, run_id, kind, target_json, params_json, state, started_at, paused_at, resumed_at, completed_at, stats_json, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		op.ID, nilString(op.RunID), string(op.Kind), string(op.Target), string(op.Params), string(op.State),
		nilTime(op.StartedAt), nilTime(op.PausedAt), nilTime(op.ResumedAt), nilTime(op.CompletedAt),
		nullable(string(op.Stats)), nullable(op.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("insert operation: %w", err)
	}
	return nil
}

// UpdateState transitions an operation to a new state, atomically updating
// the timestamp field appropriate for that transition. The error_message
// field is only written when entering StateFailed.
func (r *Operations) UpdateState(ctx context.Context, id string, state OperationState, errMsg string) error {
	now := time.Now().UTC()
	stmt, args := buildStateUpdate(id, state, now, errMsg)
	res, err := r.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("operation %s not found", id)
	}
	return nil
}

func buildStateUpdate(id string, state OperationState, now time.Time, errMsg string) (string, []any) {
	base := "UPDATE operations SET state = ?"
	args := []any{string(state)}
	switch state {
	case StateRunning:
		base += ", resumed_at = ?, paused_at = NULL"
		args = append(args, FormatTime(now))
	case StatePaused:
		base += ", paused_at = ?"
		args = append(args, FormatTime(now))
	case StateCompleted, StateStopped:
		base += ", completed_at = ?"
		args = append(args, FormatTime(now))
	case StateFailed:
		base += ", completed_at = ?, error_message = ?"
		args = append(args, FormatTime(now), errMsg)
	case StateInterrupted:
		base += ", error_message = ?"
		args = append(args, nullable(errMsg))
	}
	// Always record started_at if null.
	base += ", started_at = COALESCE(started_at, ?)"
	args = append(args, FormatTime(now))
	base += " WHERE id = ?"
	args = append(args, id)
	return base, args
}

// SetStats stores the final stats JSON payload on the operation.
func (r *Operations) SetStats(ctx context.Context, id string, stats json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `UPDATE operations SET stats_json = ? WHERE id = ?`, string(stats), id)
	if err != nil {
		return fmt.Errorf("set stats: %w", err)
	}
	return nil
}

// Get returns one operation by id.
func (r *Operations) Get(ctx context.Context, id string) (*Operation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, run_id, kind, target_json, params_json, state, started_at, paused_at, resumed_at, completed_at,
		       COALESCE(stats_json, ''), COALESCE(error_message, '')
		FROM operations WHERE id = ?
	`, id)
	op, err := scanOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return op, nil
}

// ListActive returns every operation in a non-terminal state.
func (r *Operations) ListActive(ctx context.Context) ([]Operation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, kind, target_json, params_json, state, started_at, paused_at, resumed_at, completed_at,
		       COALESCE(stats_json, ''), COALESCE(error_message, '')
		FROM operations
		WHERE state IN (?, ?, ?, ?)
		ORDER BY started_at DESC
	`, StateIdle, StateRunning, StatePaused, StateStopping)
	if err != nil {
		return nil, fmt.Errorf("list active: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

// ListByKind returns recent operations of a specific kind.
func (r *Operations) ListByKind(ctx context.Context, kind OperationKind, limit int) ([]Operation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, kind, target_json, params_json, state, started_at, paused_at, resumed_at, completed_at,
		       COALESCE(stats_json, ''), COALESCE(error_message, '')
		FROM operations
		WHERE kind = ?
		ORDER BY id DESC
		LIMIT ?
	`, string(kind), limit)
	if err != nil {
		return nil, fmt.Errorf("list by kind: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

// RecoverOrphans transitions every non-terminal row to StateInterrupted,
// returning the list of affected operations. Called once at startup per
// spec § 21.2.
func (r *Operations) RecoverOrphans(ctx context.Context, reason string) ([]Operation, error) {
	orphans, err := r.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, op := range orphans {
		if err := r.UpdateState(ctx, op.ID, StateInterrupted, reason); err != nil {
			return nil, err
		}
	}
	return orphans, nil
}

// rowScanner is the common interface for sql.Row and sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanOperation(sc rowScanner) (*Operation, error) {
	var (
		op       Operation
		runSQL   sql.NullString
		target   string
		params   string
		startSQL sql.NullString
		pauseSQL sql.NullString
		resSQL   sql.NullString
		compSQL  sql.NullString
		stats    string
		errMsg   string
	)
	if err := sc.Scan(&op.ID, &runSQL, &op.Kind, &target, &params, &op.State,
		&startSQL, &pauseSQL, &resSQL, &compSQL, &stats, &errMsg); err != nil {
		return nil, err
	}
	if runSQL.Valid {
		v := runSQL.String
		op.RunID = &v
	}
	op.Target = json.RawMessage(target)
	op.Params = json.RawMessage(params)
	if stats != "" {
		op.Stats = json.RawMessage(stats)
	}
	op.ErrorMessage = errMsg
	for _, pair := range []struct {
		sq  sql.NullString
		dst **time.Time
	}{
		{startSQL, &op.StartedAt},
		{pauseSQL, &op.PausedAt},
		{resSQL, &op.ResumedAt},
		{compSQL, &op.CompletedAt},
	} {
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
	return &op, nil
}

func nilTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return FormatTime(*t)
}

// UpsertProgress inserts or updates progress for one (operation, collection).
func (r *Operations) UpsertProgress(ctx context.Context, p Progress) error {
	if p.LastUpdated.IsZero() {
		p.LastUpdated = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO operation_progress (operation_id, collection_key, total_target, completed_count, bytes_processed, last_updated)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id, collection_key) DO UPDATE SET
			total_target = excluded.total_target,
			completed_count = excluded.completed_count,
			bytes_processed = excluded.bytes_processed,
			last_updated = excluded.last_updated
	`, p.OperationID, p.CollectionKey, p.TotalTarget, p.CompletedCount, p.BytesProcessed, FormatTime(p.LastUpdated))
	if err != nil {
		return fmt.Errorf("upsert progress: %w", err)
	}
	return nil
}

// IncProgress atomically advances completed_count and bytes_processed. If no
// row exists for the target yet it is created with the supplied totalTarget.
func (r *Operations) IncProgress(ctx context.Context, opID, collKey string, completedDelta, bytesDelta, totalTarget int64) error {
	now := FormatTime(time.Now().UTC())
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO operation_progress (operation_id, collection_key, total_target, completed_count, bytes_processed, last_updated)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id, collection_key) DO UPDATE SET
			completed_count = operation_progress.completed_count + excluded.completed_count,
			bytes_processed = operation_progress.bytes_processed + excluded.bytes_processed,
			total_target = CASE
				WHEN excluded.total_target > operation_progress.total_target
				THEN excluded.total_target
				ELSE operation_progress.total_target
			END,
			last_updated = excluded.last_updated
	`, opID, collKey, totalTarget, completedDelta, bytesDelta, now)
	if err != nil {
		return fmt.Errorf("inc progress: %w", err)
	}
	return nil
}

// ListProgress returns every progress row for an operation.
func (r *Operations) ListProgress(ctx context.Context, opID string) ([]Progress, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT operation_id, collection_key, total_target, completed_count, bytes_processed, last_updated
		FROM operation_progress WHERE operation_id = ?
		ORDER BY collection_key
	`, opID)
	if err != nil {
		return nil, fmt.Errorf("list progress: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Progress
	for rows.Next() {
		var (
			p  Progress
			ts string
		)
		if err := rows.Scan(&p.OperationID, &p.CollectionKey, &p.TotalTarget, &p.CompletedCount, &p.BytesProcessed, &ts); err != nil {
			return nil, err
		}
		t, err := ParseTime(ts)
		if err != nil {
			return nil, err
		}
		p.LastUpdated = t
		out = append(out, p)
	}
	return out, rows.Err()
}
