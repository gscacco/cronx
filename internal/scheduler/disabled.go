package scheduler

// A job the configuration disables is kept rather than removed: the scheduler
// holds it, `cronx list` names it and its history stays where it is, but it is
// not scheduled — no activation is planned for it, it is never triggered and it
// is never caught up — until it is enabled again.

// Disabled reports whether a job is configured but not scheduled. It reports
// false for a job that does not exist.
func (s *Scheduler) Disabled(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definition, known := s.config.Jobs[name]
	return known && definition.Disabled
}

// scheduledLocked reports whether a job is one the scheduler runs. The caller
// holds the lock.
func (s *Scheduler) scheduledLocked(name string) bool {
	definition, known := s.config.Jobs[name]
	return known && !definition.Disabled
}

// disabledJobs returns the names of the jobs the configuration keeps without
// scheduling them, in the order the scheduler holds them.
func (s *Scheduler) disabledJobs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var disabled []string
	for _, name := range s.names {
		if s.config.Jobs[name].Disabled {
			disabled = append(disabled, name)
		}
	}
	return disabled
}
