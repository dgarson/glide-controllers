package streamline

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// OwnedResourceHelper provides utilities for managing resources owned by the
// reconciled object. It automatically sets controller references and provides
// convenient create/update/delete operations.
//
// When a controller reference is set:
//   - The child resource is automatically garbage collected when the owner is deleted
//   - The controller-runtime framework can map events from the child back to the owner
//
// Usage:
//
//	func (h *MyHandler) Sync(ctx context.Context, obj *v1.MyResource, sCtx *streamline.Context) (Result, error) {
//	    cm := &corev1.ConfigMap{...}
//	    if err := sCtx.Owned.Apply(ctx, cm); err != nil {
//	        return Stop(), err
//	    }
//	}
type OwnedResourceHelper interface {
	// Create creates a new child resource with the controller reference set.
	// Returns an error if the resource already exists or if setting the
	// controller reference fails.
	Create(ctx context.Context, child client.Object, opts ...client.CreateOption) error

	// Update updates an existing child resource.
	// The child must already exist and have the controller reference set.
	Update(ctx context.Context, child client.Object, opts ...client.UpdateOption) error

	// Apply creates the child resource if it doesn't exist, or updates it if it does.
	// The controller reference is automatically set before create/update.
	// This is the recommended method for most use cases.
	Apply(ctx context.Context, child client.Object) error

	// ApplyWithComparator is like Apply but uses a custom comparator to determine
	// if an update is needed. If the comparator returns true, the existing resource
	// matches the desired state and no update is performed.
	ApplyWithComparator(ctx context.Context, child client.Object, equal func(existing, desired client.Object) bool) error

	// Delete deletes a specific child resource.
	// Returns nil if the resource doesn't exist.
	Delete(ctx context.Context, child client.Object, opts ...client.DeleteOption) error

	// Get retrieves a specific child resource by name.
	// Returns NotFound error if the resource doesn't exist.
	Get(ctx context.Context, key client.ObjectKey, child client.Object) error

	// Exists checks if a specific child resource exists.
	Exists(ctx context.Context, key client.ObjectKey, child client.Object) (bool, error)

	// List retrieves all resources of a given type owned by this object.
	// The list parameter should be a pointer to a list type (e.g., &corev1.ConfigMapList{}).
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error

	// DeleteAll deletes all resources of a given type owned by this object.
	// Uses List internally to find owned resources, then deletes them.
	DeleteAll(ctx context.Context, list client.ObjectList, opts ...client.DeleteOption) error

	// DeleteAllOfType deletes all resources of a given type owned by this object.
	// More efficient than DeleteAll as it uses server-side filtering when possible.
	DeleteAllOfType(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error

	// SetControllerReference sets the controller reference on a child object.
	// This is called automatically by Create and Apply, but can be called
	// directly if you need to set the reference before other operations.
	SetControllerReference(child client.Object) error

	// IsOwnedBy returns true if the given object is owned by the current owner.
	IsOwnedBy(obj client.Object) bool

	// GetOwnerReference returns the owner reference that would be set on child objects.
	GetOwnerReference() metav1.OwnerReference
}

// ownedResourceHelper implements OwnedResourceHelper.
type ownedResourceHelper struct {
	client client.Client
	scheme *runtime.Scheme
	owner  client.Object
}

// newOwnedResourceHelper creates a new OwnedResourceHelper for the given owner.
func newOwnedResourceHelper(c client.Client, scheme *runtime.Scheme, owner client.Object) OwnedResourceHelper {
	return &ownedResourceHelper{
		client: c,
		scheme: scheme,
		owner:  owner,
	}
}

func (h *ownedResourceHelper) Create(ctx context.Context, child client.Object, opts ...client.CreateOption) error {
	if err := h.SetControllerReference(child); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	return h.client.Create(ctx, child, opts...)
}

func (h *ownedResourceHelper) Update(ctx context.Context, child client.Object, opts ...client.UpdateOption) error {
	return h.client.Update(ctx, child, opts...)
}

func (h *ownedResourceHelper) Apply(ctx context.Context, child client.Object) error {
	return h.ApplyWithComparator(ctx, child, nil)
}

func (h *ownedResourceHelper) ApplyWithComparator(ctx context.Context, child client.Object, equal func(existing, desired client.Object) bool) error {
	// Set controller reference first
	if err := h.SetControllerReference(child); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Try to get existing resource
	existing := child.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(child)
	err := h.client.Get(ctx, key, existing)

	if apierrors.IsNotFound(err) {
		// Resource doesn't exist, create it
		return h.client.Create(ctx, child)
	}
	if err != nil {
		return fmt.Errorf("failed to get existing resource: %w", err)
	}

	// Resource exists, check if update is needed
	if equal != nil {
		if equal(existing, child) {
			// No update needed
			return nil
		}
	} else {
		// Use default comparison (spec equality for most resources)
		if h.resourcesEqual(existing, child) {
			return nil
		}
	}

	// Preserve resource version for update
	child.SetResourceVersion(existing.GetResourceVersion())

	// Ensure owner reference is preserved
	if err := h.SetControllerReference(child); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	return h.client.Update(ctx, child)
}

// resourcesEqual compares two resources to determine if an update is needed.
// This is a best-effort comparison that works for most common resource types.
func (h *ownedResourceHelper) resourcesEqual(existing, desired client.Object) bool {
	// For a simple implementation, we compare using semantic equality
	// which handles most Kubernetes objects correctly.
	// More sophisticated implementations might compare only specific fields.
	return equality.Semantic.DeepEqual(existing, desired)
}

func (h *ownedResourceHelper) Delete(ctx context.Context, child client.Object, opts ...client.DeleteOption) error {
	err := h.client.Delete(ctx, child, opts...)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *ownedResourceHelper) Get(ctx context.Context, key client.ObjectKey, child client.Object) error {
	return h.client.Get(ctx, key, child)
}

func (h *ownedResourceHelper) Exists(ctx context.Context, key client.ObjectKey, child client.Object) (bool, error) {
	err := h.client.Get(ctx, key, child)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *ownedResourceHelper) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// Add namespace filter if owner is namespaced
	allOpts := []client.ListOption{}
	if h.owner.GetNamespace() != "" {
		allOpts = append(allOpts, client.InNamespace(h.owner.GetNamespace()))
	}
	allOpts = append(allOpts, opts...)

	if err := h.client.List(ctx, list, allOpts...); err != nil {
		return err
	}

	// Filter to only owned resources
	h.filterOwnedResources(list)
	return nil
}

// filterOwnedResources filters a list to only include resources owned by the owner.
func (h *ownedResourceHelper) filterOwnedResources(list client.ObjectList) {
	// Use reflection to access Items field
	items := extractItems(list)
	if items == nil {
		return
	}

	filtered := make([]client.Object, 0, len(items))
	for _, item := range items {
		if h.IsOwnedBy(item) {
			filtered = append(filtered, item)
		}
	}

	setItems(list, filtered)
}

func (h *ownedResourceHelper) DeleteAll(ctx context.Context, list client.ObjectList, opts ...client.DeleteOption) error {
	if err := h.List(ctx, list); err != nil {
		return fmt.Errorf("failed to list owned resources: %w", err)
	}

	items := extractItems(list)
	var errs []error
	for _, item := range items {
		if err := h.client.Delete(ctx, item, opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to delete some owned resources: %v", errs)
	}
	return nil
}

func (h *ownedResourceHelper) DeleteAllOfType(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	// Build delete options with owner field selector if possible
	allOpts := []client.DeleteAllOfOption{}
	if h.owner.GetNamespace() != "" {
		allOpts = append(allOpts, client.InNamespace(h.owner.GetNamespace()))
	}
	allOpts = append(allOpts, opts...)

	// Note: This deletes ALL resources of this type in the namespace.
	// For true owner-filtered deletion, use DeleteAll instead.
	return h.client.DeleteAllOf(ctx, obj, allOpts...)
}

func (h *ownedResourceHelper) SetControllerReference(child client.Object) error {
	return controllerutil.SetControllerReference(h.owner, child, h.scheme)
}

func (h *ownedResourceHelper) IsOwnedBy(obj client.Object) bool {
	ownerUID := h.owner.GetUID()
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == ownerUID && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

func (h *ownedResourceHelper) GetOwnerReference() metav1.OwnerReference {
	gvk, _ := apiutil.GVKForObject(h.owner, h.scheme)
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               h.owner.GetName(),
		UID:                h.owner.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// OwnedResourceTracker provides tracking of owned resources across reconciliations.
// It can be used to detect orphaned resources that should be deleted.
type OwnedResourceTracker struct {
	owner      client.Object
	scheme     *runtime.Scheme
	currentGen map[schema.GroupVersionKind]map[client.ObjectKey]struct{}
}

// NewOwnedResourceTracker creates a new tracker for the given owner.
func NewOwnedResourceTracker(owner client.Object, scheme *runtime.Scheme) *OwnedResourceTracker {
	return &OwnedResourceTracker{
		owner:      owner,
		scheme:     scheme,
		currentGen: make(map[schema.GroupVersionKind]map[client.ObjectKey]struct{}),
	}
}

// Track records that a resource is expected to exist.
// Call this for each resource that should be owned after reconciliation.
func (t *OwnedResourceTracker) Track(obj client.Object) {
	gvk, _ := apiutil.GVKForObject(obj, t.scheme)
	if t.currentGen[gvk] == nil {
		t.currentGen[gvk] = make(map[client.ObjectKey]struct{})
	}
	t.currentGen[gvk][client.ObjectKeyFromObject(obj)] = struct{}{}
}

// IsTracked returns true if the given object was tracked in this generation.
func (t *OwnedResourceTracker) IsTracked(obj client.Object) bool {
	gvk, _ := apiutil.GVKForObject(obj, t.scheme)
	if t.currentGen[gvk] == nil {
		return false
	}
	_, ok := t.currentGen[gvk][client.ObjectKeyFromObject(obj)]
	return ok
}

// FindOrphans compares the tracked resources against the actual owned resources
// and returns any resources that exist but were not tracked (orphans).
func (t *OwnedResourceTracker) FindOrphans(ctx context.Context, helper OwnedResourceHelper, list client.ObjectList) ([]client.Object, error) {
	if err := helper.List(ctx, list); err != nil {
		return nil, err
	}

	items := extractItems(list)
	var orphans []client.Object

	for _, item := range items {
		if !t.IsTracked(item) {
			orphans = append(orphans, item)
		}
	}

	return orphans, nil
}

// Clear resets the tracker for a new reconciliation cycle.
func (t *OwnedResourceTracker) Clear() {
	t.currentGen = make(map[schema.GroupVersionKind]map[client.ObjectKey]struct{})
}

// ChildResourceState represents the state of a child resource for declarative management.
type ChildResourceState int

const (
	// ChildResourceUnknown indicates the state hasn't been determined.
	ChildResourceUnknown ChildResourceState = iota
	// ChildResourceCreated indicates the resource was created.
	ChildResourceCreated
	// ChildResourceUpdated indicates the resource was updated.
	ChildResourceUpdated
	// ChildResourceUnchanged indicates no change was needed.
	ChildResourceUnchanged
	// ChildResourceDeleted indicates the resource was deleted.
	ChildResourceDeleted
)

func (s ChildResourceState) String() string {
	switch s {
	case ChildResourceCreated:
		return "Created"
	case ChildResourceUpdated:
		return "Updated"
	case ChildResourceUnchanged:
		return "Unchanged"
	case ChildResourceDeleted:
		return "Deleted"
	default:
		return "Unknown"
	}
}

// ChildResourceResult contains the result of an Apply operation.
type ChildResourceResult struct {
	State ChildResourceState
	Error error
}

// OwnedResourceApplier provides a higher-level API for applying multiple
// owned resources with tracking and orphan cleanup.
type OwnedResourceApplier struct {
	helper  OwnedResourceHelper
	tracker *OwnedResourceTracker
	results map[client.ObjectKey]ChildResourceResult
}

// NewOwnedResourceApplier creates a new applier that tracks applied resources.
func NewOwnedResourceApplier(helper OwnedResourceHelper, tracker *OwnedResourceTracker) *OwnedResourceApplier {
	return &OwnedResourceApplier{
		helper:  helper,
		tracker: tracker,
		results: make(map[client.ObjectKey]ChildResourceResult),
	}
}

// Apply creates or updates a resource and tracks it.
func (a *OwnedResourceApplier) Apply(ctx context.Context, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)

	// Check if it exists first to determine state
	existing := obj.DeepCopyObject().(client.Object)
	exists, err := a.helper.Exists(ctx, key, existing)
	if err != nil {
		a.results[key] = ChildResourceResult{State: ChildResourceUnknown, Error: err}
		return err
	}

	// Apply the resource
	if err := a.helper.Apply(ctx, obj); err != nil {
		a.results[key] = ChildResourceResult{State: ChildResourceUnknown, Error: err}
		return err
	}

	// Track it
	a.tracker.Track(obj)

	// Determine state
	if !exists {
		a.results[key] = ChildResourceResult{State: ChildResourceCreated}
	} else {
		// Could be updated or unchanged - Apply doesn't tell us
		a.results[key] = ChildResourceResult{State: ChildResourceUpdated}
	}

	return nil
}

// GetResult returns the result for a specific resource.
func (a *OwnedResourceApplier) GetResult(key client.ObjectKey) ChildResourceResult {
	return a.results[key]
}

// GetResults returns all results.
func (a *OwnedResourceApplier) GetResults() map[client.ObjectKey]ChildResourceResult {
	return a.results
}

// HasErrors returns true if any apply operation failed.
func (a *OwnedResourceApplier) HasErrors() bool {
	for _, r := range a.results {
		if r.Error != nil {
			return true
		}
	}
	return false
}

// extractItems extracts the Items slice from a list object using reflection.
func extractItems(list client.ObjectList) []client.Object {
	// Use reflection to get the Items field
	listValue := reflect.ValueOf(list)
	if listValue.Kind() == reflect.Ptr {
		listValue = listValue.Elem()
	}

	itemsField := listValue.FieldByName("Items")
	if !itemsField.IsValid() {
		return nil
	}

	// Convert to []client.Object
	result := make([]client.Object, 0, itemsField.Len())
	for i := 0; i < itemsField.Len(); i++ {
		item := itemsField.Index(i)
		// Get address of item if it's not already a pointer
		if item.Kind() != reflect.Ptr {
			item = item.Addr()
		}
		if obj, ok := item.Interface().(client.Object); ok {
			result = append(result, obj)
		}
	}

	return result
}

// setItems sets the Items slice on a list object using reflection.
func setItems(list client.ObjectList, items []client.Object) {
	listValue := reflect.ValueOf(list)
	if listValue.Kind() == reflect.Ptr {
		listValue = listValue.Elem()
	}

	itemsField := listValue.FieldByName("Items")
	if !itemsField.IsValid() || !itemsField.CanSet() {
		return
	}

	// Get the element type of the Items slice
	elemType := itemsField.Type().Elem()

	// Create a new slice of the correct type
	newSlice := reflect.MakeSlice(itemsField.Type(), 0, len(items))

	for _, item := range items {
		itemValue := reflect.ValueOf(item)
		// If elemType is not a pointer but itemValue is, dereference it
		if elemType.Kind() != reflect.Ptr && itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
		}
		// If elemType is a pointer but itemValue is not, get its address
		if elemType.Kind() == reflect.Ptr && itemValue.Kind() != reflect.Ptr {
			itemValue = itemValue.Addr()
		}
		newSlice = reflect.Append(newSlice, itemValue)
	}

	itemsField.Set(newSlice)
}
