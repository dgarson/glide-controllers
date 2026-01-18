// Package standard demonstrates a traditional controller-runtime implementation.
// This serves as a baseline for comparison with the Streamline approach.
//
// Lines of Code: ~120 (excluding comments)
package standard

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	guestbookv1 "github.com/streamline-controllers/streamline/examples/guestbook/api/v1"
)

const (
	finalizerName = "guestbook.example.streamline.io/finalizer"
)

// GuestbookReconciler reconciles a Guestbook object using standard controller-runtime patterns.
type GuestbookReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

// Reconcile handles the reconciliation loop for Guestbook resources.
func (r *GuestbookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("guestbook", req.NamespacedName)

	// Step 1: Fetch the Guestbook instance
	guestbook := &guestbookv1.Guestbook{}
	if err := r.Get(ctx, req.NamespacedName, guestbook); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Guestbook resource not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Guestbook")
		return ctrl.Result{}, err
	}

	// Step 2: Check if the Guestbook instance is marked for deletion
	if !guestbook.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, guestbook, log)
	}

	// Step 3: Add finalizer if it doesn't exist
	if guestbook.Spec.ExternalResource != "" {
		if !controllerutil.ContainsFinalizer(guestbook, finalizerName) {
			log.Info("Adding finalizer")
			controllerutil.AddFinalizer(guestbook, finalizerName)
			if err := r.Update(ctx, guestbook); err != nil {
				log.Error(err, "Failed to add finalizer")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Step 4: Snapshot for status patching
	original := guestbook.DeepCopy()

	// Step 5: Reconcile the Guestbook (business logic)
	result, err := r.reconcileGuestbook(ctx, guestbook, log)
	if err != nil {
		return result, err
	}

	// Step 6: Patch status if changed
	if err := r.patchStatus(ctx, guestbook, original); err != nil {
		log.Error(err, "Failed to patch status")
		return ctrl.Result{}, err
	}

	return result, nil
}

// reconcileDelete handles the deletion of a Guestbook resource.
func (r *GuestbookReconciler) reconcileDelete(ctx context.Context, guestbook *guestbookv1.Guestbook, log logr.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(guestbook, finalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Finalizing Guestbook")

	// Simulate external resource cleanup
	if guestbook.Spec.ExternalResource != "" && guestbook.Status.ExternalResourceCreated {
		log.Info("Cleaning up external resource", "resource", guestbook.Spec.ExternalResource)
		// Simulate cleanup delay
		guestbook.Status.Phase = "Terminating"
		guestbook.Status.Message = "Cleaning up external resources"

		r.Recorder.Event(guestbook, "Normal", "Finalizing", "Cleaning up external resource")

		// In a real scenario, this would call an external API
		// For demo, we just mark it as cleaned up
		guestbook.Status.ExternalResourceCreated = false
	}

	// Remove finalizer
	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(guestbook, finalizerName)
	if err := r.Update(ctx, guestbook); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileGuestbook contains the main business logic.
func (r *GuestbookReconciler) reconcileGuestbook(ctx context.Context, guestbook *guestbookv1.Guestbook, log logr.Logger) (ctrl.Result, error) {
	// Validate spec
	if guestbook.Spec.Replicas < 1 {
		guestbook.Status.Phase = "Failed"
		guestbook.Status.Message = "Replicas must be at least 1"
		r.Recorder.Event(guestbook, "Warning", "InvalidSpec", "Replicas must be at least 1")
		return ctrl.Result{}, nil
	}

	// Simulate external resource creation
	if guestbook.Spec.ExternalResource != "" && !guestbook.Status.ExternalResourceCreated {
		log.Info("Creating external resource", "resource", guestbook.Spec.ExternalResource)
		guestbook.Status.ExternalResourceCreated = true
		r.Recorder.Event(guestbook, "Normal", "ExternalResourceCreated", "Created external resource")
	}

	// Update status
	guestbook.Status.Phase = "Running"
	guestbook.Status.ReadyReplicas = guestbook.Spec.Replicas
	guestbook.Status.Message = guestbook.Spec.Message
	guestbook.Status.LastUpdated = metav1.Now()

	r.Recorder.Event(guestbook, "Normal", "Synced", "Guestbook synced successfully")

	// Requeue after 5 minutes for periodic reconciliation
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// patchStatus patches the status subresource.
func (r *GuestbookReconciler) patchStatus(ctx context.Context, guestbook *guestbookv1.Guestbook, original *guestbookv1.Guestbook) error {
	patch := client.MergeFrom(original)
	if err := r.Status().Patch(ctx, guestbook, patch); err != nil {
		if apierrors.IsConflict(err) {
			return nil // Will retry on next reconciliation
		}
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuestbookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&guestbookv1.Guestbook{}).
		Complete(r)
}
