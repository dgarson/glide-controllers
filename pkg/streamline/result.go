package streamline

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Result indicates the outcome of a handler execution and determines
// whether the reconciliation should be requeued.
//
// Use the constructor functions (Stop, Requeue, RequeueAfter, Error) to create
// Result values rather than constructing them directly.
type Result struct {
	// Requeue indicates that the reconciliation should be requeued.
	// If false and RequeueAfter is zero, no requeue will occur.
	Requeue bool

	// RequeueAfter specifies the duration after which to requeue.
	// If non-zero, implies Requeue is true.
	RequeueAfter time.Duration
}

// Stop returns a Result indicating successful completion with no requeue.
// Use this when:
//   - The resource is in its desired state and no further action is needed
//   - Finalization is complete and the finalizer can be removed
//
// Example:
//
//	if obj.Status.State == desiredState {
//	    return streamline.Stop(), nil
//	}
func Stop() Result {
	return Result{
		Requeue:      false,
		RequeueAfter: 0,
	}
}

// Requeue returns a Result indicating the reconciliation should be requeued immediately.
// Use this when:
//   - The handler made progress but more work is needed
//   - You want to re-check the state after other controllers have run
//
// Example:
//
//	if !ready {
//	    sCtx.Log.Info("resource not ready, requeueing")
//	    return streamline.Requeue(), nil
//	}
func Requeue() Result {
	return Result{
		Requeue:      true,
		RequeueAfter: 0,
	}
}

// RequeueAfter returns a Result indicating the reconciliation should be requeued
// after the specified duration. Use this for:
//   - Periodic reconciliation (e.g., checking external resource status)
//   - Rate limiting (e.g., waiting for a resource to be ready)
//   - Scheduled operations (e.g., certificate renewal checks)
//
// Example:
//
//	// Recheck every 5 minutes
//	return streamline.RequeueAfter(5 * time.Minute), nil
//
//	// Wait for external resource to be ready
//	if !externalReady {
//	    return streamline.RequeueAfter(30 * time.Second), nil
//	}
func RequeueAfter(d time.Duration) Result {
	return Result{
		Requeue:      true,
		RequeueAfter: d,
	}
}

// IsZero returns true if this is a zero-value Result (equivalent to Stop()).
func (r Result) IsZero() bool {
	return !r.Requeue && r.RequeueAfter == 0
}

// ToCtrlResult converts a Streamline Result to a controller-runtime Result.
// This is used internally by the framework to translate handler results
// to the format expected by controller-runtime.
func (r Result) ToCtrlResult() ctrl.Result {
	return ctrl.Result{
		Requeue:      r.Requeue,
		RequeueAfter: r.RequeueAfter,
	}
}
