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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// FinalizerName is the finalizer string added to objects managed by Streamline
// when the handler implements FinalizingHandler.
const FinalizerName = "streamline.io/finalizer"

// GenericReconciler is the core reconciliation engine that implements reconcile.Reconciler.
// It handles the repetitive machinery of Kubernetes controllers:
//   - Type instantiation via reflection
//   - Object fetching with proper error handling
//   - Automatic finalizer management for cleanup logic
//   - Snapshot before handler execution for patch calculation
//   - Lifecycle routing (deletion vs sync paths)
//   - Smart status patching using MergePatch
//   - Automatic observedGeneration tracking (optional)
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

	// isFinalizingHandler caches whether the handler implements FinalizingHandler.
	isFinalizingHandler bool

	// autoUpdateObservedGeneration enables automatic updating of status.observedGeneration
	// after successful sync operations. When enabled, if the object implements
	// ObjectWithObservedGeneration, the framework will automatically set
	// observedGeneration to match the object's generation after Sync completes
	// without error.
	autoUpdateObservedGeneration bool
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

	// Check if handler implements FinalizingHandler at construction time
	_, isFinalizingHandler := any(handler).(FinalizingHandler[T])

	return &GenericReconciler[T]{
		Client:                       c,
		Handler:                      handler,
		Scheme:                       scheme,
		EventRecorder:                eventRecorder,
		Log:                          log,
		objType:                      objType,
		isFinalizingHandler:          isFinalizingHandler,
		autoUpdateObservedGeneration: true, // Enabled by default
	}
}

// WithAutoObservedGeneration configures whether the reconciler should automatically
// update status.observedGeneration after successful sync operations.
// This is enabled by default. Disable it if you want to manage observedGeneration
// manually in your handler.
//
// Example:
//
//	reconciler := streamline.NewGenericReconciler[*v1.MyResource](...)
//	reconciler.WithAutoObservedGeneration(false) // Disable auto-update
func (r *GenericReconciler[T]) WithAutoObservedGeneration(enabled bool) *GenericReconciler[T] {
	r.autoUpdateObservedGeneration = enabled
	return r
}

// Reconcile implements reconcile.Reconciler.
// It handles the full reconciliation workflow:
//  1. Instantiate a new instance of T
//  2. Fetch the object from the API server
//  3. Handle NotFound (return nil, object was deleted)
//  4. Manage finalizers (add/remove based on handler type and deletion state)
//  5. Snapshot the object for patch calculation
//  6. Route to Sync or Finalize based on deletion state
//  7. Calculate and apply status patch
//  8. Translate and return result
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

	// Step 3: Check deletion state and manage finalizers
	if !obj.GetDeletionTimestamp().IsZero() {
		// Object is being deleted
		return r.reconcileDelete(ctx, obj, log)
	}

	// Object is not being deleted
	return r.reconcileNormal(ctx, obj, log)
}

// reconcileNormal handles the normal (non-deletion) reconciliation path.
// It ensures the finalizer is present if needed, then calls Sync.
func (r *GenericReconciler[T]) reconcileNormal(ctx context.Context, obj T, log logr.Logger) (ctrl.Result, error) {
	// Step 3a: If handler implements FinalizingHandler, ensure finalizer is present
	if r.isFinalizingHandler {
		if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
			log.V(1).Info("adding finalizer")
			controllerutil.AddFinalizer(obj, FinalizerName)
			if err := r.Client.Update(ctx, obj); err != nil {
				log.Error(err, "failed to add finalizer")
				return ctrl.Result{}, err
			}
			// Requeue to continue with sync after finalizer is added
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Step 4: Snapshot the object for patch calculation
	original := obj.DeepCopyObject().(client.Object)

	// Step 5: Create the streamline context for the handler
	sCtx := NewContext(r.Client, r.Scheme, log, r.EventRecorder, obj)

	// Step 6: Call Sync
	result, err := r.Handler.Sync(ctx, obj, sCtx)
	if err != nil {
		log.Error(err, "sync returned error")
		return ctrl.Result{}, err
	}

	// Step 6a: Auto-update observedGeneration if enabled and successful
	if r.autoUpdateObservedGeneration {
		r.updateObservedGeneration(obj, log)
	}

	// Step 7: Calculate and apply status patch if status changed
	if err := r.patchStatus(ctx, obj, original); err != nil {
		log.Error(err, "failed to patch status")
		return ctrl.Result{}, err
	}

	// Step 8: Translate and return result
	log.V(1).Info("reconciliation complete", "requeue", result.Requeue, "requeueAfter", result.RequeueAfter)
	return result.ToCtrlResult(), nil
}

// reconcileDelete handles the deletion reconciliation path.
// It calls Finalize if the handler supports it, then removes the finalizer.
func (r *GenericReconciler[T]) reconcileDelete(ctx context.Context, obj T, log logr.Logger) (ctrl.Result, error) {
	log.V(1).Info("object is being deleted")

	// Check if our finalizer is present
	if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
		// Finalizer not present, nothing to do
		log.V(1).Info("finalizer not present, allowing deletion")
		return ctrl.Result{}, nil
	}

	// Finalizer is present, need to handle cleanup
	if r.isFinalizingHandler {
		// Step 4: Snapshot the object for patch calculation
		original := obj.DeepCopyObject().(client.Object)

		// Step 5: Create the streamline context for the handler
		sCtx := NewContext(r.Client, r.Scheme, log, r.EventRecorder, obj)

		// Step 6: Call Finalize
		fh := any(r.Handler).(FinalizingHandler[T])
		result, err := fh.Finalize(ctx, obj, sCtx)
		if err != nil {
			log.Error(err, "finalize returned error")
			return ctrl.Result{}, err
		}

		// Step 7: Patch status before checking result
		if err := r.patchStatus(ctx, obj, original); err != nil {
			log.Error(err, "failed to patch status")
			return ctrl.Result{}, err
		}

		// Check if finalization is complete
		if !result.IsZero() {
			// Finalization not complete, requeue
			log.V(1).Info("finalization not complete, requeueing", "requeue", result.Requeue, "requeueAfter", result.RequeueAfter)
			return result.ToCtrlResult(), nil
		}
	}

	// Finalization complete (or no finalizing handler), remove finalizer
	log.V(1).Info("removing finalizer")
	controllerutil.RemoveFinalizer(obj, FinalizerName)
	if err := r.Client.Update(ctx, obj); err != nil {
		log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.V(1).Info("finalizer removed, deletion can proceed")
	return ctrl.Result{}, nil
}

// newObject creates a new instance of T using reflection.
// T must be a pointer type (e.g., *v1.MyResource).
func (r *GenericReconciler[T]) newObject() T {
	// Create a new instance of the underlying struct type
	newObj := reflect.New(r.objType).Interface()
	return newObj.(T)
}

// updateObservedGeneration sets status.observedGeneration to match metadata.generation
// if the object implements ObjectWithObservedGeneration.
func (r *GenericReconciler[T]) updateObservedGeneration(obj T, log logr.Logger) {
	if owog, ok := any(obj).(ObjectWithObservedGeneration); ok {
		generation := obj.GetGeneration()
		currentObserved := owog.GetObservedGeneration()
		if currentObserved != generation {
			owog.SetObservedGeneration(generation)
			log.V(1).Info("updated observedGeneration", "generation", generation)
		}
	}
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
