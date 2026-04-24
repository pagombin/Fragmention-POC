package deleter

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pagombin/fragmention-poc/internal/metrics"
)

// Deleter-specific Prometheus metrics.
var (
	DocsDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "deleter",
		Name: "docs_deleted_total", Help: "Documents deleted by the deleter.",
	}, []string{"database", "collection", "pattern"})

	BatchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mfpoc", Subsystem: "deleter",
		Name: "batch_duration_seconds", Help: "Wall-clock time per DeleteMany batch.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	}, []string{"database", "collection", "pattern"})

	Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc", Subsystem: "deleter",
		Name: "errors_total", Help: "Deleter error count by reason.",
	}, []string{"reason"})
)

func init() {
	metrics.Registry.MustRegister(DocsDeleted, BatchDuration, Errors)
}
