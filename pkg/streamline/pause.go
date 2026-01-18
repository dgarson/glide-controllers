package streamline

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Pause-related annotations.
const (
	// AnnotationPaused indicates the resource reconciliation is paused.
	// Set to "true" to pause reconciliation.
	AnnotationPaused = "streamline.io/paused"

	// AnnotationPausedAt records when the resource was paused.
	AnnotationPausedAt = "streamline.io/paused-at"

	// AnnotationPausedBy records who or what paused the resource.
	AnnotationPausedBy = "streamline.io/paused-by"

	// AnnotationPausedReason records why the resource was paused.
	AnnotationPausedReason = "streamline.io/paused-reason"

	// AnnotationResumeAt records when the resource should automatically resume.
	AnnotationResumeAt = "streamline.io/resume-at"
)

// Pause condition constants.
const (
	// ConditionTypePaused indicates the resource is currently paused.
	ConditionTypePaused = "Paused"

	// ReasonPaused indicates the resource is paused.
	ReasonPaused = "Paused"

	// ReasonResumed indicates the resource was resumed.
	ReasonResumed = "Resumed"
)

// PauseChecker provides utilities for checking and managing resource pause state.
type PauseChecker interface {
	// IsPaused returns true if the resource is paused.
	IsPaused() bool

	// PausedAt returns when the resource was paused, or zero if not paused.
	PausedAt() time.Time

	// PausedBy returns who paused the resource, or empty if not paused.
	PausedBy() string

	// PausedReason returns why the resource was paused, or empty if not paused.
	PausedReason() string

	// ResumeAt returns when the resource should auto-resume, or zero if not set.
	ResumeAt() time.Time

	// ShouldResume returns true if the resource has an auto-resume time that has passed.
	ShouldResume() bool

	// TimeUntilResume returns the duration until auto-resume, or 0 if not set or already passed.
	TimeUntilResume() time.Duration

	// Pause marks the resource as paused.
	Pause(by, reason string)

	// Resume removes the paused annotation.
	Resume()

	// SetAutoResume sets an automatic resume time.
	SetAutoResume(at time.Time)

	// ClearAutoResume removes the automatic resume time.
	ClearAutoResume()
}

// pauseChecker implements PauseChecker.
type pauseChecker struct {
	obj client.Object
}

// NewPauseChecker creates a new PauseChecker for the given object.
func NewPauseChecker(obj client.Object) PauseChecker {
	return &pauseChecker{obj: obj}
}

func (pc *pauseChecker) IsPaused() bool {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationPaused] == "true"
}

func (pc *pauseChecker) PausedAt() time.Time {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, annotations[AnnotationPausedAt]); err == nil {
		return t
	}
	return time.Time{}
}

func (pc *pauseChecker) PausedBy() string {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return ""
	}
	return annotations[AnnotationPausedBy]
}

func (pc *pauseChecker) PausedReason() string {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return ""
	}
	return annotations[AnnotationPausedReason]
}

func (pc *pauseChecker) ResumeAt() time.Time {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, annotations[AnnotationResumeAt]); err == nil {
		return t
	}
	return time.Time{}
}

func (pc *pauseChecker) ShouldResume() bool {
	resumeAt := pc.ResumeAt()
	if resumeAt.IsZero() {
		return false
	}
	return time.Now().After(resumeAt)
}

func (pc *pauseChecker) TimeUntilResume() time.Duration {
	resumeAt := pc.ResumeAt()
	if resumeAt.IsZero() {
		return 0
	}
	remaining := time.Until(resumeAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (pc *pauseChecker) Pause(by, reason string) {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[AnnotationPaused] = "true"
	annotations[AnnotationPausedAt] = time.Now().Format(time.RFC3339)
	if by != "" {
		annotations[AnnotationPausedBy] = by
	}
	if reason != "" {
		annotations[AnnotationPausedReason] = reason
	}

	pc.obj.SetAnnotations(annotations)
}

func (pc *pauseChecker) Resume() {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return
	}

	delete(annotations, AnnotationPaused)
	delete(annotations, AnnotationPausedAt)
	delete(annotations, AnnotationPausedBy)
	delete(annotations, AnnotationPausedReason)
	delete(annotations, AnnotationResumeAt)

	pc.obj.SetAnnotations(annotations)
}

func (pc *pauseChecker) SetAutoResume(at time.Time) {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[AnnotationResumeAt] = at.Format(time.RFC3339)
	pc.obj.SetAnnotations(annotations)
}

func (pc *pauseChecker) ClearAutoResume() {
	annotations := pc.obj.GetAnnotations()
	if annotations == nil {
		return
	}

	delete(annotations, AnnotationResumeAt)
	pc.obj.SetAnnotations(annotations)
}

// IsPaused is a convenience function to check if an object is paused.
func IsPaused(obj client.Object) bool {
	return NewPauseChecker(obj).IsPaused()
}

// GetPausedReason is a convenience function to get the pause reason.
func GetPausedReason(obj client.Object) string {
	return NewPauseChecker(obj).PausedReason()
}

// PauseInfo contains information about a paused resource.
type PauseInfo struct {
	// IsPaused indicates whether the resource is paused.
	IsPaused bool

	// PausedAt is when the resource was paused.
	PausedAt time.Time

	// PausedBy is who paused the resource.
	PausedBy string

	// Reason is why the resource was paused.
	Reason string

	// ResumeAt is when the resource will auto-resume.
	ResumeAt time.Time

	// TimeUntilResume is the duration until auto-resume.
	TimeUntilResume time.Duration
}

// GetPauseInfo returns comprehensive pause information for an object.
func GetPauseInfo(obj client.Object) PauseInfo {
	pc := NewPauseChecker(obj)
	return PauseInfo{
		IsPaused:        pc.IsPaused(),
		PausedAt:        pc.PausedAt(),
		PausedBy:        pc.PausedBy(),
		Reason:          pc.PausedReason(),
		ResumeAt:        pc.ResumeAt(),
		TimeUntilResume: pc.TimeUntilResume(),
	}
}

// SetPausedCondition sets the Paused condition on an object with conditions.
func SetPausedCondition(obj client.Object, paused bool, reason, message string) {
	owc, ok := obj.(ObjectWithConditions)
	if !ok {
		return
	}

	conditions := owc.GetConditions()
	now := metav1.Now()

	// Find existing condition
	var existing *metav1.Condition
	for i := range conditions {
		if conditions[i].Type == ConditionTypePaused {
			existing = &conditions[i]
			break
		}
	}

	status := metav1.ConditionFalse
	if paused {
		status = metav1.ConditionTrue
	}

	if existing != nil {
		if existing.Status != status {
			existing.LastTransitionTime = now
		}
		existing.Status = status
		existing.Reason = reason
		existing.Message = message
		existing.ObservedGeneration = obj.GetGeneration()
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               ConditionTypePaused,
			Status:             status,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: obj.GetGeneration(),
		})
	}

	owc.SetConditions(conditions)
}

// PauseHandler provides behavior for pause-aware handlers.
type PauseHandler interface {
	// OnPause is called when reconciliation is skipped due to pause.
	// It can be used to set conditions or record events.
	OnPause(obj client.Object, sCtx *Context)

	// OnResume is called when a previously paused resource resumes.
	// It can be used to clear conditions or record events.
	OnResume(obj client.Object, sCtx *Context)
}

// DefaultPauseHandler provides default pause/resume behavior.
type DefaultPauseHandler struct{}

// OnPause sets the Paused condition and records an event.
func (h *DefaultPauseHandler) OnPause(obj client.Object, sCtx *Context) {
	info := GetPauseInfo(obj)

	message := "Reconciliation paused"
	if info.Reason != "" {
		message = "Reconciliation paused: " + info.Reason
	}
	if info.PausedBy != "" {
		message += " (by " + info.PausedBy + ")"
	}

	SetPausedCondition(obj, true, ReasonPaused, message)
	sCtx.Event.Normal("Paused", message)

	// Set progress condition
	sCtx.Conditions.SetFalse(ConditionTypeProgressing, ReasonPaused, "Reconciliation paused")
}

// OnResume clears the Paused condition and records an event.
func (h *DefaultPauseHandler) OnResume(obj client.Object, sCtx *Context) {
	SetPausedCondition(obj, false, ReasonResumed, "Reconciliation resumed")
	sCtx.Event.Normal("Resumed", "Reconciliation resumed")
}

// PauseReconcileResult is returned when reconciliation should be skipped due to pause.
// It includes the appropriate requeue behavior for auto-resume.
type PauseReconcileResult struct {
	// Skipped indicates reconciliation was skipped.
	Skipped bool

	// Result is the appropriate result to return.
	Result Result

	// Message describes why reconciliation was skipped.
	Message string
}

// CheckPauseAndSkip checks if an object is paused and returns the appropriate result.
// If the object has an auto-resume time, it will requeue accordingly.
func CheckPauseAndSkip(obj client.Object) *PauseReconcileResult {
	pc := NewPauseChecker(obj)

	if !pc.IsPaused() {
		return nil
	}

	// Check for auto-resume
	if pc.ShouldResume() {
		return nil // Resume reconciliation
	}

	result := &PauseReconcileResult{
		Skipped: true,
		Message: "reconciliation paused",
	}

	// Schedule requeue for auto-resume if set
	if remaining := pc.TimeUntilResume(); remaining > 0 {
		result.Result = RequeueAfter(remaining)
		result.Message = "reconciliation paused, will resume in " + remaining.String()
	} else {
		result.Result = Stop()
	}

	return result
}

// MaintenanceWindow represents a time window during which reconciliation is paused.
type MaintenanceWindow struct {
	// Start is when the maintenance window begins.
	Start time.Time

	// End is when the maintenance window ends.
	End time.Time

	// Reason describes the maintenance window.
	Reason string
}

// IsActive returns true if the current time is within the maintenance window.
func (mw *MaintenanceWindow) IsActive() bool {
	now := time.Now()
	return now.After(mw.Start) && now.Before(mw.End)
}

// TimeUntilEnd returns the duration until the maintenance window ends.
func (mw *MaintenanceWindow) TimeUntilEnd() time.Duration {
	if !mw.IsActive() {
		return 0
	}
	return time.Until(mw.End)
}

// PauseForMaintenance pauses an object for the duration of a maintenance window.
func PauseForMaintenance(obj client.Object, window MaintenanceWindow) {
	pc := NewPauseChecker(obj)
	pc.Pause("maintenance", window.Reason)
	pc.SetAutoResume(window.End)
}
