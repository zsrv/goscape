# NAI-86: LOC mutator family + lifecycle revert tick processor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `LOC_CHANGE` / `LOC_ADD` / `LOC_DEL` / `LOC_ANIM` script handlers + the foundation they require: `Loc` `BaseInfo`/`CurrentInfo` split, collision wiring in `Server.AddLoc`/`ChangeLoc`/`RemoveLoc`, and a per-tick `Loc.Turn` driver that fires the lifecycle revert at the scheduled tick.

**Architecture:** 4 bundles, dependency-ordered.
- **Bundle 1** rewires the `Loc` entity (immutable `BaseInfo` + mutable `CurrentInfo` + `IsActive`) and threads collision-update calls through `Server.AddLoc`/`ChangeLoc`/`RemoveLoc`. Signature changes blast through 6 test call sites.
- **Bundle 2** adds the per-tick `Loc.Turn` driver: `LifecycleTracker` interface on `pkg/entity`, `locObjTracker` field on `Server`, an `s.turnLoc` dispatch on existing `s.processZones`, and `s.RevertLoc`.
- **Bundle 3** ports the four script handlers + `LocOps` interface on `ScriptState` + `DurationValid` + dispatch wiring.
- **Bundle 4** door-click smoke + close commit.

**Tech Stack:** Go 1.26+ (per `go_version.md`); existing deps only; TS reference `LostCityRS/Engine-TS`.

**Spec:** `docs/superpowers/specs/2026-05-04-nai-86-loc-mutator-family-port-design.md`

**Pre-flight findings applied (corrects spec):**
- `s.processZones()` ALREADY EXISTS at `modules/world/tick.go:461` (currently just calls ComputeShared). Bundle 2 extends it; does not create.
- Goscape has no `pointerAdd` mechanism — direct `s.ActiveLoc = loc` assignment.
- No general `ScriptState.World` adapter; goscape uses narrow per-domain interfaces (`Configs`, `Inv`, `PlayerLookup`). Bundle 3 adds `LocOps` in this style.
- `addLocToZone` test helper uses `zn.AddStaticLoc(l)` directly, NOT `Server.AddLoc` — signature change does NOT affect it.
- `s.populateStaticLocsIntoZones()` uses `z.AddStaticLoc(loc)` directly — no change.
- `Server.AddLoc(loc)` call sites needing `, 0`: `world_zone_test.go:21`, `world_zone_test.go:55`, `tick_zone_test.go:13`, `tick_zone_test.go:38`, `player_zone_test.go:62` (5 sites).
- `Server.ChangeLoc(loc)` call sites: `world_zone_test.go:55` (1 site, inside `TestServerDispatchersTrackOncePerZone`).
- `Server.RemoveLoc(loc)` call sites: none outside the definition.

---

## File Structure

**New files:**
- `pkg/entity/lifecycle_tracker.go` — `LifecycleTracker` interface (Bundle 2)
- `modules/world/loc_tracker.go` — `locObjTracker` impl on Server (Bundle 2)
- `modules/world/loc_turn.go` — `s.turnLoc(*Loc, tick)` dispatch + `s.RevertLoc(*Loc)` (Bundle 2)
- `modules/world/loc_turn_test.go` — Bundle 2 integration tests
- `pkg/script/loc_ops.go` — `LocOps` interface (Bundle 3)
- `modules/world/script_loc_ops.go` — Server adapter implementing `script.LocOps` (Bundle 3)

**Modified files:**
- `pkg/entity/loc.go` — `BaseInfo`/`CurrentInfo` split + `Layer`/`IsActive`/`IsChanged`/`Change`/`Revert` (Bundle 1)
- `pkg/entity/loc_test.go` — new mutator tests (Bundle 1)
- `pkg/entity/nonpathing.go` — `parent any` field + `SetLifeCycle(duration, currentTick, tracker)` override (Bundle 2)
- `modules/world/world_zone.go` — `Server.AddLoc`/`ChangeLoc`/`RemoveLoc` collision-aware rewrites + signature changes (Bundle 1)
- `modules/world/world_zone_test.go` — call-site fixups + new collision tests (Bundle 1)
- `modules/world/tick_zone_test.go` — call-site fixups (Bundle 1)
- `modules/world/player_zone_test.go` — call-site fixups (Bundle 1)
- `modules/world/server.go` — `locObjTracker` field init in `New` (Bundle 2)
- `modules/world/tick.go` — extend `processZones` to drive turnLoc (Bundle 2)
- `pkg/script/state.go` — `LocOps LocOps` field (Bundle 3)
- `pkg/script/handlers_loc.go` — 4 new handlers + validators (Bundle 3)
- `pkg/script/handlers_loc_test.go` — handler unit tests (Bundle 3)
- `pkg/script/handlers.go` — dispatch wiring (Bundle 3)

---

## Bundle 1 — Loc entity + collision foundation

### Task 1.1: Loc.BaseInfo/CurrentInfo split + new accessors (TDD)

**Files:**
- Modify: `pkg/entity/loc.go`
- Test: `pkg/entity/loc_test.go`

- [ ] **Step 1: Read current loc_test.go**

Read `pkg/entity/loc_test.go` to confirm pre-existing assertions on `Type()`/`Shape()`/`Angle()` (they must stay green after the split).

- [ ] **Step 2: Write failing tests for new mutator API**

Append to `pkg/entity/loc_test.go`:

```go
func TestLocChangeMutatesCurrentInfoOnly(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	baseBefore := l.BaseInfo
	l.Change(99, 0, 1)
	if l.BaseInfo != baseBefore {
		t.Errorf("BaseInfo mutated: got %d, want %d", l.BaseInfo, baseBefore)
	}
	if l.Type() != 99 {
		t.Errorf("Type after Change: got %d, want 99", l.Type())
	}
	if l.Angle() != 1 {
		t.Errorf("Angle after Change: got %d, want 1", l.Angle())
	}
}

func TestLocRevertRestoresBaseInfo(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	l.Change(99, 0, 1)
	l.Revert()
	if l.CurrentInfo != l.BaseInfo {
		t.Errorf("Revert: CurrentInfo=%d BaseInfo=%d", l.CurrentInfo, l.BaseInfo)
	}
	if l.Type() != 42 {
		t.Errorf("Type after Revert: got %d, want 42", l.Type())
	}
}

func TestLocIsChangedReflectsBaseDelta(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	if l.IsChanged() {
		t.Error("fresh loc must report IsChanged=false")
	}
	l.Change(99, 0, 0)
	if !l.IsChanged() {
		t.Error("after Change with new type, IsChanged must be true")
	}
	l.Revert()
	if l.IsChanged() {
		t.Error("after Revert, IsChanged must be false")
	}
}

func TestLocLayerReadsFromBaseInfo(t *testing.T) {
	// shape=0 (ShapeWallStraight) → LayerWall (0)
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	if l.Layer() != 0 {
		t.Errorf("Layer for shape=0: got %d, want 0 (LayerWall)", l.Layer())
	}
	// Change shape; Layer reads BaseInfo so must be unaffected
	l.Change(42, 22, 0) // ShapeGroundDecor (LayerGroundDecor=3)
	if l.Layer() != 0 {
		t.Errorf("Layer after Change of shape: got %d, want 0 (BaseInfo unchanged)", l.Layer())
	}
}

func TestLocIsActiveDefaultFalse(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleDespawn, 42, 0, 0)
	if l.IsActive {
		t.Error("fresh loc must have IsActive=false")
	}
}
```

- [ ] **Step 3: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run "TestLocChange|TestLocRevert|TestLocIsChanged|TestLocLayer|TestLocIsActive" -v
```

Expected: tests fail to compile — `BaseInfo`, `CurrentInfo`, `Change`, `Revert`, `IsChanged`, `Layer`, `IsActive` undefined.

- [ ] **Step 4: Rewrite `pkg/entity/loc.go`**

Replace entire file with:

```go
package entity

import (
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// Loc is a scenery object: a door, a tree, a tile trap. Its render fields
// are packed into two 32-bit Info words. BaseInfo is set at construction and
// never mutated; CurrentInfo starts equal to BaseInfo and is rewritten by
// Change. The split mirrors TS Loc.baseInfo / Loc.currentInfo
// (Engine-TS/src/engine/entity/Loc.ts:9-12) and gives IsChanged its meaning.
type Loc struct {
	NonPathing
	BaseInfo    int  // immutable: packed (type, shape, angle, layer)
	CurrentInfo int  // mutable: equals BaseInfo at construction; rewritten by Change
	IsActive    bool // true after Server.AddLoc, false after Server.RemoveLoc
}

// NewLoc constructs a Loc at (level, x, z) with the given footprint and
// packed render fields. Returns a pointer so callers can call Change()
// in place. Wires the NonPathing back-pointer so the lifecycle tracker
// can recover the *Loc from a *NonPathing handle (NAI-86 Bundle 2).
func NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc {
	info := packLocInfo(typ, shape, angle)
	l := &Loc{BaseInfo: info, CurrentInfo: info}
	l.Entity = NewEntity(level, x, z, width, length, lc)
	l.parent = l
	return l
}

// packLocInfo combines the four render fields into a single int using the
// bit layout shared with the TS reference (Loc.ts:20-24):
//
//	[type:14][shape:5][angle:2][layer:2]
//
// Layer is derived from shape via pkg/pathfinder/loc.LayerOf and stored in
// BaseInfo so Layer() never changes after construction (mirrors TS layer
// reading from baseInfo). Out-of-range inputs are silently masked.
func packLocInfo(typ, shape, angle int) int {
	layer := loc.LayerOf(loc.Shape(shape))
	return (typ & 0x3FFF) |
		(shape&0x1F)<<14 |
		(angle&0x3)<<19 |
		(int(layer)&0x3)<<21
}

// Type returns the LocType id (bits 0..13 of CurrentInfo).
func (l *Loc) Type() int { return l.CurrentInfo & 0x3FFF }

// Shape returns the loc shape (bits 14..18 of CurrentInfo).
func (l *Loc) Shape() int { return (l.CurrentInfo >> 14) & 0x1F }

// Angle returns the loc rotation (bits 19..20 of CurrentInfo).
func (l *Loc) Angle() int { return (l.CurrentInfo >> 19) & 0x3 }

// Layer returns the loc shape's render layer (bits 21..22 of BaseInfo).
// Mirrors TS Loc.layer reading from baseInfo (Loc.ts:42-44).
func (l *Loc) Layer() int { return (l.BaseInfo >> 21) & 0x3 }

// IsChanged reports whether the loc's CurrentInfo has been mutated away
// from BaseInfo. Mirrors TS Loc.isChanged (Loc.ts:26-28).
func (l *Loc) IsChanged() bool { return l.CurrentInfo != l.BaseInfo }

// Change rewrites CurrentInfo to the packing of (typ, shape, angle).
// Mirrors TS Loc.change (Loc.ts:46-48). BaseInfo is not touched.
func (l *Loc) Change(typ, shape, angle int) {
	l.CurrentInfo = packLocInfo(typ, shape, angle)
}

// Revert restores CurrentInfo to BaseInfo. Mirrors TS Loc.revert (Loc.ts:50-52).
func (l *Loc) Revert() { l.CurrentInfo = l.BaseInfo }

// LocType returns the LocType ID for this loc. Satisfies the
// pkg/script.ActiveLoc interface. Alias for Type() with a less-ambiguous
// name when the loc is bound to script state.
func (l *Loc) LocType() int { return l.Type() }

// Slot returns -1 because locs are not slot-indexed (unlike Players and
// Npcs which live in server-wide slot registries). Required for the
// world.entity interface so locs can be assigned to Player.target.
func (l *Loc) Slot() int { return -1 }

// Coords returns the loc's tile position. Required for the world.entity
// interface. Reads X/Z/Level from the embedded entity.Entity (see
// entity.go:6-12 for the field layout); no allocation.
func (l *Loc) Coords() (x, z, level int) {
	return l.X, l.Z, l.Level
}

// IsValid returns the loc's intrinsic validity. Zone-membership
// (pointer still in zoneMap.Get(level,x,z).Locs) is checked separately
// by world-module helpers at the validateTarget call site, because
// pkg/entity cannot depend on modules/world. The "in world right now"
// check that gates Loc.Turn branches lives on IsActive.
func (l *Loc) IsValid() bool { return true }
```

- [ ] **Step 5: Add `parent any` field to NonPathing (interim)**

The `l.parent = l` line in `NewLoc` requires `NonPathing.parent any`. Add this field now (Bundle 2 will use it for the tracker; we add it here so loc.go compiles):

Edit `pkg/entity/nonpathing.go` to:

```go
package entity

// NonPathing is the shared concrete base for entities that don't walk —
// Locs and Objs. Exists to give zone code a single embedded base that
// future zone-event machinery can key against via interface satisfaction.
type NonPathing struct {
	Entity

	// parent is a back-pointer to the concrete *Loc / *Obj wrapping this
	// NonPathing. Populated by NewLoc / NewObj. Bundle 2's lifecycle
	// tracker iterates *NonPathing handles and recovers the concrete
	// entity through this field.
	parent any
}

// Parent returns the back-pointer set at construction. Bundle 2 of
// NAI-86 type-asserts on the result inside Server.processZones to
// dispatch turnLoc / (future) turnObj.
func (np *NonPathing) Parent() any { return np.parent }
```

- [ ] **Step 6: Wire NewObj to set parent (no behavior change yet)**

Read `pkg/entity/obj.go` to find `NewObj`. Add `o.parent = o` at the end (mirrors NewLoc). This is a one-line addition required to keep test fixtures consistent for Bundle 2's `processZones` type-switch.

```go
// At the end of NewObj before `return o`:
o.parent = o
```

- [ ] **Step 7: Run loc package tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v
```

Expected: PASS for all entity tests.

- [ ] **Step 8: Run full repo build to surface external Loc.Info consumers**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

If any caller references `loc.Info`, fix to `loc.CurrentInfo` inline. Likely sites (pre-flight grep showed clean — but verify):

```
grep -rn "\.Info\b" pkg/zone pkg/rsbuf modules/world | grep -v _test.go | grep -i loc
```

- [ ] **Step 9: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. If failures appear, inspect for `loc.Info` reads that need rewiring.

- [ ] **Step 10: Commit**

```bash
git add pkg/entity/loc.go pkg/entity/loc_test.go pkg/entity/nonpathing.go pkg/entity/obj.go
git commit --no-gpg-sign -m "refactor(entity): NAI-86 B1.1 — Loc BaseInfo/CurrentInfo split + Change/Revert/Layer/IsActive

Splits Loc.Info into immutable BaseInfo + mutable CurrentInfo per TS
Loc.ts:9-52. Adds Layer (bits 21-22 of BaseInfo per TS), IsChanged,
Change(typ,shape,angle), Revert, IsActive. NonPathing.parent
back-pointer wired so Bundle 2's lifecycle tracker can recover the
concrete entity from *NonPathing. NewObj also sets parent for symmetry.

5 new entity tests cover mutator semantics + Layer baseInfo invariance."
```

---

### Task 1.2: Server.AddLoc/ChangeLoc/RemoveLoc collision wiring + signature changes (TDD)

**Files:**
- Modify: `modules/world/world_zone.go`
- Modify: `modules/world/world_zone_test.go` (call-site fixups + new tests)
- Modify: `modules/world/tick_zone_test.go` (call-site fixups)
- Modify: `modules/world/player_zone_test.go` (call-site fixup)

- [ ] **Step 1: Write failing collision-wiring tests**

Append to `modules/world/world_zone_test.go`:

```go
func TestServerAddLocAddsCollisionWhenBlockwalk(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if !loc.IsActive {
		t.Error("AddLoc must set IsActive=true")
	}
	if !s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Error("AddLoc with BlockWalk=true should set BlockWalk flag")
	}
}

func TestServerAddLocSkipsCollisionWhenNotBlockwalk(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  false,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Error("AddLoc with BlockWalk=false should not set collision")
	}
}

func TestServerChangeLocSwapsCollision(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	s.locTypes.Configs[101] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 101},
		BlockWalk:  false,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if !s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Fatal("setup: should be blocked after AddLoc")
	}
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 1)
	if s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Error("ChangeLoc to non-blockwalk type should clear collision")
	}
	if loc.Type() != 101 {
		t.Errorf("loc.Type after Change: got %d, want 101", loc.Type())
	}
}

func TestServerRemoveLocClearsCollision(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.RemoveLoc(loc, 0)
	if loc.IsActive {
		t.Error("RemoveLoc must set IsActive=false")
	}
	if s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Error("RemoveLoc should clear BlockWalk flag")
	}
}

func TestServerChangeLocOnInactiveDespawnIsNoOp(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	// Never AddLoc → IsActive stays false
	s.ChangeLoc(loc, 100, 0, 1, 1)
	if loc.Angle() == 1 {
		t.Error("ChangeLoc on inactive DESPAWN must early-return; angle not mutated")
	}
}
```

A `newTestGamemap(t)` helper may not exist. Check:

```
grep -n "func newTestGamemap\|func.*TestGamemap" modules/world/*.go
```

If absent, add at top of `world_zone_test.go`:

```go
func newTestGamemap(t *testing.T) *gamemap.GameMap {
	t.Helper()
	pf := pathfinder.New()
	gm := gamemap.New(pf, slog.Default())
	return gm
}
```

(Adjust constructor args to match actual `gamemap.New` signature — read `pkg/gamemap/gamemap.go:33-43` to confirm.)

Add imports needed:
```go
"github.com/zsrv/goscape/pkg/gamemap"
"github.com/zsrv/goscape/pkg/objtype"
```

- [ ] **Step 2: Run tests to verify failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestServerAddLocAddsCollision|TestServerAddLocSkipsCollision|TestServerChangeLocSwapsCollision|TestServerRemoveLocClearsCollision|TestServerChangeLocOnInactive" -v
```

Expected: compile error (`AddLoc` signature mismatch — needs duration arg; `ChangeLoc` signature mismatch).

- [ ] **Step 3: Update Server.AddLoc/ChangeLoc/RemoveLoc**

Rewrite `modules/world/world_zone.go` AddLoc/ChangeLoc/RemoveLoc:

```go
// AddLoc routes a loc spawn through the world's zone map. Wires
// collision flags via gamemap.ChangeLocCollision when the loc's
// LocType has BlockWalk=true. Mirrors TS World.addLoc
// (Engine-TS/src/engine/World.ts:1337-1348).
//
// Sets loc.IsActive=true after the zone wire. duration > 0 schedules
// a despawn-revert via NonPathing.SetLifeCycle (Bundle 2 wires the
// tracker; until Bundle 2 lands SetLifeCycle is a no-op for the
// tracker side).
func (s *Server) AddLoc(loc *entitypkg.Loc, duration int) {
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddLoc(loc)
	loc.IsActive = true
	s.TrackZone(z)
	loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
}

// ChangeLoc rewrites the loc's render fields to (typ, shape, angle)
// and reschedules its lifecycle to despawn/revert at currentTick+duration.
// Mirrors TS World.changeLoc (Engine-TS/src/engine/World.ts:1350-1386).
//
// Order matters per TS: (1) early-return if DESPAWN+!IsActive (don't
// return inactive DESPAWN to game world); (2) remove old collision;
// (3) loc.Change(); (4) add new collision; (5) zone.ChangeLoc;
// (6) trackZone; (7) SetLifeCycle (duration if changed-or-DESPAWN,
// else -1 to untrack a no-op static change).
func (s *Server) ChangeLoc(loc *entitypkg.Loc, typ, shape, angle, duration int) {
	if loc.Lifecycle == entitypkg.LifecycleDespawn && !loc.IsActive {
		return
	}
	if loc.IsActive && s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(loc.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), oldLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	loc.Change(typ, shape, angle)
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(typ); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), newLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.ChangeLoc(loc)
	s.TrackZone(z)
	if loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}

// RemoveLoc deactivates a loc, clears its collision (if BlockWalk),
// and reschedules respawn (RESPAWN) or untracks (DESPAWN). Mirrors TS
// World.removeLoc (Engine-TS/src/engine/World.ts:1402-1425).
func (s *Server) RemoveLoc(loc *entitypkg.Loc, duration int) {
	if !loc.IsActive {
		return
	}
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.RemoveLoc(loc)
	loc.IsActive = false
	s.TrackZone(z)
	if loc.Lifecycle == entitypkg.LifecycleRespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}

// locTypeOrNil returns the LocType for id with bounds checking, or nil
// if id is out of range or the type is unloaded.
func (s *Server) locTypeOrNil(id int) *objtype.LocType {
	if id < 0 || id >= len(s.locTypes.Configs) {
		return nil
	}
	return s.locTypes.Configs[id]
}
```

- [ ] **Step 4: Stub `loc.SetLifeCycle` + `s.locObjTracker` to keep build green**

Bundle 2 implements these properly. For Bundle 1, add the minimum so Server.AddLoc/etc. compile.

In `pkg/entity/nonpathing.go` add:

```go
// SetLifeCycle is the duration-aware lifecycle override that registers
// the entity in a LifecycleTracker. Bundle 2 of NAI-86 lands the
// tracker; this stub records the transition tick only and ignores the
// tracker arg.
//
// TODO(NAI-86 Bundle 2): rewire to call tracker.Register / Unregister
// and remove this stub doc-line.
func (np *NonPathing) SetLifeCycle(duration, currentTick int, tracker any) {
	if duration > 0 {
		np.SetLifecycle(currentTick+duration, currentTick)
	} else {
		np.SetLifecycle(-1, currentTick)
	}
}
```

In `modules/world/server.go` add a `locObjTracker any` field on Server (Bundle 2 will give it a concrete type):

```go
// Find the Server struct (line ~44) and add inside, near other entity-tracking fields:
locObjTracker any // NAI-86 Bundle 2: replaced with *locObjTracker
```

- [ ] **Step 5: Fix call-site signature changes**

Update the 5 `s.AddLoc(loc)` and 1 `s.ChangeLoc(loc)` test call sites to pass `0` / args:

`modules/world/world_zone_test.go:21`:
```go
s.AddLoc(loc, 0)
```

`modules/world/world_zone_test.go:55-56` (TestServerDispatchersTrackOncePerZone — both AddLoc and ChangeLoc):
```go
s.AddLoc(loc, 0)
s.ChangeLoc(loc, loc.Type(), loc.Shape(), loc.Angle(), 0)
s.AnimLoc(loc, 42)
```

`modules/world/tick_zone_test.go:13`:
```go
s.AddLoc(loc, 0)
```

`modules/world/tick_zone_test.go:38`:
```go
s.AddLoc(loc, 0)
```

`modules/world/player_zone_test.go:62`:
```go
s.AddLoc(loc, 0)
```

- [ ] **Step 6: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v
```

Expected: PASS — including the 5 new collision tests.

- [ ] **Step 7: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add modules/world/world_zone.go modules/world/world_zone_test.go modules/world/tick_zone_test.go modules/world/player_zone_test.go modules/world/server.go pkg/entity/nonpathing.go
git commit --no-gpg-sign -m "feat(world): NAI-86 B1.2 — Server.AddLoc/ChangeLoc/RemoveLoc collision wiring

AddLoc/ChangeLoc/RemoveLoc now thread gamemap.ChangeLocCollision per TS
World.addLoc/changeLoc/removeLoc (World.ts:1337-1425). Signature
changes: AddLoc/ChangeLoc/RemoveLoc gain duration int param.
ChangeLoc takes (loc, typ, shape, angle, duration). 6 test call sites
updated; addLocToZone test helper bypasses Server.AddLoc and is
unaffected. populateStaticLocsIntoZones uses zone.AddStaticLoc and is
unaffected.

Loc.IsActive transitions: false → true on AddLoc → false on RemoveLoc.
ChangeLoc on inactive DESPAWN early-returns per TS guard.

Bundle 2 stub: NonPathing.SetLifeCycle(duration, currentTick, tracker)
records the transition tick only; tracker.Register/Unregister wired in
Bundle 2. Server.locObjTracker field added as 'any' for Bundle 2 to
fill in.

5 new collision tests cover BlockWalk gating + add/clear/swap order +
DESPAWN-inactive guard."
```

---

## Bundle 2 — Lifecycle revert tick processor

### Task 2.1: LifecycleTracker interface + NonPathing.SetLifeCycle proper override (TDD)

**Files:**
- Create: `pkg/entity/lifecycle_tracker.go`
- Modify: `pkg/entity/nonpathing.go`
- Test: `pkg/entity/nonpathing_test.go` (create if absent)

- [ ] **Step 1: Write failing tests**

Create `pkg/entity/nonpathing_test.go`:

```go
package entity

import "testing"

// fakeTracker records Register/Unregister calls.
type fakeTracker struct {
	registered   []*NonPathing
	unregistered []*NonPathing
}

func (t *fakeTracker) Register(np *NonPathing)   { t.registered = append(t.registered, np) }
func (t *fakeTracker) Unregister(np *NonPathing) { t.unregistered = append(t.unregistered, np) }

func TestSetLifeCyclePositiveDurationRegisters(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)

	if len(tr.registered) != 1 || tr.registered[0] != np {
		t.Errorf("Register: got %v, want [np]", tr.registered)
	}
	if np.LifecycleTick != 105 {
		t.Errorf("LifecycleTick: got %d, want 105 (currentTick=100 + duration=5)", np.LifecycleTick)
	}
	if np.LastLifecycleTick != 100 {
		t.Errorf("LastLifecycleTick: got %d, want 100", np.LastLifecycleTick)
	}
}

func TestSetLifeCycleSecondCallUnregistersFirst(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)
	np.SetLifeCycle(3, 110, tr)

	if len(tr.unregistered) != 1 || tr.unregistered[0] != np {
		t.Errorf("Unregister: got %v, want [np]", tr.unregistered)
	}
	if len(tr.registered) != 2 {
		t.Errorf("Register count: got %d, want 2", len(tr.registered))
	}
	if np.LifecycleTick != 113 {
		t.Errorf("LifecycleTick: got %d, want 113", np.LifecycleTick)
	}
}

func TestSetLifeCycleNonPositiveDurationUntracks(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)
	np.SetLifeCycle(-1, 110, nil)

	if len(tr.unregistered) != 1 {
		t.Errorf("Unregister: got %d, want 1", len(tr.unregistered))
	}
	if np.LifecycleTick != -1 {
		t.Errorf("LifecycleTick: got %d, want -1", np.LifecycleTick)
	}
}

func TestSetLifeCycleNoTrackerNoRegister(t *testing.T) {
	// Initial call with duration<=0 and tracker=nil must not panic and
	// must not register anything.
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.SetLifeCycle(0, 50, nil)
	if np.LifecycleTick != -1 {
		t.Errorf("LifecycleTick: got %d, want -1", np.LifecycleTick)
	}
}
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run "TestSetLifeCycle" -v
```

Expected: tests fail to compile — `fakeTracker.Register` doesn't satisfy current `tracker any` signature; `LifecycleTracker` undefined.

- [ ] **Step 3: Create `pkg/entity/lifecycle_tracker.go`**

```go
package entity

// LifecycleTracker is the back-channel that NonPathing.SetLifeCycle
// uses to (de)register entities for per-tick processing. The concrete
// implementation lives in modules/world (it owns the doubly-linked
// list and the per-NonPathing element back-pointer); pkg/entity stays
// dependency-free.
//
// Mirrors the role of TS World.locObjTracker: each entity with
// duration > 0 is tracked; SetLifeCycle calls during the tracker's
// iteration must be reentrant-safe (Server.processZones snapshots
// before iterating to avoid mid-iteration mutation).
type LifecycleTracker interface {
	Register(np *NonPathing)
	Unregister(np *NonPathing)
}
```

- [ ] **Step 4: Replace the stub `SetLifeCycle` in `pkg/entity/nonpathing.go`**

Replace the stub method (added in Bundle 1.2 Step 4) with:

```go
// SetLifeCycle schedules the entity's next lifecycle transition at
// currentTick + duration and (de)registers it in the supplied
// LifecycleTracker. duration <= 0 untracks. Mirrors TS
// NonPathingEntity.setLifeCycle (Engine-TS/.../NonPathingEntity.ts:11-25).
//
// Idempotent: a second call always Unregisters the previous tracker
// node before registering the new one, even if the tracker arg is the
// same pointer. duration <= 0 with tracker=nil is the "untrack only"
// shape used by Server.RevertLoc and the no-op-static-change branch
// of Server.ChangeLoc.
func (np *NonPathing) SetLifeCycle(duration, currentTick int, tracker LifecycleTracker) {
	if np.tracker != nil {
		np.tracker.Unregister(np)
		np.tracker = nil
	}
	if duration > 0 {
		tracker.Register(np)
		np.tracker = tracker
		np.SetLifecycle(currentTick+duration, currentTick)
	} else {
		np.SetLifecycle(-1, currentTick)
	}
}
```

Add `tracker LifecycleTracker` field to NonPathing struct:

```go
type NonPathing struct {
	Entity

	parent  any
	tracker LifecycleTracker
}
```

- [ ] **Step 5: Fix existing call sites for tracker type**

Update Bundle 1.2's `modules/world/server.go` field decl:

```go
// Was: locObjTracker any
locObjTracker *locObjTracker  // type defined in Task 2.2
```

Bundle 2.2 introduces `locObjTracker` type. Until 2.2 lands, this won't compile. Acceptable WITHIN Bundle 2 (tasks 2.1 and 2.2 are sequential; commit 2.1 leaves the build broken on the world module).

**Alternative**: keep the `any` type in 2.1 + cast at the SetLifeCycle call sites. We pick the cleaner sequential-commit approach: 2.1 commits broken-world + 2.2 fixes within minutes. Implementer notes this in their commit message.

Actually, simpler — keep field as `entity.LifecycleTracker` interface type (already an interface, so accepts the concrete `*locObjTracker` from Task 2.2 without type assertion):

```go
locObjTracker entity.LifecycleTracker  // *locObjTracker satisfies after Task 2.2
```

And import:
```go
"github.com/zsrv/goscape/pkg/entity"
```

And update the Server.AddLoc/ChangeLoc/RemoveLoc calls to use this typed field (no change — they already pass `s.locObjTracker`).

- [ ] **Step 6: Run entity package tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v
```

Expected: PASS for all entity tests including new SetLifeCycle ones.

- [ ] **Step 7: Build full repo; expect green (LifecycleTracker is an interface; nil satisfies)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS — `s.locObjTracker` is `entity.LifecycleTracker` interface type; defaults to nil. Server.AddLoc nil-checks the gamemap path but currently passes nil tracker to SetLifeCycle when duration > 0 → SetLifeCycle calls `tracker.Register(np)` on nil → panic.

Bundle 2.2 fixes this by initialising the tracker in Server.New. Until then, any test that calls AddLoc with duration > 0 will panic. All existing tests pass duration=0 (verified). Bundle 1.2's new collision tests pass duration=0 too. Safe.

- [ ] **Step 8: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add pkg/entity/lifecycle_tracker.go pkg/entity/nonpathing.go pkg/entity/nonpathing_test.go modules/world/server.go
git commit --no-gpg-sign -m "feat(entity): NAI-86 B2.1 — LifecycleTracker interface + NonPathing.SetLifeCycle override

LifecycleTracker is the pkg/entity-local interface that
NonPathing.SetLifeCycle uses to (de)register entities for per-tick
processing. Mirrors TS NonPathingEntity.setLifeCycle
(NonPathingEntity.ts:11-25): duration > 0 schedules transition tick
and Registers; duration <= 0 untracks. Idempotent — second call
always Unregisters before re-Registering.

Server.locObjTracker field re-typed from any to entity.LifecycleTracker.
Concrete impl lands in Task 2.2.

4 new entity tests cover Register/Unregister/re-register/untrack."
```

---

### Task 2.2: locObjTracker concrete impl on Server (TDD)

**Files:**
- Create: `modules/world/loc_tracker.go`
- Test: `modules/world/loc_tracker_test.go`
- Modify: `modules/world/server.go` (init `s.locObjTracker = newLocObjTracker()` in `New`)

- [ ] **Step 1: Write failing tests**

Create `modules/world/loc_tracker_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
)

func TestLocObjTrackerRegisterAddsToList(t *testing.T) {
	tr := newLocObjTracker()
	np := &entity.NonPathing{Entity: entity.NewEntity(0, 100, 200, 1, 1, entity.LifecycleDespawn)}
	tr.Register(np)
	count := 0
	for range tr.All() {
		count++
	}
	if count != 1 {
		t.Errorf("All() count: got %d, want 1", count)
	}
}

func TestLocObjTrackerUnregisterRemoves(t *testing.T) {
	tr := newLocObjTracker()
	np := &entity.NonPathing{Entity: entity.NewEntity(0, 100, 200, 1, 1, entity.LifecycleDespawn)}
	tr.Register(np)
	tr.Unregister(np)
	count := 0
	for range tr.All() {
		count++
	}
	if count != 0 {
		t.Errorf("All() count after Unregister: got %d, want 0", count)
	}
}

func TestLocObjTrackerReRegisterUnlinksOld(t *testing.T) {
	tr := newLocObjTracker()
	np := &entity.NonPathing{Entity: entity.NewEntity(0, 100, 200, 1, 1, entity.LifecycleDespawn)}
	tr.Register(np)
	tr.Register(np) // second register should unlink-and-re-add, not duplicate
	count := 0
	for range tr.All() {
		count++
	}
	if count != 1 {
		t.Errorf("All() count after re-Register: got %d, want 1 (no duplicate)", count)
	}
}

func TestLocObjTrackerUnregisterUnknownIsNoOp(t *testing.T) {
	tr := newLocObjTracker()
	np := &entity.NonPathing{Entity: entity.NewEntity(0, 100, 200, 1, 1, entity.LifecycleDespawn)}
	tr.Unregister(np) // no panic; no-op
}

func TestServerNewInitialisesLocObjTracker(t *testing.T) {
	s := newTestServer(t)
	if s.locObjTracker == nil {
		t.Error("Server.New must initialise locObjTracker")
	}
}
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLocObjTracker|TestServerNewInitialises" -v
```

Expected: compile errors — `newLocObjTracker` undefined, `tr.All()` undefined.

- [ ] **Step 3: Create `modules/world/loc_tracker.go`**

```go
package world

import (
	"iter"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// locObjTracker is the per-Server registry of NonPathing entities with
// pending lifecycle transitions. Iterated each tick by Server.processZones.
// Mirrors TS World.locObjTracker (Engine-TS/.../World.ts:154,964-973).
//
// Backed by pkg/zone.DoublyLinkList for O(1) Add/Unlink and an auxiliary
// map *NonPathing → *Element for O(1) Unregister-by-pointer.
type locObjTracker struct {
	list  *zone.DoublyLinkList[*entity.NonPathing]
	nodes map[*entity.NonPathing]*zone.Element[*entity.NonPathing]
}

// newLocObjTracker constructs an empty tracker. Server.New calls this
// once at server startup.
func newLocObjTracker() *locObjTracker {
	return &locObjTracker{
		list:  &zone.DoublyLinkList[*entity.NonPathing]{},
		nodes: map[*entity.NonPathing]*zone.Element[*entity.NonPathing]{},
	}
}

// Register adds np to the tracker. Idempotent — re-registering an
// already-tracked np unlinks the old node first to keep the list
// duplicate-free, matching TS behavior where setLifeCycle always
// unlinks the previous eventTracker before re-adding.
func (t *locObjTracker) Register(np *entity.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
}

// Unregister removes np from the tracker. No-op if np is not tracked.
func (t *locObjTracker) Unregister(np *entity.NonPathing) {
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
	}
}

// All returns an iterator over the tracked entries in insertion order.
// Callers that mutate the tracker mid-iteration MUST snapshot first
// (Server.processZones does this).
func (t *locObjTracker) All() iter.Seq[*entity.NonPathing] {
	return t.list.All(false)
}
```

- [ ] **Step 4: Initialise tracker in Server.New**

Find `func New(...)` in `modules/world/server.go` (around line 100-150). Inside the constructor, before `return s, nil`, add:

```go
s.locObjTracker = newLocObjTracker()
```

(The field type was set to `entity.LifecycleTracker` in Task 2.1; `*locObjTracker` satisfies the interface.)

- [ ] **Step 5: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLocObjTracker|TestServerNewInitialises" -v
```

Expected: PASS for the 5 tracker tests.

- [ ] **Step 6: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/loc_tracker.go modules/world/loc_tracker_test.go modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-86 B2.2 — locObjTracker on Server (DLL-backed lifecycle registry)

locObjTracker satisfies entity.LifecycleTracker via pkg/zone.DoublyLinkList
+ aux map[*NonPathing]*Element for O(1) Unregister. Server.New
initialises the field. Mirrors TS World.locObjTracker
(World.ts:154,964-973).

Re-Register is idempotent (unlinks old node first).
Unregister-of-unknown is no-op.

5 new tests cover Register/Unregister/re-register/no-op/Server-init."
```

---

### Task 2.3: Server.RevertLoc + s.turnLoc dispatch (TDD)

**Files:**
- Create: `modules/world/loc_turn.go`
- Test: `modules/world/loc_turn_test.go`

- [ ] **Step 1: Write failing tests**

Create `modules/world/loc_turn_test.go`:

```go
package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
)

// newLocTurnTestServer is a fixture for tick-driven Loc.Turn tests.
// Wires gamemap + locTypes + zoneMap + tracker so AddLoc/ChangeLoc work end-to-end.
func newLocTurnTestServer(t *testing.T) *Server {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	// Register two BlockWalk types
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100}, BlockWalk: true, BlockRange: true,
	}
	s.locTypes.Configs[101] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 101}, BlockWalk: false,
	}
	return s
}

func TestRevertLocSnapsToBaseInfoAndCollision(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 1)
	if s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Fatal("setup: changed-to-101 should clear collision")
	}
	if !loc.IsChanged() {
		t.Fatal("setup: loc should be IsChanged after Change")
	}
	s.RevertLoc(loc)
	if loc.IsChanged() {
		t.Error("after Revert, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after Revert, Type: got %d, want 100", loc.Type())
	}
	if !s.gamemap.Pathfinder.Flags.IsBlocked(3094, 3106, 0) {
		t.Error("after Revert, BlockWalk collision should be restored")
	}
}

func TestTurnLocDespawnFiresRemove(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 5) // schedule despawn at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if loc.IsActive {
		t.Error("after turnLoc DESPAWN at scheduled tick, IsActive must be false")
	}
}

func TestTurnLocRespawnChangedFiresRevert(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 5) // schedule revert at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if loc.IsChanged() {
		t.Error("after turnLoc RESPAWN+changed at scheduled tick, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after turnLoc Revert, Type: got %d, want 100", loc.Type())
	}
}

func TestTurnLocRespawnInactiveFiresAdd(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.RemoveLoc(loc, 5) // schedule re-add at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if !loc.IsActive {
		t.Error("after turnLoc RESPAWN+!active at scheduled tick, IsActive must be true (re-added)")
	}
}

func TestTurnLocBeforeScheduledTickIsNoOp(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 5)

	s.currentTick = 103
	s.turnLoc(loc, 103) // not the scheduled tick (105)

	if !loc.IsActive {
		t.Error("turnLoc before scheduled tick must be no-op")
	}
}
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRevertLoc|TestTurnLoc" -v
```

Expected: compile errors — `s.RevertLoc` and `s.turnLoc` undefined.

- [ ] **Step 3: Create `modules/world/loc_turn.go`**

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// turnLoc is the per-tick dispatch for a tracked Loc. Called from
// Server.processZones for each NonPathing in s.locObjTracker whose
// Parent() is a *Loc. Mirrors TS Loc.turn (Engine-TS/.../Loc.ts:54-74).
//
// Goscape uses Server.currentTick as the authoritative clock and stores
// the absolute target transition tick in LifecycleTick (set via
// Entity.SetLifecycle). TS decrements lifecycleTick-- per tick; the
// observable behavior is equivalent (deviation D-N86-4 in spec §5).
func (s *Server) turnLoc(l *entitypkg.Loc, now int) {
	if l.LifecycleTick != now {
		return
	}
	switch {
	case l.Lifecycle == entitypkg.LifecycleDespawn && l.IsActive:
		s.RemoveLoc(l, 0)
	case l.Lifecycle == entitypkg.LifecycleRespawn && l.IsChanged() && l.IsActive:
		s.RevertLoc(l)
	case l.Lifecycle == entitypkg.LifecycleRespawn && !l.IsActive:
		s.AddLoc(l, 0)
	default:
		// Mirrors TS console.error fallthrough — should not happen.
		// Unconditionally untrack to prevent unbounded re-iteration.
		s.log.Error("loc tracked but no event matched",
			"type", l.Type(), "x", l.X, "z", l.Z, "lifecycle", l.Lifecycle, "active", l.IsActive)
		l.SetLifeCycle(-1, now, nil)
	}
}

// RevertLoc snaps a RESPAWN loc's CurrentInfo back to BaseInfo, swaps
// collision, emits a zone ChangeLoc event, and untracks the lifecycle.
// Mirrors TS World.revertLoc (Engine-TS/.../World.ts:1427-1448). Called
// from turnLoc for the RESPAWN+IsChanged+IsActive branch.
func (s *Server) RevertLoc(l *entitypkg.Loc) {
	if s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(l.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), oldLt.BlockRange,
				l.Length, l.Width, l.X, l.Z, l.Level, false)
		}
	}
	l.Revert()
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(l.Type()); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), newLt.BlockRange,
				l.Length, l.Width, l.X, l.Z, l.Level, true)
		}
	}
	z := s.zoneMap.Get(l.Level, l.X, l.Z)
	z.ChangeLoc(l)
	s.TrackZone(z)
	l.SetLifeCycle(-1, s.currentTick, nil)
}
```

- [ ] **Step 4: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRevertLoc|TestTurnLoc" -v
```

Expected: PASS for all 5 turnLoc tests.

- [ ] **Step 5: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/loc_turn.go modules/world/loc_turn_test.go
git commit --no-gpg-sign -m "feat(world): NAI-86 B2.3 — Server.turnLoc + Server.RevertLoc

turnLoc is the per-tick dispatch for tracked Locs (DESPAWN+active→Remove,
RESPAWN+changed+active→Revert, RESPAWN+!active→Add) per TS Loc.turn
(Loc.ts:54-74). Default branch logs + untracks to prevent unbounded
re-iteration.

RevertLoc snaps CurrentInfo to BaseInfo, swaps collision, emits zone
event, untracks. Mirrors TS World.revertLoc (World.ts:1427-1448).

5 new tests cover Revert + each Turn branch + before-scheduled-tick no-op."
```

---

### Task 2.4: Extend Server.processZones to drive turnLoc (TDD)

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/loc_turn_test.go` (add integration test)

- [ ] **Step 1: Write failing integration test**

Append to `modules/world/loc_turn_test.go`:

```go
func TestProcessZonesFiresTurnLocAtScheduledTick(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 3) // schedule despawn at tick 103

	// tick 101, 102: not scheduled
	s.currentTick = 101
	s.processZones()
	if !loc.IsActive {
		t.Errorf("loc must stay active at tick 101 (scheduled 103)")
	}
	s.currentTick = 102
	s.processZones()
	if !loc.IsActive {
		t.Errorf("loc must stay active at tick 102")
	}

	// tick 103: scheduled
	s.currentTick = 103
	s.processZones()
	if loc.IsActive {
		t.Errorf("loc must be deactivated at scheduled tick 103")
	}
}

func TestProcessZonesSnapshotsBeforeIterating(t *testing.T) {
	// turnLoc → RemoveLoc → SetLifeCycle(-1, ..., nil) calls
	// tracker.Unregister mid-iteration. processZones must snapshot
	// to avoid undefined iteration over the modified list.
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	for i := 0; i < 5; i++ {
		loc := entitypkg.NewLoc(0, 3094+i, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
		s.AddLoc(loc, 1) // all schedule despawn at tick 101
	}

	s.currentTick = 101
	// Must not panic and must process all 5
	s.processZones()
}

func TestProcessZonesStillComputesShared(t *testing.T) {
	// pre-existing TestProcessZonesComputesShared assertion must remain
	// green after the turnLoc extension.
	s := newZoneTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.processZones()
	for z := range s.zonesTracking {
		if z.Shared() == nil {
			t.Error("Shared should be non-nil after processZones")
		}
	}
}
```

- [ ] **Step 2: Run tests; expect failures (loc still active at tick 103)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessZonesFiresTurnLoc|TestProcessZonesSnapshots" -v
```

Expected: FAIL — `processZones` doesn't yet drive turnLoc.

- [ ] **Step 3: Extend `s.processZones` in `modules/world/tick.go`**

Replace the existing 5-line `processZones` (around line 461) with:

```go
// processZones drives per-tick lifecycle transitions for tracked
// NonPathing entities (Loc / future Obj) and computes the shared
// Enclosed-event buffer for every tracked zone. Mirrors TS
// World.processZones (Engine-TS/.../World.ts:961-986).
//
// Snapshots the tracker before iterating because each turnLoc may
// mutate the tracker (RemoveLoc / RevertLoc both call SetLifeCycle(-1)
// → Unregister) and we cannot iterate a list that's being unlinked.
func (s *Server) processZones() {
	if s.locObjTracker != nil {
		// Snapshot to a slice — the tracker uses a linked list whose
		// iteration is invalidated by mid-iteration Unlink.
		var snap []*entity.NonPathing
		if t, ok := s.locObjTracker.(*locObjTracker); ok {
			for np := range t.All() {
				snap = append(snap, np)
			}
		}
		for _, np := range snap {
			switch p := np.Parent().(type) {
			case *entity.Loc:
				s.turnLoc(p, s.currentTick)
			case *entity.Obj:
				// TODO(NAI-86 D-N86-3): Obj.Turn ports later.
				_ = p
			}
		}
	}
	for z := range s.zonesTracking {
		z.ComputeShared()
	}
}
```

Add import to `tick.go`:
```go
"github.com/zsrv/goscape/pkg/entity"
```

- [ ] **Step 4: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessZones" -v
```

Expected: PASS for all processZones tests (new + pre-existing).

- [ ] **Step 5: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/tick.go modules/world/loc_turn_test.go
git commit --no-gpg-sign -m "feat(world): NAI-86 B2.4 — extend processZones to drive turnLoc

processZones now snapshots locObjTracker before iterating + dispatches
on Parent() type-switch to turnLoc / (future) turnObj per TS
World.processZones (World.ts:961-986). ComputeShared loop preserved.

Snapshot is required because RemoveLoc/RevertLoc both Unregister
mid-iteration via SetLifeCycle(-1, ..., nil); iterating an actively-
unlinked linked list is undefined.

3 new integration tests cover (a) tick-aligned firing, (b) bulk
mid-iteration Unregister, (c) pre-existing ComputeShared invariant."
```

---

## Bundle 3 — Script handlers

### Task 3.1: LocOps interface + ScriptState wiring (TDD)

**Files:**
- Create: `pkg/script/loc_ops.go`
- Modify: `pkg/script/state.go` (add `LocOps LocOps` field)

- [ ] **Step 1: Write failing test (new file)**

Create `pkg/script/loc_ops_test.go`:

```go
package script

import "testing"

// fakeLocOps records all LocOps method calls for handler-side assertions.
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
}

type changeLocCall struct {
	loc                       ActiveLoc
	typ, shape, angle, dur    int
}

type addLocCall struct {
	level, x, z, typ, shape, angle, dur int
}

type removeLocCall struct {
	loc ActiveLoc
	dur int
}

type animLocCall struct {
	loc ActiveLoc
	seq int
}

func (f *fakeLocOps) ChangeLoc(loc ActiveLoc, typ, shape, angle, dur int) error {
	f.changeCalls = append(f.changeCalls, changeLocCall{loc, typ, shape, angle, dur})
	return nil
}

func (f *fakeLocOps) AddLoc(level, x, z, typ, shape, angle, dur int) (ActiveLoc, error) {
	f.addCalls = append(f.addCalls, addLocCall{level, x, z, typ, shape, angle, dur})
	return f.atCoordHead(), nil
}

func (f *fakeLocOps) RemoveLoc(loc ActiveLoc, dur int) error {
	f.removeCalls = append(f.removeCalls, removeLocCall{loc, dur})
	return nil
}

func (f *fakeLocOps) AnimLoc(loc ActiveLoc, seq int) error {
	f.animCalls = append(f.animCalls, animLocCall{loc, seq})
	return nil
}

func (f *fakeLocOps) LocsAtCoord(level, x, z int) []ActiveLoc {
	return f.atCoord
}

func (f *fakeLocOps) atCoordHead() ActiveLoc {
	if len(f.atCoord) == 0 {
		return nil
	}
	return f.atCoord[0]
}

func TestScriptStateAcceptsLocOps(t *testing.T) {
	s := &ScriptState{}
	s.LocOps = &fakeLocOps{}
	if s.LocOps == nil {
		t.Error("LocOps field unsettable")
	}
}
```

- [ ] **Step 2: Run; expect compile failure (LocOps undefined)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestScriptStateAcceptsLocOps" -v
```

Expected: FAIL — `LocOps` undefined; `ScriptState.LocOps` field missing.

- [ ] **Step 3: Create `pkg/script/loc_ops.go`**

```go
package script

// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
// LOC_DEL / LOC_ANIM. Implementations live in modules/world (see
// script_loc_ops.go). Decouples pkg/script from world-side entity
// types; handlers pass the script-side ActiveLoc interface, the
// adapter type-asserts to the concrete *entity.Loc.
//
// LocsAtCoord returns the slice of locs at (level, x, z) for the
// LOC_ADD same-layer search branch. Returning a slice (not iter.Seq)
// keeps the interface simple — the call site iterates synchronously
// and the slice is small (≤4 layers per tile in practice).
type LocOps interface {
	ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
	AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
	RemoveLoc(loc ActiveLoc, duration int) error
	AnimLoc(loc ActiveLoc, seq int) error
	LocsAtCoord(level, x, z int) []ActiveLoc
}
```

- [ ] **Step 4: Add `LocOps LocOps` field to ScriptState**

In `pkg/script/state.go`, add inside the `ScriptState` struct, near the other lookup interfaces (after `Npcs NpcLookup` ~line 158):

```go
// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
// LOC_DEL / LOC_ANIM. Callers set this after Init if the script uses
// loc mutator opcodes. Nil disables (handlers return an explicit error).
LocOps LocOps
```

- [ ] **Step 5: Extend ActiveLoc interface to expose Layer**

Bundle 3's LOC_ADD same-layer search needs `existing.Layer() int` on the script-side ActiveLoc interface. Edit `pkg/script/active.go:698-703`:

```go
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.CurrentInfo bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
	Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
	Shape() int                // shape (0..22 valid range); consumed by LOC_SHAPE
	Layer() int                // shape's render layer (0..3); consumed by LOC_ADD same-layer search (NAI-86)
}
```

`*entity.Loc` already has `Layer()` from Bundle 1.1. Test fixtures (`fakeActiveLoc`, `mockActiveLoc`) need a `Layer()` method.

Search and update fixtures:
```
grep -rn "fakeActiveLoc\|mockActiveLoc" pkg/script/ | head
```

Add to each fixture:
```go
func (f *fakeActiveLoc) Layer() int { return f.layer }
// + add `layer int` field
```

Same for `mockActiveLoc`.

- [ ] **Step 6: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v
```

Expected: PASS for all script tests (existing + new LocOps acceptance test).

- [ ] **Step 7: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/loc_ops.go pkg/script/loc_ops_test.go pkg/script/state.go pkg/script/active.go pkg/script/handlers_loc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-86 B3.1 — LocOps interface + ScriptState.LocOps field

LocOps is the script→world mutator surface for LOC_CHANGE/ADD/DEL/ANIM
opcodes (NAI-86 Bundle 3). ScriptState.LocOps is the field handlers
read; nil disables. Mirrors the per-domain narrow-interface pattern
already established by Configs / Inv / PlayerLookup / Npcs.

ActiveLoc interface gains Layer() int for LOC_ADD's same-layer search
branch. Test fixtures (fakeActiveLoc, mockActiveLoc) updated."
```

---

### Task 3.2: handleLocChange + checkDuration validator (TDD)

**Files:**
- Modify: `pkg/script/handlers_loc.go` (append handleLocChange + checkDuration)
- Modify: `pkg/script/handlers_loc_test.go` (append handler tests)

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_loc_test.go`:

```go
func TestLocChangeCallsLocOpsWithPoppedArgs(t *testing.T) {
	s := newScriptStateWithActiveLoc(t, &fakeActiveLoc{locType: 100, shape: 0, angle: 0})
	cfg := newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	cfg.locTypes[200] = &objtype.LocType{ConfigType: objtype.ConfigType{ID: 200}}
	s.Configs = cfg
	ops := &fakeLocOps{}
	s.LocOps = ops

	// stack: [..., id=200, duration=3]
	s.PushInt(200)
	s.PushInt(3)

	if err := handleLocChange(s); err != nil {
		t.Fatalf("handleLocChange: unexpected error %v", err)
	}
	if len(ops.changeCalls) != 1 {
		t.Fatalf("ChangeLoc calls: got %d, want 1", len(ops.changeCalls))
	}
	c := ops.changeCalls[0]
	if c.typ != 200 || c.dur != 3 {
		t.Errorf("ChangeLoc args: got typ=%d dur=%d, want 200/3", c.typ, c.dur)
	}
	if c.shape != 0 || c.angle != 0 {
		t.Errorf("ChangeLoc preserves activeLoc shape/angle: got shape=%d angle=%d", c.shape, c.angle)
	}
}

func TestLocChangeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.PushInt(200)
	s.PushInt(3)
	if err := handleLocChange(s); err == nil {
		t.Error("handleLocChange without ActiveLoc must return error")
	}
}

func TestLocChangeRejectsZeroOrNegativeDuration(t *testing.T) {
	for _, dur := range []int{0, -1, -100} {
		s := newScriptStateWithActiveLoc(t, &fakeActiveLoc{locType: 100})
		s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
		s.LocOps = &fakeLocOps{}
		s.PushInt(100)
		s.PushInt(dur)
		if err := handleLocChange(s); err == nil {
			t.Errorf("handleLocChange dur=%d must reject", dur)
		}
	}
}

func TestLocChangeRejectsUnknownType(t *testing.T) {
	s := newScriptStateWithActiveLoc(t, &fakeActiveLoc{locType: 100})
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(9999) // unknown
	s.PushInt(3)
	if err := handleLocChange(s); err == nil {
		t.Error("handleLocChange with unknown type id must return error")
	}
}
```

If `newScriptStateWithActiveLoc`, `initStack`, `newConfigsWithLocType` test helpers don't exist, add minimal ones at the top of `handlers_loc_test.go`:

```go
const testStackCapacity = 1000

func initStack(s *ScriptState) {
	s.IntStack = make([]int, testStackCapacity)
	s.StringStack = make([]string, testStackCapacity)
}

func newScriptStateWithActiveLoc(t *testing.T, loc ActiveLoc) *ScriptState {
	t.Helper()
	s := &ScriptState{ActiveLoc: loc}
	initStack(s)
	return s
}

type configsLocOnly struct {
	locTypes map[int]*objtype.LocType
}

func (c *configsLocOnly) LocType(id int) *objtype.LocType { return c.locTypes[id] }
func (c *configsLocOnly) ObjType(int) *objtype.ObjType    { return nil }
func (c *configsLocOnly) NpcType(int) *objtype.NpcType    { return nil }
func (c *configsLocOnly) EnumType(int) *objtype.EnumType  { return nil }
func (c *configsLocOnly) StructType(int) *objtype.StructType { return nil }
func (c *configsLocOnly) ParamType(int) *objtype.ParamType { return nil }
func (c *configsLocOnly) InvType(int) *objtype.InvType    { return nil }
func (c *configsLocOnly) IdkType(int) *objtype.IdkType    { return nil }
func (c *configsLocOnly) SpotAnimType(int) *objtype.SpotanimType { return nil }
func (c *configsLocOnly) DbTableType(int) *objtype.DbTableType   { return nil }
func (c *configsLocOnly) DbRowType(int) *objtype.DbRowType       { return nil }
func (c *configsLocOnly) DbRowsInTable(int) []int                { return nil }
func (c *configsLocOnly) FindDbRowsInt(int32, int) []int         { return nil }
func (c *configsLocOnly) FindDbRowsStr(string, int) []int        { return nil }

func newConfigsWithLocType(id int, lt *objtype.LocType) *configsLocOnly {
	return &configsLocOnly{locTypes: map[int]*objtype.LocType{id: lt}}
}
```

Verify the existing fixture surface with:
```
grep -n "StackCapacity\|fakeActiveLoc\|configsLocOnly" pkg/script/*_test.go
```

If a Configs mock already exists, reuse it; the snippet above is a fallback.

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestLocChange" -v
```

Expected: FAIL — `handleLocChange` undefined.

- [ ] **Step 3: Implement handleLocChange + checkDuration**

Append to `pkg/script/handlers_loc.go`:

```go
// checkDuration mirrors TS DurationValid (ScriptValidators.ts:108) — a
// range validator rejecting [<1, >2147483647]. Reused by LOC_CHANGE,
// LOC_ADD, LOC_DEL.
func checkDuration(v int) error {
	if v < 1 || v > 2147483647 {
		return fmt.Errorf("duration out of range [1, 2147483647]: %d", v)
	}
	return nil
}

// handleLocChange pops [id, duration] from the int stack and asks
// LocOps to mutate the ActiveLoc's type to id, preserving shape/angle.
// Mirrors TS LOC_CHANGE (LocOps.ts:60-67):
//
//	const [id, duration] = state.popInts(2);
//	check(duration, DurationValid);
//	check(id, LocTypeValid);
//	World.changeLoc(state.activeLoc, id, state.activeLoc.shape, state.activeLoc.angle, duration);
func handleLocChange(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_CHANGE"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_CHANGE"); err != nil {
		return err
	}
	duration := s.PopInt()
	id := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_CHANGE: %w", err)
	}
	if s.Configs.LocType(id) == nil {
		return fmt.Errorf("LOC_CHANGE: unknown loc id %d", id)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_CHANGE: LocOps unavailable")
	}
	return s.LocOps.ChangeLoc(s.ActiveLoc, id, s.ActiveLoc.Shape(), s.ActiveLoc.Angle(), duration)
}
```

If `requireConfigs` doesn't exist in scope, locate via:
```
grep -n "func requireConfigs\b" pkg/script/*.go
```

(Found earlier at handlers_npc.go siblings — confirm exact location and import the helper or inline the nil-check.)

- [ ] **Step 4: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestLocChange" -v
```

Expected: PASS for all 4 LocChange tests.

- [ ] **Step 5: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_loc.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-86 B3.2 — handleLocChange + checkDuration validator

LOC_CHANGE pops [id, duration], validates DurationValid + LocTypeValid,
calls LocOps.ChangeLoc with preserved shape/angle. Mirrors TS LOC_CHANGE
(LocOps.ts:60-67).

checkDuration is the [1, 2147483647] range validator from TS
ScriptValidators.ts:108; reused by LOC_ADD/LOC_DEL in next tasks.

4 new tests: pop-order + ActiveLoc gate + duration range + unknown type."
```

---

### Task 3.3: handleLocAdd (TDD)

**Files:**
- Modify: `pkg/script/handlers_loc.go` (append handleLocAdd)
- Modify: `pkg/script/handlers_loc_test.go` (append tests)

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_loc_test.go`:

```go
func TestLocAddSameLayerCallsChangeOnExisting(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	cfg := newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.Configs = cfg
	existing := &fakeActiveLoc{locType: 50, shape: 0, angle: 0, layer: 0} // layer 0 = wall
	ops := &fakeLocOps{atCoord: []ActiveLoc{existing}}
	s.LocOps = ops

	level, x, z := 0, 3094, 3106
	coord := coordgrid.PackCoord(level, x, z)

	// stack: [coord, type=100, angle=0, shape=0 (wall→layer0), duration=3]
	s.PushInt(coord)
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0) // ShapeWallStraight → LayerWall (0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if len(ops.changeCalls) != 1 {
		t.Errorf("expected ChangeLoc on same-layer existing, got %d ChangeLoc calls", len(ops.changeCalls))
	}
	if len(ops.addCalls) != 0 {
		t.Errorf("expected no AddLoc when same-layer hit, got %d AddLoc calls", len(ops.addCalls))
	}
	if s.ActiveLoc != existing {
		t.Error("ActiveLoc must bind to the existing same-layer loc")
	}
}

func TestLocAddNoSameLayerCallsAddOnNew(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	cfg := newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.Configs = cfg
	// existing is on a DIFFERENT layer (groundDecor=3 vs wall=0)
	existing := &fakeActiveLoc{locType: 50, layer: 3}
	created := &fakeActiveLoc{locType: 100, shape: 0, angle: 0, layer: 0}
	ops := &fakeLocOps{atCoord: []ActiveLoc{existing}, addReturn: created}
	s.LocOps = ops

	coord := coordgrid.PackCoord(0, 3094, 3106)
	s.PushInt(coord)
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0) // wall layer
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if len(ops.addCalls) != 1 {
		t.Errorf("expected AddLoc, got %d AddLoc calls", len(ops.addCalls))
	}
	if len(ops.changeCalls) != 0 {
		t.Errorf("no same-layer hit should not call ChangeLoc, got %d", len(ops.changeCalls))
	}
}

func TestLocAddRejectsBadDuration(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(0) // bad duration
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd dur=0 must reject")
	}
}

func TestLocAddRejectsUnknownType(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(9999) // unknown
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd unknown type must reject")
	}
}

func TestLocAddRejectsBadShape(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(99) // shape > 22 → invalid
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd bad shape must reject")
	}
}

func TestLocAddRejectsBadAngle(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(99) // angle > 3 → invalid
	s.PushInt(0)
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd bad angle must reject")
	}
}
```

Add `addReturn ActiveLoc` field to `fakeLocOps`:

```go
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
	addReturn   ActiveLoc // returned from AddLoc
}

func (f *fakeLocOps) AddLoc(level, x, z, typ, shape, angle, dur int) (ActiveLoc, error) {
	f.addCalls = append(f.addCalls, addLocCall{level, x, z, typ, shape, angle, dur})
	return f.addReturn, nil
}
```

Add `layer int` field to `fakeActiveLoc` (if not done in 3.1):
```go
type fakeActiveLoc struct {
	locType, shape, angle, layer int
	level, x, z                  int
}
func (f *fakeActiveLoc) Layer() int { return f.layer }
```

Add coordgrid import:
```go
"github.com/zsrv/goscape/pkg/coordgrid"
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestLocAdd" -v
```

Expected: FAIL — `handleLocAdd` undefined.

- [ ] **Step 3: Implement handleLocAdd**

Append to `pkg/script/handlers_loc.go`:

```go
// handleLocAdd pops [coord, type, angle, shape, duration] and either
// (a) finds a same-layer loc at coord and changes it, or (b) creates a
// new DESPAWN-lifecycle loc. Mirrors TS LOC_ADD (LocOps.ts:18-43):
//
//	const [coord, type, angle, shape, duration] = state.popInts(5);
//	[validators]
//	for loc at zone-coord:
//	    if loc.layer === locShapeLayer(shape):
//	        World.changeLoc(loc, type, shape, angle, duration); return
//	const created = new Loc(level, x, z, locType.width, locType.length, DESPAWN, type, shape, angle);
//	World.addLoc(created, duration);
func handleLocAdd(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_ADD"); err != nil {
		return err
	}
	duration := s.PopInt()
	shape := s.PopInt()
	angle := s.PopInt()
	typ := s.PopInt()
	coord := s.PopInt()

	pos := coordgrid.UnpackCoord(coord)
	if s.Configs.LocType(typ) == nil {
		return fmt.Errorf("LOC_ADD: unknown loc id %d", typ)
	}
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkLocShape(shape); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ADD: LocOps unavailable")
	}

	wantLayer := int(loc.LayerOf(loc.Shape(shape)))
	for _, existing := range s.LocOps.LocsAtCoord(pos.Level, pos.X, pos.Z) {
		if existing.Layer() == wantLayer {
			if err := s.LocOps.ChangeLoc(existing, typ, shape, angle, duration); err != nil {
				return err
			}
			s.ActiveLoc = existing
			return nil
		}
	}
	created, err := s.LocOps.AddLoc(pos.Level, pos.X, pos.Z, typ, shape, angle, duration)
	if err != nil {
		return err
	}
	s.ActiveLoc = created
	return nil
}
```

Imports needed in handlers_loc.go (verify):
```go
"github.com/zsrv/goscape/pkg/coordgrid"
"github.com/zsrv/goscape/pkg/pathfinder/loc"
```

- [ ] **Step 4: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestLocAdd" -v
```

Expected: PASS for all 6 LocAdd tests.

- [ ] **Step 5: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_loc.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-86 B3.3 — handleLocAdd with same-layer search

LOC_ADD pops [coord, type, angle, shape, duration], validates each,
unpacks coord, then iterates LocOps.LocsAtCoord(level,x,z): if any loc
shares the new shape's layer, calls ChangeLoc on it; else AddLoc
creates a fresh DESPAWN loc. ActiveLoc binds to the resulting loc.
Mirrors TS LOC_ADD (LocOps.ts:18-43).

6 new tests: same-layer hit (Change) + cross-layer miss (Add) +
duration/type/shape/angle validators."
```

---

### Task 3.4: handleLocDel + handleLocAnim + dispatch wiring (TDD)

**Files:**
- Modify: `pkg/script/handlers_loc.go` (append handleLocDel + handleLocAnim)
- Modify: `pkg/script/handlers_loc_test.go` (append tests)
- Modify: `pkg/script/handlers.go` (dispatch)

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_loc_test.go`:

```go
func TestLocDelCallsLocOps(t *testing.T) {
	loc := &fakeActiveLoc{locType: 100}
	s := newScriptStateWithActiveLoc(t, loc)
	ops := &fakeLocOps{}
	s.LocOps = ops
	s.PushInt(5)
	if err := handleLocDel(s); err != nil {
		t.Fatalf("handleLocDel: %v", err)
	}
	if len(ops.removeCalls) != 1 || ops.removeCalls[0].dur != 5 || ops.removeCalls[0].loc != loc {
		t.Errorf("RemoveLoc call: %+v", ops.removeCalls)
	}
}

func TestLocDelRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.PushInt(5)
	if err := handleLocDel(s); err == nil {
		t.Error("handleLocDel without ActiveLoc must error")
	}
}

func TestLocDelRejectsBadDuration(t *testing.T) {
	s := newScriptStateWithActiveLoc(t, &fakeActiveLoc{})
	s.LocOps = &fakeLocOps{}
	s.PushInt(0)
	if err := handleLocDel(s); err == nil {
		t.Error("handleLocDel dur=0 must reject")
	}
}

func TestLocAnimCallsLocOps(t *testing.T) {
	loc := &fakeActiveLoc{locType: 100}
	s := newScriptStateWithActiveLoc(t, loc)
	cfg := newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	cfg.seqTypes = map[int]*objtype.SeqType{42: {ConfigType: objtype.ConfigType{ID: 42}}}
	s.Configs = cfg
	ops := &fakeLocOps{}
	s.LocOps = ops
	s.PushInt(42)
	if err := handleLocAnim(s); err != nil {
		t.Fatalf("handleLocAnim: %v", err)
	}
	if len(ops.animCalls) != 1 || ops.animCalls[0].seq != 42 {
		t.Errorf("AnimLoc call: %+v", ops.animCalls)
	}
}

func TestLocAnimRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{}
	initStack(s)
	s.PushInt(42)
	if err := handleLocAnim(s); err == nil {
		t.Error("handleLocAnim without ActiveLoc must error")
	}
}

func TestLocAnimRejectsUnknownSeq(t *testing.T) {
	s := newScriptStateWithActiveLoc(t, &fakeActiveLoc{})
	s.Configs = newConfigsWithLocType(100, &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}})
	s.LocOps = &fakeLocOps{}
	s.PushInt(9999)
	if err := handleLocAnim(s); err == nil {
		t.Error("handleLocAnim unknown seq must reject")
	}
}
```

Extend `configsLocOnly` test fixture to support SeqType:
```go
type configsLocOnly struct {
	locTypes map[int]*objtype.LocType
	seqTypes map[int]*objtype.SeqType
}

func (c *configsLocOnly) SpotAnimType(int) *objtype.SpotanimType { return nil }
// add:
// (already in Configs interface above; SpotAnimType not SeqType)
```

Wait — verify Configs interface has SeqType. From pre-flight `pkg/script/configs.go:10-31` listed `SpotAnimType(id int) *objtype.SpotanimType` but NO `SeqType(id) *objtype.SeqType`. So we need to ADD SeqType to the Configs interface (Bundle 3 pre-req):

```go
// In pkg/script/configs.go Configs interface, append:
SeqType(id int) *objtype.SeqType
```

And update the production impl in `modules/world/script.go` (find the Configs implementation; grep `func.*SpotAnimType.*objtype.SpotanimType\b`):

```
grep -n "SpotAnimType.*objtype" modules/world/*.go
```

Add a `SeqType(id int) *objtype.SeqType` method beside SpotAnimType returning `s.seqTypes.Configs[id]` (with bounds check).

Update `configsLocOnly` test fixture:
```go
func (c *configsLocOnly) SeqType(id int) *objtype.SeqType {
	return c.seqTypes[id]
}
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestLocDel|TestLocAnim" -v
```

Expected: FAIL — `handleLocDel`/`handleLocAnim` undefined; `Configs.SeqType` undefined.

- [ ] **Step 3: Add SeqType to Configs interface + production impl**

Edit `pkg/script/configs.go`:
```go
type Configs interface {
	// ... existing ...
	SeqType(id int) *objtype.SeqType
}
```

Edit `modules/world/script.go` (or wherever the Configs implementation lives — confirm with grep `func.*SpotAnimType.*objtype.SpotanimType`):

```go
func (s *Server) SeqType(id int) *objtype.SeqType {
	if s.seqTypes == nil || id < 0 || id >= len(s.seqTypes.Configs) {
		return nil
	}
	return s.seqTypes.Configs[id]
}
```

Update any other Configs implementations (test mocks elsewhere) — grep:

```
grep -rn "func.*SpotAnimType.*objtype.SpotanimType" pkg/ modules/ -t go
```

Each implementor adds `SeqType`.

- [ ] **Step 4: Implement handleLocDel + handleLocAnim**

Append to `pkg/script/handlers_loc.go`:

```go
// handleLocDel pops [duration] and removes the ActiveLoc. Mirrors TS
// LOC_DEL (LocOps.ts:74-77).
func handleLocDel(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_DEL"); err != nil {
		return err
	}
	duration := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_DEL: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_DEL: LocOps unavailable")
	}
	return s.LocOps.RemoveLoc(s.ActiveLoc, duration)
}

// handleLocAnim pops [seq], validates against Configs.SeqType, and
// dispatches an animation event for the ActiveLoc. Mirrors TS LOC_ANIM
// (LocOps.ts:50-54).
func handleLocAnim(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANIM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_ANIM"); err != nil {
		return err
	}
	seq := s.PopInt()
	if s.Configs.SeqType(seq) == nil {
		return fmt.Errorf("LOC_ANIM: unknown seq id %d", seq)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ANIM: LocOps unavailable")
	}
	return s.LocOps.AnimLoc(s.ActiveLoc, seq)
}
```

- [ ] **Step 5: Wire dispatch in handlers.go**

Edit `pkg/script/handlers.go` (around line 125, the LOC active-loc reads block). Rename comment to "LOC active-loc reads + mutations" and insert in lexical order:

```go
// LOC active-loc reads + mutations.
OpLocAdd:    handleLocAdd,
OpLocAngle:  handleLocAngle,
OpLocAnim:   handleLocAnim,
OpLocChange: handleLocChange,
OpLocDel:    handleLocDel,
OpLocName:   handleLocName,
OpLocOp:     handleLocOp,
OpLocParam:  handleLocParam,
OpLocShape:  handleLocShape,
OpLocType:   handleLocType,
```

- [ ] **Step 6: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v
```

Expected: PASS for all script tests.

- [ ] **Step 7: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_loc.go pkg/script/handlers_loc_test.go pkg/script/handlers.go pkg/script/configs.go modules/world/script.go
git commit --no-gpg-sign -m "feat(script): NAI-86 B3.4 — handleLocDel + handleLocAnim + dispatch wiring

LOC_DEL pops [duration], validates, calls LocOps.RemoveLoc. Mirrors
TS LOC_DEL (LocOps.ts:74-77). LOC_ANIM pops [seq], validates against
Configs.SeqType, calls LocOps.AnimLoc. Mirrors TS LOC_ANIM
(LocOps.ts:50-54).

Configs interface gains SeqType(id) *objtype.SeqType (production impl on
modules/world/Server). Updates dispatch wiring with all four mutators
in the renamed 'LOC active-loc reads + mutations' block.

6 new tests: handler-level pop-order/gates/validators."
```

---

### Task 3.5: Server LocOps adapter (TDD)

**Files:**
- Create: `modules/world/script_loc_ops.go`
- Test: `modules/world/script_loc_ops_test.go`

- [ ] **Step 1: Write failing tests**

Create `modules/world/script_loc_ops_test.go`:

```go
package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestServerLocOpsChangeLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	if err := ops.ChangeLoc(loc, 100, loc.Shape(), loc.Angle(), 1); err != nil {
		t.Fatalf("ChangeLoc: %v", err)
	}
	if loc.Type() != 100 {
		t.Errorf("Type after Change: got %d", loc.Type())
	}
}

func TestServerLocOpsAddLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}

	created, err := ops.AddLoc(0, 3094, 3106, 100, 0, 0, 1)
	if err != nil {
		t.Fatalf("AddLoc: %v", err)
	}
	if created == nil {
		t.Fatal("AddLoc must return a non-nil ActiveLoc")
	}
	loc, ok := created.(*entitypkg.Loc)
	if !ok {
		t.Fatalf("AddLoc returned %T, want *entity.Loc", created)
	}
	if !loc.IsActive {
		t.Error("created loc must have IsActive=true")
	}
}

func TestServerLocOpsRemoveLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	if err := ops.RemoveLoc(loc, 1); err != nil {
		t.Fatalf("RemoveLoc: %v", err)
	}
	if loc.IsActive {
		t.Error("loc must be inactive after RemoveLoc")
	}
}

func TestServerLocOpsLocsAtCoord(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	at := ops.LocsAtCoord(0, 3094, 3106)
	if len(at) != 1 {
		t.Errorf("LocsAtCoord: got %d, want 1", len(at))
	}
}

func TestServerLocOpsRejectsNonLocActiveLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}
	// pass a non-*Loc impl to ChangeLoc
	other := &fakeNonLoc{}
	if err := ops.ChangeLoc(other, 100, 0, 0, 1); err == nil {
		t.Error("ChangeLoc with non-*Loc ActiveLoc must error")
	}
}

type fakeNonLoc struct{}

func (f *fakeNonLoc) LocType() int                  { return 0 }
func (f *fakeNonLoc) Coords() (int, int, int)        { return 0, 0, 0 }
func (f *fakeNonLoc) Angle() int                    { return 0 }
func (f *fakeNonLoc) Shape() int                    { return 0 }
func (f *fakeNonLoc) Layer() int                    { return 0 }

// Suppress unused warning
var _ = objtype.LocType{}
```

- [ ] **Step 2: Run tests; expect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestServerLocOps" -v
```

Expected: FAIL — `serverLocOps` undefined.

- [ ] **Step 3: Create `modules/world/script_loc_ops.go`**

```go
package world

import (
	"fmt"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

// serverLocOps adapts *Server to the script.LocOps interface so script
// handlers can drive World mutations without leaking the *entity.Loc
// concrete type into pkg/script. Type-asserts script.ActiveLoc inputs
// to *entity.Loc; non-Loc inputs error out.
type serverLocOps struct {
	s *Server
}

func (o *serverLocOps) ChangeLoc(loc script.ActiveLoc, typ, shape, angle, duration int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.ChangeLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.ChangeLoc(l, typ, shape, angle, duration)
	return nil
}

func (o *serverLocOps) AddLoc(level, x, z, typ, shape, angle, duration int) (script.ActiveLoc, error) {
	if o.s.locTypes == nil {
		return nil, fmt.Errorf("LocOps.AddLoc: locTypes unavailable")
	}
	lt := o.s.locTypeOrNil(typ)
	if lt == nil {
		return nil, fmt.Errorf("LocOps.AddLoc: unknown loc id %d", typ)
	}
	width := lt.Width
	length := lt.Length
	if width == 0 {
		width = 1
	}
	if length == 0 {
		length = 1
	}
	created := entitypkg.NewLoc(level, x, z, width, length, entitypkg.LifecycleDespawn, typ, shape, angle)
	o.s.AddLoc(created, duration)
	return created, nil
}

func (o *serverLocOps) RemoveLoc(loc script.ActiveLoc, duration int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.RemoveLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.RemoveLoc(l, duration)
	return nil
}

func (o *serverLocOps) AnimLoc(loc script.ActiveLoc, seq int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.AnimLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.AnimLoc(l, seq)
	return nil
}

// LocsAtCoord returns the script-side ActiveLoc slice for every loc
// currently in the zone at (level, x, z). NAI-86 LOC_ADD same-layer
// search is the sole caller.
func (o *serverLocOps) LocsAtCoord(level, x, z int) []script.ActiveLoc {
	z2 := o.s.zoneMap.Get(level, x, z)
	out := make([]script.ActiveLoc, 0, len(z2.Locs))
	for _, l := range z2.Locs {
		if l.X == x && l.Z == z && l.Level == level {
			out = append(out, l)
		}
	}
	return out
}
```

**Pre-flight check**: confirm `objtype.LocType` has `Width int` / `Length int` fields:

```
grep -n "Width\b\|Length\b" pkg/objtype/loctype.go | head
```

If missing, fall back to width=1, length=1 unconditionally (TS reads `locType.width` / `locType.length` — must port if absent; if absent, this is a tracked deviation — but pre-flight should have caught it).

- [ ] **Step 4: Run tests; expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestServerLocOps" -v
```

Expected: PASS for all 5 adapter tests.

- [ ] **Step 5: Wire serverLocOps onto ScriptState in production**

Find where `Server` populates `ScriptState.Configs`/`Inv`/etc. Likely `modules/world/script.go` or similar. Grep:

```
grep -rn "s\.Configs =\|state\.Configs =\|state\.Inv =\|\.LocOps =" modules/world/ -t go
```

Add to the same site:
```go
state.LocOps = &serverLocOps{s: s}
```

- [ ] **Step 6: Run full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/script_loc_ops.go modules/world/script_loc_ops_test.go modules/world/script.go
git commit --no-gpg-sign -m "feat(world): NAI-86 B3.5 — serverLocOps adapter for script.LocOps

serverLocOps type-asserts script.ActiveLoc → *entity.Loc and dispatches
to Server.ChangeLoc/AddLoc/RemoveLoc/AnimLoc. AddLoc constructs a
DESPAWN loc using the LocType's Width/Length per TS LOC_ADD. ScriptState
gets the adapter wired at production script-init.

5 new tests cover each adapter method + non-Loc rejection.

NAI-86 Bundle 3 complete: LOC_CHANGE/ADD/DEL/ANIM handlers fully wired
end-to-end script→world. Bundle 4 = door-click smoke + close."
```

---

## Bundle 4 — Door-click smoke + sub-spec close

### Task 4.1: Door-click smoke handoff

**Files:** none (handoff only)

- [ ] **Step 1: Confirm full repo green at HEAD**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS / clean.

- [ ] **Step 2: Note HEAD SHA for smoke binding**

```
git log --oneline -1
```

Record the SHA in the close commit message (Task 4.2).

- [ ] **Step 3: Compose smoke handoff message for the user**

Per `smoke_test_server_handoff.md`, request user-launched server smoke. Message template:

> NAI-86 Bundle 3 complete at HEAD `<SHA>`. Please run the server and click the Tutorial Island newbie door. Acceptance:
> 1. Server log shows no `no handler for LOC_CHANGE` or `no handler for LOC_ADD` warnings.
> 2. Door visually animates open in the Java client.
> 3. Door tile becomes walkable; player walks through.
> 4. After ~3 ticks the door auto-reverts to closed; tile becomes blocking again.
>
> Report back which of (1)-(4) hold. Any residual = open follow-up entry; either route in-scope (≤30 LOC) or NAI-87 per `smoke_surfaces_adjacent_divergences.md`.

- [ ] **Step 4: Wait for user smoke result**

Block on user reply.

---

### Task 4.2: Sub-spec close commit

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (retire LOC_CHANGE entry; add new follow-ups if smoke surfaced any)
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (only if a new memory entry was added)

- [ ] **Step 1: Update nai_followups.md**

Read `nai_followups.md`, locate the NAI-85 close paragraph mentioning LOC_CHANGE rolling forward. Either:
- Replace with NAI-86 close paragraph (smoke result + cascade-attribution per `cascade_theory_smoke_binding.md`), OR
- Append new entry for whatever surfaced post-smoke.

- [ ] **Step 2: Compose close commit per `close_commit_memory_trailer.md`**

```bash
git add docs/superpowers/specs/2026-05-04-nai-86-loc-mutator-family-port-design.md \
        docs/superpowers/plans/2026-05-04-nai-86-loc-mutator-family-port.md
git commit --no-gpg-sign -m "close: NAI-86 — LOC_CHANGE/ADD/DEL/ANIM + lifecycle revert tick processor ported

4 bundles, ~<X> production LOC + ~<Y> test LOC across <N> commits.

Bundle 1 (B1.1, B1.2): Loc.BaseInfo/CurrentInfo split + Layer/IsActive +
Server.AddLoc/ChangeLoc/RemoveLoc collision wiring.

Bundle 2 (B2.1-B2.4): LifecycleTracker interface + locObjTracker on Server +
Server.RevertLoc + Server.turnLoc dispatch + processZones extension to drive
turnLoc from the snapshot.

Bundle 3 (B3.1-B3.5): LocOps interface + handleLocChange/Add/Del/Anim +
checkDuration validator + Configs.SeqType + serverLocOps adapter +
production wiring on ScriptState.

Bundle 4: door-click smoke at HEAD <SHA-pre-close> — <result>.

Tracked deviations from spec §5: D-N86-1 (LocObjEvent.check skipped),
D-N86-2 (IsActive carries TS isActive semantics; IsValid stays
intrinsic), D-N86-3 (Obj.Turn deferred), D-N86-4 (absolute-tick vs
TS lifecycleTick--).

Closes memory: <list any nai_followups.md entries this retired>"
```

- [ ] **Step 3: (Conditional) Save new memory entries**

If the smoke surfaced anything non-derivable per the auto-memory rules, save it to `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/`. Common candidates after a port like this:
- New cascade-blocker observed → add to `nai_followups.md`
- A non-obvious test idiom learned → save to memory if it'll generalize
- A surprising TS-fidelity decision → save to memory

Skip if nothing notable surfaced.

---

## Self-Review

**Spec coverage check:** every spec section has a task.
- §4.1 (Loc entity + collision): Tasks 1.1, 1.2 ✓
- §4.2 (Lifecycle revert tick processor): Tasks 2.1, 2.2, 2.3, 2.4 ✓
- §4.3 (Script handlers): Tasks 3.1, 3.2, 3.3, 3.4, 3.5 ✓
- §4.4 (Smoke + close): Tasks 4.1, 4.2 ✓
- §5 (Deviations): pinned in close commit ✓
- §6 (Risks): each risk has explicit mitigation in the relevant task

**Placeholder scan:** `<X>`, `<Y>`, `<N>`, `<SHA>`, `<SHA-pre-close>`, `<result>`, `<list any nai_followups.md entries this retired>` are all forward-fill markers in Task 4.2's close-commit template — filled at smoke time. No TODO/TBD in production-code steps.

**Type consistency:**
- `Server.AddLoc` signature `(loc, duration int)` — consistent across 1.2, 2.3, 3.5.
- `Server.ChangeLoc` signature `(loc, typ, shape, angle, duration int)` — consistent.
- `Server.RemoveLoc` signature `(loc, duration int)` — consistent.
- `LocOps` interface — same shape in 3.1, 3.2, 3.3, 3.4, 3.5.
- `Loc.IsActive bool`, `Loc.IsChanged() bool`, `Loc.Change(typ, shape, angle int)`, `Loc.Revert()`, `Loc.Layer() int` — all referenced consistently.
- `entity.LifecycleTracker.Register(*NonPathing)` / `Unregister(*NonPathing)` — consistent in 2.1, 2.2.
- `NonPathing.SetLifeCycle(duration, currentTick int, tracker LifecycleTracker)` — consistent in 1.2-stub, 2.1-real, 2.3.
- `serverLocOps` impl in 3.5 matches `script.LocOps` interface from 3.1.
