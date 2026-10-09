# Scheduling

cronx schedules every job with a cron expression, in the spirit of traditional
Unix `cron`: five fields, or a descriptor that stands for a fixed time.

## Expression format

```
┌──────────── minute        (0-59)
│ ┌────────── hour          (0-23)
│ │ ┌──────── day of month  (1-31)
│ │ │ ┌────── month         (1-12, or jan-dec)
│ │ │ │ ┌──── day of week   (0-6, or sun-sat; 7 also means Sunday)
│ │ │ │ │
* * * * *
```

## Supported syntax

| Form     | Meaning |
| -------- | ------- |
| `*`      | every value |
| `a`      | a single value |
| `a-b`    | the inclusive range from `a` to `b` |
| `a,b,c`  | a list of the above |
| `*/n`    | every `n`-th value, starting at the field's minimum |
| `a-b/n`  | every `n`-th value within `a-b` |

Months and days of the week also accept their three-letter names,
case-insensitively: `jan`, `feb`, ... `dec` and `sun`, `mon`, ... `sat`. The
day of week accepts both `0` and `7` for Sunday.

A value that carries a step must use `*` or a range: `*/15` and `5-59/15` are
valid, while `5/15` is rejected with an explanatory error rather than being
given a surprising meaning.

The following are deliberately **not** supported: a leading seconds field, and
the `L`, `W`, `#` and `?` operators.

## Descriptors

A fixed time — or the start of the scheduler — can be written as a descriptor
instead of as five fields:

| Descriptor  | Same as     | Time |
| ----------- | ----------- | ---- |
| `@yearly`   | `0 0 1 1 *` | midnight on 1 January |
| `@monthly`  | `0 0 1 * *` | midnight on the first day of the month |
| `@weekly`   | `0 0 * * 0` | midnight on Sunday |
| `@daily`    | `0 0 * * *` | midnight every day |
| `@midnight` | `0 0 * * *` | the same as `@daily` |
| `@hourly`   | `0 * * * *` | the start of every hour |
| `@reboot`   | —           | once, when the scheduler starts |

The names are not case-sensitive, and a descriptor is a whole expression:
`@daily` stands alone, so a field after it is refused rather than guessed at. An
unknown descriptor is answered with the list of the ones cronx has. `cronx list`
prints the descriptor a job was written with, not the fields it stands for.

### Running at startup

`@reboot` is the one descriptor that is not a time. It has no activation on the
clock, so no expression stands behind it and the answer to *when does it run
next* is not an instant: `cronx list` says `at startup` instead, where a job that
can never run gets a dash.

The job is run once, when `cronx run` starts — from the configuration it is read
from at that moment, with the command it then has. A scheduler that is already
running is not a start, so a reload that adds a job of this kind does not run it:
the scheduler log says so, and the job waits for the next start. Starting cronx
twice is two starts, and therefore two runs.

Like every other trigger, this one is judged by the overlap policy of the job and
counts against `max_parallel_jobs`; runs left in progress by a previous process
are closed before it, so what the policy sees is the state as the scheduler found
it.

## Day of month and day of week

When **both** fields are restricted, a day matches if **either** of them
matches. This is the traditional cron behaviour. For example `0 0 13 * fri`
runs on the thirteenth of the month *and* on every Friday.

When only one of the two is restricted, only that one applies.

## Timezone

Activation times are computed on the clock of the zone configured in
`[scheduler].timezone`, which defaults to `Local`, the zone of the machine. The
expression is read as a wall clock in that zone: with
`timezone = "Europe/Rome"`, `0 3 * * *` runs at 03:00 in Rome whatever zone the
machine runs in. `cronx list` prints the next activation in the same zone.

Two rules cover the nights a clock moves:

- an activation that a forward transition removes does not exist, so it is
  skipped: on the night the clock jumps from 02:00 to 03:00, a job at
  `30 2 * * *` does not run at all;
- an activation that a backward transition repeats fires once, not twice.

## Missed runs

cronx does not catch up. If the scheduler is not running when a job becomes
due, that activation is simply skipped and the next one is computed from the
current time. There are no "missed run" bursts after a restart.

## Next activation

The scheduler asks the expression for the first activation strictly after the
current time, so a job whose activation is exactly now is not run twice.

An expression that can never match, such as `0 0 31 4 *` (April has no 31st
day), reports that it has no activation instead of searching forever. The
search horizon is twelve years, which comfortably covers the largest real gap
between two activations: 29 February around a century that is not a leap year,
for example 2096 and 2104.

## When a job runs

Two questions are answered separately:

- the cron expression answers *when* a job is due;
- the execution policy answers *what happens* when it becomes due while a
  previous run is still in progress.

### Overlap

| Policy  | Behaviour when a run is still in progress |
| ------- | ----------------------------------------- |
| `skip` (default) | The new trigger is ignored and recorded as a `skipped` run, with the reason. |
| `allow` | A second run starts immediately, alongside the first. |
| `queue` | The new run waits until the previous one has finished, then starts. |

The policy is applied to a trigger **when it arrives**, before the job waits for
a free slot, so the table holds whatever `max_parallel_jobs` is: with the
default of one, a trigger that arrives while the only slot is held by the run it
overlaps is still a trigger that the policy judges — a `skipped` run, not a run
that waits its turn.

### Retries

`retry = N` means up to `N` further attempts after the first one fails. Every
attempt is recorded separately, with its own attempt number and its own log
file, so the history shows exactly what happened. There is no backoff in this
version: a retry follows immediately.

### Parallelism

`max_parallel_jobs` bounds how many jobs run at the same time. When more jobs
are due than there are free slots, the extra ones wait for a slot rather than
being dropped. The wait comes after the overlap policy of each trigger has been
applied, and a `queue`d run waits for the run of its own job before it takes a
slot, so a job that is waiting its turn does not hold one.

### Stopping

When a job exceeds its `timeout`, and when the scheduler is asked to stop while
jobs are still running, the process group is asked to stop with `SIGTERM` and is
killed with `SIGKILL` if it is still running after the grace period. The grace
period is ten seconds unless the job sets `grace_period`. The scheduler waits for
its jobs before it exits.

## Examples

| Expression            | Meaning |
| --------------------- | ------- |
| `* * * * *`           | every minute |
| `*/15 * * * *`        | every 15 minutes |
| `0 3 * * *`           | every day at 03:00 |
| `0 */6 * * *`         | every six hours |
| `0 9-17 * * mon-fri`  | hourly from 09:00 to 17:00, on weekdays |
| `0 0 1 * *`           | at midnight on the first day of every month |
| `0 0 13 * fri`        | at midnight on the 13th and on every Friday |
| `@daily`              | every day at midnight |
| `@monthly`            | at midnight on the first day of every month |
| `@reboot`             | once, when the scheduler starts |
| `30 2 * jan,jul *`    | at 02:30 in January and in July |
| `0 0 * * 7`           | at midnight on Sundays |
