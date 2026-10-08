# Reference configurations

The [`examples/configs/`](../examples/configs/) directory holds complete
configuration files to copy and adapt. They are not documentation that can go
stale: the test suite reads the same files, so an example that stops being valid
— or that stops describing the jobs it claims to describe — fails the build.

| File | What it is for |
| ---- | -------------- |
| [`minimal.toml`](../examples/configs/minimal.toml) | The smallest configuration cronx accepts: one job, the two required fields. |
| [`full.toml`](../examples/configs/full.toml) | Every field of a job, plus the global scheduler and logging settings, all three overlap policies and a schedule that can never match. |
| [`maintenance.toml`](../examples/configs/maintenance.toml) | A realistic set-up for a single server: backup, log rotation, vacuum, report. |
| [`broken.toml`](../examples/configs/broken.toml) | Deliberately invalid: the reference for what the diagnostics look like. |

The commands of the valid examples are placeholders pointing at the usual
locations (`/usr/local/bin/...`). cronx never runs a job through a shell and
never looks a command up in `PATH`, so replace them with programs that exist on
your machine before running anything.

## Trying an example

A configuration file is only ever read: cronx never rewrites it, and the runtime
state and the logs live under the home directory (`~/.cronx/`) unless the
configuration moves them; see [configuration.md](configuration.md).

```console
$ cronx validate --config examples/configs/minimal.toml
examples/configs/minimal.toml is valid

$ cronx list --config examples/configs/full.toml
JOB         SCHEDULE     NEXT
backup      0 3 * * *    2026-10-05 03:00:00 CEST
leap-check  0 0 31 4 *   -
metrics     */5 * * * *  2026-10-04 21:40:00 CEST
reindex     30 2 * * 6   2026-10-10 02:30:00 CEST
```

To run a job for real, copy the file, point its `command` at a program that
exists, and then:

```console
$ cronx run-once hello --config ./minimal.toml
job "hello" finished with status "succeeded"
```

## minimal.toml

```toml
[jobs.hello]
schedule = "*/5 * * * *"            # every five minutes
command = "/usr/local/bin/hello"
```

A job name (`hello`, here) and the two required fields are all cronx needs; the
timezone, the parallel limit and the logging level fall back to their defaults.
This is the file to start from: its single job runs every five minutes and can
be triggered at once with `run-once`, whatever the schedule says.

## full.toml

It opens with the two global tables:

```toml
[scheduler]
timezone = "Europe/Rome"            # an IANA name, or "Local" for the machine zone
max_parallel_jobs = 2               # at most two jobs running at the same time

[logging]
level = "debug"                     # debug | info | warn | error
```

The optional `[logging] path` and `[storage] path`, which move the run log and
the status database, are described in [configuration.md](configuration.md); they
are left out of the examples so that the files read exactly as they are.

and then uses every per-job field at least once:

| Job | Demonstrates |
| --- | ------------ |
| `backup` | `args`, `timeout`, `grace_period`, `retry`, `overlap = "skip"`, `working_directory`, `env` |
| `metrics` | `overlap = "allow"`: every trigger runs, even while the previous run is still going |
| `reindex` | `overlap = "queue"`: a trigger arriving while a run is in progress waits for it |
| `leap-check` | a valid expression that can never match, because 31 April does not exist |

`leap-check` is kept on purpose. An expression cronx cannot parse is rejected by
`validate`, while one that simply never matches is accepted: `cronx list` shows a
dash in the `NEXT` column and the scheduler never triggers the job.

## maintenance.toml

A set-up that could be installed on a server as it is, once the commands exist:

| Job | Schedule | Why |
| --- | -------- | --- |
| `backup` | `0 3 * * *` | Nightly, at 03:00. Long, so it has a `timeout` of two hours; it must not overlap with itself, so its policy is `skip`; it is retried once. |
| `logrotate` | `5 * * * *` | Hourly, five minutes past the hour. Quick, with a short `timeout` and `skip`. |
| `vacuum` | `0 4 * * 0` | Weekly, on Sunday at 04:00, after the backup. It passes `PGDATABASE` through `env`. |
| `report` | `0 7 * * 1-5` | On weekday mornings. `queue` keeps the reports in order instead of dropping one. |

The two overlapping policies are the ones a scheduler is usually asked for:
`skip` for a job that would only fight with itself, `queue` for a job where every
trigger must be honoured.

## broken.toml

Every mistake in the file is marked by the comment stating what cronx says about
it, and one run reports all of them at once:

```console
$ cronx validate --config examples/configs/broken.toml
cronx: invalid configuration: scheduler.max_parallel_jobs must be at least 1, got 0
logging.level "verbose" is not one of debug, error, info, warn
job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, got 4
job "backup": command "backup" must be an absolute path
job "backup": retry must not be negative, got -1
job "cleanup": schedule is not valid: cron expression "@daily" must have 5 fields, got 1
job "cleanup": overlap "sometimes" is not one of skip, allow, queue
job "cleanup": timeout "30 minutes" is not a valid duration
job "report": schedule is required
```

The rules behind each line are listed in [configuration.md](configuration.md).

## The configuration files of the test suite

`examples/configs/` is not the only place the tests read configuration from:

| File | How it is used |
| ---- | -------------- |
| `examples/configs/*.toml` | `internal/cli/integration_test.go` validates and lists every one of them, and checks that `broken.toml` reports exactly the problems shown above. Adding a file to the directory without teaching the tests about it fails the suite. |
| `internal/cli/testdata/workflow.toml` | `internal/cli/workflow_test.go`: a configuration that is really executed. Its `command` values are the test binary re-entered in a helper mode, and the placeholders `@TEST_EXECUTABLE@` and `@TEST_WORKDIR@` are replaced by paths the test controls before the file is used. |

Keeping the executed configuration in a file rather than in Go code means the
policies a job is described by — its timeout, its retries, its environment, its
working directory — can be read in a single place, while the tests only assert
what the run left behind: the exit code, the status in the history and the log
file of the run.
