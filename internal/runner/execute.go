package runner

import (
	"context"
	"os/exec"
	"time"
)

// wait waits for the process to finish, stopping it when the context is
// cancelled or the timeout expires.
func (r *Runner) wait(ctx context.Context, process *exec.Cmd, command Command, result *Result) error {
	waited := make(chan error, 1)
	go func() {
		waited <- process.Wait()
	}()

	var timeout <-chan time.Time
	if command.Timeout > 0 {
		timer := time.NewTimer(command.Timeout)
		defer timer.Stop()
		timeout = timer.C
	}

	select {
	case err := <-waited:
		return err
	case <-timeout:
		result.TimedOut = true
	case <-ctx.Done():
	}

	return stopProcess(waited, process, gracePeriod(command))
}

// stopProcess asks the job to stop and waits for it, killing it when it is
// still running once the grace period has elapsed.
func stopProcess(waited <-chan error, process *exec.Cmd, grace time.Duration) error {
	if process.Process == nil {
		return <-waited
	}

	terminateGroup(process.Process)

	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case err := <-waited:
		return err
	case <-timer.C:
		killGroup(process.Process)
		return <-waited
	}
}

// gracePeriod returns the grace period to apply to a job.
func gracePeriod(command Command) time.Duration {
	if command.GracePeriod > 0 {
		return command.GracePeriod
	}
	return DefaultGracePeriod
}

// exitCode returns the exit status reported for the process.
func exitCode(process *exec.Cmd, _ error) int {
	if process.ProcessState == nil {
		return exitedBySignal
	}
	return process.ProcessState.ExitCode()
}
