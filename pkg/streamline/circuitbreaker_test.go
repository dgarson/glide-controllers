package streamline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- CircuitState Tests ---

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		name     string
		state    CircuitState
		expected string
	}{
		{
			name:     "closed state",
			state:    CircuitClosed,
			expected: "closed",
		},
		{
			name:     "open state",
			state:    CircuitOpen,
			expected: "open",
		},
		{
			name:     "half-open state",
			state:    CircuitHalfOpen,
			expected: "half-open",
		},
		{
			name:     "unknown state",
			state:    CircuitState(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("CircuitState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- CircuitBreakerConfig Tests ---

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-breaker")

	if config.Name != "test-breaker" {
		t.Errorf("expected name 'test-breaker', got %s", config.Name)
	}
	if config.FailureThreshold != 5 {
		t.Errorf("expected FailureThreshold 5, got %d", config.FailureThreshold)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("expected SuccessThreshold 2, got %d", config.SuccessThreshold)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", config.Timeout)
	}
	if config.MaxHalfOpenRequests != 1 {
		t.Errorf("expected MaxHalfOpenRequests 1, got %d", config.MaxHalfOpenRequests)
	}
}

// --- CircuitBreaker Tests ---

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("with default values for zero config", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{Name: "test"})

		if cb.config.FailureThreshold != 5 {
			t.Errorf("expected default FailureThreshold 5, got %d", cb.config.FailureThreshold)
		}
		if cb.config.SuccessThreshold != 2 {
			t.Errorf("expected default SuccessThreshold 2, got %d", cb.config.SuccessThreshold)
		}
		if cb.config.Timeout != 30*time.Second {
			t.Errorf("expected default Timeout 30s, got %v", cb.config.Timeout)
		}
		if cb.config.MaxHalfOpenRequests != 1 {
			t.Errorf("expected default MaxHalfOpenRequests 1, got %d", cb.config.MaxHalfOpenRequests)
		}
		if cb.State() != CircuitClosed {
			t.Errorf("expected initial state CircuitClosed, got %v", cb.State())
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:                "custom",
			FailureThreshold:    10,
			SuccessThreshold:    5,
			Timeout:             1 * time.Minute,
			MaxHalfOpenRequests: 3,
		})

		if cb.config.FailureThreshold != 10 {
			t.Errorf("expected FailureThreshold 10, got %d", cb.config.FailureThreshold)
		}
		if cb.config.SuccessThreshold != 5 {
			t.Errorf("expected SuccessThreshold 5, got %d", cb.config.SuccessThreshold)
		}
		if cb.config.Timeout != 1*time.Minute {
			t.Errorf("expected Timeout 1m, got %v", cb.config.Timeout)
		}
		if cb.config.MaxHalfOpenRequests != 3 {
			t.Errorf("expected MaxHalfOpenRequests 3, got %d", cb.config.MaxHalfOpenRequests)
		}
	})
}

func TestCircuitBreaker_Name(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Name: "my-circuit"})
	if cb.Name() != "my-circuit" {
		t.Errorf("expected name 'my-circuit', got %s", cb.Name())
	}
}

func TestCircuitBreaker_Failures(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 5,
	})

	if cb.Failures() != 0 {
		t.Errorf("expected initial failures 0, got %d", cb.Failures())
	}

	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error { return testErr })
	}

	if cb.Failures() != 3 {
		t.Errorf("expected 3 failures, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		executed := false

		err := cb.Execute(func() error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !executed {
			t.Error("function was not executed")
		}
	})

	t.Run("returns error from function", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		testErr := errors.New("test error")

		err := cb.Execute(func() error {
			return testErr
		})

		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}
	})

	t.Run("rejects when circuit is open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 2,
			Timeout:          1 * time.Hour, // Long timeout so circuit stays open
		})

		testErr := errors.New("test error")
		// Cause failures to open the circuit
		for i := 0; i < 2; i++ {
			_ = cb.Execute(func() error { return testErr })
		}

		if cb.State() != CircuitOpen {
			t.Fatalf("expected circuit to be open, got %v", cb.State())
		}

		// Now execution should be rejected
		err := cb.Execute(func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})
}

func TestCircuitBreaker_ExecuteWithContext(t *testing.T) {
	t.Run("passes context to function", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		ctx := context.WithValue(context.Background(), "key", "value")
		var receivedCtx context.Context

		err := cb.ExecuteWithContext(ctx, func(c context.Context) error {
			receivedCtx = c
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if receivedCtx.Value("key") != "value" {
			t.Error("context was not passed correctly")
		}
	})

	t.Run("rejects when circuit is open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 1,
			Timeout:          1 * time.Hour,
		})

		// Open the circuit
		_ = cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})

		err := cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
			return nil
		})

		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})
}

func TestCircuitBreaker_ExecuteWithFallback(t *testing.T) {
	t.Run("does not call fallback on success", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		fallbackCalled := false

		err := cb.ExecuteWithFallback(
			func() error { return nil },
			func(err error) error {
				fallbackCalled = true
				return nil
			},
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if fallbackCalled {
			t.Error("fallback should not be called on success")
		}
	})

	t.Run("does not call fallback on normal error", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		fallbackCalled := false
		testErr := errors.New("test error")

		err := cb.ExecuteWithFallback(
			func() error { return testErr },
			func(err error) error {
				fallbackCalled = true
				return nil
			},
		)

		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}
		if fallbackCalled {
			t.Error("fallback should not be called on normal errors")
		}
	})

	t.Run("calls fallback when circuit is open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 1,
			Timeout:          1 * time.Hour,
		})

		// Open the circuit
		_ = cb.Execute(func() error { return errors.New("fail") })

		fallbackCalled := false
		fallbackResult := errors.New("fallback result")

		err := cb.ExecuteWithFallback(
			func() error { return nil },
			func(err error) error {
				fallbackCalled = true
				if !errors.Is(err, ErrCircuitOpen) {
					t.Errorf("expected ErrCircuitOpen in fallback, got %v", err)
				}
				return fallbackResult
			},
		)

		if !fallbackCalled {
			t.Error("fallback should be called when circuit is open")
		}
		if !errors.Is(err, fallbackResult) {
			t.Errorf("expected fallback result, got %v", err)
		}
	})
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	t.Run("closed to open on failure threshold", func(t *testing.T) {
		var transitions []struct{ from, to CircuitState }
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 3,
			OnStateChange: func(name string, from, to CircuitState) {
				transitions = append(transitions, struct{ from, to CircuitState }{from, to})
			},
		})

		testErr := errors.New("test error")
		for i := 0; i < 3; i++ {
			_ = cb.Execute(func() error { return testErr })
		}

		if cb.State() != CircuitOpen {
			t.Errorf("expected CircuitOpen, got %v", cb.State())
		}

		if len(transitions) != 1 {
			t.Fatalf("expected 1 transition, got %d", len(transitions))
		}
		if transitions[0].from != CircuitClosed || transitions[0].to != CircuitOpen {
			t.Errorf("expected transition from closed to open, got %v -> %v",
				transitions[0].from, transitions[0].to)
		}
	})

	t.Run("open to half-open after timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 1,
			Timeout:          10 * time.Millisecond,
		})

		// Open the circuit
		_ = cb.Execute(func() error { return errors.New("fail") })

		if cb.State() != CircuitOpen {
			t.Fatalf("expected CircuitOpen, got %v", cb.State())
		}

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)

		// State should report half-open
		if cb.State() != CircuitHalfOpen {
			t.Errorf("expected CircuitHalfOpen after timeout, got %v", cb.State())
		}
	})

	t.Run("half-open to closed on success threshold", func(t *testing.T) {
		var transitions []struct{ from, to CircuitState }
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:                "test",
			FailureThreshold:    1,
			SuccessThreshold:    2,
			Timeout:             10 * time.Millisecond,
			MaxHalfOpenRequests: 5,
			OnStateChange: func(name string, from, to CircuitState) {
				transitions = append(transitions, struct{ from, to CircuitState }{from, to})
			},
		})

		// Open the circuit
		_ = cb.Execute(func() error { return errors.New("fail") })

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)

		// Execute successful requests
		for i := 0; i < 2; i++ {
			err := cb.Execute(func() error { return nil })
			if err != nil {
				t.Fatalf("unexpected error on success %d: %v", i, err)
			}
		}

		if cb.State() != CircuitClosed {
			t.Errorf("expected CircuitClosed, got %v", cb.State())
		}

		// Check transitions: closed->open, open->half-open, half-open->closed
		if len(transitions) != 3 {
			t.Fatalf("expected 3 transitions, got %d: %+v", len(transitions), transitions)
		}
	})

	t.Run("half-open to open on failure", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			Name:             "test",
			FailureThreshold: 1,
			Timeout:          10 * time.Millisecond,
		})

		// Open the circuit
		_ = cb.Execute(func() error { return errors.New("fail") })

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)

		// Fail again in half-open state
		err := cb.Execute(func() error { return errors.New("fail again") })
		if err == nil || err.Error() != "fail again" {
			t.Errorf("expected 'fail again' error, got %v", err)
		}

		if cb.State() != CircuitOpen {
			t.Errorf("expected CircuitOpen, got %v", cb.State())
		}
	})
}

func TestCircuitBreaker_Reset(t *testing.T) {
	var transitions []struct{ from, to CircuitState }
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
		OnStateChange: func(name string, from, to CircuitState) {
			transitions = append(transitions, struct{ from, to CircuitState }{from, to})
		},
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %v", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected CircuitClosed after reset, got %v", cb.State())
	}

	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after reset, got %d", cb.Failures())
	}

	// Should have: closed->open, open->closed (reset)
	if len(transitions) != 2 {
		t.Errorf("expected 2 transitions, got %d", len(transitions))
	}
}

func TestCircuitBreaker_Reset_NoCallback_WhenAlreadyClosed(t *testing.T) {
	callCount := 0
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name: "test",
		OnStateChange: func(name string, from, to CircuitState) {
			callCount++
		},
	})

	// Reset while already closed should not trigger callback
	cb.Reset()

	if callCount != 0 {
		t.Errorf("expected 0 callback calls when resetting closed circuit, got %d", callCount)
	}
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	var transitions []struct{ from, to CircuitState }
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name: "test",
		OnStateChange: func(name string, from, to CircuitState) {
			transitions = append(transitions, struct{ from, to CircuitState }{from, to})
		},
	})

	cb.ForceOpen()

	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen, got %v", cb.State())
	}

	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].from != CircuitClosed || transitions[0].to != CircuitOpen {
		t.Errorf("expected closed->open, got %v->%v", transitions[0].from, transitions[0].to)
	}
}

func TestCircuitBreaker_ForceOpen_NoCallback_WhenAlreadyOpen(t *testing.T) {
	callCount := 0
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		OnStateChange: func(name string, from, to CircuitState) {
			callCount++
		},
	})

	// Open it first
	_ = cb.Execute(func() error { return errors.New("fail") })
	callCount = 0 // reset counter

	// Force open when already open should not trigger callback
	cb.ForceOpen()

	if callCount != 0 {
		t.Errorf("expected 0 callback calls when forcing open already-open circuit, got %d", callCount)
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 5,
	})

	// Add some failures
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}

	if cb.Failures() != 3 {
		t.Fatalf("expected 3 failures, got %d", cb.Failures())
	}

	// Success resets failures
	_ = cb.Execute(func() error { return nil })

	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after success, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_IsFailure(t *testing.T) {
	permanentErr := errors.New("permanent error")
	temporaryErr := errors.New("temporary error")

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 2,
		IsFailure: func(err error) bool {
			// Only count permanent errors as failures
			return errors.Is(err, permanentErr)
		},
	})

	// Temporary errors should not count as failures
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error { return temporaryErr })
	}

	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures for temporary errors, got %d", cb.Failures())
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to remain closed, got %v", cb.State())
	}

	// Permanent errors should count as failures
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return permanentErr })
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be open, got %v", cb.State())
	}
}

func TestCircuitBreaker_MaxHalfOpenRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    1,
		Timeout:             10 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Start first request (blocks to simulate in-flight request)
	var wg sync.WaitGroup
	startChan := make(chan struct{})
	endChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cb.Execute(func() error {
			close(startChan)
			<-endChan
			return nil
		})
	}()

	<-startChan // Wait for first request to start

	// Second request should be allowed
	executed := make(chan bool, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := cb.Execute(func() error {
			executed <- true
			<-endChan
			return nil
		})
		if err != nil {
			executed <- false
		}
	}()

	select {
	case wasExecuted := <-executed:
		if !wasExecuted {
			t.Error("second request should be allowed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("second request did not execute in time")
	}

	// Cleanup
	close(endChan)
	wg.Wait()
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 100,
	})

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				if j%2 == 0 {
					_ = cb.Execute(func() error { return nil })
				} else {
					_ = cb.Execute(func() error { return errors.New("fail") })
				}
				_ = cb.State()
				_ = cb.Failures()
			}
		}(i)
	}

	wg.Wait()
	// Test should complete without race conditions
}

// --- CircuitBreakerRegistry Tests ---

func TestNewCircuitBreakerRegistry(t *testing.T) {
	registry := NewCircuitBreakerRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestNewCircuitBreakerRegistryWithDefaults(t *testing.T) {
	customConfig := func(name string) CircuitBreakerConfig {
		return CircuitBreakerConfig{
			Name:             name,
			FailureThreshold: 10,
			SuccessThreshold: 5,
			Timeout:          1 * time.Minute,
		}
	}

	registry := NewCircuitBreakerRegistryWithDefaults(customConfig)
	cb := registry.Get("test")

	if cb.config.FailureThreshold != 10 {
		t.Errorf("expected custom FailureThreshold 10, got %d", cb.config.FailureThreshold)
	}
}

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.Get("breaker1")
	if cb1 == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
	if cb1.Name() != "breaker1" {
		t.Errorf("expected name 'breaker1', got %s", cb1.Name())
	}

	// Get same breaker again should return same instance
	cb2 := registry.Get("breaker1")
	if cb1 != cb2 {
		t.Error("expected same circuit breaker instance")
	}
}

func TestCircuitBreakerRegistry_GetWithConfig(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	config := CircuitBreakerConfig{
		Name:             "custom",
		FailureThreshold: 15,
		SuccessThreshold: 8,
	}

	cb1 := registry.GetWithConfig(config)
	if cb1 == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
	if cb1.config.FailureThreshold != 15 {
		t.Errorf("expected FailureThreshold 15, got %d", cb1.config.FailureThreshold)
	}

	// Get same breaker again returns same instance (ignores config)
	cb2 := registry.GetWithConfig(CircuitBreakerConfig{
		Name:             "custom",
		FailureThreshold: 99,
	})
	if cb1 != cb2 {
		t.Error("expected same circuit breaker instance")
	}
	// Should still have original config
	if cb2.config.FailureThreshold != 15 {
		t.Errorf("expected original FailureThreshold 15, got %d", cb2.config.FailureThreshold)
	}
}

func TestCircuitBreakerRegistry_Remove(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.Get("removable")
	registry.Remove("removable")

	cb2 := registry.Get("removable")
	if cb1 == cb2 {
		t.Error("expected different circuit breaker instance after remove")
	}
}

func TestCircuitBreakerRegistry_List(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	registry.Get("breaker1")
	registry.Get("breaker2")
	registry.Get("breaker3")

	list := registry.List()
	if len(list) != 3 {
		t.Errorf("expected 3 breakers, got %d", len(list))
	}

	// Check all names are present
	names := make(map[string]bool)
	for _, name := range list {
		names[name] = true
	}
	for _, expected := range []string{"breaker1", "breaker2", "breaker3"} {
		if !names[expected] {
			t.Errorf("expected %s in list", expected)
		}
	}
}

func TestCircuitBreakerRegistry_Reset(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.Get("breaker1")
	cb2 := registry.Get("breaker2")

	// Open both circuits
	cb1.ForceOpen()
	cb2.ForceOpen()

	if cb1.State() != CircuitOpen || cb2.State() != CircuitOpen {
		t.Fatal("expected both circuits to be open")
	}

	// Reset all
	registry.Reset()

	if cb1.State() != CircuitClosed {
		t.Errorf("expected breaker1 to be closed, got %v", cb1.State())
	}
	if cb2.State() != CircuitClosed {
		t.Errorf("expected breaker2 to be closed, got %v", cb2.State())
	}
}

func TestCircuitBreakerRegistry_States(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.Get("breaker1")
	_ = registry.Get("breaker2") // Just create breaker2, it stays closed

	cb1.ForceOpen()

	states := registry.States()

	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
	if states["breaker1"] != CircuitOpen {
		t.Errorf("expected breaker1 to be open, got %v", states["breaker1"])
	}
	if states["breaker2"] != CircuitClosed {
		t.Errorf("expected breaker2 to be closed, got %v", states["breaker2"])
	}
}

func TestCircuitBreakerRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "breaker"
			cb := registry.Get(name)
			_ = cb.Execute(func() error { return nil })
			_ = registry.List()
			_ = registry.States()
		}(i)
	}

	wg.Wait()
	// Test should complete without race conditions
}

// --- CircuitBreakerError Tests ---

func TestCircuitBreakerError_Error(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &CircuitBreakerError{
			Name:  "test",
			State: CircuitOpen,
			Cause: cause,
		}

		expected := "circuit breaker test (open): underlying error"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("without cause", func(t *testing.T) {
		err := &CircuitBreakerError{
			Name:  "test",
			State: CircuitOpen,
		}

		expected := "circuit breaker test is open"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})
}

func TestCircuitBreakerError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &CircuitBreakerError{
		Name:  "test",
		State: CircuitOpen,
		Cause: cause,
	}

	if !errors.Is(err, cause) {
		t.Error("expected errors.Is to match cause")
	}
}

func TestIsCircuitBreakerError(t *testing.T) {
	t.Run("is CircuitBreakerError", func(t *testing.T) {
		err := &CircuitBreakerError{
			Name:  "test",
			State: CircuitOpen,
		}

		if !IsCircuitBreakerError(err) {
			t.Error("expected true for CircuitBreakerError")
		}
	})

	t.Run("wrapped CircuitBreakerError", func(t *testing.T) {
		inner := &CircuitBreakerError{
			Name:  "test",
			State: CircuitOpen,
		}
		wrapped := fmt.Errorf("wrapped: %w", inner)

		if !IsCircuitBreakerError(wrapped) {
			t.Error("expected true for wrapped CircuitBreakerError")
		}
	})

	t.Run("not CircuitBreakerError", func(t *testing.T) {
		err := errors.New("regular error")

		if IsCircuitBreakerError(err) {
			t.Error("expected false for regular error")
		}
	})
}

func TestIsCircuitOpen(t *testing.T) {
	t.Run("is ErrCircuitOpen", func(t *testing.T) {
		if !IsCircuitOpen(ErrCircuitOpen) {
			t.Error("expected true for ErrCircuitOpen")
		}
	})

	t.Run("wrapped ErrCircuitOpen", func(t *testing.T) {
		wrapped := fmt.Errorf("wrapped: %w", ErrCircuitOpen)

		if !IsCircuitOpen(wrapped) {
			t.Error("expected true for wrapped ErrCircuitOpen")
		}
	})

	t.Run("not ErrCircuitOpen", func(t *testing.T) {
		err := errors.New("other error")

		if IsCircuitOpen(err) {
			t.Error("expected false for other error")
		}
	})
}

// --- CircuitBreakerMiddleware Tests ---

func TestNewCircuitBreakerMiddleware(t *testing.T) {
	registry := NewCircuitBreakerRegistry()
	middleware := NewCircuitBreakerMiddleware(registry)

	if middleware == nil {
		t.Fatal("expected non-nil middleware")
	}
}

func TestCircuitBreakerMiddleware_Wrap(t *testing.T) {
	registry := NewCircuitBreakerRegistry()
	middleware := NewCircuitBreakerMiddleware(registry)

	t.Run("successful execution", func(t *testing.T) {
		executed := false
		err := middleware.Wrap("test", func() error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !executed {
			t.Error("function was not executed")
		}
	})

	t.Run("uses same circuit breaker for same name", func(t *testing.T) {
		testErr := errors.New("fail")
		cb := registry.Get("shared")
		// Get the breaker to set threshold low
		cb.config.FailureThreshold = 1

		err := middleware.Wrap("shared", func() error {
			return testErr
		})

		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}

		// Now circuit should be open (FailureThreshold was set to 1)
		// But since we got the default config (threshold=5), we need to fail more
		for i := 0; i < 5; i++ {
			_ = middleware.Wrap("shared", func() error { return testErr })
		}

		err = middleware.Wrap("shared", func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})
}

func TestCircuitBreakerMiddleware_WrapWithContext(t *testing.T) {
	registry := NewCircuitBreakerRegistry()
	middleware := NewCircuitBreakerMiddleware(registry)

	t.Run("passes context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "key", "value")
		var receivedCtx context.Context

		err := middleware.WrapWithContext("test", ctx, func(c context.Context) error {
			receivedCtx = c
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if receivedCtx.Value("key") != "value" {
			t.Error("context was not passed correctly")
		}
	})
}

// --- TwoStageCircuitBreaker Tests ---

func TestNewTwoStageCircuitBreaker(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	if cb == nil {
		t.Fatal("expected non-nil TwoStageCircuitBreaker")
	}
	if cb.read.Name() != "test-read" {
		t.Errorf("expected read name 'test-read', got %s", cb.read.Name())
	}
	if cb.write.Name() != "test-write" {
		t.Errorf("expected write name 'test-write', got %s", cb.write.Name())
	}
}

func TestTwoStageCircuitBreaker_ExecuteRead(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 5,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	t.Run("successful read", func(t *testing.T) {
		executed := false
		err := cb.ExecuteRead(func() error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !executed {
			t.Error("read function was not executed")
		}
	})

	t.Run("read circuit opens independently", func(t *testing.T) {
		testErr := errors.New("read fail")
		_ = cb.ExecuteRead(func() error { return testErr })

		// Read circuit should be open
		if cb.ReadState() != CircuitOpen {
			t.Errorf("expected read circuit open, got %v", cb.ReadState())
		}

		// Write circuit should still be closed
		if cb.WriteState() != CircuitClosed {
			t.Errorf("expected write circuit closed, got %v", cb.WriteState())
		}
	})
}

func TestTwoStageCircuitBreaker_ExecuteWrite(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 5,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	t.Run("successful write", func(t *testing.T) {
		executed := false
		err := cb.ExecuteWrite(func() error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !executed {
			t.Error("write function was not executed")
		}
	})

	t.Run("write circuit opens independently", func(t *testing.T) {
		testErr := errors.New("write fail")
		_ = cb.ExecuteWrite(func() error { return testErr })

		// Write circuit should be open
		if cb.WriteState() != CircuitOpen {
			t.Errorf("expected write circuit open, got %v", cb.WriteState())
		}

		// Read circuit should still be closed
		if cb.ReadState() != CircuitClosed {
			t.Errorf("expected read circuit closed, got %v", cb.ReadState())
		}
	})
}

func TestTwoStageCircuitBreaker_States(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	// Initial states
	if cb.ReadState() != CircuitClosed {
		t.Errorf("expected initial read state closed, got %v", cb.ReadState())
	}
	if cb.WriteState() != CircuitClosed {
		t.Errorf("expected initial write state closed, got %v", cb.WriteState())
	}

	// Open read
	_ = cb.ExecuteRead(func() error { return errors.New("fail") })
	if cb.ReadState() != CircuitOpen {
		t.Errorf("expected read state open, got %v", cb.ReadState())
	}
	if cb.WriteState() != CircuitClosed {
		t.Errorf("expected write state still closed, got %v", cb.WriteState())
	}

	// Open write
	_ = cb.ExecuteWrite(func() error { return errors.New("fail") })
	if cb.WriteState() != CircuitOpen {
		t.Errorf("expected write state open, got %v", cb.WriteState())
	}
}

func TestTwoStageCircuitBreaker_Reset(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	// Open both circuits
	_ = cb.ExecuteRead(func() error { return errors.New("fail") })
	_ = cb.ExecuteWrite(func() error { return errors.New("fail") })

	if cb.ReadState() != CircuitOpen || cb.WriteState() != CircuitOpen {
		t.Fatal("expected both circuits to be open")
	}

	// Reset
	cb.Reset()

	if cb.ReadState() != CircuitClosed {
		t.Errorf("expected read state closed after reset, got %v", cb.ReadState())
	}
	if cb.WriteState() != CircuitClosed {
		t.Errorf("expected write state closed after reset, got %v", cb.WriteState())
	}
}

func TestTwoStageCircuitBreaker_IndependentOperation(t *testing.T) {
	readConfig := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          10 * time.Millisecond,
	}
	writeConfig := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          10 * time.Millisecond,
	}

	cb := NewTwoStageCircuitBreaker("test", readConfig, writeConfig)

	// Fail read circuit
	for i := 0; i < 2; i++ {
		_ = cb.ExecuteRead(func() error { return errors.New("fail") })
	}

	if cb.ReadState() != CircuitOpen {
		t.Errorf("expected read circuit open, got %v", cb.ReadState())
	}

	// Write should still work
	writeExecuted := false
	err := cb.ExecuteWrite(func() error {
		writeExecuted = true
		return nil
	})

	if err != nil {
		t.Errorf("expected write to succeed, got %v", err)
	}
	if !writeExecuted {
		t.Error("write was not executed while read circuit was open")
	}
}

// --- Benchmark Tests ---

func BenchmarkCircuitBreaker_Execute(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("bench"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(func() error { return nil })
	}
}

func BenchmarkCircuitBreaker_ExecuteWithFailure(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "bench",
		FailureThreshold: 1000000, // High threshold to prevent opening
	})
	testErr := errors.New("fail")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(func() error { return testErr })
	}
}

func BenchmarkCircuitBreakerRegistry_Get(b *testing.B) {
	registry := NewCircuitBreakerRegistry()
	registry.Get("bench") // Pre-create

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Get("bench")
	}
}

func BenchmarkCircuitBreaker_ConcurrentExecute(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("bench"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(func() error { return nil })
		}
	})
}

// --- Additional Edge Case Tests ---

func TestCircuitBreaker_AllowDefaultUnknownState(t *testing.T) {
	// This tests the default case in allowRequest which returns false
	// We can't directly test this without modifying internal state,
	// but we can verify the behavior is correct for known states
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	// Verify all known states work
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %v", cb.State())
	}

	cb.ForceOpen()
	if cb.State() != CircuitOpen {
		t.Errorf("expected open, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenLimitedRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    1,
		SuccessThreshold:    3,
		Timeout:             10 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Start a long-running request in half-open state
	started := make(chan struct{})
	done := make(chan struct{})

	go func() {
		_ = cb.Execute(func() error {
			close(started)
			<-done
			return nil
		})
	}()

	<-started // Wait for first request to start

	// Second request should fail because MaxHalfOpenRequests=1
	var requestBlocked int32
	go func() {
		err := cb.Execute(func() error {
			return nil
		})
		if errors.Is(err, ErrCircuitOpen) {
			atomic.StoreInt32(&requestBlocked, 1)
		}
	}()

	time.Sleep(10 * time.Millisecond)
	close(done)

	// Give time for the second request to complete
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&requestBlocked) != 1 {
		t.Error("expected second request to be blocked in half-open state")
	}
}

func TestCircuitBreaker_StateChangeCallbackName(t *testing.T) {
	var receivedName string
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "callback-test",
		FailureThreshold: 1,
		OnStateChange: func(name string, from, to CircuitState) {
			receivedName = name
		},
	})

	_ = cb.Execute(func() error { return errors.New("fail") })

	if receivedName != "callback-test" {
		t.Errorf("expected name 'callback-test' in callback, got %s", receivedName)
	}
}

func TestCircuitBreaker_TransitionFromOpenToHalfOpenOnAllowRequest(t *testing.T) {
	var transitions []struct{ from, to CircuitState }
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 1,
		Timeout:          10 * time.Millisecond,
		OnStateChange: func(name string, from, to CircuitState) {
			transitions = append(transitions, struct{ from, to CircuitState }{from, to})
		},
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Clear transitions from opening
	transitions = nil

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Execute a request - this should trigger transition to half-open
	_ = cb.Execute(func() error { return nil })

	// Should have transitioned: open -> half-open, then half-open -> closed (if success threshold met)
	// or just open -> half-open if success threshold > 1
	if len(transitions) < 1 {
		t.Error("expected at least 1 transition after timeout")
	}

	if transitions[0].from != CircuitOpen || transitions[0].to != CircuitHalfOpen {
		t.Errorf("expected first transition from open to half-open, got %v -> %v",
			transitions[0].from, transitions[0].to)
	}
}

func TestErrTooManyRequests(t *testing.T) {
	// Verify the error exists and has correct message
	if ErrTooManyRequests == nil {
		t.Error("expected ErrTooManyRequests to be non-nil")
	}
	if ErrTooManyRequests.Error() != "too many requests during half-open state" {
		t.Errorf("unexpected error message: %s", ErrTooManyRequests.Error())
	}
}

func TestCircuitBreaker_IsFailureWithNilError(t *testing.T) {
	// Test that IsFailure function is only called when err != nil
	isFailureCalled := false
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:             "test",
		FailureThreshold: 5,
		IsFailure: func(err error) bool {
			isFailureCalled = true
			return true
		},
	})

	// Success should not call IsFailure
	_ = cb.Execute(func() error { return nil })

	if isFailureCalled {
		t.Error("IsFailure should not be called for nil errors")
	}

	// Error should call IsFailure
	_ = cb.Execute(func() error { return errors.New("fail") })

	if !isFailureCalled {
		t.Error("IsFailure should be called for non-nil errors")
	}
}

func TestCircuitBreaker_HalfOpenDecrementOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    1,
		SuccessThreshold:    3,
		Timeout:             10 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Execute successful requests - each should decrement halfOpenRequests
	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error { return nil })
		if err != nil {
			t.Errorf("unexpected error on iteration %d: %v", i, err)
		}
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be closed after success threshold, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenDecrementOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             10 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	})

	// Open the circuit
	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Fail in half-open state
	_ = cb.Execute(func() error { return errors.New("fail again") })

	// Circuit should be open again
	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be open after half-open failure, got %v", cb.State())
	}
}
