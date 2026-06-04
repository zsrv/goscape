# NAI-108 Player.ResetMasks audit + load-bearing trailing-clear — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the missing TS `PathingEntity.resetPathingEntity` trailing-clear to `Player.ResetMasks` (fixing user-reported "player keeps facing NPC after walking away"), retire the `NAI-72-N-RESETENTITY-PARTIAL` tracker, and ship the full TS↔goscape audit deliverable per spec option (II).

**Architecture:** Single bundle, one subagent dispatch on Sonnet (per `execution_mode_default.md`). TDD: 5 new tests fail → impl → green. Three discrete code edits (`player_masks.go`, `player.go`, `tick.go`) + one comment-update edit (`player_masks_test.go:91`) + (δ) verify-and-pin audit. End-of-bundle review on Sonnet per `superpowers_code_reviewer_model.md`.

**Tech Stack:** Go 1.26+ (per `go_version.md`); test runner is the in-tree `go test`; mask constants at `pkg/rsbuf/visibility.go` aliased through `modules/world/masks.go`.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-108-player-resetmasks-audit-design.md` (HEAD `aa67b18`).

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `modules/world/player_masks.go` | Per-tick mask-and-ephemeral reset for `*Player`. | **Modify** `ResetMasks` (lines 50-74): add chat-metadata reset (3 LOC), add trailing-clear block (4 LOC), expand doc-comment (γ-divergence labels with TS line citations). |
| `modules/world/player_masks_test.go` | Unit tests for `ResetMasks` + chat/face/spotanim setters. | **Modify** existing `TestResetMasksClearsEphemerals` doc-comment at line 91 (one-line update for conditional persistence). **Append** 5 new NAI-108 tests after the existing `TestNewPlayerSetsEntityMaskToMaskFaceEntity` at line 221. |
| `modules/world/player.go` | `*Player` struct + `newPlayer` factory + helpers. | **Modify** struct field declaration at line 196 — DELETE the dead `chatMessage []byte` field. |
| `modules/world/tick.go` | Per-tick orchestration (`processCleanup` runs `ResetMasks` for every player and NPC). | **Modify** comment at lines 526-531 — retire `NAI-72-N-RESETENTITY-PARTIAL` reference; restate post-NAI-108 deferred set. |

No new files. Every change is local to `modules/world/`. Audit-doc deliverable already lives in the spec (§3 audit table).

---

## Pre-flight checklist (controller, BEFORE dispatching the bundle)

Per `controller_preflight.md`, verify each plan-codified premise against HEAD before subagent dispatch. Run from a fresh shell:

- [ ] **Pre-flight 1**: `git rev-parse HEAD` → matches `aa67b18` (or fast-forward descendant of it).
- [ ] **Pre-flight 2**: `rg "func \(p \*Player\) ResetMasks" modules/world/player_masks.go` → 1 match at line 56.
- [ ] **Pre-flight 3**: `rg "chatMessage" modules/ pkg/` → exactly 2 matches: `modules/world/player.go:196` (declaration) + `modules/world/tick.go:530` (comment reference). Zero reads/writes elsewhere.
- [ ] **Pre-flight 4**: `rg "NAI-72-N-RESETENTITY-PARTIAL" modules/ pkg/` → exactly 1 match at `modules/world/tick.go:530-531` (comment).
- [ ] **Pre-flight 5**: `rg "func newTestPlayer\b" modules/world/` → match at `player_test.go:17` returning `(*Player, net.Conn)`.
- [ ] **Pre-flight 6**: `rg "func newTestNpc\b" modules/world/` → match used by NPC tests; signature `(id int)` per `npc_masks_test.go:238` invocation.
- [ ] **Pre-flight 7**: `rg "MaskFaceEntity\s*=" modules/world/masks.go pkg/rsbuf/` → constant exists at `modules/world/masks.go:8` (= 4) and `pkg/rsbuf/visibility.go:18` (= 0x4).
- [ ] **Pre-flight 8**: `rg "p\.target\b\s*=" modules/world/player_masks_test.go` → 0 matches (existing tests don't touch target; the new trailing-clear tests will be the first to do so in this file).
- [ ] **Pre-flight 9**: `rg "TestResolveMovementResetsStepsTaken" modules/world/movement_test.go` → 1 match at line 214 (already pins stepsTaken (δ) equivalence; cite, don't duplicate).
- [ ] **Pre-flight 10**: Verify spec audit table line numbers haven't drifted: `sed -n '56,74p' modules/world/player_masks.go` and `sed -n '180,210p' modules/world/npc_masks.go` — both bodies match the spec §2 verbatim block.

If any pre-flight fails, STOP and update the plan/spec before dispatching.

---

## Bundle 1 — Player.ResetMasks audit + load-bearing port (single subagent dispatch on Sonnet)

### Task 1: TDD red — write the 5 NAI-108 failing tests

**Files:**
- Modify: `modules/world/player_masks_test.go` (append after line 246, end of file)

- [ ] **Step 1.1: Append 5 new tests at end of file**

Append the following block immediately after the existing `TestNewPlayerSetsEntityMaskToMaskFaceEntity` function (the file currently ends at its closing brace, ~line 246).

```go
// === NAI-108: Player.ResetMasks trailing-clear + chat metadata reset ===
//
// Mirrors NPC-side coverage at npc_masks_test.go:230-281.

// TestPlayerResetMasksTrailingClearFires — NAI-108 Task 1.
// When p.target is nil but p.faceEntity still holds a prior NPC slot,
// ResetMasks emits MaskFaceEntity and snaps faceEntity to -1, mirroring
// TS PathingEntity.ts:611-614. Closes the NAI-91 smoke-surfaced
// "player keeps facing NPC after walking away" symptom.
func TestPlayerResetMasksTrailingClearFires(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.target = nil
	p.faceEntity = 42
	p.masks = 0
	p.ResetMasks()
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (trailing clear should run)", p.faceEntity)
	}
	if p.masks&rsbuf.MaskFaceEntity == 0 {
		t.Error("masks & MaskFaceEntity: got 0, want nonzero (trailing clear should emit)")
	}
}

// TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent — NAI-108 Task 1.
// Quirk guard: trailing clear must not fire when p.target is non-nil
// (the player is still facing someone, by design). Pattern mirrors NPC
// test at npc_masks_test.go:254-267.
func TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.faceEntity = 42
	p.masks = 0
	p.ResetMasks()
	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (trailing clear should be skipped — target present)", p.faceEntity)
	}
	if p.masks&rsbuf.MaskFaceEntity != 0 {
		t.Error("masks & MaskFaceEntity: got nonzero, want 0 (trailing clear should not emit — target present)")
	}
}

// TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne — NAI-108 Task 1.
// Quirk guard: trailing clear must not fire when faceEntity is already
// -1 (no pending clear to sync). Pattern mirrors NPC test at
// npc_masks_test.go:272-281.
func TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.target = nil
	p.faceEntity = -1
	p.masks = 0
	p.ResetMasks()
	if p.masks != 0 {
		t.Errorf("masks: got 0x%x, want 0 (trailing clear should be skipped — faceEntity already -1)", p.masks)
	}
}

// TestPlayerResetMasksClearsChatMetadata — NAI-108 Task 1.
// TS Player.resetEntity at Player.ts:461-463 nulls chatColour/Effect/Rights
// each tick. Goscape resets to -1 (the sentinel used at newPlayer init,
// player.go:494-496) for TS-fidelity. The encoder gates on chatBytes != nil
// (tick.go:423), so this reset is observably-no-op; pinning it preserves
// future TS-faithfulness if a non-gated reader is added.
func TestPlayerResetMasksClearsChatMetadata(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.chatColour = 5
	p.chatEffect = 3
	p.chatRights = 2
	p.ResetMasks()
	if p.chatColour != -1 {
		t.Errorf("chatColour: got %d, want -1", p.chatColour)
	}
	if p.chatEffect != -1 {
		t.Errorf("chatEffect: got %d, want -1", p.chatEffect)
	}
	if p.chatRights != -1 {
		t.Errorf("chatRights: got %d, want -1", p.chatRights)
	}
}

// TestPlayerResetMasksChatMetadataResetIsNoOpWithoutChatBytes — NAI-108 Task 1.
// Regression pin for the spec §3 (ε) "chat reset is functionally inert"
// claim. Asserts that with chatBytes nil (the encoder gate), arbitrary
// pre-reset chatColour/Effect/Rights values do not cause a chat packet
// to be emitted on the next tick. This is the structural reason the
// chat reset is TS-fidelity polish, not a behavior change.
//
// We assert via the in-place mask state: chatBytes nil → MaskChat must
// not fire from ResetMasks. ResetMasks itself never sets MaskChat (only
// Chat() does, in player_masks.go:18). After ResetMasks runs, p.chatBytes
// is still nil and p.masks must not carry MaskChat regardless of color
// pre-state. This pins the encoder-gate equivalence claim without
// requiring the full tick.go:423 chat-encode path.
func TestPlayerResetMasksChatMetadataResetIsNoOpWithoutChatBytes(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.chatBytes = nil
	p.chatColour = 99
	p.chatEffect = 99
	p.chatRights = 99
	p.masks = 0
	p.ResetMasks()
	if p.chatBytes != nil {
		t.Error("chatBytes: should remain nil after ResetMasks")
	}
	if p.masks&rsbuf.MaskChat != 0 {
		t.Errorf("masks & MaskChat: got nonzero, want 0 (chat reset must not synthesize MaskChat)")
	}
}
```

- [ ] **Step 1.2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run 'TestPlayerResetMasks(TrailingClearFires|TrailingClearSkippedWhenTargetPresent|TrailingClearSkippedWhenFaceEntityAlreadyMinusOne|ClearsChatMetadata|ChatMetadataResetIsNoOpWithoutChatBytes)' -v
```

Expected output:
- `TestPlayerResetMasksTrailingClearFires` → **FAIL**: `faceEntity: got 42, want -1` and `masks & MaskFaceEntity: got 0, want nonzero`.
- `TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent` → **PASS** (no behavior change yet — faceEntity stays 42 because nothing clears it).
- `TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` → **PASS** (same — masks already 0, nothing emits).
- `TestPlayerResetMasksClearsChatMetadata` → **FAIL**: `chatColour: got 5, want -1` (and same for effect/rights).
- `TestPlayerResetMasksChatMetadataResetIsNoOpWithoutChatBytes` → **PASS** (ResetMasks already doesn't synthesize MaskChat).

If a "should-fail" test passes, STOP and re-read T2's planned changes — the body may already be partly there.

- [ ] **Step 1.3: Commit**

```bash
git add modules/world/player_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-108 T1 — Player.ResetMasks trailing-clear + chat-reset red

Adds 5 tests pinning the TS PathingEntity.ts:611-614 trailing-clear
and the TS Player.ts:461-463 chat-metadata reset on the *Player side.
Mirrors NPC-side coverage at npc_masks_test.go:230-281.

Two tests fail (TrailingClearFires, ClearsChatMetadata); three pass
trivially (skip guards + no-op encoder-gate pin). T2 lands the
production change to flip the failing pair green.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: green — port trailing-clear + chat reset; expand ResetMasks doc-comment

**Files:**
- Modify: `modules/world/player_masks.go:50-74` (`ResetMasks` body + doc-comment)
- Modify: `modules/world/player_masks_test.go:91` (one-line comment update for conditional persistence)

- [ ] **Step 2.1: Replace `ResetMasks` (and its doc-comment) with the NAI-108 shape**

Find the existing `ResetMasks` block at `modules/world/player_masks.go:50-74`:

```go
// ResetMasks clears mask bits and ephemeral mask state for the next tick.
// Persistent fields (animID, faceEntity, faceSquareX/Z, levels[Hitpoints],
// baseLevels[Hitpoints]) retained — S6e promoted Player HP from per-tick
// ephemeral to persistent, routed through the skill arrays. Also clears
// one-shot movement intents (tele, jump) so a single-tick teleport
// emission doesn't repeat next tick.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	p.sayText = nil
	p.chatBytes = nil
	p.damageAmt = -1
	p.damageType = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	p.exactStartX = -1
	p.exactStartZ = -1
	p.exactEndX = -1
	p.exactEndZ = -1
	p.exactBegin = -1
	p.exactFinish = -1
	p.exactDir = -1
}
```

Replace with:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Mirrors TS
// PathingEntity.resetPathingEntity (PathingEntity.ts:577-615) plus the
// Player-only fields TS resets in Player.resetEntity (Player.ts:454-467,
// non-respawn branch).
//
// Persistent-by-design (TS resets, goscape preserves):
//   - animID/animDelay (TS PathingEntity.ts:598-600) — animations carry
//     across ticks until a new PlayAnim/script-driven change.
//   - faceSquareX/Z (TS PathingEntity.ts:608-609) — non-symptomatic
//     persistence; the encoder gates on MaskFaceCoord which IS cleared
//     via `p.masks = 0` below.
//   - levels[Hitpoints] / baseLevels[Hitpoints] (S6e promotion to
//     persistent via the skill arrays).
//   - moveSpeed — see NAI-108-D-MOVESPEED-NOT-RESET (deferred audit).
//
// Handled-elsewhere (NOT in ResetMasks; equivalent goscape paths):
//   - walkDir/runDir — reset/set in movement.go:53-65 per movement step.
//   - stepsTaken — reset in movement.go:46 (pinned by
//     TestResolveMovementResetsStepsTaken at movement_test.go:214).
//   - lastTickX/Z + lastLevel — set in movement.go:48-50 per movement step.
//   - interacted/apRangeCalled — reset on SetInteraction (interaction.go:85-86),
//     ClearInteraction (interaction.go:133-134), and post-fire
//     (player_interaction_trigger.go:121).
//   - socialProtect/reportAbuseProtect — reset in tick.go:532-533 (NAI-72).
//
// Also clears one-shot movement intents (tele, jump) so a single-tick
// teleport emission doesn't repeat next tick.
//
// The trailing-clear mirrors TS PathingEntity.ts:611-614 with a
// one-tick lag deviation (Go's ResetMasks runs at tick end via
// tick.go processCleanup, so the mask bit is consumed by the NEXT
// tick's info-pass — same convention as Npc.ResetMasks at
// npc_masks.go:184-207). Closes NAI-91's "player keeps facing NPC
// after walking away" smoke residual.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	p.sayText = nil
	p.chatBytes = nil
	p.chatColour = -1
	p.chatEffect = -1
	p.chatRights = -1
	p.damageAmt = -1
	p.damageType = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	p.exactStartX = -1
	p.exactStartZ = -1
	p.exactEndX = -1
	p.exactEndZ = -1
	p.exactBegin = -1
	p.exactFinish = -1
	p.exactDir = -1
	if p.target == nil && p.faceEntity != -1 {
		p.masks |= p.entitymask
		p.faceEntity = -1
	}
}
```

- [ ] **Step 2.2: Update the now-stale doc-comment in `TestResetMasksClearsEphemerals`**

Find `modules/world/player_masks_test.go:91`:

```go
	// Persistent (animID, faceEntity, levels[3], baseLevels[3]) should stay.
```

Replace with (one line — wrap to two if your line-length convention demands it; the file uses tabs and tolerates long comments):

```go
	// Persistent (animID, levels[3], baseLevels[3]) and conditionally-persistent
	// faceEntity (target=nil/faceEntity=-1 here, so trailing-clear no-ops) should stay.
```

This makes the persistence claim accurate post-NAI-108. The test itself does NOT need assertion changes: `newTestPlayer` initializes `p.faceEntity = -1` (per `player.go:509`) and the test never sets it; the trailing-clear is skipped (skipped-when-already-(-1) branch).

- [ ] **Step 2.3: Run the NAI-108 tests + the existing ResetMasks ephemerals test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run 'TestPlayerResetMasks|TestResetMasksClearsEphemerals' -v
```

Expected: all 6 tests PASS (5 NAI-108 + 1 existing ephemerals).

- [ ] **Step 2.4: Run full `modules/world` package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: PASS. If any `_test.go` site that previously left `p.faceEntity` set with `p.target == nil` and then called `ResetMasks` now flips, that test must be re-evaluated. Pre-flight 8 confirmed zero such sites in `player_masks_test.go`; if a failure surfaces in another file, stop and report the file:line + assertion delta — this is the spec R5/R6 escalation path.

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/player_masks.go modules/world/player_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-108 T2 — Player.ResetMasks trailing-clear + chat reset

Ports TS PathingEntity.resetPathingEntity:611-614 trailing-clear and
TS Player.resetEntity:461-463 chat-metadata reset to *Player side.
Closes NAI-91 "player keeps facing NPC after walking away" smoke
residual. Chat reset is observably no-op (encoder gates on chatBytes
nil); ships for TS fidelity per spec §3 (ε).

Doc-comment expanded with full γ/δ classification per audit table
(persistent-by-design + handled-elsewhere with file:line citations).

T1 RED → GREEN; existing TestResetMasksClearsEphemerals still green
(faceEntity defaults to -1 in newTestPlayer; trailing-clear no-ops).
Comment at player_masks_test.go:91 updated for conditional-persistence
semantics per spec R5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: chatMessage dead-field retire

**Files:**
- Modify: `modules/world/player.go:196` (delete one line)

- [ ] **Step 3.1: Delete the dead field**

Find at `modules/world/player.go:196`:

```go
	chatMessage                        []byte
```

Delete this line. The surrounding chat-state group at lines 194-199:

```go
	// === chat state ===
	publicChat, privateChat, tradeDuel int
	chatMessage                        []byte
	chatColour, chatEffect, chatRights int
	mutedUntil                         time.Time
	messageCount                       int
```

becomes:

```go
	// === chat state ===
	publicChat, privateChat, tradeDuel int
	chatColour, chatEffect, chatRights int
	mutedUntil                         time.Time
	messageCount                       int
```

- [ ] **Step 3.2: Verify build + test green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: all clean. The field has zero readers/writers per pre-flight 3, so `go build` and `go test` should be unaffected.

If any compile error surfaces referencing `chatMessage`, STOP and report the file:line — pre-flight 3 should have caught it; if it's a stash/wip the user has, surface for explicit handling.

- [ ] **Step 3.3: HEAD-grep verification**

```bash
rg "chatMessage" modules/ pkg/
```

Expected: 1 match remaining at `modules/world/tick.go:530` (the comment that names `chatMessage` as part of `NAI-72-N-RESETENTITY-PARTIAL` — this gets retired in T5).

- [ ] **Step 3.4: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-108 T3 — retire chatMessage dead field

Per dead_api_polish.md: the chatMessage []byte field at player.go:196
was declared but never read or written anywhere in the repo (verified
pre-flight + post-delete go build/test green). Deleting it shrinks
the *Player struct and removes an attractive nuisance for future
ports that might bind to a non-functional field.

The TS Player.chatMessage equivalent was logged-in-comment at
tick.go:530 as part of NAI-72-N-RESETENTITY-PARTIAL; that comment
gets retired in T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: (δ) verify-and-pin audit

**Files:** read-only audit + (conditionally) test additions in:
- `modules/world/movement_test.go` (existing coverage: `TestResolveMovementResetsStepsTaken:214`)
- `modules/world/interaction_trigger_nai68_test.go` / `interaction_trigger_nai69_test.go` (interacted/apRangeCalled coverage)
- `modules/world/player_npc_test.go`, `player_info_test.go`, `rsbuf_per_tick_test.go` (lastTickX/Z coverage)

The goal is to confirm each (δ)-classified field in the spec §3 audit table has at least one existing test that pins the goscape-handler-elsewhere ↔ TS-tick-reset equivalence. Where coverage exists, no code changes — just record the mapping in the close commit body. Where coverage is missing, add a minimal pin.

- [ ] **Step 4.1: Audit `stepsTaken`**

Run:
```bash
rg -n "stepsTaken" modules/world/*_test.go
```

Existing pin: `TestResolveMovementResetsStepsTaken` at `movement_test.go:214` (pre-flight 9 confirmed). Records: stepsTaken is reset to 0 at top of `resolveMovement` regardless of waypointIndex. Equivalent to TS PathingEntity.ts:586 `stepsTaken = 0`.

**Disposition: COVERED. No new test.**

- [ ] **Step 4.2: Audit `walkDir` / `runDir`**

Run:
```bash
rg -n "walkDir\b|runDir\b" modules/world/*_test.go
```

Look for any test that asserts walkDir/runDir state across a no-movement tick (i.e. waypointIndex < 0 path through `resolveMovement` or whatever the actual no-step code path is). The expected pattern: a test that sets walkDir/runDir to a non-(-1) value (simulating prior tick), runs the no-movement path, and asserts they end at -1.

If a matching test exists (`TestResolveMovement*` family is the natural home), record its name in the close commit body.

If NO such test exists, append the following minimal pin to `modules/world/movement_test.go` (insert immediately after `TestResolveMovementResetsStepsTaken`):

```go
// TestResolveMovementResetsWalkDirRunDir — NAI-108 Task 4 (δ) verify-and-pin.
// TS PathingEntity.ts:579-580 resets walkDir/runDir to -1 every tick.
// goscape resets them in movement.go (per resolveMovement). Pins the
// equivalence: stale walkDir/runDir from a prior tick get cleared on the
// next no-movement-path tick.
func TestResolveMovementResetsWalkDirRunDir(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.walkDir = 5 // simulate stale dir from prior tick
	p.runDir = 3

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (no-movement path should leave at -1)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (no-movement path should leave at -1)", p.runDir)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestResolveMovementResets -v`. Expected: PASS.

**If the test fails** (i.e. resolveMovement does NOT reset walkDir/runDir on no-movement path), STOP. This is a real divergence per spec R1: do NOT auto-port to ResetMasks. Instead, document the finding in the audit doc + open a tracked deviation (`NAI-108-D-WALKDIR-RUNDIR-NO-RESET-ON-IDLE`) and escalate to controller for disposition (likely: defer to a future audit sub-spec, since the (γ) intentional-persistence rationale may apply).

- [ ] **Step 4.3: Audit `lastTickX/Z` + `lastLevel`**

Run:
```bash
rg -n "lastTickX\b|lastTickZ\b|lastLevel\b" modules/world/*_test.go
```

Expected: multiple test setups assigning `p.lastTickX, p.lastTickZ, p.lastLevel = ...` (pre-flight surface from `player_npc_test.go:21`, `player_info_test.go:21`, `rsbuf_per_tick_test.go:37,42`). These tests USE the field but don't pin reset semantics.

The TS contract (PathingEntity.ts:583-585) is "lastTickX = x" (same for Z/Level) every tick. In goscape, `movement.go:48-50` sets these inside a movement-step path. The post-tick state should satisfy `lastTickX == p.x` (tautologically true if no movement happened, since both update together).

**Disposition: NO-OP (semantically tautological in idle path)**. The TS reset writes back the current coord; goscape's lazy-update achieves the same end-state because nothing changes when no movement happens. Record as "TS-equivalent by tautology" in close commit body. No new test required.

If you disagree with this disposition during implementation (e.g. you find a code path where `lastTickX != p.x` between ticks and a downstream consumer reads it), STOP and escalate to controller.

- [ ] **Step 4.4: Audit `interacted`**

Run:
```bash
rg -n "p\.interacted\b\|n\.interacted\b" modules/world/*_test.go
```

Expected matches (pre-flight surface): `interaction_trigger_nai68_test.go:127,154` set `p.interacted = true` mid-test as setup state.

The TS contract (PathingEntity.ts:587) is "interacted = false" every tick. In goscape, `interacted` is set to `false` only on `SetInteraction` (`interaction.go:86`) and `ClearInteraction` (`interaction.go:134`); set to `true` mid-tick on fire (`interaction.go:390,400`). Across-tick leak risk: if a tick fires interaction (sets interacted=true) and the next tick neither calls SetInteraction nor ClearInteraction nor fires interaction, then `p.interacted` carries from prior tick.

Trace consumers: `rg "p\.interacted\b" modules/world/*.go | grep -v _test.go`. List the reading sites and assess whether any reads `p.interacted` BEFORE the same-tick re-set.

If a leak risk surfaces in audit, append to `modules/world/interaction_test.go` (or wherever feels natural):

```go
// TestPlayerInteractedDoesNotLeakAcrossIdleTick — NAI-108 Task 4 (δ) verify-and-pin.
// TS PathingEntity.ts:587 resets interacted=false every tick. goscape
// relies on SetInteraction/ClearInteraction handlers re-setting on the
// next interaction touch. Pins the no-leak contract for the idle-tick
// path (no SetInteraction/ClearInteraction call between ticks).
//
// If this fails, NAI-108-D-INTERACTED-LEAK escalation per spec R1.
func TestPlayerInteractedDoesNotLeakAcrossIdleTick(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.interacted = true // simulate prior-tick fire
	// Idle tick: ResetMasks runs at tick end; no interaction touch.
	p.ResetMasks()
	if p.interacted {
		t.Error("interacted: got true, want false (must not leak across idle tick) — NAI-108-D-INTERACTED-LEAK candidate")
	}
}
```

Run the test. **If it fails** (which it will, since ResetMasks does NOT touch p.interacted), STOP and escalate per spec R1. Disposition options:
1. Document as `NAI-108-D-INTERACTED-LEAK` open deviation; defer fix.
2. Add `p.interacted = false` to ResetMasks (scope creep into Stage A — controller decides).

**Default disposition (don't auto-port)**: skip-pin the test with `t.Skip("NAI-108-D-INTERACTED-LEAK — pinned for future fix; current goscape relies on handler-elsewhere re-set; see audit table §3")` and document the deviation. Implementer surfaces the choice; controller decides.

- [ ] **Step 4.5: Audit `apRangeCalled`**

Same shape as `interacted` (T4.4). The reset sites are at `interaction.go:85,133` and `player_interaction_trigger.go:121`; set true at `handlers_player.go:695` (`handlePApRange`).

Run:
```bash
rg "p\.apRangeCalled\b" modules/world/*.go | grep -v _test.go
```

If audit surfaces leak risk, mirror T4.4's test pattern:

```go
// TestPlayerApRangeCalledDoesNotLeakAcrossIdleTick — NAI-108 Task 4 (δ).
// TS PathingEntity.ts:588 resets apRangeCalled=false every tick.
// Pinning the no-leak contract — see T4.4 for full rationale.
func TestPlayerApRangeCalledDoesNotLeakAcrossIdleTick(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRangeCalled = true
	p.ResetMasks()
	if p.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (must not leak across idle tick) — NAI-108-D-APRANGECALLED-LEAK candidate")
	}
}
```

Default disposition: skip-pin with `t.Skip(...)` per T4.4 and document deviation.

- [ ] **Step 4.6: Run all (δ) audit tests + verify state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run 'TestResolveMovementResets|TestPlayerInteractedDoesNotLeak|TestPlayerApRangeCalledDoesNotLeak|TestPlayerResetMasks' -v
```

Expected: all PASS (or skip-pinned per T4.4/T4.5 disposition, with explicit Skip messages).

- [ ] **Step 4.7: Commit (only if step 4.2/4.4/4.5 added test code or skip-pins)**

```bash
git add modules/world/movement_test.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-108 T4 — (δ) verify-and-pin handled-elsewhere fields

Per spec §3 audit table, pins the goscape-handler-elsewhere ↔
TS-tick-reset equivalence for fields that aren't in ResetMasks but
have an equivalent reset path:

  - stepsTaken: COVERED by TestResolveMovementResetsStepsTaken
    (movement_test.go:214); cited in close commit body, no new test.
  - walkDir/runDir: <ADDED|COVERED> per audit step 4.2.
  - lastTickX/Z/Level: TS-equivalent by tautology (lazy update); no new test.
  - interacted: <SKIP-PINNED|PORTED> per audit step 4.4.
  - apRangeCalled: <SKIP-PINNED|PORTED> per audit step 4.5.

Any deviations surfaced get tracked entries in the close commit's
followup section.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If no test code was added in T4 (everything was COVERED or NO-OP), skip the commit and record the audit findings in the T6 close commit body instead.

---

### Task 5: tick.go comment update — retire NAI-72-N-RESETENTITY-PARTIAL

**Files:**
- Modify: `modules/world/tick.go:526-531` (comment block above `p.socialProtect = false`)

- [ ] **Step 5.1: Replace the NAI-72 comment block**

Find at `modules/world/tick.go:526-531`:

```go
		// NAI-72 — TS Player.resetEntity(false) at Player.ts:466-467.
		// Reset social/report spam-protect flags so the next tick admits
		// at most one social/report packet per type per player.
		// (Other resetEntity fields — protect, chatColour/Effect/Rights,
		// chatMessage, logMessage — belong to other sub-specs; tracked
		// as NAI-72-N-RESETENTITY-PARTIAL.)
```

Replace with:

```go
		// NAI-72/108 — TS Player.resetEntity(false) at Player.ts:454-467.
		// Reset social/report spam-protect flags so the next tick admits
		// at most one social/report packet per type per player.
		//
		// NAI-72-N-RESETENTITY-PARTIAL retired by NAI-108:
		//   - protect → activeScript.Protect (already-converged divergence;
		//     see interaction.go:308, player_script.go:276,297-300).
		//   - chatColour/Effect/Rights → moved to ResetMasks per TS fidelity.
		//   - chatMessage → dead field deleted (player.go:196 retired).
		//   - logMessage → TS-only, no goscape consumer (YAGNI).
		// unfocus() remains deferred per NAI-67-D-PLAYER-UNFOCUS-DEFERRED
		// (Player respawn/death sub-spec).
```

- [ ] **Step 5.2: Verify HEAD-grep state**

```bash
rg "NAI-72-N-RESETENTITY-PARTIAL" modules/ pkg/
rg "chatMessage" modules/ pkg/
```

Expected: both `rg` invocations return ZERO matches. The comment that named the tracker is gone; the dead field is gone; the only remaining `chatMessage` reference (the comment in tick.go) was just rewritten without the literal field name.

- [ ] **Step 5.3: Run full repo tests + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all clean.

- [ ] **Step 5.4: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-108 T5 — retire NAI-72-N-RESETENTITY-PARTIAL tracker

Updates tick.go:526-531 comment to reflect post-NAI-108 deferred set.
Each named field in the original NAI-72 list now has a disposition:
protect (already-converged via activeScript.Protect), chatColour/Effect/
Rights (moved to ResetMasks T2), chatMessage (dead field retired T3),
logMessage (TS-only YAGNI). unfocus() stays deferred per
NAI-67-D-PLAYER-UNFOCUS-DEFERRED.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Full-repo verification + close commit

**Files:** none (verification + close-commit only)

- [ ] **Step 6.1: Full verification suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all clean. (`-race` flag catches concurrent-access bugs in the trailing-clear introduction; `processCleanup` runs serially per the `s.playersMu.RLock()` snapshot at `tick.go:520-523`, so no race expected.)

- [ ] **Step 6.2: HEAD-grep sentinel checks (per spec §10)**

```bash
rg "chatMessage" modules/ pkg/                                       # → 0 matches
rg "NAI-72-N-RESETENTITY-PARTIAL" modules/ pkg/                      # → 0 matches
rg "p\.target == nil && p\.faceEntity" modules/world/player_masks.go # → 1 match
rg 'p\.chatColour\s*=\s*-1' modules/world/player_masks.go            # → 1 match
```

If any sentinel deviates, STOP and report.

- [ ] **Step 6.3: `git status` cleanliness check**

```bash
git status
```

Expected: clean working tree (all changes committed across T1-T5). Per `feedback_subagent_wt_path.md`: if any stray file modifications remain, surface for explicit handling — do NOT include unrelated changes in the close commit.

- [ ] **Step 6.4: `git show --stat` per-commit verification**

```bash
git log --oneline -8
git show HEAD --stat
git show HEAD~1 --stat
git show HEAD~2 --stat
git show HEAD~3 --stat
git show HEAD~4 --stat
```

Expected commit chain (newest → oldest):
1. `chore(world): NAI-108 T5 — retire NAI-72-N-RESETENTITY-PARTIAL` (1 file: `tick.go`)
2. (T4 commit if any) `test(world): NAI-108 T4 — (δ) verify-and-pin`
3. `refactor(world): NAI-108 T3 — retire chatMessage dead field` (1 file: `player.go`)
4. `feat(world): NAI-108 T2 — Player.ResetMasks trailing-clear + chat reset` (2 files: `player_masks.go`, `player_masks_test.go`)
5. `test(world): NAI-108 T1 — Player.ResetMasks trailing-clear + chat-reset red` (1 file: `player_masks_test.go`)

Per `implementer_commit_content_verify.md`: each commit's `--stat` must match its message scope. If a stray file is included in a commit, report it.

- [ ] **Step 6.5: Write the close commit**

This commit ships ONLY the close-out documentation; no code changes. Use `--allow-empty` since all code already committed.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-108 — Player.ResetMasks audit + load-bearing trailing-clear

Standard sub-spec retiring NAI-72-N-RESETENTITY-PARTIAL tracker and
the NAI-91 close-day "player keeps facing NPC after walking away"
smoke-surfaced symptom. Single bundle, 5 commits, dispatched per
subagent-driven-development on Sonnet.

Spec: docs/superpowers/specs/2026-05-05-nai-108-player-resetmasks-audit-design.md
Plan: docs/superpowers/plans/2026-05-05-nai-108-player-resetmasks-audit.md

Audit table (spec §3) classifies every TS PathingEntity.resetPathingEntity
+ Player.resetEntity reset against goscape disposition (α/β/γ/δ/ε/ζ).
Load-bearing β port is the missing trailing-clear in Player.ResetMasks
mirroring npc_masks.go:204-207 (NAI-13/14 era for NPC side).

Stage A code shipped (T2/T3/T5):
  - Player.ResetMasks: trailing-clear (β) + chat-metadata reset (ε)
    + expanded doc-comment with γ/δ classification.
  - Player.chatMessage dead field retired (ε; per dead_api_polish.md).
  - tick.go:526-531 comment updated to reflect retired tracker.

Stage B audit (T1/T4):
  - 5 NAI-108 tests added (TrailingClear×3, ChatMetadata×2).
  - (δ) handled-elsewhere coverage:
      * stepsTaken — COVERED by TestResolveMovementResetsStepsTaken
        (movement_test.go:214).
      * walkDir/runDir — <FILL FROM T4.2 ACTUAL>
      * lastTickX/Z/Level — TS-equivalent by tautology.
      * interacted — <FILL FROM T4.4 ACTUAL: SKIP-PINNED or PORTED>.
      * apRangeCalled — <FILL FROM T4.5 ACTUAL: SKIP-PINNED or PORTED>.

Surfaced/deferred:
  - NAI-108-D-MOVESPEED-NOT-RESET (new): TS resets moveSpeed each tick;
    goscape persists. Requires consumer audit before fix; opens as
    follow-up entry in nai_followups.md.
  - <FILL: any T4 leak deviations if surfaced>

Smoke handoff: user re-tests "speak with RuneScape Guide → walk away
→ confirm player face-direction releases (player no longer faces NPC
after taking a step)". Smoke binds the (β) trailing-clear fix per
cascade_theory_smoke_binding.md.

Closes:
  - nai_followups.md NAI-91 face-NPC-after-walking-away symptom.
  - NAI-72-N-RESETENTITY-PARTIAL tracker (was at tick.go:529-531).

Closes memory: nai_followups.md NAI-91 line 5176 (face-NPC-after-walking-away).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Implementer note for the close commit body**: the `<FILL FROM ...>` placeholders need the actual T4 outcomes (which fields were added vs covered vs skip-pinned). Edit before committing; do NOT commit literal `<FILL ...>` text.

- [ ] **Step 6.6: Final `git log` verification**

```bash
git log --oneline -7
```

Expected: 6 NAI-108 commits (T1 test + T2 feat + T3 refactor + optional T4 test + T5 chore + T6 close) on top of the 2 NAI-108 spec commits (`b8fe136` + `aa67b18`).

---

## End-of-bundle review (controller-dispatched, NOT in implementer scope)

Per `superpowers_code_reviewer_model.md`, dispatch a `superpowers:code-reviewer` agent on **Sonnet** (NOT Opus) to review the bundle. Prompt template:

> Review NAI-108 single-bundle implementation against the spec at `docs/superpowers/specs/2026-05-05-nai-108-player-resetmasks-audit-design.md` (HEAD `aa67b18`) and plan at `docs/superpowers/plans/2026-05-05-nai-108-player-resetmasks-audit.md`.
>
> Bundle commits: T1 test → T2 feat → T3 refactor → T4 test (optional) → T5 chore → T6 close. Last 6-7 commits on `main` post-`aa67b18`.
>
> Verification scope:
> 1. **Trailing-clear correctness** — body of new block matches spec §4 verbatim; `p.entitymask` (NOT hardcoded `rsbuf.MaskFaceEntity`); `p.target == nil && p.faceEntity != -1` guard exact.
> 2. **Chat-reset semantics** — chatColour/Effect/Rights all reset to -1 (matching `newPlayer` init at player.go:494-496), not 0 or null.
> 3. **chatMessage retire safety** — confirm zero remaining references via `rg`.
> 4. **Doc-comment accuracy** — every TS line citation in the new ResetMasks doc-comment matches HEAD of `Engine-TS` per `ts_source_canonical_path.md`. Cite verifications.
> 5. **(δ) audit completeness** — for each handled-elsewhere field in spec §3, verify the implementer's disposition (COVERED with cited test / SKIP-PINNED with deviation / PORTED) is sound. Flag any field where the audit logic is suspect.
> 6. **Test quality** — assertion shapes match NPC-side parallels at `npc_masks_test.go:230-281`; test names match the convention (`TestPlayerResetMasks*`); no test depends on internal mock helpers that don't exist.
> 7. **Commit hygiene** — each commit's `--stat` matches its message scope per `implementer_commit_content_verify.md`; no stray files; no `_test.go` changes mixed into production-only commits.
> 8. **Spec divergence flags** — the close commit's `<FILL FROM ...>` placeholders were filled with concrete T4 outcomes; no literal `<FILL>` text shipped.
>
> Report any Critical or Important issues with file:line citations. Approve only if zero such issues remain. If you find a divergence between implementer-stated outcomes and actual code/tests, flag immediately — per `verify_implementer_claims.md`, run independent grep + Read before concluding.

**Reviewer dispatch is controller's responsibility** — happens AFTER implementer reports bundle complete and BEFORE the user-driven smoke handoff.

---

## Self-review (controller, BEFORE dispatching the bundle)

After writing this plan and before dispatching, the controller reads the spec sections vs each plan task to confirm coverage:

| Spec section | Plan task(s) |
|---|---|
| §3 audit table (β trailing-clear) | T1 (red) + T2 (green) |
| §3 audit table (γ persistence labels) | T2 doc-comment expansion |
| §3 audit table (δ verify-and-pin) | T4 (per-field audit) |
| §3 audit table (ε chat metadata reset) | T1 (red) + T2 (green) |
| §3 audit table (ε chatMessage retire) | T3 |
| §3 audit table (ε protect/logMessage non-port) | T5 comment update (no code) |
| §4 Stage A item 3 (tick.go comment) | T5 |
| §6 NAI-108-D-MOVESPEED-NOT-RESET (new follow-up) | T6 close commit body + nai_followups.md update (post-close) |
| §7 retire NAI-72-N-RESETENTITY-PARTIAL | T5 + T6 |
| §7 close NAI-91 symptom | T6 close commit + smoke handoff |
| §10 verification | T6 steps 6.1-6.4 |

Coverage map verifies all spec sections are addressed. R5/R6 from spec §9 are addressed inline at T2.2 (comment update) and T1 (target field assignment pattern).

**Placeholder scan:** no `TBD`/`TODO`/`fill in details` in plan body except the deliberate `<FILL FROM T4.X ACTUAL>` markers in T6.5's close commit template (these are explicit instructions for the implementer to fill at close time, NOT plan-author placeholders).

**Type consistency:** all field names (`p.target`, `p.faceEntity`, `p.entitymask`, `p.chatColour/Effect/Rights`, `p.chatBytes`, `p.masks`) match across tasks; mask constant `rsbuf.MaskFaceEntity` matches across tests; helper signatures (`newTestPlayer(t)`, `newTestNpc(id)`) match the verified pre-flight surface.
