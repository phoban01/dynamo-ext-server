// Package usage keeps the usage set of each limited member, spec 15.2,
// at the storage level.
package usage

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"

	"github.com/phoban01/solas/pkg/apis/solas"
)

// Resource is the storage resource of the usage sets.
var Resource = solas.Resource("memberusages")

// Prefix is the storage key prefix of the usage sets.
const Prefix = "/solas.dev/memberusages"

// Store reads and writes usage sets.
type Store struct {
	s   storage.Interface
	now func() time.Time
}

// New returns a usage store on s, the raw storage of the usage sets.
func New(s storage.Interface) *Store { return &Store{s: s, now: time.Now} }

func key(member string) string { return Prefix + "/" + member }

// update changes the usage set of member with a conditional write, and
// creates it when it does not exist.
func (u *Store) update(ctx context.Context, member string, change func(*solas.MemberUsage) error) error {
	return u.s.GuaranteedUpdate(ctx, key(member), &solas.MemberUsage{}, true, nil,
		func(in runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
			mu := in.(*solas.MemberUsage).DeepCopy()
			mu.Name = member
			if err := change(mu); err != nil {
				return nil, nil, err
			}
			return mu, nil, nil
		}, nil)
}

//= spec/solas.md#15-2-usage-set
//# Before the device write of a bind, the server MUST add the device to the
//# usage set with a conditional write, on condition that the set holds
//# fewer devices than `maxDevices`.

//= spec/solas.md#15-2-usage-set
//# A bind over the limit MUST fail with `403 Forbidden`.

// Reserve adds device to the usage set of member, on condition that the
// set holds fewer than max devices. A device already in the set passes.
func (u *Store) Reserve(ctx context.Context, member, device string, max int32) error {
	return u.update(ctx, member, func(mu *solas.MemberUsage) error {
		for _, e := range mu.Entries {
			if e.Device == device {
				return nil
			}
		}
		if int32(len(mu.Entries)) >= max {
			return apierrors.NewForbidden(solas.Resource("devices"), device,
				fmt.Errorf("member %s holds or is binding %d devices, and its limit is %d", member, len(mu.Entries), max))
		}
		mu.Entries = append(mu.Entries, solas.UsageEntry{Device: device, Added: metav1.NewTime(u.now())})
		return nil
	})
}

//= spec/solas.md#15-3-cleanup
//# A clear of a device MUST remove it from the usage set of its member.

// Remove takes device out of the usage set of member.
func (u *Store) Remove(ctx context.Context, member, device string) error {
	return u.RemoveIf(ctx, member, device, func(solas.UsageEntry) bool { return true })
}

// RemoveIf takes device out of the usage set of member when keep says so
// for its entry, in the same conditional write.
func (u *Store) RemoveIf(ctx context.Context, member, device string, ok func(solas.UsageEntry) bool) error {
	return u.update(ctx, member, func(mu *solas.MemberUsage) error {
		out := mu.Entries[:0]
		for _, e := range mu.Entries {
			if e.Device == device && ok(e) {
				continue
			}
			out = append(out, e)
		}
		mu.Entries = out
		return nil
	})
}

// Entries returns the usage set of member.
func (u *Store) Entries(ctx context.Context, member string) ([]solas.UsageEntry, error) {
	var mu solas.MemberUsage
	err := u.s.Get(ctx, key(member), storage.GetOptions{IgnoreNotFound: true}, &mu)
	return mu.Entries, err
}
