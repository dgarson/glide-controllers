package streamline

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestValidationBuilder_Required(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "name")

	vb.Required(path, "")
	if !vb.HasErrors() {
		t.Error("Should have error for empty string")
	}

	vb = NewValidationBuilder()
	vb.Required(path, "value")
	if vb.HasErrors() {
		t.Error("Should not have error for non-empty string")
	}
}

func TestValidationBuilder_RequiredString(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "name")

	vb.RequiredString(path, "")
	if !vb.HasErrors() {
		t.Error("Should have error for empty string")
	}

	vb = NewValidationBuilder()
	vb.RequiredString(path, "value")
	if vb.HasErrors() {
		t.Error("Should not have error for non-empty string")
	}
}

func TestValidationBuilder_RequiredInt(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "replicas")

	vb.RequiredInt(path, 0)
	if !vb.HasErrors() {
		t.Error("Should have error for zero int")
	}

	vb = NewValidationBuilder()
	vb.RequiredInt(path, 1)
	if vb.HasErrors() {
		t.Error("Should not have error for non-zero int")
	}
}

func TestValidationBuilder_NotNegative(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "count")

	vb.NotNegative(path, -1)
	if !vb.HasErrors() {
		t.Error("Should have error for negative value")
	}

	vb = NewValidationBuilder()
	vb.NotNegative(path, 0)
	if vb.HasErrors() {
		t.Error("Should not have error for zero")
	}
}

func TestValidationBuilder_Positive(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "count")

	vb.Positive(path, 0)
	if !vb.HasErrors() {
		t.Error("Should have error for zero")
	}

	vb.Positive(path, -1)
	// Already has error

	vb = NewValidationBuilder()
	vb.Positive(path, 1)
	if vb.HasErrors() {
		t.Error("Should not have error for positive value")
	}
}

func TestValidationBuilder_InRange(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "port")

	vb.InRange(path, 100, 1, 65535)
	if vb.HasErrors() {
		t.Error("100 should be in range 1-65535")
	}

	vb.InRange(path, 0, 1, 65535)
	if !vb.HasErrors() {
		t.Error("0 should not be in range 1-65535")
	}
}

func TestValidationBuilder_MinLength(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "name")

	vb.MinLength(path, "ab", 3)
	if !vb.HasErrors() {
		t.Error("'ab' should fail min length 3")
	}

	vb = NewValidationBuilder()
	vb.MinLength(path, "abc", 3)
	if vb.HasErrors() {
		t.Error("'abc' should pass min length 3")
	}
}

func TestValidationBuilder_MaxLength(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "name")

	vb.MaxLength(path, "abcd", 3)
	if !vb.HasErrors() {
		t.Error("'abcd' should fail max length 3")
	}

	vb = NewValidationBuilder()
	vb.MaxLength(path, "abc", 3)
	if vb.HasErrors() {
		t.Error("'abc' should pass max length 3")
	}
}

func TestValidationBuilder_MatchesRegex(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "name")

	vb.MatchesRegex(path, "test123", "^[a-z]+$")
	if !vb.HasErrors() {
		t.Error("'test123' should not match ^[a-z]+$")
	}

	vb = NewValidationBuilder()
	vb.MatchesRegex(path, "test", "^[a-z]+$")
	if vb.HasErrors() {
		t.Error("'test' should match ^[a-z]+$")
	}
}

func TestValidationBuilder_OneOf(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "tier")

	vb.OneOf(path, "invalid", "small", "medium", "large")
	if !vb.HasErrors() {
		t.Error("'invalid' should not be one of [small, medium, large]")
	}

	vb = NewValidationBuilder()
	vb.OneOf(path, "medium", "small", "medium", "large")
	if vb.HasErrors() {
		t.Error("'medium' should be one of [small, medium, large]")
	}
}

func TestValidationBuilder_Immutable(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "storageClass")

	vb.Immutable(path, "standard", "premium")
	if !vb.HasErrors() {
		t.Error("Changed value should fail immutable check")
	}

	vb = NewValidationBuilder()
	vb.Immutable(path, "standard", "standard")
	if vb.HasErrors() {
		t.Error("Same value should pass immutable check")
	}
}

func TestValidationBuilder_ImmutableAfterSet(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "storageClass")

	// Empty -> value is allowed
	vb.ImmutableAfterSet(path, "", "standard")
	if vb.HasErrors() {
		t.Error("Setting initially empty field should be allowed")
	}

	// value -> different value is not allowed
	vb.ImmutableAfterSet(path, "standard", "premium")
	if !vb.HasErrors() {
		t.Error("Changing set field should fail")
	}
}

func TestValidationBuilder_Forbidden(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "deprecated")

	vb.Forbidden(path, "value", "field is deprecated")
	if !vb.HasErrors() {
		t.Error("Non-empty forbidden field should fail")
	}

	vb = NewValidationBuilder()
	vb.Forbidden(path, "", "field is deprecated")
	if vb.HasErrors() {
		t.Error("Empty forbidden field should pass")
	}
}

func TestValidationBuilder_Custom(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "value")

	vb.Custom(path, false, "invalid", "custom validation failed")
	if !vb.HasErrors() {
		t.Error("Custom false should add error")
	}

	vb = NewValidationBuilder()
	vb.Custom(path, true, "valid", "custom validation failed")
	if vb.HasErrors() {
		t.Error("Custom true should not add error")
	}
}

func TestValidationBuilder_CustomFunc(t *testing.T) {
	vb := NewValidationBuilder()

	vb.CustomFunc(func() *field.Error {
		return field.Invalid(field.NewPath("spec"), "value", "custom error")
	})
	if !vb.HasErrors() {
		t.Error("CustomFunc returning error should add it")
	}

	vb = NewValidationBuilder()
	vb.CustomFunc(func() *field.Error {
		return nil
	})
	if vb.HasErrors() {
		t.Error("CustomFunc returning nil should not add error")
	}
}

func TestValidationBuilder_Warn(t *testing.T) {
	vb := NewValidationBuilder()

	vb.Warn("This is a warning")
	warnings := vb.Warnings()

	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(warnings))
	}
	if warnings[0] != "This is a warning" {
		t.Errorf("Warning = %v", warnings[0])
	}
}

func TestValidationBuilder_WarnIf(t *testing.T) {
	vb := NewValidationBuilder()

	vb.WarnIf(true, "Should warn")
	vb.WarnIf(false, "Should not warn")

	warnings := vb.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(warnings))
	}
}

func TestValidationBuilder_WarnDeprecated(t *testing.T) {
	vb := NewValidationBuilder()
	path := field.NewPath("spec", "oldField")

	vb.WarnDeprecated(path, "Use newField instead")

	warnings := vb.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(warnings))
	}
}

func TestValidationBuilder_Build(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		vb := NewValidationBuilder()
		vb.Warn("warning")

		warnings, err := vb.Build()
		if err != nil {
			t.Errorf("Build should not return error: %v", err)
		}
		if len(warnings) != 1 {
			t.Error("Warnings should be returned")
		}
	})

	t.Run("has errors", func(t *testing.T) {
		vb := NewValidationBuilder()
		vb.RequiredString(field.NewPath("spec", "name"), "")

		warnings, err := vb.Build()
		if err == nil {
			t.Error("Build should return error")
		}
		_ = warnings
	})
}

func TestValidationBuilder_BuildError(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		vb := NewValidationBuilder()
		if vb.BuildError() != nil {
			t.Error("BuildError should return nil")
		}
	})

	t.Run("has errors", func(t *testing.T) {
		vb := NewValidationBuilder()
		vb.RequiredString(field.NewPath("spec", "name"), "")

		if vb.BuildError() == nil {
			t.Error("BuildError should return error")
		}
	})
}

func TestValidationBuilder_Merge(t *testing.T) {
	vb1 := NewValidationBuilder()
	vb1.RequiredString(field.NewPath("spec", "name"), "")
	vb1.Warn("warning1")

	vb2 := NewValidationBuilder()
	vb2.Positive(field.NewPath("spec", "count"), 0)
	vb2.Warn("warning2")

	vb1.Merge(vb2)

	if len(vb1.Errors()) != 2 {
		t.Errorf("Expected 2 errors after merge, got %d", len(vb1.Errors()))
	}
	if len(vb1.Warnings()) != 2 {
		t.Errorf("Expected 2 warnings after merge, got %d", len(vb1.Warnings()))
	}
}

func TestValidationBuilder_WithBasePath(t *testing.T) {
	vb := NewValidationBuilder().WithBasePath(field.NewPath("spec"))

	vb.RequiredString(field.NewPath("name"), "")

	errors := vb.Errors()
	if len(errors) != 1 {
		t.Fatal("Expected 1 error")
	}
	// The path should combine base + field path
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "value", false},
		{"empty slice", []int{}, true},
		{"non-empty slice", []int{1}, false},
		{"empty map", map[string]int{}, true},
		{"non-empty map", map[string]int{"a": 1}, false},
		{"zero int", 0, true},
		{"non-zero int", 1, false},
		{"zero float", 0.0, true},
		{"non-zero float", 1.5, false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"nil pointer", (*int)(nil), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmpty(tt.value); got != tt.want {
				t.Errorf("isEmpty(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestFieldErrorHelpers(t *testing.T) {
	t.Run("FieldRequired", func(t *testing.T) {
		err := FieldRequired(".spec.name")
		if err == nil {
			t.Error("Should return error")
		}
	})

	t.Run("FieldInvalid", func(t *testing.T) {
		err := FieldInvalid(".spec.replicas", -1, "must be positive")
		if err == nil {
			t.Error("Should return error")
		}
	})

	t.Run("FieldImmutable", func(t *testing.T) {
		err := FieldImmutable(".spec.storageClass")
		if err == nil {
			t.Error("Should return error")
		}
	})

	t.Run("FieldForbidden", func(t *testing.T) {
		err := FieldForbidden(".spec.deprecated", "field is deprecated")
		if err == nil {
			t.Error("Should return error")
		}
	})

	t.Run("FieldNotSupported", func(t *testing.T) {
		err := FieldNotSupported(".spec.tier", "xlarge", []string{"small", "medium", "large"})
		if err == nil {
			t.Error("Should return error")
		}
	})
}

func TestSetDefaultHelpers(t *testing.T) {
	t.Run("SetDefault", func(t *testing.T) {
		var s string
		SetDefault(&s, "default")
		if s != "default" {
			t.Errorf("SetDefault failed: got %v", s)
		}

		s = "existing"
		SetDefault(&s, "default")
		if s != "existing" {
			t.Error("SetDefault should not override existing value")
		}
	})

	t.Run("SetDefaultString", func(t *testing.T) {
		var s string
		SetDefaultString(&s, "default")
		if s != "default" {
			t.Errorf("SetDefaultString failed: got %v", s)
		}
	})

	t.Run("SetDefaultInt", func(t *testing.T) {
		var i int
		SetDefaultInt(&i, 10)
		if i != 10 {
			t.Errorf("SetDefaultInt failed: got %v", i)
		}
	})

	t.Run("SetDefaultInt32", func(t *testing.T) {
		var i int32
		SetDefaultInt32(&i, 10)
		if i != 10 {
			t.Errorf("SetDefaultInt32 failed: got %v", i)
		}
	})

	t.Run("SetDefaultInt64", func(t *testing.T) {
		var i int64
		SetDefaultInt64(&i, 10)
		if i != 10 {
			t.Errorf("SetDefaultInt64 failed: got %v", i)
		}
	})

	t.Run("SetDefaultBool", func(t *testing.T) {
		var b *bool
		SetDefaultBool(&b, true)
		if b == nil || !*b {
			t.Error("SetDefaultBool failed")
		}
	})

	t.Run("SetDefaultSlice", func(t *testing.T) {
		var s []string
		SetDefaultSlice(&s, []string{"a", "b"})
		if len(s) != 2 {
			t.Errorf("SetDefaultSlice failed: got %v", s)
		}
	})

	t.Run("SetDefaultMap", func(t *testing.T) {
		var m map[string]int
		SetDefaultMap(&m, map[string]int{"a": 1})
		if len(m) != 1 {
			t.Errorf("SetDefaultMap failed: got %v", m)
		}
	})

	t.Run("EnsureMapKey", func(t *testing.T) {
		m := map[string]int{}
		EnsureMapKey(m, "key", 10)
		if m["key"] != 10 {
			t.Errorf("EnsureMapKey failed: got %v", m["key"])
		}

		EnsureMapKey(m, "key", 20)
		if m["key"] != 10 {
			t.Error("EnsureMapKey should not override existing")
		}
	})
}

// Mock validating handler for testing
type mockValidatingHandler struct {
	createErr error
	updateErr error
	deleteErr error
}

func (h *mockValidatingHandler) ValidateCreate(ctx context.Context, obj *corev1.Pod) (admission.Warnings, error) {
	return nil, h.createErr
}

func (h *mockValidatingHandler) ValidateUpdate(ctx context.Context, oldObj, newObj *corev1.Pod) (admission.Warnings, error) {
	return nil, h.updateErr
}

func (h *mockValidatingHandler) ValidateDelete(ctx context.Context, obj *corev1.Pod) (admission.Warnings, error) {
	return nil, h.deleteErr
}

func TestWebhookValidator(t *testing.T) {
	handler := &mockValidatingHandler{}
	validator := NewWebhookValidator[*corev1.Pod](handler)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	t.Run("ValidateCreate", func(t *testing.T) {
		_, err := validator.ValidateCreate(context.Background(), pod)
		if err != nil {
			t.Errorf("ValidateCreate failed: %v", err)
		}
	})

	t.Run("ValidateCreate with error", func(t *testing.T) {
		handler.createErr = errors.New("validation failed")
		_, err := validator.ValidateCreate(context.Background(), pod)
		if err == nil {
			t.Error("ValidateCreate should return error")
		}
		handler.createErr = nil
	})

	t.Run("ValidateUpdate", func(t *testing.T) {
		_, err := validator.ValidateUpdate(context.Background(), pod, pod)
		if err != nil {
			t.Errorf("ValidateUpdate failed: %v", err)
		}
	})

	t.Run("ValidateDelete", func(t *testing.T) {
		_, err := validator.ValidateDelete(context.Background(), pod)
		if err != nil {
			t.Errorf("ValidateDelete failed: %v", err)
		}
	})
}

// Mock defaulting handler for testing
type mockDefaultingHandler struct{}

func (h *mockDefaultingHandler) Default(ctx context.Context, obj *corev1.Pod) {
	if obj.Labels == nil {
		obj.Labels = map[string]string{}
	}
	obj.Labels["defaulted"] = "true"
}

func TestWebhookDefaulter(t *testing.T) {
	handler := &mockDefaultingHandler{}
	defaulter := NewWebhookDefaulter[*corev1.Pod](handler)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	err := defaulter.Default(context.Background(), pod)
	if err != nil {
		t.Errorf("Default failed: %v", err)
	}

	if pod.Labels["defaulted"] != "true" {
		t.Error("Default should have set label")
	}
}

func TestBaseValidator(t *testing.T) {
	validator := &BaseValidator[*corev1.Pod]{}
	pod := &corev1.Pod{}

	warnings, err := validator.ValidateCreate(context.Background(), pod)
	if err != nil || warnings != nil {
		t.Error("BaseValidator.ValidateCreate should return nil")
	}

	warnings, err = validator.ValidateUpdate(context.Background(), pod, pod)
	if err != nil || warnings != nil {
		t.Error("BaseValidator.ValidateUpdate should return nil")
	}

	warnings, err = validator.ValidateDelete(context.Background(), pod)
	if err != nil || warnings != nil {
		t.Error("BaseValidator.ValidateDelete should return nil")
	}
}

func TestBaseDefaulter(t *testing.T) {
	defaulter := &BaseDefaulter[*corev1.Pod]{}
	pod := &corev1.Pod{}

	// Should not panic
	defaulter.Default(context.Background(), pod)
}

type testSpec struct {
	Name     string `validate:"required"`
	Replicas int
}

func TestSpecValidator(t *testing.T) {
	t.Run("valid struct", func(t *testing.T) {
		sv := NewSpecValidator(nil)
		sv.ValidateStruct(&testSpec{Name: "test", Replicas: 1})

		if sv.Build() != nil {
			t.Error("Valid struct should pass")
		}
	})

	t.Run("missing required", func(t *testing.T) {
		sv := NewSpecValidator(nil)
		sv.ValidateStruct(&testSpec{Replicas: 1})

		if sv.Build() == nil {
			t.Error("Missing required field should fail")
		}
	})

	t.Run("custom base path", func(t *testing.T) {
		sv := NewSpecValidator(field.NewPath("custom"))
		if sv == nil {
			t.Error("Should create validator with custom path")
		}
	})

	t.Run("non-struct value", func(t *testing.T) {
		sv := NewSpecValidator(nil)
		sv.ValidateStruct("not a struct")

		if sv.Build() != nil {
			t.Error("Non-struct should not add errors")
		}
	})
}
