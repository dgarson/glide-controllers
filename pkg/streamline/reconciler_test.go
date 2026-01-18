package streamline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// testObject is a simple object for testing.
// We use corev1.ConfigMap as it's a standard Kubernetes type.
type testHandler struct {
	syncFunc     func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error)
	finalizeFunc func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error)
}

func (h *testHandler) Sync(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
	if h.syncFunc != nil {
		return h.syncFunc(ctx, obj, sCtx)
	}
	return Stop(), nil
}

// testFinalizingHandler extends testHandler with Finalize.
type testFinalizingHandler struct {
	testHandler
}

func (h *testFinalizingHandler) Finalize(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
	if h.finalizeFunc != nil {
		return h.finalizeFunc(ctx, obj, sCtx)
	}
	return Stop(), nil
}

// Ensure interfaces are satisfied
var _ Handler[*corev1.ConfigMap] = &testHandler{}
var _ FinalizingHandler[*corev1.ConfigMap] = &testFinalizingHandler{}

// fakeClient implements a minimal client.Client for testing.
type fakeClient struct {
	client.Client
	getFunc         func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	statusPatchFunc func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error
}

func (f *fakeClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if f.getFunc != nil {
		return f.getFunc(ctx, key, obj, opts...)
	}
	return nil
}

func (f *fakeClient) Status() client.SubResourceWriter {
	return &fakeStatusWriter{patchFunc: f.statusPatchFunc}
}

type fakeStatusWriter struct {
	client.SubResourceWriter
	patchFunc func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error
}

func (f *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if f.patchFunc != nil {
		return f.patchFunc(ctx, obj, patch, opts...)
	}
	return nil
}

func (f *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func TestNewGenericReconciler(t *testing.T) {
	handler := &testHandler{}
	scheme := runtime.NewScheme()
	recorder := &mockEventRecorder{}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		&fakeClient{},
		handler,
		scheme,
		recorder,
		logr.Discard(),
	)

	if reconciler == nil {
		t.Fatal("NewGenericReconciler returned nil")
	}
	if reconciler.Handler != handler {
		t.Error("Handler not set correctly")
	}
	if reconciler.Scheme != scheme {
		t.Error("Scheme not set correctly")
	}
}

func TestGenericReconciler_Reconcile_NotFound(t *testing.T) {
	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			t.Error("Sync should not be called when object is not found")
			return Stop(), nil
		},
	}

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, key.Name)
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != nil {
		t.Errorf("Reconcile should not return error for NotFound, got: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("Reconcile should return empty result for NotFound")
	}
}

func TestGenericReconciler_Reconcile_GetError(t *testing.T) {
	expectedErr := errors.New("connection refused")

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return expectedErr
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		&testHandler{},
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != expectedErr {
		t.Errorf("Reconcile should return the Get error, got: %v", err)
	}
}

func TestGenericReconciler_Reconcile_Sync(t *testing.T) {
	syncCalled := false

	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			syncCalled = true
			if obj.Name != "test-config" {
				t.Errorf("Expected name 'test-config', got '%s'", obj.Name)
			}
			return RequeueAfter(5 * time.Minute), nil
		},
	}

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Name = "test-config"
			cm.Namespace = "default"
			return nil
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test-config"},
	})

	if err != nil {
		t.Errorf("Reconcile should not return error, got: %v", err)
	}
	if !syncCalled {
		t.Error("Sync was not called")
	}
	if !result.Requeue || result.RequeueAfter != 5*time.Minute {
		t.Errorf("Expected RequeueAfter(5m), got Requeue=%v, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
	}
}

func TestGenericReconciler_Reconcile_SyncError(t *testing.T) {
	expectedErr := errors.New("sync failed")

	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			return Stop(), expectedErr
		},
	}

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return nil
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != expectedErr {
		t.Errorf("Reconcile should return handler error, got: %v", err)
	}
}

func TestGenericReconciler_Reconcile_Deletion_NoFinalizer(t *testing.T) {
	// Handler without FinalizingHandler should allow deletion
	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			t.Error("Sync should not be called during deletion")
			return Stop(), nil
		},
	}

	now := metav1.Now()
	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Name = "test"
			cm.DeletionTimestamp = &now
			return nil
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != nil {
		t.Errorf("Reconcile should not return error, got: %v", err)
	}
	// Should return Stop() allowing deletion
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("Reconcile should return empty result for deletion without finalizer")
	}
}

func TestGenericReconciler_Reconcile_Deletion_WithFinalizer(t *testing.T) {
	finalizeCalled := false

	handler := &testFinalizingHandler{
		testHandler: testHandler{
			syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
				t.Error("Sync should not be called during deletion")
				return Stop(), nil
			},
		},
	}
	handler.finalizeFunc = func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
		finalizeCalled = true
		return Stop(), nil
	}

	now := metav1.Now()
	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Name = "test"
			cm.DeletionTimestamp = &now
			return nil
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != nil {
		t.Errorf("Reconcile should not return error, got: %v", err)
	}
	if !finalizeCalled {
		t.Error("Finalize was not called")
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("Reconcile should return empty result after successful finalization")
	}
}

func TestGenericReconciler_Reconcile_StatusPatch(t *testing.T) {
	patchCalled := false

	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			// Modify the object to trigger a patch
			if obj.Annotations == nil {
				obj.Annotations = make(map[string]string)
			}
			obj.Annotations["modified"] = "true"
			return Stop(), nil
		},
	}

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			cm := obj.(*corev1.ConfigMap)
			cm.Name = "test"
			cm.Namespace = "default"
			return nil
		},
		statusPatchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			patchCalled = true
			return nil
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	if err != nil {
		t.Errorf("Reconcile should not return error, got: %v", err)
	}
	if !patchCalled {
		t.Error("Status patch was not called")
	}
}

func TestGenericReconciler_Reconcile_StatusPatchConflict(t *testing.T) {
	// Conflict errors should be ignored (will retry on next reconciliation)
	handler := &testHandler{
		syncFunc: func(ctx context.Context, obj *corev1.ConfigMap, sCtx *Context) (Result, error) {
			return Stop(), nil
		},
	}

	fc := &fakeClient{
		getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			return nil
		},
		statusPatchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			return apierrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict"))
		},
	}

	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		fc,
		handler,
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"},
	})

	// Conflict should not return an error
	if err != nil {
		t.Errorf("Reconcile should ignore conflict errors, got: %v", err)
	}
}

func TestGenericReconciler_newObject(t *testing.T) {
	reconciler := NewGenericReconciler[*corev1.ConfigMap](
		&fakeClient{},
		&testHandler{},
		runtime.NewScheme(),
		&mockEventRecorder{},
		logr.Discard(),
	)

	obj := reconciler.newObject()

	if obj == nil {
		t.Fatal("newObject returned nil")
	}

	// Verify the object can be used as a ConfigMap
	// The type is already *corev1.ConfigMap due to generics
	obj.Name = "test"
	if obj.Name != "test" {
		t.Error("newObject returned object that cannot be modified")
	}
}
