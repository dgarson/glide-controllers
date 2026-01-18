package streamline

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Standard condition types that can be used across resources.
const (
	// ConditionTypeReady indicates the resource is ready to serve requests.
	ConditionTypeReady = "Ready"

	// ConditionTypeProgressing indicates the resource is making progress toward Ready.
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates the resource is operating in a degraded state.
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeAvailable indicates the resource is available for use.
	ConditionTypeAvailable = "Available"

	// ConditionTypeReconciled indicates the resource has been successfully reconciled.
	ConditionTypeReconciled = "Reconciled"
)

// Standard condition reasons.
const (
	// ReasonReconciling indicates reconciliation is in progress.
	ReasonReconciling = "Reconciling"

	// ReasonReconcileSuccess indicates reconciliation completed successfully.
	ReasonReconcileSuccess = "ReconcileSuccess"

	// ReasonReconcileFailed indicates reconciliation failed.
	ReasonReconcileFailed = "ReconcileFailed"

	// ReasonPending indicates the resource is pending some action.
	ReasonPending = "Pending"

	// ReasonReady indicates the resource is ready.
	ReasonReady = "Ready"

	// ReasonNotReady indicates the resource is not ready.
	ReasonNotReady = "NotReady"

	// ReasonError indicates an error occurred.
	ReasonError = "Error"

	// ReasonDependencyNotReady indicates a dependency is not ready.
	ReasonDependencyNotReady = "DependencyNotReady"

	// ReasonResourceNotFound indicates a required resource was not found.
	ReasonResourceNotFound = "ResourceNotFound"

	// ReasonInvalidSpec indicates the spec is invalid.
	ReasonInvalidSpec = "InvalidSpec"
)

// ConditionHelper provides utilities for managing standard Kubernetes conditions.
// It ensures proper lastTransitionTime handling and consistent condition management.
type ConditionHelper interface {
	// Set updates or adds a condition with the given type, status, reason, and message.
	// The lastTransitionTime is only updated if the status has changed.
	Set(conditionType string, status metav1.ConditionStatus, reason, message string)

	// SetWithObservedGeneration sets a condition and records the current generation.
	// This is useful for tracking which generation a condition applies to.
	SetWithObservedGeneration(conditionType string, status metav1.ConditionStatus, reason, message string)

	// SetTrue sets the condition to True with the given reason and message.
	SetTrue(conditionType, reason, message string)

	// SetFalse sets the condition to False with the given reason and message.
	SetFalse(conditionType, reason, message string)

	// SetUnknown sets the condition to Unknown with the given reason and message.
	SetUnknown(conditionType, reason, message string)

	// SetFromError sets a condition to False based on an error.
	// If err is nil, the condition is set to True with the success reason/message.
	// If err is non-nil, the condition is set to False with the error reason and error message.
	SetFromError(conditionType string, err error, successReason, successMessage string)

	// SetReady is a convenience method for setting the Ready condition to True.
	SetReady(reason, message string)

	// SetNotReady is a convenience method for setting the Ready condition to False.
	SetNotReady(reason, message string)

	// SetProgressing sets the Progressing condition to True.
	SetProgressing(reason, message string)

	// ClearProgressing sets the Progressing condition to False.
	ClearProgressing(reason, message string)

	// SetDegraded sets the Degraded condition to True.
	SetDegraded(reason, message string)

	// ClearDegraded sets the Degraded condition to False.
	ClearDegraded(reason, message string)

	// Get retrieves a condition by type. Returns nil if not found.
	Get(conditionType string) *metav1.Condition

	// IsTrue returns true if the condition exists and has status True.
	IsTrue(conditionType string) bool

	// IsFalse returns true if the condition exists and has status False.
	IsFalse(conditionType string) bool

	// IsUnknown returns true if the condition doesn't exist or has status Unknown.
	IsUnknown(conditionType string) bool

	// Remove deletes a condition by type.
	Remove(conditionType string)

	// GetAll returns all conditions.
	GetAll() []metav1.Condition

	// Count returns the number of conditions.
	Count() int

	// HasCondition returns true if a condition with the given type exists.
	HasCondition(conditionType string) bool

	// LastTransitionTime returns the last transition time for a condition,
	// or zero time if the condition doesn't exist.
	LastTransitionTime(conditionType string) metav1.Time

	// IsStale returns true if the condition's observedGeneration (if present)
	// is less than the object's current generation.
	IsStale(conditionType string) bool
}

// conditionHelper implements ConditionHelper for objects with conditions.
type conditionHelper struct {
	obj        client.Object
	conditions ObjectWithConditions
}

// newConditionHelper creates a new ConditionHelper for the given object.
// Returns nil if the object doesn't implement ObjectWithConditions.
func newConditionHelper(obj client.Object) ConditionHelper {
	if owc, ok := obj.(ObjectWithConditions); ok {
		return &conditionHelper{obj: obj, conditions: owc}
	}
	return &noopConditionHelper{}
}

func (c *conditionHelper) Set(conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	conditions := c.conditions.GetConditions()

	// Find existing condition
	var existing *metav1.Condition
	for i := range conditions {
		if conditions[i].Type == conditionType {
			existing = &conditions[i]
			break
		}
	}

	if existing != nil {
		// Update existing condition
		if existing.Status != status {
			existing.LastTransitionTime = now
		}
		existing.Status = status
		existing.Reason = reason
		existing.Message = message
		existing.ObservedGeneration = c.obj.GetGeneration()
	} else {
		// Add new condition
		conditions = append(conditions, metav1.Condition{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: c.obj.GetGeneration(),
		})
	}

	c.conditions.SetConditions(conditions)
}

func (c *conditionHelper) SetWithObservedGeneration(conditionType string, status metav1.ConditionStatus, reason, message string) {
	// This is the same as Set since we always include observedGeneration
	c.Set(conditionType, status, reason, message)
}

func (c *conditionHelper) SetTrue(conditionType, reason, message string) {
	c.Set(conditionType, metav1.ConditionTrue, reason, message)
}

func (c *conditionHelper) SetFalse(conditionType, reason, message string) {
	c.Set(conditionType, metav1.ConditionFalse, reason, message)
}

func (c *conditionHelper) SetUnknown(conditionType, reason, message string) {
	c.Set(conditionType, metav1.ConditionUnknown, reason, message)
}

func (c *conditionHelper) SetFromError(conditionType string, err error, successReason, successMessage string) {
	if err == nil {
		c.SetTrue(conditionType, successReason, successMessage)
	} else {
		c.SetFalse(conditionType, ReasonError, err.Error())
	}
}

func (c *conditionHelper) SetReady(reason, message string) {
	c.SetTrue(ConditionTypeReady, reason, message)
}

func (c *conditionHelper) SetNotReady(reason, message string) {
	c.SetFalse(ConditionTypeReady, reason, message)
}

func (c *conditionHelper) SetProgressing(reason, message string) {
	c.SetTrue(ConditionTypeProgressing, reason, message)
}

func (c *conditionHelper) ClearProgressing(reason, message string) {
	c.SetFalse(ConditionTypeProgressing, reason, message)
}

func (c *conditionHelper) SetDegraded(reason, message string) {
	c.SetTrue(ConditionTypeDegraded, reason, message)
}

func (c *conditionHelper) ClearDegraded(reason, message string) {
	c.SetFalse(ConditionTypeDegraded, reason, message)
}

func (c *conditionHelper) Get(conditionType string) *metav1.Condition {
	for i := range c.conditions.GetConditions() {
		if c.conditions.GetConditions()[i].Type == conditionType {
			return &c.conditions.GetConditions()[i]
		}
	}
	return nil
}

func (c *conditionHelper) IsTrue(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func (c *conditionHelper) IsFalse(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

func (c *conditionHelper) IsUnknown(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond == nil || cond.Status == metav1.ConditionUnknown
}

func (c *conditionHelper) Remove(conditionType string) {
	conditions := c.conditions.GetConditions()
	newConditions := make([]metav1.Condition, 0, len(conditions))
	for _, cond := range conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
		}
	}
	c.conditions.SetConditions(newConditions)
}

func (c *conditionHelper) GetAll() []metav1.Condition {
	return c.conditions.GetConditions()
}

func (c *conditionHelper) Count() int {
	return len(c.conditions.GetConditions())
}

func (c *conditionHelper) HasCondition(conditionType string) bool {
	return c.Get(conditionType) != nil
}

func (c *conditionHelper) LastTransitionTime(conditionType string) metav1.Time {
	cond := c.Get(conditionType)
	if cond == nil {
		return metav1.Time{}
	}
	return cond.LastTransitionTime
}

func (c *conditionHelper) IsStale(conditionType string) bool {
	cond := c.Get(conditionType)
	if cond == nil {
		return true // No condition means it's stale
	}
	return cond.ObservedGeneration < c.obj.GetGeneration()
}

// noopConditionHelper is used when the object doesn't implement ObjectWithConditions.
type noopConditionHelper struct{}

func (n *noopConditionHelper) Set(conditionType string, status metav1.ConditionStatus, reason, message string) {
}
func (n *noopConditionHelper) SetWithObservedGeneration(conditionType string, status metav1.ConditionStatus, reason, message string) {
}
func (n *noopConditionHelper) SetTrue(conditionType, reason, message string)              {}
func (n *noopConditionHelper) SetFalse(conditionType, reason, message string)             {}
func (n *noopConditionHelper) SetUnknown(conditionType, reason, message string)           {}
func (n *noopConditionHelper) SetFromError(conditionType string, err error, sr, sm string) {}
func (n *noopConditionHelper) SetReady(reason, message string)                             {}
func (n *noopConditionHelper) SetNotReady(reason, message string)                          {}
func (n *noopConditionHelper) SetProgressing(reason, message string)                       {}
func (n *noopConditionHelper) ClearProgressing(reason, message string)                     {}
func (n *noopConditionHelper) SetDegraded(reason, message string)                          {}
func (n *noopConditionHelper) ClearDegraded(reason, message string)                        {}
func (n *noopConditionHelper) Get(conditionType string) *metav1.Condition                  { return nil }
func (n *noopConditionHelper) IsTrue(conditionType string) bool                            { return false }
func (n *noopConditionHelper) IsFalse(conditionType string) bool                           { return false }
func (n *noopConditionHelper) IsUnknown(conditionType string) bool                         { return true }
func (n *noopConditionHelper) Remove(conditionType string)                                 {}
func (n *noopConditionHelper) GetAll() []metav1.Condition                                  { return nil }
func (n *noopConditionHelper) Count() int                                                  { return 0 }
func (n *noopConditionHelper) HasCondition(conditionType string) bool                      { return false }
func (n *noopConditionHelper) LastTransitionTime(conditionType string) metav1.Time {
	return metav1.Time{}
}
func (n *noopConditionHelper) IsStale(conditionType string) bool { return true }

// ConditionBuilder provides a fluent API for building conditions.
type ConditionBuilder struct {
	conditionType      string
	status             metav1.ConditionStatus
	reason             string
	message            string
	observedGeneration int64
}

// NewCondition creates a new ConditionBuilder for the given type.
func NewCondition(conditionType string) *ConditionBuilder {
	return &ConditionBuilder{
		conditionType: conditionType,
		status:        metav1.ConditionUnknown,
	}
}

// True sets the condition status to True.
func (b *ConditionBuilder) True() *ConditionBuilder {
	b.status = metav1.ConditionTrue
	return b
}

// False sets the condition status to False.
func (b *ConditionBuilder) False() *ConditionBuilder {
	b.status = metav1.ConditionFalse
	return b
}

// Unknown sets the condition status to Unknown.
func (b *ConditionBuilder) Unknown() *ConditionBuilder {
	b.status = metav1.ConditionUnknown
	return b
}

// Status sets the condition status.
func (b *ConditionBuilder) Status(status metav1.ConditionStatus) *ConditionBuilder {
	b.status = status
	return b
}

// Reason sets the condition reason.
func (b *ConditionBuilder) Reason(reason string) *ConditionBuilder {
	b.reason = reason
	return b
}

// Message sets the condition message.
func (b *ConditionBuilder) Message(message string) *ConditionBuilder {
	b.message = message
	return b
}

// Messagef sets the condition message with formatting.
func (b *ConditionBuilder) Messagef(format string, args ...interface{}) *ConditionBuilder {
	b.message = fmt.Sprintf(format, args...)
	return b
}

// ObservedGeneration sets the observed generation.
func (b *ConditionBuilder) ObservedGeneration(gen int64) *ConditionBuilder {
	b.observedGeneration = gen
	return b
}

// Build creates the metav1.Condition from the builder.
func (b *ConditionBuilder) Build() metav1.Condition {
	return metav1.Condition{
		Type:               b.conditionType,
		Status:             b.status,
		LastTransitionTime: metav1.Now(),
		Reason:             b.reason,
		Message:            b.message,
		ObservedGeneration: b.observedGeneration,
	}
}

// Apply applies the condition to the given ConditionHelper.
func (b *ConditionBuilder) Apply(helper ConditionHelper) {
	helper.Set(b.conditionType, b.status, b.reason, b.message)
}

// SummaryCondition creates a summary "Ready" condition based on other conditions.
// It returns True only if all specified condition types are True.
// If any are False, it returns False. If any are Unknown (and none False), it returns Unknown.
func SummaryCondition(helper ConditionHelper, requiredConditions ...string) metav1.Condition {
	allTrue := true
	anyFalse := false
	var falseReasons []string

	for _, condType := range requiredConditions {
		if helper.IsFalse(condType) {
			anyFalse = true
			cond := helper.Get(condType)
			if cond != nil {
				falseReasons = append(falseReasons, fmt.Sprintf("%s: %s", condType, cond.Reason))
			}
		} else if !helper.IsTrue(condType) {
			allTrue = false
		}
	}

	builder := NewCondition(ConditionTypeReady)

	if anyFalse {
		return builder.False().
			Reason(ReasonNotReady).
			Messagef("Not ready: %v", falseReasons).
			Build()
	}

	if allTrue {
		return builder.True().
			Reason(ReasonReady).
			Message("All conditions are satisfied").
			Build()
	}

	return builder.Unknown().
		Reason(ReasonReconciling).
		Message("Waiting for conditions to be satisfied").
		Build()
}

// TransitionTracker tracks condition transitions for metrics and logging.
type TransitionTracker struct {
	transitions []ConditionTransition
}

// ConditionTransition records a single condition state change.
type ConditionTransition struct {
	Type       string
	OldStatus  metav1.ConditionStatus
	NewStatus  metav1.ConditionStatus
	Reason     string
	Message    string
	Timestamp  time.Time
	Generation int64
}

// NewTransitionTracker creates a new TransitionTracker.
func NewTransitionTracker() *TransitionTracker {
	return &TransitionTracker{}
}

// Track records a transition if the status changed.
func (t *TransitionTracker) Track(condType string, oldCond, newCond *metav1.Condition, generation int64) {
	var oldStatus metav1.ConditionStatus
	if oldCond != nil {
		oldStatus = oldCond.Status
	} else {
		oldStatus = metav1.ConditionUnknown
	}

	newStatus := metav1.ConditionUnknown
	var reason, message string
	if newCond != nil {
		newStatus = newCond.Status
		reason = newCond.Reason
		message = newCond.Message
	}

	if oldStatus != newStatus {
		t.transitions = append(t.transitions, ConditionTransition{
			Type:       condType,
			OldStatus:  oldStatus,
			NewStatus:  newStatus,
			Reason:     reason,
			Message:    message,
			Timestamp:  time.Now(),
			Generation: generation,
		})
	}
}

// GetTransitions returns all recorded transitions.
func (t *TransitionTracker) GetTransitions() []ConditionTransition {
	return t.transitions
}

// HasTransitions returns true if any transitions were recorded.
func (t *TransitionTracker) HasTransitions() bool {
	return len(t.transitions) > 0
}

// Clear removes all recorded transitions.
func (t *TransitionTracker) Clear() {
	t.transitions = nil
}
