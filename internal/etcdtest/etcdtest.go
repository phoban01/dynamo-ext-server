// Package etcdtest connects tests to the etcd that scripts/test.sh
// starts.
package etcdtest

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// EndpointEnv names the variable that holds the etcd client URL.
const EndpointEnv = "SOLAS_ETCD_ENDPOINT"

// Servers returns the etcd client URLs. It skips the test when EndpointEnv
// is not set.
func Servers(t testing.TB) []string {
	t.Helper()
	endpoint := os.Getenv(EndpointEnv)
	if endpoint == "" {
		t.Skipf("%s is not set; run the tests with devbox run test", EndpointEnv)
	}
	return []string{endpoint}
}

// Prefix returns a storage prefix that no other test uses, so tests that
// share one etcd do not see each other's objects.
func Prefix(t testing.TB) string {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "/test-" + hex.EncodeToString(b)
}
