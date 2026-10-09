# Persistence

cronx keeps what actually happened in a single SQLite database. The
configuration file stays the source of truth for what *should* happen; the
database is runtime state and history, and cronx never rewrites the
configuration.

## Location

| Data              | Where                    |
| ----------------- | ------------------------ |
| Desired config    | `~/.cronx/config.toml`   |
| Runtime state     | `~/.cronx/state.db`      |
| Execution history | `~/.cronx/state.db`      |
| Run output        | `~/.cronx/logs/runs.log` |

The status database and the run log can be moved out of the home directory with
`[storage].path` and `[logging].path`; see [configuration.md](configuration.md).
Every run of every job appends to the same run log.

The run log is rotated when `[logging].max_size` is configured: what it held
becomes `runs.log.1`, the file before it `runs.log.2` and so on, up to
`[logging].max_backups` of them. The history is unaffected: the `log_path` of a
run names the run log, whose rotated files are its neighbours on disk.

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
| `log_path`    | text    | Where the output of the run was written: the shared run log.

Runs are read newest first, which is the order `cronx history` prints them in.

### `job_state`

A small cache with one row per job: the last status, the last start and finish
times and when it was updated. It exists so that the scheduler and the CLI can
answer "what happened to this job last time?" without scanning the history.

### `scheduler_lease`

Holds a single row: the lease that keeps one scheduler per state.

| Column        | Type | Meaning |
| ------------- | ---- | ------- |
| `holder`      | text | Who took the lease: the machine and the process, as `host:pid`. |
| `acquired_at` | text | When it was taken (UTC, RFC 3339). |
| `expires_at`  | text | When it stops being valid (UTC, RFC 3339). |

`cronx run` takes the lease before it schedules anything, and refuses to start
while another scheduler holds one that has not expired. The lease is renewed
while the scheduler runs and deleted when it stops, so the state is free the
moment a scheduler stops cleanly. A scheduler that was **killed** instead leaves
its lease behind, and the state becomes free again when the lease expires:
thirty seconds is the lifetime of a lease, a third of which is the interval
between two renewals. `cronx status` reports the lease, if there is one.

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

The transaction begins immediately and the stored version is checked again
inside it. Two processes that open a brand new database at the same moment
therefore take turns: the first migrates it, and the second finds the migration
already applied and does nothing, instead of failing because the tables it is
about to create already exist.

## Restart behaviour

Runs that were still in progress when the scheduler stopped cannot be observed
any more. On the next start they are closed as `failed`, with the explanation
that the scheduler stopped while they were in progress. This keeps the history
truthful and stops a stale `running` row from blocking a job whose overlap
policy is `skip`.

A scheduler that is **stopped** is not one of those cases: it stops the job it is
running, waits for it, and records the outcome before it exits, so the history is
already final when the process is gone. The repair on the next start is what
covers a process that was **killed** before it could record anything. A
scheduler that was killed leaves its lease behind, so that repair happens when
the lease expires and the next `cronx run` takes the state.

Activation times that passed while the scheduler was not running are **not**
replayed: see [scheduling.md](scheduling.md).
