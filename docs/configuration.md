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

## Structure

```toml
[scheduler]
timezone = "Europe/Rome"          # IANA name or "Local" (default: "Local")
max_parallel_jobs = 2             # at least 1 (default: 1)

[logging]
level = "info"                    # debug | info | warn | error (default: "info")

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

## Validation

`cronx validate` loads the configuration and reports every problem it finds, so
a single run lists all the mistakes at once rather than stopping at the first.
The following rules are enforced:

- unknown keys anywhere in the document are rejected (typos are not ignored);
- a job name must match `[A-Za-z0-9_-]+`;
- `schedule` and `command` are required;
- `command` must be an absolute path;
- `retry` must not be negative;
- `timeout`, when present, must be a valid, non-negative duration;
- `overlap` must be one of `skip`, `allow`, `queue`;
- `max_parallel_jobs` must be at least 1;
- `logging.level` must be one of `debug`, `info`, `warn`, `error`.

Example of explicit failure:

```console
$ cronx validate --config ./bad.toml
cronx: invalid configuration: job "backup": schedule is required
job "backup": command "backup" must be an absolute path
job "backup": overlap "sometimes" is not one of skip, allow, queue
```

## Security notes

- `command` is always used as-is. cronx never resolves it through `PATH` and
  never runs it through a shell.
- `args` are passed to the executable as individual, literal arguments. Shell
  metacharacters such as `;`, `|`, `$` or `*` have no special meaning.

## Currently deferred

The syntax of the cron expression is validated when the scheduling engine is
implemented. Until then `cronx validate` checks only that `schedule` is present.
The same applies to resolving `timezone` to an actual location, which happens
when the scheduler starts rather than at validation time.
