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

- Five fields: `minute hour day-of-month month day-of-week`.
- Operators: `*`, ranges `a-b`, lists `a,b`, steps `*/n` and `a-b/n`.
- Three-letter month and day names are accepted (case-insensitive).
- Day-of-month and day-of-week use the conventional OR semantics when both are
  restricted.
- Not supported initially: `@`-descriptors, a seconds field, `L`/`W`/`#`.
- The timezone comes from `[scheduler].timezone` (default: local). During DST
  transitions a non-existent time is skipped and an ambiguous time fires once.
- Runs missed while the scheduler is not running are skipped (no catch-up).

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

SQLite stores execution history (`runs`) and runtime state (`job_state`), and a
`schema_meta` table tracks the schema version. Timestamps are stored as UTC in
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
| D12 | State and logs: `~/.cronx/state.db` and `~/.cronx/logs/` |
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
| D26 | Schema: `schema_meta`, `runs`, `job_state` |
| D27 | Timestamps: UTC RFC 3339 |
| D28 | Run status: `scheduled`, `running`, `succeeded`, `failed`, `timed_out`, `spawn_error`, `skipped` |
| D29 | The output of every run is written to one shared log; each line carries an RFC 3339 timestamp, the job, the run and the pid |
| D30 | Scheduler log at `~/.cronx/logs/cronx.log` |
| D31 | Commands: `run`, `validate`, `list`, `history`, `run-once`, `status`, `version` |
| D32 | The run log and the status database can be relocated from the configuration: `[logging].path` and `[storage].path` (both optional) |
