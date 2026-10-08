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

[storage]
path = "/var/lib/cronx/state.db"  # the status database (default: ~/.cronx/state.db)

[jobs.backup]
schedule = "0 3 * * *"            # required
command = "/usr/local/bin/backup" # required, absolute path
args = ["--incremental", "--destination", "/backup"]
timeout = "30m"
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

### `[logging]`

| Field   | Type   | Default  | Notes |
| ------- | ------ | -------- | ----- |
| `level` | string | `"info"` | One of `debug`, `info`, `warn`, `error`. |
| `path`  | string | `~/.cronx/logs/runs.log` | Where the output of every job is written: a single file, shared by all runs. Optional. |

### `[storage]`

| Field  | Type   | Default | Notes |
| ------ | ------ | ------- | ----- |
| `path` | string | `~/.cronx/state.db` | The status database: the execution history and the runtime state. Optional. |

Both `logging.path` and `storage.path` are optional. When they are absent the
paths above are used; when they are set, cronx writes there instead. A path is
used exactly as written (no `~` expansion), and the directories it needs are
created when cronx first writes to it.

### `[jobs.<name>]`

The job name is the TOML table key. It must match `[A-Za-z0-9_-]+`.

| Field               | Type              | Default  | Notes |
| ------------------- | ----------------- | -------- | ----- |
| `schedule`          | string            | -        | Required. A five-field cron expression (see `docs/scheduling.md`). |
| `command`           | string            | -        | Required. Absolute path of the executable. |
| `args`              | array of strings  | `[]`     | Passed to the executable verbatim. |
| `timeout`           | string (duration) | none     | Go duration (for example `30m`, `1h30m`). Negative values are rejected. |
| `retry`             | integer           | `0`      | Additional attempts after the first failure; must not be negative. |
| `overlap`           | string            | `"skip"` | One of `skip`, `allow`, `queue`. |
| `working_directory` | string            | none     | Working directory of the process. |
| `env`               | table of strings  | none     | Extra environment variables for the process. |

The `working_directory`, when it is set, must exist when the job runs: a job that
cannot be started in it is recorded as a `spawn_error`, with an explanation that
names the directory rather than the command.

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
- `overlap` must be one of `skip`, `allow`, `queue`;
- `max_parallel_jobs` must be at least 1;
- `logging.level` must be one of `debug`, `info`, `warn`, `error`;
- `logging.path` and `storage.path`, when present, are taken as written.

Example of explicit failure, using the deliberately invalid
[`examples/configs/broken.toml`](../examples/configs/broken.toml):

```console
$ cronx validate --config examples/configs/broken.toml
cronx: invalid configuration: scheduler.max_parallel_jobs must be at least 1, got 0
logging.level "verbose" is not one of debug, error, info, warn
job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, got 4
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

## Currently deferred

`timezone` is stored as written and resolved to a location when the scheduler
starts, rather than when the file is validated: a name that does not exist yet
on the machine is only reported at that point.
