// Package scheduler decides when jobs run and executes them.
//
// The scheduler is the only component that turns the desired configuration into
// executions. It records everything it does in the store and captures the
// output of every run in the shared log.
package scheduler

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/schedule"
	"gscacco.com/cronx/internal/store"
)

// Options describes everything a Scheduler needs.
type Options struct {
	// Config is the desired state: which jobs exist and how they are run.
	Config config.Config
	// Store persists the execution history and the runtime state.
	Store *store.Store
	// Logs receives the output of every run: a single file shared by all jobs.
	Logs *logx.Log
	// Runner executes the jobs.
	Runner *runner.Runner
	// Clock is the source of the current time.
	Clock clock.Clock
	// Logger receives what the scheduler itself does.
	Logger *slog.Logger
	// Holder is the identity recorded as the holder of the state, so that a
	// second scheduler can be told who has it. Empty means this machine and
	// this process, which is what a scheduler started by a person is.
	Holder string
}

// Activation is a job that is due to run.
type Activation struct {
	// Job is the job to run.
	Job job.Job
	// At is the instant the run was scheduled for.
	At time.Time
}

// Scheduler runs the configured jobs when they are due.
type Scheduler struct {
	store  *store.Store
	logs   *logx.Log
	runner *runner.Runner
	clock  clock.Clock
	logger *slog.Logger
	holder string

	// mu guards everything a reload can replace while the scheduler runs: the
	// configuration, the schedules parsed from it, the locks that apply the
	// overlap policies and the activations planned for the jobs.
	mu        sync.RWMutex
	config    config.Config
	schedules map[string]schedule.Schedule
	names     []string
	next      map[string]time.Time
	locks     map[string]*sync.Mutex

	// slots bounds how many jobs run at the same time. It is set once, when
	// the scheduler is built: a reload applies the jobs, not the limit.
	slots chan struct{}
	// reload wakes the run loop when a reload has moved an activation the
	// loop is waiting for.
	reload  chan struct{}
	running sync.WaitGroup
}

// New builds a Scheduler. It fails when the configuration cannot be prepared,
// for example because a job has an invalid schedule.
func New(options Options) (*Scheduler, error) {
	switch {
	case options.Store == nil:
		return nil, errors.New("a store is required")
	case options.Logs == nil:
		return nil, errors.New("a run log is required")
	case options.Runner == nil:
		return nil, errors.New("a runner is required")
	case options.Clock == nil:
		return nil, errors.New("a clock is required")
	case options.Logger == nil:
		return nil, errors.New("a logger is required")
	}

	parallel := options.Config.Scheduler.MaxParallelJobs
	if parallel < 1 {
		parallel = 1
	}

	holder := options.Holder
	if holder == "" {
		holder = leaseHolder()
	}

	built := &Scheduler{
		config:    options.Config,
		store:     options.Store,
		logs:      options.Logs,
		runner:    options.Runner,
		clock:     options.Clock,
		logger:    options.Logger,
		holder:    holder,
		schedules: make(map[string]schedule.Schedule, len(options.Config.Jobs)),
		next:      make(map[string]time.Time, len(options.Config.Jobs)),
		locks:     make(map[string]*sync.Mutex, len(options.Config.Jobs)),
		slots:     make(chan struct{}, parallel),
		reload:    make(chan struct{}, 1),
	}

	names := make([]string, 0, len(options.Config.Jobs))
	for name, definition := range options.Config.Jobs {
		parsed, err := schedule.Parse(definition.Schedule)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", name, err)
		}
		built.schedules[name] = parsed
		built.locks[name] = &sync.Mutex{}
		names = append(names, name)
	}
	sort.Strings(names)
	built.names = names

	built.plan(built.clock.Now())
	return built, nil
}
