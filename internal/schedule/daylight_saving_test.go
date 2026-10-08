package schedule_test

// The two tests in this file name a real zone with a daylight saving
// transition in it, so they need the zone database to be present. The
// application embeds it (see internal/config); the test brings its own copy, so
// that what it checks does not depend on what the machine running it carries.

import (
	"testing"
	"time"

	_ "time/tzdata"
)

func TestNextSkipsATimeThatDaylightSavingRemoves(t *testing.T) {
	// SETUP: 02:30 of 29 March 2026 does not exist in Europe/Rome, where the
	// clock jumps from 02:00 to 03:00.
	parsed := mustParse(t, "30 2 * * *")
	zone := mustZone(t, "Europe/Rome")
	after := time.Date(2026, 3, 29, 1, 59, 0, 0, zone)

	// EXERCISE
	got, ok := parsed.Next(after)

	// VERIFY: the activation of that day is skipped, not moved, so the next
	// one is the following day's.
	want := time.Date(2026, 3, 30, 2, 30, 0, 0, zone)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence, want %s", after, want)
	}
	if !got.Equal(want) {
		t.Errorf("Next(%s) = %s, want %s", after, got, want)
	}
}

func TestNextFiresAnAmbiguousTimeOnce(t *testing.T) {
	// SETUP: 02:30 happens twice on 25 October 2026 in Europe/Rome, once on
	// summer time and once on winter time.
	parsed := mustParse(t, "30 2 * * *")
	zone := mustZone(t, "Europe/Rome")
	after := time.Date(2026, 10, 25, 1, 59, 0, 0, zone)

	// EXERCISE
	first, ok := parsed.Next(after)

	// VERIFY: the first of the two, which is the one on summer time.
	wantFirst := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence, want %s", after, wantFirst)
	}
	if !first.Equal(wantFirst) {
		t.Fatalf("Next(%s) = %s, want %s", after, first, wantFirst)
	}

	// The scheduler plans again from the activation it has just run, so the
	// second 02:30 is not an activation of its own.
	second, ok := parsed.Next(first)
	wantSecond := time.Date(2026, 10, 26, 2, 30, 0, 0, zone)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence, want %s", first, wantSecond)
	}
	if !second.Equal(wantSecond) {
		t.Errorf("Next(%s) = %s, want %s: the ambiguous time fired twice", first, second, wantSecond)
	}
}

// mustZone returns the location of a named zone, failing the test when the zone
// database does not know it.
func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("loading the zone %s: %v", name, err)
	}
	return zone
}
