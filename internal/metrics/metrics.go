// Package metrics defines the Prometheus metrics exposed by pastebin
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "pastebin"

// Registry holds all pastebin metrics together with the Go runtime and
// process collectors
var Registry = prometheus.NewRegistry()

var (
	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Build information about the running pastebin binary. Always 1.",
	}, []string{"version", "commit"})

	// HTTPRequests counts handled HTTP requests
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total number of HTTP requests handled, by method, route and status code.",
	}, []string{"method", "route", "code"})

	// HTTPRequestDuration observes the time spent handling HTTP requests
	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "Time spent handling HTTP requests, by method and route.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	// S3Requests counts HTTP requests made to S3, including retries
	S3Requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "s3",
		Name:      "requests_total",
		Help:      "Total number of requests made to S3, by operation and status code (\"error\" when no response was received).",
	}, []string{"operation", "code"})

	// S3RequestDuration observes the latency of requests made to S3
	S3RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "s3",
		Name:      "request_duration_seconds",
		Help:      "Latency of requests made to S3, by operation.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"operation"})

	// PastesCreated counts pastes stored successfully
	PastesCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "pastes_created_total",
		Help:      "Total number of pastes created.",
	})

	// PastesDeleted counts pastes deleted, by reason
	PastesDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "pastes_deleted_total",
		Help:      "Total number of pastes deleted, by reason (manual, expired).",
	}, []string{"reason"})

	// CleanupRuns counts completed cleanup runs, by result
	CleanupRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "cleanup",
		Name:      "runs_total",
		Help:      "Total number of cleanup runs, by result (success, error).",
	}, []string{"result"})

	// CleanupDuration observes the duration of cleanup runs
	CleanupDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "cleanup",
		Name:      "duration_seconds",
		Help:      "Duration of cleanup runs.",
		Buckets:   prometheus.ExponentialBuckets(0.1, 4, 8), // 0.1s to ~27m
	})

	// CleanupLastSuccess records when a cleanup run last completed without error
	CleanupLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "cleanup",
		Name:      "last_success_timestamp_seconds",
		Help:      "Unix timestamp of the last cleanup run that completed without error.",
	})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		buildInfo,
		HTTPRequests,
		HTTPRequestDuration,
		S3Requests,
		S3RequestDuration,
		PastesCreated,
		PastesDeleted,
		CleanupRuns,
		CleanupDuration,
		CleanupLastSuccess,
	)

	// Initialise label combinations that are known up front so that the
	// series exist, and rate() works, before the first event
	for _, reason := range []string{"manual", "expired"} {
		PastesDeleted.WithLabelValues(reason)
	}
	for _, result := range []string{"success", "error"} {
		CleanupRuns.WithLabelValues(result)
	}
}

// SetBuildInfo records the version and commit of the running binary
func SetBuildInfo(version, commit string) {
	buildInfo.WithLabelValues(version, commit).Set(1)
}

// Handler returns an HTTP handler that serves the metrics in the
// Prometheus exposition format
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry})
}
