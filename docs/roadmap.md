# Roadmap

cronx is built incrementally, and every step is published as a version. This
document records what is already out, what comes next and why each item sits in
the release it does. It is a plan, not a schedule: the order inside a release can
change, and an item moves only once it is implemented and tested.

The items marked `[x]` are already on `master` and wait only for their release.

## How a version is chosen

cronx follows [Semantic Versioning](https://semver.org/) while it is still
before 1.0:

- the number is `0.MINOR.PATCH`;
- a `PATCH` release contains only fixes that leave the documented behaviour
  unchanged;
- a `MINOR` release contains a new configuration key, a new command, or a change
  to behaviour a user can notice — and, where it is useful, the fixes that
  belong with it;
- `1.0.0` is the point at which the configuration schema and the command line
  are frozen and backward compatibility is promised. Until then, a change that
  would break an existing configuration file is introduced only in a `MINOR`
  release and is called out in the [changelog](../CHANGELOG.md).

A fix that has to go out on its own becomes a `PATCH` release. None of the fixes
below needs that today: each one is grouped with the work it touches.

## Released

### 0.1.0 — 2026-10-04

The first complete version: a five-field cron scheduler that runs commands
directly, records what happened and keeps their output.

- Cron expressions parsed in-house: five fields, ranges, lists, steps, month and
  day names, and the conventional OR semantics between day-of-month and
  day-of-week.
- A single TOML configuration, with unknown keys rejected rather than ignored.
- Seven commands: `run`, `validate`, `list`, `status`, `history`, `run-once`,
  `version`.
- A secure execution model: never a shell, an absolute command path, a minimal
  documented environment and no standard input.
- Timeouts that stop the whole process group, with `SIGTERM` then `SIGKILL`
  after a grace period; retries; the `skip`, `allow` and `queue` overlap
  policies; and a global `max_parallel_jobs` cap.
- SQLite persistence with versioned migrations, and runs left in progress by a
  process that was killed closed on the next start.
- A scheduler log and the captured output of every run under `~/.cronx/`.

### 0.2.0 — 2026-10-08

One shared run log and the two paths that can be moved out of `~/.cronx/`, with
the three fixes that touch the same state and log code. The change to the log
layout is why this is a `MINOR` release and not a patch: a reader of the old
`logs/<job>/<id>.log` files has to look somewhere else.

- One shared, timestamped run log: every run appends to one file and every line
  carries the time, the job, the run identifier and the pid (D29).
- `[logging].path` and `[storage].path`, so the run log and the status database
  can live anywhere (D32).
- A [`CHANGELOG.md`](../CHANGELOG.md) and the decision register reconciled with
  D29 and D32.
- The race that could make two processes migrating a brand new database fail with
  `table schema_meta already exists` is closed: a migration is applied once.
- A job whose working directory does not exist is explained by naming the
  directory, instead of the command.
- The run in flight when the scheduler stops is finished before it exits, instead
  of being left `running` until the next start.

### 0.3.0 — 2026-10-09

Scheduling you can rely on: a job starts on the clock you configured, a second
scheduler cannot duplicate it, an overlap decision is made when the trigger
arrives, and a job chooses how long it is given to stop. Every item changes
behaviour a user can see — a job starts at a different moment, or a second
scheduler refuses to start — so the release is a `MINOR`.

- `[scheduler].timezone` is applied: activations are computed on the clock of the
  configured zone, and `cronx list` prints the next run there.
- `timezone` is validated when the configuration is read, rather than when the
  scheduler starts, so no command schedules in an unexpected zone.
- One scheduler per state: a second `cronx run` against the same database refuses
  to start instead of duplicating executions, with a lease the scheduler takes,
  renews while it runs and gives back when it stops (D34).
- The overlap policy wins over a free slot, so a trigger that arrives while a job
  is still running is skipped as documented even when `max_parallel_jobs = 1`
  (D35).
- The grace period between `SIGTERM` and `SIGKILL` is configurable per job:
  `grace_period`.

### 0.4.0 — 2026-10-09

Operability: what cronx writes can now be read back and bounded, and a change to
the configuration no longer needs a restart. The release adds a command and three
configuration keys, so it is a `MINOR`.

- `cronx logs [job] [--follow] [--since]` reads the run log, so the output of a
  run can be reached without knowing where the file is (D36).
- The run log is rotated once it would pass `[logging].max_size`, keeping
  `[logging].max_backups` of the rotated files (D37).
- The history is pruned to the newest `[storage].max_runs` runs of each job,
  when the scheduler starts and whenever a job runs (D38).
- A running scheduler reloads its configuration on `SIGHUP`: the jobs of the
  file take effect at once, while a change to a setting of the scheduler itself
  is reported and needs a restart (D39).

## Planned

### 0.5.0 — cron expressiveness and a configuration to start from

- [x] `cronx init`, to write a configuration file that documents itself: the
  fields one job needs are set, and every other option is commented out with
  what it does and what happens without it (D40).
- [x] The `@`-descriptors that stand for a fixed time: `@yearly`, `@monthly`,
  `@weekly`, `@daily`, `@midnight` and `@hourly` (D41).
- [x] `@reboot`, to run a job once when the scheduler starts (D42).
- [ ] An optional seconds field, for a six-field expression (D19).
- [ ] The `L`, `W` and `#` day operators (D19).
- [ ] Optional catch-up of runs missed while the scheduler was stopped, which
  today are skipped (D21).
- [ ] `enabled = false` on a job, to keep a definition without scheduling it.

### 0.6.0 — scripting and integration

- [ ] `--json` on `list`, `status` and `history`, and `run-once --wait`, for
  scripts and dashboards.
- [ ] `CRONX_LOG` and `CRONX_STATE`, so the two paths can be overridden without
  editing the configuration, as `CRONX_CONFIG` already allows for the file
  itself.
- [ ] An on-failure action per job (a command, a webhook or an e-mail), so a
  failed run can reach someone.
- [ ] `cronx doctor`, to check the configuration, the state and the paths in one
  step.

### 0.7.0 — running as a service

- [ ] `cronx service install` and `cronx service uninstall`, writing the unit
  that keeps the scheduler running under `systemd` or `launchd`, so the
  machine-specific boilerplate in the README is generated rather than copied.

### 1.0.0 — stable

- [ ] Freeze the configuration schema and the command line, and promise backward
  compatibility for both.
- [ ] Complete the documentation and confirm the full test suite on every
  supported platform.

## Release checklist

Every release, whatever its number:

1. the corresponding entry in [`CHANGELOG.md`](../CHANGELOG.md) is written;
2. the version in `internal/cli/version.go` is bumped;
3. `make ci` is green (vet, formatting, the full suite, a build);
4. `nix build .#default` succeeds;
5. an annotated `vX.Y.Z` tag is created and pushed.

## Summary

| Release | Contains |
| ------- | -------- |
| 0.1.0 | The first complete scheduler and its seven commands. |
| 0.2.0 | One shared run log, `[logging].path` and `[storage].path`, and three small state fixes. |
| 0.3.0 | The configured timezone, one scheduler per state, the overlap decision, a configurable grace period. |
| 0.4.0 | `cronx logs`, log rotation and history retention, reload on `SIGHUP`. |
| 0.5.0 | `cronx init`, `@`-descriptors, a seconds field, `L`/`W`/`#`, catch-up, `enabled = false`. |
| 0.6.0 | `--json`, `CRONX_LOG`/`CRONX_STATE`, on-failure actions, `cronx doctor`. |
| 0.7.0 | `cronx service install` for `systemd` and `launchd`. |
| 1.0.0 | The frozen schema and command line, and the compatibility promise. |

