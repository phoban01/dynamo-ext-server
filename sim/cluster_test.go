package sim

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

func TestClusterJoinsAndBinds(t *testing.T) {
	ctx := context.Background()
	shared := newShared(false)
	if err := shared.Create(ctx, &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d1"}}); err != nil {
		t.Fatal(err)
	}
	c := newCluster("a", shared, time.Unix(1000, 0),
		Settings{Lease: 30 * time.Second, Margin: 3 * time.Second, Sweep: 10 * time.Second}, rand.New(rand.NewPCG(1, 2)), 0)
	if err := c.members.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	claim := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "job"}}
	if err := c.client.Create(ctx, claim); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := c.reconcile(ctx, client.ObjectKeyFromObject(claim)); err != nil {
			t.Fatal(err)
		}
	}
	var got claimsv1alpha1.DeviceClaim
	if err := c.client.Get(ctx, client.ObjectKeyFromObject(claim), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != claimsv1alpha1.ClaimBound || got.Status.FencingToken != 1 {
		t.Errorf("claim status = %+v", got.Status)
	}

	// A restart keeps the Member, because the new manager renews it.
	uid := c.members.Status().UID
	c.restart()
	if err := c.members.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := c.members.Status().UID; got != uid {
		t.Errorf("after restart uid = %s, want %s", got, uid)
	}
}
