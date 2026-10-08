package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/store"
)

// The lease is what keeps one scheduler per state: the scheduler takes it
// before it runs, renews it while it runs and gives it back when it stops. The
// tests drive it with instants of their own, so that expiry is a matter of
// arithmetic rather than of waiting.

// leaseTTL is the lifetime the tests grant a lease.
const leaseTTL = 30 * time.Second

// leaseStart is the instant the tests take the first lease at.
const leaseStart = "2026-01-01T10:00:00Z"

func TestAcquiringAFreeLeaseRecordsTheHolder(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)

	// EXERCISE
	held, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL)

	// VERIFY
	if err != nil {
		t.Fatalf("AcquireLease() on a free state returned an unexpected error: %v", err)
	}
	if held.Holder != "host:1" {
		t.Errorf("the lease belongs to %q, want %q", held.Holder, "host:1")
	}
	if !held.AcquiredAt.Equal(start) {
		t.Errorf("the lease was taken at %s, want %s", held.AcquiredAt, start)
	}
	if want := start.Add(leaseTTL); !held.ExpiresAt.Equal(want) {
		t.Errorf("the lease expires at %s, want %s", held.ExpiresAt, want)
	}

	current, found, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found {
		t.Fatal("the store holds no lease, want the one that was just taken")
	}
	if current.Holder != "host:1" || !current.ExpiresAt.Equal(held.ExpiresAt) {
		t.Errorf("CurrentLease() = %+v, want the lease of %q until %s", current, "host:1", held.ExpiresAt)
	}
}

func TestCurrentLeaseReportsNothingWhenTheStateIsFree(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()

	// EXERCISE
	held, found, err := subject.CurrentLease(ctx)

	// VERIFY
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if found {
		t.Errorf("CurrentLease() reported the lease %+v, want none on a state no scheduler has touched", held)
	}
}

func TestASecondHolderIsRefusedWhileTheLeaseIsValid(t *testing.T) {
	// SETUP: one scheduler holds the state.
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE: a second scheduler asks for the same state one second later.
	refused, err := subject.AcquireLease(ctx, "host:2", start.Add(time.Second), leaseTTL)

	// VERIFY: it is refused, it is told who has the state, and the holder is
	// left exactly as it was.
	if !errors.Is(err, store.ErrLeaseHeld) {
		t.Fatalf("AcquireLease() error = %v, want it to wrap %v", err, store.ErrLeaseHeld)
	}
	if !strings.Contains(err.Error(), "host:1") {
		t.Errorf("AcquireLease() error = %q, want it to name the holder", err.Error())
	}
	if refused.Holder != "" {
		t.Errorf("AcquireLease() reported the lease %+v, want none when it is refused", refused)
	}

	current, found, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found || current.Holder != "host:1" || !current.ExpiresAt.Equal(start.Add(leaseTTL)) {
		t.Errorf("CurrentLease() = %+v (found=%v), want the untouched lease of %q", current, found, "host:1")
	}
}

func TestALeaseThatHasExpiredCanBeTakenOver(t *testing.T) {
	// SETUP: the lease of a scheduler that was killed outright, so it never
	// gave the state back.
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE: a scheduler asks for the state one second after the lease of
	// the first one has expired.
	later := start.Add(leaseTTL).Add(time.Second)
	taken, err := subject.AcquireLease(ctx, "host:2", later, leaseTTL)

	// VERIFY
	if err != nil {
		t.Fatalf("AcquireLease() after the lease expired returned an unexpected error: %v", err)
	}
	if taken.Holder != "host:2" || !taken.AcquiredAt.Equal(later) {
		t.Errorf("the lease was taken by %+v, want %q from %s", taken, "host:2", later)
	}

	current, found, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found || current.Holder != "host:2" {
		t.Errorf("CurrentLease() = %+v (found=%v), want the lease of the second holder", current, found)
	}
}

func TestRenewingALeaseExtendsIt(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE: the scheduler is still alive ten seconds later.
	renewed := start.Add(10 * time.Second)
	err := subject.RenewLease(ctx, "host:1", renewed, leaseTTL)

	// VERIFY: the expiry moves and the moment it was taken does not.
	if err != nil {
		t.Fatalf("RenewLease() returned an unexpected error: %v", err)
	}
	current, found, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found {
		t.Fatal("the store holds no lease after renewing it")
	}
	if want := renewed.Add(leaseTTL); !current.ExpiresAt.Equal(want) {
		t.Errorf("the lease expires at %s, want %s after the renewal", current.ExpiresAt, want)
	}
	if !current.AcquiredAt.Equal(start) {
		t.Errorf("the lease was taken at %s, want the renewal to leave %s alone", current.AcquiredAt, start)
	}
}

func TestRenewingALeaseAnotherHolderTookIsRefused(t *testing.T) {
	// SETUP: a scheduler that was replaced while it was not looking.
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}
	later := start.Add(leaseTTL).Add(time.Second)
	if _, err := subject.AcquireLease(ctx, "host:2", later, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() of the second holder returned an unexpected error: %v", err)
	}

	// EXERCISE
	err := subject.RenewLease(ctx, "host:1", later, leaseTTL)

	// VERIFY: the renewal fails and the state stays with the new holder.
	if err == nil {
		t.Fatal("RenewLease() succeeded, want a refusal for a lease that was taken over")
	}
	if !strings.Contains(err.Error(), "host:1") {
		t.Errorf("RenewLease() error = %q, want it to name the holder", err.Error())
	}
	current, _, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if current.Holder != "host:2" {
		t.Errorf("CurrentLease() = %+v, want the lease of the holder that took it over", current)
	}
}

func TestReleasingALeaseGivesTheStateBack(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE
	err := subject.ReleaseLease(ctx, "host:1")

	// VERIFY: the state is free, and the scheduler that comes next does not
	// have to wait for a lease nobody holds any more.
	if err != nil {
		t.Fatalf("ReleaseLease() returned an unexpected error: %v", err)
	}
	if _, found, err := subject.CurrentLease(ctx); err != nil || found {
		t.Errorf("CurrentLease() found=%v (err=%v) after the lease was released, want a free state", found, err)
	}

	taken, err := subject.AcquireLease(ctx, "host:2", start.Add(time.Second), leaseTTL)
	if err != nil {
		t.Fatalf("AcquireLease() after the release returned an unexpected error: %v", err)
	}
	if taken.Holder != "host:2" {
		t.Errorf("the lease was taken by %q, want the scheduler that came after", taken.Holder)
	}
}

func TestReleasingALeaseAnotherHolderHasIsRefused(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE: a process that does not hold the state tries to give it back.
	err := subject.ReleaseLease(ctx, "host:2")

	// VERIFY
	if err == nil {
		t.Fatal("ReleaseLease() succeeded for a holder that does not have the lease, want a refusal")
	}
	current, found, err := subject.CurrentLease(ctx)
	if err != nil {
		t.Fatalf("CurrentLease() returned an unexpected error: %v", err)
	}
	if !found || current.Holder != "host:1" {
		t.Errorf("CurrentLease() = %+v (found=%v), want the lease of %q left alone", current, found, "host:1")
	}
}

func TestAHolderMayTakeItsOwnLeaseAgain(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	start := mustTime(t, leaseStart)
	if _, err := subject.AcquireLease(ctx, "host:1", start, leaseTTL); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}

	// EXERCISE: the same identity asks again, which is what a scheduler that
	// comes back without its identity changing does.
	later := start.Add(5 * time.Second)
	held, err := subject.AcquireLease(ctx, "host:1", later, leaseTTL)

	// VERIFY
	if err != nil {
		t.Fatalf("AcquireLease() by the holder itself returned an unexpected error: %v", err)
	}
	if !held.AcquiredAt.Equal(later) {
		t.Errorf("the lease was taken at %s, want %s", held.AcquiredAt, later)
	}
}
