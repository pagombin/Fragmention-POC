package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EventLevel categorizes an event by severity / origin.
type EventLevel string

// EventLevel values recognised by the events table. "audit" is special:
// § 21.10 requires audit events be streamed to stdout as JSON regardless of
// the application log level.
const (
	EventLevelInfo  EventLevel = "info"
	EventLevelWarn  EventLevel = "warn"
	EventLevelError EventLevel = "error"
	EventLevelAudit EventLevel = "audit"
)

// Event represents a single entry in the application's audit/event log.
type Event struct {
	ID          int64          `json:"id"`
	RunID       *string        `json:"run_id,omitempty"`
	OperationID *string        `json:"operation_id,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Level       EventLevel     `json:"level"`
	Category    string         `json:"category"`
	Message     string         `json:"message"`
	Context     map[string]any `json:"context,omitempty"`
}

// Events is a repository for the events table.
type Events struct{ db *sql.DB }

// NewEvents constructs an Events repository. Callers typically do this once
// at startup and inject the repository into services.
func NewEvents(s *Store) *Events { return &Events{db: s.db} }

// Record inserts an event. Returns the assigned ID.
func (r *Events) Record(ctx context.Context, e Event) (int64, error) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = EventLevelInfo
	}
	if e.Category == "" {
		return 0, errors.New("events.Record: category required")
	}
	var ctxJSON sql.NullString
	if len(e.Context) > 0 {
		b, err := json.Marshal(e.Context)
		if err != nil {
			return 0, fmt.Errorf("marshal context: %w", err)
		}
		ctxJSON = sql.NullString{String: string(b), Valid: true}
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO events (run_id, operation_id, timestamp, level, category, message, context_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		nilString(e.RunID),
		nilString(e.OperationID),
		FormatTime(e.Timestamp),
		string(e.Level),
		e.Category,
		e.Message,
		ctxJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	return res.LastInsertId()
}

// List returns up to `limit` most-recent events, newest first. When category
// or runID are non-empty they act as filters.
func (r *Events) List(ctx context.Context, category, runID string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, run_id, operation_id, timestamp, level, category, message, context_json
	      FROM events WHERE 1=1`
	args := []any{}
	if category != "" {
		q += " AND category = ?"
		args = append(args, category)
	}
	if runID != "" {
		q += " AND run_id = ?"
		args = append(args, runID)
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Event
	for rows.Next() {
		var (
			e        Event
			runSQL   sql.NullString
			opSQL    sql.NullString
			tsSQL    string
			ctxSQL   sql.NullString
		)
		if err := rows.Scan(&e.ID, &runSQL, &opSQL, &tsSQL, &e.Level, &e.Category, &e.Message, &ctxSQL); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if runSQL.Valid {
			v := runSQL.String
			e.RunID = &v
		}
		if opSQL.Valid {
			v := opSQL.String
			e.OperationID = &v
		}
		t, err := ParseTime(tsSQL)
		if err != nil {
			return nil, fmt.Errorf("parse event time: %w", err)
		}
		e.Timestamp = t
		if ctxSQL.Valid && ctxSQL.String != "" {
			_ = json.Unmarshal([]byte(ctxSQL.String), &e.Context)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func nilString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
