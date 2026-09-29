package controller_test

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clocktesting "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/phoban01/solas/internal/fakekube"
	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
	"github.com/phoban01/solas/pkg/controller/claim"
	"github.com/phoban01/solas/pkg/controller/member"
)

//= spec/solas.md#7-6-leave
//= type=test
//# The member MUST then delete its `Member`.

// TestGracefulLeave drains a member with two bound claims. It must end
// with both devices free and no Member.
func TestGracefulLeave(t *testing.T) {
	ctx := context.Background()
	c1 := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c1", UID: "u1"}}
	c2 := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c2", UID: "u2"}}
	k := fakekube.NewClient(c1, c2,
		&solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d1"}},
		&solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d2"}})

	m := &member.Manager{Client: k, Reader: k, ClusterID: "cluster-a",
		Lease: 30 * time.Second, Margin: 3 * time.Second, Clock: clocktesting.NewFakePassiveClock(time.Unix(1, 0))}
	r := &claim.Reconciler{Client: k, Reader: k, ClusterID: "cluster-a", Member: m, Resync: time.Second}
	m.Drained = r.Drained

	step := func() {
		t.Helper()
		if err := m.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		for _, c := range []*claimsv1alpha1.DeviceClaim{c1, c2} {
			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(c)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 5 {
		step()
	}
	for _, d := range []string{"d1", "d2"} {
		var dev solasv1alpha1.Device
		if err := k.Get(ctx, client.ObjectKey{Name: d}, &dev); err != nil || dev.Status.ClaimRef == nil {
			t.Fatalf("%s not bound before the leave: %v", d, err)
		}
	}

	// An operator starts the leave.
	var mem solasv1alpha1.Member
	if err := k.Get(ctx, client.ObjectKey{Name: "cluster-a"}, &mem); err != nil {
		t.Fatal(err)
	}
	mem.Status.Phase = solasv1alpha1.MemberDraining
	if err := k.Status().Update(ctx, &mem); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		step()
		if m.Status().Left {
			break
		}
	}

	if !m.Status().Left {
		t.Fatal("member did not leave")
	}
	for _, d := range []string{"d1", "d2"} {
		var dev solasv1alpha1.Device
		if err := k.Get(ctx, client.ObjectKey{Name: d}, &dev); err != nil || dev.Status.ClaimRef != nil {
			t.Errorf("%s not free after the leave: ref %+v, err %v", d, dev.Status.ClaimRef, err)
		}
	}
	if err := k.Get(ctx, client.ObjectKey{Name: "cluster-a"}, &mem); !apierrors.IsNotFound(err) {
		t.Errorf("Member still exists: %v", err)
	}
	for _, name := range []string{"c1", "c2"} {
		var got claimsv1alpha1.DeviceClaim
		if err := k.Get(ctx, client.ObjectKey{Namespace: "ns", Name: name}, &got); err != nil {
			t.Fatal(err)
		}
		if got.Status.Phase != claimsv1alpha1.ClaimSuspended {
			t.Errorf("%s phase = %s, want Suspended", name, got.Status.Phase)
		}
	}
}
