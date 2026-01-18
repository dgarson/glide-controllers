package streamline

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PrerequisiteResult represents the result of a prerequisite check.
type PrerequisiteResult struct {
	// Name is the name of the prerequisite check.
	Name string

	// Satisfied indicates whether the prerequisite is met.
	Satisfied bool

	// Message describes the current state.
	Message string

	// RetryAfter suggests when to retry if not satisfied.
	// Zero means use default retry interval.
	RetryAfter time.Duration

	// Error is any error that occurred during the check.
	Error error
}

// Prerequisite defines a condition that must be met before reconciliation can proceed.
type Prerequisite interface {
	// Name returns a unique name for this prerequisite.
	Name() string

	// Check evaluates whether the prerequisite is satisfied.
	Check(ctx context.Context) PrerequisiteResult
}

// PrerequisiteChecker manages and evaluates multiple prerequisites.
type PrerequisiteChecker struct {
	prerequisites []Prerequisite
	client        client.Client
	defaultRetry  time.Duration
}

// NewPrerequisiteChecker creates a new PrerequisiteChecker.
func NewPrerequisiteChecker(c client.Client) *PrerequisiteChecker {
	return &PrerequisiteChecker{
		prerequisites: make([]Prerequisite, 0),
		client:        c,
		defaultRetry:  10 * time.Second,
	}
}

// WithDefaultRetry sets the default retry interval for failed prerequisites.
func (pc *PrerequisiteChecker) WithDefaultRetry(d time.Duration) *PrerequisiteChecker {
	pc.defaultRetry = d
	return pc
}

// Add adds a prerequisite to the checker.
func (pc *PrerequisiteChecker) Add(p Prerequisite) *PrerequisiteChecker {
	pc.prerequisites = append(pc.prerequisites, p)
	return pc
}

// AddAll adds multiple prerequisites to the checker.
func (pc *PrerequisiteChecker) AddAll(prerequisites ...Prerequisite) *PrerequisiteChecker {
	pc.prerequisites = append(pc.prerequisites, prerequisites...)
	return pc
}

// RequireResource adds a prerequisite that requires a resource to exist.
func (pc *PrerequisiteChecker) RequireResource(key types.NamespacedName, obj client.Object) *PrerequisiteChecker {
	return pc.Add(ResourceExists(pc.client, key, obj))
}

// RequireReady adds a prerequisite that requires a resource to be ready.
func (pc *PrerequisiteChecker) RequireReady(key types.NamespacedName, obj client.Object) *PrerequisiteChecker {
	return pc.Add(ResourceReady(pc.client, key, obj))
}

// RequireSecret adds a prerequisite that requires a secret to exist.
func (pc *PrerequisiteChecker) RequireSecret(key types.NamespacedName) *PrerequisiteChecker {
	return pc.Add(SecretExists(pc.client, key))
}

// RequireSecretKey adds a prerequisite that requires a secret to have a specific key.
func (pc *PrerequisiteChecker) RequireSecretKey(key types.NamespacedName, secretKey string) *PrerequisiteChecker {
	return pc.Add(SecretHasKey(pc.client, key, secretKey))
}

// RequireConfigMap adds a prerequisite that requires a ConfigMap to exist.
func (pc *PrerequisiteChecker) RequireConfigMap(key types.NamespacedName) *PrerequisiteChecker {
	return pc.Add(ConfigMapExists(pc.client, key))
}

// RequireConfigMapKey adds a prerequisite that requires a ConfigMap to have a specific key.
func (pc *PrerequisiteChecker) RequireConfigMapKey(key types.NamespacedName, dataKey string) *PrerequisiteChecker {
	return pc.Add(ConfigMapHasKey(pc.client, key, dataKey))
}

// RequireCondition adds a prerequisite that requires a condition to be true.
func (pc *PrerequisiteChecker) RequireCondition(key types.NamespacedName, obj client.Object, conditionType string) *PrerequisiteChecker {
	return pc.Add(ConditionTrue(pc.client, key, obj, conditionType))
}

// RequireCustom adds a custom prerequisite check.
func (pc *PrerequisiteChecker) RequireCustom(name string, check func(ctx context.Context) PrerequisiteResult) *PrerequisiteChecker {
	return pc.Add(&customPrerequisite{name: name, check: check})
}

// CheckAll evaluates all prerequisites and returns the results.
func (pc *PrerequisiteChecker) CheckAll(ctx context.Context) (bool, []PrerequisiteResult) {
	if len(pc.prerequisites) == 0 {
		return true, nil
	}

	results := make([]PrerequisiteResult, 0, len(pc.prerequisites))
	allSatisfied := true

	for _, p := range pc.prerequisites {
		result := p.Check(ctx)
		results = append(results, result)
		if !result.Satisfied {
			allSatisfied = false
		}
	}

	return allSatisfied, results
}

// CheckAllOrFail evaluates all prerequisites and returns an error if any fail.
func (pc *PrerequisiteChecker) CheckAllOrFail(ctx context.Context) error {
	satisfied, results := pc.CheckAll(ctx)
	if satisfied {
		return nil
	}

	var failed []string
	for _, r := range results {
		if !r.Satisfied {
			failed = append(failed, fmt.Sprintf("%s: %s", r.Name, r.Message))
		}
	}

	return fmt.Errorf("prerequisites not satisfied: %s", strings.Join(failed, "; "))
}

// GetRetryAfter returns the suggested retry interval based on all failed prerequisites.
func (pc *PrerequisiteChecker) GetRetryAfter(results []PrerequisiteResult) time.Duration {
	var maxRetry time.Duration

	for _, r := range results {
		if !r.Satisfied {
			retry := r.RetryAfter
			if retry == 0 {
				retry = pc.defaultRetry
			}
			if retry > maxRetry {
				maxRetry = retry
			}
		}
	}

	if maxRetry == 0 {
		return pc.defaultRetry
	}
	return maxRetry
}

// FormatMessage creates a human-readable message from prerequisite results.
func (pc *PrerequisiteChecker) FormatMessage(results []PrerequisiteResult) string {
	var unsatisfied []string
	for _, r := range results {
		if !r.Satisfied {
			unsatisfied = append(unsatisfied, fmt.Sprintf("%s (%s)", r.Name, r.Message))
		}
	}

	if len(unsatisfied) == 0 {
		return "all prerequisites satisfied"
	}

	return fmt.Sprintf("waiting for: %s", strings.Join(unsatisfied, ", "))
}

// Count returns the number of prerequisites.
func (pc *PrerequisiteChecker) Count() int {
	return len(pc.prerequisites)
}

// Built-in prerequisite implementations

// resourceExistsPrerequisite checks if a resource exists.
type resourceExistsPrerequisite struct {
	client client.Client
	key    types.NamespacedName
	obj    client.Object
	gvk    schema.GroupVersionKind
}

// ResourceExists creates a prerequisite that checks if a resource exists.
func ResourceExists(c client.Client, key types.NamespacedName, obj client.Object) Prerequisite {
	return &resourceExistsPrerequisite{
		client: c,
		key:    key,
		obj:    obj,
		gvk:    obj.GetObjectKind().GroupVersionKind(),
	}
}

func (p *resourceExistsPrerequisite) Name() string {
	kind := p.gvk.Kind
	if kind == "" {
		kind = fmt.Sprintf("%T", p.obj)
	}
	return fmt.Sprintf("%s/%s exists", kind, p.key.String())
}

func (p *resourceExistsPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	obj := p.obj.DeepCopyObject().(client.Object)
	err := p.client.Get(ctx, p.key, obj)

	if err == nil {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   "resource exists",
		}
	}

	if apierrors.IsNotFound(err) {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   "resource not found",
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   fmt.Sprintf("error checking resource: %v", err),
		Error:     err,
	}
}

// resourceReadyPrerequisite checks if a resource exists and has a Ready condition.
type resourceReadyPrerequisite struct {
	client client.Client
	key    types.NamespacedName
	obj    client.Object
}

// ResourceReady creates a prerequisite that checks if a resource is ready.
func ResourceReady(c client.Client, key types.NamespacedName, obj client.Object) Prerequisite {
	return &resourceReadyPrerequisite{
		client: c,
		key:    key,
		obj:    obj,
	}
}

func (p *resourceReadyPrerequisite) Name() string {
	return fmt.Sprintf("%s is ready", p.key.String())
}

func (p *resourceReadyPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	obj := p.obj.DeepCopyObject().(client.Object)
	err := p.client.Get(ctx, p.key, obj)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return PrerequisiteResult{
				Name:      p.Name(),
				Satisfied: false,
				Message:   "resource not found",
			}
		}
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   fmt.Sprintf("error checking resource: %v", err),
			Error:     err,
		}
	}

	// Check for Ready condition
	if owc, ok := obj.(ObjectWithConditions); ok {
		for _, cond := range owc.GetConditions() {
			if cond.Type == ConditionTypeReady {
				if cond.Status == metav1.ConditionTrue {
					return PrerequisiteResult{
						Name:      p.Name(),
						Satisfied: true,
						Message:   "resource is ready",
					}
				}
				return PrerequisiteResult{
					Name:      p.Name(),
					Satisfied: false,
					Message:   fmt.Sprintf("Ready condition is %s: %s", cond.Status, cond.Message),
				}
			}
		}
	}

	// No Ready condition, assume ready if exists
	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: true,
		Message:   "resource exists (no Ready condition)",
	}
}

// conditionTruePrerequisite checks if a specific condition is true.
type conditionTruePrerequisite struct {
	client        client.Client
	key           types.NamespacedName
	obj           client.Object
	conditionType string
}

// ConditionTrue creates a prerequisite that checks if a condition is true.
func ConditionTrue(c client.Client, key types.NamespacedName, obj client.Object, conditionType string) Prerequisite {
	return &conditionTruePrerequisite{
		client:        c,
		key:           key,
		obj:           obj,
		conditionType: conditionType,
	}
}

func (p *conditionTruePrerequisite) Name() string {
	return fmt.Sprintf("%s.%s is true", p.key.String(), p.conditionType)
}

func (p *conditionTruePrerequisite) Check(ctx context.Context) PrerequisiteResult {
	obj := p.obj.DeepCopyObject().(client.Object)
	err := p.client.Get(ctx, p.key, obj)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return PrerequisiteResult{
				Name:      p.Name(),
				Satisfied: false,
				Message:   "resource not found",
			}
		}
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   fmt.Sprintf("error checking resource: %v", err),
			Error:     err,
		}
	}

	if owc, ok := obj.(ObjectWithConditions); ok {
		for _, cond := range owc.GetConditions() {
			if cond.Type == p.conditionType {
				if cond.Status == metav1.ConditionTrue {
					return PrerequisiteResult{
						Name:      p.Name(),
						Satisfied: true,
						Message:   fmt.Sprintf("%s is true", p.conditionType),
					}
				}
				return PrerequisiteResult{
					Name:      p.Name(),
					Satisfied: false,
					Message:   fmt.Sprintf("%s is %s: %s", p.conditionType, cond.Status, cond.Message),
				}
			}
		}
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   fmt.Sprintf("condition %s not found", p.conditionType),
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   "resource does not support conditions",
	}
}

// secretExistsPrerequisite checks if a secret exists.
type secretExistsPrerequisite struct {
	client client.Client
	key    types.NamespacedName
}

// SecretExists creates a prerequisite that checks if a secret exists.
func SecretExists(c client.Client, key types.NamespacedName) Prerequisite {
	return &secretExistsPrerequisite{
		client: c,
		key:    key,
	}
}

func (p *secretExistsPrerequisite) Name() string {
	return fmt.Sprintf("Secret/%s exists", p.key.String())
}

func (p *secretExistsPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	secret := &corev1.Secret{}
	err := p.client.Get(ctx, p.key, secret)

	if err == nil {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   "secret exists",
		}
	}

	if apierrors.IsNotFound(err) {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   "secret not found",
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   fmt.Sprintf("error checking secret: %v", err),
		Error:     err,
	}
}

// secretHasKeyPrerequisite checks if a secret has a specific key.
type secretHasKeyPrerequisite struct {
	client    client.Client
	key       types.NamespacedName
	secretKey string
}

// SecretHasKey creates a prerequisite that checks if a secret has a specific key.
func SecretHasKey(c client.Client, key types.NamespacedName, secretKey string) Prerequisite {
	return &secretHasKeyPrerequisite{
		client:    c,
		key:       key,
		secretKey: secretKey,
	}
}

func (p *secretHasKeyPrerequisite) Name() string {
	return fmt.Sprintf("Secret/%s[%s] exists", p.key.String(), p.secretKey)
}

func (p *secretHasKeyPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	secret := &corev1.Secret{}
	err := p.client.Get(ctx, p.key, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return PrerequisiteResult{
				Name:      p.Name(),
				Satisfied: false,
				Message:   "secret not found",
			}
		}
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   fmt.Sprintf("error checking secret: %v", err),
			Error:     err,
		}
	}

	if _, exists := secret.Data[p.secretKey]; exists {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   fmt.Sprintf("key %s exists", p.secretKey),
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   fmt.Sprintf("key %s not found in secret", p.secretKey),
	}
}

// configMapExistsPrerequisite checks if a ConfigMap exists.
type configMapExistsPrerequisite struct {
	client client.Client
	key    types.NamespacedName
}

// ConfigMapExists creates a prerequisite that checks if a ConfigMap exists.
func ConfigMapExists(c client.Client, key types.NamespacedName) Prerequisite {
	return &configMapExistsPrerequisite{
		client: c,
		key:    key,
	}
}

func (p *configMapExistsPrerequisite) Name() string {
	return fmt.Sprintf("ConfigMap/%s exists", p.key.String())
}

func (p *configMapExistsPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	cm := &corev1.ConfigMap{}
	err := p.client.Get(ctx, p.key, cm)

	if err == nil {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   "ConfigMap exists",
		}
	}

	if apierrors.IsNotFound(err) {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   "ConfigMap not found",
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   fmt.Sprintf("error checking ConfigMap: %v", err),
		Error:     err,
	}
}

// configMapHasKeyPrerequisite checks if a ConfigMap has a specific key.
type configMapHasKeyPrerequisite struct {
	client  client.Client
	key     types.NamespacedName
	dataKey string
}

// ConfigMapHasKey creates a prerequisite that checks if a ConfigMap has a specific key.
func ConfigMapHasKey(c client.Client, key types.NamespacedName, dataKey string) Prerequisite {
	return &configMapHasKeyPrerequisite{
		client:  c,
		key:     key,
		dataKey: dataKey,
	}
}

func (p *configMapHasKeyPrerequisite) Name() string {
	return fmt.Sprintf("ConfigMap/%s[%s] exists", p.key.String(), p.dataKey)
}

func (p *configMapHasKeyPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	cm := &corev1.ConfigMap{}
	err := p.client.Get(ctx, p.key, cm)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return PrerequisiteResult{
				Name:      p.Name(),
				Satisfied: false,
				Message:   "ConfigMap not found",
			}
		}
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: false,
			Message:   fmt.Sprintf("error checking ConfigMap: %v", err),
			Error:     err,
		}
	}

	if _, exists := cm.Data[p.dataKey]; exists {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   fmt.Sprintf("key %s exists", p.dataKey),
		}
	}

	// Also check BinaryData
	if _, exists := cm.BinaryData[p.dataKey]; exists {
		return PrerequisiteResult{
			Name:      p.Name(),
			Satisfied: true,
			Message:   fmt.Sprintf("key %s exists (binary)", p.dataKey),
		}
	}

	return PrerequisiteResult{
		Name:      p.Name(),
		Satisfied: false,
		Message:   fmt.Sprintf("key %s not found in ConfigMap", p.dataKey),
	}
}

// customPrerequisite wraps a custom check function.
type customPrerequisite struct {
	name  string
	check func(ctx context.Context) PrerequisiteResult
}

func (p *customPrerequisite) Name() string {
	return p.name
}

func (p *customPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	result := p.check(ctx)
	if result.Name == "" {
		result.Name = p.name
	}
	return result
}

// compositePrerequisite combines multiple prerequisites with AND logic.
type compositePrerequisite struct {
	name          string
	prerequisites []Prerequisite
}

// All creates a prerequisite that requires all sub-prerequisites to be satisfied.
func All(name string, prerequisites ...Prerequisite) Prerequisite {
	return &compositePrerequisite{
		name:          name,
		prerequisites: prerequisites,
	}
}

func (p *compositePrerequisite) Name() string {
	return p.name
}

func (p *compositePrerequisite) Check(ctx context.Context) PrerequisiteResult {
	var failed []string
	var maxRetry time.Duration

	for _, prereq := range p.prerequisites {
		result := prereq.Check(ctx)
		if !result.Satisfied {
			failed = append(failed, result.Name)
			if result.RetryAfter > maxRetry {
				maxRetry = result.RetryAfter
			}
		}
	}

	if len(failed) == 0 {
		return PrerequisiteResult{
			Name:      p.name,
			Satisfied: true,
			Message:   "all prerequisites satisfied",
		}
	}

	return PrerequisiteResult{
		Name:       p.name,
		Satisfied:  false,
		Message:    fmt.Sprintf("failed: %s", strings.Join(failed, ", ")),
		RetryAfter: maxRetry,
	}
}

// anyPrerequisite combines multiple prerequisites with OR logic.
type anyPrerequisite struct {
	name          string
	prerequisites []Prerequisite
}

// Any creates a prerequisite that requires at least one sub-prerequisite to be satisfied.
func Any(name string, prerequisites ...Prerequisite) Prerequisite {
	return &anyPrerequisite{
		name:          name,
		prerequisites: prerequisites,
	}
}

func (p *anyPrerequisite) Name() string {
	return p.name
}

func (p *anyPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	var minRetry time.Duration
	var messages []string

	for _, prereq := range p.prerequisites {
		result := prereq.Check(ctx)
		if result.Satisfied {
			return PrerequisiteResult{
				Name:      p.name,
				Satisfied: true,
				Message:   fmt.Sprintf("satisfied by: %s", result.Name),
			}
		}
		messages = append(messages, result.Message)
		if minRetry == 0 || (result.RetryAfter > 0 && result.RetryAfter < minRetry) {
			minRetry = result.RetryAfter
		}
	}

	return PrerequisiteResult{
		Name:       p.name,
		Satisfied:  false,
		Message:    strings.Join(messages, "; "),
		RetryAfter: minRetry,
	}
}

// TimeBasedPrerequisite checks if enough time has passed since a timestamp.
type timeBasedPrerequisite struct {
	name      string
	since     func() time.Time
	minElapsed time.Duration
}

// TimeSince creates a prerequisite that checks if enough time has passed.
func TimeSince(name string, since func() time.Time, minElapsed time.Duration) Prerequisite {
	return &timeBasedPrerequisite{
		name:       name,
		since:      since,
		minElapsed: minElapsed,
	}
}

func (p *timeBasedPrerequisite) Name() string {
	return p.name
}

func (p *timeBasedPrerequisite) Check(ctx context.Context) PrerequisiteResult {
	timestamp := p.since()
	if timestamp.IsZero() {
		return PrerequisiteResult{
			Name:      p.name,
			Satisfied: false,
			Message:   "no timestamp available",
		}
	}

	elapsed := time.Since(timestamp)
	if elapsed >= p.minElapsed {
		return PrerequisiteResult{
			Name:      p.name,
			Satisfied: true,
			Message:   fmt.Sprintf("%v elapsed", elapsed.Round(time.Second)),
		}
	}

	remaining := p.minElapsed - elapsed
	return PrerequisiteResult{
		Name:       p.name,
		Satisfied:  false,
		Message:    fmt.Sprintf("%v remaining", remaining.Round(time.Second)),
		RetryAfter: remaining,
	}
}
