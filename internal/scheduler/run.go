package scheduler

import (
	"context"
	"time"
)

// Run schedules and runs jobs until the context is cancelled, then waits for the
// jobs that are still running.
func (s *Scheduler) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		s.running.Wait()
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

// dispatch starts every job that is due. A job waits for a free slot, so at
// most the configured number of jobs run at the same time.
func (s *Scheduler) dispatch(ctx context.Context, now time.Time) {
	for _, activation := range s.Due(now) {
		s.running.Add(1)
		go func(activation Activation) {
			defer s.running.Done()

			select {
			case s.slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-s.slots }()

			if _, err := s.Execute(ctx, activation.Job.Name, activation.At); err != nil {
				s.logger.Error("running a job failed", "job", activation.Job.Name, "error", err)
			}
		}(activation)
	}
}
