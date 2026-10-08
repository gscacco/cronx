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

// Zoned reports the time of another clock expressed in a fixed location. It is
// how the configured timezone reaches the scheduler: the instant is the same,
// the wall clock a job is scheduled on is not.
type Zoned struct {
	// Base is the clock the instant comes from.
	Base Clock
	// Location is the zone the instant is expressed in. It must not be nil.
	Location *time.Location
}

// Now returns the current time of Base, in Location.
func (z Zoned) Now() time.Time {
	return z.Base.Now().In(z.Location)
}

// Every implementation must satisfy Clock.
var (
	_ Clock = System{}
	_ Clock = Fixed{}
	_ Clock = Zoned{}
)
