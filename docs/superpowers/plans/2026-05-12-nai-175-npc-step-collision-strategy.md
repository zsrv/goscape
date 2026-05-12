# NAI-175 — NPC stepOnce per-NPC collision-strategy port

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `(*Npc).stepOnce` (and conditionally `(*Player).stepOnce`) to TS `PathingEntity.takeStep` parity so `MoveRestrictBlocked` NPCs (Lumbridge river ducks) actually wander.

**Architecture:** Extend `(*GameMap).CanTravel` to accept `(size, extraFlag, collisionType)`; call sites plumb in the NPC/Player's `Width()`, `blockWalkFlag()`, and `getCollisionStrategy()`. Port TS axis-fallback, transient-block waypoint retention, and size>1 branch.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-12-nai-175-npc-step-collision-strategy-design.md`

**Cadence:** investigation sub-spec — Stage 1 audit → Stage 2 TDD fix → smoke handoff → conditional Stage 3 probe.

---

## File map

- **Modify** `pkg/gamemap/gamemap.go:136-140` — extend `(*GameMap).CanTravel` signature.
- **Modify** `pkg/gamemap/nai98_realcache_probe_test.go:98` — preserve TypeNormal semantics by passing explicit `(1, 0, collision.TypeNormal)`.
- **Modify** `modules/world/npc_interaction.go:339-370` — port `(*Npc).stepOnce` to TS `takeStep` parity.
- **Modify** `modules/world/movement.go:120-154` — port `(*Player).stepOnce` (T8; conditional on Stage 1 verdict).
- **Add** tests in `modules/world/npc_interaction_test.go` (existing file) — D0/D1/D2/D3 fixtures.
- **Add** tests in `pkg/gamemap/gamemap_test.go` (existing file) — strategy-parameterised wrapper coverage.
- **Append** Stage 1 verdict directly to this plan doc (Task 1 output).

---

## Task 1: Stage 1 — risk-weighted audit (Explore subagent, no code commits)

**Files:**
- Append verdict to: `docs/superpowers/plans/2026-05-12-nai-175-npc-step-collision-strategy.md` (this file, §"Stage 1 verdict")

- [ ] **Step 1: Dispatch one Explore subagent with this prompt verbatim**

```
You are auditing a TS→Go port. Read these files end-to-end before answering:

- /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts lines 175-232 (validateAndAdvanceStep), 617-683 (takeStep), 558-575 (getCollisionStrategy)
- /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Npc.ts lines 381-398 (blockWalkFlag)
- /home/owner/Code/github.com/zsrv/goscape/modules/world/npc_interaction.go lines 339-370 (Npc.stepOnce), 276-337 (Npc.updateMovement), 81-102 (wanderMode), 158-270 (processMovementInteraction)
- /home/owner/Code/github.com/zsrv/goscape/modules/world/movement.go lines 1-160 (Player.stepOnce + updateMovement)
- /home/owner/Code/github.com/zsrv/goscape/modules/world/npc.go lines 240-290 (blockWalkFlag, getCollisionStrategy)
- /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go lines 598-640 (Width, blockWalkFlag, getCollisionStrategy)
- /home/owner/Code/github.com/zsrv/goscape/pkg/gamemap/gamemap.go lines 130-145 (CanTravel wrapper)
- /home/owner/Code/github.com/zsrv/goscape/pkg/pathfinder/routefinder/stepvalidator.go lines 17-42 (CanTravel impl)
- /home/owner/Code/github.com/zsrv/goscape/pkg/pathfinder/collision/strategies.go (CanMove + Type semantics)
- /home/owner/Code/github.com/zsrv/goscape/pkg/pathfinder/collision/flag.go (Flag* constants)

Then `grep -rn "\.CanTravel(" modules/ pkg/ --include='*.go'` and list every caller.

Then answer with a short report (under 600 words):

1. CONFIRM or REJECT Bundle 0 root cause: (*Npc).stepOnce hardcodes (size=1, extraFlag=0, TypeNormal) through gamemap.CanTravel, and this prevents MoveRestrictBlocked NPCs from stepping onto FlagBlockWalk water tiles. Cite specific lines.

2. ENUMERATE all (*GameMap).CanTravel callers. For each, note whether the current TypeNormal/0/1 hardcoding is correct semantics or a latent bug.

3. Rate each of these divergences vs TS PathingEntity.takeStep as BINDING (duck symptom blocker) vs LATENT (real divergence but doesn't block duck wander):
   - D1: axis-fallback (X-only / Z-only retry when diagonal blocked)
   - D2: blocked-step waypoint retention (TS leaves waypointIndex; goscape sets -1)
   - D3: size>1 branch
   - D4: Player.stepOnce parallel port

4. List any existing test that PINS the current (1, 0, TypeNormal) wrapper semantics for non-Normal NPCs — i.e., a test that would break if we plumb per-NPC strategy. Search test files for `CanTravel\(` and for any test that creates a `MoveRestrictBlocked` NPC with a non-trivial waypoint.

5. VERDICT: which of D1/D2/D3/D4 must ship in NAI-175 Stage 2 to bind the duck symptom? Which can be deferred to NAI-176?

Output in this exact format:

## Stage 1 verdict

### 1. Root cause
[CONFIRM or REJECT, citing lines]

### 2. CanTravel callers
- [path:line] — [current semantics correct? if not, why]

### 3. Divergence ratings
- D1: [BINDING|LATENT] — [one-sentence reason]
- D2: [BINDING|LATENT] — [reason]
- D3: [BINDING|LATENT] — [reason]
- D4: [BINDING|LATENT] — [reason]

### 4. Tests pinning current semantics
[list or "none found"]

### 5. NAI-175 Stage 2 scope
- MUST SHIP: [list]
- DEFER TO NAI-176: [list]
```

- [ ] **Step 2: Append the subagent's verdict to this plan doc**

Open `docs/superpowers/plans/2026-05-12-nai-175-npc-step-collision-strategy.md` and append the verbatim verdict at the bottom under a `## Stage 1 verdict` heading.

- [ ] **Step 3: Reconcile plan-doc with verdict**

Re-read Tasks 2-9. If the verdict downgrades any of D1/D2/D3 to LATENT (deferred to NAI-176), strike the matching tasks below with a note `STAGE-1-DEFERRED: see NAI-176`. If D4 is BINDING, leave Task 8 in scope; if LATENT, replace Task 8 body with the deviation-tag content from §"D4 deferral fallback" below.

- [ ] **Step 4: Commit the verdict**

```bash
git add docs/superpowers/plans/2026-05-12-nai-175-npc-step-collision-strategy.md
git commit --no-gpg-sign -m "docs(plan): NAI-175 Stage 1 — risk-weighted audit verdict

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Refactor `(*GameMap).CanTravel` signature

**Files:**
- Modify: `pkg/gamemap/gamemap.go:136-140`
- Modify: `pkg/gamemap/nai98_realcache_probe_test.go:98`
- Modify: `modules/world/npc_interaction.go:356` (preserve current behaviour temporarily)
- Modify: `modules/world/movement.go:134` (preserve current behaviour temporarily)

This is a pure-refactor task. Behaviour is unchanged; signature is parameterised so subsequent tasks can plumb per-caller strategy.

- [ ] **Step 1: Update `(*GameMap).CanTravel` to take `(size, extraFlag, collisionType)`**

Replace `pkg/gamemap/gamemap.go:134-140` with:

```go
// CanTravel tests whether moving from (x, z, level) with the given offset
// (offsetX, offsetZ) to an adjacent tile is allowed under the given
// per-entity collision strategy. offsetX/offsetZ must be in {-1, 0, 1}.
// size is the entity's tile footprint width (1 for players and 1-tile NPCs).
// extraFlag is the entity's blockWalkFlag() (e.g. FlagBlockPlayers for
// players, FlagBlockNPCs for normal NPCs, FlagOpen for blocked NPCs).
// collisionType is the entity's getCollisionStrategy() (TypeNormal,
// TypeBlocked, TypeIndoors, TypeOutdoors).
func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ, size, extraFlag int, collisionType collision.Type) bool {
	return gm.Pathfinder.StepValidator.CanTravel(
		level, x, z, offsetX, offsetZ, size, extraFlag, collisionType,
	)
}
```

- [ ] **Step 2: Update `(*Npc).stepOnce` caller to preserve current behaviour**

Edit `modules/world/npc_interaction.go:356` — change:

```go
if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
```

to:

```go
if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, 1, 0, collision.TypeNormal) {
```

If the file does not already import `"github.com/zsrv/goscape/pkg/pathfinder/collision"`, add it to the import block.

- [ ] **Step 3: Update `(*Player).stepOnce` caller to preserve current behaviour**

Edit `modules/world/movement.go:134` — change:

```go
if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz) {
```

to:

```go
if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz, 1, 0, collision.TypeNormal) {
```

Verify the `collision` import is present in `movement.go`; if not, add it.

- [ ] **Step 4: Update the probe test caller**

Edit `pkg/gamemap/nai98_realcache_probe_test.go:98` — change:

```go
if !gm.CanTravel(level, x, z, sx, sz) {
```

to:

```go
if !gm.CanTravel(level, x, z, sx, sz, 1, 0, collision.TypeNormal) {
```

Add `"github.com/zsrv/goscape/pkg/pathfinder/collision"` to the imports of `nai98_realcache_probe_test.go` if not already present.

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all green. This is a refactor; behaviour is unchanged.

- [ ] **Step 6: Commit**

```bash
git add pkg/gamemap/gamemap.go pkg/gamemap/nai98_realcache_probe_test.go modules/world/npc_interaction.go modules/world/movement.go
git commit --no-gpg-sign -m "refactor(gamemap): NAI-175 T2 — extend CanTravel signature with (size, extraFlag, collisionType)

Pure refactor — all 3 call sites preserve current (1, 0, TypeNormal)
semantics. Subsequent NAI-175 tasks plumb per-NPC strategy.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: RED — pin blocked-NPC duck-symptom test

**Files:**
- Modify: `modules/world/npc_interaction_test.go` (existing file)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcStepOnce_BlockedNpcStepsOntoWaterTile pins NAI-175 root cause.
// A MoveRestrictBlocked NPC (duck) on a FlagBlockWalk tile must be able
// to step onto an adjacent FlagBlockWalk tile under TypeBlocked collision.
// Mirrors TS PathingEntity.takeStep at PathingEntity.ts:617-683 with
// getCollisionStrategy()==TypeBlocked and blockWalkFlag()==FlagOpen.
func TestNpcStepOnce_BlockedNpcStepsOntoWaterTile(t *testing.T) {
	s := newTestServer(t)
	// Two adjacent water tiles: (3221, 3220) and (3222, 3220). Use Add
	// (existing convention — see npc_hunt_test.go:413, npc_player_modes_test.go:457).
	s.gamemap.Pathfinder.Flags.Add(3221, 3220, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3222, 3220, 0, collision.FlagBlockWalk)

	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "duck"},
		WanderRange:  35,
		MoveRestrict: int(MoveRestrictBlocked),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	n.QueueWaypoint(3222, 3220)

	advanced, dir := n.stepOnce(s)
	if !advanced {
		t.Fatalf("blocked NPC failed to step onto adjacent water tile (advanced=%v, dir=%d); want advanced=true", advanced, dir)
	}
	if n.x != 3222 || n.z != 3220 {
		t.Fatalf("blocked NPC at wrong coord after step: got (%d,%d), want (3222,3220)", n.x, n.z)
	}
	if n.waypointIndex != -1 {
		t.Fatalf("waypointIndex after reaching dest: got %d, want -1", n.waypointIndex)
	}
}
```

Notes for the engineer:
- `s.gamemap.Pathfinder.Flags` is the FlagMap (cited from `pkg/gamemap/gamemap.go:30-31`). `Add` allocates the tile zone if absent and ORs the mask.
- `NewNpc(nid, typeId, x, z, level, typ)` is the production constructor (`modules/world/npc.go:159`). Setting `n.server = s` afterwards mirrors how `npc_interaction_test.go` constructs NPCs around line 49-95 — check there for any additional field that newTestNpc/NewNpc-call-sites commonly initialise (e.g. `n.gamemap`, `n.lastTickX/Z`); if a field other than `server` is needed for `stepOnce` to run without nil-deref, set it explicitly here.
- The test's `n.moveRestrict` field is derived from `typ.MoveRestrict` inside `NewNpc` (`modules/world/npc.go:187`). Verify by reading `NewNpc` once at the start of Task 3.

- [ ] **Step 2: Run the test and verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce_BlockedNpcStepsOntoWaterTile -v`
Expected: FAIL. The blocked NPC's step fails because `gamemap.CanTravel` still uses `(1, 0, TypeNormal)`; under TypeNormal the destination's `FlagBlockWalk` bit makes the AND mask non-zero, blocking the step. `stepOnce` then sets `waypointIndex = -1` and returns `(false, -1)`.

- [ ] **Step 3: Commit the RED**

```bash
git add modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "test(world): NAI-175 T3 — pin blocked-NPC step onto water tile (RED)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: GREEN — port `(*Npc).stepOnce` strategy plumbing

**Files:**
- Modify: `modules/world/npc_interaction.go:339-370`

This task ports the **strategy plumbing only** (the D0 baseline). D1 (axis-fallback), D2 (waypoint retention), D3 (size>1) come in Tasks 5/6/7.

- [ ] **Step 1: Rewrite `(*Npc).stepOnce`**

Replace `modules/world/npc_interaction.go:339-370` with:

```go
// stepOnce walks one tile toward the current waypoint and returns
// (advanced, dir). Mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683)
// for the strategy-plumbing arm. Decrements waypointIndex when the
// destination is reached; sets it to -1 when a CanTravel gate blocks
// the step.
//
// TS short-circuits:
//   - getCollisionStrategy() == null (MoveRestrictNoMove) → return -1
//   - blockWalkFlag() == CollisionFlag.NULL (MoveRestrictNoMove) → return -1
// Both map to (false, -1) here.
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	cs := n.getCollisionStrategy()
	if cs == nil {
		return false, -1
	}
	extraFlag := n.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, n.Width(), extraFlag, *cs) {
		n.waypointIndex = -1
		return false, -1
	}
	prevX, prevZ := n.x, n.z
	n.x += dx
	n.z += dz
	n.stepsTaken++
	// Per-step refreshZone — mirrors TS PathingEntity.ts:182-183.
	refreshNpcZone(s, n, prevX, prevZ, n.level)
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, int(dir)
}
```

Verify `"github.com/zsrv/goscape/pkg/pathfinder/collision"` is in the import block of `modules/world/npc_interaction.go`. (Task 2 should have added it; double-check.)

- [ ] **Step 2: Run the Task 3 test and verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce_BlockedNpcStepsOntoWaterTile -v`
Expected: PASS.

- [ ] **Step 3: Run the full module test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/...`
Expected: all green. If any test fails, it is pinning the old TypeNormal-everywhere behaviour for non-Normal NPCs — that test was wrong; fix it by passing the NPC's actual moveRestrict (or, if the test deliberately exercises a Normal-collision path with a different moveRestrict, update the assertion). Do NOT regress the new test.

- [ ] **Step 4: Commit GREEN**

```bash
git add modules/world/npc_interaction.go
git commit --no-gpg-sign -m "fix(world): NAI-175 T4 — (*Npc).stepOnce plumbs per-NPC collision strategy (GREEN)

Pulls blockWalkFlag() and getCollisionStrategy() from the NPC and
passes them through (*GameMap).CanTravel. MoveRestrictBlocked NPCs
(ducks) can now step onto adjacent FlagBlockWalk water tiles.

Mirrors TS PathingEntity.takeStep PathingEntity.ts:617-683 short-circuits
(cs==null → -1, extraFlag==NULL → -1).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: D2 — port blocked-step waypoint retention

**Files:**
- Modify: `modules/world/npc_interaction_test.go`
- Modify: `modules/world/npc_interaction.go:339-380` (stepOnce body)

TS `validateAndAdvanceStep:202-213`: when `takeStep` returns null (stuck), the caller returns `-1` *without* mutating `waypointIndex`. goscape currently sets `n.waypointIndex = -1` on stuck, abandoning the path. Port the TS semantic.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcStepOnce_BlockedTransientLeavesWaypointIntact pins NAI-175 D2.
// When all step directions are blocked, the NPC must return (false, -1)
// WITHOUT clearing waypointIndex — TS validateAndAdvanceStep at
// PathingEntity.ts:202-213 returns -1 without decrementing waypointIndex
// when takeStep returns null.
func TestNpcStepOnce_BlockedTransientLeavesWaypointIntact(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "boxed"},
		WanderRange:  5,
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	// Wall the NPC in: set FlagBlockWalk on all 8 neighbours so every
	// direction's per-direction mask is non-zero under TypeNormal.
	for _, off := range [][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, -1}, {1, 0}, {1, 1}} {
		s.gamemap.Pathfinder.Flags.Add(3221+off[0], 3220+off[1], 0, collision.FlagBlockWalk)
	}
	n.QueueWaypoint(3225, 3220) // far away; must reach via east-step
	preIndex := n.waypointIndex

	advanced, dir := n.stepOnce(s)
	if advanced || dir != -1 {
		t.Fatalf("boxed-in NPC: got (advanced=%v, dir=%d), want (false, -1)", advanced, dir)
	}
	if n.waypointIndex != preIndex {
		t.Fatalf("D2: waypointIndex mutated on transient block: got %d, want %d (TS leaves it intact)", n.waypointIndex, preIndex)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce_BlockedTransientLeavesWaypointIntact -v`
Expected: FAIL — `waypointIndex` was set to `-1` by the current implementation.

- [ ] **Step 3: Update `(*Npc).stepOnce` to leave waypointIndex intact on transient block**

Edit `modules/world/npc_interaction.go` — locate the CanTravel-blocked branch and replace:

```go
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, n.Width(), extraFlag, *cs) {
		n.waypointIndex = -1
		return false, -1
	}
```

with:

```go
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, n.Width(), extraFlag, *cs) {
		// NAI-175 D2: TS validateAndAdvanceStep at PathingEntity.ts:202-213
		// returns -1 without decrementing waypointIndex when takeStep
		// returns null. Leave the path intact for next-tick retry.
		return false, -1
	}
```

Also update the doc comment on `stepOnce` — remove the line "sets it to -1 when a CanTravel gate blocks the step." (no longer true).

- [ ] **Step 4: Run the test and verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce -v`
Expected: all NAI-175 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "fix(world): NAI-175 T5 — D2 stepOnce retains waypointIndex on transient block

TS validateAndAdvanceStep (PathingEntity.ts:202-213) returns -1 without
decrementing waypointIndex when takeStep returns null. goscape was
clearing it, causing wanderMode NPCs to abandon their path after one
blocked step. Port the TS retain-on-block semantic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: D1 — port axis-fallback (X-only / Z-only retry)

**Files:**
- Modify: `modules/world/npc_interaction_test.go`
- Modify: `modules/world/npc_interaction.go` (stepOnce body)

TS `takeStep:654-682`: on a diagonal step, if the diagonal is blocked, try X-only `(dx, 0)` then Z-only `(0, dz)`. The returned `dir` is the *axis-only direction*, not the diagonal.

- [ ] **Step 1: Write the failing test (X-axis fallback)**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcStepOnce_AxisFallback_X pins NAI-175 D1. When the diagonal
// is blocked but the X-only step is open, TS takeStep returns the
// X-only direction. Mirrors PathingEntity.ts:672-675.
func TestNpcStepOnce_AxisFallback_X(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "diag"},
		WanderRange:  5,
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	// Block the NE-diagonal destination (3222, 3221) but leave east
	// (3222, 3220) open. Diagonal direction = NE; expect fallback to East.
	s.gamemap.Pathfinder.Flags.Add(3222, 3221, 0, collision.FlagBlockWalk)
	n.QueueWaypoint(3225, 3225) // NE-ish dest

	advanced, dir := n.stepOnce(s)
	if !advanced {
		t.Fatalf("axis-fallback X: got advanced=false, want true with East-only step")
	}
	if n.x != 3222 || n.z != 3220 {
		t.Fatalf("axis-fallback X: stepped to (%d,%d), want (3222,3220)", n.x, n.z)
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("axis-fallback X: dir=%d, want East (%d)", dir, coordgrid.DirectionEast)
	}
}

// TestNpcStepOnce_AxisFallback_Z mirrors D1 for the Z-axis fallback
// (PathingEntity.ts:677-680).
func TestNpcStepOnce_AxisFallback_Z(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "diag"},
		WanderRange:  5,
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
	}
	n := NewNpc(1, 1, 3221, 3220, 0, typ)
	n.server = s
	// Block NE-diagonal destination (3222, 3221) AND east (3222, 3220)
	// but leave north (3221, 3221) open. Expect Z-axis fallback to North.
	s.gamemap.Pathfinder.Flags.Add(3222, 3221, 0, collision.FlagBlockWalk)
	s.gamemap.Pathfinder.Flags.Add(3222, 3220, 0, collision.FlagBlockWalk)
	n.QueueWaypoint(3225, 3225)

	advanced, dir := n.stepOnce(s)
	if !advanced {
		t.Fatalf("axis-fallback Z: got advanced=false, want true with North-only step")
	}
	if n.x != 3221 || n.z != 3221 {
		t.Fatalf("axis-fallback Z: stepped to (%d,%d), want (3221,3221)", n.x, n.z)
	}
	if dir != int(coordgrid.DirectionNorth) {
		t.Fatalf("axis-fallback Z: dir=%d, want North (%d)", dir, coordgrid.DirectionNorth)
	}
}
```

Direction constants verified — `coordgrid.DirectionEast`, `coordgrid.DirectionNorth`, etc. live at `pkg/coordgrid/coordgrid.go:11-15`.

- [ ] **Step 2: Run the tests and verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce_AxisFallback -v`
Expected: both FAIL — current `stepOnce` only tries the direct (diagonal) direction.

- [ ] **Step 3: Port axis-fallback into stepOnce**

Replace the CanTravel-blocked branch (added in Task 5) with the axis-fallback chain. The full stepOnce body should now look like:

```go
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	cs := n.getCollisionStrategy()
	if cs == nil {
		return false, -1
	}
	extraFlag := n.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s == nil || s.gamemap == nil {
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
	// NAI-175 D2: all directions blocked — leave waypointIndex intact.
	return false, -1
}

// applyStep advances the NPC one tile by (dx, dz), refreshes its zone,
// and decrements waypointIndex if the destination is reached. Factored
// from stepOnce so axis-fallback arms share the same post-step bookkeeping.
// (coordgrid.Position is the return type of coordgrid.UnpackCoord —
// verify at pkg/coordgrid/coordgrid.go:150 if unsure.)
func (n *Npc) applyStep(s *Server, dest coordgrid.Position, dx, dz, dir int) (bool, int) {
	prevX, prevZ := n.x, n.z
	n.x += dx
	n.z += dz
	n.stepsTaken++
	refreshNpcZone(s, n, prevX, prevZ, n.level)
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, dir
}
```

Verified types: `coordgrid.Position` (return of `UnpackCoord`, `pkg/coordgrid/coordgrid.go:150`); `coordgrid.Direction` (return of `Face`/`DeltaX`/`DeltaZ`, `coordgrid.go:8-15`). Direction constants are named `DirectionEast`, `DirectionNorth`, etc.

- [ ] **Step 4: Run all NAI-175 tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce -v`
Expected: all PASS (including D0/D1/D2 tests from Tasks 3, 5, 6).

- [ ] **Step 5: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "fix(world): NAI-175 T6 — D1 stepOnce axis-fallback (X-only / Z-only retry)

Ports TS PathingEntity.takeStep (PathingEntity.ts:668-682) axis-fallback:
on a blocked diagonal, try X-only then Z-only before giving up. Factors
post-step bookkeeping into applyStep helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: D3 — size>1 branch (conditional on Stage 1 verdict)

**Files:**
- Modify: `modules/world/npc_interaction_test.go`
- Modify: `modules/world/npc_interaction.go` (stepOnce body)

If Stage 1 verdict marks D3 LATENT (deferred to NAI-176), skip this task and open a `NAI-175-D-SIZE-GT-1` deviation tag in `npc_interaction.go` above `stepOnce`:

```go
// NAI-175-D-SIZE-GT-1: TS takeStep PathingEntity.ts:642-651 has a
// separate width>1 arm that uses Face(srcX, 0, x, 0) / Face(0, srcZ, 0, z)
// for axis-only checks. goscape currently uses the same single-tile
// logic for all sizes. No size>1 NPC observed broken in NAI-175 smoke;
// deferred to NAI-176. Re-grep if a size>1 wanderer (giant, dragon,
// dagannoth) regresses.
```

Otherwise, port the TS branch.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcStepOnce_Size2Width pins NAI-175 D3. A size=2 NPC stepping
// toward a NE destination should use the width>1 branch, which checks
// X-axis then Z-axis (PathingEntity.ts:642-651). It does NOT try the
// diagonal direction at all in that branch.
func TestNpcStepOnce_Size2Width(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1, DebugName: "bignpc"},
		WanderRange:  5,
		MoveRestrict: int(MoveRestrictNormal),
		Size:         2,
	}
	n := NewNpc(1, 1, 3220, 3220, 0, typ)
	n.server = s
	// 2x2 NPC occupies (3220..3221, 3220..3221). NE-step target = (3225, 3225).
	// All tiles open. Expect East step (X-axis tried first per TS width>1
	// branch).
	n.QueueWaypoint(3225, 3225)

	advanced, dir := n.stepOnce(s)
	if !advanced {
		t.Fatalf("size-2 NPC: got advanced=false, want true")
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("size-2 NPC: dir=%d, want East (TS width>1 branch tries X first)", dir)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce_Size2Width -v`
Expected: FAIL — current stepOnce returns NE (diagonal) regardless of width.

- [ ] **Step 3: Port the width>1 branch**

Insert the width>1 branch at the top of stepOnce, after the `extraFlag == FlagNull` short-circuit:

```go
	if n.Width() > 1 {
		// NAI-175 D3: TS PathingEntity.ts:642-651 — large-NPC arm uses
		// Face(srcX, 0, x, 0) / Face(0, srcZ, 0, z) for axis-only checks.
		// Does NOT try the diagonal.
		tryDirX := coordgrid.Face(n.x, 0, dest.X, 0)
		if int(tryDirX) != -1 {
			ddx := coordgrid.DeltaX(tryDirX)
			if s != nil && s.gamemap != nil && s.gamemap.CanTravel(n.level, n.x, n.z, ddx, 0, n.Width(), extraFlag, *cs) {
				return n.applyStep(s, dest, ddx, 0, int(tryDirX))
			}
		}
		tryDirZ := coordgrid.Face(0, n.z, 0, dest.Z)
		if int(tryDirZ) != -1 {
			ddz := coordgrid.DeltaZ(tryDirZ)
			if s != nil && s.gamemap != nil && s.gamemap.CanTravel(n.level, n.x, n.z, 0, ddz, n.Width(), extraFlag, *cs) {
				return n.applyStep(s, dest, 0, ddz, int(tryDirZ))
			}
		}
		// D2 retention applies here too — leave waypointIndex intact.
		return false, -1
	}
```

Place this block immediately before the existing `dx := coordgrid.DeltaX(dir); dz := coordgrid.DeltaZ(dir)` line. The width-1 codepath below is unchanged.

- [ ] **Step 4: Run all NAI-175 tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnce -v`
Expected: all PASS.

- [ ] **Step 5: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "fix(world): NAI-175 T7 — D3 stepOnce width>1 branch

Ports TS PathingEntity.takeStep width>1 arm (PathingEntity.ts:642-651):
large NPCs use Face(srcX, 0, x, 0) / Face(0, srcZ, 0, z) for axis-only
checks, never trying the diagonal.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: D4 — Player.stepOnce parity (conditional on Stage 1 verdict)

**Files:**
- Modify: `modules/world/movement.go:120-154`
- Modify: `modules/world/movement_test.go` (or wherever Player stepOnce tests live; grep first)

### D4 deferral fallback (skip Task 8 body if Stage 1 says LATENT)

If Stage 1 marks D4 LATENT, **do NOT modify movement.go beyond the Task 2 refactor**. Instead, open a deviation tag — append above `(*Player).stepOnce` in `modules/world/movement.go`:

```go
// NAI-175-D-PLAYER-STEP-COLLISION: (*Player).stepOnce passes
// (size=1, extraFlag=0, TypeNormal) to gamemap.CanTravel. TS port
// (PathingEntity.ts:617-683) calls getCollisionStrategy() and
// blockWalkFlag() per-step. For players this is mostly correct
// (MoveRestrictNormal almost always), but FlagBlockPlayers should
// be the extraFlag per Player.blockWalkFlag (player.go:608-610),
// not 0. Latent bug: players walk through NPCs whose tile carries
// FlagBlockPlayers. Not duck-symptom-binding; tracked under NAI-176.
```

Commit:

```bash
git add modules/world/movement.go
git commit --no-gpg-sign -m "docs(world): NAI-175 T8 — defer Player.stepOnce parity (NAI-175-D-PLAYER-STEP-COLLISION)

Player.stepOnce uses the same hardcoded (1, 0, TypeNormal) plumbing.
Latent (players are MoveRestrictNormal); deferred to NAI-176 per
Stage 1 audit verdict.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

Then proceed to Task 9.

### D4 in-scope port (run this only if Stage 1 says BINDING)

- [ ] **Step 1: Write failing test for player blocking via FlagBlockPlayers**

Append to the relevant `*_test.go` (locate via `grep -rn "func.*Player.*stepOnce" modules/world/*_test.go`). The test pins that a player cannot step onto a tile flagged `FlagBlockPlayers`:

```go
func TestPlayerStepOnce_FlagBlockPlayers(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 0, 3221, 3220, 0)
	s.gamemap.Pathfinder.Flags.Add(3222, 3220, 0, collision.FlagBlockPlayers)
	p.queueWaypoint(3222, 3220)

	dir, advanced := p.stepOnce()
	if advanced {
		t.Fatalf("player stepped onto FlagBlockPlayers tile (dir=%d, advanced=%v); want blocked", dir, advanced)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerStepOnce_FlagBlockPlayers -v`
Expected: FAIL — current player.stepOnce passes extraFlag=0 so FlagBlockPlayers is not in the per-direction mask.

- [ ] **Step 3: Port player.stepOnce to TS takeStep parity**

Edit `modules/world/movement.go:120-154` — apply the same shape as the NPC stepOnce port:

```go
func (p *Player) stepOnce() (coordgrid.Direction, bool) {
	if p.waypointIndex < 0 {
		return -1, false
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		p.waypointIndex--
		return -1, false
	}
	cs := p.getCollisionStrategy()
	if cs == nil {
		return -1, false
	}
	extraFlag := p.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return -1, false
	}
	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
		// Test-only path; preserve prior bypass behaviour.
		p.lastStepX = p.x
		p.lastStepZ = p.z
		p.x += coordgrid.DeltaX(dir)
		p.z += coordgrid.DeltaZ(dir)
		p.stepsTaken++
		if p.x == dest.X && p.z == dest.Z {
			p.waypointIndex--
		}
		return dir, true
	}
	gm := p.client.server.gamemap
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if gm.CanTravel(p.level, p.x, p.z, dx, dz, p.Width(), extraFlag, *cs) {
		return p.applyStep(dest, dx, dz, dir)
	}
	if dx != 0 && gm.CanTravel(p.level, p.x, p.z, dx, 0, p.Width(), extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, dest.X, p.z)
		return p.applyStep(dest, dx, 0, axisDir)
	}
	if dz != 0 && gm.CanTravel(p.level, p.x, p.z, 0, dz, p.Width(), extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, p.x, dest.Z)
		return p.applyStep(dest, 0, dz, axisDir)
	}
	return -1, false
}

func (p *Player) applyStep(dest coordgrid.Position, dx, dz int, dir coordgrid.Direction) (coordgrid.Direction, bool) {
	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++
	refreshPlayerZone(p, p.lastStepX, p.lastStepZ, p.level)
	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, true
}
```

- [ ] **Step 4: Verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all green. If any existing player-movement test fails, it almost certainly pinned the "players walk through NPCs" latent bug as expected behaviour — fix the test, don't regress the port.

- [ ] **Step 5: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "fix(world): NAI-175 T8 — port Player.stepOnce to TS takeStep parity

Plumbs Player.blockWalkFlag (FlagBlockPlayers) and getCollisionStrategy
through gamemap.CanTravel. Ports axis-fallback and waypoint retention
on transient block. Players no longer walk through tiles flagged
FlagBlockPlayers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Smoke handoff (user-driven, out of band)

**Files:** none

- [ ] **Step 1: Build the binary**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/claude/goscape ./cmd/goscape`
Expected: binary at `/tmp/claude/goscape`.

- [ ] **Step 2: Ask the user to run the server and Java client smoke**

Post this exact message to the user:

> **NAI-175 smoke handoff.** Please run the new binary (`/tmp/claude/goscape --config.file config.yaml`) and connect with the Java client. Stand near the Lumbridge river south of the castle for ~30 seconds and watch a couple of adult ducks (`[duck]` and `[duck_female]`).
>
> **Pass:** ducks visibly drift between water tiles.
> **Fail:** ducks remain stationary.
>
> Report back which one. If fail, also note whether `[duckling]` NPCs (different AI path) move.

- [ ] **Step 3: On pass — proceed to Task 10 (close).**

- [ ] **Step 4: On fail — open Stage 3 probe.**

Stage 3 conditional probe: insert `slog.Info("nai175.stepOnce", "typeId", n.typeId, "x", n.x, "z", n.z, "dx", dx, "dz", dz, "extraFlag", extraFlag, "cs", *cs, "reason", reason)` at each return path in `(*Npc).stepOnce` with the appropriate `reason` constant per the spec §7 table. Gate behind the existing NodeDebug pattern. Build, rerun smoke, capture log lines, diagnose.

---

## Task 10: Close — memory + retire Sub-H7

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai175_step_collision_strategy.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (retire Sub-H7)

- [ ] **Step 1: Write the memory entry**

Create `memory/nai175_step_collision_strategy.md`:

```markdown
---
name: NPC/Player stepOnce per-NPC collision strategy
description: NAI-175 closed the hardcoded (size=1, extraFlag=0, TypeNormal) wrapper at gamemap.CanTravel; ducks (MoveRestrictBlocked) now wander on water tiles
type: project
---
NAI-175 — duck-wander symptom at Lumbridge bound to `(*GameMap).CanTravel` hardcoding `(size=1, extraFlag=0, TypeNormal)`. Per-NPC `getCollisionStrategy()` and `blockWalkFlag()` were never plumbed. Stage 2 ported TS `PathingEntity.takeStep` (PathingEntity.ts:617-683) parity into `(*Npc).stepOnce`: strategy plumbing (D0), waypoint retention on transient block (D2), axis-fallback X-only/Z-only (D1). D3 (size>1 branch) [shipped|deferred per Stage 1 verdict]. D4 (Player.stepOnce parallel) [shipped|deferred per Stage 1 verdict].

**Why:** Engine-Field invariants for `MoveRestrictBlocked` NPCs are encoded in `TypeBlocked` collision (requires FlagBlockWalk set on destination; strips FlagBlockWalk from the per-direction mask). The wrapper bypassed this entirely. Same hardcoding was flagged 5 days prior in `nai_followups.md` Sub-H7 under NAI-98 but never the binding cause of that cascade.

**How to apply:** When porting any TS PathingEntity-derived function that calls `canTravel(...)`, always check that the call site plumbs the entity's `blockWalkFlag` and `getCollisionStrategy` — these are not optional. Grep `gamemap.CanTravel\(` to enumerate.
```

- [ ] **Step 2: Add a pointer in MEMORY.md**

Add a line under the appropriate category in `memory/MEMORY.md`:

```markdown
- [NAI-175 NPC stepOnce per-NPC collision strategy](nai175_step_collision_strategy.md) — duck-wander symptom bound to `(*GameMap).CanTravel` hardcoded `(1, 0, TypeNormal)`; ports TS PathingEntity.takeStep parity (D0/D1/D2[/D3][/D4]); retires Sub-H7
```

- [ ] **Step 3: Retire Sub-H7 in nai_followups.md**

Locate the Sub-H7 entry in `memory/nai_followups.md` (around line 5249). Replace its content with:

```markdown
**Sub-H7 — CLOSED by NAI-175 (2026-05-12)** — per-step `CanTravel` plumbing now strategy-aware; was the binding cause of the Lumbridge-duck wander symptom, not the NAI-98 cascade.
```

- [ ] **Step 4: Close commit**

```bash
git add docs/superpowers/specs/2026-05-12-nai-175-npc-step-collision-strategy-design.md \
        docs/superpowers/plans/2026-05-12-nai-175-npc-step-collision-strategy.md
git -C /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory add MEMORY.md nai175_step_collision_strategy.md nai_followups.md 2>/dev/null || true
git commit --no-gpg-sign -m "chore(close): NAI-175 — NPC stepOnce per-NPC collision strategy port, retire Sub-H7

Lumbridge ducks (MoveRestrictBlocked) wander again. (*Npc).stepOnce
ports TS PathingEntity.takeStep parity (D0 strategy plumbing + D1
axis-fallback + D2 waypoint retention[ + D3 size>1][ + D4 player parity]).
Smoke at Lumbridge passes: adult ducks visibly drift between water
tiles within 30s. Sub-H7 (NAI-98) retired.

Closes memory: nai175_step_collision_strategy.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Stage 1 verdict

_To be appended by Task 1 subagent dispatch. Do not edit manually._
