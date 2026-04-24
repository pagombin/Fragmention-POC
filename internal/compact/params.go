// Package compact orchestrates the MongoDB `compact` command across replica
// set members with safety gates per spec § 5.4. A single Orchestrator runs
// compacts in rolling order - secondaries first, then a primary stepdown
// followed by the former primary - while capturing pre/post snapshots.
package compact

import (
	"encoding/json"
	"errors"
	"time"
)

// ScopeKind selects what the orchestrator will compact.
type ScopeKind string

// Compact scope breadth.
const (
	ScopeCluster     ScopeKind = "cluster"
	ScopeDatabases   ScopeKind = "databases"
	ScopeCollections ScopeKind = "collections"
)

// Scope describes the intended compact breadth. Exactly one of Databases
// or Collections is consulted based on Kind.
type Scope struct {
	Kind        ScopeKind    `json:"kind"`
	Databases   []string     `json:"databases,omitempty"`
	Collections []CollectionRef `json:"collections,omitempty"`
}

// CollectionRef is one (database, collection) pair.
type CollectionRef struct {
	Database   string `json:"database"`
	Collection string `json:"collection"`
}

// Key returns "database.collection" for the state store.
func (c CollectionRef) Key() string { return c.Database + "." + c.Collection }

// Params are the orchestrator-level knobs.
type Params struct {
	MaxReplicationLag    time.Duration `json:"max_replication_lag_ns"`
	StepdownWaitTimeout  time.Duration `json:"stepdown_wait_timeout_ns"`
	ValidateAfterCompact bool          `json:"validate_after_compact"`
}

// Validate reports misconfigurations.
func (p Params) Validate() error {
	if p.MaxReplicationLag < 0 {
		return errors.New("max_replication_lag must be >= 0")
	}
	if p.StepdownWaitTimeout <= 0 {
		return errors.New("stepdown_wait_timeout must be > 0")
	}
	return nil
}

// PreviewResult describes the planned execution order and estimate. Emitted
// by Orchestrator.Preview before Start.
type PreviewResult struct {
	Mode              string            `json:"mode"` // "single" or "rolling"
	ExecutionOrder    []MemberPlan      `json:"execution_order"`
	TotalCollections  int               `json:"total_collections"`
	EstimatedDuration time.Duration     `json:"estimated_duration_ns"`
	Warnings          []string          `json:"warnings,omitempty"`
}

// MemberPlan is one step in the planned rolling sequence.
type MemberPlan struct {
	Member         string   `json:"member"`
	Role           string   `json:"role"`
	Collections    []string `json:"collections"`
	RequiresStepdown bool   `json:"requires_stepdown,omitempty"`
}

// Stats is the operation's completion payload.
type Stats struct {
	StartedAt        time.Time                 `json:"started_at"`
	CompletedAt      time.Time                 `json:"completed_at"`
	Duration         time.Duration             `json:"duration_ns"`
	CompactedCount   int                       `json:"compacted_count"`
	SkippedCount     int                       `json:"skipped_count"`
	FailedCount      int                       `json:"failed_count"`
	TotalBytesPre    int64                     `json:"total_bytes_pre"`
	TotalBytesPost   int64                     `json:"total_bytes_post"`
	BytesReclaimed   int64                     `json:"bytes_reclaimed"`
	PerCollection    []CollectionOutcome       `json:"per_collection"`
	ValidateResults  map[string]ValidateResult `json:"validate_results,omitempty"`
}

// CollectionOutcome is the per-collection timing + space reclaim.
type CollectionOutcome struct {
	Member          string        `json:"member"`
	Database        string        `json:"database"`
	Collection      string        `json:"collection"`
	StartedAt       time.Time     `json:"started_at"`
	CompletedAt     time.Time     `json:"completed_at"`
	Duration        time.Duration `json:"duration_ns"`
	SizeBefore      int64         `json:"size_before"`
	SizeAfter       int64         `json:"size_after"`
	FreeBefore      int64         `json:"free_before"`
	FreeAfter       int64         `json:"free_after"`
	BytesReclaimed  int64         `json:"bytes_reclaimed"`
	Error           string        `json:"error,omitempty"`
}

// ValidateResult wraps the `validate` command output we care about.
type ValidateResult struct {
	Valid  bool   `json:"valid"`
	Errors int64  `json:"errors"`
	Warnings int64 `json:"warnings,omitempty"`
	Raw    json.RawMessage `json:"raw"`
}
