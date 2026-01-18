package streamline

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component.
type HealthStatus int

const (
	// HealthStatusHealthy indicates the component is healthy.
	HealthStatusHealthy HealthStatus = iota

	// HealthStatusDegraded indicates the component is partially healthy.
	HealthStatusDegraded

	// HealthStatusUnhealthy indicates the component is unhealthy.
	HealthStatusUnhealthy

	// HealthStatusUnknown indicates the health status is unknown.
	HealthStatusUnknown
)

// String returns a string representation of the health status.
func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	case HealthStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	// Status is the health status.
	Status HealthStatus

	// Message provides additional details.
	Message string

	// Timestamp is when the check was performed.
	Timestamp time.Time

	// Duration is how long the check took.
	Duration time.Duration

	// Error is any error that occurred during the check.
	Error error

	// Details contains additional key-value details.
	Details map[string]interface{}
}

// IsHealthy returns true if the status is Healthy.
func (r HealthCheckResult) IsHealthy() bool {
	return r.Status == HealthStatusHealthy
}

// HealthCheck is a function that performs a health check.
type HealthCheck func(ctx context.Context) HealthCheckResult

// HealthChecker manages health checks for a controller.
type HealthChecker interface {
	// Register adds a health check.
	Register(name string, check HealthCheck)

	// RegisterWithTimeout adds a health check with a timeout.
	RegisterWithTimeout(name string, check HealthCheck, timeout time.Duration)

	// Unregister removes a health check.
	Unregister(name string)

	// Check performs a single health check by name.
	Check(ctx context.Context, name string) HealthCheckResult

	// CheckAll performs all health checks.
	CheckAll(ctx context.Context) map[string]HealthCheckResult

	// IsHealthy returns true if all checks pass.
	IsHealthy(ctx context.Context) bool

	// OverallStatus returns the overall health status.
	OverallStatus(ctx context.Context) HealthStatus

	// GetStatus returns the cached status without running checks.
	GetStatus() map[string]HealthCheckResult

	// SetStatus manually sets a status (for internal use).
	SetStatus(name string, result HealthCheckResult)
}

// healthChecker implements HealthChecker.
type healthChecker struct {
	checks   map[string]healthCheckEntry
	results  map[string]HealthCheckResult
	mu       sync.RWMutex
	cacheTTL time.Duration
}

type healthCheckEntry struct {
	check   HealthCheck
	timeout time.Duration
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker() HealthChecker {
	return &healthChecker{
		checks:   make(map[string]healthCheckEntry),
		results:  make(map[string]HealthCheckResult),
		cacheTTL: 10 * time.Second,
	}
}

// WithCacheTTL sets the cache TTL for health check results.
func (hc *healthChecker) WithCacheTTL(ttl time.Duration) *healthChecker {
	hc.cacheTTL = ttl
	return hc
}

func (hc *healthChecker) Register(name string, check HealthCheck) {
	hc.RegisterWithTimeout(name, check, 5*time.Second)
}

func (hc *healthChecker) RegisterWithTimeout(name string, check HealthCheck, timeout time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.checks[name] = healthCheckEntry{
		check:   check,
		timeout: timeout,
	}
}

func (hc *healthChecker) Unregister(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.checks, name)
	delete(hc.results, name)
}

func (hc *healthChecker) Check(ctx context.Context, name string) HealthCheckResult {
	hc.mu.RLock()
	entry, exists := hc.checks[name]
	hc.mu.RUnlock()

	if !exists {
		return HealthCheckResult{
			Status:    HealthStatusUnknown,
			Message:   "health check not found",
			Timestamp: time.Now(),
		}
	}

	return hc.runCheck(ctx, name, entry)
}

func (hc *healthChecker) CheckAll(ctx context.Context) map[string]HealthCheckResult {
	hc.mu.RLock()
	checks := make(map[string]healthCheckEntry, len(hc.checks))
	for name, entry := range hc.checks {
		checks[name] = entry
	}
	hc.mu.RUnlock()

	results := make(map[string]HealthCheckResult, len(checks))
	for name, entry := range checks {
		results[name] = hc.runCheck(ctx, name, entry)
	}

	return results
}

func (hc *healthChecker) runCheck(ctx context.Context, name string, entry healthCheckEntry) HealthCheckResult {
	// Check cache
	hc.mu.RLock()
	if cached, exists := hc.results[name]; exists {
		if time.Since(cached.Timestamp) < hc.cacheTTL {
			hc.mu.RUnlock()
			return cached
		}
	}
	hc.mu.RUnlock()

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, entry.timeout)
	defer cancel()

	// Run check
	start := time.Now()
	resultCh := make(chan HealthCheckResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- HealthCheckResult{
					Status:    HealthStatusUnhealthy,
					Message:   fmt.Sprintf("panic: %v", r),
					Timestamp: time.Now(),
					Duration:  time.Since(start),
				}
			}
		}()
		resultCh <- entry.check(checkCtx)
	}()

	var result HealthCheckResult
	select {
	case result = <-resultCh:
		result.Duration = time.Since(start)
		result.Timestamp = time.Now()
	case <-checkCtx.Done():
		result = HealthCheckResult{
			Status:    HealthStatusUnhealthy,
			Message:   "health check timed out",
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Error:     checkCtx.Err(),
		}
	}

	// Cache result
	hc.mu.Lock()
	hc.results[name] = result
	hc.mu.Unlock()

	return result
}

func (hc *healthChecker) IsHealthy(ctx context.Context) bool {
	results := hc.CheckAll(ctx)
	for _, result := range results {
		if result.Status == HealthStatusUnhealthy {
			return false
		}
	}
	return true
}

func (hc *healthChecker) OverallStatus(ctx context.Context) HealthStatus {
	results := hc.CheckAll(ctx)
	if len(results) == 0 {
		return HealthStatusUnknown
	}

	hasDegraded := false
	for _, result := range results {
		switch result.Status {
		case HealthStatusUnhealthy:
			return HealthStatusUnhealthy
		case HealthStatusDegraded:
			hasDegraded = true
		case HealthStatusUnknown:
			hasDegraded = true
		}
	}

	if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

func (hc *healthChecker) GetStatus() map[string]HealthCheckResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	results := make(map[string]HealthCheckResult, len(hc.results))
	for name, result := range hc.results {
		results[name] = result
	}
	return results
}

func (hc *healthChecker) SetStatus(name string, result HealthCheckResult) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	result.Timestamp = time.Now()
	hc.results[name] = result
}

// HealthReporter provides a simplified interface for controllers to report health.
type HealthReporter interface {
	// ReportHealthy reports that the component is healthy.
	ReportHealthy(component string)

	// ReportHealthyWithMessage reports healthy with a message.
	ReportHealthyWithMessage(component, message string)

	// ReportDegraded reports that the component is degraded.
	ReportDegraded(component, reason string)

	// ReportUnhealthy reports that the component is unhealthy.
	ReportUnhealthy(component, reason string)

	// ReportError reports an error.
	ReportError(component string, err error)

	// Clear removes the health status for a component.
	Clear(component string)
}

// healthReporter implements HealthReporter.
type healthReporter struct {
	checker HealthChecker
}

// NewHealthReporter creates a new HealthReporter.
func NewHealthReporter(checker HealthChecker) HealthReporter {
	return &healthReporter{checker: checker}
}

func (hr *healthReporter) ReportHealthy(component string) {
	hr.checker.SetStatus(component, HealthCheckResult{
		Status:  HealthStatusHealthy,
		Message: "healthy",
	})
}

func (hr *healthReporter) ReportHealthyWithMessage(component, message string) {
	hr.checker.SetStatus(component, HealthCheckResult{
		Status:  HealthStatusHealthy,
		Message: message,
	})
}

func (hr *healthReporter) ReportDegraded(component, reason string) {
	hr.checker.SetStatus(component, HealthCheckResult{
		Status:  HealthStatusDegraded,
		Message: reason,
	})
}

func (hr *healthReporter) ReportUnhealthy(component, reason string) {
	hr.checker.SetStatus(component, HealthCheckResult{
		Status:  HealthStatusUnhealthy,
		Message: reason,
	})
}

func (hr *healthReporter) ReportError(component string, err error) {
	hr.checker.SetStatus(component, HealthCheckResult{
		Status:  HealthStatusUnhealthy,
		Message: err.Error(),
		Error:   err,
	})
}

func (hr *healthReporter) Clear(component string) {
	hr.checker.Unregister(component)
}

// Common health checks

// PingCheck creates a health check that always returns healthy.
func PingCheck() HealthCheck {
	return func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{
			Status:  HealthStatusHealthy,
			Message: "pong",
		}
	}
}

// HTTPCheck creates a health check that makes an HTTP request.
func HTTPCheck(url string, timeout time.Duration) HealthCheck {
	return func(ctx context.Context) HealthCheckResult {
		client := &http.Client{Timeout: timeout}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("failed to create request: %v", err),
				Error:   err,
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("request failed: %v", err),
				Error:   err,
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return HealthCheckResult{
				Status:  HealthStatusHealthy,
				Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		}

		if resp.StatusCode >= 500 {
			return HealthCheckResult{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		}

		return HealthCheckResult{
			Status:  HealthStatusDegraded,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}
}

// ThresholdCheck creates a health check based on a numeric value.
func ThresholdCheck(name string, getValue func() float64, healthy, warning float64) HealthCheck {
	return func(ctx context.Context) HealthCheckResult {
		value := getValue()

		details := map[string]interface{}{
			"value":   value,
			"healthy": healthy,
			"warning": warning,
		}

		if value <= healthy {
			return HealthCheckResult{
				Status:  HealthStatusHealthy,
				Message: fmt.Sprintf("%s: %.2f (threshold: %.2f)", name, value, healthy),
				Details: details,
			}
		}

		if value <= warning {
			return HealthCheckResult{
				Status:  HealthStatusDegraded,
				Message: fmt.Sprintf("%s: %.2f exceeds healthy threshold %.2f", name, value, healthy),
				Details: details,
			}
		}

		return HealthCheckResult{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("%s: %.2f exceeds warning threshold %.2f", name, value, warning),
			Details: details,
		}
	}
}

// CompositeCheck creates a health check from multiple checks.
func CompositeCheck(checks map[string]HealthCheck) HealthCheck {
	return func(ctx context.Context) HealthCheckResult {
		results := make(map[string]interface{})
		overallStatus := HealthStatusHealthy
		var messages []string

		for name, check := range checks {
			result := check(ctx)
			results[name] = result

			if result.Status > overallStatus {
				overallStatus = result.Status
			}

			if result.Status != HealthStatusHealthy {
				messages = append(messages, fmt.Sprintf("%s: %s", name, result.Message))
			}
		}

		message := "all checks passed"
		if len(messages) > 0 {
			message = fmt.Sprintf("%d issues: %v", len(messages), messages)
		}

		return HealthCheckResult{
			Status:  overallStatus,
			Message: message,
			Details: results,
		}
	}
}

// HealthHandler returns an HTTP handler for health checks.
func HealthHandler(checker HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := checker.OverallStatus(ctx)

		switch status {
		case HealthStatusHealthy:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("healthy"))
		case HealthStatusDegraded:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("degraded"))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unhealthy"))
		}
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks.
func ReadinessHandler(checker HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if checker.IsHealthy(ctx) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	}
}

// LivenessHandler returns an HTTP handler for liveness checks.
// Liveness is simpler than readiness - it just needs to respond.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("alive"))
	}
}
