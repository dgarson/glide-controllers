package streamline

import (
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PhaseTransition defines an allowed state change.
type PhaseTransition struct {
	// From is the starting phase.
	From string

	// To is the target phase.
	To string

	// Guard is an optional function that must return true for the transition to be allowed.
	Guard func(obj client.Object) bool

	// OnTransition is called when this transition occurs.
	OnTransition func(obj client.Object)
}

// PhaseStateMachine manages phase transitions with validation.
type PhaseStateMachine struct {
	name         string
	initialPhase string
	transitions  []PhaseTransition
	onEnter      map[string][]func(obj client.Object)
	onExit       map[string][]func(obj client.Object)
	phases       map[string]bool
	mu           sync.RWMutex
}

// NewPhaseStateMachine creates a new state machine with the given initial phase.
func NewPhaseStateMachine(initial string) *PhaseStateMachine {
	sm := &PhaseStateMachine{
		initialPhase: initial,
		transitions:  make([]PhaseTransition, 0),
		onEnter:      make(map[string][]func(obj client.Object)),
		onExit:       make(map[string][]func(obj client.Object)),
		phases:       make(map[string]bool),
	}
	sm.phases[initial] = true
	return sm
}

// WithName sets a name for the state machine (useful for logging).
func (sm *PhaseStateMachine) WithName(name string) *PhaseStateMachine {
	sm.name = name
	return sm
}

// Allow adds a single allowed transition.
func (sm *PhaseStateMachine) Allow(from, to string) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.transitions = append(sm.transitions, PhaseTransition{From: from, To: to})
	sm.phases[from] = true
	sm.phases[to] = true
	return sm
}

// AllowWithGuard adds a transition with a guard condition.
func (sm *PhaseStateMachine) AllowWithGuard(from, to string, guard func(obj client.Object) bool) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.transitions = append(sm.transitions, PhaseTransition{
		From:  from,
		To:    to,
		Guard: guard,
	})
	sm.phases[from] = true
	sm.phases[to] = true
	return sm
}

// AllowFrom adds multiple transitions from a single phase.
func (sm *PhaseStateMachine) AllowFrom(from string, to ...string) *PhaseStateMachine {
	for _, t := range to {
		sm.Allow(from, t)
	}
	return sm
}

// AllowAny allows transitions from any phase to the given phases.
// Useful for error/terminal states that can be reached from anywhere.
func (sm *PhaseStateMachine) AllowAny(to ...string) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, target := range to {
		for phase := range sm.phases {
			if phase != target {
				sm.transitions = append(sm.transitions, PhaseTransition{From: phase, To: target})
			}
		}
		sm.phases[target] = true
	}
	return sm
}

// AllowBidirectional adds transitions in both directions.
func (sm *PhaseStateMachine) AllowBidirectional(phase1, phase2 string) *PhaseStateMachine {
	return sm.Allow(phase1, phase2).Allow(phase2, phase1)
}

// OnEnter registers a callback for when a phase is entered.
func (sm *PhaseStateMachine) OnEnter(phase string, fn func(obj client.Object)) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.onEnter[phase] = append(sm.onEnter[phase], fn)
	sm.phases[phase] = true
	return sm
}

// OnExit registers a callback for when a phase is exited.
func (sm *PhaseStateMachine) OnExit(phase string, fn func(obj client.Object)) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.onExit[phase] = append(sm.onExit[phase], fn)
	sm.phases[phase] = true
	return sm
}

// OnTransition registers a callback for a specific transition.
func (sm *PhaseStateMachine) OnTransition(from, to string, fn func(obj client.Object)) *PhaseStateMachine {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i := range sm.transitions {
		if sm.transitions[i].From == from && sm.transitions[i].To == to {
			existing := sm.transitions[i].OnTransition
			sm.transitions[i].OnTransition = func(obj client.Object) {
				if existing != nil {
					existing(obj)
				}
				fn(obj)
			}
			return sm
		}
	}

	// Transition doesn't exist, add it with the callback
	sm.transitions = append(sm.transitions, PhaseTransition{
		From:         from,
		To:           to,
		OnTransition: fn,
	})
	sm.phases[from] = true
	sm.phases[to] = true
	return sm
}

// InitialPhase returns the initial phase.
func (sm *PhaseStateMachine) InitialPhase() string {
	return sm.initialPhase
}

// Phases returns all known phases.
func (sm *PhaseStateMachine) Phases() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	phases := make([]string, 0, len(sm.phases))
	for phase := range sm.phases {
		phases = append(phases, phase)
	}
	return phases
}

// CanTransition checks if a transition from one phase to another is allowed.
func (sm *PhaseStateMachine) CanTransition(from, to string, obj client.Object) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, t := range sm.transitions {
		if t.From == from && t.To == to {
			if t.Guard != nil && !t.Guard(obj) {
				return false
			}
			return true
		}
	}
	return false
}

// AllowedTransitions returns all phases that can be transitioned to from the given phase.
func (sm *PhaseStateMachine) AllowedTransitions(from string, obj client.Object) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowed := make([]string, 0)
	for _, t := range sm.transitions {
		if t.From == from {
			if t.Guard == nil || t.Guard(obj) {
				allowed = append(allowed, t.To)
			}
		}
	}
	return allowed
}

// Transition attempts to transition the object to a new phase.
// Returns an error if the transition is not allowed.
func (sm *PhaseStateMachine) Transition(obj client.Object, to string) error {
	owp, ok := obj.(ObjectWithPhase)
	if !ok {
		return fmt.Errorf("object does not implement ObjectWithPhase")
	}

	from := owp.GetPhase()

	// Handle initial state (empty phase)
	if from == "" {
		from = sm.initialPhase
	}

	if from == to {
		return nil // No transition needed
	}

	sm.mu.RLock()
	var matchingTransition *PhaseTransition
	for i := range sm.transitions {
		t := &sm.transitions[i]
		if t.From == from && t.To == to {
			if t.Guard != nil && !t.Guard(obj) {
				sm.mu.RUnlock()
				return &TransitionError{
					From:    from,
					To:      to,
					Reason:  "guard condition not satisfied",
					Guarded: true,
				}
			}
			matchingTransition = t
			break
		}
	}
	sm.mu.RUnlock()

	if matchingTransition == nil {
		return &TransitionError{
			From:    from,
			To:      to,
			Reason:  "transition not allowed",
			Allowed: sm.AllowedTransitions(from, obj),
		}
	}

	// Execute exit callbacks
	sm.mu.RLock()
	exitCallbacks := sm.onExit[from]
	sm.mu.RUnlock()
	for _, fn := range exitCallbacks {
		fn(obj)
	}

	// Execute transition callback
	if matchingTransition.OnTransition != nil {
		matchingTransition.OnTransition(obj)
	}

	// Set the new phase
	owp.SetPhase(to)

	// Execute enter callbacks
	sm.mu.RLock()
	enterCallbacks := sm.onEnter[to]
	sm.mu.RUnlock()
	for _, fn := range enterCallbacks {
		fn(obj)
	}

	return nil
}

// TransitionOrFail transitions to the new phase or panics.
// Use only when the transition is guaranteed to succeed.
func (sm *PhaseStateMachine) TransitionOrFail(obj client.Object, to string) {
	if err := sm.Transition(obj, to); err != nil {
		panic(fmt.Sprintf("failed to transition to %s: %v", to, err))
	}
}

// TransitionIfAllowed attempts the transition and returns whether it succeeded.
func (sm *PhaseStateMachine) TransitionIfAllowed(obj client.Object, to string) bool {
	return sm.Transition(obj, to) == nil
}

// Initialize sets the object to the initial phase if it has no phase set.
func (sm *PhaseStateMachine) Initialize(obj client.Object) {
	if owp, ok := obj.(ObjectWithPhase); ok {
		if owp.GetPhase() == "" {
			owp.SetPhase(sm.initialPhase)
		}
	}
}

// CurrentPhase returns the current phase of an object.
func (sm *PhaseStateMachine) CurrentPhase(obj client.Object) string {
	if owp, ok := obj.(ObjectWithPhase); ok {
		phase := owp.GetPhase()
		if phase == "" {
			return sm.initialPhase
		}
		return phase
	}
	return ""
}

// IsTerminal checks if a phase is terminal (has no outgoing transitions).
func (sm *PhaseStateMachine) IsTerminal(phase string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, t := range sm.transitions {
		if t.From == phase {
			return false
		}
	}
	return true
}

// TransitionError represents a failed state transition.
type TransitionError struct {
	// From is the current phase.
	From string

	// To is the attempted target phase.
	To string

	// Reason describes why the transition failed.
	Reason string

	// Guarded indicates the transition exists but the guard failed.
	Guarded bool

	// Allowed lists the allowed target phases from the current phase.
	Allowed []string
}

// Error implements the error interface.
func (e *TransitionError) Error() string {
	if e.Guarded {
		return fmt.Sprintf("transition from %s to %s blocked by guard: %s", e.From, e.To, e.Reason)
	}
	if len(e.Allowed) > 0 {
		return fmt.Sprintf("transition from %s to %s not allowed; allowed: %v", e.From, e.To, e.Allowed)
	}
	return fmt.Sprintf("transition from %s to %s not allowed: %s", e.From, e.To, e.Reason)
}

// IsTransitionError checks if an error is a TransitionError.
func IsTransitionError(err error) bool {
	_, ok := err.(*TransitionError)
	return ok
}

// StateHistory tracks phase transitions over time.
type StateHistory struct {
	entries []StateHistoryEntry
	maxSize int
	mu      sync.RWMutex
}

// StateHistoryEntry records a single phase transition.
type StateHistoryEntry struct {
	From      string
	To        string
	Timestamp int64 // Unix timestamp
	Reason    string
}

// NewStateHistory creates a new StateHistory with the given maximum size.
func NewStateHistory(maxSize int) *StateHistory {
	return &StateHistory{
		entries: make([]StateHistoryEntry, 0),
		maxSize: maxSize,
	}
}

// Record adds a transition to the history.
func (sh *StateHistory) Record(from, to, reason string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	entry := StateHistoryEntry{
		From:   from,
		To:     to,
		Reason: reason,
	}

	if len(sh.entries) >= sh.maxSize {
		sh.entries = sh.entries[1:]
	}
	sh.entries = append(sh.entries, entry)
}

// Entries returns all history entries.
func (sh *StateHistory) Entries() []StateHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	result := make([]StateHistoryEntry, len(sh.entries))
	copy(result, sh.entries)
	return result
}

// Last returns the most recent entry.
func (sh *StateHistory) Last() *StateHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	if len(sh.entries) == 0 {
		return nil
	}
	entry := sh.entries[len(sh.entries)-1]
	return &entry
}

// Clear removes all history entries.
func (sh *StateHistory) Clear() {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.entries = sh.entries[:0]
}

// Common phase constants for typical controller patterns.
const (
	// PhasePending indicates the resource is waiting to be processed.
	PhasePending = "Pending"

	// PhaseInitializing indicates the resource is being initialized.
	PhaseInitializing = "Initializing"

	// PhaseRunning indicates the resource is running normally.
	PhaseRunning = "Running"

	// PhaseUpdating indicates the resource is being updated.
	PhaseUpdating = "Updating"

	// PhaseDegraded indicates the resource is running in a degraded state.
	PhaseDegraded = "Degraded"

	// PhaseTerminating indicates the resource is being terminated.
	PhaseTerminating = "Terminating"

	// PhaseFailed indicates the resource has failed.
	PhaseFailed = "Failed"

	// PhaseSucceeded indicates the resource has completed successfully.
	PhaseSucceeded = "Succeeded"
)

// CommonStateMachine returns a pre-configured state machine for typical controller patterns.
func CommonStateMachine() *PhaseStateMachine {
	return NewPhaseStateMachine(PhasePending).
		AllowFrom(PhasePending, PhaseInitializing, PhaseFailed).
		AllowFrom(PhaseInitializing, PhaseRunning, PhaseFailed).
		AllowFrom(PhaseRunning, PhaseUpdating, PhaseDegraded, PhaseTerminating, PhaseFailed, PhaseSucceeded).
		AllowFrom(PhaseUpdating, PhaseRunning, PhaseFailed).
		AllowFrom(PhaseDegraded, PhaseRunning, PhaseFailed, PhaseTerminating).
		AllowFrom(PhaseFailed, PhasePending). // Allow retry
		OnEnter(PhaseRunning, func(obj client.Object) {
			// Set Ready condition when entering Running
			if owc, ok := obj.(ObjectWithConditions); ok {
				conditions := owc.GetConditions()
				for i := range conditions {
					if conditions[i].Type == ConditionTypeReady {
						conditions[i].Status = "True"
						conditions[i].Reason = ReasonReady
						conditions[i].Message = "Resource is running"
						owc.SetConditions(conditions)
						return
					}
				}
			}
		}).
		OnEnter(PhaseFailed, func(obj client.Object) {
			// Set Ready=False when entering Failed
			if owc, ok := obj.(ObjectWithConditions); ok {
				conditions := owc.GetConditions()
				for i := range conditions {
					if conditions[i].Type == ConditionTypeReady {
						conditions[i].Status = "False"
						conditions[i].Reason = ReasonError
						owc.SetConditions(conditions)
						return
					}
				}
			}
		})
}

// JobStateMachine returns a state machine for job-like resources.
func JobStateMachine() *PhaseStateMachine {
	return NewPhaseStateMachine(PhasePending).
		Allow(PhasePending, PhaseRunning).
		Allow(PhasePending, PhaseFailed).
		Allow(PhaseRunning, PhaseSucceeded).
		Allow(PhaseRunning, PhaseFailed)
}
