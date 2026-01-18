package streamline

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// testObject is a minimal test object that implements various status interfaces.
type testObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            testObjectStatus `json:"status,omitempty"`
}

type testObjectStatus struct {
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	Phase              string      `json:"phase,omitempty"`
	Message            string      `json:"message,omitempty"`
	LastUpdated        metav1.Time `json:"lastUpdated,omitempty"`
}

// Implement client.Object
func (t *testObject) DeepCopyObject() runtime.Object {
	return &testObject{
		TypeMeta:   t.TypeMeta,
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
		Status:     t.Status,
	}
}

// Implement ObjectWithObservedGeneration
func (t *testObject) GetObservedGeneration() int64 {
	return t.Status.ObservedGeneration
}

func (t *testObject) SetObservedGeneration(gen int64) {
	t.Status.ObservedGeneration = gen
}

// Implement ObjectWithPhase
func (t *testObject) GetPhase() string {
	return t.Status.Phase
}

func (t *testObject) SetPhase(phase string) {
	t.Status.Phase = phase
}

// Implement ObjectWithMessage
func (t *testObject) GetMessage() string {
	return t.Status.Message
}

func (t *testObject) SetMessage(msg string) {
	t.Status.Message = msg
}

// Implement ObjectWithLastUpdated
func (t *testObject) GetLastUpdated() metav1.Time {
	return t.Status.LastUpdated
}

func (t *testObject) SetLastUpdated(t2 metav1.Time) {
	t.Status.LastUpdated = t2
}

// Verify interfaces
var _ client.Object = &testObject{}
var _ ObjectWithObservedGeneration = &testObject{}
var _ ObjectWithPhase = &testObject{}
var _ ObjectWithMessage = &testObject{}
var _ ObjectWithLastUpdated = &testObject{}

// simpleObject only implements client.Object (no status interfaces)
type simpleObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func (s *simpleObject) DeepCopyObject() runtime.Object {
	return &simpleObject{
		TypeMeta:   s.TypeMeta,
		ObjectMeta: *s.ObjectMeta.DeepCopy(),
	}
}

var _ client.Object = &simpleObject{}

func TestStatusHelper_Generation(t *testing.T) {
	obj := &testObject{}
	obj.Generation = 5

	helper := newStatusHelper(obj)

	if got := helper.Generation(); got != 5 {
		t.Errorf("Generation() = %d, want 5", got)
	}
}

func TestStatusHelper_ObservedGeneration(t *testing.T) {
	obj := &testObject{}
	obj.Status.ObservedGeneration = 3

	helper := newStatusHelper(obj)

	if got := helper.ObservedGeneration(); got != 3 {
		t.Errorf("ObservedGeneration() = %d, want 3", got)
	}
}

func TestStatusHelper_ObservedGeneration_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	if got := helper.ObservedGeneration(); got != 0 {
		t.Errorf("ObservedGeneration() = %d, want 0 for unsupported object", got)
	}
}

func TestStatusHelper_SetObservedGeneration(t *testing.T) {
	obj := &testObject{}
	helper := newStatusHelper(obj)

	result := helper.SetObservedGeneration(10)

	if !result {
		t.Error("SetObservedGeneration should return true")
	}
	if obj.Status.ObservedGeneration != 10 {
		t.Errorf("ObservedGeneration = %d, want 10", obj.Status.ObservedGeneration)
	}
}

func TestStatusHelper_SetObservedGeneration_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	result := helper.SetObservedGeneration(10)

	if result {
		t.Error("SetObservedGeneration should return false for unsupported object")
	}
}

func TestStatusHelper_MarkObserved(t *testing.T) {
	obj := &testObject{}
	obj.Generation = 7
	obj.Status.ObservedGeneration = 5

	helper := newStatusHelper(obj)

	result := helper.MarkObserved()

	if !result {
		t.Error("MarkObserved should return true")
	}
	if obj.Status.ObservedGeneration != 7 {
		t.Errorf("ObservedGeneration = %d, want 7 (matching generation)", obj.Status.ObservedGeneration)
	}
}

func TestStatusHelper_IsUpToDate(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		observed   int64
		want       bool
	}{
		{"equal", 5, 5, true},
		{"behind", 5, 3, false},
		{"ahead", 3, 5, false}, // Shouldn't happen but test anyway
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &testObject{}
			obj.Generation = tt.generation
			obj.Status.ObservedGeneration = tt.observed

			helper := newStatusHelper(obj)
			if got := helper.IsUpToDate(); got != tt.want {
				t.Errorf("IsUpToDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusHelper_IsUpToDate_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	if got := helper.IsUpToDate(); got != false {
		t.Error("IsUpToDate should return false for unsupported object")
	}
}

func TestStatusHelper_NeedsReconciliation(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		observed   int64
		want       bool
	}{
		{"equal", 5, 5, false},
		{"behind", 5, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &testObject{}
			obj.Generation = tt.generation
			obj.Status.ObservedGeneration = tt.observed

			helper := newStatusHelper(obj)
			if got := helper.NeedsReconciliation(); got != tt.want {
				t.Errorf("NeedsReconciliation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusHelper_NeedsReconciliation_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	// If we can't track, assume reconciliation is needed
	if got := helper.NeedsReconciliation(); got != true {
		t.Error("NeedsReconciliation should return true for unsupported object")
	}
}

func TestStatusHelper_Phase(t *testing.T) {
	obj := &testObject{}
	obj.Status.Phase = "Running"

	helper := newStatusHelper(obj)

	if got := helper.Phase(); got != "Running" {
		t.Errorf("Phase() = %s, want Running", got)
	}
}

func TestStatusHelper_Phase_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	if got := helper.Phase(); got != "" {
		t.Errorf("Phase() = %s, want empty string for unsupported object", got)
	}
}

func TestStatusHelper_SetPhase(t *testing.T) {
	obj := &testObject{}
	helper := newStatusHelper(obj)

	result := helper.SetPhase("Pending")

	if !result {
		t.Error("SetPhase should return true")
	}
	if obj.Status.Phase != "Pending" {
		t.Errorf("Phase = %s, want Pending", obj.Status.Phase)
	}
}

func TestStatusHelper_SetPhase_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	result := helper.SetPhase("Pending")

	if result {
		t.Error("SetPhase should return false for unsupported object")
	}
}

func TestStatusHelper_SetPhaseWithMessage(t *testing.T) {
	obj := &testObject{}
	helper := newStatusHelper(obj)

	result := helper.SetPhaseWithMessage("Failed", "Something went wrong")

	if !result {
		t.Error("SetPhaseWithMessage should return true")
	}
	if obj.Status.Phase != "Failed" {
		t.Errorf("Phase = %s, want Failed", obj.Status.Phase)
	}
	if obj.Status.Message != "Something went wrong" {
		t.Errorf("Message = %s, want 'Something went wrong'", obj.Status.Message)
	}
}

func TestStatusHelper_SetLastUpdated(t *testing.T) {
	obj := &testObject{}
	helper := newStatusHelper(obj)

	before := time.Now()
	result := helper.SetLastUpdated()
	after := time.Now()

	if !result {
		t.Error("SetLastUpdated should return true")
	}
	if obj.Status.LastUpdated.IsZero() {
		t.Error("LastUpdated should not be zero")
	}
	if obj.Status.LastUpdated.Time.Before(before) || obj.Status.LastUpdated.Time.After(after) {
		t.Error("LastUpdated should be between before and after")
	}
}

func TestStatusHelper_SetLastUpdatedTime(t *testing.T) {
	obj := &testObject{}
	helper := newStatusHelper(obj)

	specificTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := helper.SetLastUpdatedTime(specificTime)

	if !result {
		t.Error("SetLastUpdatedTime should return true")
	}
	if !obj.Status.LastUpdated.Time.Equal(specificTime) {
		t.Errorf("LastUpdated = %v, want %v", obj.Status.LastUpdated.Time, specificTime)
	}
}

func TestStatusHelper_SetLastUpdated_NotSupported(t *testing.T) {
	obj := &simpleObject{}
	helper := newStatusHelper(obj)

	result := helper.SetLastUpdated()

	if result {
		t.Error("SetLastUpdated should return false for unsupported object")
	}
}

func TestStatusAccessor_GetField(t *testing.T) {
	obj := &testObject{}
	obj.Status.Phase = "Running"
	obj.Status.ObservedGeneration = 5

	accessor := NewStatusAccessor(obj)

	phase := accessor.GetField("Phase")
	if phase != "Running" {
		t.Errorf("GetField(Phase) = %v, want Running", phase)
	}

	gen := accessor.GetField("ObservedGeneration")
	if gen != int64(5) {
		t.Errorf("GetField(ObservedGeneration) = %v, want 5", gen)
	}
}

func TestStatusAccessor_GetField_NotFound(t *testing.T) {
	obj := &testObject{}
	accessor := NewStatusAccessor(obj)

	result := accessor.GetField("NonExistent")
	if result != nil {
		t.Errorf("GetField(NonExistent) = %v, want nil", result)
	}
}

func TestStatusAccessor_SetField(t *testing.T) {
	obj := &testObject{}
	accessor := NewStatusAccessor(obj)

	result := accessor.SetField("Phase", "Completed")
	if !result {
		t.Error("SetField should return true")
	}
	if obj.Status.Phase != "Completed" {
		t.Errorf("Phase = %s, want Completed", obj.Status.Phase)
	}
}

func TestStatusAccessor_SetField_NotFound(t *testing.T) {
	obj := &testObject{}
	accessor := NewStatusAccessor(obj)

	result := accessor.SetField("NonExistent", "value")
	if result {
		t.Error("SetField should return false for non-existent field")
	}
}

func TestStatusAccessor_SetField_TypeMismatch(t *testing.T) {
	obj := &testObject{}
	accessor := NewStatusAccessor(obj)

	// Try to set a string field with an int
	result := accessor.SetField("Phase", 123)
	if result {
		t.Error("SetField should return false for type mismatch")
	}
}

func TestStatusAccessor_NoStatusField(t *testing.T) {
	obj := &simpleObject{}
	accessor := NewStatusAccessor(obj)

	result := accessor.GetField("Phase")
	if result != nil {
		t.Errorf("GetField should return nil when no Status field exists")
	}

	setResult := accessor.SetField("Phase", "Running")
	if setResult {
		t.Error("SetField should return false when no Status field exists")
	}
}
