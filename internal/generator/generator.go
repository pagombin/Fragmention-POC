// Package generator produces synthetic MongoDB documents for the loader.
// Six distinct schema templates cover the document-size spectrum from
// telemetry (~500B) through document_blob (~200KB). Each template returns a
// bson.D so field ordering matches real application data layouts, and each
// generator is seedable so reproducible runs are possible (spec § 7).
package generator

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Template is the interface every document template implements.
type Template interface {
	Name() string
	// Generate returns one document and its BSON-marshal size.
	Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error)
	// IndexSpecs describes the indexes that must exist on a collection
	// seeded with this template. Order matters for replay semantics.
	IndexSpecs() []IndexSpec
	// IDKind reports the _id generation strategy for this template. ObjectID
	// concentrates writes on the B-tree right edge; UUID distributes them
	// uniformly - see METHODOLOGY.md for the tradeoff.
	IDKind() IDKind
}

// IDKind selects the _id format.
type IDKind string

// Supported _id strategies.
const (
	IDKindObjectID IDKind = "objectid"
	IDKindUUID     IDKind = "uuid"
)

// IndexSpec is a minimal portable index description. Options is a subset;
// additional options can be added without changing the interface since the
// loader translates this into the driver's IndexModel.
type IndexSpec struct {
	Name    string         `json:"name"`
	Keys    map[string]int `json:"keys"`
	Unique  bool           `json:"unique,omitempty"`
	Sparse  bool           `json:"sparse,omitempty"`
	Partial bson.M         `json:"partial,omitempty"`
}

// Registry is a weighted template picker. Weights must be > 0 and are
// normalized internally.
type Registry struct {
	mu        sync.RWMutex
	templates []Template
	weights   []float64
	cum       []float64
}

// NewRegistry constructs a Registry from a weighted set. Supply weights in
// the same order as templates. An empty weights slice uses equal weights.
func NewRegistry(templates []Template, weights []float64) (*Registry, error) {
	if len(templates) == 0 {
		return nil, fmt.Errorf("generator: at least one template required")
	}
	if len(weights) == 0 {
		weights = make([]float64, len(templates))
		for i := range weights {
			weights[i] = 1
		}
	}
	if len(weights) != len(templates) {
		return nil, fmt.Errorf("generator: weights length %d != templates length %d", len(weights), len(templates))
	}
	var total float64
	for i, w := range weights {
		if w <= 0 {
			return nil, fmt.Errorf("generator: weight for %s must be > 0", templates[i].Name())
		}
		total += w
	}
	cum := make([]float64, len(weights))
	acc := 0.0
	for i, w := range weights {
		acc += w / total
		cum[i] = acc
	}
	return &Registry{templates: templates, weights: weights, cum: cum}, nil
}

// Pick returns a weighted-random template. Safe for concurrent use.
func (r *Registry) Pick(rng *rand.Rand) Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x := rng.Float64()
	for i, c := range r.cum {
		if x <= c {
			return r.templates[i]
		}
	}
	return r.templates[len(r.templates)-1]
}

// Templates returns the registered templates in registration order.
func (r *Registry) Templates() []Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Template(nil), r.templates...)
}

// MakeID returns a new _id consistent with the given IDKind.
func MakeID(kind IDKind) any {
	if kind == IDKindUUID {
		return uuid.NewString()
	}
	return bson.NewObjectID()
}

// defaultCommonFields returns the fields every loaded document carries for
// provenance tracking. created_at enables the TTL-simulated deletion pattern,
// and _load_run_id lets us trace which run produced which documents.
func defaultCommonFields(loadRunID string) bson.D {
	return bson.D{
		{Key: "created_at", Value: time.Now().UTC()},
		{Key: "_load_run_id", Value: loadRunID},
	}
}

// sizedText returns a random ASCII-ish string of exactly n bytes.
// hex.Encode writes 2*len(src) bytes, so we size the source accordingly.
// The output is lightly spaced so WiredTiger's dictionary compressor sees
// realistic variety rather than collapsing a long hex run.
func sizedText(r *rand.Rand, n int) string {
	if n <= 0 {
		return ""
	}
	raw := make([]byte, (n+1)/2)
	_, _ = r.Read(raw)
	hexed := make([]byte, len(raw)*2)
	hex.Encode(hexed, raw)
	if len(hexed) > n {
		hexed = hexed[:n]
	}
	out := make([]byte, 0, n)
	for i, c := range hexed {
		out = append(out, c)
		if i > 0 && i%11 == 0 && len(out) < n {
			out = append(out, ' ')
		}
	}
	if len(out) > n {
		out = out[:n]
	} else if len(out) < n {
		pad := make([]byte, n-len(out))
		for i := range pad {
			pad[i] = ' '
		}
		out = append(out, pad...)
	}
	return string(out)
}

func marshalSize(doc bson.D) int {
	raw, err := bson.Marshal(doc)
	if err != nil {
		return 0
	}
	return len(raw)
}

// pickWords returns a lorem-like sentence using gofakeit words.
func pickWords(fk *gofakeit.Faker, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fk.Word()
	}
	return strings.Join(parts, " ")
}
