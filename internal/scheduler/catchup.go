package scheduler

import (
	"context"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/schedule"
)

// catchUp returns the runs a starting scheduler owes to the jobs that ask for
// them: one per job that became due while cronx was not running.
//
// Only a job that asks with `catch_up = true` is considered, and it is given
// one run however many of its activations passed while it was not running, so
// that starting again after a long stop does not produce a burst. What says how
// far back to look is the history of the job: the newest run recorded for it is
// the last instant cronx looked at it, and an activation after that one was
// missed.
//
// A job that has never run is owed nothing — there is no instant to count its
// missed activations from — and neither is a job whose schedule has no
// activation on the clock, which is `@reboot`: its own trigger is the start.
func (s *Scheduler) catchUp(ctx context.Context, now time.Time) []Activation {
	// catching is a job that asks to be caught up, with the schedule parsed
	// for it. Both are read under the lock, so that a reload cannot change
	// the job between the decision and the run.
	type catching struct {
		definition job.Job
		parsed     schedule.Schedule
	}

	s.mu.RLock()
	candidates := make([]catching, 0, len(s.names))
	for _, name := range s.names {
		definition := s.config.Jobs[name]
		if !definition.CatchUp || definition.Disabled {
			continue
		}
		candidates = append(candidates, catching{definition: definition, parsed: s.schedules[name]})
	}
	s.mu.RUnlock()

	var owed []Activation
	for _, candidate := range candidates {
		at, missed, err := s.missed(ctx, candidate.definition, candidate.parsed, now)
		if err != nil {
			s.logger.Error("working out what a job missed failed",
				"job", candidate.definition.Name, "error", err)
			continue
		}
		if missed {
			owed = append(owed, Activation{Job: candidate.definition, At: at})
		}
	}
	return owed
}

// missed returns the first activation of a job that was missed because the
// scheduler was not running, which is the first one after the last run of the
// job when it is no longer in the future.
//
// The last run of a job is read from the history, where a run is recorded
// before the job is executed: the anchor a scheduler that was killed leaves
// behind is therefore the run in progress, not the one before it.
func (s *Scheduler) missed(ctx context.Context, definition job.Job, parsed schedule.Schedule, now time.Time) (time.Time, bool, error) {
	runs, err := s.store.Runs(ctx, definition.Name, 1)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(runs) == 0 {
		return time.Time{}, false, nil
	}

	next, ok := parsed.Next(runs[0].StartedAt)
	if !ok || next.After(now) {
		return time.Time{}, false, nil
	}
	return next, true, nil
}
