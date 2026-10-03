// Package guard checks each write below the registry, on any store. The
// registry strategy is the first check; the guard is a second one that
// does not depend on the strategy, so servers of two releases enforce the
// same rules on the stored objects, spec 11.
package guard

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
)

// Transition checks a write from old to new. old is nil for a create.
type Transition func(old, new runtime.Object) error

// store wraps a storage.Interface and checks each create and update.
type store struct {
	storage.Interface
	check Transition
}

// Wrap returns s with check on each create and update. Every other method
// passes through. A delete is not checked.
func Wrap(s storage.Interface, check Transition) storage.Interface {
	if check == nil {
		return s
	}
	return &store{Interface: s, check: check}
}

// Create checks a new object against no old object.
func (s *store) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	if err := s.check(nil, obj); err != nil {
		return rejected(err)
	}
	return s.Interface.Create(ctx, key, obj, out, ttl)
}

// GuaranteedUpdate checks each result of tryUpdate against the object it
// started from. The store commits only when the stored object is still
// that object, so the check is atomic with the write on every store.
func (s *store) GuaranteedUpdate(ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	checked := func(input runtime.Object, res storage.ResponseMeta) (runtime.Object, *uint64, error) {
		out, ttl, err := tryUpdate(input, res)
		if err != nil {
			return nil, nil, err
		}
		if err := s.check(input, out); err != nil {
			return nil, nil, rejected(err)
		}
		return out, ttl, nil
	}
	return s.Interface.GuaranteedUpdate(ctx, key, destination, ignoreNotFound, preconditions, checked, cachedExistingObject)
}

// rejected is an internal error, not a conflict, so a client does not
// retry the same write.
func rejected(err error) error {
	return apierrors.NewInternalError(fmt.Errorf("storage guard: %w", err))
}

// IsRejected reports whether err comes from the guard.
func IsRejected(err error) bool {
	return apierrors.IsInternalError(err) && containsGuard(err.Error())
}

func containsGuard(s string) bool {
	const p = "storage guard: "
	for i := 0; i+len(p) <= len(s); i++ {
		if s[i:i+len(p)] == p {
			return true
		}
	}
	return false
}
