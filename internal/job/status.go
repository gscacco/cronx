package job

// Status describes the state of a single execution of a job.
type Status string

const (
	// StatusScheduled means the run is known but has not started yet.
	StatusScheduled Status = "scheduled"
	// StatusRunning means the job is executing.
	StatusRunning Status = "running"
	// StatusSucceeded means the job finished with exit code zero.
	StatusSucceeded Status = "succeeded"
	// StatusFailed means the job ran and did not succeed.
	StatusFailed Status = "failed"
	// StatusTimedOut means the job was stopped because it exceeded its timeout.
	StatusTimedOut Status = "timed_out"
	// StatusSpawnError means the job could not be started at all.
	StatusSpawnError Status = "spawn_error"
	// StatusSkipped means the trigger did not lead to an execution, for example
	// because of the overlap policy.
	StatusSkipped Status = "skipped"
)

// IsTerminal reports whether the status is final, that is whether no further
// transition is expected.
func (s Status) IsTerminal() bool {
	return s != StatusScheduled && s != StatusRunning
}

// IsSuccessful reports whether the run completed successfully.
func (s Status) IsSuccessful() bool {
	return s == StatusSucceeded
}
