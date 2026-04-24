// Package loader implements the data-load service. It generates synthetic
// documents via internal/generator templates and writes them into the target
// MongoDB cluster at bounded throughput. A single Loader exposes the full
// spec § 5.1 lifecycle: start, pause, resume, stop, adjust parameters, and
// resume-from-persisted-progress after a restart.
package loader

import (
	"errors"
	"sync"
	"time"
)

// TargetSpec describes where one loader invocation will write. Fields
// marshal directly into operations.target_json.
type TargetSpec struct {
	// Entries must be non-empty. Each entry writes to Database / Collection
	// with an approximate BytesTarget (uncompressed) and the weighted
	// template mix described below.
	Entries []Target `json:"entries"`
}

// Target is one (database, collection) with its size target and optional
// template weight override.
type Target struct {
	Database       string             `json:"database"`
	Collection     string             `json:"collection"`
	BytesTarget    int64              `json:"bytes_target"`
	TemplateWeights map[string]float64 `json:"template_weights,omitempty"`
}

// Key returns the canonical "database.collection" key used in the state
// store progress table.
func (t Target) Key() string { return t.Database + "." + t.Collection }

// Params are the knobs the operator can tune. All are live-adjustable
// except WriteConcern (which changes driver behavior below the pool layer).
type Params struct {
	Workers          int           `json:"workers"`
	BatchSize        int           `json:"batch_size"`
	DocsPerSecond    float64       `json:"docs_per_second"` // 0 = unlimited
	WriteConcern     string        `json:"write_concern"`
	Seed             int64         `json:"seed"`
	LoadRunID        string        `json:"load_run_id"`
	StorageHeadroom  float64       `json:"storage_headroom_percent"`
	ForceStart       bool          `json:"force_start"`
}

// Validate reports common misconfigurations. Called before starting an
// operation and whenever parameters are adjusted mid-flight.
func (p Params) Validate() error {
	if p.Workers < 0 {
		return errors.New("workers must be >= 0")
	}
	if p.BatchSize <= 0 {
		return errors.New("batch_size must be > 0")
	}
	if p.DocsPerSecond < 0 {
		return errors.New("docs_per_second must be >= 0")
	}
	return nil
}

// live holds the parameters that workers read on every iteration. The
// pointer + mutex pattern keeps per-iteration reads cheap while allowing
// atomic replacement on PATCH /loader/{id}/params.
type live struct {
	mu sync.RWMutex
	p  Params
}

func newLive(p Params) *live { return &live{p: p} }

func (l *live) get() Params {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.p
}

func (l *live) set(p Params) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.p = p
}

// Stats is the JSON payload stored in operations.stats_json on completion.
type Stats struct {
	DocsInserted  int64         `json:"docs_inserted"`
	BytesInserted int64         `json:"bytes_inserted"`
	BatchesOK     int64         `json:"batches_ok"`
	BatchesFailed int64         `json:"batches_failed"`
	StartedAt     time.Time     `json:"started_at"`
	CompletedAt   time.Time     `json:"completed_at"`
	Duration      time.Duration `json:"duration_ns"`
}
