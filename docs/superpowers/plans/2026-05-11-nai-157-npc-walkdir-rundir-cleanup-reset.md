# NAI-157 — NPC walkDir/runDir per-tick cleanup reset (compressed spec+plan)

**Cadence:** compressed (≤~15 LOC behavioral change + 2 new test pins). One Sonnet implementer. No formal code review.

**Tech stack:** Go 1.26+, project root `/home/owner/Code/github.com/zsrv/goscape`.

## 1. Motivation

A user-reported visual bug: NPCs that the player talks to (or that are otherwise script-delayed) appear to keep walking in their previous direction for several ticks — visually clipping through walls — and then a follow-up dialog click sends the player toward the drifted client-side position before pathfinding back to the actual server position.

### Root cause (verified against TS source and HEAD)

`processCleanup` does not reset `n.walkDir` / `n.runDir` for NPCs.

In TS:

- `World.processCleanup` calls `npc.resetEntity(false)` for every NPC (`World.ts:1150-1153`).
- `Npc.resetEntity(false)` calls `super.resetPathingEntity()` (`Npc.ts:315`).
- `PathingEntity.resetPathingEntity()` unconditionally resets `this.walkDir = -1; this.runDir = -1` (`PathingEntity.ts:579-580`).

In goscape:

- `processCleanup` calls `n.ResetMasks()` for every NPC (`modules/world/tick.go:654-656`).
- `Npc.ResetMasks()` does NOT touch `walkDir` or `runDir` (`modules/world/npc_masks.go:207-219`).
- The only resets of `n.walkDir` / `n.runDir` to `-1` live inside `(*Npc).updateMovement` (`modules/world/npc_interaction.go:280-337`).
- `updateMovement` is reached only via `processMovementInteraction`, which has an early-return for `n.delayed || n.dead` at `npc_interaction.go:159-161`.

Consequence: when an NPC walks at tick N (setting `walkDir=SOUTH`) and is then script-delayed at tick N+1 (`SetDelayed` at `npc.go:337-340` — fires from queue/timer/hunt scripts via the `ActiveNpc` adapter), `processMovementInteraction` early-returns. `updateMovement` never runs. `processInfo` (`tick.go:558-572`) pushes the stale `walkDir`/`runDir` to rsbuf, and `NpcInfo.writeNpcs` (`pkg/rsbuf/npcinfo.go:118-178`) encodes a walk/run delta. The Java client (which does not perform collision validation on movement deltas) animates one more tile of motion per stale tick, accumulating drift across the delay window.

The Player path does not have this bug: `(*Player).resolveMovement` (`modules/world/movement.go:40-117`) is called unconditionally from `processPathing` and always overwrites `walkDir`/`runDir` to a fresh value or `-1`.

### Symptom-to-mechanism cross-walk (user log snippet)

| Observation | Mechanism |
|---|---|
| NPC keeps moving one direction post-stop, clipping walls | Stale `walkDir`/`runDir` re-encoded each delayed tick; client trusts dir codes without collision check |
| Player runs to perceived NPC tile first | `MOVE_OPCLICK` packet carries client-derived waypoints to *client-side* (drifted) NPC position; server queues them verbatim (`modules/world/movement.go:175-195`) |
| Player then runs back to actual NPC | `pathToPathingTarget` re-paths to server-truth NPC tile via `pathToTarget` after waypoints exhaust (`modules/world/interaction.go:768-811`) |

## 2. Risk

LOW.

- Behavior matches TS exactly (one canonical reference site).
- Player path already relies on equivalent per-tick `walkDir`/`runDir` overwriting.
- No new entry/exit points added; reset is purely additive at an existing cleanup site.
- All in-tree tests that read `n.walkDir`/`n.runDir` do so right after a movement step (within the same tick, before `processCleanup` could fire); none read the value across a tick boundary. (Verified: `rg "n\.walkDir|n\.runDir" modules/world/*_test.go` — all references are co-located with `updateMovement`/`stepOnce` calls or NPC ctor setup.)

## 3. Scope

### 3.1 Production patch — `modules/world/npc_masks.go`

Extend `(*Npc).ResetMasks()` to also reset `walkDir` and `runDir`. This is the minimal-diff option (one method, two fields). Doc-comment updated to cite the TS-fidelity rationale and the cleanup-pass timing.

Before (current at `modules/world/npc_masks.go:207-219`):

```go
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
	if n.target == nil && n.faceEntity != -1 {
		n.masks |= n.entitymask
		n.faceEntity = -1
	}
}
```

After:

```go
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
	// NAI-157: mirror TS PathingEntity.resetPathingEntity (Engine-TS
	// PathingEntity.ts:579-580). TS resets walkDir/runDir for every NPC
	// in processCleanup via Npc.resetEntity(false). Without an equivalent
	// reset here, a script-delayed NPC retains the previous tick's
	// walkDir/runDir — processMovementInteraction (npc_interaction.go:
	// 158-161) early-returns on n.delayed, so updateMovement never
	// re-resets these fields. processInfo (tick.go:558-572) then re-pushes
	// the stale dir into rsbuf, NpcInfo encodes a walk/run delta, and the
	// Java client visually drifts the NPC across the delay window.
	n.walkDir = -1
	n.runDir = -1
	if n.target == nil && n.faceEntity != -1 {
		n.masks |= n.entitymask
		n.faceEntity = -1
	}
}
```

Note: scope is intentionally limited to `walkDir` / `runDir`. TS `resetPathingEntity` resets a broader set (`moveSpeed`, `jump`, `tele`, `lastTickX/Z/Level`, `stepsTaken`, `interacted`, `apRangeCalled`, `exact*`, `animId/Delay`). Most of those fields either don't exist on `*Npc`, are reset elsewhere on the goscape path, or — like `tele` / `lastTickX/Z/Level` — are written inside `processMovementInteraction` after the `delayed||dead` early-return (`npc_interaction.go:163-164`). That broader alignment is out of scope for this user-visible bug fix; the same `delayed`-early-return mechanism makes `tele` / `lastTickX/Z/Level` similar candidates but is filed as a follow-up.

### 3.2 New test pin — `modules/world/npc_masks_test.go`

Add a unit test pinning that `ResetMasks` clears `walkDir`/`runDir`:

```go
// TestNpcResetMasksClearsWalkDirRunDir pins NAI-157: ResetMasks must
// reset walkDir and runDir so a script-delayed NPC's stale step dir
// does not leak into the next tick's NpcInfo writes. Mirrors TS
// PathingEntity.resetPathingEntity (PathingEntity.ts:579-580) reached
// via Npc.resetEntity(false) from World.processCleanup (World.ts:1152).
func TestNpcResetMasksClearsWalkDirRunDir(t *testing.T) {
	n := &Npc{walkDir: 4, runDir: 7}

	n.ResetMasks()

	if n.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1", n.walkDir)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1", n.runDir)
	}
}
```

### 3.3 New test pin — `modules/world/npc_masks_test.go`

End-to-end integration pin: an NPC that took a step on tick N, then becomes delayed on tick N+1, must NOT have stale `walkDir`/`runDir` when `processInfo` reads them.

```go
// TestNpcDelayedAfterStepClearsStaleWalkDir pins NAI-157 root-cause
// fix: when an NPC walks at tick N and is script-delayed at tick N+1,
// processCleanup's ResetMasks call at the end of tick N clears the
// step dir so tick N+1's processInfo sees walkDir == -1.
//
// Without the fix, processMovementInteraction early-returns on
// n.delayed (npc_interaction.go:159-161), updateMovement never runs,
// and the rsbuf encoder receives the stale dir — visible client-side
// as the NPC continuing to walk through walls.
func TestNpcDelayedAfterStepClearsStaleWalkDir(t *testing.T) {
	n := &Npc{}
	// Simulate a step having occurred earlier this tick.
	n.walkDir = 4
	n.runDir = 5

	// End-of-tick cleanup runs.
	n.ResetMasks()

	// Simulate the NPC being script-delayed for the next tick.
	n.delayed = true

	// Even though processMovementInteraction would early-return next
	// tick (and updateMovement therefore would not run), the dir state
	// is already cleared. processInfo would push -1 / -1.
	if n.walkDir != -1 || n.runDir != -1 {
		t.Fatalf("stale dir leaked past cleanup: walkDir=%d runDir=%d", n.walkDir, n.runDir)
	}
}
```

### 3.4 Existing tests — audit only, no inversions expected

`rg "ResetMasks" modules/world/` to confirm existing callers do not assert that `walkDir`/`runDir` are *non-`-1`* after `ResetMasks`. (Predicted result: no such assertion exists; ResetMasks is asserted on mask bits / damage / spotanim resets only.)

The implementer must run this grep at task start and report findings before editing. If a test does rely on stale dir survival past cleanup, escalate — it would be encoding the bug.

## 4. Out of scope (filed as follow-ups, not implemented here)

- **`tele`, `lastTickX/Z/Level` cleanup-pass alignment**: same `delayed`-early-return mechanism affects these. `tele` staleness self-heals via NpcInfo's remove-and-re-add path (`pkg/rsbuf/npcinfo.go:130`), so user-visibility is bounded. `lastTickX/Z/Level` is read by NPC info code and AI math; staleness across delay windows could produce subtle off-by-one regressions but is not the user's reported symptom. Track as **NAI-157-FU-PATHING-ENTITY-FULL-RESET**.
- **NPC `stepsTaken` reset**: `n.stepsTaken++` at `npc_interaction.go:363` is never reset to 0 anywhere. Player resets at `movement.go:46`. Same TS-fidelity gap, not user-reported. Track as **NAI-157-FU-NPC-STEPSTAKEN-RESET**.

## 5. Commit message

```
fix(world): NAI-157 — reset NPC walkDir/runDir at tick-end cleanup

processCleanup's n.ResetMasks() did not reset walkDir/runDir, diverging
from TS where World.processCleanup → npc.resetEntity(false) →
PathingEntity.resetPathingEntity() unconditionally resets both fields
(PathingEntity.ts:579-580).

In goscape the only resets live inside (*Npc).updateMovement
(npc_interaction.go:280-337), which is unreachable when
processMovementInteraction early-returns on n.delayed || n.dead
(npc_interaction.go:159-161). Script-delayed NPCs therefore retained
the previous tick's step direction, processInfo pushed it into rsbuf,
NpcInfo encoded a walk/run delta each delayed tick, and the Java client
visually drifted the NPC through walls.

Closes the user-reported bug where NPCs continued walking visually
during dialog/queue delays, and a follow-up OPNPC1 sent the player to
the drifted client-side position before pathToTarget re-paths to the
actual server NPC tile.

Closes memory: nai_followups.md (NAI-157 entry).
```

## 6. Definition of Done

1. `modules/world/npc_masks.go` `ResetMasks` patch applied with NAI-157 doc-comment.
2. Two new tests in `modules/world/npc_masks_test.go` pass:
   - `TestNpcResetMasksClearsWalkDirRunDir`
   - `TestNpcDelayedAfterStepClearsStaleWalkDir`
3. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "ResetMasks|StaleWalkDir"` passes.
4. Full module test suite green: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`.
5. Commit lands with the message above and `--no-gpg-sign`.
6. Two follow-ups appended to `nai_followups.md` under a new `## From NAI-157 (2026-05-11)` section.
