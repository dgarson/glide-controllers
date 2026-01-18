package streamline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed indicates the circuit is closed and requests are allowed.
	CircuitClosed CircuitState = iota

	// CircuitOpen indicates the circuit is open and requests are blocked.
	CircuitOpen

	// CircuitHalfOpen indicates the circuit is testing if the service has recovered.
	CircuitHalfOpen
)

// String returns a string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Common circuit breaker errors.
var (
	// ErrCircuitOpen is returned when the circuit is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrTooManyRequests is returned when too many requests are in flight during half-open.
	ErrTooManyRequests = errors.New("too many requests during half-open state")
)

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies the circuit breaker.
	Name string

	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int

	// SuccessThreshold is the number of successes in half-open to close the circuit.
	SuccessThreshold int

	// Timeout is how long to wait before transitioning from open to half-open.
	Timeout time.Duration

	// MaxHalfOpenRequests is the maximum number of requests allowed in half-open state.
	MaxHalfOpenRequests int

	// OnStateChange is called when the circuit state changes.
	OnStateChange func(name string, from, to CircuitState)

	// IsFailure determines if an error should count as a failure.
	// If nil, all non-nil errors are considered failures.
	IsFailure func(err error) bool
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                name,
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config CircuitBreakerConfig

	state             CircuitState
	failures          int
	successes         int
	lastStateChange   time.Time
	halfOpenRequests  int

	mu sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxHalfOpenRequests <= 0 {
		config.MaxHalfOpenRequests = 1
	}

	return &CircuitBreaker{
		config:          config,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
	}
}

// Execute runs a function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	cb.beforeRequest()
	err := fn()
	cb.afterRequest(err)

	return err
}

// ExecuteWithContext runs a function with circuit breaker protection and context.
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	cb.beforeRequest()
	err := fn(ctx)
	cb.afterRequest(err)

	return err
}

// ExecuteWithFallback runs a function with circuit breaker protection and a fallback.
func (cb *CircuitBreaker) ExecuteWithFallback(fn func() error, fallback func(error) error) error {
	err := cb.Execute(fn)
	if err != nil && errors.Is(err, ErrCircuitOpen) {
		return fallback(err)
	}
	return err
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Check if we should transition from open to half-open
	if cb.state == CircuitOpen && time.Since(cb.lastStateChange) >= cb.config.Timeout {
		return CircuitHalfOpen
	}

	return cb.state
}

// Name returns the circuit breaker name.
func (cb *CircuitBreaker) Name() string {
	return cb.config.Name
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()

	if oldState != CircuitClosed && cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, CircuitClosed)
	}
}

// ForceOpen forces the circuit to open state.
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = CircuitOpen
	cb.lastStateChange = time.Now()

	if oldState != CircuitOpen && cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, CircuitOpen)
	}
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastStateChange) >= cb.config.Timeout {
			cb.transitionTo(CircuitHalfOpen)
			return cb.halfOpenRequests < cb.config.MaxHalfOpenRequests
		}
		return false

	case CircuitHalfOpen:
		return cb.halfOpenRequests < cb.config.MaxHalfOpenRequests

	default:
		return false
	}
}

func (cb *CircuitBreaker) beforeRequest() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenRequests++
	}
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	isFailure := err != nil
	if cb.config.IsFailure != nil && err != nil {
		isFailure = cb.config.IsFailure(err)
	}

	if isFailure {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case CircuitClosed:
		cb.failures = 0

	case CircuitHalfOpen:
		cb.successes++
		cb.halfOpenRequests--
		if cb.successes >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}
	}
}

func (cb *CircuitBreaker) onFailure() {
	switch cb.state {
	case CircuitClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}

	case CircuitHalfOpen:
		cb.halfOpenRequests--
		cb.transitionTo(CircuitOpen)
	}
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	switch newState {
	case CircuitClosed:
		cb.failures = 0
		cb.successes = 0
		cb.halfOpenRequests = 0
	case CircuitOpen:
		cb.successes = 0
		cb.halfOpenRequests = 0
	case CircuitHalfOpen:
		cb.successes = 0
		cb.halfOpenRequests = 0
	}

	if cb.config.OnStateChange != nil && oldState != newState {
		cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// CircuitBreakerRegistry manages multiple circuit breakers.
type CircuitBreakerRegistry interface {
	// Get returns the circuit breaker with the given name, creating it if needed.
	Get(name string) *CircuitBreaker

	// GetWithConfig returns a circuit breaker with a specific configuration.
	GetWithConfig(config CircuitBreakerConfig) *CircuitBreaker

	// Remove removes a circuit breaker from the registry.
	Remove(name string)

	// List returns all circuit breaker names.
	List() []string

	// Reset resets all circuit breakers.
	Reset()

	// States returns the current state of all circuit breakers.
	States() map[string]CircuitState
}

// circuitBreakerRegistry implements CircuitBreakerRegistry.
type circuitBreakerRegistry struct {
	breakers      map[string]*CircuitBreaker
	defaultConfig func(name string) CircuitBreakerConfig
	mu            sync.RWMutex
}

// NewCircuitBreakerRegistry creates a new registry.
func NewCircuitBreakerRegistry() CircuitBreakerRegistry {
	return &circuitBreakerRegistry{
		breakers:      make(map[string]*CircuitBreaker),
		defaultConfig: DefaultCircuitBreakerConfig,
	}
}

// NewCircuitBreakerRegistryWithDefaults creates a registry with custom defaults.
func NewCircuitBreakerRegistryWithDefaults(defaultConfig func(name string) CircuitBreakerConfig) CircuitBreakerRegistry {
	return &circuitBreakerRegistry{
		breakers:      make(map[string]*CircuitBreaker),
		defaultConfig: defaultConfig,
	}
}

func (r *circuitBreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[name]
	r.mu.RUnlock()

	if exists {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists = r.breakers[name]; exists {
		return cb
	}

	cb = NewCircuitBreaker(r.defaultConfig(name))
	r.breakers[name] = cb
	return cb
}

func (r *circuitBreakerRegistry) GetWithConfig(config CircuitBreakerConfig) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, exists := r.breakers[config.Name]; exists {
		return cb
	}

	cb := NewCircuitBreaker(config)
	r.breakers[config.Name] = cb
	return cb
}

func (r *circuitBreakerRegistry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.breakers, name)
}

func (r *circuitBreakerRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.breakers))
	for name := range r.breakers {
		names = append(names, name)
	}
	return names
}

func (r *circuitBreakerRegistry) Reset() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}

func (r *circuitBreakerRegistry) States() map[string]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make(map[string]CircuitState, len(r.breakers))
	for name, cb := range r.breakers {
		states[name] = cb.State()
	}
	return states
}

// CircuitBreakerError wraps an error with circuit breaker context.
type CircuitBreakerError struct {
	// Name is the circuit breaker name.
	Name string

	// State is the circuit state when the error occurred.
	State CircuitState

	// Cause is the underlying error.
	Cause error
}

// Error implements the error interface.
func (e *CircuitBreakerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("circuit breaker %s (%s): %v", e.Name, e.State, e.Cause)
	}
	return fmt.Sprintf("circuit breaker %s is %s", e.Name, e.State)
}

// Unwrap returns the underlying error.
func (e *CircuitBreakerError) Unwrap() error {
	return e.Cause
}

// IsCircuitBreakerError checks if an error is a CircuitBreakerError.
func IsCircuitBreakerError(err error) bool {
	var cbErr *CircuitBreakerError
	return errors.As(err, &cbErr)
}

// IsCircuitOpen checks if an error is due to an open circuit.
func IsCircuitOpen(err error) bool {
	return errors.Is(err, ErrCircuitOpen)
}

// CircuitBreakerMiddleware wraps a function with circuit breaker protection.
type CircuitBreakerMiddleware struct {
	registry CircuitBreakerRegistry
}

// NewCircuitBreakerMiddleware creates a new middleware.
func NewCircuitBreakerMiddleware(registry CircuitBreakerRegistry) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{registry: registry}
}

// Wrap wraps a function with circuit breaker protection.
func (m *CircuitBreakerMiddleware) Wrap(name string, fn func() error) error {
	return m.registry.Get(name).Execute(fn)
}

// WrapWithContext wraps a function with context and circuit breaker protection.
func (m *CircuitBreakerMiddleware) WrapWithContext(name string, ctx context.Context, fn func(context.Context) error) error {
	return m.registry.Get(name).ExecuteWithContext(ctx, fn)
}

// TwoStageCircuitBreaker provides separate circuits for different operation types.
type TwoStageCircuitBreaker struct {
	read  *CircuitBreaker
	write *CircuitBreaker
}

// NewTwoStageCircuitBreaker creates a two-stage circuit breaker.
func NewTwoStageCircuitBreaker(name string, readConfig, writeConfig CircuitBreakerConfig) *TwoStageCircuitBreaker {
	readConfig.Name = name + "-read"
	writeConfig.Name = name + "-write"

	return &TwoStageCircuitBreaker{
		read:  NewCircuitBreaker(readConfig),
		write: NewCircuitBreaker(writeConfig),
	}
}

// ExecuteRead executes a read operation.
func (cb *TwoStageCircuitBreaker) ExecuteRead(fn func() error) error {
	return cb.read.Execute(fn)
}

// ExecuteWrite executes a write operation.
func (cb *TwoStageCircuitBreaker) ExecuteWrite(fn func() error) error {
	return cb.write.Execute(fn)
}

// ReadState returns the read circuit state.
func (cb *TwoStageCircuitBreaker) ReadState() CircuitState {
	return cb.read.State()
}

// WriteState returns the write circuit state.
func (cb *TwoStageCircuitBreaker) WriteState() CircuitState {
	return cb.write.State()
}

// Reset resets both circuits.
func (cb *TwoStageCircuitBreaker) Reset() {
	cb.read.Reset()
	cb.write.Reset()
}
