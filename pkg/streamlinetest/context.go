package streamlinetest

import (
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// FakeContextOptions configures a fake context for testing.
type FakeContextOptions struct {
	object        client.Object
	client        client.Client
	scheme        *runtime.Scheme
	logger        logr.Logger
	existingObjs  []client.Object
	eventRecorder *FakeEventRecorder
}

// FakeContextOption is a functional option for configuring FakeContextOptions.
type FakeContextOption func(*FakeContextOptions)

// WithObject sets the primary object being reconciled.
func WithObject(obj client.Object) FakeContextOption {
	return func(o *FakeContextOptions) {
		o.object = obj
	}
}

// WithClient sets a custom client.
func WithClient(c client.Client) FakeContextOption {
	return func(o *FakeContextOptions) {
		o.client = c
	}
}

// WithScheme sets the runtime scheme.
func WithScheme(scheme *runtime.Scheme) FakeContextOption {
	return func(o *FakeContextOptions) {
		o.scheme = scheme
	}
}

// WithLogger sets a custom logger.
func WithLogger(log logr.Logger) FakeContextOption {
	return func(o *FakeContextOptions) {
		o.logger = log
	}
}

// WithExistingObjects adds objects that should exist in the fake client.
func WithExistingObjects(objs ...client.Object) FakeContextOption {
	return func(o *FakeContextOptions) {
		o.existingObjs = append(o.existingObjs, objs...)
	}
}

// NewFakeContext creates a new Context for testing handlers.
// The context includes:
// - A fake client (or custom client if provided)
// - A discard logger (or custom logger if provided)
// - A fake event recorder that captures events
// - Status, Conditions, and Owned helpers configured for the object
//
// Example:
//
//	ctx := streamlinetest.NewFakeContext(t,
//	    streamlinetest.WithObject(myResource),
//	    streamlinetest.WithExistingObjects(configMap, secret),
//	)
func NewFakeContext(t *testing.T, opts ...FakeContextOption) *TestContext {
	t.Helper()

	options := &FakeContextOptions{
		scheme:        runtime.NewScheme(),
		logger:        logr.Discard(),
		eventRecorder: NewFakeEventRecorder(),
	}

	// Register core types
	_ = corev1.AddToScheme(options.scheme)

	for _, opt := range opts {
		opt(options)
	}

	// Create fake client if not provided
	var fakeClient *FakeClient
	if options.client == nil {
		fakeClient = NewFakeClient(options.existingObjs...)
		options.client = fakeClient
	} else if fc, ok := options.client.(*FakeClient); ok {
		fakeClient = fc
	}

	// Use a placeholder object if none provided
	if options.object == nil {
		options.object = &corev1.ConfigMap{}
	}

	// Create the streamline context
	sCtx := streamline.NewContext(
		options.client,
		options.scheme,
		options.logger,
		options.eventRecorder,
		options.object,
	)

	return &TestContext{
		Context:       sCtx,
		T:             t,
		EventRecorder: options.eventRecorder,
		FakeClient:    fakeClient, // May be nil if custom client provided
	}
}

// TestContext wraps streamline.Context with additional testing utilities.
type TestContext struct {
	*streamline.Context
	T             *testing.T
	EventRecorder *FakeEventRecorder
	FakeClient    *FakeClient
}

// AssertEventRecorded checks that an event was recorded with the given type and reason.
func (tc *TestContext) AssertEventRecorded(eventType, reason string) {
	tc.T.Helper()
	if !tc.EventRecorder.HasEvent(eventType, reason) {
		tc.T.Errorf("Expected event with type=%s reason=%s, but it was not recorded. Events: %v",
			eventType, reason, tc.EventRecorder.Events())
	}
}

// AssertNoEvents checks that no events were recorded.
func (tc *TestContext) AssertNoEvents() {
	tc.T.Helper()
	if len(tc.EventRecorder.Events()) > 0 {
		tc.T.Errorf("Expected no events, but got: %v", tc.EventRecorder.Events())
	}
}

// AssertEventCount checks the number of recorded events.
func (tc *TestContext) AssertEventCount(expected int) {
	tc.T.Helper()
	actual := len(tc.EventRecorder.Events())
	if actual != expected {
		tc.T.Errorf("Expected %d events, got %d: %v", expected, actual, tc.EventRecorder.Events())
	}
}

// ClearEvents removes all recorded events.
func (tc *TestContext) ClearEvents() {
	tc.EventRecorder.Clear()
}

// FakeEventRecorder captures events for testing.
type FakeEventRecorder struct {
	events []RecordedEvent
}

// RecordedEvent represents a captured event.
type RecordedEvent struct {
	Object    runtime.Object
	EventType string
	Reason    string
	Message   string
}

// NewFakeEventRecorder creates a new FakeEventRecorder.
func NewFakeEventRecorder() *FakeEventRecorder {
	return &FakeEventRecorder{
		events: make([]RecordedEvent, 0),
	}
}

// Event records an event.
func (r *FakeEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	r.events = append(r.events, RecordedEvent{
		Object:    object,
		EventType: eventType,
		Reason:    reason,
		Message:   message,
	})
}

// Eventf records a formatted event.
func (r *FakeEventRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventType, reason, messageFmt) // Simplified for testing
}

// AnnotatedEventf records an annotated event.
func (r *FakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventType, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventType, reason, messageFmt, args...)
}

// Events returns all recorded events.
func (r *FakeEventRecorder) Events() []RecordedEvent {
	return r.events
}

// Clear removes all recorded events.
func (r *FakeEventRecorder) Clear() {
	r.events = make([]RecordedEvent, 0)
}

// HasEvent returns true if an event with the given type and reason was recorded.
func (r *FakeEventRecorder) HasEvent(eventType, reason string) bool {
	for _, e := range r.events {
		if e.EventType == eventType && e.Reason == reason {
			return true
		}
	}
	return false
}

// EventCount returns the number of recorded events.
func (r *FakeEventRecorder) EventCount() int {
	return len(r.events)
}

// GetEvents returns events matching the given type and reason.
func (r *FakeEventRecorder) GetEvents(eventType, reason string) []RecordedEvent {
	var result []RecordedEvent
	for _, e := range r.events {
		if e.EventType == eventType && e.Reason == reason {
			result = append(result, e)
		}
	}
	return result
}

// NormalEvents returns all Normal events.
func (r *FakeEventRecorder) NormalEvents() []RecordedEvent {
	var result []RecordedEvent
	for _, e := range r.events {
		if e.EventType == corev1.EventTypeNormal {
			result = append(result, e)
		}
	}
	return result
}

// WarningEvents returns all Warning events.
func (r *FakeEventRecorder) WarningEvents() []RecordedEvent {
	var result []RecordedEvent
	for _, e := range r.events {
		if e.EventType == corev1.EventTypeWarning {
			result = append(result, e)
		}
	}
	return result
}
