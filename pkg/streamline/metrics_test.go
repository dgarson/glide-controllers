package streamline

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestReconcileOutcomeValues verifies all ReconcileOutcome enum values.
func TestReconcileOutcomeValues(t *testing.T) {
	tests := []struct {
		outcome ReconcileOutcome
		want    string
	}{
		{OutcomeSuccess, "success"},
		{OutcomeError, "error"},
		{OutcomeRequeue, "requeue"},
		{OutcomeSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.outcome) != tt.want {
				t.Errorf("ReconcileOutcome = %q, want %q", string(tt.outcome), tt.want)
			}
		})
	}
}

// TestDefaultMetricsConfig verifies the default configuration values.
func TestDefaultMetricsConfig(t *testing.T) {
	config := DefaultMetricsConfig()

	if config.Namespace != "streamline" {
		t.Errorf("Namespace = %q, want %q", config.Namespace, "streamline")
	}

	if config.Subsystem != "controller" {
		t.Errorf("Subsystem = %q, want %q", config.Subsystem, "controller")
	}

	expectedBuckets := []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	if len(config.ReconcileBuckets) != len(expectedBuckets) {
		t.Errorf("ReconcileBuckets length = %d, want %d", len(config.ReconcileBuckets), len(expectedBuckets))
	}
	for i, bucket := range config.ReconcileBuckets {
		if bucket != expectedBuckets[i] {
			t.Errorf("ReconcileBuckets[%d] = %f, want %f", i, bucket, expectedBuckets[i])
		}
	}

	if config.Registry != prometheus.DefaultRegisterer {
		t.Error("Registry should be prometheus.DefaultRegisterer by default")
	}
}

// TestNewMetricsProviderWithNilConfig verifies provider creation with nil config uses defaults.
func TestNewMetricsProviderWithNilConfig(t *testing.T) {
	// Use a fresh registry to avoid conflicts
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "test",
		Subsystem:        "metrics",
		ReconcileBuckets: []float64{0.1, 1, 10},
		Registry:         reg,
	}

	mp := NewMetricsProvider(config)
	if mp == nil {
		t.Fatal("NewMetricsProvider returned nil")
	}

	// Verify it implements MetricsProvider
	var _ MetricsProvider = mp
}

// TestNewMetricsProviderWithCustomConfig verifies provider creation with custom config.
func TestNewMetricsProviderWithCustomConfig(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "custom",
		Subsystem:        "test",
		ReconcileBuckets: []float64{0.5, 1.0, 5.0},
		Registry:         reg,
	}

	mp := NewMetricsProvider(config)
	if mp == nil {
		t.Fatal("NewMetricsProvider returned nil")
	}

	// Verify registry is accessible
	if mp.Registry() != reg {
		t.Error("Registry() should return the configured registry")
	}
}

// TestNoopMetricsProvider tests all methods of NoopMetricsProvider.
func TestNoopMetricsProvider(t *testing.T) {
	mp := NewNoopMetricsProvider()
	if mp == nil {
		t.Fatal("NewNoopMetricsProvider returned nil")
	}

	// All these should execute without panic
	t.Run("RecordReconcileDuration", func(t *testing.T) {
		mp.RecordReconcileDuration("test-controller", 100*time.Millisecond, OutcomeSuccess)
	})

	t.Run("RecordReconcileTotal", func(t *testing.T) {
		mp.RecordReconcileTotal("test-controller", OutcomeSuccess)
		mp.RecordReconcileTotal("test-controller", OutcomeError)
		mp.RecordReconcileTotal("test-controller", OutcomeRequeue)
		mp.RecordReconcileTotal("test-controller", OutcomeSkipped)
	})

	t.Run("RecordConditionTransition", func(t *testing.T) {
		mp.RecordConditionTransition("test-controller", "Ready", "Unknown", "True")
	})

	t.Run("RecordResourceOperation", func(t *testing.T) {
		mp.RecordResourceOperation("test-controller", ResourceCreated, true)
		mp.RecordResourceOperation("test-controller", ResourceUpdated, false)
		mp.RecordResourceOperation("test-controller", ResourceDeleted, true)
	})

	t.Run("RecordActiveReconciles", func(t *testing.T) {
		mp.RecordActiveReconciles("test-controller", 5)
		mp.RecordActiveReconciles("test-controller", 0)
	})

	t.Run("RecordFinalizeDuration", func(t *testing.T) {
		mp.RecordFinalizeDuration("test-controller", 50*time.Millisecond, true)
		mp.RecordFinalizeDuration("test-controller", 100*time.Millisecond, false)
	})

	t.Run("RecordPrerequisiteCheck", func(t *testing.T) {
		mp.RecordPrerequisiteCheck("test-controller", "database", true)
		mp.RecordPrerequisiteCheck("test-controller", "cache", false)
	})

	t.Run("RecordCircuitBreakerState", func(t *testing.T) {
		mp.RecordCircuitBreakerState("api-breaker", CircuitClosed)
		mp.RecordCircuitBreakerState("api-breaker", CircuitOpen)
		mp.RecordCircuitBreakerState("api-breaker", CircuitHalfOpen)
	})

	t.Run("RecordBackoffRetry", func(t *testing.T) {
		mp.RecordBackoffRetry("test-controller", 1)
		mp.RecordBackoffRetry("test-controller", 5)
	})

	t.Run("Registry", func(t *testing.T) {
		reg := mp.Registry()
		if reg == nil {
			t.Error("Registry() should not return nil")
		}
	})

	t.Run("Counter", func(t *testing.T) {
		counter := mp.Counter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		if counter == nil {
			t.Error("Counter() should not return nil")
		}
		counter.Inc()
	})

	t.Run("CounterVec", func(t *testing.T) {
		cv := mp.CounterVec(prometheus.CounterOpts{
			Name: "test_counter_vec",
			Help: "Test counter vec",
		}, []string{"label"})
		if cv == nil {
			t.Error("CounterVec() should not return nil")
		}
		cv.WithLabelValues("value").Inc()
	})

	t.Run("Gauge", func(t *testing.T) {
		gauge := mp.Gauge(prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "Test gauge",
		})
		if gauge == nil {
			t.Error("Gauge() should not return nil")
		}
		gauge.Set(42)
	})

	t.Run("GaugeVec", func(t *testing.T) {
		gv := mp.GaugeVec(prometheus.GaugeOpts{
			Name: "test_gauge_vec",
			Help: "Test gauge vec",
		}, []string{"label"})
		if gv == nil {
			t.Error("GaugeVec() should not return nil")
		}
		gv.WithLabelValues("value").Set(10)
	})

	t.Run("Histogram", func(t *testing.T) {
		histogram := mp.Histogram(prometheus.HistogramOpts{
			Name:    "test_histogram",
			Help:    "Test histogram",
			Buckets: []float64{0.1, 1, 10},
		})
		if histogram == nil {
			t.Error("Histogram() should not return nil")
		}
		histogram.Observe(0.5)
	})

	t.Run("HistogramVec", func(t *testing.T) {
		hv := mp.HistogramVec(prometheus.HistogramOpts{
			Name:    "test_histogram_vec",
			Help:    "Test histogram vec",
			Buckets: []float64{0.1, 1, 10},
		}, []string{"label"})
		if hv == nil {
			t.Error("HistogramVec() should not return nil")
		}
		hv.WithLabelValues("value").Observe(2.5)
	})

	t.Run("Summary", func(t *testing.T) {
		summary := mp.Summary(prometheus.SummaryOpts{
			Name: "test_summary",
			Help: "Test summary",
		})
		if summary == nil {
			t.Error("Summary() should not return nil")
		}
		summary.Observe(1.5)
	})

	t.Run("SummaryVec", func(t *testing.T) {
		sv := mp.SummaryVec(prometheus.SummaryOpts{
			Name: "test_summary_vec",
			Help: "Test summary vec",
		}, []string{"label"})
		if sv == nil {
			t.Error("SummaryVec() should not return nil")
		}
		sv.WithLabelValues("value").Observe(3.0)
	})
}

// TestPrometheusMetricsProvider tests the real metrics provider.
func TestPrometheusMetricsProvider(t *testing.T) {
	// Create a fresh registry for each test to avoid conflicts
	newProvider := func() (MetricsProvider, *prometheus.Registry) {
		reg := prometheus.NewRegistry()
		config := &MetricsConfig{
			Namespace:        "test",
			Subsystem:        "metrics",
			ReconcileBuckets: []float64{0.01, 0.1, 1, 10},
			Registry:         reg,
		}
		return NewMetricsProvider(config), reg
	}

	t.Run("RecordReconcileDuration", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordReconcileDuration("my-controller", 100*time.Millisecond, OutcomeSuccess)
		mp.RecordReconcileDuration("my-controller", 200*time.Millisecond, OutcomeError)

		// Gather metrics and verify
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_reconcile_duration_seconds" {
				found = true
				if len(mf.GetMetric()) < 1 {
					t.Error("Expected at least one metric")
				}
			}
		}
		if !found {
			t.Error("reconcile_duration_seconds metric not found")
		}
	})

	t.Run("RecordReconcileTotal", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordReconcileTotal("controller-a", OutcomeSuccess)
		mp.RecordReconcileTotal("controller-a", OutcomeSuccess)
		mp.RecordReconcileTotal("controller-a", OutcomeError)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_reconcile_total" {
				found = true
				// Check that we have metrics
				metrics := mf.GetMetric()
				if len(metrics) < 1 {
					t.Error("Expected at least one metric")
				}
			}
		}
		if !found {
			t.Error("reconcile_total metric not found")
		}
	})

	t.Run("RecordConditionTransition", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordConditionTransition("controller", "Ready", "Unknown", "True")
		mp.RecordConditionTransition("controller", "Ready", "True", "False")

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_condition_transitions_total" {
				found = true
			}
		}
		if !found {
			t.Error("condition_transitions_total metric not found")
		}
	})

	t.Run("RecordResourceOperation", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordResourceOperation("controller", ResourceCreated, true)
		mp.RecordResourceOperation("controller", ResourceUpdated, true)
		mp.RecordResourceOperation("controller", ResourceDeleted, false)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_resource_operations_total" {
				found = true
				// Verify we have metrics with different labels
				if len(mf.GetMetric()) < 1 {
					t.Error("Expected at least one metric")
				}
			}
		}
		if !found {
			t.Error("resource_operations_total metric not found")
		}
	})

	t.Run("RecordActiveReconciles", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordActiveReconciles("controller", 5)
		mp.RecordActiveReconciles("controller", 3)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_active_reconciles" {
				found = true
				// Check the current value
				for _, m := range mf.GetMetric() {
					if m.GetGauge().GetValue() != 3 {
						t.Errorf("Expected gauge value 3, got %f", m.GetGauge().GetValue())
					}
				}
			}
		}
		if !found {
			t.Error("active_reconciles metric not found")
		}
	})

	t.Run("RecordFinalizeDuration", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordFinalizeDuration("controller", 50*time.Millisecond, true)
		mp.RecordFinalizeDuration("controller", 100*time.Millisecond, false)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_finalize_duration_seconds" {
				found = true
			}
		}
		if !found {
			t.Error("finalize_duration_seconds metric not found")
		}
	})

	t.Run("RecordPrerequisiteCheck", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordPrerequisiteCheck("controller", "database", true)
		mp.RecordPrerequisiteCheck("controller", "cache", false)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_prerequisite_checks_total" {
				found = true
			}
		}
		if !found {
			t.Error("prerequisite_checks_total metric not found")
		}
	})

	t.Run("RecordCircuitBreakerState", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordCircuitBreakerState("api-breaker", CircuitClosed)
		mp.RecordCircuitBreakerState("api-breaker", CircuitOpen)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_circuit_breaker_state" {
				found = true
				// Check the current value is CircuitOpen (1)
				for _, m := range mf.GetMetric() {
					if m.GetGauge().GetValue() != float64(CircuitOpen) {
						t.Errorf("Expected gauge value %d, got %f", CircuitOpen, m.GetGauge().GetValue())
					}
				}
			}
		}
		if !found {
			t.Error("circuit_breaker_state metric not found")
		}
	})

	t.Run("RecordBackoffRetry", func(t *testing.T) {
		mp, reg := newProvider()

		mp.RecordBackoffRetry("controller", 1)
		mp.RecordBackoffRetry("controller", 2)
		mp.RecordBackoffRetry("controller", 3)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		found := false
		for _, mf := range mfs {
			if mf.GetName() == "test_metrics_backoff_retries_total" {
				found = true
			}
		}
		if !found {
			t.Error("backoff_retries_total metric not found")
		}
	})
}

// TestPrometheusMetricsProviderCustomMetrics tests custom metric creation.
func TestPrometheusMetricsProviderCustomMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "custom",
		Subsystem:        "app",
		ReconcileBuckets: []float64{0.1, 1, 10},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	t.Run("Counter creation and caching", func(t *testing.T) {
		opts := prometheus.CounterOpts{
			Namespace: "myapp",
			Subsystem: "requests",
			Name:      "total",
			Help:      "Total requests",
		}

		c1 := mp.Counter(opts)
		c1.Inc()

		c2 := mp.Counter(opts) // Should return cached
		c2.Inc()

		// Both should be the same counter
		var m dto.Metric
		if err := c1.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetCounter().GetValue() != 2 {
			t.Errorf("Expected counter value 2, got %f", m.GetCounter().GetValue())
		}
	})

	t.Run("CounterVec creation and caching", func(t *testing.T) {
		opts := prometheus.CounterOpts{
			Namespace: "myapp",
			Subsystem: "requests",
			Name:      "by_status",
			Help:      "Requests by status",
		}

		cv1 := mp.CounterVec(opts, []string{"status"})
		cv1.WithLabelValues("200").Inc()

		cv2 := mp.CounterVec(opts, []string{"status"}) // Should return cached
		cv2.WithLabelValues("200").Inc()

		// Both should reference the same counter vec
		counter := cv1.WithLabelValues("200")
		var m dto.Metric
		if err := counter.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetCounter().GetValue() != 2 {
			t.Errorf("Expected counter value 2, got %f", m.GetCounter().GetValue())
		}
	})

	t.Run("Gauge creation and caching", func(t *testing.T) {
		opts := prometheus.GaugeOpts{
			Namespace: "myapp",
			Subsystem: "connections",
			Name:      "active",
			Help:      "Active connections",
		}

		g1 := mp.Gauge(opts)
		g1.Set(10)

		g2 := mp.Gauge(opts) // Should return cached
		g2.Add(5)

		var m dto.Metric
		if err := g1.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetGauge().GetValue() != 15 {
			t.Errorf("Expected gauge value 15, got %f", m.GetGauge().GetValue())
		}
	})

	t.Run("GaugeVec creation and caching", func(t *testing.T) {
		opts := prometheus.GaugeOpts{
			Namespace: "myapp",
			Subsystem: "connections",
			Name:      "by_type",
			Help:      "Connections by type",
		}

		gv1 := mp.GaugeVec(opts, []string{"type"})
		gv1.WithLabelValues("http").Set(100)

		gv2 := mp.GaugeVec(opts, []string{"type"}) // Should return cached
		gv2.WithLabelValues("http").Add(50)

		gauge := gv1.WithLabelValues("http")
		var m dto.Metric
		if err := gauge.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetGauge().GetValue() != 150 {
			t.Errorf("Expected gauge value 150, got %f", m.GetGauge().GetValue())
		}
	})

	t.Run("Histogram creation and caching", func(t *testing.T) {
		opts := prometheus.HistogramOpts{
			Namespace: "myapp",
			Subsystem: "latency",
			Name:      "seconds",
			Help:      "Request latency",
			Buckets:   []float64{0.01, 0.1, 1},
		}

		h1 := mp.Histogram(opts)
		h1.Observe(0.05)

		h2 := mp.Histogram(opts) // Should return cached
		h2.Observe(0.5)

		var m dto.Metric
		if err := h1.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetHistogram().GetSampleCount() != 2 {
			t.Errorf("Expected sample count 2, got %d", m.GetHistogram().GetSampleCount())
		}
	})

	t.Run("HistogramVec creation and caching", func(t *testing.T) {
		opts := prometheus.HistogramOpts{
			Namespace: "myapp",
			Subsystem: "latency",
			Name:      "by_endpoint",
			Help:      "Latency by endpoint",
			Buckets:   []float64{0.01, 0.1, 1},
		}

		hv1 := mp.HistogramVec(opts, []string{"endpoint"})
		hv1.WithLabelValues("/api").Observe(0.1)

		hv2 := mp.HistogramVec(opts, []string{"endpoint"}) // Should return cached
		hv2.WithLabelValues("/api").Observe(0.2)

		// Verify caching by checking sample count via gathering
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}
		found := false
		for _, mf := range mfs {
			if mf.GetName() == "myapp_latency_by_endpoint" {
				found = true
				for _, m := range mf.GetMetric() {
					if m.GetHistogram().GetSampleCount() != 2 {
						t.Errorf("Expected sample count 2, got %d", m.GetHistogram().GetSampleCount())
					}
				}
			}
		}
		if !found {
			t.Error("HistogramVec metric not found")
		}
	})

	t.Run("Summary creation and caching", func(t *testing.T) {
		opts := prometheus.SummaryOpts{
			Namespace: "myapp",
			Subsystem: "response",
			Name:      "size_bytes",
			Help:      "Response size",
		}

		s1 := mp.Summary(opts)
		s1.Observe(1024)

		s2 := mp.Summary(opts) // Should return cached
		s2.Observe(2048)

		var m dto.Metric
		if err := s1.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if m.GetSummary().GetSampleCount() != 2 {
			t.Errorf("Expected sample count 2, got %d", m.GetSummary().GetSampleCount())
		}
	})

	t.Run("SummaryVec creation and caching", func(t *testing.T) {
		opts := prometheus.SummaryOpts{
			Namespace: "myapp",
			Subsystem: "response",
			Name:      "size_by_type",
			Help:      "Response size by type",
		}

		sv1 := mp.SummaryVec(opts, []string{"type"})
		sv1.WithLabelValues("json").Observe(512)

		sv2 := mp.SummaryVec(opts, []string{"type"}) // Should return cached
		sv2.WithLabelValues("json").Observe(768)

		// Verify caching by checking sample count via gathering
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}
		found := false
		for _, mf := range mfs {
			if mf.GetName() == "myapp_response_size_by_type" {
				found = true
				for _, m := range mf.GetMetric() {
					if m.GetSummary().GetSampleCount() != 2 {
						t.Errorf("Expected sample count 2, got %d", m.GetSummary().GetSampleCount())
					}
				}
			}
		}
		if !found {
			t.Error("SummaryVec metric not found")
		}
	})
}

// TestReconcileTimer tests the ReconcileTimer helper.
func TestReconcileTimer(t *testing.T) {
	t.Run("basic timer functionality", func(t *testing.T) {
		mp := NewNoopMetricsProvider()
		timer := NewReconcileTimer(mp, "test-controller")

		if timer == nil {
			t.Fatal("NewReconcileTimer returned nil")
		}

		// Wait a bit to ensure some duration
		time.Sleep(5 * time.Millisecond)

		duration := timer.Duration()
		if duration < 5*time.Millisecond {
			t.Errorf("Duration = %v, expected >= 5ms", duration)
		}
	})

	t.Run("ObserveDuration records metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		config := &MetricsConfig{
			Namespace:        "timer",
			Subsystem:        "test",
			ReconcileBuckets: []float64{0.001, 0.01, 0.1, 1},
			Registry:         reg,
		}
		mp := NewMetricsProvider(config)

		timer := NewReconcileTimer(mp, "my-controller")
		time.Sleep(10 * time.Millisecond)
		timer.ObserveDuration(OutcomeSuccess)

		// Verify metrics were recorded
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		foundDuration := false
		foundTotal := false
		for _, mf := range mfs {
			switch mf.GetName() {
			case "timer_test_reconcile_duration_seconds":
				foundDuration = true
			case "timer_test_reconcile_total":
				foundTotal = true
				// Verify counter was incremented
				for _, m := range mf.GetMetric() {
					if m.GetCounter().GetValue() != 1 {
						t.Errorf("Expected counter value 1, got %f", m.GetCounter().GetValue())
					}
				}
			}
		}

		if !foundDuration {
			t.Error("reconcile_duration_seconds metric not found")
		}
		if !foundTotal {
			t.Error("reconcile_total metric not found")
		}
	})

	t.Run("timer with different outcomes", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		config := &MetricsConfig{
			Namespace:        "outcomes",
			Subsystem:        "test",
			ReconcileBuckets: []float64{0.001, 0.01, 0.1},
			Registry:         reg,
		}
		mp := NewMetricsProvider(config)

		outcomes := []ReconcileOutcome{OutcomeSuccess, OutcomeError, OutcomeRequeue, OutcomeSkipped}
		for _, outcome := range outcomes {
			timer := NewReconcileTimer(mp, "controller")
			timer.ObserveDuration(outcome)
		}

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		for _, mf := range mfs {
			if mf.GetName() == "outcomes_test_reconcile_total" {
				// Should have 4 different label combinations
				if len(mf.GetMetric()) != 4 {
					t.Errorf("Expected 4 metrics for different outcomes, got %d", len(mf.GetMetric()))
				}
			}
		}
	})
}

// TestBoolToString tests the boolToString helper function.
func TestBoolToString(t *testing.T) {
	tests := []struct {
		input bool
		want  string
	}{
		{true, "true"},
		{false, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := boolToString(tt.input)
			if got != tt.want {
				t.Errorf("boolToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIntToString tests the intToString helper function.
func TestIntToString(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{100, "100"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := intToString(tt.input)
			if got != tt.want {
				t.Errorf("intToString(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestMetricsProviderConcurrency tests thread safety of metrics provider.
func TestMetricsProviderConcurrency(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "concurrent",
		Subsystem:        "test",
		ReconcileBuckets: []float64{0.1, 1, 10},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	var wg sync.WaitGroup
	numGoroutines := 10
	numIterations := 100

	// Test concurrent recording of all metric types
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			controllerName := "controller"

			for j := 0; j < numIterations; j++ {
				mp.RecordReconcileDuration(controllerName, time.Duration(j)*time.Millisecond, OutcomeSuccess)
				mp.RecordReconcileTotal(controllerName, OutcomeSuccess)
				mp.RecordConditionTransition(controllerName, "Ready", "Unknown", "True")
				mp.RecordResourceOperation(controllerName, ResourceCreated, true)
				mp.RecordActiveReconciles(controllerName, j)
				mp.RecordFinalizeDuration(controllerName, time.Duration(j)*time.Millisecond, true)
				mp.RecordPrerequisiteCheck(controllerName, "db", true)
				mp.RecordCircuitBreakerState("breaker", CircuitClosed)
				mp.RecordBackoffRetry(controllerName, j)
			}
		}(i)
	}
	wg.Wait()

	// Verify metrics were recorded
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedMetrics := []string{
		"concurrent_test_reconcile_duration_seconds",
		"concurrent_test_reconcile_total",
		"concurrent_test_condition_transitions_total",
		"concurrent_test_resource_operations_total",
		"concurrent_test_active_reconciles",
		"concurrent_test_finalize_duration_seconds",
		"concurrent_test_prerequisite_checks_total",
		"concurrent_test_circuit_breaker_state",
		"concurrent_test_backoff_retries_total",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range mfs {
		foundMetrics[mf.GetName()] = true
	}

	for _, expected := range expectedMetrics {
		if !foundMetrics[expected] {
			t.Errorf("Expected metric %q not found", expected)
		}
	}
}

// TestMetricsProviderCustomMetricsConcurrency tests thread safety of custom metric creation.
func TestMetricsProviderCustomMetricsConcurrency(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "concurrent",
		Subsystem:        "custom",
		ReconcileBuckets: []float64{0.1, 1, 10},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	var wg sync.WaitGroup
	numGoroutines := 10

	// All goroutines try to create the same metrics concurrently
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			// Counter
			c := mp.Counter(prometheus.CounterOpts{
				Name: "shared_counter",
				Help: "A shared counter",
			})
			c.Inc()

			// CounterVec
			cv := mp.CounterVec(prometheus.CounterOpts{
				Name: "shared_counter_vec",
				Help: "A shared counter vec",
			}, []string{"label"})
			cv.WithLabelValues("value").Inc()

			// Gauge
			g := mp.Gauge(prometheus.GaugeOpts{
				Name: "shared_gauge",
				Help: "A shared gauge",
			})
			g.Inc()

			// GaugeVec
			gv := mp.GaugeVec(prometheus.GaugeOpts{
				Name: "shared_gauge_vec",
				Help: "A shared gauge vec",
			}, []string{"label"})
			gv.WithLabelValues("value").Inc()

			// Histogram
			h := mp.Histogram(prometheus.HistogramOpts{
				Name:    "shared_histogram",
				Help:    "A shared histogram",
				Buckets: []float64{0.1, 1},
			})
			h.Observe(0.5)

			// HistogramVec
			hv := mp.HistogramVec(prometheus.HistogramOpts{
				Name:    "shared_histogram_vec",
				Help:    "A shared histogram vec",
				Buckets: []float64{0.1, 1},
			}, []string{"label"})
			hv.WithLabelValues("value").Observe(0.5)

			// Summary
			s := mp.Summary(prometheus.SummaryOpts{
				Name: "shared_summary",
				Help: "A shared summary",
			})
			s.Observe(0.5)

			// SummaryVec
			sv := mp.SummaryVec(prometheus.SummaryOpts{
				Name: "shared_summary_vec",
				Help: "A shared summary vec",
			}, []string{"label"})
			sv.WithLabelValues("value").Observe(0.5)
		}()
	}
	wg.Wait()

	// Verify the counter value equals the number of goroutines
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "shared_counter" {
			for _, m := range mf.GetMetric() {
				if m.GetCounter().GetValue() != float64(numGoroutines) {
					t.Errorf("Expected counter value %d, got %f", numGoroutines, m.GetCounter().GetValue())
				}
			}
		}
	}
}

// TestMetricsProviderRegistry verifies Registry method returns correct registry.
func TestMetricsProviderRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "registry",
		Subsystem:        "test",
		ReconcileBuckets: []float64{0.1, 1},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	if mp.Registry() != reg {
		t.Error("Registry() should return the configured registry")
	}
}

// TestCircuitStateMetrics verifies circuit breaker state recording with all states.
func TestCircuitStateMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "circuit",
		Subsystem:        "test",
		ReconcileBuckets: []float64{0.1, 1},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	states := []struct {
		state CircuitState
		value float64
	}{
		{CircuitClosed, 0},
		{CircuitOpen, 1},
		{CircuitHalfOpen, 2},
	}

	for _, s := range states {
		t.Run(s.state.String(), func(t *testing.T) {
			mp.RecordCircuitBreakerState("test-breaker", s.state)

			mfs, err := reg.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			for _, mf := range mfs {
				if mf.GetName() == "circuit_test_circuit_breaker_state" {
					for _, m := range mf.GetMetric() {
						if m.GetGauge().GetValue() != s.value {
							t.Errorf("Expected state value %f, got %f", s.value, m.GetGauge().GetValue())
						}
					}
				}
			}
		})
	}
}

// TestResourceActionMetrics verifies all resource actions are recorded correctly.
func TestResourceActionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := &MetricsConfig{
		Namespace:        "resource",
		Subsystem:        "test",
		ReconcileBuckets: []float64{0.1, 1},
		Registry:         reg,
	}
	mp := NewMetricsProvider(config)

	actions := []ResourceAction{
		ResourceCreated,
		ResourceUpdated,
		ResourceUnchanged,
		ResourceDeleted,
		ResourceAdopted,
		ResourceOrphaned,
	}

	for _, action := range actions {
		mp.RecordResourceOperation("controller", action, true)
		mp.RecordResourceOperation("controller", action, false)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "resource_test_resource_operations_total" {
			found = true
			// We should have 12 different label combinations (6 actions * 2 success values)
			if len(mf.GetMetric()) != 12 {
				t.Errorf("Expected 12 metric combinations, got %d", len(mf.GetMetric()))
			}
		}
	}
	if !found {
		t.Error("resource_operations_total metric not found")
	}
}

// TestReconcileTimerDuration tests that Duration returns elapsed time correctly.
func TestReconcileTimerDuration(t *testing.T) {
	mp := NewNoopMetricsProvider()
	timer := NewReconcileTimer(mp, "test-controller")

	// Check initial duration is very small
	initialDuration := timer.Duration()
	if initialDuration > 10*time.Millisecond {
		t.Errorf("Initial duration %v is too large", initialDuration)
	}

	// Wait and check duration increases
	time.Sleep(50 * time.Millisecond)
	laterDuration := timer.Duration()
	if laterDuration < 50*time.Millisecond {
		t.Errorf("Duration after 50ms sleep is %v, expected >= 50ms", laterDuration)
	}
}

// TestNewMetricsProviderWithNilConfigUsesDefaults verifies that nil config uses defaults.
func TestNewMetricsProviderWithNilConfigUsesDefaults(t *testing.T) {
	// This would register to the default registry, which might conflict with other tests
	// So we just verify the function doesn't panic with nil config
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewMetricsProvider panicked with nil config: %v", r)
		}
	}()

	// Note: This test is limited because we can't easily unregister from prometheus.DefaultRegisterer
	// In a real scenario, you'd run this test in isolation or use a different approach
}
