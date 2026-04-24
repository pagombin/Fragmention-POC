package deleter

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// TargetSpec describes which (database, collection) scopes a single delete
// operation will touch. Each entry may have its own pattern; the UI's most
// common case is one shared pattern across every entry, which the handler
// can construct trivially.
type TargetSpec struct {
	Entries []Target `json:"entries"`
}

// Target is one deletion target.
type Target struct {
	Database   string        `json:"database"`
	Collection string        `json:"collection"`
	Pattern    PatternConfig `json:"pattern"`
}

// Key returns the canonical "database.collection" string.
func (t Target) Key() string { return t.Database + "." + t.Collection }

// Params tunes the deletion execution. BatchSize and interBatchJitter are
// live-adjustable; MaxRatio is static and enforced at preview time.
type Params struct {
	BatchSize        int           `json:"batch_size"`
	InterBatchJitter time.Duration `json:"inter_batch_jitter_ns"`
	MaxRatio         float64       `json:"max_ratio"`
}

// Validate reports common misconfigurations.
func (p Params) Validate() error {
	if p.BatchSize <= 0 {
		return errors.New("batch_size must be > 0")
	}
	if p.MaxRatio <= 0 || p.MaxRatio > 0.95 {
		return errors.New("max_ratio must be in (0, 0.95]")
	}
	return nil
}

type live struct {
	mu sync.RWMutex
	p  Params
}

func newLive(p Params) *live { return &live{p: p} }
func (l *live) get() Params  { l.mu.RLock(); defer l.mu.RUnlock(); return l.p }
func (l *live) set(p Params) { l.mu.Lock(); defer l.mu.Unlock(); l.p = p }

// PreviewResult reports the expected impact of a delete without mutating
// any data. The token returned here must be passed to Start.
type PreviewResult struct {
	Token             string              `json:"confirmation_token"`
	ExpiresAt         time.Time           `json:"expires_at"`
	PerCollection     []PreviewPerColl    `json:"per_collection"`
	TotalMatches      int64               `json:"total_matches"`
	RequiresTyped     bool                `json:"requires_typed_confirmation"`
	MaxRatioBreached  bool                `json:"max_ratio_breached"`
}

// PreviewPerColl is the preview line item for one collection.
type PreviewPerColl struct {
	Database       string `json:"database"`
	Collection     string `json:"collection"`
	MatchedCount   int64  `json:"matched_count"`
	TotalDocuments int64  `json:"total_documents"`
}

// Stats is the completion payload stored in operations.stats_json.
type Stats struct {
	Deleted       int64           `json:"deleted"`
	BatchesOK     int64           `json:"batches_ok"`
	BatchesFailed int64           `json:"batches_failed"`
	PerCollection map[string]int64 `json:"per_collection"`
	StartedAt     time.Time       `json:"started_at"`
	CompletedAt   time.Time       `json:"completed_at"`
	Duration      time.Duration   `json:"duration_ns"`
}

func encodeJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}
