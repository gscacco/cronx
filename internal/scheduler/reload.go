package scheduler

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/schedule"
)

// Reload replaces the jobs the scheduler runs with those of the given
// configuration, and plans their activations again.
//
// A reload takes effect from the moment it is applied: a job whose schedule
// changed follows the new one, a job that is gone is not run again, and a job
// that is new is picked up. Runs in progress are left alone, and a run waiting
// for the run of its own job still waits for that one, because the lock the
// overlap policy is applied with belongs to the job name and is kept across a
// reload.
//
// A change to the settings of the scheduler itself — the timezone, the parallel
// limit, the log path, the state path — cannot reach a process that is already
// running: it is reported and ignored, and needs a restart. A configuration
// that cannot be used is refused whole, so the scheduler keeps running with the
// one it has rather than half of two.
func (s *Scheduler) Reload(configuration config.Config) error {
	schedules := make(map[string]schedule.Schedule, len(configuration.Jobs))
	names := make([]string, 0, len(configuration.Jobs))
	for name, definition := range configuration.Jobs {
		parsed, err := schedule.Parse(definition.Schedule)
		if err != nil {
			return fmt.Errorf("job %q: %w", name, err)
		}
		schedules[name] = parsed
		names = append(names, name)
	}
	sort.Strings(names)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.reportIgnored(configuration)
	s.reportJobChanges(names)
	s.reportDisabledJobs(configuration)
	s.reportStartupJobs(configuration, schedules)

	s.config.Jobs = configuration.Jobs
	s.schedules = schedules
	s.names = names
	for _, name := range names {
		if _, known := s.locks[name]; !known {
			s.locks[name] = &sync.Mutex{}
		}
	}

	// The activations are planned from scratch, so that a job which is gone
	// leaves nothing behind and a job which is new is waited for.
	s.next = make(map[string]time.Time, len(names))
	s.planLocked(s.clock.Now())

	// The run loop is waiting for an activation that may have moved, so it is
	// woken up to work the wait out again.
	select {
	case s.reload <- struct{}{}:
	default:
	}
	return nil
}

// reportIgnored names the settings a reload cannot apply, so that a person is
// told why the change they made did nothing instead of finding it out later.
// The caller holds the lock.
func (s *Scheduler) reportIgnored(configuration config.Config) {
	settings := []struct {
		name    string
		running any
		loaded  any
	}{
		{"scheduler.timezone", s.config.Scheduler.Timezone, configuration.Scheduler.Timezone},
		{"scheduler.max_parallel_jobs", s.config.Scheduler.MaxParallelJobs, configuration.Scheduler.MaxParallelJobs},
		{"logging.level", s.config.Logging.Level, configuration.Logging.Level},
		{"logging.path", s.config.Logging.Path, configuration.Logging.Path},
		{"logging.max_size", s.config.Logging.MaxSize, configuration.Logging.MaxSize},
		{"logging.max_backups", s.config.Logging.MaxBackups, configuration.Logging.MaxBackups},
		{"storage.path", s.config.Storage.Path, configuration.Storage.Path},
		{"storage.max_runs", s.config.Storage.MaxRuns, configuration.Storage.MaxRuns},
	}
	for _, setting := range settings {
		if setting.running == setting.loaded {
			continue
		}
		s.logger.Warn("a setting changed and needs a restart, so the running one is kept",
			"setting", setting.name, "running", setting.running, "loaded", setting.loaded)
	}
}

// reportJobChanges says which jobs a reload added and which it left behind, so
// that the scheduler log explains what the new configuration did. The caller
// holds the lock.
func (s *Scheduler) reportJobChanges(names []string) {
	before := make(map[string]bool, len(s.names))
	for _, name := range s.names {
		before[name] = true
	}

	after := make(map[string]bool, len(names))
	var added []string
	for _, name := range names {
		after[name] = true
		if !before[name] {
			added = append(added, name)
		}
	}
	var removed []string
	for _, name := range s.names {
		if !after[name] {
			removed = append(removed, name)
		}
	}

	if len(added) > 0 {
		s.logger.Info("jobs added by the reload", "jobs", strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		s.logger.Info("jobs removed by the reload", "jobs", strings.Join(removed, ", "))
	}
}

// reportDisabledJobs says which jobs a reload stopped scheduling and which it
// started scheduling again. A job the configuration keeps without running it is
// otherwise only visible in `cronx list`, so the log says why it is not being
// triggered any more. The caller holds the lock.
func (s *Scheduler) reportDisabledJobs(configuration config.Config) {
	var disabled, enabled []string
	for name, definition := range configuration.Jobs {
		previous, known := s.config.Jobs[name]
		switch {
		case !known:
			continue // a job the reload added, which is reported as added
		case definition.Disabled && !previous.Disabled:
			disabled = append(disabled, name)
		case !definition.Disabled && previous.Disabled:
			enabled = append(enabled, name)
		}
	}

	for _, group := range []struct {
		names   []string
		message string
	}{
		{disabled, "jobs disabled by the reload, which are not scheduled"},
		{enabled, "jobs enabled by the reload, which are scheduled again"},
	} {
		if len(group.names) == 0 {
			continue
		}
		sort.Strings(group.names)
		s.logger.Info(group.message, "jobs", strings.Join(group.names, ", "))
	}
}

// reportStartupJobs names the jobs a reload turns into jobs that run when the
// scheduler starts. A scheduler that is already running is not a start, so they
// are not run now: they wait for the next one, and a reload says so instead of
// leaving a person waiting for a run that is never coming. A job that is
// disabled is left out, because it is not run at the next start either. The
// caller holds the lock.
func (s *Scheduler) reportStartupJobs(configuration config.Config, schedules map[string]schedule.Schedule) {
	var waiting []string
	for name, definition := range configuration.Jobs {
		if definition.Disabled || !schedules[name].RunsAtStartup() {
			continue
		}
		if previous, known := s.config.Jobs[name]; known && !previous.Disabled && s.schedules[name].RunsAtStartup() {
			continue // already one of them: it ran when this scheduler started
		}
		waiting = append(waiting, name)
	}
	if len(waiting) == 0 {
		return
	}
	sort.Strings(waiting)
	s.logger.Warn("jobs that run when the scheduler starts were added, and are run at the next start",
		"jobs", strings.Join(waiting, ", "))
}
