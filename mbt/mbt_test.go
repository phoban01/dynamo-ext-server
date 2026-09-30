package mbt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phoban01/solas/internal/ddbtest"
	"github.com/phoban01/solas/pkg/apiserver"
	"github.com/phoban01/solas/pkg/storage/dynamo"
)

func newDriver(t *testing.T) *Driver {
	t.Helper()
	c := ddbtest.Client(t)
	table := ddbtest.TableName(t)
	ddbtest.DeleteTable(t, c, table)
	if err := dynamo.EnsureTable(context.Background(), c, table); err != nil {
		t.Fatal(err)
	}
	d, stop, err := NewDriver(apiserver.RESTOptionsGetter{
		Dynamo:          dynamo.Config{Client: c, Table: table, PollInterval: 20 * time.Millisecond},
		Codec:           apiserver.StorageCodec(),
		EncodeVersioner: apiserver.StorageVersioner(),
		Prefix:          "/registry",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return d
}

// traces returns the trace files: MBT_TRACES, a directory from
// scripts/mbt.sh, or the checked-in test data.
func traces(t *testing.T) []string {
	t.Helper()
	dir := os.Getenv("MBT_TRACES")
	if dir == "" {
		dir = "testdata"
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.itf.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no traces in %s: %v", dir, err)
	}
	return files
}

// TestMBT replays each trace against the real REST stores.
func TestMBT(t *testing.T) {
	for _, f := range traces(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			tr, err := Load(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := newDriver(t).Replay(tr); err != nil {
				t.Fatal(err)
			}
		})
	}
}
