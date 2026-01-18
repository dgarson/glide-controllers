package streamline

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

func TestCleanupPlan_Execute(t *testing.T) {
	executionOrder := []string{}

	plan := NewCleanupPlan().
		Add("task1", 1, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task1")
			return nil
		}).
		Add("task2", 2, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task2")
			return nil
		}).
		Add("task3", 3, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task3")
			return nil
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	expected := []string{"task1", "task2", "task3"}
	if len(executionOrder) != len(expected) {
		t.Fatalf("Expected %d tasks, got %d", len(expected), len(executionOrder))
	}

	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("Task %d: expected %s, got %s", i, name, executionOrder[i])
		}
	}
}

func TestCleanupPlan_ExecuteOrder(t *testing.T) {
	executionOrder := []string{}

	// Add tasks out of order
	plan := NewCleanupPlan().
		Add("third", 30, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "third")
			return nil
		}).
		Add("first", 10, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "first")
			return nil
		}).
		Add("second", 20, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "second")
			return nil
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	expected := []string{"first", "second", "third"}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("Task %d: expected %s, got %s", i, name, executionOrder[i])
		}
	}
}

func TestCleanupPlan_ExecuteError(t *testing.T) {
	expectedErr := errors.New("task failed")
	executionOrder := []string{}

	plan := NewCleanupPlan().
		Add("task1", 1, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task1")
			return nil
		}).
		Add("task2", 2, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task2")
			return expectedErr
		}).
		Add("task3", 3, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task3")
			return nil
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Should have executed task1 and task2, but not task3
	if len(executionOrder) != 2 {
		t.Errorf("Expected 2 tasks executed, got %d", len(executionOrder))
	}

	if executionOrder[0] != "task1" || executionOrder[1] != "task2" {
		t.Error("Unexpected execution order")
	}
}

func TestCleanupPlan_TaskCount(t *testing.T) {
	plan := NewCleanupPlan()
	if plan.TaskCount() != 0 {
		t.Errorf("Expected 0 tasks, got %d", plan.TaskCount())
	}

	plan.Add("task1", 1, func(ctx context.Context, sCtx *Context) error { return nil })
	if plan.TaskCount() != 1 {
		t.Errorf("Expected 1 task, got %d", plan.TaskCount())
	}

	plan.Add("task2", 2, func(ctx context.Context, sCtx *Context) error { return nil })
	if plan.TaskCount() != 2 {
		t.Errorf("Expected 2 tasks, got %d", plan.TaskCount())
	}
}

func TestCleanupProgress_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress CleanupProgress
		want     bool
	}{
		{
			name:     "complete",
			progress: CleanupProgress{TotalTasks: 3, CompletedTasks: 3},
			want:     true,
		},
		{
			name:     "incomplete",
			progress: CleanupProgress{TotalTasks: 3, CompletedTasks: 2},
			want:     false,
		},
		{
			name:     "zero total",
			progress: CleanupProgress{TotalTasks: 0, CompletedTasks: 0},
			want:     false,
		},
		{
			name:     "more than total",
			progress: CleanupProgress{TotalTasks: 3, CompletedTasks: 5},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.progress.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupProgress_PercentComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress CleanupProgress
		want     int
	}{
		{
			name:     "zero",
			progress: CleanupProgress{TotalTasks: 3, CompletedTasks: 0},
			want:     0,
		},
		{
			name:     "half",
			progress: CleanupProgress{TotalTasks: 4, CompletedTasks: 2},
			want:     50,
		},
		{
			name:     "complete",
			progress: CleanupProgress{TotalTasks: 3, CompletedTasks: 3},
			want:     100,
		},
		{
			name:     "zero total",
			progress: CleanupProgress{TotalTasks: 0, CompletedTasks: 0},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.progress.PercentComplete(); got != tt.want {
				t.Errorf("PercentComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFinalizerSet(t *testing.T) {
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "default",
			Finalizers: []string{"existing-finalizer"},
		},
	}

	fs := NewFinalizerSet(obj)

	// Test Has
	if !fs.Has("existing-finalizer") {
		t.Error("Should have existing-finalizer")
	}
	if fs.Has("nonexistent") {
		t.Error("Should not have nonexistent")
	}

	// Test Add
	added := fs.Add("new-finalizer")
	if !added {
		t.Error("Should return true when adding new finalizer")
	}
	if !fs.Has("new-finalizer") {
		t.Error("Should have new-finalizer after Add")
	}

	// Test Add duplicate
	added = fs.Add("new-finalizer")
	if added {
		t.Error("Should return false when adding duplicate")
	}

	// Test Count
	if fs.Count() != 2 {
		t.Errorf("Expected 2 finalizers, got %d", fs.Count())
	}

	// Test List
	list := fs.List()
	if len(list) != 2 {
		t.Errorf("Expected 2 finalizers in list, got %d", len(list))
	}

	// Test Remove
	removed := fs.Remove("existing-finalizer")
	if !removed {
		t.Error("Should return true when removing existing finalizer")
	}
	if fs.Has("existing-finalizer") {
		t.Error("Should not have existing-finalizer after Remove")
	}

	// Test Remove nonexistent
	removed = fs.Remove("nonexistent")
	if removed {
		t.Error("Should return false when removing nonexistent")
	}

	// Test Clear
	fs.Add("another")
	cleared := fs.Clear()
	if !cleared {
		t.Error("Should return true when clearing finalizers")
	}
	if fs.Count() != 0 {
		t.Error("Should have no finalizers after Clear")
	}

	// Test Clear empty
	cleared = fs.Clear()
	if cleared {
		t.Error("Should return false when clearing already empty")
	}
}

func TestCleanupTimeout(t *testing.T) {
	timeout := CleanupTimeout{
		Duration:    5 * time.Minute,
		GracePeriod: 30 * time.Second,
	}

	// Test not expired
	recentStart := metav1.Now()
	if timeout.IsExpired(&recentStart) {
		t.Error("Should not be expired for recent start")
	}

	// Test expired
	oldStart := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	if !timeout.IsExpired(&oldStart) {
		t.Error("Should be expired for old start")
	}

	// Test nil start
	if timeout.IsExpired(nil) {
		t.Error("Should not be expired for nil start")
	}

	// Test TimeRemaining
	remaining := timeout.TimeRemaining(&recentStart)
	if remaining <= 0 || remaining > 5*time.Minute {
		t.Errorf("Unexpected time remaining: %v", remaining)
	}

	// Test TimeRemaining for expired
	remaining = timeout.TimeRemaining(&oldStart)
	if remaining != 0 {
		t.Errorf("Expected 0 remaining for expired, got %v", remaining)
	}
}

func TestDefaultCleanupTimeout(t *testing.T) {
	timeout := DefaultCleanupTimeout()

	if timeout.Duration != 5*time.Minute {
		t.Errorf("Expected 5 minute duration, got %v", timeout.Duration)
	}
	if timeout.GracePeriod != 30*time.Second {
		t.Errorf("Expected 30 second grace period, got %v", timeout.GracePeriod)
	}
}

// testObjectWithProgress implements ObjectWithCleanupProgress for testing.
type testObjectWithProgress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            testStatusWithProgress `json:"status,omitempty"`
}

type testStatusWithProgress struct {
	CleanupProgress *CleanupProgress `json:"cleanupProgress,omitempty"`
}

func (t *testObjectWithProgress) DeepCopyObject() runtime.Object {
	cp := &testObjectWithProgress{
		TypeMeta:   t.TypeMeta,
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
	}
	if t.Status.CleanupProgress != nil {
		cp.Status.CleanupProgress = &CleanupProgress{
			TotalTasks:     t.Status.CleanupProgress.TotalTasks,
			CompletedTasks: t.Status.CleanupProgress.CompletedTasks,
			CurrentTask:    t.Status.CleanupProgress.CurrentTask,
		}
	}
	return cp
}

func (t *testObjectWithProgress) GetCleanupProgress() *CleanupProgress {
	return t.Status.CleanupProgress
}

func (t *testObjectWithProgress) SetCleanupProgress(progress *CleanupProgress) {
	t.Status.CleanupProgress = progress
}

var _ ObjectWithCleanupProgress = &testObjectWithProgress{}

func TestTrackedCleanupPlan(t *testing.T) {
	obj := &testObjectWithProgress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	executionOrder := []string{}

	plan := NewTrackedCleanupPlan(obj).
		Add("task1", 1, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task1")
			return nil
		}).
		Add("task2", 2, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task2")
			return nil
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Check progress was updated
	progress := obj.GetCleanupProgress()
	if progress == nil {
		t.Fatal("Progress should be set")
	}
	if progress.TotalTasks != 2 {
		t.Errorf("Expected 2 total tasks, got %d", progress.TotalTasks)
	}
	if progress.CompletedTasks != 2 {
		t.Errorf("Expected 2 completed tasks, got %d", progress.CompletedTasks)
	}
	if !progress.IsComplete() {
		t.Error("Progress should be complete")
	}
}

func TestTrackedCleanupPlan_ResumeFromProgress(t *testing.T) {
	obj := &testObjectWithProgress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Status: testStatusWithProgress{
			CleanupProgress: &CleanupProgress{
				TotalTasks:     3,
				CompletedTasks: 1, // Already completed first task
			},
		},
	}

	executionOrder := []string{}

	plan := NewTrackedCleanupPlan(obj).
		Add("task1", 1, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task1")
			return nil
		}).
		Add("task2", 2, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task2")
			return nil
		}).
		Add("task3", 3, func(ctx context.Context, sCtx *Context) error {
			executionOrder = append(executionOrder, "task3")
			return nil
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Should have skipped task1 and only run task2 and task3
	if len(executionOrder) != 2 {
		t.Errorf("Expected 2 tasks executed, got %d: %v", len(executionOrder), executionOrder)
	}

	if executionOrder[0] != "task2" || executionOrder[1] != "task3" {
		t.Errorf("Expected task2, task3; got %v", executionOrder)
	}
}

func TestTrackedCleanupPlan_ErrorTracking(t *testing.T) {
	obj := &testObjectWithProgress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	expectedErr := errors.New("task failed")

	plan := NewTrackedCleanupPlan(obj).
		Add("task1", 1, func(ctx context.Context, sCtx *Context) error {
			return nil
		}).
		Add("task2", 2, func(ctx context.Context, sCtx *Context) error {
			return expectedErr
		})

	sCtx := &Context{Log: logr.Discard()}

	err := plan.Execute(context.Background(), sCtx)

	if err == nil {
		t.Error("Expected error")
	}

	progress := obj.GetCleanupProgress()
	if progress.CompletedTasks != 1 {
		t.Errorf("Expected 1 completed task, got %d", progress.CompletedTasks)
	}
	if progress.FailedTask != "task2" {
		t.Errorf("Expected failed task 'task2', got '%s'", progress.FailedTask)
	}
	if progress.FailureMessage != "task failed" {
		t.Errorf("Expected failure message 'task failed', got '%s'", progress.FailureMessage)
	}
}
