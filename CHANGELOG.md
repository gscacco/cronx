# Changelog

All notable changes to cronx are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The rules for choosing a version are described in [docs/roadmap.md](docs/roadmap.md).

## [Unreleased]

### Added

- One scheduler per state: `cronx run` takes a lease on the state, renews it
  while it runs and gives it back when it stops, so a second `cronx run` on the
  same state refuses to start instead of scheduling the same jobs again. A
  scheduler killed outright cannot give the lease back, and its lease expires
  after thirty seconds.
- `cronx status` reports whether a scheduler is driving the state, naming the
  holder and when its lease expires.
- `[jobs.<name>].grace_period` chooses how long a job is given to stop after
  `SIGTERM` before it is killed with `SIGKILL`: ten seconds unless the job asks
  for another duration. The runner already honoured the value; the configuration
  could not express it, although [security.md](docs/security.md) promised it per
  job.
- `[scheduler].timezone` is applied: activation times are computed on the clock
  of the configured zone, so a job runs at the wall clock time of that zone
  rather than of the machine, and `cronx list` prints the next run there. The
  zone database is embedded in the binary, so an IANA name resolves even where
  the system carries no `/usr/share/zoneinfo`.

### Fixed

- A `timezone` that names no zone is no longer ignored: it is reported, with the
  name, while the configuration is read, so `validate` and every other command
  refuse the file instead of scheduling jobs in an unexpected zone.
- The overlap policy of a job is applied to a trigger when it arrives, before
  the job waits for a free slot. With the default `max_parallel_jobs = 1`, a
  trigger that arrived while the job was still running waited for the slot and
  then ran the job, instead of being recorded as a `skipped` run as
  [scheduling.md](docs/scheduling.md) promises. A `queued` run now also waits
  for the run of its own job before it takes a slot.

## [0.2.0] - 2026-10-08

### Changed

- The output of every run now goes to one shared log, `~/.cronx/logs/runs.log`,
  instead of one file per run. Every line carries the time, the job, the run
  identifier and the pid of the process, so the output of concurrent jobs can be
  told apart. A reader of the old `logs/<job>/<id>.log` files must look in the
  new file.

### Added

- `[logging].path`, to choose where the run log is written.
- `[storage].path`, to choose where the status database is written.

### Fixed

- A run that is in flight when the scheduler is stopped is finished before the
  scheduler exits, instead of being left `running` until the next start.
- A job whose `working_directory` does not exist is now explained by naming the
  directory, instead of naming the command as if the program were missing.
- Two cronx processes that open a brand new state database at the same moment no
  longer race: the migration is applied once, and the second process finds it
  done instead of failing with `table schema_meta already exists`.

## [0.1.0] - 2026-10-04

### Added

- Cron expressions parsed in-house: five fields, ranges, lists, steps, month and
  day names, and the conventional OR semantics between day-of-month and
  day-of-week.
- A single TOML configuration file, with unknown keys rejected rather than
  ignored.
- The commands `run`, `validate`, `list`, `status`, `history`, `run-once` and
  `version`.
- A secure execution model: jobs are never run through a shell, `command` must
  be an absolute path, a job receives a minimal documented environment and no
  standard input.
- Timeouts that stop the whole process group with `SIGTERM`, then `SIGKILL`
  after a grace period.
- The `skip`, `allow` and `queue` overlap policies, retries, and a global
  `max_parallel_jobs` cap.
- SQLite persistence with versioned migrations; runs left in progress by a
  process that was killed are closed on the next start.
- A scheduler log and the captured output of every run under `~/.cronx/`.
