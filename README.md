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

## Repository layout

```
cmd/cronx/       command-line entry point
internal/        implementation packages (not importable by other modules)
docs/            design and reference documentation
```

## License

MIT. See [LICENSE](LICENSE).
