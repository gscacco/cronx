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
| `scheduling_test.go` | a scheduled job runs, is recorded and is reported by `history` and `status`; a trigger that arrives while the job is still running is skipped with the default of one job at a time, and no second execution happens |
| `singleton_test.go` | a second `cronx run` on the same state is refused and schedules nothing, the state is given back when the scheduler stops, and `status` reports the scheduler that holds it |
| `execution_test.go` | the arguments reach the process verbatim (shell metacharacters included), the working directory is the configured one, a missing one is a `spawn_error` that names the directory, and the environment of a job is exactly the documented minimal one plus its own |
| `timeout_test.go` | a job that outlives its timeout is stopped, the process group it started is stopped with it, and a job that refuses SIGTERM is killed after the grace period, which is the ten second default unless the job sets one |
| `retry_test.go` | a failing job is attempted once per configured retry, every attempt is recorded separately, and the attempts stop as soon as one succeeds |
| `persistence_test.go` | the history and the database survive a restart, and runs left in progress by a process that was killed are closed with an explanation |
| `logging_test.go` | both streams of a job are kept in the shared run log, every line carries a readable timestamp and identifies the run, and the log is readable only by its owner |
| `logs_test.go` | `cronx logs` prints what a run printed, only for the job that is named, nothing for a job that never ran, and nothing at all before the first run |
| `rotation_test.go` | a run log that passes its configured size is rotated, only the configured number of rotated files is kept, and no log is rotated when no size is configured |
| `retention_test.go` | the history is trimmed to the configured number of runs of each job when the scheduler starts, and a pruned history is what `history` reports |
| `reload_test.go` | a scheduler sent `SIGHUP` picks up a job the file added, refuses a file that cannot be used and keeps running, and applies the reload that follows the refusal |
| `paths_test.go` | the run log and the status database are written where `[logging].path` and `[storage].path` say, and nowhere else |
| `timezone_test.go` | the next run is computed on the clock of `[scheduler].timezone`, not on the clock of the machine |
| `configuration_test.go` | a configuration that is wrong in one way is rejected by every command, with a useful explanation and without touching the state; the configuration is found through `--config`, `CRONX_CONFIG` or the home directory |
| `init_test.go` | `cronx init` writes a configuration where the flag, the environment variable or the home directory says, with the permissions everything else gets, refuses to replace a file that exists without `--force`, and leaves the installation otherwise untouched |

The race two processes can meet when they open a brand new database at the same
moment, and the outcome of a run that is in flight when the scheduler stops, are
covered by the package tests instead, because they need an interleaving or a
cancellation that no process can be asked for: `internal/store` applies a
migration a second time and opens one database from several goroutines at once,
and `internal/scheduler` cancels the context of a running job and checks that
its row is finished.

The lease that keeps one scheduler per state is covered the same way, in
`internal/store`: a state is taken, refused to a second holder, taken over once
the lease of the first one has expired, renewed, and given back. Expiry is a
matter of arithmetic between two instants, which a test decides without waiting
half a minute for it to arrive.

The order between the overlap policy of a job and the wait for a free slot is
covered in `internal/scheduler`, which dispatches twice with a single slot and
a job that is still running: the second trigger has to be recorded as skipped
while the run it overlaps is going. Reaching the same order through `cronx run`
costs two real minute boundaries, which is what `scheduling_test.go` pays
instead, with the default of one job at a time.

The reading side of the run log is checked at both levels. `internal/logx`
reads back the lines the writer produced, so the two cannot drift apart, and it
rejects what is not a run log line. `internal/cli` drives `logs` against a log
the test wrote: the lines of one job and of every job, `--since` as a duration
and as an instant, a log that does not exist yet, a line that is not a run log
line, and `--follow`, stopped through the context of the command.

The rotation of the run log is covered in `internal/logx`, which writes lines
until the file must be renamed and then reads back which lines ended up where:
the line that did not fit, the files that moved one step back, the oldest one
that was dropped, a log written by a process before the one that rotates it, and
a single line larger than the limit, which is kept rather than rotated away.
`internal/config` checks how a size is written and that a configuration asking
for something impossible is rejected, and `test/integration` runs a job four
times against a limit that holds one line and checks that the last three runs
are the three files on disk.

Pruning is checked at both levels as well. `internal/store` seeds a history and
reads back which runs were deleted, per job, with a run in progress among them
and with a keep that asks for nothing. `internal/scheduler` starts a scheduler
over a history that is longer than the configuration keeps, and dispatches a
trigger to a job, to check that both moments trim it. `test/integration` records
five runs with `run-once`, starts the scheduler and stops it, and then asks
`cronx history` what is left.

A reload is checked at both levels too. `internal/scheduler` calls `Reload`
directly, because the rule it pins is what replaces what: the jobs of the new
configuration, the activations planned again, the settings that cannot be
applied and are reported instead, and a configuration that is refused whole when
a schedule in it cannot be read. `test/integration` sends the running scheduler a
real `SIGHUP` — after adding a job to the file, and after breaking it — and
reads the scheduler log to see what it did with it.

The nights a clock moves are checked the same way, in `internal/schedule`: the
activation after a daylight saving transition is asked for directly, because no
test can wait for one to arrive. What the two rules in
[scheduling.md](scheduling.md) promise is what the test reads back.

Catch-up and a job that is disabled are rules about what the scheduler does over
time, so `internal/scheduler` owns them and seeds the history they need: it
records a run in the past and starts a scheduler over it, to check that a job
that asks is run once however many activations it missed, that a job that does
not ask is not run, that what was caught up is not caught up again at the next
start, that an empty history, an `@reboot` job and a reload catch nothing up, and
that a disabled job has no activation, no run and no catch-up until a reload
enables it. `internal/cli` lists the reference configuration that disables a job,
so that the `disabled` the `NEXT` column prints is the one a person reads.

The file `cronx init` writes is checked at both levels, because what it holds is
documentation that has to stay true. `internal/cli` reads the written file back
and holds it to three promises: it is a valid configuration both as it stands and
with every commented option turned on, it does not repeat an option the file
already sets, and every option the reference configuration uses is documented in
it. `test/integration` runs the command against a fresh installation, through
`--config` and `CRONX_CONFIG`, and checks that it writes nothing but that file.

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

Nothing is outstanding today. The disagreements this section used to carry have
all been closed. A trigger that waited for a free slot instead of meeting the
overlap policy was resolved in 0.3.0: the policy is applied to a trigger when it
arrives (D35), and both `test/integration` and `internal/scheduler` now assert
the outcome with a single slot. The absence of a `cronx logs` command was closed
on `master` (D36): [cli.md](cli.md) lists the command and `logs_test.go` drives
it, so reading the run log is checked through the command line rather than by
opening the file directly.
