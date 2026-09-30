package sim

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

func TestStoreRoutesClaimsLocally(t *testing.T) {
	ctx := context.Background()
	shared := newShared(false)
	a, b := newRouter(shared), newRouter(shared)

	claim := &claimsv1alpha1.DeviceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "job"}}
	if err := a.Create(ctx, claim); err != nil {
		t.Fatal(err)
	}
	var got claimsv1alpha1.DeviceClaim
	if err := b.Get(ctx, client.ObjectKeyFromObject(claim), &got); err == nil {
		t.Error("a claim of cluster a is visible in cluster b")
	}
	if err := a.Get(ctx, client.ObjectKeyFromObject(claim), &got); err != nil {
		t.Errorf("cluster a cannot read its own claim: %v", err)
	}

	dev := &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "d1"}}
	if err := a.Create(ctx, dev); err != nil {
		t.Fatal(err)
	}
	var d solasv1alpha1.Device
	if err := b.Get(ctx, client.ObjectKey{Name: "d1"}, &d); err != nil {
		t.Errorf("a Device created in a is not visible in b: %v", err)
	}
	d.Status.ClaimRef = &solasv1alpha1.ClaimRef{Member: "b", MemberUID: "m", Namespace: "ns", Name: "x", UID: "u"}
	if err := b.Status().Update(ctx, &d); err != nil {
		t.Fatal(err)
	}
	if err := a.Get(ctx, client.ObjectKey{Name: "d1"}, &d); err != nil || d.Status.FencingToken != 1 {
		t.Errorf("bind through b: token %d, err %v; want 1", d.Status.FencingToken, err)
	}
}
