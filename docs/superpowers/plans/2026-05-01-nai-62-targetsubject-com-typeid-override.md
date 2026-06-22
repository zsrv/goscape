# NAI-62 — `targetSubject.com → typeId` dispatch override Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.getOpTrigger` / `getApTrigger` `targetSubject.com → typeId` override (Player.ts:993-997, 1027-1031) to all 8 player-side trigger-lookup sites, fix the OpPlayerU producer-side `useObj` drop, and canonicalise `SetInteraction com=0 → -1` per TS PathingEntity.ts:520 truthy.

**Architecture:** Two-bundle sub-spec. **Bundle 1** is foundation: storage canonicalisation in `SetInteraction`, a new `resolveTriggerTypeId` helper (defined but unwired), the OpPlayerU producer fix, and 4 tests. **Bundle 2** is consumer fan-out: 8 callsite edits in `interaction_trigger.go` + `player_interaction_trigger.go` that thread the helper into every player-side `GetByTrigger` call, plus 8 per-site override tests. B2 dispatches against B1 at HEAD; all work commits directly to `main` (no worktree, no feature branch).

**Tech Stack:** Go 1.26+. Existing test helpers: `makeOpLocFixture` / `makeOpNpcFixture` / `makeOpPlayerFixture` / `makeApTriggerFixture` (existing fixtures), `newNoopScriptFile` (test/script fixture), `buildNpcSayScript` (NPC-Say marker script), `newTestPlayer` (returns Player + conn). Build/test commands prefix all `go` invocations with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`.

---

## Pre-Bundle: Controller pre-flight

Run **before dispatching Bundle 1**:

```bash
git status                              # must be clean
git rev-parse HEAD                      # record SHA — used as B1's parent
grep -n "p\.targetSubject\.com = com" modules/world/interaction.go    # must hit line 59
grep -n "targetOpPlayerU, -1)" modules/world/handler_op_player.go     # must hit line 216
grep -n "GetByTrigger" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
# expected: 6 hits in interaction_trigger.go + 2 hits in player_interaction_trigger.go = 8 hits
```

If any line number drifts, halt and update the plan before dispatching.

---

# Bundle 1: Foundation

Single-implementer dispatch. Single commit at end of bundle.

## Task 1.1: SetInteraction com=0 → -1 canonicalisation

**Files:**
- Modify: `modules/world/interaction.go:59`
- Test: `modules/world/interaction_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/interaction_test.go` (after `TestSetInteractionPassesMinusOneForNonComOps`, around line 442):

```go
// TestSetInteractionComZeroCanonicalisation verifies that SetInteraction
// canonicalises com=0 to com=-1 at storage time, matching TS truthy
// PathingEntity.ts:520: `targetSubject.com = com ? com : -1`. NAI-62: this
// boundary affects OpPlayerU's useObj=0 case post-producer-fix.
func TestSetInteractionComZeroCanonicalisation(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.targetSubject.com = 999 // stale prior value

	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 1, 0)
	if p.targetSubject.com != -1 {
		t.Errorf("com=0 canonicalisation: got %d, want -1 (TS PathingEntity.ts:520)", p.targetSubject.com)
	}

	// Sanity: positive com is preserved
	p.SetInteraction(InteractionEngine, fake, 1, 12345)
	if p.targetSubject.com != 12345 {
		t.Errorf("positive com: got %d, want 12345", p.targetSubject.com)
	}

	// Sanity: -1 sentinel is preserved
	p.SetInteraction(InteractionEngine, fake, 1, -1)
	if p.targetSubject.com != -1 {
		t.Errorf("-1 sentinel: got %d, want -1", p.targetSubject.com)
	}
}
```

`fakeEntity` is defined at `interaction_test.go:445`; `newTestPlayer` at `player_test.go:15`. Both already in scope.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetInteractionComZeroCanonicalisation -v`
Expected: FAIL — `com=0 canonicalisation: got 0, want -1` (current code stores `com` as-is at interaction.go:59).

- [ ] **Step 3: Apply the fix**

Edit `modules/world/interaction.go` line 59. Replace:

```go
	p.targetSubject.com = com
```

with:

```go
	// TS PathingEntity.ts:520 truthy: com=0 → -1. Lookup-side checks
	// use != -1, so canonicalising at storage means a single sentinel
	// reaches resolveTriggerTypeId.
	if com == 0 {
		p.targetSubject.com = -1
	} else {
		p.targetSubject.com = com
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetInteractionComZeroCanonicalisation -v`
Expected: PASS.

- [ ] **Step 5: Run the full `modules/world` test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Existing `TestSetInteractionStoresComField` (interaction_test.go:414) passes a positive `com=12345`; `TestSetInteractionPassesMinusOneForNonComOps` passes `com=-1`. Both still green post-fix because only the `com==0` branch flips.

- [ ] **Step 6: Hold the commit**

Do NOT commit yet — Bundle 1 commits all of 1.1–1.4 atomically at the end of Task 1.4.

---

## Task 1.2: resolveTriggerTypeId helper + unit test

**Files:**
- Modify: `modules/world/interaction_trigger.go` (add helper after `apObjTriggerForOp`)
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestResolveTriggerTypeId pins the typeId override semantics ported from
// TS Player.getOpTrigger:993-995 / getApTrigger:1027-1029. NAI-62.
func TestResolveTriggerTypeId(t *testing.T) {
	p := &Player{}

	// com == -1: returns the default typeId.
	p.targetSubject.com = -1
	if got := resolveTriggerTypeId(p, 42); got != 42 {
		t.Errorf("com=-1: got %d, want 42 (default)", got)
	}

	// com != -1: returns com (override wins).
	p.targetSubject.com = 7777
	if got := resolveTriggerTypeId(p, 42); got != 7777 {
		t.Errorf("com=7777: got %d, want 7777 (override)", got)
	}

	// Boundary: com == -1 with default == -1 still returns -1.
	p.targetSubject.com = -1
	if got := resolveTriggerTypeId(p, -1); got != -1 {
		t.Errorf("com=-1 default=-1: got %d, want -1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveTriggerTypeId -v`
Expected: FAIL — `undefined: resolveTriggerTypeId` (compile error).

- [ ] **Step 3: Add the helper**

Edit `modules/world/interaction_trigger.go`. Locate the `apObjTriggerForOp` function (around line 433-444). Immediately AFTER it, insert:

```go
// resolveTriggerTypeId mirrors the typeId override in TS Player.getOpTrigger
// (Player.ts:993-995) and Player.getApTrigger (Player.ts:1027-1029): when
// targetSubject.com is set (≠ -1), it overrides the entity's typeId for
// trigger lookup. categoryId is NEVER overridden — the override flips only
// the type slot. Used by every player-side fire helper to thread spellCom
// (T-handlers) and useObj (OpPlayerU) into script-key resolution.
//
// Storage convention: SetInteraction canonicalises com=0 → -1 (matching
// TS PathingEntity.ts:520 truthy), so the != -1 check here behaves
// identically to TS !== -1 even at the com=0 boundary.
func resolveTriggerTypeId(p *Player, defaultTypeId int) int {
	if p.targetSubject.com != -1 {
		return p.targetSubject.com
	}
	return defaultTypeId
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveTriggerTypeId -v`
Expected: PASS.

- [ ] **Step 5: Hold the commit**

---

## Task 1.3: OpPlayerU producer fix

**Files:**
- Modify: `modules/world/handler_op_player.go:216`, `:130-143` (doc trailer)
- Test: `modules/world/handler_op_player_test.go:303-337` (existing test update); append new test

- [ ] **Step 1: Update the existing happy-path test to assert `com == useObj`**

In `modules/world/handler_op_player_test.go`, replace the doc-comment at lines 303-305:

```go
// TestHandleOpPlayerU_HappyPath — valid OPPLAYERU request sets target,
// targetOp = targetOpPlayerU, targetSubject.com = -1 (useCom discarded),
// lastUseItem = useObj, lastUseSlot = useSlot, kind = Engine.
```

with:

```go
// TestHandleOpPlayerU_HappyPath — valid OPPLAYERU request sets target,
// targetOp = targetOpPlayerU, targetSubject.com = useObj (NAI-62: useObj
// threaded through SetInteraction for trigger-lookup override per TS
// OpPlayerUHandler.ts:77 + Player.ts:993-995), lastUseItem = useObj,
// lastUseSlot = useSlot, kind = Engine.
```

And replace lines 335-337:

```go
	if clicker.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1", clicker.targetSubject.com)
	}
```

with:

```go
	if clicker.targetSubject.com != useObj {
		t.Errorf("targetSubject.com: got %d, want %d (useObj — NAI-62 producer fix per TS OpPlayerUHandler.ts:77)", clicker.targetSubject.com, useObj)
	}
```

- [ ] **Step 2: Append a new test for the useObj=0 canonicalisation flow**

Append to `modules/world/handler_op_player_test.go` (immediately after `TestHandleOpPlayerU_HappyPath`):

```go
// TestHandleOpPlayerU_UseObjZeroCanonicalisation pins the TS truthy quirk
// (PathingEntity.ts:520) end-to-end: when useObj=0 from the wire, the
// producer threads it through SetInteraction, which canonicalises 0 → -1.
// NAI-62 — verifies §3.1 + §3.2 compose correctly.
func TestHandleOpPlayerU_UseObjZeroCanonicalisation(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const (
		invType = 93
		useCom  = 149
		useObj  = 0 // <-- TS truthy boundary
		useSlot = 3
	)

	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		useCom: {RootLayer: useCom, Usable: true},
	})
	clicker.tabs[0] = useCom

	seedOpPlayerUInv(t, s, clicker, invType, useCom, useObj, useSlot)

	if err := handleOpPlayerU(clicker, opPlayerUPayload(other.slot, useObj, useSlot, useCom)); err != nil {
		t.Fatalf("handleOpPlayerU: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (useObj=0 canonicalised per TS PathingEntity.ts:520)", clicker.targetSubject.com)
	}
	if clicker.lastUseItem != 0 {
		t.Errorf("lastUseItem: got %d, want 0 (useObj is preserved on lastUseItem; only com is canonicalised)", clicker.lastUseItem)
	}
}
```

`seedOpPlayerUInv` is defined at `handler_op_player_test.go:288-301`. It calls `inv.Items[useSlot] = &inventory.Item{Id: useObj, Count: 1}` — with `useObj=0`, this seeds slot with `Id=0`. Then `handleOpPlayerU` calls `inv.HasAt(useSlot, useObj)` which compares the stored Item.Id (0) against the parameter (0) — matches, so the gate passes. (If `HasAt` rejects `Id=0` as a sentinel, the test will fail at `len(payload) < 8` or earlier; in that case fall back: implementer adjusts the fixture or splits the test into two — `useObj=0` flow fails the inv check, but `SetInteraction(0)` canonicalisation is already pinned by Task 1.1's test, so this test is a defense-in-depth pin only. Implementer decides; both outcomes prove the producer + canonicalisation path works.)

- [ ] **Step 3: Run both tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayerU_HappyPath|TestHandleOpPlayerU_UseObjZeroCanonicalisation" -v`

Expected: BOTH FAIL.
- `TestHandleOpPlayerU_HappyPath`: `targetSubject.com: got -1, want 1511 (useObj — NAI-62 producer fix …)` — current handler_op_player.go:216 passes -1 to SetInteraction.
- `TestHandleOpPlayerU_UseObjZeroCanonicalisation`: `target: got <nil>, want other` (current passes -1 to SetInteraction so the canonicalisation path isn't even exercised), OR may pass on `com=-1` if `HasAt` accepts `Id=0`. Either way, document the pre-fix state in the run.

- [ ] **Step 4: Apply the producer fix**

Edit `modules/world/handler_op_player.go` line 216. Replace:

```go
	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
```

with:

```go
	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, useObj)
```

- [ ] **Step 5: Update the doc-comment trailer**

Edit `modules/world/handler_op_player.go` lines 130-143. The current "On success" trailer ends:

```go
// On success: ClearPendingAction (after rsbuf.HasPlayer reject, before members check)
// → snapshot p.lastUseItem=useObj, p.lastUseSlot=useSlot →
// SetInteraction(Engine, other, targetOpPlayerU, -1).
```

Replace the `SetInteraction(...)` line at the end with:

```go
// SetInteraction(Engine, other, targetOpPlayerU, useObj) (NAI-62: useObj
// threaded for trigger-lookup override per TS OpPlayerUHandler.ts:77 +
// Player.ts:993-995; useObj=0 canonicalised to com=-1 by SetInteraction
// per TS PathingEntity.ts:520).
```

- [ ] **Step 6: Run both tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayerU_HappyPath|TestHandleOpPlayerU_UseObjZeroCanonicalisation" -v`
Expected: BOTH PASS.

- [ ] **Step 7: Hold the commit**

---

## Task 1.4: Update doc-comments + Bundle 1 commit

**Files:**
- Modify: `modules/world/interaction.go:45-55` (SetInteraction doc)
- Modify: `modules/world/player.go:104-110` (targetSubject field doc)

- [ ] **Step 1: Update SetInteraction doc-comment**

Edit `modules/world/interaction.go` lines 45-48 (the existing `SetInteraction` doc-comment). Current:

```go
// SetInteraction anchors the interaction state machine on a target entity.
// For OpLocT the com parameter carries the spell-component ID; for OpLocU
// pass -1 (item tracking uses lastUseItem/lastUseSlot instead). For
// OpLoc1..5 and OpNpc1..5, callers pass -1.
```

Replace with:

```go
// SetInteraction anchors the interaction state machine on a target entity.
// The com parameter carries:
//   - OpLocT/OpNpcT/OpObjT/OpPlayerT: spellCom (UI component ID of the spell).
//   - OpPlayerU: useObj (the obj/item ID used on the target player; NAI-62
//     producer fix per TS OpPlayerUHandler.ts:77).
//   - OpLoc1..5 / OpNpc1..5 / OpObj1..5 / OpLocU / OpNpcU / OpObjU: -1.
// Storage canonicalises com=0 → -1 (NAI-62, matching TS truthy
// PathingEntity.ts:520) so the lookup-side != -1 override check in
// resolveTriggerTypeId behaves identically to TS !== -1.
```

- [ ] **Step 2: Update targetSubject field doc-comment**

Edit `modules/world/player.go` lines 104-110. Current:

```go
	// targetSubject snapshots the identity of the interaction target at
	// click time. Components:
	//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
	//     lifecycle gate (set by OpLoc handlers after SetInteraction).
	//   com — spell-component ID for OpLocT; -1 for OpLoc1..5 and OpLocU.
	//     Scripts read via ActivePlayer.TargetSubjectCom() (S6m).
	// S6m: com field resurrected from S6j shrink to carry spellCom.
```

Replace with:

```go
	// targetSubject snapshots the identity of the interaction target at
	// click time. Components:
	//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
	//     lifecycle gate (set by OpLoc handlers after SetInteraction).
	//   com — payload ID:
	//     - OpLocT/OpNpcT/OpObjT/OpPlayerT: spellCom (UI component ID).
	//     - OpPlayerU: useObj (item ID; NAI-62 producer fix).
	//     - OpLoc1..5 / OpNpc1..5 / OpObj1..5 / OpLocU / OpNpcU / OpObjU: -1.
	//     Canonicalised by SetInteraction: com=0 → -1 (NAI-62, matching TS
	//     PathingEntity.ts:520 truthy). Consumed at trigger lookup via
	//     resolveTriggerTypeId (NAI-62, mirrors TS Player.getOpTrigger:993-995
	//     / getApTrigger:1027-1029) and by scripts via
	//     ActivePlayer.TargetSubjectCom().
```

- [ ] **Step 3: Run vet + full test suite + build**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green, no vet warnings, build succeeds.

- [ ] **Step 4: Commit Bundle 1**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go \
        modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go \
        modules/world/handler_op_player.go modules/world/handler_op_player_test.go \
        modules/world/player.go

git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-62 B1 — targetSubject.com canonicalisation, resolveTriggerTypeId helper, OpPlayerU producer fix

Closes the com=0 → -1 truthy boundary (TS PathingEntity.ts:520) by
canonicalising SetInteraction's com store; introduces the
resolveTriggerTypeId helper that B2 will wire into all 8 player-side
trigger-lookup sites; fixes the OpPlayerU producer-side useObj drop
(TS OpPlayerUHandler.ts:77).

Helper is added but unwired — no observable trigger-dispatch change
in this commit. B2 lands the consumer fan-out.

Tests: TestResolveTriggerTypeId (helper unit, 2+1 boundary branches),
TestSetInteractionComZeroCanonicalisation,
TestHandleOpPlayerU_UseObjZeroCanonicalisation; updated
TestHandleOpPlayerU_HappyPath assertion (com == useObj).

Refs: TS Player.ts:993-997, 1027-1031; PathingEntity.ts:520;
OpPlayerUHandler.ts:77.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Verify commit**

```bash
git --no-pager show --stat HEAD
git --no-pager log --oneline -3
```

Expected: HEAD is the B1 commit, file list matches the `git add` above, parent matches the pre-flight SHA.

---

# Bundle 2: Consumer fan-out

Single-implementer dispatch against B1 at HEAD. Single commit at end of bundle.

## Task 2.0: B2 controller pre-flight

Run before dispatching Task 2.1:

```bash
git status                              # must be clean
git --no-pager log --oneline -2         # confirm B1 is HEAD

# Re-confirm callsite line numbers haven't shifted post-B1.
grep -n "GetByTrigger(trigger," modules/world/interaction_trigger.go
# expected: 6 hits at lines 76, 146, 317, 377, 478, 535 (pre-B2)

grep -n "GetByTrigger(trigger," modules/world/player_interaction_trigger.go
# expected: 2 hits at lines 55, 86 (pre-B2)

grep -n "func resolveTriggerTypeId" modules/world/interaction_trigger.go
# must hit exactly 1 line (B1 added the helper after apObjTriggerForOp)
```

If any line number drifts or `resolveTriggerTypeId` is missing, halt and re-derive from HEAD before dispatching Task 2.1.

---

## Per-site override test strategy

The 8 sites have different observable post-states between "script ran" and "no script found". Each task below uses one of three assertion strategies:

| Strategy | Helpers using it | Signal |
|---|---|---|
| **NPC_SAY marker** | `fireOpTriggerNpc`, `fireApTriggerNpc` | `npc.sayText` after `buildNpcSayScript` runs |
| **Loc/Obj absence-pin (Op)** | `fireOpTriggerLoc`, `fireOpTriggerObj` | `bytes.Contains(drained, []byte("Nothing interesting happens.")) == false` |
| **apRange-preservation (Ap)** | `fireApTriggerLoc`, `fireApTriggerObj`, `fireApTriggerPlayer` | `p.apRange != -1` (no-script path sets `apRange = -1`) |
| **OpMes marker (Player)** | `fireOpTriggerPlayer`, `fireApTriggerPlayer` | drain target's conn; assert MARKER substring (script's Self = target) |

Each per-site test registers ONLY the override-keyed script (NOT a default-keyed one). Pre-fix dispatch falls through to the default-typeId lookup, finds nothing, takes the no-script-found branch — emits "Nothing interesting happens." / sets `apRange=-1` / clears interaction without writing the marker. Post-fix dispatch hits the override-keyed script, runs it, observable side-effect appears.

This gives a clean RED-pre-fix / GREEN-post-fix gate for every site.

---

## Task 2.1: fireOpTriggerNpc — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:76`, doc trailer near line 70
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerNpcOverridesTypeIdFromTargetSubjectCom pins NAI-62: when
// p.targetSubject.com != -1, fireOpTriggerNpc must look up the script at
// (trigger, com, …) instead of (trigger, npc.typeId, …). TS Player.getOpTrigger
// (Player.ts:993-995).
func TestFireOpTriggerNpcOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()

	// Register ONLY at the override key. If fireOpTriggerNpc still uses
	// npc.typeId for lookup (pre-fix), this script is unreachable and
	// npc.sayText stays empty.
	const overrideTypeId = 7777
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpc1, overrideTypeId, "opnpc1-override-fired"))

	p.SetInteraction(InteractionEngine, npc, 1, overrideTypeId)
	p.interacted = true

	tryFireOpTrigger(p)

	if string(npc.sayText) != "opnpc1-override-fired" {
		t.Errorf("npc.sayText: got %q, want %q (override script must run because targetSubject.com=%d overrides default npc.typeId=%d per TS Player.ts:993-995)",
			npc.sayText, "opnpc1-override-fired", overrideTypeId, npc.typeId)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerNpcOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL — `npc.sayText: got "", want "opnpc1-override-fired"` (override script unreachable because `interaction_trigger.go:76` still uses `npc.typeId`).

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 76. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom / useObj override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerNpcOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Existing `TestFireOpTriggerNpcFiresOpNpcTTrigger` (interaction_trigger_test.go:885) registers the script at `npc.typeId` AND seeds `targetSubject.com=7777` — post-fix the override flips lookup to typeId=7777, but no script is registered there → silent clear path. **This test will FAIL post-fix without an update.** Implementer must update the test:

In `interaction_trigger_test.go`, locate `TestFireOpTriggerNpcFiresOpNpcTTrigger` (line 885) and `TestFireOpTriggerNpcFiresOpNpcUTrigger` (line 908). Both call `p.SetInteraction(...,  7777)` and register the script at `npc.typeId=0`. Update them so the script registration matches the override:

```go
// Line 888 (TestFireOpTriggerNpcFiresOpNpcTTrigger): change typeID arg from 0 → 7777
s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpcT, 7777, "opnpct-fired"))

// Line 911 (TestFireOpTriggerNpcFiresOpNpcUTrigger): SetInteraction passes -1 for com,
// so the override does not fire — typeID stays at 0 (the default npc.typeId for
// makeOpNpcFixture). Verify that fixture's npc.typeId == 0 first; if so, the
// existing call buildNpcSayScript(..., 0, ...) is correct. NO CHANGE needed for
// the OpNpcU test (com=-1, no override).
```

After updates, re-run `go test ./modules/world/...` and confirm all green.

- [ ] **Step 6: Hold the commit**

---

## Task 2.2: fireOpTriggerLoc — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:146`, doc trailer near line 140
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerLocOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Strategy: register override-keyed script only. Pre-fix takes the
// "Nothing interesting happens." default-op path; post-fix runs the
// override script (no message emitted because the script is OpReturn-only).
func TestFireOpTriggerLocOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, loc, cc := makeOpLocTriggerFixture(t)

	const overrideTypeId = 7778
	// Override targetSubject.com to the sentinel; SetInteraction was already
	// called by the fixture with op=1, com=-1, so we must overwrite directly
	// rather than re-call SetInteraction (which would also reset the
	// loc-identity fields).
	p.targetSubject.com = overrideTypeId

	// Register the no-op script at the override key only.
	sf := newNoopScriptFile(t, script.TriggerOpLoc1, overrideTypeId, -1)
	s.scriptProvider.Register(sf)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("drained bytes: contained \"Nothing interesting happens.\" — override should have run override-keyed script for targetSubject.com=%d (default loc.Type()=%d), got %x",
			overrideTypeId, loc.Type(), got)
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after override fire")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerLocOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL — drained bytes contains `"Nothing interesting happens."` because the helper looks up `loc.Type()` (default), not `7778`.

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 146. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerLocOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Existing `TestFireOpTriggerLocFiresOpLocTTrigger` (interaction_trigger_test.go:639) — same audit as Task 2.1 Step 5. The test calls `p.SetInteraction(... loc, targetOpLocT, 7777)` (com=7777) and registers the script at `loc.Type()`. Post-fix the override flips lookup to typeId=7777 → script unreachable → "Nothing interesting happens." path. **Update**: change line 647 from:

```go
sf := newNoopScriptFile(t, script.TriggerOpLocT, loc.Type(), -1)
```

to:

```go
sf := newNoopScriptFile(t, script.TriggerOpLocT, 7777, -1)
```

(7777 = the spellCom passed to SetInteraction at line 641.)

For `TestFireOpTriggerLocFiresOpLocUTrigger` (line 662): SetInteraction is called with `com=-1`, so override does not fire. Existing registration at `loc.Type()` remains correct. NO CHANGE.

- [ ] **Step 6: Hold the commit**

---

## Task 2.3: fireApTriggerNpc — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:317`, doc trailer near line 311
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerNpcOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Same NPC_SAY marker strategy as TestFireOpTriggerNpcOverrides… but at
// approach distance (apRange-eligible).
func TestFireApTriggerNpcOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()

	const overrideTypeId = 7779
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApNpc1, overrideTypeId, "apnpc1-override-fired"))

	p.SetInteraction(InteractionEngine, npc, 1, overrideTypeId)
	p.interacted = true

	tryFireApTrigger(p)

	if string(npc.sayText) != "apnpc1-override-fired" {
		t.Errorf("npc.sayText: got %q, want %q (override script must run because targetSubject.com=%d overrides default npc.typeId=%d per TS Player.ts:1027-1029)",
			npc.sayText, "apnpc1-override-fired", overrideTypeId, npc.typeId)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerNpcOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL.

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 317. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerNpcOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all green. Audit existing `TestFireApTriggerNpc*` tests in interaction_trigger_test.go — if any call `SetInteraction(... com=K)` with `K != -1` and register the script at `npc.typeId` (not `K`), they need the same fix as Task 2.1 Step 5. Re-check line numbers via:

```bash
grep -n "buildNpcSayScript.*ApNpc\|fireApTriggerNpc" modules/world/interaction_trigger_test.go
```

- [ ] **Step 6: Hold the commit**

---

## Task 2.4: fireApTriggerLoc — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:377`, doc trailer near line 371
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerLocOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Strategy: register override-keyed script only. Pre-fix takes the
// no-AP-script path which sets p.apRange = -1; post-fix runs the
// override script and apRange is preserved (>0).
func TestFireApTriggerLocOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	const overrideTypeId = 7780
	p.targetSubject.com = overrideTypeId

	// Register the no-op script at the override key only.
	sf := newNoopScriptFile(t, script.TriggerApLoc1, overrideTypeId, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have run override-keyed script for targetSubject.com=%d (default loc.Type()=%d)",
			overrideTypeId, loc.Type())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerLocOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL — `apRange: got -1` because the helper looks up `loc.Type()` (default).

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 377. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerLocOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all green. Audit existing `TestFireApTriggerLoc*` tests:

```bash
grep -n "TestFireApTriggerLoc" modules/world/interaction_trigger_test.go
```

If any test calls `SetInteraction(... com=K)` with `K != -1` and registers the script at `loc.Type()`, update its registration key to `K`. Same fix-up pattern as Task 2.2 Step 5.

- [ ] **Step 6: Hold the commit**

---

## Task 2.5: fireOpTriggerObj — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:478`, doc trailer near line 470
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerObjOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Strategy parallels Task 2.2's Loc absence-pin: register override-keyed
// script only; pre-fix takes the "Nothing interesting happens." path;
// post-fix runs the script.
func TestFireOpTriggerObjOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, obj, cc := makeOpObjTriggerFixture(t)

	const overrideTypeId = 7781
	p.targetSubject.com = overrideTypeId

	sf := newNoopScriptFile(t, script.TriggerOpObj1, overrideTypeId, -1)
	s.scriptProvider.Register(sf)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("drained bytes: contained \"Nothing interesting happens.\" — override should have run override-keyed script for targetSubject.com=%d (default obj.Type=%d), got %x",
			overrideTypeId, obj.Type, got)
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after override fire")
	}
}
```

If `makeOpObjTriggerFixture` does not exist, look for `makeOpObjFixture` in `handler_opobj_test.go` and adapt the same way `makeOpLocTriggerFixture` adapts `makeOpLocFixture` (see interaction_trigger_test.go:286-294 for the wrapping pattern). If neither exists, build it inline:

```go
func makeOpObjTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s, p, obj, cc := makeOpObjFixture(t)  // grep for actual signature
	p.SetInteraction(InteractionEngine, obj, 1, -1)
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return s, p, obj, cc
}
```

(Verify `makeOpObjFixture`'s signature before writing this; pre-flight step in Task 2.0 should have captured it, but if not, `grep -n "func makeOpObjFixture" modules/world/handler_opobj_test.go`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerObjOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL.

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 478. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, obj.Type, category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerObjOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Audit existing `TestFireOpTriggerObj*` tests; same `K != -1` registration-key-update pattern as Task 2.2 Step 5 if applicable.

- [ ] **Step 6: Hold the commit**

---

## Task 2.6: fireApTriggerObj — wire helper + override test

**Files:**
- Modify: `modules/world/interaction_trigger.go:535`, doc trailer near line 528
- Test: `modules/world/interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Strategy parallels Task 2.4's apRange-preservation pin.
func TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, obj := makeApObjTriggerFixture(t)
	_ = obj

	const overrideTypeId = 7782
	p.targetSubject.com = overrideTypeId

	sf := newNoopScriptFile(t, script.TriggerApObj1, overrideTypeId, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have run override-keyed script for targetSubject.com=%d", overrideTypeId)
	}
}
```

If `makeApObjTriggerFixture` does not exist, build it inline using `makeOpObjFixture` + AP-distance positioning (see `makeApTriggerFixture` at interaction_trigger_test.go:417 for the Loc pattern; same shape but obj coords).

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL.

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/interaction_trigger.go` line 535. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, obj.Type, category)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Audit existing `TestFireApTriggerObj*` tests.

- [ ] **Step 6: Hold the commit**

---

## Task 2.7: fireOpTriggerPlayer — wire helper + override test

**Files:**
- Modify: `modules/world/player_interaction_trigger.go:55`, doc trailer near line 41
- Test: `modules/world/player_interaction_trigger_test.go` (append)

The Player-target case needs a target-side observable since the script's `Self` is the target player (not the clicker). Use an `OpMes` marker script that writes a unique string to `target.MessageGame()`; drain target's conn.

- [ ] **Step 1: Add target-conn helper if not already present**

Check `modules/world/handler_op_player_test.go:19-37`'s `makeOpPlayerFixture` — it currently creates `other` via `newTestPlayer(t)` but discards `other`'s conn (assigns to `_`).

Add a new fixture variant. Edit `modules/world/handler_op_player_test.go` (or a shared test file) to add:

```go
// makeOpPlayerFixtureWithBothConns is makeOpPlayerFixture but also returns
// `other`'s conn so tests can drain target-side traffic (e.g. OpMes
// MessageGame writes for trigger-dispatch verification — NAI-62).
func makeOpPlayerFixtureWithBothConns(t *testing.T) (*Server, *Player, *Player, net.Conn, net.Conn) {
	t.Helper()
	s := newTestServer(t)

	clicker, cc := newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	clicker.slot = 1
	s.players[1] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	other, cc2 := newTestPlayer(t)
	other.client.server = s
	other.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	other.slot = 2
	s.players[2] = other
	s.rsbuf.AddPlayer(int32(other.slot))

	return s, clicker, other, cc, cc2
}
```

Place it adjacent to `makeOpPlayerFixture` at `handler_op_player_test.go:19`.

- [ ] **Step 2: Add an OpMes marker-script helper if not already present**

Check `modules/world/interaction_trigger_test.go` for an existing `OpMes` script builder (parallels `buildNpcSayScript` for player-side observables). If absent, add to `modules/world/interaction_trigger_test.go`:

```go
// buildPlayerMesScript produces a tiny [push <text>, MES, RETURN] script
// keyed at (trigger, typeID)-specific. The MES opcode calls Self.MessageGame
// (handlers.go:616-622), so for Player-target triggers (Self == target) the
// emitted text appears on target's conn. NAI-62 per-site override pinning.
func buildPlayerMesScript(trigger script.ServerTriggerType, typeID int, text string) *script.ScriptFile {
	key := script.LookupKeyForType(trigger, typeID)
	return &script.ScriptFile{
		Name:             "[opplayer1,test]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{text, "", ""},
		InstructionCount: 3,
	}
}
```

(The `OpMes` constant is at `pkg/script/opcode.go` — grep `grep -n "OpMes\b" pkg/script/opcode.go` to confirm before referencing.)

- [ ] **Step 3: Write the failing override test**

Append to `modules/world/player_interaction_trigger_test.go`:

```go
// TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Player-target script's Self == target, so MES opcode emits MessageGame
// on target's conn. Pre-fix lookup uses (trigger, -1, -1); override
// registers at (trigger, K, -1) which is unreachable → no MessageGame
// on target's conn. Post-fix → script runs → marker appears.
func TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, _, cc2 := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7783
	const marker = "opplayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerOpPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc2)
	fireOpTriggerPlayer(clicker, s, other)
	other.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from target conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d (default Player-target lookup typeId=-1), got %x",
			marker, overrideTypeId, got)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL — marker substring missing because `player_interaction_trigger.go:55` looks up `(trigger, -1, -1)`.

- [ ] **Step 5: Wire the helper at the callsite**

Edit `modules/world/player_interaction_trigger.go` line 55. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, -1, -1)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — useObj override default (-1) when set.
	// Player has no NpcType/LocType/ObjType counterpart in TS so the
	// default typeId is -1 (matches TS's getOpTrigger early skip of the
	// type-fetching if-block when target is a Player).
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 7: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all green. Audit existing `TestFireOpTriggerPlayer*` tests in `player_interaction_trigger_test.go`. Any test that pre-fix passed because `(trigger, -1, -1)` matched its registered script will now flip to lookup `(trigger, com, -1)` if `targetSubject.com != -1`. If any existing test sets `targetSubject.com` to a non-(-1) value AND registers the script at the global-tier (typeId=-1), it must be updated to register at `(trigger, com, -1)`. Re-run on each fix.

- [ ] **Step 8: Hold the commit**

---

## Task 2.8: fireApTriggerPlayer — wire helper + override test

**Files:**
- Modify: `modules/world/player_interaction_trigger.go:86`, doc trailer near line 70
- Test: `modules/world/player_interaction_trigger_test.go` (append)

- [ ] **Step 1: Write the failing override test**

Append to `modules/world/player_interaction_trigger_test.go`:

```go
// TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Same OpMes marker strategy as Task 2.7; AP variant. Also asserts
// p.apRange != -1 as a secondary signal (no-script path sets apRange = -1
// per fireApTriggerPlayer:88).
func TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, _, cc2 := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7784
	const marker = "applayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerApPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc2)
	fireApTriggerPlayer(clicker, s, other)
	other.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from target conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d, got %x",
			marker, overrideTypeId, got)
	}
	if clicker.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have prevented the no-script path")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom -v`
Expected: FAIL — both `apRange == -1` and missing marker.

- [ ] **Step 3: Wire the helper at the callsite**

Edit `modules/world/player_interaction_trigger.go` line 86. Current:

```go
	sf := srv.scriptProvider.GetByTrigger(trigger, -1, -1)
```

Replace with:

```go
	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — useObj override default (-1) when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green. Audit existing `TestFireApTriggerPlayer*` tests; same pattern as Task 2.7 Step 7.

- [ ] **Step 6: Hold the commit**

---

## Task 2.9: B2 cross-foot + commit

- [ ] **Step 1: Cross-foot the 8 callsites**

Run:

```bash
grep -n "GetByTrigger(trigger," modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
```

Expected: 8 hits, ALL with `resolveTriggerTypeId(p, …)` as the second argument. If any hit still uses a raw `npc.typeId` / `loc.Type()` / `obj.Type` / `-1` second-arg, it's a missed site — fix it before committing.

- [ ] **Step 2: Run vet + full suite + build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green.

- [ ] **Step 3: Commit Bundle 2**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go \
        modules/world/player_interaction_trigger.go modules/world/player_interaction_trigger_test.go \
        modules/world/handler_op_player_test.go

git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-62 B2 — wire resolveTriggerTypeId into all 8 player-side trigger lookups

Threads p.targetSubject.com through to script dispatch at the 6 fire
helpers in interaction_trigger.go (Op/Ap × Npc/Loc/Obj) and the 2 in
player_interaction_trigger.go (Op/Ap × Player), matching TS
Player.getOpTrigger:993-997 / getApTrigger:1027-1031.

Behavioural change: spell-on-X clicks (OpLocT/OpNpcT/OpObjT/OpPlayerT)
now key trigger lookup by spellCom; OpPlayerU clicks key by useObj.
Op1-5 / Ap1-5 / U-handler clicks unchanged (com=-1).

Tests: 8 new per-site override tests using three assertion strategies
(NPC_SAY marker for NPC sites, "Nothing interesting happens." absence-pin
for OpLoc/OpObj, apRange-preservation for ApLoc/ApObj/ApPlayer, OpMes
target-conn drain for OpPlayer/ApPlayer). makeOpPlayerFixtureWithBothConns
+ buildPlayerMesScript test helpers added.

Refs: TS Player.ts:993-997, 1027-1031.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Verify commit**

```bash
git --no-pager show --stat HEAD
git --no-pager log --oneline -4
```

Expected: HEAD is the B2 commit, parent is the B1 commit.

---

# NAI-62 Close

## Task 3.0: Close commit + tracker update

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append `## NAI-62 — CLOSED YYYY-MM-DD` block at the end (after the existing NAI-61 close).

- [ ] **Step 1: Append the NAI-62 close block to nai_followups.md**

Read the current end of `nai_followups.md` (the file ends with the NAI-61 close block we landed earlier). Append the new section:

```markdown

## NAI-62 — CLOSED YYYY-MM-DD

Closed by spec/plan `docs/superpowers/specs/2026-05-01-nai-62-targetsubject-com-typeid-override-design.md`
+ `docs/superpowers/plans/2026-05-01-nai-62-targetsubject-com-typeid-override.md`.

Two-bundle sub-spec porting TS Player.getOpTrigger / getApTrigger
override semantics (Player.ts:993-997, 1027-1031) to all 8 player-side
trigger-lookup sites + OpPlayerU producer fix + SetInteraction com=0
canonicalisation.

**B1 (foundation):** SetInteraction com=0 → -1 canonicalisation,
resolveTriggerTypeId helper definition (unwired), OpPlayerU producer fix
(handler_op_player.go:216 -1 → useObj per TS OpPlayerUHandler.ts:77),
4 new tests.

**B2 (consumer fan-out):** 8 callsite edits (Op/Ap × Npc/Loc/Obj/Player)
threading resolveTriggerTypeId into every player-side GetByTrigger;
8 per-site override tests + 2 new test helpers
(makeOpPlayerFixtureWithBothConns, buildPlayerMesScript).

**Behavioural impact:** spell-on-X clicks now key dispatch by spellCom;
OpPlayerU clicks key by useObj. Previously inaccessible spell-keyed and
useObj-keyed scripts in the LostCityRS data pack are now reachable.

**Tracker delta:** retires the "NAI-62 candidate" carve-out under
`## NAI-61 — CLOSED 2026-05-01`.
```

Replace `YYYY-MM-DD` with the actual close date (read via `date +%Y-%m-%d`).

- [ ] **Step 2: Verify there are no stale references to retire**

```bash
rg -n "NAI-62" $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/
rg -n "NAI-62" docs/superpowers/
```

Expected: matches in spec, plan, and the new close block. No other references should exist.

- [ ] **Step 3: Commit the close**

```bash
git --no-pager log --oneline -2

# Stage only the memory file (the spec + plan are already committed).
git add $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md

git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-62 — targetSubject.com → typeId dispatch override

Two-bundle sub-spec:
- B1: SetInteraction com=0 canonicalisation, resolveTriggerTypeId helper, OpPlayerU producer fix
- B2: 8-site consumer fan-out

All player-side trigger-lookup sites now match TS Player.getOpTrigger /
getApTrigger override semantics (Player.ts:993-997, 1027-1031).

Closes memory: nai_followups.md → "## NAI-61 — CLOSED 2026-05-01" →
"NAI-62 candidate" block.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Final verification**

```bash
git --no-pager log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: log shows spec → B1 → B2 → close, all four green.

---

## Self-review notes

**Spec coverage map:**

| Spec section | Plan task |
|---|---|
| §3.1 SetInteraction canonicalisation | Task 1.1, Task 1.4 |
| §3.2 OpPlayerU producer fix | Task 1.3 |
| §3.3 resolveTriggerTypeId helper | Task 1.2 |
| §3.4 8 callsite edits | Tasks 2.1-2.8 |
| §3.5 doc-comment trailers (8 helpers + OpPlayerU) | Tasks 2.1-2.8 (helper-callsite trailers), Task 1.3 (OpPlayerU trailer) |
| §4.1 helper unit test | Task 1.2 |
| §4.2 SetInteraction com=0 test | Task 1.1 |
| §4.3 OpPlayerU producer + useObj=0 tests | Task 1.3 |
| §4.4 8 per-site override tests | Tasks 2.1-2.8 |
| §5.1 B1 commit shape | Task 1.4 |
| §5.2 B2 commit shape (incl. cross-foot grep) | Task 2.9 |
| §5.3 close commit | Task 3.0 |

**Type consistency:**
- `resolveTriggerTypeId(p *Player, defaultTypeId int) int` — same signature in Tasks 1.2, 2.1-2.8.
- `buildPlayerMesScript(trigger script.ServerTriggerType, typeID int, text string) *script.ScriptFile` — same signature in Tasks 2.7, 2.8.
- `makeOpPlayerFixtureWithBothConns(t *testing.T) (*Server, *Player, *Player, net.Conn, net.Conn)` — same signature in Tasks 2.7, 2.8.

**Placeholder scan:** none found.
