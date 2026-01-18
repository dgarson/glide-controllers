package streamline

import (
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// mockEventRecorder captures events for testing.
type mockEventRecorder struct {
	events []mockEvent
}

type mockEvent struct {
	object    runtime.Object
	eventType string
	reason    string
	message   string
}

func (m *mockEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	m.events = append(m.events, mockEvent{
		object:    object,
		eventType: eventType,
		reason:    reason,
		message:   message,
	})
}

func (m *mockEventRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, mockEvent{
		object:    object,
		eventType: eventType,
		reason:    reason,
		message:   messageFmt, // Simplified for testing
	})
}

func (m *mockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventType, reason, messageFmt string, args ...interface{}) {
	m.Eventf(object, eventType, reason, messageFmt, args...)
}

var _ record.EventRecorder = &mockEventRecorder{}

func TestNewContext(t *testing.T) {
	recorder := &mockEventRecorder{}
	log := logr.Discard()
	scheme := runtime.NewScheme()
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	ctx := NewContext(nil, scheme, log, recorder, obj)

	if ctx == nil {
		t.Fatal("NewContext returned nil")
	}
	if ctx.Event == nil {
		t.Error("NewContext should set Event")
	}
	if ctx.Owned == nil {
		t.Error("NewContext should set Owned")
	}
}

func TestEventHelper_Normal(t *testing.T) {
	recorder := &mockEventRecorder{}
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	helper := &eventHelper{
		recorder: recorder,
		object:   obj,
	}

	helper.Normal("TestReason", "Test message")

	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recorder.events))
	}

	event := recorder.events[0]
	if event.eventType != corev1.EventTypeNormal {
		t.Errorf("Expected event type %s, got %s", corev1.EventTypeNormal, event.eventType)
	}
	if event.reason != "TestReason" {
		t.Errorf("Expected reason 'TestReason', got %s", event.reason)
	}
	if event.message != "Test message" {
		t.Errorf("Expected message 'Test message', got %s", event.message)
	}
}

func TestEventHelper_Normalf(t *testing.T) {
	recorder := &mockEventRecorder{}
	obj := &corev1.Pod{}

	helper := &eventHelper{
		recorder: recorder,
		object:   obj,
	}

	helper.Normalf("TestReason", "Test %s %d", "message", 42)

	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recorder.events))
	}

	event := recorder.events[0]
	if event.eventType != corev1.EventTypeNormal {
		t.Errorf("Expected event type %s, got %s", corev1.EventTypeNormal, event.eventType)
	}
}

func TestEventHelper_Warning(t *testing.T) {
	recorder := &mockEventRecorder{}
	obj := &corev1.Pod{}

	helper := &eventHelper{
		recorder: recorder,
		object:   obj,
	}

	helper.Warning("WarningReason", "Warning message")

	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recorder.events))
	}

	event := recorder.events[0]
	if event.eventType != corev1.EventTypeWarning {
		t.Errorf("Expected event type %s, got %s", corev1.EventTypeWarning, event.eventType)
	}
	if event.reason != "WarningReason" {
		t.Errorf("Expected reason 'WarningReason', got %s", event.reason)
	}
}

func TestEventHelper_Warningf(t *testing.T) {
	recorder := &mockEventRecorder{}
	obj := &corev1.Pod{}

	helper := &eventHelper{
		recorder: recorder,
		object:   obj,
	}

	helper.Warningf("WarningReason", "Warning %s", "formatted")

	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recorder.events))
	}

	event := recorder.events[0]
	if event.eventType != corev1.EventTypeWarning {
		t.Errorf("Expected event type %s, got %s", corev1.EventTypeWarning, event.eventType)
	}
}

func TestEventHelper_MultipleEvents(t *testing.T) {
	recorder := &mockEventRecorder{}
	obj := &corev1.Pod{}

	helper := &eventHelper{
		recorder: recorder,
		object:   obj,
	}

	helper.Normal("First", "First event")
	helper.Warning("Second", "Second event")
	helper.Normal("Third", "Third event")

	if len(recorder.events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(recorder.events))
	}

	// Verify order
	if recorder.events[0].reason != "First" {
		t.Error("First event should have reason 'First'")
	}
	if recorder.events[1].reason != "Second" {
		t.Error("Second event should have reason 'Second'")
	}
	if recorder.events[2].reason != "Third" {
		t.Error("Third event should have reason 'Third'")
	}
}
