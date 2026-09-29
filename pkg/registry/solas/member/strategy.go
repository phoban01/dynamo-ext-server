// Package member holds the registry of the Member resource.
package member

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"

	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/apis/solas/validation"
)

// strategy handles creates, updates, and deletes of members.
type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// statusStrategy handles updates of the status subresource.
type statusStrategy struct {
	strategy
}

// NewStrategy returns the strategy for members.
func NewStrategy(typer runtime.ObjectTyper) strategy {
	return strategy{typer, names.SimpleNameGenerator}
}

// NewStatusStrategy returns the strategy for the status of members.
func NewStatusStrategy(s strategy) statusStrategy {
	return statusStrategy{s}
}

//= spec/solas.md#7-1-resource
//# `Member` MUST be a cluster-scoped resource in the group `solas.dev`,
//# version `v1alpha1`.

func (strategy) NamespaceScoped() bool { return false }

// PrepareForCreate sets the phase to Active when it is empty.
func (strategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	m := obj.(*solas.Member)
	if m.Status.Phase == "" {
		m.Status.Phase = solas.MemberActive
	}
}

func (strategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	obj.(*solas.Member).Status = old.(*solas.Member).Status
}

func (strategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return validation.ValidateMember(obj.(*solas.Member))
}

func (strategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string { return nil }

func (strategy) AllowCreateOnUpdate(context.Context) bool { return false }

func (strategy) AllowUnconditionalUpdate(context.Context) bool { return false }

func (strategy) Canonicalize(obj runtime.Object) {}

func (strategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateMemberUpdate(obj.(*solas.Member), old.(*solas.Member))
}

func (strategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string { return nil }

// GetResetFields tells server-side apply that the main resource does not
// own the status.
func (strategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{
		"solas.dev/v1alpha1": fieldpath.NewSet(fieldpath.MakePathOrDie("status")),
	}
}

func (statusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	d, o := obj.(*solas.Member), old.(*solas.Member)
	d.Spec = o.Spec
	// Labels and annotations belong to the main resource.
	d.Labels, d.Annotations = o.Labels, o.Annotations
}

func (statusStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateMemberStatusUpdate(obj.(*solas.Member), old.(*solas.Member))
}

func (statusStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

// GetResetFields tells server-side apply that the status subresource does
// not own the spec or the metadata.
func (statusStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{
		"solas.dev/v1alpha1": fieldpath.NewSet(
			fieldpath.MakePathOrDie("spec"),
			fieldpath.MakePathOrDie("metadata", "labels"),
			fieldpath.MakePathOrDie("metadata", "annotations"),
		),
	}
}

// GetAttrs returns the labels and fields that selectors match.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	d, ok := obj.(*solas.Member)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a Member")
	}
	return labels.Set(d.Labels), generic.ObjectMetaFieldsSet(&d.ObjectMeta, false), nil
}

// Match returns a predicate for members.
func Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{Label: label, Field: field, GetAttrs: GetAttrs}
}
