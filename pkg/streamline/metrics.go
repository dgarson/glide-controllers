package streamline

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ReconcileOutcome represents the outcome of a reconciliation.
type ReconcileOutcome string

const (
	// OutcomeSuccess indicates the reconciliation completed successfully.
	OutcomeSuccess ReconcileOutcome = "success"

	// OutcomeError indicates the reconciliation failed with an error.
	OutcomeError ReconcileOutcome = "error"

	// OutcomeRequeue indicates the reconciliation requested a requeue.
	OutcomeRequeue ReconcileOutcome = "requeue"

	// OutcomeSkipped indicates the reconciliation was skipped (e.g., paused resource).
	OutcomeSkipped ReconcileOutcome = "skipped"
)

// MetricsProvider provides access to controller metrics.
// It automatically collects standard operational metrics and allows
// users to register custom metrics for business-specific monitoring.
type MetricsProvider interface {
	// RecordReconcileDuration records the duration of a reconciliation.
	RecordReconcileDuration(controllerName string, duration time.Duration, outcome ReconcileOutcome)

	// RecordReconcileTotal increments the total reconciliation counter.
	RecordReconcileTotal(controllerName string, outcome ReconcileOutcome)

	// RecordConditionTransition records a condition state change.
	RecordConditionTransition(controllerName, conditionType, fromStatus, toStatus string)

	// RecordResourceOperation records a resource operation (create, update, delete).
	RecordResourceOperation(controllerName string, operation ResourceAction, success bool)

	// RecordActiveReconciles sets the number of active reconciles.
	RecordActiveReconciles(controllerName string, count int)

	// RecordFinalizeDuration records the duration of a finalization.
	RecordFinalizeDuration(controllerName string, duration time.Duration, success bool)

	// RecordPrerequisiteCheck records a prerequisite check result.
	RecordPrerequisiteCheck(controllerName, prerequisiteName string, satisfied bool)

	// RecordCircuitBreakerState records circuit breaker state changes.
	RecordCircuitBreakerState(name string, state CircuitState)

	// RecordBackoffRetry records a backoff retry attempt.
	RecordBackoffRetry(controllerName string, attempt int)

	// Registry returns the underlying Prometheus registry for custom metrics.
	Registry() prometheus.Registerer

	// Counter creates or retrieves a counter metric.
	Counter(opts prometheus.CounterOpts) prometheus.Counter

	// CounterVec creates or retrieves a counter vector metric.
	CounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec

	// Gauge creates or retrieves a gauge metric.
	Gauge(opts prometheus.GaugeOpts) prometheus.Gauge

	// GaugeVec creates or retrieves a gauge vector metric.
	GaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec

	// Histogram creates or retrieves a histogram metric.
	Histogram(opts prometheus.HistogramOpts) prometheus.Histogram

	// HistogramVec creates or retrieves a histogram vector metric.
	HistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec

	// Summary creates or retrieves a summary metric.
	Summary(opts prometheus.SummaryOpts) prometheus.Summary

	// SummaryVec creates or retrieves a summary vector metric.
	SummaryVec(opts prometheus.SummaryOpts, labelNames []string) *prometheus.SummaryVec
}

// MetricsConfig configures the metrics provider.
type MetricsConfig struct {
	// Namespace is the Prometheus namespace for all metrics.
	// Default: "streamline"
	Namespace string

	// Subsystem is the Prometheus subsystem for all metrics.
	// Default: "controller"
	Subsystem string

	// ReconcileBuckets are the histogram buckets for reconcile duration.
	// Default: {0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	ReconcileBuckets []float64

	// Registry is the Prometheus registry to use.
	// Default: prometheus.DefaultRegisterer
	Registry prometheus.Registerer
}

// DefaultMetricsConfig returns the default metrics configuration.
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Namespace: "streamline",
		Subsystem: "controller",
		ReconcileBuckets: []float64{
			0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
		},
		Registry: prometheus.DefaultRegisterer,
	}
}

// metricsProvider implements MetricsProvider.
type metricsProvider struct {
	config *MetricsConfig
	mu     sync.RWMutex

	// Standard metrics
	reconcileDuration      *prometheus.HistogramVec
	reconcileTotal         *prometheus.CounterVec
	conditionTransitions   *prometheus.CounterVec
	resourceOperations     *prometheus.CounterVec
	activeReconciles       *prometheus.GaugeVec
	finalizeDuration       *prometheus.HistogramVec
	prerequisiteChecks     *prometheus.CounterVec
	circuitBreakerState    *prometheus.GaugeVec
	backoffRetries         *prometheus.CounterVec

	// Custom metrics cache
	counters   map[string]prometheus.Counter
	counterVecs map[string]*prometheus.CounterVec
	gauges     map[string]prometheus.Gauge
	gaugeVecs  map[string]*prometheus.GaugeVec
	histograms map[string]prometheus.Histogram
	histogramVecs map[string]*prometheus.HistogramVec
	summaries  map[string]prometheus.Summary
	summaryVecs map[string]*prometheus.SummaryVec
}

// NewMetricsProvider creates a new MetricsProvider with the given configuration.
func NewMetricsProvider(config *MetricsConfig) MetricsProvider {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	mp := &metricsProvider{
		config:      config,
		counters:    make(map[string]prometheus.Counter),
		counterVecs: make(map[string]*prometheus.CounterVec),
		gauges:      make(map[string]prometheus.Gauge),
		gaugeVecs:   make(map[string]*prometheus.GaugeVec),
		histograms:  make(map[string]prometheus.Histogram),
		histogramVecs: make(map[string]*prometheus.HistogramVec),
		summaries:   make(map[string]prometheus.Summary),
		summaryVecs: make(map[string]*prometheus.SummaryVec),
	}

	mp.initStandardMetrics()
	return mp
}

func (mp *metricsProvider) initStandardMetrics() {
	mp.reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation in seconds",
			Buckets:   mp.config.ReconcileBuckets,
		},
		[]string{"controller", "outcome"},
	)

	mp.reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "reconcile_total",
			Help:      "Total number of reconciliations",
		},
		[]string{"controller", "outcome"},
	)

	mp.conditionTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "condition_transitions_total",
			Help:      "Total number of condition state transitions",
		},
		[]string{"controller", "condition_type", "from_status", "to_status"},
	)

	mp.resourceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "resource_operations_total",
			Help:      "Total number of resource operations",
		},
		[]string{"controller", "operation", "success"},
	)

	mp.activeReconciles = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "active_reconciles",
			Help:      "Number of active reconciliations",
		},
		[]string{"controller"},
	)

	mp.finalizeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "finalize_duration_seconds",
			Help:      "Duration of finalization in seconds",
			Buckets:   mp.config.ReconcileBuckets,
		},
		[]string{"controller", "success"},
	)

	mp.prerequisiteChecks = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "prerequisite_checks_total",
			Help:      "Total number of prerequisite checks",
		},
		[]string{"controller", "prerequisite", "satisfied"},
	)

	mp.circuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "circuit_breaker_state",
			Help:      "Current state of circuit breakers (0=closed, 1=open, 2=half-open)",
		},
		[]string{"name"},
	)

	mp.backoffRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: mp.config.Namespace,
			Subsystem: mp.config.Subsystem,
			Name:      "backoff_retries_total",
			Help:      "Total number of backoff retry attempts",
		},
		[]string{"controller", "attempt"},
	)

	// Register all standard metrics
	mp.config.Registry.MustRegister(
		mp.reconcileDuration,
		mp.reconcileTotal,
		mp.conditionTransitions,
		mp.resourceOperations,
		mp.activeReconciles,
		mp.finalizeDuration,
		mp.prerequisiteChecks,
		mp.circuitBreakerState,
		mp.backoffRetries,
	)
}

func (mp *metricsProvider) RecordReconcileDuration(controllerName string, duration time.Duration, outcome ReconcileOutcome) {
	mp.reconcileDuration.WithLabelValues(controllerName, string(outcome)).Observe(duration.Seconds())
}

func (mp *metricsProvider) RecordReconcileTotal(controllerName string, outcome ReconcileOutcome) {
	mp.reconcileTotal.WithLabelValues(controllerName, string(outcome)).Inc()
}

func (mp *metricsProvider) RecordConditionTransition(controllerName, conditionType, fromStatus, toStatus string) {
	mp.conditionTransitions.WithLabelValues(controllerName, conditionType, fromStatus, toStatus).Inc()
}

func (mp *metricsProvider) RecordResourceOperation(controllerName string, operation ResourceAction, success bool) {
	mp.resourceOperations.WithLabelValues(controllerName, string(operation), boolToString(success)).Inc()
}

func (mp *metricsProvider) RecordActiveReconciles(controllerName string, count int) {
	mp.activeReconciles.WithLabelValues(controllerName).Set(float64(count))
}

func (mp *metricsProvider) RecordFinalizeDuration(controllerName string, duration time.Duration, success bool) {
	mp.finalizeDuration.WithLabelValues(controllerName, boolToString(success)).Observe(duration.Seconds())
}

func (mp *metricsProvider) RecordPrerequisiteCheck(controllerName, prerequisiteName string, satisfied bool) {
	mp.prerequisiteChecks.WithLabelValues(controllerName, prerequisiteName, boolToString(satisfied)).Inc()
}

func (mp *metricsProvider) RecordCircuitBreakerState(name string, state CircuitState) {
	mp.circuitBreakerState.WithLabelValues(name).Set(float64(state))
}

func (mp *metricsProvider) RecordBackoffRetry(controllerName string, attempt int) {
	mp.backoffRetries.WithLabelValues(controllerName, intToString(attempt)).Inc()
}

func (mp *metricsProvider) Registry() prometheus.Registerer {
	return mp.config.Registry
}

func (mp *metricsProvider) Counter(opts prometheus.CounterOpts) prometheus.Counter {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if c, exists := mp.counters[key]; exists {
		return c
	}

	c := prometheus.NewCounter(opts)
	mp.config.Registry.MustRegister(c)
	mp.counters[key] = c
	return c
}

func (mp *metricsProvider) CounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if cv, exists := mp.counterVecs[key]; exists {
		return cv
	}

	cv := prometheus.NewCounterVec(opts, labelNames)
	mp.config.Registry.MustRegister(cv)
	mp.counterVecs[key] = cv
	return cv
}

func (mp *metricsProvider) Gauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if g, exists := mp.gauges[key]; exists {
		return g
	}

	g := prometheus.NewGauge(opts)
	mp.config.Registry.MustRegister(g)
	mp.gauges[key] = g
	return g
}

func (mp *metricsProvider) GaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if gv, exists := mp.gaugeVecs[key]; exists {
		return gv
	}

	gv := prometheus.NewGaugeVec(opts, labelNames)
	mp.config.Registry.MustRegister(gv)
	mp.gaugeVecs[key] = gv
	return gv
}

func (mp *metricsProvider) Histogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if h, exists := mp.histograms[key]; exists {
		return h
	}

	h := prometheus.NewHistogram(opts)
	mp.config.Registry.MustRegister(h)
	mp.histograms[key] = h
	return h
}

func (mp *metricsProvider) HistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if hv, exists := mp.histogramVecs[key]; exists {
		return hv
	}

	hv := prometheus.NewHistogramVec(opts, labelNames)
	mp.config.Registry.MustRegister(hv)
	mp.histogramVecs[key] = hv
	return hv
}

func (mp *metricsProvider) Summary(opts prometheus.SummaryOpts) prometheus.Summary {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if s, exists := mp.summaries[key]; exists {
		return s
	}

	s := prometheus.NewSummary(opts)
	mp.config.Registry.MustRegister(s)
	mp.summaries[key] = s
	return s
}

func (mp *metricsProvider) SummaryVec(opts prometheus.SummaryOpts, labelNames []string) *prometheus.SummaryVec {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	key := opts.Namespace + "_" + opts.Subsystem + "_" + opts.Name
	if sv, exists := mp.summaryVecs[key]; exists {
		return sv
	}

	sv := prometheus.NewSummaryVec(opts, labelNames)
	mp.config.Registry.MustRegister(sv)
	mp.summaryVecs[key] = sv
	return sv
}

// NoopMetricsProvider is a MetricsProvider that does nothing.
// Useful for testing or when metrics are disabled.
type NoopMetricsProvider struct{}

// NewNoopMetricsProvider creates a new no-op metrics provider.
func NewNoopMetricsProvider() MetricsProvider {
	return &NoopMetricsProvider{}
}

func (n *NoopMetricsProvider) RecordReconcileDuration(string, time.Duration, ReconcileOutcome) {}
func (n *NoopMetricsProvider) RecordReconcileTotal(string, ReconcileOutcome)                   {}
func (n *NoopMetricsProvider) RecordConditionTransition(string, string, string, string)        {}
func (n *NoopMetricsProvider) RecordResourceOperation(string, ResourceAction, bool)            {}
func (n *NoopMetricsProvider) RecordActiveReconciles(string, int)                              {}
func (n *NoopMetricsProvider) RecordFinalizeDuration(string, time.Duration, bool)              {}
func (n *NoopMetricsProvider) RecordPrerequisiteCheck(string, string, bool)                    {}
func (n *NoopMetricsProvider) RecordCircuitBreakerState(string, CircuitState)                  {}
func (n *NoopMetricsProvider) RecordBackoffRetry(string, int)                                  {}

func (n *NoopMetricsProvider) Registry() prometheus.Registerer {
	return prometheus.NewRegistry()
}

func (n *NoopMetricsProvider) Counter(opts prometheus.CounterOpts) prometheus.Counter {
	return prometheus.NewCounter(opts)
}

func (n *NoopMetricsProvider) CounterVec(opts prometheus.CounterOpts, labels []string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(opts, labels)
}

func (n *NoopMetricsProvider) Gauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	return prometheus.NewGauge(opts)
}

func (n *NoopMetricsProvider) GaugeVec(opts prometheus.GaugeOpts, labels []string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(opts, labels)
}

func (n *NoopMetricsProvider) Histogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	return prometheus.NewHistogram(opts)
}

func (n *NoopMetricsProvider) HistogramVec(opts prometheus.HistogramOpts, labels []string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(opts, labels)
}

func (n *NoopMetricsProvider) Summary(opts prometheus.SummaryOpts) prometheus.Summary {
	return prometheus.NewSummary(opts)
}

func (n *NoopMetricsProvider) SummaryVec(opts prometheus.SummaryOpts, labels []string) *prometheus.SummaryVec {
	return prometheus.NewSummaryVec(opts, labels)
}

// Helper functions
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intToString(i int) string {
	return fmt.Sprintf("%d", i)
}

// ReconcileTimer is a helper for timing reconciliations.
type ReconcileTimer struct {
	start      time.Time
	controller string
	metrics    MetricsProvider
}

// NewReconcileTimer creates a new timer for a reconciliation.
func NewReconcileTimer(metrics MetricsProvider, controllerName string) *ReconcileTimer {
	return &ReconcileTimer{
		start:      time.Now(),
		controller: controllerName,
		metrics:    metrics,
	}
}

// ObserveDuration records the duration with the given outcome.
func (t *ReconcileTimer) ObserveDuration(outcome ReconcileOutcome) {
	duration := time.Since(t.start)
	t.metrics.RecordReconcileDuration(t.controller, duration, outcome)
	t.metrics.RecordReconcileTotal(t.controller, outcome)
}

// Duration returns the elapsed time since the timer was created.
func (t *ReconcileTimer) Duration() time.Duration {
	return time.Since(t.start)
}
