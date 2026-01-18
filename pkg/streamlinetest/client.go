package streamlinetest

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeClient is a simple in-memory client for testing.
// It supports basic CRUD operations and allows configuring errors.
type FakeClient struct {
	mu      sync.RWMutex
	objects map[objectKey]client.Object
	scheme  *runtime.Scheme

	// Error injection
	getError    error
	createError error
	updateError error
	deleteError error
	listError   error
	patchError  error

	// Tracking
	getCalls    []client.ObjectKey
	createCalls []client.Object
	updateCalls []client.Object
	deleteCalls []client.Object
}

type objectKey struct {
	gvk       schema.GroupVersionKind
	namespace string
	name      string
}

// NewFakeClient creates a new FakeClient with optional initial objects.
func NewFakeClient(objs ...client.Object) *FakeClient {
	fc := &FakeClient{
		objects:     make(map[objectKey]client.Object),
		scheme:      runtime.NewScheme(),
		getCalls:    make([]client.ObjectKey, 0),
		createCalls: make([]client.Object, 0),
		updateCalls: make([]client.Object, 0),
		deleteCalls: make([]client.Object, 0),
	}

	for _, obj := range objs {
		fc.addObject(obj)
	}

	return fc
}

// addObject adds an object to the fake store.
func (c *FakeClient) addObject(obj client.Object) {
	key := c.keyFor(obj)
	c.objects[key] = obj.DeepCopyObject().(client.Object)
}

// keyFor returns the objectKey for an object.
func (c *FakeClient) keyFor(obj client.Object) objectKey {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		// Try to get GVK from type
		gvk = gvkFromType(obj)
	}
	return objectKey{
		gvk:       gvk,
		namespace: obj.GetNamespace(),
		name:      obj.GetName(),
	}
}

// gvkFromType attempts to determine GVK from the Go type.
func gvkFromType(obj client.Object) schema.GroupVersionKind {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return schema.GroupVersionKind{
		Kind: t.Name(),
	}
}

// Get retrieves an object.
func (c *FakeClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.getCalls = append(c.getCalls, key)

	if c.getError != nil {
		return c.getError
	}

	objKey := objectKey{
		gvk:       gvkFromType(obj),
		namespace: key.Namespace,
		name:      key.Name,
	}

	stored, ok := c.objects[objKey]
	if !ok {
		return apierrors.NewNotFound(
			schema.GroupResource{Group: objKey.gvk.Group, Resource: objKey.gvk.Kind},
			key.Name,
		)
	}

	// Copy stored object to the provided object
	copyInto(stored, obj)
	return nil
}

// List retrieves a list of objects.
func (c *FakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.listError != nil {
		return c.listError
	}

	// Get list options
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	// Get the GVK for the list's item type
	itemGVK := gvkFromListType(list)

	// Collect matching objects
	var items []client.Object
	for key, obj := range c.objects {
		// Check GVK match
		if key.gvk.Kind != itemGVK.Kind {
			continue
		}

		// Check namespace filter
		if listOpts.Namespace != "" && key.namespace != listOpts.Namespace {
			continue
		}

		items = append(items, obj.DeepCopyObject().(client.Object))
	}

	// Set items on the list
	setListItems(list, items)
	return nil
}

// Create creates an object.
func (c *FakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.createCalls = append(c.createCalls, obj)

	if c.createError != nil {
		return c.createError
	}

	key := c.keyFor(obj)
	if _, exists := c.objects[key]; exists {
		return apierrors.NewAlreadyExists(
			schema.GroupResource{Group: key.gvk.Group, Resource: key.gvk.Kind},
			obj.GetName(),
		)
	}

	// Set resource version
	obj.SetResourceVersion("1")
	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// Delete deletes an object.
func (c *FakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteCalls = append(c.deleteCalls, obj)

	if c.deleteError != nil {
		return c.deleteError
	}

	key := c.keyFor(obj)
	if _, exists := c.objects[key]; !exists {
		return apierrors.NewNotFound(
			schema.GroupResource{Group: key.gvk.Group, Resource: key.gvk.Kind},
			obj.GetName(),
		)
	}

	delete(c.objects, key)
	return nil
}

// Update updates an object.
func (c *FakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.updateCalls = append(c.updateCalls, obj)

	if c.updateError != nil {
		return c.updateError
	}

	key := c.keyFor(obj)
	if _, exists := c.objects[key]; !exists {
		return apierrors.NewNotFound(
			schema.GroupResource{Group: key.gvk.Group, Resource: key.gvk.Kind},
			obj.GetName(),
		)
	}

	// Increment resource version
	rv := obj.GetResourceVersion()
	if rv == "" {
		rv = "0"
	}
	obj.SetResourceVersion(fmt.Sprintf("%s+", rv))
	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// Patch patches an object.
func (c *FakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.patchError != nil {
		return c.patchError
	}

	key := c.keyFor(obj)
	if _, exists := c.objects[key]; !exists {
		return apierrors.NewNotFound(
			schema.GroupResource{Group: key.gvk.Group, Resource: key.gvk.Kind},
			obj.GetName(),
		)
	}

	// For simplicity, just replace the object
	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// DeleteAllOf deletes all objects of a type.
func (c *FakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.deleteError != nil {
		return c.deleteError
	}

	gvk := gvkFromType(obj)
	deleteOpts := &client.DeleteAllOfOptions{}
	for _, opt := range opts {
		opt.ApplyToDeleteAllOf(deleteOpts)
	}

	for key := range c.objects {
		if key.gvk.Kind == gvk.Kind {
			if deleteOpts.Namespace == "" || key.namespace == deleteOpts.Namespace {
				delete(c.objects, key)
			}
		}
	}

	return nil
}

// Apply implements server-side apply.
func (c *FakeClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	// For testing purposes, Apply is a no-op since we don't have server-side apply support
	// Most tests won't use this method directly
	return nil
}

// Status returns the status writer.
func (c *FakeClient) Status() client.SubResourceWriter {
	return &fakeStatusWriter{client: c}
}

// SubResource returns a subresource client.
func (c *FakeClient) SubResource(subResource string) client.SubResourceClient {
	return &fakeSubResourceClient{client: c, subResource: subResource}
}

// Scheme returns the scheme.
func (c *FakeClient) Scheme() *runtime.Scheme {
	return c.scheme
}

// RESTMapper returns nil (not implemented).
func (c *FakeClient) RESTMapper() meta.RESTMapper {
	return nil
}

// GroupVersionKindFor returns the GVK for an object.
func (c *FakeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return gvkFromType(obj.(client.Object)), nil
}

// IsObjectNamespaced returns whether an object is namespaced.
func (c *FakeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

// Error injection methods

// WithGetError configures an error to return on Get calls.
func (c *FakeClient) WithGetError(err error) *FakeClient {
	c.getError = err
	return c
}

// WithCreateError configures an error to return on Create calls.
func (c *FakeClient) WithCreateError(err error) *FakeClient {
	c.createError = err
	return c
}

// WithUpdateError configures an error to return on Update calls.
func (c *FakeClient) WithUpdateError(err error) *FakeClient {
	c.updateError = err
	return c
}

// WithDeleteError configures an error to return on Delete calls.
func (c *FakeClient) WithDeleteError(err error) *FakeClient {
	c.deleteError = err
	return c
}

// WithListError configures an error to return on List calls.
func (c *FakeClient) WithListError(err error) *FakeClient {
	c.listError = err
	return c
}

// WithPatchError configures an error to return on Patch calls.
func (c *FakeClient) WithPatchError(err error) *FakeClient {
	c.patchError = err
	return c
}

// ClearErrors removes all configured errors.
func (c *FakeClient) ClearErrors() {
	c.getError = nil
	c.createError = nil
	c.updateError = nil
	c.deleteError = nil
	c.listError = nil
	c.patchError = nil
}

// Tracking methods

// GetCalls returns all keys passed to Get.
func (c *FakeClient) GetCalls() []client.ObjectKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getCalls
}

// CreateCalls returns all objects passed to Create.
func (c *FakeClient) CreateCalls() []client.Object {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.createCalls
}

// UpdateCalls returns all objects passed to Update.
func (c *FakeClient) UpdateCalls() []client.Object {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updateCalls
}

// DeleteCalls returns all objects passed to Delete.
func (c *FakeClient) DeleteCalls() []client.Object {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deleteCalls
}

// ClearCalls resets all call tracking.
func (c *FakeClient) ClearCalls() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls = make([]client.ObjectKey, 0)
	c.createCalls = make([]client.Object, 0)
	c.updateCalls = make([]client.Object, 0)
	c.deleteCalls = make([]client.Object, 0)
}

// fakeStatusWriter implements client.SubResourceWriter for status updates.
type fakeStatusWriter struct {
	client *FakeClient
}

func (w *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (w *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return w.client.Update(ctx, obj)
}

func (w *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.client.Patch(ctx, obj, patch)
}

// fakeSubResourceClient implements client.SubResourceClient.
type fakeSubResourceClient struct {
	client      *FakeClient
	subResource string
}

func (c *fakeSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}

func (c *fakeSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (c *fakeSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return c.client.Update(ctx, obj)
}

func (c *fakeSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return c.client.Patch(ctx, obj, patch)
}

// copyInto copies the source object into the destination.
func copyInto(src, dst client.Object) {
	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)

	if srcVal.Kind() == reflect.Ptr {
		srcVal = srcVal.Elem()
	}
	if dstVal.Kind() == reflect.Ptr {
		dstVal = dstVal.Elem()
	}

	dstVal.Set(srcVal)
}

// gvkFromListType attempts to determine the item GVK from a list type.
func gvkFromListType(list client.ObjectList) schema.GroupVersionKind {
	t := reflect.TypeOf(list)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Get Items field
	itemsField, ok := t.FieldByName("Items")
	if !ok {
		return schema.GroupVersionKind{}
	}

	// Get element type of the slice
	elemType := itemsField.Type.Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	return schema.GroupVersionKind{
		Kind: elemType.Name(),
	}
}

// setListItems sets the Items field on a list object.
func setListItems(list client.ObjectList, items []client.Object) {
	listVal := reflect.ValueOf(list)
	if listVal.Kind() == reflect.Ptr {
		listVal = listVal.Elem()
	}

	itemsField := listVal.FieldByName("Items")
	if !itemsField.IsValid() || !itemsField.CanSet() {
		return
	}

	elemType := itemsField.Type().Elem()
	newSlice := reflect.MakeSlice(itemsField.Type(), 0, len(items))

	for _, item := range items {
		itemVal := reflect.ValueOf(item)
		if elemType.Kind() != reflect.Ptr && itemVal.Kind() == reflect.Ptr {
			itemVal = itemVal.Elem()
		}
		newSlice = reflect.Append(newSlice, itemVal)
	}

	itemsField.Set(newSlice)
}

// Verify interface compliance
var _ client.Client = &FakeClient{}
