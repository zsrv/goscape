# Quick start

This page gets a single-process goscape server running on your own machine in a
few minutes, using the bundled example configuration. If you have not yet read
what the server is and how its parts fit together, start with the
[Administrator's Guide overview](index.md); for production layouts see
[Deployment scenarios](deployment.md), and for how configuration itself works see
[Configuration](configuration.md).

## Before you start

You need three things:

- **Go 1.26 or newer.** goscape is compiled from source with the Go toolchain.
  The minimum version is the one named in the project's `go.mod` (`go 1.26`); an
  older toolchain refuses to build the module.
- **A packed game cache.** The `ondemand` module serves the game cache to
  connecting clients, and the `world` module reads map data from it. Both read
  it from `./data/pack` by default. Produce it with the cache-packing tooling
  (`make pack`) before players connect — the [Operations runbook](operations.md)
  covers packing in detail.
- **(Optional) `grpcurl`.** The login smoke-test script below uses it to trigger
  a single login without a game client.

## Start the bundled server

The repository ships a minimal "run everything" preset at
`examples/bundled/goscape.yaml`. From the repository root, run:

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml
```

A few notes on that command:

- `--config.file` names the configuration file to load. **There is no default
  config path — this flag is always required.**
- `CGO_ENABLED=0` produces a pure-Go build with no C dependencies, and
  `-trimpath` removes local filesystem paths from the binary. Both are optional
  for a quick local run but match how release binaries are built.

The bundled preset is self-contained: the `login` and `friends` modules persist
to a local SQLite database file, so **no external services (no separate database
server) are required** to run it.

## What the bundled preset starts

The bundled file sets `target: all`, so a single process runs every module.
It opens the following network listeners:

| Port | Module | Protocol | What it is for |
|------|--------|----------|----------------|
| 8080 | `ondemand` | HTTP | Serves the game cache to connecting clients. |
| 43594 | `world` | TCP | Carries live gameplay; this is the port a game client connects to. |
| 2004 | `login` | gRPC | Authenticates players; consumed by the `world` module. |
| 2005 | `friends` | gRPC | Friends list and private messaging; consumed by the `world` module. |

Only the OnDemand port (8080) and the login port (2004) are set explicitly in the
bundled file; the world port (43594, the classic RuneScape game port) and the
friends port (2005) come from built-in defaults because the bundled file does not
override them. Every port and bind address is configurable — see
[Configuration](configuration.md) and the [Config reference](config-reference.md).

On first run the server creates its state under a local `data` directory:
`data/goscape.db` (the SQLite database) and `data/players/` (player save files).
The overview page describes the [data layout](index.md#data-layout) in full.

## Verify a config before booting

You can validate a configuration file — check that every key is understood and
every value is legal — without starting the server:

```bash
go run ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml \
  --config.verify=true
```

The process loads and validates the config, prints any problems, and exits: `0`
if the config is valid, non-zero if it is not. Nothing binds a port.

!!! warning "`--config.verify` is a value flag: the `=true` is required"
    Write `--config.verify=true`, not a bare `--config.verify`. The bare form is
    rejected before the server starts with:

    ```text
    flag needs an argument: -config.verify
    ```

    The same rule applies to `--config.expand-env=true`. See
    [Configuration](configuration.md#meta-flags-are-value-flags) for the full set
    of meta-flags.

## Connecting a client

The Java game client reaches a running server through two of the
listeners above: it fetches the game cache and the world bootstrap from the
OnDemand HTTP server (port 8080), then connects to the world's TCP port (43594)
for gameplay. Point the client's host at the machine running goscape. With the
bundled preset every listener binds to `127.0.0.1`, so the client must run on the
same machine; change the listen addresses (see the
[Config reference](config-reference.md)) to accept connections from elsewhere.

To confirm the login service is answering without a full client, run the bundled
smoke-test script, which sends one login request through `grpcurl`:

```bash
bash examples/bundled/scripts/fake-login.sh
```

It targets `localhost:2004` (the login gRPC port) by default and, with the
bundled `auto_register: true` setting, creates the demo account on first use.

## Stopping the server

Press `Ctrl-C` (or send `SIGTERM`). A single signal handler at the top of the
process asks every module to shut down in dependency order; there is no partial,
half-running state left behind. The [service lifecycle](index.md#service-lifecycle)
section of the overview explains what happens during shutdown.
