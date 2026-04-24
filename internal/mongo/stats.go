package mongo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CollectionType classifies a collection so the collector, deleter, and
// compact orchestrator can handle it appropriately. Capped and time-series
// collections do not support `compact` the same way regular collections do.
type CollectionType string

// Collection types surfaced to the UI and reports.
const (
	CollTypeRegular    CollectionType = "regular"
	CollTypeCapped     CollectionType = "capped"
	CollTypeTimeSeries CollectionType = "timeseries"
	CollTypeView       CollectionType = "view"
	CollTypeClustered  CollectionType = "clustered"
	CollTypeSystem     CollectionType = "system"
)

// DatabaseSummary is the compact result of dbStats for list views.
type DatabaseSummary struct {
	Name         string    `json:"name"`
	Collections  int64     `json:"collections"`
	DataSize     int64     `json:"data_size"`
	StorageSize  int64     `json:"storage_size"`
	IndexSize    int64     `json:"index_size"`
	FsUsedSize   int64     `json:"fs_used_size"`
	FsTotalSize  int64     `json:"fs_total_size"`
	SampledAt    time.Time `json:"sampled_at"`
}

// CollectionSummary is the compact result of collStats for list views.
// FreeStorageSize / StorageSize is the fragmentation ratio for this scope.
type CollectionSummary struct {
	Database          string         `json:"database"`
	Name              string         `json:"name"`
	Type              CollectionType `json:"type"`
	Count             int64          `json:"count"`
	Size              int64          `json:"size"`
	StorageSize       int64          `json:"storage_size"`
	FreeStorageSize   int64          `json:"free_storage_size"`
	TotalIndexSize    int64          `json:"total_index_size"`
	NumIndexes        int32          `json:"num_indexes"`
	AvgObjSize        float64        `json:"avg_obj_size"`
	Compressor        string         `json:"compressor,omitempty"`
	FragmentationRatio float64       `json:"fragmentation_ratio"`
	SampledAt         time.Time      `json:"sampled_at"`
}

// ListDatabases returns a summary for every database visible to the client.
// System databases (admin, config, local) are included but flagged via the
// `System` predicate on DatabaseSummary.
func (c *Client) ListDatabases(ctx context.Context) ([]DatabaseSummary, error) {
	raw, err := c.raw.ListDatabases(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("listDatabases: %w", err)
	}
	var out []DatabaseSummary
	for _, db := range raw.Databases {
		s, err := c.DatabaseStats(ctx, db.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// DatabaseStats runs dbStats for one database. Bytes-level fields are read as
// int64; if the server returns numbers larger than int64 we truncate rather
// than fail (the POC workloads stay well within int64 anyway).
func (c *Client) DatabaseStats(ctx context.Context, dbName string) (DatabaseSummary, error) {
	var m bson.M
	if err := c.raw.Database(dbName).RunCommand(ctx, bson.D{
		{Key: "dbStats", Value: 1},
		{Key: "scale", Value: 1},
	}).Decode(&m); err != nil {
		return DatabaseSummary{}, fmt.Errorf("dbStats %s: %w", dbName, err)
	}
	return DatabaseSummary{
		Name:        dbName,
		Collections: asInt64(m["collections"]),
		DataSize:    asInt64(m["dataSize"]),
		StorageSize: asInt64(m["storageSize"]),
		IndexSize:   asInt64(m["indexSize"]),
		FsUsedSize:  asInt64(m["fsUsedSize"]),
		FsTotalSize: asInt64(m["fsTotalSize"]),
		SampledAt:   time.Now().UTC(),
	}, nil
}

// ListCollections enumerates collections within a database with per-collection
// summary stats. Views and system collections are returned but tagged.
func (c *Client) ListCollections(ctx context.Context, dbName string) ([]CollectionSummary, error) {
	db := c.raw.Database(dbName)
	cur, err := db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("listCollections %s: %w", dbName, err)
	}
	var out []CollectionSummary
	for _, spec := range cur {
		if spec.Type == "view" {
			out = append(out, CollectionSummary{Database: dbName, Name: spec.Name, Type: CollTypeView, SampledAt: time.Now().UTC()})
			continue
		}
		s, err := c.CollectionStats(ctx, dbName, spec.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// CollectionStats runs collStats for a specific collection and returns a
// summary. The caller receives the canonical fragmentation ratio
// (freeStorageSize/storageSize) alongside the raw numerator and denominator,
// per the spec's requirement that both the ratio and its components are
// surfaced (see § 20).
func (c *Client) CollectionStats(ctx context.Context, dbName, coll string) (CollectionSummary, error) {
	db := c.raw.Database(dbName)
	var m bson.M
	if err := db.RunCommand(ctx, bson.D{
		{Key: "collStats", Value: coll},
		{Key: "scale", Value: 1},
	}).Decode(&m); err != nil {
		return CollectionSummary{}, fmt.Errorf("collStats %s.%s: %w", dbName, coll, err)
	}
	cs := CollectionSummary{
		Database:       dbName,
		Name:           coll,
		Type:           classifyCollection(dbName, coll, m),
		Count:          asInt64(m["count"]),
		Size:           asInt64(m["size"]),
		StorageSize:    asInt64(m["storageSize"]),
		FreeStorageSize: asInt64(m["freeStorageSize"]),
		TotalIndexSize: asInt64(m["totalIndexSize"]),
		NumIndexes:     clampInt32(asInt64(m["nindexes"])),
		AvgObjSize:     asFloat(m["avgObjSize"]),
		SampledAt:      time.Now().UTC(),
	}
	if wt, ok := m["wiredTiger"].(bson.M); ok {
		if cm, ok := wt["creationString"].(string); ok {
			cs.Compressor = extractCompressor(cm)
		}
	}
	if cs.StorageSize > 0 {
		cs.FragmentationRatio = float64(cs.FreeStorageSize) / float64(cs.StorageSize)
	}
	return cs, nil
}

// IndexSizes returns a map[indexName]bytes for a single collection, extracted
// from `collStats.indexSizes`. Used by the collector's per-index sampling.
func (c *Client) IndexSizes(ctx context.Context, dbName, coll string) (map[string]int64, error) {
	var m bson.M
	if err := c.raw.Database(dbName).RunCommand(ctx, bson.D{
		{Key: "collStats", Value: coll},
		{Key: "scale", Value: 1},
	}).Decode(&m); err != nil {
		return nil, fmt.Errorf("collStats %s.%s: %w", dbName, coll, err)
	}
	sz, _ := m["indexSizes"].(bson.M)
	out := make(map[string]int64, len(sz))
	for k, v := range sz {
		out[k] = asInt64(v)
	}
	return out, nil
}

func classifyCollection(dbName, coll string, stats bson.M) CollectionType {
	switch dbName {
	case "admin", "config", "local":
		return CollTypeSystem
	}
	if strings.HasPrefix(coll, "system.") {
		return CollTypeSystem
	}
	if v, ok := stats["capped"].(bool); ok && v {
		return CollTypeCapped
	}
	if _, ok := stats["timeseries"]; ok {
		return CollTypeTimeSeries
	}
	if _, ok := stats["clusteredIndex"]; ok {
		return CollTypeClustered
	}
	return CollTypeRegular
}

// extractCompressor pulls "block_compressor=<value>" out of the WiredTiger
// creation string. Returns an empty string when absent.
func extractCompressor(creationString string) string {
	const key = "block_compressor="
	i := strings.Index(creationString, key)
	if i < 0 {
		return ""
	}
	rest := creationString[i+len(key):]
	end := strings.IndexAny(rest, ",)")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	}
	return 0
}

// clampInt32 bounds v to the int32 range to satisfy gosec G115. For
// nindexes and similar metadata this clamp is never reached in practice.
func clampInt32(v int64) int32 {
	const maxI32 = int64(^uint32(0) >> 1)
	if v > maxI32 {
		return int32(maxI32)
	}
	if v < -maxI32-1 {
		return int32(-maxI32 - 1)
	}
	return int32(v)
}
