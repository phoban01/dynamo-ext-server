package dynamo

import (
	"context"
	"testing"

	"k8s.io/apiserver/pkg/apis/example"
	"k8s.io/apiserver/pkg/storage"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

//= spec/solas.md#12-1-seal
//= type=test
//# A sealed store MUST reject every write.

//= spec/solas.md#12-1-seal
//= type=test
//# A sealed store MUST serve reads.

//= spec/solas.md#12-1-seal
//= type=test
//# A write that fails because the store is sealed MUST return
//# `503 ServiceUnavailable`.

//= spec/solas.md#12-1-seal
//= type=test
//# An unseal removes the attribute `sealed`.

func TestSeal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cfg := Config{Client: s.client, Table: s.table}
	pod := func(name string) *example.Pod {
		return &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
	}
	if err := s.Create(ctx, "/pods/ns/p1", pod("p1"), &example.Pod{}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, "/pods/ns/p2", pod("p2"), &example.Pod{}, 0); err != nil {
		t.Fatal(err)
	}

	if err := Seal(ctx, cfg, "/", []string{"/pods"}); err != nil {
		t.Fatal(err)
	}
	if sealed, err := IsStoreSealed(ctx, cfg, "/", []string{"/pods"}); err != nil || !sealed {
		t.Fatalf("IsStoreSealed = %v, %v; want true", sealed, err)
	}

	writes := map[string]func() error{
		"create": func() error { return s.Create(ctx, "/pods/ns/p3", pod("p3"), &example.Pod{}, 0) },
		"update": func() error {
			return s.GuaranteedUpdate(ctx, "/pods/ns/p1", &example.Pod{}, false, nil,
				func(in runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
					p := in.(*example.Pod).DeepCopy()
					p.Labels = map[string]string{"x": "y"}
					return p, nil, nil
				}, nil)
		},
		"delete": func() error {
			return s.Delete(ctx, "/pods/ns/p2", &example.Pod{}, nil, storage.ValidateAllObjectFunc, nil, storage.DeleteOptions{})
		},
	}
	for name, write := range writes {
		if err := write(); !IsSealed(err) {
			t.Errorf("%s on a sealed store: err = %v, want 503 store is sealed", name, err)
		}
	}

	// Reads keep working, and the seal changed no object.
	var got example.Pod
	if err := s.Get(ctx, "/pods/ns/p1", storage.GetOptions{}, &got); err != nil || got.Labels != nil {
		t.Errorf("get on a sealed store = %+v, %v; want p1 unchanged", got.ObjectMeta, err)
	}
	var list example.PodList
	if err := s.GetList(ctx, "/pods/ns", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, &list); err != nil || len(list.Items) != 2 {
		t.Errorf("list on a sealed store = %d items, %v; want 2", len(list.Items), err)
	}

	if err := Unseal(ctx, cfg, "/", []string{"/pods"}); err != nil {
		t.Fatal(err)
	}
	for name, write := range writes {
		if err := write(); err != nil {
			t.Errorf("%s after unseal: %v", name, err)
		}
	}
}

//= spec/solas.md#12-1-seal
//= type=test
//# To seal a resource that has no counter item, the tool MUST create the
//# counter item with the attribute `sealed`.

func TestSealNewResource(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := Seal(ctx, Config{Client: s.client, Table: s.table}, "/", []string{"/pods"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, "/pods/ns/p1", &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"}}, &example.Pod{}, 0); !IsSealed(err) {
		t.Errorf("first create on a sealed new resource: err = %v, want 503 store is sealed", err)
	}
}
