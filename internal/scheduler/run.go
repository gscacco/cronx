package scheduler

import (
	"context"
	"time"
)

// Run schedules and runs jobs until the context is cancelled, then waits for the
// jobs that are still running.
//
// One scheduler drives a state at a time: the lease is taken before the first
// activation is planned, kept alive while the scheduler runs and given back once
// the jobs that were running have finished, so that a second scheduler is
// refused instead of running the same jobs a second time.
func (s *Scheduler) Run(ctx context.Context) (err error) {
	runCtx, cancel := context.WithCancel(ctx)

	held, err := s.takeLease(runCtx, cancel)
	if err != nil {
		cancel()
		return err
	}

	// The jobs of a scheduler that is stopping are still its own: the state is
	// given back only once they are done.
	defer func() {
		cancel()
		s.running.Wait()
		held.release()

		if lost := held.failure(); lost != nil {
			err = lost
		}
	}()

	if err := s.begin(runCtx); err != nil {
		return err
	}

	for {
		next, ok := s.nextActivation()
		if !ok {
			s.logger.Info("no job has a future activation; waiting to be stopped")
			<-runCtx.Done()
			return nil
		}

		wait := next.Sub(s.clock.Now())
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)

		select {
		case <-runCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
			s.dispatch(runCtx, s.clock.Now())
		}
	}
}

// begin prepares a run: jobs left in progress by a previous process are closed,
// and the schedule is planned again so that activations missed while cronx was
// not running are skipped.
func (s *Scheduler) begin(ctx context.Context) error {
	interrupted, err := s.store.InterruptStaleRuns(ctx, s.clock.Now())
	if err != nil {
		return err
	}
	if interrupted > 0 {
		s.logger.Warn("closed runs left in progress by a previous process", "runs", interrupted)
	}

	s.plan(s.clock.Now())
	s.logger.Info("scheduler started", "jobs", len(s.names))
	return nil
}

// dispatch starts every job that is due.
//
// Each trigger is judged by the overlap policy of its job when it arrives, and
// always before the job waits for a free slot: a trigger that arrives while the
// job is still running is skipped as configured, even when the only slot is the
// one the run in progress is holding.
func (s *Scheduler) dispatch(ctx context.Context, now time.Time) {
	for _, activation := range s.Due(now) {
		s.start(ctx, activation)
	}
}

// start prepares one activation. Whether it leads to a run is decided now, when
// it was triggered, and never revisited; the run itself waits for the previous
// run of the same job and then for a free slot.
func (s *Scheduler) start(ctx context.Context, activation Activation) {
	s.running.Add(1)

	decision := s.acquire(activation.Job)
	if !decision.proceed {
		defer s.running.Done()
		if err := s.recordSkip(ctx, activation.Job); err != nil {
			s.logger.Error("recording a skipped run failed", "job", activation.Job.Name, "error", err)
		}
		return
	}

	go func() {
		defer s.running.Done()
		defer decision.release()

		// The run of the same job comes first: a trigger does not take a slot
		// to sit on while it waits its turn.
		decision.wait()

		select {
		case s.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-s.slots }()

		// The outcome is recorded in the history and in the run log.
		s.attempt(ctx, activation.Job, activation.At)
	}()
}
