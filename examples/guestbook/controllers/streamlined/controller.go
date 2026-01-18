// Package streamlined demonstrates a Streamline-based controller implementation.
// Compare this with the standard package to see the reduction in boilerplate.
//
// Lines of Code: ~55 (excluding comments) - 54% reduction from standard (~120 lines)
package streamlined

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	guestbookv1 "github.com/streamline-controllers/streamline/examples/guestbook/api/v1"
	"github.com/streamline-controllers/streamline/pkg/streamline"
)

// GuestbookHandler implements the business logic for Guestbook resources.
// It implements both Handler and FinalizingHandler interfaces.
type GuestbookHandler struct{}

// Ensure interface compliance at compile time.
var _ streamline.Handler[*guestbookv1.Guestbook] = &GuestbookHandler{}
var _ streamline.FinalizingHandler[*guestbookv1.Guestbook] = &GuestbookHandler{}

// Sync handles creation and update events.
// The object is guaranteed to exist and not be marked for deletion.
func (h *GuestbookHandler) Sync(ctx context.Context, guestbook *guestbookv1.Guestbook, sCtx *streamline.Context) (streamline.Result, error) {
	// Validate spec
	if guestbook.Spec.Replicas < 1 {
		guestbook.Status.Phase = "Failed"
		guestbook.Status.Message = "Replicas must be at least 1"
		sCtx.Event.Warning("InvalidSpec", "Replicas must be at least 1")
		return streamline.Stop(), nil
	}

	// Simulate external resource creation
	if guestbook.Spec.ExternalResource != "" && !guestbook.Status.ExternalResourceCreated {
		sCtx.Log.Info("Creating external resource", "resource", guestbook.Spec.ExternalResource)
		guestbook.Status.ExternalResourceCreated = true
		sCtx.Event.Normal("ExternalResourceCreated", "Created external resource")
	}

	// Update status
	guestbook.Status.Phase = "Running"
	guestbook.Status.ReadyReplicas = guestbook.Spec.Replicas
	guestbook.Status.Message = guestbook.Spec.Message
	guestbook.Status.LastUpdated = metav1.Now()

	sCtx.Event.Normal("Synced", "Guestbook synced successfully")

	// Requeue after 5 minutes for periodic reconciliation
	return streamline.RequeueAfter(5 * time.Minute), nil
}

// Finalize handles cleanup when the resource is being deleted.
// Return Stop() when cleanup is complete to allow finalizer removal.
func (h *GuestbookHandler) Finalize(ctx context.Context, guestbook *guestbookv1.Guestbook, sCtx *streamline.Context) (streamline.Result, error) {
	// Simulate external resource cleanup
	if guestbook.Spec.ExternalResource != "" && guestbook.Status.ExternalResourceCreated {
		sCtx.Log.Info("Cleaning up external resource", "resource", guestbook.Spec.ExternalResource)
		guestbook.Status.Phase = "Terminating"
		guestbook.Status.Message = "Cleaning up external resources"
		sCtx.Event.Normal("Finalizing", "Cleaning up external resource")

		// In a real scenario, this would call an external API
		guestbook.Status.ExternalResourceCreated = false
	}

	// Return Stop() to indicate cleanup is complete
	return streamline.Stop(), nil
}
