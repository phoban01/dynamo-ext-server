// Package format holds the range of object formats that this release
// reads and writes, spec 11.1. The storage guard checks it, and the member
// controller reports it in the Member.
package format

// Min and Max are the oldest and the newest format that this release
// reads and writes.
const (
	Min = 1
	Max = 1
)
