# Sub-spec 4b-1: Entity Prerequisites — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a new `pkg/entity/` package containing `Entity`, `NonPathing`, `Loc`, `Obj`, and the `Lifecycle` enum, plus add `coordgrid.PackZoneCoord`. Pure data types — no callers yet.

**Architecture:** 5 new files under `pkg/entity/` with focused, single-responsibility content. One additive edit to `pkg/coordgrid/`. All methods on pointer receivers. TDD per task.

**Tech Stack:** Go 1.26. No external dependencies beyond the existing coordgrid package.

**Build prefix:** All `go` commands below use `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.

**Commit policy:** `git commit --no-gpg-sign`. Conventional-commits style.

**Spec reference:** `docs/superpowers/specs/2026-04-20-entity-prereqs-design.md`.

---

## File Structure

**Create:**
- `pkg/entity/lifecycle.go` — Lifecycle enum (3 values)
- `pkg/entity/entity.go` — Entity struct + lifecycle helpers
- `pkg/entity/nonpathing.go` — NonPathing marker struct (embeds Entity)
- `pkg/entity/loc.go` — Loc with packed Info word
- `pkg/entity/obj.go` — Obj with receiver/reveal/count
- `pkg/entity/entity_test.go` — lifecycle helper tests
- `pkg/entity/loc_test.go` — Info round-trip tests
- `pkg/entity/obj_test.go` — Obj default-value tests

**Modify:**
- `pkg/coordgrid/coordgrid.go` — add `PackZoneCoord`
- `pkg/coordgrid/coordgrid_test.go` — add tests for `PackZoneCoord`

---

## Task 1: Lifecycle enum + Entity base + tests

Foundation for both Loc and Obj. Includes the interesting testable behaviour (lifecycle helpers).

**Files:**
- Create: `pkg/entity/lifecycle.go`
- Create: `pkg/entity/entity.go`
- Create: `pkg/entity/entity_test.go`

- [ ] **Step 1.1: Write the failing tests**

Create `pkg/entity/entity_test.go`:

```go
package entity

import "testing"

func TestCheckLifecycleForever(t *testing.T) {
	e := Entity{Lifecycle: LifecycleForever, LifecycleTick: 999}
	for _, tick := range []int{0, 100, 1000, 1_000_000} {
		if !e.CheckLifecycle(tick) {
			t.Errorf("Forever should be alive at tick %d; got false", tick)
		}
	}
}

func TestCheckLifecycleRespawn(t *testing.T) {
	e := Entity{Lifecycle: LifecycleRespawn, LifecycleTick: 100}
	if e.CheckLifecycle(100) {
		t.Error("Respawn should be dead at the exact respawn tick (boundary)")
	}
	if e.CheckLifecycle(99) {
		t.Error("Respawn should be dead before the respawn tick")
	}
	if !e.CheckLifecycle(101) {
		t.Error("Respawn should be alive after the respawn tick")
	}
}

func TestCheckLifecycleDespawn(t *testing.T) {
	e := Entity{Lifecycle: LifecycleDespawn, LifecycleTick: 100}
	if !e.CheckLifecycle(99) {
		t.Error("Despawn should be alive before the despawn tick")
	}
	if e.CheckLifecycle(100) {
		t.Error("Despawn should be dead at the exact despawn tick (boundary)")
	}
	if e.CheckLifecycle(101) {
		t.Error("Despawn should be dead after the despawn tick")
	}
}

func TestUpdateLifecycleMatchesOnlyOnTickAndNotForever(t *testing.T) {
	for _, lc := range []Lifecycle{LifecycleRespawn, LifecycleDespawn} {
		e := Entity{Lifecycle: lc, LifecycleTick: 42}
		if !e.UpdateLifecycle(42) {
			t.Errorf("lifecycle %v: expected fire at exact tick match", lc)
		}
		if e.UpdateLifecycle(41) || e.UpdateLifecycle(43) {
			t.Errorf("lifecycle %v: expected silence off the exact tick", lc)
		}
	}
	// Forever never fires.
	e := Entity{Lifecycle: LifecycleForever, LifecycleTick: 42}
	if e.UpdateLifecycle(42) {
		t.Error("Forever should never fire UpdateLifecycle")
	}
}

func TestSetLifecycleRecordsBothTicks(t *testing.T) {
	var e Entity
	e.SetLifecycle(100, 50)
	if e.LifecycleTick != 100 {
		t.Errorf("LifecycleTick: got %d, want 100", e.LifecycleTick)
	}
	if e.LastLifecycleTick != 50 {
		t.Errorf("LastLifecycleTick: got %d, want 50", e.LastLifecycleTick)
	}
}

func TestNewEntitySetsSpawnFields(t *testing.T) {
	e := NewEntity(2, 3094, 3106, 1, 1, LifecycleRespawn)
	if e.Level != 2 || e.X != 3094 || e.Z != 3106 {
		t.Errorf("position: got (%d,%d,%d), want (2,3094,3106)", e.Level, e.X, e.Z)
	}
	if e.Width != 1 || e.Length != 1 {
		t.Errorf("size: got (%d,%d), want (1,1)", e.Width, e.Length)
	}
	if e.Lifecycle != LifecycleRespawn {
		t.Errorf("lifecycle: got %v, want Respawn", e.Lifecycle)
	}
	if e.LifecycleTick != 0 || e.LastLifecycleTick != 0 {
		t.Error("runtime tick fields should be zero after NewEntity")
	}
}
```

- [ ] **Step 1.2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v`
Expected: FAIL — package doesn't exist yet; `undefined: Entity`, `undefined: Lifecycle*`, etc.

- [ ] **Step 1.3: Implement `lifecycle.go`**

Create `pkg/entity/lifecycle.go`:

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

- [ ] **Step 1.4: Implement `entity.go`**

Create `pkg/entity/entity.go`:

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

// NewEntity constructs the immutable portion of an Entity. Runtime fields
// (LifecycleTick, LastLifecycleTick) start at their zero values.
func NewEntity(level, x, z, width, length int, lc Lifecycle) Entity {
	return Entity{
		Level: level, X: x, Z: z, Width: width, Length: length,
		Lifecycle: lc,
	}
}

// UpdateLifecycle reports whether the given tick exactly matches the
// scheduled transition AND this entity is not a static. Zone code uses
// this to decide when to fire a spawn/despawn zone event.
func (e *Entity) UpdateLifecycle(tick int) bool {
	return e.LifecycleTick == tick && e.Lifecycle != LifecycleForever
}

// CheckLifecycle reports whether this entity is currently in the world at
// `tick`. Statics are always alive; Respawn entities are alive once their
// respawn tick has passed; Despawn entities are alive until their despawn
// tick has passed. Equal-to-transition-tick counts as the "dead" half.
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
// `currentTick` as the tick on which the transition was scheduled.
func (e *Entity) SetLifecycle(transitionTick, currentTick int) {
	e.LifecycleTick = transitionTick
	e.LastLifecycleTick = currentTick
}
```

- [ ] **Step 1.5: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v`
Expected: all 6 tests PASS.

- [ ] **Step 1.6: Run `go vet`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/entity/`
Expected: no output.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/entity/lifecycle.go pkg/entity/entity.go pkg/entity/entity_test.go
git commit --no-gpg-sign -m "feat(entity): add Lifecycle enum and Entity base type

Port of rs-server-225's entity base. Lifecycle {Forever, Respawn, Despawn}
drives CheckLifecycle/UpdateLifecycle on explicit tick args — no global
World dependency. SetLifecycle takes currentTick explicitly to record the
scheduling tick."
```

---

## Task 2: NonPathing marker struct

One-liner wrapper that embeds Entity. Exists so later sub-specs can unify Loc and Obj under a common pointer type.

**Files:**
- Create: `pkg/entity/nonpathing.go`

- [ ] **Step 2.1: Implement NonPathing**

Create `pkg/entity/nonpathing.go`:

```go
package entity

// NonPathing is the shared concrete base for entities that don't walk —
// Locs and Objs. Exists to give zone code a single embedded base that
// future zone-event machinery can key against via interface satisfaction.
type NonPathing struct {
	Entity
}
```

- [ ] **Step 2.2: Build to verify compilation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/entity/`
Expected: exits 0.

- [ ] **Step 2.3: Commit**

```bash
git add pkg/entity/nonpathing.go
git commit --no-gpg-sign -m "feat(entity): add NonPathing marker struct

Empty struct that embeds Entity; shared by Loc and Obj. Gives later zone
code a common embedded base."
```

---

## Task 3: Loc with packed Info word

The interesting part of this task is the bitfield layout — worth its own tests to catch a future 'simplification' that breaks the wire format.

**Files:**
- Create: `pkg/entity/loc.go`
- Create: `pkg/entity/loc_test.go`

- [ ] **Step 3.1: Write the failing tests**

Create `pkg/entity/loc_test.go`:

```go
package entity

import "testing"

func TestLocInfoRoundTrip(t *testing.T) {
	l := NewLoc(0, 3094, 3106, 1, 1, LifecycleForever, 5000, 10, 3)
	if l.Type() != 5000 {
		t.Errorf("Type: got %d, want 5000", l.Type())
	}
	if l.Shape() != 10 {
		t.Errorf("Shape: got %d, want 10", l.Shape())
	}
	if l.Angle() != 3 {
		t.Errorf("Angle: got %d, want 3", l.Angle())
	}
}

func TestLocInfoBoundaryValues(t *testing.T) {
	l := NewLoc(0, 0, 0, 1, 1, LifecycleForever, 0x3FFF, 0x1F, 0x3)
	if l.Type() != 0x3FFF {
		t.Errorf("Type at max: got %d, want %d", l.Type(), 0x3FFF)
	}
	if l.Shape() != 0x1F {
		t.Errorf("Shape at max: got %d, want %d", l.Shape(), 0x1F)
	}
	if l.Angle() != 0x3 {
		t.Errorf("Angle at max: got %d, want %d", l.Angle(), 0x3)
	}
}

func TestLocInfoOverflowSilentlyMasks(t *testing.T) {
	// type=0x4001 is 0b100000000000001 — one bit too wide; should mask to 1.
	// shape=0x20 is 0b100000 — one bit too wide; should mask to 0.
	// angle=0x4 is 0b100 — one bit too wide; should mask to 0.
	l := NewLoc(0, 0, 0, 1, 1, LifecycleForever, 0x4001, 0x20, 0x4)
	if l.Type() != 1 {
		t.Errorf("Type=0x4001 should mask to 1; got %d", l.Type())
	}
	if l.Shape() != 0 {
		t.Errorf("Shape=0x20 should mask to 0; got %d", l.Shape())
	}
	if l.Angle() != 0 {
		t.Errorf("Angle=0x4 should mask to 0; got %d", l.Angle())
	}
}

func TestLocCarriesEntityFields(t *testing.T) {
	l := NewLoc(2, 3094, 3106, 3, 2, LifecycleDespawn, 100, 0, 0)
	if l.Level != 2 || l.X != 3094 || l.Z != 3106 {
		t.Errorf("position: got (%d,%d,%d)", l.Level, l.X, l.Z)
	}
	if l.Width != 3 || l.Length != 2 {
		t.Errorf("size: got (%d,%d), want (3,2)", l.Width, l.Length)
	}
	if l.Lifecycle != LifecycleDespawn {
		t.Errorf("lifecycle: got %v, want Despawn", l.Lifecycle)
	}
}
```

- [ ] **Step 3.2: Run tests — verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestLoc -v`
Expected: FAIL — `undefined: NewLoc`.

- [ ] **Step 3.3: Implement `loc.go`**

Create `pkg/entity/loc.go`:

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
// packed rendering fields. Returns a pointer so callers can mutate Info
// in place (shape changes, angle changes) without re-allocating.
func NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc {
	l := &Loc{Info: packLocInfo(typ, shape, angle)}
	l.Entity = NewEntity(level, x, z, width, length, lc)
	return l
}

// packLocInfo combines the three render fields into a single int using the
// bit layout shared with the TS reference: [type:14][shape:5][angle:2].
// Out-of-range inputs are silently masked.
func packLocInfo(typ, shape, angle int) int {
	return (typ & 0x3FFF) | (shape&0x1F)<<14 | (angle&0x3)<<19
}

// Type returns the ObjType id (bits 0..13).
func (l *Loc) Type() int { return l.Info & 0x3FFF }

// Shape returns the loc shape (bits 14..18).
func (l *Loc) Shape() int { return (l.Info >> 14) & 0x1F }

// Angle returns the loc rotation (bits 19..20).
func (l *Loc) Angle() int { return (l.Info >> 19) & 0x3 }
```

- [ ] **Step 3.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestLoc -v`
Expected: all 4 tests PASS.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/entity/loc.go pkg/entity/loc_test.go
git commit --no-gpg-sign -m "feat(entity): add Loc with packed Info word

Loc wraps NonPathing and stores type/shape/angle in a single int with
[type:14][shape:5][angle:2] bit layout matching the TS reference.
Out-of-range inputs are silently masked (tested explicitly)."
```

---

## Task 4: Obj with receiver/reveal/count

**Files:**
- Create: `pkg/entity/obj.go`
- Create: `pkg/entity/obj_test.go`

- [ ] **Step 4.1: Write the failing tests**

Create `pkg/entity/obj_test.go`:

```go
package entity

import "testing"

func TestObjDefaultsArePublicAndUnmodified(t *testing.T) {
	o := NewObj(0, 3094, 3106, LifecycleDespawn, 995, 100)
	if o.ReceiverID != -1 {
		t.Errorf("ReceiverID: got %d, want -1 (public)", o.ReceiverID)
	}
	if o.Reveal != -1 {
		t.Errorf("Reveal: got %d, want -1", o.Reveal)
	}
	if o.LastChange != -1 {
		t.Errorf("LastChange: got %d, want -1", o.LastChange)
	}
	if o.Type != 995 || o.Count != 100 {
		t.Errorf("Type/Count: got (%d,%d), want (995,100)", o.Type, o.Count)
	}
}

func TestObjIsAlways1x1(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleDespawn, 1, 1)
	if o.Width != 1 || o.Length != 1 {
		t.Errorf("Obj footprint: got (%d,%d), want (1,1)", o.Width, o.Length)
	}
}

func TestObjCarriesEntityFields(t *testing.T) {
	o := NewObj(3, 2000, 3000, LifecycleRespawn, 42, 7)
	if o.Level != 3 || o.X != 2000 || o.Z != 3000 {
		t.Errorf("position: got (%d,%d,%d)", o.Level, o.X, o.Z)
	}
	if o.Lifecycle != LifecycleRespawn {
		t.Errorf("lifecycle: got %v, want Respawn", o.Lifecycle)
	}
}

func TestObjRevealConstantValue(t *testing.T) {
	if ObjReveal != 100 {
		t.Errorf("ObjReveal: got %d, want 100", ObjReveal)
	}
}
```

- [ ] **Step 4.2: Run tests — verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestObj -v`
Expected: FAIL — `undefined: NewObj`, `undefined: ObjReveal`.

- [ ] **Step 4.3: Implement `obj.go`**

Create `pkg/entity/obj.go`:

```go
package entity

// ObjReveal is the number of ticks a private drop stays private before
// automatically becoming visible to all players. Mirrors TS Obj.REVEAL = 100.
const ObjReveal = 100

// Obj is a ground item — an entry in the game-world ground-layer inventory.
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

// NewObj constructs a 1×1 ground item with public visibility by default
// (ReceiverID -1, Reveal -1). Callers that drop a private item must set
// ReceiverID and Reveal after construction.
func NewObj(level, x, z int, lc Lifecycle, typ, count int) *Obj {
	o := &Obj{
		Type: typ, Count: count,
		ReceiverID: -1, Reveal: -1, LastChange: -1,
	}
	o.Entity = NewEntity(level, x, z, 1, 1, lc)
	return o
}
```

- [ ] **Step 4.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestObj -v`
Expected: 4/4 PASS.

- [ ] **Step 4.5: Full entity package test + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v`
Expected: all tests across the package PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/entity/`
Expected: no output.

- [ ] **Step 4.6: Commit**

```bash
git add pkg/entity/obj.go pkg/entity/obj_test.go
git commit --no-gpg-sign -m "feat(entity): add Obj with receiver/reveal/count

Obj is a 1x1 ground item. ReceiverID is a player slot (-1 means public).
Reveal is the tick countdown before a private drop becomes public; -1 if
already public. LastChange records the last tick Count was modified."
```

---

## Task 5: `coordgrid.PackZoneCoord`

**Files:**
- Modify: `pkg/coordgrid/coordgrid.go`
- Modify: `pkg/coordgrid/coordgrid_test.go`

- [ ] **Step 5.1: Write the failing tests**

Append to `pkg/coordgrid/coordgrid_test.go`:

```go
func TestPackZoneCoordCorners(t *testing.T) {
	if got := PackZoneCoord(0, 0); got != 0x00 {
		t.Errorf("(0,0): got %#x, want 0x00", got)
	}
	if got := PackZoneCoord(7, 7); got != 0x77 {
		t.Errorf("(7,7): got %#x, want 0x77", got)
	}
}

func TestPackZoneCoordWorldAbsolute(t *testing.T) {
	// (3094 & 7) == 6; (3106 & 7) == 2; byte = (6<<4) | 2 = 0x62.
	if got := PackZoneCoord(3094, 3106); got != 0x62 {
		t.Errorf("(3094,3106): got %#x, want 0x62", got)
	}
}

func TestPackZoneCoordDiscardsHighBits(t *testing.T) {
	// High bits must not leak — (3200,3200) and (0,0) differ only in bits >= 3.
	if PackZoneCoord(3200, 3200) != PackZoneCoord(0, 0) {
		t.Error("PackZoneCoord should only look at the low 3 bits of x and z")
	}
}
```

- [ ] **Step 5.2: Run the tests — verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/ -run TestPackZoneCoord -v`
Expected: FAIL — `undefined: PackZoneCoord`.

- [ ] **Step 5.3: Implement `PackZoneCoord`**

Append to `pkg/coordgrid/coordgrid.go` (below `PackCoord` or wherever is stylistically appropriate for this file — it's a coord-packing helper):

```go
// PackZoneCoord packs the zone-local low bits of a world-absolute (x, z)
// into a single byte: (x&7)<<4 | (z&7). Used inside every zone-nested
// packet encoder to identify which tile within the 8x8 zone an event
// refers to.
func PackZoneCoord(x, z int) byte {
	return byte((x&0x7)<<4 | (z & 0x7))
}
```

- [ ] **Step 5.4: Run the tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/ -run TestPackZoneCoord -v`
Expected: 3/3 PASS.

- [ ] **Step 5.5: Full suite + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step 5.6: Commit**

```bash
git add pkg/coordgrid/coordgrid.go pkg/coordgrid/coordgrid_test.go
git commit --no-gpg-sign -m "feat(coordgrid): add PackZoneCoord for zone-nested packets

byte((x&7)<<4 | (z&7)) — identifies one of the 64 tiles in an 8x8 zone.
Prerequisite for the zone-nested packet encoders landing in sub-spec 4b-2."
```

---

## Final Verification

- [ ] **Step F.1: Full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS across all packages.

- [ ] **Step F.2: Race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS (no races expected — pkg/entity is pure data).

- [ ] **Step F.3: `go vet`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step F.4: Verify package count**

Run: `ls pkg/entity/ | wc -l`
Expected: `8` (5 production + 3 test files).

---

## Spec coverage map (self-review)

| Spec requirement | Task |
|---|---|
| `Lifecycle` enum (Forever/Respawn/Despawn) | Task 1 |
| `Entity` base struct + 4 helpers | Task 1 |
| `NonPathing` embed-only marker | Task 2 |
| `Loc` with packed Info + Type/Shape/Angle getters | Task 3 |
| `Obj` with ReceiverID/Reveal/LastChange | Task 4 |
| `ObjReveal = 100` constant | Task 4 |
| `coordgrid.PackZoneCoord` | Task 5 |
| All acceptance criteria (tests + vet + race) | Task F |

No gaps. Every spec bullet maps to a specific task.
