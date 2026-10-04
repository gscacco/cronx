package clock_test

import (
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
)

func TestFixedAlwaysReturnsTheSameInstant(t *testing.T) {
	// SETUP
	instant := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	subject := clock.Fixed{T: instant}

	// EXERCISE
	first := subject.Now()
	second := subject.Now()

	// VERIFY
	if !first.Equal(instant) {
		t.Errorf("Fixed.Now() = %s, want %s", first, instant)
	}
	if !second.Equal(instant) {
		t.Errorf("Fixed.Now() called again = %s, want %s", second, instant)
	}
}

func TestSystemReportsTheCurrentTime(t *testing.T) {
	// SETUP
	before := time.Now()
	subject := clock.System{}

	// EXERCISE
	got := subject.Now()

	// VERIFY
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("System.Now() = %s, want a time between %s and %s", got, before, after)
	}
}

func TestImplementationsSatisfyTheClockInterface(t *testing.T) {
	// SETUP
	instant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// EXERCISE
	clocks := map[string]clock.Clock{
		"System": clock.System{},
		"Fixed":  clock.Fixed{T: instant},
	}

	// VERIFY
	for name, subject := range clocks {
		if subject.Now().IsZero() {
			t.Errorf("%s.Now() = zero time, want a usable instant", name)
		}
	}
}
