// Package testing provides test utilities for streamline controllers.
//
// This package includes:
//   - TestContext for setting up test environments
//   - FakeClient with configurable behavior
//   - FakeRecorder for capturing events
//   - Assertions for common validation patterns
//   - Scenario-based testing helpers
package testing

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// TestContext provides a complete testing environment for controllers.
type TestContext struct {
	// Client is the fake Kubernetes client for testing.
	Client client.Client

	// Scheme is the runtime scheme for type information.
	Scheme *runtime.Scheme

	// Recorder captures events for assertions.
	Recorder *FakeRecorder

	// Log is the logger for the test context.
	Log logr.Logger

	// T is the testing context.
	T *testing.T

	// tracker tracks objects for assertions
	tracker *objectTracker

	// mu protects concurrent access
	mu sync.RWMutex
}

// NewTestContext creates a new TestContext for testing.
func NewTestContext(t *testing.T, scheme *runtime.Scheme, objs ...client.Object) *TestContext {
	t.Helper()

	if scheme == nil {
		scheme = runtime.NewScheme()
	}

	// Add core types to scheme
	_ = corev1.AddToScheme(scheme)

	// Build fake client with initial objects
	clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		clientBuilder = clientBuilder.WithObjects(objs...)
	}

	tc := &TestContext{
		Client:   clientBuilder.Build(),
		Scheme:   scheme,
		Recorder: NewFakeRecorder(100),
		Log:      ctrllog.Log.WithName("test"),
		T:        t,
		tracker:  newObjectTracker(),
	}

	return tc
}

// Create adds an object to the fake client.
func (tc *TestContext) Create(ctx context.Context, obj client.Object) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if err := tc.Client.Create(ctx, obj); err != nil {
		return err
	}
	tc.tracker.recordCreate(obj)
	return nil
}

// Update updates an object in the fake client.
func (tc *TestContext) Update(ctx context.Context, obj client.Object) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if err := tc.Client.Update(ctx, obj); err != nil {
		return err
	}
	tc.tracker.recordUpdate(obj)
	return nil
}

// Delete removes an object from the fake client.
func (tc *TestContext) Delete(ctx context.Context, obj client.Object) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if err := tc.Client.Delete(ctx, obj); err != nil {
		return err
	}
	tc.tracker.recordDelete(obj)
	return nil
}

// Get retrieves an object from the fake client.
func (tc *TestContext) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	return tc.Client.Get(ctx, key, obj)
}

// List lists objects from the fake client.
func (tc *TestContext) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return tc.Client.List(ctx, list, opts...)
}

// CreateAll creates multiple objects.
func (tc *TestContext) CreateAll(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		if err := tc.Create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

// GetCreatedObjects returns all objects that were created during the test.
func (tc *TestContext) GetCreatedObjects() []client.Object {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.tracker.created
}

// GetUpdatedObjects returns all objects that were updated during the test.
func (tc *TestContext) GetUpdatedObjects() []client.Object {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.tracker.updated
}

// GetDeletedObjects returns all objects that were deleted during the test.
func (tc *TestContext) GetDeletedObjects() []client.Object {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.tracker.deleted
}

// GetEvents returns all recorded events.
func (tc *TestContext) GetEvents() []Event {
	return tc.Recorder.GetEvents()
}

// ClearEvents clears all recorded events.
func (tc *TestContext) ClearEvents() {
	tc.Recorder.Clear()
}

// ResetTracker clears the object tracker.
func (tc *TestContext) ResetTracker() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.tracker = newObjectTracker()
}

// NewStreamlineContext creates a streamline.Context for testing handlers.
func (tc *TestContext) NewStreamlineContext(obj runtime.Object) *streamline.Context {
	return streamline.NewContext(tc.Client, tc.Log, tc.Recorder, obj)
}

// objectTracker tracks object operations.
type objectTracker struct {
	created []client.Object
	updated []client.Object
	deleted []client.Object
}

func newObjectTracker() *objectTracker {
	return &objectTracker{}
}

func (ot *objectTracker) recordCreate(obj client.Object) {
	ot.created = append(ot.created, obj.DeepCopyObject().(client.Object))
}

func (ot *objectTracker) recordUpdate(obj client.Object) {
	ot.updated = append(ot.updated, obj.DeepCopyObject().(client.Object))
}

func (ot *objectTracker) recordDelete(obj client.Object) {
	ot.deleted = append(ot.deleted, obj.DeepCopyObject().(client.Object))
}

// Event represents a recorded Kubernetes event.
type Event struct {
	// Type is the event type (Normal or Warning).
	Type string

	// Reason is the event reason.
	Reason string

	// Message is the event message.
	Message string

	// Object is the object the event is about.
	ObjectRef ObjectRef

	// Timestamp is when the event was recorded.
	Timestamp time.Time
}

// ObjectRef identifies an object.
type ObjectRef struct {
	Kind      string
	Namespace string
	Name      string
}

// FakeRecorder is a fake event recorder for testing.
type FakeRecorder struct {
	events   []Event
	mu       sync.RWMutex
	maxCount int
}

// NewFakeRecorder creates a new FakeRecorder.
func NewFakeRecorder(maxEvents int) *FakeRecorder {
	return &FakeRecorder{
		events:   make([]Event, 0),
		maxCount: maxEvents,
	}
}

// Event implements record.EventRecorder.
func (r *FakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.events) >= r.maxCount {
		return
	}

	ref := extractObjectRef(object)
	r.events = append(r.events, Event{
		Type:      eventtype,
		Reason:    reason,
		Message:   message,
		ObjectRef: ref,
		Timestamp: time.Now(),
	})
}

// Eventf implements record.EventRecorder.
func (r *FakeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

// AnnotatedEventf implements record.EventRecorder.
func (r *FakeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventtype, reason, messageFmt, args...)
}

// GetEvents returns all recorded events.
func (r *FakeRecorder) GetEvents() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Event, len(r.events))
	copy(result, r.events)
	return result
}

// Clear removes all recorded events.
func (r *FakeRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = r.events[:0]
}

// HasEvent returns true if an event with the given type and reason was recorded.
func (r *FakeRecorder) HasEvent(eventType, reason string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.events {
		if e.Type == eventType && e.Reason == reason {
			return true
		}
	}
	return false
}

// HasEventWithMessage returns true if an event with the given type, reason, and message was recorded.
func (r *FakeRecorder) HasEventWithMessage(eventType, reason, message string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.events {
		if e.Type == eventType && e.Reason == reason && e.Message == message {
			return true
		}
	}
	return false
}

// CountEvents returns the number of events with the given type and reason.
func (r *FakeRecorder) CountEvents(eventType, reason string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, e := range r.events {
		if e.Type == eventType && e.Reason == reason {
			count++
		}
	}
	return count
}

func extractObjectRef(obj runtime.Object) ObjectRef {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return ObjectRef{}
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	return ObjectRef{
		Kind:      gvk.Kind,
		Namespace: accessor.GetNamespace(),
		Name:      accessor.GetName(),
	}
}

// Verify FakeRecorder implements record.EventRecorder
var _ record.EventRecorder = (*FakeRecorder)(nil)

// Assertions provides assertion helpers for tests.
type Assertions struct {
	t  *testing.T
	tc *TestContext
}

// NewAssertions creates a new Assertions helper.
func NewAssertions(t *testing.T, tc *TestContext) *Assertions {
	return &Assertions{t: t, tc: tc}
}

// ObjectExists asserts that an object exists.
func (a *Assertions) ObjectExists(ctx context.Context, key types.NamespacedName, obj client.Object) {
	a.t.Helper()
	err := a.tc.Get(ctx, key, obj)
	if err != nil {
		a.t.Errorf("expected object %s to exist, but got error: %v", key, err)
	}
}

// ObjectNotExists asserts that an object does not exist.
func (a *Assertions) ObjectNotExists(ctx context.Context, key types.NamespacedName, obj client.Object) {
	a.t.Helper()
	err := a.tc.Get(ctx, key, obj)
	if err == nil {
		a.t.Errorf("expected object %s to not exist, but it does", key)
	} else if !apierrors.IsNotFound(err) {
		a.t.Errorf("unexpected error checking if object %s exists: %v", key, err)
	}
}

// ConditionTrue asserts that a condition is True.
func (a *Assertions) ConditionTrue(obj client.Object, conditionType string) {
	a.t.Helper()
	a.assertConditionStatus(obj, conditionType, metav1.ConditionTrue)
}

// ConditionFalse asserts that a condition is False.
func (a *Assertions) ConditionFalse(obj client.Object, conditionType string) {
	a.t.Helper()
	a.assertConditionStatus(obj, conditionType, metav1.ConditionFalse)
}

// ConditionUnknown asserts that a condition is Unknown.
func (a *Assertions) ConditionUnknown(obj client.Object, conditionType string) {
	a.t.Helper()
	a.assertConditionStatus(obj, conditionType, metav1.ConditionUnknown)
}

// ConditionHasReason asserts that a condition has a specific reason.
func (a *Assertions) ConditionHasReason(obj client.Object, conditionType, expectedReason string) {
	a.t.Helper()
	cond := a.getCondition(obj, conditionType)
	if cond == nil {
		a.t.Errorf("expected condition %s to exist", conditionType)
		return
	}
	if cond.Reason != expectedReason {
		a.t.Errorf("expected condition %s to have reason %q, got %q", conditionType, expectedReason, cond.Reason)
	}
}

// ConditionHasMessage asserts that a condition contains a specific message.
func (a *Assertions) ConditionHasMessage(obj client.Object, conditionType, expectedMessage string) {
	a.t.Helper()
	cond := a.getCondition(obj, conditionType)
	if cond == nil {
		a.t.Errorf("expected condition %s to exist", conditionType)
		return
	}
	if cond.Message != expectedMessage {
		a.t.Errorf("expected condition %s to have message %q, got %q", conditionType, expectedMessage, cond.Message)
	}
}

// Phase asserts that an object has a specific phase.
func (a *Assertions) Phase(obj client.Object, expectedPhase string) {
	a.t.Helper()
	if owp, ok := obj.(streamline.ObjectWithPhase); ok {
		actualPhase := owp.GetPhase()
		if actualPhase != expectedPhase {
			a.t.Errorf("expected phase %q, got %q", expectedPhase, actualPhase)
		}
	} else {
		a.t.Error("object does not implement ObjectWithPhase")
	}
}

// ObservedGenerationEquals asserts that observedGeneration equals the object's generation.
func (a *Assertions) ObservedGenerationEquals(obj client.Object) {
	a.t.Helper()
	if owog, ok := obj.(streamline.ObjectWithObservedGeneration); ok {
		observed := owog.GetObservedGeneration()
		generation := obj.GetGeneration()
		if observed != generation {
			a.t.Errorf("expected observedGeneration %d to equal generation %d", observed, generation)
		}
	} else {
		a.t.Error("object does not implement ObjectWithObservedGeneration")
	}
}

// EventRecorded asserts that an event with the given type and reason was recorded.
func (a *Assertions) EventRecorded(eventType, reason string) {
	a.t.Helper()
	if !a.tc.Recorder.HasEvent(eventType, reason) {
		events := a.tc.Recorder.GetEvents()
		a.t.Errorf("expected event (type=%s, reason=%s), but not found. Events: %v", eventType, reason, events)
	}
}

// EventNotRecorded asserts that no event with the given type and reason was recorded.
func (a *Assertions) EventNotRecorded(eventType, reason string) {
	a.t.Helper()
	if a.tc.Recorder.HasEvent(eventType, reason) {
		a.t.Errorf("did not expect event (type=%s, reason=%s), but it was recorded", eventType, reason)
	}
}

// NormalEventRecorded asserts that a Normal event was recorded.
func (a *Assertions) NormalEventRecorded(reason string) {
	a.t.Helper()
	a.EventRecorded(corev1.EventTypeNormal, reason)
}

// WarningEventRecorded asserts that a Warning event was recorded.
func (a *Assertions) WarningEventRecorded(reason string) {
	a.t.Helper()
	a.EventRecorded(corev1.EventTypeWarning, reason)
}

// ResultIsStop asserts that a result indicates Stop.
func (a *Assertions) ResultIsStop(result streamline.Result) {
	a.t.Helper()
	if result.Requeue || result.RequeueAfter > 0 {
		a.t.Errorf("expected Stop() result, got Requeue=%v, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
	}
}

// ResultIsRequeue asserts that a result indicates immediate requeue.
func (a *Assertions) ResultIsRequeue(result streamline.Result) {
	a.t.Helper()
	if !result.Requeue || result.RequeueAfter != 0 {
		a.t.Errorf("expected Requeue() result, got Requeue=%v, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
	}
}

// ResultIsRequeueAfter asserts that a result indicates requeue after a duration.
func (a *Assertions) ResultIsRequeueAfter(result streamline.Result, expected time.Duration) {
	a.t.Helper()
	if result.RequeueAfter != expected {
		a.t.Errorf("expected RequeueAfter(%v), got RequeueAfter=%v", expected, result.RequeueAfter)
	}
}

// NoError asserts that err is nil.
func (a *Assertions) NoError(err error) {
	a.t.Helper()
	if err != nil {
		a.t.Errorf("expected no error, got: %v", err)
	}
}

// Error asserts that err is not nil.
func (a *Assertions) Error(err error) {
	a.t.Helper()
	if err == nil {
		a.t.Error("expected an error, got nil")
	}
}

// ErrorContains asserts that err contains the given substring.
func (a *Assertions) ErrorContains(err error, substr string) {
	a.t.Helper()
	if err == nil {
		a.t.Errorf("expected error containing %q, got nil", substr)
		return
	}
	if !containsString(err.Error(), substr) {
		a.t.Errorf("expected error containing %q, got: %v", substr, err)
	}
}

// OwnerReferenceSet asserts that the object has an owner reference to the owner.
func (a *Assertions) OwnerReferenceSet(obj, owner client.Object) {
	a.t.Helper()
	refs := obj.GetOwnerReferences()
	ownerUID := owner.GetUID()

	for _, ref := range refs {
		if ref.UID == ownerUID {
			return
		}
	}

	a.t.Errorf("expected object to have owner reference to %s/%s", owner.GetNamespace(), owner.GetName())
}

// FinalizerPresent asserts that the object has a specific finalizer.
func (a *Assertions) FinalizerPresent(obj client.Object, finalizer string) {
	a.t.Helper()
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			return
		}
	}
	a.t.Errorf("expected finalizer %q to be present, got: %v", finalizer, obj.GetFinalizers())
}

// FinalizerAbsent asserts that the object does not have a specific finalizer.
func (a *Assertions) FinalizerAbsent(obj client.Object, finalizer string) {
	a.t.Helper()
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			a.t.Errorf("expected finalizer %q to be absent, but it was present", finalizer)
			return
		}
	}
}

func (a *Assertions) assertConditionStatus(obj client.Object, conditionType string, expectedStatus metav1.ConditionStatus) {
	a.t.Helper()
	cond := a.getCondition(obj, conditionType)
	if cond == nil {
		a.t.Errorf("expected condition %s to exist", conditionType)
		return
	}
	if cond.Status != expectedStatus {
		a.t.Errorf("expected condition %s to be %s, got %s", conditionType, expectedStatus, cond.Status)
	}
}

func (a *Assertions) getCondition(obj client.Object, conditionType string) *metav1.Condition {
	if owc, ok := obj.(streamline.ObjectWithConditions); ok {
		for _, c := range owc.GetConditions() {
			if c.Type == conditionType {
				return &c
			}
		}
	}
	return nil
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// HandlerTestSuite provides a test harness for testing handlers.
type HandlerTestSuite[T client.Object] struct {
	Handler streamline.Handler[T]
	TC      *TestContext
	Assert  *Assertions
}

// NewHandlerTestSuite creates a new test suite for a handler.
func NewHandlerTestSuite[T client.Object](t *testing.T, scheme *runtime.Scheme, handler streamline.Handler[T]) *HandlerTestSuite[T] {
	tc := NewTestContext(t, scheme)
	return &HandlerTestSuite[T]{
		Handler: handler,
		TC:      tc,
		Assert:  NewAssertions(t, tc),
	}
}

// RunSync executes the handler's Sync method with the given object.
func (ts *HandlerTestSuite[T]) RunSync(ctx context.Context, obj T) (streamline.Result, error) {
	sCtx := ts.TC.NewStreamlineContext(obj)
	return ts.Handler.Sync(ctx, obj, sCtx)
}

// RunFinalize executes the handler's Finalize method if it implements FinalizingHandler.
func (ts *HandlerTestSuite[T]) RunFinalize(ctx context.Context, obj T) (streamline.Result, error) {
	if fh, ok := any(ts.Handler).(streamline.FinalizingHandler[T]); ok {
		sCtx := ts.TC.NewStreamlineContext(obj)
		return fh.Finalize(ctx, obj, sCtx)
	}
	return streamline.Result{}, fmt.Errorf("handler does not implement FinalizingHandler")
}

// WithObject creates the object in the test context and returns it.
func (ts *HandlerTestSuite[T]) WithObject(ctx context.Context, obj T) T {
	if err := ts.TC.Create(ctx, obj); err != nil {
		ts.TC.T.Fatalf("failed to create object: %v", err)
	}
	return obj
}

// FaultyClient wraps a client with configurable failures for testing error handling.
type FaultyClient struct {
	client.Client
	faults []Fault
	mu     sync.RWMutex
}

// Fault defines a failure to inject.
type Fault struct {
	// Operation is the operation to fail (Get, List, Create, Update, Delete, Patch).
	Operation Operation

	// GVK limits the fault to a specific GroupVersionKind.
	// Empty GVK matches all types.
	GVK schema.GroupVersionKind

	// Error is the error to return.
	Error error

	// Probability is the chance of failure (0.0 to 1.0).
	// 1.0 means always fail.
	Probability float64

	// Count is the number of times to trigger this fault.
	// -1 means infinite.
	Count int

	// triggered tracks how many times this fault has been triggered
	triggered int
}

// Operation represents a client operation.
type Operation string

const (
	OperationGet    Operation = "Get"
	OperationList   Operation = "List"
	OperationCreate Operation = "Create"
	OperationUpdate Operation = "Update"
	OperationDelete Operation = "Delete"
	OperationPatch  Operation = "Patch"
)

// NewFaultyClient creates a new FaultyClient wrapping the given client.
func NewFaultyClient(c client.Client) *FaultyClient {
	return &FaultyClient{
		Client: c,
		faults: make([]Fault, 0),
	}
}

// InjectFault adds a fault to the client.
func (fc *FaultyClient) InjectFault(f Fault) *FaultyClient {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.faults = append(fc.faults, f)
	return fc
}

// FailGet adds a fault that fails Get operations.
func (fc *FaultyClient) FailGet(gvk schema.GroupVersionKind, err error) *FaultyClient {
	return fc.InjectFault(Fault{
		Operation:   OperationGet,
		GVK:         gvk,
		Error:       err,
		Probability: 1.0,
		Count:       -1,
	})
}

// FailGetOnce adds a fault that fails Get operations once.
func (fc *FaultyClient) FailGetOnce(gvk schema.GroupVersionKind, err error) *FaultyClient {
	return fc.InjectFault(Fault{
		Operation:   OperationGet,
		GVK:         gvk,
		Error:       err,
		Probability: 1.0,
		Count:       1,
	})
}

// FailCreate adds a fault that fails Create operations.
func (fc *FaultyClient) FailCreate(gvk schema.GroupVersionKind, err error) *FaultyClient {
	return fc.InjectFault(Fault{
		Operation:   OperationCreate,
		GVK:         gvk,
		Error:       err,
		Probability: 1.0,
		Count:       -1,
	})
}

// FailUpdate adds a fault that fails Update operations.
func (fc *FaultyClient) FailUpdate(gvk schema.GroupVersionKind, err error) *FaultyClient {
	return fc.InjectFault(Fault{
		Operation:   OperationUpdate,
		GVK:         gvk,
		Error:       err,
		Probability: 1.0,
		Count:       -1,
	})
}

// FailUpdateWithConflict adds a fault that fails Update operations with a conflict error.
func (fc *FaultyClient) FailUpdateWithConflict(gvk schema.GroupVersionKind) *FaultyClient {
	return fc.FailUpdate(gvk, apierrors.NewConflict(
		schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind},
		"test",
		fmt.Errorf("conflict"),
	))
}

// ClearFaults removes all injected faults.
func (fc *FaultyClient) ClearFaults() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.faults = fc.faults[:0]
}

// Get implements client.Client.
func (fc *FaultyClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if err := fc.checkFault(OperationGet, obj); err != nil {
		return err
	}
	return fc.Client.Get(ctx, key, obj, opts...)
}

// Create implements client.Client.
func (fc *FaultyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := fc.checkFault(OperationCreate, obj); err != nil {
		return err
	}
	return fc.Client.Create(ctx, obj, opts...)
}

// Update implements client.Client.
func (fc *FaultyClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if err := fc.checkFault(OperationUpdate, obj); err != nil {
		return err
	}
	return fc.Client.Update(ctx, obj, opts...)
}

// Delete implements client.Client.
func (fc *FaultyClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := fc.checkFault(OperationDelete, obj); err != nil {
		return err
	}
	return fc.Client.Delete(ctx, obj, opts...)
}

// Patch implements client.Client.
func (fc *FaultyClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := fc.checkFault(OperationPatch, obj); err != nil {
		return err
	}
	return fc.Client.Patch(ctx, obj, patch, opts...)
}

func (fc *FaultyClient) checkFault(op Operation, obj client.Object) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	gvk := obj.GetObjectKind().GroupVersionKind()

	for i := range fc.faults {
		f := &fc.faults[i]
		if f.Operation != op {
			continue
		}

		// Check GVK match
		if f.GVK.Kind != "" && !reflect.DeepEqual(f.GVK, gvk) {
			continue
		}

		// Check count
		if f.Count != -1 && f.triggered >= f.Count {
			continue
		}

		// Check probability (simplified - always trigger if > 0)
		if f.Probability > 0 {
			f.triggered++
			return f.Error
		}
	}

	return nil
}

// Status implements client.Client.
func (fc *FaultyClient) Status() client.StatusWriter {
	return &faultyStatusWriter{
		StatusWriter: fc.Client.Status(),
		fc:           fc,
	}
}

type faultyStatusWriter struct {
	client.StatusWriter
	fc *FaultyClient
}

func (fsw *faultyStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if err := fsw.fc.checkFault(OperationUpdate, obj); err != nil {
		return err
	}
	return fsw.StatusWriter.Update(ctx, obj, opts...)
}

func (fsw *faultyStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if err := fsw.fc.checkFault(OperationPatch, obj); err != nil {
		return err
	}
	return fsw.StatusWriter.Patch(ctx, obj, patch, opts...)
}
