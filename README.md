# cronx

cronx is a small, local job scheduler for Unix-like systems, written in Go and
inspired by Unix `cron`. It reads a single TOML file describing the jobs to run,
starts each of them on its own cron schedule, and records what happened: every
run is written to a local SQLite database and its output to a log file. It is one
binary that runs in the foreground, with no daemon manager, no database server
and no network service.

## Status

**Version: 0.1.0**

This is an early release. Version 0.1.0 covers the core of a scheduler and is
usable for small, personal or single-machine set-ups: schedules, direct
shell-free execution, retries, timeouts, overlap policies, execution history and
logs. The behaviour is not frozen yet, so details may still change between
releases.

## Features

Implemented in 0.1.0:

- **Cron-style schedules.** Five-field expressions with ranges, lists, steps and
  month or day-of-week names, for example `0 9-17 * * mon-fri`.
- **One configuration file.** Every job is described in the same TOML file, which
  is validated before anything runs and whose problems are all reported at once.
- **No shell.** A job is started directly as an executable plus a list of
  arguments; the path must be absolute and the arguments are passed verbatim.
- **Per-job execution policy.** `args`, `working_directory`, `env`, `timeout`,
  `retry` and `overlap` (`skip`, `allow` or `queue`).
- **Retries.** `retry = N` allows up to `N` further attempts, and every attempt is
  recorded separately with its own log file.
- **Timeouts.** A job that exceeds its `timeout` is stopped together with the
  processes it started, first with `SIGTERM` and then with `SIGKILL`.
- **Overlap policies.** A trigger that arrives while the job is still running is
  skipped, run anyway, or queued, according to the job's policy.
- **Parallelism limit.** `max_parallel_jobs` bounds how many jobs run at the same
  time.
- **Execution history.** One row per attempt in a local SQLite database, with its
  status, start time, duration, exit code and log path, readable through
  `cronx history` and `cronx status`.
- **Logs.** One file per run holding the output of the job, plus the scheduler's
  own log file; both readable only by their owner.
- **Restart behaviour.** Runs left in progress by a process that died are closed
  on the next start, and activations missed while cronx was not running are not
  replayed.
- **Command line.** `validate`, `list`, `status`, `history`, `run-once` and `run`,
  with a `--config` flag; `run-once` exits non-zero when the job fails, so it can
  be used from scripts.

Not implemented in 0.1.0:

- **Job dependencies** are not supported.
- **`[scheduler].timezone`** is accepted and kept, but activation times are
  computed in the local timezone of the machine running cronx.
- **The grace period** between `SIGTERM` and `SIGKILL` is fixed at ten seconds; it
  cannot be set from the configuration file.
- **There is no lock on the state.** Start one scheduler per home directory;
  two processes would both schedule the same jobs.
- **`overlap = "skip"`** only takes effect when more than one job may run at a
  time (`max_parallel_jobs` of at least 2). With the default of 1, a trigger that
  arrives while the job is still running waits for a free slot and then runs.

The last four items are behaviours the test suite found, with the evidence, in
[docs/testing.md](docs/testing.md).

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
make build   # go build -o bin/cronx ./cmd/cronx
make test    # go test ./... — unit tests and the end-to-end suite
make lint    # go vet and a gofmt check
make ci      # lint, tests, build
```

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
in its check phase:

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
logs always live under `~/.cronx/`, whichever file is in use.

```toml
[scheduler]
max_parallel_jobs = 2

[logging]
level = "info"

[jobs.backup]
schedule = "0 3 * * *"            # five-field cron expression
command = "/usr/local/bin/backup" # required, absolute path
args = ["--incremental"]          # passed verbatim, never through a shell
timeout = "30m"
retry = 1                         # one further attempt if it fails
overlap = "skip"                  # skip | allow | queue
working_directory = "/var/lib/backup"
env = { TIER = "gold" }           # jobs do not inherit the environment
```

Only `schedule` and `command` are required; every other field has a default.

> See [Configuration](docs/configuration.md) for the complete reference and
> [Scheduling](docs/scheduling.md) for the expression syntax and the overlap,
> retry and parallelism policies.

### Running the scheduler

```sh
cronx run
```

The scheduler stays in the foreground, writes to the standard error and to
`~/.cronx/logs/cronx.log`, and stops on `Ctrl-C` (`SIGINT`) or `SIGTERM`: the
jobs still running are stopped with it, and it waits for them before exiting. To
keep it running permanently, use the service manager of your operating system.

### Inspecting jobs, history and state

```sh
cronx list                     # configured jobs and when each runs next
cronx status                   # the last outcome of every job
cronx history                  # the last 20 runs, newest first
cronx history backup --limit 5 # only one job, fewer rows
```

### Running a job by hand

```sh
cronx run-once backup
```

The run is recorded in the history and its output goes to the usual log file,
exactly as for a scheduled run, and the policies of the job still apply. The
command exits with status 1 when the job did not succeed.

### Validating a configuration

```sh
cronx validate
cronx validate --config examples/configs/maintenance.toml
```

It prints whether the file is valid or every problem it found. Nothing is
started, and no state is written.

### Logs

| Path                            | Contents |
| ------------------------------- | -------- |
| `~/.cronx/logs/cronx.log`       | What the scheduler itself did (written by `cronx run`). |
| `~/.cronx/logs/<job>/<id>.log`  | Everything one run wrote to its standard output and standard error; `<id>` is the `ID` column of `cronx history`. |
| `~/.cronx/state.db`             | The history and the runtime state, as SQLite. |

There is no `cronx logs` command: read the files directly, with `tail -f`,
`cat`, or `sqlite3 ~/.cronx/state.db`. The scheduler log is appended to, no log
is ever rotated automatically, and only their owner can read them.

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

## Quick Start

The commands below use only programs that exist on a normal system, so they can
be followed as they are.

**1. Build the binary**

```sh
go build -o bin/cronx ./cmd/cronx
```

**2. Create a configuration with one job**

```sh
cat > ~/.cronx/config.toml <<'EOF'
[jobs.hello]
schedule = "*/5 * * * *"
command = "/bin/echo"
args = ["hello from cronx"]
EOF
```

(create `~/.cronx` first if it does not exist yet: `mkdir -p ~/.cronx`).

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

$ cat ~/.cronx/logs/hello/1.log
hello from cronx
```

Press `Ctrl-C` in the terminal of `cronx run` to stop the scheduler.

## Security

What version 0.1.0 does:

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
