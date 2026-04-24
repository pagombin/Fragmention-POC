// Package metrics owns the Prometheus registry used by the application.
// Metrics are grouped by domain (loader, deleter, compact, workload,
// collector). Labels are kept low-cardinality.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Registry is the application-wide Prometheus registry. Using a dedicated
// registry (rather than prometheus.DefaultRegisterer) keeps tests hermetic
// and avoids picking up unrelated metrics from library code.
var Registry = prometheus.NewRegistry()

// Application-level metrics exported from every phase. Services defined in
// later phases register their own additional metrics into the same registry.
var (
	InfoGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "mfpoc",
		Name:      "info",
		Help:      "Build identity labels; value is always 1.",
	}, []string{"version", "commit", "go_version"})

	ServerInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "mfpoc",
		Subsystem: "mongo",
		Name:      "server_info",
		Help:      "MongoDB server identity labels; value is always 1.",
	}, []string{"version", "storage_engine", "compressor", "is_srv"})

	CollectorTick = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mfpoc",
		Subsystem: "collector",
		Name:      "ticks_total",
		Help:      "Number of collector ticks, by outcome.",
	}, []string{"outcome"})

	CollectorDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mfpoc",
		Subsystem: "collector",
		Name:      "tick_duration_seconds",
		Help:      "Wall-clock time per collector tick.",
		Buckets:   prometheus.ExponentialBuckets(0.005, 2, 12),
	}, []string{"scope"})

	CollectorFragRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "mfpoc",
		Subsystem: "collector",
		Name:      "fragmentation_ratio",
		Help:      "Latest fragmentation ratio per scope.",
	}, []string{"scope", "scope_id"})

	CollectorStorageBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "mfpoc",
		Subsystem: "collector",
		Name:      "storage_bytes",
		Help:      "Latest storageSize per scope.",
	}, []string{"scope", "scope_id"})

	CollectorFreeStorageBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "mfpoc",
		Subsystem: "collector",
		Name:      "free_storage_bytes",
		Help:      "Latest freeStorageSize per scope.",
	}, []string{"scope", "scope_id"})
)

func init() {
	Registry.MustRegister(
		InfoGauge,
		ServerInfo,
		CollectorTick,
		CollectorDuration,
		CollectorFragRatio,
		CollectorStorageBytes,
		CollectorFreeStorageBytes,
	)
}
