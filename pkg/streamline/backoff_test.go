package streamline

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

func TestDefaultBackoffConfig(t *testing.T) {
	config := DefaultBackoffConfig()

	if config.InitialInterval != 1*time.Second {
		t.Errorf("InitialInterval = %v, want %v", config.InitialInterval, 1*time.Second)
	}
	if config.MaxInterval != 5*time.Minute {
		t.Errorf("MaxInterval = %v, want %v", config.MaxInterval, 5*time.Minute)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want %v", config.Multiplier, 2.0)
	}
	if config.RandomizationFactor != 0.1 {
		t.Errorf("RandomizationFactor = %v, want %v", config.RandomizationFactor, 0.1)
	}
}

func TestExponentialBackoff(t *testing.T) {
	t.Run("basic exponential growth", func(t *testing.T) {
		backoff := ExponentialBackoff(BackoffConfig{
			InitialInterval:     1 * time.Second,
			MaxInterval:         1 * time.Minute,
			Multiplier:          2.0,
			RandomizationFactor: 0, // No jitter for predictable tests
		})

		// Expected: 1s, 2s, 4s, 8s, 16s, 32s, 60s (capped)
		expected := []time.Duration{
			1 * time.Second,
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			16 * time.Second,
			32 * time.Second,
			60 * time.Second, // Capped at max
		}

		for i, exp := range expected {
			got := backoff.NextBackoff(i + 1)
			if got != exp {
				t.Errorf("NextBackoff(%d) = %v, want %v", i+1, got, exp)
			}
		}
	})

	t.Run("default values applied", func(t *testing.T) {
		backoff := ExponentialBackoff(BackoffConfig{})

		// Should have defaults applied
		got := backoff.NextBackoff(1)
		if got < 900*time.Millisecond || got > 1100*time.Millisecond {
			t.Errorf("NextBackoff(1) with defaults = %v, expected around 1s", got)
		}
	})

	t.Run("with randomization", func(t *testing.T) {
		backoff := ExponentialBackoff(BackoffConfig{
			InitialInterval:     1 * time.Second,
			MaxInterval:         1 * time.Minute,
			Multiplier:          2.0,
			RandomizationFactor: 0.5,
		})

		// With 50% randomization, 1s should be between 0.5s and 1.5s
		for i := 0; i < 10; i++ {
			got := backoff.NextBackoff(1)
			if got < 500*time.Millisecond || got > 1500*time.Millisecond {
				t.Errorf("NextBackoff(1) with jitter = %v, expected 0.5s-1.5s", got)
			}
		}
	})

	t.Run("max elapsed time", func(t *testing.T) {
		backoff := ExponentialBackoff(BackoffConfig{
			InitialInterval:     1 * time.Second,
			MaxInterval:         1 * time.Minute,
			Multiplier:          2.0,
			MaxElapsedTime:      1 * time.Nanosecond, // Immediately expired
			RandomizationFactor: 0,
		})

		// Wait a tiny bit to exceed max elapsed time
		time.Sleep(time.Millisecond)

		got := backoff.NextBackoff(1)
		if got != 0 {
			t.Errorf("NextBackoff after max elapsed = %v, want 0", got)
		}
	})

	t.Run("reset", func(t *testing.T) {
		backoff := ExponentialBackoff(BackoffConfig{
			InitialInterval:     1 * time.Second,
			MaxElapsedTime:      1 * time.Hour,
			RandomizationFactor: 0,
		})

		backoff.Reset()
		got := backoff.NextBackoff(1)
		if got != 1*time.Second {
			t.Errorf("After reset NextBackoff(1) = %v, want 1s", got)
		}
	})
}

func TestLinearBackoff(t *testing.T) {
	t.Run("basic linear growth", func(t *testing.T) {
		backoff := LinearBackoff(1*time.Second, 2*time.Second, 10*time.Second)

		// Expected: 1s, 3s, 5s, 7s, 9s, 10s (capped)
		expected := []time.Duration{
			1 * time.Second,
			3 * time.Second,
			5 * time.Second,
			7 * time.Second,
			9 * time.Second,
			10 * time.Second, // Capped
			10 * time.Second, // Still capped
		}

		for i, exp := range expected {
			got := backoff.NextBackoff(i + 1)
			if got != exp {
				t.Errorf("NextBackoff(%d) = %v, want %v", i+1, got, exp)
			}
		}
	})

	t.Run("with jitter", func(t *testing.T) {
		backoff := LinearBackoffWithJitter(1*time.Second, 1*time.Second, 10*time.Second, 0.5)

		// With 50% jitter, 1s base should be between 0.5s and 1.5s
		for i := 0; i < 10; i++ {
			got := backoff.NextBackoff(1)
			if got < 500*time.Millisecond || got > 1500*time.Millisecond {
				t.Errorf("NextBackoff(1) with jitter = %v, expected 0.5s-1.5s", got)
			}
		}
	})

	t.Run("reset is no-op", func(t *testing.T) {
		backoff := LinearBackoff(1*time.Second, 1*time.Second, 10*time.Second)
		backoff.Reset() // Should not panic
		got := backoff.NextBackoff(1)
		if got != 1*time.Second {
			t.Errorf("After reset NextBackoff(1) = %v, want 1s", got)
		}
	})
}

func TestConstantBackoff(t *testing.T) {
	t.Run("always returns same duration", func(t *testing.T) {
		backoff := ConstantBackoff(5 * time.Second)

		for i := 1; i <= 10; i++ {
			got := backoff.NextBackoff(i)
			if got != 5*time.Second {
				t.Errorf("NextBackoff(%d) = %v, want 5s", i, got)
			}
		}
	})

	t.Run("with jitter", func(t *testing.T) {
		backoff := ConstantBackoffWithJitter(5*time.Second, 0.2)

		// With 20% jitter, 5s should be between 4s and 6s
		for i := 0; i < 10; i++ {
			got := backoff.NextBackoff(1)
			if got < 4*time.Second || got > 6*time.Second {
				t.Errorf("NextBackoff(1) with jitter = %v, expected 4s-6s", got)
			}
		}
	})

	t.Run("reset is no-op", func(t *testing.T) {
		backoff := ConstantBackoff(5 * time.Second)
		backoff.Reset() // Should not panic
	})
}

func TestNoBackoff(t *testing.T) {
	backoff := NoBackoff()

	for i := 1; i <= 10; i++ {
		got := backoff.NextBackoff(i)
		if got != 0 {
			t.Errorf("NextBackoff(%d) = %v, want 0", i, got)
		}
	}

	backoff.Reset() // Should not panic
}

func TestWithMaxAttempts(t *testing.T) {
	base := ConstantBackoff(1 * time.Second)
	backoff := WithMaxAttempts(base, 3)

	// Should return durations for attempts 1-3
	for i := 1; i <= 3; i++ {
		got := backoff.NextBackoff(i)
		if got != 1*time.Second {
			t.Errorf("NextBackoff(%d) = %v, want 1s", i, got)
		}
	}

	// Should return 0 after max attempts
	got := backoff.NextBackoff(4)
	if got != 0 {
		t.Errorf("NextBackoff(4) after max attempts = %v, want 0", got)
	}
}

func TestWithMaxAttempts_Reset(t *testing.T) {
	base := ExponentialBackoff(BackoffConfig{
		InitialInterval:     1 * time.Second,
		RandomizationFactor: 0,
	})
	backoff := WithMaxAttempts(base, 3)

	backoff.Reset()
	got := backoff.NextBackoff(1)
	if got != 1*time.Second {
		t.Errorf("After reset NextBackoff(1) = %v, want 1s", got)
	}
}

func TestWithJitter(t *testing.T) {
	base := ConstantBackoff(1 * time.Second)
	backoff := WithJitter(base, 0.5)

	// With 50% jitter, 1s should be between 0.5s and 1.5s
	for i := 0; i < 10; i++ {
		got := backoff.NextBackoff(1)
		if got < 500*time.Millisecond || got > 1500*time.Millisecond {
			t.Errorf("NextBackoff(1) with jitter = %v, expected 0.5s-1.5s", got)
		}
	}
}

func TestWithJitter_Reset(t *testing.T) {
	base := ExponentialBackoff(BackoffConfig{
		InitialInterval:     1 * time.Second,
		RandomizationFactor: 0,
	})
	backoff := WithJitter(base, 0.1)

	backoff.Reset()
	got := backoff.NextBackoff(1)
	if got < 900*time.Millisecond || got > 1100*time.Millisecond {
		t.Errorf("After reset NextBackoff(1) = %v, expected around 1s", got)
	}
}

func TestWithJitter_ZeroInterval(t *testing.T) {
	base := NoBackoff()
	backoff := WithJitter(base, 0.5)

	got := backoff.NextBackoff(1)
	if got != 0 {
		t.Errorf("Jitter on 0 interval = %v, want 0", got)
	}
}

func TestBackoffTracker(t *testing.T) {
	key := types.NamespacedName{Namespace: "test", Name: "resource"}

	t.Run("new tracker has zero attempts", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)
		if tracker.GetAttempts(key) != 0 {
			t.Error("New tracker should have 0 attempts")
		}
	})

	t.Run("record failures increments attempts", func(t *testing.T) {
		tracker := NewBackoffTracker(ConstantBackoff(1 * time.Second))

		tracker.RecordFailure(key)
		if tracker.GetAttempts(key) != 1 {
			t.Errorf("After 1 failure, attempts = %d, want 1", tracker.GetAttempts(key))
		}

		tracker.RecordFailure(key)
		if tracker.GetAttempts(key) != 2 {
			t.Errorf("After 2 failures, attempts = %d, want 2", tracker.GetAttempts(key))
		}
	})

	t.Run("record success resets attempts", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)

		tracker.RecordFailure(key)
		tracker.RecordFailure(key)
		tracker.RecordSuccess(key)

		if tracker.GetAttempts(key) != 0 {
			t.Errorf("After success, attempts = %d, want 0", tracker.GetAttempts(key))
		}
	})

	t.Run("get backoff returns duration based on attempts", func(t *testing.T) {
		tracker := NewBackoffTracker(ConstantBackoff(5 * time.Second))

		// No failures, no backoff
		if tracker.GetBackoff(key) != 0 {
			t.Error("With 0 attempts, backoff should be 0")
		}

		tracker.RecordFailure(key)
		got := tracker.GetBackoff(key)
		if got != 5*time.Second {
			t.Errorf("After failure, backoff = %v, want 5s", got)
		}
	})

	t.Run("reset removes entry", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)

		tracker.RecordFailure(key)
		tracker.Reset(key)

		if tracker.GetAttempts(key) != 0 {
			t.Error("After reset, attempts should be 0")
		}
	})

	t.Run("reset all clears everything", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)

		key2 := types.NamespacedName{Namespace: "test", Name: "resource2"}
		tracker.RecordFailure(key)
		tracker.RecordFailure(key2)
		tracker.ResetAll()

		if tracker.GetAttempts(key) != 0 || tracker.GetAttempts(key2) != 0 {
			t.Error("After reset all, all attempts should be 0")
		}
	})

	t.Run("get last failure", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)

		// No failure recorded
		if !tracker.GetLastFailure(key).IsZero() {
			t.Error("Last failure should be zero for new key")
		}

		before := time.Now()
		tracker.RecordFailure(key)
		after := time.Now()

		lastFailure := tracker.GetLastFailure(key)
		if lastFailure.Before(before) || lastFailure.After(after) {
			t.Errorf("Last failure time %v not between %v and %v", lastFailure, before, after)
		}
	})

	t.Run("cleanup removes old entries", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)

		tracker.RecordFailure(key)
		// Access happened during RecordFailure, so lastAccess is set

		// Cleanup with very long maxAge should keep entries
		tracker.Cleanup(1 * time.Hour)
		if tracker.GetAttempts(key) != 1 {
			t.Error("Cleanup with long maxAge should not remove recent entries")
		}

		// Cleanup with zero maxAge should remove all
		tracker.Cleanup(0)
		if tracker.GetAttempts(key) != 0 {
			t.Error("Cleanup with 0 maxAge should remove all entries")
		}
	})

	t.Run("record success on non-existent key is safe", func(t *testing.T) {
		tracker := NewBackoffTracker(nil)
		tracker.RecordSuccess(key) // Should not panic
	})
}

func TestBackoffTracker_DefaultStrategy(t *testing.T) {
	tracker := NewBackoffTracker(nil)
	key := types.NamespacedName{Namespace: "test", Name: "resource"}

	tracker.RecordFailure(key)
	backoff := tracker.GetBackoff(key)

	// Should use default exponential backoff, initial ~1s
	if backoff < 900*time.Millisecond || backoff > 1100*time.Millisecond {
		t.Errorf("Default backoff = %v, expected around 1s", backoff)
	}
}

func TestRetryPolicy(t *testing.T) {
	t.Run("default policy", func(t *testing.T) {
		policy := DefaultRetryPolicy()

		if policy.MaxAttempts != 10 {
			t.Errorf("MaxAttempts = %d, want 10", policy.MaxAttempts)
		}
	})

	t.Run("should retry retryable errors", func(t *testing.T) {
		policy := DefaultRetryPolicy()

		err := Retryable(nil)
		if !policy.ShouldRetry(1, err) {
			t.Error("Should retry retryable errors")
		}
	})

	t.Run("should not retry permanent errors", func(t *testing.T) {
		policy := DefaultRetryPolicy()

		err := Permanent(nil)
		if policy.ShouldRetry(1, err) {
			t.Error("Should not retry permanent errors")
		}
	})

	t.Run("should not retry nil errors", func(t *testing.T) {
		policy := DefaultRetryPolicy()

		if policy.ShouldRetry(1, nil) {
			t.Error("Should not retry nil errors")
		}
	})

	t.Run("should not retry after max attempts", func(t *testing.T) {
		policy := RetryPolicy{
			Strategy:    ConstantBackoff(1 * time.Second),
			MaxAttempts: 3,
		}

		err := Retryable(nil)
		if policy.ShouldRetry(3, err) {
			t.Error("Should not retry at max attempts")
		}
	})

	t.Run("custom retryable errors function", func(t *testing.T) {
		policy := RetryPolicy{
			Strategy:    ConstantBackoff(1 * time.Second),
			MaxAttempts: 10,
			RetryableErrors: func(err error) bool {
				return false // Never retry
			},
		}

		err := Retryable(nil)
		if policy.ShouldRetry(1, err) {
			t.Error("Custom function should prevent retry")
		}
	})

	t.Run("next delay", func(t *testing.T) {
		policy := RetryPolicy{
			Strategy: ConstantBackoff(5 * time.Second),
		}

		delay := policy.NextDelay(1)
		if delay != 5*time.Second {
			t.Errorf("NextDelay = %v, want 5s", delay)
		}
	})

	t.Run("unlimited attempts when max is 0", func(t *testing.T) {
		policy := RetryPolicy{
			Strategy:    ConstantBackoff(1 * time.Second),
			MaxAttempts: 0, // Unlimited
		}

		err := Retryable(nil)
		if !policy.ShouldRetry(1000, err) {
			t.Error("With MaxAttempts=0, should allow unlimited retries")
		}
	})
}

func TestRetryContext(t *testing.T) {
	t.Run("remaining attempts unlimited", func(t *testing.T) {
		rc := &RetryContext{
			Attempt:       5,
			TotalAttempts: 0, // Unlimited
		}

		if rc.RemainingAttempts() != -1 {
			t.Errorf("RemainingAttempts = %d, want -1 for unlimited", rc.RemainingAttempts())
		}
	})

	t.Run("remaining attempts calculated", func(t *testing.T) {
		rc := &RetryContext{
			Attempt:       3,
			TotalAttempts: 10,
		}

		if rc.RemainingAttempts() != 7 {
			t.Errorf("RemainingAttempts = %d, want 7", rc.RemainingAttempts())
		}
	})

	t.Run("remaining attempts when exceeded", func(t *testing.T) {
		rc := &RetryContext{
			Attempt:       15,
			TotalAttempts: 10,
		}

		if rc.RemainingAttempts() != 0 {
			t.Errorf("RemainingAttempts = %d, want 0 when exceeded", rc.RemainingAttempts())
		}
	})

	t.Run("is last attempt", func(t *testing.T) {
		rc := &RetryContext{
			Attempt:       10,
			TotalAttempts: 10,
		}

		if !rc.IsLastAttempt() {
			t.Error("Should be last attempt")
		}
	})

	t.Run("is not last attempt with unlimited", func(t *testing.T) {
		rc := &RetryContext{
			Attempt:       1000,
			TotalAttempts: 0, // Unlimited
		}

		if rc.IsLastAttempt() {
			t.Error("Should never be last attempt with unlimited")
		}
	})
}

func TestQuickBackoff(t *testing.T) {
	backoff := QuickBackoff()

	// First attempt should be around 100ms
	got := backoff.NextBackoff(1)
	if got < 80*time.Millisecond || got > 120*time.Millisecond {
		t.Errorf("QuickBackoff first attempt = %v, expected around 100ms", got)
	}
}

func TestSlowBackoff(t *testing.T) {
	backoff := SlowBackoff()

	// First attempt should be around 5s
	got := backoff.NextBackoff(1)
	if got < 4500*time.Millisecond || got > 5500*time.Millisecond {
		t.Errorf("SlowBackoff first attempt = %v, expected around 5s", got)
	}
}
