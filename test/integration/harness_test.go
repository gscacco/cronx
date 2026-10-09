package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// The placeholders a configuration fixture uses for the paths only a test
// knows.
const (
	// scratchPlaceholder is replaced by the scratch directory of the test.
	scratchPlaceholder = "@TEST_DIR@"
	// jobHelperPlaceholder is replaced by the compiled job program.
	jobHelperPlaceholder = "@JOB_HELPER@"
)

// The budgets the tests wait with. They are generous on purpose: a cron
// expression schedules to the second at its finest, but the scheduling tests
// use five-field expressions, whose activations are a minute apart, so "the job
// runs soon" can legitimately take almost a minute.
//
// Only the scheduling tests wait that long, because they are the only ones that
// need a real activation: they run in parallel, and the fast suite (go test
// -short) skips them. Everything else waits for a process to do something it was
// asked to do, which takes at most the grace period of a job that refuses to
// stop.
const (
	// pollInterval is how often a test looks again at what it waits for.
	pollInterval = 100 * time.Millisecond
	// activationBudget covers the wait for the next minute boundary plus the
	// time the run itself needs.
	activationBudget = 75 * time.Second
	// secondsBudget covers an activation that is seconds away, which is what a
	// schedule with a seconds field has. It is a fraction of activationBudget
	// on purpose: a test that waits for it is watching a job fire inside the
	// minute rather than on it.
	secondsBudget = 30 * time.Second
	// overlapBudget covers two activations, because a run is only skipped by
	// the activation that arrives after the first one.
	overlapBudget = 150 * time.Second
	// settleBudget covers what happens without a new activation: a process
	// being stopped, a run being closed when the scheduler starts again.
	settleBudget = 30 * time.Second
	// commandBudget bounds a command that is expected to finish on its own.
	commandBudget = 60 * time.Second
	// stopBudget is how long a scheduler is given to stop after SIGTERM
	// before it is killed. It is longer than the grace period a job is given
	// before it is killed, so that stopping a busy scheduler is not mistaken
	// for it hanging.
	stopBudget = 30 * time.Second
)

// environment is one isolated cronx installation: its own home directory, its
// own configuration, its own state database and its own logs. Nothing a test
// does can reach the real ones.
type environment struct {
	t          *testing.T
	home       string
	dir        string
	database   *store.Store
	started    int
	schedulers int
}

// newEnvironment prepares an isolated home for a test.
func newEnvironment(t *testing.T) *environment {
	t.Helper()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	dir := filepath.Join(base, "scratch")
	for _, path := range []string{home, dir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("creating %s: %v", path, err)
		}
	}
	return &environment{t: t, home: home, dir: dir}
}

// path returns the path of a file in the scratch directory of the test, which
// is where the configurations keep everything a job produces.
func (e *environment) path(name string) string {
	return filepath.Join(e.dir, name)
}

// homePath returns the path of a file inside the home directory of the test.
func (e *environment) homePath(name ...string) string {
	return filepath.Join(append([]string{e.home}, name...)...)
}

// defaultConfigPath returns where cronx looks for its configuration when
// nothing selects another file.
func (e *environment) defaultConfigPath() string {
	return e.homePath(".cronx", "config.toml")
}

// statePath returns the path of the state database.
func (e *environment) statePath() string {
	return e.homePath(".cronx", "state.db")
}

// logsDirectory returns the directory the log files live in.
func (e *environment) logsDirectory() string {
	return e.homePath(".cronx", "logs")
}

// runsLog returns the path of the log every run writes to: a single file
// shared by all the jobs of the installation.
func (e *environment) runsLog() string {
	return filepath.Join(e.logsDirectory(), "runs.log")
}

// schedulerLog returns the path of the scheduler's own log file.
func (e *environment) schedulerLog() string {
	return filepath.Join(e.logsDirectory(), "cronx.log")
}

// makeDirectory creates a directory in the scratch directory of the test and
// returns its path.
func (e *environment) makeDirectory(name string) string {
	e.t.Helper()
	path := e.path(name)
	if err := os.MkdirAll(path, 0o700); err != nil {
		e.t.Fatalf("creating %s: %v", path, err)
	}
	return path
}

// makeFile writes a file, failing the test when it cannot be written.
func (e *environment) makeFile(path, content string) {
	e.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatalf("writing %s: %v", path, err)
	}
}

// readFile returns the content of a file, failing the test when it cannot be
// read.
func (e *environment) readFile(path string) string {
	e.t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}

// lines returns the newline-terminated lines of a file, so that a file a job
// appends to can be counted without looking at a half written line.
func (e *environment) lines(path string) []string {
	e.t.Helper()
	var complete []string
	for _, line := range strings.Split(e.readFile(path), "\n") {
		if line != "" {
			complete = append(complete, line)
		}
	}
	return complete
}

// configure renders a configuration fixture into the scratch directory of the
// test and returns the path of the file.
func (e *environment) configure(fixture string) string {
	e.t.Helper()
	return e.configureAt(fixture, e.path(filepath.Base(fixture)))
}

// configureAt renders a configuration fixture to the given path.
func (e *environment) configureAt(fixture, path string) string {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		e.t.Fatalf("creating the directory of %s: %v", path, err)
	}

	contents := e.readFile(filepath.Join("testdata", fixture))
	contents = strings.NewReplacer(
		scratchPlaceholder, e.dir,
		jobHelperPlaceholder, jobHelper,
	).Replace(contents)

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		e.t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// childEnvironment is the environment a cronx process is started with. It is
// deliberately minimal, and it never inherits the environment of the test run:
// a test that proves a job does not inherit the environment of the scheduler
// would otherwise prove nothing.
func (e *environment) childEnvironment(extra ...string) []string {
	environment := []string{
		"HOME=" + e.home,
		"PATH=/usr/bin:/bin",
		// A fixed time zone keeps the instants the tests read comparable,
		// whatever the zone of the machine running them.
		"TZ=UTC",
	}
	return append(environment, extra...)
}

// result is what a finished cronx process printed and the status it exited
// with.
type result struct {
	stdout string
	stderr string
	code   int
}

// run executes cronx to completion and returns what it produced.
func (e *environment) run(args ...string) result {
	e.t.Helper()
	return e.runWithEnvironment(nil, args...)
}

// runWithEnvironment executes cronx to completion with extra environment
// variables.
func (e *environment) runWithEnvironment(extra []string, args ...string) result {
	e.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()

	command := exec.CommandContext(ctx, cronxBinary, args...)
	command.Env = e.childEnvironment(extra...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr

	err := command.Run()

	outcome := result{stdout: stdout.String(), stderr: stderr.String()}
	var exited *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exited):
		outcome.code = exited.ExitCode()
	case ctx.Err() != nil:
		e.t.Fatalf("cronx %v did not finish within %s\nstdout:\n%s\nstderr:\n%s",
			args, commandBudget, outcome.stdout, outcome.stderr)
	default:
		e.t.Fatalf("running cronx %v: %v", args, err)
	}
	return outcome
}

// runOK executes cronx and fails the test unless it succeeds.
func (e *environment) runOK(args ...string) result {
	e.t.Helper()
	outcome := e.run(args...)
	if outcome.code != 0 {
		e.t.Fatalf("cronx %v exited with status %d, want success\nstdout:\n%s\nstderr:\n%s",
			args, outcome.code, outcome.stdout, outcome.stderr)
	}
	return outcome
}

// runOnce runs one job immediately and returns what the command produced.
func (e *environment) runOnce(configPath, name string) result {
	e.t.Helper()
	return e.run("run-once", name, "--config", configPath)
}

// startScheduler starts the scheduler and waits until it has opened its state:
// only then is it safe for the test to read the database, because the first
// thing a scheduler does is migrate it. A test that reads the database while a
// scheduler is migrating it would be racing the migration.
func (e *environment) startScheduler(configPath string) *process {
	e.t.Helper()

	e.schedulers++
	wanted := e.schedulers
	running := e.start("run", "--config", configPath)
	e.waitFor("the scheduler to start", activationBudget, func() bool {
		if !running.running() {
			e.t.Fatalf("the scheduler exited before it started\nstdout:\n%s\nstderr:\n%s",
				running.stdout(), running.stderr())
		}
		return strings.Count(readProcessOutput(e.schedulerLog()), schedulerStarted) >= wanted
	})
	return running
}

// schedulerStarted is what a scheduler records once it has opened its state and
// planned the activations.
const schedulerStarted = "scheduler started"

// process is a cronx process that is expected to keep running. Its output is
// written to files in the scratch directory, so that a test which fails while
// the process is still alive can still read it.
type process struct {
	t        *testing.T
	command  *exec.Cmd
	waited   chan error
	outPath  string
	errPath  string
	finished bool
	err      error
}

// start launches cronx in the background and returns the process. The process
// is stopped when the test ends, whether it succeeded or not.
func (e *environment) start(args ...string) *process {
	e.t.Helper()

	e.started++
	outPath := e.path(fmt.Sprintf("cronx-%d.out", e.started))
	errPath := e.path(fmt.Sprintf("cronx-%d.err", e.started))
	stdout, err := os.Create(outPath)
	if err != nil {
		e.t.Fatalf("creating %s: %v", outPath, err)
	}
	stderr, err := os.Create(errPath)
	if err != nil {
		e.t.Fatalf("creating %s: %v", errPath, err)
	}

	command := exec.Command(cronxBinary, args...)
	command.Env = e.childEnvironment()
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		e.t.Fatalf("starting cronx %v: %v", args, err)
	}

	running := &process{
		t:       e.t,
		command: command,
		waited:  make(chan error, 1),
		outPath: outPath,
		errPath: errPath,
	}
	go func() {
		err := command.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		running.waited <- err
	}()

	e.t.Cleanup(func() { running.close() })
	return running
}

// running reports whether the process is still alive.
func (p *process) running() bool {
	p.collect()
	return !p.finished
}

// wait waits for the process to end within the budget, reporting whether it
// did.
func (p *process) wait(budget time.Duration) (int, bool) {
	if !p.collect() {
		select {
		case p.err = <-p.waited:
			p.finished = true
		case <-time.After(budget):
			return 0, false
		}
	}
	return p.exitCode(), true
}

// exitCode returns the status the process exited with.
func (p *process) exitCode() int {
	if p.err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(p.err, &exited) {
		return exited.ExitCode()
	}
	return -1
}

// stop asks the process to stop the way a service manager would and waits for
// it, killing it only when it does not obey. It returns the status the process
// exited with.
func (p *process) stop() int {
	p.t.Helper()
	if !p.running() {
		return p.exitCode()
	}
	if err := p.command.Process.Signal(syscall.SIGTERM); err != nil {
		p.t.Fatalf("asking cronx to stop: %v", err)
	}
	code, stopped := p.wait(stopBudget)
	if !stopped {
		p.t.Errorf("cronx did not stop within %s, so it was killed\nstdout:\n%s\nstderr:\n%s",
			stopBudget, p.stdout(), p.stderr())
		_ = p.command.Process.Kill()
		code, _ = p.wait(stopBudget)
	}
	return code
}

// kill removes the process without giving it a chance to clean up.
func (p *process) kill() {
	p.t.Helper()
	if !p.running() {
		return
	}
	if err := p.command.Process.Kill(); err != nil {
		p.t.Fatalf("killing cronx: %v", err)
	}
	if _, stopped := p.wait(stopBudget); !stopped {
		p.t.Fatalf("cronx survived being killed")
	}
}

// close makes sure the process is gone, and reports what it printed when the
// test failed. It is registered as a cleanup, so that the processes of a
// failing test are removed exactly like those of a passing one.
func (p *process) close() {
	if p.running() {
		_ = p.command.Process.Signal(syscall.SIGTERM)
		if _, stopped := p.wait(stopBudget); !stopped {
			_ = p.command.Process.Kill()
			_, _ = p.wait(stopBudget)
		}
	}
	if p.t.Failed() {
		p.t.Logf("cronx printed while the test ran\nstdout:\n%s\nstderr:\n%s", p.stdout(), p.stderr())
	}
}

// collect reaps the process when it has already ended, reporting whether it
// has.
func (p *process) collect() bool {
	if p.finished {
		return true
	}
	select {
	case p.err = <-p.waited:
		p.finished = true
		return true
	default:
		return false
	}
}

// stdout returns what the process has written so far.
func (p *process) stdout() string { return readProcessOutput(p.outPath) }

// stderr returns what the process has written so far.
func (p *process) stderr() string { return readProcessOutput(p.errPath) }

// readProcessOutput reads the output collected from a process, returning an
// empty string when there is none.
func readProcessOutput(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// defaultRunLimit is how many runs a helper reads at most.
const defaultRunLimit = 20

// state opens the state database of the environment. Reading it while the
// scheduler is running is safe: the database is opened in write-ahead logging
// mode, so a reader sees the latest committed state.
//
// The database is never created by a test: it is created by a cronx process, so
// that a test cannot race the migrations of a process that is starting.
func (e *environment) state() *store.Store {
	e.t.Helper()
	if e.database != nil {
		return e.database
	}
	if _, err := os.Stat(e.statePath()); err != nil {
		e.t.Fatalf("the state database does not exist: %v\n%s", err, e.describe())
	}
	persistent, err := store.Open(e.statePath())
	if err != nil {
		e.t.Fatalf("opening the state database: %v", err)
	}
	e.database = persistent
	e.t.Cleanup(func() { _ = persistent.Close() })
	return persistent
}

// stateInfo describes the state database file, so that a test can tell whether
// a later command replaced it.
func (e *environment) stateInfo() os.FileInfo {
	e.t.Helper()
	info, err := os.Stat(e.statePath())
	if err != nil {
		e.t.Fatalf("looking at the state database: %v", err)
	}
	return info
}

// schemaVersion returns the schema version recorded in the database.
func (e *environment) schemaVersion() int {
	e.t.Helper()
	version, err := e.state().SchemaVersion(context.Background())
	if err != nil {
		e.t.Fatalf("reading the schema version: %v", err)
	}
	return version
}

// runs returns the recorded runs of a job, newest first. An empty name returns
// the runs of every job.
func (e *environment) runs(name string) []store.Run {
	e.t.Helper()
	runs, err := e.state().Runs(context.Background(), name, defaultRunLimit)
	if err != nil {
		e.t.Fatalf("reading the runs of %q: %v", name, err)
	}
	return runs
}

// latestRun returns the run of a job that was recorded last.
func (e *environment) latestRun(name string) store.Run {
	e.t.Helper()
	runs := e.runs(name)
	if len(runs) == 0 {
		e.t.Fatalf("no run of %q has been recorded\n%s", name, e.describe())
	}
	return runs[0]
}

// runningRuns counts the runs of a job that are still in progress.
func (e *environment) runningRuns(name string) int {
	e.t.Helper()
	count, err := e.state().RunningRuns(context.Background(), name)
	if err != nil {
		e.t.Fatalf("counting the runs of %q in progress: %v", name, err)
	}
	return count
}

// waitForRun waits until a run of the job reaches the given status.
func (e *environment) waitForRun(name string, status job.Status, budget time.Duration) store.Run {
	e.t.Helper()
	return e.waitForRunWhere(name, budget, func(run store.Run) bool {
		return run.Status == status
	})
}

// waitForRunWhere waits until a run of the job satisfies want.
func (e *environment) waitForRunWhere(name string, budget time.Duration, want func(store.Run) bool) store.Run {
	e.t.Helper()
	deadline := time.Now().Add(budget)
	for {
		for _, run := range e.runs(name) {
			if want(run) {
				return run
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("no run of %q reached the expected state within %s\n%s", name, budget, e.describe())
		}
		time.Sleep(pollInterval)
	}
}

// waitForRuns waits until the recorded runs of a job satisfy want, and returns
// the runs that did. A wait on a snapshot is what a job that keeps firing asks
// for: a run is written down before the job is executed, so a wait followed by a
// second read can find the next run of the job already in progress.
func (e *environment) waitForRuns(name string, budget time.Duration, want func([]store.Run) bool) []store.Run {
	e.t.Helper()
	deadline := time.Now().Add(budget)
	for {
		runs := e.runs(name)
		if want(runs) {
			return runs
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("the runs of %q never reached the expected state within %s\n%s",
				name, budget, e.describe())
		}
		time.Sleep(pollInterval)
	}
}

// report is what the job helper records about the process it ran in.
type report struct {
	Argv    []string          `json:"argv"`
	Payload []string          `json:"payload"`
	Dir     string            `json:"dir"`
	PID     int               `json:"pid"`
	PGID    int               `json:"pgid"`
	Env     map[string]string `json:"env"`
}

// waitForReport waits until the job helper has written its report and returns
// it.
func (e *environment) waitForReport(path string, budget time.Duration) report {
	e.t.Helper()
	deadline := time.Now().Add(budget)
	for {
		content, err := os.ReadFile(path)
		switch {
		case err == nil:
			var recorded report
			if err := json.Unmarshal(content, &recorded); err != nil {
				e.t.Fatalf("reading the report %s: %v\n%s", path, err, content)
			}
			return recorded
		case !errors.Is(err, os.ErrNotExist):
			e.t.Fatalf("reading the report %s: %v", path, err)
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("%s was not written within %s\n%s", path, budget, e.describe())
		}
		time.Sleep(pollInterval)
	}
}

// waitFor waits until check reports true.
func (e *environment) waitFor(description string, budget time.Duration, check func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(budget)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("waiting for %s: still not true after %s\n%s", description, budget, e.describe())
		}
		time.Sleep(pollInterval)
	}
}

// describe summarises the state of the environment, for a failure message.
func (e *environment) describe() string {
	var summary strings.Builder
	if _, err := os.Stat(e.statePath()); err != nil {
		fmt.Fprintf(&summary, "no state database at %s", e.statePath())
	} else {
		fmt.Fprintf(&summary, "runs recorded:")
		for _, run := range e.runs("") {
			fmt.Fprintf(&summary, "\n  id=%d job=%s attempt=%d status=%s exit=%s error=%q log=%s",
				run.ID, run.Job, run.Attempt, run.Status, exitCodeText(run), run.Error, run.LogPath)
		}
	}
	if log := readProcessOutput(e.schedulerLog()); log != "" {
		fmt.Fprintf(&summary, "\nscheduler log:\n%s", lastLines(log, 20))
	}
	return summary.String()
}

// exitCodeText renders the exit code of a run, or a dash when no process ran.
func exitCodeText(run store.Run) string {
	if run.ExitCode == nil {
		return "-"
	}
	return strconv.Itoa(*run.ExitCode)
}

// lastLines returns at most n trailing lines of text.
func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// historyRow is one row of "cronx history", as the command renders it.
type historyRow struct {
	id       string
	job      string
	attempt  string
	status   string
	started  string
	duration string
	exit     string
}

// parseHistoryRow reads the columns of a row of "cronx history".
func parseHistoryRow(t *testing.T, fields []string) historyRow {
	t.Helper()
	const columns = 6
	if len(fields) != columns+3 {
		t.Fatalf("history row = %v, want an identifier, a job, an attempt, a status, an instant, a duration and an exit status",
			fields)
	}
	return historyRow{
		id:       fields[0],
		job:      fields[1],
		attempt:  fields[2],
		status:   fields[3],
		started:  strings.Join(fields[4:7], " "),
		duration: fields[7],
		exit:     fields[8],
	}
}

// historyRows returns the rows of "cronx history" that describe a job.
func historyRows(t *testing.T, output, name string) []historyRow {
	t.Helper()
	var rows []historyRow
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == name {
			rows = append(rows, parseHistoryRow(t, fields))
		}
	}
	return rows
}

// historyRowForAttempt returns the history row of one attempt of a job.
func historyRowForAttempt(t *testing.T, output, name string, attempt int) historyRow {
	t.Helper()
	wanted := strconv.Itoa(attempt)
	for _, row := range historyRows(t, output, name) {
		if row.attempt == wanted {
			return row
		}
	}
	t.Fatalf("history of %q:\n%s\nwant a row for attempt %d", name, output, attempt)
	return historyRow{}
}

// permissionMode returns the permission bits of a path.
func permissionMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("looking at %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// physicalPath returns a path as the processes of the machine report it, so
// that a comparison holds also where a temporary directory is reached through a
// symbolic link.
func physicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving %s: %v", path, err)
	}
	return resolved
}
