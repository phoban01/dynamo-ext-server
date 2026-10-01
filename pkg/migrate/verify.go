package migrate

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
)

//= spec/solas.md#12-2-copy
//# After the copy, the tool MUST verify that each object in the destination
//# equals its source, except for the resource version.

// Verify compares every Device and Member of two stores. It ignores the
// resource version. It returns an error that names each missing object,
// each extra object, and each difference.
func Verify(ctx context.Context, src, dst *Store) error {
	var problems []string
	for _, r := range resources {
		want, err := byName(ctx, src, r)
		if err != nil {
			return err
		}
		got, err := byName(ctx, dst, r)
		if err != nil {
			return err
		}
		for name, w := range want {
			g, ok := got[name]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s/%s is missing", r.gr.Resource, name))
				continue
			}
			if !equality.Semantic.DeepEqual(w, g) {
				problems = append(problems, fmt.Sprintf("%s/%s differs:\n%s", r.gr.Resource, name, diff.Diff(w, g)))
			}
		}
		for name := range got {
			if _, ok := want[name]; !ok {
				problems = append(problems, fmt.Sprintf("%s/%s is only in the destination", r.gr.Resource, name))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the stores differ:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

// byName lists a resource and clears each resource version.
func byName(ctx context.Context, s *Store, r resource) (map[string]runtime.Object, error) {
	objs, err := s.list(ctx, r)
	if err != nil {
		return nil, err
	}
	out := map[string]runtime.Object{}
	for _, o := range objs {
		m, err := meta.Accessor(o)
		if err != nil {
			return nil, err
		}
		m.SetResourceVersion("")
		out[m.GetName()] = o
	}
	return out, nil
}
