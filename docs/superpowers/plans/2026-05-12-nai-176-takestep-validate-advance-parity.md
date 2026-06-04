# NAI-176 — takeStep + validateAndAdvanceStep parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the three NAI-175-deferred deviation arms (D2 waypoint retention, D3 size>1, D4 Player.stepOnce parity) plus stretch `MoveStrategyFly` enum to full TS `PathingEntity.takeStep` + `validateAndAdvanceStep` parity.

**Architecture:** Refactor `stepOnce` on both `*Npc` and `*Player` from `(advanced bool, dir int)` to `(dir int, status stepStatus)` (tri-state: `stepMoved` / `stepDone` / `stepBlocked`). Introduce `validateAndAdvanceStep` wrappers on both types that own `waypointIndex` bookkeeping + recursive try-next-waypoint cascade. Position-update stays inside `stepOnce.applyStep` (pragmatic factoring — TS applies position in the wrapper, but goscape's existing `applyStep` factoring is preserved). Four sequential bundles; B4 (FLY enum) is a stretch with no content callsite yet.

**Tech Stack:** Go 1.26+ (per `go_version.md`). No new deps. Modules touched: `modules/world/`.

**Spec:** `docs/superpowers/specs/2026-05-12-nai-176-takestep-validate-advance-parity-design.md` (committed at `7aa1b5d`).

**TS canonical:** `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:202-232` (validateAndAdvanceStep), `617-683` (takeStep). Per `ts_source_canonical_path.md`.

---

## Pre-flight verified (already done; controller pre-flight)

- **R3 (width=2 fixture):** `NewNpc(..., typ)` with `typ.Size = 2` propagates to `n.size` at `npc.go:173`. Existing fixture sites: `npc_test.go:765`, `npc_interaction_test.go:1581`. No new setter needed.
- **R4 (lastStepX/Z consumers):** `interaction_test.go:1029-1077`, `interaction_canaccess_gate_test.go:171`, `player_script.go:565`, `npc_script.go:185`. `(*Player).stepOnce` writes `lastStepX = p.x` / `lastStepZ = p.z` BEFORE position mutation (`movement.go:150-151`); pre-step capture semantics preserved by keeping the writes in their current order inside the moved arm.
- **stepOnce call sites enumerated:** Production: `movement.go:91` (player walk), `movement.go:103` (player run), `npc_interaction.go:308` (npc walk), `npc_interaction.go:317` (npc run). Tests: `movement_test.go:181`, `:225`; `npc_interaction_test.go:1682`, `:2172`, `:2203`, `:2233`. All convert in lock-step inside B1 (NPC) and B3 (Player).
- **CollisionFlag availability:** `collision.FlagBlockPlayers`, `collision.FlagBlockNPCs`, `collision.FlagBlockWalk` all imported via `modules/world/collision/...`. No new imports needed.

---

## File map

### B1 — D2 (NPC stepStatus + validateAndAdvanceStep + waypoint retention)

- **Create** type `stepStatus` in `modules/world/movement_consts.go` (append after existing const blocks at line 32).
- **Modify** `modules/world/npc_interaction.go`:
  - `(*Npc).stepOnce` signature → `(int, stepStatus)`; rewrite body per §3 table; remove `n.waypointIndex = -1` clear at line 397.
  - `(*Npc).applyStep` signature → `(int, stepStatus)` returning `stepMoved`; keep dest-check decrement of `waypointIndex` inline.
  - Add `(*Npc).validateAndAdvanceStep(s *Server) (int, bool)` immediately after `applyStep`.
  - `(*Npc).updateMovement` (line 308 + 317) — switch both calls to `validateAndAdvanceStep`.
  - Retire `NAI-175-D-WAYPOINT-RETENTION` doc-comment block at `npc_interaction.go:353-358` (replace with updated docstring; the SIZE-GT-1 tag stays until B2).
- **Modify** `modules/world/npc_interaction_test.go` — convert existing tests at lines 1682, 2172, 2203, 2233 to new signature; add D2 pin tests + integration test.

### B2 — D3 (size>1 pre-branch)

- **Modify** `modules/world/npc_interaction.go` — add `n.size > 1` pre-branch inside `(*Npc).stepOnce` between strategy/extraFlag guards and the existing `dir := coordgrid.Face(...)` body.
- **Modify** `modules/world/npc_interaction.go` — retire `NAI-175-D-SIZE-GT-1` doc-comment at lines 353-358.
- **Add** D3 pin tests to `modules/world/npc_interaction_test.go`.

### B3 — D4 (Player parity)

- **Modify** `modules/world/movement.go`:
  - `(*Player).stepOnce` signature → `(int, stepStatus)`; rewrite body to mirror Npc.stepOnce (strategy/extraFlag plumbing + D1 axis-fallback). Preserve inline `lastStepX/Z` + `refreshPlayerZone` bookkeeping in the moved arm.
  - Factor a small `(*Player).applyStep(dest coordgrid.Position, dx, dz, dir int) (int, stepStatus)` helper to dedupe the three moved arms (direct, X-only, Z-only).
  - Add `(*Player).validateAndAdvanceStep() (int, bool)` immediately after `stepOnce`.
  - Caller at `movement.go:91` + `:103` — switch to `validateAndAdvanceStep`.
  - Retire `NAI-175-D-PLAYER-STEP-COLLISION` doc-comment block at lines 122-129.
- **Modify** `modules/world/movement_test.go` — convert existing tests at lines 181, 225 to new signature; add D4 pin tests.

### B4 (stretch) — MoveStrategyFly

- **Modify** `modules/world/movement_consts.go` — add `MoveStrategyFly` constant after `MoveStrategyNaive` at line 31.
- **Modify** `modules/world/npc_interaction.go` — add `MoveStrategyFly` early-return inside `(*Npc).stepOnce` between zero-delta guard and direct-travel canTravel check (mirrors TS L663-665).
- **Modify** `modules/world/movement.go` — same early-return inside `(*Player).stepOnce`.
- **Carve** `NAI-176-D-FLY-NO-CONTENT-WIRES` doc-comment tag above the new early-return in both files.

---

## Bundle B1 — D2 wrapper + signature flip (NPC)

Cascade: NAI-175-D-WAYPOINT-RETENTION retired. ~80 LOC production + ~60 LOC tests.

### Task B1.T1 — Add `stepStatus` type

**Files:**
- Modify: `modules/world/movement_consts.go` (append after existing const blocks, line 32+)

- [ ] **Step 1: Add the type**

```go
// stepStatus is the tri-state return classification from (*Npc).stepOnce
// and (*Player).stepOnce. Mirrors TS PathingEntity.takeStep's
// (number | null) return where the wrapper (validateAndAdvanceStep)
// dispatches on the value:
//
//   stepBlocked = TS null   → transient block; waypointIndex preserved (NAI-176 D2)
//   stepDone    = TS -1     → waypoint reached or no-move; wrapper decrements + recurses
//   stepMoved   = TS number → position applied inline; wrapper returns dir
//
// Mirrors PathingEntity.ts:617-683 (takeStep) + 202-232 (validateAndAdvanceStep).
type stepStatus int

const (
	stepMoved stepStatus = iota
	stepDone
	stepBlocked
)
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add modules/world/movement_consts.go
git commit --no-gpg-sign -m "refactor(world): NAI-176 T1 — add stepStatus tri-state for takeStep parity"
```

### Task B1.T2 — Write failing D2 pin test

**Files:**
- Test: `modules/world/npc_interaction_test.go` (append to end of file)

- [ ] **Step 1: Add the test**

```go
// TestNpcStepOnce_TransientBlock_PreservesWaypointIndex pins NAI-176 D2.
// TS PathingEntity.takeStep:682 returns null when all canTravel arms
// fail — wrapper (validateAndAdvanceStep) returns -1 WITHOUT decrementing
// waypointIndex. Goscape's pre-NAI-176 stepOnce cleared waypointIndex to -1
// in this branch (npc_interaction.go:397), losing the queued destination.
//
// Setup: MoveRestrictNormal NPC at (3221, 3220) heading north to (3221, 3221).
// Block the north tile with FlagBlockWalk so all canTravel arms fail.
// The X-only fallback (dx=0) and Z-only fallback (dx=0,dz=1 = same as direct)
// also fail — falls through to stepBlocked.
func TestNpcStepOnce_TransientBlock_PreservesWaypointIndex(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Pathfinder.Flags.Add(3221, 3221, 0, collision.FlagBlockWalk)

	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "blocked"},
		WanderRange:  5,
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	n.QueueWaypoint(3221, 3221) // sets waypointIndex = 0

	wantWaypointIndex := n.waypointIndex
	dir, status := n.stepOnce(s)

	if status != stepBlocked {
		t.Fatalf("blocked stepOnce: got status=%v dir=%d, want stepBlocked", status, dir)
	}
	if n.waypointIndex != wantWaypointIndex {
		t.Fatalf("waypointIndex after stepBlocked: got %d, want %d (D2: must NOT clear)",
			n.waypointIndex, wantWaypointIndex)
	}
	if n.x != 3221 || n.z != 3220 {
		t.Fatalf("position after stepBlocked: got (%d,%d), want (3221,3220) unchanged",
			n.x, n.z)
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcStepOnce_TransientBlock_PreservesWaypointIndex -v`
Expected: compile error — `(*Npc).stepOnce` currently returns `(bool, int)`, not `(int, stepStatus)`. This is the RED that drives B1.T3.

### Task B1.T3 — Refactor stepOnce + applyStep to tri-state; introduce wrapper

**Files:**
- Modify: `modules/world/npc_interaction.go:339-415`

- [ ] **Step 1: Rewrite stepOnce**

Replace `(*Npc).stepOnce` body (lines 339-399) with:

```go
// stepOnce walks one tile toward the current waypoint and returns
// (dir, status). Mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683)
// single-tile (width=1) arm. Position update + dest-check decrement of
// waypointIndex happen inline via applyStep; transient-block / done /
// no-move classifications go to the validateAndAdvanceStep wrapper for
// waypointIndex bookkeeping.
//
// Tri-state contract (mirrors TS takeStep return number | null):
//   stepBlocked = TS null   → all canTravel arms failed; wrapper preserves waypointIndex (NAI-176 D2)
//   stepDone    = TS -1     → strategy null / extraFlag null / Face==-1; wrapper decrements
//   stepMoved   = TS number → moved; position applied via applyStep
//
// NAI-175 status: D0 strategy plumbing (T4) + D1 axis-fallback (T6) +
// D2 wrapper waypoint retention (NAI-176 B1) shipped.
//
// NAI-175-D-SIZE-GT-1: TS takeStep PathingEntity.ts:642-651 has a
// separate width>1 arm that uses Face(srcX, 0, x, 0) / Face(0, srcZ, 0, z)
// for axis-only checks. goscape currently uses the same single-tile
// logic for all sizes. No size>1 NPC observed broken in NAI-175 smoke;
// deferred to NAI-176 B2.
func (n *Npc) stepOnce(s *Server) (int, stepStatus) {
	if n.waypointIndex < 0 {
		return -1, stepBlocked
	}
	cs := n.getCollisionStrategy()
	if cs == nil {
		return -1, stepDone
	}
	extraFlag := n.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return -1, stepDone
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		// TS L659-661: dx==0 && dz==0 → -1 (waypoint reached on current tile).
		return -1, stepDone
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s == nil || s.gamemap == nil {
		// Test-fixture path: no gamemap → skip collision and apply step.
		return n.applyStep(s, dest, dx, dz, int(dir))
	}
	// NAI-175 D1: TS takeStep PathingEntity.ts:668-682 — direct, then X-only,
	// then Z-only fallback before giving up.
	if s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, n.Width(), extraFlag, *cs) {
		return n.applyStep(s, dest, dx, dz, int(dir))
	}
	if dx != 0 && s.gamemap.CanTravel(n.level, n.x, n.z, dx, 0, n.Width(), extraFlag, *cs) {
		axisDir := coordgrid.Face(n.x, n.z, dest.X, n.z)
		return n.applyStep(s, dest, dx, 0, int(axisDir))
	}
	if dz != 0 && s.gamemap.CanTravel(n.level, n.x, n.z, 0, dz, n.Width(), extraFlag, *cs) {
		axisDir := coordgrid.Face(n.x, n.z, n.x, dest.Z)
		return n.applyStep(s, dest, 0, dz, int(axisDir))
	}
	// NAI-176 D2: TS L682 returns null here (transient block); wrapper
	// preserves waypointIndex.
	return -1, stepBlocked
}
```

- [ ] **Step 2: Rewrite applyStep**

Replace `(*Npc).applyStep` (lines 405-415) with:

```go
// applyStep advances the NPC one tile by (dx, dz), refreshes its zone,
// and decrements waypointIndex if the destination is reached. Factored
// from stepOnce so axis-fallback arms share the same post-step bookkeeping.
// Returns (dir, stepMoved) — applyStep is only invoked from the moved arms.
func (n *Npc) applyStep(s *Server, dest coordgrid.Position, dx, dz, dir int) (int, stepStatus) {
	prevX, prevZ := n.x, n.z
	n.x += dx
	n.z += dz
	n.stepsTaken++
	refreshNpcZone(s, n, prevX, prevZ, n.level)
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return dir, stepMoved
}
```

- [ ] **Step 3: Add validateAndAdvanceStep wrapper**

Insert immediately after `applyStep`:

```go
// validateAndAdvanceStep wraps stepOnce with the TS waypointIndex
// bookkeeping + recursive try-next-waypoint cascade. Mirrors TS
// PathingEntity.validateAndAdvanceStep (PathingEntity.ts:202-232).
// Returns (dir, true) when a step landed, (-1, false) when blocked /
// done / no-move.
func (n *Npc) validateAndAdvanceStep(s *Server) (int, bool) {
	dir, status := n.stepOnce(s)
	switch status {
	case stepBlocked:
		// NAI-176 D2: waypointIndex preserved; entity stays put this tick.
		return -1, false
	case stepDone:
		n.waypointIndex--
		if n.waypointIndex >= 0 {
			return n.validateAndAdvanceStep(s)
		}
		return -1, false
	case stepMoved:
		// Position already applied inside stepOnce.applyStep.
		return dir, true
	}
	return -1, false
}
```

- [ ] **Step 4: Update existing tests at npc_interaction_test.go:1682, 2172, 2203, 2233**

Each of these calls `n.stepOnce(s)` and reads `(advanced bool, dir int)`. Convert to `(dir int, status stepStatus)`.

For example, `:1682`:
```go
// BEFORE: ok, _ := n.stepOnce(s)
//         if !ok { t.Fatal("stepOnce returned false") }

// AFTER:
_, status := n.stepOnce(s)
if status != stepMoved {
	t.Fatalf("stepOnce returned status=%v, want stepMoved", status)
}
```

`:2172` (TestNpcStepOnce_BlockedNpcStepsOntoWaterTile):
```go
// BEFORE:
// advanced, dir := n.stepOnce(s)
// if !advanced { t.Fatalf("...; got advanced=%v, dir=%d", advanced, dir) }

// AFTER:
dir, status := n.stepOnce(s)
if status != stepMoved {
	t.Fatalf("blocked NPC failed to step onto adjacent water tile (status=%v, dir=%d); want stepMoved", status, dir)
}
```

`:2203` (TestNpcStepOnce_AxisFallback_X):
```go
// BEFORE: advanced, dir := n.stepOnce(s); if !advanced { ... }

// AFTER:
dir, status := n.stepOnce(s)
if status != stepMoved {
	t.Fatalf("axis-fallback X: got status=%v, want stepMoved", status)
}
```

`:2233` (TestNpcStepOnce_AxisFallback_Z): same shape as :2203, dir == North check unchanged.

- [ ] **Step 5: Run all NPC tests to verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcStepOnce -v`
Expected: all `TestNpcStepOnce_*` green, including the new `TransientBlock_PreservesWaypointIndex` from B1.T2.

### Task B1.T4 — Switch updateMovement to wrapper

**Files:**
- Modify: `modules/world/npc_interaction.go:308, 317`

- [ ] **Step 1: Switch caller**

Replace lines 308-325 of `updateMovement`:

```go
// BEFORE:
// advanced1, dir1 := n.stepOnce(s)
// if !advanced1 {
//     n.walkDir = -1
//     n.runDir = -1
//     return false
// }
// n.walkDir = dir1
//
// if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0 {
//     advanced2, dir2 := n.stepOnce(s)
//     if advanced2 {
//         n.runDir = dir2
//     } else {
//         n.runDir = -1
//     }
// } else {
//     n.runDir = -1
// }

// AFTER:
dir1, advanced1 := n.validateAndAdvanceStep(s)
if !advanced1 {
	n.walkDir = -1
	n.runDir = -1
	return false
}
n.walkDir = dir1

if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0 {
	dir2, advanced2 := n.validateAndAdvanceStep(s)
	if advanced2 {
		n.runDir = dir2
	} else {
		n.runDir = -1
	}
} else {
	n.runDir = -1
}
```

- [ ] **Step 2: Run all modules/world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: green.

### Task B1.T5 — Add D2 wrapper-recursion integration test

**Files:**
- Test: `modules/world/npc_interaction_test.go` (append after the D2 pin from B1.T2)

- [ ] **Step 1: Add the test**

```go
// TestNpcValidateAndAdvanceStep_DoneCascade_TriesNextWaypoint pins NAI-176
// D2 wrapper recursion. TS validateAndAdvanceStep (PathingEntity.ts:209-211)
// recurses into itself when stepDone (TS -1) but waypointIndex still ≥ 0
// after decrement — the next waypoint becomes the new target. Goscape pre-
// NAI-176 had no wrapper and could not advance through a "skip-this-waypoint"
// signal.
//
// Setup: NPC at (3221, 3220) with TWO queued waypoints. queueWaypoints
// stores reversed (first_step at index n-1). To get stepDone on the first
// pop, we queue the NPC's CURRENT tile as waypoint[1] (= index 1 = first
// step). Then waypoint[0] is one tile east. Wrapper:
//   1. takeStep: Face(3221,3220, 3221,3220) == -1 → stepDone
//   2. waypointIndex--; recurse
//   3. takeStep: Face(3221,3220, 3222,3220) == East → stepMoved
//   4. return (East, true)
func TestNpcValidateAndAdvanceStep_DoneCascade_TriesNextWaypoint(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "twohop"},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s

	// queueWaypoints (npc_ai.go:101) reverses input on copy and sets
	// waypointIndex = len(packed)-1. stepOnce reads waypoints[waypointIndex],
	// which is packed[0]. To pop NPC's current tile first (→ Face==-1 →
	// stepDone), put it at packed[0]. packed[1] is the next waypoint
	// (one tile east).
	packed := []int{
		coordgrid.PackCoord(0, 3221, 3220), // popped first → Face==-1 → stepDone
		coordgrid.PackCoord(0, 3222, 3220), // popped second → step east
	}
	n.queueWaypoints(packed)
	if n.waypointIndex != 1 {
		t.Fatalf("setup: waypointIndex after queueWaypoints: got %d, want 1", n.waypointIndex)
	}

	dir, advanced := n.validateAndAdvanceStep(s)

	if !advanced {
		t.Fatalf("wrapper recursion: got advanced=false, want true (should recurse through stepDone)")
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("wrapper recursion: dir=%d, want East (%d)", dir, coordgrid.DirectionEast)
	}
	if n.x != 3222 || n.z != 3220 {
		t.Fatalf("wrapper recursion: stepped to (%d,%d), want (3222,3220)", n.x, n.z)
	}
}
```

> **Plan-author verified:** `(*Npc).queueWaypoints` at `npc_ai.go:101-112`. Iteration `for input := len(packed)-1; input >= 0; input--, output++` copies `waypoints[output] = packed[input]`, then `waypointIndex = len(packed)-1`. So `packed[0]` lands at `waypoints[len(packed)-1]` and is the *first* popped by stepOnce. Test setup above places the current-tile coord at `packed[0]` so the wrapper sees stepDone on the first invocation.

- [ ] **Step 2: Run the new test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcValidateAndAdvanceStep_DoneCascade -v`
Expected: PASS.

### Task B1.T6 — Add updateMovement run-recursion integration test

**Files:**
- Test: `modules/world/npc_interaction_test.go` (append after B1.T5)

- [ ] **Step 1: Add the test**

```go
// TestNpcUpdateMovement_RunSpeed_RecursesThroughDoneWaypoint pins NAI-176
// cross-arm: running NPC with two queued waypoints where the first is at
// the NPC's tile (Face==-1 → stepDone). updateMovement's walk-arm wrapper
// recurses through the done signal, takes one step. Run-arm wrapper then
// runs again from the new position with waypointIndex now at the next
// waypoint. Both walkDir and runDir should populate when both succeed.
func TestNpcUpdateMovement_RunSpeed_RecursesThroughDoneWaypoint(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "runner"},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedRun

	// Input [current-tile, one-east, two-east]; queueWaypoints reverses so
	// first-popped == input[0] (current tile, stepDone), second == east,
	// third == 2 east.
	packed := []int{
		coordgrid.PackCoord(0, 3221, 3220),
		coordgrid.PackCoord(0, 3222, 3220),
		coordgrid.PackCoord(0, 3223, 3220),
	}
	n.queueWaypoints(packed)

	moved := n.updateMovement(s)

	if !moved {
		t.Fatalf("updateMovement: got moved=false, want true")
	}
	if n.walkDir != int(coordgrid.DirectionEast) {
		t.Fatalf("walkDir: got %d, want East (%d)", n.walkDir, coordgrid.DirectionEast)
	}
	if n.runDir != int(coordgrid.DirectionEast) {
		t.Fatalf("runDir: got %d, want East (%d) — run-arm should also step", n.runDir, coordgrid.DirectionEast)
	}
	if n.x != 3223 || n.z != 3220 {
		t.Fatalf("position after walk+run: got (%d,%d), want (3223,3220)", n.x, n.z)
	}
}
```

- [ ] **Step 2: Run the new test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcUpdateMovement_RunSpeed_RecursesThroughDoneWaypoint -v`
Expected: PASS.

### Task B1.T7 — Retire NAI-175-D-WAYPOINT-RETENTION tag + full-suite green

**Files:**
- Modify: `modules/world/npc_interaction.go` (doc-comment block lines 349-358 — D2 + D3 deferred notes)

- [ ] **Step 1: Verify deviation tags retired**

Per `retire_deviation_grep_all_comments.md`: enumerate all references first.

Run: `rg "NAI-175-D-WAYPOINT-RETENTION" pkg/ modules/ cmd/`
Expected: zero production hits (the doc-comment block from B1.T3 already removed the WAYPOINT-RETENTION tag; SIZE-GT-1 still present and retires in B2). Doc hits in `docs/superpowers/specs/2026-05-12-nai-175-...` + `docs/superpowers/plans/2026-05-12-nai-175-...` are historical and stay.

- [ ] **Step 2: Run full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: green.

- [ ] **Step 3: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-176 B1 — (*Npc).validateAndAdvanceStep wrapper + D2 retention

stepOnce signature flips to (dir, stepStatus); waypointIndex bookkeeping +
try-next-waypoint recursion move into the new validateAndAdvanceStep wrapper.
Retires NAI-175-D-WAYPOINT-RETENTION. Mirrors TS PathingEntity.takeStep +
validateAndAdvanceStep (PathingEntity.ts:617-683 + 202-232).
EOF
)"
```

---

## Bundle B2 — D3 size>1 pre-branch

Cascade: NAI-175-D-SIZE-GT-1 retired. ~25 LOC production + ~50 LOC tests.

### Task B2.T1 — Write failing D3 X-axis pin

**Files:**
- Test: `modules/world/npc_interaction_test.go` (append)

- [ ] **Step 1: Add the test**

```go
// TestNpcStepOnce_WidthGt1_PrefersXAxis pins NAI-176 D3. TS takeStep at
// PathingEntity.ts:642-651 splits on this.width > 1: tries Face(srcX, 0, x, 0)
// (X-only) first, then Face(0, srcZ, 0, z) (Z-only). Width=2 NPC at (3220,3220)
// targeting (3222, 3222). X-only step (East, 2 wide) is allowed; Z-only is
// blocked. Expect East step + stepMoved.
func TestNpcStepOnce_WidthGt1_PrefersXAxis(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	// Width=2 NPC occupies (3220,3220)+(3221,3220)+(3220,3221)+(3221,3221).
	// Block the Z-axis target row (3220,3222)+(3221,3222) with FlagBlockWalk.
	s.gamemap.Pathfinder.Flags.Add(3220, 3222, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3221, 3222, 0, collision.FlagBlockWalk)

	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "wide"},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         2,
	}
	n := NewNpc(1, 1, 3220, 3220, 0, typ)
	n.server = s
	n.QueueWaypoint(3222, 3222)

	dir, status := n.stepOnce(s)

	if status != stepMoved {
		t.Fatalf("width>1 X-axis: got status=%v, want stepMoved", status)
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("width>1 X-axis: dir=%d, want East (%d)", dir, coordgrid.DirectionEast)
	}
	if n.x != 3221 || n.z != 3220 {
		t.Fatalf("width>1 X-axis: stepped to (%d,%d), want (3221,3220)", n.x, n.z)
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcStepOnce_WidthGt1_PrefersXAxis -v`
Expected: FAIL — current stepOnce uses width=2 in the existing canTravel call, but the *axis-selection* is `Face(n.x,n.z, dest.X,dest.Z)` which returns NE-diagonal, not East-only. The diagonal canTravel may pass through narrowly or fail; either way the dir won't be East unless the new width>1 branch is in place.

### Task B2.T2 — Implement D3 width>1 pre-branch

**Files:**
- Modify: `modules/world/npc_interaction.go` (inside `(*Npc).stepOnce`, between extraFlag guard and the existing `dir := coordgrid.Face(...)` line)

- [ ] **Step 1: Insert the pre-branch**

Inside `(*Npc).stepOnce`, after the extraFlag guard and before `dest := coordgrid.UnpackCoord(...)`, add:

```go
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])

	// NAI-176 D3: TS PathingEntity.takeStep:642-651 — width>1 NPCs use
	// axis-only Face checks (no diagonal step). X-axis tried first;
	// returns stepBlocked if both axes fail.
	if n.size > 1 {
		if s == nil || s.gamemap == nil {
			// Test-fixture path with no gamemap — fall through to single-tile
			// arm (Face on full delta). Width>1 only matters when canTravel
			// actually runs.
		} else {
			tryDirX := coordgrid.Face(n.x, 0, dest.X, 0)
			if tryDirX != -1 && s.gamemap.CanTravel(n.level, n.x, n.z, coordgrid.DeltaX(tryDirX), 0, n.Width(), extraFlag, *cs) {
				return n.applyStep(s, dest, coordgrid.DeltaX(tryDirX), 0, int(tryDirX))
			}
			tryDirZ := coordgrid.Face(0, n.z, 0, dest.Z)
			if tryDirZ != -1 && s.gamemap.CanTravel(n.level, n.x, n.z, 0, coordgrid.DeltaZ(tryDirZ), n.Width(), extraFlag, *cs) {
				return n.applyStep(s, dest, 0, coordgrid.DeltaZ(tryDirZ), int(tryDirZ))
			}
			// NAI-176 D2 + D3: both axes failed → stepBlocked (TS L651 null).
			return -1, stepBlocked
		}
	}

	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
```

(Remove the now-duplicate `dest := ...` line that was below the original extraFlag guard.)

- [ ] **Step 2: Run X-axis test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcStepOnce_WidthGt1_PrefersXAxis -v`
Expected: PASS.

### Task B2.T3 — Add Z-axis fallback + both-blocked pins

**Files:**
- Test: `modules/world/npc_interaction_test.go` (append after B2.T1's test)

- [ ] **Step 1: Add the tests**

```go
// TestNpcStepOnce_WidthGt1_FallsThroughToZ pins TS L647-649: when X-only
// canTravel fails, try Z-only.
func TestNpcStepOnce_WidthGt1_FallsThroughToZ(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	// Width=2 NPC at (3220,3220)→(3222,3222). Block the X-axis target column
	// (3222,3220)+(3222,3221) so X-only step fails; leave (3220,3222)+(3221,3222)
	// open so Z-only step lands.
	s.gamemap.Pathfinder.Flags.Add(3222, 3220, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3222, 3221, 0, collision.FlagBlockWalk)

	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "wide"},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         2,
	}
	n := NewNpc(1, 1, 3220, 3220, 0, typ)
	n.server = s
	n.QueueWaypoint(3222, 3222)

	dir, status := n.stepOnce(s)

	if status != stepMoved {
		t.Fatalf("width>1 Z-axis: got status=%v, want stepMoved", status)
	}
	if dir != int(coordgrid.DirectionNorth) {
		t.Fatalf("width>1 Z-axis: dir=%d, want North (%d)", dir, coordgrid.DirectionNorth)
	}
	if n.x != 3220 || n.z != 3221 {
		t.Fatalf("width>1 Z-axis: stepped to (%d,%d), want (3220,3221)", n.x, n.z)
	}
}

// TestNpcStepOnce_WidthGt1_BothBlocked pins TS L651 null when both axes fail.
func TestNpcStepOnce_WidthGt1_BothBlocked(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	// Width=2 NPC at (3220,3220)→(3222,3222). Block both X-only and Z-only
	// target footprints.
	s.gamemap.Pathfinder.Flags.Add(3222, 3220, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3222, 3221, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3220, 3222, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3221, 3222, 0, collision.FlagBlockWalk)

	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "wide"},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         2,
	}
	n := NewNpc(1, 1, 3220, 3220, 0, typ)
	n.server = s
	n.QueueWaypoint(3222, 3222)

	wantWaypointIndex := n.waypointIndex
	_, status := n.stepOnce(s)

	if status != stepBlocked {
		t.Fatalf("width>1 both-blocked: got status=%v, want stepBlocked", status)
	}
	if n.waypointIndex != wantWaypointIndex {
		t.Fatalf("width>1 both-blocked waypointIndex: got %d, want %d (D2: preserved)",
			n.waypointIndex, wantWaypointIndex)
	}
}
```

- [ ] **Step 2: Run the new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcStepOnce_WidthGt1 -v`
Expected: both PASS.

### Task B2.T4 — Retire NAI-175-D-SIZE-GT-1 tag + commit

**Files:**
- Modify: `modules/world/npc_interaction.go` (doc-comment block above `(*Npc).stepOnce`)

- [ ] **Step 1: Enumerate references**

Run: `rg "NAI-175-D-SIZE-GT-1" pkg/ modules/ cmd/ docs/`
Expected: production hit at `npc_interaction.go` only; doc-only hits in `docs/superpowers/specs/2026-05-12-nai-175-...` and `docs/superpowers/plans/2026-05-12-nai-175-...`. Production retire only; doc hits stay as history.

- [ ] **Step 2: Update the doc-comment above `(*Npc).stepOnce`**

Remove the `NAI-175-D-SIZE-GT-1` paragraph from the doc-comment (block currently retained from B1.T7's update). The remaining doc-comment should read:

```go
// stepOnce walks one tile toward the current waypoint and returns
// (dir, status). Mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683).
// Position update + dest-check decrement of waypointIndex happen inline via
// applyStep; transient-block / done / no-move classifications go to the
// validateAndAdvanceStep wrapper for waypointIndex bookkeeping.
//
// NAI-175 status: D0 strategy plumbing (T4) + D1 axis-fallback (T6) +
// D2 wrapper waypoint retention (NAI-176 B1) + D3 width>1 axis split
// (NAI-176 B2) all shipped.
```

- [ ] **Step 3: Run full repo tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-176 B2 — (*Npc).stepOnce width>1 axis-only arm (D3)

Adds TS PathingEntity.takeStep:642-651 width>1 pre-branch (X-only then
Z-only Face checks; no diagonal step for multi-tile NPCs). Retires
NAI-175-D-SIZE-GT-1.
EOF
)"
```

---

## Bundle B3 — D4 Player parity

Cascade: NAI-175-D-PLAYER-STEP-COLLISION retired. ~70 LOC production + ~80 LOC tests.

### Task B3.T1 — Write failing D4 plumbing pin

**Files:**
- Test: `modules/world/movement_test.go` (append to end of file)

- [ ] **Step 1: Add the test**

```go
// TestPlayerStepOnce_PlumbsBlockWalkFlag pins NAI-176 D4. TS Player.blockWalkFlag
// (Player.ts:706-708) is unconditional FlagBlockPlayers. Goscape pre-NAI-176
// passed extraFlag=0 to gamemap.CanTravel (movement.go:144), so a tile carrying
// only FlagBlockPlayers (e.g., one occupied by another player or a BlockWalkAll
// NPC) was traversable by the moving player. Post-fix: the same tile should
// block the step (status = stepBlocked).
func TestPlayerStepOnce_PlumbsBlockWalkFlag(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Place FlagBlockPlayers on the east tile. Pre-NAI-176 step plumbs
	// extraFlag=0 (passes); post-NAI-176 plumbs FlagBlockPlayers (blocks).
	s.gamemap.Pathfinder.Flags.Add(3201, 3200, 0, collision.FlagBlockPlayers)
	p.queueWaypoint(3201, 3200)

	wantWaypointIndex := p.waypointIndex
	dir, status := p.stepOnce()

	if status != stepBlocked {
		t.Fatalf("player step over FlagBlockPlayers tile: got status=%v dir=%d, want stepBlocked", status, dir)
	}
	if p.waypointIndex != wantWaypointIndex {
		t.Fatalf("waypointIndex after stepBlocked: got %d, want %d (D2: must NOT clear)",
			p.waypointIndex, wantWaypointIndex)
	}
	if p.x != 3200 || p.z != 3200 {
		t.Fatalf("position after blocked step: got (%d,%d), want (3200,3200) unchanged", p.x, p.z)
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPlayerStepOnce_PlumbsBlockWalkFlag -v`
Expected: compile error — `(*Player).stepOnce` currently returns `(coordgrid.Direction, bool)`, not `(int, stepStatus)`.

### Task B3.T2 — Refactor Player.stepOnce to tri-state + plumbing + axis-fallback

**Files:**
- Modify: `modules/world/movement.go:120-164`

- [ ] **Step 1: Replace stepOnce body**

Replace `(*Player).stepOnce` (lines 120-164) with:

```go
// stepOnce walks one tile toward the current waypoint and returns
// (dir, status). Mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683)
// for width=1 entities (Player.Width() ≡ 1). Position update + dest-check
// decrement of waypointIndex happen inline via applyStep; transient-block /
// done / no-move classifications go to validateAndAdvanceStep for
// waypointIndex bookkeeping.
//
// NAI-176 B3: plumbs p.blockWalkFlag() (= FlagBlockPlayers) and
// p.getCollisionStrategy() per-step + D1 axis-fallback (X-only / Z-only).
// Retires NAI-175-D-PLAYER-STEP-COLLISION.
func (p *Player) stepOnce() (int, stepStatus) {
	if p.waypointIndex < 0 {
		return -1, stepBlocked
	}
	cs := p.getCollisionStrategy()
	if cs == nil {
		return -1, stepDone
	}
	extraFlag := p.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return -1, stepDone
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		return -1, stepDone
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
		// Test-fixture path: no gamemap → skip collision and apply step.
		return p.applyStep(dest, dx, dz, int(dir))
	}
	gm := p.client.server.gamemap
	if gm.CanTravel(p.level, p.x, p.z, dx, dz, 1, extraFlag, *cs) {
		return p.applyStep(dest, dx, dz, int(dir))
	}
	if dx != 0 && gm.CanTravel(p.level, p.x, p.z, dx, 0, 1, extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, dest.X, p.z)
		return p.applyStep(dest, dx, 0, int(axisDir))
	}
	if dz != 0 && gm.CanTravel(p.level, p.x, p.z, 0, dz, 1, extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, p.x, dest.Z)
		return p.applyStep(dest, 0, dz, int(axisDir))
	}
	// NAI-176 D2: TS L682 returns null (transient block); wrapper preserves
	// waypointIndex.
	return -1, stepBlocked
}

// applyStep advances the player one tile by (dx, dz), refreshes zone
// presence, and decrements waypointIndex if the destination is reached.
// lastStepX/Z capture pre-step position (consumed by interaction.go and
// player_script.go follower paths — see NAI-174). Returns (dir, stepMoved).
func (p *Player) applyStep(dest coordgrid.Position, dx, dz, dir int) (int, stepStatus) {
	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++
	refreshPlayerZone(p, p.lastStepX, p.lastStepZ, p.level)
	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, stepMoved
}

// validateAndAdvanceStep wraps stepOnce with waypointIndex bookkeeping
// + recursive try-next-waypoint cascade. Mirrors TS PathingEntity.
// validateAndAdvanceStep (PathingEntity.ts:202-232).
func (p *Player) validateAndAdvanceStep() (int, bool) {
	dir, status := p.stepOnce()
	switch status {
	case stepBlocked:
		return -1, false
	case stepDone:
		p.waypointIndex--
		if p.waypointIndex >= 0 {
			return p.validateAndAdvanceStep()
		}
		return -1, false
	case stepMoved:
		return dir, true
	}
	return -1, false
}
```

- [ ] **Step 2: Switch updateMovement caller**

Replace lines 91-107 of `(*Player).resolveMovement`:

```go
// BEFORE:
// dir, ok := p.stepOnce()
// if !ok {
//     p.walkDir = -1
//     p.runDir = -1
//     p.tempRun = 0
//     return
// }
// p.walkDir = int(dir)
// p.runDir = -1
//
// if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
//     dir2, ok2 := p.stepOnce()
//     if ok2 {
//         p.runDir = int(dir2)
//     }
// }

// AFTER:
dir, ok := p.validateAndAdvanceStep()
if !ok {
	p.walkDir = -1
	p.runDir = -1
	// NAI-135: step blocked → no steps → tempRun reset (TS Player.ts:670-673).
	p.tempRun = 0
	return
}
p.walkDir = dir
p.runDir = -1

if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
	dir2, ok2 := p.validateAndAdvanceStep()
	if ok2 {
		p.runDir = dir2
	}
}
```

- [ ] **Step 3: Update existing tests at movement_test.go:181, 225**

`:181` (TestPlayerStepRefreshesZoneAcrossZoneBoundary):
```go
// BEFORE: dir, ok := p.stepOnce(); if !ok { t.Fatalf("...dir=%d", dir) }

// AFTER:
dir, status := p.stepOnce()
if status != stepMoved {
	t.Fatalf("stepOnce status: got %v (dir=%d), want stepMoved", status, dir)
}
```

`:225` (TestPlayerStepIntraZoneNoSubscriptionChange):
```go
// BEFORE: if _, ok := p.stepOnce(); !ok { t.Fatal("stepOnce ok: got false") }

// AFTER:
if _, status := p.stepOnce(); status != stepMoved {
	t.Fatalf("stepOnce status: got %v, want stepMoved", status)
}
```

- [ ] **Step 4: Run player tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPlayerStep -v`
Expected: green, including the new `TestPlayerStepOnce_PlumbsBlockWalkFlag` from B3.T1.

### Task B3.T3 — Add Player axis-fallback + no-move pins

**Files:**
- Test: `modules/world/movement_test.go` (append after B3.T1)

- [ ] **Step 1: Add the tests**

```go
// TestPlayerStepOnce_AxisFallback_XOnly pins NAI-176 D4 + D1 for Player.
// Direct diagonal blocked, X-only open → step east.
func TestPlayerStepOnce_AxisFallback_XOnly(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Block NE-diagonal (3201, 3201); leave east (3201, 3200) open.
	s.gamemap.Pathfinder.Flags.Add(3201, 3201, 0, collision.FlagBlockWalk)
	p.queueWaypoint(3205, 3205)

	dir, status := p.stepOnce()

	if status != stepMoved {
		t.Fatalf("axis-fallback X: got status=%v, want stepMoved", status)
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("axis-fallback X: dir=%d, want East (%d)", dir, coordgrid.DirectionEast)
	}
	if p.x != 3201 || p.z != 3200 {
		t.Fatalf("axis-fallback X: stepped to (%d,%d), want (3201,3200)", p.x, p.z)
	}
}

// TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked pins
// the wrapper's response to MoveRestrictNoMove: stepDone via cs==nil,
// wrapper decrements then sees waypointIndex<0 and returns (-1, false).
// waypointIndex transitions 0 → -1 (legitimate decrement, not a clear).
func TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	p.moveRestrict = MoveRestrictNoMove
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.queueWaypoint(3201, 3200)

	dir, advanced := p.validateAndAdvanceStep()

	if advanced {
		t.Fatalf("NoMove: got advanced=true (dir=%d), want false", dir)
	}
	if p.x != 3200 || p.z != 3200 {
		t.Fatalf("NoMove: position changed to (%d,%d), want (3200,3200)", p.x, p.z)
	}
}
```

- [ ] **Step 2: Run the new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPlayerStepOnce_AxisFallback|TestPlayerValidateAndAdvanceStep_NoMoveRestrict" -v`
Expected: PASS.

### Task B3.T4 — Retire NAI-175-D-PLAYER-STEP-COLLISION + commit

**Files:**
- Modify: `modules/world/movement.go` (doc-comment block lines 120-129)

- [ ] **Step 1: Enumerate references**

Run: `rg "NAI-175-D-PLAYER-STEP-COLLISION" pkg/ modules/ cmd/ docs/`
Expected: production hit at `movement.go` only; doc hits in NAI-175 spec/plan stay as history.

- [ ] **Step 2: Confirm the new stepOnce doc-comment (from B3.T2) already says "Retires NAI-175-D-PLAYER-STEP-COLLISION."**

No further edit needed if B3.T2's docstring landed verbatim.

- [ ] **Step 3: Run full repo tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-176 B3 — (*Player).stepOnce takeStep parity (D4)

Player.stepOnce now mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683):
plumbs p.blockWalkFlag() (= FlagBlockPlayers) + p.getCollisionStrategy(),
adds D1 axis-fallback (X-only / Z-only). Adds (*Player).validateAndAdvanceStep
wrapper + (*Player).applyStep helper. resolveMovement switches to the wrapper.
Retires NAI-175-D-PLAYER-STEP-COLLISION.
EOF
)"
```

---

## Bundle B4 (stretch) — MoveStrategyFly

Cascade: NAI-176-D-FLY-NO-CONTENT-WIRES carved. ~15 LOC; no tests (no content wires it).

### Task B4.T1 — Add MoveStrategyFly constant

**Files:**
- Modify: `modules/world/movement_consts.go:29-32`

- [ ] **Step 1: Extend the const block**

```go
const (
	MoveStrategySmart MoveStrategy = iota
	MoveStrategyNaive
	MoveStrategyFly
)
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: success.

### Task B4.T2 — Add FLY early-return in (*Npc).stepOnce

**Files:**
- Modify: `modules/world/npc_interaction.go` (inside `(*Npc).stepOnce`, between zero-delta `Face==-1` guard and the existing `dx/dz` computation, OR between the existing `dx/dz` lines and the first `canTravel` call — matches TS L663-665 placement)

- [ ] **Step 1: Insert the early-return**

After `dx := coordgrid.DeltaX(dir); dz := coordgrid.DeltaZ(dir)` and before the `if s == nil || s.gamemap == nil` test-fixture guard:

```go
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)

	// NAI-176-D-FLY-NO-CONTENT-WIRES: TS PathingEntity.takeStep:663-665
	// — MoveStrategyFly bypasses collision entirely. No NpcType or content
	// in goscape's cache currently assigns MoveStrategyFly; the enum +
	// early-return ship for engine-fidelity only. To retire: when first
	// FLY-moveStrategy content (wyvern, dragon) ports + a smoke binds.
	if n.moveStrategy == MoveStrategyFly {
		return n.applyStep(s, dest, dx, dz, int(dir))
	}

	if s == nil || s.gamemap == nil {
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: success.

### Task B4.T3 — Add FLY early-return in (*Player).stepOnce

**Files:**
- Modify: `modules/world/movement.go` (inside `(*Player).stepOnce`, same placement as B4.T2)

- [ ] **Step 1: Insert the early-return**

After `dx := coordgrid.DeltaX(dir); dz := coordgrid.DeltaZ(dir)` and before the `if p.client == nil || ...` guard:

```go
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)

	// NAI-176-D-FLY-NO-CONTENT-WIRES: TS PathingEntity.takeStep:663-665
	// — MoveStrategyFly bypasses collision entirely. No content currently
	// assigns MoveStrategyFly to Player; engine-fidelity only.
	if p.moveStrategy == MoveStrategyFly {
		return p.applyStep(dest, dx, dz, int(dir))
	}

	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
```

- [ ] **Step 2: Run full repo tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: green.

- [ ] **Step 3: Commit**

```bash
git add modules/world/movement_consts.go modules/world/movement.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-176 B4 — MoveStrategyFly enum + takeStep early-return

Ports TS PathingEntity.takeStep:663-665 MoveStrategy.FLY early-return on
both (*Npc).stepOnce and (*Player).stepOnce. No NpcType / content currently
assigns MoveStrategyFly; engine-fidelity only. Carves
NAI-176-D-FLY-NO-CONTENT-WIRES deviation tag.
EOF
)"
```

---

## Close — memory + close commit

After B4 (or B3 if B4 is skipped):

- [ ] **Step 1: Re-grep deviation tags**

Run: `rg "NAI-175-D-(WAYPOINT-RETENTION|SIZE-GT-1|PLAYER-STEP-COLLISION)" pkg/ modules/ cmd/`
Expected: zero production hits. Doc hits in `docs/superpowers/specs/2026-05-12-nai-175-...` + `docs/superpowers/plans/2026-05-12-nai-175-...` are historical and stay.

- [ ] **Step 2: Update memory entry**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai175_step_collision_strategy.md`:

- Change description line to reflect closed status of D2/D3/D4.
- Add a paragraph at end: "Closed by NAI-176 (date YYYY-MM-DD): D2 wrapper waypoint retention (B1), D3 width>1 axis-only arm (B2), D4 Player.stepOnce parity (B3) all ported; D-FLY tag remains in npc_interaction.go + movement.go for the no-content-wires stretch."
- Update the "How to apply" line: "All NAI-175 D-tags retired except NAI-176-D-FLY-NO-CONTENT-WIRES. When porting a flying NPC, retire that one too."

- [ ] **Step 3: Final smoke check (optional)**

If implementer wants a sanity smoke after B3: Lumbridge tick-loop should be unaffected (D2/D3/D4 are all latent). The pre-existing duck-wander smoke from NAI-175 should remain green:
- Start server (user handoff per `smoke_test_server_handoff.md`).
- Walk near Lumbridge water; adult ducks should drift between water tiles within 30s (same NAI-175 acceptance).
- No new symptom expected (R5 acceptance).

Smoke is OPTIONAL — close commit is fine without it given R5 explicitly accepted no-symptom-binding.

- [ ] **Step 4: Close commit**

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai175_step_collision_strategy.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-176 — port NAI-175 D2/D3/D4 deviation arms

B1 (D2 wrapper waypoint retention) + B2 (D3 width>1 axis-only arm) +
B3 (D4 Player.stepOnce parity) + B4 (MoveStrategyFly stretch) all shipped.
Three NAI-175-D-* tags retired. One new tag carved:
NAI-176-D-FLY-NO-CONTENT-WIRES (in npc_interaction.go + movement.go).

Closes memory: nai175_step_collision_strategy.md
EOF
)"
```

---

## Coverage cross-check (plan vs. spec §4)

| Spec §4 test                                                       | Plan task                  |
| ------------------------------------------------------------------ | -------------------------- |
| `TestNpcStepOnce_TransientBlock_PreservesWaypointIndex` (D2)       | B1.T2                      |
| `TestNpcValidateAndAdvanceStep_DoneCascade_TriesNextWaypoint` (D2) | B1.T5                      |
| Update existing `TestNpcStepOnce_*` to tri-state                   | B1.T3 step 4               |
| `TestNpcStepOnce_WidthGt1_PrefersXAxis` (D3)                       | B2.T1                      |
| `TestNpcStepOnce_WidthGt1_FallsThroughToZ` (D3)                    | B2.T3                      |
| `TestNpcStepOnce_WidthGt1_BothBlocked` (D3)                        | B2.T3                      |
| `TestPlayerStepOnce_PlumbsBlockWalkFlag` (D4)                      | B3.T1                      |
| `TestPlayerStepOnce_AxisFallback_XOnly` (D4)                       | B3.T3                      |
| `TestPlayerStepOnce_TransientBlock_PreservesWaypointIndex` (D4)    | B3.T1 (subsumed via waypoint-index assertion in same test) |
| `TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked`   | B3.T3                      |
| `TestNpcUpdateMovement_RunSpeed_RecursesThroughDoneWaypoint`       | B1.T6                      |
| Update existing `TestPlayerStepOnce_*` to tri-state                | B3.T2 step 3               |

All spec §4 tests have a plan task. B3.T1 doubles up the D4-plumbing assertion and the D2-style waypointIndex-preserved assertion in one test (the FlagBlockPlayers tile only blocks under the new extraFlag plumbing; the waypointIndex assertion gates the D2 path simultaneously). Spec lists them as two test names but the plan implements them as one with combined assertions — flag if reviewer wants split.

## Risks recap (from spec §5)

- **R1 — Signature flip ripple.** Atomic in B1 (NPC) and B3 (Player). All in-package callers.
- **R2 — Recursion depth.** Bounded by `[25]int` waypoint capacity. Informational.
- **R3 — Width=2 fixture.** Verified at pre-flight: `NewNpc(..., typ)` with `typ.Size = 2` works.
- **R4 — Player lastStepX/Z.** Verified at pre-flight: write order preserved by keeping the captures inside `(*Player).applyStep` at top, before position mutation.
- **R5 — No smoke binding.** Accepted; TS-fidelity-only sub-spec close.
