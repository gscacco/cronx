package scheduler_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/scheduler"
	"gscacco.com/cronx/internal/store"
)

// The lease is what makes one scheduler per state: a scheduler that finds the
// state held by another one refuses to start instead of running the same jobs a
// second time, and gives the state back when it stops.

func TestASchedulerRefusesAStateAnotherSchedulerHolds(t *testing.T) {
	// SETUP: one scheduler runs on the state, and a second one, with an
	// identity of its own, is built around the same state.
	persistent, runs := openTestState(t, clock.System{})
	first := buildSchedulerOn(t, persistent, runs, clock.System{}, "first", helperJob(t, "backup", "ok"))
	second := buildSchedulerOn(t, persistent, runs, clock.System{}, "second", helperJob(t, "backup", "ok"))

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- first.Run(ctx) }()
	waitForLease(t, persistent, true)

	// EXERCISE: the second scheduler is asked to start.
	err := second.Run(context.Background())

	// VERIFY: it refuses, it says who has the state, and the first one is left
	// running with the state still its own.
	if !errors.Is(err, store.ErrLeaseHeld) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, store.ErrLeaseHeld)
	}
	if !strings.Contains(err.Error(), "first") {
		t.Errorf("Run() error = %q, want it to name the scheduler that holds the state", err.Error())
	}
	select {
	case err := <-stopped:
		t.Errorf("the first scheduler stopped with %v, want it to keep running", err)
	default:
	}
	held, found, err := persistent.CurrentLease(context.Background())
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found || held.Holder != "first" {
		t.Errorf("CurrentLease() = %+v (found=%v), want the state to be left with %q", held, found, "first")
	}

	// The scheduler that holds the state stops cleanly.
	if err := stopScheduler(t, cancel, stopped); err != nil {
		t.Errorf("the first scheduler returned %v when it stopped, want a clean stop", err)
	}
}

func TestASchedulerGivesTheStateBackWhenItStops(t *testing.T) {
	// SETUP: a scheduler runs on the state.
	built, persistent, runs := buildScheduler(t, helperJob(t, "backup", "ok"))
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- built.Run(ctx) }()
	waitForLease(t, persistent, true)

	// EXERCISE: the scheduler is asked to stop.
	err := stopScheduler(t, cancel, stopped)

	// VERIFY: it stopped cleanly, the state is free, and the next scheduler
	// takes it without waiting for a lease nobody holds any more.
	if err != nil {
		t.Fatalf("Run() returned %v when it stopped, want a clean stop", err)
	}
	if held, found, err := persistent.CurrentLease(context.Background()); err != nil || found {
		t.Errorf("the state is held by %+v (found=%v, err=%v) after the scheduler stopped, want it free",
			held, found, err)
	}

	next := buildSchedulerOn(t, persistent, runs, clock.System{}, "next", helperJob(t, "backup", "ok"))
	nextCtx, nextCancel := context.WithCancel(context.Background())
	nextStopped := make(chan error, 1)
	go func() { nextStopped <- next.Run(nextCtx) }()
	waitForLease(t, persistent, true)
	if err := stopScheduler(t, nextCancel, nextStopped); err != nil {
		t.Errorf("the scheduler that came after returned %v when it stopped, want a clean stop", err)
	}
}

// waitForLease waits until the state is held, or until it is free.
func waitForLease(t *testing.T, persistent *store.Store, want bool) {
	t.Helper()
	waitFor(t, "the lease of the state to change", func() bool {
		_, found, err := persistent.CurrentLease(context.Background())
		if err != nil {
			t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
		}
		return found == want
	})
}

// stopScheduler asks a scheduler that runs in the background to stop and
// returns what it reported.
func stopScheduler(t *testing.T, cancel context.CancelFunc, stopped <-chan error) error {
	t.Helper()
	cancel()
	select {
	case err := <-stopped:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduler did not stop")
		return nil
	}
}

func TestTheLeaseOfASchedulerLastsHalfAMinute(t *testing.T) {
	// SETUP: a scheduler runs on the state.
	built, persistent, _ := buildScheduler(t, helperJob(t, "backup", "ok"))
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- built.Run(ctx) }()
	waitForLease(t, persistent, true)

	// VERIFY: the lease is valid for the documented half minute, which is how
	// long a scheduler killed outright can hold a state it no longer drives.
	held, found, err := persistent.CurrentLease(context.Background())
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found {
		t.Fatal("the scheduler holds no lease, want the one it took when it started")
	}
	if got := held.ExpiresAt.Sub(held.AcquiredAt); got != scheduler.LeaseTTL {
		t.Errorf("the lease of the scheduler lasts %s, want %s", got, scheduler.LeaseTTL)
	}
	if want := 30 * time.Second; scheduler.LeaseTTL != want {
		t.Errorf("scheduler.LeaseTTL = %s, want the documented %s", scheduler.LeaseTTL, want)
	}

	if err := stopScheduler(t, cancel, stopped); err != nil {
		t.Errorf("Run() returned %v when it stopped, want a clean stop", err)
	}
}
