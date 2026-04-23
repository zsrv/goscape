# NAI-13 — PLAYER* Modes + entitymask Plumbing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four PLAYER\* NPC modes from TS `Npc.ts:746-830` and wire `entitymask` at NPC construction so `SetInteraction`'s face-entity mask bit becomes functional. Closes the NAI-11 PLAYER\* deferral and restores the NAI-11-memorialized flipped assertion in `TestNpcTurnHuntAndConsumeSetsTarget`.

**Architecture:** One new file `modules/world/npc_player_modes.go` hosting the four mode methods + a file-scoped escape-direction table. Mask-plumbing adds one line to `NewNpc` and one line to `resetDefaults`. Dispatch wiring is folded into each per-mode task so every task lands fully green. `targetWithinMaxRange` gains two branches (PLAYERFOLLOW early-return + PLAYERESCAPE retreat-maxrange with corner-removal quirk).

**Tech Stack:** Go 1.26+, existing `pkg/pathfinder/collision` (`FlagMap.IsFlagged`, `FlagWallNorth/South/East/West`), existing `pkg/coordgrid` (`DistanceToSW`, `DistanceTo`), existing `modules/world` NPC infrastructure (`pathToTarget`, `queueWaypoint`, `updateMovement`, `validateTarget`, `targetWithinMaxRange`, `resetDefaults`, `SetInteraction`), existing `pkg/rsbuf` (`NpcMaskFaceEntity`), existing `pkg/objtype` NPC-mode constants.

---

## Test commands (reference)

Full suite: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Focused package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Single test: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestName -v`
Race detector: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

---

## Task 1: Mask plumbing (entitymask at construction + resetDefaults emit)

Wire `n.entitymask = rsbuf.NpcMaskFaceEntity` in `NewNpc` (mirrors TS `PathingEntity.ts:107`) and add `n.masks |= n.entitymask` to `resetDefaults` (mirrors TS `Npc.ts:416`). This task is independent of the dispatch wiring — it validates that the existing `SetInteraction` mask-emission line becomes functional.

**Files:**
- Modify: `modules/world/npc.go` (NewNpc, ~line 158)
- Modify: `modules/world/npc_interaction.go` (resetDefaults, lines 32-39)
- Test: `modules/world/npc_masks_test.go` (3 new tests)

- [ ] **Step 1.1: Write the failing test — entitymask set at construction**

Append to `modules/world/npc_masks_test.go`:

```go
// TestNewNpcSetsEntityMaskToFaceEntity — NAI-13 Task 1.
// Mirrors TS PathingEntity.ts:107 where `this.entitymask = entitymask` is
// set at construction. For NPC, this is NpcMaskFaceEntity; the consumer
// is SetInteraction / resetDefaults which emit the faceEntity bit via
// `n.masks |= n.entitymask`. Before this change the field was always 0
// and the `|=` lines were no-ops.
func TestNewNpcSetsEntityMaskToFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	if n.entitymask != rsbuf.NpcMaskFaceEntity {
		t.Errorf("entitymask: got %d, want %d (NpcMaskFaceEntity)", n.entitymask, rsbuf.NpcMaskFaceEntity)
	}
}
```

- [ ] **Step 1.2: Run test — verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewNpcSetsEntityMaskToFaceEntity -v`
Expected: FAIL with `entitymask: got 0, want <nonzero>`.

- [ ] **Step 1.3: Implement entitymask assignment in NewNpc**

Modify `modules/world/npc.go`. Insert one line in the `NewNpc` struct literal (between `changeTypeID: -1,` and the closing `}`):

```go
		changeTypeID:    -1,
		entitymask:      rsbuf.NpcMaskFaceEntity,
	}
	n.targetOp = n.defaultMode()
	return n
}
```

- [ ] **Step 1.4: Run test — verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewNpcSetsEntityMaskToFaceEntity -v`
Expected: PASS.

- [ ] **Step 1.5: Write the failing test — resetDefaults emits entitymask**

Append to `modules/world/npc_masks_test.go`:

```go
// TestResetDefaultsEmitsEntityMask — NAI-13 Task 1.
// Mirrors TS Npc.ts:416 where resetDefaults ends with `this.masks |=
// this.entitymask`. Before NAI-13 the `|=` line in resetDefaults did
// not exist; this test guards the new line + proves entitymask is
// non-zero post-construction.
func TestResetDefaultsEmitsEntityMask(t *testing.T) {
	n := newTestNpc(1)
	n.masks = 0 // clear any construction-time bits
	n.resetDefaults()
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Errorf("masks & NpcMaskFaceEntity: got 0, want nonzero (resetDefaults should emit faceEntity bit)")
	}
}
```

- [ ] **Step 1.6: Run test — verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResetDefaultsEmitsEntityMask -v`
Expected: FAIL — resetDefaults does not set the bit yet.

- [ ] **Step 1.7: Implement resetDefaults mask emission**

Modify `modules/world/npc_interaction.go`. Replace the existing `resetDefaults` body (lines 32-39) with:

```go
// resetDefaults clears target/targetOp to defaultMode baseline and re-emits
// the faceEntity mask bit. Matches TS Npc.resetDefaults at
// Engine-TS/.../Npc.ts:411-425 (the `this.masks |= this.entitymask` at
// :416). INTENTIONALLY does NOT clear apRange, apRangeCalled, faceEntity,
// or the rest of masks — those are overwritten only by the next
// SetInteraction call.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
	n.masks |= n.entitymask
}
```

- [ ] **Step 1.8: Run test — verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResetDefaultsEmitsEntityMask -v`
Expected: PASS.

- [ ] **Step 1.9: Write the failing test — SetInteraction emits entitymask**

This test proves the previously-no-op line at `npc_interaction.go:537-540` is now functional. Append to `modules/world/npc_masks_test.go`:

```go
// TestSetInteractionEmitsEntityMask — NAI-13 Task 1.
// The line `n.masks |= n.entitymask` at npc_interaction.go SetInteraction
// was a no-op before NAI-13 because entitymask was 0. With entitymask
// now wired in NewNpc (Step 1.3), that line emits the faceEntity bit.
// Uses an *Npc target to avoid Player construction overhead — SetInteraction's
// target branch that sets faceEntity fires for both Player and Npc targets.
func TestSetInteractionEmitsEntityMask(t *testing.T) {
	n := newTestNpc(1)
	n.masks = 0
	n.server = &Server{log: discardLogger()} // SetInteraction does not touch log on happy path

	// Use a live *Npc target (has Coords, IsValid via !dead). Npc is an entity.
	target := newTestNpc(2)

	ok := n.SetInteraction(InteractionScript, target, objtype.NPCModeOpNpc1, 0)
	if !ok {
		t.Fatal("SetInteraction returned false (target.IsValid failed?)")
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Errorf("masks & NpcMaskFaceEntity: got 0, want nonzero (SetInteraction should emit faceEntity bit for *Npc target)")
	}
}
```

- [ ] **Step 1.10: Run test — verify pass**

(This test should PASS on the first run because the `|=` line already exists in `SetInteraction`; entitymask is now non-zero so the bit flips.)

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetInteractionEmitsEntityMask -v`
Expected: PASS.

If it fails: re-check Step 1.3 landed correctly, and verify `SetInteraction` at `npc_interaction.go:541` contains `n.masks |= n.entitymask` on either Player-branch or Npc-branch of the target-kind switch.

- [ ] **Step 1.11: Run full package test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -v -run "TestNewNpcSetsEntityMaskToFaceEntity|TestResetDefaultsEmitsEntityMask|TestSetInteractionEmitsEntityMask"`
Expected: 3 PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all previously-green tests still green. **If any previously-green test breaks, investigate before proceeding — NAI-11 assertions on `n.masks` values may be affected.**

- [ ] **Step 1.12: Commit**

```bash
git add modules/world/npc.go modules/world/npc_interaction.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 wire entitymask at NPC construction + resetDefaults

Set n.entitymask = rsbuf.NpcMaskFaceEntity in NewNpc (mirrors TS
PathingEntity.ts:107) and add n.masks |= n.entitymask to resetDefaults
(mirrors TS Npc.ts:416). This makes the previously-no-op faceEntity
mask-emission line in SetInteraction functional.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: targetWithinMaxRange — PLAYERFOLLOW early-return + PLAYERESCAPE retreat branch

Add the two new branches to `targetWithinMaxRange` per TS `Npc.ts:633-635` (PLAYERFOLLOW always-true) and `Npc.ts:657-673` (PLAYERESCAPE retreat-maxrange with corner-removal quirk, mirroring the OP-trigger shape). Independent of dispatch wiring.

**Files:**
- Modify: `modules/world/npc_interaction.go` (targetWithinMaxRange, lines 397-447)
- Test: `modules/world/npc_player_modes_test.go` (new file, 4 tests)

- [ ] **Step 2.1: Write the failing test — PLAYERFOLLOW always in range**

Create `modules/world/npc_player_modes_test.go` with:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestTargetWithinMaxRangePlayerFollowAlwaysTrue — NAI-13 Task 2.
// Mirrors TS Npc.ts:633-635 where PLAYERFOLLOW returns true unconditionally
// at the top of targetWithinMaxRange. Rationale: PLAYERFOLLOW has no
// retreat-range semantics; the player is free to roam arbitrarily far.
func TestTargetWithinMaxRangePlayerFollowAlwaysTrue(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "follow_npc"},
		MaxRange:    2, // deliberately tiny
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.startX, n.startZ = 3094, 3106

	// Player target 100 tiles away on both axes.
	target := &Npc{nid: 2, x: 3194, z: 3206, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerFollow

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true (PLAYERFOLLOW must always return true per TS Npc.ts:633-635)")
	}
}
```

- [ ] **Step 2.2: Run test — verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTargetWithinMaxRangePlayerFollowAlwaysTrue -v`
Expected: FAIL — existing `targetWithinMaxRange` falls through to the `default` branch (SW-distance ≤ maxrange+1 = 3) which rejects a target at distance 100.

- [ ] **Step 2.3: Write the failing test — PLAYERESCAPE rejects only when BOTH exceed**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestTargetWithinMaxRangePlayerEscapeRejectsOnlyWhenBothExceed — NAI-13 Task 2.
// Mirrors TS Npc.ts:657-673: PLAYERESCAPE rejects only when BOTH the NPC's
// and the target's distance-from-start exceed maxrange. Threshold is `>
// maxrange` (strict, no +1, no corner quirk). This lets the NPC flee away
// from start while the target is still inside the retreat box, and vice
// versa.
func TestTargetWithinMaxRangePlayerEscapeRejectsOnlyWhenBothExceed(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	// NPC at (107, 100) — distanceToEscape = 7 > maxrange (5).
	n.x, n.z = 107, 100
	// Target at (108, 100) — targetDistanceFromStart = 8 > maxrange (5).
	target := &Npc{nid: 2, x: 108, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got true, want false — BOTH NPC(d=7) AND target(d=8) exceed maxrange=5")
	}
}
```

- [ ] **Step 2.4: Write the two asymmetric-allowance tests (AND-gate semantics)**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyTargetExceeds — NAI-13 Task 2.
// TS :671: `targetDistanceFromStart > maxrange && distanceToEscape >
// maxrange`. AND-gated — if the NPC is still inside its retreat box,
// validateTarget lets the interaction continue even though the target
// drifted outside. This is the critical semantic difference vs. OP/default
// branches which reject on EITHER side exceeding.
func TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyTargetExceeds(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	n.x, n.z = 100, 100 // NPC on its spawn tile → distanceToEscape = 0 (not > 5)
	// Target at (108, 100) — targetDistanceFromStart = 8 > maxrange (5).
	target := &Npc{nid: 2, x: 108, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true — only target exceeds; NPC still in retreat box")
	}
}

// TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyNpcExceeds — NAI-13 Task 2.
// Mirror of the above: NPC has fled outside the retreat box but the target
// is still nearby. AND-gate keeps the interaction alive.
func TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyNpcExceeds(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	n.x, n.z = 107, 100 // NPC fled 7 tiles → distanceToEscape = 7 > 5
	// Target at (102, 100) — targetDistanceFromStart = 2 (not > 5)
	target := &Npc{nid: 2, x: 102, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true — only NPC exceeds; target still in retreat box")
	}
}
```

- [ ] **Step 2.5: Write the regression-guard test — existing branches unchanged**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestTargetWithinMaxRangeOpTriggerUnchanged — NAI-13 Task 2 regression guard.
// Confirms the existing OP-trigger branch at targetWithinMaxRange lines
// 425-435 still fires for OP modes (PLAYERFOLLOW/PLAYERESCAPE must NOT
// capture these). Uses an OP NPC mode and verifies the maxrange+1
// Chebyshev shape still works.
func TestTargetWithinMaxRangeOpTriggerUnchanged(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "op_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	target := &Npc{nid: 2, x: 105, z: 100, level: 0, typ: typ} // dx=5
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1 // OP-trigger band

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange (OP, dx=5): got false, want true — OP-branch regression")
	}
}
```

- [ ] **Step 2.6: Run tests — verify the 5 new tests (4 fail, 1 pass)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestTargetWithinMaxRange" -v`
Expected:
- `TestTargetWithinMaxRangePlayerFollowAlwaysTrue` FAIL
- `TestTargetWithinMaxRangePlayerEscapeRejectsOnlyWhenBothExceed` FAIL
- `TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyTargetExceeds` FAIL
- `TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyNpcExceeds` FAIL
- `TestTargetWithinMaxRangeOpTriggerUnchanged` PASS (regression guard on existing behavior)

- [ ] **Step 2.7: Implement the two new branches**

Modify `modules/world/npc_interaction.go`. Replace the entire `targetWithinMaxRange` function (lines 397-447) with:

```go
// targetWithinMaxRange enforces the per-mode maxrange rules on n.target.
// Five branches: PLAYERFOLLOW (always true), PLAYERESCAPE (AND-gated
// retreat using NPC-to-start AND target-to-start), OP (maxrange+1 with
// corner-removal quirk), AP (maxrange + attackrange SW-distance), default
// (maxrange+1 SW-distance). Matches TS Npc.targetWithinMaxRange at
// Engine-TS/.../Npc.ts:629-680.
func (n *Npc) targetWithinMaxRange() bool {
	if n.target == nil {
		return true
	}
	if n.typ == nil {
		return false
	}

	// TS :633-635 — PLAYERFOLLOW has no retreat bound.
	if n.targetOp == objtype.NPCModePlayerFollow {
		return true
	}

	maxrng := int(n.typ.MaxRange)
	attackrng := int(n.typ.AttackRange)

	tx, tz, _ := n.target.Coords()
	dx := tx - n.startX
	if dx < 0 {
		dx = -dx
	}
	dz := tz - n.startZ
	if dz < 0 {
		dz = -dz
	}

	// TS :657-673 — PLAYERESCAPE retreat. Size-aware distanceTo from BOTH
	// NPC and target to (startX, startZ); rejects only when BOTH exceed
	// maxrange. No +1, no corner-removal — shape is distinct from the OP
	// branch. For size-1 NPC/Player (the only case this era supports),
	// DistanceToSW is equivalent to the TS size-aware distanceTo; the
	// size-approximation inherits NAI-12's tracked follow-up.
	if n.targetOp == objtype.NPCModePlayerEscape {
		distanceToEscape := coordgrid.DistanceToSW(n.x, n.z, n.startX, n.startZ)
		targetDistanceFromStart := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		if targetDistanceFromStart > maxrng && distanceToEscape > maxrng {
			return false
		}
		return true
	}

	switch {
	case checkOpTrigger(n.targetOp):
		// TS :640-648 — maxrange+1 with corner-removal quirk.
		maxAxis := max(dx, dz)
		if maxAxis > maxrng+1 {
			return false
		}
		if dx == maxrng+1 && dz == maxrng+1 {
			return false
		}
		return true

	case checkApTrigger(n.targetOp):
		// TS :651-654 — SW-distance up to maxrange + attackrange.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+attackrng

	default:
		// TS :676 — SW-distance up to maxrange + 1.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+1
	}
}
```

Note: the previous DEVIATION comment at the old function header (lines 402-404) is removed by this rewrite.

- [ ] **Step 2.8: Run tests — verify all 5 pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestTargetWithinMaxRange" -v`
Expected: 5 PASS.

- [ ] **Step 2.9: Run full package — verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 2.10: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_player_modes_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 targetWithinMaxRange PLAYERFOLLOW + PLAYERESCAPE branches

Add PLAYERFOLLOW early-return of true (TS Npc.ts:633-635) and
PLAYERESCAPE retreat-maxrange branch with corner-removal quirk
(TS Npc.ts:657-673). Remove the NAI-11 DEVIATION note pointing at
these missing branches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: playerFaceMode + dispatch case

Trivial type-guard-only method + dispatch wiring. The face-entity bit is already emitted by `SetInteraction`'s `n.masks |= n.entitymask` (now functional per Task 1), so `playerFaceMode` has no body beyond the type guard.

**Files:**
- Create: `modules/world/npc_player_modes.go`
- Modify: `modules/world/npc_interaction.go` (processMovementInteraction switch, lines 176-184; DEVIATION comment at lines 141-143)
- Test: `modules/world/npc_player_modes_test.go`

- [ ] **Step 3.1: Write the failing test — dispatch routes to playerFaceMode**

Append to `modules/world/npc_player_modes_test.go`:

```go
// playerModeFixture builds a minimal Server + Npc + Player target ready
// for processMovementInteraction dispatch tests. NPC at (3094, 3106);
// player at same tile (caller should move as needed). Players and NPCs
// are registered in s.grid / s.npcs / s.players. The returned Player has
// p.active = true so Player.IsValid() returns true and validateTarget's
// Gate 4 passes. s.gamemap is NOT wired by default — callers that need
// wall-flag seeding for PLAYERESCAPE add it via
// `s.gamemap = gamemap.New(...)` after calling this helper.
func playerModeFixture(t *testing.T) (*Server, *Npc, *Player) {
	t.Helper()
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		MaxRange:    10,
		AttackRange: 1,
		WanderRange: 1, // so defaultMode() returns NPCModeWander (not None)
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.server = s
	p := addPlayerToServer(t, s, 1, 3094, 3106, 0)
	p.active = true // required for Player.IsValid() → validateTarget Gate 4
	return s, n, p
}

// TestProcessMovementInteractionDispatchPlayerFace — NAI-13 Task 3.
// PLAYERFACE is a no-op mode (type guard only) — after dispatch, the NPC's
// target MUST still be set (resetDefaults-stub behavior from NAI-11 clears
// target; NAI-13 dispatch must route to playerFaceMode which leaves state
// alone). Mask-wise, the faceEntity bit comes from the earlier
// SetInteraction call, not from playerFaceMode itself.
func TestProcessMovementInteractionDispatchPlayerFace(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3094, 3108 // close enough for validateTarget
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFace, 0)

	n.processMovementInteraction(s)

	if n.target == nil {
		t.Error("target: got nil, want non-nil — PLAYERFACE dispatch must NOT reset (this is the NAI-11 stub behavior)")
	}
	if n.targetOp != objtype.NPCModePlayerFace {
		t.Errorf("targetOp: got %d, want NPCModePlayerFace (%d)", n.targetOp, objtype.NPCModePlayerFace)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (was set by SetInteraction earlier)")
	}
}
```

Also prepend the required imports — the file currently imports only `testing` and `objtype`. Replace the import block at the top of `npc_player_modes_test.go` with:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

- [ ] **Step 3.2: Write the type-guard test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerFaceNonPlayerTargetLogsAndReturns — NAI-13 Task 3.
// TS throws on type mismatch (Npc.ts:816-818); Go logs + returns.
// Verifies the method does not panic and leaves state unchanged when
// the target is unexpectedly non-Player.
func TestPlayerFaceNonPlayerTargetLogsAndReturns(t *testing.T) {
	s, n, _ := playerModeFixture(t)
	other := newTestNpc(2)
	n.target = other
	n.targetOp = objtype.NPCModePlayerFace

	// Direct method call — not via dispatch.
	n.playerFaceMode(s)

	if n.target != other {
		t.Error("target: mutated on non-Player input — expected log-and-return")
	}
}
```

- [ ] **Step 3.3: Run tests — verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessMovementInteractionDispatchPlayerFace|TestPlayerFaceNonPlayerTargetLogsAndReturns" -v`
Expected: compile error — `n.playerFaceMode undefined`.

- [ ] **Step 3.4: Create `npc_player_modes.go` with the empty file + playerFaceMode**

Create `modules/world/npc_player_modes.go`:

```go
package world

// PLAYER* NPC mode implementations. Each per-tick mode method is dispatched
// from (*Npc).processMovementInteraction's targeted-mode switch when
// n.targetOp matches the corresponding NPCModePlayer* constant. Mirrors
// TS Engine-TS/src/engine/entity/Npc.ts:746-830.
//
// Contract (inherited from the processMovementInteraction prelude):
//  - n.target != nil
//  - validateTarget() returned true
//  - n.typ != nil (enforced by validateTarget's maxrange branch)
//
// On *Player target-type mismatch, each method logs a warn and returns
// without mutating state. TS throws in this case (Npc.ts:748/804/817/823);
// Go's tick loop has no throw-and-recover scope, so we log-and-return.
// This is a minor deviation tracked in the NAI-13 spec § error handling.

// playerFaceMode — TS Npc.ts:815-819. No body beyond the type guard:
// the faceEntity mask bit is emitted by SetInteraction's
// `n.masks |= n.entitymask` line at the time the interaction was
// anchored, not per-tick here. The `s` arg is taken for symmetry with
// other mode methods (noMode / wanderMode / aiMode) and is unused.
func (n *Npc) playerFaceMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		if n.server != nil && n.server.log != nil {
			n.server.log.Warn("playerFaceMode: non-Player target",
				"nid", n.nid, "targetOp", n.targetOp)
		}
		return
	}
}
```

- [ ] **Step 3.5: Wire dispatch for PLAYERFACE**

Modify `modules/world/npc_interaction.go`. Replace the targeted-mode dispatch switch (lines 175-184) with:

```go
	// Targeted-mode dispatch.
	switch n.targetOp {
	case objtype.NPCModePlayerEscape,
		objtype.NPCModePlayerFollow,
		objtype.NPCModePlayerFaceClose:
		// NAI-13: PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACECLOSE land in
		// later tasks. Until then they still fall through to resetDefaults.
		n.resetDefaults()
	case objtype.NPCModePlayerFace:
		n.playerFaceMode(s)
	default:
		n.aiMode(s)
	}
```

Leave the DEVIATION comment at lines 141-143 in place for now — it's still accurate for 3 of the 4 modes. It will be removed in Task 6 when the last mode lands.

- [ ] **Step 3.6: Run tests — verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessMovementInteractionDispatchPlayerFace|TestPlayerFaceNonPlayerTargetLogsAndReturns" -v`
Expected: 2 PASS.

- [ ] **Step 3.7: Run full package — verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 3.8: Commit**

```bash
git add modules/world/npc_player_modes.go modules/world/npc_player_modes_test.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 add playerFaceMode + dispatch

Port TS Npc.ts:815-819. Pure type-guard body; the faceEntity mask
bit is emitted by SetInteraction (now functional after NAI-13 Task 1
wired entitymask at construction). Dispatch switch expands to route
NPCModePlayerFace through the new method instead of the
resetDefaults stub.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: playerFaceCloseMode + dispatch case

Distance-1 Chebyshev gate + type guard. If the player moves to Chebyshev distance > 1, `resetDefaults` fires and the mode ends. Per TS `Npc.ts:821-829`.

**Files:**
- Modify: `modules/world/npc_player_modes.go`
- Modify: `modules/world/npc_interaction.go` (processMovementInteraction switch)
- Test: `modules/world/npc_player_modes_test.go`

- [ ] **Step 4.1: Write the three tests**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerFaceCloseWithinRangeNoops — NAI-13 Task 4.
// Chebyshev distance ≤ 1 → state unchanged, target preserved. TS Npc.ts:826
// inverts this: `> 1 → resetDefaults`.
func TestPlayerFaceCloseWithinRangeNoops(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3095, 3106 // Chebyshev 1
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)
	origTarget := n.target

	n.playerFaceCloseMode(s)

	if n.target != origTarget {
		t.Error("target: mutated despite within-range (Chebyshev=1)")
	}
	if n.targetOp != objtype.NPCModePlayerFaceClose {
		t.Errorf("targetOp: got %d, want NPCModePlayerFaceClose", n.targetOp)
	}
}

// TestPlayerFaceCloseBeyondRangeResetsDefaults — NAI-13 Task 4.
// TS Npc.ts:826-828: `distanceTo(this, target) > 1 → resetDefaults`.
func TestPlayerFaceCloseBeyondRangeResetsDefaults(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3096, 3106 // Chebyshev 2
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.playerFaceCloseMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (resetDefaults should clear target)", n.target)
	}
	if n.targetOp != n.defaultMode() {
		t.Errorf("targetOp: got %d, want defaultMode (%d)", n.targetOp, n.defaultMode())
	}
}

// TestPlayerFaceCloseAsymmetricAxisQuirk — NAI-13 Task 4.
// Quirk guard: the Chebyshev gate MUST reject targets that are "far on
// one axis but same on the other" (i.e. dx=2, dz=0). This catches a bug
// where someone might write `if dx > 1 && dz > 1` (with AND instead of
// using max) — that form would let (+2, 0) through.
func TestPlayerFaceCloseAsymmetricAxisQuirk(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3096, 3106 // dx=2, dz=0 — single-axis beyond-range
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.playerFaceCloseMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil — single-axis dx=2 must be beyond Chebyshev-1 range", n.target)
	}
}
```

- [ ] **Step 4.2: Run tests — verify fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerFaceClose" -v`
Expected: compile error — `n.playerFaceCloseMode undefined`.

- [ ] **Step 4.3: Implement playerFaceCloseMode**

Append to `modules/world/npc_player_modes.go`:

```go
// playerFaceCloseMode — TS Npc.ts:821-829. Keeps the interaction active only
// while the target Player is within Chebyshev distance 1; otherwise clears
// the interaction via resetDefaults. The faceEntity mask bit is (as with
// playerFaceMode) emitted by the original SetInteraction call. The `s`
// arg is taken for symmetry with other mode methods and is unused.
func (n *Npc) playerFaceCloseMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		if n.server != nil && n.server.log != nil {
			n.server.log.Warn("playerFaceCloseMode: non-Player target",
				"nid", n.nid, "targetOp", n.targetOp)
		}
		return
	}

	// TS CoordGrid.distanceTo(this, target) — size-aware Chebyshev.
	// NAI-13 inherits the 1,1,1,1 size approximation tracked as the
	// NAI-12 "size-aware LoS" follow-up; single-tile NPCs + single-tile
	// Players reduce this to plain max(|dx|, |dz|).
	tx, tz, _ := p.Coords()
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if max(dx, dz) > 1 {
		n.resetDefaults()
		return
	}
}
```

- [ ] **Step 4.4: Wire dispatch for PLAYERFACECLOSE**

Modify `modules/world/npc_interaction.go`. Update the targeted-mode switch (from Task 3) to:

```go
	// Targeted-mode dispatch.
	switch n.targetOp {
	case objtype.NPCModePlayerEscape,
		objtype.NPCModePlayerFollow:
		// NAI-13: PLAYERESCAPE / PLAYERFOLLOW land in later tasks.
		n.resetDefaults()
	case objtype.NPCModePlayerFace:
		n.playerFaceMode(s)
	case objtype.NPCModePlayerFaceClose:
		n.playerFaceCloseMode(s)
	default:
		n.aiMode(s)
	}
```

- [ ] **Step 4.5: Run tests — verify all 3 pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerFaceClose" -v`
Expected: 3 PASS.

- [ ] **Step 4.6: Write + run the dispatch-routing test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestProcessMovementInteractionDispatchPlayerFaceClose — NAI-13 Task 4.
// Player beyond Chebyshev-1 → the mode resetDefaults-clears target.
// Proves the dispatch switch routes to playerFaceCloseMode (not the
// resetDefaults stub — which would also nil target but wouldn't trigger
// the distance-gate logic we just wrote).
func TestProcessMovementInteractionDispatchPlayerFaceClose(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3094, 3109 // Chebyshev 3 — beyond face-close range
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.processMovementInteraction(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (playerFaceCloseMode distance gate should fire)", n.target)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessMovementInteractionDispatchPlayerFaceClose" -v`
Expected: PASS.

- [ ] **Step 4.7: Run full package — verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 4.8: Commit**

```bash
git add modules/world/npc_player_modes.go modules/world/npc_player_modes_test.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 add playerFaceCloseMode + dispatch

Port TS Npc.ts:821-829. Chebyshev distance-1 gate: beyond range,
resetDefaults clears the interaction. Type-guard on non-Player
follows the established log-and-return pattern.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: playerFollowMode + dispatch case + NAI-11 flipped-test restoration

Simplest motion mode (`pathToTarget` + `updateMovement`) plus the NAI-11 visible-regression fix: flip `TestNpcTurnHuntAndConsumeSetsTarget` back to its original "target is the hunted NPC" assertion.

**Files:**
- Modify: `modules/world/npc_player_modes.go`
- Modify: `modules/world/npc_interaction.go` (processMovementInteraction switch)
- Modify: `modules/world/npc_hunt_test.go` (TestNpcTurnHuntAndConsumeSetsTarget)
- Test: `modules/world/npc_player_modes_test.go`

- [ ] **Step 5.1: Write the failing tests**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerFollowQueuesWaypointAtTarget — NAI-13 Task 5.
// TS Npc.ts:801-812: `pathToTarget(); updateMovement()`. Naive-path port
// inherited from NAI-11: pathToTarget queues a single waypoint at the
// player's current tile. The SMART branch is still deferred (NAI-11).
func TestPlayerFollowQueuesWaypointAtTarget(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3100, 3112
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	n.playerFollowMode(s)

	// queueWaypoint writes waypoints[0] = coordgrid.PackCoord(level, x, z).
	// Round-trip via UnpackCoord for a clean assertion.
	if n.waypointIndex != 0 {
		t.Fatalf("waypointIndex: got %d, want 0 (waypoint should be queued)", n.waypointIndex)
	}
	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != p.x || pos.Z != p.z || pos.Level != p.level {
		t.Errorf("waypoint: got (level=%d, x=%d, z=%d), want (level=%d, x=%d, z=%d)",
			pos.Level, pos.X, pos.Z, p.level, p.x, p.z)
	}
}

// TestPlayerFollowAdvancesOneTile — NAI-13 Task 5.
// Proves updateMovement actually runs (not just pathToTarget). After one
// tick the NPC should be one tile closer to the player (typ.MoveSpeed
// defaults to Instant/Walk = 1 step/tick via NpcType.MoveRestrict).
func TestPlayerFollowAdvancesOneTile(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3094, 3106
	p.x, p.z = 3094, 3112 // +6 Z
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	startZ := n.z
	n.playerFollowMode(s)

	if n.z == startZ {
		t.Errorf("z: got %d, want != %d (updateMovement should have stepped)", n.z, startZ)
	}
}

// TestPlayerFollowNonPlayerTargetLogsAndReturns — NAI-13 Task 5.
// Type-guard behavior. TS throws (Npc.ts:804-806); Go logs + returns.
func TestPlayerFollowNonPlayerTargetLogsAndReturns(t *testing.T) {
	s, n, _ := playerModeFixture(t)
	other := newTestNpc(2)
	n.target = other
	n.targetOp = objtype.NPCModePlayerFollow

	n.playerFollowMode(s)

	// No waypoint should have been queued.
	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no waypoint on non-Player target)", n.waypointIndex)
	}
	if n.target != other {
		t.Error("target: mutated on non-Player input")
	}
}
```

Now add the `coordgrid` import to `npc_player_modes_test.go`'s import block:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

- [ ] **Step 5.2: Run tests — verify fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerFollow" -v`
Expected: compile error — `n.playerFollowMode undefined`.

- [ ] **Step 5.3: Implement playerFollowMode**

Append to `modules/world/npc_player_modes.go`:

```go
// playerFollowMode — TS Npc.ts:801-812. Each tick, path toward the Player's
// current tile and step one waypoint. Naive-only: the SMART A* branch in
// pathToTarget is deferred from NAI-11 (PathingEntity.ts:457-508).
func (n *Npc) playerFollowMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		if n.server != nil && n.server.log != nil {
			n.server.log.Warn("playerFollowMode: non-Player target",
				"nid", n.nid, "targetOp", n.targetOp)
		}
		return
	}
	n.pathToTarget()
	n.updateMovement(s)
}
```

- [ ] **Step 5.4: Wire dispatch for PLAYERFOLLOW**

Modify `modules/world/npc_interaction.go`. Update the targeted-mode switch (from Task 4) to:

```go
	// Targeted-mode dispatch.
	switch n.targetOp {
	case objtype.NPCModePlayerEscape:
		// NAI-13: PLAYERESCAPE lands in Task 6.
		n.resetDefaults()
	case objtype.NPCModePlayerFollow:
		n.playerFollowMode(s)
	case objtype.NPCModePlayerFace:
		n.playerFaceMode(s)
	case objtype.NPCModePlayerFaceClose:
		n.playerFaceCloseMode(s)
	default:
		n.aiMode(s)
	}
```

- [ ] **Step 5.5: Run tests — verify 3 pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerFollow" -v`
Expected: 3 PASS.

- [ ] **Step 5.6: Write + run the dispatch-routing test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestProcessMovementInteractionDispatchPlayerFollow — NAI-13 Task 5.
// Proves the dispatch switch now routes PLAYERFOLLOW to playerFollowMode
// (rather than the resetDefaults stub). Observable effect: waypoint
// queued at the player's tile.
func TestProcessMovementInteractionDispatchPlayerFollow(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3099, 3111
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	n.processMovementInteraction(s)

	if n.target == nil {
		t.Fatal("target: got nil, want non-nil (PLAYERFOLLOW should preserve target, not resetDefaults-stub)")
	}
	if n.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0 (pathToTarget should have queued a waypoint)", n.waypointIndex)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessMovementInteractionDispatchPlayerFollow" -v`
Expected: PASS.

- [ ] **Step 5.7: Flip the NAI-11 memorialized assertion**

Modify `modules/world/npc_hunt_test.go`. Replace the body of `TestNpcTurnHuntAndConsumeSetsTarget` (lines 263-309) with:

```go
func TestNpcTurnHuntAndConsumeSetsTarget(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)

	// Prepare the NPC to run a full turn() and reach consumeHuntTarget:
	//   - Avoid the Events block by setting lifecycle to Forever.
	//   - Place the NPC at a known coord with a configured huntRange.
	n.lifecycle = NpcLifecycleForever
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Configure the hunt for NPC-type hunt.
	hunt.Type = objtype.HuntModeNpc
	hunt.Rate = 1
	hunt.FindNewMode = 4 // PLAYERFOLLOW (interaction branch)
	hunt.FindKeepHunting = true
	hunt.NobodyNear = objtype.HuntNobodyNearKeepHunting
	hunt.CheckNpc = -1
	hunt.CheckCategory = -1

	hunted := addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)
	s.npcs[1] = n

	// Run the full tick.
	n.turn(s)

	// NAI-13 (restored): PLAYERFOLLOW now dispatches to playerFollowMode
	// (TS Npc.ts:801-812) instead of the NAI-11 resetDefaults stub. The
	// target is preserved across the turn. This is the original pre-NAI-11
	// assertion shape, restored by NAI-13 Task 5.
	if n.target == nil {
		t.Errorf("target: got nil, want the hunted NPC (PLAYERFOLLOW should preserve target)")
	} else if n.target.Slot() != hunted.nid {
		t.Errorf("target: got nid %d, want %d (hunted NPC)", n.target.Slot(), hunted.nid)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (cleared by consumeHuntTarget)", n.huntTarget)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset by consumeHuntTarget)", n.huntClock)
	}
	if n.huntMode != 0 {
		t.Errorf("huntMode: got %d, want 0 (FindKeepHunting=true preserves it)", n.huntMode)
	}
}
```

Note two changes from the previous body: (1) the `addNpcToServerAt` result is captured as `hunted` so we can assert `n.target.Slot() == hunted.nid`; (2) the assertion + comment block is inverted.

- [ ] **Step 5.8: Run the flipped test — verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcTurnHuntAndConsumeSetsTarget" -v`
Expected: PASS.

- [ ] **Step 5.9: Run full package — verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 5.10: Commit**

```bash
git add modules/world/npc_player_modes.go modules/world/npc_player_modes_test.go modules/world/npc_interaction.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 add playerFollowMode + dispatch + flip NAI-11 test

Port TS Npc.ts:801-812: pathToTarget + updateMovement each tick. Naive
pathing only; SMART A* branch remains deferred (NAI-11). Dispatch
switch now routes NPCModePlayerFollow to the new method.

Flip TestNpcTurnHuntAndConsumeSetsTarget back to its original pre-NAI-11
"target is the hunted NPC" assertion now that PLAYERFOLLOW preserves
the target across the turn.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: playerEscapeMode + final dispatch case

The largest task — quadrant-directed flee with wall-flag collision check, maxrange boundary, and axis fallback. Per TS `Npc.ts:746-799`.

**Files:**
- Modify: `modules/world/npc_player_modes.go` (add escape direction table + playerEscapeMode)
- Modify: `modules/world/npc_interaction.go` (final dispatch case + remove DEVIATION comment at lines 141-143)
- Test: `modules/world/npc_player_modes_test.go`

- [ ] **Step 6.1: Write the four quadrant-direction tests**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerEscapeQuadrantPosXPosZ — NAI-13 Task 6.
// TS Npc.ts:758-760: when target.x >= npc.x AND target.z >= npc.z,
// direction = SOUTH_WEST; NPC candidate tile is (nx-1, nz-1).
// In RS coord semantics this is: target is NE of NPC → NPC flees SW.
func TestPlayerEscapeQuadrantPosXPosZ(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3101, 3101 // target at (npc.x+1, npc.z+1)
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	if n.waypointIndex != 0 {
		t.Fatalf("waypointIndex: got %d, want 0 (waypoint should be queued)", n.waypointIndex)
	}
	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3099 || pos.Z != 3099 {
		t.Errorf("waypoint: got (%d, %d), want (3099, 3099) [NE target → SW flee delta (-1, -1)]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeQuadrantPosXNegZ — NAI-13 Task 6. TS :761-763.
// target.x >= npc.x AND target.z < npc.z → direction = NORTH_WEST;
// candidate (nx-1, nz+1). Target SE of NPC → NPC flees NW.
func TestPlayerEscapeQuadrantPosXNegZ(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3101, 3099 // target at (npc.x+1, npc.z-1)
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3099 || pos.Z != 3101 {
		t.Errorf("waypoint: got (%d, %d), want (3099, 3101) [SE target → NW flee delta (-1, +1)]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeQuadrantNegXPosZ — NAI-13 Task 6. TS :764-766.
// target.x < npc.x AND target.z >= npc.z → direction = SOUTH_EAST;
// candidate (nx+1, nz-1). Target NW of NPC → NPC flees SE.
func TestPlayerEscapeQuadrantNegXPosZ(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3099, 3101
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3101 || pos.Z != 3099 {
		t.Errorf("waypoint: got (%d, %d), want (3101, 3099) [NW target → SE flee delta (+1, -1)]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeQuadrantNegXNegZ — NAI-13 Task 6. TS :767-770.
// target.x < npc.x AND target.z < npc.z → direction = NORTH_EAST;
// candidate (nx+1, nz+1). Target SW of NPC → NPC flees NE.
func TestPlayerEscapeQuadrantNegXNegZ(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3099, 3099
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3101 || pos.Z != 3101 {
		t.Errorf("waypoint: got (%d, %d), want (3101, 3101) [SW target → NE flee delta (+1, +1)]", pos.X, pos.Z)
	}
}
```

- [ ] **Step 6.2: Write the abandon-gate test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerEscapeDistanceGateAbandons — NAI-13 Task 6. TS Npc.ts:751-754.
// When the NPC is already 25+ SW-tiles from the target, resetDefaults fires
// (interaction ends). SW-distance = max(|dx|, |dz|) per coordgrid.DistanceToSW.
func TestPlayerEscapeDistanceGateAbandons(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3126, 3100 // dx=26 > 25
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (distance-gate should have resetDefaults'd)", n.target)
	}
	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no waypoint on abandon)", n.waypointIndex)
	}
}
```

- [ ] **Step 6.3: Write the wall-blocked test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// withWallFlag installs a single directional wall flag at (x, z, level).
// Unlike withBlockingWall (which is bidirectional LoS+LoW), this seeds
// exactly one flag bit — the PLAYERESCAPE wall-check test requires a
// direction-pair match against the quadrant's WALL_{N|S}|WALL_{E|W} mask.
// The NPC's own tile's zone must be allocated so Get() returns tile-level
// flags (not FlagNull).
func withWallFlag(t *testing.T, s *Server, x, z, level, flag int) {
	t.Helper()
	s.gamemap.Pathfinder.Flags.Add(x, z, level, flag)
}

// TestPlayerEscapeBlockedByWallResetsDefaults — NAI-13 Task 6.
// TS Npc.ts:775-778: when the candidate flee tile's wall flags match the
// quadrant's direction-pair, resetDefaults fires instead of a waypoint.
// Setup: target at (+1, +1) → direction SW → candidate tile (nx-1, nz-1) →
// flags WALL_SOUTH | WALL_WEST must trigger the reject.
func TestPlayerEscapeBlockedByWallResetsDefaults(t *testing.T) {
	s, n, p := playerModeFixture(t)
	s.gamemap = gamemap.New(discardLogger())
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	p.x, p.z = 3101, 3101 // target at (+1, +1) → flee to (3099, 3099)

	// Seed the candidate tile with WALL_SOUTH|WALL_WEST so IsFlagged returns true.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3099, 3099, 0)
	withWallFlag(t, s, 3099, 3099, 0, collision.FlagWallSouth|collision.FlagWallWest)

	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (wall-check should resetDefaults)", n.target)
	}
	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no waypoint on wall block)", n.waypointIndex)
	}
}
```

Add imports for `gamemap` and `collision` at the top of the file. Update the import block:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

- [ ] **Step 6.4: Write the maxrange + axis-fallback tests**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerEscapeWithinMaxRangeQueuesDiagonal — NAI-13 Task 6.
// TS Npc.ts:780-790: candidate tile within DistanceToSW of startXZ <
// typ.MaxRange → queue the diagonal waypoint and stop.
// Setup: startX,Z = 3100,3100; MaxRange = 10; target at (+1, +1) →
// candidate (3099, 3099); distance from start = max(|nx-1-startX|, |nz-1-startZ|)
// = 1 < 10 → diagonal waypoint.
func TestPlayerEscapeWithinMaxRangeQueuesDiagonal(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	n.typ.MaxRange = 10
	p.x, p.z = 3101, 3101
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3099 || pos.Z != 3099 {
		t.Errorf("waypoint: got (%d, %d), want (3099, 3099) [within-maxrange diagonal]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeBeyondMaxRangeNorthAxisFallback — NAI-13 Task 6.
// TS Npc.ts:793-794: direction NE or NW + candidate beyond MaxRange of
// startXZ → single-axis fallback on Z (queue at (n.x, mz) — keep X fixed).
// Setup: NPC at startXZ; target at (-5, -5) so direction is NE;
// candidate is (nx+1, nz+1) = (3101, 3101). MaxRange = 0 forces the
// fallback. Fallback waypoint: (n.x, mz) = (3100, 3101).
func TestPlayerEscapeBeyondMaxRangeNorthAxisFallback(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	n.typ.MaxRange = 0 // candidate's distance-from-start (=1) >= MaxRange → fallback
	p.x, p.z = 3095, 3095 // target to NE direction (tx < nx && tz < nz)
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3100 || pos.Z != 3101 {
		t.Errorf("waypoint: got (%d, %d), want (3100, 3101) [NE/NW fallback on Z axis]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeBeyondMaxRangeSouthAxisFallback — NAI-13 Task 6.
// TS Npc.ts:795-796: direction SE or SW + beyond MaxRange → fallback on
// X axis (queue at (mx, n.z)). Setup: target at (+5, +5) → direction SW
// → candidate (nx-1, nz-1). MaxRange = 0 forces fallback: (3099, 3100).
func TestPlayerEscapeBeyondMaxRangeSouthAxisFallback(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	n.typ.MaxRange = 0
	p.x, p.z = 3105, 3105 // SW direction
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.playerEscapeMode(s)

	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != 3099 || pos.Z != 3100 {
		t.Errorf("waypoint: got (%d, %d), want (3099, 3100) [SE/SW fallback on X axis]", pos.X, pos.Z)
	}
}

// TestPlayerEscapeNonPlayerTargetLogsAndReturns — NAI-13 Task 6.
// Type-guard. TS Npc.ts:748 throws; Go logs + returns.
func TestPlayerEscapeNonPlayerTargetLogsAndReturns(t *testing.T) {
	s, n, _ := playerModeFixture(t)
	other := newTestNpc(2)
	n.target = other
	n.targetOp = objtype.NPCModePlayerEscape

	n.playerEscapeMode(s)

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no waypoint on non-Player target)", n.waypointIndex)
	}
	if n.target != other {
		t.Error("target: mutated on non-Player input")
	}
}
```

- [ ] **Step 6.5: Run tests — verify fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerEscape" -v`
Expected: compile error — `n.playerEscapeMode undefined`.

- [ ] **Step 6.6: Implement the escape-direction table + playerEscapeMode**

Append to `modules/world/npc_player_modes.go`:

```go
// escapeDirection captures the per-quadrant flee data for playerEscapeMode.
// Each record maps "where the player is relative to the NPC" to the flee
// step delta, the wall-flag pair that blocks the candidate tile, and the
// axis-fallback formula. Mirrors TS Npc.ts:758-797's quadrant if/else.
type escapeDirection struct {
	dx, dz    int
	wallFlags int
	// fallbackUseNpcX: true → fallback waypoint is (n.x, mz)  [N variants]
	//                  false → fallback waypoint is (mx, n.z) [S variants]
	fallbackUseNpcX bool
}

// pickEscapeDirection picks the quadrant record for a target relative to
// the NPC. Matches TS Npc.ts:758-770 exactly. Orientation in RS coords:
// +x = east, +z = north. So target at (+x, +z) from NPC is NE of NPC; NPC
// flees SW (delta (-1, -1)), checking WALL_SOUTH|WALL_WEST on the candidate
// tile. Fallback-axis comments: "NPC fallback X" means the fallback
// waypoint uses the NPC's x and the candidate's z (keeps X fixed).
func pickEscapeDirection(npcX, npcZ, targetX, targetZ int) escapeDirection {
	switch {
	case targetX >= npcX && targetZ >= npcZ:
		// Target NE of NPC → flee SW. TS: direction = SOUTH_WEST.
		// fallbackUseNpcX=false → axis fallback moves along X (mx, n.z).
		return escapeDirection{dx: -1, dz: -1,
			wallFlags:       collision.FlagWallSouth | collision.FlagWallWest,
			fallbackUseNpcX: false}
	case targetX >= npcX && targetZ < npcZ:
		// Target SE of NPC → flee NW. TS: direction = NORTH_WEST.
		// fallbackUseNpcX=true → axis fallback moves along Z (n.x, mz).
		return escapeDirection{dx: -1, dz: +1,
			wallFlags:       collision.FlagWallNorth | collision.FlagWallWest,
			fallbackUseNpcX: true}
	case targetX < npcX && targetZ >= npcZ:
		// Target NW of NPC → flee SE. TS: direction = SOUTH_EAST.
		// fallbackUseNpcX=false → axis fallback moves along X (mx, n.z).
		return escapeDirection{dx: +1, dz: -1,
			wallFlags:       collision.FlagWallSouth | collision.FlagWallEast,
			fallbackUseNpcX: false}
	default:
		// Target SW of NPC → flee NE. TS: direction = NORTH_EAST.
		// fallbackUseNpcX=true → axis fallback moves along Z (n.x, mz).
		return escapeDirection{dx: +1, dz: +1,
			wallFlags:       collision.FlagWallNorth | collision.FlagWallEast,
			fallbackUseNpcX: true}
	}
}

// playerEscapeMode — TS Npc.ts:746-799. Each tick:
//
//  1. Type-guard on *Player.
//  2. Abandon if SW-distance to target > 25.
//  3. Pick flee quadrant → candidate tile.
//  4. If the candidate tile's wall flags block the flee direction,
//     resetDefaults (can't move there; give up).
//  5. If the candidate is still within the NPC's MaxRange of its start,
//     queue a diagonal waypoint at (mx, mz).
//  6. Otherwise fall back to a single-axis waypoint: NE/NW → (n.x, mz);
//     SE/SW → (mx, n.z). This is the "walk along other axis" branch.
//
// Step 4's wall check requires a wired gamemap. When s.gamemap is nil
// (test fixtures that don't seed collision data), the wall check is
// skipped — same convention as NAI-12's inApproachDistance LoS short-circuit.
func (n *Npc) playerEscapeMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		if s != nil && s.log != nil {
			s.log.Warn("playerEscapeMode: non-Player target",
				"nid", n.nid, "targetOp", n.targetOp)
		}
		return
	}

	tx, tz, _ := p.Coords()

	// TS :751-754 — abandon if already > 25 tiles SW-distance.
	if coordgrid.DistanceToSW(n.x, n.z, tx, tz) > 25 {
		n.resetDefaults()
		return
	}

	// TS :756-770 — quadrant pick + flee-direction deltas.
	dir := pickEscapeDirection(n.x, n.z, tx, tz)
	mx := n.x + dir.dx
	mz := n.z + dir.dz

	// TS :775-778 — wall-flag check. Skip when gamemap is nil (test fixture).
	if s != nil && s.gamemap != nil &&
		s.gamemap.Pathfinder.Flags.IsFlagged(mx, mz, n.level, dir.wallFlags) {
		n.resetDefaults()
		return
	}

	// TS :780-790 — within-maxrange diagonal waypoint.
	if coordgrid.DistanceToSW(mx, mz, n.startX, n.startZ) < int(n.typ.MaxRange) {
		n.queueWaypoint(mx, mz)
		n.updateMovement(s)
		return
	}

	// TS :793-797 — axis fallback.
	if dir.fallbackUseNpcX {
		n.queueWaypoint(n.x, mz)
	} else {
		n.queueWaypoint(mx, n.z)
	}
	n.updateMovement(s)
}
```

Update `npc_player_modes.go`'s import block to include `coordgrid` and `collision`. The file starts with no imports today (Task 3 created it with only a package-level comment); replace the header with:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)
```

- [ ] **Step 6.7: Wire dispatch for PLAYERESCAPE + remove DEVIATION comment**

Modify `modules/world/npc_interaction.go`. Update the targeted-mode switch (from Task 5) to the final form:

```go
	// Targeted-mode dispatch.
	switch n.targetOp {
	case objtype.NPCModePlayerEscape:
		n.playerEscapeMode(s)
	case objtype.NPCModePlayerFollow:
		n.playerFollowMode(s)
	case objtype.NPCModePlayerFace:
		n.playerFaceMode(s)
	case objtype.NPCModePlayerFaceClose:
		n.playerFaceCloseMode(s)
	default:
		n.aiMode(s)
	}
```

Also in `modules/world/npc_interaction.go`, update the `processMovementInteraction` doc comment to remove the DEVIATION block at the NAI-11 lines 141-143. The final doc comment should read:

```go
// processMovementInteraction is the NPC's per-tick movement + interaction
// dispatcher. Replaces the inline wander/patrol/advanceWaypoint block
// that NAI-2..NAI-10 kept in npc_ai.go (the block is collapsed to a
// single call by Task 30). Mirrors TS Npc.processMovementInteraction
// at Engine-TS/.../Npc.ts:562-603.
//
// Dispatch order matches TS:
//  1. delayed / dead bail.
//  2. Last-tick coord bookkeeping + tele flag reset.
//  3. Null-targetOp failsafe → defaultMode.
//  4. Targetless modes (None / Wander / Patrol).
//  5. Targeted-mode prelude (target-nil or validateTarget-fail → resetDefaults).
//  6. Targeted-mode dispatch: PLAYER* modes → dedicated methods (NAI-13);
//     everything else routes to aiMode.
```

- [ ] **Step 6.8: Run the escape tests — verify all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerEscape" -v`
Expected: 9 PASS (4 quadrant + 1 abandon + 1 wall + 1 diagonal + 2 fallback + 1 non-Player guard = 10; double-check the count).

- [ ] **Step 6.9: Write + run the dispatch-routing test**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestProcessMovementInteractionDispatchPlayerEscape — NAI-13 Task 6.
// Target within retreat-maxrange (so validateTarget passes) but past the
// 25-tile abandon gate (so playerEscapeMode's first check fires and
// resetDefaults). Proves the dispatch switch routes to playerEscapeMode
// (not the fallback resetDefaults stub which would also nil target but
// wouldn't require the specific geometry).
//
// Math: NPC at (3100, 3100) with startXZ = (3100, 3100) and MaxRange = 30.
// Player at (3127, 3100): dx = 27. Retreat maxrange accepts maxAxis <=
// maxrange+1 = 31, so validateTarget passes. NPC-to-player SW-distance
// = 27 > 25, so playerEscapeMode's abandon gate fires.
func TestProcessMovementInteractionDispatchPlayerEscape(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3100, 3100
	n.startX, n.startZ = 3100, 3100
	n.typ.MaxRange = 30
	p.x, p.z = 3127, 3100
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerEscape, 0)

	n.processMovementInteraction(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (playerEscapeMode abandon-gate should fire)", n.target)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessMovementInteractionDispatchPlayerEscape" -v`
Expected: PASS.

- [ ] **Step 6.10: Run full package — verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 6.11: Commit**

```bash
git add modules/world/npc_player_modes.go modules/world/npc_player_modes_test.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-13 add playerEscapeMode + final dispatch case

Port TS Npc.ts:746-799: quadrant-directed flee with wall-flag check,
MaxRange boundary, and single-axis fallback. Dispatch switch now
routes all four PLAYER* modes to their dedicated methods; the NAI-11
resetDefaults stub is fully replaced.

Preserves the five TS quirks exactly:
- SW-distance 25 abandon gate (not Chebyshev);
- quadrant tie-breaks on >= / < signs;
- direction-pair wall flags per quadrant;
- within-maxrange diagonal waypoint vs. axis-fallback split;
- N-variant fallback on Z axis; S-variant on X.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: NAI-13 close — memory update + final verification sweep

Update the NAI-11 deferral in `nai_followups.md` to "Resolved 2026-04-23 (NAI-13)" and run the final verification sweep.

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

- [ ] **Step 7.1: Update the memory file**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`.

Find the "From NAI-11 (2026-04-22) → Deferred: PLAYER\* mode implementations" section. Prepend a **Resolved 2026-04-23 (NAI-13)** block before the existing body, keeping the original text below a `---` rule for historical context. The new block should read:

```
### Deferred: PLAYER* mode implementations

**Resolved 2026-04-23 (NAI-13)** in commits for Tasks 1–6 of
`docs/superpowers/plans/2026-04-23-nai-13-player-modes.md`. All four
PLAYER* modes (PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE /
PLAYERFACECLOSE) are ported from TS Npc.ts:746-830. `n.entitymask` is
wired at NewNpc (mirroring TS PathingEntity.ts:107), making
SetInteraction's `n.masks |= n.entitymask` line functional (previously
a no-op). `targetWithinMaxRange` gained two branches: PLAYERFOLLOW
early-return (TS :633-635) and PLAYERESCAPE retreat-maxrange with
corner-removal quirk (TS :657-673). The NAI-11-memorialized flipped
assertion in `TestNpcTurnHuntAndConsumeSetsTarget` was restored to its
original "target is the hunted NPC" shape. See
`docs/superpowers/specs/2026-04-23-nai-13-player-modes-design.md`.

Tracked deviations (all inherited from existing NAI-11/NAI-12
deferrals, no new):

- Naive-only `pathToTarget` in PLAYERFOLLOW (SMART branch still
  deferred from NAI-11).
- Size-approximated `DistanceTo(1,1,1,1)` in PLAYERFACECLOSE (inherits
  NAI-12's size-aware LoS deferral).
- `log-and-return` on non-Player type-guard instead of TS's throw
  (Go tick-loop doesn't have throw-and-recover scope).

---

_Original deferral body (preserved for historical context):_
```

Then leave the existing deferral text below the `---`.

- [ ] **Step 7.2: Verify the spec file references match reality**

Run the following greps to ensure no stale references remain:

```bash
grep -n "PLAYER\* modes is deferred\|PLAYER\*-mode is deferred\|when PLAYER\* modes are implemented\|DEVIATION: PLAYERESCAPE" modules/world/*.go
```

Expected: no output (all stale references scrubbed). If anything remains, tighten that reference in the same task commit.

- [ ] **Step 7.3: Final verification sweep — full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all green across the entire module.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: all green under the race detector.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 7.4: Confirm commit shape matches NAI cadence**

Run: `git log --oneline -10`
Expected: six NAI-13 feature commits in order (Task 1 through Task 6), then an NAI-13 close chore commit we're about to create.

- [ ] **Step 7.5: Create the NAI-13 close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(nai): NAI-13 closed — PLAYER* modes + entitymask plumbing

All six tasks of the NAI-13 plan landed. Four PLAYER* NPC modes
(ESCAPE / FOLLOW / FACE / FACECLOSE) are live; entitymask is wired at
NPC construction; targetWithinMaxRange has the PLAYERFOLLOW and
PLAYERESCAPE branches; and the NAI-11 flipped test assertion is
restored. See docs/superpowers/specs/2026-04-23-nai-13-player-modes-design.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7.6: Final summary**

Print: "NAI-13 closed. 7 commits (Task 1..6 + close). Memory updated. All tests green. Tracked deviations folded into NAI-11/NAI-12 existing deferrals; no new deferrals introduced."
