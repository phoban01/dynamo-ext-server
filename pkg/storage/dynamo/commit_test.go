package dynamo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// eventVersions returns the versions in the event log of the store.
func eventVersions(t *testing.T, s *store) []uint64 {
	t.Helper()
	var got []uint64
	var start map[string]types.AttributeValue
	for {
		out, err := s.client.Query(context.Background(), &dynamodb.QueryInput{
			TableName:                 aws.String(s.table),
			KeyConditionExpression:    aws.String("pk = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str(s.eventPK())},
			ConsistentRead:            aws.Bool(true),
			ExclusiveStartKey:         start,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range out.Items {
			var v uint64
			if _, err := fmt.Sscanf(strAttr(item, attrSK), "%d", &v); err != nil {
				t.Fatal(err)
			}
			got = append(got, v)
		}
		if out.LastEvaluatedKey == nil {
			return got
		}
		start = out.LastEvaluatedKey
	}
}

//= spec/solas.md#3-1-issue
//= type=test
//# The first write to a resource MUST get resource version 2.

func TestCommitSequence(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	rv1, err := s.commit(ctx, write{sk: "/a", action: actCreate, value: []byte("1")})
	if err != nil || rv1 != 2 {
		t.Fatalf("create = %d, %v; want 2, nil", rv1, err)
	}
	rv2, err := s.commit(ctx, write{sk: "/a", action: actUpdate, expectRV: 2, value: []byte("2"), prev: []byte("1")})
	if err != nil || rv2 != 3 {
		t.Fatalf("update = %d, %v; want 3, nil", rv2, err)
	}
	rv3, err := s.commit(ctx, write{sk: "/a", action: actDelete, expectRV: 3, value: []byte("2"), prev: []byte("2")})
	if err != nil || rv3 != 4 {
		t.Fatalf("delete = %d, %v; want 4, nil", rv3, err)
	}
	if got := eventVersions(t, s); fmt.Sprint(got) != "[1 2 3 4]" {
		t.Errorf("event log = %v, want [1 2 3 4] (1 is INIT)", got)
	}
}

//= spec/solas.md#2-3-writes
//= type=test
//# For an update, the object action MUST be a put on condition that `rv`
//# equals the version that the server read.

//= spec/solas.md#2-3-writes
//= type=test
//# For a delete, the object action MUST be a delete on condition that `rv`
//# equals the version that the server read.

func TestCommitObjectConflict(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.commit(ctx, write{sk: "/a", action: actCreate, value: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		w    write
	}{
		{"create existing", write{sk: "/a", action: actCreate, value: []byte("x")}},
		{"update stale", write{sk: "/a", action: actUpdate, expectRV: 7, value: []byte("x"), prev: []byte("1")}},
		{"delete stale", write{sk: "/a", action: actDelete, expectRV: 7, value: []byte("1"), prev: []byte("1")}},
		{"update missing", write{sk: "/b", action: actUpdate, expectRV: 1, value: []byte("x"), prev: []byte("1")}},
	}
	for _, tt := range tests {
		if _, err := s.commit(ctx, tt.w); !errors.Is(err, errObjectConflict) {
			t.Errorf("%s: err = %v, want errObjectConflict", tt.name, err)
		}
	}
	if n, err := s.readCounter(ctx); err != nil || n != 2 {
		t.Errorf("counter = %d, %v; want 2, nil after failed writes", n, err)
	}
}

//= spec/solas.md#3-1-issue
//= type=test
//# Each write MUST get a resource version exactly one more than the
//# previous write to the same resource.

//= spec/solas.md#4-1-event-log
//= type=test
//# Each write MUST add one event item to the event log of its resource.

func TestCommitConcurrentWritersHaveNoGaps(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const writers, each = 8, 5
	var mu sync.Mutex
	var rvs []uint64
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				rv, err := s.commit(ctx, write{sk: fmt.Sprintf("/w%d-%d", w, i), action: actCreate, value: []byte("x")})
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				rvs = append(rvs, rv)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	sort.Slice(rvs, func(i, j int) bool { return rvs[i] < rvs[j] })
	for i, rv := range rvs {
		if rv != uint64(i+2) {
			t.Fatalf("versions = %v, want 2 to %d with no gaps or repeats", rvs, writers*each+1)
		}
	}
	if got := eventVersions(t, s); len(got) != writers*each+1 || got[len(got)-1] != writers*each+1 {
		t.Errorf("event log = %v, want 1 to %d", got, writers*each+1)
	}
}
