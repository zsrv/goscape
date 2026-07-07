# Architecture

This page is for someone about to **modify engine code**. It goes a level deeper
than the operator-facing tour in the [Administrator's Guide](../admin/index.md),
which already covers the module set, the `--target` flag, the dependency graph,
network surfaces, and data layout — read that first for the runtime picture; this
page does not repeat it. Everything below describes the `rev-274` layout; the
same patterns hold across revision branches even where individual packages
differ.

## Service lifecycle (dskit)

Every long-running component is a **service**, using a port of
[Grafana's dskit](https://github.com/grafana/dskit) under `pkg/dskit/`. Services
follow a strict state machine:

```
New → Starting → Running → Stopping → Terminated
```

A failure in any of `Starting`, `Running`, or `Stopping` sends the service to a
terminal `Failed` state instead of `Terminated`. The key types live in
`pkg/dskit/services/`:

- **`services.Service`** — the interface every component implements.
- **`services.BasicService`** — the standard concrete implementation, built with
  `NewBasicService(startingFn, runningFn, stoppingFn)`. Each module's
  `NewXxxService(...)` factory wraps its server in one of these: the `startingFn`
  binds listeners and dependencies, the `runningFn` blocks until shutdown, and
  the `stoppingFn` tears down. (`NewIdleService` is the degenerate form with no
  running phase.)
- **`services.Manager`** — starts and stops a group of services and reports group
  state transitions to registered listeners. A listener implements
  `ManagerListener` (constructible via `NewManagerListener(healthy, stopped,
  failure)`), whose `failure` callback fires when any managed service fails.
- **`services.FailureWatcher`** — the failure-surfacing helper. `WatchManager`
  (or `WatchService`) registers a listener whose failure callback pushes a
  descriptive error onto a channel exposed by `Chan()`; the App root selects on
  that channel so that **one service failure brings the whole process down**
  rather than leaving a half-running server. This is the mechanism behind the
  "one failure stops everything" behaviour described in the admin guide.

## Module system and the registration pattern

The named modules (`ondemand`, `world`, `login`, `friends`, plus the invisible
`common` and `database`) are wired by `pkg/dskit/modules.Manager`, which resolves
the dependency graph and initialises modules in topological order. The wiring
lives in `cmd/goscape/app/modules.go`:

```go
mm.RegisterModule(Common, nil, modules.UserInvisibleModule)
mm.RegisterModule(OnDemand, g.initOnDemand)
mm.RegisterModule(Database, g.initDatabase, modules.UserInvisibleModule)
// …
mm.AddDependency(World, Common, Login, Friends)   // World needs these first
```

Each `RegisterModule` binds a module name to an `init` function returning a
`services.Service` (or `nil` for a pure graph anchor / a disabled module).
`UserInvisibleModule` marks the internal `common` and `database` modules so they
cannot be chosen directly as a `--target`. `AddDependency` declares the edges; a
module's init function can rely on every dependency having been initialised
before it runs (for example, `initOnDemand` reads the already-built
`g.world.Server` because `OnDemand` depends on `World`).

A disabled module returns `nil` rather than an idle placeholder service — an
idle stand-in would falsely report `Running` and vacuously satisfy a dependent's
readiness wait.

### Per-module file convention

Each feature module under `modules/<name>/` follows the same three-file shape:

- **`config.go`** — a `Config` struct with `RegisterFlagsAndApplyDefaults(f
  *flag.FlagSet)` (defaults + flag registration) and `Validate()` (fail-fast
  config checks).
- **`<name>.go`** — the top-level struct, its `New(cfg, logger, …)` constructor,
  and the `NewXxxService(...)` factory that wraps the server in a
  `services.BasicService`. For example `modules/world/world.go` defines
  `world.New(...)` and `world.NewWorldService(...)`.
- **`server.go`** — the network listener itself (a raw TCP accept loop for
  `world`; the OnDemand handlers are registered on a `pkg/dskit/server` HTTP
  mux). Some modules split additional concerns into further files
  (`handler.go`, `health.go`, and so on) but the config/top-level/server split
  is the backbone.

Adding a module means registering it in `modules.go`, wiring its dependencies,
and adding its `Config` to the app config struct.

## Packet buffer conventions (`pkg/io/packet`)

The RS2 wire format is read and written through `packet.Packet`, which wraps a
`bytes.Buffer` with RS2-specific accessors. The naming is terse and consistent,
and learning it makes the protocol code readable at a glance:

- **`G` = get (read), `P` = put (write).** The digit is the width in bytes:
  `G1`/`P1` read/write one unsigned byte, `G2`/`P2` two, `G4`/`P4` four,
  `G8`/`P8` eight. So `G4` gets four unsigned bytes as a `uint32` and `P1(v)`
  puts one unsigned byte.
- **Suffixed variants** cover the format's special encodings: `GSmart`/`PSmart`
  (1-or-2-byte "smart" ints), `GVarInt`/`PVarInt`, `GBool`/`PBool`, and the
  ordering/transform variants in `alt.go` (`P1Alt1`, `P4Alt2`, …) that mirror
  the client's obfuscated byte orders.
- **Strings** carry their terminator in the name: `GJStr`/`PJStr` (JagString),
  `GJStrLF`/`PJStrLF` (newline-terminated), and `GJStrNUL`/`PJStrNUL`
  (null-terminated). `GJStrLF` reads a newline-terminated JagString.
- **`RSAEnc` / `RSADec`** encrypt/decrypt the buffer's contents with the login
  RSA key; `GData`/`PData` move raw byte runs.
- **Bit-level I/O happens on the same `Packet`**, not on a separate type. The
  struct carries a bit cursor (`BitPos`) alongside the byte cursor (`Pos`);
  calling `AccessBits()` switches the stream position to bit access, after
  which `GBit(n)` / `PBit(n, value)` read/write *n* bits at a time, and
  `AccessBytes()` rounds the bit cursor back up to a byte boundary before any
  byte accessor is used again. This mode switch is how the protocol packs
  sub-byte fields — the player-info/NPC-info bitmasks in movement and
  appearance updates are built this way.

Because these accessors are the byte boundary, they are pinned by tests and must
stay byte-faithful — see the [Porting lessons](porting-lessons.md) on
byte-parity. The [Protocol](../protocol/index.md) guide documents the wire
formats these methods implement.

## Connection lifecycle and ISAAC placement

The `world` module is a raw TCP server: `server.go` runs `net.Listen`, an
`Accept` loop, and a goroutine per connection. Each connection is driven by a
`client` state machine (`modules/world/client.go`) that starts at
`ClientStateLogin` (`0`) and advances to `ClientStateGame` (`1`) once login
completes (`ClientStateOndemand` and `ClientStateClosed` are the other states).

**ISAAC ciphers are armed during the login handshake, not before it.** The login
request carries the client's ISAAC seed array inside the RSA-encrypted block. In
`server_login.go`, after the RSA block is decrypted, the server seeds its
**decryptor** stream directly from that seed array, then adds `50` to each seed
element and seeds the **encryptor** stream from the adjusted array — matching the
client, which does the mirror-image thing. From that point on every game packet
is ISAAC-enciphered: `player.go`'s write path enciphers the opcode before the
payload, and its read path deciphers the opcode before dispatch. Only after this
handshake does the connection move to `ClientStateGame`. The step-by-step wire
view is in the [login handshake](../protocol/login.md) and
[ISAAC](../protocol/isaac.md) protocol pages.

## Signal-handling ownership

There is exactly **one real OS signal handler in the process, and the App root
owns it.** At startup the App installs `signals.NewHandler(...)`
(`pkg/dskit/signals`); when it receives a stop signal it calls `StopAsync()` on
the services `Manager`, which cascades shutdown through every service's
`Stopping` state in reverse dependency order — each module waits for the modules
that depend on it to finish stopping first.

The catch is that `pkg/dskit/server` (the HTTP server the OnDemand module uses)
would install its *own* signal handler by default, and so would an embedded
world server. To keep ownership at the root, the App calls
`DisableSignalHandling` on those configs before constructing them —
`server.DisableSignalHandling(&g.cfg.OnDemand.Server)` and
`world.DisableSignalHandling(&g.cfg.World)` in `modules.go` — which swaps the
module's handler for a no-op channel. If you add a module that embeds a
signal-capable server, disable its handler the same way: the App root is the
single owner of process shutdown.
