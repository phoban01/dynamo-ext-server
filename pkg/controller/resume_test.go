package controller_test

import (
	"context"
	"testing"
	"time"

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

//= spec/solas.md#12-4-switch
//= type=test
//# On the destination, the member renews its `Member`, which has the same
//# UID, spec 7.2, and its claims go back to `Bound`.

// TestResumeAfterSwitch starts a member on a store that a migration
// filled: its Member has the same UID and an old renew time, its claim is
// Suspended, and the devices keep their holders and tokens. The member
// renews its Member, the claim goes back to Bound, and the next bind of a
// copied device gets the old token plus 1.
func TestResumeAfterSwitch(t *testing.T) {
	ctx := context.Background()
	old := metav1.NewMicroTime(time.Unix(10, 0))
	mem := &solasv1alpha1.Member{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", UID: "uid-a"},
		Spec:   solasv1alpha1.MemberSpec{LeaseDurationSeconds: 30},
		Status: solasv1alpha1.MemberStatus{RenewTime: &old, Phase: solasv1alpha1.MemberActive}}
	held := &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d1"}}
	held.Status.ClaimRef = &solasv1alpha1.ClaimRef{Member: "cluster-a", MemberUID: "uid-a", Namespace: "ns", Name: "c1", UID: "u1"}
	held.Status.FencingToken = 4
	free := &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d2"}}
	free.Status.FencingToken = 7
	c1 := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c1", UID: "u1",
		Finalizers: []string{claimsv1alpha1.ReleaseFinalizer}}}
	c1.Status = claimsv1alpha1.DeviceClaimStatus{Phase: claimsv1alpha1.ClaimSuspended, DeviceName: "d1", MemberUID: "uid-a", FencingToken: 4}
	c2 := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c2", UID: "u2"}}
	k := fakekube.NewClient(mem, held, free, c1, c2)

	m := &member.Manager{Client: k, Reader: k, ClusterID: "cluster-a",
		Lease: 30 * time.Second, Margin: 3 * time.Second, Clock: clocktesting.NewFakePassiveClock(time.Unix(1000, 0))}
	r := &claim.Reconciler{Client: k, Reader: k, ClusterID: "cluster-a", Member: m, Resync: time.Second}
	m.MarkLost = r.MarkLost

	for range 5 {
		if err := m.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		for _, c := range []*claimsv1alpha1.DeviceClaim{c1, c2} {
			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(c)}); err != nil {
				t.Fatal(err)
			}
		}
	}

	if st := m.Status(); st.UID != "uid-a" || !st.Live {
		t.Fatalf("member status = %+v, want live with the copied uid-a", st)
	}
	get := func(name string) *claimsv1alpha1.DeviceClaim {
		var c claimsv1alpha1.DeviceClaim
		if err := k.Get(ctx, client.ObjectKey{Namespace: "ns", Name: name}, &c); err != nil {
			t.Fatal(err)
		}
		return &c
	}
	if c := get("c1"); c.Status.Phase != claimsv1alpha1.ClaimBound || c.Status.DeviceName != "d1" || c.Status.FencingToken != 4 {
		t.Errorf("c1 = %+v, want Bound on d1 with token 4", c.Status)
	}
	if c := get("c2"); c.Status.Phase != claimsv1alpha1.ClaimBound || c.Status.DeviceName != "d2" || c.Status.FencingToken != 8 {
		t.Errorf("c2 = %+v, want Bound on d2 with token 8, the copied 7 plus 1", c.Status)
	}
}
