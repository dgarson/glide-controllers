package streamline

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dry run annotations.
const (
	// AnnotationDryRun indicates the resource should be reconciled in dry run mode.
	// Set to "true" to enable dry run.
	AnnotationDryRun = "streamline.io/dry-run"

	// AnnotationDryRunResult stores the result of the last dry run.
	AnnotationDryRunResult = "streamline.io/dry-run-result"
)

// DryRunAction represents an action that would be taken.
type DryRunAction string

const (
	// DryRunActionCreate indicates a resource would be created.
	DryRunActionCreate DryRunAction = "create"

	// DryRunActionUpdate indicates a resource would be updated.
	DryRunActionUpdate DryRunAction = "update"

	// DryRunActionDelete indicates a resource would be deleted.
	DryRunActionDelete DryRunAction = "delete"

	// DryRunActionPatch indicates a resource would be patched.
	DryRunActionPatch DryRunAction = "patch"

	// DryRunActionSkip indicates no action would be taken.
	DryRunActionSkip DryRunAction = "skip"
)

// DryRunChange represents a change that would be made.
type DryRunChange struct {
	// Action is the type of change.
	Action DryRunAction

	// Object is the object that would be affected.
	Object client.Object

	// Description is a human-readable description of the change.
	Description string

	// Diff contains the changes that would be made.
	Diff *ResourceDiff

	// Before is the state before the change (for updates).
	Before client.Object

	// After is the state after the change.
	After client.Object
}

// DryRunResult contains all changes that would be made.
type DryRunResult struct {
	// Changes is the list of changes that would be made.
	Changes []DryRunChange

	// Errors contains any errors that occurred during dry run.
	Errors []error

	// Summary is a human-readable summary of all changes.
	Summary string
}

// HasChanges returns true if any changes would be made.
func (r *DryRunResult) HasChanges() bool {
	return len(r.Changes) > 0
}

// HasErrors returns true if any errors occurred.
func (r *DryRunResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// CountByAction returns the count of changes by action type.
func (r *DryRunResult) CountByAction(action DryRunAction) int {
	count := 0
	for _, change := range r.Changes {
		if change.Action == action {
			count++
		}
	}
	return count
}

// DryRunRecorder records changes that would be made during dry run.
type DryRunRecorder interface {
	// RecordCreate records that a resource would be created.
	RecordCreate(obj client.Object, description string)

	// RecordUpdate records that a resource would be updated.
	RecordUpdate(before, after client.Object, description string)

	// RecordDelete records that a resource would be deleted.
	RecordDelete(obj client.Object, description string)

	// RecordPatch records that a resource would be patched.
	RecordPatch(obj client.Object, diff *ResourceDiff, description string)

	// RecordSkip records that no action would be taken.
	RecordSkip(obj client.Object, reason string)

	// RecordError records an error that occurred.
	RecordError(err error)

	// GetResult returns the accumulated dry run result.
	GetResult() *DryRunResult

	// Clear resets the recorder.
	Clear()
}

// dryRunRecorder implements DryRunRecorder.
type dryRunRecorder struct {
	changes []DryRunChange
	errors  []error
	mu      sync.RWMutex
}

// NewDryRunRecorder creates a new DryRunRecorder.
func NewDryRunRecorder() DryRunRecorder {
	return &dryRunRecorder{
		changes: make([]DryRunChange, 0),
		errors:  make([]error, 0),
	}
}

func (r *dryRunRecorder) RecordCreate(obj client.Object, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, DryRunChange{
		Action:      DryRunActionCreate,
		Object:      obj.DeepCopyObject().(client.Object),
		Description: description,
		After:       obj.DeepCopyObject().(client.Object),
	})
}

func (r *dryRunRecorder) RecordUpdate(before, after client.Object, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, DryRunChange{
		Action:      DryRunActionUpdate,
		Object:      after.DeepCopyObject().(client.Object),
		Description: description,
		Before:      before.DeepCopyObject().(client.Object),
		After:       after.DeepCopyObject().(client.Object),
	})
}

func (r *dryRunRecorder) RecordDelete(obj client.Object, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, DryRunChange{
		Action:      DryRunActionDelete,
		Object:      obj.DeepCopyObject().(client.Object),
		Description: description,
		Before:      obj.DeepCopyObject().(client.Object),
	})
}

func (r *dryRunRecorder) RecordPatch(obj client.Object, diff *ResourceDiff, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, DryRunChange{
		Action:      DryRunActionPatch,
		Object:      obj.DeepCopyObject().(client.Object),
		Description: description,
		Diff:        diff,
	})
}

func (r *dryRunRecorder) RecordSkip(obj client.Object, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, DryRunChange{
		Action:      DryRunActionSkip,
		Object:      obj.DeepCopyObject().(client.Object),
		Description: reason,
	})
}

func (r *dryRunRecorder) RecordError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errors = append(r.errors, err)
}

func (r *dryRunRecorder) GetResult() *DryRunResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := &DryRunResult{
		Changes: make([]DryRunChange, len(r.changes)),
		Errors:  make([]error, len(r.errors)),
	}
	copy(result.Changes, r.changes)
	copy(result.Errors, r.errors)
	result.Summary = r.generateSummary()
	return result
}

func (r *dryRunRecorder) generateSummary() string {
	creates := 0
	updates := 0
	deletes := 0
	skips := 0

	for _, change := range r.changes {
		switch change.Action {
		case DryRunActionCreate:
			creates++
		case DryRunActionUpdate, DryRunActionPatch:
			updates++
		case DryRunActionDelete:
			deletes++
		case DryRunActionSkip:
			skips++
		}
	}

	return fmt.Sprintf("%d creates, %d updates, %d deletes, %d skipped, %d errors",
		creates, updates, deletes, skips, len(r.errors))
}

func (r *dryRunRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = r.changes[:0]
	r.errors = r.errors[:0]
}

// DryRunClient wraps a client to intercept mutations during dry run.
type DryRunClient struct {
	client.Client
	recorder DryRunRecorder
	readOnly bool
}

// NewDryRunClient creates a new dry run client.
func NewDryRunClient(c client.Client) *DryRunClient {
	return &DryRunClient{
		Client:   c,
		recorder: NewDryRunRecorder(),
		readOnly: true,
	}
}

// WithRecorder sets a custom recorder.
func (c *DryRunClient) WithRecorder(recorder DryRunRecorder) *DryRunClient {
	c.recorder = recorder
	return c
}

// AllowWrites allows the client to actually perform writes (use with caution).
func (c *DryRunClient) AllowWrites() *DryRunClient {
	c.readOnly = false
	return c
}

// GetRecorder returns the dry run recorder.
func (c *DryRunClient) GetRecorder() DryRunRecorder {
	return c.recorder
}

// GetResult returns the dry run result.
func (c *DryRunClient) GetResult() *DryRunResult {
	return c.recorder.GetResult()
}

// Create intercepts create operations.
func (c *DryRunClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	key := client.ObjectKeyFromObject(obj)
	c.recorder.RecordCreate(obj, fmt.Sprintf("Would create %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if c.readOnly {
		return nil
	}
	return c.Client.Create(ctx, obj, opts...)
}

// Update intercepts update operations.
func (c *DryRunClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	key := client.ObjectKeyFromObject(obj)

	// Get the current state for comparison
	existing := obj.DeepCopyObject().(client.Object)
	if err := c.Client.Get(ctx, key, existing); err != nil {
		c.recorder.RecordError(err)
		return err
	}

	c.recorder.RecordUpdate(existing, obj, fmt.Sprintf("Would update %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if c.readOnly {
		return nil
	}
	return c.Client.Update(ctx, obj, opts...)
}

// Delete intercepts delete operations.
func (c *DryRunClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	key := client.ObjectKeyFromObject(obj)
	c.recorder.RecordDelete(obj, fmt.Sprintf("Would delete %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if c.readOnly {
		return nil
	}
	return c.Client.Delete(ctx, obj, opts...)
}

// Patch intercepts patch operations.
func (c *DryRunClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	key := client.ObjectKeyFromObject(obj)
	c.recorder.RecordPatch(obj, nil, fmt.Sprintf("Would patch %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if c.readOnly {
		return nil
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// Status returns a dry run status writer.
func (c *DryRunClient) Status() client.StatusWriter {
	return &dryRunStatusWriter{
		StatusWriter: c.Client.Status(),
		recorder:     c.recorder,
		readOnly:     c.readOnly,
	}
}

type dryRunStatusWriter struct {
	client.StatusWriter
	recorder DryRunRecorder
	readOnly bool
}

func (w *dryRunStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	key := client.ObjectKeyFromObject(obj)
	w.recorder.RecordUpdate(nil, obj, fmt.Sprintf("Would update status of %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if w.readOnly {
		return nil
	}
	return w.StatusWriter.Update(ctx, obj, opts...)
}

func (w *dryRunStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	key := client.ObjectKeyFromObject(obj)
	w.recorder.RecordPatch(obj, nil, fmt.Sprintf("Would patch status of %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, key))

	if w.readOnly {
		return nil
	}
	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}

// IsDryRun checks if an object has dry run mode enabled.
func IsDryRun(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationDryRun] == "true"
}

// SetDryRun enables or disables dry run mode on an object.
func SetDryRun(obj client.Object, enabled bool) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	if enabled {
		annotations[AnnotationDryRun] = "true"
	} else {
		delete(annotations, AnnotationDryRun)
		delete(annotations, AnnotationDryRunResult)
	}

	obj.SetAnnotations(annotations)
}

// SetDryRunResult stores the dry run result on an object.
func SetDryRunResult(obj client.Object, result *DryRunResult) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[AnnotationDryRunResult] = result.Summary
	obj.SetAnnotations(annotations)
}

// GetDryRunResult retrieves the dry run result from an object.
func GetDryRunResult(obj client.Object) string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return ""
	}
	return annotations[AnnotationDryRunResult]
}

// DryRunResourceManager wraps a ResourceManager for dry run operations.
type DryRunResourceManager struct {
	manager  ResourceManager
	recorder DryRunRecorder
	readOnly bool
}

// NewDryRunResourceManager creates a new dry run resource manager.
func NewDryRunResourceManager(manager ResourceManager, recorder DryRunRecorder) *DryRunResourceManager {
	return &DryRunResourceManager{
		manager:  manager,
		recorder: recorder,
		readOnly: true,
	}
}

// Ensure records the ensure operation without actually performing it.
func (dm *DryRunResourceManager) Ensure(ctx context.Context, desired client.Object, opts ...EnsureOption) (ResourceResult, error) {
	key := client.ObjectKeyFromObject(desired)

	// Check if resource exists
	existing := desired.DeepCopyObject().(client.Object)
	exists, err := dm.manager.Exists(ctx, key, existing)
	if err != nil {
		dm.recorder.RecordError(err)
		return ResourceResult{}, err
	}

	if !exists {
		dm.recorder.RecordCreate(desired, fmt.Sprintf("Would create %s", key))
		return ResourceResult{
			Object:  desired,
			Action:  ResourceCreated,
			Changed: true,
		}, nil
	}

	// Compare with existing
	calc := NewDiffCalculator()
	diff, err := calc.Diff(existing, desired)
	if err != nil {
		dm.recorder.RecordError(err)
		return ResourceResult{}, err
	}

	if diff.HasChanges {
		dm.recorder.RecordUpdate(existing, desired, fmt.Sprintf("Would update %s: %s", key, diff.Summary))
		return ResourceResult{
			Object:  desired,
			Action:  ResourceUpdated,
			Changed: true,
			Diff:    diff.Summary,
		}, nil
	}

	dm.recorder.RecordSkip(existing, "No changes needed")
	return ResourceResult{
		Object:  existing,
		Action:  ResourceUnchanged,
		Changed: false,
	}, nil
}

// Delete records the delete operation without actually performing it.
func (dm *DryRunResourceManager) Delete(ctx context.Context, obj client.Object) (ResourceResult, error) {
	key := client.ObjectKeyFromObject(obj)

	exists, err := dm.manager.Exists(ctx, key, obj.DeepCopyObject().(client.Object))
	if err != nil {
		dm.recorder.RecordError(err)
		return ResourceResult{}, err
	}

	if !exists {
		dm.recorder.RecordSkip(obj, "Resource does not exist")
		return ResourceResult{
			Object:  obj,
			Action:  ResourceUnchanged,
			Changed: false,
		}, nil
	}

	dm.recorder.RecordDelete(obj, fmt.Sprintf("Would delete %s", key))
	return ResourceResult{
		Object:  obj,
		Action:  ResourceDeleted,
		Changed: true,
	}, nil
}

// Get delegates to the real manager (reads are allowed in dry run).
func (dm *DryRunResourceManager) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	return dm.manager.Get(ctx, key, obj)
}

// Exists delegates to the real manager.
func (dm *DryRunResourceManager) Exists(ctx context.Context, key types.NamespacedName, obj client.Object) (bool, error) {
	return dm.manager.Exists(ctx, key, obj)
}

// GetRecorder returns the dry run recorder.
func (dm *DryRunResourceManager) GetRecorder() DryRunRecorder {
	return dm.recorder
}

// GetResult returns the dry run result.
func (dm *DryRunResourceManager) GetResult() *DryRunResult {
	return dm.recorder.GetResult()
}
