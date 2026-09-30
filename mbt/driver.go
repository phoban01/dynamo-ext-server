package mbt

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/apiserver"
	"github.com/phoban01/solas/pkg/registry/solas/device"
	"github.com/phoban01/solas/pkg/registry/solas/member"
)

// Driver replays traces against the real REST stores of solas-apiserver.
type Driver struct {
	devices      *device.REST
	deviceStatus *device.StatusREST
	members      *member.REST
	memberStatus *member.StatusREST

	// uids maps a model member UID to the real one.
	uids map[int64]types.UID
	// devRV and memRV map a model resource version to the real one.
	devRV map[string]map[int64]string
	memRV map[string]map[int64]string
}

// NewDriver builds the REST stores on the given storage.
func NewDriver(getter generic.RESTOptionsGetter) (*Driver, func(), error) {
	d, ds, err := device.NewREST(apiserver.Scheme, getter)
	if err != nil {
		return nil, nil, err
	}
	m, ms, err := member.NewREST(apiserver.Scheme, getter)
	if err != nil {
		return nil, nil, err
	}
	stop := func() {
		d.Store.DestroyFunc()
		m.Store.DestroyFunc()
	}
	return &Driver{
		devices: d, deviceStatus: ds, members: m, memberStatus: ms,
		uids:  map[int64]types.UID{},
		devRV: map[string]map[int64]string{},
		memRV: map[string]map[int64]string{},
	}, stop, nil
}

func ctx() context.Context {
	return genericapirequest.WithNamespace(genericapirequest.NewContext(), metav1.NamespaceNone)
}

// Replay applies each step of the trace and compares the state after it.
func (d *Driver) Replay(tr *Trace) error {
	if len(tr.Steps) == 0 {
		return fmt.Errorf("empty trace")
	}
	for name, dev := range tr.Steps[0].State.Devices {
		obj, err := d.devices.Create(ctx(), &solas.Device{ObjectMeta: metav1.ObjectMeta{Name: name}},
			rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
		d.setDevRV(name, dev.RV, obj.(*solas.Device).ResourceVersion)
	}
	if err := d.compare(tr.Steps[0].State); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	for i := 1; i < len(tr.Steps); i++ {
		step, prev := tr.Steps[i], tr.Steps[i-1].State
		if err := d.apply(step, prev); err != nil {
			return fmt.Errorf("step %d (%s %v): %w", i, step.Action, step.Picks, err)
		}
		if err := d.compare(step.State); err != nil {
			return fmt.Errorf("step %d (%s %v): state differs: %w", i, step.Action, step.Picks, err)
		}
	}
	return nil
}

// apply does the server write of one step. Actions that stay inside a
// controller in the model, such as tick or observe, write nothing.
func (d *Driver) apply(step Step, prev State) error {
	cur := step.State
	switch step.Action {
	case "join":
		c := step.Pick("c")
		pm, cm := prev.Members[c], cur.Members[c]
		if pm.Exists {
			// The controller renews the Member it finds, spec 7.2.
			return d.renew(c, pm.RV, cm.RV, true)
		}
		obj, err := d.members.Create(ctx(), &solas.Member{
			ObjectMeta: metav1.ObjectMeta{Name: c},
			Spec:       solas.MemberSpec{LeaseDurationSeconds: 30},
			Status:     solas.MemberStatus{Phase: solas.MemberActive},
		}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		m := obj.(*solas.Member)
		d.uids[cm.UID] = m.UID
		d.setMemRV(c, cm.RV, m.ResourceVersion)
		return nil

	case "renewLand":
		c := step.Pick("c")
		cl, pm := prev.Clusters[c], prev.Members[c]
		ok := pm.Exists && pm.UID == cl.UID && pm.RV == cl.RenewRV
		return d.renew(c, cl.RenewRV, cur.Members[c].RV, ok)

	case "sweepDelete":
		c, m := step.Pick("c"), step.Pick("m")
		rv, err := d.realMemRV(m, prev.Clusters[c].Seen[m].RV)
		if err != nil {
			return err
		}
		return d.deleteMember(m, rv)

	case "finishLeave":
		c := step.Pick("c")
		rv, err := d.realMemRV(c, prev.Members[c].RV)
		if err != nil {
			return err
		}
		return d.deleteMember(c, rv)

	case "landSome":
		msg, ok := step.Msg()
		if !ok {
			return fmt.Errorf("no message")
		}
		applied := cur.Devices[msg.Dev].RV != prev.Devices[msg.Dev].RV
		uid, known := d.uids[msg.MUID]
		if !known {
			return fmt.Errorf("no real UID for model member UID %d", msg.MUID)
		}
		ref := &solas.ClaimRef{Member: msg.Cluster, MemberUID: uid, Namespace: "ns", Name: msg.Claim, UID: claimUID(msg.Claim)}
		return d.writeRef(msg.Dev, msg.RV, cur.Devices[msg.Dev].RV, ref, applied)

	case "sweepClear", "release", "drainRelease", "releaseDuplicate", "clearOrphan":
		dev := step.Pick("d")
		return d.writeRef(dev, prev.Devices[dev].RV, cur.Devices[dev].RV, nil, true)
	}
	return nil
}

// renew updates the renew time of Member c with the real version of
// fromRV. wantOK says whether the model applied the write.
func (d *Driver) renew(c string, fromRV, toRV int64, wantOK bool) error {
	cur, err := d.members.Get(ctx(), c, &metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) && !wantOK {
			return nil
		}
		return fmt.Errorf("get member %s: %w", c, err)
	}
	m := cur.(*solas.Member).DeepCopy()
	rv, known := d.memRV[c][fromRV]
	if !known {
		rv = "0" // a version the model never gave this Member
	}
	m.ResourceVersion = rv
	now := metav1.NewMicroTime(time.Now())
	m.Status.RenewTime = &now
	obj, _, err := d.memberStatus.Update(ctx(), c, rest.DefaultUpdatedObjectInfo(m),
		rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	if (err == nil) != wantOK {
		return fmt.Errorf("renew of %s: model applied=%v, server err=%v", c, wantOK, err)
	}
	if err == nil {
		d.setMemRV(c, toRV, obj.(*solas.Member).ResourceVersion)
	}
	return nil
}

func (d *Driver) deleteMember(name, rv string) error {
	_, _, err := d.members.Delete(ctx(), name, rest.ValidateAllObjectFunc,
		&metav1.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &rv}})
	if err != nil {
		return fmt.Errorf("delete member %s at %s: model applied it, server err=%v", name, rv, err)
	}
	return nil
}

// writeRef sets or clears the claimRef of device dev with the real version
// of fromRV. wantOK says whether the model applied the write.
func (d *Driver) writeRef(dev string, fromRV, toRV int64, ref *solas.ClaimRef, wantOK bool) error {
	rv, err := d.realDevRV(dev, fromRV)
	if err != nil {
		return err
	}
	cur, err := d.devices.Get(ctx(), dev, &metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get device %s: %w", dev, err)
	}
	obj := cur.(*solas.Device).DeepCopy()
	obj.ResourceVersion = rv
	obj.Status.ClaimRef = ref
	out, _, err := d.deviceStatus.Update(ctx(), dev, rest.DefaultUpdatedObjectInfo(obj),
		rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	if (err == nil) != wantOK {
		return fmt.Errorf("write of %s ref: model applied=%v, server err=%v", dev, wantOK, err)
	}
	if err == nil {
		d.setDevRV(dev, toRV, out.(*solas.Device).ResourceVersion)
	}
	return nil
}

// compare checks every device and member against the model state.
func (d *Driver) compare(want State) error {
	for name, wd := range want.Devices {
		obj, err := d.devices.Get(ctx(), name, &metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get device %s: %w", name, err)
		}
		got := obj.(*solas.Device)
		ref := got.Status.ClaimRef
		switch {
		case wd.Held != (ref != nil):
			return fmt.Errorf("%s: model held=%v, server claimRef=%+v", name, wd.Held, ref)
		case wd.Held && (ref.Member != wd.Ref.Member || ref.Name != wd.Ref.Claim || ref.MemberUID != d.uids[wd.Ref.MUID]):
			return fmt.Errorf("%s: model ref=%+v, server ref=%+v", name, wd.Ref, *ref)
		case got.Status.FencingToken != wd.Token:
			return fmt.Errorf("%s: model token=%d, server token=%d", name, wd.Token, got.Status.FencingToken)
		}
	}
	for name, wm := range want.Members {
		obj, err := d.members.Get(ctx(), name, &metav1.GetOptions{})
		exists := err == nil
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("get member %s: %w", name, err)
		}
		if exists != wm.Exists {
			return fmt.Errorf("member %s: model exists=%v, server exists=%v", name, wm.Exists, exists)
		}
		if exists && obj.(*solas.Member).UID != d.uids[wm.UID] {
			return fmt.Errorf("member %s: model uid %d is %s, server uid %s", name, wm.UID, d.uids[wm.UID], obj.(*solas.Member).UID)
		}
	}
	return nil
}

func (d *Driver) setDevRV(dev string, model int64, real string) {
	if d.devRV[dev] == nil {
		d.devRV[dev] = map[int64]string{}
	}
	d.devRV[dev][model] = real
}

func (d *Driver) setMemRV(m string, model int64, real string) {
	if d.memRV[m] == nil {
		d.memRV[m] = map[int64]string{}
	}
	d.memRV[m][model] = real
}

func (d *Driver) realDevRV(dev string, model int64) (string, error) {
	rv, ok := d.devRV[dev][model]
	if !ok {
		return "", fmt.Errorf("no real version of %s for model version %d", dev, model)
	}
	return rv, nil
}

func (d *Driver) realMemRV(m string, model int64) (string, error) {
	rv, ok := d.memRV[m][model]
	if !ok {
		return "", fmt.Errorf("no real version of member %s for model version %d", m, model)
	}
	return rv, nil
}

func claimUID(claim string) types.UID { return types.UID("uid-" + claim) }
