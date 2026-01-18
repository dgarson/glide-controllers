package streamline

import (
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorClassification determines how an error should be handled by the reconciler.
type ErrorClassification int

const (
	// ErrorRetryable indicates the operation can be retried with exponential backoff.
	// Use for transient errors like network issues, API rate limiting, or temporary
	// unavailability of dependencies.
	ErrorRetryable ErrorClassification = iota

	// ErrorPermanent indicates the error is permanent and retrying won't help.
	// Use for validation errors, invalid configuration, or unrecoverable states.
	// The controller will set a Failed condition and not requeue.
	ErrorPermanent

	// ErrorTransient indicates a very short-lived error that should be retried immediately.
	// Use for optimistic locking conflicts or brief network blips.
	ErrorTransient

	// ErrorTerminal indicates the resource should be marked as failed and
	// no further reconciliation should occur until the resource is modified.
	ErrorTerminal
)

// String returns a string representation of the error classification.
func (ec ErrorClassification) String() string {
	switch ec {
	case ErrorRetryable:
		return "retryable"
	case ErrorPermanent:
		return "permanent"
	case ErrorTransient:
		return "transient"
	case ErrorTerminal:
		return "terminal"
	default:
		return "unknown"
	}
}

// ClassifiedError wraps an error with classification information for the reconciler.
type ClassifiedError struct {
	// Cause is the underlying error.
	Cause error

	// Classification determines retry behavior.
	Classification ErrorClassification

	// RetryAfter provides a hint for when to retry.
	// Only used for ErrorRetryable classification.
	RetryAfter time.Duration

	// Reason is a short, machine-readable reason for the error.
	// Used for setting condition reasons.
	Reason string

	// Message is a human-readable message describing the error.
	// Used for setting condition messages and events.
	Message string
}

// Error implements the error interface.
func (e *ClassifiedError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return "classified error"
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *ClassifiedError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns true if the error is classified as retryable.
func (e *ClassifiedError) IsRetryable() bool {
	return e.Classification == ErrorRetryable
}

// IsPermanent returns true if the error is classified as permanent.
func (e *ClassifiedError) IsPermanent() bool {
	return e.Classification == ErrorPermanent
}

// IsTransient returns true if the error is classified as transient.
func (e *ClassifiedError) IsTransient() bool {
	return e.Classification == ErrorTransient
}

// IsTerminal returns true if the error is classified as terminal.
func (e *ClassifiedError) IsTerminal() bool {
	return e.Classification == ErrorTerminal
}

// Retryable creates a retryable error that will be retried with exponential backoff.
//
// Use for:
//   - Network errors that may resolve
//   - API rate limiting
//   - Temporary unavailability of dependencies
//   - Optimistic locking conflicts (though Transient may be better)
//
// Example:
//
//	if err := externalAPI.Call(); err != nil {
//	    return streamline.Stop(), streamline.Retryable(err)
//	}
func Retryable(err error) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorRetryable,
		Reason:         ReasonError,
	}
}

// RetryableWithReason creates a retryable error with a specific reason.
func RetryableWithReason(err error, reason, message string) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorRetryable,
		Reason:         reason,
		Message:        message,
	}
}

// RetryableAfter creates a retryable error with a specific retry delay.
//
// Use when you know approximately when the error condition will clear.
//
// Example:
//
//	if rateLimited {
//	    return streamline.Stop(), streamline.RetryableAfter(err, 30*time.Second)
//	}
func RetryableAfter(err error, after time.Duration) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorRetryable,
		RetryAfter:     after,
		Reason:         ReasonError,
	}
}

// Permanent creates a permanent error that won't be retried.
// The controller will set a Failed/Error condition and stop reconciling
// until the resource spec is modified.
//
// Use for:
//   - Validation errors
//   - Invalid configuration
//   - Missing required dependencies that won't appear
//   - Unrecoverable states
//
// Example:
//
//	if spec.Replicas < 0 {
//	    return streamline.Stop(), streamline.Permanent(
//	        fmt.Errorf("replicas cannot be negative: %d", spec.Replicas))
//	}
func Permanent(err error) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorPermanent,
		Reason:         ReasonError,
	}
}

// PermanentWithReason creates a permanent error with a specific reason.
//
// Example:
//
//	return streamline.Stop(), streamline.PermanentWithReason(
//	    err,
//	    streamline.ReasonInvalidSpec,
//	    "Database connection string is malformed")
func PermanentWithReason(err error, reason, message string) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorPermanent,
		Reason:         reason,
		Message:        message,
	}
}

// Transient creates a transient error that will be retried immediately with jitter.
// Use for very short-lived errors like optimistic locking conflicts.
//
// Example:
//
//	if apierrors.IsConflict(err) {
//	    return streamline.Stop(), streamline.Transient(err)
//	}
func Transient(err error) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorTransient,
		Reason:         ReasonError,
	}
}

// Terminal creates a terminal error that stops all reconciliation.
// The resource will be marked as failed and no further reconciliation
// will occur until the resource is deleted and recreated.
//
// Use sparingly for truly unrecoverable situations.
//
// Example:
//
//	if dataCorruption {
//	    return streamline.Stop(), streamline.Terminal(
//	        fmt.Errorf("data corruption detected, manual intervention required"))
//	}
func Terminal(err error) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorTerminal,
		Reason:         ReasonError,
	}
}

// TerminalWithReason creates a terminal error with a specific reason.
func TerminalWithReason(err error, reason, message string) error {
	return &ClassifiedError{
		Cause:          err,
		Classification: ErrorTerminal,
		Reason:         reason,
		Message:        message,
	}
}

// WrapRetryable wraps an error as retryable if it's not already classified.
func WrapRetryable(err error) error {
	if err == nil {
		return nil
	}
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		return err // Already classified
	}
	return Retryable(err)
}

// WrapPermanent wraps an error as permanent if it's not already classified.
func WrapPermanent(err error) error {
	if err == nil {
		return nil
	}
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		return err // Already classified
	}
	return Permanent(err)
}

// ClassifyError returns the classification of an error.
// If the error is not a ClassifiedError, it attempts to classify based on
// common Kubernetes error types.
func ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ErrorRetryable
	}

	// Check if already classified
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		return ce.Classification
	}

	// Classify based on Kubernetes error types
	return classifyKubernetesError(err)
}

// classifyKubernetesError attempts to classify Kubernetes API errors.
func classifyKubernetesError(err error) ErrorClassification {
	// Transient errors - retry immediately
	if apierrors.IsConflict(err) {
		return ErrorTransient
	}
	if apierrors.IsServerTimeout(err) {
		return ErrorTransient
	}

	// Retryable errors - retry with backoff
	if apierrors.IsServiceUnavailable(err) {
		return ErrorRetryable
	}
	if apierrors.IsTooManyRequests(err) {
		return ErrorRetryable
	}
	if apierrors.IsTimeout(err) {
		return ErrorRetryable
	}
	if apierrors.IsInternalError(err) {
		return ErrorRetryable
	}

	// Permanent errors - don't retry
	if apierrors.IsNotFound(err) {
		return ErrorPermanent
	}
	if apierrors.IsBadRequest(err) {
		return ErrorPermanent
	}
	if apierrors.IsInvalid(err) {
		return ErrorPermanent
	}
	if apierrors.IsForbidden(err) {
		return ErrorPermanent
	}
	if apierrors.IsUnauthorized(err) {
		return ErrorPermanent
	}
	if apierrors.IsMethodNotSupported(err) {
		return ErrorPermanent
	}

	// Default to retryable for unknown errors
	return ErrorRetryable
}

// GetRetryAfter returns the retry delay from a classified error, or 0 if not set.
func GetRetryAfter(err error) time.Duration {
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		return ce.RetryAfter
	}

	// Check for Kubernetes rate limit errors
	if status, ok := err.(apierrors.APIStatus); ok {
		if details := status.Status().Details; details != nil {
			if details.RetryAfterSeconds > 0 {
				return time.Duration(details.RetryAfterSeconds) * time.Second
			}
		}
	}

	return 0
}

// GetErrorReason returns the reason from a classified error, or a default.
func GetErrorReason(err error) string {
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		if ce.Reason != "" {
			return ce.Reason
		}
	}
	return ReasonError
}

// GetErrorMessage returns the message from a classified error, or the error string.
func GetErrorMessage(err error) string {
	var ce *ClassifiedError
	if errors.As(err, &ce) {
		if ce.Message != "" {
			return ce.Message
		}
	}
	return err.Error()
}

// IsRetryable returns true if the error should be retried.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	classification := ClassifyError(err)
	return classification == ErrorRetryable || classification == ErrorTransient
}

// IsPermanent returns true if the error should not be retried.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	classification := ClassifyError(err)
	return classification == ErrorPermanent || classification == ErrorTerminal
}

// ErrorList collects multiple errors that occurred during reconciliation.
type ErrorList struct {
	errors []error
}

// NewErrorList creates a new ErrorList.
func NewErrorList() *ErrorList {
	return &ErrorList{}
}

// Add adds an error to the list. Nil errors are ignored.
func (el *ErrorList) Add(err error) {
	if err != nil {
		el.errors = append(el.errors, err)
	}
}

// AddAll adds multiple errors to the list.
func (el *ErrorList) AddAll(errs ...error) {
	for _, err := range errs {
		el.Add(err)
	}
}

// HasErrors returns true if any errors were collected.
func (el *ErrorList) HasErrors() bool {
	return len(el.errors) > 0
}

// Count returns the number of errors.
func (el *ErrorList) Count() int {
	return len(el.errors)
}

// Errors returns all collected errors.
func (el *ErrorList) Errors() []error {
	return el.errors
}

// Error returns a combined error if any errors were collected, nil otherwise.
func (el *ErrorList) Error() error {
	if !el.HasErrors() {
		return nil
	}
	if len(el.errors) == 1 {
		return el.errors[0]
	}
	return &MultiError{Errors: el.errors}
}

// Classification returns the most severe classification among all errors.
// Priority: Terminal > Permanent > Retryable > Transient
func (el *ErrorList) Classification() ErrorClassification {
	if !el.HasErrors() {
		return ErrorRetryable
	}

	hasTerminal := false
	hasPermanent := false
	hasRetryable := false

	for _, err := range el.errors {
		switch ClassifyError(err) {
		case ErrorTerminal:
			hasTerminal = true
		case ErrorPermanent:
			hasPermanent = true
		case ErrorRetryable:
			hasRetryable = true
		}
	}

	if hasTerminal {
		return ErrorTerminal
	}
	if hasPermanent {
		return ErrorPermanent
	}
	if hasRetryable {
		return ErrorRetryable
	}
	return ErrorTransient
}

// MultiError represents multiple errors.
type MultiError struct {
	Errors []error
}

// Error implements the error interface.
func (me *MultiError) Error() string {
	if len(me.Errors) == 0 {
		return "no errors"
	}
	if len(me.Errors) == 1 {
		return me.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred: %v", len(me.Errors), me.Errors)
}

// Unwrap returns the errors for errors.Is/As compatibility.
func (me *MultiError) Unwrap() []error {
	return me.Errors
}

// DependencyError represents an error due to a missing or unready dependency.
type DependencyError struct {
	// DependencyKind is the kind of the dependency (e.g., "Secret", "ConfigMap").
	DependencyKind string

	// DependencyName is the name of the dependency.
	DependencyName string

	// DependencyNamespace is the namespace of the dependency.
	DependencyNamespace string

	// Reason describes why the dependency check failed.
	Reason string

	// Cause is the underlying error, if any.
	Cause error
}

// Error implements the error interface.
func (de *DependencyError) Error() string {
	name := de.DependencyName
	if de.DependencyNamespace != "" {
		name = de.DependencyNamespace + "/" + de.DependencyName
	}

	msg := fmt.Sprintf("dependency %s %s: %s", de.DependencyKind, name, de.Reason)
	if de.Cause != nil {
		msg += fmt.Sprintf(" (cause: %v)", de.Cause)
	}
	return msg
}

// Unwrap returns the underlying error.
func (de *DependencyError) Unwrap() error {
	return de.Cause
}

// NewDependencyNotFoundError creates a DependencyError for a missing dependency.
func NewDependencyNotFoundError(kind, namespace, name string) error {
	return &DependencyError{
		DependencyKind:      kind,
		DependencyName:      name,
		DependencyNamespace: namespace,
		Reason:              "not found",
	}
}

// NewDependencyNotReadyError creates a DependencyError for an unready dependency.
func NewDependencyNotReadyError(kind, namespace, name, reason string) error {
	return &DependencyError{
		DependencyKind:      kind,
		DependencyName:      name,
		DependencyNamespace: namespace,
		Reason:              reason,
	}
}

// ValidationError represents a spec validation error.
type ValidationError struct {
	// Field is the field path that failed validation (e.g., ".spec.replicas").
	Field string

	// Value is the invalid value.
	Value interface{}

	// Reason describes why validation failed.
	Reason string
}

// Error implements the error interface.
func (ve *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s (value: %v)", ve.Field, ve.Reason, ve.Value)
}

// NewValidationError creates a ValidationError.
func NewValidationError(field string, value interface{}, reason string) error {
	return Permanent(&ValidationError{
		Field:  field,
		Value:  value,
		Reason: reason,
	})
}

// NewRequiredFieldError creates a ValidationError for a missing required field.
func NewRequiredFieldError(field string) error {
	return NewValidationError(field, nil, "field is required")
}

// NewInvalidFieldError creates a ValidationError for an invalid field value.
func NewInvalidFieldError(field string, value interface{}, reason string) error {
	return NewValidationError(field, value, reason)
}

// NewImmutableFieldError creates a ValidationError for an attempt to change an immutable field.
func NewImmutableFieldError(field string, oldValue, newValue interface{}) error {
	return NewValidationError(field, newValue, fmt.Sprintf("field is immutable (was: %v)", oldValue))
}
