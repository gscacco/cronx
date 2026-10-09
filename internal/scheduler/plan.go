package scheduler

import "time"

// Next returns the next activation of a job after the given instant. It reports
// false when the job does not exist, when it is disabled, when its schedule has
// no activation within the search horizon, and when the job runs when the
// scheduler starts rather than on the clock.
func (s *Scheduler) Next(name string, after time.Time) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	parsed, known := s.schedules[name]
	if !known || !s.scheduledLocked(name) {
		return time.Time{}, false
	}
	return parsed.Next(after)
}

// RunsAtStartup reports whether a job is triggered by the scheduler starting
// rather than by the clock. It reports false for a job that does not exist and
// for a job that is disabled, which is never triggered at all.
func (s *Scheduler) RunsAtStartup(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	parsed, known := s.schedules[name]
	return known && s.scheduledLocked(name) && parsed.RunsAtStartup()
}

// Startup returns the activations a starting scheduler owes to the jobs that
// run at startup rather than on the clock. The instant of each of them is the
// one the scheduler started at, which is what the run is recorded against.
func (s *Scheduler) Startup(now time.Time) []Activation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var starting []Activation
	for _, name := range s.names {
		if !s.scheduledLocked(name) || !s.schedules[name].RunsAtStartup() {
			continue
		}
		starting = append(starting, Activation{Job: s.config.Jobs[name], At: now})
	}
	return starting
}

// Due returns the activations that have arrived at or before now and schedules
// the next one of each.
//
// Every activation is reported at most once: when the scheduler has been unable
// to keep up, the activations in between are skipped rather than replayed.
func (s *Scheduler) Due(now time.Time) []Activation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dueLocked(now)
}

// dueLocked is Due with the lock already held.
func (s *Scheduler) dueLocked(now time.Time) []Activation {
	var due []Activation
	for _, name := range s.names {
		at, known := s.next[name]
		if !known || at.After(now) {
			continue
		}
		due = append(due, Activation{Job: s.config.Jobs[name], At: at})
		s.advanceLocked(name, now)
	}
	return due
}

// plan sets the next activation of every job to the first one after the given
// instant.
func (s *Scheduler) plan(after time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planLocked(after)
}

// planLocked is plan with the lock already held.
func (s *Scheduler) planLocked(after time.Time) {
	for _, name := range s.names {
		s.advanceLocked(name, after)
	}
}

// advanceLocked moves the next activation of a job to the first one after the
// given instant, forgetting the job when it has none. A job the configuration
// disables is forgotten as well: it has no activation to wait for, and enabling
// it again is what plans one.
func (s *Scheduler) advanceLocked(name string, after time.Time) {
	if !s.scheduledLocked(name) {
		delete(s.next, name)
		return
	}
	next, ok := s.schedules[name].Next(after)
	if !ok {
		delete(s.next, name)
		return
	}
	s.next[name] = next
}

// nextActivation returns the earliest activation across every job.
func (s *Scheduler) nextActivation() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var earliest time.Time
	found := false
	for _, at := range s.next {
		if !found || at.Before(earliest) {
			earliest = at
			found = true
		}
	}
	return earliest, found
}

// jobList returns the names of the configured jobs, copied so that a caller can
// work with them while a reload replaces them.
func (s *Scheduler) jobList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.names...)
}
