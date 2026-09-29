package dynamo

import (
	"testing"
)

func testKeyStore(t *testing.T, prefix, resourcePrefix string) *store {
	t.Helper()
	s, err := newStore(Config{Client: fakeAPI{}}, nil, nil, nil, prefix, resourcePrefix, testGR)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestKeysSortKey(t *testing.T) {
	s := testKeyStore(t, "/registry", "/pods")
	tests := []struct {
		key       string
		recursive bool
		want      string
		wantErr   bool
	}{
		{key: "/pods/ns/name", want: "ns/name"},
		{key: "/pods/name", want: "name"},
		{key: "/pods/", recursive: true, want: ""},
		{key: "/pods", recursive: true, want: ""},
		{key: "/pods/ns", recursive: true, want: "ns/"},
		{key: "/pods/", wantErr: true},
		{key: "/podsx/name", wantErr: true},
		{key: "/other/name", wantErr: true},
		{key: "/pods/../name", wantErr: true},
		{key: "/pods/./name", wantErr: true},
	}
	for _, tt := range tests {
		got, err := s.sortKey(tt.key, tt.recursive)
		if (err != nil) != tt.wantErr {
			t.Errorf("sortKey(%q, %v) error = %v, wantErr %v", tt.key, tt.recursive, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("sortKey(%q, %v) = %q, want %q", tt.key, tt.recursive, got, tt.want)
		}
		if err == nil && !tt.recursive {
			if back := s.storageKey(got); back != tt.key {
				t.Errorf("storageKey(%q) = %q, want %q", got, back, tt.key)
			}
		}
	}
}

func TestKeysPartitions(t *testing.T) {
	s := testKeyStore(t, "/registry", "/pods")
	if got, want := s.objectPK(), "obj#/registry/pods"; got != want {
		t.Errorf("objectPK = %q, want %q", got, want)
	}
	if got, want := s.eventPK(), "ev#/registry/pods"; got != want {
		t.Errorf("eventPK = %q, want %q", got, want)
	}
	if pk, sk := s.counterKey(); pk != "rv" || sk != "/registry/pods" {
		t.Errorf("counterKey = %q, %q", pk, sk)
	}
	if s := testKeyStore(t, "/", "/pods"); s.resource != "/pods" {
		t.Errorf("resource with prefix / = %q, want /pods", s.resource)
	}
}

func TestKeysEventSortKeyOrder(t *testing.T) {
	if got := eventSK(42); got != "00000000000000000042" {
		t.Errorf("eventSK(42) = %q", got)
	}
	// String order must equal number order.
	if !(eventSK(9) < eventSK(10) && eventSK(99) < eventSK(100)) {
		t.Error("event sort keys do not sort by version")
	}
}
