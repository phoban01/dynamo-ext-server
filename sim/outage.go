package sim

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// errStoreDown is the error of every call to the shared store during an
// outage.
var errStoreDown = errors.New("sim: the shared store is down")

// down is the shared store during an outage: every call fails.
type down struct{ client.Client }

func (down) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errStoreDown
}
func (down) List(context.Context, client.ObjectList, ...client.ListOption) error { return errStoreDown }
func (down) Create(context.Context, client.Object, ...client.CreateOption) error { return errStoreDown }
func (down) Delete(context.Context, client.Object, ...client.DeleteOption) error { return errStoreDown }
func (down) Update(context.Context, client.Object, ...client.UpdateOption) error { return errStoreDown }
func (down) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return errStoreDown
}
func (down) DeleteAllOf(context.Context, client.Object, ...client.DeleteAllOfOption) error {
	return errStoreDown
}
func (down) Status() client.SubResourceWriter            { return downSub{} }
func (down) SubResource(string) client.SubResourceClient { return downSub{} }

type downSub struct{}

func (downSub) Get(context.Context, client.Object, client.Object, ...client.SubResourceGetOption) error {
	return errStoreDown
}
func (downSub) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return errStoreDown
}
func (downSub) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return errStoreDown
}
func (downSub) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return errStoreDown
}
func (downSub) Apply(context.Context, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
	return errStoreDown
}
