# Sub-Spec 1: Tick Infrastructure, Player Registry, and Input Pipeline

**Date:** 2026-04-19  
**Project:** goscape — Go rewrite of LostCityRS/Engine-TS  
**Scope:** First of four sub-specs for the world tick loop. Delivers the 600ms game cycle, a thread-safe player registry, ISAAC-rate-limited client input dispatch, and a skeleton output pipeline. Does not implement the 7 per-player update functions (sub-specs 2–4).

---

## Context

The TypeScript reference server runs a single-threaded 600ms `cycle()` with nine ordered phases. Go's per-connection goroutines create a concurrency problem: the reader goroutine appends raw bytes to `c.in` while the tick loop needs to drain and dispatch packets from that same buffer.

The chosen approach is a **mutex-protected shared buffer** (`c.inMu`): the reader goroutine holds the mutex only for the `bufferData()` call; the tick goroutine holds it for the full per-player drain loop. This keeps all packet dispatch, ISAAC decryption, and rate-limit logic under the tick loop's control — matching the TS architecture exactly.

---

## Files

| File | Action |
|------|--------|
| `modules/world/player.go` | New — `Player` struct, `processIn`, `processOut`, `readPacket`, `encodeOut`, `writeOut` |
| `modules/world/tick.go` | New — `runTickLoop`, `processClientsIn`, `processClientsOut` |
| `modules/world/client.go` | Modify — add `inMu sync.Mutex`, `player *Player` |
| `modules/world/server.go` | Modify — add registry fields, `addPlayer`/`removePlayer`, start tick loop in `Run()` |
| `modules/world/client_game.go` | Delete — `handleGame()` inner loop moves to `Player.readPacket()` in `player.go` |
| `modules/world/handlers_game.go` | Modify — change handler signature to `func(*Player, []byte) error` |
| `pkg/io/protocol/game/client/prot.go` | Modify — add `Category int` to `Op`, populate for all 50 opcodes |
| `pkg/io/protocol/game/server/prot.go` | New — `ServerOp` type and the 6 modal + logout opcodes needed by `encodeOut` |

---

## Concurrency Model

```
[Reader goroutine per connection]       [Tick goroutine — single, 600ms]
─────────────────────────────────       ─────────────────────────────────
bufr.Read()  ← blocks on TCP            processClientsIn()
  inMu.Lock()                             for each player (RLock snapshot):
  bufferData(raw bytes)                     inMu.Lock()
  inMu.Unlock()                             readPacket() loop
  loop                                      inMu.Unlock()

                                        processClientsOut()
                                          for each player:
                                            encodeOut() + flushWrite()
```

**`inMu` contract:**
- Reader holds it only for `bufferData()` (microseconds). Never held during `bufr.Read()`.
- Tick goroutine holds it for the full `readPacket()` drain for one player.
- All other `client` fields (ISAAC ciphers, `bufw`, `state`) are owned exclusively by the tick goroutine after login completes; they need no mutex.

**`playersMu` contract:**
- `processClientsIn` and `processClientsOut` take an `RLock`, copy `playerLoop` into a local slice, then release. Login/logout mutate the registry under a write lock without disturbing the iteration copy.

---

## Player Registry

Fields added to `Server`:

```go
players     [2048]*Player   // indexed by RS2 slot; slot 0 unused
playerLoop  []*Player       // ordered iteration slice
playersMu   sync.RWMutex
currentTick int             // monotonically incrementing
```

**`addPlayer(p *Player) error`**: write-locks, scans `players[1:2048]` for a nil entry, assigns `p.slot`, appends to `playerLoop`. Returns an error (world full) if no slot is available.

**`removePlayer(p *Player)`**: write-locks, nils `players[p.slot]`, removes `p` from `playerLoop` by slot.

---

## Player Struct

```go
type Player struct {
    slot   int
    client *client

    // per-tick tracking
    playtime      int
    afkEventReady bool
    lastConnected int // tick number
    lastResponse  int // tick number

    // per-tick rate-limit counters (reset each tick)
    userLimit       int
    clientLimit     int
    restrictedLimit int

    // modal state (encodeOut)
    modalMain        int
    modalChat        int
    modalSide        int
    lastModalMain    int
    lastModalChat    int
    lastModalSide    int
    modalState       int
    refreshModal     bool
    refreshModalClose bool
}
```

---

## Op Category

`Op` in `pkg/io/protocol/game/client/prot.go` gains:

```go
Category int  // CategoryClientEvent | CategoryUserEvent | CategoryRestrictedEvent
```

```go
const (
    CategoryClientEvent     = 0 // limit 20/tick
    CategoryUserEvent       = 1 // limit 5/tick
    CategoryRestrictedEvent = 2 // limit 2/tick
)
```

Category assignments (verified against TS decoders):
- **USER_EVENT**: `MOVE_GAMECLICK`, `MOVE_OPCLICK`, `MOVE_MINIMAPCLICK`, all `OPxxx` interaction packets (OPOBJ, OPNPC, OPLOC, OPPLAYER, OPHELD, INV_BUTTON, IF_BUTTON, RESUME_*, CLOSE_MODAL, MESSAGE_PUBLIC, MESSAGE_PRIVATE, CHAT_SETMODE, CLIENT_CHEAT, REPORT_ABUSE, FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, IDK_SAVEDESIGN, TUT_CLICKSIDE)
- **RESTRICTED_EVENT**: `EVENT_TRACKING` (opcode 81), `REBUILD_GETMAPS` (opcode 150)
- **CLIENT_EVENT**: everything else (anticheat OPLOGIC/CYCLELOGIC, EVENT_CAMERA_POSITION, NO_TIMEOUT, IDLE_TIMER)

---

## Input Pipeline

**`Player.processIn(currentTick int)`**:

1. `p.playtime++`
2. If `currentTick % 500 == 0`: roll `p.afkEventReady` via `rand.Float64()`
3. If client disconnected or `c.state != ClientStateGame`: return
4. Reset `userLimit`, `clientLimit`, `restrictedLimit` to 0
5. `c.inMu.Lock(); defer c.inMu.Unlock()`
6. Loop while `userLimit < 5 && clientLimit < 20 && restrictedLimit < 2`:
   - Call `p.readPacket()` → break if false (buffer empty or partial read)
   - On success: increment counter for `op.Category`

**`Player.readPacket() bool`** — moves logic from `handleGame()`'s inner loop:

1. If `c.opcode == -1`: peek 1 byte, ISAAC-decrypt `(raw − decryptor.GetNext()) & 0xff`, look up `Ops[decrypted]`. Unknown → close connection.
2. If `c.waiting == -1`: read 1-byte length. If `c.waiting == -2`: read 2-byte length; close if > 1600.
3. If `c.in.Len() < c.waiting`: return false (partial — resumes next tick, cursor preserved in `c.opcode`/`c.waiting`).
4. Consume payload, reset `c.opcode = -1`, call `gameHandlers[opcode](p, payload)`.
5. Return true.

**Handler signature change:**

```go
// was:
var gameHandlers [256]func(*client, []byte) error
// becomes:
var gameHandlers [256]func(*Player, []byte) error
```

Handlers access the logger via `p.client.log`.

---

## Reader Goroutine Change

`handleTCPConn` in `server.go` dispatches on state:

```
ClientStateLogin  → c.handleData()              (existing — no change)
ClientStateGame   → c.inMu.Lock()
                    c.bufferData(msg)
                    c.inMu.Unlock()
                    // tick loop owns dispatch
```

---

## Login / Logout Integration

**Login** — at the end of `sendLoginOK()`, after `c.state = ClientStateGame`:

```go
p := newPlayer(c)
if err := c.server.addPlayer(p); err != nil {
    // OpWorldFull must be added to loginresp if not already present
    return c.sendLoginError(loginresp.OpWorldFull.Opcode)
}
c.player = p
```

**Logout** — `handleTCPConn`'s deferred cleanup gains:

```go
if c.player != nil {
    c.server.removePlayer(c.player)
    c.player = nil
}
```

---

## Tick Loop

`runTickLoop` starts as a goroutine inside `Server.Run()`. It uses `s.quit` (closed by `Shutdown()`) as its stop signal.

```
nextTick = time.Now()
loop:
  start    = time.Now()
  drift    = max(0, start − nextTick)

  processClientsIn()
  // processNpcs()    — stub
  // processPlayers() — stub
  // processLogouts() — stub (deregistration handled by reader goroutine)
  // processLogins()  — stub (registration handled by sendLoginOK)
  // processZones()   — stub
  // processInfo()    — stub
  processClientsOut()
  // processCleanup() — stub

  currentTick++
  nextTick += 600ms
  delay = max(0, 600ms − elapsed − drift)

  select {
  case <-s.quit:  return
  case <-time.After(delay):
  }
```

Drift compensation: if a tick takes 50ms, the next tick fires 550ms later, not 600ms.

---

## Output Pipeline

**`Player.processOut()`**:

1. `p.updateMap()`      — stub (no-op)
2. `p.updatePlayers()`  — stub (no-op)
3. `p.updateNpcs()`     — stub (no-op)
4. `p.updateZones()`    — stub (no-op)
5. `p.updateInvs()`     — stub (no-op)
6. `p.updateStats()`    — stub (no-op)
7. `p.updateAfkZones()` — stub (no-op)
8. `p.encodeOut()`      — real: modal state management
9. `p.client.flushWrite()` — single flush per tick

**`Player.encodeOut()`** — mirrors TS `NetworkPlayer.encodeOut()`:

Compares `modalMain/Chat/Side` against `lastModalMain/Chat/Side`. On divergence:
- `refreshModalClose` → send `IF_CLOSE`
- `refreshModal` → send `IF_OPENMAIN`, `IF_OPENCHAT`, `IF_OPENSIDE`, or `IF_OPENMAINSID` based on `modalState` bitmask

For sub-spec 1 all modal fields default to zero, so `encodeOut` is a no-op on every tick. The machinery is wired for later sub-specs.

**`Player.writeOut(op ServerOp, payload []byte)`**:

1. ISAAC-encrypt: `encrypted = byte((int(op.Opcode) + int(p.client.encryptor.GetNext())) & 0xff)`
2. Write encrypted byte to `c.bufw`
3. If `op.PayloadSize == -1`: write 1-byte length; if `-2`: write 2-byte length
4. Write payload
5. No flush — `flushWrite()` is called once by `processOut`

**`ServerOp`** in `pkg/io/protocol/game/server/prot.go`:

```go
type ServerOp struct {
    Opcode      byte
    PayloadSize int
}
```

Sub-spec 1 defines only the 6 modal opcodes + `LOGOUT`. Remaining ~40 server opcodes added in sub-specs 2–4.

---

## What This Sub-Spec Does NOT Include

- Player coordinate fields, stats, inventory, entity flags, timers, movement queue (sub-spec 2)
- Real `updatePlayers` / `updateNpcs` implementations (sub-spec 3)
- Real `updateZones` / `updateInvs` / `updateStats` / `updateAfkZones` (sub-spec 4)
- `processLogouts` / `processLogins` tick phases (sub-specs 2+)
- Bandwidth telemetry counters
