package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/store"
)

// LeaseTTL is how long the lease of a scheduler stays valid without being
// renewed. It is the state a scheduler killed outright holds for, so it is also
// how long such a state waits before another scheduler can take it over.
const LeaseTTL = 30 * time.Second

// leaseRenewal is how often the lease is renewed while the scheduler runs.
// Three renewals fit in a lifetime, so that one failure to reach the database
// does not cost the scheduler its state.
const leaseRenewal = LeaseTTL / 3

// leaseHolder is the identity a scheduler records when it takes a state: the
// machine it runs on and the process it is. A refusal can then name who holds
// the state, and a person can find that process.
func leaseHolder() string {
	machine, err := os.Hostname()
	if err != nil {
		machine = "unknown"
	}
	return fmt.Sprintf("%s:%d", machine, os.Getpid())
}

// stateLease is the hold a scheduler has on a state: the row it recorded, and
// the renewal that keeps it from expiring while the scheduler runs.
type stateLease struct {
	store  *store.Store
	clock  clock.Clock
	lease  store.Lease
	logger *slog.Logger

	// stopped is closed when the renewal has ended.
	stopped chan struct{}
	// lost carries the failure that ended the renewal, when it was one.
	lost chan error
}

// takeLease takes the state for this scheduler and starts keeping it.
//
// A state another scheduler holds, and that has not expired, is refused: it is
// the second scheduler that gives way, not the one already driving the jobs. A
// renewal that fails later stops the scheduler through stop, because a
// scheduler that no longer holds the state must not go on driving it.
func (s *Scheduler) takeLease(ctx context.Context, stop context.CancelFunc) (*stateLease, error) {
	lease, err := s.store.AcquireLease(ctx, s.holder, s.clock.Now(), LeaseTTL)
	if err != nil {
		s.logger.Error("refusing to start", "error", err)
		return nil, err
	}

	held := &stateLease{
		store:   s.store,
		clock:   s.clock,
		lease:   lease,
		logger:  s.logger,
		stopped: make(chan struct{}),
		lost:    make(chan error, 1),
	}
	s.logger.Info("the state is held",
		"holder", lease.Holder, "until", lease.ExpiresAt.UTC().Format(time.RFC3339))

	go held.renew(ctx, stop)
	return held, nil
}

// renew keeps the lease from expiring until the scheduler stops. A renewal that
// fails means the state is no longer this scheduler's, so the scheduler is
// stopped, reporting the failure that caused it.
func (l *stateLease) renew(ctx context.Context, stop context.CancelFunc) {
	defer close(l.stopped)

	ticker := time.NewTicker(leaseRenewal)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := l.store.RenewLease(ctx, l.lease.Holder, l.clock.Now(), LeaseTTL); err != nil {
				l.logger.Error("the state was lost", "error", err)
				l.lost <- err
				stop()
				return
			}
		}
	}
}

// release gives the state back, once the renewal has ended.
func (l *stateLease) release() {
	<-l.stopped

	// The scheduler is stopping, so the contexts it ran with are cancelled
	// already and cannot carry the last write it makes.
	if err := l.store.ReleaseLease(context.Background(), l.lease.Holder); err != nil {
		l.logger.Warn("giving the state back failed", "error", err)
	}
}

// failure returns the renewal failure that stopped the scheduler, if there was
// one. It is read after the run loop has ended, which is after the renewal
// reported the failure.
func (l *stateLease) failure() error {
	select {
	case err := <-l.lost:
		return err
	default:
		return nil
	}
}
