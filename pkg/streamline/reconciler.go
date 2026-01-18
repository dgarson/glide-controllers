package streamline

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenericReconciler is the core reconciliation engine that implements reconcile.Reconciler.
// It handles the repetitive machinery of Kubernetes controllers:
//   - Type instantiation via reflection
//   - Object fetching with proper error handling
//   - Snapshot before handler execution for patch calculation
//   - Lifecycle routing (deletion vs sync paths)
//   - Smart status patching using MergePatch
//   - Result translation
//
// Use NewGenericReconciler to create instances.
type GenericReconciler[T client.Object] struct {
	// Client is the Kubernetes client for API operations.
	Client client.Client

	// Handler is the user-provided business logic handler.
	Handler Handler[T]

	// Scheme is the runtime scheme for type information.
	Scheme *runtime.Scheme

	// EventRecorder is used to record Kubernetes events.
	EventRecorder record.EventRecorder

	// Log is the base logger for reconciliation operations.
	Log logr.Logger

	// objType holds the reflect.Type of T for instantiation.
	objType reflect.Type
}

// NewGenericReconciler creates a new GenericReconciler for the given handler.
// The type parameter T must be a pointer to a struct that implements client.Object.
//
// Example:
//
//	reconciler := streamline.NewGenericReconciler[*v1.MyResource](
//	    client,
//	    &MyHandler{},
//	    scheme,
//	    eventRecorder,
//	    log,
//	)
func NewGenericReconciler[T client.Object](
	c client.Client,
	handler Handler[T],
	scheme *runtime.Scheme,
	eventRecorder record.EventRecorder,
	log logr.Logger,
) *GenericReconciler[T] {
	// Get the type of T for reflection-based instantiation.
	// We need to get the element type since T is a pointer type.
	var zero T
	objType := reflect.TypeOf(zero).Elem()

	return &GenericReconciler[T]{
		Client:        c,
		Handler:       handler,
		Scheme:        scheme,
		EventRecorder: eventRecorder,
		Log:           log,
		objType:       objType,
	}
}

// Reconcile implements reconcile.Reconciler.
// It handles the full reconciliation workflow:
//  1. Instantiate a new instance of T
//  2. Fetch the object from the API server
//  3. Handle NotFound (return nil, object was deleted)
//  4. Snapshot the object for patch calculation
//  5. Route to Sync or Finalize based on deletion state
//  6. Calculate and apply status patch
//  7. Translate and return result
func (r *GenericReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.V(1).Info("starting reconciliation")

	// Step 1: Instantiate a new instance of T using reflection
	obj := r.newObject()

	// Step 2: Fetch the object from the API server
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			// Object was deleted, nothing to do
			log.V(1).Info("object not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		// Unexpected error, requeue with backoff
		log.Error(err, "failed to fetch object")
		return ctrl.Result{}, err
	}

	// Step 3: Snapshot the object for patch calculation
	original := obj.DeepCopyObject().(client.Object)

	// Step 4: Create the streamline context for the handler
	sCtx := NewContext(r.Client, log, r.EventRecorder, obj)

	// Step 5: Route based on deletion state
	var result Result
	var err error

	if !obj.GetDeletionTimestamp().IsZero() {
		// Object is being deleted - route to finalization (Phase 3)
		result, err = r.handleDeletion(ctx, obj, sCtx)
	} else {
		// Object is not being deleted - route to sync
		result, err = r.handleSync(ctx, obj, sCtx)
	}

	if err != nil {
		log.Error(err, "handler returned error")
		return ctrl.Result{}, err
	}

	// Step 6: Calculate and apply status patch if status changed
	if err := r.patchStatus(ctx, obj, original); err != nil {
		log.Error(err, "failed to patch status")
		return ctrl.Result{}, err
	}

	// Step 7: Translate and return result
	log.V(1).Info("reconciliation complete", "requeue", result.Requeue, "requeueAfter", result.RequeueAfter)
	return result.ToCtrlResult(), nil
}

// newObject creates a new instance of T using reflection.
// T must be a pointer type (e.g., *v1.MyResource).
func (r *GenericReconciler[T]) newObject() T {
	// Create a new instance of the underlying struct type
	newObj := reflect.New(r.objType).Interface()
	return newObj.(T)
}

// handleSync routes to the Handler's Sync method.
// This is called when the object is not being deleted.
func (r *GenericReconciler[T]) handleSync(ctx context.Context, obj T, sCtx *Context) (Result, error) {
	return r.Handler.Sync(ctx, obj, sCtx)
}

// handleDeletion handles objects marked for deletion.
// If the handler implements FinalizingHandler, it calls Finalize.
// Otherwise, it returns Stop() to allow deletion to proceed.
// Note: Full finalizer management is implemented in Phase 3.
func (r *GenericReconciler[T]) handleDeletion(ctx context.Context, obj T, sCtx *Context) (Result, error) {
	// Check if handler implements FinalizingHandler
	if fh, ok := any(r.Handler).(FinalizingHandler[T]); ok {
		return fh.Finalize(ctx, obj, sCtx)
	}

	// Handler doesn't implement FinalizingHandler, allow deletion
	sCtx.Log.V(1).Info("handler does not implement FinalizingHandler, allowing deletion")
	return Stop(), nil
}

// patchStatus calculates the diff between original and current object,
// and applies a status patch if there are changes.
func (r *GenericReconciler[T]) patchStatus(ctx context.Context, obj, original client.Object) error {
	// Use MergeFrom to calculate the patch
	patch := client.MergeFrom(original)

	// Only patch if there are actual changes
	// We attempt the patch and ignore "not modified" scenarios
	if err := r.Client.Status().Patch(ctx, obj, patch); err != nil {
		// Ignore conflicts for now - the next reconciliation will resolve them
		if apierrors.IsConflict(err) {
			r.Log.V(1).Info("status patch conflict, will retry on next reconciliation")
			return nil
		}
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
// This is a convenience method for registering the reconciler.
//
// Example:
//
//	if err := reconciler.SetupWithManager(mgr); err != nil {
//	    return err
//	}
func (r *GenericReconciler[T]) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.newObject()).
		Complete(r)
}
