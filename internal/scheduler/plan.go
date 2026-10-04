package scheduler

import "time"

// Next returns the next activation of a job after the given instant. It reports
// false when the job does not exist, or when its schedule has no activation
// within the search horizon.
func (s *Scheduler) Next(name string, after time.Time) (time.Time, bool) {
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
	var due []Activation
	for _, name := range s.names {
		at, known := s.next[name]
		if !known || at.After(now) {
			continue
		}
		due = append(due, Activation{Job: s.config.Jobs[name], At: at})
		s.advance(name, now)
	}
	return due
}

// plan sets the next activation of every job to the first one after the given
// instant.
func (s *Scheduler) plan(after time.Time) {
	for _, name := range s.names {
		s.advance(name, after)
	}
}

// advance moves the next activation of a job to the first one after the given
// instant, forgetting the job when it has none.
func (s *Scheduler) advance(name string, after time.Time) {
	next, ok := s.schedules[name].Next(after)
	if !ok {
		delete(s.next, name)
		return
	}
	s.next[name] = next
}

// nextActivation returns the earliest activation across every job.
func (s *Scheduler) nextActivation() (time.Time, bool) {
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
