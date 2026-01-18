package streamline

import (
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestStop(t *testing.T) {
	result := Stop()

	if result.Requeue {
		t.Error("Stop() should have Requeue=false")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("Stop() should have RequeueAfter=0, got %v", result.RequeueAfter)
	}
}

func TestRequeue(t *testing.T) {
	result := Requeue()

	if !result.Requeue {
		t.Error("Requeue() should have Requeue=true")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("Requeue() should have RequeueAfter=0, got %v", result.RequeueAfter)
	}
}

func TestRequeueAfter(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"zero duration", 0},
		{"one second", time.Second},
		{"five minutes", 5 * time.Minute},
		{"one hour", time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RequeueAfter(tt.duration)

			if !result.Requeue {
				t.Error("RequeueAfter() should have Requeue=true")
			}
			if result.RequeueAfter != tt.duration {
				t.Errorf("RequeueAfter(%v) should have RequeueAfter=%v, got %v",
					tt.duration, tt.duration, result.RequeueAfter)
			}
		})
	}
}

func TestResult_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected bool
	}{
		{"Stop result", Stop(), true},
		{"Requeue result", Requeue(), false},
		{"RequeueAfter result", RequeueAfter(time.Second), false},
		{"zero value", Result{}, true},
		{"only Requeue true", Result{Requeue: true}, false},
		{"only RequeueAfter set", Result{RequeueAfter: time.Second}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsZero(); got != tt.expected {
				t.Errorf("Result.IsZero() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestResult_ToCtrlResult(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected ctrl.Result
	}{
		{
			name:     "Stop result",
			result:   Stop(),
			expected: ctrl.Result{Requeue: false, RequeueAfter: 0},
		},
		{
			name:     "Requeue result",
			result:   Requeue(),
			expected: ctrl.Result{Requeue: true, RequeueAfter: 0},
		},
		{
			name:     "RequeueAfter result",
			result:   RequeueAfter(5 * time.Minute),
			expected: ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Minute},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.ToCtrlResult()
			if got.Requeue != tt.expected.Requeue {
				t.Errorf("ToCtrlResult().Requeue = %v, want %v", got.Requeue, tt.expected.Requeue)
			}
			if got.RequeueAfter != tt.expected.RequeueAfter {
				t.Errorf("ToCtrlResult().RequeueAfter = %v, want %v", got.RequeueAfter, tt.expected.RequeueAfter)
			}
		})
	}
}
