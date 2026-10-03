package device

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"

	"github.com/phoban01/solas/pkg/apis/solas"
)

// REST is the store of devices.
type REST struct {
	*genericregistry.Store
}

// StatusREST is the store of the status subresource of devices.
type StatusREST struct {
	store *genericregistry.Store
}

// NewREST returns the stores of devices and their status.
func NewREST(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter) (*REST, *StatusREST, error) {
	return NewRESTWithPolicies(scheme, optsGetter, nil)
}

// NewRESTWithPolicies returns the stores of devices, with the lookup of
// member policies that protection needs, spec 10.10.
func NewRESTWithPolicies(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter, policies PolicyLookup) (*REST, *StatusREST, error) {
	strategy := NewStrategy(scheme)
	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &solas.Device{} },
		NewListFunc:               func() runtime.Object { return &solas.DeviceList{} },
		PredicateFunc:             Match,
		DefaultQualifiedResource:  solas.Resource("devices"),
		SingularQualifiedResource: solas.Resource("device"),

		CreateStrategy:      strategy,
		UpdateStrategy:      strategy,
		DeleteStrategy:      strategy,
		ResetFieldsStrategy: strategy,

		TableConvertor: tableConvertor{},
	}
	//= spec/solas.md#5-1-resource
	//# The solas API server MUST serve `Device` from the table.
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, err
	}

	//= spec/solas.md#5-1-resource
	//# `Device` MUST have a `status` subresource.
	statusStore := *store
	status := NewStatusStrategy(strategy)
	status.policies = policies
	statusStore.UpdateStrategy = status
	statusStore.ResetFieldsStrategy = status
	return &REST{store}, &StatusREST{store: &statusStore}, nil
}

// Delete rejects the delete of a bound device.
func (r *REST) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	return r.Store.Delete(ctx, name, rejectBound(deleteValidation), options)
}

// DeleteCollection rejects the delete of each bound device.
func (r *REST) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	return r.Store.DeleteCollection(ctx, rejectBound(deleteValidation), options, listOptions)
}

//= spec/solas.md#5-4-delete
//# The server MUST reject a delete of a bound device.

//= spec/solas.md#5-4-delete
//# The server MUST check this on the state that the delete removes, using
//# the delete condition on `rv` from section 2.3.

// rejectBound adds a check to deleteValidation. The store runs the check
// inside its delete loop, on the object that the conditional delete then
// removes. A bind that commits first changes rv, so the delete reads the
// device again and the check sees the claimRef.
func rejectBound(deleteValidation rest.ValidateObjectFunc) rest.ValidateObjectFunc {
	return func(ctx context.Context, obj runtime.Object) error {
		d, ok := obj.(*solas.Device)
		if !ok {
			return fmt.Errorf("unexpected object %T", obj)
		}
		if ref := d.Status.ClaimRef; ref != nil {
			return apierrors.NewConflict(solas.Resource("devices"), d.Name,
				fmt.Errorf("device is bound to claim %s/%s of member %s", ref.Namespace, ref.Name, ref.Member))
		}
		if deleteValidation == nil {
			return nil
		}
		return deleteValidation(ctx, obj)
	}
}

// New returns an empty device.
func (r *StatusREST) New() runtime.Object { return &solas.Device{} }

// Destroy does nothing; the main store owns the storage.
func (r *StatusREST) Destroy() {}

// Get returns a device, so that clients can read before they update status.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update updates the status of a device.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Status updates never create a device.
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

// GetResetFields returns the fields that the status subresource resets.
func (r *StatusREST) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return r.store.GetResetFields()
}
