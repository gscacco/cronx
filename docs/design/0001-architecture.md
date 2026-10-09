# 0001 - Architecture and foundational decisions

- Status: Accepted
- Date: 2026-10-04

## Context

cronx is a small local job scheduler inspired by Unix `cron`. It is not a
CI/CD system, a workflow orchestration platform, a distributed scheduler, a
cloud service, a container orchestrator, a web application, an Airflow
replacement or a Kubernetes component.

The guiding principle is:

> Keep the system small while making the core behaviour reliable.

The project is developed incrementally with Test-Driven Development. Decisions
that affect users, security, dependencies or the architecture are recorded here
and approved before implementation.

## Separation of concerns

| Concern           | Store      |
| ----------------- | ---------- |
| Desired config    | TOML       |
| Runtime state     | SQLite     |
| Execution history | SQLite     |
| Logs              | filesystem |

TOML is the source of truth for the desired scheduler configuration. SQLite is
never used as a configuration store.

## Dependencies

Every dependency, and its reason, is listed here.

| Dependency           | Purpose                | Rationale |
| -------------------- | ---------------------- | --------- |
| `spf13/cobra`        | command-line interface | Approved. Standard, small CLI framework for Go. |
| `BurntSushi/toml`    | TOML parsing           | Approved. Struct decoding plus strict unknown-key detection. |
| `modernc.org/sqlite` | SQLite driver          | Approved. Pure Go, no cgo, portable and cross-compilable. |
| (cron expressions)   | scheduling             | Implemented in-house on the standard library only. |

The scheduler, the cron expression parser and the process execution layer rely
on the Go standard library only. Tests use the standard `testing` package only.

## Package layout

```
cmd/cronx/          command-line entry point
internal/config/    TOML loading and validation
internal/job/       domain model (job and execution policy)
internal/schedule/  cron expression parsing and next-time computation
internal/clock/     injectable time source (deterministic tests)
internal/runner/    secure process execution (never a shell)
internal/store/     SQLite schema, migrations, history, runtime state
internal/logx/      logging setup: the scheduler log and the shared run log
internal/scheduler/ orchestration loop and execution policies
internal/cli/       command-line commands
```

Everything lives under `internal/`: cronx exposes no public Go API.

## Scheduling semantics

- Five fields: `minute hour day-of-month month day-of-week`, or six, with the
  optional seconds field in front of them (D43), or a descriptor: `@yearly`,
  `@monthly`, `@weekly`, `@daily`, `@midnight` and `@hourly` stand for a fixed
  time (D41), and `@reboot` runs a job once, when the scheduler starts (D42).
- Operators: `*`, ranges `a-b`, lists `a,b`, steps `*/n` and `a-b/n`.
- Three-letter month and day names are accepted (case-insensitive).
- Day-of-month and day-of-week use the conventional OR semantics when both are
  restricted.
- Not supported: `L`/`W`/`#`.
- The timezone comes from `[scheduler].timezone` (default: local). During DST
  transitions a non-existent time is skipped and an ambiguous time fires once.
- Runs missed while the scheduler is not running are skipped (no catch-up).
- The overlap policy of a job is applied to a trigger when it arrives, before the
  job waits for a free slot (D35): the policy holds whatever `max_parallel_jobs`
  is.

## Secure process execution

cronx must never run a job through a shell. The executable and its arguments are
always kept as separate values and passed directly to the process API
(`os/exec`):

```text
command = executable path (absolute)
args    = array of strings
```

Shell metacharacters supplied as arguments remain literal arguments. Explicit
security tests guard this behaviour.

## Persistence

SQLite stores execution history (`runs`) and runtime state (`job_state`), a
`schema_meta` table tracks the schema version, and a `scheduler_lease` row is the
lease that keeps one scheduler per state (D34). Timestamps are stored as UTC in
RFC 3339 format. Job output is written to a single log file on the filesystem and
referenced from the history.

## Approved decision register

| # | Decision |
| - | -------- |
| D1 | CLI framework: `spf13/cobra` |
| D2 | TOML library: `BurntSushi/toml` |
| D3 | SQLite driver: `modernc.org/sqlite` (pure Go) |
| D4 | Cron engine: implemented in-house on the standard library |
| D5 | Tests: standard `testing` package only |
| D6 | Module path: `gscacco.com/cronx` |
| D7 | Go version: pinned toolchain in the flake; `go.mod` aligned to it (1.26, because nixpkgs removed the end-of-life 1.25 toolchain) |
| D8 | License: MIT |
| D9 | Continuous integration: a host-agnostic `make ci`, run through the flake |
| D10 | Commit messages: plain descriptive messages |
| D11 | Configuration file: `~/.cronx/config.toml`, overridable via `--config` or `CRONX_CONFIG` |
| D12 | State and logs default to `~/.cronx/state.db` and `~/.cronx/logs/` (movable, see D32) |
| D13 | `command` must be an absolute path; cronx performs no PATH lookup |
| D14 | `args` is an array of strings passed verbatim |
| D15 | `working_directory` is optional |
| D16 | The parent environment is not inherited; a documented minimal environment plus per-job overrides is used |
| D17 | Cron syntax and operators as described above |
| D18 | Day-of-month and day-of-week use OR semantics |
| D19 | `@`-descriptors and a seconds field are out of scope initially |
| D20 | Timezone from configuration; DST handled as described above |
| D21 | Missed runs are skipped |
| D22 | Timeout: `SIGTERM`, then `SIGKILL` after a grace period, applied to the process group |
| D23 | `retry = N` means N additional attempts after the first |
| D24 | `overlap`: `skip` (default), `allow`, `queue` |
| D25 | A global `max_parallel_jobs` cap |
| D26 | Schema: `schema_meta`, `runs`, `job_state`, `scheduler_lease` |
| D27 | Timestamps: UTC RFC 3339 |
| D28 | Run status: `scheduled`, `running`, `succeeded`, `failed`, `timed_out`, `spawn_error`, `skipped` |
| D29 | The output of every run is written to one shared log; each line carries an RFC 3339 timestamp, the job, the run and the pid |
| D30 | Scheduler log at `~/.cronx/logs/cronx.log` |
| D31 | Commands: `run`, `validate`, `list`, `history`, `run-once`, `status`, `version` |
| D32 | The run log and the status database can be relocated from the configuration: `[logging].path` and `[storage].path` (both optional) |
| D33 | Versions follow Semantic Versioning: before 1.0 a new key, a new command or a visible behaviour change is a `MINOR` release and a fix on its own a `PATCH`; the schema and the command line freeze at 1.0.0 |
| D34 | One scheduler per state: a lease row in the database, taken before the first activation, renewed every ten seconds and given back when the scheduler stops, so a second `cronx run` is refused; a lease of a scheduler killed outright expires after thirty seconds |
| D35 | The overlap policy is applied to a trigger when it arrives and before the job waits for a free slot, so `skip` holds whatever `max_parallel_jobs` is, and a `queue`d run waits for the run of its own job before taking a slot |
| D36 | `cronx logs [job] [--follow] [--since]` joins the commands of D31: it reads the run log at `[logging].path` — the format of D29, read back by splitting each line into the instant, the job, the run, the pid and the output — prints the lines of one job or of every job, drops with `--since` what is older than a duration counted back from now or than an RFC 3339 instant, keeps printing with `--follow` as the log grows, opens no state, writes nothing and prints nothing when no line matches |
| D37 | The run log is rotated by the scheduler itself when a line would take it past `[logging].max_size`: the file is renamed to `<path>.1`, the files before it move one step back, at most `[logging].max_backups` of them are kept (three unless the configuration says otherwise) and the oldest is dropped; a log is never rotated before its first line, so a single line larger than the limit is written rather than dropped, and no rotation happens at all unless `max_size` is configured. Only the run log is rotated: the scheduler log holds one line per event of the scheduler, not whatever a job prints |
| D38 | `[storage].max_runs` bounds the history: the scheduler deletes the runs of a job beyond the newest N, at start for every job and at each dispatch for the jobs that are due, so that a scheduler left running for months does not grow the database without bound either. Runs still in progress are never deleted, whatever their age, and a keep of zero or less deletes nothing, so a caller given no limit cannot empty a history by mistake; without `max_runs` the history keeps every run |
| D39 | A running scheduler reloads its configuration on `SIGHUP`: the file is read and validated, and the jobs it holds replace the ones being run, with the activations planned again from that moment — a job added, removed or rescheduled takes effect without a restart. A file that cannot be read or used is refused whole and reported, and the running configuration is kept. A change to a setting of the scheduler itself — the timezone, the parallel limit, the log path and its rotation, the state path and its retention — is reported and ignored, because what reads it was built when the scheduler started: it needs a restart |
| D40 | `cronx init [--force]` writes a configuration file at the path the flag, the environment variable or the home directory selects: the smallest configuration cronx accepts — one job, with its schedule, its command and the arguments of the command — with every other option written next to it commented out, with what it does and what happens without it, so that the file is also its own reference. The directory it writes into is created with `0700` and the file with `0600`; a file that already exists is never replaced without `--force`, and the command writes nothing else: no state is opened and no log is created. The tests hold the file to being a valid configuration both as it stands and with every commented option turned on, and to documenting every option the reference configuration uses |
| D41 | A descriptor that stands for a fixed time — `@yearly`, `@monthly`, `@weekly`, `@daily`, `@midnight` and `@hourly` — is accepted wherever a five-field expression is, and means what traditional cron means by it: `0 0 1 1 *`, `0 0 1 * *`, `0 0 * * 0`, `0 0 * * *`, the same `0 0 * * *` and `0 * * * *`. A descriptor is a whole expression and not a field of one, so what is not a case-insensitive name of one — a misspelled one, an unknown one, and one followed by fields — is refused, with the list of the accepted ones in the message for anything that is not one of them. A job keeps the descriptor it was written with, so `cronx list` shows it as its author wrote it, and the tests hold the six to producing the same activations as the expressions they stand for (D19, D17) |
| D42 | `@reboot` is the descriptor that is not a time: it stands for no expression, so a job that asks for it has no activation on the clock — `Next` reports none and `cronx list` says `at startup` where a schedule that can never match gets a dash — and it is run once, when `cronx run` starts, from the configuration read at that moment. The start is the trigger: it is judged by the overlap policy of the job and counts against `max_parallel_jobs` like every other trigger, and runs left in progress by a previous process are closed first, so the policy sees the state as the scheduler found it. A scheduler that is already running is not a start, so a reload that turns a job into one of these does not run it: the scheduler log says so and the job waits for the next start. The tests hold the trigger to firing exactly once on a start, in the scheduler and through the real binary |
| D43 | An expression may state the seconds as well, as a sixth field in front of the five a traditional one has: the count of the fields is what says which form was written, and the seconds field accepts what the minutes field accepts — `*`, values from 0 to 59, ranges, lists and the steps `*/n` and `a-b/n`, and no names, because there are none for a second — while a five-field expression is exactly the expression it always was, whose activations are on the minute because the seconds it leaves unsaid are zero. The seconds field is the only optional one, so what is neither five fields nor six is refused with the count named in the message. An activation is computed to the second: the search starts at the following second for an expression that states the seconds and at the start of the following minute for one that does not, a step to another minute puts the seconds back at zero so that the field applies to that minute from its beginning, and a step inside the minute walks a second at a time, worked out from the instant rather than from the wall clock it is in, because a wall clock that daylight saving repeats names two instants: the walk then crosses the repeated hour once, taking from its second pass the activations of the part of it the walk has not reached yet, so that every wall time of that hour is an activation exactly once (D20). Everything that follows from an activation is unchanged: the overlap policy judges each trigger as it arrives and `max_parallel_jobs` bounds what runs, so a job that fires several times a minute is judged several times a minute (D35); a job that is due is still planned for its next activation rather than replayed, so a seconds field catches up no more than any other schedule (D21); and `cronx list` and the history show the seconds of an instant because they always have. The tests hold a six-field expression to parsing and to its activations, hold a seconds field of zero to producing the same activations as the five-field expression it doubles, hold the scheduler to planning and dispatching an activation that is between two minute boundaries, hold an expression that states the seconds to firing an hour daylight saving repeats once, and hold the real binary to producing two runs of the same job seconds apart (D17, D19) |

