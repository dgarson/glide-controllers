package streamline

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)


func TestPrerequisiteChecker_Empty(t *testing.T) {
	pc := NewPrerequisiteChecker(nil)

	satisfied, results := pc.CheckAll(context.Background())
	if !satisfied {
		t.Error("Empty checker should be satisfied")
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestPrerequisiteChecker_WithDefaultRetry(t *testing.T) {
	pc := NewPrerequisiteChecker(nil).WithDefaultRetry(30 * time.Second)

	// Create a failing prerequisite
	pc.RequireCustom("test", func(ctx context.Context) PrerequisiteResult {
		return PrerequisiteResult{Satisfied: false, Message: "failed"}
	})

	_, results := pc.CheckAll(context.Background())
	retry := pc.GetRetryAfter(results)
	if retry != 30*time.Second {
		t.Errorf("GetRetryAfter = %v, want 30s", retry)
	}
}

func TestPrerequisiteChecker_Add(t *testing.T) {
	pc := NewPrerequisiteChecker(nil)

	pc.Add(&customPrerequisite{
		name: "test",
		check: func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: true}
		},
	})

	if pc.Count() != 1 {
		t.Errorf("Count = %d, want 1", pc.Count())
	}
}

func TestPrerequisiteChecker_AddAll(t *testing.T) {
	pc := NewPrerequisiteChecker(nil)

	prereq1 := &customPrerequisite{name: "test1", check: func(ctx context.Context) PrerequisiteResult {
		return PrerequisiteResult{Satisfied: true}
	}}
	prereq2 := &customPrerequisite{name: "test2", check: func(ctx context.Context) PrerequisiteResult {
		return PrerequisiteResult{Satisfied: true}
	}}

	pc.AddAll(prereq1, prereq2)

	if pc.Count() != 2 {
		t.Errorf("Count = %d, want 2", pc.Count())
	}
}

func TestPrerequisiteChecker_CheckAll(t *testing.T) {
	t.Run("all satisfied", func(t *testing.T) {
		pc := NewPrerequisiteChecker(nil)
		pc.RequireCustom("pass1", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: true, Message: "ok"}
		})
		pc.RequireCustom("pass2", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: true, Message: "ok"}
		})

		satisfied, results := pc.CheckAll(context.Background())
		if !satisfied {
			t.Error("Should be satisfied when all pass")
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("some failed", func(t *testing.T) {
		pc := NewPrerequisiteChecker(nil)
		pc.RequireCustom("pass", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: true}
		})
		pc.RequireCustom("fail", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: false, Message: "not ready"}
		})

		satisfied, results := pc.CheckAll(context.Background())
		if satisfied {
			t.Error("Should not be satisfied when some fail")
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})
}

func TestPrerequisiteChecker_CheckAllOrFail(t *testing.T) {
	t.Run("all satisfied", func(t *testing.T) {
		pc := NewPrerequisiteChecker(nil)
		pc.RequireCustom("pass", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Satisfied: true}
		})

		err := pc.CheckAllOrFail(context.Background())
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("some failed", func(t *testing.T) {
		pc := NewPrerequisiteChecker(nil)
		pc.RequireCustom("fail", func(ctx context.Context) PrerequisiteResult {
			return PrerequisiteResult{Name: "fail", Satisfied: false, Message: "not ready"}
		})

		err := pc.CheckAllOrFail(context.Background())
		if err == nil {
			t.Error("Expected error when prerequisites fail")
		}
	})
}

func TestPrerequisiteChecker_GetRetryAfter(t *testing.T) {
	pc := NewPrerequisiteChecker(nil).WithDefaultRetry(10 * time.Second)

	results := []PrerequisiteResult{
		{Satisfied: true},
		{Satisfied: false, RetryAfter: 5 * time.Second},
		{Satisfied: false, RetryAfter: 15 * time.Second},
		{Satisfied: false}, // Should use default
	}

	retry := pc.GetRetryAfter(results)
	if retry != 15*time.Second {
		t.Errorf("GetRetryAfter = %v, want 15s (max)", retry)
	}
}

func TestPrerequisiteChecker_FormatMessage(t *testing.T) {
	pc := NewPrerequisiteChecker(nil)

	t.Run("all satisfied", func(t *testing.T) {
		results := []PrerequisiteResult{
			{Satisfied: true},
		}
		msg := pc.FormatMessage(results)
		if msg != "all prerequisites satisfied" {
			t.Errorf("FormatMessage = %v", msg)
		}
	})

	t.Run("some failed", func(t *testing.T) {
		results := []PrerequisiteResult{
			{Name: "test1", Satisfied: false, Message: "not found"},
			{Name: "test2", Satisfied: true},
		}
		msg := pc.FormatMessage(results)
		if msg == "all prerequisites satisfied" {
			t.Error("Should not report all satisfied")
		}
	})
}

func TestSecretExists(t *testing.T) {
	scheme := setupTestScheme()

	t.Run("secret exists", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

		prereq := SecretExists(c, types.NamespacedName{Namespace: "default", Name: "test-secret"})
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when secret exists")
		}
	})

	t.Run("secret not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		prereq := SecretExists(c, types.NamespacedName{Namespace: "default", Name: "missing"})
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when secret is missing")
		}
	})
}

func TestSecretHasKey(t *testing.T) {
	scheme := setupTestScheme()

	t.Run("key exists", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"password": []byte("secret"),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

		prereq := SecretHasKey(c, types.NamespacedName{Namespace: "default", Name: "test-secret"}, "password")
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when key exists")
		}
	})

	t.Run("key missing", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"other": []byte("value"),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

		prereq := SecretHasKey(c, types.NamespacedName{Namespace: "default", Name: "test-secret"}, "password")
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when key is missing")
		}
	})

	t.Run("secret not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		prereq := SecretHasKey(c, types.NamespacedName{Namespace: "default", Name: "missing"}, "password")
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when secret is missing")
		}
	})
}

func TestConfigMapExists(t *testing.T) {
	scheme := setupTestScheme()

	t.Run("configmap exists", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		prereq := ConfigMapExists(c, types.NamespacedName{Namespace: "default", Name: "test-config"})
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when configmap exists")
		}
	})

	t.Run("configmap not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		prereq := ConfigMapExists(c, types.NamespacedName{Namespace: "default", Name: "missing"})
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when configmap is missing")
		}
	})
}

func TestConfigMapHasKey(t *testing.T) {
	scheme := setupTestScheme()

	t.Run("key in data", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
			Data: map[string]string{
				"config.yaml": "content",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		prereq := ConfigMapHasKey(c, types.NamespacedName{Namespace: "default", Name: "test-config"}, "config.yaml")
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when key exists in Data")
		}
	})

	t.Run("key in binary data", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
			BinaryData: map[string][]byte{
				"binary.bin": []byte("data"),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		prereq := ConfigMapHasKey(c, types.NamespacedName{Namespace: "default", Name: "test-config"}, "binary.bin")
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when key exists in BinaryData")
		}
	})

	t.Run("key missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		prereq := ConfigMapHasKey(c, types.NamespacedName{Namespace: "default", Name: "test-config"}, "missing")
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when key is missing")
		}
	})
}

func TestResourceExists(t *testing.T) {
	scheme := setupTestScheme()

	t.Run("resource exists", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

		prereq := ResourceExists(c, types.NamespacedName{Namespace: "default", Name: "test-pod"}, &corev1.Pod{})
		result := prereq.Check(context.Background())

		if !result.Satisfied {
			t.Error("Should be satisfied when resource exists")
		}
	})

	t.Run("resource not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		prereq := ResourceExists(c, types.NamespacedName{Namespace: "default", Name: "missing"}, &corev1.Pod{})
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when resource is missing")
		}
	})
}

// mockObjectWithConditionsForPrereq implements ObjectWithConditions for testing prerequisites.
type mockObjectWithConditionsForPrereq struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	conditions []metav1.Condition
}

func (m *mockObjectWithConditionsForPrereq) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockObjectWithConditionsForPrereq) SetConditions(conditions []metav1.Condition) {
	m.conditions = conditions
}

func (m *mockObjectWithConditionsForPrereq) DeepCopyObject() runtime.Object {
	return &mockObjectWithConditionsForPrereq{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: *m.ObjectMeta.DeepCopy(),
		conditions: append([]metav1.Condition{}, m.conditions...),
	}
}

func TestCustomPrerequisite(t *testing.T) {
	pc := NewPrerequisiteChecker(nil)

	called := false
	pc.RequireCustom("custom-check", func(ctx context.Context) PrerequisiteResult {
		called = true
		return PrerequisiteResult{
			Satisfied: true,
			Message:   "custom check passed",
		}
	})

	satisfied, _ := pc.CheckAll(context.Background())
	if !satisfied {
		t.Error("Should be satisfied")
	}
	if !called {
		t.Error("Custom check function should have been called")
	}
}

func TestAll_Composite(t *testing.T) {
	t.Run("all satisfied", func(t *testing.T) {
		prereq := All("all-check",
			&customPrerequisite{name: "p1", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: true}
			}},
			&customPrerequisite{name: "p2", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: true}
			}},
		)

		result := prereq.Check(context.Background())
		if !result.Satisfied {
			t.Error("All should be satisfied when all sub-prereqs are satisfied")
		}
	})

	t.Run("one failed", func(t *testing.T) {
		prereq := All("all-check",
			&customPrerequisite{name: "p1", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: true}
			}},
			&customPrerequisite{name: "p2", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Name: "p2", Satisfied: false, Message: "failed"}
			}},
		)

		result := prereq.Check(context.Background())
		if result.Satisfied {
			t.Error("All should not be satisfied when any sub-prereq fails")
		}
	})
}

func TestAny_Composite(t *testing.T) {
	t.Run("all satisfied", func(t *testing.T) {
		prereq := Any("any-check",
			&customPrerequisite{name: "p1", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: true}
			}},
			&customPrerequisite{name: "p2", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: true}
			}},
		)

		result := prereq.Check(context.Background())
		if !result.Satisfied {
			t.Error("Any should be satisfied when any sub-prereq is satisfied")
		}
	})

	t.Run("one satisfied", func(t *testing.T) {
		prereq := Any("any-check",
			&customPrerequisite{name: "p1", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: false, Message: "failed"}
			}},
			&customPrerequisite{name: "p2", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Name: "p2", Satisfied: true}
			}},
		)

		result := prereq.Check(context.Background())
		if !result.Satisfied {
			t.Error("Any should be satisfied when at least one sub-prereq is satisfied")
		}
	})

	t.Run("all failed", func(t *testing.T) {
		prereq := Any("any-check",
			&customPrerequisite{name: "p1", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: false}
			}},
			&customPrerequisite{name: "p2", check: func(ctx context.Context) PrerequisiteResult {
				return PrerequisiteResult{Satisfied: false}
			}},
		)

		result := prereq.Check(context.Background())
		if result.Satisfied {
			t.Error("Any should not be satisfied when all sub-prereqs fail")
		}
	})
}

func TestTimeSince(t *testing.T) {
	t.Run("enough time elapsed", func(t *testing.T) {
		past := time.Now().Add(-10 * time.Minute)
		prereq := TimeSince("time-check", func() time.Time { return past }, 5*time.Minute)

		result := prereq.Check(context.Background())
		if !result.Satisfied {
			t.Error("Should be satisfied when enough time has elapsed")
		}
	})

	t.Run("not enough time elapsed", func(t *testing.T) {
		recent := time.Now().Add(-1 * time.Minute)
		prereq := TimeSince("time-check", func() time.Time { return recent }, 5*time.Minute)

		result := prereq.Check(context.Background())
		if result.Satisfied {
			t.Error("Should not be satisfied when not enough time has elapsed")
		}
		if result.RetryAfter == 0 {
			t.Error("RetryAfter should be set for time-based prerequisites")
		}
	})

	t.Run("zero timestamp", func(t *testing.T) {
		prereq := TimeSince("time-check", func() time.Time { return time.Time{} }, 5*time.Minute)

		result := prereq.Check(context.Background())
		if result.Satisfied {
			t.Error("Should not be satisfied with zero timestamp")
		}
	})
}

func TestPrerequisiteChecker_FluentAPI(t *testing.T) {
	scheme := setupTestScheme()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("secret"),
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.yaml": "content",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, cm).Build()

	pc := NewPrerequisiteChecker(c).
		WithDefaultRetry(30 * time.Second).
		RequireSecret(types.NamespacedName{Namespace: "default", Name: "db-creds"}).
		RequireSecretKey(types.NamespacedName{Namespace: "default", Name: "db-creds"}, "password").
		RequireConfigMap(types.NamespacedName{Namespace: "default", Name: "app-config"}).
		RequireConfigMapKey(types.NamespacedName{Namespace: "default", Name: "app-config"}, "config.yaml")

	if pc.Count() != 4 {
		t.Errorf("Count = %d, want 4", pc.Count())
	}

	satisfied, _ := pc.CheckAll(context.Background())
	if !satisfied {
		t.Error("All prerequisites should be satisfied")
	}
}

// errorClient is a mock client that returns errors.
type errorClient struct {
	client.Client
	err error
}

func (c *errorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	return c.err
}

func TestPrerequisiteErrorHandling(t *testing.T) {
	t.Run("non-NotFound error", func(t *testing.T) {
		c := &errorClient{err: errors.New("network error")}

		prereq := SecretExists(c, types.NamespacedName{Namespace: "default", Name: "test"})
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied on error")
		}
		if result.Error == nil {
			t.Error("Error should be set")
		}
	})

	t.Run("NotFound error", func(t *testing.T) {
		c := &errorClient{err: apierrors.NewNotFound(schema.GroupResource{}, "test")}

		prereq := SecretExists(c, types.NamespacedName{Namespace: "default", Name: "test"})
		result := prereq.Check(context.Background())

		if result.Satisfied {
			t.Error("Should not be satisfied when not found")
		}
		if result.Error != nil {
			t.Error("Error should not be set for NotFound")
		}
	})
}

func TestPrerequisiteName(t *testing.T) {
	key := types.NamespacedName{Namespace: "default", Name: "test"}

	tests := []struct {
		prereq   Prerequisite
		contains string
	}{
		{SecretExists(nil, key), "Secret"},
		{SecretHasKey(nil, key, "password"), "password"},
		{ConfigMapExists(nil, key), "ConfigMap"},
		{ConfigMapHasKey(nil, key, "config"), "config"},
		{All("all-test"), "all-test"},
		{Any("any-test"), "any-test"},
		{TimeSince("time-test", nil, 0), "time-test"},
	}

	for _, tt := range tests {
		name := tt.prereq.Name()
		if name == "" {
			t.Errorf("Name should not be empty for %T", tt.prereq)
		}
	}
}
