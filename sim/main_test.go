package sim

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestMain(m *testing.M) {
	// The controllers log through controller-runtime; the simulator does
	// not need their logs.
	log.SetLogger(logr.Discard())
	os.Exit(m.Run())
}
