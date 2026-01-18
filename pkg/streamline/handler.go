package streamline

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler defines the business logic for a resource T.
// Implement this interface to provide reconciliation logic for your custom resource.
//
// The generic type parameter T must satisfy client.Object, meaning it must be
// a Kubernetes object type (typically a pointer to a struct that implements
// runtime.Object and metav1.Object).
type Handler[T client.Object] interface {
	// Sync is called for creation and update events.
	// The object is guaranteed to exist and not be marked for deletion.
	//
	// Parameters:
	//   - ctx: The context from the reconciliation request, carrying deadlines and cancellation signals
	//   - obj: The Kubernetes object being reconciled, already fetched from the API server
	//   - sCtx: The Streamline context providing logging, events, and client access
	//
	// Returns:
	//   - Result: Indicates whether to requeue and after how long
	//   - error: If non-nil, the reconciliation will be retried with exponential backoff
	//
	// The handler may modify obj.Status directly; changes will be automatically
	// patched to the API server after Sync returns.
	Sync(ctx context.Context, obj T, sCtx *Context) (Result, error)
}

// FinalizingHandler is an optional interface that extends Handler with cleanup logic.
// Implement this interface if your resource needs to perform cleanup operations
// before being deleted (e.g., deleting external resources, revoking credentials).
//
// When a handler implements FinalizingHandler, the framework will automatically:
//   - Add a finalizer to the resource when it's first reconciled
//   - Call Finalize() instead of Sync() when the resource is being deleted
//   - Remove the finalizer only after Finalize() returns Stop()
type FinalizingHandler[T client.Object] interface {
	Handler[T]

	// Finalize is called when the object is marked for deletion (DeletionTimestamp is set).
	//
	// Parameters:
	//   - ctx: The context from the reconciliation request
	//   - obj: The Kubernetes object being deleted
	//   - sCtx: The Streamline context providing logging, events, and client access
	//
	// Returns:
	//   - Result: Return Stop() to confirm cleanup is finished and the finalizer can be removed.
	//     Return Requeue() or RequeueAfter() to retry cleanup later.
	//   - error: If non-nil, the reconciliation will be retried with exponential backoff.
	//     The finalizer will NOT be removed if an error is returned.
	//
	// Important: The finalizer is only removed when Finalize returns Stop() with no error.
	// This ensures that cleanup is fully complete before the object is garbage collected.
	Finalize(ctx context.Context, obj T, sCtx *Context) (Result, error)
}
