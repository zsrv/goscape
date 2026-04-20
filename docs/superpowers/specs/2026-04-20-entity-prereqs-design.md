# Sub-spec 4b-1: Entity Prerequisites — Design

**Status:** Draft → ready for plan
**Scope:** New `pkg/entity/` package (Entity, NonPathing, Loc, Obj, Lifecycle) + `coordgrid.PackZoneCoord`. Prerequisite types for sub-specs 4b-2/3/4.
**Out of scope:** Zone subsystem (4b-3), zone-packet encoders (4b-2), `Player.updateZones` wiring (4b-4).

---

## Goal

Port the core non-pathing entity types from the rs-server-225 Go project (with normalization) into a new `pkg/entity/` package. Add the one missing coordinate helper (`PackZoneCoord`) needed by zone-nested packet encoders. Ship working, tested types with **no** integration into `modules/world/` — those callers arrive in 4b-4.

After this sub-spec, downstream work will be able to:
- Construct `*entity.Loc` and `*entity.Obj` with the correct packed-info / visibility semantics.
- Ask any entity whether it is currently alive at a given tick (`CheckLifecycle`) or transitioning this tick (`UpdateLifecycle`).
- Pack a world-absolute coordinate into a zone-local byte with `coordgrid.PackZoneCoord`.

## Architecture

Single new package, no existing-package edits beyond `coordgrid`. Five tiny files in `pkg/entity/`, each one focused on one type:

```
pkg/entity/
├── lifecycle.go     Lifecycle enum (3 values)
├── entity.go        Entity base struct + tick-based lifecycle helpers
├── nonpathing.go    Embed-only marker struct
├── loc.go           Loc (static or dynamic scenery)
└── obj.go           Obj (ground item)
```

Types have **no** zone awareness, **no** network code, **no** tick-loop dependency beyond passing `int currentTick` through helper args. All methods on pointer receivers so future callers can mutate in place without copying.

## Components

### `pkg/entity/lifecycle.go`

```go
package entity

// Lifecycle describes how a non-pathing entity comes into and goes out of
// existence. Locs and Objs both use this.
type Lifecycle int

const (
	LifecycleForever Lifecycle = iota // statics — never despawn
	LifecycleRespawn                  // engine-added; comes back after a timer
	LifecycleDespawn                  // script-added; goes away after a timer
)
```

### `pkg/entity/entity.go`

```go
package entity

// Entity is the shared base for non-pathing world entities (Loc, Obj).
// The spawn pose (Level/X/Z/Width/Length) is fixed at construction; the
// lifecycle fields advance as the tick counter does.
type Entity struct {
	Level, X, Z, Width, Length int
	Lifecycle                  Lifecycle

	LifecycleTick     int // tick on which the next transition fires
	LastLifecycleTick int // tick on which the last transition fired
}

// NewEntity constructs the immutable portion of an Entity.
func NewEntity(level, x, z, width, length int, lc Lifecycle) Entity {
	return Entity{
		Level: level, X: x, Z: z, Width: width, Length: length,
		Lifecycle: lc,
	}
}

// UpdateLifecycle reports whether the given tick exactly matches the scheduled
// transition AND this entity is not a static. Zone code uses this to decide
// when to fire a spawn/despawn zone event.
func (e *Entity) UpdateLifecycle(tick int) bool {
	return e.LifecycleTick == tick && e.Lifecycle != LifecycleForever
}

// CheckLifecycle reports whether this entity is currently in the world at
// `tick`. Statics are always alive; Respawn entities are alive once their
// respawn tick has passed; Despawn entities are alive until their despawn
// tick has passed.
func (e *Entity) CheckLifecycle(tick int) bool {
	switch e.Lifecycle {
	case LifecycleForever:
		return true
	case LifecycleRespawn:
		return e.LifecycleTick < tick
	case LifecycleDespawn:
		return e.LifecycleTick > tick
	default:
		return false
	}
}

// SetLifecycle schedules the next transition at `transitionTick`, recording
// `currentTick` as the tick the transition was scheduled on.
func (e *Entity) SetLifecycle(transitionTick, currentTick int) {
	e.LifecycleTick = transitionTick
	e.LastLifecycleTick = currentTick
}
```

Deliberate change vs rs-server-225: `SetLifecycle` takes `currentTick int` explicitly; rs-server-225 had a broken `LastLifecycleTick = 0 // TODO: needs to be World.currentTick`.

### `pkg/entity/nonpathing.go`

```go
package entity

// NonPathing is the shared concrete base for entities that don't walk —
// Locs and Objs. Exists to give zone code a single pointer type
// (`*NonPathing`) that can key into per-entity event tracking maps.
type NonPathing struct {
	Entity
}
```

Dropped rs-server-225's empty `ResetEntity(respawn bool)` method — no caller in 4b-1 scope; can reintroduce under 4b-3 if script-driven respawn needs it.

### `pkg/entity/loc.go`

```go
package entity

// Loc is a scenery object: a door, a tree, a tile trap. Its type/shape/angle
// are packed into a single 32-bit Info word for wire-efficient comparison
// between the current state and the cache-loaded base state.
type Loc struct {
	NonPathing
	Info int
}

// NewLoc constructs a Loc at (level, x, z) with the given footprint and
// packed rendering fields. Returns a pointer so callers can mutate Info in
// place (shape changes, angle changes) without re-allocating.
func NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc {
	l := &Loc{Info: packLocInfo(typ, shape, angle)}
	l.Entity = NewEntity(level, x, z, width, length, lc)
	return l
}

// packLocInfo combines the three render fields into a single int using the
// bit layout shared with the TS reference: [type:14][shape:5][angle:2].
func packLocInfo(typ, shape, angle int) int {
	return (typ & 0x3FFF) | (shape&0x1F)<<14 | (angle&0x3)<<19
}

func (l *Loc) Type() int  { return l.Info & 0x3FFF }
func (l *Loc) Shape() int { return (l.Info >> 14) & 0x1F }
func (l *Loc) Angle() int { return (l.Info >> 19) & 0x3 }
```

### `pkg/entity/obj.go`

```go
package entity

// ObjReveal is the number of ticks a private drop stays private before
// becoming visible to all players. Mirrors TS `Obj.REVEAL = 100`.
const ObjReveal = 100

// Obj is a ground item.
type Obj struct {
	NonPathing

	// Construction properties.
	Type  int // ObjType id
	Count int // stack size

	// Runtime state.
	ReceiverID int // -1 = public; else the owning player's slot
	Reveal     int // tick countdown until OBJ_REVEAL fires; -1 if already public
	LastChange int // last tick Count was modified; -1 if never
}

// NewObj constructs a 1×1 private-to-the-dropper ground item. The caller is
// responsible for setting ReceiverID and Reveal when the item drops; the
// zero-state is "already-public, never changed".
func NewObj(level, x, z int, lc Lifecycle, typ, count int) *Obj {
	o := &Obj{
		Type: typ, Count: count,
		ReceiverID: -1, Reveal: -1, LastChange: -1,
	}
	o.Entity = NewEntity(level, x, z, 1, 1, lc)
	return o
}
```

`ReceiverID` is `int` (slot) rather than `hash64 uint64`. Rationale: matches rs-server-225, avoids introducing a new `hash64` field on Player for this sub-spec. 4b-3 will revisit if TS parity turns out to be load-bearing.

### `pkg/coordgrid/coordgrid.go`

Append one helper:

```go
// PackZoneCoord packs the zone-local low bits of a world-absolute (x, z) into
// a single byte: (x&7)<<4 | (z&7). Used inside every zone-nested packet
// encoder to identify which tile within the 8×8 zone an event refers to.
func PackZoneCoord(x, z int) byte {
	return byte((x&0x7)<<4 | (z & 0x7))
}
```

No behavioural changes to existing functions.

## Data Flow

None — 4b-1 is pure data types. No inbound/outbound information crosses any subsystem boundary in this sub-spec.

## Error Handling

None required. All constructors accept raw ints; bitfield overflow is silently masked (documented behaviour). No I/O, no external systems.

## Testing

### `pkg/entity/entity_test.go`
- `TestCheckLifecycleForever` — `LifecycleForever` entity is alive at tick 0, 10, 10000 regardless of `LifecycleTick`.
- `TestCheckLifecycleRespawn` — alive only when `tick > LifecycleTick`; boundary: equal → dead.
- `TestCheckLifecycleDespawn` — alive only when `tick < LifecycleTick`; boundary: equal → dead.
- `TestUpdateLifecycleMatchesOnlyOnTickAndNotForever` — fires at exact match for Respawn/Despawn; never for Forever.
- `TestSetLifecycleRecordsBothTicks` — `SetLifecycle(100, 50)` sets LifecycleTick=100 AND LastLifecycleTick=50.

### `pkg/entity/loc_test.go`
- `TestLocInfoRoundTrip` — `NewLoc(0,0,0,1,1,LifecycleForever, 5000, 10, 3)` yields Type()==5000, Shape()==10, Angle()==3.
- `TestLocInfoBoundaryValues` — type=0x3FFF, shape=0x1F, angle=0x3 all roundtrip losslessly.
- `TestLocInfoOverflowMasks` — type=0x4001 masks to 1; shape=0x20 masks to 0; angle=0x4 masks to 0. (Silent masking is documented, not a bug.)

### `pkg/entity/obj_test.go`
- `TestObjDefaults` — ReceiverID/Reveal/LastChange all default to -1; Width/Length both 1.
- `TestObjRevealConstantValue` — `ObjReveal == 100`.

### `pkg/coordgrid/coordgrid_test.go` (extend existing)
- `TestPackZoneCoordCorners` — (0,0)→0x00, (7,7)→0x77.
- `TestPackZoneCoordWorldAbsolute` — (3094, 3106) → `(3094&7=6)<<4 | (3106&7=2) == 0x62`.
- `TestPackZoneCoordDiscardsHighBits` — (3200, 3200) == (0, 0).

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` is clean.
3. `pkg/entity/` has exactly 5 production files + 3 test files.
4. `coordgrid.PackZoneCoord` is importable and tested.
5. Zero changes to any file outside `pkg/entity/` and `pkg/coordgrid/`.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/entity/lifecycle.go` | ~12 |
| `pkg/entity/entity.go` | ~55 |
| `pkg/entity/nonpathing.go` | ~8 |
| `pkg/entity/loc.go` | ~55 |
| `pkg/entity/obj.go` | ~35 |
| `pkg/entity/entity_test.go` | ~80 |
| `pkg/entity/loc_test.go` | ~50 |
| `pkg/entity/obj_test.go` | ~25 |
| `pkg/coordgrid/coordgrid.go` | +5 |
| `pkg/coordgrid/coordgrid_test.go` | +30 |
| **Total** | **~355** |

## Dependencies & Risks

- **rs-server-225 files are read-only reference** — cp + sed pattern described in project notes. Here we don't literally `cp` because the normalizations (pointer returns, removed `ResetEntity`, explicit `currentTick` arg) are enough that rewriting by hand is clearer than scripted transforms.
- **No risk of breaking existing code** — all changes are additive; no existing code imports `pkg/entity/`.
- **Bitfield overflow is silent** — `packLocInfo(0x4001, ...)` masks to 1 with no warning. This matches TS behaviour and is tested explicitly so future refactors don't "fix" it.

## Deferred to Later Sub-specs

- **4b-2:** Zone packet encoders that read Loc/Obj fields to build wire bytes.
- **4b-3:** `Zone`, `ZoneMap`, `ZoneGrid`, `ZoneEvent`, event queueing, `World.addLoc/delLoc/addObj/delObj`.
- **4b-4:** `Player.updateZones`, `BuildArea.RebuildZones`, `processZones` tick phase, integration.
- **Beyond 4b:** Static loc/obj loading from cache maps; script-driven spawn/despawn lifecycle tick advancement.
