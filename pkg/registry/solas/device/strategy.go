// Package device holds the registry of the Device resource.
package device

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

// strategy handles creates, updates, and deletes of devices.
type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// statusStrategy handles updates of the status subresource.
type statusStrategy struct {
	strategy
}

// NewStrategy returns the strategy for devices.
func NewStrategy(typer runtime.ObjectTyper) strategy {
	return strategy{typer, names.SimpleNameGenerator}
}

// NewStatusStrategy returns the strategy for the status of devices.
func NewStatusStrategy(s strategy) statusStrategy {
	return statusStrategy{s}
}

//= spec/solas.md#5-1-resource
//# `Device` MUST be a cluster-scoped resource in the group `solas.dev`,
//# version `v1alpha1`.

func (strategy) NamespaceScoped() bool { return false }

// PrepareForCreate drops the status, so a device starts free.
func (strategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	obj.(*solas.Device).Status = solas.DeviceStatus{}
}

//= spec/solas.md#5-3-status-updates
//# A spec update MUST NOT change `status`.

func (strategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	obj.(*solas.Device).Status = old.(*solas.Device).Status
}

func (strategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return validation.ValidateDevice(obj.(*solas.Device))
}

func (strategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string { return nil }

func (strategy) AllowCreateOnUpdate() bool { return false }

//= spec/solas.md#5-3-status-updates
//# The server MUST NOT accept an unconditional update of `Device` status.

//= spec/solas.md#5-3-status-updates
//# Each status update MUST carry the resource version that the client read.

func (strategy) AllowUnconditionalUpdate() bool { return false }

func (strategy) Canonicalize(obj runtime.Object) {}

func (strategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateDeviceUpdate(obj.(*solas.Device), old.(*solas.Device))
}

func (strategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string { return nil }

// GetResetFields tells server-side apply that the main resource does not
// own the status.
func (strategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{
		"solas.dev/v1alpha1": fieldpath.NewSet(fieldpath.MakePathOrDie("status")),
	}
}

//= spec/solas.md#5-3-status-updates
//# A status update MUST NOT change `spec`.

func (statusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	d, o := obj.(*solas.Device), old.(*solas.Device)
	d.Spec = o.Spec
	// Labels and annotations belong to the main resource.
	d.Labels, d.Annotations = o.Labels, o.Annotations
}

func (statusStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateDeviceStatusUpdate(obj.(*solas.Device), old.(*solas.Device))
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
	d, ok := obj.(*solas.Device)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a Device")
	}
	return labels.Set(d.Labels), generic.ObjectMetaFieldsSet(&d.ObjectMeta, false), nil
}

// Match returns a predicate for devices.
func Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{Label: label, Field: field, GetAttrs: GetAttrs}
}
