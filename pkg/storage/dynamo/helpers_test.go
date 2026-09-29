package dynamo

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var testGR = schema.GroupResource{Group: "example.apiserver.k8s.io", Resource: "pods"}

// fakeAPI satisfies API for tests that do not call DynamoDB.
type fakeAPI struct{ API }
