package streamline

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Context provides helper utilities for handler implementations.
// It wraps common dependencies (client, logger, event recorder) and provides
// convenient access methods.
//
// The Context is created fresh for each reconciliation and contains
// pre-configured logger with namespace/name keys already set.
type Context struct {
	// Client provides access to the Kubernetes API server.
	// Use this for reading/writing resources beyond the primary resource.
	Client client.Client

	// Log is a structured logger pre-configured with the resource's
	// namespace and name. Use this for all logging within handlers.
	Log logr.Logger

	// Event provides a simplified interface for recording Kubernetes events.
	// Events are automatically associated with the resource being reconciled.
	Event EventHelper

	// Status provides utilities for managing common status fields like
	// observedGeneration, phase, and lastUpdated.
	// Available for all objects, but methods will no-op if the object
	// doesn't implement the corresponding interface.
	Status StatusHelper

	// Conditions provides utilities for managing standard Kubernetes conditions.
	// Available for objects that implement ObjectWithConditions.
	// Methods will no-op if the object doesn't support conditions.
	Conditions ConditionHelper

	// Object provides direct access to the object being reconciled.
	// This is the same object passed to Sync/Finalize, provided here
	// for convenience when using helper methods.
	Object client.Object
}

// NewContext creates a new Context for handler execution.
// This is typically called by the framework, not by user code.
func NewContext(c client.Client, log logr.Logger, eventRecorder record.EventRecorder, obj runtime.Object) *Context {
	clientObj := obj.(client.Object)
	return &Context{
		Client: c,
		Log:    log,
		Event: &eventHelper{
			recorder: eventRecorder,
			object:   obj,
		},
		Status:     newStatusHelper(clientObj),
		Conditions: newConditionHelper(clientObj),
		Object:     clientObj,
	}
}

// EventHelper provides a simplified interface for recording Kubernetes events.
// It automatically associates events with the resource being reconciled,
// eliminating the need to pass the object reference for each event.
type EventHelper interface {
	// Normal records a normal event with the given reason and message.
	// Normal events indicate successful operations or informational messages.
	//
	// Example:
	//   sCtx.Event.Normal("Synced", "Successfully synced resource")
	Normal(reason, message string)

	// Normalf records a normal event with a formatted message.
	//
	// Example:
	//   sCtx.Event.Normalf("Scaled", "Scaled replicas from %d to %d", oldReplicas, newReplicas)
	Normalf(reason, messageFmt string, args ...interface{})

	// Warning records a warning event with the given reason and message.
	// Warning events indicate potential issues or degraded operations.
	//
	// Example:
	//   sCtx.Event.Warning("InvalidConfig", "Size cannot be negative")
	Warning(reason, message string)

	// Warningf records a warning event with a formatted message.
	//
	// Example:
	//   sCtx.Event.Warningf("RetryFailed", "Failed to sync after %d attempts", retryCount)
	Warningf(reason, messageFmt string, args ...interface{})
}

// eventHelper implements EventHelper by wrapping a Kubernetes EventRecorder.
type eventHelper struct {
	recorder record.EventRecorder
	object   runtime.Object
}

func (e *eventHelper) Normal(reason, message string) {
	e.recorder.Event(e.object, corev1.EventTypeNormal, reason, message)
}

func (e *eventHelper) Normalf(reason, messageFmt string, args ...interface{}) {
	e.recorder.Eventf(e.object, corev1.EventTypeNormal, reason, messageFmt, args...)
}

func (e *eventHelper) Warning(reason, message string) {
	e.recorder.Event(e.object, corev1.EventTypeWarning, reason, message)
}

func (e *eventHelper) Warningf(reason, messageFmt string, args ...interface{}) {
	e.recorder.Eventf(e.object, corev1.EventTypeWarning, reason, messageFmt, args...)
}
