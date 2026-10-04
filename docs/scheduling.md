# Scheduling

cronx schedules every job with a five-field cron expression, in the spirit of
traditional Unix `cron`.

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

The following are deliberately **not** supported: descriptors such as `@daily`,
a leading seconds field, the `L`, `W`, `#` and `?` operators.

## Day of month and day of week

When **both** fields are restricted, a day matches if **either** of them
matches. This is the traditional cron behaviour. For example `0 0 13 * fri`
runs on the thirteenth of the month *and* on every Friday.

When only one of the two is restricted, only that one applies.

## Timezone

Activation times are computed in the timezone of the reference instant. In
practice that is the timezone configured in `[scheduler].timezone`, which
defaults to `Local`.

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
| `30 2 * jan,jul *`    | at 02:30 in January and in July |
| `0 0 * * 7`           | at midnight on Sundays |
