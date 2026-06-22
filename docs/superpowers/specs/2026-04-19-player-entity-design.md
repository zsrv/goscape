# Sub-Spec 2: Player Entity, GameMap, and Movement

**Date:** 2026-04-19
**Project:** goscape — Go rewrite of LostCityRS/Engine-TS
**Scope:** Second of four sub-specs for the world tick loop. Delivers the full RS2 player entity state (flattened TS `Entity → PathingEntity → Player` hierarchy), server-side collision-aware pathfinding, per-tick movement resolution, and the `processLogins`/`processLogouts` tick phases.

---

## Context

Sub-spec 1 delivered a tick infrastructure with a minimal `Player` carrying only network and modal fields. Sub-spec 2 expands `Player` to the full RS2 state and makes movement actually work: a MOVE_GAMECLICK packet now queues waypoints, the tick loop advances the player one tile per tick (two on run), and collision checks prevent walking through walls.

Server-side pathfinding requires a collision-flag map loaded from the RS2 map pack. Rather than write this from scratch, sub-spec 2 vendors three existing Go packages from the user's older project at `github.com/zsrv/rs-server-225`:

- `ext/routefinder/` — a complete port of the `@2004scape/rsmod-pathfinder` library
- `entity/position.go` — CoordGrid-style coordinate utilities
- `engine/gamemap.go` + map-pack parsers — collision loading

Vendored code is adapted to goscape's module path and conventions; it does not become an external dependency.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/coordgrid/coordgrid.go` | **New (vendored)** | Packed-coord pack/unpack, zone/local conversion, 8-way direction, `Direction` enum |
| `pkg/coordgrid/coordgrid_test.go` | New | Tests for each direction case and zone boundaries |
| `pkg/pathfinder/*.go` | **New (vendored)** | Collision flags, `FlagMap`, `RouteFinder`, `StepValidator`, `LineValidator`, `NaiveRouteFinder`, reach checks |
| `pkg/pathfinder/*_test.go` | New (vendored) | Vendored tests from `rs-server-225/ext/routefinder/` |
| `pkg/gamemap/gamemap.go` | **New** | `GameMap` struct wrapping `*pathfinder.PathFinderAPI`; collision mutation wrappers (`ChangeLandCollision`, `ChangeLocCollision`, `ChangeNPCCollision`, `ChangePlayerCollision`, `ChangeRoofCollision`) |
| `pkg/gamemap/load.go` | **New** | `loadGround`, `loadLocs`, `loadObjs`, `loadNPCs` parsers for `m*_*`, `l*_*`, `n*_*`, `o*_*` pack files |
| `pkg/gamemap/multimap.go` | **New** | `IsMulti` / `IsFreeToPlay` packed-coord lookup tables |
| `pkg/gamemap/gamemap_test.go` | New | Synthetic 2-mapsquare fixture; verifies collision flags written to the FlagMap |
| `pkg/cache/packfile.go` | **New (vendored)** | Generic pack-file reader; used by gamemap to load map pack entries |
| `modules/world/player.go` | Modify | Expand `Player` struct to the full field set from section 2 of this spec; add `newPlayer(c *client, username string)` |
| `modules/world/movement.go` | **New** | `pathToMoveClick`, `queueWaypoint`, `queueWaypoints`, `resolveMovement`, direction/step logic |
| `modules/world/movement_test.go` | **New** | Unit tests for `pathToMoveClick`, `resolveMovement` step advance, block detection, run-energy drain |
| `modules/world/tick.go` | Modify | Add `processPathing` phase; implement `processLogouts` and `processLogins` (replacing sub-spec 1 stubs) |
| `modules/world/server.go` | Modify | Add `gamemap *gamemap.GameMap` field, `newPlayers []*Player` queue, `appendNewPlayer` method; call `gamemap.Init` on startup; update `sendLoginOK` logic |
| `modules/world/client.go` | Modify | `sendLoginOK` appends to `newPlayers` instead of calling `addPlayer` directly |
| `modules/world/handlers_game.go` | Modify | `handleMoveClick` calls `p.pathToMoveClick(...)` with decoded waypoints instead of just logging |
| `modules/world/config.go` | Modify | Add `CachePath string` with default `./data/pack/server`; add `NodeClientRouteFinder bool` (default `true`) |

---

## Vendoring Rules

1. Only copy files actually required for sub-spec 2 behaviour. Skip sub-packages not reached by the call graph from `FindPathDefault`, `StepValidator.CanTravel`, and `NaiveRouteFinder.FindNaivePath`. If uncertainty, copy; audit is cheaper than re-vendoring later.
2. Rewrite every `github.com/zsrv/rs-server-225/...` import to `github.com/zsrv/goscape/...`.
3. Any file with `// TODO` markers gets reviewed. If the TODO blocks sub-spec 2, port the missing logic from the TS reference at `$HOME/Code/github.com/LostCityRS/Engine-TS/src/` rather than shipping a stub.
4. Vendored tests come along unless they depend on subsystems not yet in goscape (e.g., script engine). When in doubt, keep the test and skip with `t.Skip(...)` rather than delete it.
5. Adapt logging from any previous style to `log/slog`. Adapt binary I/O to `pkg/io/packet.Packet` (goscape's existing RS2 buffer).
6. Do not preserve the older project's package layout inside vendored trees — flatten where it makes sense for goscape's conventions. e.g., if `routefinder/routefinder/routefinder.go` exists, move it to `pkg/pathfinder/routefinder.go`.

---

## Player Struct

All fields live on a single `*Player` — TS inheritance flattens to Go composition-by-field. Fields are grouped by concern but the underlying struct is flat.

```go
type Player struct {
    // === network (from sub-spec 1) ===
    slot   int
    client *client

    // === identity ===
    username      string
    username37    uint64
    hash64        uint64
    displayName   string
    uid           int
    members       bool
    staffModLevel int32

    // === coordinates & level (Entity) ===
    x, z, level      int
    originX, originZ int
    lastTickX, lastTickZ, lastLevel int
    lastStepX, lastStepZ            int

    // === movement (PathingEntity) ===
    moveSpeed     int
    moveRestrict  int
    moveStrategy  int
    blockWalk     int
    walkDir, runDir int
    waypointIndex int
    waypoints     [25]int
    tele, jump    bool
    stepsTaken    int
    followX, followZ int
    targetX, targetZ int
    faceAngleX, faceAngleZ int

    // === interaction target ===
    target        entity
    targetOp      int
    targetSubject struct{ typ, com int }
    apRange       int
    apRangeCalled bool
    interacted    bool
    repathed      bool
    delayed       bool
    delayedUntil  int

    // === masks ===
    masks      int
    entitymask int

    // === appearance ===
    body           [7]int
    colors         [5]int
    gender         int
    combatLevel    int
    headicons      int
    appearanceInv  int
    appearanceBuf  []byte
    lastAppearance int

    // === stats & vars ===
    stats      [21]int32
    levels     [21]uint8
    baseLevels [21]uint8
    lastStats  [21]int32
    lastLevels [21]uint8
    vars       []int32
    varsString []string

    // === run energy ===
    run, tempRun             int
    runenergy, lastRunEnergy int
    runweight                int

    // === chat state ===
    publicChat, privateChat, tradeDuel int
    chatMessage                        []byte
    chatColour, chatEffect, chatRights int
    mutedUntil                         time.Time
    messageCount                       int

    // === session flags ===
    playtime                        int
    lastResponse, lastConnected     int
    requestLogout, requestIdleLogout, loggingOut bool
    preventLogoutMessage            string
    preventLogoutUntil              int
    reconnecting, lowMemory, webClient bool
    afkEventReady, moveClickRequest bool

    // === modal (from sub-spec 1) ===
    modalMain, modalChat, modalSide             int
    lastModalMain, lastModalChat, lastModalSide int
    modalState                                  int
    refreshModal, refreshModalClose             bool

    // === per-tick rate limits (from sub-spec 1) ===
    userLimit, clientLimit, restrictedLimit int

    // === last* fields — for opheld/opheldu/opheldt/inv_button echo suppression ===
    lastItem, lastSlot, lastUseItem, lastUseSlot, lastTargetSlot, lastCom int

    // === deferred to later sub-specs — declared but unused ===
    // invs, queue, weakQueue, engineQueue, timers, heroPoints,
    // activeScript, buildArea
}
```

**Defaults (mirroring TS constructor):**
- `slot = -1`, `uid = -1`
- `x, z = 3094, 3106`, `level = 0` (tutorial island — matches TS `Player` constructor)
- `originX, originZ = -1`
- `runenergy = 10000`
- `walkDir, runDir = -1`
- `waypointIndex = -1`
- `combatLevel = 3`
- `body = [0, 10, 18, 26, 33, 36, 42]`, `gender = 0`, `colors = [0,0,0,0,0]`
- `moveSpeed = MoveSpeedInstant` (until first movement tick)
- `moveStrategy = MoveStrategySmart`
- `moveRestrict = MoveRestrictNormal`
- `blockWalk = BlockWalkNpc`

**Entity interface:**

```go
type entity interface {
    // placeholder for sub-spec 2 — only *Player satisfies it
    Slot() int
    Coords() (x, z, level int)
}
```

Only `*Player` satisfies `entity` in sub-spec 2. `*Npc`, `*Loc`, `*Obj` types are not introduced here.

---

## Movement Behaviour

### `pathToMoveClick`

Mirrors `PathingEntity.pathToMoveClick` from the TS engine. Called from `handleMoveClick` (and `handleMoveMinimapClick`) once per valid packet:

```go
func (p *Player) pathToMoveClick(packedCoords []int, needsFinding bool) {
    switch p.moveStrategy {
    case MoveStrategySmart:
        if needsFinding {
            destX, destZ := coordgrid.Unpack(packedCoords[0])
            route := p.server.gamemap.Pathfinder.FindPathDefault(p.level, p.x, p.z, destX, destZ)
            p.queueWaypoints(route.Coords())
        } else {
            p.queueWaypoints(packedCoords)
        }
    case MoveStrategyNaive:
        destX, destZ := coordgrid.Unpack(packedCoords[len(packedCoords)-1])
        p.queueWaypoint(destX, destZ)
    }
}
```

`needsFinding` is `!cfg.NodeClientRouteFinder`. The default `NodeClientRouteFinder = true` means the Java client does its own pathfinding and the server trusts the sent waypoints — this matches the TS default.

### `handleMoveClick` update

The existing handler (currently logs path and returns) changes to decode waypoints, pack them, and dispatch:

```go
func handleMoveClick(p *Player, payload []byte) error {
    // ... existing decode of ctrlHeld, startX, startZ, deltas ...
    packed := make([]int, 0, len(path))
    for _, pt := range path {
        packed = append(packed, coordgrid.Pack(pt.x, pt.z, p.level))
    }
    needsFinding := !p.server.cfg.NodeClientRouteFinder
    p.pathToMoveClick(packed, needsFinding)
    return nil
}
```

### Per-tick step resolution — `processPathing`

New tick phase inserted between `processClientsIn` and `processClientsOut`:

```
processClientsIn()    // populates p.waypoints via pathToMoveClick
processPathing()      // advances one tile (walk) or two tiles (run)
processClientsOut()   // stubs for now — sub-spec 3 fills in player-info with walk/run dirs
```

### `Player.resolveMovement`

Called from `processPathing` for each player. Returns nothing — mutates `p` directly.

1. Capture `p.lastTickX = p.x; p.lastTickZ = p.z; p.lastLevel = p.level` at the start.
2. If `p.waypointIndex < 0` (no path): set `walkDir = -1, runDir = -1`, return.
3. Call `stepOnce` to advance one tile toward `waypoints[p.waypointIndex]`. Record `walkDir = <direction taken>`.
4. If `p.moveSpeed == MoveSpeedRun && p.runenergy > 0`: call `stepOnce` again. Record `runDir = <second direction>`.
5. `stepOnce` checks `gamemap.Pathfinder.StepValidator.CanTravel(level, x, z, direction, size=1, extraFlag=0, collisionType=NORMAL)` — if blocked, clears the whole waypoint queue (`waypointIndex = -1`) and returns without advancing.
6. After a successful step, update `lastStepX/Z = <pre-step coords>`, then `x/z = <post-step coords>`, and advance `stepsTaken++`.
7. When `x,z == waypoints[waypointIndex]` (current waypoint reached), decrement `waypointIndex` to advance to the next waypoint. When `waypointIndex == -1`, path is done.
8. Drain `runenergy` on each running step (TS formula: `runenergy -= (67 + 67*runweight/64) / 100`, floored at 0).

### Direction constants

Use `coordgrid.Direction` enum (from vendored `position.go`):
- `DirectionNorthwest = 0`
- `DirectionNorth = 1`
- `DirectionNortheast = 2`
- `DirectionWest = 3`
- `DirectionEast = 4`
- `DirectionSouthwest = 5`
- `DirectionSouth = 6`
- `DirectionSoutheast = 7`

---

## processLogins and processLogouts

Both replace sub-spec 1 stubs in `tick.go`.

### Timeout constants

From TS `World.ts`:
- `timeoutNoResponse = 100` ticks (60s — force logout after no client response)
- `timeoutNoConnection = 50` ticks (30s — idle-logout request after socket silence)

When `cfg.NodeDebugSocket` is `true`, both timeouts are disabled (matches the existing read-deadline behaviour for debug/bot testing).

### processLogouts

```go
func (s *Server) processLogouts() {
    if s.cfg.NodeDebugSocket {
        return
    }

    players := s.snapshotPlayerLoop()
    for _, p := range players {
        force := false
        if s.currentTick - p.lastResponse >= timeoutNoResponse {
            p.loggingOut = true
            force = true
        } else if s.currentTick - p.lastConnected >= timeoutNoConnection {
            p.requestIdleLogout = true
        }

        if p.requestLogout || p.requestIdleLogout {
            if s.currentTick >= p.preventLogoutUntil {
                p.loggingOut = true
            }
            p.requestLogout = false
            p.requestIdleLogout = false
        }

        if p.loggingOut && (force || s.currentTick >= p.preventLogoutUntil) {
            p.writeOut(gameserver.OpLogout, nil)
            p.client.flushWrite()
            p.client.conn.Close()
            s.removePlayer(p)
        }
    }
}
```

Sub-spec 2 omits: LOGOUT script execution, inventory save, session logs, reconnection handling. These belong in later sub-specs (script engine, inventory, reconnect).

### processLogins

`Server` gains a `newPlayers []*Player` slice guarded by `playersMu`. `sendLoginOK` changes:

```go
// sub-spec 1 (old):
// if err := c.server.addPlayer(p); err != nil { return ... }

// sub-spec 2 (new):
c.server.appendNewPlayer(p)
c.player = p
```

`appendNewPlayer` acquires the write lock on `playersMu` and appends to `newPlayers`. It does not assign a slot yet — `processLogins` does that on the next tick.

```go
func (s *Server) processLogins() {
    s.playersMu.Lock()
    batch := s.newPlayers
    s.newPlayers = nil
    s.playersMu.Unlock()

    for _, p := range batch {
        if err := s.addPlayerLocked(p); err != nil {
            // world became full between appendNewPlayer and here — reject
            p.writeOut(gameserver.OpLogout, nil)
            p.client.flushWrite()
            p.client.conn.Close()
            continue
        }
        p.lastConnected = s.currentTick
        p.lastResponse  = s.currentTick
        p.originX = p.x
        p.originZ = p.z
    }
}
```

`addPlayerLocked` is a refactor of the existing `addPlayer`: the locking moves out so the two phases (snapshot + slot assignment) can be combined. Callers that used `addPlayer` directly (tests) are migrated to acquire the lock explicitly or use the new wrapper that still locks.

### Tick ordering

```go
func (s *Server) tick() {
    s.processClientsIn()
    s.processPathing()
    s.processLogouts()
    s.processLogins()
    s.processClientsOut()
    s.currentTick++
}
```

`processLogouts` runs before `processLogins` so that a player logging in on the same tick as someone logging out can take their slot.

### Reader-goroutine deferred cleanup

Keep the existing `removePlayer` in the defer block — defensive path for abrupt disconnects where `processLogouts` hasn't noticed yet.

### lastResponse update

`processClientsIn` updates `p.lastResponse = s.currentTick` whenever a non-zero number of bytes was drained (matches TS `decodeIn`).

---

## GameMap

### Package layout

```
pkg/gamemap/
  gamemap.go        — GameMap struct, Init, collision mutation wrappers
  load.go           — loadGround / loadLocs / loadObjs / loadNPCs parsers
  multimap.go       — IsMulti / IsFreeToPlay packed-coord lookup tables
```

### GameMap struct

```go
type GameMap struct {
    Pathfinder *pathfinder.PathFinderAPI
    multimap   []int  // packed coords marking multi-combat zones
    freemap    []int  // packed coords marking F2P zones
    log        *slog.Logger
}

func New(log *slog.Logger) *GameMap { ... }
func (gm *GameMap) Init(cacheDir string) error { ... }
func (gm *GameMap) ChangeLandCollision(x, z, level int, add bool) { ... }
func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, x, z, level int, add bool) { ... }
func (gm *GameMap) ChangeNPCCollision(size, x, z, level int, add bool) { ... }
func (gm *GameMap) ChangePlayerCollision(size, x, z, level int, add bool) { ... }
func (gm *GameMap) ChangeRoofCollision(x, z, level int, add bool) { ... }
func (gm *GameMap) IsMulti(coord int) bool { ... }
func (gm *GameMap) IsFreeToPlay(x, z int) bool { ... }
```

### Init loading

Matches the TS `GameMap.load()` flow (`$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/GameMap.ts`):

1. `filepath.Glob(cacheDir + "/maps/m*")` to enumerate mapsquare data files. Each filename encodes `mapSquareX_mapSquareZ` (e.g., `m50_50`).
2. For each `(mapSquareX, mapSquareZ)`:
   - Read `m{mapSquareX}_{mapSquareZ}` → `loadGround` → mark blocked floor tiles in `Pathfinder.Flags` (when `land & BLOCK_MAP_SQUARE != 0`)
   - Read `l{mapSquareX}_{mapSquareZ}` → `loadLocs` → call `Pathfinder.ChangeLoc` for each wall/scenery with `blockwalk != 0`
   - Read `n{mapSquareX}_{mapSquareZ}` if present → store positions in a side list (sub-spec 2 does not spawn NPCs, but later sub-specs will)
3. Read `maps/multiway.csv` (if present) → parse CSV rows → populate `multimap`
4. Read `maps/free2play.csv` (if present) → parse CSV rows → populate `freemap`
5. Log: `"loaded map: X mapsquares, Y blocked tiles, Z locs, elapsed=…"`

Missing files within a mapsquare set are treated as "no data for this mapsquare" rather than errors — some mapsquares only have `m*` (ground only, no locs/NPCs).

`Init` runs synchronously during `NewServer`; startup blocks until map loading completes or errors.

### Cache integration

`pkg/cache/packfile.go` is vendored from `rs-server-225/cache/packfile.go` — a generic reader for `pack/server/*` files that returns raw bytes by index name. `gamemap.Init` uses it.

### Config

```go
// modules/world/config.go
type Config struct {
    // ... existing fields ...
    CachePath             string
    NodeClientRouteFinder bool
}
```

Defaults: `CachePath = "./data/pack/server"`, `NodeClientRouteFinder = true`. The existing config-layering code (defaults → file → env → flags) picks these up automatically once the struct fields are added and `RegisterFlagsAndApplyDefaults` is updated.

### Startup wiring

```go
// NewServer (abbreviated):
gm := gamemap.New(logger)
if err := gm.Init(cfg.CachePath); err != nil {
    return nil, fmt.Errorf("failed to load game map: %w", err)
}
s.gamemap = gm
```

---

## Testing Strategy

| Package | Tests |
|---------|-------|
| `pkg/coordgrid` | Pure-math unit tests: each of 8 direction cases, zone/local conversion at mapsquare boundaries, pack/unpack round-trip |
| `pkg/pathfinder` | Vendored tests from `rs-server-225/ext/routefinder/` + smoke test: `FindPathDefault` returns a non-empty path on a small synthetic FlagMap |
| `pkg/gamemap` | Synthetic 2-mapsquare fixture built in-memory; verify `ChangeLocCollision` sets the right flags in `Pathfinder.Flags`. Full-cache load gated behind `//go:build mappack` |
| `modules/world/player_test.go` | Extend with: `pathToMoveClick` queues waypoints correctly (both SMART with findPath, SMART with trust-client, NAIVE), `resolveMovement` advances one tile on walk / two on run, stops at blocked tiles, updates `lastTickX/Z`, drains `runenergy` |
| `modules/world/tick_test.go` | `processLogouts` sets `loggingOut` after 100 ticks of no response, `processLogins` moves players from `newPlayers` to `playerLoop` on next tick, rejects world-full gracefully. |
| Integration | `TestMoveGameClickAdvancesPlayer` — set up player, write encrypted MOVE_GAMECLICK, tick three times, assert player advanced along the waypoint |

---

## What Sub-Spec 2 Does NOT Include

- `invs` map, `Inventory` type, inventory packets — sub-spec 4
- Script queues (`queue`, `weakQueue`, `engineQueue`), `activeScript`, `ScriptRunner` — deferred (not in original 4-spec decomposition; may warrant its own sub-spec)
- `timers`, `EntityTimer` — deferred
- `heroPoints`, damage tracking, combat — deferred
- `buildArea`, zone tracking for info blocks — sub-spec 3
- Player/NPC info encoding — sub-spec 3
- NPCs as pathing entities — sub-spec 2 loads NPC positions from pack files but does not instantiate `*Npc`
- `updatePlayers`, `updateNpcs`, `updateZones`, `updateInvs`, `updateStats`, `updateAfkZones` — remain stubs
- LOGOUT script trigger — deferred with the script engine
- Inventory persistence / player save files — deferred
- Reconnect handling (`reconnecting` field wiring) — deferred

**Observable outcome of sub-spec 2:** a logged-in player can click-to-move; the server validates the path against collision data and advances the player one tile per tick (two on run). Players get disconnected after 60s of no packets or 30s of no connection. The player is not yet visible to other players — that requires sub-spec 3's player-info block.
