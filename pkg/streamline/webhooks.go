package streamline

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidatingHandler defines the interface for validation webhooks.
type ValidatingHandler[T client.Object] interface {
	// ValidateCreate validates an object on creation.
	// Returns warnings (non-blocking) and an error (blocking).
	ValidateCreate(ctx context.Context, obj T) (warnings admission.Warnings, err error)

	// ValidateUpdate validates an object on update.
	// Both old and new objects are provided for comparison.
	ValidateUpdate(ctx context.Context, oldObj, newObj T) (warnings admission.Warnings, err error)

	// ValidateDelete validates an object on deletion.
	ValidateDelete(ctx context.Context, obj T) (warnings admission.Warnings, err error)
}

// DefaultingHandler defines the interface for defaulting webhooks.
type DefaultingHandler[T client.Object] interface {
	// Default applies defaults to an object.
	Default(ctx context.Context, obj T)
}

// ValidationBuilder provides a fluent API for building field validations.
type ValidationBuilder struct {
	errors   field.ErrorList
	warnings admission.Warnings
	basePath *field.Path
}

// NewValidationBuilder creates a new ValidationBuilder.
func NewValidationBuilder() *ValidationBuilder {
	return &ValidationBuilder{
		errors:   make(field.ErrorList, 0),
		warnings: make(admission.Warnings, 0),
	}
}

// WithBasePath sets the base path for all field errors.
func (vb *ValidationBuilder) WithBasePath(path *field.Path) *ValidationBuilder {
	vb.basePath = path
	return vb
}

// path returns the full path combining base path and the given path.
func (vb *ValidationBuilder) path(p *field.Path) *field.Path {
	if vb.basePath == nil {
		return p
	}
	return vb.basePath.Child(p.String())
}

// Required validates that a field is not empty.
func (vb *ValidationBuilder) Required(path *field.Path, value interface{}) *ValidationBuilder {
	if isEmpty(value) {
		vb.errors = append(vb.errors, field.Required(vb.path(path), ""))
	}
	return vb
}

// RequiredString validates that a string is not empty.
func (vb *ValidationBuilder) RequiredString(path *field.Path, value string) *ValidationBuilder {
	if value == "" {
		vb.errors = append(vb.errors, field.Required(vb.path(path), ""))
	}
	return vb
}

// RequiredInt validates that an int is not zero.
func (vb *ValidationBuilder) RequiredInt(path *field.Path, value int) *ValidationBuilder {
	if value == 0 {
		vb.errors = append(vb.errors, field.Required(vb.path(path), ""))
	}
	return vb
}

// NotNegative validates that a number is not negative.
func (vb *ValidationBuilder) NotNegative(path *field.Path, value int) *ValidationBuilder {
	if value < 0 {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value, "must not be negative"))
	}
	return vb
}

// Positive validates that a number is positive.
func (vb *ValidationBuilder) Positive(path *field.Path, value int) *ValidationBuilder {
	if value <= 0 {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value, "must be positive"))
	}
	return vb
}

// InRange validates that a number is within a range.
func (vb *ValidationBuilder) InRange(path *field.Path, value, min, max int) *ValidationBuilder {
	if value < min || value > max {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value,
			fmt.Sprintf("must be between %d and %d", min, max)))
	}
	return vb
}

// MinLength validates minimum string length.
func (vb *ValidationBuilder) MinLength(path *field.Path, value string, min int) *ValidationBuilder {
	if len(value) < min {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value,
			fmt.Sprintf("must be at least %d characters", min)))
	}
	return vb
}

// MaxLength validates maximum string length.
func (vb *ValidationBuilder) MaxLength(path *field.Path, value string, max int) *ValidationBuilder {
	if len(value) > max {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value,
			fmt.Sprintf("must be at most %d characters", max)))
	}
	return vb
}

// MatchesRegex validates that a string matches a regex pattern.
func (vb *ValidationBuilder) MatchesRegex(path *field.Path, value, pattern string) *ValidationBuilder {
	matched, err := regexp.MatchString(pattern, value)
	if err != nil || !matched {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value,
			fmt.Sprintf("must match pattern %s", pattern)))
	}
	return vb
}

// OneOf validates that a value is one of the allowed values.
func (vb *ValidationBuilder) OneOf(path *field.Path, value string, allowed ...string) *ValidationBuilder {
	for _, a := range allowed {
		if value == a {
			return vb
		}
	}
	vb.errors = append(vb.errors, field.NotSupported(vb.path(path), value, allowed))
	return vb
}

// Immutable validates that a field has not changed.
func (vb *ValidationBuilder) Immutable(path *field.Path, oldValue, newValue interface{}) *ValidationBuilder {
	if !reflect.DeepEqual(oldValue, newValue) {
		vb.errors = append(vb.errors, field.Forbidden(vb.path(path), "field is immutable"))
	}
	return vb
}

// ImmutableAfterSet validates that a field cannot change once set.
func (vb *ValidationBuilder) ImmutableAfterSet(path *field.Path, oldValue, newValue interface{}) *ValidationBuilder {
	if !isEmpty(oldValue) && !reflect.DeepEqual(oldValue, newValue) {
		vb.errors = append(vb.errors, field.Forbidden(vb.path(path), "field is immutable once set"))
	}
	return vb
}

// Forbidden validates that a field is not set.
func (vb *ValidationBuilder) Forbidden(path *field.Path, value interface{}, reason string) *ValidationBuilder {
	if !isEmpty(value) {
		vb.errors = append(vb.errors, field.Forbidden(vb.path(path), reason))
	}
	return vb
}

// Custom adds a custom validation.
func (vb *ValidationBuilder) Custom(path *field.Path, valid bool, value interface{}, reason string) *ValidationBuilder {
	if !valid {
		vb.errors = append(vb.errors, field.Invalid(vb.path(path), value, reason))
	}
	return vb
}

// CustomFunc runs a custom validation function.
func (vb *ValidationBuilder) CustomFunc(fn func() *field.Error) *ValidationBuilder {
	if err := fn(); err != nil {
		vb.errors = append(vb.errors, err)
	}
	return vb
}

// Warn adds a warning (non-blocking).
func (vb *ValidationBuilder) Warn(message string) *ValidationBuilder {
	vb.warnings = append(vb.warnings, message)
	return vb
}

// WarnIf adds a warning if the condition is true.
func (vb *ValidationBuilder) WarnIf(condition bool, message string) *ValidationBuilder {
	if condition {
		vb.warnings = append(vb.warnings, message)
	}
	return vb
}

// WarnDeprecated adds a deprecation warning.
func (vb *ValidationBuilder) WarnDeprecated(path *field.Path, message string) *ValidationBuilder {
	vb.warnings = append(vb.warnings, fmt.Sprintf("%s is deprecated: %s", path.String(), message))
	return vb
}

// HasErrors returns true if any validation errors were recorded.
func (vb *ValidationBuilder) HasErrors() bool {
	return len(vb.errors) > 0
}

// Errors returns the validation errors.
func (vb *ValidationBuilder) Errors() field.ErrorList {
	return vb.errors
}

// Warnings returns the validation warnings.
func (vb *ValidationBuilder) Warnings() admission.Warnings {
	return vb.warnings
}

// Build returns the validation result as warnings and error.
func (vb *ValidationBuilder) Build() (admission.Warnings, error) {
	if len(vb.errors) == 0 {
		return vb.warnings, nil
	}
	return vb.warnings, vb.errors.ToAggregate()
}

// BuildError returns just the error.
func (vb *ValidationBuilder) BuildError() error {
	if len(vb.errors) == 0 {
		return nil
	}
	return vb.errors.ToAggregate()
}

// Merge merges another builder's results into this one.
func (vb *ValidationBuilder) Merge(other *ValidationBuilder) *ValidationBuilder {
	vb.errors = append(vb.errors, other.errors...)
	vb.warnings = append(vb.warnings, other.warnings...)
	return vb
}

// isEmpty checks if a value is empty.
func isEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	default:
		return false
	}
}

// Convenience field error constructors

// FieldRequired creates a required field error.
func FieldRequired(field string) error {
	return fmt.Errorf("field %s is required", field)
}

// FieldInvalid creates an invalid field error.
func FieldInvalid(fieldPath string, value interface{}, reason string) error {
	return fmt.Errorf("field %s is invalid: %s (value: %v)", fieldPath, reason, value)
}

// FieldImmutable creates an immutable field error.
func FieldImmutable(fieldPath string) error {
	return fmt.Errorf("field %s is immutable", fieldPath)
}

// FieldForbidden creates a forbidden field error.
func FieldForbidden(fieldPath string, reason string) error {
	return fmt.Errorf("field %s is forbidden: %s", fieldPath, reason)
}

// FieldNotSupported creates a not-supported value error.
func FieldNotSupported(fieldPath string, value interface{}, supported []string) error {
	return fmt.Errorf("field %s has unsupported value %v; supported values: %s",
		fieldPath, value, strings.Join(supported, ", "))
}

// DefaultingBuilder provides a fluent API for applying defaults.
type DefaultingBuilder[T client.Object] struct {
	obj T
}

// NewDefaultingBuilder creates a new DefaultingBuilder.
func NewDefaultingBuilder[T client.Object](obj T) *DefaultingBuilder[T] {
	return &DefaultingBuilder[T]{obj: obj}
}

// SetDefault sets a value if the current value is empty.
func SetDefault[T any](ptr *T, defaultValue T) {
	if ptr != nil && isEmpty(*ptr) {
		*ptr = defaultValue
	}
}

// SetDefaultString sets a string if it's empty.
func SetDefaultString(ptr *string, defaultValue string) {
	if ptr != nil && *ptr == "" {
		*ptr = defaultValue
	}
}

// SetDefaultInt sets an int if it's zero.
func SetDefaultInt(ptr *int, defaultValue int) {
	if ptr != nil && *ptr == 0 {
		*ptr = defaultValue
	}
}

// SetDefaultInt32 sets an int32 if it's zero.
func SetDefaultInt32(ptr *int32, defaultValue int32) {
	if ptr != nil && *ptr == 0 {
		*ptr = defaultValue
	}
}

// SetDefaultInt64 sets an int64 if it's zero.
func SetDefaultInt64(ptr *int64, defaultValue int64) {
	if ptr != nil && *ptr == 0 {
		*ptr = defaultValue
	}
}

// SetDefaultBool sets a bool if the pointer is nil.
func SetDefaultBool(ptr **bool, defaultValue bool) {
	if ptr != nil && *ptr == nil {
		*ptr = &defaultValue
	}
}

// SetDefaultSlice sets a slice if it's nil or empty.
func SetDefaultSlice[T any](ptr *[]T, defaultValue []T) {
	if ptr != nil && len(*ptr) == 0 {
		*ptr = defaultValue
	}
}

// SetDefaultMap sets a map if it's nil or empty.
func SetDefaultMap[K comparable, V any](ptr *map[K]V, defaultValue map[K]V) {
	if ptr != nil && len(*ptr) == 0 {
		*ptr = defaultValue
	}
}

// EnsureMapKey ensures a map key exists with a default value.
func EnsureMapKey[K comparable, V any](m map[K]V, key K, defaultValue V) {
	if _, exists := m[key]; !exists {
		m[key] = defaultValue
	}
}

// WebhookValidator provides a generic validator for streamline handlers.
type WebhookValidator[T client.Object] struct {
	handler ValidatingHandler[T]
}

// NewWebhookValidator creates a new webhook validator.
func NewWebhookValidator[T client.Object](handler ValidatingHandler[T]) *WebhookValidator[T] {
	return &WebhookValidator[T]{handler: handler}
}

// ValidateCreate implements admission.CustomValidator.
func (v *WebhookValidator[T]) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.handler.ValidateCreate(ctx, obj.(T))
}

// ValidateUpdate implements admission.CustomValidator.
func (v *WebhookValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return v.handler.ValidateUpdate(ctx, oldObj.(T), newObj.(T))
}

// ValidateDelete implements admission.CustomValidator.
func (v *WebhookValidator[T]) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.handler.ValidateDelete(ctx, obj.(T))
}

// WebhookDefaulter provides a generic defaulter for streamline handlers.
type WebhookDefaulter[T client.Object] struct {
	handler DefaultingHandler[T]
}

// NewWebhookDefaulter creates a new webhook defaulter.
func NewWebhookDefaulter[T client.Object](handler DefaultingHandler[T]) *WebhookDefaulter[T] {
	return &WebhookDefaulter[T]{handler: handler}
}

// Default implements admission.CustomDefaulter.
func (d *WebhookDefaulter[T]) Default(ctx context.Context, obj runtime.Object) error {
	d.handler.Default(ctx, obj.(T))
	return nil
}

// BaseValidator provides a base implementation of ValidatingHandler.
// Embed this in your validator to provide default implementations.
type BaseValidator[T client.Object] struct{}

// ValidateCreate returns no warnings and no error by default.
func (v *BaseValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate returns no warnings and no error by default.
func (v *BaseValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete returns no warnings and no error by default.
func (v *BaseValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	return nil, nil
}

// BaseDefaulter provides a base implementation of DefaultingHandler.
// Embed this in your defaulter to provide a default implementation.
type BaseDefaulter[T client.Object] struct{}

// Default does nothing by default.
func (d *BaseDefaulter[T]) Default(ctx context.Context, obj T) {}

// SpecValidator validates a spec struct using reflection.
type SpecValidator struct {
	errors   field.ErrorList
	specPath *field.Path
}

// NewSpecValidator creates a new spec validator.
func NewSpecValidator(specPath *field.Path) *SpecValidator {
	if specPath == nil {
		specPath = field.NewPath("spec")
	}
	return &SpecValidator{
		specPath: specPath,
	}
}

// ValidateStruct validates a struct by checking for required fields.
// Fields with the `validate:"required"` tag are checked.
func (sv *SpecValidator) ValidateStruct(spec interface{}) *SpecValidator {
	v := reflect.ValueOf(spec)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return sv
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fieldVal := v.Field(i)
		fieldType := t.Field(i)
		fieldPath := sv.specPath.Child(strings.ToLower(fieldType.Name))

		// Check validate tag
		tag := fieldType.Tag.Get("validate")
		if strings.Contains(tag, "required") && isEmpty(fieldVal.Interface()) {
			sv.errors = append(sv.errors, field.Required(fieldPath, ""))
		}
	}

	return sv
}

// Errors returns the validation errors.
func (sv *SpecValidator) Errors() field.ErrorList {
	return sv.errors
}

// Build returns the validation result.
func (sv *SpecValidator) Build() error {
	if len(sv.errors) == 0 {
		return nil
	}
	return sv.errors.ToAggregate()
}
