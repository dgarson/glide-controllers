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
// # Core Features
//
// The framework provides the following foundational capabilities:
//
// ## Resource Management
//
// ResourceManager provides high-level abstraction for managing child/owned resources:
//
//	result, err := sCtx.Resources.Ensure(ctx, deployment)
//	if result.Action == streamline.ResourceCreated {
//	    sCtx.Event.Normal("Created", "Created deployment")
//	}
//
// ## Error Classification
//
// Intelligent error handling with retry classification:
//
//	// Permanent errors won't be retried
//	return streamline.Stop(), streamline.Permanent(err)
//
//	// Retryable errors use exponential backoff
//	return streamline.Stop(), streamline.Retryable(err)
//
//	// Transient errors retry immediately
//	return streamline.Stop(), streamline.Transient(err)
//
// ## Metrics
//
// Built-in Prometheus metrics with custom metric support:
//
//	metrics := streamline.NewMetricsProvider(nil)
//	reconciler.WithMetrics(metrics, "mycontroller")
//
// ## Prerequisite Checks
//
// Declarative dependency checking:
//
//	prereqs := streamline.NewPrerequisiteChecker(sCtx.Client).
//	    RequireSecretKey(secretKey, "password").
//	    RequireReady(databaseKey, &Database{})
//	if satisfied, results := prereqs.CheckAll(ctx); !satisfied {
//	    sCtx.Conditions.SetNotReady(streamline.ReasonDependencyNotReady, prereqs.FormatMessage(results))
//	    return streamline.RequeueAfter(prereqs.GetRetryAfter(results)), nil
//	}
//
// ## Pause/Resume
//
// Annotation-based pause control:
//
//	reconciler.WithPauseSupport(true)
//	// Objects with streamline.io/paused=true will be skipped
//
// ## State Machine
//
// Formal phase transition management:
//
//	sm := streamline.CommonStateMachine()
//	if err := sm.Transition(obj, streamline.PhaseRunning); err != nil {
//	    // Handle invalid transition
//	}
//
// ## Circuit Breaker
//
// Protection for external dependencies:
//
//	cb := sCtx.Breakers.Get("external-api")
//	err := cb.Execute(func() error {
//	    return callExternalAPI()
//	})
//
// ## Dry Run Mode
//
// Preview changes without applying:
//
//	if streamline.IsDryRun(obj) {
//	    recorder := streamline.NewDryRunRecorder()
//	    // Use DryRunClient instead of real client
//	}
//
// ## Health Checks
//
// Integrated health reporting:
//
//	reconciler.WithHealthReporter(reporter)
//	reporter.ReportHealthy("reconcile")
//
// ## Webhook Helpers
//
// Fluent validation API:
//
//	v := streamline.NewValidationBuilder().
//	    RequiredString(field.NewPath("spec", "name"), obj.Spec.Name).
//	    InRange(field.NewPath("spec", "replicas"), obj.Spec.Replicas, 1, 100)
//	return v.Build()
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
//
// # Testing
//
// The testing subpackage provides comprehensive test utilities:
//
//	tc := testing.NewTestContext(t, scheme)
//	suite := testing.NewHandlerTestSuite(t, scheme, handler)
//	result, err := suite.RunSync(ctx, obj)
//	suite.Assert.ConditionTrue(obj, "Ready")
package streamline
