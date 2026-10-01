package member_test

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/phoban01/solas/internal/teststore"
	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/apiserver"
	"github.com/phoban01/solas/pkg/registry/solas/member"
)

//= spec/solas.md#7-3-renew
//= type=test
//# The renew MUST carry the resource version that the member last read or
//# wrote.

// TestRenewWithStaleVersionConflicts runs on each store: a renew with the
// version the member wrote passes, and a renew with an older version
// fails with 409 Conflict.
func TestRenewWithStaleVersionConflicts(t *testing.T) {
	teststore.Run(t, testRenewWithStaleVersionConflicts)
}

func testRenewWithStaleVersionConflicts(t *testing.T, g apiserver.RESTOptionsGetter) {
	r, status, err := member.NewREST(apiserver.Scheme, g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Store.DestroyFunc)
	ctx := genericapirequest.WithNamespace(context.Background(), metav1.NamespaceNone)

	obj, err := r.Create(ctx, &solas.Member{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: solas.MemberSpec{LeaseDurationSeconds: 30}},
		rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	created := obj.(*solas.Member)

	renew := func(m *solas.Member) (*solas.Member, error) {
		m = m.DeepCopy()
		now := metav1.NewMicroTime(time.Now())
		m.Status.RenewTime = &now
		out, _, err := status.Update(ctx, m.Name, rest.DefaultUpdatedObjectInfo(m), rest.ValidateAllObjectFunc,
			rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return out.(*solas.Member), nil
	}
	renewed, err := renew(created)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.UID != created.UID || renewed.Status.RenewTime == nil {
		t.Fatalf("renewed member = %+v, want the same UID with a renew time", renewed)
	}
	if _, err := renew(created); !apierrors.IsConflict(err) {
		t.Errorf("renew with the version from before the last renew: err = %v, want 409 Conflict", err)
	}
	if _, err := renew(renewed); err != nil {
		t.Errorf("renew with the last version: %v", err)
	}
}
