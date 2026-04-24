package loader

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pagombin/fragmention-poc/internal/metrics"
)

// Loader-specific Prometheus metrics. Labels are kept low-cardinality:
// `database.collection` is high cardinality across the cluster but bounded
// per run to a few dozen scopes.
var (
	DocsInserted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "loader",
		Name: "docs_inserted_total", Help: "Documents inserted by the loader.",
	}, []string{"database", "collection"})

	BytesInserted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "loader",
		Name: "bytes_inserted_total", Help: "Estimated BSON bytes inserted by the loader.",
	}, []string{"database", "collection"})

	BatchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mfpoc", Subsystem: "loader",
		Name: "insert_duration_seconds", Help: "Wall-clock time per InsertMany batch.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	}, []string{"database", "collection"})

	Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "loader",
		Name: "errors_total", Help: "Loader error count by reason.",
	}, []string{"reason"})

	ActiveWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "mfpoc", Subsystem: "loader",
		Name: "active_workers", Help: "Currently-running loader worker goroutines.",
	})
)

func init() {
	metrics.Registry.MustRegister(DocsInserted, BytesInserted, BatchDuration, Errors, ActiveWorkers)
}
