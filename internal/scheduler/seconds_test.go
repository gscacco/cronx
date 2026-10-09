package scheduler_test

import (
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
)

// A schedule with a seconds field asks for activations that are not on the
// minute. Nothing in the scheduler may round that away: the tests below hold the
// planning, the activation it reports and the run loop to the second.

func TestNextKeepsTheSecondsOfASchedule(t *testing.T) {
	// SETUP
	definition := helperJob(t, "ticks", "ok")
	definition.Schedule = "*/5 * * * * *"
	started := time.Date(2026, 1, 1, 10, 0, 3, 0, time.UTC)
	subject, _, _ := buildSchedulerWithClock(t, clock.Fixed{T: started}, definition)

	// EXERCISE
	next, ok := subject.Next(definition.Name, started)

	// VERIFY
	if !ok {
		t.Fatalf("Next() reported no activation, want the next five seconds")
	}
	if want := time.Date(2026, 1, 1, 10, 0, 5, 0, time.UTC); !next.Equal(want) {
		t.Errorf("Next() = %s, want %s", next, want)
	}
}

func TestDueReportsAnActivationBetweenTwoMinuteBoundaries(t *testing.T) {
	// SETUP: a job that runs every five seconds, started three seconds into a
	// minute, so its first activation is inside the minute it started in.
	definition := helperJob(t, "ticks", "ok")
	definition.Schedule = "*/5 * * * * *"
	started := time.Date(2026, 1, 1, 10, 0, 3, 0, time.UTC)
	subject, _, _ := buildSchedulerWithClock(t, clock.Fixed{T: started}, definition)
	activation := time.Date(2026, 1, 1, 10, 0, 5, 0, time.UTC)

	// EXERCISE + VERIFY: the activation that has arrived is reported once, and
	// the one still four seconds away is not.
	if due := subject.Due(activation.Add(-time.Second)); len(due) != 0 {
		t.Fatalf("Due() a second early returned %d activations, want none", len(due))
	}

	due := subject.Due(activation)
	if len(due) != 1 {
		t.Fatalf("Due() returned %d activations, want the one that was due", len(due))
	}
	if !due[0].At.Equal(activation) {
		t.Errorf("Activation.At = %s, want the scheduled instant %s", due[0].At, activation)
	}

	// The job is planned for the next five seconds, which is still inside the
	// minute.
	next, ok := subject.Next(definition.Name, activation)
	if want := activation.Add(5 * time.Second); !ok || !next.Equal(want) {
		t.Errorf("Next() = %s (reported %v), want %s", next, ok, want)
	}

	// Nothing else is due before then.
	if due := subject.Due(activation.Add(time.Second)); len(due) != 0 {
		t.Errorf("Due() returned %d activations a second after the last one, want none", len(due))
	}
}

func TestDueDoesNotReplayTheSecondsMissedWhileStopped(t *testing.T) {
	// SETUP: a job that runs every five seconds, and a scheduler that asks
	// what is due two minutes later, as one restarted after a stop does.
	definition := helperJob(t, "ticks", "ok")
	definition.Schedule = "*/5 * * * * *"
	started := time.Date(2026, 1, 1, 10, 0, 3, 0, time.UTC)
	subject, _, _ := buildSchedulerWithClock(t, clock.Fixed{T: started}, definition)

	// EXERCISE
	due := subject.Due(started.Add(2 * time.Minute))

	// VERIFY: the activation that was due is reported once, and the ones
	// between it and now are gone: a seconds field is not a catch-up (D21).
	if len(due) != 1 {
		t.Fatalf("Due() returned %d activations, want only the one that was due", len(due))
	}
	next, ok := subject.Next(definition.Name, started.Add(2*time.Minute))
	want := time.Date(2026, 1, 1, 10, 2, 5, 0, time.UTC)
	if !ok || !next.Equal(want) {
		t.Errorf("Next() = %s (reported %v), want %s", next, ok, want)
	}
}
