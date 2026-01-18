package streamline

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

// BackoffStrategy defines how to calculate backoff delays.
type BackoffStrategy interface {
	// NextBackoff returns the next backoff duration given the attempt number.
	// attempt starts at 1 for the first retry.
	NextBackoff(attempt int) time.Duration

	// Reset resets the strategy state if applicable.
	Reset()
}

// BackoffConfig holds configuration for backoff strategies.
type BackoffConfig struct {
	// InitialInterval is the starting backoff interval.
	InitialInterval time.Duration

	// MaxInterval is the maximum backoff interval.
	MaxInterval time.Duration

	// Multiplier is the factor by which the interval increases.
	Multiplier float64

	// MaxElapsedTime is the maximum total time for all retries.
	// Zero means no limit.
	MaxElapsedTime time.Duration

	// RandomizationFactor adds jitter to prevent thundering herd.
	// 0 means no randomization, 0.5 means ±50%.
	RandomizationFactor float64
}

// DefaultBackoffConfig returns a sensible default configuration.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         5 * time.Minute,
		Multiplier:          2.0,
		MaxElapsedTime:      0,
		RandomizationFactor: 0.1,
	}
}

// exponentialBackoff implements exponential backoff.
type exponentialBackoff struct {
	config    BackoffConfig
	startTime time.Time
	mu        sync.Mutex
}

// ExponentialBackoff creates an exponential backoff strategy.
//
// The backoff duration increases exponentially with each attempt:
//   - Attempt 1: InitialInterval
//   - Attempt 2: InitialInterval * Multiplier
//   - Attempt 3: InitialInterval * Multiplier^2
//   - ...up to MaxInterval
//
// Example:
//
//	backoff := ExponentialBackoff(BackoffConfig{
//	    InitialInterval: 1 * time.Second,
//	    MaxInterval:     5 * time.Minute,
//	    Multiplier:      2.0,
//	})
func ExponentialBackoff(config BackoffConfig) BackoffStrategy {
	if config.InitialInterval == 0 {
		config.InitialInterval = 1 * time.Second
	}
	if config.MaxInterval == 0 {
		config.MaxInterval = 5 * time.Minute
	}
	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	return &exponentialBackoff{
		config:    config,
		startTime: time.Now(),
	}
}

func (b *exponentialBackoff) NextBackoff(attempt int) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check max elapsed time
	if b.config.MaxElapsedTime > 0 && time.Since(b.startTime) > b.config.MaxElapsedTime {
		return 0 // Stop retrying
	}

	// Calculate base interval
	interval := float64(b.config.InitialInterval) * math.Pow(b.config.Multiplier, float64(attempt-1))

	// Cap at max interval
	if interval > float64(b.config.MaxInterval) {
		interval = float64(b.config.MaxInterval)
	}

	// Add randomization
	if b.config.RandomizationFactor > 0 {
		delta := b.config.RandomizationFactor * interval
		minInterval := interval - delta
		maxInterval := interval + delta
		interval = minInterval + rand.Float64()*(maxInterval-minInterval)
	}

	return time.Duration(interval)
}

func (b *exponentialBackoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startTime = time.Now()
}

// linearBackoff implements linear backoff.
type linearBackoff struct {
	initial   time.Duration
	increment time.Duration
	max       time.Duration
	jitter    float64
}

// LinearBackoff creates a linear backoff strategy.
//
// The backoff duration increases linearly with each attempt:
//   - Attempt 1: InitialInterval
//   - Attempt 2: InitialInterval + Increment
//   - Attempt 3: InitialInterval + 2*Increment
//   - ...up to Max
func LinearBackoff(initial, increment, max time.Duration) BackoffStrategy {
	return &linearBackoff{
		initial:   initial,
		increment: increment,
		max:       max,
	}
}

// LinearBackoffWithJitter creates a linear backoff with jitter.
func LinearBackoffWithJitter(initial, increment, max time.Duration, jitter float64) BackoffStrategy {
	return &linearBackoff{
		initial:   initial,
		increment: increment,
		max:       max,
		jitter:    jitter,
	}
}

func (b *linearBackoff) NextBackoff(attempt int) time.Duration {
	interval := b.initial + time.Duration(attempt-1)*b.increment

	if interval > b.max {
		interval = b.max
	}

	if b.jitter > 0 {
		delta := float64(interval) * b.jitter
		interval = time.Duration(float64(interval) + (rand.Float64()*2-1)*delta)
	}

	return interval
}

func (b *linearBackoff) Reset() {}

// constantBackoff implements constant backoff.
type constantBackoff struct {
	interval time.Duration
	jitter   float64
}

// ConstantBackoff creates a constant backoff strategy that always returns the same duration.
func ConstantBackoff(interval time.Duration) BackoffStrategy {
	return &constantBackoff{interval: interval}
}

// ConstantBackoffWithJitter creates a constant backoff with jitter.
func ConstantBackoffWithJitter(interval time.Duration, jitter float64) BackoffStrategy {
	return &constantBackoff{
		interval: interval,
		jitter:   jitter,
	}
}

func (b *constantBackoff) NextBackoff(attempt int) time.Duration {
	interval := b.interval

	if b.jitter > 0 {
		delta := float64(interval) * b.jitter
		interval = time.Duration(float64(interval) + (rand.Float64()*2-1)*delta)
	}

	return interval
}

func (b *constantBackoff) Reset() {}

// noBackoff implements immediate retry.
type noBackoff struct{}

// NoBackoff creates a strategy that returns zero delay (immediate retry).
func NoBackoff() BackoffStrategy {
	return &noBackoff{}
}

func (b *noBackoff) NextBackoff(attempt int) time.Duration {
	return 0
}

func (b *noBackoff) Reset() {}

// cappedBackoff wraps a strategy with maximum attempts.
type cappedBackoff struct {
	strategy    BackoffStrategy
	maxAttempts int
}

// WithMaxAttempts wraps a backoff strategy with a maximum attempt limit.
// Returns 0 duration after maxAttempts, signaling to stop retrying.
func WithMaxAttempts(strategy BackoffStrategy, maxAttempts int) BackoffStrategy {
	return &cappedBackoff{
		strategy:    strategy,
		maxAttempts: maxAttempts,
	}
}

func (b *cappedBackoff) NextBackoff(attempt int) time.Duration {
	if attempt > b.maxAttempts {
		return 0
	}
	return b.strategy.NextBackoff(attempt)
}

func (b *cappedBackoff) Reset() {
	b.strategy.Reset()
}

// jitteredBackoff wraps a strategy with jitter.
type jitteredBackoff struct {
	strategy BackoffStrategy
	factor   float64
}

// WithJitter wraps a backoff strategy with jitter.
// factor is the jitter factor (e.g., 0.1 for ±10%).
func WithJitter(strategy BackoffStrategy, factor float64) BackoffStrategy {
	return &jitteredBackoff{
		strategy: strategy,
		factor:   factor,
	}
}

func (b *jitteredBackoff) NextBackoff(attempt int) time.Duration {
	interval := b.strategy.NextBackoff(attempt)

	if b.factor > 0 && interval > 0 {
		delta := float64(interval) * b.factor
		interval = time.Duration(float64(interval) + (rand.Float64()*2-1)*delta)
	}

	return interval
}

func (b *jitteredBackoff) Reset() {
	b.strategy.Reset()
}

// BackoffTracker tracks backoff state per resource.
type BackoffTracker interface {
	// RecordFailure records a failure for a resource, incrementing its attempt count.
	RecordFailure(key types.NamespacedName)

	// RecordSuccess records a success for a resource, resetting its attempt count.
	RecordSuccess(key types.NamespacedName)

	// GetAttempts returns the current attempt count for a resource.
	GetAttempts(key types.NamespacedName) int

	// GetBackoff returns the next backoff duration for a resource.
	GetBackoff(key types.NamespacedName) time.Duration

	// Reset resets the state for a resource.
	Reset(key types.NamespacedName)

	// ResetAll resets state for all resources.
	ResetAll()

	// GetLastFailure returns when the resource last failed.
	GetLastFailure(key types.NamespacedName) time.Time

	// Cleanup removes old entries that haven't been accessed recently.
	Cleanup(maxAge time.Duration)
}

// backoffEntry holds state for a single resource.
type backoffEntry struct {
	attempts    int
	lastFailure time.Time
	lastAccess  time.Time
}

// backoffTracker implements BackoffTracker.
type backoffTracker struct {
	strategy BackoffStrategy
	entries  map[types.NamespacedName]*backoffEntry
	mu       sync.RWMutex
}

// NewBackoffTracker creates a new BackoffTracker with the given strategy.
func NewBackoffTracker(strategy BackoffStrategy) BackoffTracker {
	if strategy == nil {
		strategy = ExponentialBackoff(DefaultBackoffConfig())
	}
	return &backoffTracker{
		strategy: strategy,
		entries:  make(map[types.NamespacedName]*backoffEntry),
	}
}

func (bt *backoffTracker) getOrCreate(key types.NamespacedName) *backoffEntry {
	entry, exists := bt.entries[key]
	if !exists {
		entry = &backoffEntry{}
		bt.entries[key] = entry
	}
	entry.lastAccess = time.Now()
	return entry
}

func (bt *backoffTracker) RecordFailure(key types.NamespacedName) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	entry := bt.getOrCreate(key)
	entry.attempts++
	entry.lastFailure = time.Now()
}

func (bt *backoffTracker) RecordSuccess(key types.NamespacedName) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if entry, exists := bt.entries[key]; exists {
		entry.attempts = 0
		entry.lastFailure = time.Time{}
	}
}

func (bt *backoffTracker) GetAttempts(key types.NamespacedName) int {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	if entry, exists := bt.entries[key]; exists {
		return entry.attempts
	}
	return 0
}

func (bt *backoffTracker) GetBackoff(key types.NamespacedName) time.Duration {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	if entry, exists := bt.entries[key]; exists && entry.attempts > 0 {
		return bt.strategy.NextBackoff(entry.attempts)
	}
	return 0
}

func (bt *backoffTracker) Reset(key types.NamespacedName) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	delete(bt.entries, key)
}

func (bt *backoffTracker) ResetAll() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	bt.entries = make(map[types.NamespacedName]*backoffEntry)
}

func (bt *backoffTracker) GetLastFailure(key types.NamespacedName) time.Time {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	if entry, exists := bt.entries[key]; exists {
		return entry.lastFailure
	}
	return time.Time{}
}

func (bt *backoffTracker) Cleanup(maxAge time.Duration) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for key, entry := range bt.entries {
		if entry.lastAccess.Before(cutoff) {
			delete(bt.entries, key)
		}
	}
}

// RetryPolicy combines a backoff strategy with retry conditions.
type RetryPolicy struct {
	// Strategy is the backoff strategy to use.
	Strategy BackoffStrategy

	// MaxAttempts is the maximum number of attempts (0 = unlimited).
	MaxAttempts int

	// RetryableErrors defines which errors should be retried.
	// If nil, uses default error classification.
	RetryableErrors func(error) bool

	// OnRetry is called before each retry attempt.
	OnRetry func(attempt int, err error, nextDelay time.Duration)
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		Strategy:    ExponentialBackoff(DefaultBackoffConfig()),
		MaxAttempts: 10,
		RetryableErrors: func(err error) bool {
			return IsRetryable(err)
		},
	}
}

// ShouldRetry determines if a retry should be attempted.
func (p *RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if err == nil {
		return false
	}

	if p.MaxAttempts > 0 && attempt >= p.MaxAttempts {
		return false
	}

	if p.RetryableErrors != nil {
		return p.RetryableErrors(err)
	}

	return IsRetryable(err)
}

// NextDelay returns the delay before the next retry.
func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	return p.Strategy.NextBackoff(attempt)
}

// RetryContext provides context for retry operations.
type RetryContext struct {
	// Attempt is the current attempt number (starts at 1).
	Attempt int

	// TotalAttempts is the total number of attempts allowed (0 = unlimited).
	TotalAttempts int

	// LastError is the error from the previous attempt.
	LastError error

	// ElapsedTime is the total time spent retrying.
	ElapsedTime time.Duration

	// NextDelay is the delay before the next retry.
	NextDelay time.Duration
}

// RemainingAttempts returns the number of remaining attempts.
func (rc *RetryContext) RemainingAttempts() int {
	if rc.TotalAttempts == 0 {
		return -1 // Unlimited
	}
	remaining := rc.TotalAttempts - rc.Attempt
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsLastAttempt returns true if this is the final attempt.
func (rc *RetryContext) IsLastAttempt() bool {
	if rc.TotalAttempts == 0 {
		return false
	}
	return rc.Attempt >= rc.TotalAttempts
}

// QuickBackoff returns a fast backoff for transient errors.
func QuickBackoff() BackoffStrategy {
	return ExponentialBackoff(BackoffConfig{
		InitialInterval:     100 * time.Millisecond,
		MaxInterval:         5 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.2,
	})
}

// SlowBackoff returns a slow backoff for rate limiting or external services.
func SlowBackoff() BackoffStrategy {
	return ExponentialBackoff(BackoffConfig{
		InitialInterval:     5 * time.Second,
		MaxInterval:         10 * time.Minute,
		Multiplier:          2.0,
		RandomizationFactor: 0.1,
	})
}
