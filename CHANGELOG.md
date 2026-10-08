# Changelog

All notable changes to cronx are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The rules for choosing a version are described in [docs/roadmap.md](docs/roadmap.md).

## [Unreleased]

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
