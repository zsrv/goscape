# Sub-spec 4b-3: Zone Subsystem — Design

**Status:** Draft → ready for plan
**Scope:** `pkg/zone/` — Zone + ZoneEvent + ZoneMap + ZoneGrid + event-queueing methods. Pure data + composition; no Player/World integration.
**Out of scope:** `Player.updateZones`, `writeFullFollows` stateful replay, `processZones` tick phase, `BuildArea.RebuildZones`, static loc loading — all in 4b-4 or later.

---

## Goal

Give 4b-4 a Zone subsystem it can drive. After this sub-spec, callers can:

- Look up a zone by `(level, worldX, worldZ)` via `ZoneMap.Get`.
- Mutate it: `zone.AddLoc`, `zone.RemoveObj`, `zone.AnimMap`, etc. Each call encodes the zone-nested packet (via 4b-2's encoders), stores the bytes in a `ZoneEvent`, and maintains the per-zone active-entity list.
- Compose the per-tick shared buffer with `zone.ComputeShared()`.
- Read out the shared bytes and the per-event list for 4b-4's per-player delivery pass.
- Reset per-tick state with `zone.Reset()`.

## Architecture

New package `pkg/zone/` with 4 production files. Imports `pkg/entity` (4b-1 Loc/Obj) and `pkg/rsbuf` (4b-2 encoders + opcode constants). No external runtime dependencies. No imports from `modules/world` — that direction comes later when 4b-4 wires `Server` to `ZoneMap`.

```
pkg/zone/
├── event.go    ZoneEvent + ZoneEventType (2 values)
├── grid.go     ZoneGrid — per-level 2D bitmap, vendored from rs-server-225
├── map.go      ZoneMap — lazy zone lookup by (level, worldX, worldZ)
└── zone.go     Zone — active entities + event queue + composed shared buffer
```

## Components

### 1. `pkg/zone/event.go`

```go
package zone

// ZoneEventType distinguishes events that are broadcast to every observer
// from those that are routed to a specific recipient.
type ZoneEventType int

const (
	ZoneEventEnclosed ZoneEventType = iota // shared across all observers
	ZoneEventFollows                       // per-receiver; filtered at write time
)

// ZoneEvent carries one already-encoded zone-nested message. Bytes is
// exactly [opcode_byte, ...payload] ready to concat into the shared buffer
// (Enclosed) or write per-player inside a PartialFollows wrapper (Follows).
type ZoneEvent struct {
	Type       ZoneEventType
	ReceiverID int    // -1 = public (applies to both types; FOLLOWS filters, ENCLOSED ignores)
	Bytes      []byte
}
```

### 2. `pkg/zone/zone.go`

```go
package zone

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// PublicReceiver is the sentinel ReceiverID for events visible to all
// players. Mirrors TS Obj.NO_RECEIVER = -1n.
const PublicReceiver = -1

// Zone is an 8x8 tile region of the world. It owns the active dynamic
// entities inside the region, a per-tick event queue, and a composed
// shared buffer of every Enclosed event (built by ComputeShared).
type Zone struct {
	Index       int
	X, Z, Level int // X and Z in zone units (tile >> 3)

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int // entity pointer → indexes into events

	shared []byte
}

// New constructs a zone for the given packed index and (level, zoneX, zoneZ).
// Callers typically reach Zones through ZoneMap.Get rather than calling this.
func New(index, level, x, z int) *Zone

// Reset clears per-tick state: events, entityEvents, shared. Called from
// processCleanup at the end of each tick. Locs and Objs persist.
func (z *Zone) Reset()

// Shared returns the composed enclosed-event buffer for the current tick,
// or nil if no Enclosed events were queued.
func (z *Zone) Shared() []byte

// Events returns the per-tick event queue (read-only view for callers that
// need to iterate Follows events per-player).
func (z *Zone) Events() []ZoneEvent

// ComputeShared concatenates the Bytes of every Enclosed event into the
// zone's shared buffer. Must be called once per tick before any per-player
// delivery pass. Safe to call on a zone with zero Enclosed events — sets
// shared to nil.
func (z *Zone) ComputeShared()

// ---- loc mutations ----

// AddLoc activates a dynamic (DESPAWN-lifecycle) loc and queues a
// LOC_ADD_CHANGE enclosed event. For non-DESPAWN locs, adds the event only
// (used by script-driven loc changes on statics).
func (z *Zone) AddLoc(loc *entity.Loc)

// ChangeLoc emits a LOC_ADD_CHANGE event against an existing active loc.
// Used when a loc's type/shape/angle changed.
func (z *Zone) ChangeLoc(loc *entity.Loc)

// RemoveLoc removes a dynamic loc from Locs (and purges any pending events
// for it), then emits a LOC_DEL enclosed event.
func (z *Zone) RemoveLoc(loc *entity.Loc)

// AnimLoc queues a LOC_ANIM enclosed event against an existing loc.
// Does NOT modify the Locs slice.
func (z *Zone) AnimLoc(loc *entity.Loc, seq int)

// MergeLoc queues a LOC_MERGE enclosed event.
// Deltas (east/south/west/north) are absolute tile coords; the encoder subtracts src.
func (z *Zone) MergeLoc(
	loc *entity.Loc,
	playerSlot, startCycle, endCycle int,
	east, south, west, north int,
)

// ---- obj mutations ----

// AddObj adds an obj to the zone's Objs list (if DESPAWN lifecycle), sets
// isActive, and queues either an ENCLOSED OBJ_ADD (public) or FOLLOWS
// OBJ_ADD (private drop) event depending on receiverID.
func (z *Zone) AddObj(obj *entity.Obj, receiverID int)

// ChangeObj updates obj.Count and queues a FOLLOWS OBJ_COUNT event routed
// to the current receiver.
func (z *Zone) ChangeObj(obj *entity.Obj, oldCount, newCount, currentTick int)

// RemoveObj removes a dynamic obj from Objs, purges pending events, and
// queues an appropriate OBJ_DEL (ENCLOSED if public; FOLLOWS if private).
// Skips event queuing if this same tick already transitioned the obj's
// lifecycle — matches TS's lastLifecycleTick check.
func (z *Zone) RemoveObj(obj *entity.Obj, currentTick int)

// RevealObj transitions a private drop to public and queues an ENCLOSED
// OBJ_REVEAL event with the ORIGINAL receiver (so they see the transition).
// Simplified vs TS: does not consult ObjType tradeable/members flags.
// TODO(beyond-4b): wire tradeability gating.
func (z *Zone) RevealObj(obj *entity.Obj, receiverSlot int)

// ---- non-entity events ----

// AnimMap queues a MAP_ANIM enclosed event at a tile.
func (z *Zone) AnimMap(x, zCoord, spotanim, height, delay int)

// MapProjAnim queues a MAP_PROJANIM enclosed event for a projectile
// traveling (srcX, srcZ) → (dstX, dstZ). Deltas are computed here.
func (z *Zone) MapProjAnim(
	srcX, srcZ, dstX, dstZ int,
	target, spotanim, srcHeight, dstHeight int,
	startDelay, endDelay, peak, arc int,
)

// ---- private ----

func (z *Zone) queueEvent(np *entity.NonPathing, e ZoneEvent)
func (z *Zone) clearQueuedEvents(np *entity.NonPathing)
```

**Implementation notes:**
- Each mutation method uses `packet.NewPacket(nil)`, prepends the opcode constant from `pkg/rsbuf` (e.g., `rsbuf.ZoneOpLocAddChange`), calls the matching `rsbuf.EncodeLoc*`, then stores `buf.Data` as `ZoneEvent.Bytes`.
- `queueEvent` records the event's index in `entityEvents[np]` so `clearQueuedEvents` can purge it before a subsequent event for the same entity.
- `clearQueuedEvents` sets `events[i].Bytes = nil` for each tracked index (tombstone — cheaper than splicing); `ComputeShared` skips nil-Bytes entries.

### 3. `pkg/zone/map.go`

```go
package zone

// ZoneIndex packs (worldX, worldZ, level) into a single int using the same
// bit layout as the TS reference:
//   zone_x = x >> 3, zone_z = z >> 3
//   index = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int

// UnpackIndex reverses ZoneIndex. Returns tile-unit coordinates (zone_x<<3).
func UnpackIndex(index int) (worldX, worldZ, level int)

type ZoneMap struct {
	zones map[int]*Zone
	grids map[int]*ZoneGrid
}

func NewZoneMap() *ZoneMap
func (m *ZoneMap) Get(level, worldX, worldZ int) *Zone // creates if missing
func (m *ZoneMap) GetByIndex(index int) *Zone          // creates if missing
func (m *ZoneMap) Grid(level int) *ZoneGrid            // creates if missing
func (m *ZoneMap) ZoneCount() int
func (m *ZoneMap) LocCount() int
func (m *ZoneMap) ObjCount() int
```

### 4. `pkg/zone/grid.go` — vendored from rs-server-225

Direct port of `rs-server-225/engine/zone/grid.go` with one rename: `Grid` → `ZoneGrid` and `NewGrid` → `NewZoneGrid`. Per-level 2D bitmap used later for pathfinding/aggro "zone contains player" lookups.

```go
const (
	ZoneGridSize        = 2048
	zoneGridIntBits     = 5
	zoneGridIntBitsFlag = (1 << zoneGridIntBits) - 1
	ZoneGridDefaultSize = ZoneGridSize * (ZoneGridSize >> zoneGridIntBits)
)

type ZoneGrid struct {
	grid []int32
}

func NewZoneGrid() ZoneGrid
func (g *ZoneGrid) Flag(zoneX, zoneZ int)
func (g *ZoneGrid) Unflag(zoneX, zoneZ int)
func (g *ZoneGrid) IsFlagged(zoneX, zoneZ, radius int) bool
```

`NewZoneGrid()` defaults the internal slice size to `ZoneGridDefaultSize`. rs-server-225 took size as a param — we collapse that since the size is fixed.

## Data Flow

```
Caller (4b-4)
    │
    │  world.addLoc(loc) ─►  zoneMap.Get(level, x, z).AddLoc(loc)
    ▼
Zone.AddLoc
    │
    │  1. rsbuf.EncodeLocAddChange(buf, coord, shape, angle, type)
    │  2. event.Bytes = [ZoneOpLocAddChange, ...buf.Data]
    │  3. events = append(events, event)
    │  4. entityEvents[loc] = append(..., idx)
    │  5. if loc.Lifecycle == Despawn: Locs = append(Locs, loc)
    ▼
                       ... (more mutations across the tick) ...
                                                       │
                                                       ▼
                                         (once per tick, before delivery)
                                         Zone.ComputeShared()
                                                       │
                                                       │  shared = concat(e.Bytes for e in events if Enclosed)
                                                       ▼
(per-player delivery — lives in 4b-4, reads Zone.Shared() / Zone.Events())
                                                       │
                                                       ▼
                                         (end of tick)
                                         Zone.Reset()  (clears events, entityEvents, shared)
```

## Error Handling

None. Zone methods are pure-data mutators; no I/O, no error paths. `ZoneMap.Get` never returns nil (zones are created on first access). Encoder calls in `pkg/rsbuf` don't return errors.

## Testing

Focused on observable state changes (event queue, entity lists, shared buffer contents), not byte-level wire details (covered by 4b-2 tests).

### `pkg/zone/event_test.go` (~20 LOC)
- `TestZoneEventTypeValues` — Enclosed=0, Follows=1.

### `pkg/zone/map_test.go` (~80 LOC)
- `TestZoneIndexRoundTrip` — (3094, 3106, 0) packs and unpacks to (3088, 3104, 0) at tile-granularity (zone-aligned).
- `TestZoneMapGetCreatesOnce` — first `Get(0, 3094, 3106)` creates; second returns the same pointer.
- `TestZoneMapGridPerLevel` — `Grid(0)` ≠ `Grid(1)`.
- `TestZoneMapCountsAggregateAcrossZones` — add 2 Locs to zone A, 3 Objs to zone B; `LocCount()==2`, `ObjCount()==3`, `ZoneCount()==2`.

### `pkg/zone/grid_test.go` (~35 LOC)
- `TestZoneGridFlagUnflag` — flag (100, 200), IsFlagged(100, 200, 0) is true; unflag, false.
- `TestZoneGridRadiusSearch` — flag (100, 200); IsFlagged(105, 205, radius=6) true; radius=4 false.

### `pkg/zone/zone_test.go` (~220 LOC)
- `TestAddLocQueuesEnclosedLocAddChange` — after `AddLoc(loc)`, events[0].Type==Enclosed, events[0].Bytes[0] == ZoneOpLocAddChange.
- `TestAddLocDespawnAppendsToLocs` — dynamic loc → ends up in Locs.
- `TestAddLocRespawnDoesNotAppendToLocs` — static loc lifecycle → no Locs append.
- `TestChangeLocEmitsLocAddChange` — Bytes[0] == ZoneOpLocAddChange.
- `TestRemoveLocEmitsLocDelAndPurges` — after AddLoc + RemoveLoc in the same tick: Locs empty, events has LOC_ADD_CHANGE (tombstoned) + LOC_DEL; after ComputeShared, shared only contains LOC_DEL bytes (purged tombstone skipped).
- `TestAnimLocDoesNotTouchLocs` — AnimLoc fires event but Locs unchanged.
- `TestMergeLocEmitsLocMerge` — Bytes[0] == ZoneOpLocMerge.
- `TestAddObjPublicIsEnclosed` — receiverID=-1 → Enclosed OBJ_ADD.
- `TestAddObjPrivateIsFollows` — receiverID=5 → Follows OBJ_ADD, ReceiverID field set to 5.
- `TestChangeObjEmitsFollowsObjCount` — Follows OBJ_COUNT with newCount visible in Bytes.
- `TestRemoveObjPurgesPendingAdd` — AddObj + RemoveObj same tick: Objs empty; ComputeShared omits the add bytes (tombstoned).
- `TestRemoveObjSkipsEventIfLifecycleTransitionedThisTick` — mimics TS `lastLifecycleTick === currentTick` skip.
- `TestAnimMapEnclosed` — ZoneOpMapAnim enclosed event.
- `TestMapProjAnimEnclosed` — ZoneOpMapProjAnim enclosed event.
- `TestComputeSharedConcatsOnlyEnclosed` — 3 Enclosed + 2 Follows → Shared() = concat of 3 Enclosed's Bytes.
- `TestComputeSharedNilOnEmpty` — no events → Shared()==nil.
- `TestResetClearsEventsEntityEventsShared` — Reset wipes all three.
- `TestAnimLocAndRemoveLocSharedOrder` — queue AnimLoc then RemoveLoc → Shared has anim bytes first, then del bytes (queue-order preserved).

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
3. `pkg/zone/` has exactly 4 production files + 4 test files.
4. Zero changes outside `pkg/zone/`.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/zone/event.go` | ~30 |
| `pkg/zone/zone.go` | ~280 |
| `pkg/zone/map.go` | ~65 |
| `pkg/zone/grid.go` | ~60 |
| `pkg/zone/event_test.go` | ~20 |
| `pkg/zone/zone_test.go` | ~240 |
| `pkg/zone/map_test.go` | ~80 |
| `pkg/zone/grid_test.go` | ~35 |
| **Total** | **~810** |

## Dependencies & Risks

- **`pkg/entity` (4b-1)** — Loc, Obj, NonPathing, Lifecycle.
- **`pkg/rsbuf` (4b-2)** — all 11 zone encoders + opcode constants.
- **`pkg/coordgrid`** — `PackZoneCoord`.
- **No risk to existing code** — 100% additive.
- **LinkList vs slice tradeoff**: Zone.Locs/Objs use plain slices. Removal is O(n) per entity but zone caps at 128 locs / 129 objs, so worst-case ~128 pointer compares per removal. Acceptable.
- **Tombstoning for clearQueuedEvents**: setting `events[i].Bytes = nil` and skipping nil in ComputeShared is O(events-per-tick) not O(splice). Matches TS semantics.

## Simplifications vs TS Reference

| TS feature | 4b-3 approach | Rationale |
|---|---|---|
| `Set<ZoneEvent>` backing | `[]ZoneEvent` slice | Set semantics unused (no dedup needed); indexable for tombstone purging |
| `DoublyLinkList<Loc>` | `[]*entity.Loc` | Zone caps bound iteration cost; simpler |
| Obj eviction at `OBJS=129` in `AddObj` | Not yet; `// TODO` | No production usage until PvM drops |
| `enter/leave` for Player/Npc tracking | Not in 4b-3 | Player/Npc integration is 4b-4 concern |
| `ObjType.tradeable`/members check in `RevealObj` | Not yet; `// TODO` | Needs obj-config package wiring |
| `writeFullFollows` replay | Not in 4b-3 | Moved to 4b-4 where Player state is accessible |
| `Zone.receiver64 bigint` (name hash) | `ReceiverID int` (slot) | Simpler; matches rs-server-225 |

## Deferred to 4b-4

- `processZones` tick phase that iterates `zonesTracking` and calls `ComputeShared`.
- `Server` getting a `zoneMap *zone.ZoneMap` field + `zonesTracking map[*zone.Zone]struct{}` set.
- `Server.AddLoc/RemoveLoc/...` dispatchers that look up the right Zone and call its method, then `TrackZone(zone)`.
- `Player.updateZones` — iterates `BuildArea.ActiveZones`, writes the three outer packets per zone.
- `writeFullFollows` replay logic.
- `BuildArea.RebuildZones` — populates the 7×7 zone grid around origin.
- `Zone.Enter/Leave` — Player/Npc membership tracking with ZoneGrid flag/unflag side effects.
