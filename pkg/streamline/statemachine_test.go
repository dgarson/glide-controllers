package streamline

import (
	"errors"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// stateMachineTestObject implements ObjectWithPhase and ObjectWithConditions for testing.
type stateMachineTestObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            stateMachineTestStatus `json:"status,omitempty"`
}

type stateMachineTestStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (s *stateMachineTestObject) DeepCopyObject() runtime.Object {
	return &stateMachineTestObject{
		TypeMeta:   s.TypeMeta,
		ObjectMeta: *s.ObjectMeta.DeepCopy(),
		Status:     s.Status,
	}
}

func (s *stateMachineTestObject) GetPhase() string {
	return s.Status.Phase
}

func (s *stateMachineTestObject) SetPhase(phase string) {
	s.Status.Phase = phase
}

func (s *stateMachineTestObject) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

func (s *stateMachineTestObject) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

// Verify interface implementations
var _ client.Object = &stateMachineTestObject{}
var _ ObjectWithPhase = &stateMachineTestObject{}
var _ ObjectWithConditions = &stateMachineTestObject{}

// nonPhaseObject only implements client.Object, not ObjectWithPhase
type nonPhaseObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func (n *nonPhaseObject) DeepCopyObject() runtime.Object {
	return &nonPhaseObject{
		TypeMeta:   n.TypeMeta,
		ObjectMeta: *n.ObjectMeta.DeepCopy(),
	}
}

var _ client.Object = &nonPhaseObject{}

// TestNewPhaseStateMachine tests the constructor function
func TestNewPhaseStateMachine(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	if sm.InitialPhase() != "Pending" {
		t.Errorf("InitialPhase() = %s, want Pending", sm.InitialPhase())
	}

	phases := sm.Phases()
	if len(phases) != 1 {
		t.Errorf("len(Phases()) = %d, want 1", len(phases))
	}
	if phases[0] != "Pending" {
		t.Errorf("Phases()[0] = %s, want Pending", phases[0])
	}
}

// TestPhaseStateMachine_WithName tests setting a name on the state machine
func TestPhaseStateMachine_WithName(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").WithName("MyStateMachine")

	if sm.name != "MyStateMachine" {
		t.Errorf("name = %s, want MyStateMachine", sm.name)
	}
}

// TestPhaseStateMachine_Allow tests adding simple transitions
func TestPhaseStateMachine_Allow(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		Allow("Running", "Completed")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	if !sm.CanTransition("Pending", "Running", obj) {
		t.Error("Should be able to transition from Pending to Running")
	}
	if !sm.CanTransition("Running", "Completed", obj) {
		t.Error("Should be able to transition from Running to Completed")
	}
	if sm.CanTransition("Pending", "Completed", obj) {
		t.Error("Should not be able to transition from Pending to Completed")
	}
}

// TestPhaseStateMachine_AllowWithGuard tests transitions with guard conditions
func TestPhaseStateMachine_AllowWithGuard(t *testing.T) {
	guardPassed := false

	sm := NewPhaseStateMachine("Pending").
		AllowWithGuard("Pending", "Running", func(obj client.Object) bool {
			return guardPassed
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	// Guard should block transition
	if sm.CanTransition("Pending", "Running", obj) {
		t.Error("Guard should block transition when guardPassed is false")
	}

	// Guard should allow transition
	guardPassed = true
	if !sm.CanTransition("Pending", "Running", obj) {
		t.Error("Guard should allow transition when guardPassed is true")
	}
}

// TestPhaseStateMachine_AllowFrom tests adding multiple transitions from one phase
func TestPhaseStateMachine_AllowFrom(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		AllowFrom("Pending", "Running", "Failed", "Cancelled")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	allowed := sm.AllowedTransitions("Pending", obj)
	if len(allowed) != 3 {
		t.Errorf("AllowedTransitions should have 3 items, got %d", len(allowed))
	}

	expectedTargets := map[string]bool{"Running": true, "Failed": true, "Cancelled": true}
	for _, target := range allowed {
		if !expectedTargets[target] {
			t.Errorf("Unexpected allowed target: %s", target)
		}
	}
}

// TestPhaseStateMachine_AllowAny tests transitions that can come from any phase
func TestPhaseStateMachine_AllowAny(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		Allow("Running", "Completed").
		AllowAny("Failed")

	obj := &stateMachineTestObject{}

	// Failed should be reachable from any existing phase
	if !sm.CanTransition("Pending", "Failed", obj) {
		t.Error("Should be able to transition from Pending to Failed")
	}
	if !sm.CanTransition("Running", "Failed", obj) {
		t.Error("Should be able to transition from Running to Failed")
	}
	if !sm.CanTransition("Completed", "Failed", obj) {
		t.Error("Should be able to transition from Completed to Failed")
	}
}

// TestPhaseStateMachine_AllowBidirectional tests bidirectional transitions
func TestPhaseStateMachine_AllowBidirectional(t *testing.T) {
	sm := NewPhaseStateMachine("Running").
		AllowBidirectional("Running", "Paused")

	obj := &stateMachineTestObject{}

	if !sm.CanTransition("Running", "Paused", obj) {
		t.Error("Should be able to transition from Running to Paused")
	}
	if !sm.CanTransition("Paused", "Running", obj) {
		t.Error("Should be able to transition from Paused to Running")
	}
}

// TestPhaseStateMachine_OnEnter tests OnEnter callbacks
func TestPhaseStateMachine_OnEnter(t *testing.T) {
	enterCalled := false
	enteredPhase := ""

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnEnter("Running", func(obj client.Object) {
			enterCalled = true
			if owp, ok := obj.(ObjectWithPhase); ok {
				enteredPhase = owp.GetPhase()
			}
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if !enterCalled {
		t.Error("OnEnter callback was not called")
	}
	if enteredPhase != "Running" {
		t.Errorf("enteredPhase = %s, want Running", enteredPhase)
	}
}

// TestPhaseStateMachine_OnExit tests OnExit callbacks
func TestPhaseStateMachine_OnExit(t *testing.T) {
	exitCalled := false
	exitedPhase := ""

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnExit("Pending", func(obj client.Object) {
			exitCalled = true
			if owp, ok := obj.(ObjectWithPhase); ok {
				exitedPhase = owp.GetPhase()
			}
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if !exitCalled {
		t.Error("OnExit callback was not called")
	}
	// Exit callback is called before phase change
	if exitedPhase != "Pending" {
		t.Errorf("exitedPhase = %s, want Pending", exitedPhase)
	}
}

// TestPhaseStateMachine_OnTransition tests transition-specific callbacks
func TestPhaseStateMachine_OnTransition(t *testing.T) {
	transitionCalled := false

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnTransition("Pending", "Running", func(obj client.Object) {
			transitionCalled = true
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if !transitionCalled {
		t.Error("OnTransition callback was not called")
	}
}

// TestPhaseStateMachine_OnTransition_Chained tests chaining multiple OnTransition callbacks
func TestPhaseStateMachine_OnTransition_Chained(t *testing.T) {
	callOrder := []string{}

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnTransition("Pending", "Running", func(obj client.Object) {
			callOrder = append(callOrder, "first")
		}).
		OnTransition("Pending", "Running", func(obj client.Object) {
			callOrder = append(callOrder, "second")
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if len(callOrder) != 2 {
		t.Errorf("Expected 2 callbacks, got %d", len(callOrder))
	}
	if callOrder[0] != "first" || callOrder[1] != "second" {
		t.Errorf("Callbacks called in wrong order: %v", callOrder)
	}
}

// TestPhaseStateMachine_OnTransition_NewTransition tests OnTransition creating a new transition
func TestPhaseStateMachine_OnTransition_NewTransition(t *testing.T) {
	transitionCalled := false

	sm := NewPhaseStateMachine("Pending").
		OnTransition("Running", "Completed", func(obj client.Object) {
			transitionCalled = true
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Running"

	// This transition was implicitly created by OnTransition
	err := sm.Transition(obj, "Completed")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if !transitionCalled {
		t.Error("OnTransition callback was not called")
	}
}

// TestPhaseStateMachine_Transition_Success tests successful transitions
func TestPhaseStateMachine_Transition_Success(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if obj.Status.Phase != "Running" {
		t.Errorf("Phase = %s, want Running", obj.Status.Phase)
	}
}

// TestPhaseStateMachine_Transition_SamePhase tests transitioning to the same phase
func TestPhaseStateMachine_Transition_SamePhase(t *testing.T) {
	sm := NewPhaseStateMachine("Running")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Running"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition to same phase should not fail: %v", err)
	}
}

// TestPhaseStateMachine_Transition_EmptyPhase tests transitioning from empty phase
func TestPhaseStateMachine_Transition_EmptyPhase(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running")

	obj := &stateMachineTestObject{}
	// Empty phase should default to initial phase

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition from empty phase should succeed: %v", err)
	}

	if obj.Status.Phase != "Running" {
		t.Errorf("Phase = %s, want Running", obj.Status.Phase)
	}
}

// TestPhaseStateMachine_Transition_NotAllowed tests invalid transitions
func TestPhaseStateMachine_Transition_NotAllowed(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Completed")
	if err == nil {
		t.Fatal("Transition should fail when not allowed")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Error should be TransitionError, got %T", err)
	}

	if transErr.From != "Pending" {
		t.Errorf("TransitionError.From = %s, want Pending", transErr.From)
	}
	if transErr.To != "Completed" {
		t.Errorf("TransitionError.To = %s, want Completed", transErr.To)
	}
	if transErr.Guarded {
		t.Error("TransitionError.Guarded should be false")
	}
}

// TestPhaseStateMachine_Transition_GuardFailed tests transition blocked by guard
func TestPhaseStateMachine_Transition_GuardFailed(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		AllowWithGuard("Pending", "Running", func(obj client.Object) bool {
			return false // Always block
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err == nil {
		t.Fatal("Transition should fail when guard blocks it")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Error should be TransitionError, got %T", err)
	}

	if !transErr.Guarded {
		t.Error("TransitionError.Guarded should be true")
	}
}

// TestPhaseStateMachine_Transition_NotObjectWithPhase tests transition with non-phase object
func TestPhaseStateMachine_Transition_NotObjectWithPhase(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &nonPhaseObject{}

	err := sm.Transition(obj, "Running")
	if err == nil {
		t.Fatal("Transition should fail for non-ObjectWithPhase object")
	}

	if err.Error() != "object does not implement ObjectWithPhase" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestPhaseStateMachine_TransitionOrFail_Success tests TransitionOrFail on success
func TestPhaseStateMachine_TransitionOrFail_Success(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	// Should not panic
	sm.TransitionOrFail(obj, "Running")

	if obj.Status.Phase != "Running" {
		t.Errorf("Phase = %s, want Running", obj.Status.Phase)
	}
}

// TestPhaseStateMachine_TransitionOrFail_Panic tests TransitionOrFail on failure
func TestPhaseStateMachine_TransitionOrFail_Panic(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	defer func() {
		if r := recover(); r == nil {
			t.Error("TransitionOrFail should panic on invalid transition")
		}
	}()

	sm.TransitionOrFail(obj, "Running")
}

// TestPhaseStateMachine_TransitionIfAllowed tests TransitionIfAllowed
func TestPhaseStateMachine_TransitionIfAllowed(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	// Should succeed
	if !sm.TransitionIfAllowed(obj, "Running") {
		t.Error("TransitionIfAllowed should return true for valid transition")
	}

	// Should fail but not panic
	if sm.TransitionIfAllowed(obj, "Completed") {
		t.Error("TransitionIfAllowed should return false for invalid transition")
	}
}

// TestPhaseStateMachine_Initialize tests the Initialize method
func TestPhaseStateMachine_Initialize(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &stateMachineTestObject{}

	sm.Initialize(obj)

	if obj.Status.Phase != "Pending" {
		t.Errorf("Phase = %s, want Pending", obj.Status.Phase)
	}
}

// TestPhaseStateMachine_Initialize_AlreadySet tests Initialize when phase is already set
func TestPhaseStateMachine_Initialize_AlreadySet(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Running"

	sm.Initialize(obj)

	// Should not change existing phase
	if obj.Status.Phase != "Running" {
		t.Errorf("Phase = %s, want Running (unchanged)", obj.Status.Phase)
	}
}

// TestPhaseStateMachine_Initialize_NotObjectWithPhase tests Initialize with non-phase object
func TestPhaseStateMachine_Initialize_NotObjectWithPhase(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &nonPhaseObject{}

	// Should not panic
	sm.Initialize(obj)
}

// TestPhaseStateMachine_CurrentPhase tests CurrentPhase method
func TestPhaseStateMachine_CurrentPhase(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Running"

	if phase := sm.CurrentPhase(obj); phase != "Running" {
		t.Errorf("CurrentPhase() = %s, want Running", phase)
	}
}

// TestPhaseStateMachine_CurrentPhase_Empty tests CurrentPhase with empty phase
func TestPhaseStateMachine_CurrentPhase_Empty(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &stateMachineTestObject{}

	// Should return initial phase when empty
	if phase := sm.CurrentPhase(obj); phase != "Pending" {
		t.Errorf("CurrentPhase() = %s, want Pending (initial)", phase)
	}
}

// TestPhaseStateMachine_CurrentPhase_NotObjectWithPhase tests CurrentPhase with non-phase object
func TestPhaseStateMachine_CurrentPhase_NotObjectWithPhase(t *testing.T) {
	sm := NewPhaseStateMachine("Pending")

	obj := &nonPhaseObject{}

	if phase := sm.CurrentPhase(obj); phase != "" {
		t.Errorf("CurrentPhase() = %s, want empty string", phase)
	}
}

// TestPhaseStateMachine_IsTerminal tests IsTerminal method
func TestPhaseStateMachine_IsTerminal(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		Allow("Running", "Completed")

	if sm.IsTerminal("Pending") {
		t.Error("Pending should not be terminal")
	}
	if sm.IsTerminal("Running") {
		t.Error("Running should not be terminal")
	}
	if !sm.IsTerminal("Completed") {
		t.Error("Completed should be terminal")
	}
}

// TestPhaseStateMachine_AllowedTransitions tests AllowedTransitions method
func TestPhaseStateMachine_AllowedTransitions(t *testing.T) {
	guardPassed := true

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		AllowWithGuard("Pending", "Failed", func(obj client.Object) bool {
			return guardPassed
		})

	obj := &stateMachineTestObject{}

	allowed := sm.AllowedTransitions("Pending", obj)
	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed transitions, got %d", len(allowed))
	}

	// When guard fails, transition should not be in allowed list
	guardPassed = false
	allowed = sm.AllowedTransitions("Pending", obj)
	if len(allowed) != 1 {
		t.Errorf("Expected 1 allowed transition when guard fails, got %d", len(allowed))
	}
	if allowed[0] != "Running" {
		t.Errorf("Expected Running to be allowed, got %s", allowed[0])
	}
}

// TestPhaseStateMachine_CallbackOrder tests the order of callback execution
func TestPhaseStateMachine_CallbackOrder(t *testing.T) {
	callOrder := []string{}

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnExit("Pending", func(obj client.Object) {
			callOrder = append(callOrder, "exit")
		}).
		OnTransition("Pending", "Running", func(obj client.Object) {
			callOrder = append(callOrder, "transition")
		}).
		OnEnter("Running", func(obj client.Object) {
			callOrder = append(callOrder, "enter")
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	expected := []string{"exit", "transition", "enter"}
	if len(callOrder) != len(expected) {
		t.Fatalf("Expected %d callbacks, got %d", len(expected), len(callOrder))
	}

	for i, exp := range expected {
		if callOrder[i] != exp {
			t.Errorf("Callback order[%d] = %s, want %s", i, callOrder[i], exp)
		}
	}
}

// TestPhaseStateMachine_MultipleOnEnterCallbacks tests multiple OnEnter callbacks
func TestPhaseStateMachine_MultipleOnEnterCallbacks(t *testing.T) {
	callCount := 0

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnEnter("Running", func(obj client.Object) {
			callCount++
		}).
		OnEnter("Running", func(obj client.Object) {
			callCount++
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 OnEnter callbacks, got %d", callCount)
	}
}

// TestPhaseStateMachine_MultipleOnExitCallbacks tests multiple OnExit callbacks
func TestPhaseStateMachine_MultipleOnExitCallbacks(t *testing.T) {
	callCount := 0

	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		OnExit("Pending", func(obj client.Object) {
			callCount++
		}).
		OnExit("Pending", func(obj client.Object) {
			callCount++
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Pending"

	err := sm.Transition(obj, "Running")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 OnExit callbacks, got %d", callCount)
	}
}

// TestTransitionError_Error tests TransitionError error message formatting
func TestTransitionError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TransitionError
		contains string
	}{
		{
			name: "guarded transition",
			err: &TransitionError{
				From:    "Pending",
				To:      "Running",
				Reason:  "guard condition not satisfied",
				Guarded: true,
			},
			contains: "blocked by guard",
		},
		{
			name: "not allowed with alternatives",
			err: &TransitionError{
				From:    "Pending",
				To:      "Completed",
				Reason:  "transition not allowed",
				Allowed: []string{"Running", "Failed"},
			},
			contains: "allowed:",
		},
		{
			name: "not allowed without alternatives",
			err: &TransitionError{
				From:   "Pending",
				To:     "Unknown",
				Reason: "transition not allowed",
			},
			contains: "not allowed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			if len(errMsg) == 0 {
				t.Error("Error message should not be empty")
			}
			// Check that From and To are in the message
			if !containsString(errMsg, tt.err.From) {
				t.Errorf("Error message should contain From phase: %s", errMsg)
			}
			if !containsString(errMsg, tt.err.To) {
				t.Errorf("Error message should contain To phase: %s", errMsg)
			}
			if !containsString(errMsg, tt.contains) {
				t.Errorf("Error message should contain %q: %s", tt.contains, errMsg)
			}
		})
	}
}

// TestIsTransitionError tests the IsTransitionError helper function
func TestIsTransitionError(t *testing.T) {
	transErr := &TransitionError{From: "A", To: "B"}
	genericErr := errors.New("generic error")

	if !IsTransitionError(transErr) {
		t.Error("IsTransitionError should return true for TransitionError")
	}
	if IsTransitionError(genericErr) {
		t.Error("IsTransitionError should return false for generic error")
	}
	if IsTransitionError(nil) {
		t.Error("IsTransitionError should return false for nil")
	}
}

// TestStateHistory_Record tests recording transitions
func TestStateHistory_Record(t *testing.T) {
	history := NewStateHistory(10)

	history.Record("Pending", "Running", "initialization complete")

	entries := history.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.From != "Pending" {
		t.Errorf("Entry.From = %s, want Pending", entry.From)
	}
	if entry.To != "Running" {
		t.Errorf("Entry.To = %s, want Running", entry.To)
	}
	if entry.Reason != "initialization complete" {
		t.Errorf("Entry.Reason = %s, want 'initialization complete'", entry.Reason)
	}
}

// TestStateHistory_MaxSize tests that history respects max size
func TestStateHistory_MaxSize(t *testing.T) {
	history := NewStateHistory(3)

	history.Record("A", "B", "first")
	history.Record("B", "C", "second")
	history.Record("C", "D", "third")
	history.Record("D", "E", "fourth")

	entries := history.Entries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries (max size), got %d", len(entries))
	}

	// First entry should have been evicted
	if entries[0].From != "B" {
		t.Errorf("First entry should be B->C, got %s->%s", entries[0].From, entries[0].To)
	}
	if entries[2].From != "D" {
		t.Errorf("Last entry should be D->E, got %s->%s", entries[2].From, entries[2].To)
	}
}

// TestStateHistory_Last tests getting the last entry
func TestStateHistory_Last(t *testing.T) {
	history := NewStateHistory(10)

	// Empty history
	if history.Last() != nil {
		t.Error("Last() should return nil for empty history")
	}

	history.Record("A", "B", "first")
	history.Record("B", "C", "second")

	last := history.Last()
	if last == nil {
		t.Fatal("Last() should not return nil")
	}
	if last.From != "B" || last.To != "C" {
		t.Errorf("Last entry should be B->C, got %s->%s", last.From, last.To)
	}
}

// TestStateHistory_Clear tests clearing history
func TestStateHistory_Clear(t *testing.T) {
	history := NewStateHistory(10)

	history.Record("A", "B", "")
	history.Record("B", "C", "")

	history.Clear()

	entries := history.Entries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", len(entries))
	}
}

// TestStateHistory_Entries_Copy tests that Entries returns a copy
func TestStateHistory_Entries_Copy(t *testing.T) {
	history := NewStateHistory(10)
	history.Record("A", "B", "")

	entries := history.Entries()
	entries[0].From = "X"

	// Original should be unchanged
	original := history.Entries()
	if original[0].From != "A" {
		t.Error("Entries() should return a copy, not the original slice")
	}
}

// TestStateHistory_Concurrency tests thread-safety of StateHistory
func TestStateHistory_Concurrency(t *testing.T) {
	history := NewStateHistory(100)
	var wg sync.WaitGroup

	// Multiple goroutines writing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				history.Record("A", "B", "")
			}
		}(i)
	}

	// Multiple goroutines reading
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = history.Entries()
				_ = history.Last()
			}
		}()
	}

	wg.Wait()

	// Just verify it didn't panic and has reasonable size
	entries := history.Entries()
	if len(entries) == 0 || len(entries) > 100 {
		t.Errorf("Unexpected number of entries: %d", len(entries))
	}
}

// TestCommonStateMachine tests the preset CommonStateMachine
func TestCommonStateMachine(t *testing.T) {
	sm := CommonStateMachine()

	if sm.InitialPhase() != PhasePending {
		t.Errorf("Initial phase should be %s, got %s", PhasePending, sm.InitialPhase())
	}

	obj := &stateMachineTestObject{}

	// Test expected transitions
	testCases := []struct {
		from    string
		to      string
		allowed bool
	}{
		{PhasePending, PhaseInitializing, true},
		{PhasePending, PhaseFailed, true},
		{PhaseInitializing, PhaseRunning, true},
		{PhaseInitializing, PhaseFailed, true},
		{PhaseRunning, PhaseUpdating, true},
		{PhaseRunning, PhaseDegraded, true},
		{PhaseRunning, PhaseTerminating, true},
		{PhaseRunning, PhaseFailed, true},
		{PhaseRunning, PhaseSucceeded, true},
		{PhaseUpdating, PhaseRunning, true},
		{PhaseUpdating, PhaseFailed, true},
		{PhaseDegraded, PhaseRunning, true},
		{PhaseDegraded, PhaseFailed, true},
		{PhaseDegraded, PhaseTerminating, true},
		{PhaseFailed, PhasePending, true},
		// Not allowed
		{PhasePending, PhaseRunning, false},
		{PhasePending, PhaseSucceeded, false},
		{PhaseSucceeded, PhaseRunning, false},
	}

	for _, tc := range testCases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			result := sm.CanTransition(tc.from, tc.to, obj)
			if result != tc.allowed {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tc.from, tc.to, result, tc.allowed)
			}
		})
	}
}

// TestCommonStateMachine_OnEnterRunning tests OnEnter callback for Running phase
func TestCommonStateMachine_OnEnterRunning(t *testing.T) {
	sm := CommonStateMachine()

	obj := &stateMachineTestObject{}
	obj.Status.Phase = PhaseInitializing
	obj.Status.Conditions = []metav1.Condition{
		{
			Type:   ConditionTypeReady,
			Status: metav1.ConditionFalse,
			Reason: "Initializing",
		},
	}

	err := sm.Transition(obj, PhaseRunning)
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	// Check that Ready condition was updated
	for _, cond := range obj.Status.Conditions {
		if cond.Type == ConditionTypeReady {
			if cond.Status != "True" {
				t.Errorf("Ready condition status = %s, want True", cond.Status)
			}
			if cond.Reason != ReasonReady {
				t.Errorf("Ready condition reason = %s, want %s", cond.Reason, ReasonReady)
			}
			return
		}
	}
	t.Error("Ready condition not found")
}

// TestCommonStateMachine_OnEnterFailed tests OnEnter callback for Failed phase
func TestCommonStateMachine_OnEnterFailed(t *testing.T) {
	sm := CommonStateMachine()

	obj := &stateMachineTestObject{}
	obj.Status.Phase = PhaseRunning
	obj.Status.Conditions = []metav1.Condition{
		{
			Type:   ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: ReasonReady,
		},
	}

	err := sm.Transition(obj, PhaseFailed)
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	// Check that Ready condition was updated
	for _, cond := range obj.Status.Conditions {
		if cond.Type == ConditionTypeReady {
			if cond.Status != "False" {
				t.Errorf("Ready condition status = %s, want False", cond.Status)
			}
			if cond.Reason != ReasonError {
				t.Errorf("Ready condition reason = %s, want %s", cond.Reason, ReasonError)
			}
			return
		}
	}
	t.Error("Ready condition not found")
}

// TestJobStateMachine tests the preset JobStateMachine
func TestJobStateMachine(t *testing.T) {
	sm := JobStateMachine()

	if sm.InitialPhase() != PhasePending {
		t.Errorf("Initial phase should be %s, got %s", PhasePending, sm.InitialPhase())
	}

	obj := &stateMachineTestObject{}

	// Test expected transitions
	testCases := []struct {
		from    string
		to      string
		allowed bool
	}{
		{PhasePending, PhaseRunning, true},
		{PhasePending, PhaseFailed, true},
		{PhaseRunning, PhaseSucceeded, true},
		{PhaseRunning, PhaseFailed, true},
		// Not allowed
		{PhasePending, PhaseSucceeded, false},
		{PhaseSucceeded, PhaseRunning, false},
		{PhaseFailed, PhaseRunning, false},
	}

	for _, tc := range testCases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			result := sm.CanTransition(tc.from, tc.to, obj)
			if result != tc.allowed {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tc.from, tc.to, result, tc.allowed)
			}
		})
	}
}

// TestPhaseStateMachine_Concurrency tests thread-safety of state machine operations
func TestPhaseStateMachine_Concurrency(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		Allow("Running", "Completed")

	var wg sync.WaitGroup

	// Multiple goroutines checking transitions
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj := &stateMachineTestObject{}
			for j := 0; j < 100; j++ {
				_ = sm.CanTransition("Pending", "Running", obj)
				_ = sm.AllowedTransitions("Pending", obj)
				_ = sm.Phases()
				_ = sm.IsTerminal("Completed")
			}
		}()
	}

	// Goroutine adding transitions (this tests write safety)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			sm.Allow("Running", "Paused")
		}
	}()

	wg.Wait()
}

// TestPhaseStateMachine_Phases tests the Phases method
func TestPhaseStateMachine_Phases(t *testing.T) {
	sm := NewPhaseStateMachine("Pending").
		Allow("Pending", "Running").
		Allow("Running", "Completed").
		Allow("Running", "Failed")

	phases := sm.Phases()

	expectedPhases := map[string]bool{
		"Pending":   true,
		"Running":   true,
		"Completed": true,
		"Failed":    true,
	}

	if len(phases) != len(expectedPhases) {
		t.Errorf("Expected %d phases, got %d", len(expectedPhases), len(phases))
	}

	for _, phase := range phases {
		if !expectedPhases[phase] {
			t.Errorf("Unexpected phase: %s", phase)
		}
	}
}

// TestPhaseConstants tests that phase constants have expected values
func TestPhaseConstants(t *testing.T) {
	tests := []struct {
		constant string
		value    string
	}{
		{"PhasePending", PhasePending},
		{"PhaseInitializing", PhaseInitializing},
		{"PhaseRunning", PhaseRunning},
		{"PhaseUpdating", PhaseUpdating},
		{"PhaseDegraded", PhaseDegraded},
		{"PhaseTerminating", PhaseTerminating},
		{"PhaseFailed", PhaseFailed},
		{"PhaseSucceeded", PhaseSucceeded},
	}

	for _, tt := range tests {
		t.Run(tt.constant, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("%s should not be empty", tt.constant)
			}
		})
	}
}

// TestPhaseStateMachine_ComplexWorkflow tests a complex multi-step workflow
func TestPhaseStateMachine_ComplexWorkflow(t *testing.T) {
	phaseLog := []string{}

	sm := NewPhaseStateMachine("Created").
		Allow("Created", "Validating").
		Allow("Validating", "Provisioning").
		Allow("Provisioning", "Running").
		Allow("Running", "Stopping").
		Allow("Stopping", "Stopped").
		Allow("Stopped", "Running").
		AllowAny("Failed").
		OnEnter("Running", func(obj client.Object) {
			phaseLog = append(phaseLog, "entered-running")
		}).
		OnExit("Running", func(obj client.Object) {
			phaseLog = append(phaseLog, "exited-running")
		})

	obj := &stateMachineTestObject{}
	obj.Status.Phase = "Created"

	// Walk through the workflow
	steps := []string{"Validating", "Provisioning", "Running", "Stopping", "Stopped", "Running"}

	for _, step := range steps {
		if err := sm.Transition(obj, step); err != nil {
			t.Fatalf("Failed to transition to %s: %v", step, err)
		}
		if obj.Status.Phase != step {
			t.Errorf("Phase = %s, want %s", obj.Status.Phase, step)
		}
	}

	// Check callbacks were called correctly
	expectedLog := []string{"entered-running", "exited-running", "entered-running"}
	if len(phaseLog) != len(expectedLog) {
		t.Errorf("Phase log length = %d, want %d: %v", len(phaseLog), len(expectedLog), phaseLog)
	}

	// Test that Failed is reachable from anywhere
	obj.Status.Phase = "Provisioning"
	if err := sm.Transition(obj, "Failed"); err != nil {
		t.Errorf("Should be able to transition to Failed from any phase: %v", err)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
