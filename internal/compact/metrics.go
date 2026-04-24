package compact

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pagombin/fragmention-poc/internal/metrics"
)

// Compact-specific Prometheus metrics. Labels keep member+db+coll bounded
// per run.
var (
	Started = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "compact",
		Name: "started_total", Help: "Compact starts by member.",
	}, []string{"member"})

	Completed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "compact",
		Name: "completed_total", Help: "Compact completions by outcome.",
	}, []string{"member", "outcome"})

	Duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mfpoc", Subsystem: "compact",
		Name: "duration_seconds", Help: "Per-collection compact duration.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 14),
	}, []string{"member"})

	ReclaimBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "compact",
		Name: "reclaim_bytes_total", Help: "Bytes reclaimed by compact per member+collection.",
	}, []string{"member", "database", "collection"})

	Stepdown = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "compact",
		Name: "stepdowns_total", Help: "Primary stepdowns initiated by compact.",
	})
)

func init() {
	metrics.Registry.MustRegister(Started, Completed, Duration, ReclaimBytes, Stepdown)
}
