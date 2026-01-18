// Package streamline provides a high-level, opinionated framework built on top of
// Kubernetes controller-runtime. It leverages Go Generics (1.20+) to abstract away
// the repetitive machinery of Kubernetes controllers (loops, cache fetches, finalizers,
// patch management).
//
// # Philosophy
//
// Streamline moves developers from thinking about "Event Loops" (low-level plumbing)
// to thinking about "State Transitions" (business logic).
//
// # Architecture
//
// The framework consists of three layers:
//
//   - Layer 1: Registration Facade - Builder pattern that bridges controller-runtime
//     Manager with the generic engine
//   - Layer 2: Generic Engine (Middleware) - Core reconciliation logic implementing
//     fetch, snapshot, lifecycle routing, and smart patching
//   - Layer 3: Context Wrapper - Helper struct providing logging, events, and client access
//
// # Basic Usage
//
// Define a handler for your resource type:
//
//	type MyHandler struct{}
//
//	func (h *MyHandler) Sync(ctx context.Context, obj *MyObj, sCtx *streamline.Context) (streamline.Result, error) {
//	    // Business logic only - object is already fetched
//	    obj.Status.State = "Running"
//	    return streamline.RequeueAfter(10 * time.Minute), nil
//	}
//
// For resources requiring cleanup logic, implement FinalizingHandler:
//
//	func (h *MyHandler) Finalize(ctx context.Context, obj *MyObj, sCtx *streamline.Context) (streamline.Result, error) {
//	    // Cleanup external resources
//	    if err := h.cleanupExternalResource(obj); err != nil {
//	        return streamline.Requeue(), err
//	    }
//	    return streamline.Stop(), nil
//	}
package streamline
