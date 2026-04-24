package loader

import (
	"context"
	"fmt"

	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
)

// PreflightResult describes whether a load is safe to start given current
// cluster free space and the caller's target bytes.
type PreflightResult struct {
	EstimatedBytes    int64   `json:"estimated_bytes"`
	FreeBytes         int64   `json:"free_bytes"`
	TotalBytes        int64   `json:"total_bytes"`
	HeadroomPercent   float64 `json:"headroom_percent"`
	Safe              bool    `json:"safe"`
	Reason            string  `json:"reason,omitempty"`
}

// Preflight checks whether the load is likely to fit on disk with the
// configured safety margin. Implements spec § 5.1 "Pre-flight storage check".
// Compression reduces the on-disk footprint, but since we cannot know the
// codec's ratio a priori we conservatively compare against the raw estimate.
func Preflight(ctx context.Context, mc *mongoClient.Client, spec TargetSpec, headroomPct float64) (PreflightResult, error) {
	var total int64
	for _, t := range spec.Entries {
		total += t.BytesTarget
	}
	dbs, err := mc.ListDatabases(ctx)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("list databases: %w", err)
	}
	var free, tot int64
	for _, db := range dbs {
		if db.FsTotalSize > tot {
			tot = db.FsTotalSize
			free = db.FsTotalSize - db.FsUsedSize
		}
	}
	required := int64(float64(total) * (1.0 + headroomPct/100.0))
	res := PreflightResult{
		EstimatedBytes:  total,
		FreeBytes:       free,
		TotalBytes:      tot,
		HeadroomPercent: headroomPct,
		Safe:            free >= required || free == 0, // 0 = unknown (cluster didn't report fsTotalSize)
	}
	if !res.Safe {
		res.Reason = fmt.Sprintf("need %d bytes with %d%% headroom, have %d free", required, int(headroomPct), free)
	}
	return res, nil
}
