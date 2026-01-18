package streamline

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// setupTestScheme creates a scheme with core v1 types for testing.
func setupTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func TestDryRunResult_HasChanges(t *testing.T) {
	tests := []struct {
		name    string
		changes []DryRunChange
		want    bool
	}{
		{"no changes", nil, false},
		{"empty changes", []DryRunChange{}, false},
		{"has changes", []DryRunChange{{Action: DryRunActionCreate}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DryRunResult{Changes: tt.changes}
			if got := r.HasChanges(); got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDryRunResult_HasErrors(t *testing.T) {
	tests := []struct {
		name   string
		errors []error
		want   bool
	}{
		{"no errors", nil, false},
		{"empty errors", []error{}, false},
		{"has errors", []error{errors.New("test")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DryRunResult{Errors: tt.errors}
			if got := r.HasErrors(); got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDryRunResult_CountByAction(t *testing.T) {
	r := &DryRunResult{
		Changes: []DryRunChange{
			{Action: DryRunActionCreate},
			{Action: DryRunActionCreate},
			{Action: DryRunActionUpdate},
			{Action: DryRunActionDelete},
			{Action: DryRunActionSkip},
		},
	}

	if got := r.CountByAction(DryRunActionCreate); got != 2 {
		t.Errorf("CountByAction(Create) = %v, want 2", got)
	}
	if got := r.CountByAction(DryRunActionUpdate); got != 1 {
		t.Errorf("CountByAction(Update) = %v, want 1", got)
	}
	if got := r.CountByAction(DryRunActionDelete); got != 1 {
		t.Errorf("CountByAction(Delete) = %v, want 1", got)
	}
	if got := r.CountByAction(DryRunActionPatch); got != 0 {
		t.Errorf("CountByAction(Patch) = %v, want 0", got)
	}
}

func TestDryRunRecorder(t *testing.T) {
	scheme := setupTestScheme()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	t.Run("RecordCreate", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordCreate(pod, "Would create pod")

		result := recorder.GetResult()
		if len(result.Changes) != 1 {
			t.Fatalf("Expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Action != DryRunActionCreate {
			t.Errorf("Action = %v, want Create", result.Changes[0].Action)
		}
	})

	t.Run("RecordUpdate", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		newPod := pod.DeepCopy()
		newPod.Labels = map[string]string{"app": "test"}
		recorder.RecordUpdate(pod, newPod, "Would update pod")

		result := recorder.GetResult()
		if len(result.Changes) != 1 {
			t.Fatalf("Expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Action != DryRunActionUpdate {
			t.Errorf("Action = %v, want Update", result.Changes[0].Action)
		}
		if result.Changes[0].Before == nil {
			t.Error("Before should be set for update")
		}
	})

	t.Run("RecordDelete", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordDelete(pod, "Would delete pod")

		result := recorder.GetResult()
		if len(result.Changes) != 1 {
			t.Fatalf("Expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Action != DryRunActionDelete {
			t.Errorf("Action = %v, want Delete", result.Changes[0].Action)
		}
	})

	t.Run("RecordPatch", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		diff := &ResourceDiff{HasChanges: true, Summary: "test diff"}
		recorder.RecordPatch(pod, diff, "Would patch pod")

		result := recorder.GetResult()
		if len(result.Changes) != 1 {
			t.Fatalf("Expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Action != DryRunActionPatch {
			t.Errorf("Action = %v, want Patch", result.Changes[0].Action)
		}
		if result.Changes[0].Diff != diff {
			t.Error("Diff should be set")
		}
	})

	t.Run("RecordSkip", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordSkip(pod, "No changes needed")

		result := recorder.GetResult()
		if len(result.Changes) != 1 {
			t.Fatalf("Expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Action != DryRunActionSkip {
			t.Errorf("Action = %v, want Skip", result.Changes[0].Action)
		}
	})

	t.Run("RecordError", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordError(errors.New("test error"))

		result := recorder.GetResult()
		if len(result.Errors) != 1 {
			t.Fatalf("Expected 1 error, got %d", len(result.Errors))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordCreate(pod, "test")
		recorder.RecordError(errors.New("test"))
		recorder.Clear()

		result := recorder.GetResult()
		if len(result.Changes) != 0 {
			t.Error("Changes should be cleared")
		}
		if len(result.Errors) != 0 {
			t.Error("Errors should be cleared")
		}
	})

	t.Run("Summary generation", func(t *testing.T) {
		recorder := NewDryRunRecorder()
		recorder.RecordCreate(pod, "create")
		recorder.RecordCreate(pod, "create2")
		recorder.RecordUpdate(pod, pod, "update")
		recorder.RecordDelete(pod, "delete")
		recorder.RecordSkip(pod, "skip")
		recorder.RecordError(errors.New("error"))

		result := recorder.GetResult()
		if result.Summary == "" {
			t.Error("Summary should not be empty")
		}
		// Summary format: "2 creates, 1 updates, 1 deletes, 1 skipped, 1 errors"
		expected := "2 creates, 1 updates, 1 deletes, 1 skipped, 1 errors"
		if result.Summary != expected {
			t.Errorf("Summary = %v, want %v", result.Summary, expected)
		}
	})

	_ = scheme // Avoid unused variable
}

func TestDryRunClient(t *testing.T) {
	scheme := setupTestScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test", Image: "test:latest"},
			},
		},
	}

	t.Run("Create in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		dryClient := NewDryRunClient(c)

		err := dryClient.Create(context.Background(), pod.DeepCopy())
		if err != nil {
			t.Errorf("Create failed: %v", err)
		}

		result := dryClient.GetResult()
		if !result.HasChanges() {
			t.Error("Should have recorded change")
		}
		if result.CountByAction(DryRunActionCreate) != 1 {
			t.Error("Should have recorded create action")
		}

		// Verify pod was NOT actually created
		actual := &corev1.Pod{}
		err = c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-pod"}, actual)
		if err == nil {
			t.Error("Pod should not have been created in read-only mode")
		}
	})

	t.Run("Delete in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod.DeepCopy()).Build()
		dryClient := NewDryRunClient(c)

		err := dryClient.Delete(context.Background(), pod.DeepCopy())
		if err != nil {
			t.Errorf("Delete failed: %v", err)
		}

		result := dryClient.GetResult()
		if result.CountByAction(DryRunActionDelete) != 1 {
			t.Error("Should have recorded delete action")
		}

		// Verify pod was NOT actually deleted
		actual := &corev1.Pod{}
		err = c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-pod"}, actual)
		if err != nil {
			t.Error("Pod should not have been deleted in read-only mode")
		}
	})

	t.Run("Update in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod.DeepCopy()).Build()
		dryClient := NewDryRunClient(c)

		updatedPod := pod.DeepCopy()
		updatedPod.Labels = map[string]string{"app": "updated"}
		err := dryClient.Update(context.Background(), updatedPod)
		if err != nil {
			t.Errorf("Update failed: %v", err)
		}

		result := dryClient.GetResult()
		if result.CountByAction(DryRunActionUpdate) != 1 {
			t.Error("Should have recorded update action")
		}
	})

	t.Run("Patch in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod.DeepCopy()).Build()
		dryClient := NewDryRunClient(c)

		err := dryClient.Patch(context.Background(), pod.DeepCopy(), client.MergeFrom(pod))
		if err != nil {
			t.Errorf("Patch failed: %v", err)
		}

		result := dryClient.GetResult()
		if result.CountByAction(DryRunActionPatch) != 1 {
			t.Error("Should have recorded patch action")
		}
	})

	t.Run("AllowWrites enables actual writes", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		dryClient := NewDryRunClient(c).AllowWrites()

		err := dryClient.Create(context.Background(), pod.DeepCopy())
		if err != nil {
			t.Errorf("Create failed: %v", err)
		}

		// Verify pod WAS actually created
		actual := &corev1.Pod{}
		err = c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-pod"}, actual)
		if err != nil {
			t.Error("Pod should have been created with AllowWrites")
		}
	})

	t.Run("WithRecorder", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := NewDryRunRecorder()
		dryClient := NewDryRunClient(c).WithRecorder(recorder)

		if dryClient.GetRecorder() != recorder {
			t.Error("Should use custom recorder")
		}
	})

	t.Run("Status writer in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod.DeepCopy()).Build()
		dryClient := NewDryRunClient(c)

		updatedPod := pod.DeepCopy()
		err := dryClient.Status().Update(context.Background(), updatedPod)
		if err != nil {
			t.Errorf("Status update failed: %v", err)
		}

		result := dryClient.GetResult()
		if !result.HasChanges() {
			t.Error("Should have recorded status update")
		}
	})

	t.Run("Status Patch in read-only mode", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod.DeepCopy()).Build()
		dryClient := NewDryRunClient(c)

		err := dryClient.Status().Patch(context.Background(), pod.DeepCopy(), client.MergeFrom(pod))
		if err != nil {
			t.Errorf("Status patch failed: %v", err)
		}

		result := dryClient.GetResult()
		if result.CountByAction(DryRunActionPatch) != 1 {
			t.Error("Should have recorded status patch")
		}
	})
}

func TestIsDryRun(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{"no annotations", nil, false},
		{"empty annotations", map[string]string{}, false},
		{"dry-run false", map[string]string{AnnotationDryRun: "false"}, false},
		{"dry-run true", map[string]string{AnnotationDryRun: "true"}, true},
		{"dry-run invalid", map[string]string{AnnotationDryRun: "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			if got := IsDryRun(obj); got != tt.want {
				t.Errorf("IsDryRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetDryRun(t *testing.T) {
	t.Run("enable dry run", func(t *testing.T) {
		obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{}}
		SetDryRun(obj, true)

		if !IsDryRun(obj) {
			t.Error("Should be dry run after SetDryRun(true)")
		}
	})

	t.Run("disable dry run", func(t *testing.T) {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnotationDryRun:       "true",
					AnnotationDryRunResult: "1 creates",
				},
			},
		}
		SetDryRun(obj, false)

		if IsDryRun(obj) {
			t.Error("Should not be dry run after SetDryRun(false)")
		}
		if obj.Annotations[AnnotationDryRunResult] != "" {
			t.Error("Dry run result should be cleared")
		}
	})
}

func TestSetDryRunResult(t *testing.T) {
	obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{}}
	result := &DryRunResult{Summary: "1 creates, 0 updates, 0 deletes, 0 skipped, 0 errors"}

	SetDryRunResult(obj, result)

	if obj.Annotations[AnnotationDryRunResult] != result.Summary {
		t.Errorf("Result annotation = %v, want %v", obj.Annotations[AnnotationDryRunResult], result.Summary)
	}
}

func TestGetDryRunResult(t *testing.T) {
	t.Run("no annotations", func(t *testing.T) {
		obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{}}
		if got := GetDryRunResult(obj); got != "" {
			t.Errorf("GetDryRunResult() = %v, want empty", got)
		}
	})

	t.Run("has result", func(t *testing.T) {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnotationDryRunResult: "test result",
				},
			},
		}
		if got := GetDryRunResult(obj); got != "test result" {
			t.Errorf("GetDryRunResult() = %v, want 'test result'", got)
		}
	})
}

func TestDryRunActionConstants(t *testing.T) {
	// Verify constants have expected values
	if DryRunActionCreate != "create" {
		t.Errorf("DryRunActionCreate = %v", DryRunActionCreate)
	}
	if DryRunActionUpdate != "update" {
		t.Errorf("DryRunActionUpdate = %v", DryRunActionUpdate)
	}
	if DryRunActionDelete != "delete" {
		t.Errorf("DryRunActionDelete = %v", DryRunActionDelete)
	}
	if DryRunActionPatch != "patch" {
		t.Errorf("DryRunActionPatch = %v", DryRunActionPatch)
	}
	if DryRunActionSkip != "skip" {
		t.Errorf("DryRunActionSkip = %v", DryRunActionSkip)
	}
}
