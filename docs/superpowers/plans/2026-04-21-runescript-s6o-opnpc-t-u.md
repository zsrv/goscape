# OpNpcT + OpNpcU Handler Implementation Plan (S6o)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close S6n-D1 by wiring OpNpcT (spell-on-NPC, opcode 134, 4-byte payload) and OpNpcU (item-on-NPC, opcode 202, 8-byte payload) end-to-end, mirroring S6m's OpLocT/OpLocU work with NPC-specific simplifications.

**Architecture:** Two tasks. Task 1 adds `targetOpNpcT=8` / `targetOpNpcU=9` sentinels to the existing block in `interaction.go`, extends `apNpcTriggerForOp` with T/U cases, and refactors `fireOpTriggerNpc` to use `apNpcTriggerForOp + 7` (byte-equivalent for 1..5, gains T/U dispatch). Task 2 adds `handleOpNpcT` and `handleOpNpcU` with 5 core validation gates each (delayed, payload, slot-bounds, npc-not-nil/dead, NpcType-exists), wires them at opcodes 134/202, and covers with 12 validation tests.

**Tech Stack:** Go 1.26 (stdlib only). Tests reuse the existing `makeOpNpcFixture` helper from `handler_opnpc_test.go` (S6b).

**Spec reference:** `docs/superpowers/specs/2026-04-21-runescript-s6o-opnpc-t-u-design.md` (commit `8436553`).

**Build commands (per CLAUDE.md):**
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- Test one: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestName -v`
- Vet: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

**Commit policy (per CLAUDE.md):** All commits use `git commit --no-gpg-sign`.

---

## File Structure

| File | Created/Modified | Responsibility | Task |
|---|---|---|---|
| `modules/world/interaction.go` | Modify | Add `targetOpNpcT=8`, `targetOpNpcU=9` to sentinel block | 1 |
| `modules/world/interaction_trigger.go` | Modify | Extend `apNpcTriggerForOp` with T/U cases; refactor `fireOpTriggerNpc` to use helper+7; remove stale S6n-D1 note from `fireApTriggerNpc` | 1 |
| `modules/world/interaction_trigger_test.go` | Modify | Extend `apNpcTriggerForOp` table tests with T/U; add 4 fire-dispatch tests | 1 |
| `modules/world/handler_opnpc.go` | Modify | Add `handleOpNpcT` + `handleOpNpcU` | 2 |
| `modules/world/handler_opnpc_test.go` | Modify | 12 validation tests (6 per handler) | 2 |
| `modules/world/handlers_game.go` | Modify | Wire `gameHandlers[134]=handleOpNpcT`, `gameHandlers[202]=handleOpNpcU` | 2 |

**Existing infrastructure already in place (verify, don't modify):**
- `OPNPCT=134` (4-byte), `OPNPCU=202` (8-byte) — `pkg/io/protocol/game/client/prot.go:65-66`
- `TriggerApNpcT=9`, `TriggerApNpcU=8`, `TriggerOpNpcT=16`, `TriggerOpNpcU=15` — `pkg/script/trigger.go:17-25`
- `Player.lastUseItem int`, `Player.lastUseSlot int` — `player.go:175` (S6m consumer)
- `targetSubject.com int` field — `player.go:182-184` (S6m)
- `Player.TargetSubjectCom() int` accessor — S6m plumbing
- `SetInteraction(kind, target, op, com int)` — S6m signature (unchanged)
- `apNpcTriggerForOp` (S6n) at `interaction_trigger.go:200` — extended, not replaced
- `fireApTriggerNpc` (S6n) at `interaction_trigger.go:~274` — gets T/U for free via helper extension; docstring cleanup only
- `fireOpTriggerNpc` (S6j) at `interaction_trigger.go:60-94` — refactored to use helper+7
- `makeOpNpcFixture(t) (*Server, *Player, *Npc)` — `handler_opnpc_test.go:13` (S6b)
- `p2Payload(slot int) []byte` — `handler_opnpc_test.go:38` (2-byte encoder used by OpNpc1..5 tests)
- NpcType in fixture uses `typeID=0` (not 7); Task 2 tests use this

---

## Task 1: Sentinels + apNpcTriggerForOp T/U Extension + fireOpTriggerNpc Refactor

**Goal:** Extend the NPC AP helper to handle T/U sentinels and refactor `fireOpTriggerNpc` to use it. After this task, T/U trigger dispatch works end-to-end when a handler sets `p.targetOp = 8 or 9` — but no handler does that yet (Task 2 adds them).

**Files:**
- Modify: `modules/world/interaction.go`
- Modify: `modules/world/interaction_trigger.go`
- Modify: `modules/world/interaction_trigger_test.go`

### Step-by-step

- [ ] **Step 1.1: Add `targetOpNpcT` and `targetOpNpcU` sentinels**

In `modules/world/interaction.go`, find the existing sentinel block (around line 24-27):

```go
const (
	targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
)
```

Replace with:

```go
// Sentinel targetOp values for non-op-numbered T/U interaction variants.
// OpLoc1..5/OpNpc1..5 use op = 1..5 (the op slot clicked); T/U variants
// use these sentinels so fireXxxTriggerYyy can dispatch to the correct
// single-trigger (e.g. APLOCT, OPNPCU). The targetOp interpretation is
// per-entity-type: tryFireXxxTrigger type-switches on p.target first,
// then each branch reads targetOp independently. Distinct NPC values
// (8, 9) chosen for clarity — reusing 6, 7 is safe via type-switch
// but less self-documenting.
const (
	targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
	targetOpNpcT = 8 // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU = 9 // APNPCU / OPNPCU dispatch marker (S6o)
)
```

- [ ] **Step 1.2: Run build — expect PASS (sentinels are additive)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: PASS.

- [ ] **Step 1.3: Modify `TestApNpcTriggerForOpValidValues` to add T/U cases**

In `modules/world/interaction_trigger_test.go`, find the existing `TestApNpcTriggerForOpValidValues` table test (from S6n). The current `cases` slice has 5 entries for ops 1..5. Extend it with 2 more rows:

```go
		{1, script.TriggerApNpc1, "OpNpc1"},
		{2, script.TriggerApNpc2, "OpNpc2"},
		{3, script.TriggerApNpc3, "OpNpc3"},
		{4, script.TriggerApNpc4, "OpNpc4"},
		{5, script.TriggerApNpc5, "OpNpc5"},
		{targetOpNpcT, script.TriggerApNpcT, "OpNpcT"}, // NEW (S6o)
		{targetOpNpcU, script.TriggerApNpcU, "OpNpcU"}, // NEW (S6o)
```

No other changes to the test function body.

- [ ] **Step 1.4: Modify `TestApNpcTriggerForOpInvalidValues` to remove `8`**

In the same file, find `TestApNpcTriggerForOpInvalidValues` (from S6n). The current `invalid` slice is likely `[]int{0, 6, 7, 8, -1, 100, -100}` or similar.

After S6o, `8` becomes VALID (targetOpNpcT) and `9` becomes VALID (targetOpNpcU). Both must be removed from the invalid list. The remaining invalid values are:

```go
invalid := []int{0, 6, 7, -1, 100, -100}
```

If the current list included `9`, remove it too. Also verify `6` and `7` stay in the invalid list — those are `targetOpLocT`/`targetOpLocU` (Loc sentinels), which are NOT valid inputs to `apNpcTriggerForOp` (the Npc variant). The invalid set asserts that Npc-specific cases 8/9 are distinct from Loc cases 6/7.

- [ ] **Step 1.5: Run the modified tests — expect FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestApNpcTriggerForOp" -v`

Expected: FAIL — the helper currently returns `ok=false` for ops 8 and 9 (both the existing body and the TestValidValues extension will fail). The new valid-case subtests (OpNpcT, OpNpcU) fail with "ok=false, want true".

- [ ] **Step 1.6: Extend `apNpcTriggerForOp` with T/U cases**

In `modules/world/interaction_trigger.go`, find `apNpcTriggerForOp` (around line 200). Replace the entire function (docstring + body) with:

```go
// apNpcTriggerForOp returns the APNPC trigger for the player's
// targetOp. fireOpTriggerNpc derives the OPNPC trigger by adding 7
// (TS Player.ts:~997 offset convention):
//
//	APNPC1..5 (3..7) + 7 → OPNPC1..5 (10..14)
//	APNPCT    (9)    + 7 → OPNPCT    (16)
//	APNPCU    (8)    + 7 → OPNPCU    (15)
//
// NPC variant of apLocTriggerForOp. Parallel shape after S6o: 1..5
// ops + T/U sentinels. Returns ok=false for invalid op.
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
	case op == targetOpNpcT:
		return script.TriggerApNpcT, true
	case op == targetOpNpcU:
		return script.TriggerApNpcU, true
	default:
		return 0, false
	}
}
```

This removes the old S6n docstring block that said "Does NOT handle T/U sentinels (DEVIATION S6n-D1)" — that deviation is now closed.

- [ ] **Step 1.7: Run the helper tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestApNpcTriggerForOp" -v`

Expected: all table subtests PASS (5 original 1..5 + 2 new T/U valid; remaining invalid cases rejected).

- [ ] **Step 1.8: Refactor `fireOpTriggerNpc` to use `apNpcTriggerForOp + 7`**

In `modules/world/interaction_trigger.go`, find `fireOpTriggerNpc` (around lines 49-94). Locate this specific block (within the function body, after the npc.dead gate):

```go
	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
```

Replace with:

```go
	apTrigger, ok := apNpcTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APNPC→OPNPC offset per TS Player.ts:~997
```

Byte-equivalent for 1..5: `TriggerApNpc1 + (op-1) + 7 = TriggerOpNpc1 + (op-1)` because `TriggerOpNpc1 = TriggerApNpc1 + 7` (numerically verified: 10 = 3 + 7). Everything else in `fireOpTriggerNpc` unchanged (npc.dead gate, category lookup, script init, resumeOrFinish, terminal ClearInteraction).

- [ ] **Step 1.9: Run existing fire tests to verify byte-equivalence for 1..5**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestTryFireOpTrigger" -v`

Expected: all existing S6j OpNpc tests (TestTryFireOpTrigger_HappyPath, TestTryFireOpTrigger_NoScript, TestTryFireOpTrigger_DeadNpc, TestTryFireOpTrigger_WrongTargetType, TestTryFireOpTrigger_BadOp, TestTryFireOpTrigger_ScriptSuspends, TestTryFireOpTrigger_PlayerDelayed, TestTryFireOpTrigger_ReClickResetsFired, TestTryFireOpTrigger_CategoryFallback, TestTryFireOpTrigger_GlobalFallback, and any S6m OpLoc tests) PASS.

- [ ] **Step 1.10: Remove stale S6n-D1 note from `fireApTriggerNpc` docstring**

In `modules/world/interaction_trigger.go`, find `fireApTriggerNpc` (around line 274 based on prior sub-specs). Find and delete this block from its docstring:

```go
// DEVIATION S6n-D1: APNPC T/U sentinels not wired. OpNpcT/OpNpcU
// handlers don't exist in goscape yet; when they land,
// apNpcTriggerForOp gains matching cases and this fire function
// needs a sentinel-aware op-range gate update.
```

The function body stays byte-identical — it calls `apNpcTriggerForOp` which now handles T/U internally. Only the docstring comment block is removed.

- [ ] **Step 1.11: Write failing test for `fireOpTriggerNpc` firing OPNPCT**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerNpcFiresOpNpcTTrigger verifies that when p.targetOp
// is targetOpNpcT (8) and an OPNPCT script is registered, fireOpTriggerNpc
// dispatches to it via apNpcTriggerForOp + 7. Mirrors S6m's
// TestFireOpTriggerLocFiresOpLocTTrigger.
func TestFireOpTriggerNpcFiresOpNpcTTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpcT, 0, "opnpct-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, 7777)
	p.interacted = true

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPNPCT fire")
	}
	if string(npc.sayText) != "opnpct-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "opnpct-fired")
	}
}
```

Note: This uses the existing `buildNpcSayScript` helper from `interaction_trigger_test.go` (per earlier S6m-era file inspection). Verify the helper signature with: `grep -n "func buildNpcSayScript" modules/world/interaction_trigger_test.go`. The helper accepts `(trigger, typeID, text)` and builds a script that makes the NPC `Say(text)`.

If `buildNpcSayScript` does NOT exist, use `newNoopScriptFile(t, trigger, typeID, -1)` and drop the `sayText` assertion — the test still proves the script fires because `target == nil` after Finished.

- [ ] **Step 1.12: Run test — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFireOpTriggerNpcFiresOpNpcTTrigger -v`

Expected: PASS (Step 1.8 already wired the dispatch path).

- [ ] **Step 1.13: Add 3 more fire-dispatch tests**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerNpcFiresOpNpcUTrigger verifies targetOpNpcU (9) →
// OPNPCU dispatch at contact.
func TestFireOpTriggerNpcFiresOpNpcUTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpcU, 0, "opnpcu-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	p.interacted = true

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPNPCU fire")
	}
	if string(npc.sayText) != "opnpcu-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "opnpcu-fired")
	}
}

// TestFireApTriggerNpcFiresApNpcTTrigger verifies targetOpNpcT (8) →
// APNPCT dispatch at approach distance. fireApTriggerNpc delegates to
// apNpcTriggerForOp which now handles T/U after S6o Task 1.
func TestFireApTriggerNpcFiresApNpcTTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApNpcT, 0, "apnpct-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, 7777)
	p.interacted = true

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APNPCT fire")
	}
	if string(npc.sayText) != "apnpct-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "apnpct-fired")
	}
}

// TestFireApTriggerNpcFiresApNpcUTrigger verifies targetOpNpcU (9) →
// APNPCU dispatch at approach distance.
func TestFireApTriggerNpcFiresApNpcUTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApNpcU, 0, "apnpcu-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	p.interacted = true

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APNPCU fire")
	}
	if string(npc.sayText) != "apnpcu-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "apnpcu-fired")
	}
}
```

Note: All 3 fire tests use the same `buildNpcSayScript` helper. If that helper doesn't exist with the expected signature, fall back to `newNoopScriptFile(t, trigger, typeID, -1)` and drop the `sayText` assertions — the `target == nil` + `interactionFired == true` assertions still prove the dispatch routed correctly.

- [ ] **Step 1.14: Run all 4 fire-dispatch tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireOpTriggerNpcFires|TestFireApTriggerNpcFires" -v`

Expected: 4 tests PASS.

- [ ] **Step 1.15: Run the full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass. Particularly verify:
- All S6j OpNpc tests pass (byte-equivalent refactor for 1..5)
- All S6m OpLoc tests pass (Loc path unchanged)
- All S6n APNPC tests pass (fireApTriggerNpc body unchanged)

- [ ] **Step 1.16: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 1.17: Commit Task 1**

```bash
git add modules/world/interaction.go modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): apNpcTriggerForOp T/U extension + fireOpTriggerNpc refactor (S6o-1)

Extends NPC trigger-dispatch helper to handle T/U sentinels and
refactors fireOpTriggerNpc to use apNpcTriggerForOp + 7 pattern
(byte-equivalent for 1..5, gains T/U dispatch for free).

- targetOpNpcT=8, targetOpNpcU=9 constants added to sentinel block
  alongside targetOpLocT=6, targetOpLocU=7 (S6m). Distinct values
  chosen for clarity — reusing 6/7 is safe via type-switch but less
  self-documenting.
- apNpcTriggerForOp switch extended to handle targetOpNpcT (→
  TriggerApNpcT=9) and targetOpNpcU (→ TriggerApNpcU=8). Parallel
  shape to apLocTriggerForOp after S6o.
- fireOpTriggerNpc inline `TriggerOpNpc1 + (op-1)` replaced with
  apNpcTriggerForOp + 7. Verified numerically: APNPC1..5 (3..7)+7 =
  OPNPC1..5 (10..14); APNPCT (9)+7 = OPNPCT (16); APNPCU (8)+7 =
  OPNPCU (15).
- fireApTriggerNpc body unchanged — gets T/U dispatch via helper.
  Stale S6n-D1 comment block removed from docstring (deviation
  closed by this task).

No handler sets these sentinels yet — Task 2 (S6o-2) adds
handleOpNpcT/handleOpNpcU which will route state through targetOpNpcT
and targetOpNpcU.

6 tests: 2 modified helper tests (TestApNpcTriggerForOpValidValues +
InvalidValues — T/U rows added, 8/9 removed from invalid) + 4 new
fire-dispatch tests (OP × T/U + AP × T/U).

Closes S6n-D1.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6o-opnpc-t-u-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6o-opnpc-t-u.md (Task 1)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handleOpNpcT + handleOpNpcU + Opcode Wiring

**Goal:** Add both click handlers. After this task, spell-on-NPC and item-on-NPC wire packets decode, validate, and mutate player state. The triggers fire end-to-end via Task 1's extended dispatch.

**Files:**
- Modify: `modules/world/handler_opnpc.go`
- Modify: `modules/world/handler_opnpc_test.go`
- Modify: `modules/world/handlers_game.go`

### Step-by-step

- [ ] **Step 2.1: Write failing test for `handleOpNpcT` happy path**

In `modules/world/handler_opnpc_test.go`, append the following. Note the fixture helper `makeOpNpcFixture` (line 13) returns `(*Server, *Player, *Npc)` — no conn. NPC typeID in fixture is `0`. Existing `p2Payload(slot int) []byte` encodes 2 bytes.

```go
// p2x2Payload encodes (a: u16, b: u16) into 4 bytes big-endian.
// Used by OpNpcT payload construction: slot + spellCom.
func p2x2Payload(a, b int) []byte {
	return []byte{
		byte(a >> 8), byte(a),
		byte(b >> 8), byte(b),
	}
}

// TestHandleOpNpcTSetsInteraction verifies a valid OpNpcT request sets
// interaction state with targetOp=targetOpNpcT and targetSubject.com
// carrying the spellCom.
func TestHandleOpNpcTSetsInteraction(t *testing.T) {
	_, p, npc := makeOpNpcFixture(t)

	if err := handleOpNpcT(p, p2x2Payload(1, 7777)); err != nil {
		t.Fatalf("handleOpNpcT: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != targetOpNpcT {
		t.Errorf("targetOp: got %d, want targetOpNpcT (%d)", p.targetOp, targetOpNpcT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
}
```

- [ ] **Step 2.2: Run test — expect compile failure `handleOpNpcT undefined`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpNpcTSetsInteraction -v`

- [ ] **Step 2.3: Implement `handleOpNpcT`**

In `modules/world/handler_opnpc.go`, append after the existing `handleOpNpc5` wrapper (the file currently ends with `handleOpNpc1..5` wrappers):

```go
// handleOpNpcT is the handler for OPNPCT (opcode 134, 4-byte payload).
// Spell-on-NPC: player drags a spell icon onto an NPC.
// Payload = (slot:G2, spellCom:G2).
//
// Validation gates (mirrors TS OpNpcTHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D1: TS validates spellCom references a component
// with ComActionTarget.NPC flag AND that the component is visible in
// the player's interface stack. Skipped here because goscape has no
// component registry yet. Effective risk: client can forge spellCom
// values; scripts reading p.TargetSubjectCom() get raw wire values.
// Follow-up: "component registry + ComActionTarget validation"
// sub-spec (bundle with S6m-D1).
//
// Unlike handleOpNpc (handler_opnpc.go:40-44), there is NO per-op
// validation gate — T/U variants don't index into NpcType.Op.
//
// No targetSubject.{typ,x,z,level} snapshot — NPCs have no in-place
// mutation risk (unlike Loc's packed Info bitfield). npc.dead is the
// lifecycle gate, checked at fire time (fireApTriggerNpc/fireOpTriggerNpc).
//
// On success: ClearPendingAction → SetInteraction(Engine, npc,
// targetOpNpcT, spellCom).
func handleOpNpcT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	spellCom := int(r.G2())

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, spellCom)
	return nil
}
```

Verify the `packet` import is already present at the top of `handler_opnpc.go` (it's used by the existing `handleOpNpc`). If not, add it: `"github.com/zsrv/goscape/pkg/io/packet"`.

- [ ] **Step 2.4: Run test — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpNpcTSetsInteraction -v`

Expected: PASS.

- [ ] **Step 2.5: Add 5 more OpNpcT validation tests**

Append to `modules/world/handler_opnpc_test.go`:

```go
// TestHandleOpNpcTDelayedPlayerRejected verifies delayed → UnsetMapFlag.
func TestHandleOpNpcTDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

// TestHandleOpNpcTShortPayloadRejected verifies <4 bytes → UnsetMapFlag.
func TestHandleOpNpcTShortPayloadRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcT(p, []byte{0x00, 0x01}) // only 2 bytes

	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpNpcTInvalidSlotRejected verifies slot >= len(s.npcs) → UnsetMapFlag.
func TestHandleOpNpcTInvalidSlotRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcT(p, p2x2Payload(9999, 7777)) // slot 9999 > len(s.npcs)

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcTDeadNpcRejected verifies dead NPC → UnsetMapFlag.
func TestHandleOpNpcTDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].dead = true

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil for dead NPC")
	}
}

// TestHandleOpNpcTMissingNpcTypeRejected verifies nil typ → UnsetMapFlag.
func TestHandleOpNpcTMissingNpcTypeRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].typ = nil

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil when NpcType is nil")
	}
}
```

- [ ] **Step 2.6: Run OpNpcT tests — expect all 6 PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpNpcT" -v`

Expected: 6 tests pass.

- [ ] **Step 2.7: Write failing test for `handleOpNpcU` happy path**

Append to `modules/world/handler_opnpc_test.go`:

```go
// p2x4Payload encodes 4 u16 values into 8 bytes big-endian.
// Used by OpNpcU payload construction: slot + useObj + useSlot + useCom.
func p2x4Payload(a, b, c, d int) []byte {
	return []byte{
		byte(a >> 8), byte(a),
		byte(b >> 8), byte(b),
		byte(c >> 8), byte(c),
		byte(d >> 8), byte(d),
	}
}

// TestHandleOpNpcUSetsInteraction verifies a valid OpNpcU request sets
// interaction state with targetOp=targetOpNpcU, stores useObj/useSlot
// in p.lastUseItem/lastUseSlot, and passes -1 for com (useCom discarded).
func TestHandleOpNpcUSetsInteraction(t *testing.T) {
	_, p, npc := makeOpNpcFixture(t)

	if err := handleOpNpcU(p, p2x4Payload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != targetOpNpcU {
		t.Errorf("targetOp: got %d, want targetOpNpcU (%d)", p.targetOp, targetOpNpcU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpNpcU passes -1)", p.targetSubject.com)
	}
}
```

- [ ] **Step 2.8: Run test — expect compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpNpcUSetsInteraction -v`

Expected: FAIL — `handleOpNpcU undefined`.

- [ ] **Step 2.9: Implement `handleOpNpcU`**

Append to `modules/world/handler_opnpc.go`:

```go
// handleOpNpcU is the handler for OPNPCU (opcode 202, 8-byte payload).
// Item-on-NPC: player drags an inventory item onto an NPC (e.g., feed
// pet, give gift, sacrifice item).
// Payload = (slot:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpNpcUHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// DEVIATION S6o-D3: TS does an inventory-listener lookup by useCom +
// slot-bounds + item-at-slot-matches-useObj validation. Goscape's
// invListeners is a slice not keyed map, so this lookup shape doesn't
// translate. Skip; scripts reading p.LastUseItem()/p.LastUseSlot() get
// raw wire values. (Mirrors S6m-D3.)
//
// DEVIATION S6o-D4: TS checks members-only items against NODE_MEMBERS
// config. Skipped — no members-config surface. (Mirrors S6m-D4.)
//
// On success: set p.lastUseItem=useObj, p.lastUseSlot=useSlot →
// ClearPendingAction → SetInteraction(Engine, npc, targetOpNpcU, -1).
func handleOpNpcU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	_ = int(r.G2()) // useCom — deliberately discarded (S6o-D2/D3)

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	return nil
}
```

- [ ] **Step 2.10: Run test — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpNpcUSetsInteraction -v`

- [ ] **Step 2.11: Add 5 more OpNpcU validation tests**

Append to `modules/world/handler_opnpc_test.go`:

```go
// TestHandleOpNpcUDelayedPlayerRejected verifies delayed → UnsetMapFlag,
// and that lastUseItem is NOT clobbered when validation fails (leak-
// prevention: state mutation happens only after all gates pass).
func TestHandleOpNpcUDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged on rejection

	_ = handleOpNpcU(p, p2x4Payload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

// TestHandleOpNpcUShortPayloadRejected verifies <8 bytes → UnsetMapFlag.
func TestHandleOpNpcUShortPayloadRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcU(p, []byte{0x00, 0x01, 0x00, 0x02}) // only 4 bytes

	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpNpcUInvalidSlotRejected verifies slot OOB → UnsetMapFlag.
func TestHandleOpNpcUInvalidSlotRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcU(p, p2x4Payload(9999, 1511, 3, 149)) // slot 9999 OOB

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcUDeadNpcRejected verifies dead NPC → UnsetMapFlag.
func TestHandleOpNpcUDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].dead = true

	_ = handleOpNpcU(p, p2x4Payload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for dead NPC")
	}
}

// TestHandleOpNpcUMissingNpcTypeRejected verifies nil typ → UnsetMapFlag.
func TestHandleOpNpcUMissingNpcTypeRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].typ = nil

	_ = handleOpNpcU(p, p2x4Payload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil when NpcType is nil")
	}
}
```

- [ ] **Step 2.12: Run all 12 handler tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpNpcT|TestHandleOpNpcU" -v`

Expected: 12 tests PASS.

- [ ] **Step 2.13: Wire opcodes in handlers_game.go**

In `modules/world/handlers_game.go`, find the existing OPNPC1..5 block (around lines 27-31). Append near it:

```go
gameHandlers[134] = handleOpNpcT // OPNPCT
gameHandlers[202] = handleOpNpcU // OPNPCU
```

Opcodes verified against `pkg/io/protocol/game/client/prot.go:65-66`.

- [ ] **Step 2.14: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass (no regressions).

- [ ] **Step 2.15: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 2.16: Run race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: no races.

- [ ] **Step 2.17: Commit Task 2**

```bash
git add modules/world/handler_opnpc.go modules/world/handler_opnpc_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): handleOpNpcT + handleOpNpcU + 12 validation tests (S6o-2)

Adds the two sibling OPNPC click handlers — NPC parallel to S6m's
OpLocT/OpLocU work:

- handleOpNpcT (opcode 134, 4-byte payload: slot, spellCom).
  Spell-on-NPC. Stores spellCom via SetInteraction(..., spellCom)
  into p.targetSubject.com. Fires APNPCT (single trigger, not
  per-op) via S6o-1's extended apNpcTriggerForOp.
- handleOpNpcU (opcode 202, 8-byte payload: +useObj, useSlot,
  useCom). Item-on-NPC. Stores useObj/useSlot in p.lastUseItem/
  p.lastUseSlot (existing S6m fields); useCom deliberately
  discarded per DEVIATION S6o-D2/D3.

Both handlers run 5 core TS validation gates (delayed, payload-
length, slot-bounds, npc-nil/dead, NpcType-nil). SIMPLER than S6m
Loc T/U handlers because NPCs are slot-indexed (no viewport check,
no GetLoc lookup, no targetSubject snapshot).

Complex validation (component-visibility, item-in-slot match,
members-only gate) explicitly deferred via S6o-D1..D4 with
follow-ups bundled with their S6m counterparts.

12 tests (6 per handler): happy path + delayed + short-payload +
invalid-slot + dead-npc + missing-NpcType. OpNpcU delayed-rejection
test verifies lastUseItem is NOT clobbered before validation passes
(leak-prevention, mirrors S6m's TestHandleOpLocUDelayedPlayerRejected).

Opcodes wired: OPNPCT=134, OPNPCU=202 in handlers_game.go.

End-to-end: spell-on-NPC and item-on-NPC clicks now route +
APNPCT/OPNPCT/APNPCU/OPNPCU scripts fire. S6n-D1 FULLY CLOSED.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6o-opnpc-t-u-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6o-opnpc-t-u.md (Task 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes (for plan-author use)

**1. Spec coverage:**
- §1 Goal — Tasks 1+2 collectively wire OpNpcT/OpNpcU end-to-end. ✅
- §2 Architecture — Task 1 (sentinels + helper + refactor) / Task 2 (handlers + wiring). ✅
- §3 File map — all 6 modified files appear in task headers. ✅
- §5.1 Sentinels — Task 1 Step 1.1. ✅
- §5.2 apNpcTriggerForOp T/U extension — Task 1 Step 1.6. ✅
- §5.3 fireOpTriggerNpc refactor — Task 1 Step 1.8. ✅
- §5.4 fireApTriggerNpc comment cleanup — Task 1 Step 1.10. ✅
- §5.5 handleOpNpcT — Task 2 Step 2.3. ✅
- §5.6 handleOpNpcU — Task 2 Step 2.9. ✅
- §5.7 Opcode wiring — Task 2 Step 2.13. ✅
- §6 Test plan — 6 (Task 1) + 12 (Task 2) = 18 total. ✅
- §8 Deviations — S6o-D1..D4 present in handler docstrings; S6n-D1 closed by comment removal. ✅

**2. Type consistency:**
- `targetOpNpcT = 8`, `targetOpNpcU = 9` consistent across Task 1 definition + all Task 2 call sites and test assertions. ✅
- `apNpcTriggerForOp(op int) (script.ServerTriggerType, bool)` signature preserved from S6n. ✅
- `handleOpNpcT(p *Player, payload []byte) error` + `handleOpNpcU` both follow existing `handleOpNpc1..5` wrapper signature. ✅
- `makeOpNpcFixture(t) (*Server, *Player, *Npc)` — 3-return signature used consistently in all new tests. ✅
- `p2x2Payload(a, b int) []byte` / `p2x4Payload(a, b, c, d int) []byte` follow existing `p2Payload` naming precedent. ✅

**3. Placeholder scan:** No TBD / TODO. Step 1.11 and 1.13 notes about `buildNpcSayScript` vs `newNoopScriptFile` fallback are legitimate implementer-verification instructions (helper-name may differ slightly; plan provides both options with the correct assertions for each). Step 2.3's `packet` import note is verify-and-adapt guidance, not a placeholder.

**4. Scope:** 2 tasks. Task 1 is small-medium (~60 LOC impl + ~100 test LOC for 6 tests + 2 modified). Task 2 is medium (~100 LOC impl + ~230 test LOC for 12 tests). Total ~160 LOC impl + ~330 test LOC — within the S6n/S6m cadence range. Build green at every commit (Task 1 byte-equivalent for 1..5; Task 2 adds consumers for Task 1's sentinels).
