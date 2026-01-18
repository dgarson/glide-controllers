package streamline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// HealthStatus Tests
// =============================================================================

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   HealthStatus
		expected string
	}{
		{
			name:     "healthy status",
			status:   HealthStatusHealthy,
			expected: "healthy",
		},
		{
			name:     "degraded status",
			status:   HealthStatusDegraded,
			expected: "degraded",
		},
		{
			name:     "unhealthy status",
			status:   HealthStatusUnhealthy,
			expected: "unhealthy",
		},
		{
			name:     "unknown status",
			status:   HealthStatusUnknown,
			expected: "unknown",
		},
		{
			name:     "invalid status value",
			status:   HealthStatus(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("HealthStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHealthStatus_Ordering(t *testing.T) {
	// Verify that status values are ordered correctly for comparison
	if HealthStatusHealthy >= HealthStatusDegraded {
		t.Error("HealthStatusHealthy should be less than HealthStatusDegraded")
	}
	if HealthStatusDegraded >= HealthStatusUnhealthy {
		t.Error("HealthStatusDegraded should be less than HealthStatusUnhealthy")
	}
	if HealthStatusUnhealthy >= HealthStatusUnknown {
		t.Error("HealthStatusUnhealthy should be less than HealthStatusUnknown")
	}
}

// =============================================================================
// HealthCheckResult Tests
// =============================================================================

func TestHealthCheckResult_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		result   HealthCheckResult
		expected bool
	}{
		{
			name: "healthy result",
			result: HealthCheckResult{
				Status: HealthStatusHealthy,
			},
			expected: true,
		},
		{
			name: "degraded result",
			result: HealthCheckResult{
				Status: HealthStatusDegraded,
			},
			expected: false,
		},
		{
			name: "unhealthy result",
			result: HealthCheckResult{
				Status: HealthStatusUnhealthy,
			},
			expected: false,
		},
		{
			name: "unknown result",
			result: HealthCheckResult{
				Status: HealthStatusUnknown,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsHealthy(); got != tt.expected {
				t.Errorf("HealthCheckResult.IsHealthy() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHealthCheckResult_Fields(t *testing.T) {
	now := time.Now()
	testErr := errors.New("test error")
	result := HealthCheckResult{
		Status:    HealthStatusDegraded,
		Message:   "test message",
		Timestamp: now,
		Duration:  100 * time.Millisecond,
		Error:     testErr,
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	if result.Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusDegraded)
	}
	if result.Message != "test message" {
		t.Errorf("Message = %v, want %v", result.Message, "test message")
	}
	if !result.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", result.Timestamp, now)
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want %v", result.Duration, 100*time.Millisecond)
	}
	if result.Error != testErr {
		t.Errorf("Error = %v, want %v", result.Error, testErr)
	}
	if result.Details["key"] != "value" {
		t.Errorf("Details[key] = %v, want %v", result.Details["key"], "value")
	}
}

// =============================================================================
// HealthChecker Tests
// =============================================================================

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker()
	if hc == nil {
		t.Fatal("NewHealthChecker() returned nil")
	}

	// Should return empty status initially
	status := hc.GetStatus()
	if len(status) != 0 {
		t.Errorf("GetStatus() = %v, want empty map", status)
	}
}

func TestHealthChecker_WithCacheTTL(t *testing.T) {
	hc := NewHealthChecker().(*healthChecker)

	// Default TTL should be 10 seconds
	if hc.cacheTTL != 10*time.Second {
		t.Errorf("default cacheTTL = %v, want %v", hc.cacheTTL, 10*time.Second)
	}

	// Set custom TTL
	result := hc.WithCacheTTL(30 * time.Second)
	if result != hc {
		t.Error("WithCacheTTL should return the same healthChecker")
	}
	if hc.cacheTTL != 30*time.Second {
		t.Errorf("cacheTTL = %v, want %v", hc.cacheTTL, 30*time.Second)
	}
}

func TestHealthChecker_Register(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Register a simple check
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{
			Status:  HealthStatusHealthy,
			Message: "test passed",
		}
	})

	// Verify check is registered
	result := hc.Check(ctx, "test")
	if result.Status != HealthStatusHealthy {
		t.Errorf("Check result status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Message != "test passed" {
		t.Errorf("Check result message = %v, want %v", result.Message, "test passed")
	}
}

func TestHealthChecker_RegisterWithTimeout(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Register with custom timeout
	hc.RegisterWithTimeout("slow-check", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{
			Status:  HealthStatusHealthy,
			Message: "completed",
		}
	}, 30*time.Second)

	result := hc.Check(ctx, "slow-check")
	if result.Status != HealthStatusHealthy {
		t.Errorf("Check result status = %v, want %v", result.Status, HealthStatusHealthy)
	}
}

func TestHealthChecker_Unregister(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Register and verify
	hc.Register("to-remove", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy}
	})
	result := hc.Check(ctx, "to-remove")
	if result.Status != HealthStatusHealthy {
		t.Fatal("Check should be registered")
	}

	// Unregister
	hc.Unregister("to-remove")

	// Verify check is gone
	result = hc.Check(ctx, "to-remove")
	if result.Status != HealthStatusUnknown {
		t.Errorf("After unregister, status = %v, want %v", result.Status, HealthStatusUnknown)
	}
	if result.Message != "health check not found" {
		t.Errorf("After unregister, message = %v, want 'health check not found'", result.Message)
	}
}

func TestHealthChecker_Check_NotFound(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	result := hc.Check(ctx, "non-existent")
	if result.Status != HealthStatusUnknown {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnknown)
	}
	if result.Message != "health check not found" {
		t.Errorf("Message = %v, want 'health check not found'", result.Message)
	}
	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestHealthChecker_Check_Caching(t *testing.T) {
	hc := NewHealthChecker().(*healthChecker)
	hc.WithCacheTTL(100 * time.Millisecond)
	ctx := context.Background()

	callCount := 0
	hc.Register("cached", func(ctx context.Context) HealthCheckResult {
		callCount++
		return HealthCheckResult{
			Status:  HealthStatusHealthy,
			Message: fmt.Sprintf("call %d", callCount),
		}
	})

	// First call
	result1 := hc.Check(ctx, "cached")
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}

	// Second call within TTL - should use cache
	result2 := hc.Check(ctx, "cached")
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (cached)", callCount)
	}
	if result2.Message != result1.Message {
		t.Error("Cached result should have same message")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third call after TTL - should refresh
	result3 := hc.Check(ctx, "cached")
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (cache expired)", callCount)
	}
	if result3.Message == result1.Message {
		t.Error("After cache expiry, result should be different")
	}
}

func TestHealthChecker_Check_Timeout(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Register a slow check with a short timeout
	hc.RegisterWithTimeout("slow", func(ctx context.Context) HealthCheckResult {
		select {
		case <-time.After(5 * time.Second):
			return HealthCheckResult{Status: HealthStatusHealthy}
		case <-ctx.Done():
			return HealthCheckResult{Status: HealthStatusUnhealthy, Error: ctx.Err()}
		}
	}, 50*time.Millisecond)

	start := time.Now()
	result := hc.Check(ctx, "slow")
	elapsed := time.Since(start)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
	if result.Message != "health check timed out" {
		t.Errorf("Message = %v, want 'health check timed out'", result.Message)
	}
	if result.Error == nil {
		t.Error("Error should not be nil for timeout")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Check took %v, should timeout around 50ms", elapsed)
	}
}

func TestHealthChecker_Check_Panic(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	hc.Register("panicking", func(ctx context.Context) HealthCheckResult {
		panic("test panic")
	})

	result := hc.Check(ctx, "panicking")
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
	if result.Message != "panic: test panic" {
		t.Errorf("Message = %v, want 'panic: test panic'", result.Message)
	}
}

func TestHealthChecker_CheckAll(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	hc.Register("check1", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok1"}
	})
	hc.Register("check2", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusDegraded, Message: "ok2"}
	})
	hc.Register("check3", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusUnhealthy, Message: "ok3"}
	})

	results := hc.CheckAll(ctx)

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	if results["check1"].Status != HealthStatusHealthy {
		t.Errorf("check1 status = %v, want %v", results["check1"].Status, HealthStatusHealthy)
	}
	if results["check2"].Status != HealthStatusDegraded {
		t.Errorf("check2 status = %v, want %v", results["check2"].Status, HealthStatusDegraded)
	}
	if results["check3"].Status != HealthStatusUnhealthy {
		t.Errorf("check3 status = %v, want %v", results["check3"].Status, HealthStatusUnhealthy)
	}
}

func TestHealthChecker_CheckAll_Empty(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	results := hc.CheckAll(ctx)
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestHealthChecker_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		statuses []HealthStatus
		expected bool
	}{
		{
			name:     "all healthy",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusHealthy},
			expected: true,
		},
		{
			name:     "one degraded",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusDegraded},
			expected: true, // Degraded is not unhealthy
		},
		{
			name:     "one unhealthy",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusUnhealthy},
			expected: false,
		},
		{
			name:     "empty checks",
			statuses: []HealthStatus{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := NewHealthChecker()
			ctx := context.Background()

			for i, status := range tt.statuses {
				s := status // capture
				hc.Register(fmt.Sprintf("check%d", i), func(ctx context.Context) HealthCheckResult {
					return HealthCheckResult{Status: s}
				})
			}

			if got := hc.IsHealthy(ctx); got != tt.expected {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHealthChecker_OverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []HealthStatus
		expected HealthStatus
	}{
		{
			name:     "empty checks",
			statuses: []HealthStatus{},
			expected: HealthStatusUnknown,
		},
		{
			name:     "all healthy",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusHealthy},
			expected: HealthStatusHealthy,
		},
		{
			name:     "one degraded",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusDegraded},
			expected: HealthStatusDegraded,
		},
		{
			name:     "one unknown",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusUnknown},
			expected: HealthStatusDegraded, // Unknown is treated as degraded
		},
		{
			name:     "one unhealthy",
			statuses: []HealthStatus{HealthStatusHealthy, HealthStatusUnhealthy},
			expected: HealthStatusUnhealthy,
		},
		{
			name:     "unhealthy takes precedence",
			statuses: []HealthStatus{HealthStatusDegraded, HealthStatusUnhealthy, HealthStatusUnknown},
			expected: HealthStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := NewHealthChecker()
			ctx := context.Background()

			for i, status := range tt.statuses {
				s := status // capture
				hc.Register(fmt.Sprintf("check%d", i), func(ctx context.Context) HealthCheckResult {
					return HealthCheckResult{Status: s}
				})
			}

			if got := hc.OverallStatus(ctx); got != tt.expected {
				t.Errorf("OverallStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHealthChecker_GetStatus(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Initially empty
	if len(hc.GetStatus()) != 0 {
		t.Error("GetStatus() should be empty initially")
	}

	// Register and run a check
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok"}
	})
	hc.Check(ctx, "test")

	// Now should have cached result
	status := hc.GetStatus()
	if len(status) != 1 {
		t.Errorf("len(status) = %d, want 1", len(status))
	}
	if status["test"].Status != HealthStatusHealthy {
		t.Errorf("test status = %v, want %v", status["test"].Status, HealthStatusHealthy)
	}
}

func TestHealthChecker_SetStatus(t *testing.T) {
	hc := NewHealthChecker()

	// Set status directly
	hc.SetStatus("manual", HealthCheckResult{
		Status:  HealthStatusDegraded,
		Message: "manually set",
	})

	status := hc.GetStatus()
	if len(status) != 1 {
		t.Errorf("len(status) = %d, want 1", len(status))
	}
	if status["manual"].Status != HealthStatusDegraded {
		t.Errorf("manual status = %v, want %v", status["manual"].Status, HealthStatusDegraded)
	}
	if status["manual"].Message != "manually set" {
		t.Errorf("manual message = %v, want 'manually set'", status["manual"].Message)
	}
	if status["manual"].Timestamp.IsZero() {
		t.Error("Timestamp should be set automatically")
	}
}

func TestHealthChecker_Concurrency(t *testing.T) {
	hc := NewHealthChecker()
	ctx := context.Background()

	// Register a check
	hc.Register("concurrent", func(ctx context.Context) HealthCheckResult {
		time.Sleep(10 * time.Millisecond)
		return HealthCheckResult{Status: HealthStatusHealthy}
	})

	// Run many concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 5 {
			case 0:
				hc.Check(ctx, "concurrent")
			case 1:
				hc.CheckAll(ctx)
			case 2:
				hc.IsHealthy(ctx)
			case 3:
				hc.GetStatus()
			case 4:
				hc.SetStatus("manual", HealthCheckResult{Status: HealthStatusHealthy})
			}
		}(i)
	}

	wg.Wait()
	// If we get here without a race detector complaint, we're good
}

// =============================================================================
// HealthReporter Tests
// =============================================================================

func TestNewHealthReporter(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)
	if hr == nil {
		t.Fatal("NewHealthReporter() returned nil")
	}
}

func TestHealthReporter_ReportHealthy(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	hr.ReportHealthy("component1")

	status := hc.GetStatus()
	if status["component1"].Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", status["component1"].Status, HealthStatusHealthy)
	}
	if status["component1"].Message != "healthy" {
		t.Errorf("Message = %v, want 'healthy'", status["component1"].Message)
	}
}

func TestHealthReporter_ReportHealthyWithMessage(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	hr.ReportHealthyWithMessage("component1", "all systems go")

	status := hc.GetStatus()
	if status["component1"].Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", status["component1"].Status, HealthStatusHealthy)
	}
	if status["component1"].Message != "all systems go" {
		t.Errorf("Message = %v, want 'all systems go'", status["component1"].Message)
	}
}

func TestHealthReporter_ReportDegraded(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	hr.ReportDegraded("component1", "high latency")

	status := hc.GetStatus()
	if status["component1"].Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v", status["component1"].Status, HealthStatusDegraded)
	}
	if status["component1"].Message != "high latency" {
		t.Errorf("Message = %v, want 'high latency'", status["component1"].Message)
	}
}

func TestHealthReporter_ReportUnhealthy(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	hr.ReportUnhealthy("component1", "connection lost")

	status := hc.GetStatus()
	if status["component1"].Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", status["component1"].Status, HealthStatusUnhealthy)
	}
	if status["component1"].Message != "connection lost" {
		t.Errorf("Message = %v, want 'connection lost'", status["component1"].Message)
	}
}

func TestHealthReporter_ReportError(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)
	testErr := errors.New("database connection failed")

	hr.ReportError("component1", testErr)

	status := hc.GetStatus()
	if status["component1"].Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", status["component1"].Status, HealthStatusUnhealthy)
	}
	if status["component1"].Message != "database connection failed" {
		t.Errorf("Message = %v, want 'database connection failed'", status["component1"].Message)
	}
	if status["component1"].Error != testErr {
		t.Errorf("Error = %v, want %v", status["component1"].Error, testErr)
	}
}

func TestHealthReporter_Clear(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	// Set a status
	hr.ReportHealthy("component1")
	if len(hc.GetStatus()) != 1 {
		t.Fatal("Status should be set")
	}

	// Clear it
	hr.Clear("component1")
	if len(hc.GetStatus()) != 0 {
		t.Error("Status should be cleared")
	}
}

func TestHealthReporter_StatusTransitions(t *testing.T) {
	hc := NewHealthChecker()
	hr := NewHealthReporter(hc)

	// Start healthy
	hr.ReportHealthy("service")
	if hc.GetStatus()["service"].Status != HealthStatusHealthy {
		t.Error("Should be healthy")
	}

	// Transition to degraded
	hr.ReportDegraded("service", "slow")
	if hc.GetStatus()["service"].Status != HealthStatusDegraded {
		t.Error("Should be degraded")
	}

	// Transition to unhealthy
	hr.ReportUnhealthy("service", "down")
	if hc.GetStatus()["service"].Status != HealthStatusUnhealthy {
		t.Error("Should be unhealthy")
	}

	// Recover to healthy
	hr.ReportHealthy("service")
	if hc.GetStatus()["service"].Status != HealthStatusHealthy {
		t.Error("Should be healthy again")
	}
}

// =============================================================================
// Built-in Health Checks Tests
// =============================================================================

func TestPingCheck(t *testing.T) {
	check := PingCheck()
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Message != "pong" {
		t.Errorf("Message = %v, want 'pong'", result.Message)
	}
}

func TestHTTPCheck_Success(t *testing.T) {
	// Create a test server that returns 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	check := HTTPCheck(server.URL, 5*time.Second)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Message != "HTTP 200" {
		t.Errorf("Message = %v, want 'HTTP 200'", result.Message)
	}
}

func TestHTTPCheck_2xxSuccess(t *testing.T) {
	// Test various 2xx status codes
	codes := []int{200, 201, 202, 204}
	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			check := HTTPCheck(server.URL, 5*time.Second)
			result := check(context.Background())

			if result.Status != HealthStatusHealthy {
				t.Errorf("Status = %v, want %v for HTTP %d", result.Status, HealthStatusHealthy, code)
			}
		})
	}
}

func TestHTTPCheck_ServerError(t *testing.T) {
	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	check := HTTPCheck(server.URL, 5*time.Second)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
	if result.Message != "HTTP 500" {
		t.Errorf("Message = %v, want 'HTTP 500'", result.Message)
	}
}

func TestHTTPCheck_5xxErrors(t *testing.T) {
	codes := []int{500, 502, 503, 504}
	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			check := HTTPCheck(server.URL, 5*time.Second)
			result := check(context.Background())

			if result.Status != HealthStatusUnhealthy {
				t.Errorf("Status = %v, want %v for HTTP %d", result.Status, HealthStatusUnhealthy, code)
			}
		})
	}
}

func TestHTTPCheck_ClientError(t *testing.T) {
	// Create a test server that returns 4xx
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	check := HTTPCheck(server.URL, 5*time.Second)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusDegraded)
	}
	if result.Message != "HTTP 404" {
		t.Errorf("Message = %v, want 'HTTP 404'", result.Message)
	}
}

func TestHTTPCheck_ConnectionError(t *testing.T) {
	// Use an invalid URL that will fail to connect
	check := HTTPCheck("http://localhost:59999", 100*time.Millisecond)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
	if result.Error == nil {
		t.Error("Error should not be nil")
	}
}

func TestHTTPCheck_InvalidURL(t *testing.T) {
	check := HTTPCheck("://invalid", 5*time.Second)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
	if result.Error == nil {
		t.Error("Error should not be nil for invalid URL")
	}
}

func TestThresholdCheck_Healthy(t *testing.T) {
	getValue := func() float64 { return 50.0 }
	check := ThresholdCheck("cpu", getValue, 70.0, 90.0)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Details == nil {
		t.Fatal("Details should not be nil")
	}
	if result.Details["value"] != 50.0 {
		t.Errorf("Details[value] = %v, want 50.0", result.Details["value"])
	}
	if result.Details["healthy"] != 70.0 {
		t.Errorf("Details[healthy] = %v, want 70.0", result.Details["healthy"])
	}
	if result.Details["warning"] != 90.0 {
		t.Errorf("Details[warning] = %v, want 90.0", result.Details["warning"])
	}
}

func TestThresholdCheck_AtHealthyThreshold(t *testing.T) {
	getValue := func() float64 { return 70.0 }
	check := ThresholdCheck("cpu", getValue, 70.0, 90.0)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v (at threshold is healthy)", result.Status, HealthStatusHealthy)
	}
}

func TestThresholdCheck_Degraded(t *testing.T) {
	getValue := func() float64 { return 80.0 }
	check := ThresholdCheck("cpu", getValue, 70.0, 90.0)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusDegraded)
	}
}

func TestThresholdCheck_AtWarningThreshold(t *testing.T) {
	getValue := func() float64 { return 90.0 }
	check := ThresholdCheck("cpu", getValue, 70.0, 90.0)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v (at warning threshold is degraded)", result.Status, HealthStatusDegraded)
	}
}

func TestThresholdCheck_Unhealthy(t *testing.T) {
	getValue := func() float64 { return 95.0 }
	check := ThresholdCheck("cpu", getValue, 70.0, 90.0)
	ctx := context.Background()

	result := check(ctx)
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
}

func TestCompositeCheck_AllHealthy(t *testing.T) {
	checks := map[string]HealthCheck{
		"check1": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok1"}
		},
		"check2": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok2"}
		},
	}

	composite := CompositeCheck(checks)
	ctx := context.Background()

	result := composite(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Message != "all checks passed" {
		t.Errorf("Message = %v, want 'all checks passed'", result.Message)
	}
	if result.Details == nil {
		t.Error("Details should not be nil")
	}
	if len(result.Details) != 2 {
		t.Errorf("len(Details) = %d, want 2", len(result.Details))
	}
}

func TestCompositeCheck_OneDegraded(t *testing.T) {
	checks := map[string]HealthCheck{
		"check1": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok"}
		},
		"check2": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusDegraded, Message: "slow"}
		},
	}

	composite := CompositeCheck(checks)
	ctx := context.Background()

	result := composite(ctx)
	if result.Status != HealthStatusDegraded {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusDegraded)
	}
}

func TestCompositeCheck_OneUnhealthy(t *testing.T) {
	checks := map[string]HealthCheck{
		"check1": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusHealthy, Message: "ok"}
		},
		"check2": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusDegraded, Message: "slow"}
		},
		"check3": func(ctx context.Context) HealthCheckResult {
			return HealthCheckResult{Status: HealthStatusUnhealthy, Message: "down"}
		},
	}

	composite := CompositeCheck(checks)
	ctx := context.Background()

	result := composite(ctx)
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusUnhealthy)
	}
}

func TestCompositeCheck_Empty(t *testing.T) {
	checks := map[string]HealthCheck{}

	composite := CompositeCheck(checks)
	ctx := context.Background()

	result := composite(ctx)
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Message != "all checks passed" {
		t.Errorf("Message = %v, want 'all checks passed'", result.Message)
	}
}

// =============================================================================
// HTTP Handler Tests
// =============================================================================

func TestHealthHandler_Healthy(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy}
	})

	handler := HealthHandler(hc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "healthy" {
		t.Errorf("Body = %s, want 'healthy'", w.Body.String())
	}
}

func TestHealthHandler_Degraded(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusDegraded}
	})

	handler := HealthHandler(hc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "degraded" {
		t.Errorf("Body = %s, want 'degraded'", w.Body.String())
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusUnhealthy}
	})

	handler := HealthHandler(hc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if w.Body.String() != "unhealthy" {
		t.Errorf("Body = %s, want 'unhealthy'", w.Body.String())
	}
}

func TestHealthHandler_Unknown(t *testing.T) {
	hc := NewHealthChecker()
	// No checks registered, so status is unknown

	handler := HealthHandler(hc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Unknown is treated as unhealthy
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if w.Body.String() != "unhealthy" {
		t.Errorf("Body = %s, want 'unhealthy'", w.Body.String())
	}
}

func TestReadinessHandler_Ready(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy}
	})

	handler := ReadinessHandler(hc)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ready" {
		t.Errorf("Body = %s, want 'ready'", w.Body.String())
	}
}

func TestReadinessHandler_NotReady(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusUnhealthy}
	})

	handler := ReadinessHandler(hc)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if w.Body.String() != "not ready" {
		t.Errorf("Body = %s, want 'not ready'", w.Body.String())
	}
}

func TestReadinessHandler_DegradedIsReady(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("test", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusDegraded}
	})

	handler := ReadinessHandler(hc)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Degraded is still considered ready (not unhealthy)
	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ready" {
		t.Errorf("Body = %s, want 'ready'", w.Body.String())
	}
}

func TestLivenessHandler(t *testing.T) {
	handler := LivenessHandler()
	req := httptest.NewRequest("GET", "/live", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "alive" {
		t.Errorf("Body = %s, want 'alive'", w.Body.String())
	}
}

func TestLivenessHandler_MultipleRequests(t *testing.T) {
	handler := LivenessHandler()

	// Liveness should always respond the same way
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/live", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Status code = %d, want %d", i, w.Code, http.StatusOK)
		}
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestHealthChecker_FullWorkflow(t *testing.T) {
	hc := NewHealthChecker().(*healthChecker)
	hc.WithCacheTTL(50 * time.Millisecond)
	hr := NewHealthReporter(hc)
	ctx := context.Background()

	// Register various checks
	hc.Register("ping", PingCheck())
	hc.Register("threshold", ThresholdCheck("memory", func() float64 { return 60.0 }, 70.0, 90.0))

	// Check overall status (CheckAll only runs registered checks)
	results := hc.CheckAll(ctx)
	if len(results) != 2 {
		t.Errorf("Expected 2 check results from CheckAll, got %d", len(results))
	}

	// Report status via HealthReporter (sets cached status, not registered checks)
	hr.ReportHealthy("database")
	hr.ReportDegraded("cache", "high latency")

	// Cached status should include reporter statuses plus checks that have been run
	status := hc.GetStatus()
	if len(status) < 4 {
		t.Errorf("Expected at least 4 cached statuses, got %d", len(status))
	}

	// Verify the reporter-set statuses are in cache
	if status["database"].Status != HealthStatusHealthy {
		t.Errorf("database status = %v, want %v", status["database"].Status, HealthStatusHealthy)
	}
	if status["cache"].Status != HealthStatusDegraded {
		t.Errorf("cache status = %v, want %v", status["cache"].Status, HealthStatusDegraded)
	}

	// Overall should be healthy because OverallStatus only checks registered checks
	overall := hc.OverallStatus(ctx)
	if overall != HealthStatusHealthy {
		t.Errorf("Overall status = %v, want %v", overall, HealthStatusHealthy)
	}

	// Now register a degraded check and verify overall reflects it
	hc.Register("degraded-service", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusDegraded, Message: "slow"}
	})

	// Wait for cache to expire
	time.Sleep(60 * time.Millisecond)

	// Now overall should be degraded
	overall = hc.OverallStatus(ctx)
	if overall != HealthStatusDegraded {
		t.Errorf("Overall status after adding degraded check = %v, want %v", overall, HealthStatusDegraded)
	}

	// Unregister the degraded check
	hc.Unregister("degraded-service")

	// Wait for cache to expire again
	time.Sleep(60 * time.Millisecond)

	// Now should be healthy again
	if !hc.IsHealthy(ctx) {
		t.Error("Should be healthy after removing degraded check")
	}
}

func TestHTTPHandlers_Integration(t *testing.T) {
	hc := NewHealthChecker()
	hc.Register("service", func(ctx context.Context) HealthCheckResult {
		return HealthCheckResult{Status: HealthStatusHealthy}
	})

	// Create a mux with all handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/health", HealthHandler(hc))
	mux.HandleFunc("/ready", ReadinessHandler(hc))
	mux.HandleFunc("/live", LivenessHandler())

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Test all endpoints
	endpoints := []struct {
		path     string
		expected int
	}{
		{"/health", http.StatusOK},
		{"/ready", http.StatusOK},
		{"/live", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			resp, err := client.Get(server.URL + ep.path)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", ep.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != ep.expected {
				t.Errorf("GET %s status = %d, want %d", ep.path, resp.StatusCode, ep.expected)
			}
		})
	}
}

func TestCompositeCheck_WithRealChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checks := map[string]HealthCheck{
		"ping":      PingCheck(),
		"http":      HTTPCheck(server.URL, 5*time.Second),
		"threshold": ThresholdCheck("cpu", func() float64 { return 30.0 }, 70.0, 90.0),
	}

	composite := CompositeCheck(checks)
	result := composite(context.Background())

	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if len(result.Details) != 3 {
		t.Errorf("len(Details) = %d, want 3", len(result.Details))
	}
}
