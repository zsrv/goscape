# NAI-92: Full SMART pathfinding port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `PathingEntity.pathToTarget` (PathingEntity.ts:457-508) and `Npc.pathToTarget` override (Npc.ts:319-335) faithfully to goscape. Reshape `pathToTarget` from `(tx, tz int)` to arg-less form that reads `p.target` directly and dispatches by target type to shape-aware findPath helpers. Closes the long-standing NAI-11 SMART-pathfinding deferral and unblocks the Survival Expert NPC reachability symptom flagged by NAI-91 smoke.

**Architecture:** Seven sequential bundles. B1 lays the foundation: named wrapper API in `pkg/pathfinder/routefinder/api.go` (`FindPathPlain`/`FindPathToEntity`/`FindPathToLoc`/`FindNaivePath`), `coordgrid.Intersects`, and entity-side helpers (`Width()`/`Length()`/`blockWalkFlag()`/`getCollisionStrategy()` on Player+Npc, plus a `pathingEntity` interface). B2-B5 build out Player's `pathToTarget()` SMART (Loc → PathingEntity → Obj) + NAIVE + nomove-else branches. B6 reshapes Npc's `pathToTarget` to mirror Npc.ts:319-335 (intersect-shortcut + base delegation). B7 retires the legacy `(tx, tz)` signature comments, confirms `FindPathDefault` is gone from production, and hands off to user-launched smoke.

**Tech Stack:** Go 1.26+, Go test (table-driven matrix). TDD per `superpowers:test-driven-development`. Per `runescript_cadence` two-stage review per bundle.

**Spec:** `docs/superpowers/specs/2026-05-04-nai-92-smart-pathfinding-port-design.md` (committed at `ff4b612`).

**Predecessor close:** NAI-91 (commit `7eb9742`) — closed door re-click reach gate; left Survival Expert NPC unreachable per §10 deferral, routed to NAI-92.

---

## File Structure

**Created:**
- `modules/world/pathing.go` — new file housing the `pathingEntity` interface, `routeToWaypoints` shared helper if needed.

**Modified (production):**
- `pkg/pathfinder/routefinder/api.go` — rename `FindPathDefault` → `FindPathPlain`; add `FindPathToEntity`, `FindPathToLoc`, `FindNaivePath` wrappers.
- `pkg/coordgrid/coordgrid.go` — add `Intersects` helper.
- `modules/world/player.go` — add `Width()`, `Length()`, `blockWalkFlag()`, `getCollisionStrategy()` methods.
- `modules/world/npc.go` — add `Width()`, `Length()`, `blockWalkFlag()`, `getCollisionStrategy()` methods.
- `modules/world/movement.go` — update `pathToMoveClick` caller from `FindPathDefault` → `FindPathPlain`.
- `modules/world/interaction.go` — replace `pathToTarget(tx, tz)` body with arg-less type-switch + sub-method dispatch (`pathToTargetSmart`/`pathToTargetNaive`/`pathToTargetNoStrategy`); update single caller at `interaction.go:237`.
- `modules/world/npc_interaction.go` — replace `(*Npc).pathToTarget` body with intersect-shortcut + base delegation; add `pathToTargetBase`, `pathToTargetSmart`, `pathToTargetNaive`, `pathToTargetNoStrategy` NPC-side methods.

**Modified (tests):**
- `pkg/pathfinder/routefinder/api_test.go` — add 4 wrapper tests pinning the `FindPath` arg vector for each new wrapper.
- `pkg/coordgrid/coordgrid_test.go` — add `Intersects` matrix tests.
- `modules/world/player_test.go` / `modules/world/npc_test.go` — add `blockWalkFlag` + `getCollisionStrategy` parametrized tests.
- `modules/world/interaction_test.go` — migrate ~6 existing `pathToTarget(tx, tz)` test sites; add SMART/Loc, SMART/PathingEntity, SMART/Obj, NAIVE, no-strategy matrices.
- `modules/world/interaction_debug_test.go` — migrate 1 existing site.
- `modules/world/npc_interaction_test.go` — migrate ~2 existing sites; add intersect-shortcut + base-delegation matrices.
- `modules/world/npc_player_modes_test.go` — migrate ~2 existing sites.
- `modules/world/walk_trigger_fallback_test.go` — verify still GREEN (no signature touch).

---

## Bundle 1 — Wrapper API + entity helpers + coordgrid.Intersects

**Bundle goal:** Pure-additive groundwork. After B1, all wrapper APIs and entity-side helpers exist. The single `FindPathDefault` → `FindPathPlain` rename is atomic with the one caller update at `movement.go:142`.

**Files touched:**
- Create: `modules/world/pathing.go`
- Modify: `pkg/pathfinder/routefinder/api.go`, `pkg/coordgrid/coordgrid.go`, `modules/world/player.go`, `modules/world/npc.go`, `modules/world/movement.go`
- Test: `pkg/pathfinder/routefinder/api_test.go`, `pkg/coordgrid/coordgrid_test.go`, `modules/world/player_test.go`, `modules/world/npc_test.go`

### Task 1.1 — Pre-flight HEAD verification

- [ ] **Step 1: Re-grep all premise line numbers**

```bash
rg -n "FindPathDefault\b" pkg/pathfinder/routefinder/api.go modules/world/movement.go
rg -n "func \(pf PathFinderAPI\)" pkg/pathfinder/routefinder/api.go
rg -n "MoveRestrict\w+" modules/world/movement_consts.go
rg -n "type entity interface" modules/world/movement_consts.go
rg -n "size\s+int\b|moveRestrict\s+MoveRestrict" modules/world/npc.go
rg -n "moveRestrict\s+MoveRestrict|moveStrategy\s+MoveStrategy" modules/world/player.go
```

Expected:
- `api.go:37` — `func (pf PathFinderAPI) FindPathDefault(level, srcX, srcZ, destX, destZ int) Route`
- `movement.go:142` — sole production caller `gamemap.Pathfinder.FindPathDefault(...)`
- `movement_consts.go:17-24` — `MoveRestrictNormal/Blocked/Indoors/Outdoors/NoMove/Passthru`
- `movement_consts.go:45-49` — `entity` interface
- `npc.go:121` — `size int` field; `npc.go:58` — `moveRestrict MoveRestrict`
- `player.go:95-96` — `moveRestrict MoveRestrict`, `moveStrategy MoveStrategy`

If any line number drifted, use the new line. If a signature drifted, **stop and report**.

- [ ] **Step 2: Confirm `pathingEntity` is unused**

```bash
rg -n "pathingEntity\b" modules/world/
```

Expected: zero hits. (NAI-92 introduces the symbol.)

### Task 1.2 — Add `coordgrid.Intersects`

**Files:**
- Modify: `pkg/coordgrid/coordgrid.go` (append at EOF)
- Test: `pkg/coordgrid/coordgrid_test.go` (append at EOF)

This is TDD.

- [ ] **Step 1: Write failing matrix tests**

Append to `pkg/coordgrid/coordgrid_test.go`:

```go
func TestIntersects(t *testing.T) {
	cases := []struct {
		name                   string
		sx, sz, sw, sl         int
		dx, dz, dw, dl         int
		want                   bool
	}{
		{"identical 1x1", 5, 5, 1, 1, 5, 5, 1, 1, true},
		{"adjacent east 1x1", 5, 5, 1, 1, 6, 5, 1, 1, false},
		{"adjacent north 1x1", 5, 5, 1, 1, 5, 6, 1, 1, false},
		{"src contains dest", 5, 5, 3, 3, 6, 6, 1, 1, true},
		{"dest contains src", 5, 5, 1, 1, 4, 4, 3, 3, true},
		{"overlap NE corner", 5, 5, 2, 2, 6, 6, 2, 2, true},
		{"disjoint far east", 5, 5, 1, 1, 10, 5, 1, 1, false},
		{"disjoint far north", 5, 5, 1, 1, 5, 10, 1, 1, false},
		{"touching edge east", 5, 5, 2, 1, 7, 5, 1, 1, false}, // src right edge at 7, dest at 7 → touching, NOT overlap
		{"touching edge north", 5, 5, 1, 2, 5, 7, 1, 1, false},
		{"2x2 vs 2x2 overlap one tile", 5, 5, 2, 2, 6, 6, 2, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Intersects(tc.sx, tc.sz, tc.sw, tc.sl, tc.dx, tc.dz, tc.dw, tc.dl)
			if got != tc.want {
				t.Errorf("Intersects(%d,%d,%d,%d, %d,%d,%d,%d) = %v, want %v",
					tc.sx, tc.sz, tc.sw, tc.sl, tc.dx, tc.dz, tc.dw, tc.dl, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run, expect compile failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/ -run TestIntersects -v
```

Expected: `undefined: Intersects` compile error.

- [ ] **Step 3: Add `Intersects` to `coordgrid.go`**

Append:

```go
// Intersects reports whether two axis-aligned bounding boxes overlap.
// Mirrors TS CoordGrid.intersects (Engine-TS/.../CoordGrid.ts:144-150):
// touching edges (e.g. src right at x=7, dest at x=7) do NOT overlap.
func Intersects(srcX, srcZ, srcW, srcL, destX, destZ, destW, destL int) bool {
	srcHorizontal := srcX + srcW
	srcVertical := srcZ + srcL
	destHorizontal := destX + destW
	destVertical := destZ + destL
	return !(destX >= srcHorizontal || destHorizontal <= srcX || destZ >= srcVertical || destVertical <= srcZ)
}
```

- [ ] **Step 4: Run, expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/ -run TestIntersects -v
```

Expected: PASS for all 11 cases.

- [ ] **Step 5: Commit**

```bash
git add pkg/coordgrid/coordgrid.go pkg/coordgrid/coordgrid_test.go
git commit --no-gpg-sign -m "feat(coordgrid): NAI-92 B1 — add Intersects bbox overlap helper

Mirrors TS CoordGrid.intersects (CoordGrid.ts:144-150). Used by
NAI-92's pathToTarget SMART branch to gate NODE_CLIENT_ROUTEFINDER
shortcut and Npc-side intersect→FindNaivePath fallback.
"
```

### Task 1.3 — Rename `FindPathDefault` → `FindPathPlain` + add wrapper API

**Files:**
- Modify: `pkg/pathfinder/routefinder/api.go` (lines 36-44)
- Modify: `modules/world/movement.go` (line 142, single caller)
- Test: `pkg/pathfinder/routefinder/api_test.go` (new file or append)

This is a rename + atomic caller update. No test for the rename itself — the existing `pathToMoveClick` tests at `movement_test.go` cover it. Tests for the new wrappers come in 1.4.

- [ ] **Step 1: Read current api.go state**

Open `pkg/pathfinder/routefinder/api.go` lines 36-45.

- [ ] **Step 2: Rename + add wrappers in api.go**

Replace lines 36-44 (the `FindPathDefault` + `FindPath` block) with:

```go
// FindPathPlain mirrors TS findPath (GameMap.ts:378-380). Hardcodes the
// shape-blind 1×1 default search; equivalent to the prior FindPathDefault.
// Used by MOVE_CLICK pipeline (movement.go pathToMoveClick) and by SMART
// pathToTarget's Obj-different-tile fallback branch.
func (pf PathFinderAPI) FindPathPlain(level, srcX, srcZ, destX, destZ int) Route {
	return pf.FindPath(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)
}

// FindPathToEntity mirrors TS findPathToEntity (GameMap.ts:382-384).
// shape=-2 is the entity-target sentinel for rsmod's reach search.
// Used by SMART pathToTarget for *Player / *Npc targets.
func (pf PathFinderAPI) FindPathToEntity(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength int) Route {
	return pf.FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, 0, -2, true, 0, 25, collision.TypeNormal)
}

// FindPathToLoc mirrors TS findPathToLoc (GameMap.ts:386-388). Threads
// loc shape/angle/forceapproach (blockAccessFlags) into rsmod's reach
// search. Used by SMART pathToTarget for *Loc targets.
func (pf PathFinderAPI) FindPathToLoc(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags int) Route {
	return pf.FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, true, blockAccessFlags, 25, collision.TypeNormal)
}

// FindNaivePath mirrors TS findNaivePath (GameMap.ts:390-392). Uses the
// NaiveRouteFinder for straight-line stepping with collision flags.
// Used by SMART pathToTarget's NODE_CLIENT_ROUTEFINDER intersect-shortcut
// and by Npc-side pathToTarget intersect-shortcut, plus NAIVE strategy.
func (pf PathFinderAPI) FindNaivePath(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag int, collisionType collision.Type) Route {
	return pf.NaiveRouteFinder.FindRoute(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag, collisionType)
}

// Deprecated
func (pf PathFinderAPI) FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape int, moveNear bool, blockAccessFlags, maxWaypoints int, collisionType collision.Type) Route {
	return pf.RouteFinder.FindRoute(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, moveNear, blockAccessFlags, maxWaypoints, collisionType)
}
```

Note: `FindPathDefault` is **gone**. `FindPath` (the 14-arg low-level) is preserved.

- [ ] **Step 3: Update the single production caller**

In `modules/world/movement.go:142`, replace:

```go
route := p.client.server.gamemap.Pathfinder.FindPathDefault(p.level, p.x, p.z, dest.X, dest.Z)
```

with:

```go
route := p.client.server.gamemap.Pathfinder.FindPathPlain(p.level, p.x, p.z, dest.X, dest.Z)
```

- [ ] **Step 4: Run full repo tests + grep for stale refs**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
rg "FindPathDefault" pkg/ modules/
```

Expected: all tests PASS; rg returns zero hits in production code (only spec/plan doc references remain).

- [ ] **Step 5: Commit**

```bash
git add pkg/pathfinder/routefinder/api.go modules/world/movement.go
git commit --no-gpg-sign -m "refactor(pathfinder): NAI-92 B1 — rename FindPathDefault→FindPathPlain, add wrapper API

Adds TS GameMap.ts:378-391 parity wrappers — FindPathToEntity (shape=-2
entity sentinel), FindPathToLoc (shape/angle/forceapproach threaded),
FindNaivePath (NaiveRouteFinder pass-through). Renames FindPathDefault
to FindPathPlain to free the canonical TS findPath name. Single-caller
atomic update at movement.go pathToMoveClick.

Sets up B2-B6 SMART pathfinding port in Player.pathToTarget /
Npc.pathToTarget.
"
```

### Task 1.4 — Wrapper API tests

**Files:**
- Create: `pkg/pathfinder/routefinder/api_test.go` (if not exists; otherwise append)

This is TDD.

- [ ] **Step 1: Check existing test file**

```bash
ls pkg/pathfinder/routefinder/api_test.go 2>/dev/null
```

If absent, create. If present, plan to append.

- [ ] **Step 2: Write the four wrapper tests**

```go
package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// newTestPathFinderAPI builds a PathFinderAPI with an empty FlagMap so
// FindPath calls return a Route deterministically (no walls, src=dest).
func newTestPathFinderAPI() PathFinderAPI {
	return NewPathFinderAPI()
}

func TestFindPathPlain_DelegatesToFindPath_WithDefaultArgs(t *testing.T) {
	pf := newTestPathFinderAPI()

	// Same call signature as old FindPathDefault: 5 args, 14-arg expansion.
	// Pin behavioural equivalence by calling both forms with identical inputs
	// and asserting equal Routes. (This is a structural-parity pin, not a
	// pathfinding-correctness pin.)
	wrapper := pf.FindPathPlain(0, 100, 100, 100, 100)
	expanded := pf.FindPath(0, 100, 100, 100, 100, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathPlain != FindPath(... 1, 1, 1, 0, -1, true, 0, 25, Normal)")
	}
}

func TestFindPathToEntity_DelegatesToFindPath_WithEntitySentinel(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindPathToEntity(0, 100, 100, 105, 105, 1, 2, 3)
	// shape=-2 is the entity-target sentinel.
	expanded := pf.FindPath(0, 100, 100, 105, 105, 1, 2, 3, 0, -2, true, 0, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathToEntity != FindPath(... srcSize, destW, destL, 0, -2, true, 0, 25, Normal)")
	}
}

func TestFindPathToLoc_DelegatesToFindPath_WithLocShapeAngle(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindPathToLoc(0, 100, 100, 105, 105, 1, 1, 1, 2 /*angleEast*/, 0 /*wallStraight*/, 7 /*forceapproach*/)
	expanded := pf.FindPath(0, 100, 100, 105, 105, 1, 1, 1, 2, 0, true, 7, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathToLoc != FindPath(... srcSize, destW, destL, angle, shape, true, blockAccessFlags, 25, Normal)")
	}
}

func TestFindNaivePath_DelegatesToNaiveRouteFinder(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindNaivePath(0, 100, 100, 105, 105, 1, 1, 1, 1, 0, collision.TypeNormal)
	expanded := pf.NaiveRouteFinder.FindRoute(0, 100, 100, 105, 105, 1, 1, 1, 1, 0, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindNaivePath != NaiveRouteFinder.FindRoute pass-through")
	}
}

// routesEqual compares two Routes by Waypoints + Alternative + Success.
func routesEqual(a, b Route) bool {
	if a.Alternative != b.Alternative || a.Success != b.Success {
		return false
	}
	if len(a.Waypoints) != len(b.Waypoints) {
		return false
	}
	for i := range a.Waypoints {
		if a.Waypoints[i] != b.Waypoints[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: Run**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/routefinder/ -run "TestFind(PathPlain|PathToEntity|PathToLoc|NaivePath)" -v
```

Expected: PASS x4.

- [ ] **Step 4: Commit**

```bash
git add pkg/pathfinder/routefinder/api_test.go
git commit --no-gpg-sign -m "test(pathfinder): NAI-92 B1 — wrapper API parity tests

Pins each new wrapper's translation to the underlying FindPath
arg vector (or NaiveRouteFinder.FindRoute pass-through for
FindNaivePath). Structural-parity pins; pathfinding correctness
is covered by RouteFinder/NaiveRouteFinder unit tests.
"
```

### Task 1.5 — Add `Width()`/`Length()` + `pathingEntity` interface

**Files:**
- Create: `modules/world/pathing.go`
- Modify: `modules/world/player.go` (append accessors near the existing `Coords()`/`Slot()` methods at line ~534)
- Modify: `modules/world/npc.go` (append accessors after `Slot()` at line ~215)

- [ ] **Step 1: Create `modules/world/pathing.go`**

```go
package world

// pathingEntity is the dimensioned entity interface used by pathToTarget's
// type-switch and SMART/NAIVE branch dispatch. Mirrors TS PathingEntity's
// (width, length) inheritance from the Entity base. *Player and *Npc are
// the two concrete implementations.
type pathingEntity interface {
	entity
	Width() int
	Length() int
}
```

- [ ] **Step 2: Add accessors on Player**

In `modules/world/player.go`, append after `Coords()` at line ~537:

```go
// Width returns the player's tile footprint width. Players are always 1×1.
// Mirrors TS PathingEntity.width inheritance from Entity (Entity.ts).
func (p *Player) Width() int { return 1 }

// Length returns the player's tile footprint length. Players are always 1×1.
func (p *Player) Length() int { return 1 }
```

- [ ] **Step 3: Add accessors on Npc**

In `modules/world/npc.go`, append after `Slot()` at line ~215:

```go
// Width returns the NPC's tile footprint width. NPCs are square (size×size);
// width and length both return n.size. Mirrors TS Npc.width which equals
// NpcType.size at construction.
func (n *Npc) Width() int { return n.size }

// Length returns the NPC's tile footprint length. Square: equals Width().
func (n *Npc) Length() int { return n.size }
```

- [ ] **Step 4: Run repo tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all PASS (no behavioural change yet — `pathingEntity` interface is unconsumed).

- [ ] **Step 5: Commit**

```bash
git add modules/world/pathing.go modules/world/player.go modules/world/npc.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B1 — add pathingEntity interface + Width/Length accessors

Player.Width()=Length()=1 (always 1×1). Npc.Width()=Length()=n.size
(square). pathingEntity interface in new pathing.go for use by B2+
pathToTarget type-switch.
"
```

### Task 1.6 — Add `blockWalkFlag()` + `getCollisionStrategy()` on Player + Npc

**Files:**
- Modify: `modules/world/player.go`
- Modify: `modules/world/npc.go`
- Test: `modules/world/player_test.go` (append)
- Test: `modules/world/npc_test.go` (append, or create if absent)

This is TDD. The MoveRestrict→flag/strategy mapping is read from TS `PathingEntity.blockWalkFlag` (PathingEntity.ts) + `Player.blockWalkFlag` override (Player.ts:706+) + `Npc.blockWalkFlag` override (Npc.ts:381+).

**Plan-author preflight: read TS to derive the exact mapping. Read these three sites BEFORE writing the test code:**

```bash
sed -n '700,720p' /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts
sed -n '375,400p' /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Npc.ts
rg -n "blockWalkFlag\(\)\|getCollisionStrategy\(\)" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts
```

Use the actual TS return tables to fill in the test cases below. **Do not infer; read.**

- [ ] **Step 1: Write failing tests (Player side)**

Append to `modules/world/player_test.go`:

```go
func TestPlayer_BlockWalkFlag_PerMoveRestrict(t *testing.T) {
	cases := []struct {
		name      string
		restrict  MoveRestrict
		want      int
	}{
		// Fill in from TS Player.blockWalkFlag — DO NOT INFER.
		// Expected (subject to TS-source verification):
		// {"normal", MoveRestrictNormal, collision.FlagBlockPlayers},
		// {"nomove", MoveRestrictNoMove, collision.FlagNull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{moveRestrict: tc.restrict}
			if got := p.blockWalkFlag(); got != tc.want {
				t.Errorf("blockWalkFlag(%v) = %d, want %d", tc.restrict, got, tc.want)
			}
		})
	}
}

func TestPlayer_GetCollisionStrategy_NoMoveReturnsNil(t *testing.T) {
	p := &Player{moveRestrict: MoveRestrictNoMove}
	if got := p.getCollisionStrategy(); got != nil {
		t.Errorf("getCollisionStrategy(NoMove) = %v, want nil", got)
	}
}

func TestPlayer_GetCollisionStrategy_NormalReturnsTypeNormal(t *testing.T) {
	p := &Player{moveRestrict: MoveRestrictNormal}
	got := p.getCollisionStrategy()
	if got == nil {
		t.Fatalf("getCollisionStrategy(Normal) = nil, want *collision.TypeNormal")
	}
	if *got != collision.TypeNormal {
		t.Errorf("getCollisionStrategy(Normal) = %v, want TypeNormal", *got)
	}
}
```

- [ ] **Step 2: Run, expect compile failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayer_(BlockWalkFlag|GetCollisionStrategy)" -v
```

Expected: `undefined: (*Player).blockWalkFlag` / `getCollisionStrategy`.

- [ ] **Step 3: Implement on Player**

In `modules/world/player.go`, append:

```go
// blockWalkFlag returns the CollisionFlag this player imposes on its
// occupied tile during pathfinding. Mirrors TS Player.blockWalkFlag
// (Player.ts:706+). Returns collision.FlagNull for MoveRestrictNoMove.
//
// Plan-author note: exact return-value mapping is derived from TS source
// at impl time — see B1 Task 1.6 plan-author preflight.
func (p *Player) blockWalkFlag() int {
	switch p.moveRestrict {
	case MoveRestrictNoMove:
		return collision.FlagNull
	case MoveRestrictNormal, MoveRestrictBlocked, MoveRestrictIndoors, MoveRestrictOutdoors, MoveRestrictPassthru:
		return collision.FlagBlockPlayers
	default:
		return collision.FlagBlockPlayers
	}
}

// getCollisionStrategy returns the collision search type for this player,
// or nil for MoveRestrictNoMove. Mirrors TS Player.getCollisionStrategy.
func (p *Player) getCollisionStrategy() *collision.Type {
	switch p.moveRestrict {
	case MoveRestrictNoMove:
		return nil
	case MoveRestrictBlocked:
		t := collision.TypeBlocked
		return &t
	case MoveRestrictIndoors:
		t := collision.TypeIndoors
		return &t
	case MoveRestrictOutdoors:
		t := collision.TypeOutdoors
		return &t
	default:
		t := collision.TypeNormal
		return &t
	}
}
```

**Implementer-side validation:** before implementing, **verify** each `collision.Type*` constant exists by grepping `pkg/pathfinder/collision/`. If any constant is absent, register `NAI-92-D-COLLISION-TYPE-MAP` deviation and stub with `collision.TypeNormal` for the missing variants, with a doc-comment label per `defensive_gate_doc_comment_label`.

- [ ] **Step 4: Run player tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayer_(BlockWalkFlag|GetCollisionStrategy)" -v
```

Expected: PASS.

- [ ] **Step 5: Mirror tests + impl on Npc**

Append parallel tests to `modules/world/npc_test.go` (`TestNpc_BlockWalkFlag_PerMoveRestrict` etc., per TS Npc.ts:381+ mapping). Implement `(*Npc).blockWalkFlag` and `(*Npc).getCollisionStrategy` in `modules/world/npc.go` using `n.moveRestrict` switch.

NPC variant: TS Npc.blockWalkFlag returns `collision.FlagBlockNPCs` for normal moveRestrict (vs Player's `FlagBlockPlayers`). Verify at impl time.

- [ ] **Step 6: Run + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "(TestPlayer|TestNpc)_(BlockWalkFlag|GetCollisionStrategy)" -v
```

Expected: PASS.

```bash
git add modules/world/player.go modules/world/npc.go modules/world/player_test.go modules/world/npc_test.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B1 — add blockWalkFlag + getCollisionStrategy

Player + Npc methods mirroring TS Player.blockWalkFlag (Player.ts:706+),
Npc.blockWalkFlag (Npc.ts:381+), and PathingEntity.getCollisionStrategy.
Returns collision.FlagNull / nil for MoveRestrictNoMove (signals no walking).

Used by B5 NAIVE branch and no-strategy else branch in pathToTarget.
"
```

---

## Bundle 2 — Player `pathToTarget()` SMART + `*Loc` arm + signature reshape

**Bundle goal:** Reshape `Player.pathToTarget(tx, tz)` → `Player.pathToTarget()` (arg-less). Implement the type-switch entry point and the `*entitypkg.Loc` SMART branch wired to `FindPathToLoc`. Migrate the 6 player-side test fixtures.

**Files touched:**
- Modify: `modules/world/interaction.go` (lines 566-572 `pathToTarget`; line 237 caller; new `pathToTargetSmart` method)
- Modify: `modules/world/interaction_test.go` (~6 sites + new tests)
- Modify: `modules/world/interaction_debug_test.go` (1 site)

### Task 2.1 — Pre-flight test-site enumeration

- [ ] **Step 1: Enumerate ALL `pathToTarget` call sites in tests**

```bash
rg -n "\.pathToTarget\b\|\.pathToTarget\(" modules/world/*_test.go
```

**Expected sites (verify; this is the `enumerate_all_sites` requirement):**
- `modules/world/handlers_game_test.go:281` (in comment, skip)
- `modules/world/npc_player_modes_test.go:274,275,297,348` (mostly comments + 1 call)
- `modules/world/interaction_test.go` — multiple
- `modules/world/interaction_debug_test.go:336` (comment)
- `modules/world/npc_interaction_test.go:587,603` (calls)
- `modules/world/walk_trigger_fallback_test.go` — none (uses `pathToMoveClick`)

For each call site, list:
- file:line
- current call shape: `p.pathToTarget(tx, tz)` vs `n.pathToTarget()`
- target type the test fixture expects (Loc, Npc, Player, Obj, none)

Save this enumeration to a scratch comment in interaction.go or attach to commit body.

- [ ] **Step 2: Sanity-check production callers**

```bash
rg -n "\.pathToTarget\b" modules/world/*.go | grep -v _test
```

Expected:
- `modules/world/interaction.go:237` — `p.pathToTarget(tx, tz)` — UPDATE in this bundle
- `modules/world/interaction.go:568` — `func (p *Player) pathToTarget(tx, tz int)` — REPLACE in this bundle
- `modules/world/npc_interaction.go:227` — `n.pathToTarget()` — UNCHANGED
- `modules/world/npc_interaction.go:378` — `func (n *Npc) pathToTarget()` — DEFER to B6
- `modules/world/npc_player_modes.go:68` — `n.pathToTarget()` — UNCHANGED

### Task 2.2 — Write failing test for SMART/Loc arm

**Files:**
- Modify: `modules/world/interaction_test.go` (append)

This test pins the `FindPathToLoc` call shape: shape, angle, forceapproach threaded correctly. Use a mock Pathfinder via captured-args pattern.

- [ ] **Step 1: Read existing test patterns for mock pathfinder**

```bash
rg -n "Pathfinder\s*=\|gamemap.Pathfinder\|newServerWithGamemap\|locTypeOrNil" modules/world/*_test.go | head -10
```

This is to find an existing fixture pattern. If none, create a minimal one inline.

- [ ] **Step 2: Append failing test**

In `modules/world/interaction_test.go`, append:

```go
// TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle pins NAI-92 B2's
// SMART/Loc dispatch. Fixture: door at (3098, 3107, 0) with shape
// wall_straight (0), angle west (0). Player at (3097, 3107, 0).
// Expectation: pathToTarget() calls FindPathToLoc(level, 3097, 3107,
// 3098, 3107, 1 /*srcSize=Player.Width*/, 1 /*locWidth*/, 1 /*locLength*/,
// 0 /*angle west*/, 0 /*wall_straight*/, 0 /*forceapproach*/).
func TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 3097, 3107, 0)

	// Construct door Loc. wall_straight=0, angle west=0.
	loc := entitypkg.NewLoc(0, 3098, 3107, 1, 1, nil, /*typ=*/ 1234, /*shape=*/ 0, /*angle=*/ 0)
	p.target = loc

	// Register loc type with ForceApproach=0 (default) so locTypeOrNil
	// returns a non-nil cfg with the expected blockAccessFlags.
	srv.locTypes[1234] = &objtype.LocType{ForceApproach: 0}

	p.pathToTarget()

	// Assert FindPathToLoc was called with the threaded args.
	call, ok := srv.pathfinderRecorder.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.angle != 0 || call.shape != 0 {
		t.Errorf("angle/shape: got (%d, %d), want (0, 0)", call.angle, call.shape)
	}
	if call.blockAccessFlags != 0 {
		t.Errorf("blockAccessFlags: got %d, want 0", call.blockAccessFlags)
	}
	if call.destWidth != 1 || call.destLength != 1 {
		t.Errorf("destWH: got (%d, %d), want (1, 1)", call.destWidth, call.destLength)
	}
	if call.srcSize != 1 {
		t.Errorf("srcSize: got %d, want 1", call.srcSize)
	}
}
```

**Plan-author note for fixture helpers:** `newPathToTargetTestServer` and `pathfinderRecorder` are introduced in this task. The server fixture wraps `gamemap.Pathfinder` with a mock that records `FindPathToLoc` calls. Implementer can either:
1. Add a `Pathfinder` interface seam (most TS-faithful), OR
2. Use a global var + factory so tests can inject a recorder.

**Recommended path:** add a minimal `pathfinderForTarget` interface in `modules/world/pathing.go` that the test fixture satisfies. The production path uses `srv.gamemap.Pathfinder` (which already implements). Recorder lives in `interaction_test.go`.

- [ ] **Step 3: Run, expect compile failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle -v
```

Expected: compile failure (`pathToTarget()` mismatch — expects `(int, int)`, fixtures missing).

- [ ] **Step 4: Commit RED state (optional, OR proceed to 2.3)**

Skip the RED commit; proceed to 2.3 implementation.

### Task 2.3 — Implement SMART/Loc arm + signature reshape

- [ ] **Step 1: Replace `pathToTarget` in `modules/world/interaction.go`**

At line 568 (current `func (p *Player) pathToTarget(tx, tz int)`), replace the entire function body with:

```go
// pathToTarget queues waypoints from p.x/p.z to p.target, dispatched by
// target type with shape-aware findPath helpers. Mirrors TS
// PathingEntity.pathToTarget (PathingEntity.ts:457-508).
//
// Single point-of-entry replacing NAI-11's naive (tx, tz int) signature.
// NAI-92 B2 ports the SMART/*Loc arm; B3-B5 fill in the remaining branches.
func (p *Player) pathToTarget() {
	if p.target == nil {
		return
	}

	switch p.moveStrategy {
	case MoveStrategySmart:
		p.pathToTargetSmart()
	case MoveStrategyNaive:
		p.pathToTargetNaive()
	default:
		p.pathToTargetNoStrategy()
	}
}

// pathToTargetSmart dispatches by target type for the SMART strategy.
// NAI-92 B2 implements the *Loc arm; B3 adds *Player/*Npc; B4 adds *Obj.
func (p *Player) pathToTargetSmart() {
	srv := p.client.server
	pf := srv.gamemap.Pathfinder
	tx, tz, _ := p.target.Coords()

	switch t := p.target.(type) {
	case *entitypkg.Loc:
		var fap int
		if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		route := pf.FindPathToLoc(p.level, p.x, p.z, tx, tz, p.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)
		p.queueWaypoints(routeToPacked(route))
	default:
		// NAI-92 B3-B4 fill in *Player/*Npc/*Obj. For now, fall back to the
		// pre-NAI-92 shape-blind FindPathPlain so B2 doesn't regress
		// PathingEntity/Obj targets in flight.
		route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
		p.queueWaypoints(routeToPacked(route))
	}
}

// pathToTargetNaive — NAI-92 B5 fills this in.
func (p *Player) pathToTargetNaive() {
	tx, tz, _ := p.target.Coords()
	p.queueWaypoint(tx, tz)
}

// pathToTargetNoStrategy — NAI-92 B5 fills this in.
func (p *Player) pathToTargetNoStrategy() {
	tx, tz, _ := p.target.Coords()
	p.queueWaypoint(tx, tz)
}
```

- [ ] **Step 2: Update the production caller at `interaction.go:237`**

Replace:

```go
if !p.repathed {
    tx, tz, _ := p.target.Coords()
    p.pathToTarget(tx, tz)
    p.repathed = true
}
```

with:

```go
if !p.repathed {
    p.pathToTarget()
    p.repathed = true
}
```

- [ ] **Step 3: Migrate test fixtures**

For each enumerated call site from Task 2.1 Step 1 that calls `p.pathToTarget(tx, tz)`:

1. Set `p.target` to a fixture-appropriate target before the call.
2. Replace `p.pathToTarget(tx, tz)` with `p.pathToTarget()`.

**For tests that pass coords without a real target (rare):** construct a stub `entity` impl (the same pattern as `nonNpcEntity` in `interaction_trigger_test.go:94`) returning the test coords.

**Site-by-site update list will be enumerated by the implementer at Task 2.1 Step 1**; codify in the commit body.

- [ ] **Step 4: Add the test fixture helpers**

In `modules/world/interaction_test.go` (or a new `pathToTarget_test_helpers.go` if size warrants), add:

```go
// pathfinderRecorder captures FindPath* calls for assertion.
type pathfinderRecorder struct {
	findPathToLocCalls    []findPathToLocCall
	findPathToEntityCalls []findPathToEntityCall
	findNaivePathCalls    []findNaivePathCall
	findPathPlainCalls    []findPathPlainCall
}

type findPathToLocCall struct {
	level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags int
}
// ...similar for the other three.

// lastFindPathToLoc returns the most recent recorded call.
func (r *pathfinderRecorder) lastFindPathToLoc() (findPathToLocCall, bool) {
	if len(r.findPathToLocCalls) == 0 {
		return findPathToLocCall{}, false
	}
	return r.findPathToLocCalls[len(r.findPathToLocCalls)-1], true
}
```

**Plan-author note:** the recorder is invoked through whatever seam the implementer chose at Task 2.2 Step 2. If the chosen seam is "interface", the recorder satisfies the interface. If "global var", the recorder swaps the var.

**Recommended:** add a `Pathfinder pathfinderInterface` field on `Server` (or replace the existing pathfinder reference with an interface-typed view) so tests can inject the recorder. Production sets it from `gamemap.Pathfinder`.

- [ ] **Step 5: Run full repo tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: PASS. (Pre-NAI-92 PathingEntity/Obj targets still work via FindPathPlain fallback in `default:` arm.)

- [ ] **Step 6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go modules/world/interaction_debug_test.go modules/world/pathing.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B2 — Player.pathToTarget SMART+Loc arm, signature reshape

Reshapes Player.pathToTarget from (tx, tz int) to arg-less form per TS
PathingEntity.pathToTarget (PathingEntity.ts:457-508). Adds type-switch
entry point + pathToTargetSmart sub-method dispatching on *entitypkg.Loc
to FindPathToLoc with shape/angle/forceapproach threaded through. *Player
/ *Npc / *Obj branches fall back to FindPathPlain in this bundle (B3-B4
ports them).

Migrates ~6 test fixtures from (tx, tz) to setting p.target + arg-less
call. Drops the explicit Coords() lookup in interaction.go:237 caller.

Test recorder fixture (pathfinderRecorder) added for FindPath* call
shape assertions used through B6.
"
```

### Task 2.4 — Additional SMART/Loc tests

Append to `modules/world/interaction_test.go`:

- [ ] **Step 1: Force-approach threading test**

```go
// TestPlayer_PathToTarget_LocTarget_ForceApproachThreaded pins that
// LocType.ForceApproach is threaded into the FindPathToLoc
// blockAccessFlags argument.
func TestPlayer_PathToTarget_LocTarget_ForceApproachThreaded(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, nil, 1234, 0, 0)
	p.target = loc

	srv.locTypes[1234] = &objtype.LocType{ForceApproach: 7}

	p.pathToTarget()

	call, ok := srv.pathfinderRecorder.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.blockAccessFlags != 7 {
		t.Errorf("blockAccessFlags: got %d, want 7 (LocType.ForceApproach)", call.blockAccessFlags)
	}
}
```

- [ ] **Step 2: Nil-loc-type defensive test**

```go
// TestPlayer_PathToTarget_LocTarget_NilLocTypeUsesZeroForceApproach
// pins the goscape-defensive guard in pathToTargetSmart that handles
// locTypeOrNil returning nil (e.g. test fixtures with no registered type).
// (goscape defensive; TS skips this check)
func TestPlayer_PathToTarget_LocTarget_NilLocTypeUsesZeroForceApproach(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, nil, 9999, 0, 0)
	p.target = loc
	// Note: no registration in srv.locTypes — locTypeOrNil returns nil.

	p.pathToTarget()

	call, ok := srv.pathfinderRecorder.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.blockAccessFlags != 0 {
		t.Errorf("blockAccessFlags: got %d, want 0 (nil locType→zero)", call.blockAccessFlags)
	}
}
```

- [ ] **Step 3: No-target no-op test**

```go
func TestPlayer_PathToTarget_NoTarget_NoOp(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	p.target = nil

	p.pathToTarget()

	if p.waypointIndex >= 0 {
		t.Errorf("expected no waypoints, got waypointIndex=%d", p.waypointIndex)
	}
	if _, ok := srv.pathfinderRecorder.lastFindPathToLoc(); ok {
		t.Errorf("FindPathToLoc unexpectedly called")
	}
}
```

- [ ] **Step 4: Run + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget -v
```

Expected: PASS.

```bash
git add modules/world/interaction_test.go
git commit --no-gpg-sign -m "test(world): NAI-92 B2 — additional SMART/Loc dispatch matrix

Pins ForceApproach threading, nil-locType defensive fallback,
and no-target no-op for Player.pathToTarget.
"
```

---

## Bundle 3 — Player `pathToTarget()` SMART + `*Player`/`*Npc` arm

**Bundle goal:** Replace the B2 `default:` fallback with the proper `*Player` / `*Npc` SMART branches. Wires the NODE_CLIENT_ROUTEFINDER + intersect shortcut.

**Files touched:**
- Modify: `modules/world/interaction.go` (replace `pathToTargetSmart` body)
- Modify: `modules/world/interaction_test.go` (append PathingEntity tests)

### Task 3.1 — Pre-flight HEAD verification

- [ ] **Step 1: Re-grep B2 surface**

```bash
rg -n "func \(p \*Player\) pathToTargetSmart" modules/world/interaction.go
rg -n "cfg.NodeClientRoutefinder\b" modules/world/
```

Expected:
- `pathToTargetSmart` exists (B2).
- `NodeClientRoutefinder` is read at `walk_trigger_fallback.go:37` and other walk-click sites.

### Task 3.2 — Failing test: `*Npc` target, no-intersect, FindPathToEntity

**Files:**
- Modify: `modules/world/interaction_test.go`

- [ ] **Step 1: Append test**

```go
// TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity pins
// the SMART/PathingEntity arm without the intersect shortcut. Fixture:
// Survival Expert NPC at (3104, 3093) + player at (3101, 3105). bbox
// disjoint → FindPathToEntity called with srcSize=1, destWidth=destLength=npc.size.
func TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = false // server-routefinder mode
	p := newPathToTargetTestPlayer(srv, 3101, 3105, 0)

	npc := newPathToTargetTestNpc(srv, 3104, 3093, 0, /*size=*/ 1)
	p.target = npc

	p.pathToTarget()

	call, ok := srv.pathfinderRecorder.lastFindPathToEntity()
	if !ok {
		t.Fatalf("FindPathToEntity not called")
	}
	if call.srcSize != 1 {
		t.Errorf("srcSize: got %d, want 1", call.srcSize)
	}
	if call.destWidth != 1 || call.destLength != 1 {
		t.Errorf("destWH: got (%d, %d), want (1, 1)", call.destWidth, call.destLength)
	}

	// Negative pin: FindNaivePath must NOT have been called.
	if _, ok := srv.pathfinderRecorder.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called (should have used FindPathToEntity)")
	}
}
```

- [ ] **Step 2: Run, expect FAIL (B2's `default:` arm called FindPathPlain instead)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity -v
```

Expected: FAIL (FindPathToEntity not called).

### Task 3.3 — Implement SMART/PathingEntity arm

- [ ] **Step 1: Replace the `default:` arm in `pathToTargetSmart`**

In `modules/world/interaction.go`, replace the type-switch in `pathToTargetSmart`:

```go
func (p *Player) pathToTargetSmart() {
	srv := p.client.server
	pf := srv.gamemap.Pathfinder
	tx, tz, _ := p.target.Coords()

	switch t := p.target.(type) {
	case *entitypkg.Loc:
		var fap int
		if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		route := pf.FindPathToLoc(p.level, p.x, p.z, tx, tz, p.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)
		p.queueWaypoints(routeToPacked(route))

	case pathingEntity:
		// *Player or *Npc. NODE_CLIENT_ROUTEFINDER + bbox-intersect → naive shortcut.
		tw, tl := t.Width(), t.Length()
		if srv.cfg.NodeClientRoutefinder && coordgrid.Intersects(p.x, p.z, p.Width(), p.Length(), tx, tz, tw, tl) {
			route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), tw, tl, 0, collision.TypeNormal)
			p.queueWaypoints(routeToPacked(route))
		} else {
			route := pf.FindPathToEntity(p.level, p.x, p.z, tx, tz, p.Width(), tw, tl)
			p.queueWaypoints(routeToPacked(route))
		}

	default:
		// *Obj branches (B4) and any unhandled subject — fall back to plain.
		route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
		p.queueWaypoints(routeToPacked(route))
	}
}
```

**Important:** Go type-switch on interface — `case *entitypkg.Loc` matches the concrete type FIRST; `case pathingEntity` matches any type satisfying the interface. The order matters: Loc must come BEFORE pathingEntity (Loc does NOT satisfy pathingEntity since it has no Width/Length methods on `*Loc`... actually Loc.Width is a FIELD not a method; check this). If `*Loc` satisfies `pathingEntity`, the `case *entitypkg.Loc` branch must come first.

**Plan-author note:** if `*entitypkg.Loc` satisfies `pathingEntity` (because of embedded Entity providing `Width`/`Length` field-accessors), the `case` ordering is critical. Go's spec: "the first matching case is chosen." Order is correct above (Loc before pathingEntity).

If `*Loc` does NOT satisfy `pathingEntity` (because Width/Length are fields, not methods), the case ordering is irrelevant for correctness but still document for clarity.

- [ ] **Step 2: Run failing test, expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget_NpcTarget -v
```

Expected: PASS.

- [ ] **Step 3: Run full module tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B3 — Player.pathToTarget SMART+PathingEntity arm

Wires *Player/*Npc target SMART branch to FindPathToEntity (shape=-2
sentinel via wrapper). NODE_CLIENT_ROUTEFINDER + bbox-intersect
shortcut routes to FindNaivePath per TS PathingEntity.ts:464-468.
*Loc branch unchanged (B2). *Obj branches still in default fallback
(B4 ports).

Fixes Survival Expert NPC reachability symptom flagged at NAI-91 smoke
(player at 3101,3105 → NPC at 3104,3093 across cabin wall) — pathfinder
now uses correct entity-shape sentinel for path search.
"
```

### Task 3.4 — Additional SMART/PathingEntity matrix

Append:

- [ ] **Step 1: NODE_CLIENT_ROUTEFINDER + intersect → FindNaivePath**

```go
func TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_Intersect_UsesNaivePath(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = true
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	npc := newPathToTargetTestNpc(srv, 100, 100, 0, /*size=*/ 1) // same tile = bbox intersect
	p.target = npc

	p.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindNaivePath(); !ok {
		t.Fatalf("FindNaivePath not called (intersect+NCR should shortcut)")
	}
	if _, ok := srv.pathfinderRecorder.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called (intersect should shortcut)")
	}
}
```

- [ ] **Step 2: NODE_CLIENT_ROUTEFINDER + no-intersect → FindPathToEntity**

```go
func TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_NoIntersect_UsesFindPathToEntity(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = true
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	npc := newPathToTargetTestNpc(srv, 200, 200, 0, /*size=*/ 1) // disjoint bbox
	p.target = npc

	p.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindPathToEntity(); !ok {
		t.Fatalf("FindPathToEntity not called (no intersect should use full search)")
	}
}
```

- [ ] **Step 3: Symmetry — `*Player` target dispatches same as `*Npc`**

```go
func TestPlayer_PathToTarget_PlayerTarget_DispatchesSameAsNpc(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = false
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	other := newPathToTargetTestPlayer(srv, 105, 105, 0)
	p.target = other

	p.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindPathToEntity(); !ok {
		t.Fatalf("FindPathToEntity not called for *Player target")
	}
}
```

- [ ] **Step 4: Run + commit**

```bash
git add modules/world/interaction_test.go
git commit --no-gpg-sign -m "test(world): NAI-92 B3 — additional SMART/PathingEntity matrix

Pins NODE_CLIENT_ROUTEFINDER intersect shortcut, no-intersect fallthrough,
and *Player/*Npc dispatch symmetry.
"
```

---

## Bundle 4 — Player `pathToTarget()` SMART + `*Obj` arm

**Bundle goal:** Replace the `default:` fallback with the explicit `*entitypkg.Obj` arm: same-tile workaround + different-tile FindPathPlain.

### Task 4.1 — Failing test: same-tile workaround

- [ ] **Step 1: Append test**

```go
func TestPlayer_PathToTarget_ObjTarget_SameTile_QueuesSingleWaypoint(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	obj := entitypkg.NewObj(0, 100, 100, 1, 1, nil, /*typ=*/ 1234, /*count=*/ 1)
	p.target = obj

	p.pathToTarget()

	// Same-tile workaround: queueWaypoint(target.x, target.z), no FindPath* call.
	if p.waypointIndex < 0 {
		t.Fatalf("waypoint not queued")
	}
	if _, ok := srv.pathfinderRecorder.lastFindPathPlain(); ok {
		t.Errorf("FindPathPlain unexpectedly called for same-tile Obj")
	}
}

func TestPlayer_PathToTarget_ObjTarget_DifferentTile_UsesFindPathPlain(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	obj := entitypkg.NewObj(0, 105, 105, 1, 1, nil, 1234, 1)
	p.target = obj

	p.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindPathPlain(); !ok {
		t.Fatalf("FindPathPlain not called for different-tile Obj")
	}
}
```

- [ ] **Step 2: Run, expect both currently passing via the default-arm fallback**

The same-tile test will FAIL (default arm calls FindPathPlain even for same-tile, no special workaround). The different-tile test will PASS (default arm already does FindPathPlain).

### Task 4.2 — Implement `*Obj` arm

- [ ] **Step 1: Replace `default:` in `pathToTargetSmart`**

```go
case *entitypkg.Obj:
	if p.x == tx && p.z == tz {
		// TS workaround: findPath returns 0,0 if src==dest. Queue one waypoint.
		// (PathingEntity.ts:472-473)
		p.queueWaypoint(tx, tz)
	} else {
		route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
		p.queueWaypoints(routeToPacked(route))
	}

default:
	// Unhandled subject type. Per `defensive_gate_doc_comment_label`:
	// goscape defensive; TS pathToTarget does not have a fallthrough default.
	// (goscape defensive; TS skips this check)
	route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
	p.queueWaypoints(routeToPacked(route))
```

- [ ] **Step 2: Run + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget_ObjTarget -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all PASS.

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B4 — Player.pathToTarget SMART+Obj arm

Implements *entitypkg.Obj branch: same-tile shortcut queues a single
waypoint (TS workaround at PathingEntity.ts:472-473 documents that
findPath returns 0,0 when src==dest); different-tile uses FindPathPlain.
Closes the SMART branch dispatch — Loc/PathingEntity/Obj all routed.
default: arm retained as goscape-defensive fallback.
"
```

---

## Bundle 5 — Player NAIVE branch + no-strategy else branch

**Bundle goal:** Implement `pathToTargetNaive` (NAIVE strategy: PathingEntity → FindNaivePath, others → single waypoint) and `pathToTargetNoStrategy` (everything → single waypoint) with `getCollisionStrategy` / `blockWalkFlag` early-returns.

### Task 5.1 — Failing tests for NAIVE branch

Append:

```go
func TestPlayer_PathToTarget_NaiveStrategy_PathingEntityTarget_UsesFindNaivePath(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	npc := newPathToTargetTestNpc(srv, 105, 105, 0, 1)
	p.target = npc

	p.pathToTarget()

	call, ok := srv.pathfinderRecorder.lastFindNaivePath()
	if !ok {
		t.Fatalf("FindNaivePath not called")
	}
	// extraFlag should equal p.blockWalkFlag() per TS PathingEntity.pathToTarget NAIVE arm.
	if call.extraFlag != p.blockWalkFlag() {
		t.Errorf("extraFlag: got %d, want %d (blockWalkFlag)", call.extraFlag, p.blockWalkFlag())
	}
}

func TestPlayer_PathToTarget_NaiveStrategy_LocTarget_QueuesSingleWaypoint(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, nil, 1234, 0, 0)
	p.target = loc

	p.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called for Loc target in NAIVE")
	}
	if p.waypointIndex < 0 {
		t.Errorf("expected single waypoint queued, got waypointIndex=%d", p.waypointIndex)
	}
}

func TestPlayer_PathToTarget_NaiveStrategy_NoMove_NoOp(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	p.moveRestrict = MoveRestrictNoMove
	p.target = newPathToTargetTestNpc(srv, 105, 105, 0, 1)

	p.pathToTarget()

	if p.waypointIndex >= 0 {
		t.Errorf("expected no waypoints (NoMove), got waypointIndex=%d", p.waypointIndex)
	}
}
```

- [ ] **Step 1: Run, expect FAIL** (current `pathToTargetNaive` is the B2 stub that always queues a single waypoint; FindNaivePath path missing).

### Task 5.2 — Implement NAIVE branch

- [ ] **Step 1: Replace `pathToTargetNaive` in `interaction.go`**

```go
// pathToTargetNaive — NAIVE strategy. PathingEntity targets use
// FindNaivePath with the entity's blockWalkFlag; everything else queues
// a single waypoint at the target tile. Mirrors TS PathingEntity.pathToTarget
// NAIVE arm at PathingEntity.ts:477-493.
func (p *Player) pathToTargetNaive() {
	cs := p.getCollisionStrategy()
	if cs == nil {
		// nomove: no walking allowed.
		return
	}
	extraFlag := p.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return
	}

	tx, tz, _ := p.target.Coords()
	if t, ok := p.target.(pathingEntity); ok {
		pf := p.client.server.gamemap.Pathfinder
		route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), t.Width(), t.Length(), extraFlag, *cs)
		p.queueWaypoints(routeToPacked(route))
	} else {
		p.queueWaypoint(tx, tz)
	}
}
```

- [ ] **Step 2: Run NAIVE tests, expect green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget_NaiveStrategy -v
```

Expected: PASS.

### Task 5.3 — Implement no-strategy else branch + tests

- [ ] **Step 1: Replace `pathToTargetNoStrategy`**

```go
// pathToTargetNoStrategy is TS PathingEntity.pathToTarget's third else
// branch (PathingEntity.ts:494-507): runs the same nomove + blockwalk
// guards as NAIVE but always queues a single waypoint.
func (p *Player) pathToTargetNoStrategy() {
	if p.getCollisionStrategy() == nil {
		return
	}
	if p.blockWalkFlag() == collision.FlagNull {
		return
	}
	tx, tz, _ := p.target.Coords()
	p.queueWaypoint(tx, tz)
}
```

- [ ] **Step 2: Test — only runs when MoveStrategy is neither Smart nor Naive**

```go
// TestPlayer_PathToTarget_NoStrategyBranch_QueuesSingleWaypoint pins the
// third else-branch in PathingEntity.pathToTarget. NAI-11's MoveStrategy
// enum has only Smart + Naive, so this branch is engaged via an
// out-of-range strategy value (defensive future-proofing).
func TestPlayer_PathToTarget_NoStrategyBranch_QueuesSingleWaypoint(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(srv, 100, 100, 0)
	p.moveStrategy = MoveStrategy(99) // out of enum range → default branch
	p.target = newPathToTargetTestNpc(srv, 105, 105, 0, 1)

	p.pathToTarget()

	if p.waypointIndex < 0 {
		t.Errorf("expected single waypoint, got waypointIndex=%d", p.waypointIndex)
	}
}
```

- [ ] **Step 3: Run + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayer_PathToTarget -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all PASS.

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B5 — Player.pathToTarget NAIVE + no-strategy branches

Implements pathToTargetNaive: PathingEntity targets → FindNaivePath
with blockWalkFlag/collisionStrategy threaded; non-PathingEntity → single
waypoint. pathToTargetNoStrategy: TS PathingEntity.ts:494-507 third else
branch — nomove guards + single waypoint, no pathfinder.

Closes Player.pathToTarget per TS PathingEntity.pathToTarget
(PathingEntity.ts:457-508). All four moveStrategy paths covered.
"
```

---

## Bundle 6 — Npc `pathToTarget` override + base dispatch

**Bundle goal:** Replace `(*Npc).pathToTarget` body (currently naive single waypoint) with TS Npc.ts:319-335 override (intersect-shortcut + base delegation). Add `pathToTargetBase`, `pathToTargetSmart`, `pathToTargetNaive`, `pathToTargetNoStrategy` on Npc mirroring Player's.

**Files touched:**
- Modify: `modules/world/npc_interaction.go` (replace `pathToTarget`; add `pathToTargetBase` + four sub-methods)
- Modify: `modules/world/npc_interaction_test.go`

### Task 6.1 — Pre-flight + test-site enumeration

- [ ] **Step 1: Re-grep**

```bash
rg -n "func \(n \*Npc\) pathToTarget\|n\.pathToTarget\b" modules/world/
```

Expected:
- `npc_interaction.go:227` — caller `n.pathToTarget()`
- `npc_interaction.go:378` — definition (naive single waypoint)
- `npc_player_modes.go:68` — caller in PLAYERFOLLOW mode
- Test sites: enumerate from `_test.go` greps

### Task 6.2 — Failing test: PathingEntity intersect shortcut

```go
// TestNpc_PathToTarget_PlayerTarget_Intersect_UsesFindNaivePath pins TS
// Npc.pathToTarget override (Npc.ts:319-335): when target is a
// PathingEntity AND bbox intersects, shortcut to FindNaivePath. Note the
// shortcut is UNCONDITIONAL (no NodeClientRoutefinder gate, unlike
// PathingEntity.pathToTarget base).
func TestNpc_PathToTarget_PlayerTarget_Intersect_UsesFindNaivePath(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = false // confirm gate is unconditional
	n := newPathToTargetTestNpc(srv, 100, 100, 0, /*size=*/ 1)
	target := newPathToTargetTestPlayer(srv, 100, 100, 0) // same tile = intersect
	n.target = target

	n.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindNaivePath(); !ok {
		t.Fatalf("FindNaivePath not called (intersect shortcut should fire)")
	}
}

func TestNpc_PathToTarget_PlayerTarget_NoIntersect_DelegatesToBase(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	n := newPathToTargetTestNpc(srv, 100, 100, 0, 1)
	target := newPathToTargetTestPlayer(srv, 200, 200, 0) // disjoint
	n.target = target

	n.pathToTarget()

	// Delegates to base SMART → FindPathToEntity.
	if _, ok := srv.pathfinderRecorder.lastFindPathToEntity(); !ok {
		t.Fatalf("FindPathToEntity not called (base SMART arm)")
	}
	if _, ok := srv.pathfinderRecorder.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called for no-intersect")
	}
}

func TestNpc_PathToTarget_LocTarget_NotPathingEntity_DelegatesToBase(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	n := newPathToTargetTestNpc(srv, 100, 100, 0, 1)
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, nil, 1234, 0, 0)
	n.target = loc

	n.pathToTarget()

	if _, ok := srv.pathfinderRecorder.lastFindPathToLoc(); !ok {
		t.Fatalf("FindPathToLoc not called (base SMART/Loc arm)")
	}
}

func TestNpc_PathToTarget_NoTarget_NoOp(t *testing.T) {
	srv := newPathToTargetTestServer(t)
	n := newPathToTargetTestNpc(srv, 100, 100, 0, 1)
	n.target = nil

	n.pathToTarget()

	if n.waypointIndex >= 0 {
		t.Errorf("expected no waypoints, got waypointIndex=%d", n.waypointIndex)
	}
}
```

- [ ] **Step 1: Run, expect FAIL** (current Npc.pathToTarget is single-waypoint stub; no intersect logic, no FindPath* calls).

### Task 6.3 — Implement Npc override + base + sub-methods

- [ ] **Step 1: Replace `(*Npc).pathToTarget`**

In `modules/world/npc_interaction.go`, replace lines ~371-384:

```go
// pathToTarget mirrors TS Npc.pathToTarget (Npc.ts:319-335). Override of
// PathingEntity.pathToTarget that short-circuits PathingEntity targets
// to FindNaivePath when bbox-intersect (UNCONDITIONAL — no
// NodeClientRoutefinder gate, unlike Player-side). Otherwise delegates
// to pathToTargetBase which mirrors PathingEntity.pathToTarget.
func (n *Npc) pathToTarget() {
	if n.target == nil {
		return
	}

	if t, ok := n.target.(pathingEntity); ok {
		tx, tz, _ := t.Coords()
		tw, tl := t.Width(), t.Length()
		if coordgrid.Intersects(n.x, n.z, n.Width(), n.Length(), tx, tz, tw, tl) {
			pf := n.server.gamemap.Pathfinder
			route := pf.FindNaivePath(n.level, n.x, n.z, tx, tz, n.Width(), n.Length(), tw, tl, 0, collision.TypeNormal)
			n.queueWaypoints(routeToPacked(route))
			return
		}
	}

	n.pathToTargetBase()
}

// pathToTargetBase mirrors TS PathingEntity.pathToTarget (PathingEntity.ts:457-508).
// Identical structure to Player.pathToTarget but operates on Npc state.
func (n *Npc) pathToTargetBase() {
	switch n.moveStrategy {
	case MoveStrategySmart:
		n.pathToTargetSmart()
	case MoveStrategyNaive:
		n.pathToTargetNaive()
	default:
		n.pathToTargetNoStrategy()
	}
}

// pathToTargetSmart — Npc-side analogue of Player.pathToTargetSmart.
// See modules/world/interaction.go for the cross-reference;
// dispatch logic is duplicated rather than factored because of
// asymmetric server-access (Player: client.server, Npc: server).
func (n *Npc) pathToTargetSmart() {
	srv := n.server
	pf := srv.gamemap.Pathfinder
	tx, tz, _ := n.target.Coords()

	switch t := n.target.(type) {
	case *entitypkg.Loc:
		var fap int
		if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		route := pf.FindPathToLoc(n.level, n.x, n.z, tx, tz, n.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)
		n.queueWaypoints(routeToPacked(route))

	case pathingEntity:
		// Note: PathingEntity intersect shortcut is handled in pathToTarget
		// (the override entry point), not here — TS structure mirrored.
		// This branch handles the no-intersect case.
		// NODE_CLIENT_ROUTEFINDER intersect-shortcut from Player.pathToTargetSmart
		// is NOT mirrored here because Npc.pathToTarget already short-circuits
		// intersect cases UNCONDITIONALLY. The no-intersect case falls through
		// to FindPathToEntity unconditionally on the NPC side.
		tw, tl := t.Width(), t.Length()
		route := pf.FindPathToEntity(n.level, n.x, n.z, tx, tz, n.Width(), tw, tl)
		n.queueWaypoints(routeToPacked(route))

	case *entitypkg.Obj:
		if n.x == tx && n.z == tz {
			n.queueWaypoint(tx, tz)
		} else {
			route := pf.FindPathPlain(n.level, n.x, n.z, tx, tz)
			n.queueWaypoints(routeToPacked(route))
		}

	default:
		// (goscape defensive; TS skips this check)
		route := pf.FindPathPlain(n.level, n.x, n.z, tx, tz)
		n.queueWaypoints(routeToPacked(route))
	}
}

// pathToTargetNaive — Npc-side analogue of Player.pathToTargetNaive.
// Cross-reference: modules/world/interaction.go pathToTargetNaive.
func (n *Npc) pathToTargetNaive() {
	cs := n.getCollisionStrategy()
	if cs == nil {
		return
	}
	extraFlag := n.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return
	}

	tx, tz, _ := n.target.Coords()
	if t, ok := n.target.(pathingEntity); ok {
		pf := n.server.gamemap.Pathfinder
		route := pf.FindNaivePath(n.level, n.x, n.z, tx, tz, n.Width(), n.Length(), t.Width(), t.Length(), extraFlag, *cs)
		n.queueWaypoints(routeToPacked(route))
	} else {
		n.queueWaypoint(tx, tz)
	}
}

// pathToTargetNoStrategy — Npc-side analogue of Player.pathToTargetNoStrategy.
func (n *Npc) pathToTargetNoStrategy() {
	if n.getCollisionStrategy() == nil {
		return
	}
	if n.blockWalkFlag() == collision.FlagNull {
		return
	}
	tx, tz, _ := n.target.Coords()
	n.queueWaypoint(tx, tz)
}
```

**Plan-author note:** the test fixture `newPathToTargetTestNpc` must implement `n.queueWaypoints` and `n.queueWaypoint` analogously to Player's. If those methods don't exist on Npc yet, add them in this task.

```bash
rg -n "func \(n \*Npc\) queueWaypoint\b\|func \(n \*Npc\) QueueWaypoint\b" modules/world/
```

Expected: at least one `QueueWaypoint` (capital — exported). If only `QueueWaypoint` exists, use that name; otherwise add `queueWaypoint` / `queueWaypoints` paralleling Player.

- [ ] **Step 2: Run + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpc_PathToTarget -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all PASS.

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go modules/world/npc_player_modes_test.go
git commit --no-gpg-sign -m "feat(world): NAI-92 B6 — Npc.pathToTarget override + base dispatch

Mirrors TS Npc.pathToTarget (Npc.ts:319-335): non-PathingEntity → base
delegation; PathingEntity + intersect → FindNaivePath shortcut (UNCONDITIONAL,
no NodeClientRoutefinder gate, unlike Player-side); PathingEntity + no
intersect → base SMART → FindPathToEntity.

Adds pathToTargetBase + pathToTargetSmart/Naive/NoStrategy NPC-side
methods mirroring Player. Dispatch logic is duplicated rather than
factored because of asymmetric server-access (Player: client.server,
Npc: server). Cross-reference comments at each pair.

Closes NAI-11 SMART pathfinding deferral on the NPC side. NPC pathing
is now shape-aware for all target types.
"
```

---

## Bundle 7 — Cleanup + smoke handoff

**Bundle goal:** Retire legacy comments. Confirm `FindPathDefault` is gone from production. Hand off to user-launched smoke.

### Task 7.1 — Doc-comment cleanup

- [ ] **Step 1: Grep for stale references**

```bash
rg -n "SMART branch deferred\|SMART pathfinding branch in pathToTarget\|naive-only\|Naive-only port" modules/world/
rg -n "FindPathDefault\b" pkg/ modules/
rg -n "pathToTarget\(.*int\b" modules/
```

Expected:
- `SMART branch deferred` — should appear in npc_interaction.go:376 (B6 should have replaced this); verify clean.
- `FindPathDefault` — zero hits in pkg/ + modules/.
- `pathToTarget\(.*int\b` — zero hits (no leftover `(tx, tz)` calls).

- [ ] **Step 2: Update interaction.go pathToTarget docstring**

Confirm the docstring at the new `pathToTarget()` definition matches:

```go
// pathToTarget queues waypoints from p.x/p.z to p.target via shape-aware
// findPath helpers. Mirrors TS PathingEntity.pathToTarget (PathingEntity.ts:457-508).
//
// Type-switches on p.target to select the appropriate FindPath* wrapper:
//   - *entitypkg.Loc:        FindPathToLoc with shape/angle/forceapproach
//   - *Player / *Npc:        FindPathToEntity (shape=-2 entity sentinel),
//                            FindNaivePath shortcut on NODE_CLIENT_ROUTEFINDER+intersect
//   - *entitypkg.Obj same:   queueWaypoint (TS workaround)
//   - *entitypkg.Obj diff:   FindPathPlain (TS findPath)
//
// NAIVE strategy: PathingEntity → FindNaivePath, others → single waypoint.
// No-strategy else: nomove guards + single waypoint.
//
// History: NAI-11 deferred the SMART branch with a stub queueing a single
// waypoint at target.Coords(). NAI-92 closes the deferral.
```

- [ ] **Step 3: Same for npc_interaction.go**

Replace the docstring at `(*Npc).pathToTarget` with one referencing TS Npc.ts:319-335 + the cross-reference to PathingEntity.

- [ ] **Step 4: Commit**

```bash
git add modules/world/interaction.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "docs(world): NAI-92 B7 — pathToTarget docstring cleanup

Replaces NAI-11 deferral framing with NAI-92's full SMART port history.
References TS PathingEntity.pathToTarget (PathingEntity.ts:457-508) and
TS Npc.pathToTarget (Npc.ts:319-335) verbatim line numbers.
"
```

### Task 7.2 — Final regression sweep + close commit

- [ ] **Step 1: Run full repo tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS, vet clean.

- [ ] **Step 2: Final invariants grep**

```bash
rg "FindPathDefault" pkg/ modules/
rg "SMART branch deferred" modules/
rg "SMART pathfinding branch in pathToTarget" pkg/ modules/
rg "pathToTarget\(.*int\b" modules/
rg "// Naive-only port" modules/
```

All expected zero in production code.

- [ ] **Step 3: Hand off to user-launched smoke**

Per `smoke_test_server_handoff` memory: smoke is user-launched. Output the smoke checklist:

```
SMOKE: NAI-92 — full SMART pathfinding port

Server: please run goscape with the standard config.
Client: launch Java client, connect to 127.0.0.1:43594, log in.

Test 1: Survival Expert NPC reachability (PRIMARY NAI-92 binding)
  - Walk to outside Tutorial Island cabin (~3101, 3105, 0).
  - Click Survival Expert NPC (typeId=943, inside cabin).
  - Expected: player paths through cabin door, reaches NPC, OP arm fires.
  - Pre-NAI-92 behavior: player did not move (steps_taken=0).

Test 2: Door re-click regression (NAI-91 invariant)
  - Walk to RuneScape Guide cabin door at ~(3098, 3107, 0).
  - First click: player walks to door tile, door opens.
  - Second click from door tile: player walks west to (3097, 3107).
  - Expected: both clicks succeed without "I can't reach that!" error.

Test 3: Multi-tile Loc approach
  - Find any 2x1 Loc (bank booth in Lumbridge, market stall, etc.).
  - Click from arbitrary direction.
  - Expected: player approaches from correct angle/shape per loc.angle.

Test 4: NPC follow / OPNPC interaction
  - Click Hans (Lumbridge Castle) for "Talk-to" interaction.
  - Expected: player paths to NPC, dialog opens.

Test 5: Obj pickup
  - Drop a ground item; walk away; click to pick up.
  - Expected: player paths to obj tile, obj is picked up.

After all 5 pass: confirm to controller for close commit.
On any FAIL: capture frame A/T logs, screenshots, route to NAI-93.
```

- [ ] **Step 4: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "chore(close): NAI-92 — full SMART pathfinding port [smoke green]

7-bundle port complete:
  B1: wrapper API (FindPathPlain/FindPathToEntity/FindPathToLoc/
      FindNaivePath), coordgrid.Intersects, Width/Length/blockWalkFlag/
      getCollisionStrategy on Player+Npc, pathingEntity interface.
  B2: Player.pathToTarget signature reshape (tx,tz)→() + SMART/Loc arm.
  B3: SMART/PathingEntity arm (Player+Npc targets, NCR intersect shortcut).
  B4: SMART/Obj arm (same-tile workaround + diff-tile FindPathPlain).
  B5: NAIVE branch + no-strategy else branch.
  B6: Npc.pathToTarget override (Npc.ts:319-335) + pathToTargetBase
      + four NPC-side sub-methods.
  B7: cleanup + smoke handoff.

Smoke (user-launched 2026-MM-DD):
  - Survival Expert reachable through cabin door [PRIMARY] ✓
  - Door re-click (NAI-91 regression) ✓
  - Multi-tile Loc approach ✓
  - NPC follow / OPNPC ✓
  - Obj pickup ✓

Closes:
  - NAI-11 SMART pathfinding deferral (docs/superpowers/specs/2026-04-22-
    nai-11-npc-movement-interaction-design.md §690-691).

Untouched (residual deviations):
  - NAI-91-D-OPERABLE-CHEB-FALLBACK (reachedEntity / reachedObj ports).
  - S6l-D4 (LOS-in-approach).

Closes memory: nai_followups (NAI-91 \"untouched: Survival Expert\" entry).
"
```

---

## Self-Review

After authoring:

**Spec coverage:**
- §1 Goal → B1-B7 cover the goal end-to-end. ✓
- §2 Static binding → preserved as commit narrative in B3, B6, B7 close. ✓
- §3 Cadence → 7-bundle decomposition matches §5 exactly. ✓
- §4.1 Wrapper API → Task 1.3. ✓
- §4.2 coordgrid.Intersects → Task 1.2. ✓
- §4.3 blockWalkFlag/getCollisionStrategy → Task 1.6. ✓
- §4.4 Player pathToTarget reshape → Tasks 2.3, 3.3, 4.2, 5.2, 5.3. ✓
- §4.5 Npc pathToTarget reshape → Task 6.3. ✓
- §4.6 Caller updates → Task 2.3 Step 2 (interaction.go:237) + Task 2.3 Step 3 (test fixtures). ✓
- §5 Bundle decomposition → matches plan. ✓
- §6 Tests → distributed across bundle TDD steps. ✓
- §7 Smoke → Task 7.2 Step 3. ✓
- §8 Deviations — NAI-91-D-OPERABLE-CHEB-FALLBACK referenced in B7 close commit; NAI-92-D-COLLISION-TYPE-MAP gated as conditional in Task 1.6. ✓
- §9 Risk register — R1/R5 addressed in Task 1.6 / 1.5; R2 mitigation comments in B6 Task 6.3; R3 in B2 Task 2.1; R4 in B1 Task 1.3 (NaiveRouteFinder.FindRoute pass-through pinned). ✓

**Placeholder scan:** none — every "code step" has a concrete code block. Plan-author preflight notes are bounded with explicit grep commands and "do not infer" directives.

**Type consistency:**
- `FindPathPlain` / `FindPathToEntity` / `FindPathToLoc` / `FindNaivePath` — consistent across all bundles. ✓
- `pathToTargetSmart` / `pathToTargetNaive` / `pathToTargetNoStrategy` — consistent. ✓
- `pathingEntity` interface — defined Task 1.5; consumed Tasks 3.3, 5.2, 6.3. ✓
- `routeToPacked` — pre-existing helper at `modules/world/movement.go:181`. ✓
- `entitypkg.Loc` / `entitypkg.Obj` — verified import alias at `interaction.go:5`. ✓
- `collision.Type*` — risk-flagged in Task 1.6 with conditional deviation. ✓

**Test fixture runnability** (per `plan_runnable_test_fixtures`):
- `newPathToTargetTestServer`, `newPathToTargetTestPlayer`, `newPathToTargetTestNpc`, `pathfinderRecorder` — introduced in B2 Task 2.3 Step 4. Used uniformly through B6.
- Mental compile check: `entitypkg.NewLoc(level, x, z, w, l, lc Lifecycle, typ, shape, angle int) *Loc` — call shape from `pkg/entity/loc.go:23`. ✓
- `entitypkg.NewObj` signature must be verified in B4 Task 4.1 plan-author preflight (sig might differ from `NewLoc`).

If any issue surfaced during execution, the controller's `controller_preflight` and `verify_implementer_claims` per-bundle re-runs catch it.
