package schedule_test

// The tests in this file name a real zone with a daylight saving
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
func TestNextWithASecondsFieldFiresAnAmbiguousHourOnce(t *testing.T) {
	// SETUP: 02:00 to 02:59 happens twice on 25 October 2026 in Europe/Rome,
	// once on summer time and once on winter time, and this expression asks
	// for every minute of the hour, on the second.
	parsed := mustParse(t, "0 * * * * *")
	zone := mustZone(t, "Europe/Rome")

	// The last minute before the repeated hour runs on summer time, and the
	// minute after it is the first of the two 02:00s.
	before := time.Date(2026, 10, 25, 1, 59, 0, 0, zone)
	first, ok := parsed.Next(before)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence", before)
	}
	wantFirst := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) // 02:00:00 CEST
	if !first.Equal(wantFirst) {
		t.Fatalf("Next(%s) = %s, want %s", before, first, wantFirst)
	}

	// The minutes of the hour are not run twice: the one after the first
	// 02:00 is the 02:01 of the winter time pass, so the minutes skipped on
	// summer time are the ones run now, and each of them runs once.
	second, ok := parsed.Next(first)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence", first)
	}
	wantSecond := time.Date(2026, 10, 25, 1, 1, 0, 0, time.UTC) // 02:01:00 CET
	if !second.Equal(wantSecond) {
		t.Errorf("Next(%s) = %s, want %s", first, second, wantSecond)
	}

	// The two passes of the hour rejoin at its end: the last minute of the
	// winter time pass is followed by 03:00, and the 02:00 on winter time is
	// never an activation of its own.
	last := time.Date(2026, 10, 25, 2, 59, 59, 0, zone)
	after, ok := parsed.Next(last)
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence", last)
	}
	wantAfter := time.Date(2026, 10, 25, 3, 0, 0, 0, zone)
	if !after.Equal(wantAfter) {
		t.Errorf("Next(%s) = %s, want %s", last, after, wantAfter)
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
