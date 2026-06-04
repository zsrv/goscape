# NAI-14 — `faceEntity` Clear-on-Reset + Player `entitymask` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the last visible-fidelity gaps in the NPC face-entity mask
lifecycle by porting TS `Npc.ts:407-408`, `:415`, and
`PathingEntity.ts:611-614`; wire `Player.entitymask = rsbuf.MaskFaceEntity`
at construction to retire the latent-no-op parallel of NAI-13's NPC-side
fix.

**Architecture:** Surgical port. Three inline-extensions in existing
methods (`resetDefaults`, `clearInteraction`, `ResetMasks`) + one struct-
literal line in `NewPlayer`. Preserves NAI-11's deliberate "stripped-down
resetDefaults" shape — `apRange`/`apRangeCalled`/`targetSubject`
non-clearing stays as a tracked deviation. No new files; no new types;
no new helpers.

**Tech Stack:** Go 1.26+. Existing `pkg/rsbuf` constants
(`NpcMaskFaceEntity = 0x4`, `MaskFaceEntity = 0x4`), existing
`modules/world` NPC/Player infrastructure. Tests use existing fixtures:
`newTestNpc(t)`, `newTestPlayer(t)`, `discardLogger()`.

**Spec:** `docs/superpowers/specs/2026-04-23-nai-14-face-entity-clearing-design.md`

---

## Task 1: `resetDefaults` + `clearInteraction` faceEntity clearing

Ports TS `Npc.ts:415` (`faceEntity = -1` in `resetDefaults`) and
`Npc.ts:407-408` (`faceEntity = -1` + `masks |= FACE_ENTITY` in
`clearInteraction`). Two commits within this task.

### Part A — `resetDefaults` faceEntity clear

**Files:**
- Modify: `modules/world/npc_interaction.go:32-42` — add `n.faceEntity = -1`; rewrite doc comment
- Modify: `modules/world/npc_interaction_test.go:738-769` — flip `n.faceEntity` assertion from 99 to -1
- Modify: `modules/world/npc_masks_test.go` — add new `TestNpcResetDefaultsClearsFaceEntity`

- [ ] **Step 1: Flip the existing test's faceEntity assertion**

In `modules/world/npc_interaction_test.go`, locate
`TestNpcResetDefaultsClearsTargetKeepsOtherState` (starts line 738).
Change the block at approximately lines 756-759 from:

```go
	// These stay untouched — next SetInteraction call will overwrite.
	if n.faceEntity != 99 {
		t.Errorf("faceEntity: got %d, want 99 (resetDefaults must not clear)", n.faceEntity)
	}
```

to:

```go
	// NAI-14: resetDefaults now clears faceEntity per TS Npc.ts:415.
	// apRange/apRangeCalled/targetSubject deliberately stay untouched
	// (NAI-11 stripped shape — next SetInteraction call overwrites).
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (resetDefaults should clear per TS Npc.ts:415)", n.faceEntity)
	}
```

Leave every other block in the test unchanged. The `n.masks != 0xff`
assertion stays valid — `NpcMaskFaceEntity = 0x4` ORs into 0xff without
changing the value.

- [ ] **Step 2: Add the fresh-assert companion test**

In `modules/world/npc_masks_test.go`, append at the end of the file
(after the last existing test):

```go
// TestNpcResetDefaultsClearsFaceEntity — NAI-14 Task 1.
// Named companion for the faceEntity-clear half of resetDefaults that
// lands with NAI-14. Separated from TestResetDefaultsEmitsEntityMask
// (NAI-13 mask-bit assertion) and from
// TestNpcResetDefaultsClearsTargetKeepsOtherState (the regression guard
// for the stripped NAI-11 shape). Mirrors TS Npc.ts:415:
// `this.faceEntity = -1;`.
func TestNpcResetDefaultsClearsFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	n.faceEntity = 42
	n.resetDefaults()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (resetDefaults should clear per TS Npc.ts:415)", n.faceEntity)
	}
}
```

- [ ] **Step 3: Run both tests to verify they fail**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcResetDefaultsClearsTargetKeepsOtherState|TestNpcResetDefaultsClearsFaceEntity' -v
```

Expected: **both FAIL** with `faceEntity: got 42, want -1` (the fresh
test) and `faceEntity: got 99, want -1` (the flipped test).

- [ ] **Step 4: Implement the `resetDefaults` change**

In `modules/world/npc_interaction.go`, the current method (around
lines 38-42) reads:

```go
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
	n.masks |= n.entitymask
}
```

Change it to:

```go
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
	n.faceEntity = -1
	n.masks |= n.entitymask
}
```

Also rewrite the doc comment at lines 32-37 (currently ending
with "... INTENTIONALLY does NOT clear apRange, apRangeCalled,
faceEntity, or the rest of masks — those are overwritten only by the
next SetInteraction call.") to:

```go
// resetDefaults clears target/targetOp to defaultMode baseline, clears
// faceEntity, and emits the faceEntity mask bit. Matches TS
// Npc.resetDefaults at Engine-TS/.../Npc.ts:411-425 — specifically the
// `faceEntity = -1` at :415 and `this.masks |= this.entitymask` at :416.
//
// INTENTIONALLY does NOT clear apRange, apRangeCalled, or targetSubject
// — those are overwritten only by the next SetInteraction call. This is
// a deliberate NAI-11-era deviation from TS resetDefaults, which delegates
// through clearInteraction; Go keeps the flat shape as a tracked deviation
// (see docs/superpowers/specs/2026-04-23-nai-14-face-entity-clearing-design.md).
```

- [ ] **Step 5: Run both tests to verify they pass**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcResetDefaultsClearsTargetKeepsOtherState|TestNpcResetDefaultsClearsFaceEntity' -v
```

Expected: **both PASS**.

- [ ] **Step 6: Run the full `modules/world` package to check for regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v
```

Expected: all tests pass. No previously-green test should have gone red.

- [ ] **Step 7: Commit Part A**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-14 Task 1a resetDefaults clears faceEntity

Mirrors TS Npc.ts:415. Flips
TestNpcResetDefaultsClearsTargetKeepsOtherState faceEntity assertion
from 99 to -1 and adds fresh-assert TestNpcResetDefaultsClearsFaceEntity.
Doc comment rewritten — apRange/apRangeCalled/targetSubject non-clearing
stays as NAI-11 tracked deviation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Part B — `clearInteraction` faceEntity clear + mask emit

**Files:**
- Modify: `modules/world/npc_interaction.go:44-53` — add two lines; rewrite doc comment
- Modify: `modules/world/npc_interaction_test.go:771-797` — add faceEntity + mask assertions
- Modify: `modules/world/npc_masks_test.go` — add new `TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity`

- [ ] **Step 1: Extend the existing test with faceEntity + mask assertions**

In `modules/world/npc_interaction_test.go`, locate
`TestNpcClearInteractionResetsState` (starts line 771). Current setup
lines 773-778:

```go
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.apRange = 5
	n.apRangeCalled = true
	n.targetSubject = npcTargetSubject{com: 42, typ: 1}
```

Add two lines before the call to seed state that proves the clear:

```go
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.apRange = 5
	n.apRangeCalled = true
	n.targetSubject = npcTargetSubject{com: 42, typ: 1}
	n.faceEntity = 42
	n.masks = 0
```

Then, after the existing `targetSubject` assertion (around line 794-796),
append two new assertion blocks before the closing brace of the test:

```go
	if n.targetSubject.com != -1 || n.targetSubject.typ != -1 {
		t.Errorf("targetSubject: got %+v, want {-1,-1}", n.targetSubject)
	}
	// NAI-14: clearInteraction now clears faceEntity and emits the
	// entitymask bit per TS Npc.ts:407-408.
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (clearInteraction should clear per TS Npc.ts:407)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (clearInteraction should emit per TS Npc.ts:408)")
	}
}
```

Verify the test file already imports `"github.com/zsrv/goscape/pkg/rsbuf"`
— other tests in the file use `rsbuf.*`, so the import should exist. If
not, add it.

- [ ] **Step 2: Add the fresh-assert companion test**

In `modules/world/npc_masks_test.go`, append after the test added in
Part A:

```go
// TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity — NAI-14 Task 1.
// Named companion for the faceEntity-clear + mask-emit pair that
// clearInteraction gains in NAI-14. Mirrors TS Npc.ts:407-408:
// `this.faceEntity = -1; this.masks |= NpcInfoProt.FACE_ENTITY;`.
// Separated from TestNpcClearInteractionResetsState (the full-state
// regression guard) so the TS-line mapping is explicit in one test name.
func TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	n.faceEntity = 42
	n.masks = 0
	n.clearInteraction()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (clearInteraction should clear per TS Npc.ts:407)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (clearInteraction should emit per TS Npc.ts:408)")
	}
}
```

- [ ] **Step 3: Run both tests to verify they fail**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcClearInteractionResetsState|TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity' -v
```

Expected: **both FAIL** — `faceEntity: got 42, want -1` and
`masks & NpcMaskFaceEntity: got 0, want nonzero`.

- [ ] **Step 4: Implement the `clearInteraction` change**

In `modules/world/npc_interaction.go`, the current method (around lines
47-53) reads:

```go
func (n *Npc) clearInteraction() {
	n.target = nil
	n.targetOp = -1
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
}
```

Change it to:

```go
func (n *Npc) clearInteraction() {
	n.target = nil
	n.targetOp = -1
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
	n.faceEntity = -1
	n.masks |= n.entitymask
}
```

Also rewrite the doc comment at lines 44-46 (currently "... Does NOT
touch faceEntity/masks — those are cleared by the masks frame-pass, not
here.") to:

```go
// clearInteraction resets interaction state to idle: target, targetOp,
// apRange, apRangeCalled, targetSubject, faceEntity. Emits the
// faceEntity mask bit so clients see the NPC stop facing its old target.
// Matches TS Npc.clearInteraction at Engine-TS/.../Npc.ts:402-409,
// which overrides PathingEntity.clearInteraction (PathingEntity.ts:550-556)
// with the `faceEntity = -1` and `masks |= FACE_ENTITY` tail at :407-408.
```

- [ ] **Step 5: Run both tests to verify they pass**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcClearInteractionResetsState|TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity' -v
```

Expected: **both PASS**.

- [ ] **Step 6: Run the full `modules/world` package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v
```

Expected: all tests pass. The 24+ `clearInteraction` call sites in
`npc_interaction_trigger.go` now also emit the mask bit — no existing
test should be asserting "mask bit NOT set after a trigger clear"
because that was previously a silent no-op. If any test fails, examine
its expectation and see whether it was accidentally codifying the
silent no-op.

- [ ] **Step 7: Commit Part B**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-14 Task 1b clearInteraction clears faceEntity + emits mask

Mirrors TS Npc.ts:407-408. Extends TestNpcClearInteractionResetsState
with faceEntity=-1 and masks|=NpcMaskFaceEntity assertions; adds
fresh-assert TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity.
Doc comment rewritten.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `ResetMasks` trailing defensive clear

Ports TS `PathingEntity.ts:611-614` — the trailing `if !target &&
faceEntity !== -1 { masks |= entitymask; faceEntity = -1 }` clear.
Adapted for Go's tick-end-cleanup timing (see spec § Error handling);
the mask bit survives into the next tick's info-pass.

**Files:**
- Modify: `modules/world/npc_masks.go:66-78` — add 4-line conditional + 2 comment lines
- Modify: `modules/world/npc_masks_test.go` — add 3 new tests

- [ ] **Step 1: Add three failing tests**

In `modules/world/npc_masks_test.go`, append after the tests added in
Task 1:

```go
// TestResetMasksTrailingClearFires — NAI-14 Task 2.
// When target is nil but faceEntity is still set, ResetMasks emits the
// entitymask bit and clears faceEntity. Mirrors TS
// PathingEntity.ts:611-614 with one-tick-lag deviation (Go's ResetMasks
// runs at tick end, so the mask bit is consumed by the next tick's
// info-pass).
func TestResetMasksTrailingClearFires(t *testing.T) {
	n := newTestNpc(1)
	n.target = nil
	n.faceEntity = 42
	n.masks = 0
	n.ResetMasks()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (trailing clear should run)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (trailing clear should emit)")
	}
}

// TestResetMasksTrailingClearSkippedWhenTargetPresent — NAI-14 Task 2.
// Quirk guard: trailing clear must not fire when target is non-nil
// (the NPC is still facing someone, by design).
func TestResetMasksTrailingClearSkippedWhenTargetPresent(t *testing.T) {
	n := newTestNpc(1)
	other := newTestNpc(2)
	n.target = other
	n.faceEntity = 42
	n.masks = 0
	n.ResetMasks()
	if n.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (trailing clear should be skipped — target present)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity != 0 {
		t.Error("masks & NpcMaskFaceEntity: got nonzero, want 0 (trailing clear should not emit — target present)")
	}
}

// TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne — NAI-14 Task 2.
// Quirk guard: trailing clear must not fire when faceEntity is already
// -1 (no pending clear to sync).
func TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne(t *testing.T) {
	n := newTestNpc(1)
	n.target = nil
	n.faceEntity = -1
	n.masks = 0
	n.ResetMasks()
	if n.masks != 0 {
		t.Errorf("masks: got 0x%x, want 0 (trailing clear should be skipped — faceEntity already -1)", n.masks)
	}
}
```

- [ ] **Step 2: Run the tests to verify the Fires test fails**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestResetMasksTrailingClear' -v
```

Expected:
- `TestResetMasksTrailingClearFires` — **FAIL** (`faceEntity: got 42, want -1`).
- `TestResetMasksTrailingClearSkippedWhenTargetPresent` — **PASS** (ResetMasks currently doesn't touch faceEntity; 42 stays 42; mask bit unset).
- `TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` — **PASS** (masks is reset to 0 by the existing `n.masks = 0` line; 0 stays 0).

The two passing tests are the guard tests — they pass vacuously
pre-impl, then must continue passing post-impl to prove the guard
conditions work.

- [ ] **Step 3: Implement the trailing clear in `ResetMasks`**

In `modules/world/npc_masks.go`, the current method (lines 66-78) reads:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceEntity, faceSquareX/Z, changeTypeID, curHP, baseHP) are
// retained across ticks — S6d promoted curHP/baseHP from ephemeral to
// persistent. damageAmt / damageType remain per-tick hitsplat payload.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
}
```

Change it to:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceSquareX/Z, changeTypeID, curHP, baseHP) are retained across
// ticks — S6d promoted curHP/baseHP from ephemeral to persistent.
// damageAmt / damageType remain per-tick hitsplat payload. faceEntity is
// retained unless the trailing-clear condition below fires.
//
// The trailing clear mirrors TS PathingEntity.ts:611-614: when the NPC
// has no target but still has a lingering faceEntity, the entitymask
// bit is re-emitted and faceEntity is snapped to -1 so the client
// receives the "stopped facing" update. Go's ResetMasks runs at tick
// end (tick.go processCleanup), so the mask bit survives into the next
// tick's info-pass — a one-tick lag vs TS which fires at tick start.
// Accepted deviation; all "official" target-clear paths
// (resetDefaults, clearInteraction) emit the mask same-tick, so this
// conditional is a defensive net for stray n.target = nil assignments.
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

Note the doc comment changes: removed `faceEntity` from the "retained"
list, added the trailing-clear semantics block.

- [ ] **Step 4: Run the three tests to verify they all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestResetMasksTrailingClear' -v
```

Expected: **all three PASS**.

- [ ] **Step 5: Run the full `modules/world` package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v
```

Expected: all tests pass. In particular,
`TestNpcResetMasksClearsEphemerals` at `npc_test.go:90-95` and
`TestNpcDamagePersistsAcrossResetMasks` / `TestNpcBaseHPPersistsAcrossResetMasks`
in `npc_masks_test.go` must still pass — the trailing clear is gated
and does not fire on their fixtures (neither sets `target = nil` AND
`faceEntity != -1`).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_masks.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-14 Task 2 ResetMasks trailing faceEntity clear

Mirrors TS PathingEntity.ts:611-614 with one-tick-lag deviation
(Go's ResetMasks runs at tick end vs TS's tick-start timing; mask
bit survives into next tick's info-pass). Defensive net for stray
n.target = nil assignments — the common paths (resetDefaults,
clearInteraction) emit same-tick post-NAI-14 Task 1. Three new tests:
positive fire + two quirk guards (target present, faceEntity already
-1).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Player `entitymask` wiring + close

Wires `Player.entitymask = rsbuf.MaskFaceEntity` at construction —
mirrors TS `PathingEntity.ts:107`. Assignment is currently absent
(field declared at `player.go:115` but never assigned). Closes NAI-11
"Deferred: Npc.entitymask mask plumbing" memory entry and the NAI-13
close-note's mask-plumbing tail.

**Files:**
- Modify: `modules/world/player.go:345-355` — add one line in Player struct literal
- Modify: `modules/world/player_masks_test.go` — add new `TestNewPlayerSetsEntityMaskToMaskFaceEntity`
- Modify (outside repo): `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — mark entries Resolved

- [ ] **Step 1: Add failing test**

In `modules/world/player_masks_test.go`, append at the end of the file:

```go
// TestNewPlayerSetsEntityMaskToMaskFaceEntity — NAI-14 Task 3.
// Mirrors TS PathingEntity.ts:107 where `this.entitymask = entitymask`
// is set at construction. For Player, this is rsbuf.MaskFaceEntity.
// Parallel of NAI-13's TestNewNpcSetsEntityMaskToFaceEntity on the NPC
// side — closes the Player-side latent-no-op where p.entitymask was
// declared at player.go:115 but never assigned.
//
// No `p.masks |= p.entitymask` sites exist today on the Player side
// (grep-verified), so this assignment is structural future-proofing.
// Future Player-interaction port sub-specs that need the face-entity
// mask bit should use `p.entitymask` (not hardcode `rsbuf.MaskFaceEntity`).
func TestNewPlayerSetsEntityMaskToMaskFaceEntity(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.entitymask != rsbuf.MaskFaceEntity {
		t.Errorf("entitymask: got %d, want %d (MaskFaceEntity)", p.entitymask, rsbuf.MaskFaceEntity)
	}
}
```

Verify the file already imports `"github.com/zsrv/goscape/pkg/rsbuf"`
(line 6 in the file). `newTestPlayer(t)` is the existing fixture used
by other tests in this file.

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewPlayerSetsEntityMaskToMaskFaceEntity' -v
```

Expected: **FAIL** — `entitymask: got 0, want 4 (MaskFaceEntity)`.

- [ ] **Step 3: Implement the `NewPlayer` change**

In `modules/world/player.go`, locate the `NewPlayer` struct literal
around lines 345-355:

```go
		exactStartX:    -1,
		exactStartZ:    -1,
		exactEndX:      -1,
		exactEndZ:      -1,
		exactBegin:     -1,
		exactFinish:    -1,
		exactDir:       -1,
		faceEntity:     -1,
		faceSquareX:    -1,
		faceSquareZ:    -1,
	}
```

Insert `entitymask: rsbuf.MaskFaceEntity,` after the `faceEntity: -1,`
line:

```go
		exactStartX:    -1,
		exactStartZ:    -1,
		exactEndX:      -1,
		exactEndZ:      -1,
		exactBegin:     -1,
		exactFinish:    -1,
		exactDir:       -1,
		faceEntity:     -1,
		entitymask:     rsbuf.MaskFaceEntity,
		faceSquareX:    -1,
		faceSquareZ:    -1,
	}
```

Verify `player.go` already imports `"github.com/zsrv/goscape/pkg/rsbuf"`
(it does — the file's top imports include it for other mask constants
like `MaskAnim`, `MaskChat` used elsewhere in the package; but confirm
in this file specifically, and add the import if missing).

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewPlayerSetsEntityMaskToMaskFaceEntity' -v
```

Expected: **PASS**.

- [ ] **Step 5: Full verification sweep**

Run the entire test suite with race detector:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all packages green. No data-race warnings (the changes are
single-goroutine ephemeral state; no shared-memory concern).

Also grep for stale doc-comment strings that should have been removed
in Task 1:

```bash
grep -rn 'INTENTIONALLY does NOT clear.*faceEntity' modules/world/
grep -rn 'Does NOT touch faceEntity/masks' modules/world/
```

Expected: both return no matches (the doc-comment rewrites in Task 1
removed these strings).

- [ ] **Step 6: Commit the Player wiring**

```bash
git add modules/world/player.go modules/world/player_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-14 Task 3 wire Player.entitymask at construction

Mirrors TS PathingEntity.ts:107. Closes the Player-side parallel of
NAI-13's NPC-side fix — p.entitymask was declared at player.go:115
but never assigned, a latent-no-op mirror of the NPC pre-NAI-13 state.
No `p.masks |= p.entitymask` sites exist today (grep-verified); this
assignment is structural future-proofing for Player-interaction port
sub-specs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Update `nai_followups.md` memory entries**

Open `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`.

**Update A:** Locate the NAI-11 § "Deferred: Npc.entitymask mask
plumbing" entry (around lines 389-396). Prepend a Resolved block before
the existing body so the format matches the other resolved entries in
the file:

```markdown
### Deferred: Npc.entitymask mask plumbing

**Resolved 2026-04-23 (NAI-14).** NAI-13 wired the NPC-construction
half (`n.entitymask = rsbuf.NpcMaskFaceEntity` in NewNpc). NAI-14
closes the remaining pieces: `faceEntity = -1` + mask emission in
`resetDefaults` (TS Npc.ts:415) and `clearInteraction` (TS
Npc.ts:407-408), defensive trailing clear in `ResetMasks` (TS
PathingEntity.ts:611-614), and the Player-side construction assignment
(`p.entitymask = rsbuf.MaskFaceEntity` in NewPlayer — mirrors TS
PathingEntity.ts:107). Audit finding: sites 534/540 tracked in NAI-13's
close note were already ported at `npc_interaction.go:604-611`; only
site 612 needed a Go-side port. See
`docs/superpowers/specs/2026-04-23-nai-14-face-entity-clearing-design.md`.

---

_Original deferral body (preserved for historical context):_

[existing body stays here, unchanged]
```

**Update B:** Locate the NAI-11 § "Resolved: PLAYER* mode implementations"
(around lines 271-305). Its "Tracked deviations" list at the bottom does
not currently reference the NAI-14 mask-plumbing tail — add one line:

Before:
```markdown
Tracked deviations (all inherited from existing NAI-11/NAI-12
deferrals; no new NAI-13 deferrals introduced):
```

After:
```markdown
Tracked deviations (all inherited from existing NAI-11/NAI-12
deferrals; no new NAI-13 deferrals introduced; mask-plumbing tail —
PathingEntity.ts site 612, faceEntity clear in resetDefaults/
clearInteraction, and Player.entitymask wiring — closed by NAI-14
on 2026-04-23):
```

Memory updates don't commit to the goscape repo — they live outside
it in the user's home directory.

- [ ] **Step 8: Final close commit**

A marker commit denoting NAI-14 closure, matching the pattern
`chore(nai): NAI-N closed — ...` from NAI-13's close commit
(`18b99d0 chore(nai): NAI-13 closed — PLAYER* modes + entitymask plumbing`).
This commit has no file changes — it's a series-marker.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai): NAI-14 closed — faceEntity clearing + Player entitymask

Closes TS Npc.ts:407-408, :415, and PathingEntity.ts:611-614 on the
NPC side; wires Player.entitymask at construction. Audit finding:
PathingEntity.ts sites 534/540 were already ported. Preserves NAI-11's
deliberate "stripped-down resetDefaults" shape (apRange/apRangeCalled/
targetSubject stay non-cleared).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 9: Verify the final state**

```bash
git log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected:
- `git log` shows the four NAI-14 commits on top (1a feat, 1b feat, 2 feat, 3 feat, close chore = 5 commits total if counted with close).
- Full test suite green across all packages.

---

## Post-implementation cross-check

Verify spec coverage item-by-item (match spec's Goal §1-§7 against
tasks):

| Spec Goal | Task / Step |
|---|---|
| §1 `resetDefaults` adds `n.faceEntity = -1` | Task 1 Part A Step 4 |
| §2 `clearInteraction` adds `n.faceEntity = -1` + `n.masks \|= n.entitymask` | Task 1 Part B Step 4 |
| §3 `ResetMasks` adds conditional trailing clear | Task 2 Step 3 |
| §4 `NewPlayer` assigns `p.entitymask = rsbuf.MaskFaceEntity` | Task 3 Step 3 |
| §5 Tests flipped (2) | Task 1 Part A Step 1 + Part B Step 1 |
| §6 New tests added (6) | Task 1 Part A Step 2 + Part B Step 2 + Task 2 Step 1 (×3) + Task 3 Step 1 |
| §7 Memory entry resolution | Task 3 Step 7 |

Test inventory (8 tests = 2 flipped + 6 new):
- Flipped (2): `TestNpcResetDefaultsClearsTargetKeepsOtherState` (Task 1 Part A Step 1), `TestNpcClearInteractionResetsState` (Task 1 Part B Step 1).
- New (6): `TestNpcResetDefaultsClearsFaceEntity` (Task 1 Part A Step 2), `TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity` (Task 1 Part B Step 2), `TestResetMasksTrailingClearFires` + `TestResetMasksTrailingClearSkippedWhenTargetPresent` + `TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` (Task 2 Step 1), `TestNewPlayerSetsEntityMaskToMaskFaceEntity` (Task 3 Step 1).
