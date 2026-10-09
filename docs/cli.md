# Command line

```
cronx <command> [flags]
```

Every command accepts the global flag `--config`, which selects the
configuration file. When it is not given, `CRONX_CONFIG` is used, and failing
that the default `~/.cronx/config.toml`. The runtime state and the logs live
under `~/.cronx/` unless the configuration moves them; see
[configuration.md](configuration.md).

| Command | Purpose |
| ------- | ------- |
| `init` | Write a configuration file to start from, with every other option commented out. |
| `validate` | Check the configuration file and report every problem it finds. |
| `list` | Show the configured jobs, their schedule and when each runs next. |
| `status` | Show the last outcome of every job and whether a run is in progress. |
| `history [job]` | Show the execution history, newest first. |
| `logs [job]` | Print the output of the runs, as the run log holds it. |
| `run-once <job>` | Run a job immediately, whatever its schedule. |
| `run` | Run the scheduler in the foreground until it is stopped. |
| `version` | Print the version of cronx. |

Every command that reads the configuration can be tried against the files kept in
[`examples/configs/`](../examples/configs/), which are described one by one in
[examples.md](examples.md).

The exit status is `0` on success and `1` on failure. `run-once` fails when the
job did not succeed, so it can be used in scripts.

## init

```console
$ cronx init
/home/user/.cronx/config.toml written
```

The file is written where the configuration is looked for, and the directory it
lives in is created if it is not there yet. It holds one job — the smallest one
worth having, with its schedule, its command and the arguments of the command —
and every other option cronx knows, commented out with what it does and what
happens without it, so that the file can be read as its own reference:

```toml
[scheduler]
# timezone = "Local"            # an IANA name such as "Europe/Rome", or "Local" for the zone of the machine
# max_parallel_jobs = 1         # how many jobs may run at the same time

# The jobs. A job is one [jobs.<name>] table; the name may hold letters, digits,
# - and _. Its schedule is a cron expression of five fields, or of six with the
# seconds field in front of them, or a descriptor: @yearly, @monthly, @weekly,
# @daily, @midnight and @hourly stand for a fixed time, and @reboot runs the job
# once, when the scheduler starts. The two day fields also accept the day
# operators: L for the last day of a month or of a week, LW for the last weekday
# of a month, nW for the weekday nearest to its n-th day, and nL and n#m for the
# last and the n-th occurrence of a weekday in a month.
[jobs.hello]
schedule = "*/5 * * * *"        # a five-field cron expression: every five minutes
command = "/bin/echo"           # required, an absolute path: cronx never looks a command up in PATH
args = ["hello from cronx"]     # passed to the program as they are, never through a shell
# timeout = "30m"               # stop it, and the processes it started, after this long (default: no timeout)
# ...
```

The file gets the permissions of everything else cronx writes: `0700` for the
directory and `0600` for the file. A configuration that is already there is never
replaced — the command stops, names the file and leaves it as it was — unless
`--force` is given. Nothing else happens: no state is opened and no log is
created, so the command can be run before there is an installation at all.

## validate

```console
$ cronx validate
/home/user/.cronx/config.toml is valid

$ cronx validate --config examples/configs/broken.toml
cronx: invalid configuration: scheduler.max_parallel_jobs must be at least 1, got 0
logging.level "verbose" is not one of debug, error, info, warn
job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, or 6 with the seconds field first, got 4
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
dash in the `NEXT` column. A job that runs when the scheduler starts, which is
what `@reboot` asks for, says `at startup` there instead: it is waiting for a
scheduler that is not running, which is a different thing from a schedule that
can never match. A job that is kept without being scheduled, which is what
`enabled = false` asks for, says `disabled`: it has no activation at all until it
is enabled again. See [configuration.md](configuration.md).

The `NEXT` column is a wall clock in the zone of `[scheduler].timezone`: with
`timezone = "Europe/Rome"` the times above are Rome's, whatever zone the machine
running cronx is in.

## status

```console
$ cronx status
JOB     LAST STATUS  LAST RUN                  IN PROGRESS
backup  succeeded    2026-10-04 03:00:00 CEST  no
cleanup never run    -                         no

scheduler running since 2026-10-04 12:00:00 CEST (workstation:4182), lease until 2026-10-04 12:00:30 CEST
```

The last line says whether a scheduler is driving the state. Without one the
jobs are not being scheduled at all, however healthy their last runs look. The
lease it names is the right to run a scheduler on the state: the holder is the
machine and the process, and the expiry is when the state is free again. A lease
that has expired was left behind by a scheduler that was killed rather than
stopped, and is reported as what it is — a state nothing is driving, which the
next `cronx run` takes.

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

The history is trimmed to the newest `[storage] max_runs` runs of each job when
that is configured, so it does not grow without bound; see
[persistence.md](persistence.md). Without it the history keeps every run.

## logs

```console
$ cronx logs backup
2026-10-06T03:00:00Z backup id=12 pid=4182 starting the backup
2026-10-06T03:00:01Z backup id=12 pid=4182 done

$ cronx logs --since 1h --follow
```

It prints the run log, oldest line first, exactly as it is written: with a job
name only that job's lines, without one the lines of every job. `--since` leaves
out what was written before the given moment, written either as a duration
counted back from now (`30m`, `2h`) or as an RFC 3339 instant
(`2026-10-06T03:00:00Z`). `--follow` keeps printing as the log grows, until the
command is interrupted, which is what `tail -f` does for a file.

The log is the file `[logging].path` points at, `~/.cronx/logs/runs.log` by
default. The command reads it and nothing else: it opens no state database,
writes nothing and creates nothing, so it is safe to run while a scheduler is
running and its output can be piped. A line that is not a run log line — a file
that is not the run log, or something a person appended — is left out. When no
line matches, nothing is printed and the exit status is still `0`.

When the run log is rotated (`[logging].max_size`), the command reads the file
that is being written: the lines that were rotated away are in `runs.log.1` and
the files after it, which can be read directly or with `cronx logs` after the
file has been moved back. `--follow` notices the rotation and carries on with the
new file.

## run-once

```console
$ cronx run-once backup
job "backup" finished with status "succeeded"
```

The run is recorded in the history exactly as a scheduled run would be, and its
output goes to the run log. The overlap and retry policies, the timeout and the
grace period of the job still apply: a `run-once` that arrives while the job is
already running is skipped when the overlap policy is `skip`.

A job that is disabled is run like any other one. What `enabled = false` turns
off is the scheduling, not the job, and this command is an action of the person
running it.

`run-once` takes no lease on the state: it is an action of the person running it,
not a scheduler driving the state, so it runs whether or not `cronx run` is
running.

## run

```console
$ cronx run
```

The scheduler stays in the foreground. It writes to `~/.cronx/logs/cronx.log`
and to the standard error. Interrupting it (`Ctrl-C`, `SIGINT` or `SIGTERM`)
stops the jobs that are still running, waits for them and then exits. To keep it
running in the background, use the usual tools of your operating system, for
example a service manager.

One scheduler drives a state at a time. `cronx run` takes a lease on the state
before it schedules anything, renews it while it runs and gives it back when it
stops, so a second `cronx run` on the same state refuses to start instead of
scheduling the same jobs a second time:

```console
$ cronx run
cronx: another scheduler is running: workstation:4182 holds the state until 2026-10-04T10:00:30Z
```

A scheduler that is killed outright cannot give the lease back, so it keeps the
state for at most thirty seconds, the lifetime of a lease. `cronx status`
reports the scheduler that holds the state, if any.

A change to the configuration file does not need a restart. Send the scheduler
`SIGHUP` (`kill -HUP 4182`) and it reads the file again:

```console
$ kill -HUP 4182
```

The jobs of the new file take effect from that moment: a job whose schedule
changed follows the new one, a job that was added is picked up, a job that is
gone is never run again, and a job that was disabled — or enabled again — is
scheduled accordingly. Runs in progress are left alone. A file that cannot be
read or used is refused — the scheduler records why, and keeps running with the
configuration it has — and a change to `[scheduler]`, `[logging]` or `[storage]`
cannot reach a process that is already running: it is reported and ignored, and
needs a restart. The reload is recorded in the scheduler log, which
`cronx logs --follow` does not read; watch `~/.cronx/logs/cronx.log` or the
standard error of `cronx run` to see it happen.

## version

```console
$ cronx version
cronx 0.4.0
```

The command prints the version of cronx and nothing else, so it can be used in
scripts. It reads no configuration and writes no state.

## Logs

| File | Contents |
| ---- | -------- |
| `~/.cronx/logs/cronx.log` | What the scheduler itself did. |
| `~/.cronx/logs/runs.log` | The output of every run of every job, in one file. |

Every line of the run log begins with the instant it was written, the job, the
run identifier and the pid of the process, so that the output of concurrent jobs
can be told apart:

```console
2026-10-06T03:00:00Z backup id=12 pid=4182 starting the backup
2026-10-06T03:00:01Z backup id=12 pid=4182 done
```

Both files are appended to and only their owner can read them. Their location
can be moved with `[logging] path` (the run log) and `[storage] path` (the status
database): see [configuration.md](configuration.md). The run log is the file
`cronx logs` reads back, so the lines above are what that command prints.

The run log is rotated once it would pass `[logging] max_size`, if that is
configured: the file becomes `runs.log.1`, the files before it move one step
back, and at most `[logging] max_backups` of them are kept. The scheduler log is
not rotated: it holds one line per event of the scheduler itself rather than
whatever a job prints.
