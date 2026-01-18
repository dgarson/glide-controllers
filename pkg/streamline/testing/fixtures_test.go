package testing

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// =============================================================================
// Test Objects - ConfigMap-based mock objects for testing
// =============================================================================

// testObjectWithConditions wraps ConfigMap with conditions support for testing.
type testObjectWithConditions struct {
	corev1.ConfigMap
	conditions []metav1.Condition
}

func (t *testObjectWithConditions) GetConditions() []metav1.Condition {
	return t.conditions
}

func (t *testObjectWithConditions) SetConditions(conditions []metav1.Condition) {
	t.conditions = conditions
}

func (t *testObjectWithConditions) DeepCopyObject() runtime.Object {
	copy := &testObjectWithConditions{
		ConfigMap:  *t.ConfigMap.DeepCopy(),
		conditions: make([]metav1.Condition, len(t.conditions)),
	}
	for i, c := range t.conditions {
		copy.conditions[i] = *c.DeepCopy()
	}
	return copy
}

// testObjectWithPhase wraps ConfigMap with phase support for testing.
type testObjectWithPhase struct {
	corev1.ConfigMap
	phase string
}

func (t *testObjectWithPhase) GetPhase() string {
	return t.phase
}

func (t *testObjectWithPhase) SetPhase(phase string) {
	t.phase = phase
}

func (t *testObjectWithPhase) DeepCopyObject() runtime.Object {
	return &testObjectWithPhase{
		ConfigMap: *t.ConfigMap.DeepCopy(),
		phase:     t.phase,
	}
}

// mockHandler is a simple mock handler for testing HandlerTestSuite.
type mockHandler struct {
	syncFunc     func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error)
	finalizeFunc func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error)
}

func (m *mockHandler) Sync(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
	if m.syncFunc != nil {
		return m.syncFunc(ctx, obj, sCtx)
	}
	return streamline.Stop(), nil
}

func (m *mockHandler) Finalize(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
	if m.finalizeFunc != nil {
		return m.finalizeFunc(ctx, obj, sCtx)
	}
	return streamline.Stop(), nil
}

// =============================================================================
// TestContext Tests
// =============================================================================

func TestNewTestContext(t *testing.T) {
	t.Run("with nil scheme creates default scheme", func(t *testing.T) {
		tc := NewTestContext(t, nil)
		if tc.Scheme == nil {
			t.Error("expected scheme to be created, got nil")
		}
		if tc.Client == nil {
			t.Error("expected client to be created, got nil")
		}
		if tc.Recorder == nil {
			t.Error("expected recorder to be created, got nil")
		}
	})

	t.Run("with custom scheme uses provided scheme", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		tc := NewTestContext(t, scheme)
		if tc.Scheme != scheme {
			t.Error("expected custom scheme to be used")
		}
	})

	t.Run("with initial objects", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "default",
			},
		}

		tc := NewTestContext(t, nil, cm)

		// Verify object exists
		retrieved := &corev1.ConfigMap{}
		err := tc.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, retrieved)
		if err != nil {
			t.Errorf("expected object to exist, got error: %v", err)
		}
		if retrieved.Name != "test-cm" {
			t.Errorf("expected name 'test-cm', got %q", retrieved.Name)
		}
	})

	t.Run("with multiple initial objects", func(t *testing.T) {
		cm1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"},
		}
		cm2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"},
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "default"},
		}

		tc := NewTestContext(t, nil, cm1, cm2, secret)

		ctx := context.Background()
		retrieved1 := &corev1.ConfigMap{}
		if err := tc.Get(ctx, types.NamespacedName{Name: "cm1", Namespace: "default"}, retrieved1); err != nil {
			t.Errorf("expected cm1 to exist: %v", err)
		}

		retrieved2 := &corev1.ConfigMap{}
		if err := tc.Get(ctx, types.NamespacedName{Name: "cm2", Namespace: "default"}, retrieved2); err != nil {
			t.Errorf("expected cm2 to exist: %v", err)
		}

		retrieved3 := &corev1.Secret{}
		if err := tc.Get(ctx, types.NamespacedName{Name: "secret1", Namespace: "default"}, retrieved3); err != nil {
			t.Errorf("expected secret1 to exist: %v", err)
		}
	})
}

func TestTestContext_Create(t *testing.T) {
	tc := NewTestContext(t, nil)
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	err := tc.Create(ctx, cm)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify object exists
	retrieved := &corev1.ConfigMap{}
	if err := tc.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, retrieved); err != nil {
		t.Errorf("expected object to exist: %v", err)
	}

	// Verify tracked
	created := tc.GetCreatedObjects()
	if len(created) != 1 {
		t.Errorf("expected 1 created object, got %d", len(created))
	}
}

func TestTestContext_Update(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	tc := NewTestContext(t, nil, cm)
	ctx := context.Background()

	// Fetch and update
	retrieved := &corev1.ConfigMap{}
	if err := tc.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, retrieved); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}

	retrieved.Data["key"] = "new-value"
	if err := tc.Update(ctx, retrieved); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify update
	updated := &corev1.ConfigMap{}
	if err := tc.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, updated); err != nil {
		t.Errorf("expected object to exist: %v", err)
	}
	if updated.Data["key"] != "new-value" {
		t.Errorf("expected updated value, got %q", updated.Data["key"])
	}

	// Verify tracked
	updatedObjs := tc.GetUpdatedObjects()
	if len(updatedObjs) != 1 {
		t.Errorf("expected 1 updated object, got %d", len(updatedObjs))
	}
}

func TestTestContext_Delete(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
	}

	tc := NewTestContext(t, nil, cm)
	ctx := context.Background()

	if err := tc.Delete(ctx, cm); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify deleted
	retrieved := &corev1.ConfigMap{}
	err := tc.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, retrieved)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got: %v", err)
	}

	// Verify tracked
	deleted := tc.GetDeletedObjects()
	if len(deleted) != 1 {
		t.Errorf("expected 1 deleted object, got %d", len(deleted))
	}
}

func TestTestContext_Get(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	tc := NewTestContext(t, nil, cm)
	ctx := context.Background()

	t.Run("existing object", func(t *testing.T) {
		retrieved := &corev1.ConfigMap{}
		err := tc.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, retrieved)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if retrieved.Data["key"] != "value" {
			t.Errorf("expected data key 'value', got %q", retrieved.Data["key"])
		}
	})

	t.Run("non-existing object", func(t *testing.T) {
		retrieved := &corev1.ConfigMap{}
		err := tc.Get(ctx, types.NamespacedName{Name: "non-existent", Namespace: "default"}, retrieved)
		if !apierrors.IsNotFound(err) {
			t.Errorf("expected NotFound error, got: %v", err)
		}
	})
}

func TestTestContext_List(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"},
	}
	cm3 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm3", Namespace: "other"},
	}

	tc := NewTestContext(t, nil, cm1, cm2, cm3)
	ctx := context.Background()

	t.Run("list all", func(t *testing.T) {
		list := &corev1.ConfigMapList{}
		if err := tc.List(ctx, list); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(list.Items) != 3 {
			t.Errorf("expected 3 items, got %d", len(list.Items))
		}
	})

	t.Run("list with namespace filter", func(t *testing.T) {
		list := &corev1.ConfigMapList{}
		if err := tc.List(ctx, list, client.InNamespace("default")); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(list.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(list.Items))
		}
	})
}

func TestTestContext_CreateAll(t *testing.T) {
	tc := NewTestContext(t, nil)
	ctx := context.Background()

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"},
	}

	if err := tc.CreateAll(ctx, cm1, cm2); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify both exist
	for _, name := range []string{"cm1", "cm2"} {
		retrieved := &corev1.ConfigMap{}
		if err := tc.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, retrieved); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	// Verify tracked
	created := tc.GetCreatedObjects()
	if len(created) != 2 {
		t.Errorf("expected 2 created objects, got %d", len(created))
	}
}

func TestTestContext_GetCreatedObjects(t *testing.T) {
	tc := NewTestContext(t, nil)
	ctx := context.Background()

	// Initially empty
	if len(tc.GetCreatedObjects()) != 0 {
		t.Error("expected empty created objects initially")
	}

	// Create some objects
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"},
	}
	_ = tc.Create(ctx, cm1)
	_ = tc.Create(ctx, cm2)

	created := tc.GetCreatedObjects()
	if len(created) != 2 {
		t.Errorf("expected 2 created objects, got %d", len(created))
	}
}

func TestTestContext_GetUpdatedObjects(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	tc := NewTestContext(t, nil, cm)
	ctx := context.Background()

	// Initially empty
	if len(tc.GetUpdatedObjects()) != 0 {
		t.Error("expected empty updated objects initially")
	}

	// Update object
	retrieved := &corev1.ConfigMap{}
	_ = tc.Get(ctx, types.NamespacedName{Name: "cm", Namespace: "default"}, retrieved)
	retrieved.Data["key"] = "new-value"
	_ = tc.Update(ctx, retrieved)

	updated := tc.GetUpdatedObjects()
	if len(updated) != 1 {
		t.Errorf("expected 1 updated object, got %d", len(updated))
	}
}

func TestTestContext_GetDeletedObjects(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc := NewTestContext(t, nil, cm)
	ctx := context.Background()

	// Initially empty
	if len(tc.GetDeletedObjects()) != 0 {
		t.Error("expected empty deleted objects initially")
	}

	// Delete object
	_ = tc.Delete(ctx, cm)

	deleted := tc.GetDeletedObjects()
	if len(deleted) != 1 {
		t.Errorf("expected 1 deleted object, got %d", len(deleted))
	}
}

func TestTestContext_GetEvents(t *testing.T) {
	tc := NewTestContext(t, nil)

	// Initially empty
	if len(tc.GetEvents()) != 0 {
		t.Error("expected empty events initially")
	}

	// Record some events
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")
	tc.Recorder.Event(cm, corev1.EventTypeWarning, "Failed", "Failed to do something")

	events := tc.GetEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestTestContext_ClearEvents(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")

	if len(tc.GetEvents()) != 1 {
		t.Fatal("expected 1 event before clear")
	}

	tc.ClearEvents()

	if len(tc.GetEvents()) != 0 {
		t.Error("expected empty events after clear")
	}
}

func TestTestContext_ResetTracker(t *testing.T) {
	tc := NewTestContext(t, nil)
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	_ = tc.Create(ctx, cm)

	if len(tc.GetCreatedObjects()) != 1 {
		t.Fatal("expected 1 created object before reset")
	}

	tc.ResetTracker()

	if len(tc.GetCreatedObjects()) != 0 {
		t.Error("expected empty created objects after reset")
	}
	if len(tc.GetUpdatedObjects()) != 0 {
		t.Error("expected empty updated objects after reset")
	}
	if len(tc.GetDeletedObjects()) != 0 {
		t.Error("expected empty deleted objects after reset")
	}
}

func TestTestContext_NewStreamlineContext(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	sCtx := tc.NewStreamlineContext(cm)
	if sCtx == nil {
		t.Fatal("expected non-nil streamline context")
	}
	if sCtx.Client == nil {
		t.Error("expected client to be set")
	}
	if sCtx.Object == nil {
		t.Error("expected object to be set")
	}
}

// =============================================================================
// FakeRecorder Tests
// =============================================================================

func TestNewFakeRecorder(t *testing.T) {
	recorder := NewFakeRecorder(100)
	if recorder == nil {
		t.Fatal("expected non-nil recorder")
	}
	if recorder.maxCount != 100 {
		t.Errorf("expected maxCount 100, got %d", recorder.maxCount)
	}
	if len(recorder.events) != 0 {
		t.Error("expected empty events initially")
	}
}

func TestFakeRecorder_Event(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	cm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})

	recorder.Event(cm, corev1.EventTypeNormal, "TestReason", "Test message")

	events := recorder.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Type != corev1.EventTypeNormal {
		t.Errorf("expected type %q, got %q", corev1.EventTypeNormal, e.Type)
	}
	if e.Reason != "TestReason" {
		t.Errorf("expected reason 'TestReason', got %q", e.Reason)
	}
	if e.Message != "Test message" {
		t.Errorf("expected message 'Test message', got %q", e.Message)
	}
	if e.ObjectRef.Kind != "ConfigMap" {
		t.Errorf("expected kind 'ConfigMap', got %q", e.ObjectRef.Kind)
	}
	if e.ObjectRef.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", e.ObjectRef.Namespace)
	}
	if e.ObjectRef.Name != "cm" {
		t.Errorf("expected name 'cm', got %q", e.ObjectRef.Name)
	}
}

func TestFakeRecorder_Eventf(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Eventf(cm, corev1.EventTypeWarning, "Scaled", "Scaled from %d to %d replicas", 1, 3)

	events := recorder.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Message != "Scaled from 1 to 3 replicas" {
		t.Errorf("expected formatted message, got %q", events[0].Message)
	}
}

func TestFakeRecorder_AnnotatedEventf(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	annotations := map[string]string{"key": "value"}
	recorder.AnnotatedEventf(cm, annotations, corev1.EventTypeNormal, "Annotated", "Message with %s", "args")

	events := recorder.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Message != "Message with args" {
		t.Errorf("expected formatted message, got %q", events[0].Message)
	}
}

func TestFakeRecorder_GetEvents(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Event(cm, corev1.EventTypeNormal, "Event1", "Message1")
	recorder.Event(cm, corev1.EventTypeWarning, "Event2", "Message2")

	events := recorder.GetEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Verify it returns a copy
	events[0].Reason = "Modified"
	originalEvents := recorder.GetEvents()
	if originalEvents[0].Reason == "Modified" {
		t.Error("expected GetEvents to return a copy, not the original slice")
	}
}

func TestFakeRecorder_Clear(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Event(cm, corev1.EventTypeNormal, "Event1", "Message1")
	recorder.Event(cm, corev1.EventTypeWarning, "Event2", "Message2")

	if len(recorder.GetEvents()) != 2 {
		t.Fatal("expected 2 events before clear")
	}

	recorder.Clear()

	if len(recorder.GetEvents()) != 0 {
		t.Error("expected 0 events after clear")
	}
}

func TestFakeRecorder_HasEvent(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")
	recorder.Event(cm, corev1.EventTypeWarning, "Failed", "Failed to create")

	tests := []struct {
		name      string
		eventType string
		reason    string
		expected  bool
	}{
		{"existing normal event", corev1.EventTypeNormal, "Created", true},
		{"existing warning event", corev1.EventTypeWarning, "Failed", true},
		{"wrong type", corev1.EventTypeWarning, "Created", false},
		{"wrong reason", corev1.EventTypeNormal, "Updated", false},
		{"non-existing event", corev1.EventTypeNormal, "NonExistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.HasEvent(tt.eventType, tt.reason)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFakeRecorder_HasEventWithMessage(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource successfully")

	tests := []struct {
		name      string
		eventType string
		reason    string
		message   string
		expected  bool
	}{
		{"exact match", corev1.EventTypeNormal, "Created", "Created resource successfully", true},
		{"wrong message", corev1.EventTypeNormal, "Created", "Wrong message", false},
		{"wrong type", corev1.EventTypeWarning, "Created", "Created resource successfully", false},
		{"wrong reason", corev1.EventTypeNormal, "Updated", "Created resource successfully", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.HasEventWithMessage(tt.eventType, tt.reason, tt.message)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFakeRecorder_CountEvents(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	recorder.Event(cm, corev1.EventTypeNormal, "Synced", "Message 1")
	recorder.Event(cm, corev1.EventTypeNormal, "Synced", "Message 2")
	recorder.Event(cm, corev1.EventTypeNormal, "Synced", "Message 3")
	recorder.Event(cm, corev1.EventTypeWarning, "Failed", "Error message")

	tests := []struct {
		name      string
		eventType string
		reason    string
		expected  int
	}{
		{"count multiple synced", corev1.EventTypeNormal, "Synced", 3},
		{"count single failed", corev1.EventTypeWarning, "Failed", 1},
		{"count non-existing", corev1.EventTypeNormal, "NonExistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := recorder.CountEvents(tt.eventType, tt.reason)
			if count != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, count)
			}
		})
	}
}

func TestFakeRecorder_MaxEventsLimit(t *testing.T) {
	maxEvents := 3
	recorder := NewFakeRecorder(maxEvents)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	// Add more events than the limit
	for i := 0; i < maxEvents+5; i++ {
		recorder.Event(cm, corev1.EventTypeNormal, "Event", fmt.Sprintf("Message %d", i))
	}

	events := recorder.GetEvents()
	if len(events) != maxEvents {
		t.Errorf("expected %d events (max limit), got %d", maxEvents, len(events))
	}

	// Verify the first events were kept
	if events[0].Message != "Message 0" {
		t.Errorf("expected first message to be 'Message 0', got %q", events[0].Message)
	}
}

// =============================================================================
// Assertions Tests
// =============================================================================

func TestNewAssertions(t *testing.T) {
	tc := NewTestContext(t, nil)
	assertions := NewAssertions(t, tc)

	if assertions == nil {
		t.Fatal("expected non-nil assertions")
	}
	if assertions.t != t {
		t.Error("expected t to be set")
	}
	if assertions.tc != tc {
		t.Error("expected tc to be set")
	}
}

func TestAssertions_ObjectExists(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "default"},
	}
	tc := NewTestContext(t, nil, cm)

	t.Run("existing object passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ObjectExists(context.Background(), types.NamespacedName{Name: "existing", Namespace: "default"}, &corev1.ConfigMap{})
		if mockT.Failed() {
			t.Error("expected test to pass for existing object")
		}
	})

	t.Run("non-existing object fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ObjectExists(context.Background(), types.NamespacedName{Name: "non-existing", Namespace: "default"}, &corev1.ConfigMap{})
		if !mockT.Failed() {
			t.Error("expected test to fail for non-existing object")
		}
	})
}

func TestAssertions_ObjectNotExists(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "default"},
	}
	tc := NewTestContext(t, nil, cm)

	t.Run("non-existing object passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ObjectNotExists(context.Background(), types.NamespacedName{Name: "non-existing", Namespace: "default"}, &corev1.ConfigMap{})
		if mockT.Failed() {
			t.Error("expected test to pass for non-existing object")
		}
	})

	t.Run("existing object fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ObjectNotExists(context.Background(), types.NamespacedName{Name: "existing", Namespace: "default"}, &corev1.ConfigMap{})
		if !mockT.Failed() {
			t.Error("expected test to fail for existing object")
		}
	})
}

func TestAssertions_ConditionTrue(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("true condition passes", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionTrue(obj, "Ready")
		if mockT.Failed() {
			t.Error("expected test to pass for true condition")
		}
	})

	t.Run("false condition fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionTrue(obj, "Ready")
		if !mockT.Failed() {
			t.Error("expected test to fail for false condition")
		}
	})

	t.Run("missing condition fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionTrue(obj, "Ready")
		if !mockT.Failed() {
			t.Error("expected test to fail for missing condition")
		}
	})
}

func TestAssertions_ConditionFalse(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("false condition passes", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionFalse(obj, "Ready")
		if mockT.Failed() {
			t.Error("expected test to pass for false condition")
		}
	})

	t.Run("true condition fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionFalse(obj, "Ready")
		if !mockT.Failed() {
			t.Error("expected test to fail for true condition")
		}
	})
}

func TestAssertions_ConditionUnknown(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("unknown condition passes", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionUnknown},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionUnknown(obj, "Ready")
		if mockT.Failed() {
			t.Error("expected test to pass for unknown condition")
		}
	})

	t.Run("true condition fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionUnknown(obj, "Ready")
		if !mockT.Failed() {
			t.Error("expected test to fail for true condition")
		}
	})
}

func TestAssertions_ConditionHasReason(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("matching reason passes", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionHasReason(obj, "Ready", "AllGood")
		if mockT.Failed() {
			t.Error("expected test to pass for matching reason")
		}
	})

	t.Run("non-matching reason fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "DifferentReason"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionHasReason(obj, "Ready", "AllGood")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-matching reason")
		}
	})

	t.Run("missing condition fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionHasReason(obj, "Ready", "AllGood")
		if !mockT.Failed() {
			t.Error("expected test to fail for missing condition")
		}
	})
}

func TestAssertions_ConditionHasMessage(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("matching message passes", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Message: "Resource is ready"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionHasMessage(obj, "Ready", "Resource is ready")
		if mockT.Failed() {
			t.Error("expected test to pass for matching message")
		}
	})

	t.Run("non-matching message fails", func(t *testing.T) {
		obj := &testObjectWithConditions{
			ConfigMap: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Message: "Different message"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ConditionHasMessage(obj, "Ready", "Resource is ready")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-matching message")
		}
	})
}

func TestAssertions_EventRecorded(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")

	t.Run("existing event passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.EventRecorded(corev1.EventTypeNormal, "Created")
		if mockT.Failed() {
			t.Error("expected test to pass for existing event")
		}
	})

	t.Run("non-existing event fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.EventRecorded(corev1.EventTypeNormal, "NonExistent")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-existing event")
		}
	})
}

func TestAssertions_EventNotRecorded(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")

	t.Run("non-existing event passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.EventNotRecorded(corev1.EventTypeNormal, "NonExistent")
		if mockT.Failed() {
			t.Error("expected test to pass for non-existing event")
		}
	})

	t.Run("existing event fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.EventNotRecorded(corev1.EventTypeNormal, "Created")
		if !mockT.Failed() {
			t.Error("expected test to fail for existing event")
		}
	})
}

func TestAssertions_NormalEventRecorded(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeNormal, "Created", "Created resource")

	t.Run("existing normal event passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.NormalEventRecorded("Created")
		if mockT.Failed() {
			t.Error("expected test to pass for existing normal event")
		}
	})

	t.Run("non-existing normal event fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.NormalEventRecorded("NonExistent")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-existing normal event")
		}
	})
}

func TestAssertions_WarningEventRecorded(t *testing.T) {
	tc := NewTestContext(t, nil)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}
	tc.Recorder.Event(cm, corev1.EventTypeWarning, "Failed", "Failed to create")

	t.Run("existing warning event passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.WarningEventRecorded("Failed")
		if mockT.Failed() {
			t.Error("expected test to pass for existing warning event")
		}
	})

	t.Run("non-existing warning event fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.WarningEventRecorded("NonExistent")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-existing warning event")
		}
	})
}

func TestAssertions_ResultIsStop(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("stop result passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsStop(streamline.Stop())
		if mockT.Failed() {
			t.Error("expected test to pass for stop result")
		}
	})

	t.Run("requeue result fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsStop(streamline.Requeue())
		if !mockT.Failed() {
			t.Error("expected test to fail for requeue result")
		}
	})

	t.Run("requeue after result fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsStop(streamline.RequeueAfter(5 * time.Second))
		if !mockT.Failed() {
			t.Error("expected test to fail for requeue after result")
		}
	})
}

func TestAssertions_ResultIsRequeue(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("requeue result passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeue(streamline.Requeue())
		if mockT.Failed() {
			t.Error("expected test to pass for requeue result")
		}
	})

	t.Run("stop result fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeue(streamline.Stop())
		if !mockT.Failed() {
			t.Error("expected test to fail for stop result")
		}
	})

	t.Run("requeue after result fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeue(streamline.RequeueAfter(5 * time.Second))
		if !mockT.Failed() {
			t.Error("expected test to fail for requeue after result")
		}
	})
}

func TestAssertions_ResultIsRequeueAfter(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("correct duration passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeueAfter(streamline.RequeueAfter(5*time.Second), 5*time.Second)
		if mockT.Failed() {
			t.Error("expected test to pass for correct duration")
		}
	})

	t.Run("incorrect duration fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeueAfter(streamline.RequeueAfter(5*time.Second), 10*time.Second)
		if !mockT.Failed() {
			t.Error("expected test to fail for incorrect duration")
		}
	})

	t.Run("stop result fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ResultIsRequeueAfter(streamline.Stop(), 5*time.Second)
		if !mockT.Failed() {
			t.Error("expected test to fail for stop result")
		}
	})
}

func TestAssertions_NoError(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("nil error passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.NoError(nil)
		if mockT.Failed() {
			t.Error("expected test to pass for nil error")
		}
	})

	t.Run("non-nil error fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.NoError(errors.New("some error"))
		if !mockT.Failed() {
			t.Error("expected test to fail for non-nil error")
		}
	})
}

func TestAssertions_Error(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("non-nil error passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.Error(errors.New("some error"))
		if mockT.Failed() {
			t.Error("expected test to pass for non-nil error")
		}
	})

	t.Run("nil error fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.Error(nil)
		if !mockT.Failed() {
			t.Error("expected test to fail for nil error")
		}
	})
}

func TestAssertions_ErrorContains(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("matching substring passes", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ErrorContains(errors.New("connection refused: host unreachable"), "connection refused")
		if mockT.Failed() {
			t.Error("expected test to pass for matching substring")
		}
	})

	t.Run("non-matching substring fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ErrorContains(errors.New("connection refused"), "timeout")
		if !mockT.Failed() {
			t.Error("expected test to fail for non-matching substring")
		}
	})

	t.Run("nil error fails", func(t *testing.T) {
		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.ErrorContains(nil, "error")
		if !mockT.Failed() {
			t.Error("expected test to fail for nil error")
		}
	})
}

func TestAssertions_OwnerReferenceSet(t *testing.T) {
	tc := NewTestContext(t, nil)

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid-123",
		},
	}

	t.Run("owner reference present passes", func(t *testing.T) {
		owned := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owned",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{UID: "owner-uid-123"},
				},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.OwnerReferenceSet(owned, owner)
		if mockT.Failed() {
			t.Error("expected test to pass when owner reference is present")
		}
	})

	t.Run("owner reference missing fails", func(t *testing.T) {
		owned := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "owned",
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.OwnerReferenceSet(owned, owner)
		if !mockT.Failed() {
			t.Error("expected test to fail when owner reference is missing")
		}
	})

	t.Run("wrong owner reference fails", func(t *testing.T) {
		owned := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owned",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{UID: "different-uid"},
				},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.OwnerReferenceSet(owned, owner)
		if !mockT.Failed() {
			t.Error("expected test to fail when owner reference is wrong")
		}
	})
}

func TestAssertions_FinalizerPresent(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("finalizer present passes", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{"my.finalizer/cleanup", "other.finalizer"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerPresent(obj, "my.finalizer/cleanup")
		if mockT.Failed() {
			t.Error("expected test to pass when finalizer is present")
		}
	})

	t.Run("finalizer absent fails", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{"other.finalizer"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerPresent(obj, "my.finalizer/cleanup")
		if !mockT.Failed() {
			t.Error("expected test to fail when finalizer is absent")
		}
	})

	t.Run("empty finalizers fails", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerPresent(obj, "my.finalizer/cleanup")
		if !mockT.Failed() {
			t.Error("expected test to fail when no finalizers exist")
		}
	})
}

func TestAssertions_FinalizerAbsent(t *testing.T) {
	tc := NewTestContext(t, nil)

	t.Run("finalizer absent passes", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{"other.finalizer"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerAbsent(obj, "my.finalizer/cleanup")
		if mockT.Failed() {
			t.Error("expected test to pass when finalizer is absent")
		}
	})

	t.Run("empty finalizers passes", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerAbsent(obj, "my.finalizer/cleanup")
		if mockT.Failed() {
			t.Error("expected test to pass when no finalizers exist")
		}
	})

	t.Run("finalizer present fails", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Finalizers: []string{"my.finalizer/cleanup", "other.finalizer"},
			},
		}

		mockT := &testing.T{}
		assertions := NewAssertions(mockT, tc)
		assertions.FinalizerAbsent(obj, "my.finalizer/cleanup")
		if !mockT.Failed() {
			t.Error("expected test to fail when finalizer is present")
		}
	})
}

// =============================================================================
// FaultyClient Tests
// =============================================================================

func TestNewFaultyClient(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fc := NewFaultyClient(baseClient)
	if fc == nil {
		t.Fatal("expected non-nil faulty client")
	}
	if fc.Client != baseClient {
		t.Error("expected base client to be wrapped")
	}
	if len(fc.faults) != 0 {
		t.Error("expected empty faults initially")
	}
}

func TestFaultyClient_InjectFault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fc := NewFaultyClient(baseClient)

	fault := Fault{
		Operation:   OperationGet,
		Error:       errors.New("test error"),
		Probability: 1.0,
		Count:       -1,
	}

	result := fc.InjectFault(fault)
	if result != fc {
		t.Error("expected InjectFault to return the client for chaining")
	}
	if len(fc.faults) != 1 {
		t.Errorf("expected 1 fault, got %d", len(fc.faults))
	}
}

func TestFaultyClient_FailGet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected get error")

	fc := NewFaultyClient(baseClient).FailGet(gvk, testErr)

	result := &corev1.ConfigMap{}
	result.SetGroupVersionKind(gvk)

	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err == nil {
		t.Error("expected error from FailGet")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_FailGetOnce(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected get error once")

	fc := NewFaultyClient(baseClient).FailGetOnce(gvk, testErr)

	result := &corev1.ConfigMap{}
	result.SetGroupVersionKind(gvk)

	// First call should fail
	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err == nil {
		t.Error("expected error from first FailGetOnce call")
	}

	// Second call should succeed
	result2 := &corev1.ConfigMap{}
	result2.SetGroupVersionKind(gvk)
	err = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result2)
	if err != nil {
		t.Errorf("expected no error from second call, got: %v", err)
	}
}

func TestFaultyClient_FailCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected create error")

	fc := NewFaultyClient(baseClient).FailCreate(gvk, testErr)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	cm.SetGroupVersionKind(gvk)

	err := fc.Create(context.Background(), cm)
	if err == nil {
		t.Error("expected error from FailCreate")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_FailUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected update error")

	fc := NewFaultyClient(baseClient).FailUpdate(gvk, testErr)

	updateCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	updateCm.SetGroupVersionKind(gvk)

	err := fc.Update(context.Background(), updateCm)
	if err == nil {
		t.Error("expected error from FailUpdate")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_FailUpdateWithConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	fc := NewFaultyClient(baseClient).FailUpdateWithConflict(gvk)

	updateCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	updateCm.SetGroupVersionKind(gvk)

	err := fc.Update(context.Background(), updateCm)
	if err == nil {
		t.Error("expected conflict error from FailUpdateWithConflict")
	}
	if !apierrors.IsConflict(err) {
		t.Errorf("expected IsConflict error, got: %v", err)
	}
}

func TestFaultyClient_ClearFaults(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected error")

	fc := NewFaultyClient(baseClient).FailGet(gvk, testErr)

	// Verify fault is active
	result := &corev1.ConfigMap{}
	result.SetGroupVersionKind(gvk)
	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err == nil {
		t.Error("expected error before ClearFaults")
	}

	// Clear faults
	fc.ClearFaults()

	if len(fc.faults) != 0 {
		t.Errorf("expected 0 faults after clear, got %d", len(fc.faults))
	}

	// Verify fault is cleared
	result2 := &corev1.ConfigMap{}
	result2.SetGroupVersionKind(gvk)
	err = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result2)
	if err != nil {
		t.Errorf("expected no error after ClearFaults, got: %v", err)
	}
}

func TestFaultyClient_Get(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	fc := NewFaultyClient(baseClient)

	result := &corev1.ConfigMap{}
	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if result.Data["key"] != "value" {
		t.Errorf("expected data key 'value', got %q", result.Data["key"])
	}
}

func TestFaultyClient_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fc := NewFaultyClient(baseClient)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	err := fc.Create(context.Background(), cm)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify it was created
	result := &corev1.ConfigMap{}
	err = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err != nil {
		t.Errorf("expected object to exist: %v", err)
	}
}

func TestFaultyClient_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "old"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	fc := NewFaultyClient(baseClient)

	// Fetch and update
	result := &corev1.ConfigMap{}
	_ = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	result.Data["key"] = "new"

	err := fc.Update(context.Background(), result)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify update
	result2 := &corev1.ConfigMap{}
	_ = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result2)
	if result2.Data["key"] != "new" {
		t.Errorf("expected updated value 'new', got %q", result2.Data["key"])
	}
}

func TestFaultyClient_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	fc := NewFaultyClient(baseClient)

	err := fc.Delete(context.Background(), cm)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify deletion
	result := &corev1.ConfigMap{}
	err = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got: %v", err)
	}
}

func TestFaultyClient_Patch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "old"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	fc := NewFaultyClient(baseClient)

	// Fetch the object first
	existing := &corev1.ConfigMap{}
	_ = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, existing)

	// Create updated version
	updated := existing.DeepCopy()
	updated.Data["key"] = "patched"

	err := fc.Patch(context.Background(), updated, client.MergeFrom(existing))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify patch
	result := &corev1.ConfigMap{}
	_ = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if result.Data["key"] != "patched" {
		t.Errorf("expected patched value, got %q", result.Data["key"])
	}
}

func TestFaultyClient_StatusWriterUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected status update error")

	fc := NewFaultyClient(baseClient).FailUpdate(gvk, testErr)

	// Get the status writer
	statusWriter := fc.Status()
	if statusWriter == nil {
		t.Fatal("expected non-nil status writer")
	}

	// Try to update status
	updateCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	updateCm.SetGroupVersionKind(gvk)

	err := statusWriter.Update(context.Background(), updateCm)
	if err == nil {
		t.Error("expected error from status update")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_StatusWriterPatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	testErr := errors.New("injected status patch error")

	// Use a fault without GVK matching (empty GVK matches all operations)
	fc := NewFaultyClient(baseClient).InjectFault(Fault{
		Operation:   OperationPatch,
		Error:       testErr,
		Probability: 1.0,
		Count:       -1,
	})

	// Get the status writer
	statusWriter := fc.Status()

	// Fetch the object
	existing := &corev1.ConfigMap{}
	_ = fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, existing)

	// Try to patch status
	updated := existing.DeepCopy()

	err := statusWriter.Patch(context.Background(), updated, client.MergeFrom(existing))
	if err == nil {
		t.Error("expected error from status patch")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_DeleteFault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testErr := errors.New("injected delete error")

	fc := NewFaultyClient(baseClient).InjectFault(Fault{
		Operation:   OperationDelete,
		GVK:         gvk,
		Error:       testErr,
		Probability: 1.0,
		Count:       -1,
	})

	deleteCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	deleteCm.SetGroupVersionKind(gvk)

	err := fc.Delete(context.Background(), deleteCm)
	if err == nil {
		t.Error("expected error from delete fault")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %q, got %q", testErr, err)
	}
}

func TestFaultyClient_FaultWithoutGVK(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	testErr := errors.New("injected error for all types")

	// Fault without GVK should match all types
	fc := NewFaultyClient(baseClient).InjectFault(Fault{
		Operation:   OperationGet,
		Error:       testErr,
		Probability: 1.0,
		Count:       -1,
	})

	result := &corev1.ConfigMap{}
	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, result)
	if err == nil {
		t.Error("expected error from fault without GVK")
	}
}

// =============================================================================
// HandlerTestSuite Tests
// =============================================================================

func TestNewHandlerTestSuite(t *testing.T) {
	handler := &mockHandler{}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ts := NewHandlerTestSuite(t, scheme, handler)
	if ts == nil {
		t.Fatal("expected non-nil test suite")
	}
	if ts.Handler != handler {
		t.Error("expected handler to be set")
	}
	if ts.TC == nil {
		t.Error("expected test context to be set")
	}
	if ts.Assert == nil {
		t.Error("expected assertions to be set")
	}
}

func TestHandlerTestSuite_RunSync(t *testing.T) {
	t.Run("handler returns Stop", func(t *testing.T) {
		handler := &mockHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
				return streamline.Stop(), nil
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		ts := NewHandlerTestSuite(t, scheme, handler)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		result, err := ts.RunSync(context.Background(), cm)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result.Requeue {
			t.Error("expected no requeue")
		}
	})

	t.Run("handler returns Requeue", func(t *testing.T) {
		handler := &mockHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
				return streamline.Requeue(), nil
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		ts := NewHandlerTestSuite(t, scheme, handler)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		result, err := ts.RunSync(context.Background(), cm)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if !result.Requeue {
			t.Error("expected requeue")
		}
	})

	t.Run("handler returns RequeueAfter", func(t *testing.T) {
		handler := &mockHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
				return streamline.RequeueAfter(5 * time.Minute), nil
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		ts := NewHandlerTestSuite(t, scheme, handler)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		result, err := ts.RunSync(context.Background(), cm)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result.RequeueAfter != 5*time.Minute {
			t.Errorf("expected RequeueAfter 5m, got %v", result.RequeueAfter)
		}
	})

	t.Run("handler returns error", func(t *testing.T) {
		testErr := errors.New("sync error")
		handler := &mockHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
				return streamline.Stop(), testErr
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		ts := NewHandlerTestSuite(t, scheme, handler)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		_, err := ts.RunSync(context.Background(), cm)
		if err == nil {
			t.Error("expected error from handler")
		}
		if err.Error() != testErr.Error() {
			t.Errorf("expected error %q, got %q", testErr, err)
		}
	})

	t.Run("handler can use streamline context", func(t *testing.T) {
		var receivedCtx *streamline.Context
		handler := &mockHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
				receivedCtx = sCtx
				sCtx.Event.Normal("TestEvent", "Test message")
				return streamline.Stop(), nil
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		ts := NewHandlerTestSuite(t, scheme, handler)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		_, err := ts.RunSync(context.Background(), cm)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}

		if receivedCtx == nil {
			t.Fatal("expected streamline context to be passed to handler")
		}

		// Verify event was recorded
		if !ts.TC.Recorder.HasEvent(corev1.EventTypeNormal, "TestEvent") {
			t.Error("expected TestEvent to be recorded")
		}
	})
}

func TestHandlerTestSuite_WithObject(t *testing.T) {
	handler := &mockHandler{}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ts := NewHandlerTestSuite(t, scheme, handler)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	result := ts.WithObject(context.Background(), cm)
	if result != cm {
		t.Error("expected WithObject to return the same object")
	}

	// Verify object exists in client
	retrieved := &corev1.ConfigMap{}
	err := ts.TC.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, retrieved)
	if err != nil {
		t.Errorf("expected object to exist: %v", err)
	}
	if retrieved.Data["key"] != "value" {
		t.Errorf("expected data key 'value', got %q", retrieved.Data["key"])
	}

	// Verify tracked
	created := ts.TC.GetCreatedObjects()
	if len(created) != 1 {
		t.Errorf("expected 1 created object, got %d", len(created))
	}
}

func TestHandlerTestSuite_IntegrationTest(t *testing.T) {
	// This test demonstrates a complete handler test workflow
	handler := &mockHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *streamline.Context) (streamline.Result, error) {
			// Simulate a handler that creates a child ConfigMap
			child := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.Name + "-child",
					Namespace: obj.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       obj.Name,
							UID:        obj.UID,
						},
					},
				},
				Data: map[string]string{"parent": obj.Name},
			}

			if err := sCtx.Client.Create(ctx, child); err != nil {
				if !apierrors.IsAlreadyExists(err) {
					sCtx.Event.Warning("CreateFailed", "Failed to create child")
					return streamline.Requeue(), err
				}
			}

			sCtx.Event.Normal("Synced", "Successfully synced")
			return streamline.Stop(), nil
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ts := NewHandlerTestSuite(t, scheme, handler)
	ctx := context.Background()

	// Create the parent object
	parent := ts.WithObject(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: "default",
			UID:       "parent-uid",
		},
	})

	// Run sync
	result, err := ts.RunSync(ctx, parent)
	ts.Assert.NoError(err)
	ts.Assert.ResultIsStop(result)

	// Verify child was created
	ts.Assert.ObjectExists(ctx, types.NamespacedName{Name: "parent-child", Namespace: "default"}, &corev1.ConfigMap{})

	// Verify event was recorded
	ts.Assert.NormalEventRecorded("Synced")
}

// =============================================================================
// Edge Cases and Concurrency Tests
// =============================================================================

func TestTestContext_ConcurrentOperations(t *testing.T) {
	tc := NewTestContext(t, nil)
	ctx := context.Background()

	// Run concurrent creates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("cm-%d", idx),
					Namespace: "default",
				},
			}
			_ = tc.Create(ctx, cm)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	created := tc.GetCreatedObjects()
	if len(created) != 10 {
		t.Errorf("expected 10 created objects, got %d", len(created))
	}
}

func TestFakeRecorder_ConcurrentEvents(t *testing.T) {
	recorder := NewFakeRecorder(100)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
	}

	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			recorder.Event(cm, corev1.EventTypeNormal, "Concurrent", fmt.Sprintf("Message %d", idx))
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	events := recorder.GetEvents()
	if len(events) != 20 {
		t.Errorf("expected 20 events, got %d", len(events))
	}
}

func TestFaultyClient_MultipleFaults(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	gvkConfigMap := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	gvkSecret := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}

	fc := NewFaultyClient(baseClient).
		FailGet(gvkConfigMap, errors.New("configmap get error")).
		FailCreate(gvkSecret, errors.New("secret create error"))

	// ConfigMap Get should fail
	cm := &corev1.ConfigMap{}
	cm.SetGroupVersionKind(gvkConfigMap)
	err := fc.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, cm)
	if err == nil || err.Error() != "configmap get error" {
		t.Errorf("expected 'configmap get error', got: %v", err)
	}

	// Secret Create should fail
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	secret.SetGroupVersionKind(gvkSecret)
	err = fc.Create(context.Background(), secret)
	if err == nil || err.Error() != "secret create error" {
		t.Errorf("expected 'secret create error', got: %v", err)
	}

	// ConfigMap Create should succeed (no fault for this)
	cmCreate := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "default"},
	}
	err = fc.Create(context.Background(), cmCreate)
	if err != nil {
		t.Errorf("expected ConfigMap Create to succeed, got: %v", err)
	}
}
