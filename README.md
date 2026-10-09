# cronx

cronx is a small, local job scheduler for Unix-like systems, written in Go and inspired by `cron`.

It runs scheduled jobs defined in a single TOML configuration file, executes each job according to its own cron schedule, and keeps a persistent record of every execution in a local SQLite database. Job output is written to a shared log file.

cronx is designed to be **simple, self-contained, and easy to inspect**:

* one executable
* one TOML configuration file
* one local SQLite database
* one shared log file
* no daemon manager
* no database server
* no network service

cronx runs in the foreground and requires no external infrastructure. It is intended for users who need reliable scheduled job execution without introducing the complexity of a larger scheduling system.


## Status

**Released version: 0.4.0**.

This is an early release. Version 0.4.0 covers the core of a scheduler and is
usable for small, personal or single-machine set-ups: schedules, direct
shell-free execution, retries, timeouts, overlap policies, execution history and
logs. It adds `cronx logs`, the rotation of the run log, the pruning of the
history, and the reload of the configuration on `SIGHUP`, to the feature set of
0.3.0; see the [roadmap](docs/roadmap.md) for what comes next. The behaviour is
not frozen yet, so details may still change between releases.

## Features

Implemented:

- **Cron-style schedules.** Five-field expressions with ranges, lists, steps and
  month or day-of-week names, for example `0 9-17 * * mon-fri`, and the
  descriptors of a fixed time, from `@yearly` to `@hourly`.
- **The zone you schedule in.** `[scheduler].timezone` chooses the clock
  activation times are computed on: an IANA name such as `Europe/Rome`, or
  `Local` for the zone of the machine.
- **One configuration file.** Every job is described in the same TOML file, which
  is validated before anything runs and whose problems are all reported at once.
- **A configuration to start from.** `cronx init` writes a file with the fields
  one job needs set and every other option commented out and explained, so the
  file doubles as a reference. The directory and the file get the permissions of
  everything else cronx writes, and a configuration that is already there is
  never replaced.
- **No shell.** A job is started directly as an executable plus a list of
  arguments; the path must be absolute and the arguments are passed verbatim.
- **Per-job execution policy.** `args`, `working_directory`, `env`, `timeout`,
  `grace_period`, `retry` and `overlap` (`skip`, `allow` or `queue`).
- **Retries.** `retry = N` allows up to `N` further attempts, and every attempt is
  recorded separately in the history. The output of all of them goes to the one
  run log, each line stamped with its attempt's run id.
- **Timeouts.** A job that exceeds its `timeout` is stopped together with the
  processes it started, first with `SIGTERM` and, once its `grace_period` is over
  (ten seconds by default), with `SIGKILL`.
- **Overlap policies.** A trigger that arrives while the job is still running is
  skipped, run anyway, or queued, according to the job's policy. The policy is
  applied to the trigger when it arrives, before the job waits for a free slot,
  so it holds whatever `max_parallel_jobs` is.
- **Parallelism limit.** `max_parallel_jobs` bounds how many jobs run at the same
  time.
- **Execution history.** One row per attempt in a local SQLite database, with its
  status, start time, duration, exit code and the log it wrote to, readable
  through `cronx history` and `cronx status`.
- **A history that does not grow without bound.** `[storage].max_runs` keeps the
  newest runs of every job and deletes the rest, when the scheduler starts and
  whenever a job runs.
- **Logs.** Every run appends to one run log, whose lines carry the time, the
  job, the run and the pid, plus the scheduler's own log file; both readable
  only by their owner.
- **A log that does not grow without bound.** `[logging].max_size` rotates the
  run log once it would pass a size, and `[logging].max_backups` says how many
  rotated files are kept. Without a size the log simply grows.
- **Reading the log back.** `cronx logs [job]` prints the run log, so the output
  of a run can be read without knowing where the file is: a job name selects one
  job, `--since` leaves out what is older, and `--follow` keeps printing as the
  log grows.
- **Restart behaviour.** Runs left in progress by a process that died are closed
  on the next start, and activations missed while cronx was not running are not
  replayed.
- **One scheduler per state.** `cronx run` holds a lease on the state, so a
  second one refuses to start instead of scheduling the same jobs again;
  `cronx status` says who is running and until when.
- **Command line.** `init`, `validate`, `list`, `status`, `history`, `logs`,
  `run-once`, `run` and `version`, with a `--config` flag; `run-once` exits
  non-zero when the job fails, so it can be used from scripts.
- **A change without a restart.** A running scheduler reads its configuration
  again when it is sent `SIGHUP`: a job that was added, removed or rescheduled
  takes effect at once, and a file that cannot be used is refused while the
  scheduler keeps running.

Not implemented yet:

- **Job dependencies** are not supported.

## Build

cronx is an ordinary Go program. Building it requires **Go 1.26 or newer** (the
version declared in [`go.mod`](go.mod)) and nothing else: there is no C
dependency, because the SQLite driver is pure Go.

From a checkout of this repository:

```sh
go build -o bin/cronx ./cmd/cronx
```

The executable is written to `bin/cronx`. Other ways to build it:

```sh
go build ./...          # compiles every package and writes no binary
go build ./cmd/cronx    # writes ./cronx in the current directory
go run ./cmd/cronx --help
```

The repository also ships a `Makefile` whose targets only call `go`:

```sh
make build      # go build -o bin/cronx ./cmd/cronx
make test       # the fast suite: go test -short ./..., in seconds
make test-full  # every test, including the two that wait for a real minute boundary
make lint       # go vet and a gofmt check
make ci         # lint, the full suite, build
```

`make test` is the everyday command, and takes about ten seconds: it leaves out
the two end-to-end tests that wait for a real minute boundary. What each suite
covers is listed in [`docs/testing.md`](docs/testing.md).

The resulting binary is self-contained. It needs no shell, no Nix and no
installed library at runtime.

## Development with Nix

Nix is **optional**. The application does not require it, neither to build nor to
run; the [flake](flake.nix) exists to make the development environment
reproducible.

```sh
nix develop
```

The shell provides the pinned Go 1.26 toolchain, Git and the `sqlite3` command
line, which is handy for looking at `~/.cronx/state.db`. Everything described
above works unchanged inside it:

```sh
nix develop -c make ci      # vet, formatting check, all tests, build
nix develop -c make build   # build ./bin/cronx inside the shell
```

The flake also packages the binary, and building the package runs the test suite
in its check phase — the full one, including the minute waits, so it takes a
minute or two:

```sh
nix build .#default         # result/bin/cronx
```

`x86_64-linux`, `aarch64-linux`, `x86_64-darwin` and `aarch64-darwin` are
supported.

## Usage

### Configuration

cronx reads one TOML file. Its path is resolved in this order:

1. the `--config` flag,
2. the `CRONX_CONFIG` environment variable,
3. `~/.cronx/config.toml` (the default).

The file is only ever read: cronx never rewrites it. The runtime state and the
logs live under `~/.cronx/` unless the file moves them with `[logging].path` and
`[storage].path`.

```toml
[scheduler]
max_parallel_jobs = 2

[logging]
level = "info"
path = "/var/log/cronx/runs.log"  # optional: where every job's output is written

[storage]
path = "/var/lib/cronx/state.db"  # optional: the status database

[jobs.backup]
schedule = "0 3 * * *"            # a cron expression, or @daily for midnight
command = "/usr/local/bin/backup" # required, absolute path
args = ["--incremental"]          # passed verbatim, never through a shell
timeout = "30m"
grace_period = "15s"              # time to stop after SIGTERM (default: 10s)
retry = 1                         # one further attempt if it fails
overlap = "skip"                  # skip | allow | queue
working_directory = "/var/lib/backup"
env = { TIER = "gold" }           # jobs do not inherit the environment
```

Only `schedule` and `command` are required; every other field has a default.

> See [Configuration](docs/configuration.md) for the complete reference and
> [Scheduling](docs/scheduling.md) for the expression syntax and the overlap,
> retry and parallelism policies.

### Writing a configuration to start from

```sh
cronx init
```

It writes `~/.cronx/config.toml`, or the file `--config` or `CRONX_CONFIG`
names, creating the directory it lives in. One job is set, and every other option
is written next to it commented out, with what it does and what happens without
it, so that the file can be read as its own reference. A configuration that is
already there is never replaced: the command stops and names the file, and
`cronx init --force` is what replaces it.

### Running the scheduler

```sh
cronx run
```

The scheduler stays in the foreground, writes to the standard error and to
`~/.cronx/logs/cronx.log`, and stops on `Ctrl-C` (`SIGINT`) or `SIGTERM`: the
jobs still running are stopped with it, and it waits for them before exiting. To
keep it running permanently, use the service manager of your operating system.
A change to the configuration does not need a restart: send it `SIGHUP` and it
reads the file again, applying the jobs the file holds; see
[Command line](docs/cli.md).

### Inspecting jobs, history and state

```sh
cronx list                     # configured jobs and when each runs next
cronx status                   # the last outcome of every job
cronx history                  # the last 20 runs, newest first
cronx history backup --limit 5 # only one job, fewer rows
cronx logs backup              # what the runs of one job printed
cronx logs --follow            # the output of every job, as it arrives
```

### Running a job by hand

```sh
cronx run-once backup
```

The run is recorded in the history and its output goes to the run log, exactly
as for a scheduled run, and the policies of the job still apply. The command
exits with status 1 when the job did not succeed.

### Validating a configuration

```sh
cronx validate
cronx validate --config examples/configs/maintenance.toml
```

It prints whether the file is valid or every problem it found. Nothing is
started, and no state is written.

### Checking the version

```sh
cronx version
```

It prints the version of cronx and reads no configuration.

### Logs

| Path                       | Contents |
| -------------------------- | -------- |
| `~/.cronx/logs/cronx.log`  | What the scheduler itself did (written by `cronx run`). |
| `~/.cronx/logs/runs.log`   | The output of every run of every job, one file, each line stamped with the time, the job, the run id and the pid. |
| `~/.cronx/state.db`        | The history and the runtime state, as SQLite. |

The run log and the status database can be moved with `[logging].path` and
`[storage].path`; see [Configuration](docs/configuration.md). `cronx logs` reads
the run log back — `cronx logs backup`, `cronx logs --since 1h --follow` — and
the other files can be read directly, with `cat` or
`sqlite3 ~/.cronx/state.db`. The run log is rotated once it would pass
`[logging].max_size`, keeping the last `[logging].max_backups` files as
`runs.log.1`, `runs.log.2` and so on; nothing is rotated unless a size is
configured. Only their owner can read the files.

> Every command, its output and its exit status are described in
> [Command line](docs/cli.md).

## Documentation

| Document | Contents |
| -------- | -------- |
| [Configuration](docs/configuration.md) | Every field of the configuration file, its default and the validation rules. |
| [Command line](docs/cli.md) | Every command, its output, its exit status and the log files. |
| [Scheduling](docs/scheduling.md) | Cron expression syntax, the day-of-month and day-of-week rules, and the overlap, retry and parallelism policies. |
| [Security](docs/security.md) | The execution model, the environment of a job, signals and process groups, and what cronx does not do. |
| [Persistence](docs/persistence.md) | The database, the run statuses, migrations and restart behaviour. |
| [Example configurations](docs/examples.md) | The files in [`examples/configs/`](examples/configs/) explained one by one. |
| [Testing](docs/testing.md) | How to run the tests, what they verify, and what they found. |
| [Architecture](docs/design/0001-architecture.md) | The design decisions behind the implementation. |
| [Roadmap](docs/roadmap.md) | What is released, what each next version contains, and how a version number is chosen. |
| [Changelog](CHANGELOG.md) | What changed in each release, and what is on `master` but not released yet. |

## Quick Start

The commands below use only programs that exist on a normal system, so they can
be followed as they are.

**1. Build the binary**

```sh
go build -o bin/cronx ./cmd/cronx
```

**2. Create a configuration with one job**

```console
$ ./bin/cronx init
/home/user/.cronx/config.toml written
```

The file it writes defines one job named `hello`, which runs `/bin/echo` every
five minutes, and documents every other option as a comment. Add your own jobs to
it, with a `command` that exists on your machine.

**3. Check the file before trusting it**

```console
$ ./bin/cronx validate
/home/user/.cronx/config.toml is valid
```

**4. See the job and when it runs next**

```console
$ ./bin/cronx list
JOB    SCHEDULE     NEXT
hello  */5 * * * *  2026-10-04 22:20:00 CEST
```

**5. Run the job once, now**

```console
$ ./bin/cronx run-once hello
job "hello" finished with status "succeeded"
```

**6. Start the scheduler and leave it running**

```sh
./bin/cronx run
```

**7. In another terminal, inspect the outcome and the output of the run**

```console
$ ./bin/cronx status
JOB    LAST STATUS  LAST RUN                  IN PROGRESS
hello  succeeded    2026-10-04 22:19:11 CEST  no

$ ./bin/cronx history
ID  JOB    ATTEMPT  STATUS     STARTED                   DURATION  EXIT
1   hello  1        succeeded  2026-10-04 22:19:11 CEST  2ms       0

$ cronx logs
2026-10-04T22:19:11Z hello id=1 pid=4213 hello from cronx
```

Press `Ctrl-C` in the terminal of `cronx run` to stop the scheduler.

## Security

What cronx does:

- **Commands are executed directly.** A job is started through the process API of
  the operating system, never through a shell.
- **`command` and `args` are passed without invoking a shell.** The executable
  path and each argument are separate values, so `;`, `|`, `$` and `*` remain
  literal arguments and are neither expanded nor interpreted.
- **`command` must be an absolute path**, and it is never looked up in `PATH`.
- **The configuration is validated** before anything runs: unknown keys, invalid
  schedules, relative commands and out-of-range values are rejected.
- **The environment is not inherited.** A job receives only
  `PATH=/usr/local/bin:/usr/bin:/bin` plus the variables of its own `env` table,
  and its standard input is not connected to anything.
- **A job runs in a process group of its own**, so a timeout or a shutdown
  signals the whole group instead of leaving the processes the job started
  behind.
- **State and logs are local and private.** They live under `~/.cronx/`, with
  directories restricted to their owner (`0700`) and files to their owner
  (`0600`).
- **Jobs are not sandboxed.** A job has the same identity and the same filesystem
  access as the scheduler; cronx does not drop privileges, and it does not use
  cgroups, namespaces or resource limits. Run the scheduler as a dedicated,
  unprivileged user, and use an external tool (a container, `systemd`, `bwrap`,
  `firejail`) when a job needs stronger isolation.

The complete security model is in [docs/security.md](docs/security.md); there is
no separate `SECURITY.md`.

## License

cronx is released under the **MIT License**. See [LICENSE](LICENSE).
