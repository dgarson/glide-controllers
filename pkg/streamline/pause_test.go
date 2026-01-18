package streamline

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockClientObject is a simple mock for testing pause functionality.
type mockClientObject struct {
	client.Object
	annotations map[string]string
	name        string
	namespace   string
	generation  int64
}

func (m *mockClientObject) GetAnnotations() map[string]string {
	return m.annotations
}

func (m *mockClientObject) SetAnnotations(annotations map[string]string) {
	m.annotations = annotations
}

func (m *mockClientObject) GetName() string {
	return m.name
}

func (m *mockClientObject) GetNamespace() string {
	return m.namespace
}

func (m *mockClientObject) GetGeneration() int64 {
	return m.generation
}

func newMockObject() *mockClientObject {
	return &mockClientObject{
		name:       "test",
		namespace:  "default",
		generation: 1,
	}
}

func newMockObjectWithAnnotations(annotations map[string]string) *mockClientObject {
	return &mockClientObject{
		annotations: annotations,
		name:        "test",
		namespace:   "default",
		generation:  1,
	}
}

func TestPauseChecker_IsPaused(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{"no annotations", nil, false},
		{"empty annotations", map[string]string{}, false},
		{"paused false", map[string]string{AnnotationPaused: "false"}, false},
		{"paused true", map[string]string{AnnotationPaused: "true"}, true},
		{"paused invalid", map[string]string{AnnotationPaused: "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			if got := pc.IsPaused(); got != tt.expected {
				t.Errorf("IsPaused() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPauseChecker_PausedAt(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	formatted := now.Format(time.RFC3339)

	tests := []struct {
		name        string
		annotations map[string]string
		expectZero  bool
	}{
		{"no annotations", nil, true},
		{"no paused-at", map[string]string{}, true},
		{"invalid time", map[string]string{AnnotationPausedAt: "invalid"}, true},
		{"valid time", map[string]string{AnnotationPausedAt: formatted}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			got := pc.PausedAt()
			if tt.expectZero && !got.IsZero() {
				t.Errorf("PausedAt() = %v, want zero", got)
			}
			if !tt.expectZero && got.IsZero() {
				t.Error("PausedAt() is zero, want non-zero")
			}
		})
	}
}

func TestPauseChecker_PausedBy(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{"no annotations", nil, ""},
		{"no paused-by", map[string]string{}, ""},
		{"has paused-by", map[string]string{AnnotationPausedBy: "admin"}, "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			if got := pc.PausedBy(); got != tt.expected {
				t.Errorf("PausedBy() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPauseChecker_PausedReason(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{"no annotations", nil, ""},
		{"no reason", map[string]string{}, ""},
		{"has reason", map[string]string{AnnotationPausedReason: "maintenance"}, "maintenance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			if got := pc.PausedReason(); got != tt.expected {
				t.Errorf("PausedReason() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPauseChecker_ResumeAt(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	formatted := future.Format(time.RFC3339)

	tests := []struct {
		name        string
		annotations map[string]string
		expectZero  bool
	}{
		{"no annotations", nil, true},
		{"no resume-at", map[string]string{}, true},
		{"invalid time", map[string]string{AnnotationResumeAt: "invalid"}, true},
		{"valid time", map[string]string{AnnotationResumeAt: formatted}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			got := pc.ResumeAt()
			if tt.expectZero && !got.IsZero() {
				t.Errorf("ResumeAt() = %v, want zero", got)
			}
			if !tt.expectZero && got.IsZero() {
				t.Error("ResumeAt() is zero, want non-zero")
			}
		})
	}
}

func TestPauseChecker_ShouldResume(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{"no resume time", nil, false},
		{"resume time in past", map[string]string{AnnotationResumeAt: past}, true},
		{"resume time in future", map[string]string{AnnotationResumeAt: future}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			if got := pc.ShouldResume(); got != tt.expected {
				t.Errorf("ShouldResume() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPauseChecker_TimeUntilResume(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name        string
		annotations map[string]string
		expectZero  bool
	}{
		{"no resume time", nil, true},
		{"resume time in past", map[string]string{AnnotationResumeAt: past}, true},
		{"resume time in future", map[string]string{AnnotationResumeAt: future}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newMockObjectWithAnnotations(tt.annotations)
			pc := NewPauseChecker(obj)
			got := pc.TimeUntilResume()
			if tt.expectZero && got != 0 {
				t.Errorf("TimeUntilResume() = %v, want 0", got)
			}
			if !tt.expectZero && got == 0 {
				t.Error("TimeUntilResume() = 0, want > 0")
			}
		})
	}
}

func TestPauseChecker_Pause(t *testing.T) {
	obj := newMockObject()
	pc := NewPauseChecker(obj)

	pc.Pause("admin", "maintenance window")

	if !pc.IsPaused() {
		t.Error("Should be paused after Pause()")
	}
	if pc.PausedBy() != "admin" {
		t.Errorf("PausedBy() = %v, want admin", pc.PausedBy())
	}
	if pc.PausedReason() != "maintenance window" {
		t.Errorf("PausedReason() = %v, want maintenance window", pc.PausedReason())
	}
	if pc.PausedAt().IsZero() {
		t.Error("PausedAt() should not be zero after Pause()")
	}
}

func TestPauseChecker_Pause_EmptyValues(t *testing.T) {
	obj := newMockObject()
	pc := NewPauseChecker(obj)

	pc.Pause("", "")

	if !pc.IsPaused() {
		t.Error("Should be paused after Pause() even with empty values")
	}
	// Empty values should not set the annotations
	if pc.PausedBy() != "" {
		t.Error("PausedBy should be empty when not provided")
	}
}

func TestPauseChecker_Resume(t *testing.T) {
	obj := newMockObjectWithAnnotations(map[string]string{
		AnnotationPaused:       "true",
		AnnotationPausedAt:     time.Now().Format(time.RFC3339),
		AnnotationPausedBy:     "admin",
		AnnotationPausedReason: "maintenance",
		AnnotationResumeAt:     time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	pc := NewPauseChecker(obj)

	pc.Resume()

	if pc.IsPaused() {
		t.Error("Should not be paused after Resume()")
	}
	if pc.PausedBy() != "" {
		t.Error("PausedBy should be cleared")
	}
	if pc.PausedReason() != "" {
		t.Error("PausedReason should be cleared")
	}
	if !pc.ResumeAt().IsZero() {
		t.Error("ResumeAt should be cleared")
	}
}

func TestPauseChecker_Resume_NilAnnotations(t *testing.T) {
	obj := newMockObject()
	pc := NewPauseChecker(obj)

	// Should not panic
	pc.Resume()
}

func TestPauseChecker_SetAutoResume(t *testing.T) {
	obj := newMockObject()
	pc := NewPauseChecker(obj)

	future := time.Now().Add(1 * time.Hour)
	pc.SetAutoResume(future)

	resumeAt := pc.ResumeAt()
	if resumeAt.IsZero() {
		t.Error("ResumeAt should not be zero after SetAutoResume()")
	}
}

func TestPauseChecker_ClearAutoResume(t *testing.T) {
	obj := newMockObjectWithAnnotations(map[string]string{
		AnnotationResumeAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	pc := NewPauseChecker(obj)

	pc.ClearAutoResume()

	if !pc.ResumeAt().IsZero() {
		t.Error("ResumeAt should be zero after ClearAutoResume()")
	}
}

func TestPauseChecker_ClearAutoResume_NilAnnotations(t *testing.T) {
	obj := newMockObject()
	pc := NewPauseChecker(obj)

	// Should not panic
	pc.ClearAutoResume()
}

func TestIsPaused_Convenience(t *testing.T) {
	pausedObj := newMockObjectWithAnnotations(map[string]string{AnnotationPaused: "true"})
	notPausedObj := newMockObject()

	if !IsPaused(pausedObj) {
		t.Error("IsPaused should return true for paused object")
	}
	if IsPaused(notPausedObj) {
		t.Error("IsPaused should return false for non-paused object")
	}
}

func TestGetPausedReason_Convenience(t *testing.T) {
	obj := newMockObjectWithAnnotations(map[string]string{
		AnnotationPausedReason: "test reason",
	})

	if got := GetPausedReason(obj); got != "test reason" {
		t.Errorf("GetPausedReason() = %v, want 'test reason'", got)
	}
}

func TestGetPauseInfo(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)

	obj := newMockObjectWithAnnotations(map[string]string{
		AnnotationPaused:       "true",
		AnnotationPausedAt:     now.Format(time.RFC3339),
		AnnotationPausedBy:     "admin",
		AnnotationPausedReason: "maintenance",
		AnnotationResumeAt:     future.Format(time.RFC3339),
	})

	info := GetPauseInfo(obj)

	if !info.IsPaused {
		t.Error("IsPaused should be true")
	}
	if info.PausedBy != "admin" {
		t.Errorf("PausedBy = %v, want admin", info.PausedBy)
	}
	if info.Reason != "maintenance" {
		t.Errorf("Reason = %v, want maintenance", info.Reason)
	}
	if info.TimeUntilResume == 0 {
		t.Error("TimeUntilResume should be > 0")
	}
}

// mockObjectWithConditions implements ObjectWithConditions for testing.
type mockObjectWithConditions struct {
	*mockClientObject
	conditions []metav1.Condition
}

func (m *mockObjectWithConditions) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockObjectWithConditions) SetConditions(conditions []metav1.Condition) {
	m.conditions = conditions
}

func TestSetPausedCondition(t *testing.T) {
	t.Run("sets paused true", func(t *testing.T) {
		obj := &mockObjectWithConditions{
			mockClientObject: newMockObject(),
		}

		SetPausedCondition(obj, true, ReasonPaused, "Test pause message")

		conditions := obj.GetConditions()
		if len(conditions) != 1 {
			t.Fatalf("Expected 1 condition, got %d", len(conditions))
		}

		cond := conditions[0]
		if cond.Type != ConditionTypePaused {
			t.Errorf("Type = %v, want %v", cond.Type, ConditionTypePaused)
		}
		if cond.Status != metav1.ConditionTrue {
			t.Errorf("Status = %v, want True", cond.Status)
		}
		if cond.Reason != ReasonPaused {
			t.Errorf("Reason = %v, want %v", cond.Reason, ReasonPaused)
		}
	})

	t.Run("sets paused false", func(t *testing.T) {
		obj := &mockObjectWithConditions{
			mockClientObject: newMockObject(),
			conditions: []metav1.Condition{
				{
					Type:               ConditionTypePaused,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             ReasonPaused,
					Message:            "Paused",
				},
			},
		}

		SetPausedCondition(obj, false, ReasonResumed, "Resumed")

		conditions := obj.GetConditions()
		cond := conditions[0]
		if cond.Status != metav1.ConditionFalse {
			t.Errorf("Status = %v, want False", cond.Status)
		}
	})

	t.Run("no-op for non-condition object", func(t *testing.T) {
		obj := newMockObject()
		// Should not panic
		SetPausedCondition(obj, true, ReasonPaused, "message")
	})
}

func TestCheckPauseAndSkip(t *testing.T) {
	t.Run("not paused", func(t *testing.T) {
		obj := newMockObject()
		result := CheckPauseAndSkip(obj)

		if result != nil {
			t.Error("Should return nil for non-paused object")
		}
	})

	t.Run("paused", func(t *testing.T) {
		obj := newMockObjectWithAnnotations(map[string]string{
			AnnotationPaused: "true",
		})
		result := CheckPauseAndSkip(obj)

		if result == nil {
			t.Fatal("Should return result for paused object")
		}
		if !result.Skipped {
			t.Error("Skipped should be true")
		}
	})

	t.Run("paused with auto-resume passed", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		obj := newMockObjectWithAnnotations(map[string]string{
			AnnotationPaused:   "true",
			AnnotationResumeAt: past,
		})
		result := CheckPauseAndSkip(obj)

		if result != nil {
			t.Error("Should return nil when auto-resume time has passed")
		}
	})

	t.Run("paused with auto-resume future", func(t *testing.T) {
		future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		obj := newMockObjectWithAnnotations(map[string]string{
			AnnotationPaused:   "true",
			AnnotationResumeAt: future,
		})
		result := CheckPauseAndSkip(obj)

		if result == nil {
			t.Fatal("Should return result for paused object with future auto-resume")
		}
		if result.Result.RequeueAfter == 0 {
			t.Error("Should have RequeueAfter set for auto-resume")
		}
	})
}

func TestMaintenanceWindow(t *testing.T) {
	t.Run("active window", func(t *testing.T) {
		window := MaintenanceWindow{
			Start:  time.Now().Add(-1 * time.Hour),
			End:    time.Now().Add(1 * time.Hour),
			Reason: "Test maintenance",
		}

		if !window.IsActive() {
			t.Error("Window should be active")
		}
		if window.TimeUntilEnd() == 0 {
			t.Error("TimeUntilEnd should be > 0")
		}
	})

	t.Run("past window", func(t *testing.T) {
		window := MaintenanceWindow{
			Start:  time.Now().Add(-2 * time.Hour),
			End:    time.Now().Add(-1 * time.Hour),
			Reason: "Past maintenance",
		}

		if window.IsActive() {
			t.Error("Window should not be active")
		}
		if window.TimeUntilEnd() != 0 {
			t.Error("TimeUntilEnd should be 0 for inactive window")
		}
	})

	t.Run("future window", func(t *testing.T) {
		window := MaintenanceWindow{
			Start:  time.Now().Add(1 * time.Hour),
			End:    time.Now().Add(2 * time.Hour),
			Reason: "Future maintenance",
		}

		if window.IsActive() {
			t.Error("Window should not be active")
		}
	})
}

func TestPauseForMaintenance(t *testing.T) {
	obj := newMockObject()
	window := MaintenanceWindow{
		Start:  time.Now(),
		End:    time.Now().Add(1 * time.Hour),
		Reason: "Planned maintenance",
	}

	PauseForMaintenance(obj, window)

	pc := NewPauseChecker(obj)
	if !pc.IsPaused() {
		t.Error("Should be paused after PauseForMaintenance")
	}
	if pc.PausedBy() != "maintenance" {
		t.Errorf("PausedBy = %v, want maintenance", pc.PausedBy())
	}
	if pc.PausedReason() != "Planned maintenance" {
		t.Errorf("PausedReason = %v", pc.PausedReason())
	}
	if pc.ResumeAt().IsZero() {
		t.Error("ResumeAt should be set")
	}
}
