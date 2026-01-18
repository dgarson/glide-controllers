package testing

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// Scenario defines a test case for a handler.
type Scenario[T client.Object] struct {
	// Name is the name of the test case.
	Name string

	// Description provides additional context for the test.
	Description string

	// Setup is called before running the scenario to set up any required state.
	// Use this to create prerequisite objects.
	Setup func(ctx context.Context, tc *TestContext) error

	// Object is the primary object to reconcile.
	Object T

	// PreExisting lists objects that should exist before reconciliation.
	PreExisting []client.Object

	// ExpectedResult is the expected result from the handler.
	ExpectedResult *streamline.Result

	// ExpectedError indicates whether an error is expected.
	ExpectedError bool

	// ExpectedErrorContains checks if the error contains this substring.
	ExpectedErrorContains string

	// Assertions are additional assertions to run after the handler completes.
	Assertions []Assertion[T]

	// Cleanup is called after the scenario completes.
	Cleanup func(ctx context.Context, tc *TestContext) error
}

// Assertion is a function that performs an assertion on the test result.
type Assertion[T client.Object] func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error)

// ScenarioRunner runs test scenarios for a handler.
type ScenarioRunner[T client.Object] struct {
	Handler streamline.Handler[T]
	Scheme  *runtime.Scheme
}

// NewScenarioRunner creates a new ScenarioRunner.
func NewScenarioRunner[T client.Object](handler streamline.Handler[T], scheme *runtime.Scheme) *ScenarioRunner[T] {
	return &ScenarioRunner[T]{
		Handler: handler,
		Scheme:  scheme,
	}
}

// Run executes all scenarios as subtests.
func (sr *ScenarioRunner[T]) Run(t *testing.T, scenarios []Scenario[T]) {
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			sr.runScenario(t, scenario)
		})
	}
}

// RunScenario executes a single scenario.
func (sr *ScenarioRunner[T]) runScenario(t *testing.T, scenario Scenario[T]) {
	ctx := context.Background()

	// Create test context with pre-existing objects
	tc := NewTestContext(t, sr.Scheme, scenario.PreExisting...)

	// Run setup if provided
	if scenario.Setup != nil {
		if err := scenario.Setup(ctx, tc); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	// Ensure cleanup runs
	if scenario.Cleanup != nil {
		defer func() {
			if err := scenario.Cleanup(ctx, tc); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}()
	}

	// Create streamline context
	sCtx := tc.NewStreamlineContext(scenario.Object)

	// Run the handler
	result, err := sr.Handler.Sync(ctx, scenario.Object, sCtx)

	// Check error expectations
	if scenario.ExpectedError {
		if err == nil {
			t.Error("expected an error but got nil")
		} else if scenario.ExpectedErrorContains != "" && !containsString(err.Error(), scenario.ExpectedErrorContains) {
			t.Errorf("expected error containing %q, got: %v", scenario.ExpectedErrorContains, err)
		}
	} else if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check result expectations
	if scenario.ExpectedResult != nil {
		if result.Requeue != scenario.ExpectedResult.Requeue {
			t.Errorf("expected Requeue=%v, got %v", scenario.ExpectedResult.Requeue, result.Requeue)
		}
		if result.RequeueAfter != scenario.ExpectedResult.RequeueAfter {
			t.Errorf("expected RequeueAfter=%v, got %v", scenario.ExpectedResult.RequeueAfter, result.RequeueAfter)
		}
	}

	// Run additional assertions
	for _, assertion := range scenario.Assertions {
		assertion(t, tc, scenario.Object, result, err)
	}
}

// Common assertion helpers

// AssertPhaseEquals creates an assertion that checks the object's phase.
func AssertPhaseEquals[T client.Object](expectedPhase string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if owp, ok := any(obj).(streamline.ObjectWithPhase); ok {
			if owp.GetPhase() != expectedPhase {
				t.Errorf("expected phase %q, got %q", expectedPhase, owp.GetPhase())
			}
		} else {
			t.Error("object does not implement ObjectWithPhase")
		}
	}
}

// AssertConditionTrue creates an assertion that checks a condition is True.
func AssertConditionTrue[T client.Object](conditionType string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.ConditionTrue(obj, conditionType)
	}
}

// AssertConditionFalse creates an assertion that checks a condition is False.
func AssertConditionFalse[T client.Object](conditionType string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.ConditionFalse(obj, conditionType)
	}
}

// AssertConditionWithReason creates an assertion that checks a condition's reason.
func AssertConditionWithReason[T client.Object](conditionType, expectedReason string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.ConditionHasReason(obj, conditionType, expectedReason)
	}
}

// AssertEventRecorded creates an assertion that checks for a recorded event.
func AssertEventRecorded[T client.Object](eventType, reason string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.EventRecorded(eventType, reason)
	}
}

// AssertNormalEvent creates an assertion that checks for a Normal event.
func AssertNormalEvent[T client.Object](reason string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.NormalEventRecorded(reason)
	}
}

// AssertWarningEvent creates an assertion that checks for a Warning event.
func AssertWarningEvent[T client.Object](reason string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		assertions := NewAssertions(t, tc)
		assertions.WarningEventRecorded(reason)
	}
}

// AssertResourceCreated creates an assertion that checks if a resource was created.
func AssertResourceCreated[T client.Object](kind, namespace, name string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		created := tc.GetCreatedObjects()
		for _, o := range created {
			if o.GetObjectKind().GroupVersionKind().Kind == kind &&
				o.GetNamespace() == namespace &&
				o.GetName() == name {
				return
			}
		}
		t.Errorf("expected %s %s/%s to be created", kind, namespace, name)
	}
}

// AssertNoRequeue creates an assertion that checks for Stop() result.
func AssertNoRequeue[T client.Object]() Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if result.Requeue || result.RequeueAfter > 0 {
			t.Errorf("expected no requeue, got Requeue=%v, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
		}
	}
}

// AssertRequeue creates an assertion that checks for immediate requeue.
func AssertRequeue[T client.Object]() Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if !result.Requeue {
			t.Error("expected requeue, but Requeue was false")
		}
	}
}

// AssertRequeueAfter creates an assertion that checks for requeue after duration.
func AssertRequeueAfter[T client.Object](expected time.Duration) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if result.RequeueAfter != expected {
			t.Errorf("expected RequeueAfter=%v, got %v", expected, result.RequeueAfter)
		}
	}
}

// AssertNoError creates an assertion that checks for no error.
func AssertNoError[T client.Object]() Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	}
}

// AssertError creates an assertion that checks for an error.
func AssertError[T client.Object]() Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if err == nil {
			t.Error("expected an error, got nil")
		}
	}
}

// AssertErrorContains creates an assertion that checks error message.
func AssertErrorContains[T client.Object](substr string) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("expected error containing %q, got nil", substr)
			return
		}
		if !containsString(err.Error(), substr) {
			t.Errorf("expected error containing %q, got: %v", substr, err)
		}
	}
}

// Custom creates a custom assertion function.
func Custom[T client.Object](fn func(t *testing.T, tc *TestContext, obj T)) Assertion[T] {
	return func(t *testing.T, tc *TestContext, obj T, result streamline.Result, err error) {
		t.Helper()
		fn(t, tc, obj)
	}
}

// TableTest provides a simpler table-driven test structure.
type TableTest[T client.Object] struct {
	Name           string
	Object         T
	PreExisting    []client.Object
	WantRequeue    bool
	WantRequeueAfter time.Duration
	WantError      bool
	WantPhase      string
	WantConditions map[string]bool // conditionType -> expectedStatus (true=True, false=False)
	WantEvents     []string        // list of expected event reasons
}

// RunTableTests runs a slice of table tests.
func RunTableTests[T client.Object](t *testing.T, handler streamline.Handler[T], scheme *runtime.Scheme, tests []TableTest[T]) {
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tc := NewTestContext(t, scheme, tt.PreExisting...)
			sCtx := tc.NewStreamlineContext(tt.Object)

			result, err := handler.Sync(context.Background(), tt.Object, sCtx)

			// Check error
			if tt.WantError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.WantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check result
			if tt.WantRequeue && !result.Requeue && result.RequeueAfter == 0 {
				t.Error("expected requeue but got none")
			}
			if tt.WantRequeueAfter > 0 && result.RequeueAfter != tt.WantRequeueAfter {
				t.Errorf("expected RequeueAfter=%v, got %v", tt.WantRequeueAfter, result.RequeueAfter)
			}

			// Check phase
			if tt.WantPhase != "" {
				if owp, ok := any(tt.Object).(streamline.ObjectWithPhase); ok {
					if owp.GetPhase() != tt.WantPhase {
						t.Errorf("expected phase %q, got %q", tt.WantPhase, owp.GetPhase())
					}
				}
			}

			// Check conditions
			for condType, wantTrue := range tt.WantConditions {
				assertions := NewAssertions(t, tc)
				if wantTrue {
					assertions.ConditionTrue(tt.Object, condType)
				} else {
					assertions.ConditionFalse(tt.Object, condType)
				}
			}

			// Check events
			for _, reason := range tt.WantEvents {
				if !tc.Recorder.HasEvent("Normal", reason) && !tc.Recorder.HasEvent("Warning", reason) {
					t.Errorf("expected event with reason %q", reason)
				}
			}
		})
	}
}
