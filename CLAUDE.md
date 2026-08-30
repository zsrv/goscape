# CLAUDE.md

## Project Context

This is a Go rewrite of the TypeScript RuneScape server `LostCityRS/Engine-TS`. It is designed to communicate with the Java RuneScape client `LostCityRS/Client-Java`.

## Commands

```bash
# Run (requires a config file; see examples/)
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml

# Build binary
CGO_ENABLED=0 go build -trimpath -o /go/bin/goscape ./cmd/goscape

# Run all tests
go test ./...

# Run a single test
go test ./pkg/io/packet/... -run TestName

# Run with race detector
go test -race ./...

# Build both binaries: goscape (server) + goscape-cli (cache/RuneScript tooling)
make all

# Pack the game cache from CACHE_SRC_DIR into CACHE_OUT_DIR (uses goscape-cli)
make pack

# Build container images (goscape + goscape-cli)
make images
```

`cmd/goscape-cli/` is the offline tooling binary. Subcommands: `pack`
(game cache), `compile` (RuneScript), `rsa` (login key gen/info), `worldmap`,
`jag`. Run `goscape-cli <cmd> --help` for flags.

## Configuration

Config follows a layered precedence: **defaults → config file → env vars → CLI flags**.

Example configs live in `examples/`: `examples/bundled/goscape.yaml` is a minimal
"run everything" preset; `examples/full-config-reference.yaml` documents **every**
option at its default (copy only what you override). There is no default config
path — `--config.file` is always required. Decoding is strict: an unknown key is a
fatal boot error.

The `--target` flag (or `target:` in the config file) selects which modules to run:
- `ondemand` — HTTP OnDemand server only
- `world` — TCP game server only
- `login` — gRPC login service only
- `friends` — friends server only
- `account` — portal (SSR web app) + AccountService gRPC only
- `hiscore` — read-only hiscores JSON API only
- `all` (default) — all of the above

Verify a config file without starting: `--config.verify=true`. Expand env vars in config: `--config.expand-env=true`. (Both are value flags and require the `=true`; the bare form errors with `flag needs an argument`.)

The `database:` section selects the central database backend (`sqlite` default or `postgres` via `database.backend`; postgres enables running login and friends on different hosts against one network central DB). Every DB-using module — `login`, `friends`, `account`, `hiscore` — is an independent client of that one DB with its own pool; the invisible `database` module is only the migration anchor.

`account` and `hiscore` are goscape extensions with no Engine-TS counterpart (outside the TS fidelity ledger). Both default to `enable: false`; `account` additionally requires `public_url` when enabled, and game login only routes through it when `login.auth_mode: account` (plus `login.account_grpc_address`) is set — the default `local` mode is unchanged.

## Architecture

### Service Lifecycle (dskit)

Everything in `pkg/dskit/` is a port of [Grafana's dskit](https://github.com/grafana/dskit). Services follow a strict state machine: `New → Starting → Running → Stopping → Terminated/Failed`. The key types:

- `services.Service` — the interface every long-running component implements
- `services.BasicService` — `New(startingFn, runFn, stoppingFn)` — the standard concrete impl
- `services.Manager` — starts/stops a group of services and notifies via `ManagerListener`
- `services.FailureWatcher` — wraps a Manager and surfaces any service failure as a channel event

### Module System

`pkg/dskit/modules.Manager` resolves a dependency graph of named modules and initialises them in topological order. Modules and their dependencies (from `cmd/goscape/app/modules.go`, where `X → Y` means X depends on Y so Y starts first):

```
common    invisible; no deps — exists only to anchor the graph
database  invisible; central-DB migration anchor (pkg/gamedb)  → common
friends   friends server                                       → common, database
login     gRPC login service                                   → common, database
world     TCP game server (world.Server)                       → common, login, friends
ondemand  HTTP OnDemand server (dskit server + OnDemand)       → common, world
account   portal + AccountService gRPC                         → common, database
hiscore   read-only hiscores JSON API (dskit server)           → common, database
all       composite "run everything" target                    → ondemand, friends, login, world, account, hiscore
```

Adding a new module: register it in `modules.go`, wire its dependencies, and add its config to `cmd/goscape/app/config.go`.

### Module Packages

Each feature module lives under `modules/<name>/` and contains:
- `config.go` — `Config` struct with `RegisterFlagsAndApplyDefaults` and `Validate`
- `<name>.go` — the top-level struct, `New(cfg, logger)`, and a `NewXxxService(...)` factory that wraps the server in a `services.BasicService`
- `server.go` — the network listener (TCP for world, HTTP via dskit server for ondemand)

### Networking

**OnDemand module** (`modules/ondemand/`): uses `pkg/dskit/server` which wraps `net/http`. Handlers are registered on `server.HTTP` (a `*http.ServeMux`).

**World module** (`modules/world/`): raw TCP server. `server.go` runs `net.Listen` → `Accept` loop → per-connection goroutine. Each connection goes through a `client` state machine starting at `ClientStateLogin`. ISAAC cipher streams are established after the login handshake.

**Hiscore module** (`modules/hiscore/`): like ondemand, an HTTP surface on `pkg/dskit/server` (the caller in `modules.go` owns the `*server.Server`, which supplies request logging, timeouts and source-IP extraction). Read-only over the `hiscore`/`hiscore_large` tables that `modules/login` writes on logout; see `docs/superpowers/specs/2026-08-19-hiscore-api-design.md`.

**Login / friends / account modules**: gRPC listeners. `account` runs two — the server-rendered portal over HTTP and `AccountService` over gRPC.

### Binary I/O

`pkg/io/packet/` contains the core RS2 binary packet buffer:
- `Packet` — wraps `bytes.Buffer` with RS2-specific read/write methods (`G1`/`P1`, `G4`/`P4`, `GJStrLF`/`PJStrLF`, `RSADec`/`RSAEnc`, etc.)
- Bit-level I/O on the same `Packet` — `AccessBits()`/`AccessBytes()` switch modes; `GBit`/`PBit` read/write n bits

`pkg/io/protocol/` defines `Operation` (opcode + payload size constant) and `CheckPacketLength` for handling partial TCP reads. Dynamic packet sizes use `-1` (1-byte length prefix) or `-2` (2-byte length prefix).

`pkg/io/protocol/login/req/` and `resp/` implement `encoding.BinaryMarshaler` / `encoding.BinaryUnmarshaler` for the RS2 login protocol packets.

`pkg/io/isaac/` — ISAAC PRNG used for packet encryption after login.

### Signal Handling

`pkg/dskit/signals.Handler` intercepts OS signals. When embedded inside a module, `DisableSignalHandling` replaces it with a no-op channel — the `App` root owns the single real signal handler and calls `sm.StopAsync()` on the `services.Manager`.
