# Testing

cronx is verified at two levels. The packages under `internal/` hold unit tests,
which exercise the components directly and use an in-memory clock and database
where that helps. `test/integration` holds end-to-end tests, which drive the
compiled `cronx` binary as a real process: real configuration files, the real
scheduler, the real SQLite database and real log files on disk. Nothing is
mocked and nothing is linked into the test process.

## Running the tests

```sh
nix develop            # the pinned Go toolchain, Git and the SQLite CLI
make test              # the fast suite: seconds
make test-full         # every test, including the ones that wait for a minute boundary
make ci                # go vet, formatting, the full suite, a build
```

Individually:

```sh
go test ./...                  # every package
go test -short ./...           # the same without the minute waits, which is what make test runs
go test ./test/integration/ -v # only the end-to-end tests
go test ./test/integration/ -run TestTheHistorySurvivesARestart -v
```

The fast suite takes seconds and the full one a minute or two. Two end-to-end
tests have to wait for a real minute boundary, because a cron expression decides
*minutes*: the scheduled run that gets recorded, and the overlapping trigger that
gets skipped, which needs two boundaries in a row. They are marked
`t.Parallel()`, so they overlap with each other, and they skip themselves under
`go test -short`, which is what `make test` uses; `make test-full` — and the
check phase of the Nix package — runs them. Everything else is fast, because a
test that needs recorded runs produces them with `run-once`, or by killing a
process, instead of waiting for the clock. The slowest test left in the fast
suite watches the ten second grace period before a job that refuses SIGTERM is
killed, which is the only wait that cannot be shortened without changing what is
being tested.

## What the integration tests check

| Test file | What it verifies |
| --------- | ---------------- |
| `binary_test.go` | a configuration file is walked through `validate`, `list` and `run-once`, and the run reaches the file the job writes and the row SQLite records |
| `scheduling_test.go` | a scheduled job runs, is recorded and is reported by `history` and `status`; a trigger that arrives while the job is still running is skipped and no second execution happens |
| `execution_test.go` | the arguments reach the process verbatim (shell metacharacters included), the working directory is the configured one, a missing one is a `spawn_error` that names the directory, and the environment of a job is exactly the documented minimal one plus its own |
| `timeout_test.go` | a job that outlives its timeout is stopped, the process group it started is stopped with it, and a job that refuses SIGTERM is killed after the grace period |
| `retry_test.go` | a failing job is attempted once per configured retry, every attempt is recorded separately, and the attempts stop as soon as one succeeds |
| `persistence_test.go` | the history and the database survive a restart, and runs left in progress by a process that was killed are closed with an explanation |
| `logging_test.go` | both streams of a job are kept in the shared run log, every line carries a readable timestamp and identifies the run, and the log is readable only by its owner |
| `paths_test.go` | the run log and the status database are written where `[logging].path` and `[storage].path` say, and nowhere else |
| `timezone_test.go` | the next run is computed on the clock of `[scheduler].timezone`, not on the clock of the machine |
| `configuration_test.go` | a configuration that is wrong in one way is rejected by every command, with a useful explanation and without touching the state; the configuration is found through `--config`, `CRONX_CONFIG` or the home directory |

The race two processes can meet when they open a brand new database at the same
moment, and the outcome of a run that is in flight when the scheduler stops, are
covered by the package tests instead, because they need an interleaving or a
cancellation that no process can be asked for: `internal/store` applies a
migration a second time and opens one database from several goroutines at once,
and `internal/scheduler` cancels the context of a running job and checks that
its row is finished.

The nights a clock moves are checked the same way, in `internal/schedule`: the
activation after a daylight saving transition is asked for directly, because no
test can wait for one to arrive. What the two rules in
[scheduling.md](scheduling.md) promise is what the test reads back.

## How the tests are put together

```
test/integration/
  main_test.go       builds cronx and the job helper, then runs the tests
  harness_test.go    isolated homes, fixtures, running the binary, waiting
  *_test.go          the tests themselves
  testdata/*.toml    one configuration fixture per scenario
  testdata/job/      the helper program the jobs run
  testdata/invalid/  configurations that are wrong in exactly one way
```

* **The fixture is a file, not a string.** A test renders a fixture, replacing
  `@TEST_DIR@` with a directory of its own and `@JOB_HELPER@` with the compiled
  helper, and then drives the commands against the result. There is no other way
  to configure the test, so what the test exercises is a configuration file.
* **Isolation.** Every test gets its own home directory from `t.TempDir()`: its
  own `~/.cronx/config.toml`, its own state database, its own logs. The
  environment a cronx process is started with holds only `HOME`, `PATH` and
  `TZ=UTC`, so the state and the configuration of the person running the tests
  are never touched, and neither is their time zone. No fixed port, no fixed
  directory, no shared state between tests.
* **Waiting, not sleeping.** A test waits for the condition it is about with a
  bounded poll (`waitFor`, `waitForRun`, `waitForReport`), and fails with the
  recorded runs and the tail of the scheduler log when the budget runs out. The
  budgets are named constants in `harness_test.go`; the ones that wait for an
  activation are generous because of the minute granularity of cron.
* **Cleanup.** Every process a test starts is stopped when the test ends, even
  when an assertion failed: first with SIGTERM, then with SIGKILL. A job that
  outlived a scheduler which was killed on purpose is removed by the identifier
  the helper reported, so a failing test does not leave a sleeping process
  behind.
* **The tests build the binary they drive.** `TestMain` runs `go build` for
  `./cmd/cronx` and for the job helper, so the Go toolchain has to be in `PATH`
  where the tests run, with somewhere to keep its build cache. The development
  shell and the check phase of the Nix package both provide that; the package
  provides it together with vendored dependencies, which is why the tests can
  build offline there.

## The job helper

`test/integration/testdata/job` is a small program that exists only for these
tests. It is built by `TestMain` and used as the `command` of a job, which is
how a test observes what a process really received. It understands a list of
verbs, each taking a fixed number of arguments:

| Verb | What it does |
| ---- | ------------ |
| `report <path>` | writes what it sees, as JSON: its arguments, the payload after the first `--`, its working directory, its identifier, its process group and its environment |
| `out <text>`, `err <text>` | writes to standard output or standard error |
| `write <path> <content>`, `append <path> <content>` | writes or appends to a file |
| `sleep <duration>` | sleeps |
| `exit <code>` | exits with a status |
| `fail <path> <times> <code>` | fails until its own attempt counter says otherwise |
| `spawn <report> <duration>` | starts a child of itself, in the same process group, and then sleeps |
| `ignore-term <duration>` | refuses SIGTERM and then sleeps |

Everything after the first `--` is payload: `report` records it and nothing else
looks at it. That is how a test passes arguments a shell would have interpreted
without any shell being involved. The program lives under `testdata`, which the
Go toolchain ignores, so it is not part of `go build ./...`, `go vet ./...` or
the Nix package; the tests build it explicitly by path.

## What the suite found but does not fix

The suite checks cronx against its documentation. Where the two do not agree,
the disagreement is recorded here rather than papered over, and no test asserts
either side of it.

* **There is no singleton scheduler.** Two `cronx run` processes using the same
  configuration and the same state database both start, and both run the jobs
  they find due: the overlap policies are enforced inside one process, not
  across processes, and nothing takes a lock on the state. cronx does not
  document a singleton guarantee, so no test asserts one. Running the scheduler
  twice on the same state is therefore a way to duplicate executions, and a
  deliberate decision — a lock file, or a lease in the database — is needed
  before it can be prevented.
* **The grace period is not configurable per job.** [security.md](security.md)
  says the grace period between SIGTERM and SIGKILL "is configurable per job";
  the configuration has no such field and the runner always uses ten seconds.
  The suite checks the ten seconds, not the configurability.
* **There is no `cronx logs` command.** [cli.md](cli.md) does not list one, so
  the suite checks the run log directly: `~/.cronx/logs/runs.log`, whose lines
  carry the run identifier that `cronx history` reports.
* **A trigger can wait for a slot instead of meeting the overlap policy.**
  `max_parallel_jobs` bounds how many jobs run at the same time, and a trigger
  that arrives when every slot is taken waits for one. The overlap policy is
  only applied after that wait, so with the default `max_parallel_jobs = 1` a
  trigger that arrives while a long job is still running is **not** skipped: it
  waits for the slot and then runs the job. Measured with a job that sleeps for
  90 seconds on a `* * * * *` schedule and `overlap = "skip"`: the job ran three
  times back to back and the history holds three executions, none of them
  `skipped`, although [scheduling.md](scheduling.md) says a skipped trigger "is
  ignored and recorded as a `skipped` run". With a second slot the very same
  configuration skips the trigger as documented, which is what the overlap test
  uses. Which of the two rules should win for the single-slot case is a
  behavioural decision, so no test asserts either outcome.
