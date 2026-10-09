# Configuration

cronx is configured with a single TOML file. The file describes the desired
state of the scheduler: it is the source of truth for which jobs exist and how
they are executed. Runtime state and execution history live elsewhere (SQLite),
so the configuration file is never rewritten by cronx.

## File location

The configuration path is resolved in this order:

1. the `--config` command-line flag;
2. the `CRONX_CONFIG` environment variable;
3. the default `~/.cronx/config.toml`.

`cronx init` writes a file there when there is not one yet: the smallest
configuration cronx accepts, with every other option commented out and
explained, so that the file doubles as a reference. It refuses to replace a file
that already exists unless it is given `--force`, and it writes nothing else.
See [cli.md](cli.md) for the command.

## Applying a change

A running scheduler reads the file again whenever it is sent `SIGHUP`, so a job
that was added, removed or rescheduled takes effect without a restart. The jobs
are applied whole and at once; the reload is refused if the file cannot be read
or used, and the scheduler then keeps running with the configuration it had.

Not every setting can be applied to a process that is already running, because
what reads it was built when the scheduler started. They are the whole
`[scheduler]` table, the `path`, `max_size` and `max_backups` of `[logging]`, and
the `path` and `max_runs` of `[storage]`. A reload reports each of them that has
changed and ignores it; a restart is what applies them. See [cli.md](cli.md) for
how the reload is asked for and recorded.

## Reference examples

Complete files to copy are kept in
[`examples/configs/`](../examples/configs/) and are described one by one in
[examples.md](examples.md): the smallest file cronx accepts, one using every
field below, a realistic set-up for a server and one that is deliberately
invalid. The test suite reads them, so they cannot drift away from what cronx
accepts.

## Structure

```toml
[scheduler]
timezone = "Europe/Rome"          # IANA name or "Local" (default: "Local")
max_parallel_jobs = 2             # at least 1 (default: 1)

[logging]
level = "info"                    # debug | info | warn | error (default: "info")
path  = "/var/log/cronx/runs.log" # where every job's output is written (default: ~/.cronx/logs/runs.log)
max_size = "10MB"                 # rotate the run log past this size (default: never)
max_backups = 3                   # how many rotated run logs are kept (default: 3)

[storage]
path = "/var/lib/cronx/state.db"  # the status database (default: ~/.cronx/state.db)
max_runs = 1000                   # how many runs of each job the history keeps (default: all)

[jobs.backup]
schedule = "0 3 * * *"            # required
command = "/usr/local/bin/backup" # required, absolute path
args = ["--incremental", "--destination", "/backup"]
timeout = "30m"
grace_period = "15s"              # time to stop after SIGTERM (default: 10s)
retry = 2
overlap = "skip"
working_directory = "/var/lib/backup"
env = { TIER = "gold" }
```

## Field reference

### `[scheduler]`

| Field               | Type    | Default   | Notes |
| ------------------- | ------- | --------- | ----- |
| `timezone`          | string  | `"Local"` | IANA timezone name or `Local`. |
| `max_parallel_jobs` | integer | `1`       | Maximum number of jobs running at the same time; must be at least 1. |

`timezone` is the clock every activation is computed on: with
`timezone = "Europe/Rome"` a job whose schedule says `30 2 * * *` runs at 02:30
in Rome, whatever the zone of the machine. `Local`, the default, means the zone
of the machine. The zone database travels with the binary, so a name resolves
even where the system carries no `/usr/share/zoneinfo`.

### `[logging]`

| Field         | Type    | Default  | Notes |
| ------------- | ------- | -------- | ----- |
| `level`       | string  | `"info"` | One of `debug`, `info`, `warn`, `error`. |
| `path`        | string  | `~/.cronx/logs/runs.log` | Where the output of every job is written: a single file, shared by all runs. Optional. |
| `max_size`    | string (size) | none | Rotate the run log once it would pass this size. Optional; without it the log is never rotated. |
| `max_backups` | integer | `3`      | How many rotated run logs are kept. Optional, and only meaningful together with `max_size`. |

A size is a number of bytes, optionally followed by `KB`, `MB` or `GB`
(case-insensitive): `"512KB"`, `"10MB"`, `"1GB"`, `"1048576"`.

#### Rotation of the run log

With `max_size` set, the run log stops growing without bound. When a line would
take the file past the given size, the file is renamed to `<path>.1` before the
line is written, the files before it move one step back — `<path>.1` becomes
`<path>.2` — and the oldest beyond `max_backups` is deleted. A new, empty log
takes the place of the first one.

```console
$ ls -l ~/.cronx/logs/
runs.log      # the runs that are happening now
runs.log.1    # the runs before them
runs.log.2    # the runs before those
```

`runs.log.1` is therefore the most recent history and the higher the number the
older the file. `cronx logs` reads the current file: a line that was rotated away
is in one of the numbered files, which can be read directly.

Two details are worth knowing. The rotation happens on a line boundary, so a line
is never cut in half, and a log is never rotated before its first line: a job
that prints more at once than the run log may hold still has that line kept,
rather than rotated away the moment it is written. And only the run log is
rotated. `~/.cronx/logs/cronx.log` holds one line per event of the scheduler
itself rather than whatever a job prints, so it has no size to bound.

### `[storage]`

| Field      | Type    | Default | Notes |
| ---------- | ------- | ------- | ----- |
| `path`     | string  | `~/.cronx/state.db` | The status database: the execution history and the runtime state. Optional. |
| `max_runs` | integer | none | How many runs of each job the history keeps; must be at least 1. Optional, and without it nothing is pruned. |

Both `logging.path` and `storage.path` are optional. When they are absent the
paths above are used; when they are set, cronx writes there instead. A path is
used exactly as written (no `~` expansion), and the directories it needs are
created when cronx first writes to it.

#### Pruning the history

Without `max_runs` the database keeps one row per attempt of every job, for
ever. With it, the scheduler keeps the newest `max_runs` runs of each job and
deletes the older ones. The trimming happens when the scheduler starts, for
every job, and again each time a trigger arrives for a job, so that a scheduler
left running for months bounds the database as well as one that is restarted.

```console
$ cronx history --limit 3
ID   JOB     ATTEMPT  STATUS     STARTED                   DURATION  EXIT
512  backup  1        succeeded  2026-10-06 03:00:00 CEST  1.2s      0
511  backup  2        failed     2026-10-05 03:00:00 CEST  0.1s      1
510  backup  1        failed     2026-10-05 03:00:00 CEST  0.1s      1
```

The count is per job, so a job that runs every minute cannot push the rare runs
of another job out of the history. Two things are never deleted: a run that is
still in progress, because its outcome has not been recorded yet, and the
`job_state` row of each job, which holds only the last outcome and is what
`cronx status` reads. The run log is not touched: `[logging].max_size` is what
bounds that one.

### `[jobs.<name>]`

The job name is the TOML table key. It must match `[A-Za-z0-9_-]+`.

| Field               | Type              | Default  | Notes |
| ------------------- | ----------------- | -------- | ----- |
| `schedule`          | string            | -        | Required. A cron expression: five fields, six with the seconds field in front of them, or a descriptor such as `@daily` (see `docs/scheduling.md`). |
| `command`           | string            | -        | Required. Absolute path of the executable. |
| `args`              | array of strings  | `[]`     | Passed to the executable verbatim. |
| `timeout`           | string (duration) | none     | Go duration (for example `30m`, `1h30m`). Negative values are rejected. |
| `grace_period`      | string (duration) | `10s`    | How long the process is given to exit after `SIGTERM` before it is killed with `SIGKILL`. Must be greater than 0. |
| `retry`             | integer           | `0`      | Additional attempts after the first failure; must not be negative. |
| `overlap`           | string            | `"skip"` | One of `skip`, `allow`, `queue`. |
| `working_directory` | string            | none     | Working directory of the process. |
| `env`               | table of strings  | none     | Extra environment variables for the process. |

The `working_directory`, when it is set, must exist when the job runs: a job that
cannot be started in it is recorded as a `spawn_error`, with an explanation that
names the directory rather than the command.

The `grace_period` is what a job is given to stop on its own before it is killed.
When its `timeout` expires, and when the scheduler is asked to stop while the job
is still running, the whole process group is asked to terminate with `SIGTERM`
and is killed with `SIGKILL` once the grace period is over. Ten seconds is the
default; a job that needs longer to shut down cleanly asks for more. See
[security.md](security.md) for the process group and the signals.

## Validation

`cronx validate` loads the configuration and reports every problem it finds, so
a single run lists all the mistakes at once rather than stopping at the first.
The following rules are enforced:

- unknown keys anywhere in the document are rejected (typos are not ignored);
- a job name must match `[A-Za-z0-9_-]+`;
- `schedule` and `command` are required;
- `schedule` must be a valid cron expression, as described in
  [scheduling.md](scheduling.md);
- `command` must be an absolute path;
- `retry` must not be negative;
- `timeout`, when present, must be a valid, non-negative duration;
- `grace_period`, when present, must be a valid duration greater than 0;
- `overlap` must be one of `skip`, `allow`, `queue`;
- `max_parallel_jobs` must be at least 1;
- `timezone` must be `Local` or a name the zone database knows;
- `logging.level` must be one of `debug`, `info`, `warn`, `error`;
- `logging.max_size`, when present, must be a size greater than zero, written as
  a number of bytes or with a `KB`, `MB` or `GB` suffix;
- `logging.max_backups`, when present, must be at least 1 and needs
  `logging.max_size`: without a size nothing is rotated, so there would be
  nothing to keep;
- `logging.path` and `storage.path`, when present, are taken as written;
- `storage.max_runs`, when present, must be at least 1.

Example of explicit failure, using the deliberately invalid
[`examples/configs/broken.toml`](../examples/configs/broken.toml):

```console
$ cronx validate --config examples/configs/broken.toml
cronx: invalid configuration: scheduler.max_parallel_jobs must be at least 1, got 0
logging.level "verbose" is not one of debug, error, info, warn
job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, or 6 with the seconds field first, got 4
job "backup": command "backup" must be an absolute path
...
```

The whole output, and the mistake behind each line, is listed in
[examples.md](examples.md).

## Security notes

- `command` is always used as-is. cronx never resolves it through `PATH` and
  never runs it through a shell.
- `args` are passed to the executable as individual, literal arguments. Shell
  metacharacters such as `;`, `|`, `$` or `*` have no special meaning.
