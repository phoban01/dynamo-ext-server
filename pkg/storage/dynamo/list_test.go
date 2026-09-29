package dynamo

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/example"
	"k8s.io/apiserver/pkg/storage"
)

func createPods(t *testing.T, s *store, names ...string) {
	t.Helper()
	for _, name := range names {
		pod := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
		if err := s.Create(context.Background(), "/pods/ns/"+name, pod, &example.Pod{}, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListContinueAfterWriteIsGone(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createPods(t, s, "a", "b", "c")

	opts := storage.ListOptions{Recursive: true, Predicate: storage.Everything}
	opts.Predicate.Limit = 1
	first := &example.PodList{}
	if err := s.GetList(ctx, "/pods/", opts, first); err != nil {
		t.Fatal(err)
	}
	if first.Continue == "" {
		t.Fatal("first page has no continue token")
	}

	// With no write in between, the next page is served.
	opts.Predicate.Continue = first.Continue
	second := &example.PodList{}
	if err := s.GetList(ctx, "/pods/", opts, second); err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Name != "b" {
		t.Fatalf("second page = %v, want [b]", second.Items)
	}

	// After a write, the old snapshot is gone.
	createPods(t, s, "d")
	opts.Predicate.Continue = second.Continue
	err := s.GetList(ctx, "/pods/", opts, &example.PodList{})
	if !apierrors.IsResourceExpired(err) {
		t.Fatalf("third page after a write: err = %v, want 410 Gone", err)
	}
}

func TestListExactVersion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createPods(t, s, "a", "b")

	for _, tt := range []struct {
		rv   uint64
		want func(error) bool
	}{
		{rv: 3, want: func(err error) bool { return err == nil }},
		{rv: 2, want: apierrors.IsResourceExpired},
		{rv: 4, want: storage.IsTooLargeResourceVersion},
	} {
		opts := storage.ListOptions{
			Recursive:            true,
			Predicate:            storage.Everything,
			ResourceVersion:      strconv.FormatUint(tt.rv, 10),
			ResourceVersionMatch: metav1.ResourceVersionMatchExact,
		}
		if err := s.GetList(ctx, "/pods/", opts, &example.PodList{}); !tt.want(err) {
			t.Errorf("exact list at %d: unexpected err %v", tt.rv, err)
		}
	}
}

// writeDuringQuery commits a write during the first object query, so the
// two counter reads of the list differ.
type writeDuringQuery struct {
	API
	s     *store
	fired atomic.Bool
}

func (w *writeDuringQuery) Query(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	out, err := w.API.Query(ctx, in, opts...)
	if err == nil && w.fired.CompareAndSwap(false, true) {
		pod := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "late", Namespace: "ns"}}
		if err := w.s.Create(ctx, "/pods/ns/late", pod, &example.Pod{}, 0); err != nil {
			return nil, fmt.Errorf("write during query: %w", err)
		}
	}
	return out, err
}

func TestListRetriesWhenAWriteLandsDuringTheQuery(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	createPods(t, s, "a")
	s.client = &writeDuringQuery{API: s.client, s: s}

	list := &example.PodList{}
	if err := s.GetList(ctx, "/pods/", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		t.Fatal(err)
	}
	// The first attempt saw only "a" at version 2, but the counter moved to
	// 3. The retry must return both objects at version 3.
	if list.ResourceVersion != "3" || len(list.Items) != 2 {
		t.Errorf("list = rv %s with %d items, want rv 3 with 2 items", list.ResourceVersion, len(list.Items))
	}
}
