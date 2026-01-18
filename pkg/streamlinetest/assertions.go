package streamlinetest

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// AssertErrorContains fails if err is nil or doesn't contain the expected string.
func AssertErrorContains(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Errorf("Expected error containing %q, got nil", contains)
		return
	}
	if !containsString(err.Error(), contains) {
		t.Errorf("Expected error containing %q, got: %v", contains, err)
	}
}

// AssertNoRequeue fails if the result indicates a requeue.
func AssertNoRequeue(t *testing.T, result streamline.Result) {
	t.Helper()
	if result.Requeue || result.RequeueAfter > 0 {
		t.Errorf("Expected no requeue, got Requeue=%v, RequeueAfter=%v",
			result.Requeue, result.RequeueAfter)
	}
}

// AssertRequeue fails if the result doesn't indicate an immediate requeue.
func AssertRequeue(t *testing.T, result streamline.Result) {
	t.Helper()
	if !result.Requeue {
		t.Errorf("Expected Requeue=true, got Requeue=%v, RequeueAfter=%v",
			result.Requeue, result.RequeueAfter)
	}
}

// AssertRequeueAfter fails if the result doesn't indicate a requeue after the expected duration.
func AssertRequeueAfter(t *testing.T, result streamline.Result, expected time.Duration) {
	t.Helper()
	if result.RequeueAfter != expected {
		t.Errorf("Expected RequeueAfter=%v, got RequeueAfter=%v",
			expected, result.RequeueAfter)
	}
}

// AssertRequeueAfterRange fails if the requeue duration is outside the expected range.
func AssertRequeueAfterRange(t *testing.T, result streamline.Result, min, max time.Duration) {
	t.Helper()
	if result.RequeueAfter < min || result.RequeueAfter > max {
		t.Errorf("Expected RequeueAfter between %v and %v, got %v",
			min, max, result.RequeueAfter)
	}
}

// AssertStop fails if the result is not a Stop (no requeue).
func AssertStop(t *testing.T, result streamline.Result) {
	t.Helper()
	if result.Requeue || result.RequeueAfter > 0 {
		t.Errorf("Expected Stop (no requeue), got Requeue=%v, RequeueAfter=%v",
			result.Requeue, result.RequeueAfter)
	}
}

// AssertConditionTrue fails if the condition is not True.
func AssertConditionTrue(t *testing.T, obj streamline.ObjectWithConditions, conditionType string) {
	t.Helper()
	helper := newConditionChecker(obj)
	if !helper.IsTrue(conditionType) {
		cond := helper.Get(conditionType)
		if cond == nil {
			t.Errorf("Condition %s not found", conditionType)
		} else {
			t.Errorf("Expected condition %s to be True, got %s (reason: %s, message: %s)",
				conditionType, cond.Status, cond.Reason, cond.Message)
		}
	}
}

// AssertConditionFalse fails if the condition is not False.
func AssertConditionFalse(t *testing.T, obj streamline.ObjectWithConditions, conditionType string) {
	t.Helper()
	helper := newConditionChecker(obj)
	if !helper.IsFalse(conditionType) {
		cond := helper.Get(conditionType)
		if cond == nil {
			t.Errorf("Condition %s not found", conditionType)
		} else {
			t.Errorf("Expected condition %s to be False, got %s (reason: %s)",
				conditionType, cond.Status, cond.Reason)
		}
	}
}

// AssertConditionUnknown fails if the condition is not Unknown or missing.
func AssertConditionUnknown(t *testing.T, obj streamline.ObjectWithConditions, conditionType string) {
	t.Helper()
	helper := newConditionChecker(obj)
	if !helper.IsUnknown(conditionType) {
		cond := helper.Get(conditionType)
		t.Errorf("Expected condition %s to be Unknown, got %s", conditionType, cond.Status)
	}
}

// AssertConditionReason fails if the condition doesn't have the expected reason.
func AssertConditionReason(t *testing.T, obj streamline.ObjectWithConditions, conditionType, expectedReason string) {
	t.Helper()
	helper := newConditionChecker(obj)
	cond := helper.Get(conditionType)
	if cond == nil {
		t.Errorf("Condition %s not found", conditionType)
		return
	}
	if cond.Reason != expectedReason {
		t.Errorf("Expected condition %s reason %q, got %q",
			conditionType, expectedReason, cond.Reason)
	}
}

// AssertConditionMessage fails if the condition message doesn't contain the expected string.
func AssertConditionMessage(t *testing.T, obj streamline.ObjectWithConditions, conditionType, contains string) {
	t.Helper()
	helper := newConditionChecker(obj)
	cond := helper.Get(conditionType)
	if cond == nil {
		t.Errorf("Condition %s not found", conditionType)
		return
	}
	if !containsString(cond.Message, contains) {
		t.Errorf("Expected condition %s message to contain %q, got %q",
			conditionType, contains, cond.Message)
	}
}

// AssertNoCondition fails if the condition exists.
func AssertNoCondition(t *testing.T, obj streamline.ObjectWithConditions, conditionType string) {
	t.Helper()
	helper := newConditionChecker(obj)
	if cond := helper.Get(conditionType); cond != nil {
		t.Errorf("Expected no condition %s, but found it with status %s",
			conditionType, cond.Status)
	}
}

// AssertPhase fails if the phase doesn't match.
func AssertPhase(t *testing.T, obj streamline.ObjectWithPhase, expected string) {
	t.Helper()
	actual := obj.GetPhase()
	if actual != expected {
		t.Errorf("Expected phase %q, got %q", expected, actual)
	}
}

// AssertObservedGeneration fails if observedGeneration doesn't match generation.
func AssertObservedGeneration(t *testing.T, obj streamline.ObjectWithObservedGeneration, expected int64) {
	t.Helper()
	actual := obj.GetObservedGeneration()
	if actual != expected {
		t.Errorf("Expected observedGeneration %d, got %d", expected, actual)
	}
}

// AssertUpToDate fails if observedGeneration != generation.
func AssertUpToDate(t *testing.T, obj interface {
	streamline.ObjectWithObservedGeneration
	GetGeneration() int64
}) {
	t.Helper()
	gen := obj.GetGeneration()
	obs := obj.GetObservedGeneration()
	if gen != obs {
		t.Errorf("Expected observedGeneration (%d) to equal generation (%d)", obs, gen)
	}
}

// AssertEventRecorded fails if the event was not recorded in the context.
func AssertEventRecorded(t *testing.T, ctx *TestContext, eventType, reason string) {
	t.Helper()
	ctx.AssertEventRecorded(eventType, reason)
}

// AssertNormalEvent fails if a Normal event with the reason was not recorded.
func AssertNormalEvent(t *testing.T, ctx *TestContext, reason string) {
	t.Helper()
	ctx.AssertEventRecorded(corev1.EventTypeNormal, reason)
}

// AssertWarningEvent fails if a Warning event with the reason was not recorded.
func AssertWarningEvent(t *testing.T, ctx *TestContext, reason string) {
	t.Helper()
	ctx.AssertEventRecorded(corev1.EventTypeWarning, reason)
}

// AssertNoEvents fails if any events were recorded.
func AssertNoEvents(t *testing.T, ctx *TestContext) {
	t.Helper()
	ctx.AssertNoEvents()
}

// conditionChecker provides condition checking utilities.
type conditionChecker struct {
	obj streamline.ObjectWithConditions
}

func newConditionChecker(obj streamline.ObjectWithConditions) *conditionChecker {
	return &conditionChecker{obj: obj}
}

func (c *conditionChecker) Get(conditionType string) *metav1.Condition {
	for i := range c.obj.GetConditions() {
		if c.obj.GetConditions()[i].Type == conditionType {
			return &c.obj.GetConditions()[i]
		}
	}
	return nil
}

func (c *conditionChecker) IsTrue(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func (c *conditionChecker) IsFalse(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

func (c *conditionChecker) IsUnknown(conditionType string) bool {
	cond := c.Get(conditionType)
	return cond == nil || cond.Status == metav1.ConditionUnknown
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
