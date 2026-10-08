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

## Planned

### 0.2.0 — one run log, and paths you can move

The output of every run goes to a single file, and the two files cronx writes
can be moved out of `~/.cronx/`. Both features are already merged and wait for
this release; the three fixes ride along because they touch the same state and
log code. The change to the log layout is why this is a `MINOR` release and not
a patch: a reader of the old `logs/<job>/<id>.log` files has to look somewhere
else.

- [x] One shared, timestamped run log: every run appends to one file and every
  line carries the time, the job, the run identifier and the pid (D29).
- [x] `[logging].path` and `[storage].path`, so the run log and the status
  database can live anywhere (D32).
- [x] A [`CHANGELOG.md`](../CHANGELOG.md) and the decision register reconciled
  with D29 and D32.
- [x] Close the race that can make two processes migrating a brand new database
  fail with `table schema_meta already exists` ([testing.md](testing.md)).
- [x] Name the missing working directory, instead of reporting the command
  ([testing.md](testing.md)).
- [x] Finish the run that is in flight when the scheduler stops, instead of
  leaving it `running` until the next start ([testing.md](testing.md)).

### 0.3.0 — scheduling you can rely on

Every item here changes behaviour a user can see — a job starts at a different
moment, or a second scheduler refuses to start — so the release is a `MINOR`.

- [ ] Apply `[scheduler].timezone`: the key is parsed today but the scheduler
  reads the machine clock and ignores it ([testing.md](testing.md)).
- [ ] Validate `timezone` in `cronx validate`, rather than only when the
  scheduler starts ([configuration.md](configuration.md)).
- [ ] Allow one scheduler per state: a second `cronx run` against the same
  database should refuse to start instead of duplicating executions
  ([testing.md](testing.md)).
- [ ] Make the overlap policy win over a free slot, so a trigger that arrives
  while a job is still running is skipped as documented even when
  `max_parallel_jobs = 1` ([testing.md](testing.md)).
- [ ] Make the grace period between `SIGTERM` and `SIGKILL` configurable per
  job, which [security.md](security.md) already promises but the configuration
  does not offer.

### 0.4.0 — operability: logs and data

- [ ] `cronx logs [job] [--follow] [--since]`, so the run log can be read
  without knowing its path ([testing.md](testing.md)).
- [ ] Rotation and retention for the logs, and pruning for the history, since no
  log is rotated today and the database grows without bound ([cli.md](cli.md)).
- [ ] Reload the configuration on `SIGHUP`, so a change does not need a restart.

### 0.5.0 — cron expressiveness

- [ ] `@`-descriptors: `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`,
  `@midnight` and `@reboot` (D19).
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
| 0.5.0 | `@`-descriptors, a seconds field, `L`/`W`/`#`, catch-up, `enabled = false`. |
| 0.6.0 | `--json`, `CRONX_LOG`/`CRONX_STATE`, on-failure actions, `cronx doctor`. |
| 0.7.0 | `cronx service install` for `systemd` and `launchd`. |
| 1.0.0 | The frozen schema and command line, and the compatibility promise. |

