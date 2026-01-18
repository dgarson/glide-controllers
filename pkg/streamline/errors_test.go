package streamline

import (
	"errors"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestErrorClassification_String(t *testing.T) {
	tests := []struct {
		classification ErrorClassification
		expected       string
	}{
		{ErrorRetryable, "retryable"},
		{ErrorPermanent, "permanent"},
		{ErrorTransient, "transient"},
		{ErrorTerminal, "terminal"},
		{ErrorClassification(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.classification.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifiedError(t *testing.T) {
	cause := errors.New("test error")

	t.Run("Error returns cause message", func(t *testing.T) {
		ce := &ClassifiedError{Cause: cause, Classification: ErrorRetryable}
		if ce.Error() != "test error" {
			t.Errorf("Error() = %v, want %v", ce.Error(), "test error")
		}
	})

	t.Run("Error returns message when no cause", func(t *testing.T) {
		ce := &ClassifiedError{Message: "custom message", Classification: ErrorPermanent}
		if ce.Error() != "custom message" {
			t.Errorf("Error() = %v, want %v", ce.Error(), "custom message")
		}
	})

	t.Run("Error returns default when no cause or message", func(t *testing.T) {
		ce := &ClassifiedError{Classification: ErrorTransient}
		if ce.Error() != "classified error" {
			t.Errorf("Error() = %v, want %v", ce.Error(), "classified error")
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		ce := &ClassifiedError{Cause: cause}
		if ce.Unwrap() != cause {
			t.Errorf("Unwrap() did not return cause")
		}
	})

	t.Run("IsRetryable", func(t *testing.T) {
		ce := &ClassifiedError{Classification: ErrorRetryable}
		if !ce.IsRetryable() {
			t.Error("IsRetryable() should be true")
		}
		ce.Classification = ErrorPermanent
		if ce.IsRetryable() {
			t.Error("IsRetryable() should be false for permanent")
		}
	})

	t.Run("IsPermanent", func(t *testing.T) {
		ce := &ClassifiedError{Classification: ErrorPermanent}
		if !ce.IsPermanent() {
			t.Error("IsPermanent() should be true")
		}
	})

	t.Run("IsTransient", func(t *testing.T) {
		ce := &ClassifiedError{Classification: ErrorTransient}
		if !ce.IsTransient() {
			t.Error("IsTransient() should be true")
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		ce := &ClassifiedError{Classification: ErrorTerminal}
		if !ce.IsTerminal() {
			t.Error("IsTerminal() should be true")
		}
	})
}

func TestRetryable(t *testing.T) {
	cause := errors.New("network error")
	err := Retryable(cause)

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("Retryable should return ClassifiedError")
	}

	if ce.Classification != ErrorRetryable {
		t.Errorf("Classification = %v, want %v", ce.Classification, ErrorRetryable)
	}

	if !errors.Is(err, cause) {
		t.Error("errors.Is should find cause")
	}
}

func TestRetryableWithReason(t *testing.T) {
	cause := errors.New("api error")
	err := RetryableWithReason(cause, "RateLimited", "API rate limit exceeded")

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("RetryableWithReason should return ClassifiedError")
	}

	if ce.Reason != "RateLimited" {
		t.Errorf("Reason = %v, want %v", ce.Reason, "RateLimited")
	}
	if ce.Message != "API rate limit exceeded" {
		t.Errorf("Message = %v, want %v", ce.Message, "API rate limit exceeded")
	}
}

func TestRetryableAfter(t *testing.T) {
	cause := errors.New("rate limited")
	delay := 30 * time.Second
	err := RetryableAfter(cause, delay)

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("RetryableAfter should return ClassifiedError")
	}

	if ce.RetryAfter != delay {
		t.Errorf("RetryAfter = %v, want %v", ce.RetryAfter, delay)
	}
}

func TestPermanent(t *testing.T) {
	cause := errors.New("validation error")
	err := Permanent(cause)

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("Permanent should return ClassifiedError")
	}

	if ce.Classification != ErrorPermanent {
		t.Errorf("Classification = %v, want %v", ce.Classification, ErrorPermanent)
	}
}

func TestPermanentWithReason(t *testing.T) {
	cause := errors.New("invalid spec")
	err := PermanentWithReason(cause, ReasonInvalidSpec, "Replicas must be positive")

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("PermanentWithReason should return ClassifiedError")
	}

	if ce.Reason != ReasonInvalidSpec {
		t.Errorf("Reason = %v, want %v", ce.Reason, ReasonInvalidSpec)
	}
}

func TestTransient(t *testing.T) {
	cause := errors.New("conflict")
	err := Transient(cause)

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("Transient should return ClassifiedError")
	}

	if ce.Classification != ErrorTransient {
		t.Errorf("Classification = %v, want %v", ce.Classification, ErrorTransient)
	}
}

func TestTerminal(t *testing.T) {
	cause := errors.New("data corruption")
	err := Terminal(cause)

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("Terminal should return ClassifiedError")
	}

	if ce.Classification != ErrorTerminal {
		t.Errorf("Classification = %v, want %v", ce.Classification, ErrorTerminal)
	}
}

func TestTerminalWithReason(t *testing.T) {
	cause := errors.New("unrecoverable")
	err := TerminalWithReason(cause, "DataCorruption", "Manual intervention required")

	var ce *ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatal("TerminalWithReason should return ClassifiedError")
	}

	if ce.Reason != "DataCorruption" {
		t.Errorf("Reason = %v, want %v", ce.Reason, "DataCorruption")
	}
}

func TestWrapRetryable(t *testing.T) {
	t.Run("wraps unclassified error", func(t *testing.T) {
		err := errors.New("plain error")
		wrapped := WrapRetryable(err)

		var ce *ClassifiedError
		if !errors.As(wrapped, &ce) {
			t.Fatal("WrapRetryable should wrap as ClassifiedError")
		}
		if ce.Classification != ErrorRetryable {
			t.Errorf("Classification = %v, want %v", ce.Classification, ErrorRetryable)
		}
	})

	t.Run("does not wrap already classified error", func(t *testing.T) {
		original := Permanent(errors.New("permanent"))
		wrapped := WrapRetryable(original)

		var ce *ClassifiedError
		if !errors.As(wrapped, &ce) {
			t.Fatal("Should be ClassifiedError")
		}
		if ce.Classification != ErrorPermanent {
			t.Errorf("Classification changed from %v to retryable", ErrorPermanent)
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		if WrapRetryable(nil) != nil {
			t.Error("WrapRetryable(nil) should return nil")
		}
	})
}

func TestWrapPermanent(t *testing.T) {
	t.Run("wraps unclassified error", func(t *testing.T) {
		err := errors.New("plain error")
		wrapped := WrapPermanent(err)

		var ce *ClassifiedError
		if !errors.As(wrapped, &ce) {
			t.Fatal("WrapPermanent should wrap as ClassifiedError")
		}
		if ce.Classification != ErrorPermanent {
			t.Errorf("Classification = %v, want %v", ce.Classification, ErrorPermanent)
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		if WrapPermanent(nil) != nil {
			t.Error("WrapPermanent(nil) should return nil")
		}
	})
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorClassification
	}{
		{"nil error", nil, ErrorRetryable},
		{"already classified retryable", Retryable(errors.New("test")), ErrorRetryable},
		{"already classified permanent", Permanent(errors.New("test")), ErrorPermanent},
		{"already classified transient", Transient(errors.New("test")), ErrorTransient},
		{"already classified terminal", Terminal(errors.New("test")), ErrorTerminal},
		{"conflict error", apierrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict")), ErrorTransient},
		{"not found error", apierrors.NewNotFound(schema.GroupResource{}, "test"), ErrorPermanent},
		{"bad request error", apierrors.NewBadRequest("bad"), ErrorPermanent},
		{"invalid error", apierrors.NewInvalid(schema.GroupKind{}, "test", nil), ErrorPermanent},
		{"forbidden error", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), ErrorPermanent},
		{"unauthorized error", apierrors.NewUnauthorized("unauthorized"), ErrorPermanent},
		{"service unavailable", apierrors.NewServiceUnavailable("unavailable"), ErrorRetryable},
		{"too many requests", apierrors.NewTooManyRequests("throttled", 10), ErrorRetryable},
		{"internal error", apierrors.NewInternalError(errors.New("internal")), ErrorRetryable},
		{"unknown error", errors.New("unknown"), ErrorRetryable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.expected {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetRetryAfter(t *testing.T) {
	t.Run("from ClassifiedError", func(t *testing.T) {
		err := RetryableAfter(errors.New("test"), 30*time.Second)
		got := GetRetryAfter(err)
		if got != 30*time.Second {
			t.Errorf("GetRetryAfter() = %v, want %v", got, 30*time.Second)
		}
	})

	t.Run("from non-classified error", func(t *testing.T) {
		err := errors.New("test")
		got := GetRetryAfter(err)
		if got != 0 {
			t.Errorf("GetRetryAfter() = %v, want 0", got)
		}
	})

	t.Run("from API status with retry after", func(t *testing.T) {
		err := apierrors.NewTooManyRequests("throttled", 60)
		got := GetRetryAfter(err)
		if got != 60*time.Second {
			t.Errorf("GetRetryAfter() = %v, want %v", got, 60*time.Second)
		}
	})
}

func TestGetErrorReason(t *testing.T) {
	t.Run("from ClassifiedError with reason", func(t *testing.T) {
		err := RetryableWithReason(errors.New("test"), "CustomReason", "message")
		got := GetErrorReason(err)
		if got != "CustomReason" {
			t.Errorf("GetErrorReason() = %v, want %v", got, "CustomReason")
		}
	})

	t.Run("from ClassifiedError without reason", func(t *testing.T) {
		err := Retryable(errors.New("test"))
		got := GetErrorReason(err)
		if got != ReasonError {
			t.Errorf("GetErrorReason() = %v, want %v", got, ReasonError)
		}
	})

	t.Run("from non-classified error", func(t *testing.T) {
		err := errors.New("test")
		got := GetErrorReason(err)
		if got != ReasonError {
			t.Errorf("GetErrorReason() = %v, want %v", got, ReasonError)
		}
	})
}

func TestGetErrorMessage(t *testing.T) {
	t.Run("from ClassifiedError with message", func(t *testing.T) {
		err := RetryableWithReason(errors.New("cause"), "reason", "custom message")
		got := GetErrorMessage(err)
		if got != "custom message" {
			t.Errorf("GetErrorMessage() = %v, want %v", got, "custom message")
		}
	})

	t.Run("from ClassifiedError without message", func(t *testing.T) {
		err := Retryable(errors.New("cause error"))
		got := GetErrorMessage(err)
		if got != "cause error" {
			t.Errorf("GetErrorMessage() = %v, want %v", got, "cause error")
		}
	})
}

func TestIsRetryable(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("IsRetryable(nil) should be false")
	}
	if !IsRetryable(Retryable(errors.New("test"))) {
		t.Error("IsRetryable should be true for retryable error")
	}
	if !IsRetryable(Transient(errors.New("test"))) {
		t.Error("IsRetryable should be true for transient error")
	}
	if IsRetryable(Permanent(errors.New("test"))) {
		t.Error("IsRetryable should be false for permanent error")
	}
}

func TestIsPermanent(t *testing.T) {
	if IsPermanent(nil) {
		t.Error("IsPermanent(nil) should be false")
	}
	if !IsPermanent(Permanent(errors.New("test"))) {
		t.Error("IsPermanent should be true for permanent error")
	}
	if !IsPermanent(Terminal(errors.New("test"))) {
		t.Error("IsPermanent should be true for terminal error")
	}
	if IsPermanent(Retryable(errors.New("test"))) {
		t.Error("IsPermanent should be false for retryable error")
	}
}

func TestErrorList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		el := NewErrorList()
		if el.HasErrors() {
			t.Error("HasErrors() should be false for empty list")
		}
		if el.Count() != 0 {
			t.Errorf("Count() = %v, want 0", el.Count())
		}
		if el.Error() != nil {
			t.Error("Error() should be nil for empty list")
		}
	})

	t.Run("single error", func(t *testing.T) {
		el := NewErrorList()
		el.Add(errors.New("error1"))

		if !el.HasErrors() {
			t.Error("HasErrors() should be true")
		}
		if el.Count() != 1 {
			t.Errorf("Count() = %v, want 1", el.Count())
		}
		if el.Error().Error() != "error1" {
			t.Errorf("Error() = %v, want error1", el.Error())
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		el := NewErrorList()
		el.Add(errors.New("error1"))
		el.Add(errors.New("error2"))
		el.AddAll(errors.New("error3"), nil) // nil should be ignored

		if el.Count() != 3 {
			t.Errorf("Count() = %v, want 3", el.Count())
		}
	})

	t.Run("nil error ignored", func(t *testing.T) {
		el := NewErrorList()
		el.Add(nil)
		if el.HasErrors() {
			t.Error("nil error should be ignored")
		}
	})

	t.Run("classification priority", func(t *testing.T) {
		el := NewErrorList()
		el.Add(Retryable(errors.New("retryable")))
		el.Add(Permanent(errors.New("permanent")))
		el.Add(Transient(errors.New("transient")))

		// Permanent takes priority over retryable and transient
		if el.Classification() != ErrorPermanent {
			t.Errorf("Classification() = %v, want %v", el.Classification(), ErrorPermanent)
		}
	})

	t.Run("terminal takes highest priority", func(t *testing.T) {
		el := NewErrorList()
		el.Add(Permanent(errors.New("permanent")))
		el.Add(Terminal(errors.New("terminal")))

		if el.Classification() != ErrorTerminal {
			t.Errorf("Classification() = %v, want %v", el.Classification(), ErrorTerminal)
		}
	})
}

func TestMultiError(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		me := &MultiError{}
		if me.Error() != "no errors" {
			t.Errorf("Error() = %v, want 'no errors'", me.Error())
		}
	})

	t.Run("single", func(t *testing.T) {
		me := &MultiError{Errors: []error{errors.New("single")}}
		if me.Error() != "single" {
			t.Errorf("Error() = %v, want 'single'", me.Error())
		}
	})

	t.Run("multiple", func(t *testing.T) {
		me := &MultiError{Errors: []error{errors.New("e1"), errors.New("e2")}}
		if me.Error() != "2 errors occurred: [e1 e2]" {
			t.Errorf("Error() = %v", me.Error())
		}
	})

	t.Run("unwrap", func(t *testing.T) {
		errs := []error{errors.New("e1"), errors.New("e2")}
		me := &MultiError{Errors: errs}
		unwrapped := me.Unwrap()
		if len(unwrapped) != 2 {
			t.Errorf("Unwrap() returned %d errors, want 2", len(unwrapped))
		}
	})
}

func TestDependencyError(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		de := &DependencyError{
			DependencyKind: "Secret",
			DependencyName: "db-creds",
			Reason:         "not found",
		}
		expected := "dependency Secret db-creds: not found"
		if de.Error() != expected {
			t.Errorf("Error() = %v, want %v", de.Error(), expected)
		}
	})

	t.Run("with namespace", func(t *testing.T) {
		de := &DependencyError{
			DependencyKind:      "ConfigMap",
			DependencyName:      "config",
			DependencyNamespace: "default",
			Reason:              "missing key",
		}
		if de.Error() != "dependency ConfigMap default/config: missing key" {
			t.Errorf("Error() = %v", de.Error())
		}
	})

	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("network error")
		de := &DependencyError{
			DependencyKind: "Secret",
			DependencyName: "creds",
			Reason:         "unavailable",
			Cause:          cause,
		}
		if de.Unwrap() != cause {
			t.Error("Unwrap() should return cause")
		}
	})
}

func TestNewDependencyNotFoundError(t *testing.T) {
	err := NewDependencyNotFoundError("Secret", "ns", "name")
	var de *DependencyError
	if !errors.As(err, &de) {
		t.Fatal("Should be DependencyError")
	}
	if de.Reason != "not found" {
		t.Errorf("Reason = %v, want 'not found'", de.Reason)
	}
}

func TestNewDependencyNotReadyError(t *testing.T) {
	err := NewDependencyNotReadyError("Database", "ns", "db", "initializing")
	var de *DependencyError
	if !errors.As(err, &de) {
		t.Fatal("Should be DependencyError")
	}
	if de.Reason != "initializing" {
		t.Errorf("Reason = %v, want 'initializing'", de.Reason)
	}
}

func TestValidationError(t *testing.T) {
	ve := &ValidationError{
		Field:  ".spec.replicas",
		Value:  -1,
		Reason: "must be positive",
	}
	expected := "validation failed for .spec.replicas: must be positive (value: -1)"
	if ve.Error() != expected {
		t.Errorf("Error() = %v, want %v", ve.Error(), expected)
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError(".spec.name", "", "required")
	// Should be wrapped as permanent
	if !IsPermanent(err) {
		t.Error("Validation error should be permanent")
	}
}

func TestNewRequiredFieldError(t *testing.T) {
	err := NewRequiredFieldError(".spec.image")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("Should contain ValidationError")
	}
	if ve.Reason != "field is required" {
		t.Errorf("Reason = %v", ve.Reason)
	}
}

func TestNewInvalidFieldError(t *testing.T) {
	err := NewInvalidFieldError(".spec.replicas", -5, "must be non-negative")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("Should contain ValidationError")
	}
	if ve.Value != -5 {
		t.Errorf("Value = %v, want -5", ve.Value)
	}
}

func TestNewImmutableFieldError(t *testing.T) {
	err := NewImmutableFieldError(".spec.storageClass", "standard", "premium")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("Should contain ValidationError")
	}
	if ve.Field != ".spec.storageClass" {
		t.Errorf("Field = %v", ve.Field)
	}
}

// Helper to create API errors with retry-after
func createTooManyRequestsWithRetry(seconds int64) error {
	return &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    429,
			Reason:  metav1.StatusReasonTooManyRequests,
			Message: "too many requests",
			Details: &metav1.StatusDetails{
				RetryAfterSeconds: int32(seconds),
			},
		},
	}
}

func TestGetRetryAfterFromAPIStatus(t *testing.T) {
	err := createTooManyRequestsWithRetry(45)
	got := GetRetryAfter(err)
	if got != 45*time.Second {
		t.Errorf("GetRetryAfter() = %v, want %v", got, 45*time.Second)
	}
}

func TestErrorsIntegration(t *testing.T) {
	// Test that errors work correctly with errors.Is and errors.As
	t.Run("errors.Is with wrapped error", func(t *testing.T) {
		sentinel := errors.New("sentinel")
		wrapped := Retryable(fmt.Errorf("wrapped: %w", sentinel))

		if !errors.Is(wrapped, sentinel) {
			t.Error("errors.Is should find sentinel through classification wrapper")
		}
	})

	t.Run("errors.As with wrapped error", func(t *testing.T) {
		de := &DependencyError{
			DependencyKind: "Secret",
			DependencyName: "test",
			Reason:         "not found",
		}
		wrapped := Retryable(de)

		var found *DependencyError
		if !errors.As(wrapped, &found) {
			t.Error("errors.As should find DependencyError through classification wrapper")
		}
	})
}
