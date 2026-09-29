package dynamo

import (
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/storage"
)

// objectPK is the partition of the object items of this resource.
func (s *store) objectPK() string {
	//= spec/solas.md#2-2-items
	//# An object item MUST have `pk` equal to `obj#` followed by the resource.
	return "obj#" + s.resource
}

// eventPK is the partition of the event log of this resource.
func (s *store) eventPK() string {
	//= spec/solas.md#2-2-items
	//# An event item MUST have `pk` equal to `ev#` followed by the resource.
	return "ev#" + s.resource
}

// counterKey returns the key of the counter item of this resource.
func (s *store) counterKey() (pk, sk string) {
	//= spec/solas.md#2-2-items
	//# A counter item MUST have `pk` equal to `rv` and `sk` equal to the
	//# resource.
	return "rv", s.resource
}

// eventSK formats a resource version as an event sort key.
func eventSK(rv uint64) string {
	//= spec/solas.md#2-2-items
	//# An event item MUST have `sk` equal to its resource version as a decimal
	//# string, padded with zeros to 20 digits.
	return fmt.Sprintf("%020d", rv)
}

//= spec/solas.md#2-2-items
//# An object item MUST have `sk` equal to the part of its storage key after
//# the resource name.

// sortKey maps a storage key to the sort key of its object item. The sort
// key starts with "/", for example "/ns/name" for "/pods/ns/name". For a
// recursive key the result is a prefix.
func (s *store) sortKey(key string, recursive bool) (string, error) {
	prepared, err := storage.PrepareKey(s.resourcePrefix, key, recursive)
	if err != nil {
		return "", err
	}
	rest := strings.TrimPrefix(prepared, s.base())
	if !strings.HasPrefix(rest, "/") {
		return "", fmt.Errorf("invalid key: %q lacks resource prefix: %q", key, s.resourcePrefix)
	}
	return rest, nil
}

// storageKey maps a sort key back to its storage key.
func (s *store) storageKey(sk string) string {
	return s.base() + sk
}

// base is the resource prefix with no trailing "/".
func (s *store) base() string {
	return strings.TrimSuffix(s.resourcePrefix, "/")
}
