// Package format holds the range of object formats that this release
// reads and writes, spec 11.1. The storage guard checks it, and the member
// controller reports it in the Member.
package format

import "sync/atomic"

// Min and Max are the oldest and the newest format that this release
// reads and writes.
const (
	Min = 1
	Max = 1
)

// finalized is the finalized format of the store, which this server
// writes in. The server reads it from the store at start and on a slow
// poll, spec 11.2.
var finalized atomic.Int32

func init() { finalized.Store(1) }

// SetFinalized records the finalized format that the server read.
func SetFinalized(f int32) { finalized.Store(f) }

// Write returns the format of each write: the finalized format, spec 11.2.
func Write() int { return int(finalized.Load()) }
