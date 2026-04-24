// Package deleter implements the deletion service. Five fragmentation
// patterns produce controlled "holes" in WiredTiger's block layout:
// random_by_id, range_by_field, modulo, ttl_simulated, prefix_by_id.
// Every delete invocation goes through the preview → confirmation-token →
// execute flow required by spec § 5.2.
package deleter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PatternKind enumerates the deletion strategies.
type PatternKind string

// Canonical pattern names. These are persisted in operations.params_json so
// renaming them is a breaking change.
const (
	PatternRandomByID   PatternKind = "random_by_id"
	PatternRangeByField PatternKind = "range_by_field"
	PatternModulo       PatternKind = "modulo"
	PatternTTLSimulated PatternKind = "ttl_simulated"
	PatternPrefixByID   PatternKind = "prefix_by_id"
)

// PatternConfig carries the pattern-specific tuning dials. Only the fields
// relevant to the chosen pattern are read.
type PatternConfig struct {
	Kind    PatternKind `json:"kind"`
	// Ratio applies to patterns that sample candidates: random_by_id, modulo.
	Ratio   float64     `json:"ratio,omitempty"`
	// Field applies to range_by_field and ttl_simulated.
	Field   string      `json:"field,omitempty"`
	// RangeBefore is an inclusive upper bound (range_by_field, ttl_simulated).
	RangeBefore *time.Time `json:"range_before,omitempty"`
	// Modulus applies to modulo (delete every Nth doc by _id hash).
	Modulus int `json:"modulus,omitempty"`
	// Prefix applies to prefix_by_id (delete docs whose _id hex starts with p).
	Prefix  string `json:"prefix,omitempty"`
	// Seed is used for deterministic candidate sampling.
	Seed    int64  `json:"seed,omitempty"`
}

// Validate checks pattern-specific constraints.
func (c PatternConfig) Validate(maxRatio float64) error {
	switch c.Kind {
	case PatternRandomByID, PatternModulo:
		if c.Ratio <= 0 || c.Ratio > maxRatio {
			return fmt.Errorf("ratio must be in (0, %g]", maxRatio)
		}
	case PatternRangeByField:
		if c.Field == "" {
			return errors.New("range_by_field requires field")
		}
	case PatternTTLSimulated:
		if c.RangeBefore == nil {
			return errors.New("ttl_simulated requires range_before")
		}
	case PatternPrefixByID:
		if c.Prefix == "" {
			return errors.New("prefix_by_id requires prefix")
		}
	default:
		return fmt.Errorf("unknown pattern %q", c.Kind)
	}
	return nil
}

// Pattern is the interface every deletion strategy implements.
type Pattern interface {
	Name() PatternKind
	// BuildFilter returns a MongoDB filter that narrows the candidate set to
	// the documents this pattern deletes.
	BuildFilter(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) (bson.M, error)
	// EstimateMatchCount returns the expected number of deletes for the
	// supplied filter. Fast for range/ttl (server-side CountDocuments);
	// for sampling patterns, it may be an estimate based on ratio.
	EstimateMatchCount(ctx context.Context, coll *mongo.Collection, cfg PatternConfig, filter bson.M) (int64, error)
	// CaptureCandidates optionally returns a stable list of _id values so
	// pause/resume deletes from the same set. Patterns that don't need this
	// (range/ttl/prefix are server-side deterministic) may return nil.
	CaptureCandidates(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) ([]any, error)
}

// randomByID selects a random sample of _id values. Because the candidate
// set is sampled client-side, it is captured up-front and persisted so
// pause/resume deletes from the same set (spec § 5.2).
type randomByID struct{}

func (randomByID) Name() PatternKind { return PatternRandomByID }

func (randomByID) BuildFilter(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) (bson.M, error) {
	// Filter is applied per-batch using _id IN (<batch>). This method returns
	// a pass-through filter; the actual batching happens in the executor.
	return bson.M{}, nil
}

func (randomByID) EstimateMatchCount(ctx context.Context, coll *mongo.Collection, cfg PatternConfig, filter bson.M) (int64, error) {
	total, err := coll.EstimatedDocumentCount(ctx)
	if err != nil {
		return 0, err
	}
	return int64(float64(total) * cfg.Ratio), nil
}

func (randomByID) CaptureCandidates(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) ([]any, error) {
	cur, err := coll.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var ids []any
	for cur.Next(ctx) {
		var doc struct {
			ID any `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	if cur.Err() != nil {
		return nil, cur.Err()
	}
	rng := rand.New(rand.NewSource(cfg.Seed))
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	targetN := int(float64(len(ids)) * cfg.Ratio)
	if targetN > len(ids) {
		targetN = len(ids)
	}
	return ids[:targetN], nil
}

// rangeByField deletes where a user-specified field falls in a range.
// TTLSimulated is a thin wrapper that uses `created_at`.
type rangeByField struct{ name PatternKind }

func (r rangeByField) Name() PatternKind { return r.name }

func (rangeByField) BuildFilter(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) (bson.M, error) {
	field := cfg.Field
	if field == "" {
		field = "created_at"
	}
	if cfg.RangeBefore == nil {
		return nil, errors.New("range pattern requires range_before")
	}
	return bson.M{field: bson.M{"$lt": *cfg.RangeBefore}}, nil
}

func (rangeByField) EstimateMatchCount(ctx context.Context, coll *mongo.Collection, cfg PatternConfig, filter bson.M) (int64, error) {
	return coll.CountDocuments(ctx, filter)
}

func (rangeByField) CaptureCandidates(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) ([]any, error) {
	return nil, nil // server-side filter is stable across pause/resume
}

// modulo deletes every Nth document (uniform holes; maximal fragmentation).
// We hash the _id bytes and check `hash mod N == 0`.
type modulo struct{}

func (modulo) Name() PatternKind { return PatternModulo }

func (modulo) BuildFilter(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) (bson.M, error) {
	// No server-side filter; the candidate walker applies the modulo check.
	return bson.M{}, nil
}

func (modulo) EstimateMatchCount(ctx context.Context, coll *mongo.Collection, cfg PatternConfig, filter bson.M) (int64, error) {
	total, err := coll.EstimatedDocumentCount(ctx)
	if err != nil {
		return 0, err
	}
	n := cfg.Modulus
	if n <= 0 {
		return 0, errors.New("modulo requires modulus > 0")
	}
	return total / int64(n), nil
}

func (modulo) CaptureCandidates(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) ([]any, error) {
	if cfg.Modulus <= 0 {
		return nil, errors.New("modulo requires modulus > 0")
	}
	cur, err := coll.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var ids []any
	i := 0
	for cur.Next(ctx) {
		var doc struct {
			ID any `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		h := hashAny(doc.ID)
		if h%uint64(cfg.Modulus) == 0 {
			ids = append(ids, doc.ID)
		}
		i++
	}
	return ids, cur.Err()
}

// prefixByID deletes documents whose _id hex representation starts with
// cfg.Prefix. Works for both ObjectID and UUID _ids.
type prefixByID struct{}

func (prefixByID) Name() PatternKind { return PatternPrefixByID }

func (prefixByID) BuildFilter(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) (bson.M, error) {
	// ^prefix anchored regex. Works on string _ids (UUID). For ObjectID we
	// fall back to the executor which compares byte prefixes.
	return bson.M{"_id": bson.M{"$regex": "^" + cfg.Prefix}}, nil
}

func (prefixByID) EstimateMatchCount(ctx context.Context, coll *mongo.Collection, cfg PatternConfig, filter bson.M) (int64, error) {
	// The filter is indexed on _id for UUID strings. For ObjectID this may
	// not match since ObjectID is bson.ObjectID, not a string.
	return coll.CountDocuments(ctx, filter)
}

func (prefixByID) CaptureCandidates(ctx context.Context, coll *mongo.Collection, cfg PatternConfig) ([]any, error) {
	return nil, nil
}

// PatternFor returns the Pattern implementation for cfg.Kind.
func PatternFor(cfg PatternConfig) (Pattern, error) {
	switch cfg.Kind {
	case PatternRandomByID:
		return randomByID{}, nil
	case PatternRangeByField:
		return rangeByField{name: PatternRangeByField}, nil
	case PatternTTLSimulated:
		return rangeByField{name: PatternTTLSimulated}, nil
	case PatternModulo:
		return modulo{}, nil
	case PatternPrefixByID:
		return prefixByID{}, nil
	}
	return nil, fmt.Errorf("unknown pattern %q", cfg.Kind)
}

// hashAny hashes an _id value into a uint64 for modulo bucketing.
func hashAny(v any) uint64 {
	switch x := v.(type) {
	case string:
		return fnv64(x)
	case bson.ObjectID:
		return binary.BigEndian.Uint64(x[:8])
	case []byte:
		if len(x) >= 8 {
			return binary.BigEndian.Uint64(x[:8])
		}
	}
	return fnv64(fmt.Sprintf("%v", v))
}

// fnv64 is a cheap non-cryptographic string hash. We don't need collision
// resistance; pseudo-random bucketing is sufficient for modulo.
func fnv64(s string) uint64 {
	const (
		offset64 uint64 = 1469598103934665603
		prime64  uint64 = 1099511628211
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
