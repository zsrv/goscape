# Sub-spec 4a: Stats, AFK, and Inventory Listeners — Design

**Status:** Draft → ready for plan
**Sub-spec scope:** Stats + AFK zone state + listener-routed inventory updates + modal-close stop-transmit hook.
**Out of scope (deferred to 4b):** Zone subsystem (`pkg/zone`, zone packets, `processZones`, `BuildArea.RebuildZones`).

---

## Goal

Replace the four no-op stub functions in `processOut()` (`updateZones`, `updateInvs`, `updateStats`, `updateAfkZones`) with three working diff-driven update loops plus AFK state tracking. Stub `updateZones` remains until 4b.

After this sub-spec a logged-in player will:
- Receive `UpdateStat` packets for any stat/level changes.
- Receive `UpdateRunEnergy` packets when their wire-visible energy (energy/100) changes.
- Receive `UpdateInvFull` packets only for inventories they have an active listener on.
- Receive `UpdateInvStopTransmit` for every listener when a modal closes.
- Have `lastAfkZone` and `afkZones[2]` tracked correctly tick-by-tick.

## Architecture

Three new packet senders, three rewritten `Player.update*` functions, one new opcode bundle on `prot.go`, one extended `InventoryListener` struct, and a new optional `Server.invs` map for world-shared inventories. All new state lives on existing `Player`/`Server` structs — no new packages.

The `processOut()` invocation order is unchanged:

```
updateMap → updatePlayers → updateNpcs → updateZones (stub) → updateInvs → updateStats → updateAfkZones → encodeOut → flushWrite
```

## Components

### 1. New server opcodes — `pkg/io/protocol/game/server/prot.go`

Add to the `var (...)` block:

```go
OpUpdateStat              = Op{Opcode: 44, PayloadSize: 6}
OpUpdateRunEnergy         = Op{Opcode: 68, PayloadSize: 1}
OpUpdateInvStopTransmit   = Op{Opcode: 15, PayloadSize: 2}
```

Wire formats (matching TS reference encoders):
- **UpdateStat**: `p1(stat) p4(exp/10) p1(level)` — XP divided by 10 lossy on wire.
- **UpdateRunEnergy**: `p1(energy/100)` — internal 0–10000, wire 0–100.
- **UpdateInvStopTransmit**: `p2(component)`.

### 2. New senders

Three small files under `modules/world/`, each with a single function and table tests in a sibling `_test.go`:

#### `modules/world/stat_update.go`

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

func sendUpdateStat(p *Player, stat, exp, level int) {
    buf := packet.NewPacket(nil)
    buf.P1(uint8(stat))
    buf.P4(uint32(exp / 10))
    buf.P1(uint8(level))
    p.writeOut(gameserver.OpUpdateStat, buf.Data)
}

func sendUpdateRunEnergy(p *Player, energy int) {
    buf := packet.NewPacket(nil)
    buf.P1(uint8(energy / 100))
    p.writeOut(gameserver.OpUpdateRunEnergy, buf.Data)
}
```

#### `modules/world/inv_stop_transmit.go`

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

func sendUpdateInvStopTransmit(p *Player, com int) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    p.writeOut(gameserver.OpUpdateInvStopTransmit, buf.Data)
}
```

### 3. Extend `InventoryListener` — `modules/world/player.go`

```go
type InventoryListener struct {
    Type      int  // InvType id
    Com       int  // UI component id
    Source    int  // -1 = world, else owning player slot
    FirstSeen bool // true until first send; clears on first UpdateInvFull
}
```

When code registers a listener, it must set `FirstSeen: true`. Registration sites (in 4a, only test setup and a future `RegisterInvListener` helper) must reflect this.

### 4. World-inventory lookup — `modules/world/server.go`

Add to `Server`:

```go
invs map[int]*inventory.Inventory // world-shared inventories keyed by InvType id
```

Initialise in `NewServer` to `make(map[int]*inventory.Inventory)`. Empty by default — populated later when banks/shops are wired up. Guards: nil-check on access.

### 5. `updateInvs()` rewrite — `modules/world/player.go`

Replace the existing `updateInvs()` (which incorrectly iterates `p.invs`):

```go
func (p *Player) updateInvs() {
    for i := range p.invListeners {
        l := &p.invListeners[i]

        var inv *inventory.Inventory
        if l.Source == -1 {
            // World-shared inventory.
            if p.client == nil || p.client.server == nil {
                continue
            }
            inv = p.client.server.invs[l.Type]
        } else {
            // Player inventory: source is the owning player's slot.
            other := p.client.server.players[l.Source]
            if other == nil {
                continue
            }
            inv = other.invs[l.Type]
        }
        if inv == nil {
            continue
        }

        if inv.Update || l.FirstSeen {
            sendUpdateInvFullCom(p, l.Com, inv)
            l.FirstSeen = false
        }
    }
    // Clear inv.Update flags AFTER all listeners have observed them, so that
    // multiple listeners on the same inv each get the snapshot.
    for _, inv := range p.invs {
        inv.Update = false
    }
    if p.client != nil && p.client.server != nil {
        for _, inv := range p.client.server.invs {
            inv.Update = false
        }
    }
}
```

The existing `sendUpdateInvFull` is renamed to `sendUpdateInvFullCom` (parameter is `com int`, not `invId`) — the old name implied the wrong indirection. Existing callers (none yet outside `updateInvs`) update accordingly.

> **TODO(4b):** Add `UpdateRunWeight` emission and `runWeightChanged`/`firstSeen` tracking once weight calculation exists. The TS reference also fires `UpdateRunWeight` on first-seen for any listener whose `InvType.runweight` is true.

### 6. `updateStats()` rewrite — `modules/world/player.go`

```go
func (p *Player) updateStats() {
    for i := 0; i < 21; i++ {
        if p.stats[i] != p.lastStats[i] || p.levels[i] != p.lastLevels[i] {
            sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
            p.lastStats[i] = p.stats[i]
            p.lastLevels[i] = p.levels[i]
        }
    }
    if p.runenergy/100 != p.lastRunEnergy/100 {
        sendUpdateRunEnergy(p, p.runenergy)
        p.lastRunEnergy = p.runenergy
    }
}
```

Note: `lastRunEnergy` initialises to `-1` in `newPlayer`, so the first call after login always fires (since `-1/100 == 0` and `10000/100 == 100`). This is correct — the client needs an initial value.

> **First-tick stat baseline:** TS Player ctor sets `lastStats`/`lastLevels` to `-1` so all 21 fire on first tick. We currently zero-init both. Fix: in `newPlayer`, set `lastStats[i] = -1` and `lastLevels[i] = 255` for each `i` (sentinel values that can never legitimately equal `stats[i]`/`levels[i]`).

### 7. `updateAfkZones()` — `modules/world/player.go`

Add fields to Player:
```go
afkZones    [2]int32
lastAfkZone int
```

Implementation (port of TS Player.ts:2051):
```go
func (p *Player) updateAfkZones() {
    if p.lastAfkZone < 1000 {
        p.lastAfkZone++
    }
    if p.withinAfkZone() {
        return
    }
    coord := packAfkCoord(0, p.x-10, p.z-10) // level always 0; only x/z matter
    if p.moveSpeed == MoveSpeedInstant && p.jump {
        p.afkZones[1] = coord
    } else {
        p.afkZones[1] = p.afkZones[0]
    }
    p.afkZones[0] = coord
    p.lastAfkZone = 0
}

func (p *Player) withinAfkZone() bool {
    const size = 21
    for i := 0; i < len(p.afkZones); i++ {
        zx, zz := unpackAfkCoord(p.afkZones[i])
        if rectsIntersect(p.x, p.z, 1, 1, zx, zz, size, size) {
            return true
        }
    }
    return false
}

func (p *Player) IsZonesAfk() bool { return p.lastAfkZone == 1000 }
```

`packAfkCoord(level, x, z) int32` packs to the same `(level<<28) | ((x&0x3FFF)<<14) | (z&0x3FFF)` format used elsewhere; `unpackAfkCoord` reverses. `rectsIntersect(ax, az, aw, ah, bx, bz, bw, bh)` is a tiny axis-aligned overlap helper. Both live in a new `modules/world/afkzone.go` file, ~30 LOC total.

### 8. Modal-close stop-transmit hook — `modules/world/player.go::encodeOut`

In the existing `if modalChanged` block, before clearing `refreshModalClose`, add:

```go
if p.refreshModalClose {
    p.writeOut(gameserver.OpIfClose, nil)
    for _, l := range p.invListeners {
        sendUpdateInvStopTransmit(p, l.Com)
    }
    p.invListeners = p.invListeners[:0]
}
```

> **Approximation note:** TS only stops transmit for listeners bound to the closing modal's components; we don't yet have a component→modal mapping, so we clear ALL listeners. This is conservative but correct — listeners can be re-registered the next time the same modal opens. Document with a TODO comment.

## Data Flow

```
                            Tick boundary
                                  |
                                  v
                             processOut()
                                  |
   ┌──────────────┬───────────────┼──────────────┬──────────────┐
   |              |               |              |              |
updateMap   updatePlayers   updateNpcs    updateZones      updateInvs
                                            (stub)             |
                                                               v
                                                    iterate invListeners:
                                                    if inv.Update || FirstSeen:
                                                        UpdateInvFull
                                                        FirstSeen = false
                                                    inv.Update = false (post-loop)
                                                               |
                                                               v
                                                         updateStats
                                                               |
                                                               v
                                                       updateAfkZones
                                                               |
                                                               v
                                                          encodeOut
                                                          (may emit
                                                           IfClose +
                                                           Stop bundle)
                                                               |
                                                               v
                                                          flushWrite
```

## Error Handling

- `updateInvs` nil-checks `p.client`, `p.client.server`, the resolved player, and the resolved inventory. Any nil short-circuits that listener.
- `sendUpdateStat` indexes `[21]` arrays — bounds enforced by struct definition.
- AFK pack/unpack uses bit masks; out-of-range coords clamp via mask, no panic possible.
- `updateAfkZones` uses `MoveSpeedInstant` — confirm the constant exists (it does, in player.go:248).

## Testing

All tests in `modules/world/`. Use existing `newTestPlayer` / `newTestServer` helpers.

### `stat_update_test.go`
- `TestUpdateStatsSendsOnlyChanged` — set `stats[3]=100, levels[3]=10`, leave `last*` at sentinel. Run once. Verify exactly one packet, opcode 44, payload `{3, 0,0,0,10, 10}` (XP 100/10=10).
- `TestUpdateStatsNoEmit` — `stats==lastStats && levels==lastLevels`, run, verify no bytes written.
- `TestUpdateStatsRunEnergyCoarseGrain` — set `runenergy=10000, lastRunEnergy=-1`, run → packet fires (wire 100). Bump to 10050, run → no packet (still wire 100). Bump to 10100, run → packet fires (wire 101? no — 10100/100=101 but wire byte is 101 which still encodes correctly).

### `inv_update_test.go` (new)
- `TestUpdateInvsFirstSeenFires` — listener `{Source: 1, Type: 93, Com: 149, FirstSeen: true}`, owning player has inv 93 with `Update=false`. Run. Verify one full inv packet for component 149.
- `TestUpdateInvsRespectsDirty` — second tick with `Update=false, FirstSeen=false` → no packet. Set `Update=true` → packet fires next tick.
- `TestUpdateInvsWorldSource` — register `{Source: -1, Type: 0, Com: 200, FirstSeen: true}`, populate `s.invs[0]` with a 1-slot inv. Run. Verify packet sent.
- `TestUpdateInvsClearsUpdateFlag` — after a tick that sent the packet, `inv.Update == false`.

### `afkzone_test.go`
- `TestAfkZoneIncrementsWhileStill` — at coord (3094,3106). Call 5×. Assert `lastAfkZone == 5`, `afkZones[0]` set on first call, unchanged after.
- `TestAfkZoneRecentersOnLeave` — call once at (3094,3106). Move to (3094+25, 3106). Call again. Assert `afkZones[0]` updated to `pack(0, x-10, z-10)`, `afkZones[1] == old afkZones[0]`, `lastAfkZone == 0`.
- `TestAfkZoneSaturatesAt1000` — call 1500×. Assert `lastAfkZone == 1000`.
- `TestAfkZoneInstantJumpUsesNewCoord` — set `MoveSpeed=MoveSpeedInstant, jump=true`. Move out. Both `afkZones[0]` and `afkZones[1]` get the new coord.
- `TestZonesAfk` — `lastAfkZone = 1000` → `IsZonesAfk() == true`. 999 → false.

### `modal_close_test.go`
- `TestModalCloseEmitsStopTransmit` — register two listeners, set `refreshModalClose=true`, run `encodeOut`. Verify one IfClose packet + two Stop packets, listener slice empty.
- `TestNoStopTransmitWithoutModalClose` — listeners present, `refreshModalClose=false`, run → no Stop packets, listeners intact.


## Files to Touch

**Create:**
- `modules/world/stat_update.go` (~25 LOC)
- `modules/world/stat_update_test.go` (~80 LOC)
- `modules/world/inv_stop_transmit.go` (~12 LOC)
- `modules/world/afkzone.go` (~50 LOC)
- `modules/world/afkzone_test.go` (~80 LOC)
- `modules/world/modal_close_test.go` (~50 LOC)

**Modify:**
- `pkg/io/protocol/game/server/prot.go` — add 3 opcodes
- `modules/world/player.go` — extend `InventoryListener`, rewrite `updateStats`/`updateInvs`/`updateAfkZones`, add `afkZones`/`lastAfkZone` fields, fix `lastStats`/`lastLevels` init in `newPlayer`, hook stop-transmit into `encodeOut`
- `modules/world/server.go` — add `invs` field + init
- `modules/world/inv_update.go` — rename `sendUpdateInvFull` → `sendUpdateInvFullCom`, change parameter name `invId` → `com`, drop the misleading comment
- `modules/world/inv_update_test.go` — new file, FirstSeen + dirty + World source coverage

**Estimated total:** ~360 LOC across 11 files (6 new, 5 modified).

## Dependencies & Risks

- **No new external packages.**
- **No `pkg/coord` package exists** — inline the pack/unpack helpers in `afkzone.go`.
- **Existing `sendUpdateInvFull` callers** — currently zero outside `updateInvs`, so the rename is risk-free.
- **Init sentinel values for `lastStats`/`lastLevels`** — using `-1`/`255` is safe because `stats[i]` is `int32` (XP can theoretically be `-1` as a placeholder but is always non-negative for real gameplay) and `levels[i]` is `uint8` (max 255 — `255` as sentinel is unreachable since max real level is 99). If a future test sets `levels[i] = 255`, the test will need to bump `lastLevels[i]` to a different sentinel.

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` passes.
3. All four `Player.update*` functions called from `processOut()` either do real work or are an explicit `{}` with a `// TODO(4b)` comment.
4. The four stub functions in `player.go:323-334` (current state) replaced.
5. Player on first tick after login receives: 21 UpdateStat packets (one per skill), 1 UpdateRunEnergy packet (wire value 100), 0 UpdateInv packets (no listeners registered yet).
