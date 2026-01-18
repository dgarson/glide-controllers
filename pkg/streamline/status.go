package streamline

import (
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectWithObservedGeneration is an interface for objects that track observed generation
// in their status. When implemented, the framework will automatically update
// status.observedGeneration to match metadata.generation after successful reconciliation.
//
// Example implementation:
//
//	type MyResourceStatus struct {
//	    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
//	    // other status fields...
//	}
//
//	func (r *MyResource) GetObservedGeneration() int64 {
//	    return r.Status.ObservedGeneration
//	}
//
//	func (r *MyResource) SetObservedGeneration(gen int64) {
//	    r.Status.ObservedGeneration = gen
//	}
type ObjectWithObservedGeneration interface {
	client.Object

	// GetObservedGeneration returns the observed generation from the object's status.
	GetObservedGeneration() int64

	// SetObservedGeneration sets the observed generation in the object's status.
	SetObservedGeneration(generation int64)
}

// ObjectWithPhase is an interface for objects that track a phase in their status.
// When implemented, the StatusHelper provides convenient phase management methods.
//
// Example implementation:
//
//	func (r *MyResource) GetPhase() string {
//	    return r.Status.Phase
//	}
//
//	func (r *MyResource) SetPhase(phase string) {
//	    r.Status.Phase = phase
//	}
type ObjectWithPhase interface {
	client.Object

	// GetPhase returns the current phase from the object's status.
	GetPhase() string

	// SetPhase sets the phase in the object's status.
	SetPhase(phase string)
}

// ObjectWithConditions is an interface for objects that track conditions in their status.
// When implemented, the ConditionHelper provides standard condition management.
//
// Example implementation:
//
//	func (r *MyResource) GetConditions() []metav1.Condition {
//	    return r.Status.Conditions
//	}
//
//	func (r *MyResource) SetConditions(conditions []metav1.Condition) {
//	    r.Status.Conditions = conditions
//	}
type ObjectWithConditions interface {
	client.Object

	// GetConditions returns the conditions slice from the object's status.
	GetConditions() []metav1.Condition

	// SetConditions sets the conditions slice in the object's status.
	SetConditions(conditions []metav1.Condition)
}

// StatusHelper provides utilities for common status management operations.
// It is available through Context.Status and operates on the object being reconciled.
type StatusHelper interface {
	// Generation returns the current metadata.generation of the object.
	Generation() int64

	// ObservedGeneration returns the status.observedGeneration if the object
	// implements ObjectWithObservedGeneration, otherwise returns 0.
	ObservedGeneration() int64

	// SetObservedGeneration updates status.observedGeneration to the given value
	// if the object implements ObjectWithObservedGeneration.
	// Returns true if the value was set, false if the object doesn't support it.
	SetObservedGeneration(generation int64) bool

	// MarkObserved sets status.observedGeneration to match metadata.generation.
	// This indicates that the current generation has been observed and processed.
	// Returns true if the value was set, false if the object doesn't support it.
	MarkObserved() bool

	// IsUpToDate returns true if observedGeneration equals generation,
	// indicating the object's current spec has been fully reconciled.
	// Returns false if the object doesn't implement ObjectWithObservedGeneration.
	IsUpToDate() bool

	// NeedsReconciliation returns true if observedGeneration < generation,
	// indicating there are unprocessed spec changes.
	// Returns true if the object doesn't implement ObjectWithObservedGeneration
	// (assumes reconciliation is always needed if we can't track it).
	NeedsReconciliation() bool

	// Phase returns the current phase if the object implements ObjectWithPhase,
	// otherwise returns an empty string.
	Phase() string

	// SetPhase sets the phase if the object implements ObjectWithPhase.
	// Returns true if the value was set, false if the object doesn't support it.
	SetPhase(phase string) bool

	// SetPhaseWithMessage sets both phase and a status message if the object
	// supports ObjectWithPhase and ObjectWithMessage interfaces.
	// Returns true if at least the phase was set.
	SetPhaseWithMessage(phase, message string) bool

	// SetLastUpdated sets the lastUpdated timestamp to now if the object
	// implements ObjectWithLastUpdated.
	// Returns true if the value was set, false if the object doesn't support it.
	SetLastUpdated() bool

	// SetLastUpdatedTime sets the lastUpdated timestamp to the given time if the object
	// implements ObjectWithLastUpdated.
	// Returns true if the value was set, false if the object doesn't support it.
	SetLastUpdatedTime(t time.Time) bool
}

// ObjectWithMessage is an interface for objects that track a message in their status.
type ObjectWithMessage interface {
	client.Object

	// GetMessage returns the message from the object's status.
	GetMessage() string

	// SetMessage sets the message in the object's status.
	SetMessage(message string)
}

// ObjectWithLastUpdated is an interface for objects that track when they were last updated.
type ObjectWithLastUpdated interface {
	client.Object

	// GetLastUpdated returns the last updated timestamp from the object's status.
	GetLastUpdated() metav1.Time

	// SetLastUpdated sets the last updated timestamp in the object's status.
	SetLastUpdated(t metav1.Time)
}

// statusHelper implements StatusHelper for a given object.
type statusHelper struct {
	obj client.Object
}

// newStatusHelper creates a new StatusHelper for the given object.
func newStatusHelper(obj client.Object) StatusHelper {
	return &statusHelper{obj: obj}
}

func (s *statusHelper) Generation() int64 {
	return s.obj.GetGeneration()
}

func (s *statusHelper) ObservedGeneration() int64 {
	if owog, ok := s.obj.(ObjectWithObservedGeneration); ok {
		return owog.GetObservedGeneration()
	}
	return 0
}

func (s *statusHelper) SetObservedGeneration(generation int64) bool {
	if owog, ok := s.obj.(ObjectWithObservedGeneration); ok {
		owog.SetObservedGeneration(generation)
		return true
	}
	return false
}

func (s *statusHelper) MarkObserved() bool {
	return s.SetObservedGeneration(s.Generation())
}

func (s *statusHelper) IsUpToDate() bool {
	if owog, ok := s.obj.(ObjectWithObservedGeneration); ok {
		return owog.GetObservedGeneration() == s.obj.GetGeneration()
	}
	return false
}

func (s *statusHelper) NeedsReconciliation() bool {
	if owog, ok := s.obj.(ObjectWithObservedGeneration); ok {
		return owog.GetObservedGeneration() < s.obj.GetGeneration()
	}
	// If we can't track, assume reconciliation is needed
	return true
}

func (s *statusHelper) Phase() string {
	if owp, ok := s.obj.(ObjectWithPhase); ok {
		return owp.GetPhase()
	}
	return ""
}

func (s *statusHelper) SetPhase(phase string) bool {
	if owp, ok := s.obj.(ObjectWithPhase); ok {
		owp.SetPhase(phase)
		return true
	}
	return false
}

func (s *statusHelper) SetPhaseWithMessage(phase, message string) bool {
	phaseSet := s.SetPhase(phase)
	if owm, ok := s.obj.(ObjectWithMessage); ok {
		owm.SetMessage(message)
	}
	return phaseSet
}

func (s *statusHelper) SetLastUpdated() bool {
	return s.SetLastUpdatedTime(time.Now())
}

func (s *statusHelper) SetLastUpdatedTime(t time.Time) bool {
	if owlu, ok := s.obj.(ObjectWithLastUpdated); ok {
		owlu.SetLastUpdated(metav1.NewTime(t))
		return true
	}
	return false
}

// StatusAccessor provides a way to access common status fields using reflection.
// This is useful for objects that don't implement the status interfaces but have
// conventional status field names.
type StatusAccessor struct {
	obj client.Object
}

// NewStatusAccessor creates a StatusAccessor for the given object.
// It uses reflection to access status fields by conventional names.
func NewStatusAccessor(obj client.Object) *StatusAccessor {
	return &StatusAccessor{obj: obj}
}

// GetField retrieves a field from the object's Status struct by name.
// Returns nil if the field doesn't exist or the object doesn't have a Status field.
func (a *StatusAccessor) GetField(fieldName string) interface{} {
	statusValue := a.getStatusValue()
	if !statusValue.IsValid() {
		return nil
	}

	field := statusValue.FieldByName(fieldName)
	if !field.IsValid() {
		return nil
	}

	return field.Interface()
}

// SetField sets a field in the object's Status struct by name.
// Returns true if the field was set, false if it doesn't exist or is not settable.
func (a *StatusAccessor) SetField(fieldName string, value interface{}) bool {
	statusValue := a.getStatusValue()
	if !statusValue.IsValid() {
		return false
	}

	field := statusValue.FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() {
		return false
	}

	val := reflect.ValueOf(value)
	if !val.Type().AssignableTo(field.Type()) {
		return false
	}

	field.Set(val)
	return true
}

// getStatusValue returns the reflect.Value of the Status field.
func (a *StatusAccessor) getStatusValue() reflect.Value {
	objValue := reflect.ValueOf(a.obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}

	if objValue.Kind() != reflect.Struct {
		return reflect.Value{}
	}

	statusField := objValue.FieldByName("Status")
	if !statusField.IsValid() {
		return reflect.Value{}
	}

	// If Status is a pointer, dereference it
	if statusField.Kind() == reflect.Ptr {
		if statusField.IsNil() {
			return reflect.Value{}
		}
		statusField = statusField.Elem()
	}

	return statusField
}

// ObservedGenerationTracker provides auto-tracking capability that can be enabled
// on the GenericReconciler.
type ObservedGenerationTracker interface {
	// ShouldUpdateObservedGeneration returns true if the observed generation
	// should be updated after a successful sync.
	ShouldUpdateObservedGeneration() bool

	// UpdateObservedGeneration updates the observed generation on the object.
	// Returns true if the object supports observed generation tracking.
	UpdateObservedGeneration(obj client.Object) bool
}

// autoObservedGenerationTracker implements automatic observed generation tracking.
type autoObservedGenerationTracker struct {
	enabled bool
}

func (t *autoObservedGenerationTracker) ShouldUpdateObservedGeneration() bool {
	return t.enabled
}

func (t *autoObservedGenerationTracker) UpdateObservedGeneration(obj client.Object) bool {
	if owog, ok := obj.(ObjectWithObservedGeneration); ok {
		owog.SetObservedGeneration(obj.GetGeneration())
		return true
	}
	return false
}
