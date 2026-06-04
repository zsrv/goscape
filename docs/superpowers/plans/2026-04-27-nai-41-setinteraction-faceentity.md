# NAI-41 — `Player.SetInteraction` face-entity TS-fidelity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `Player.faceEntity` writes from contact-time (`processInteraction`) to anchor-time (`SetInteraction`) and add the previously-missing `*Player`-target branch, mirroring the in-codebase `Npc.SetInteraction` template. Closes `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` and the pre-existing *Player→*Npc contact-time-write divergence.

**Architecture:** Single TDD task. Extend `Player.SetInteraction` with a type-switch on `target.(type)` for `*Player`/`*Npc`/default branches, with the TS idempotency check (`if faceEntity != X`) inlined to match `Npc.SetInteraction:651-666`. Delete the redundant contact-time write block in `processInteraction`. Drop the closed deviation comment block. Track one new deviation (`NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`) in the `default` arm.

**Tech Stack:** Go 1.26+ (per `go_version.md`; use `use-modern-go` skill). TS source: `LostCityRS/Engine-TS` only per `ts_source_canonical_path.md`. HEAD baseline: `8d78b9f` (NAI-41 spec commit).

---

## Spec reference

Spec at `docs/superpowers/specs/2026-04-27-nai-41-setinteraction-faceentity-design.md`. Test buckets map to the single task as:
- §7 new tests #1–4 → Task 1 Step 1.1 (write all four failing tests in one file edit)
- §7 existing-test assertion delete (`TestProcessInteractionInRangeFacesTarget`) → Task 1 Step 1.5

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/interaction.go` | modify | extend `Player.SetInteraction` (lines 47-57); delete contact-time block (96-100); delete deviation comment (101-112); add new `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` comment in default arm |
| `modules/world/interaction_test.go` | modify | append 4 new SetInteraction face-entity tests; delete 2 assertions in `TestProcessInteractionInRangeFacesTarget` |

## Pre-flight checks (controller)

Per `controller_preflight.md`: re-grep each premise against HEAD before dispatching the task.

| Check | Command | Expected at HEAD `8d78b9f` |
|-------|---------|---------------------------|
| `Player.SetInteraction` body shape | `sed -n '43,57p' modules/world/interaction.go` | matches §3 of spec |
| `processInteraction` contact branch | `sed -n '95,118p' modules/world/interaction.go` | shows `SetFaceEntity(npc.nid)` block at 98-100 + deviation comment at 101-112 |
| `Player.entitymask` init | `rg -n 'entitymask:\s*rsbuf\.MaskFaceEntity' modules/world/player.go` | `player.go:411` |
| `Player.slot` field type | `rg -n '^\tslot\s+int' modules/world/player.go` | `player.go:63` |
| `Player.faceEntity` default | `rg -n 'faceEntity:\s*-1' modules/world/player.go` | `player.go:410` |
| Npc.SetInteraction template | `sed -n '650,668p' modules/world/npc_interaction.go` | shows the type-switch with `slot := t.slot + 32768` etc. |
| `MaskFaceEntity` alias | `rg -n 'MaskFaceEntity\s*=\s*4' modules/world/masks.go` | `masks.go:8` |
| `entity` interface | `rg -n '^type entity interface' modules/world/movement_consts.go` | `movement_consts.go:45` |
| existing test fixtures | `rg -n 'func (newTestPlayer|makeInteractionNpc|makeInteractionPlayer)\b' modules/world/` | `player_test.go:14`, `interaction_test.go:15`, `interaction_test.go:31` |
| no other `p.faceEntity` writers in `world` | `rg -n '\bp\.faceEntity\s*=' modules/world/` | only `player_masks.go:52` (inside `SetFaceEntity`) plus possibly the contact-time block being removed |
| existing test pinning contact-time write | `rg -n 'p\.faceEntity != npc\.nid\|p\.faceEntity == npc\.nid' modules/world/interaction_test.go` | `interaction_test.go:143` |

Also re-grep for any **non-test** reader of `p.faceEntity` between SetInteraction and end-of-tick (the wire encoder path):

```
rg -n '\bp\.faceEntity\b|\bplayer\.faceEntity\b' modules/world/ pkg/rsbuf/
```

Expected: only writes (the contact-time block being removed, plus `SetFaceEntity`) and end-of-tick reads via the player_info encoder. If any mid-tick reader surfaces unexpectedly, halt and report.

---

### Task 1: Anchor-time face-entity dispatch in `Player.SetInteraction`

**Goal:** Single TDD cycle that writes the 4 new tests + the 2-assertion delete first (RED), then ports the dispatch + deletes the redundant contact-time block (GREEN).

**Files:**
- Modify: `modules/world/interaction.go` (extend lines 47-57; delete lines 98-100; delete lines 101-112; add new deviation comment in default arm of new switch)
- Modify: `modules/world/interaction_test.go` (append 4 tests at end of file; delete 2 assertions at lines 143-148)

#### Step 1.1: Write the 4 failing tests + delete redundant assertions

**Append** to `modules/world/interaction_test.go` (after the last existing test):

```go
// --- NAI-41: Player.SetInteraction face-entity TS-fidelity ---------------
// Mirrors TS PathingEntity.setInteraction (PathingEntity.ts:530-541) and
// the in-codebase Npc.SetInteraction template (npc_interaction.go:651-666).

// TestSetInteractionPlayerTargetSetsFaceEntity pins the *Player branch:
// faceEntity = target.slot + 32768, MaskFaceEntity bit set. The +32768
// magic encodes "this is a player slot" on the client wire.
func TestSetInteractionPlayerTargetSetsFaceEntity(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// Use a second player as the target. slot=-1 default would yield
	// faceEntity=32767 — pick a non-default slot so the formula assertion
	// catches accidental sign drops or off-by-one errors.
	other, _ := newTestPlayer(t)
	other.slot = 5

	p.SetInteraction(InteractionEngine, other, 1, -1)

	wantFE := other.slot + 32768 // 32773
	if p.faceEntity != wantFE {
		t.Errorf("faceEntity: got %d, want %d (slot+32768)", p.faceEntity, wantFE)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set after SetInteraction with *Player target")
	}
}

// TestSetInteractionNpcTargetSetsFaceEntity pins the *Npc branch:
// faceEntity = npc.nid, MaskFaceEntity bit set, AT SetInteraction time
// (not at contact). Supersedes the contact-time pin previously in
// TestProcessInteractionInRangeFacesTarget.
func TestSetInteractionNpcTargetSetsFaceEntity(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d (npc.nid)", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set after SetInteraction with *Npc target")
	}
}

// TestSetInteractionLocTargetDoesNotSetFaceEntity pins the deferred
// default branch: *Loc target leaves faceEntity untouched and
// MaskFaceEntity bit clear. Closes the spec's "deviation is intentional,
// not a partial port" contract for NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ.
func TestSetInteractionLocTargetDoesNotSetFaceEntity(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 100, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (default; *Loc branch must not write)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity != 0 {
		t.Error("MaskFaceEntity bit must NOT be set after SetInteraction with *Loc target")
	}
}

// TestSetInteractionFaceEntityIdempotent pins the TS idempotency check
// at PathingEntity.ts:532 / 538 (`if (this.faceEntity !== X)`). Without
// this check, repeated SetInteraction calls with the same target re-emit
// MaskFaceEntity needlessly. We reset masks=0 between calls to isolate
// the second call's mask-emission decision.
func TestSetInteractionFaceEntityIdempotent(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.masks&MaskFaceEntity == 0 {
		t.Fatal("first SetInteraction should set MaskFaceEntity")
	}
	p.masks = 0 // isolate the second call's emission decision

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if p.masks&MaskFaceEntity != 0 {
		t.Error("second SetInteraction with same target must NOT re-emit MaskFaceEntity (TS idempotency check at PathingEntity.ts:532)")
	}
	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity should remain %d (npc.nid) after idempotent second call, got %d", npc.nid, p.faceEntity)
	}
}
```

**Delete** the two faceEntity/MaskFaceEntity assertions in `TestProcessInteractionInRangeFacesTarget` (currently `interaction_test.go:143-148`). After the edit, the test body should look like:

```go
// TestProcessInteractionInRangeFacesTarget verifies adjacent target triggers
// interacted=true and fires the OP trigger. NAI-41: faceEntity write
// timing moved to SetInteraction-time; this test no longer pins faceEntity
// (covered by TestSetInteractionNpcTargetSetsFaceEntity).
func TestProcessInteractionInRangeFacesTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if !p.interacted {
		t.Error("interacted should be true when adjacent to target")
	}
}
```

(The test docstring update + the deletion of the two `if p.faceEntity != npc.nid {…}` and `if p.masks&MaskFaceEntity == 0 {…}` blocks. Imports already include `io2 "github.com/zsrv/goscape/pkg/io/isaac"` from the file header — no import change needed.)

#### Step 1.2: Run new tests to verify they fail in the expected way

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetInteractionPlayerTargetSetsFaceEntity|TestSetInteractionNpcTargetSetsFaceEntity|TestSetInteractionLocTargetDoesNotSetFaceEntity|TestSetInteractionFaceEntityIdempotent|TestProcessInteractionInRangeFacesTarget' -v
```

Expected:
- `TestSetInteractionPlayerTargetSetsFaceEntity` — **FAIL**: `faceEntity: got -1, want 32773 (slot+32768)` (no *Player branch yet).
- `TestSetInteractionNpcTargetSetsFaceEntity` — **FAIL**: `faceEntity: got -1, want 7 (npc.nid)` (no anchor-time *Npc write yet).
- `TestSetInteractionLocTargetDoesNotSetFaceEntity` — **PASS** (default behavior is already "no write"; this test pins the post-port behavior is the same). Acceptable to pass at RED — the deviation comment in Step 1.3 is the doc-side closure.
- `TestSetInteractionFaceEntityIdempotent` — **FAIL**: first-call assertion `t.Fatal("first SetInteraction should set MaskFaceEntity")` (no anchor-time write yet).
- `TestProcessInteractionInRangeFacesTarget` — **PASS** (the contact-time write still runs; the deletion just narrows what we assert).

If any mismatch from the above, halt and report — the RED-state assertions must match the spec premise before proceeding.

#### Step 1.3: Implement the anchor-time dispatch in `Player.SetInteraction`

**Replace** the body of `Player.SetInteraction` in `modules/world/interaction.go` (currently lines 47-57) with:

```go
// SetInteraction anchors the interaction state machine on a target entity.
// For OpLocT the com parameter carries the spell-component ID; for OpLocU
// pass -1 (item tracking uses lastUseItem/lastUseSlot instead). For
// OpLoc1..5 and OpNpc1..5, callers pass -1.
//
// faceEntity dispatch mirrors TS PathingEntity.setInteraction
// (PathingEntity.ts:530-541) and the in-codebase Npc.SetInteraction
// template (npc_interaction.go:651-666). NAI-41 closed
// NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK and the pre-existing
// *Player→*Npc contact-time-write divergence by moving the faceEntity
// write here from processInteraction's contact branch.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
	p.target = target
	p.targetOp = op
	p.targetSubject.com = com
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false

	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if p.faceEntity != slot {
			p.faceEntity = slot
			p.masks |= p.entitymask
		}
	case *Npc:
		if p.faceEntity != t.nid {
			p.faceEntity = t.nid
			p.masks |= p.entitymask
		}
	default:
		// DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: TS L542-545 sets
		// targetX = CoordGrid.fine(target.x, target.width) and targetZ
		// analogously for *Loc/*Obj targets. Player has no targetX/Z
		// fields and no consumer reads them; deferred to the focus/
		// step-tracking sub-spec that closes NAI-34-D3 (which already
		// touches Player fine-coord infra).
	}
}
```

#### Step 1.4: Remove redundant contact-time write + deviation comment in `processInteraction`

**Replace** the contact-distance branch in `processInteraction` (`modules/world/interaction.go` currently lines 95-117) with the post-port shape:

```go
	if inOperableDistance(p.x, p.z, tx, tz) {
		// Contact range — fire OP. Matches TS Player.ts:1123-1135 (OP
		// checked before AP at contact). NAI-41 moved the faceEntity
		// write to SetInteraction time; no contact-time write needed.
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return
	}
```

(This deletes the `if npc, ok := p.target.(*Npc); ok { p.SetFaceEntity(npc.nid) }` block and the entire `DEVIATION NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` comment block.)

#### Step 1.5: Run the full test set and verify everything passes

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetInteractionPlayerTargetSetsFaceEntity|TestSetInteractionNpcTargetSetsFaceEntity|TestSetInteractionLocTargetDoesNotSetFaceEntity|TestSetInteractionFaceEntityIdempotent|TestProcessInteractionInRangeFacesTarget' -v
```

Expected: all 5 PASS.

#### Step 1.6: Run the entire `modules/world` and `pkg/rsbuf` suites

Per `verify_implementer_claims.md` (cross-package green). Also run `pkg/script` because `npc_script.go` and `player_script.go` indirectly route through SetInteraction adapters.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/rsbuf/... ./pkg/script/...
```

Expected: all green. If any red, halt and diagnose root cause (do NOT mark as "pre-existing" without a HEAD~1 verification per `verify_implementer_claims.md`).

#### Step 1.7: Commit

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-41 — Player.SetInteraction face-entity TS-fidelity

Port TS PathingEntity.setInteraction L530-541 (faceEntity dispatch with
idempotency check) into Player.SetInteraction. Mirrors the in-codebase
Npc.SetInteraction template (npc_interaction.go:651-666).

- *Player target: faceEntity = slot + 32768
- *Npc target:    faceEntity = nid
- *Loc/*Obj:      deferred (NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ)

Deletes the now-redundant contact-time SetFaceEntity(npc.nid) write in
processInteraction and the NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK
deviation comment block. Behavioral effect: clicker's
player_facingmask updates on the click tick (matching TS) instead of
the contact-tick (or never, for *Player targets).

Closes NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK and the pre-existing
*Player→*Npc contact-time-write divergence (never numbered before).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

#### Step 1.8: Verify commit content matches stated diff

Per `implementer_commit_content_verify.md`:

```bash
git show HEAD --stat
git status
```

Expected:
- `git show HEAD --stat` shows exactly 2 files changed: `modules/world/interaction.go` and `modules/world/interaction_test.go`. Insertions ≈ 90 (4 new tests ≈ 75 + dispatch ≈ 15); deletions ≈ 20 (contact-time block ≈ 4, deviation comment ≈ 12, two assertion blocks ≈ 6).
- `git status` shows clean (no leftover staged or unstaged changes).

If the diff scope mismatches, halt and report — do not amend.

---

## Post-task: NAI-41 close-commit memory trailer

Per `close_commit_memory_trailer.md`: NAI-41 close commit (Step 1.7 above) should include `Closes memory:` trailer if any memory entries were directly applied during the implementation cycle. For this minimal sub-spec, the relevant memory entries are read-only references (`plan_grep_helper_patterns.md`, `dead_api_polish.md`, etc.) — no new memory production at task close. The post-close handoff (per `post_task_handoff.md`) updates `nai_followups.md` with the NAI-41 close section + the new `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` deviation entry.

---

## Self-review checklist

**Spec coverage:**
- §4.1 (in scope) item 1 — type-switch port → Step 1.3.
- §4.1 item 2 — delete contact-time block → Step 1.4.
- §4.1 item 3 — delete deviation comment → Step 1.4 (same edit).
- §4.1 item 4 — inline TS idempotency check → Step 1.3 code block (`if p.faceEntity != X`).
- §5.1 new function body → Step 1.3 code block (verbatim).
- §5.2 new processInteraction contact branch → Step 1.4 code block.
- §6.1 closes NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK + pre-existing timing → covered by Step 1.4 deletion + Step 1.3 *Npc branch + commit message.
- §6.2 new NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ → Step 1.3 default-arm comment.
- §7 test #1 (Player target) → Step 1.1 test 1.
- §7 test #2 (Npc target) → Step 1.1 test 2.
- §7 test #3 (Loc target) → Step 1.1 test 3.
- §7 test #4 (idempotency) → Step 1.1 test 4.
- §7 existing-test assertion delete → Step 1.1 second block.

**Placeholder scan:** no TBDs/TODOs/"add appropriate handling" patterns. All code blocks are runnable as-is.

**Type / signature consistency:**
- `Player.SetInteraction` signature unchanged — no caller updates needed.
- `entity` interface (`Slot()`, `Coords()`, `IsValid()`) — `*Player`, `*Npc`, `*entitypkg.Loc` all already satisfy.
- `MaskFaceEntity` is the package-local alias at `modules/world/masks.go:8` (= 4) — same constant used by the existing test at the file's pre-port line 146.
- Test fixture types: `entitypkg.NewLoc(0, 105, 100, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)` matches the existing `TestProcessInteractionRoutesToApBranch` pattern at `interaction_test.go:384`.

**Plan-runnable test fixtures (per `plan_runnable_test_fixtures.md`):** mentally traced all 4 new tests:
- `newTestPlayer` returns `slot=-1, faceEntity=-1, masks=0, entitymask=rsbuf.MaskFaceEntity`. Setting `other.slot = 5` overrides default.
- `makeInteractionNpc(t, s, 7, 100, 100, 0)` produces `npc.nid = 7` (per nid assignment at `interaction_test.go:24`).
- `makeInteractionPlayer(t, s, 99, 100, 0)` returns a wired `*Player, wait func()`.
- `entitypkg.NewLoc(0, 105, 100, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)` matches existing usage; the level-0/x-105/z-100 values don't matter for this test (we never call `processInteraction`).
- All four tests run synchronously; only `TestSetInteractionNpcTargetSetsFaceEntity` and `TestSetInteractionFaceEntityIdempotent` need the `wait()` defer because they call `makeInteractionPlayer` (which kicks off `drainConn`); `TestSetInteractionPlayerTargetSetsFaceEntity` uses `newTestPlayer` for the target so no second drain is needed; `TestSetInteractionLocTargetDoesNotSetFaceEntity` uses `makeInteractionPlayer` for the player so it includes `defer wait()`.

**Cadence:** single TDD task, sized for compressed-cadence-with-review. One feat commit; optional polish commit if final review surfaces stale comments. Two-stage review (spec compliance → code quality) per `runescript_cadence.md`.
