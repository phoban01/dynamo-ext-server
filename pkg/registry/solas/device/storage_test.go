package device_test

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/phoban01/solas/internal/teststore"
	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/apiserver"
	"github.com/phoban01/solas/pkg/registry/solas/device"
)

func newREST(t *testing.T, g apiserver.RESTOptionsGetter) (*device.REST, *device.StatusREST) {
	t.Helper()
	r, status, err := device.NewREST(apiserver.Scheme, g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Store.DestroyFunc)
	return r, status
}

func create(t *testing.T, r *device.REST, name string) *solas.Device {
	t.Helper()
	obj, err := r.Create(clusterCtx(), &solas.Device{ObjectMeta: metav1.ObjectMeta{Name: name}},
		rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return obj.(*solas.Device)
}

func bind(ctx context.Context, status *device.StatusREST, d *solas.Device, claim string) (*solas.Device, error) {
	d = d.DeepCopy()
	d.Status.ClaimRef = &solas.ClaimRef{Member: "a", MemberUID: "mu", Namespace: "ns", Name: claim, UID: types.UID("u-" + claimUID(claim))}
	obj, _, err := status.Update(ctx, d.Name, rest.DefaultUpdatedObjectInfo(d), rest.ValidateAllObjectFunc,
		rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return obj.(*solas.Device), nil
}

func claimUID(claim string) string { return claim }

//= spec/solas.md#5-4-delete
//= type=test
//# The server MUST reject a delete of a bound device.

func TestDeleteOfBoundDeviceIsRejected(t *testing.T) {
	teststore.Run(t, testDeleteOfBoundDeviceIsRejected)
}

func testDeleteOfBoundDeviceIsRejected(t *testing.T, g apiserver.RESTOptionsGetter) {
	ctx := clusterCtx()
	r, status := newREST(t, g)
	bound := create(t, r, "d1")
	create(t, r, "d2")
	if _, err := bind(ctx, status, bound, "c1"); err != nil {
		t.Fatal(err)
	}

	_, _, err := r.Delete(ctx, "d1", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
	if !apierrors.IsConflict(err) {
		t.Errorf("delete of bound device: err = %v, want Conflict", err)
	}
	if _, _, err := r.Delete(ctx, "d2", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}); err != nil {
		t.Errorf("delete of free device: %v", err)
	}
}

//= spec/solas.md#5-3-status-updates
//= type=test
//# Each status update MUST carry the resource version that the client read.

func TestStatusUpdateWithStaleVersionConflicts(t *testing.T) {
	teststore.Run(t, testStatusUpdateWithStaleVersionConflicts)
}

func testStatusUpdateWithStaleVersionConflicts(t *testing.T, g apiserver.RESTOptionsGetter) {
	ctx := clusterCtx()
	r, status := newREST(t, g)
	d := create(t, r, "d1")
	if _, err := bind(ctx, status, d, "c1"); err != nil {
		t.Fatal(err)
	}
	// d still has the version from before the bind.
	_, err := bind(ctx, status, d, "c2")
	if !apierrors.IsConflict(err) {
		t.Errorf("bind with stale version: err = %v, want Conflict", err)
	}
}

func TestListAndWatch(t *testing.T) { teststore.Run(t, testListAndWatch) }

func testListAndWatch(t *testing.T, g apiserver.RESTOptionsGetter) {
	ctx := clusterCtx()
	r, _ := newREST(t, g)
	create(t, r, "d1")

	// The watch cache starts empty and asks clients to retry until it has
	// loaded, as real clients do.
	var obj runtime.Object
	var err error
	deadline := time.Now().Add(10 * time.Second)
	for {
		obj, err = r.List(ctx, &metainternalversion.ListOptions{})
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	list := obj.(*solas.DeviceList)
	if len(list.Items) != 1 {
		t.Fatalf("list = %d items, want 1", len(list.Items))
	}

	// The watch waits for the cache too.
	var w watch.Interface
	deadline = time.Now().Add(10 * time.Second)
	for {
		w, err = r.Watch(ctx, &metainternalversion.ListOptions{ResourceVersion: list.ResourceVersion})
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	create(t, r, "d2")
	select {
	case e := <-w.ResultChan():
		// The cache wraps objects, so read the name through meta.Accessor.
		m, err := meta.Accessor(e.Object)
		if err != nil {
			t.Fatal(err)
		}
		if e.Type != watch.Added || m.GetName() != "d2" {
			t.Errorf("event = %v %v, want ADDED d2", e.Type, e.Object)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no watch event after 10s")
	}
}

var _ runtime.Object = &solas.Device{}

// clusterCtx is a request context for a cluster-scoped resource.
func clusterCtx() context.Context {
	return genericapirequest.WithNamespace(genericapirequest.NewContext(), metav1.NamespaceNone)
}
