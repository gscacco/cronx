package scheduler

import "time"

// Next returns the next activation of a job after the given instant. It reports
// false when the job does not exist, or when its schedule has no activation
// within the search horizon.
func (s *Scheduler) Next(name string, after time.Time) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	parsed, known := s.schedules[name]
	if !known {
		return time.Time{}, false
	}
	return parsed.Next(after)
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
// given instant, forgetting the job when it has none.
func (s *Scheduler) advanceLocked(name string, after time.Time) {
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
