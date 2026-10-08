package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrLeaseHeld is reported when a scheduler asks for a state another scheduler
// holds and whose lease has not expired.
var ErrLeaseHeld = errors.New("the state is held by another scheduler")

// Lease is the right to run the scheduler on a state database.
//
// The lease is what keeps one scheduler per state: a scheduler takes it before
// it runs, renews it while it runs and gives it back when it stops, so a second
// scheduler is refused instead of running the same jobs a second time.
type Lease struct {
	// Holder identifies the scheduler that took the lease.
	Holder string
	// AcquiredAt is the instant the lease was taken.
	AcquiredAt time.Time
	// ExpiresAt is the instant the lease stops being valid. A lease whose
	// expiry has passed belongs to a scheduler that is gone.
	ExpiresAt time.Time
}

// HeldError describes the lease that stood in the way of taking a state. It
// wraps ErrLeaseHeld, so that errors.Is tells a refusal apart from a real
// failure.
type HeldError struct {
	// Lease is the lease that is held.
	Lease Lease
}

// Error describes the refusal the way a person reads it.
func (e *HeldError) Error() string {
	return fmt.Sprintf("another scheduler is running: %s holds the state until %s",
		e.Lease.Holder, formatTime(e.Lease.ExpiresAt))
}

// Unwrap makes errors.Is(err, ErrLeaseHeld) report true for a refusal.
func (e *HeldError) Unwrap() error { return ErrLeaseHeld }

// AcquireLease takes the lease for holder until ttl has passed since now.
//
// A lease another holder took and that has not expired is refused, with an
// error that wraps ErrLeaseHeld. A lease whose expiry has passed — a scheduler
// that was killed instead of stopping — is taken over: the holder of a state is
// the scheduler that is alive, not the one that was there first. Taking the
// lease again as its own holder succeeds, so that a scheduler coming back
// without its identity changing does not refuse itself.
func (s *Store) AcquireLease(ctx context.Context, holder string, now time.Time, ttl time.Duration) (Lease, error) {
	lease := Lease{Holder: holder, AcquiredAt: now, ExpiresAt: now.Add(ttl)}

	var held *Lease
	err := s.inTransaction(ctx, func(tx *sql.Tx) error {
		current, found, err := currentLease(ctx, tx)
		if err != nil {
			return err
		}
		if found && current.Holder != holder && current.ExpiresAt.After(now) {
			held = &current
			return nil
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO scheduler_lease (id, holder, acquired_at, expires_at)
			VALUES (1, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				holder = excluded.holder,
				acquired_at = excluded.acquired_at,
				expires_at = excluded.expires_at`,
			lease.Holder, formatTime(lease.AcquiredAt), formatTime(lease.ExpiresAt))
		if err != nil {
			return fmt.Errorf("taking the lease for %s: %w", holder, err)
		}
		return nil
	})
	if err != nil {
		return Lease{}, err
	}
	if held != nil {
		return Lease{}, &HeldError{Lease: *held}
	}
	return lease, nil
}

// RenewLease extends the lease of holder to ttl after now. It fails when holder
// does not hold the lease, which is what a scheduler that was taken over learns
// when it tries to keep the state.
func (s *Store) RenewLease(ctx context.Context, holder string, now time.Time, ttl time.Duration) error {
	return s.changeLease(ctx, holder,
		`UPDATE scheduler_lease SET expires_at = ? WHERE id = 1 AND holder = ?`,
		formatTime(now.Add(ttl)), holder)
}

// ReleaseLease gives the state back, so that the next scheduler takes it
// without waiting for the lease to expire. It fails when holder does not hold
// the lease.
func (s *Store) ReleaseLease(ctx context.Context, holder string) error {
	return s.changeLease(ctx, holder,
		`DELETE FROM scheduler_lease WHERE id = 1 AND holder = ?`,
		holder)
}

// CurrentLease returns the lease recorded in the state, if there is one.
func (s *Store) CurrentLease(ctx context.Context) (Lease, bool, error) {
	return currentLease(ctx, s.db)
}

// changeLease performs a change that only the holder of the lease may make.
// A change nobody made is reported as the lease not being held, so that a
// scheduler which was taken over learns it when it tries to keep the state.
func (s *Store) changeLease(ctx context.Context, holder, query string, arguments ...any) error {
	result, err := s.db.ExecContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("changing the lease of %s: %w", holder, err)
	}

	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("changing the lease of %s: %w", holder, err)
	}
	if changed == 0 {
		return fmt.Errorf("the lease of %s is not held", holder)
	}
	return nil
}

// currentLease reads the lease through the given handle, so that it can also be
// read inside the transaction that is about to take it.
func currentLease(ctx context.Context, q querier) (Lease, bool, error) {
	var holder, acquiredAt, expiresAt string
	err := q.QueryRowContext(ctx,
		`SELECT holder, acquired_at, expires_at FROM scheduler_lease WHERE id = 1`).
		Scan(&holder, &acquiredAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, false, nil
	}
	if err != nil {
		return Lease{}, false, fmt.Errorf("reading the lease: %w", err)
	}

	acquired, err := parseTime(acquiredAt)
	if err != nil {
		return Lease{}, false, err
	}
	expires, err := parseTime(expiresAt)
	if err != nil {
		return Lease{}, false, err
	}
	return Lease{Holder: holder, AcquiredAt: acquired, ExpiresAt: expires}, true, nil
}
