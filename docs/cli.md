# Command line

```
cronx <command> [flags]
```

Every command accepts the global flag `--config`, which selects the
configuration file. When it is not given, `CRONX_CONFIG` is used, and failing
that the default `~/.cronx/config.toml`. The runtime state and the logs always
live under `~/.cronx/`.

| Command | Purpose |
| ------- | ------- |
| `validate` | Check the configuration file and report every problem it finds. |
| `list` | Show the configured jobs, their schedule and when each runs next. |
| `status` | Show the last outcome of every job and whether a run is in progress. |
| `history [job]` | Show the execution history, newest first. |
| `run-once <job>` | Run a job immediately, whatever its schedule. |
| `run` | Run the scheduler in the foreground until it is stopped. |

Every command below can be tried against the configuration files kept in
[`examples/configs/`](../examples/configs/), which are described one by one in
[examples.md](examples.md).

The exit status is `0` on success and `1` on failure. `run-once` fails when the
job did not succeed, so it can be used in scripts.

## validate

```console
$ cronx validate
/home/user/.cronx/config.toml is valid

$ cronx validate --config examples/configs/broken.toml
cronx: invalid configuration: scheduler.max_parallel_jobs must be at least 1, got 0
logging.level "verbose" is not one of debug, error, info, warn
job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, got 4
job "backup": command "backup" must be an absolute path
...
```

All problems are reported at once; the complete output of that file is in
[examples.md](examples.md). The rules are listed in
[configuration.md](configuration.md).

## list

```console
$ cronx list
JOB     SCHEDULE      NEXT
backup  0 3 * * *     2026-10-05 03:00:00 CEST
cleanup */15 * * * *  2026-10-04 21:30:00 CEST
```

A job whose expression can never match, such as `0 0 31 4 *`, is shown with a
dash in the `NEXT` column.

## status

```console
$ cronx status
JOB     LAST STATUS  LAST RUN                  IN PROGRESS
backup  succeeded    2026-10-04 03:00:00 CEST  no
cleanup never run    -                         no
```

## history

```console
$ cronx history --limit 5
ID  JOB     ATTEMPT  STATUS     STARTED                   DURATION  EXIT
12  backup  1        succeeded  2026-10-04 03:00:00 CEST  1.2s      0
11  backup  2        failed     2026-10-03 03:00:00 CEST  0.1s      1
10  backup  1        failed     2026-10-03 03:00:00 CEST  0.1s      1

$ cronx history backup
```

Each row is one attempt. The `DURATION` is how long the process ran and `EXIT`
is its exit status, shown as a dash when no process ran, as for a run skipped by
the overlap policy.

## run-once

```console
$ cronx run-once backup
job "backup" finished with status "succeeded"
```

The run is recorded in the history exactly as a scheduled run would be, and its
output goes to the usual log file. The overlap, retry and timeout policies of
the job still apply: a `run-once` that arrives while the job is already running
is skipped when the overlap policy is `skip`.

## run

```console
$ cronx run
```

The scheduler stays in the foreground. It writes to `~/.cronx/logs/cronx.log`
and to the standard error. Interrupting it (`Ctrl-C`, `SIGINT` or `SIGTERM`)
stops the jobs that are still running, waits for them and then exits. To keep it
running in the background, use the usual tools of your operating system, for
example a service manager.

## Logs

| File | Contents |
| ---- | -------- |
| `~/.cronx/logs/cronx.log` | What the scheduler itself did. |
| `~/.cronx/logs/<job>/<run id>.log` | Everything the job wrote to its standard output and standard error. |

Logs are appended to and are never rotated automatically; the files are only
readable by their owner.
