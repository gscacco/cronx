# Changelog

All notable changes to cronx are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The rules for choosing a version are described in [docs/roadmap.md](docs/roadmap.md).

## [Unreleased]

### Added

- `cronx init`, to write a configuration file to start from: one job, with the
  fields it needs, and every other option cronx knows written next to it
  commented out, with what it does and what happens without it, so that the file
  is also its own reference. The file is written where `--config`,
  `CRONX_CONFIG` or the home directory points, its directory is created with
  `0700` and the file with `0600`. A configuration that is already there is never
  replaced — the command stops and names the file — unless `--force` is given,
  and nothing else is touched: no state is opened and no log is created (D40).

## [0.4.0] - 2026-10-09

### Added

- `cronx logs [job]`, to read the run log from the command line: with a job name
  only the lines of that job, without one the lines of every job. `--since`
  leaves out what was written before a duration counted back from now (`30m`) or
  before an RFC 3339 instant, and `--follow` keeps printing as the log grows. The
  command reads the log and nothing else: it opens no state, writes nothing and
  creates nothing (D36).
- The run log is rotated once it would pass `[logging].max_size`: the file is
  renamed to `runs.log.1`, the files before it move one step back, and at most
  `[logging].max_backups` of them are kept, three unless the configuration says
  otherwise. A size is written as a number of bytes or with a `KB`, `MB` or `GB`
  suffix. Without a size nothing is rotated and the log grows as before, and no
  log is rotated before its first line, so a line larger than the limit is kept
  rather than dropped (D37).
- `[storage].max_runs` bounds the history: the scheduler keeps the newest N runs
  of each job and deletes the rest, when it starts and whenever a trigger arrives
  for a job, so that a scheduler left running for months does not grow the
  database without bound either. A run still in progress is never deleted, and
  without the key every run is kept (D38).
- A running scheduler reloads its configuration when it is sent `SIGHUP`: the
  jobs of the file that is read replace the ones being run, so a job that was
  added, removed or rescheduled takes effect without a restart. A file that
  cannot be read or used is refused and reported, and the running configuration
  is kept. A change to `[scheduler]`, `[logging] path` and its rotation, or
  `[storage] path` and `max_runs` is reported and ignored, because what reads it
  was built when the scheduler started: it needs a restart (D39).

## [0.3.0] - 2026-10-09

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
