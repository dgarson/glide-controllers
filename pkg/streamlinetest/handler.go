package streamlinetest

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// FakeHandler is a configurable handler for testing.
// Use NewFakeHandler to create instances.
type FakeHandler[T client.Object] struct {
	syncFunc     func(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error)
	finalizeFunc func(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error)

	// Tracking
	syncCalls     []T
	finalizeCalls []T
}

// NewFakeHandler creates a new FakeHandler with default implementations
// that return Stop() with no error.
func NewFakeHandler[T client.Object]() *FakeHandler[T] {
	return &FakeHandler[T]{
		syncCalls:     make([]T, 0),
		finalizeCalls: make([]T, 0),
	}
}

// WithSync sets the Sync implementation.
func (h *FakeHandler[T]) WithSync(fn func(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error)) *FakeHandler[T] {
	h.syncFunc = fn
	return h
}

// WithFinalize sets the Finalize implementation.
func (h *FakeHandler[T]) WithFinalize(fn func(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error)) *FakeHandler[T] {
	h.finalizeFunc = fn
	return h
}

// Sync implements streamline.Handler.
func (h *FakeHandler[T]) Sync(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error) {
	h.syncCalls = append(h.syncCalls, obj)
	if h.syncFunc != nil {
		return h.syncFunc(ctx, obj, sCtx)
	}
	return streamline.Stop(), nil
}

// Finalize implements streamline.FinalizingHandler.
func (h *FakeHandler[T]) Finalize(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error) {
	h.finalizeCalls = append(h.finalizeCalls, obj)
	if h.finalizeFunc != nil {
		return h.finalizeFunc(ctx, obj, sCtx)
	}
	return streamline.Stop(), nil
}

// SyncCalls returns all objects passed to Sync.
func (h *FakeHandler[T]) SyncCalls() []T {
	return h.syncCalls
}

// FinalizeCalls returns all objects passed to Finalize.
func (h *FakeHandler[T]) FinalizeCalls() []T {
	return h.finalizeCalls
}

// SyncCallCount returns the number of Sync calls.
func (h *FakeHandler[T]) SyncCallCount() int {
	return len(h.syncCalls)
}

// FinalizeCallCount returns the number of Finalize calls.
func (h *FakeHandler[T]) FinalizeCallCount() int {
	return len(h.finalizeCalls)
}

// ClearCalls resets all call tracking.
func (h *FakeHandler[T]) ClearCalls() {
	h.syncCalls = make([]T, 0)
	h.finalizeCalls = make([]T, 0)
}

// Verify interface compliance
var _ streamline.Handler[client.Object] = &FakeHandler[client.Object]{}
var _ streamline.FinalizingHandler[client.Object] = &FakeHandler[client.Object]{}

// HandlerRecorder wraps a handler and records all calls for inspection.
type HandlerRecorder[T client.Object] struct {
	handler       streamline.Handler[T]
	syncCalls     []HandlerCall[T]
	finalizeCalls []HandlerCall[T]
}

// HandlerCall records a single handler invocation.
type HandlerCall[T client.Object] struct {
	Object T
	Result streamline.Result
	Error  error
}

// NewHandlerRecorder wraps a handler with call recording.
func NewHandlerRecorder[T client.Object](handler streamline.Handler[T]) *HandlerRecorder[T] {
	return &HandlerRecorder[T]{
		handler:       handler,
		syncCalls:     make([]HandlerCall[T], 0),
		finalizeCalls: make([]HandlerCall[T], 0),
	}
}

// Sync implements streamline.Handler and records the call.
func (r *HandlerRecorder[T]) Sync(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error) {
	result, err := r.handler.Sync(ctx, obj, sCtx)
	r.syncCalls = append(r.syncCalls, HandlerCall[T]{
		Object: obj,
		Result: result,
		Error:  err,
	})
	return result, err
}

// Finalize implements streamline.FinalizingHandler and records the call.
func (r *HandlerRecorder[T]) Finalize(ctx context.Context, obj T, sCtx *streamline.Context) (streamline.Result, error) {
	if fh, ok := any(r.handler).(streamline.FinalizingHandler[T]); ok {
		result, err := fh.Finalize(ctx, obj, sCtx)
		r.finalizeCalls = append(r.finalizeCalls, HandlerCall[T]{
			Object: obj,
			Result: result,
			Error:  err,
		})
		return result, err
	}
	return streamline.Stop(), nil
}

// SyncCalls returns all recorded Sync calls.
func (r *HandlerRecorder[T]) SyncCalls() []HandlerCall[T] {
	return r.syncCalls
}

// FinalizeCalls returns all recorded Finalize calls.
func (r *HandlerRecorder[T]) FinalizeCalls() []HandlerCall[T] {
	return r.finalizeCalls
}

// LastSyncCall returns the most recent Sync call, or nil if none.
func (r *HandlerRecorder[T]) LastSyncCall() *HandlerCall[T] {
	if len(r.syncCalls) == 0 {
		return nil
	}
	return &r.syncCalls[len(r.syncCalls)-1]
}

// LastFinalizeCall returns the most recent Finalize call, or nil if none.
func (r *HandlerRecorder[T]) LastFinalizeCall() *HandlerCall[T] {
	if len(r.finalizeCalls) == 0 {
		return nil
	}
	return &r.finalizeCalls[len(r.finalizeCalls)-1]
}

// ClearCalls resets all recorded calls.
func (r *HandlerRecorder[T]) ClearCalls() {
	r.syncCalls = make([]HandlerCall[T], 0)
	r.finalizeCalls = make([]HandlerCall[T], 0)
}
