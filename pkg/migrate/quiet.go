package migrate

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/utils/clock"
)

// Versions returns the resource version of every Device and Member, by
// resource and name.
func (s *Store) Versions(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range resources {
		objs, err := s.list(ctx, r)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			m, err := meta.Accessor(o)
			if err != nil {
				return nil, err
			}
			out[r.gr.Resource+"/"+m.GetName()] = m.GetResourceVersion()
		}
	}
	return out, nil
}

//= spec/solas.md#12-3-a-store-that-cannot-be-sealed
//# The tool MUST then list the `Member` objects, wait `D` by its own clock,
//# and list them again.

//= spec/solas.md#12-3-a-store-that-cannot-be-sealed
//# The tool MUST NOT copy when a `Member` renewed in that time, or when a
//# `Device` or a `Member` changed.

// Quiet lists, waits d by clk, and lists again. It returns an error when
// an object changed in that time. A renew of a Member is a change of its
// resource version, so a renew fails the check too.
func Quiet(ctx context.Context, versions func(context.Context) (map[string]string, error),
	d time.Duration, clk clock.Clock) error {
	before, err := versions(ctx)
	if err != nil {
		return err
	}
	select {
	case <-clk.After(d):
	case <-ctx.Done():
		return ctx.Err()
	}
	after, err := versions(ctx)
	if err != nil {
		return err
	}
	for k, v := range after {
		if before[k] != v {
			return fmt.Errorf("%s changed in the last %v: stop solas in every member cluster first", k, d)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			return fmt.Errorf("%s was deleted in the last %v: stop solas in every member cluster first", k, d)
		}
	}
	return nil
}
