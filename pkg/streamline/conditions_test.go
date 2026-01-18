package streamline

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// testObjectWithConditions implements ObjectWithConditions.
type testObjectWithConditions struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            testStatusWithConditions `json:"status,omitempty"`
}

type testStatusWithConditions struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (t *testObjectWithConditions) DeepCopyObject() runtime.Object {
	conditions := make([]metav1.Condition, len(t.Status.Conditions))
	copy(conditions, t.Status.Conditions)
	return &testObjectWithConditions{
		TypeMeta:   t.TypeMeta,
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
		Status:     testStatusWithConditions{Conditions: conditions},
	}
}

func (t *testObjectWithConditions) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

func (t *testObjectWithConditions) SetConditions(conditions []metav1.Condition) {
	t.Status.Conditions = conditions
}

var _ client.Object = &testObjectWithConditions{}
var _ ObjectWithConditions = &testObjectWithConditions{}

func TestConditionHelper_Set(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Generation = 5
	helper := newConditionHelper(obj)

	helper.Set("Ready", metav1.ConditionTrue, "AllGood", "Everything is working")

	conditions := obj.Status.Conditions
	if len(conditions) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(conditions))
	}

	cond := conditions[0]
	if cond.Type != "Ready" {
		t.Errorf("Type = %s, want Ready", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", cond.Status)
	}
	if cond.Reason != "AllGood" {
		t.Errorf("Reason = %s, want AllGood", cond.Reason)
	}
	if cond.Message != "Everything is working" {
		t.Errorf("Message = %s, want 'Everything is working'", cond.Message)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
	if cond.LastTransitionTime.IsZero() {
		t.Error("LastTransitionTime should not be zero")
	}
}

func TestConditionHelper_Set_UpdateExisting(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Generation = 5
	obj.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "NotReady",
			Message:            "Initial",
			LastTransitionTime: metav1.Now(),
		},
	}
	originalTime := obj.Status.Conditions[0].LastTransitionTime

	helper := newConditionHelper(obj)

	// Update to True - should update LastTransitionTime
	helper.Set("Ready", metav1.ConditionTrue, "AllGood", "Now ready")

	cond := obj.Status.Conditions[0]
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", cond.Status)
	}
	if cond.LastTransitionTime.Equal(&originalTime) {
		t.Error("LastTransitionTime should be updated when status changes")
	}
}

func TestConditionHelper_Set_SameStatus_NoTransitionTimeUpdate(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Generation = 5
	originalTime := metav1.Now()
	obj.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "OldReason",
			Message:            "Old message",
			LastTransitionTime: originalTime,
		},
	}

	helper := newConditionHelper(obj)

	// Same status, different reason/message - should NOT update LastTransitionTime
	helper.Set("Ready", metav1.ConditionTrue, "NewReason", "New message")

	cond := obj.Status.Conditions[0]
	if cond.Reason != "NewReason" {
		t.Errorf("Reason = %s, want NewReason", cond.Reason)
	}
	if !cond.LastTransitionTime.Equal(&originalTime) {
		t.Error("LastTransitionTime should NOT change when status is the same")
	}
}

func TestConditionHelper_SetTrue(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetTrue("Available", "Ready", "Resource is available")

	cond := helper.Get("Available")
	if cond == nil {
		t.Fatal("Condition should exist")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", cond.Status)
	}
}

func TestConditionHelper_SetFalse(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetFalse("Available", "NotReady", "Resource is not available")

	cond := helper.Get("Available")
	if cond == nil {
		t.Fatal("Condition should exist")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %s, want False", cond.Status)
	}
}

func TestConditionHelper_SetUnknown(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetUnknown("Available", "Checking", "Checking resource status")

	cond := helper.Get("Available")
	if cond == nil {
		t.Fatal("Condition should exist")
	}
	if cond.Status != metav1.ConditionUnknown {
		t.Errorf("Status = %s, want Unknown", cond.Status)
	}
}

func TestConditionHelper_SetFromError_WithError(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	err := errors.New("connection failed")
	helper.SetFromError("Synced", err, "SyncSuccess", "Synced successfully")

	cond := helper.Get("Synced")
	if cond == nil {
		t.Fatal("Condition should exist")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %s, want False", cond.Status)
	}
	if cond.Reason != ReasonError {
		t.Errorf("Reason = %s, want %s", cond.Reason, ReasonError)
	}
	if cond.Message != "connection failed" {
		t.Errorf("Message = %s, want 'connection failed'", cond.Message)
	}
}

func TestConditionHelper_SetFromError_NoError(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetFromError("Synced", nil, "SyncSuccess", "Synced successfully")

	cond := helper.Get("Synced")
	if cond == nil {
		t.Fatal("Condition should exist")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", cond.Status)
	}
	if cond.Reason != "SyncSuccess" {
		t.Errorf("Reason = %s, want SyncSuccess", cond.Reason)
	}
}

func TestConditionHelper_SetReady(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetReady(ReasonReady, "All systems operational")

	if !helper.IsTrue(ConditionTypeReady) {
		t.Error("Ready condition should be True")
	}
}

func TestConditionHelper_SetNotReady(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetNotReady(ReasonNotReady, "Missing dependencies")

	if !helper.IsFalse(ConditionTypeReady) {
		t.Error("Ready condition should be False")
	}
}

func TestConditionHelper_SetProgressing(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetProgressing(ReasonReconciling, "Creating resources")

	if !helper.IsTrue(ConditionTypeProgressing) {
		t.Error("Progressing condition should be True")
	}
}

func TestConditionHelper_ClearProgressing(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetProgressing(ReasonReconciling, "Creating resources")
	helper.ClearProgressing(ReasonReconcileSuccess, "Done")

	if !helper.IsFalse(ConditionTypeProgressing) {
		t.Error("Progressing condition should be False")
	}
}

func TestConditionHelper_SetDegraded(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetDegraded("HighLatency", "Response times are elevated")

	if !helper.IsTrue(ConditionTypeDegraded) {
		t.Error("Degraded condition should be True")
	}
}

func TestConditionHelper_ClearDegraded(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetDegraded("HighLatency", "Response times are elevated")
	helper.ClearDegraded("Normal", "Back to normal")

	if !helper.IsFalse(ConditionTypeDegraded) {
		t.Error("Degraded condition should be False")
	}
}

func TestConditionHelper_Get(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
	}
	helper := newConditionHelper(obj)

	ready := helper.Get("Ready")
	if ready == nil {
		t.Fatal("Ready condition should exist")
	}
	if ready.Status != metav1.ConditionTrue {
		t.Errorf("Ready status = %s, want True", ready.Status)
	}

	available := helper.Get("Available")
	if available == nil {
		t.Fatal("Available condition should exist")
	}
	if available.Status != metav1.ConditionFalse {
		t.Errorf("Available status = %s, want False", available.Status)
	}

	nonExistent := helper.Get("NonExistent")
	if nonExistent != nil {
		t.Error("NonExistent condition should be nil")
	}
}

func TestConditionHelper_IsTrue(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
	}
	helper := newConditionHelper(obj)

	if !helper.IsTrue("Ready") {
		t.Error("IsTrue(Ready) should return true")
	}
	if helper.IsTrue("Available") {
		t.Error("IsTrue(Available) should return false")
	}
	if helper.IsTrue("NonExistent") {
		t.Error("IsTrue(NonExistent) should return false")
	}
}

func TestConditionHelper_IsFalse(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
	}
	helper := newConditionHelper(obj)

	if helper.IsFalse("Ready") {
		t.Error("IsFalse(Ready) should return false")
	}
	if !helper.IsFalse("Available") {
		t.Error("IsFalse(Available) should return true")
	}
	if helper.IsFalse("NonExistent") {
		t.Error("IsFalse(NonExistent) should return false")
	}
}

func TestConditionHelper_IsUnknown(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Checking", Status: metav1.ConditionUnknown},
	}
	helper := newConditionHelper(obj)

	if helper.IsUnknown("Ready") {
		t.Error("IsUnknown(Ready) should return false")
	}
	if !helper.IsUnknown("Checking") {
		t.Error("IsUnknown(Checking) should return true")
	}
	if !helper.IsUnknown("NonExistent") {
		t.Error("IsUnknown(NonExistent) should return true")
	}
}

func TestConditionHelper_Remove(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
		{Type: "Progressing", Status: metav1.ConditionTrue},
	}
	helper := newConditionHelper(obj)

	helper.Remove("Available")

	if len(obj.Status.Conditions) != 2 {
		t.Errorf("Expected 2 conditions after remove, got %d", len(obj.Status.Conditions))
	}
	if helper.HasCondition("Available") {
		t.Error("Available condition should be removed")
	}
	if !helper.HasCondition("Ready") {
		t.Error("Ready condition should still exist")
	}
	if !helper.HasCondition("Progressing") {
		t.Error("Progressing condition should still exist")
	}
}

func TestConditionHelper_GetAll(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
	}
	helper := newConditionHelper(obj)

	all := helper.GetAll()
	if len(all) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(all))
	}
}

func TestConditionHelper_Count(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Available", Status: metav1.ConditionFalse},
	}
	helper := newConditionHelper(obj)

	if count := helper.Count(); count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}
}

func TestConditionHelper_HasCondition(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
	}
	helper := newConditionHelper(obj)

	if !helper.HasCondition("Ready") {
		t.Error("HasCondition(Ready) should return true")
	}
	if helper.HasCondition("NonExistent") {
		t.Error("HasCondition(NonExistent) should return false")
	}
}

func TestConditionHelper_LastTransitionTime(t *testing.T) {
	obj := &testObjectWithConditions{}
	now := metav1.Now()
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: now},
	}
	helper := newConditionHelper(obj)

	ltt := helper.LastTransitionTime("Ready")
	if !ltt.Equal(&now) {
		t.Errorf("LastTransitionTime = %v, want %v", ltt, now)
	}

	nonExistent := helper.LastTransitionTime("NonExistent")
	if !nonExistent.IsZero() {
		t.Error("LastTransitionTime for non-existent should be zero")
	}
}

func TestConditionHelper_IsStale(t *testing.T) {
	obj := &testObjectWithConditions{}
	obj.Generation = 5
	obj.Status.Conditions = []metav1.Condition{
		{Type: "Current", Status: metav1.ConditionTrue, ObservedGeneration: 5},
		{Type: "Stale", Status: metav1.ConditionTrue, ObservedGeneration: 3},
	}
	helper := newConditionHelper(obj)

	if helper.IsStale("Current") {
		t.Error("Current condition should not be stale")
	}
	if !helper.IsStale("Stale") {
		t.Error("Stale condition should be stale")
	}
	if !helper.IsStale("NonExistent") {
		t.Error("Non-existent condition should be considered stale")
	}
}

func TestNoopConditionHelper(t *testing.T) {
	// When object doesn't implement ObjectWithConditions
	obj := &simpleObject{}
	helper := newConditionHelper(obj)

	// All operations should be no-ops and return safe defaults
	helper.Set("Ready", metav1.ConditionTrue, "Test", "Test")
	helper.SetTrue("Ready", "Test", "Test")
	helper.SetFalse("Ready", "Test", "Test")
	helper.SetUnknown("Ready", "Test", "Test")
	helper.SetFromError("Ready", nil, "Test", "Test")
	helper.SetReady("Test", "Test")
	helper.SetNotReady("Test", "Test")
	helper.SetProgressing("Test", "Test")
	helper.ClearProgressing("Test", "Test")
	helper.SetDegraded("Test", "Test")
	helper.ClearDegraded("Test", "Test")
	helper.Remove("Ready")

	if helper.Get("Ready") != nil {
		t.Error("Get should return nil for noop helper")
	}
	if helper.IsTrue("Ready") {
		t.Error("IsTrue should return false for noop helper")
	}
	if helper.IsFalse("Ready") {
		t.Error("IsFalse should return false for noop helper")
	}
	if !helper.IsUnknown("Ready") {
		t.Error("IsUnknown should return true for noop helper")
	}
	if helper.GetAll() != nil {
		t.Error("GetAll should return nil for noop helper")
	}
	if helper.Count() != 0 {
		t.Error("Count should return 0 for noop helper")
	}
	if helper.HasCondition("Ready") {
		t.Error("HasCondition should return false for noop helper")
	}
	ltt := helper.LastTransitionTime("Ready")
	if !ltt.IsZero() {
		t.Error("LastTransitionTime should return zero for noop helper")
	}
	if !helper.IsStale("Ready") {
		t.Error("IsStale should return true for noop helper")
	}
}

func TestConditionBuilder(t *testing.T) {
	cond := NewCondition("Ready").
		True().
		Reason("AllGood").
		Message("Everything works").
		ObservedGeneration(5).
		Build()

	if cond.Type != "Ready" {
		t.Errorf("Type = %s, want Ready", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", cond.Status)
	}
	if cond.Reason != "AllGood" {
		t.Errorf("Reason = %s, want AllGood", cond.Reason)
	}
	if cond.Message != "Everything works" {
		t.Errorf("Message = %s, want 'Everything works'", cond.Message)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
	if cond.LastTransitionTime.IsZero() {
		t.Error("LastTransitionTime should not be zero")
	}
}

func TestConditionBuilder_Messagef(t *testing.T) {
	cond := NewCondition("Status").
		False().
		Reason("Failed").
		Messagef("Failed after %d attempts: %s", 3, "timeout").
		Build()

	if cond.Message != "Failed after 3 attempts: timeout" {
		t.Errorf("Message = %s, want 'Failed after 3 attempts: timeout'", cond.Message)
	}
}

func TestConditionBuilder_Apply(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	NewCondition("Ready").
		True().
		Reason("AllGood").
		Message("Works").
		Apply(helper)

	if !helper.IsTrue("Ready") {
		t.Error("Ready condition should be True after Apply")
	}
}

func TestSummaryCondition_AllTrue(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetTrue("DatabaseReady", "Connected", "Database connected")
	helper.SetTrue("CacheReady", "Connected", "Cache connected")
	helper.SetTrue("APIReady", "Connected", "API connected")

	summary := SummaryCondition(helper, "DatabaseReady", "CacheReady", "APIReady")

	if summary.Status != metav1.ConditionTrue {
		t.Errorf("Summary status = %s, want True", summary.Status)
	}
	if summary.Reason != ReasonReady {
		t.Errorf("Summary reason = %s, want %s", summary.Reason, ReasonReady)
	}
}

func TestSummaryCondition_SomeFalse(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetTrue("DatabaseReady", "Connected", "Database connected")
	helper.SetFalse("CacheReady", "Disconnected", "Cache disconnected")
	helper.SetTrue("APIReady", "Connected", "API connected")

	summary := SummaryCondition(helper, "DatabaseReady", "CacheReady", "APIReady")

	if summary.Status != metav1.ConditionFalse {
		t.Errorf("Summary status = %s, want False", summary.Status)
	}
	if summary.Reason != ReasonNotReady {
		t.Errorf("Summary reason = %s, want %s", summary.Reason, ReasonNotReady)
	}
}

func TestSummaryCondition_SomeUnknown(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetTrue("DatabaseReady", "Connected", "Database connected")
	helper.SetUnknown("CacheReady", "Checking", "Checking cache")
	helper.SetTrue("APIReady", "Connected", "API connected")

	summary := SummaryCondition(helper, "DatabaseReady", "CacheReady", "APIReady")

	if summary.Status != metav1.ConditionUnknown {
		t.Errorf("Summary status = %s, want Unknown", summary.Status)
	}
}

func TestSummaryCondition_Missing(t *testing.T) {
	obj := &testObjectWithConditions{}
	helper := newConditionHelper(obj)

	helper.SetTrue("DatabaseReady", "Connected", "Database connected")
	// CacheReady is missing

	summary := SummaryCondition(helper, "DatabaseReady", "CacheReady")

	if summary.Status != metav1.ConditionUnknown {
		t.Errorf("Summary status = %s, want Unknown when condition is missing", summary.Status)
	}
}

func TestTransitionTracker(t *testing.T) {
	tracker := NewTransitionTracker()

	if tracker.HasTransitions() {
		t.Error("New tracker should have no transitions")
	}

	oldCond := &metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse}
	newCond := &metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood"}

	tracker.Track("Ready", oldCond, newCond, 5)

	if !tracker.HasTransitions() {
		t.Error("Tracker should have transitions after Track")
	}

	transitions := tracker.GetTransitions()
	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	tr := transitions[0]
	if tr.Type != "Ready" {
		t.Errorf("Type = %s, want Ready", tr.Type)
	}
	if tr.OldStatus != metav1.ConditionFalse {
		t.Errorf("OldStatus = %s, want False", tr.OldStatus)
	}
	if tr.NewStatus != metav1.ConditionTrue {
		t.Errorf("NewStatus = %s, want True", tr.NewStatus)
	}
	if tr.Reason != "AllGood" {
		t.Errorf("Reason = %s, want AllGood", tr.Reason)
	}
	if tr.Generation != 5 {
		t.Errorf("Generation = %d, want 5", tr.Generation)
	}

	tracker.Clear()
	if tracker.HasTransitions() {
		t.Error("Tracker should have no transitions after Clear")
	}
}

func TestTransitionTracker_NoTransition(t *testing.T) {
	tracker := NewTransitionTracker()

	// Same status - no transition
	oldCond := &metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}
	newCond := &metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}

	tracker.Track("Ready", oldCond, newCond, 5)

	if tracker.HasTransitions() {
		t.Error("Tracker should not record transition when status is unchanged")
	}
}

func TestTransitionTracker_NilOldCondition(t *testing.T) {
	tracker := NewTransitionTracker()

	newCond := &metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}

	tracker.Track("Ready", nil, newCond, 5)

	if !tracker.HasTransitions() {
		t.Error("Tracker should record transition from nil (Unknown) to True")
	}

	tr := tracker.GetTransitions()[0]
	if tr.OldStatus != metav1.ConditionUnknown {
		t.Errorf("OldStatus = %s, want Unknown for nil old condition", tr.OldStatus)
	}
}
