# Scheduling

cronx schedules every job with a cron expression, in the spirit of traditional
Unix `cron`: five fields, six with the seconds field in front of them, or a
descriptor that stands for a fixed time.

## Expression format

The five fields a traditional expression has:

```
┌──────────── minute        (0-59)
│ ┌────────── hour          (0-23)
│ │ ┌──────── day of month  (1-31)
│ │ │ ┌────── month         (1-12, or jan-dec)
│ │ │ │ ┌──── day of week   (0-6, or sun-sat; 7 also means Sunday)
│ │ │ │ │
* * * * *
```

An expression that states the seconds as well carries the seconds in front of
them, which makes six fields:

```
┌──────────── second        (0-59)
│
* * * * * *
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

The `?` operator of Quartz-style crons is deliberately **not** supported: it is
written as `*` here, which means the same thing.

## Day operators

The two day fields — day of month and day of week — accept the `L`, `W` and `#`
operators, which name days a plain value cannot. They are the two fields the
operators belong to: in the others `L`, `W` and `#` are refused, and the error
names the field that does take them.

| Operator | Field | Meaning |
| -------- | ----- | ------- |
| `L`      | day of month | the last day of the month, whether it is the 28th, the 30th or the 31st |
| `LW`     | day of month | the last weekday of the month: the last day of it that is neither a Saturday nor a Sunday |
| `nW`     | day of month | the weekday nearest to the n-th day of the month |
| `L`      | day of week | the last day of the week, which is Saturday where Sunday is the first day of it |
| `nL`     | day of week | the last occurrence of the weekday n in the month |
| `n#m`    | day of week | the m-th occurrence of the weekday n in the month, counted from 1 |

An operator is a whole list element, so it carries neither a range nor a step:
`L/2` and `1-L` are refused rather than given a meaning, while `1,15,L` is a list
of three days. The weekday of `nL` and `n#m` is a number or a three-letter name,
and `nW` takes a day of the month, from 1 to 31.

```
0 0 L * *        the last day of every month
0 7 LW * *       at 07:00 on the last weekday of every month
0 9 15W * *      at 09:00 on the weekday nearest to the 15th
0 4 * * L        at 04:00 every Saturday
0 22 * * 5L      at 22:00 on the last Friday of every month
30 3 * * sun#1   at 03:30 on the first Sunday of every month
0 6 1,15,L * *   at 06:00 on the 1st, the 15th and the last day
```

A month that does not hold the day an operator names is a month the job does not
run in: a `31W` is nothing at all in a February of 28 days, and the next
activation is in the month after it. `n#m` is the same for an occurrence a month
does not hold, such as a fifth Saturday, and `nW` never reaches into a
neighbouring month to find the day it was asked for. A day an operator selects is
a day like any other for the OR semantics below: `0 0 L * mon` runs on the last
day of the month and on every Monday.

## Seconds

The seconds field is the optional one, and it is the first. It accepts what the
other fields accept — `*`, a value, a range, a list and a step — and it takes no
names, because there are none for a second.

A five-field expression leaves the seconds unsaid, and what it leaves unsaid is
zero: its activations are on the minute, exactly as they always were. Writing the
seconds is what makes a job run inside the minute, and a job that runs several
times a minute is a job whose triggers the overlap policy judges one by one.

```
*/10 * * * * *    every ten seconds
0,30 * * * * *    at :00 and :30 of every minute
30 * * * * *      at :30 of every minute
0 0 3 * * *       every day at 03:00, the same as 0 3 * * *
```

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
twice is two starts, and therefore two runs. The same holds for a job that asks
to be caught up, whose trigger is the start as well: a reload catches nothing up
(see [Missed runs](#missed-runs)).

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
- an activation that a backward transition repeats fires once, not twice: an
  expression that states the seconds walks through the repeated hour once, taking
  the activations it has not reached yet from that hour's second pass, so no wall
  time of it runs twice either.

## Missed runs

By default cronx does not catch up. If the scheduler is not running when a job
becomes due, that activation is simply skipped and the next one is computed from
the current time. There are no "missed run" bursts after a restart, and the
seconds field changes nothing about that: a job that fires every ten seconds is
not run thirty times to make up for the five minutes the scheduler was down.

A job can ask for the opposite with `catch_up = true`. A job that asks for it is
run **once** when the scheduler starts, if at least one of its activations
passed while cronx was not running:

- the trigger is the **start** of the scheduler, so a reload catches nothing up:
  a scheduler that is already running is not a start, exactly as for `@reboot`;
- it is one run, however many activations were missed. It stands for the earliest
  of them — the first activation after the last run of the job — and the run it
  leaves in the history is what the next start counts from, so what was made up
  for is not made up for again;
- what says how far back cronx looks is the history of the job, and a run is
  written to it before the job is executed: a scheduler that was killed in the
  middle of a run therefore leaves the anchor where it should be;
- a job that has **never run** has no instant to count its missed activations
  from, so it is not caught up: it waits for its next activation, or for
  `cronx run-once`;
- a schedule with no activation on the clock is not caught up either, which is
  `@reboot`: the start is already its trigger;
- a job that is **disabled** is not caught up: it is not scheduled at all;
- the run is judged by the overlap policy of the job and counts against
  `max_parallel_jobs` like every other trigger. It is recorded in the history at
  the instant it started, like any other run, and the scheduler log names the
  missed activation it stands for.

## Disabled jobs

`enabled = false` keeps a job in the configuration without scheduling it. The
job is still listed by `cronx list`, whose `NEXT` column says `disabled` where a
job that can never match gets a dash and one that runs at startup says
`at startup`. Its history is untouched: `cronx status` and `cronx history` report
what the job did before it was disabled.

What stops is the scheduling. No activation is planned for it, so it is never
triggered, never caught up and never holds a slot. Setting `enabled` back to
`true` is what schedules it again, from the moment the scheduler reads the file:
a reload is enough, as it is for any other change to a job.

`cronx run-once` runs a disabled job like any other. It is an action of the
person running it rather than a trigger of the scheduler, and what `enabled`
turns off is the scheduling, not the job.

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
| `*/10 * * * * *`      | every ten seconds |
| `0,30 * * * * *`      | at :00 and :30 of every minute |
| `0 0 3 * * *`         | every day at 03:00, the same as `0 3 * * *` |
| `@daily`              | every day at midnight |
| `@monthly`            | at midnight on the first day of every month |
| `@reboot`             | once, when the scheduler starts |
| `30 2 * jan,jul *`    | at 02:30 in January and in July |
| `0 0 * * 7`           | at midnight on Sundays |
| `0 0 L * *`           | at midnight on the last day of every month |
| `0 7 LW * *`          | at 07:00 on the last weekday of every month |
| `0 9 15W * *`         | at 09:00 on the weekday nearest to the 15th |
| `0 22 * * 5L`         | at 22:00 on the last Friday of every month |
| `0 0 * * L`           | at midnight every Saturday |
| `30 3 * * sun#1`      | at 03:30 on the first Sunday of every month |
