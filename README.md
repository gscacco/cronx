# cronx

A small, reliable, portable and secure local job scheduler inspired by Unix
`cron`.

cronx preserves the simplicity of cron while adding a few carefully selected
capabilities: reliable job execution, execution history, retries, timeouts and,
in a later development step, job dependencies.

## Status

Early development. The project is built incrementally, one small and separately
verifiable step at a time. The architecture and the approved decisions are
recorded in [`docs/design/`](docs/design/).

## Design principles

- Keep the system small while making the core behaviour reliable.
- cronx never runs a job through a shell: the executable and its arguments are
  always kept as separate values, so shell metacharacters remain literal.
- TOML describes the desired configuration, SQLite stores the runtime state and
  the execution history, and the filesystem stores the logs.
- Very few dependencies; the Go standard library is preferred.

## Requirements

- [Nix](https://nixos.org/) with flakes enabled, for the reproducible
  development environment.
- The produced binary is a normal, portable Go executable and does not require
  Nix at runtime.

## Development

Enter the reproducible development shell (it provides the pinned Go toolchain,
Git and the SQLite CLI):

```sh
nix develop
```

Common tasks, all runnable inside the development shell:

```sh
make build   # build ./bin/cronx
make test    # run the test suite
make vet     # run go vet
make fmt     # format Go sources
make lint    # go vet + formatting check
make ci      # what continuous integration runs
```

`make test` runs the unit tests and the end-to-end tests under
[`test/integration/`](test/integration/), which drive the compiled binary as a
real process. They are described in [docs/testing.md](docs/testing.md).

## Quick start

Write `~/.cronx/config.toml`:

```toml
[scheduler]
timezone = "Europe/Rome"
max_parallel_jobs = 2

[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
args = ["--incremental"]
timeout = "30m"
retry = 1

[jobs.report]
schedule = "*/15 * * * *"
command = "/usr/local/bin/report"
```

Then:

```sh
cronx validate          # check the file before trusting it
cronx list              # see when each job runs next
cronx run-once backup   # try a job immediately
cronx run               # leave the scheduler running
```

`cronx run` stays in the foreground and writes to `~/.cronx/logs/cronx.log`.
The output of each run is kept in `~/.cronx/logs/<job>/<run id>.log`, and the
history is in `~/.cronx/state.db`.

## Usage

The configuration path is resolved from `--config`, then `CRONX_CONFIG`, then
`~/.cronx/config.toml`.

```sh
cronx validate                   # check the configuration file
cronx list                       # jobs, their schedule and when they run next
cronx status                     # the last outcome of every job
cronx history backup --limit 10  # the execution history
cronx run-once backup            # run a job right now
cronx run                        # run the scheduler in the foreground
```

Every command is described in [docs/cli.md](docs/cli.md), the configuration
format in [docs/configuration.md](docs/configuration.md), the scheduling
semantics in [docs/scheduling.md](docs/scheduling.md), the execution and
security model in [docs/security.md](docs/security.md) and the database in
[docs/persistence.md](docs/persistence.md). How the tests verify all of that is
in [docs/testing.md](docs/testing.md).

Complete configuration files to copy and try are kept in
[`examples/configs/`](examples/configs/) and described one by one in
[docs/examples.md](docs/examples.md).

## Repository layout

```
cmd/cronx/          command-line entry point
internal/config/    configuration loading and validation
internal/job/       job domain model
internal/schedule/  cron expression parsing and next-activation computation
internal/clock/     injectable time source
internal/runner/    secure, shell-free job execution
internal/store/     SQLite persistence: history and runtime state
internal/logx/      log layout and per-run log files
internal/scheduler/ the scheduling loop and the execution policies
internal/cli/       command-line commands
test/integration/   end-to-end tests that drive the compiled binary
examples/           configuration files kept as a reference
docs/               design and reference documentation
```

## License

MIT. See [LICENSE](LICENSE).
