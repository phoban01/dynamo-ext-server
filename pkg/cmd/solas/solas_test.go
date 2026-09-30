package solas

import (
	"context"
	"testing"
)

// TestFlagsOfBothParts checks that one command takes the flags of the API
// server part and of the controller part, ADR 0012.
func TestFlagsOfBothParts(t *testing.T) {
	cmd := NewCommand(context.Background(), NewOptions())
	for _, name := range []string{
		"secure-port", "dynamodb-table", "dynamodb-endpoint", "storage-prefix",
		"cluster-id", "lease-duration", "lease-margin", "sweep-interval", "leader-elect",
	} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("no flag --%s", name)
		}
	}
}

func TestBothPartsValidate(t *testing.T) {
	o := NewOptions()
	cmd := NewCommand(context.Background(), o)
	cmd.SetArgs([]string{"--cluster-id=Not_A_Name"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("the command ran with an invalid --cluster-id")
	}
}
