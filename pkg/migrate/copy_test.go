package migrate

import (
	"context"
	"io"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sstorage "k8s.io/apiserver/pkg/storage"

	"github.com/phoban01/solas/pkg/apis/solas"
)

// seed writes two devices, one bound with token 3 and a preemption
// request, and one member, at the storage level.
func seed(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	now := metav1.NewTime(time.Unix(1000, 0))
	bound := &solas.Device{ObjectMeta: metav1.ObjectMeta{Name: "d1", UID: "uid-d1", CreationTimestamp: now,
		Labels: map[string]string{"kind": "gpu"}}}
	bound.Spec.Preemptible = true
	bound.Status.ClaimRef = &solas.ClaimRef{Member: "a", MemberUID: "uid-a", Namespace: "work", Name: "train", UID: "uid-train", Priority: 1}
	bound.Status.FencingToken = 3
	bound.Status.Preemption = &solas.PreemptionRequest{Claim: solas.ClaimRef{Member: "b", MemberUID: "uid-b", Namespace: "work", Name: "urgent", UID: "uid-urgent", Priority: 5}}
	free := &solas.Device{ObjectMeta: metav1.ObjectMeta{Name: "d2", UID: "uid-d2", CreationTimestamp: now}}
	free.Status.FencingToken = 7
	renew := metav1.NewMicroTime(time.Unix(2000, 0))
	member := &solas.Member{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "uid-a", CreationTimestamp: now},
		Spec: solas.MemberSpec{LeaseDurationSeconds: 30}, Status: solas.MemberStatus{RenewTime: &renew, Phase: solas.MemberActive}}
	for _, d := range []*solas.Device{bound, free} {
		if err := s.Devices.Create(ctx, devices.prefix()+"/"+d.Name, d, &solas.Device{}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Members.Create(ctx, members.prefix()+"/a", member, &solas.Member{}, 0); err != nil {
		t.Fatal(err)
	}
}

//= spec/solas.md#12-2-copy
//= type=test
//# The copy MUST keep the name, UID, labels, annotations, spec, status, and
//# creation time of each object.

//= spec/solas.md#12-2-copy
//= type=test
//# The tool MUST NOT copy into a store that holds a `Device` or a `Member`.

func TestCopy(t *testing.T) {
	ctx := context.Background()
	src, _ := openDynamo(t)
	dst, _ := openEtcd(t)
	seed(t, src)

	rep, err := Copy(ctx, src, dst, true, io.Discard)
	if err != nil || rep != (Report{Devices: 2, Members: 1}) {
		t.Fatalf("dry run = %+v, %v; want 2 devices and 1 member", rep, err)
	}
	if err := dst.Empty(ctx); err != nil {
		t.Fatalf("a dry run wrote: %v", err)
	}

	if _, err := Copy(ctx, src, dst, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var d1 solas.Device
	if err := dst.Devices.Get(ctx, devices.prefix()+"/d1", getOpts, &d1); err != nil {
		t.Fatal(err)
	}
	if d1.UID != "uid-d1" || d1.Status.FencingToken != 3 || d1.Status.ClaimRef == nil || d1.Status.ClaimRef.Name != "train" ||
		d1.Status.Preemption == nil || d1.Status.Preemption.Claim.Name != "urgent" || d1.Labels["kind"] != "gpu" ||
		!d1.Spec.Preemptible || !d1.CreationTimestamp.Equal(&metav1.Time{Time: time.Unix(1000, 0)}) {
		t.Errorf("copied d1 = %+v", d1)
	}
	var a solas.Member
	if err := dst.Members.Get(ctx, members.prefix()+"/a", getOpts, &a); err != nil {
		t.Fatal(err)
	}
	if a.UID != types.UID("uid-a") || a.Status.RenewTime == nil || a.Spec.LeaseDurationSeconds != 30 {
		t.Errorf("copied member = %+v", a)
	}

	if _, err := Copy(ctx, src, dst, false, io.Discard); err == nil {
		t.Error("a second copy into a store that holds Devices passed")
	}
}

var getOpts = k8sstorage.GetOptions{}
