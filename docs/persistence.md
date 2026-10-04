# Persistence

cronx keeps what actually happened in a single SQLite database. The
configuration file stays the source of truth for what *should* happen; the
database is runtime state and history, and cronx never rewrites the
configuration.

## Location

| Data              | Where              |
| ----------------- | ------------------ |
| Desired config    | `~/.cronx/config.toml` |
| Runtime state     | `~/.cronx/state.db` |
| Execution history | `~/.cronx/state.db` |
| Run output        | `~/.cronx/logs/`   |

The database file and its directory are created automatically when needed, with
owner-only permissions. The connection enables the write-ahead log, a five
second busy timeout and foreign keys.

## Schema

### `schema_meta`

Holds a single row with the schema version.

### `runs`

One row per execution.

| Column        | Type    | Meaning |
| ------------- | ------- | ------- |
| `id`          | integer | Identifier of the run, increasing over time. |
| `job`         | text    | Name of the job. |
| `attempt`     | integer | 1 for the first try, then 2, 3... `0` for a run that never attempted an execution. |
| `status`      | text    | See the statuses below. |
| `started_at`  | text    | When the run was attempted (UTC, RFC 3339). |
| `finished_at` | text    | When the outcome was observed, empty while the run is in progress. |
| `exit_code`   | integer | Exit status of the process, empty when no process ran. |
| `duration_ms` | integer | How long the run took, in milliseconds. |
| `error`       | text    | Why the run failed, when there is a reason. |
| `log_path`    | text    | Where the output of the run was written. |

Runs are read newest first, which is the order `cronx history` prints them in.

### `job_state`

A small cache with one row per job: the last status, the last start and finish
times and when it was updated. It exists so that the scheduler and the CLI can
answer "what happened to this job last time?" without scanning the history.

## Statuses

| Status        | Meaning |
| ------------- | ------- |
| `scheduled`   | The run is known but has not started. |
| `running`     | The job is executing. |
| `succeeded`   | The process exited with status zero. |
| `failed`      | The process exited with a non-zero status. |
| `timed_out`   | The job was stopped because it exceeded its timeout. |
| `spawn_error` | The job could not be started at all. |
| `skipped`     | The trigger did not lead to an execution, for example because the overlap policy is `skip`. |

`scheduled` and `running` are the two non-final states.

## Time

Timestamps are stored in UTC, as RFC 3339, so that the database is unambiguous
regardless of the timezone the scheduler runs in. The scheduler converts to the
configured timezone when it decides *when* to run jobs, not when it records
*what* happened.

## Migrations

The schema is versioned. When the database is opened, pending migrations are
applied in order, each inside a transaction, and the resulting version is
recorded in `schema_meta`. If the stored version is newer than the version the
running binary understands, cronx refuses to open the database rather than risk
corrupting it.

## Restart behaviour

Runs that were still in progress when the scheduler stopped cannot be observed
any more. On the next start they are closed as `failed`, with the explanation
that the scheduler stopped while they were in progress. This keeps the history
truthful and stops a stale `running` row from blocking a job whose overlap
policy is `skip`.

Activation times that passed while the scheduler was not running are **not**
replayed: see [scheduling.md](scheduling.md).
