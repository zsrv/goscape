# Sub-spec 4b-4: Zone Tick Integration — Design

**Status:** Draft → ready for plan
**Scope:** Wire `pkg/zone` into the tick loop. Ship the last remaining `{}` stub — `Player.updateZones`. After this sub-spec, `server.AddObj(obj, PublicReceiver)` produces a correct `UpdateZonePartialEnclosed` packet delivered to every player whose build area overlaps.
**Out of scope:** Script-driven lifecycle transitions, `Zone.Enter/Leave` for player/npc membership tracking, static loc/obj cache loading, OBJS=129 eviction, `ObjType.tradeable` gating, hash64 obj-receiver routing.

---

## Goal

Close sub-spec 4b by connecting 4b-1 (entity types), 4b-2 (encoders), and 4b-3 (zone subsystem) into the tick loop. Specifically:

- `Server` owns the world's `ZoneMap` and a per-tick `zonesTracking` set.
- `Server.AddLoc/AddObj/...` dispatchers route mutations to the right Zone and track it.
- `BuildArea.Rebuild` populates `ActiveZones` with the existing 13×13 window (`for dx := -6; dx <= 6`).
- `Player.updateZones` iterates `ActiveZones`, emits the three outer zone packets, and synthesizes `writeFullFollows` replay for newly-loaded zones.
- `processZones` tick phase computes shared buffers; `processCleanup` resets tracked zones.

## Architecture

Five integration surgeries + one new-file pair + a small `coordgrid` migration:

```
pkg/coordgrid/coordgrid.go        + ZoneIndex / UnpackIndex (moved from pkg/zone)
pkg/zone/map.go                   re-export coordgrid versions (or use directly)
pkg/io/protocol/game/server/prot.go   + 10 top-level zone Op{} entries
pkg/buildarea/buildarea.go        Rebuild also fills ActiveZones
modules/world/server.go           + zoneMap, zonesTracking fields + TrackZone
modules/world/world_zone.go NEW   11 dispatcher methods
modules/world/player.go           updateZones() — real
modules/world/player_zone.go NEW  writeFullFollows, writePartialFollows, sendZoneNested
modules/world/tick.go             + processZones phase; extend processCleanup
```

## Components

### 1. Move `ZoneIndex`/`UnpackIndex` to `pkg/coordgrid`

Append to `pkg/coordgrid/coordgrid.go`:

```go
// ZoneIndex packs (worldX, worldZ, level) into a single int using the
// layout shared with the TS reference's ZoneMap.zoneIndex:
//   zone_x = worldX >> 3, zone_z = worldZ >> 3
//   index  = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int {
	return ((worldX >> 3) & 0x7FF) | (((worldZ >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

// UnpackZoneIndex reverses ZoneIndex. Returns TILE-unit coordinates at
// the zone's SW corner (zoneX<<3, zoneZ<<3).
func UnpackZoneIndex(index int) (worldX, worldZ, level int) {
	worldX = (index & 0x7FF) << 3
	worldZ = ((index >> 11) & 0x7FF) << 3
	level = (index >> 22) & 0x3
	return
}
```

Update `pkg/zone/map.go` — delete the local `ZoneIndex`/`UnpackIndex` and route callers through `coordgrid.ZoneIndex`/`coordgrid.UnpackZoneIndex`. The old test `TestZoneIndexRoundTrip` in `pkg/zone/map_test.go` moves (or is re-targeted) to `pkg/coordgrid/coordgrid_test.go`.

### 2. 10 top-level zone opcodes — `pkg/io/protocol/game/server/prot.go`

Zone-nested opcodes double as top-level outgoing opcodes when written per-player inside `UpdateZonePartialFollows`. Sizes match the Java client's `SERVERPROT_SIZES`:

```go
OpLocAddChange = Op{Opcode: 59,  PayloadSize: 4}
OpLocAnim      = Op{Opcode: 42,  PayloadSize: 4}
OpLocDel       = Op{Opcode: 76,  PayloadSize: 2}
OpLocMerge     = Op{Opcode: 23,  PayloadSize: 14}
OpMapAnim      = Op{Opcode: 191, PayloadSize: 6}
OpMapProjAnim  = Op{Opcode: 69,  PayloadSize: 15}
OpObjAdd       = Op{Opcode: 223, PayloadSize: 5}
OpObjCount     = Op{Opcode: 151, PayloadSize: 7}
OpObjDel       = Op{Opcode: 49,  PayloadSize: 3}
OpObjReveal    = Op{Opcode: 50,  PayloadSize: 7}
```

### 3. `BuildArea.Rebuild` extension

Inside the existing `Rebuild` double-for loop (after the existing mapsquare population), populate `ActiveZones` too:

```go
// existing: ba.Mapsquares[uint16((mapX<<8)|mapZ)] = true
ba.ActiveZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true
```

Level is always 0 at the build-area layer for goscape's current map model. The TS reference similarly hardcodes level 0 in `BuildArea.rebuildZones`.

### 4. `Server` fields — `modules/world/server.go`

```go
type Server struct {
    // ... existing fields ...
    zoneMap       *zone.ZoneMap
    zonesTracking map[*zone.Zone]struct{}
}
```

`NewServer` init:
```go
zoneMap:       zone.NewZoneMap(),
zonesTracking: map[*zone.Zone]struct{}{},
```

Plus:
```go
// TrackZone marks a zone as modified this tick. Idempotent (map semantics).
func (s *Server) TrackZone(z *zone.Zone) { s.zonesTracking[z] = struct{}{} }
```

### 5. Dispatchers — `modules/world/world_zone.go` (new)

11 thin wrappers around `ZoneMap.Get(...).Method(...)` + `TrackZone`:

```go
package world

import (
    "github.com/zsrv/goscape/pkg/entity"
    "github.com/zsrv/goscape/pkg/zone"
)

func (s *Server) AddLoc(loc *entity.Loc) {
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.AddLoc(loc)
    s.TrackZone(z)
}

func (s *Server) ChangeLoc(loc *entity.Loc) {
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.ChangeLoc(loc)
    s.TrackZone(z)
}

func (s *Server) RemoveLoc(loc *entity.Loc) {
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.RemoveLoc(loc)
    s.TrackZone(z)
}

func (s *Server) AnimLoc(loc *entity.Loc, seq int) {
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.AnimLoc(loc, seq)
    s.TrackZone(z)
}

func (s *Server) MergeLoc(
    loc *entity.Loc,
    playerSlot, startCycle, endCycle int,
    east, south, west, north int,
) {
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.MergeLoc(loc, playerSlot, startCycle, endCycle, east, south, west, north)
    s.TrackZone(z)
}

func (s *Server) AddObj(obj *entity.Obj, receiverID int) {
    z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
    z.AddObj(obj, receiverID)
    s.TrackZone(z)
}

func (s *Server) ChangeObj(obj *entity.Obj, oldCount, newCount int) {
    z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
    z.ChangeObj(obj, oldCount, newCount, s.currentTick)
    s.TrackZone(z)
}

func (s *Server) RemoveObj(obj *entity.Obj) {
    z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
    z.RemoveObj(obj, s.currentTick)
    s.TrackZone(z)
}

func (s *Server) RevealObj(obj *entity.Obj, receiverSlot int) {
    z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
    z.RevealObj(obj, receiverSlot)
    s.TrackZone(z)
}

func (s *Server) AnimMap(level, x, zc, spotanim, height, delay int) {
    z := s.zoneMap.Get(level, x, zc)
    z.AnimMap(x, zc, spotanim, height, delay)
    s.TrackZone(z)
}

func (s *Server) MapProjAnim(
    level, srcX, srcZ, dstX, dstZ int,
    target, spotanim, srcHeight, dstHeight int,
    startDelay, endDelay, peak, arc int,
) {
    z := s.zoneMap.Get(level, srcX, srcZ)
    z.MapProjAnim(srcX, srcZ, dstX, dstZ,
        target, spotanim, srcHeight, dstHeight,
        startDelay, endDelay, peak, arc)
    s.TrackZone(z)
}
```

### 6. `Player.updateZones` — `modules/world/player.go`

Replace the `{}` stub with:

```go
func (p *Player) updateZones() {
    if p.buildArea == nil || p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server

    // Unload zones no longer active.
    for idx := range p.buildArea.LoadedZones {
        if !p.buildArea.ActiveZones[idx] {
            delete(p.buildArea.LoadedZones, idx)
        }
    }

    // Deliver each active zone.
    for idx := range p.buildArea.ActiveZones {
        z := s.zoneMap.GetByIndex(idx)

        if !p.buildArea.LoadedZones[idx] {
            p.writeFullFollows(z, s.currentTick)
        }

        if shared := z.Shared(); len(shared) > 0 {
            buf := packet.NewPacket(nil)
            rsbuf.EncodeZonePartialEnclosed(buf, z.X, z.Z, p.originX, p.originZ, shared)
            p.writeOut(gameserver.OpUpdateZonePartialEnclosed, buf.Bytes())
        }

        p.writePartialFollows(z)
        p.buildArea.LoadedZones[idx] = true
    }
}
```

### 7. `Player.writeFullFollows` + `writePartialFollows` + `sendZoneNested` — `modules/world/player_zone.go` (new)

```go
package world

import (
    "fmt"

    "github.com/zsrv/goscape/pkg/coordgrid"
    "github.com/zsrv/goscape/pkg/entity"
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
    "github.com/zsrv/goscape/pkg/rsbuf"
    "github.com/zsrv/goscape/pkg/zone"
)

// writeFullFollows sends UpdateZoneFullFollows (client zone reset) followed
// by a PartialFollows wrapper + synthesized messages for every currently-
// active dynamic loc/obj in the zone (skipping anything transitioned this
// tick — that arrives via the Enclosed buffer instead).
func (p *Player) writeFullFollows(z *zone.Zone, currentTick int) {
    buf := packet.NewPacket(nil)
    rsbuf.EncodeZoneFullFollows(buf, z.X, z.Z, p.originX, p.originZ)
    p.writeOut(gameserver.OpUpdateZoneFullFollows, buf.Bytes())

    hasMessages := false
    ensureHeader := func() {
        if hasMessages {
            return
        }
        hb := packet.NewPacket(nil)
        rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
        p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
        hasMessages = true
    }

    for _, obj := range z.Objs {
        if obj.LastLifecycleTick == currentTick {
            continue
        }
        if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.slot {
            continue
        }
        if !obj.CheckLifecycle(currentTick) {
            continue
        }
        ensureHeader()
        pb := packet.NewPacket(nil)
        rsbuf.EncodeObjAdd(pb, coordgrid.PackZoneCoord(obj.X, obj.Z), obj.Type, obj.Count)
        p.writeOut(gameserver.OpObjAdd, pb.Bytes())
    }

    for _, loc := range z.Locs {
        if loc.LastLifecycleTick == currentTick {
            continue
        }
        if loc.Lifecycle == entity.LifecycleDespawn && loc.CheckLifecycle(currentTick) {
            ensureHeader()
            pb := packet.NewPacket(nil)
            rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z), loc.Shape(), loc.Angle(), loc.Type())
            p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
        }
        // Respawn-lifecycle (static) branches deferred — goscape doesn't load statics yet.
    }
}

// writePartialFollows iterates the zone's per-tick Follows events filtered
// by recipient, emitting a PartialFollows header (once, if any match) then
// each event as its own top-level zone-nested packet.
func (p *Player) writePartialFollows(z *zone.Zone) {
    hasAnyForMe := false
    for _, e := range z.Events() {
        if e.Type != zone.ZoneEventFollows || e.Bytes == nil {
            continue
        }
        if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.slot {
            continue
        }
        if !hasAnyForMe {
            hb := packet.NewPacket(nil)
            rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
            p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
            hasAnyForMe = true
        }
        sendZoneNested(p, e.Bytes)
    }
}

// sendZoneNested dispatches [opcode, ...payload] bytes as a top-level packet.
func sendZoneNested(p *Player, b []byte) {
    if len(b) == 0 {
        return
    }
    p.writeOut(zoneNestedOp(b[0]), b[1:])
}

func zoneNestedOp(op byte) gameserver.Op {
    switch op {
    case rsbuf.ZoneOpLocAddChange:
        return gameserver.OpLocAddChange
    case rsbuf.ZoneOpLocAnim:
        return gameserver.OpLocAnim
    case rsbuf.ZoneOpLocDel:
        return gameserver.OpLocDel
    case rsbuf.ZoneOpLocMerge:
        return gameserver.OpLocMerge
    case rsbuf.ZoneOpMapAnim:
        return gameserver.OpMapAnim
    case rsbuf.ZoneOpMapProjAnim:
        return gameserver.OpMapProjAnim
    case rsbuf.ZoneOpObjAdd:
        return gameserver.OpObjAdd
    case rsbuf.ZoneOpObjCount:
        return gameserver.OpObjCount
    case rsbuf.ZoneOpObjDel:
        return gameserver.OpObjDel
    case rsbuf.ZoneOpObjReveal:
        return gameserver.OpObjReveal
    }
    panic(fmt.Sprintf("unknown zone-nested opcode %d", op))
}
```

### 8. Tick phase — `modules/world/tick.go`

Add a new phase between `processInfo` and `processClientsOut`:

```go
s.processInfo()
s.processZones()       // ← NEW
s.processClientsOut()
```

Implementation:
```go
func (s *Server) processZones() {
    for z := range s.zonesTracking {
        z.ComputeShared()
    }
}
```

Extend `processCleanup` (after the existing ResetMasks loops):

```go
for z := range s.zonesTracking {
    z.Reset()
}
clear(s.zonesTracking)
```

## Data Flow

```
Game code
    │  server.AddObj(obj, PublicReceiver)
    ▼
Server.AddObj → zoneMap.Get().AddObj() → TrackZone(z)

    (tick proceeds)

processInfo           — compute player/npc info caches
    ▼
processZones (NEW)    — foreach z in zonesTracking: z.ComputeShared()
    ▼
processClientsOut
    │  foreach player: processOut() → updateZones()
    │      unload inactive-zone entries from LoadedZones
    │      foreach idx in ActiveZones:
    │          z := zoneMap.GetByIndex(idx)
    │          if !loaded: writeFullFollows(z)
    │          emit PartialEnclosed(z.Shared())
    │          emit PartialFollows for per-player events
    │          LoadedZones[idx] = true
    ▼
processCleanup
    │  foreach z in zonesTracking: z.Reset()
    │  clear zonesTracking
```

## Error Handling

- Nil-guard `buildArea`, `client`, `client.server`, `zoneMap` (none of these should be nil after login, but defensive).
- `zoneNestedOp` panics on unknown opcode — unreachable in practice, serves as assertion.
- `processCleanup` MUST clear `zonesTracking`; otherwise next tick re-processes stale zones.

## Testing

### `modules/world/world_zone_test.go`
- `TestServerAddLocTracksZone` — after `s.AddLoc(loc)`, `len(s.zonesTracking)==1`.
- `TestServerAddObjRoutesByCoord` — objs at two different zones land in two different Zone instances.
- `TestServerChangeObjPassesCurrentTick` — mock obj, `s.currentTick=42`, call `ChangeObj`; `obj.LastChange==42`.
- `TestServerDispatchersTrackOncePerZone` — call 3 mutations on the same zone; `zonesTracking` set contains exactly 1 entry.

### `modules/world/player_zone_test.go`
- `TestUpdateZonesSendsPartialEnclosedForActiveZone` — player at (3094, 3106), ActiveZones populated, one zone with a LocAddChange event; `updateZones()` emits OpUpdateZonePartialEnclosed with correct zone-relative header and shared payload.
- `TestUpdateZonesFullFollowsOnFirstLoad` — first call of `updateZones` for a new zone emits OpUpdateZoneFullFollows. Second call does NOT.
- `TestUpdateZonesUnloadsDroppedZones` — populate LoadedZones; call with empty ActiveZones; LoadedZones becomes empty.
- `TestWriteFullFollowsReplaysActiveLocs` — zone has a dynamic Loc; FullFollows emits PartialFollows wrapper + OpLocAddChange.
- `TestWriteFullFollowsSkipsThisTickTransitions` — loc has `LastLifecycleTick == currentTick`; skipped.
- `TestPartialFollowsFiltersByReceiver` — zone has Follows event with `ReceiverID=7`; `p.slot=3` → skipped; `p.slot=7` → included.
- `TestSendZoneNestedUnknownOpcodePanics` — `sendZoneNested(p, []byte{255})` panics.

### `modules/world/tick_zone_test.go`
- `TestProcessZonesComputesShared` — 2 tracked zones with Enclosed events; after `processZones`, both `Shared()` non-nil.
- `TestProcessCleanupResetsAndClearsTracking` — after `processCleanup`, tracked zones have `Shared()==nil` and `len(zonesTracking)==0`.

### `pkg/buildarea/buildarea_test.go` (extend)
- `TestRebuildPopulatesActiveZones` — Rebuild at (3094, 3106, 100). Expect `len(ActiveZones) == 169` (13×13 grid from the existing `for dx := -6; dx <= 6` loop).

### `pkg/coordgrid/coordgrid_test.go` (extend)
- `TestZoneIndexPacksAndUnpacks` — (3094, 3106, 0) roundtrips to tile-SW (3088, 3104, 0).
- `TestZoneIndexDistinguishesLevels` — same x/z at different levels → different indexes.

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `go test -race ./...` passes.
3. `go vet ./...` clean.
4. Only the 4 remaining `Player.update*` stubs are gone: `updateZones` now real; `updateInvs`, `updateStats`, `updateAfkZones` already real from 4a. No `update*` stubs remain in `modules/world/player.go` or `modules/world/afkzone.go`.
5. Zero regressions in existing 4a / 4b-1-3 tests.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/coordgrid/coordgrid.go` | +15 |
| `pkg/coordgrid/coordgrid_test.go` | +30 |
| `pkg/zone/map.go` | -8 (remove duplicates) |
| `pkg/zone/map_test.go` | migrate 1 test to coordgrid |
| `pkg/io/protocol/game/server/prot.go` | +12 |
| `pkg/buildarea/buildarea.go` | +5 |
| `pkg/buildarea/buildarea_test.go` | +25 |
| `modules/world/server.go` | +8 |
| `modules/world/world_zone.go` | ~100 |
| `modules/world/world_zone_test.go` | ~120 |
| `modules/world/player.go` | +35 (updateZones) |
| `modules/world/player_zone.go` | ~100 |
| `modules/world/player_zone_test.go` | ~150 |
| `modules/world/tick.go` | +18 |
| `modules/world/tick_zone_test.go` | ~80 |
| **Total** | **~685** |

Slightly larger than the initial estimate. Keep as one sub-spec — the pieces are tightly coupled.

## Dependencies & Risks

- **pkg/zone (4b-3)** — all Zone methods.
- **pkg/rsbuf (4b-2)** — all 13 encoders + opcode constants.
- **pkg/entity (4b-1)** — Loc, Obj, Lifecycle.
- **pkg/coordgrid** — needs `ZoneIndex`/`UnpackZoneIndex` added.
- **No risk of breaking 4a wire behavior** — new opcodes are additive; existing top-level opcodes untouched.
- **Test-fixture complexity**: several tests need a full Player setup via `newTestPlayer` + a Server with `zoneMap`. Follows existing patterns from `player_npc_test.go`.

## Open Simplifications (Documented TODOs)

- Private drops filter on `p.slot`, not `hash64`. (4b-1 choice.)
- `writeFullFollows` skips Respawn-lifecycle loc branches (no static loading yet).
- `RevealObj` dispatcher doesn't consult tradeable/members flags.
- `processZones` doesn't call `entity.turn()` (no script-driven lifecycle transitions yet).
- No `Zone.Enter/Leave` wiring — Player/Npc zone membership for `ZoneGrid` flagging deferred.

All flagged as `// TODO(beyond-4b):` in code.
