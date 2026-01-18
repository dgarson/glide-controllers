package streamline

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ResourceAction describes what operation was performed on a resource.
type ResourceAction string

const (
	// ResourceCreated indicates the resource was newly created.
	ResourceCreated ResourceAction = "created"

	// ResourceUpdated indicates the resource was updated.
	ResourceUpdated ResourceAction = "updated"

	// ResourceUnchanged indicates the resource already matched desired state.
	ResourceUnchanged ResourceAction = "unchanged"

	// ResourceDeleted indicates the resource was deleted.
	ResourceDeleted ResourceAction = "deleted"

	// ResourceAdopted indicates an existing resource was adopted by adding owner reference.
	ResourceAdopted ResourceAction = "adopted"

	// ResourceOrphaned indicates the owner reference was removed from the resource.
	ResourceOrphaned ResourceAction = "orphaned"
)

// ResourceResult represents the outcome of a resource operation.
type ResourceResult struct {
	// Object is the resource that was operated on.
	Object client.Object

	// Action describes what operation was performed.
	Action ResourceAction

	// Changed indicates whether any modifications were made.
	Changed bool

	// Diff contains a human-readable description of changes made.
	// Empty if no changes were made or if diff generation failed.
	Diff string
}

// ResourceManager handles the lifecycle of child/owned resources.
// It provides a high-level abstraction for creating, updating, and deleting
// resources with proper owner references for garbage collection.
//
// All operations automatically set owner references and manage resource
// lifecycle according to Kubernetes garbage collection semantics.
type ResourceManager interface {
	// Ensure creates the resource if it doesn't exist, or updates it if it does.
	// Owner references are automatically set on the resource.
	// The comparison uses the resource's spec to determine if an update is needed.
	//
	// Options can be used to customize behavior:
	//   - WithMergeLabels: Merge labels instead of replacing
	//   - WithMergeAnnotations: Merge annotations instead of replacing
	//   - WithUpdateStrategy: Specify how updates should be performed
	//
	// Example:
	//   result, err := sCtx.Resources.Ensure(ctx, deployment)
	//   if err != nil {
	//       return streamline.Requeue(), err
	//   }
	//   if result.Action == streamline.ResourceCreated {
	//       sCtx.Event.Normal("Created", "Created deployment")
	//   }
	Ensure(ctx context.Context, desired client.Object, opts ...EnsureOption) (ResourceResult, error)

	// EnsureAll creates or updates multiple resources in a single call.
	// Resources are processed in order, and the function returns on the first error.
	// For parallel processing, use EnsureAllParallel.
	EnsureAll(ctx context.Context, resources []client.Object, opts ...EnsureOption) ([]ResourceResult, error)

	// EnsureOwned ensures the resource exists with the owner reference, but does not
	// update the resource if it already exists. Use this for resources that may be
	// modified by other controllers or users.
	EnsureOwned(ctx context.Context, desired client.Object) (ResourceResult, error)

	// Delete removes the resource if it exists.
	// Returns ResourceDeleted if the resource was deleted, ResourceUnchanged if it didn't exist.
	Delete(ctx context.Context, obj client.Object) (ResourceResult, error)

	// DeleteIfExists is an alias for Delete that makes the intent clearer.
	DeleteIfExists(ctx context.Context, obj client.Object) (ResourceResult, error)

	// DeleteAll removes multiple resources.
	DeleteAll(ctx context.Context, resources []client.Object) ([]ResourceResult, error)

	// List returns all resources of the given type owned by the parent.
	// The list parameter must be a pointer to a List type (e.g., &appsv1.DeploymentList{}).
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error

	// ListOwned returns all resources of the given type owned by the parent resource.
	// This is a convenience wrapper around List that adds an owner reference filter.
	ListOwned(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error

	// Adopt claims an existing resource by adding an owner reference.
	// The resource must exist. Use this for "adopting" pre-existing resources.
	Adopt(ctx context.Context, obj client.Object) (ResourceResult, error)

	// Orphan removes the owner reference from a resource without deleting it.
	// The resource continues to exist but will no longer be garbage collected
	// when the parent is deleted.
	Orphan(ctx context.Context, obj client.Object) (ResourceResult, error)

	// Get retrieves a resource by name. Returns an error if not found.
	Get(ctx context.Context, key types.NamespacedName, obj client.Object) error

	// Exists checks if a resource exists.
	Exists(ctx context.Context, key types.NamespacedName, obj client.Object) (bool, error)

	// IsOwned checks if the resource has an owner reference to the parent.
	IsOwned(obj client.Object) bool

	// GetOwnerReference returns the owner reference for the parent resource.
	GetOwnerReference() metav1.OwnerReference
}

// UpdateStrategy defines how updates should be performed.
type UpdateStrategy int

const (
	// UpdateStrategyReplace replaces the entire spec (default).
	UpdateStrategyReplace UpdateStrategy = iota

	// UpdateStrategyMerge merges the desired spec with the existing spec.
	UpdateStrategyMerge

	// UpdateStrategyServerSideApply uses server-side apply for updates.
	UpdateStrategyServerSideApply
)

// EnsureOption configures behavior of the Ensure operation.
type EnsureOption func(*ensureOptions)

type ensureOptions struct {
	mergeLabels      bool
	mergeAnnotations bool
	updateStrategy   UpdateStrategy
	fieldManager     string
	forceConflicts   bool
	ignoreFields     []string
}

// WithMergeLabels merges labels from the desired object with existing labels,
// rather than replacing them entirely.
func WithMergeLabels() EnsureOption {
	return func(o *ensureOptions) {
		o.mergeLabels = true
	}
}

// WithMergeAnnotations merges annotations from the desired object with existing
// annotations, rather than replacing them entirely.
func WithMergeAnnotations() EnsureOption {
	return func(o *ensureOptions) {
		o.mergeAnnotations = true
	}
}

// WithUpdateStrategy sets the update strategy.
func WithUpdateStrategy(strategy UpdateStrategy) EnsureOption {
	return func(o *ensureOptions) {
		o.updateStrategy = strategy
	}
}

// WithFieldManager sets the field manager name for server-side apply.
func WithFieldManager(manager string) EnsureOption {
	return func(o *ensureOptions) {
		o.fieldManager = manager
	}
}

// WithForceConflicts forces conflicts during server-side apply.
func WithForceConflicts() EnsureOption {
	return func(o *ensureOptions) {
		o.forceConflicts = true
	}
}

// WithIgnoreFields specifies fields to ignore when comparing resources.
// Use JSON path notation (e.g., ".spec.replicas", ".metadata.annotations").
func WithIgnoreFields(fields ...string) EnsureOption {
	return func(o *ensureOptions) {
		o.ignoreFields = append(o.ignoreFields, fields...)
	}
}

// resourceManager implements ResourceManager.
type resourceManager struct {
	client client.Client
	scheme *runtime.Scheme
	owner  client.Object
	log    logr.Logger
}

// NewResourceManager creates a new ResourceManager for managing owned resources.
// The owner is the parent resource that will own all managed resources.
func NewResourceManager(c client.Client, scheme *runtime.Scheme, owner client.Object, log logr.Logger) ResourceManager {
	return &resourceManager{
		client: c,
		scheme: scheme,
		owner:  owner,
		log:    log.WithName("resources"),
	}
}

func (rm *resourceManager) Ensure(ctx context.Context, desired client.Object, opts ...EnsureOption) (ResourceResult, error) {
	options := &ensureOptions{
		updateStrategy: UpdateStrategyReplace,
		fieldManager:   "streamline",
	}
	for _, opt := range opts {
		opt(options)
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rm.owner, desired, rm.scheme); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	key := client.ObjectKeyFromObject(desired)
	log := rm.log.WithValues("kind", desired.GetObjectKind().GroupVersionKind().Kind, "name", key.Name, "namespace", key.Namespace)

	// Try to get existing resource
	existing := desired.DeepCopyObject().(client.Object)
	err := rm.client.Get(ctx, key, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new resource
			log.V(1).Info("creating resource")
			if err := rm.client.Create(ctx, desired); err != nil {
				return ResourceResult{}, fmt.Errorf("failed to create resource: %w", err)
			}
			return ResourceResult{
				Object:  desired,
				Action:  ResourceCreated,
				Changed: true,
			}, nil
		}
		return ResourceResult{}, fmt.Errorf("failed to get resource: %w", err)
	}

	// Resource exists, check if update is needed
	needsUpdate, diff := rm.needsUpdate(existing, desired, options)
	if !needsUpdate {
		log.V(2).Info("resource is up to date")
		return ResourceResult{
			Object:  existing,
			Action:  ResourceUnchanged,
			Changed: false,
		}, nil
	}

	// Perform update based on strategy
	log.V(1).Info("updating resource", "diff", diff)
	if err := rm.performUpdate(ctx, existing, desired, options); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to update resource: %w", err)
	}

	return ResourceResult{
		Object:  desired,
		Action:  ResourceUpdated,
		Changed: true,
		Diff:    diff,
	}, nil
}

func (rm *resourceManager) EnsureAll(ctx context.Context, resources []client.Object, opts ...EnsureOption) ([]ResourceResult, error) {
	results := make([]ResourceResult, 0, len(resources))
	for _, res := range resources {
		result, err := rm.Ensure(ctx, res, opts...)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (rm *resourceManager) EnsureOwned(ctx context.Context, desired client.Object) (ResourceResult, error) {
	// Set owner reference
	if err := controllerutil.SetControllerReference(rm.owner, desired, rm.scheme); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	key := client.ObjectKeyFromObject(desired)
	log := rm.log.WithValues("kind", desired.GetObjectKind().GroupVersionKind().Kind, "name", key.Name, "namespace", key.Namespace)

	// Try to get existing resource
	existing := desired.DeepCopyObject().(client.Object)
	err := rm.client.Get(ctx, key, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new resource
			log.V(1).Info("creating owned resource")
			if err := rm.client.Create(ctx, desired); err != nil {
				return ResourceResult{}, fmt.Errorf("failed to create resource: %w", err)
			}
			return ResourceResult{
				Object:  desired,
				Action:  ResourceCreated,
				Changed: true,
			}, nil
		}
		return ResourceResult{}, fmt.Errorf("failed to get resource: %w", err)
	}

	// Resource exists, don't update
	return ResourceResult{
		Object:  existing,
		Action:  ResourceUnchanged,
		Changed: false,
	}, nil
}

func (rm *resourceManager) Delete(ctx context.Context, obj client.Object) (ResourceResult, error) {
	key := client.ObjectKeyFromObject(obj)
	log := rm.log.WithValues("kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", key.Name, "namespace", key.Namespace)

	err := rm.client.Delete(ctx, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ResourceResult{
				Object:  obj,
				Action:  ResourceUnchanged,
				Changed: false,
			}, nil
		}
		return ResourceResult{}, fmt.Errorf("failed to delete resource: %w", err)
	}

	log.V(1).Info("deleted resource")
	return ResourceResult{
		Object:  obj,
		Action:  ResourceDeleted,
		Changed: true,
	}, nil
}

func (rm *resourceManager) DeleteIfExists(ctx context.Context, obj client.Object) (ResourceResult, error) {
	return rm.Delete(ctx, obj)
}

func (rm *resourceManager) DeleteAll(ctx context.Context, resources []client.Object) ([]ResourceResult, error) {
	results := make([]ResourceResult, 0, len(resources))
	for _, res := range resources {
		result, err := rm.Delete(ctx, res)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (rm *resourceManager) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return rm.client.List(ctx, list, opts...)
}

func (rm *resourceManager) ListOwned(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// Add owner field selector if available, otherwise filter client-side
	allOpts := append(opts, client.InNamespace(rm.owner.GetNamespace()))
	if err := rm.client.List(ctx, list, allOpts...); err != nil {
		return err
	}

	// Filter to only owned resources
	rm.filterOwned(list)
	return nil
}

func (rm *resourceManager) Adopt(ctx context.Context, obj client.Object) (ResourceResult, error) {
	key := client.ObjectKeyFromObject(obj)
	log := rm.log.WithValues("kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", key.Name, "namespace", key.Namespace)

	// Fetch current state
	if err := rm.client.Get(ctx, key, obj); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to get resource: %w", err)
	}

	// Check if already owned
	if rm.IsOwned(obj) {
		return ResourceResult{
			Object:  obj,
			Action:  ResourceUnchanged,
			Changed: false,
		}, nil
	}

	// Add owner reference
	if err := controllerutil.SetControllerReference(rm.owner, obj, rm.scheme); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := rm.client.Update(ctx, obj); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to update resource: %w", err)
	}

	log.V(1).Info("adopted resource")
	return ResourceResult{
		Object:  obj,
		Action:  ResourceAdopted,
		Changed: true,
	}, nil
}

func (rm *resourceManager) Orphan(ctx context.Context, obj client.Object) (ResourceResult, error) {
	key := client.ObjectKeyFromObject(obj)
	log := rm.log.WithValues("kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", key.Name, "namespace", key.Namespace)

	// Fetch current state
	if err := rm.client.Get(ctx, key, obj); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to get resource: %w", err)
	}

	// Check if owned
	if !rm.IsOwned(obj) {
		return ResourceResult{
			Object:  obj,
			Action:  ResourceUnchanged,
			Changed: false,
		}, nil
	}

	// Remove owner reference
	ownerRefs := obj.GetOwnerReferences()
	newOwnerRefs := make([]metav1.OwnerReference, 0, len(ownerRefs))
	ownerUID := rm.owner.GetUID()
	for _, ref := range ownerRefs {
		if ref.UID != ownerUID {
			newOwnerRefs = append(newOwnerRefs, ref)
		}
	}
	obj.SetOwnerReferences(newOwnerRefs)

	if err := rm.client.Update(ctx, obj); err != nil {
		return ResourceResult{}, fmt.Errorf("failed to update resource: %w", err)
	}

	log.V(1).Info("orphaned resource")
	return ResourceResult{
		Object:  obj,
		Action:  ResourceOrphaned,
		Changed: true,
	}, nil
}

func (rm *resourceManager) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	return rm.client.Get(ctx, key, obj)
}

func (rm *resourceManager) Exists(ctx context.Context, key types.NamespacedName, obj client.Object) (bool, error) {
	err := rm.client.Get(ctx, key, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (rm *resourceManager) IsOwned(obj client.Object) bool {
	ownerUID := rm.owner.GetUID()
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == ownerUID {
			return true
		}
	}
	return false
}

func (rm *resourceManager) GetOwnerReference() metav1.OwnerReference {
	gvk := rm.owner.GetObjectKind().GroupVersionKind()
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               rm.owner.GetName(),
		UID:                rm.owner.GetUID(),
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}
}

// needsUpdate compares existing and desired resources to determine if an update is needed.
func (rm *resourceManager) needsUpdate(existing, desired client.Object, opts *ensureOptions) (bool, string) {
	// Compare using JSON representation of spec
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return true, "failed to marshal existing"
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return true, "failed to marshal desired"
	}

	var existingMap, desiredMap map[string]interface{}
	if err := json.Unmarshal(existingJSON, &existingMap); err != nil {
		return true, "failed to unmarshal existing"
	}
	if err := json.Unmarshal(desiredJSON, &desiredMap); err != nil {
		return true, "failed to unmarshal desired"
	}

	// Remove metadata fields that shouldn't affect comparison
	cleanMetadata(existingMap)
	cleanMetadata(desiredMap)

	// Compare specs
	existingSpec, _ := existingMap["spec"]
	desiredSpec, _ := desiredMap["spec"]

	if !reflect.DeepEqual(existingSpec, desiredSpec) {
		return true, generateDiff(existingSpec, desiredSpec)
	}

	// Compare labels if not merging
	if !opts.mergeLabels {
		existingLabels := existing.GetLabels()
		desiredLabels := desired.GetLabels()
		if !reflect.DeepEqual(existingLabels, desiredLabels) {
			return true, "labels differ"
		}
	}

	// Compare annotations if not merging
	if !opts.mergeAnnotations {
		existingAnnotations := existing.GetAnnotations()
		desiredAnnotations := desired.GetAnnotations()
		if !reflect.DeepEqual(existingAnnotations, desiredAnnotations) {
			return true, "annotations differ"
		}
	}

	return false, ""
}

// performUpdate applies the update to the resource.
func (rm *resourceManager) performUpdate(ctx context.Context, existing, desired client.Object, opts *ensureOptions) error {
	// Copy resource version for optimistic locking
	desired.SetResourceVersion(existing.GetResourceVersion())
	desired.SetUID(existing.GetUID())
	desired.SetCreationTimestamp(existing.GetCreationTimestamp())

	// Merge labels if requested
	if opts.mergeLabels {
		mergedLabels := existing.GetLabels()
		if mergedLabels == nil {
			mergedLabels = make(map[string]string)
		}
		for k, v := range desired.GetLabels() {
			mergedLabels[k] = v
		}
		desired.SetLabels(mergedLabels)
	}

	// Merge annotations if requested
	if opts.mergeAnnotations {
		mergedAnnotations := existing.GetAnnotations()
		if mergedAnnotations == nil {
			mergedAnnotations = make(map[string]string)
		}
		for k, v := range desired.GetAnnotations() {
			mergedAnnotations[k] = v
		}
		desired.SetAnnotations(mergedAnnotations)
	}

	switch opts.updateStrategy {
	case UpdateStrategyServerSideApply:
		patchOpts := []client.PatchOption{
			client.FieldOwner(opts.fieldManager),
		}
		if opts.forceConflicts {
			patchOpts = append(patchOpts, client.ForceOwnership)
		}
		return rm.client.Patch(ctx, desired, client.Apply, patchOpts...)
	case UpdateStrategyMerge:
		patch := client.MergeFrom(existing)
		return rm.client.Patch(ctx, desired, patch)
	default:
		return rm.client.Update(ctx, desired)
	}
}

// filterOwned removes non-owned resources from the list.
func (rm *resourceManager) filterOwned(list client.ObjectList) {
	items := reflect.ValueOf(list).Elem().FieldByName("Items")
	if !items.IsValid() {
		return
	}

	ownerUID := rm.owner.GetUID()
	filtered := reflect.MakeSlice(items.Type(), 0, items.Len())

	for i := 0; i < items.Len(); i++ {
		item := items.Index(i)
		var obj client.Object
		if item.Kind() == reflect.Ptr {
			obj = item.Interface().(client.Object)
		} else {
			obj = item.Addr().Interface().(client.Object)
		}

		for _, ref := range obj.GetOwnerReferences() {
			if ref.UID == ownerUID {
				filtered = reflect.Append(filtered, item)
				break
			}
		}
	}

	items.Set(filtered)
}

// cleanMetadata removes metadata fields that shouldn't affect comparison.
func cleanMetadata(m map[string]interface{}) {
	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")
		delete(metadata, "creationTimestamp")
		delete(metadata, "generation")
		delete(metadata, "managedFields")
		delete(metadata, "selfLink")
	}
	// Remove status from comparison
	delete(m, "status")
}

// generateDiff creates a human-readable diff between two values.
func generateDiff(existing, desired interface{}) string {
	existingJSON, _ := json.MarshalIndent(existing, "", "  ")
	desiredJSON, _ := json.MarshalIndent(desired, "", "  ")

	if len(existingJSON) > 500 || len(desiredJSON) > 500 {
		return "spec differs (diff too large to display)"
	}

	return fmt.Sprintf("existing: %s, desired: %s", string(existingJSON), string(desiredJSON))
}

func boolPtr(b bool) *bool {
	return &b
}

// ResourceSet manages a collection of related resources for batch operations.
// It provides convenient methods for managing multiple resources together.
type ResourceSet struct {
	resources []client.Object
	manager   ResourceManager
}

// NewResourceSet creates a new ResourceSet with the given manager.
func NewResourceSet(manager ResourceManager) *ResourceSet {
	return &ResourceSet{
		manager:   manager,
		resources: make([]client.Object, 0),
	}
}

// Add adds a resource to the set.
func (rs *ResourceSet) Add(obj client.Object) *ResourceSet {
	rs.resources = append(rs.resources, obj)
	return rs
}

// AddAll adds multiple resources to the set.
func (rs *ResourceSet) AddAll(objs ...client.Object) *ResourceSet {
	rs.resources = append(rs.resources, objs...)
	return rs
}

// Count returns the number of resources in the set.
func (rs *ResourceSet) Count() int {
	return len(rs.resources)
}

// EnsureAll creates or updates all resources in the set.
func (rs *ResourceSet) EnsureAll(ctx context.Context, opts ...EnsureOption) ([]ResourceResult, error) {
	return rs.manager.EnsureAll(ctx, rs.resources, opts...)
}

// DeleteAll deletes all resources in the set.
func (rs *ResourceSet) DeleteAll(ctx context.Context) ([]ResourceResult, error) {
	return rs.manager.DeleteAll(ctx, rs.resources)
}

// Resources returns the resources in the set.
func (rs *ResourceSet) Resources() []client.Object {
	return rs.resources
}

// DiffCalculator computes differences between resources for logging and debugging.
type DiffCalculator interface {
	// Diff computes the difference between two resources.
	Diff(existing, desired client.Object) (*ResourceDiff, error)

	// HasDrift checks if the actual state differs from desired state.
	HasDrift(existing, desired client.Object) bool
}

// ResourceDiff represents the differences between two resources.
type ResourceDiff struct {
	// HasChanges indicates whether there are any differences.
	HasChanges bool

	// Fields contains the individual field differences.
	Fields []FieldDiff

	// Summary is a human-readable summary of the changes.
	Summary string
}

// FieldDiff represents a difference in a single field.
type FieldDiff struct {
	// Path is the JSON path to the field (e.g., ".spec.replicas").
	Path string

	// Existing is the current value.
	Existing interface{}

	// Desired is the desired value.
	Desired interface{}
}

// diffCalculator implements DiffCalculator.
type diffCalculator struct{}

// NewDiffCalculator creates a new DiffCalculator.
func NewDiffCalculator() DiffCalculator {
	return &diffCalculator{}
}

func (dc *diffCalculator) Diff(existing, desired client.Object) (*ResourceDiff, error) {
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal existing: %w", err)
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal desired: %w", err)
	}

	var existingMap, desiredMap map[string]interface{}
	if err := json.Unmarshal(existingJSON, &existingMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal existing: %w", err)
	}
	if err := json.Unmarshal(desiredJSON, &desiredMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Remove fields that shouldn't be compared
	cleanMetadata(existingMap)
	cleanMetadata(desiredMap)

	fields := dc.compareRecursive("", existingMap, desiredMap)

	return &ResourceDiff{
		HasChanges: len(fields) > 0,
		Fields:     fields,
		Summary:    dc.generateSummary(fields),
	}, nil
}

func (dc *diffCalculator) HasDrift(existing, desired client.Object) bool {
	diff, err := dc.Diff(existing, desired)
	if err != nil {
		return true // Assume drift on error
	}
	return diff.HasChanges
}

func (dc *diffCalculator) compareRecursive(prefix string, existing, desired map[string]interface{}) []FieldDiff {
	var diffs []FieldDiff

	// Check all keys in desired
	for key, desiredVal := range desired {
		path := prefix + "." + key
		existingVal, exists := existing[key]

		if !exists {
			diffs = append(diffs, FieldDiff{
				Path:     path,
				Existing: nil,
				Desired:  desiredVal,
			})
			continue
		}

		if !reflect.DeepEqual(existingVal, desiredVal) {
			// If both are maps, recurse
			existingMap, existingIsMap := existingVal.(map[string]interface{})
			desiredMap, desiredIsMap := desiredVal.(map[string]interface{})
			if existingIsMap && desiredIsMap {
				diffs = append(diffs, dc.compareRecursive(path, existingMap, desiredMap)...)
			} else {
				diffs = append(diffs, FieldDiff{
					Path:     path,
					Existing: existingVal,
					Desired:  desiredVal,
				})
			}
		}
	}

	// Check for keys in existing but not in desired
	for key, existingVal := range existing {
		if _, exists := desired[key]; !exists {
			diffs = append(diffs, FieldDiff{
				Path:     prefix + "." + key,
				Existing: existingVal,
				Desired:  nil,
			})
		}
	}

	return diffs
}

func (dc *diffCalculator) generateSummary(fields []FieldDiff) string {
	if len(fields) == 0 {
		return "no changes"
	}
	if len(fields) == 1 {
		return fmt.Sprintf("1 field changed: %s", fields[0].Path)
	}
	return fmt.Sprintf("%d fields changed", len(fields))
}
