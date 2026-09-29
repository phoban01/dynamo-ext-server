// Package fakekube gives controller tests a fake client that behaves like
// solas-apiserver where the controllers depend on it.
package fakekube

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
	"github.com/phoban01/solas/pkg/controller"
)

// NewClient returns a fake client with the objects. It gives each created
// object a UID, and it applies the Device status rules of spec 5.3: a new
// holder needs a free device, and a bind raises the fencing token.
func NewClient(objs ...client.Object) client.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(controller.NewScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&solasv1alpha1.Device{}, &solasv1alpha1.Member{}, &claimsv1alpha1.DeviceClaim{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if obj.GetUID() == "" {
					obj.SetUID(NewUID())
				}
				if m, ok := obj.(*solasv1alpha1.Member); ok && m.Status.Phase == "" {
					m.Status.Phase = solasv1alpha1.MemberActive
				}
				return c.Create(ctx, obj, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if d, ok := obj.(*solasv1alpha1.Device); ok && sub == "status" {
					if err := deviceStatus(ctx, c, d); err != nil {
						return err
					}
				}
				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()
}

// deviceStatus applies the status strategy of solas-apiserver to d.
func deviceStatus(ctx context.Context, c client.Client, d *solasv1alpha1.Device) error {
	var old solasv1alpha1.Device
	if err := c.Get(ctx, client.ObjectKeyFromObject(d), &old); err != nil {
		return err
	}
	if old.ResourceVersion != d.ResourceVersion {
		return apierrors.NewConflict(schema.GroupResource{Group: "solas.dev", Resource: "devices"}, d.Name,
			fmt.Errorf("resource version %s is not %s", d.ResourceVersion, old.ResourceVersion))
	}
	if o, n := old.Status.ClaimRef, d.Status.ClaimRef; o != nil && n != nil && *o != *n {
		return apierrors.NewInvalid(schema.GroupKind{Group: "solas.dev", Kind: "Device"}, d.Name,
			field.ErrorList{field.Forbidden(field.NewPath("status", "claimRef"), "cannot change the holder")})
	}
	d.Status.FencingToken = old.Status.FencingToken
	if old.Status.ClaimRef == nil && d.Status.ClaimRef != nil {
		d.Status.FencingToken++
	}
	return nil
}

// NewUID returns a random UID.
func NewUID() types.UID {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return types.UID(hex.EncodeToString(b))
}
