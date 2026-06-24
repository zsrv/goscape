# Logging Cleanup Design

**Date:** 2026-06-24
**Branch:** rev-274 (then ported to rev-254, rev-245.2, rev-244, rev-225)
**Scope:** Clean up the server's logging: introduce a finer `trace` level below `debug`, give every module an optional per-module level override, stamp a `component=` attribute on every line, and sweep all call sites against a written level contract (re-leveling noisy lines, removing stray debug output, and filling coverage gaps).

## Context

Logging is a **goscape-specific addition**, not part of the Engine-TS port — `cmd/goscape/app/app.go:34` even comments the global logger as "my addition". It is therefore **not bound by the faithful-translation policy**; we are free to design it for operability.

Current state:

- `pkg/util/log/log.go` wraps `log/slog` with `NewLogger(level, format, w)` → text (`slog.NewTextHandler`) or JSON handler, both with `AddSource: true`, everything written to **stdout**. This file is **byte-identical across all 5 rev branches** (`c7c375f6…`).
- **One global level.** `Config.LogLevel slog.Level` (`cmd/goscape/app/config.go:16`, flag `log.level`, default `info`) plus `Config.LogFormat`. The config struct's level/format lines are identical across all 5 branches.
- **Partial per-module override.** **ondemand** (`modules.go:41-44`, via `g.cfg.OnDemand.Server.LogLevel *slog.Level`) and **world** (`modules.go:143-146`, via `modules/world/config.go:15` `LogLevel *slog.Level`) already resolve an override and fall back to the global. **login** and **friends** do not — they create their loggers with the **global** level only.
- **`NewLogger` is shared with the CLI.** `pkg/util/log.NewLogger` is called by 5 `cmd/goscape-cli` tools (pack/unpack/compile/worldmap/smoke_pack), all passing `slog.Level`. Per the non-goals, CLI tooling is out of scope, so `NewLogger` keeps its `slog.Level` signature.
- **No component attribution.** Each module gets a fresh `*slog.Logger` in `modules.go` (lines 46/100/122/148) but none stamp which subsystem a line came from. The sole exception is `slogLoggerBridge`, which sets `component=logger_bridge` (`modules/world/logger_bridge.go:27`).
- **Call-site distribution** (server code, non-test): `warn` 108, `info` 51, `debug` 37, `error` 34. **158 of ~200 calls live in `modules/world`.**

### Known smells (found during exploration)

1. `cmd/goscape/main.go:42` — `fmt.Printf("%+v\n", config) // DEBUG` dumps the **entire config (including the RSA private-key path) to stdout on every boot**, regardless of level.
2. `modules/world/server.go:1033` — `Info("received data", "num_bytes", …)` fires **per packet, per connection** at `info`.
3. `modules/world/server.go:1034` — `Debug("received data payload", "data", …)` dumps **raw bytes** per packet.
4. `modules/world/server.go:1182` — `Info("partial packet data received, waiting for more", …)` fires per partial TCP read at `info`.
5. `modules/world/client.go:157` — `Debug("sent data", …, "data", …)` dumps **raw bytes** per outgoing packet.
6. `modules/world/server.go:1095` — `Info("unhandled client state", …)` is an unexpected condition logged as informational.
7. `modules/world/server.go:990` — `Info("connection closed", …)` is per-connection and floods `info` on a busy server.

## Goals

All four requested outcomes, via the mechanism below:

1. **Tame noise at the default level** — re-bucket per-packet/per-tick lines and remove stray debug output so `info` is quiet and useful.
2. **Per-subsystem granularity** — per-module level overrides plus a `component=` tag on every line for fine-grained grep/jq filtering.
3. **A finer level** — a named `trace` level below `debug` for the firehose output.
4. **Fill coverage gaps** — add logs where error paths currently surface nothing.

## Design decisions

- **Approach A (extend the current pattern), not a central registry.** Each module keeps its own `*slog.Logger`; per-module overrides reuse the exact mechanism ondemand already has; component granularity is delivered by a `component=` attribute (grep/jq-filterable) rather than a per-component level matcher. Rejected the central `logging.For("world.net")` registry as machinery the chosen scope does not need.
- **One `component=` attribute per line, dotted `<module>.<subsystem>`** — no separate redundant `module=` attribute. The prefix *is* the module.
- **`trace` is a named level**, not a re-bucket onto `debug` and not a second `trace2`. One new level keeps `debug` usable without over-engineering.
- **Trace ergonomics via a package helper**, not a wrapper logger type — only a handful of firehose sites need it, so changing every `log *slog.Logger` field to a wrapper is not worth it.
- **`log.Level` is a config-only type.** It exists to parse/marshal `"trace"` in YAML/flags. It is converted to `slog.Level` at the `modules.go`/`main.go` boundary; `NewLogger` and every logger field stay `slog.Level`. This keeps the CLI tooling (non-goal) untouched.
- **Default behavior unchanged.** Global default stays `info`, format stays `text`, output stays stdout. Every new knob is opt-in (nil override = inherit global).
- **Shared infra is one identical patch on all 5 branches; the call-site sweep is per-branch** (each branch has a subset of call sites).

## The level contract

Every call site is measured against this. The contract is the durable artifact of this work.

| Level | Meaning | Frequency rule |
|---|---|---|
| `error` | Operation failed, needs operator attention, not recoverable in context | Rare |
| `warn` | Unexpected but handled — degraded mode, client misbehavior, recoverable | Uncommon |
| `info` | Significant lifecycle / operational milestones | **Never per-connection, per-packet, or per-tick** |
| `debug` | Per-connection lifecycle, handshake steps, occasional diagnostics | Per-connection OK |
| `trace` | Raw byte dumps, per-packet, per-tick firehose | Unbounded OK |

Level ordering: `trace (-8) < debug (-4) < info (0) < warn (4) < error (8)`.

## Component taxonomy

One `component=` attr per line, lowercase, dotted. Declared as constants in one place so the set is discoverable.

| Component | Seam (rev-274 line refs) | Covers |
|---|---|---|
| `world.net` | per-conn client (`client.go:126`), accept/read loop (`server.go:950`) | packet I/O, connection lifecycle |
| `world.server` | `Server` (`server.go:433`) | listen / load / start / stop |
| `world.tick` | tick loop (`tick.go`) | per-tick processing |
| `world.script` | `script.go`, `npc_script.go` | RuneScript engine |
| `world.friends` | friends client/bridge/subscriber (`server.go:459-470`) | friends-server RPC |
| `world.login` | login client/bridge (`server.go:473`) | login-server RPC |
| `world.content` | content watcher / reload (`content_watcher.go`, `reload.go`) | hot-reload |
| `world` | `World` (`world.go:37`) | module-level fallback |
| `login` / `friends` / `ondemand` | module roots in `modules.go` | single-component modules |

Modules without sub-parts carry `component=<module>` (e.g. `component=login`). `logger_bridge.go`'s existing `component=logger_bridge` is realigned into this scheme (e.g. `world.report`).

## Files

### New

| File | Purpose |
|---|---|
| `pkg/util/log/level.go` | `type Level slog.Level`; `LevelTrace`/`LevelDebug`/`LevelInfo`/`LevelWarn`/`LevelError`; `UnmarshalText`/`MarshalText`/`String` (trace-aware, delegates to slog otherwise) |
| `pkg/util/log/level_test.go` | round-trip parse/format incl. `"trace"`, offsets, error cases |
| `pkg/util/log/log_test.go` | `NewLogger` renders `TRACE`; level filtering; `source` trimmed to `file.go:line` (no test file exists today) |

### Modified — shared infra (identical patch on all 5 branches)

| File | Change |
|---|---|
| `pkg/util/log/log.go` | Keep `slog.Level` signatures; add a shared `*slog.HandlerOptions`-building helper with a `ReplaceAttr` that (a) renders `LevelTrace` as `TRACE` and (b) trims `source` to `file.go:line`; add `func Trace(l *slog.Logger, msg string, args ...any)` helper |
| `cmd/goscape/app/config.go` | `LogLevel` field type `slog.Level` → `log.Level`; flag default `log.LevelInfo`; help text lists `trace` |
| `cmd/goscape/main.go` | `log.NewLogger(slog.Level(config.LogLevel), …)`; **remove** the `fmt.Printf("%+v\n", config) // DEBUG` line (smell #1) |
| `cmd/goscape/app/modules.go` | Stamp `component=<module>` on each module logger; convert override/global `log.Level`→`slog.Level` at the `NewLogger` boundary; **add** override resolution for login + friends (ondemand + world already resolve theirs) |
| `modules/world/config.go` | Change existing `LogLevel *slog.Level` → `*log.Level` (already wired in `modules.go:143-146`); swap the now-unused `log/slog` import for `pkg/util/log` |
| `modules/login/config.go` | **New** `LogLevel *log.Level` (`yaml:"log_level"`, **YAML-only**, no flag — matches world's existing precedent) |
| `modules/friends/config.go` | **New** `LogLevel *log.Level` (`yaml:"log_level"`, **YAML-only**, no flag — matches world's existing precedent) |
| `examples/full-config-reference.yaml` | Document `trace` on `log_level` + the new per-module `log_level` keys |
| `examples/bundled/goscape.yaml` | Leave as-is unless it already sets `log_level`; the full reference is where the new keys are documented |

ondemand already has `ondemand.server.log_level`; it is kept as-is and documented.

### Modified — per-branch call-site sweep

`modules/world/*` (and a few `cmd`/`pkg` lines): derive child loggers at the component seams, then sweep every call site against the level contract. Concrete known fixes on rev-274:

- **trace** ← `server.go:1033` (`received data`), `server.go:1034` (`received data payload`), `server.go:1182` (`partial packet data received`), `client.go:157` (`sent data` raw bytes).
- **warn** ← `server.go:1095` (`unhandled client state`).
- **debug** ← `server.go:990` (`connection closed`) and other per-connection lifecycle lines.
- **Coverage gaps:** error-return paths that currently log nothing get a `warn`/`error` per the contract. Genuinely ambiguous re-levels are surfaced to the user, not guessed.

The full sweep enumerates all ~200 call sites; the implementation plan batches it by file/module.

## Rollout

1. Land the full change on **rev-274** (most complete branch).
2. Apply the **shared-infra patch** (identical) to rev-254 → rev-245.2 → rev-244 → rev-225.
3. Re-run the **per-branch call-site sweep** on each (each branch has a subset of sites).
4. Update + verify each branch's example configs.

Per-branch config caveat (memory `helm_chart_per_branch_config`): example configs must only contain keys the branch's struct knows (`yaml.UnmarshalStrict`). Since the struct gains the same fields on every branch, the new keys are safe everywhere.

## Testing

- **Unit `pkg/util/log`:** `Level` round-trips incl. `"trace"` and `+N` offsets; `NewLogger` renders the level name `TRACE`; level filtering (a `trace` record is dropped at `debug`, emitted at `trace`); `source` rendered as `file.go:line`.
- **Config:** per-module override parses from YAML and flags; `--config.verify=true` still passes with the new keys present; nil override inherits the global.
- **Build + `go test ./...`**, with `-race` on touched packages (race detector works on this box; needs `CGO_ENABLED=1`).
- **Manual smoke (user-launched, per convention `smoke_test_server_handoff`):** at `log_level: info` no per-packet lines appear; at `world.log_level: trace` the firehose appears only under `component=world.*` while other modules stay at the global level.

## Non-goals

- No runtime/dynamic level changes (no per-component level matcher, no HTTP level endpoint).
- No change to log destination (stays stdout) or to the text/json format set.
- No OpenTelemetry (the `main.go:40` TODO is out of scope).
- No changes to `cmd/goscape-cli` tooling loggers (offline tools, separate concern).
