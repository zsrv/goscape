# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Context

This is a Go rewrite of the TypeScript RuneScape server at `/home/owner/Code/github.com/LostCityRS/Engine-TS`. It is designed to communicate with the Java RuneScape client at `/home/owner/Code/github.com/LostCityRS/Client-Java`.

## Commands

```bash
# Run (requires config.yaml)
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml

# Build binary
CGO_ENABLED=0 go build -trimpath -o /go/bin/goscape ./cmd/goscape

# Run all tests
go test ./...

# Run a single test
go test ./pkg/io/packet/... -run TestName

# Run with race detector
go test -race ./...

# Build container image
make build-image
```

## Configuration

Config follows a layered precedence: **defaults → config file → env vars → CLI flags**.

The `--target` flag (or `target:` in config.yaml) selects which modules to run:
- `ondemand` — HTTP OnDemand server only
- `world` — TCP game server only
- `login` — gRPC login service only
- `friends` — friends server only
- `all` (default) — all of the above

Verify a config file without starting: `--config.verify`. Expand env vars in config: `--config.expand-env`.

## Architecture

### Service Lifecycle (dskit)

Everything in `pkg/dskit/` is a port of [Grafana's dskit](https://github.com/grafana/dskit). Services follow a strict state machine: `New → Starting → Running → Stopping → Terminated/Failed`. The key types:

- `services.Service` — the interface every long-running component implements
- `services.BasicService` — `New(startingFn, runFn, stoppingFn)` — the standard concrete impl
- `services.Manager` — starts/stops a group of services and notifies via `ManagerListener`
- `services.FailureWatcher` — wraps a Manager and surfaces any service failure as a channel event

### Module System

`pkg/dskit/modules.Manager` resolves a dependency graph of named modules and initialises them in topological order. Modules are registered in `cmd/goscape/app/modules.go`:

```
common (invisible)
  ├── ondemand →  HTTP OnDemand server (dskit server + ondemand.OnDemand)
  ├── friends  →  friends server (SQLite)
  ├── login    →  gRPC login service (SQLite)
  └── world    →  TCP game server (world.Server)

all (composite target)
  ├── ondemand
  ├── friends
  ├── login
  └── world
```

Adding a new module: register it in `modules.go`, add dependencies, and add its config to `cmd/goscape/app/config.go`.

### Module Packages

Each feature module lives under `modules/<name>/` and contains:
- `config.go` — `Config` struct with `RegisterFlagsAndApplyDefaults` and `Validate`
- `<name>.go` — the top-level struct, `New(cfg, logger)`, and a `NewXxxService(...)` factory that wraps the server in a `services.BasicService`
- `server.go` — the network listener (TCP for world, HTTP via dskit server for ondemand)

### Networking

**OnDemand module** (`modules/ondemand/`): uses `pkg/dskit/server` which wraps `net/http`. Handlers are registered on `server.HTTP` (a `*http.ServeMux`).

**World module** (`modules/world/`): raw TCP server. `server.go` runs `net.Listen` → `Accept` loop → per-connection goroutine. Each connection goes through a `client` state machine starting at `ClientStateLogin`. ISAAC cipher streams are established after the login handshake.

### Binary I/O

`pkg/io/packet/` contains the core RS2 binary packet buffer:
- `Packet` — wraps `bytes.Buffer` with RS2-specific read/write methods (`G1`/`P1`, `G4`/`P4`, `GJStrLF`/`PJStrLF`, `RSADec`/`RSAEnc`, etc.)
- `PacketBit` — bit-level reader/writer

`pkg/io/protocol/` defines `Operation` (opcode + payload size constant) and `CheckPacketLength` for handling partial TCP reads. Dynamic packet sizes use `-1` (1-byte length prefix) or `-2` (2-byte length prefix).

`pkg/io/protocol/login/req/` and `resp/` implement `encoding.BinaryMarshaler` / `encoding.BinaryUnmarshaler` for the RS2 login protocol packets.

`pkg/io/isaac/` — ISAAC PRNG used for packet encryption after login.

### Signal Handling

`pkg/dskit/signals.Handler` intercepts OS signals. When embedded inside a module, `DisableSignalHandling` replaces it with a no-op channel — the `App` root owns the single real signal handler and calls `sm.StopAsync()` on the `services.Manager`.
