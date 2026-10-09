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
		case <-s.reload:
			// A reload moved the activations, so the wait is worked out
			// again rather than served to the end.
			timer.Stop()
			continue
		case <-timer.C:
			s.dispatch(runCtx, s.clock.Now())
		}
	}
}

// begin prepares a run: jobs left in progress by a previous process are closed,
// the history is trimmed to what the configuration keeps, the schedule is
// planned again so that activations missed while cronx was not running are
// skipped, and the jobs that run when the scheduler starts are triggered, since
// this start is their activation.
func (s *Scheduler) begin(ctx context.Context) error {
	interrupted, err := s.store.InterruptStaleRuns(ctx, s.clock.Now())
	if err != nil {
		return err
	}
	if interrupted > 0 {
		s.logger.Warn("closed runs left in progress by a previous process", "runs", interrupted)
	}

	s.plan(s.clock.Now())
	names := s.jobList()
	for _, name := range names {
		s.prune(ctx, name)
	}
	for _, activation := range s.Startup(s.clock.Now()) {
		s.start(ctx, activation)
	}
	s.logger.Info("scheduler started", "jobs", len(names))
	return nil
}

// dispatch starts every job that is due.
//
// Each trigger is judged by the overlap policy of its job when it arrives, and
// always before the job waits for a free slot: a trigger that arrives while the
// job is still running is skipped as configured, even when the only slot is the
// one the run in progress is holding. The history of a job that is due is
// trimmed as well, so that one left running for months does not grow without
// bound either.
func (s *Scheduler) dispatch(ctx context.Context, now time.Time) {
	for _, activation := range s.Due(now) {
		s.start(ctx, activation)
		s.prune(ctx, activation.Job.Name)
	}
}

// prune trims the history of a job to the number of runs the configuration
// keeps. A history that cannot be trimmed is reported but does not stop the
// scheduler: running the jobs matters more than the bookkeeping.
func (s *Scheduler) prune(ctx context.Context, name string) {
	s.mu.RLock()
	keep := s.config.Storage.MaxRuns
	s.mu.RUnlock()
	if keep < 1 {
		return
	}
	deleted, err := s.store.PruneRuns(ctx, name, keep)
	if err != nil {
		s.logger.Error("pruning the history failed", "job", name, "error", err)
		return
	}
	if deleted > 0 {
		s.logger.Info("pruned the history", "job", name, "runs", deleted, "kept", keep)
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
