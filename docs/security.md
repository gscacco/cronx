# Security

cronx runs programs on your behalf, so its execution model is deliberately
small and predictable. Everything below is a guarantee the implementation is
tested against.

## No shell, ever

A job is started directly, through the operating system's process API, with the
executable path and each argument as separate values. cronx never builds a
command line and never hands it to a shell.

This means that shell metacharacters given as arguments are **literal
arguments**:

```toml
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
args = ["--exclude", "; rm -rf /", "$(whoami)", "*"]
```

`backup` receives exactly four arguments. Nothing is expanded, no command
substitution happens, no glob is expanded and nothing is executed because of the
metacharacters. The test suite includes explicit tests for this behaviour.

## Commands are absolute paths

`command` must be an absolute path. cronx never looks a command up through
`PATH`, so a job can not accidentally run a different program because of how the
scheduler happened to be started. A relative command is rejected when the
configuration is validated and again when the job is about to run.

## The environment is not inherited

A job does **not** inherit the environment of the scheduler. It receives a
minimal, documented environment:

```
PATH=/usr/local/bin:/usr/bin:/bin
```

`PATH` is provided only so that the programs a job starts can find their own
helpers; cronx itself never uses it to locate the job's command. A job that
needs anything else — `HOME`, `TZ`, an API token — declares it explicitly:

```toml
[jobs.backup]
env = { HOME = "/home/backup", TIER = "gold" }
```

The practical effect is that a secret exported in the scheduler's shell does not
silently leak into every job.

## Standard input is not connected

A job's standard input is never connected to the scheduler's. A job that reads
from standard input simply sees end of input instead of consuming the
scheduler's input.

## Timeouts, signals and process groups

A job runs in a **process group of its own**. When a job exceeds its `timeout`,
cronx stops the whole group:

1. the group is asked to terminate with `SIGTERM`;
2. if it is still running after the grace period, it is killed with `SIGKILL`.

Because the whole group is signalled, the processes a job started are stopped
together with it instead of being left behind. The grace period defaults to ten
seconds and is configured per job with `grace_period`
([configuration.md](configuration.md)).

## What cronx does not do

Being honest about the boundaries matters as much as the guarantees:

- cronx does **not** sandbox jobs. A job runs with the same operating-system
  identity and the same filesystem access as the scheduler.
- cronx does **not** use cgroups, namespaces, seccomp or resource limits.
- cronx does **not** run jobs as a different user.

If a job needs stronger isolation, run it under an external tool (a container,
`systemd`, `bwrap`, `firejail`) and have cronx start that tool.

## Recommendations

- Run the scheduler as a dedicated, unprivileged user.
- Treat the configuration file as sensitive: it contains the commands that will
  be executed. Keep it readable only by its owner.
- Prefer passing secrets through a job's `env` from a file the job reads itself,
  rather than exporting them in the scheduler's environment.
