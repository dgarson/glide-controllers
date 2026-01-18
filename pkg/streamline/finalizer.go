package streamline

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupTask represents a single cleanup operation during finalization.
// Tasks can be ordered and tracked for progress reporting.
type CleanupTask struct {
	// Name is a unique identifier for this task.
	Name string

	// Description is a human-readable description of what this task does.
	Description string

	// Order determines the execution order. Lower numbers run first.
	// Tasks with the same order may run in any order (or potentially in parallel).
	Order int

	// Run executes the cleanup task.
	// Return nil when the task is complete.
	// Return an error to indicate failure (will be retried).
	// The task should be idempotent.
	Run func(ctx context.Context, sCtx *Context) error
}

// CleanupPlan represents an ordered set of cleanup tasks for finalization.
type CleanupPlan struct {
	tasks []CleanupTask
}

// NewCleanupPlan creates a new cleanup plan.
func NewCleanupPlan() *CleanupPlan {
	return &CleanupPlan{
		tasks: make([]CleanupTask, 0),
	}
}

// AddTask adds a cleanup task to the plan.
func (p *CleanupPlan) AddTask(task CleanupTask) *CleanupPlan {
	p.tasks = append(p.tasks, task)
	return p
}

// Add is a convenience method to add a task with just a name and function.
func (p *CleanupPlan) Add(name string, order int, run func(ctx context.Context, sCtx *Context) error) *CleanupPlan {
	return p.AddTask(CleanupTask{
		Name:  name,
		Order: order,
		Run:   run,
	})
}

// Execute runs all cleanup tasks in order.
// Returns the first error encountered, or nil if all tasks complete successfully.
func (p *CleanupPlan) Execute(ctx context.Context, sCtx *Context) error {
	// Sort tasks by order
	tasks := p.sortedTasks()

	for _, task := range tasks {
		sCtx.Log.V(1).Info("executing cleanup task", "task", task.Name)
		if err := task.Run(ctx, sCtx); err != nil {
			sCtx.Log.Error(err, "cleanup task failed", "task", task.Name)
			return fmt.Errorf("cleanup task %s failed: %w", task.Name, err)
		}
		sCtx.Log.V(1).Info("cleanup task completed", "task", task.Name)
	}

	return nil
}

// sortedTasks returns tasks sorted by order.
func (p *CleanupPlan) sortedTasks() []CleanupTask {
	// Simple insertion sort since we expect small number of tasks
	result := make([]CleanupTask, len(p.tasks))
	copy(result, p.tasks)

	for i := 1; i < len(result); i++ {
		key := result[i]
		j := i - 1
		for j >= 0 && result[j].Order > key.Order {
			result[j+1] = result[j]
			j--
		}
		result[j+1] = key
	}

	return result
}

// TaskCount returns the number of tasks in the plan.
func (p *CleanupPlan) TaskCount() int {
	return len(p.tasks)
}

// CleanupProgress tracks the progress of multi-step finalization.
type CleanupProgress struct {
	// TotalTasks is the total number of cleanup tasks.
	TotalTasks int `json:"totalTasks"`

	// CompletedTasks is the number of completed tasks.
	CompletedTasks int `json:"completedTasks"`

	// CurrentTask is the name of the currently executing task.
	CurrentTask string `json:"currentTask,omitempty"`

	// StartedAt is when cleanup started.
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// LastUpdated is when progress was last updated.
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// FailedTask is the name of the task that failed, if any.
	FailedTask string `json:"failedTask,omitempty"`

	// FailureMessage contains the error message if a task failed.
	FailureMessage string `json:"failureMessage,omitempty"`
}

// IsComplete returns true if all tasks are complete.
func (p *CleanupProgress) IsComplete() bool {
	return p.CompletedTasks >= p.TotalTasks && p.TotalTasks > 0
}

// PercentComplete returns the completion percentage (0-100).
func (p *CleanupProgress) PercentComplete() int {
	if p.TotalTasks == 0 {
		return 0
	}
	return (p.CompletedTasks * 100) / p.TotalTasks
}

// ObjectWithCleanupProgress is an interface for objects that track cleanup progress.
type ObjectWithCleanupProgress interface {
	client.Object

	// GetCleanupProgress returns the current cleanup progress.
	GetCleanupProgress() *CleanupProgress

	// SetCleanupProgress sets the cleanup progress.
	SetCleanupProgress(progress *CleanupProgress)
}

// TrackedCleanupPlan wraps a CleanupPlan with progress tracking.
type TrackedCleanupPlan struct {
	plan *CleanupPlan
	obj  ObjectWithCleanupProgress
}

// NewTrackedCleanupPlan creates a cleanup plan with progress tracking.
func NewTrackedCleanupPlan(obj ObjectWithCleanupProgress) *TrackedCleanupPlan {
	return &TrackedCleanupPlan{
		plan: NewCleanupPlan(),
		obj:  obj,
	}
}

// AddTask adds a task to the tracked plan.
func (t *TrackedCleanupPlan) AddTask(task CleanupTask) *TrackedCleanupPlan {
	t.plan.AddTask(task)
	return t
}

// Add is a convenience method to add a task.
func (t *TrackedCleanupPlan) Add(name string, order int, run func(ctx context.Context, sCtx *Context) error) *TrackedCleanupPlan {
	t.plan.Add(name, order, run)
	return t
}

// Execute runs all cleanup tasks with progress tracking.
func (t *TrackedCleanupPlan) Execute(ctx context.Context, sCtx *Context) error {
	tasks := t.plan.sortedTasks()

	// Initialize progress if needed
	progress := t.obj.GetCleanupProgress()
	if progress == nil || progress.TotalTasks == 0 {
		now := metav1.Now()
		progress = &CleanupProgress{
			TotalTasks:     len(tasks),
			CompletedTasks: 0,
			StartedAt:      &now,
			LastUpdated:    &now,
		}
		t.obj.SetCleanupProgress(progress)
	}

	// Skip already completed tasks
	startIdx := progress.CompletedTasks
	if startIdx >= len(tasks) {
		return nil // Already complete
	}

	for i := startIdx; i < len(tasks); i++ {
		task := tasks[i]

		// Update current task
		now := metav1.Now()
		progress.CurrentTask = task.Name
		progress.LastUpdated = &now
		t.obj.SetCleanupProgress(progress)

		sCtx.Log.V(1).Info("executing cleanup task",
			"task", task.Name,
			"progress", fmt.Sprintf("%d/%d", i+1, len(tasks)))

		if err := task.Run(ctx, sCtx); err != nil {
			// Record failure
			progress.FailedTask = task.Name
			progress.FailureMessage = err.Error()
			progress.LastUpdated = &now
			t.obj.SetCleanupProgress(progress)

			sCtx.Log.Error(err, "cleanup task failed", "task", task.Name)
			return fmt.Errorf("cleanup task %s failed: %w", task.Name, err)
		}

		// Mark task complete
		progress.CompletedTasks++
		progress.CurrentTask = ""
		progress.FailedTask = ""
		progress.FailureMessage = ""
		progress.LastUpdated = &now
		t.obj.SetCleanupProgress(progress)

		sCtx.Log.V(1).Info("cleanup task completed",
			"task", task.Name,
			"progress", fmt.Sprintf("%d/%d", progress.CompletedTasks, progress.TotalTasks))
	}

	return nil
}

// FinalizerSet manages multiple finalizers on an object.
// This is useful when multiple controllers need to perform cleanup.
type FinalizerSet struct {
	obj client.Object
}

// NewFinalizerSet creates a new finalizer set for the given object.
func NewFinalizerSet(obj client.Object) *FinalizerSet {
	return &FinalizerSet{obj: obj}
}

// Has returns true if the object has the specified finalizer.
func (fs *FinalizerSet) Has(finalizer string) bool {
	for _, f := range fs.obj.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// Add adds a finalizer if not already present.
// Returns true if the finalizer was added (object was modified).
func (fs *FinalizerSet) Add(finalizer string) bool {
	if fs.Has(finalizer) {
		return false
	}
	fs.obj.SetFinalizers(append(fs.obj.GetFinalizers(), finalizer))
	return true
}

// Remove removes a finalizer if present.
// Returns true if the finalizer was removed (object was modified).
func (fs *FinalizerSet) Remove(finalizer string) bool {
	finalizers := fs.obj.GetFinalizers()
	for i, f := range finalizers {
		if f == finalizer {
			fs.obj.SetFinalizers(append(finalizers[:i], finalizers[i+1:]...))
			return true
		}
	}
	return false
}

// List returns all finalizers on the object.
func (fs *FinalizerSet) List() []string {
	return fs.obj.GetFinalizers()
}

// Count returns the number of finalizers.
func (fs *FinalizerSet) Count() int {
	return len(fs.obj.GetFinalizers())
}

// Clear removes all finalizers from the object.
// Returns true if any finalizers were removed.
func (fs *FinalizerSet) Clear() bool {
	if len(fs.obj.GetFinalizers()) == 0 {
		return false
	}
	fs.obj.SetFinalizers(nil)
	return true
}

// CleanupTimeout represents a timeout configuration for cleanup operations.
type CleanupTimeout struct {
	// Duration is the maximum time allowed for cleanup.
	Duration time.Duration

	// GracePeriod is additional time after Duration before force-completing.
	GracePeriod time.Duration
}

// DefaultCleanupTimeout returns a reasonable default timeout.
func DefaultCleanupTimeout() CleanupTimeout {
	return CleanupTimeout{
		Duration:    5 * time.Minute,
		GracePeriod: 30 * time.Second,
	}
}

// IsExpired returns true if the cleanup has exceeded its timeout.
func (t CleanupTimeout) IsExpired(startTime *metav1.Time) bool {
	if startTime == nil {
		return false
	}
	deadline := startTime.Add(t.Duration)
	return time.Now().After(deadline)
}

// IsInGracePeriod returns true if cleanup is in the grace period.
func (t CleanupTimeout) IsInGracePeriod(startTime *metav1.Time) bool {
	if startTime == nil {
		return false
	}
	deadline := startTime.Add(t.Duration)
	graceDeadline := deadline.Add(t.GracePeriod)
	now := time.Now()
	return now.After(deadline) && now.Before(graceDeadline)
}

// TimeRemaining returns the time remaining before timeout.
func (t CleanupTimeout) TimeRemaining(startTime *metav1.Time) time.Duration {
	if startTime == nil {
		return t.Duration
	}
	deadline := startTime.Add(t.Duration)
	remaining := time.Until(deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// DependentCleanup represents cleanup that depends on external resources being deleted.
type DependentCleanup struct {
	// ResourceRef identifies the dependent resource.
	ResourceRef client.ObjectKey

	// Type is the GVK of the dependent resource.
	Type client.Object

	// CheckFunc is an optional function to check if cleanup is complete.
	// If nil, just checks if the resource exists.
	CheckFunc func(ctx context.Context, c client.Client, ref client.ObjectKey) (bool, error)
}

// WaitForDeletion creates a cleanup task that waits for a resource to be deleted.
func WaitForDeletion(name string, order int, ref client.ObjectKey, objType client.Object) CleanupTask {
	return CleanupTask{
		Name:        name,
		Description: fmt.Sprintf("Wait for %s/%s to be deleted", ref.Namespace, ref.Name),
		Order:       order,
		Run: func(ctx context.Context, sCtx *Context) error {
			// Create a new instance of the type to check
			obj := objType.DeepCopyObject().(client.Object)
			err := sCtx.Client.Get(ctx, ref, obj)
			if err != nil {
				if client.IgnoreNotFound(err) == nil {
					// Resource is gone, cleanup complete
					return nil
				}
				return err
			}
			// Resource still exists
			return fmt.Errorf("waiting for %s/%s to be deleted", ref.Namespace, ref.Name)
		},
	}
}

// DeleteResource creates a cleanup task that deletes a resource.
func DeleteResource(name string, order int, ref client.ObjectKey, objType client.Object) CleanupTask {
	return CleanupTask{
		Name:        name,
		Description: fmt.Sprintf("Delete %s/%s", ref.Namespace, ref.Name),
		Order:       order,
		Run: func(ctx context.Context, sCtx *Context) error {
			obj := objType.DeepCopyObject().(client.Object)
			obj.SetName(ref.Name)
			obj.SetNamespace(ref.Namespace)

			err := sCtx.Client.Delete(ctx, obj)
			if client.IgnoreNotFound(err) != nil {
				return err
			}
			return nil
		},
	}
}

// DeleteOwnedResources creates a cleanup task that deletes all owned resources of a type.
func DeleteOwnedResources(name string, order int, listType client.ObjectList) CleanupTask {
	return CleanupTask{
		Name:        name,
		Description: fmt.Sprintf("Delete all owned %T", listType),
		Order:       order,
		Run: func(ctx context.Context, sCtx *Context) error {
			return sCtx.Owned.DeleteAll(ctx, listType)
		},
	}
}
