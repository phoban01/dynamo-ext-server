package dynamo

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/apitesting"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/apis/example"
	examplev1 "k8s.io/apiserver/pkg/apis/example/v1"

	"github.com/phoban01/solas/internal/ddbtest"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
	codec  = apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)
)

func init() {
	metav1.AddToGroupVersion(scheme, metav1.SchemeGroupVersion)
	utilruntime.Must(example.AddToScheme(scheme))
	utilruntime.Must(examplev1.AddToScheme(scheme))
}

func newPod() runtime.Object     { return &example.Pod{} }
func newPodList() runtime.Object { return &example.PodList{} }

var testGR = schema.GroupResource{Group: "example.apiserver.k8s.io", Resource: "pods"}

// fakeAPI satisfies API for tests that do not call DynamoDB.
type fakeAPI struct{ API }

// newTestStore returns a store on a new table in dynamodb-local.
func newTestStore(t *testing.T) *store {
	t.Helper()
	c := ddbtest.Client(t)
	table := ddbtest.TableName(t)
	ddbtest.DeleteTable(t, c, table)
	if err := EnsureTable(context.Background(), c, table); err != nil {
		t.Fatal(err)
	}
	s, err := newStore(Config{Client: c, Table: table, PollInterval: 20 * time.Millisecond},
		codec, newPod, newPodList, "/", "/pods", testGR)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
