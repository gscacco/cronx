// Package clock provides an injectable source of the current time, so that
// behaviour depending on the wall clock can be tested deterministically.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
}

// System is the clock backed by the operating system.
type System struct{}

// Now returns the current time.
func (System) Now() time.Time {
	return time.Now()
}

// Fixed always reports the same instant. It is intended for tests.
type Fixed struct {
	// T is the instant the clock always reports.
	T time.Time
}

// Now returns T.
func (f Fixed) Now() time.Time {
	return f.T
}

// Both implementations must satisfy Clock.
var (
	_ Clock = System{}
	_ Clock = Fixed{}
)
