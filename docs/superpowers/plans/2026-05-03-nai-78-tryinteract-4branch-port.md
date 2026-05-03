# NAI-78 — tryInteract 4-branch port (Tutorial Island RS Guide door) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Tutorial Island RS Guide door symptom ("OPLOC1 received, no movement, no script effect") by porting TS `Player.tryInteract` (Engine-TS Player.ts:1113-1184) 4-branch dispatch to goscape, replacing the current 2-branch shape that returns `true` from the AP block even when no AP script is registered.

**Architecture:** Single-file refactor of `modules/world/interaction.go::tryInteract` plus 3 new pkg-level helpers in `modules/world/interaction_trigger.go` (`getOpTrigger`, `getApTrigger`, `triggerTypeAndCategory`) and 1 new helper in `modules/world/interaction.go` (`defaultOp`). Fire-helper signatures and test fixtures UNCHANGED — internal `if sf == nil { … }` branches in fire helpers remain as defensive-only code (unreachable post-refactor because tryInteract gates on resolved-trigger-non-nil). Net: ~150 LOC change + ~250 LOC new tests.

**Tech Stack:** Go 1.26+. Testing: standard `testing` package; existing fixture helpers `makeOpLocTriggerFixture`, `newApTriggerNpcFixture`, `newNoopScriptFile`, `buildPOpLocScript`, `s.scriptProvider.Register`.

**Predecessors:** spec at `docs/superpowers/specs/2026-05-03-nai-78-tryinteract-4branch-port-design.md` (commit `b4d1656`).

---

## Pre-flight (Bundle 0 — controller pre-flight, no commits)

Per `controller_preflight.md` + `investigation_subspec_cadence.md`. Run before T1 dispatch. If any premise fails, HALT and re-run with corrected line numbers.

```bash
# P1: tryInteract is 2-branch at HEAD.
sed -n '310,343p' modules/world/interaction.go

# P2: fireApTriggerLoc has apRange=-1 sentinel at line 433.
sed -n '425,436p' modules/world/interaction_trigger.go

# P3: fireOpTriggerLoc has NIH "Nothing interesting happens." at line 158.
sed -n '155,165p' modules/world/interaction_trigger.go

# P4: fireApTriggerObj has apRange=-1 sentinel at line 622.
sed -n '618,626p' modules/world/interaction_trigger.go

# P5: fireApTriggerPlayer has apRange=-1 sentinel at line 116 (no interactionFired set).
sed -n '113,118p' modules/world/player_interaction_trigger.go

# P6: tryFireOpTrigger/tryFireApTrigger have 1-arg signature (no sf parameter).
grep -n "func tryFireOpTrigger\|func tryFireApTrigger" modules/world/interaction_trigger.go

# P7: getOpTrigger / getApTrigger / defaultOp do NOT yet exist in production code.
grep -rn "func getOpTrigger\|func getApTrigger\|func defaultOp" modules/world/

# P8: apLocTriggerForOp / apNpcTriggerForOp / apObjTriggerForOp / apPlayerTriggerForOp exist.
grep -n "func apLocTriggerForOp\|func apNpcTriggerForOp\|func apObjTriggerForOp" modules/world/interaction_trigger.go
grep -n "func apPlayerTriggerForOp" modules/world/player_interaction_trigger.go

# P9: resolveTriggerTypeId exists and overrides typeId via targetSubject.com.
sed -n '505,520p' modules/world/interaction_trigger.go

# P10: Player.MessageGame signature is `func (p *Player) MessageGame(msg string)`.
grep -n "func (p \*Player) MessageGame" modules/world/message_game.go

# P11: Player.waypoints field is [25]int (fixed-size array, not slice).
grep -n "waypoints\s*\[" modules/world/player.go

# P12: Loc.Type() and Obj.Type are the public type accessors.
grep -n "func (l \*Loc) Type\|func (o \*Obj) Type" pkg/entity/loc.go pkg/entity/obj.go
# Note: Obj exposes `Type` as a struct field (not a method); Loc exposes `Type()` as a method.
grep -n "Type\s*int\s*\$\|Type \s*int" pkg/entity/obj.go

# P13: ObjType.Category exists.
grep -n "Category\s*int" pkg/objtype/objtype.go pkg/objtype/loctype.go

# P14: Npc.typeId field + Npc.typ pointer (cached *NpcType).
grep -n "typeId int\|typ\s*\*objtype" modules/world/npc.go

# P15: Server.locTypes / Server.objTypes / Server.scriptProvider field names.
grep -n "scriptProvider\|locTypes\|objTypes" modules/world/server.go | head -10

# P16: 31+ test call sites for tryFireOpTrigger/tryFireApTrigger — should NOT change.
grep -c "tryFireOpTrigger\|tryFireApTrigger" modules/world/*_test.go
```

Expected: P1-P5 all confirm cited line numbers and code shape. P6 returns 1-arg signatures (no sf param). P7 returns 0 results. P8-P15 confirm helper/field availability. P16 returns >25 across test files (no signature change required).

---

## Task 1: Add `getOpTrigger` / `getApTrigger` resolution helpers

**Files:**
- Modify: `modules/world/interaction_trigger.go` — append 3 new pkg-level helpers adjacent to `apObjTriggerForOp` (~line 503, before `resolveTriggerTypeId`)
- Test: `modules/world/interaction_trigger_test.go` — append 8 new tests at end of file

Mirrors LostCityRS/Engine-TS Player.ts:966-998 (getOpTrigger) and 1000-1032 (getApTrigger). Pure addition — no callers yet (T3 wires them in).

- [ ] **Step 1: Write failing tests for `getOpTrigger`**

Append to `modules/world/interaction_trigger_test.go`:

```go
// --- NAI-78 T1: getOpTrigger / getApTrigger resolution helpers ---

func TestGetOpTrigger_LocTargetResolvesViaTriggerOpLoc1(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	want := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(want)

	got := getOpTrigger(p, s)
	if got != want {
		t.Errorf("getOpTrigger: got %p, want %p (TS Player.ts:966-998)", got, want)
	}
}

func TestGetOpTrigger_LocTarget_NoScriptReturnsNil(t *testing.T) {
	s, p, _, _ := makeOpLocTriggerFixture(t)
	// No script registered.
	got := getOpTrigger(p, s)
	if got != nil {
		t.Errorf("getOpTrigger: got %p, want nil (no [oploc1] registered)", got)
	}
}

func TestGetOpTrigger_NpcTargetResolvesViaTriggerOpNpc1(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	want := newNoopScriptFile(t, script.TriggerOpNpc1, npc.typeId, -1)
	s.scriptProvider.Register(want)

	got := getOpTrigger(p, s)
	if got != want {
		t.Errorf("getOpTrigger: got %p, want %p", got, want)
	}
}

func TestGetOpTrigger_NilTargetReturnsNil(t *testing.T) {
	s, p, _, _ := makeOpLocTriggerFixture(t)
	p.target = nil

	got := getOpTrigger(p, s)
	if got != nil {
		t.Errorf("getOpTrigger: got %p, want nil (TS Player.ts:967-969)", got)
	}
}

func TestGetOpTrigger_InvalidOpReturnsNil(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	p.targetOp = 99 // out of [1..5] + non-T/U
	_ = loc

	got := getOpTrigger(p, s)
	if got != nil {
		t.Errorf("getOpTrigger: got %p, want nil (apLocTriggerForOp ok=false)", got)
	}
}

func TestGetOpTrigger_TargetSubjectComOverridesTypeId(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	// targetSubject.com=42 → resolveTriggerTypeId returns 42 (TS Player.ts:993-995).
	p.targetSubject.com = 42
	want := newNoopScriptFile(t, script.TriggerOpLoc1, 42, -1)
	s.scriptProvider.Register(want)
	// Counter-pin: a script keyed at the loc's actual type must NOT be returned.
	deceiver := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(deceiver)

	got := getOpTrigger(p, s)
	if got != want {
		t.Errorf("getOpTrigger: got %p, want %p (com override per TS Player.ts:993-995)", got, want)
	}
}
```

- [ ] **Step 2: Run tests; verify FAIL with "undefined: getOpTrigger"**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestGetOpTrigger -count=1
```

Expected: COMPILE FAIL with `undefined: getOpTrigger`.

- [ ] **Step 3: Implement `getOpTrigger` + `getApTrigger` + shared helpers**

Append to `modules/world/interaction_trigger.go` immediately AFTER `apObjTriggerForOp` (line 503) and BEFORE `resolveTriggerTypeId` (line 515):

```go
// apTriggerForTarget dispatches to the per-entity-type apXxxTriggerForOp
// helper. Returns ok=false when target is nil or targetOp is unsupported
// for the target's concrete type. Internal — used by getOpTrigger and
// getApTrigger to share the type-switch.
func apTriggerForTarget(p *Player) (script.ServerTriggerType, bool) {
	switch p.target.(type) {
	case *Npc:
		return apNpcTriggerForOp(p.targetOp)
	case *entitypkg.Loc:
		return apLocTriggerForOp(p.targetOp)
	case *Player:
		return apPlayerTriggerForOp(p.targetOp)
	case *entitypkg.Obj:
		return apObjTriggerForOp(p.targetOp)
	}
	return 0, false
}

// triggerTypeAndCategory derives (typeId, categoryId) from the target's
// type registry, applying the targetSubject.com override per TS
// Player.getOpTrigger:993-995 / Player.getApTrigger:1027-1029.
//
// Player target: typeId stays -1 (TS Player.ts:971-972 default — Player
// branch doesn't set type) and categoryId stays -1 (provider falls
// through LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal).
//
// Internal — used by getOpTrigger and getApTrigger.
func triggerTypeAndCategory(p *Player, srv *Server) (typeId, categoryId int) {
	typeId = -1
	categoryId = -1

	switch tgt := p.target.(type) {
	case *Npc:
		typeId = tgt.typeId
		if tgt.typ != nil {
			categoryId = tgt.typ.Category
		} else {
			categoryId = 0
		}
	case *entitypkg.Loc:
		typeId = tgt.Type()
		categoryId = 0
		if locId := tgt.Type(); srv.locTypes != nil && locId >= 0 && locId < len(srv.locTypes.Configs) {
			if lt := srv.locTypes.Configs[locId]; lt != nil {
				categoryId = lt.Category
			}
		}
	case *entitypkg.Obj:
		typeId = tgt.Type
		categoryId = 0
		if srv.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(srv.objTypes.Configs) {
			if ot := srv.objTypes.Configs[tgt.Type]; ot != nil {
				categoryId = ot.Category
			}
		}
	case *Player:
		// typeId, categoryId stay -1.
	}

	typeId = resolveTriggerTypeId(p, typeId)
	return typeId, categoryId
}

// getOpTrigger resolves the [op<entity><op>,<typeId>] script for the
// player's anchored target. Mirrors LostCityRS/Engine-TS
// Player.ts:966-998. Returns nil if target is nil, op is unsupported,
// or no script registered. Used by tryInteract (interaction.go) to gate
// branch 1 (OP fire).
//
// The +7 offset converts an APXXX trigger into the matching OPXXX trigger
// per TS Player.ts:997 ScriptProvider.getByTrigger(this.targetOp + 7, …).
func getOpTrigger(p *Player, srv *Server) *script.ScriptFile {
	if p.target == nil {
		return nil
	}
	apTrigger, ok := apTriggerForTarget(p)
	if !ok {
		return nil
	}
	typeId, categoryId := triggerTypeAndCategory(p, srv)
	return srv.scriptProvider.GetByTrigger(apTrigger+7, typeId, categoryId)
}

// getApTrigger resolves the [ap<entity><op>,<typeId>] script. Mirror of
// getOpTrigger without the +7 offset. Mirrors LostCityRS/Engine-TS
// Player.ts:1000-1032. Used by tryInteract to gate branch 2 (AP fire).
func getApTrigger(p *Player, srv *Server) *script.ScriptFile {
	if p.target == nil {
		return nil
	}
	apTrigger, ok := apTriggerForTarget(p)
	if !ok {
		return nil
	}
	typeId, categoryId := triggerTypeAndCategory(p, srv)
	return srv.scriptProvider.GetByTrigger(apTrigger, typeId, categoryId)
}
```

- [ ] **Step 4: Run tests; verify PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestGetOpTrigger -count=1
```

Expected: PASS for all 6 tests. If `triggerTypeAndCategory` reports a different typeId for any test, re-grep field names per Pre-flight P12-P14.

- [ ] **Step 5: Write failing tests for `getApTrigger`**

Append to `modules/world/interaction_trigger_test.go`:

```go
func TestGetApTrigger_LocTargetResolvesViaTriggerApLoc1(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	want := newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1)
	s.scriptProvider.Register(want)

	got := getApTrigger(p, s)
	if got != want {
		t.Errorf("getApTrigger: got %p, want %p (TS Player.ts:1000-1032)", got, want)
	}
}

func TestGetApTrigger_LocTarget_NoScriptReturnsNil(t *testing.T) {
	s, p, _, _ := makeOpLocTriggerFixture(t)
	// No [aploc1] registered. The door symptom: this returns nil →
	// tryInteract branch 2 must NOT fire.
	got := getApTrigger(p, s)
	if got != nil {
		t.Errorf("getApTrigger: got %p, want nil (door bug regression — no [aploc1])", got)
	}
}
```

- [ ] **Step 6: Run tests; verify PASS** (helper already wired into Step 3 with `getApTrigger`)

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestGetApTrigger -count=1
```

Expected: PASS for both tests. (If they fail, T1 step 3 implementation is incomplete.)

- [ ] **Step 7: Run full package suite; verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS. Stale IDE diagnostics about unused funcs (`getOpTrigger`/`getApTrigger` only have test consumers) are expected and should be ignored per `verify_implementer_claims.md` failure-mode-1 — the helpers WILL get production consumers in T3.

- [ ] **Step 8: Commit**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-78 T1 — getOpTrigger/getApTrigger resolution helpers

Pure addition of LostCityRS/Engine-TS Player.ts:966-998 (getOpTrigger)
and Player.ts:1000-1032 (getApTrigger) ported to goscape. Plus
triggerTypeAndCategory + apTriggerForTarget shared helpers that
factor out the per-entity-type type-switch and category resolution.

No production callers yet — T3 wires them into the rewritten
tryInteract 4-branch dispatch. Existing fire-helper internal
resolution is preserved (will become defensive-only post-T3).

Tests pin: Loc/Npc/Player/Obj resolution, nil-target early return,
unsupported targetOp early return, targetSubject.com override per
TS Player.ts:993-995.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `defaultOp` NIH helper

**Files:**
- Modify: `modules/world/interaction.go` — append `defaultOp` pkg-level func adjacent to `tryInteract` (~line 344, after the closing `}` of tryInteract)
- Test: `modules/world/interaction_test.go` (new file if doesn't exist; otherwise append)

Mirrors LostCityRS/Engine-TS Player.ts:1072-1097 (defaultOp). Skips the NODE_PRODUCTION-gated dev "No trigger for [...]" debug message — goscape has no equivalent dev/prod flag.

- [ ] **Step 1: Verify `interaction_test.go` test file shape**

```bash
ls modules/world/interaction_test.go 2>/dev/null && echo "exists" || echo "create new"
```

If file doesn't exist, T2 will create it (with package decl + imports). If it exists, append the new tests.

- [ ] **Step 2: Write failing test**

Append (or create with `package world` + imports) to `modules/world/interaction_test.go`:

```go
// --- NAI-78 T2: defaultOp NIH helper ---

// TestDefaultOp_EmitsNIHAndClearsWaypoints pins TS
// LostCityRS/Engine-TS Player.ts:1072-1097. defaultOp must emit
// "Nothing interesting happens." to the player AND clear the
// waypoint queue (waypointIndex = -1). Goscape skips the
// NODE_PRODUCTION-gated dev "No trigger for [...]" debug line.
func TestDefaultOp_EmitsNIHAndClearsWaypoints(t *testing.T) {
	s, p, _, _ := makeOpLocTriggerFixture(t)
	_ = s

	// Pre-state: active waypoint queue.
	p.waypointIndex = 5
	p.waypoints[5] = 0xCAFE

	// Drain pre-existing client writes so we only see defaultOp's emit.
	flushClientWrites(t, p)

	defaultOp(p)

	// Assert: waypointIndex cleared.
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS Player.ts:1096 clearWaypoints)", p.waypointIndex)
	}

	// Assert: "Nothing interesting happens." emitted on the wire.
	got := drainClientWrites(t, p)
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("expected MessageGame(\"Nothing interesting happens.\") on wire; got %q", got)
	}
}
```

If `flushClientWrites` / `drainClientWrites` helpers don't exist by those exact names, search for the existing NIH-emit pattern at `handler_opobj_test.go:583-602` or `handler_opheld_test.go:420-456` — both exercise `bytes.Contains(got, []byte("Nothing interesting happens."))` and use whatever fixture method drains the player's `client.netOut` / `client.write` buffer. Mirror that pattern verbatim.

Add imports as needed:

```go
import (
	"bytes"
	"testing"
)
```

- [ ] **Step 3: Run test; verify FAIL with "undefined: defaultOp"**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestDefaultOp -count=1
```

Expected: COMPILE FAIL with `undefined: defaultOp`.

- [ ] **Step 4: Implement `defaultOp`**

Append to `modules/world/interaction.go` immediately AFTER `tryInteract` (current line 343, the closing `}`):

```go
// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// Skips the NODE_PRODUCTION-gated dev "No trigger for [...]" debug
// message at TS Player.ts:1076-1093 — goscape has no equivalent dev/prod
// flag and the chat-only path matches all known production-mode TS
// behavior.
func defaultOp(p *Player) {
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}
```

Note: `clearWaypoints()` in TS sets BOTH `this.waypoints = []` AND `this.waypointIndex = -1`. Goscape's `waypoints` is a `[25]int` fixed array; setting `waypointIndex = -1` is the operative no-path signal (matches the pattern at `interaction.go:128-131` ClearInteraction and at `interaction_trigger.go:130, 459` fire-helper clears, none of which zero the array).

- [ ] **Step 5: Run test; verify PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestDefaultOp -count=1
```

Expected: PASS.

- [ ] **Step 6: Run full package suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS. `defaultOp` has no production callers yet (T3 wires it in); IDE may flag as unused — ignore per `verify_implementer_claims.md` failure-mode-1.

- [ ] **Step 7: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-78 T2 — defaultOp NIH helper

Adds defaultOp(p) helper porting LostCityRS/Engine-TS
Player.ts:1072-1097. Emits "Nothing interesting happens." +
clears waypointIndex.

Skips the NODE_PRODUCTION-gated dev "No trigger for [...]" debug
message — goscape has no equivalent dev/prod flag.

T3 wires this into tryInteract branch 4 (NIH fallback when no [op…]
script registered for an operable target).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: tryInteract 4-branch rewrite

**Files:**
- Modify: `modules/world/interaction.go:310-343` — replace tryInteract body with 4-branch dispatch using T1 helpers
- Test: `modules/world/interaction_test.go` — append 7 new tests for the 4 branches + retry edge cases

The atomic fix. After T3, the door symptom is resolved at the engine level. Fire-helper signatures and bodies are UNCHANGED; their internal `if sf == nil { … }` branches become defensive-only (unreachable post-refactor because tryInteract gates on resolved-trigger-non-nil before invoking them).

- [ ] **Step 1: Write failing tests for the 4 branches + 1 retry**

Append to `modules/world/interaction_test.go`:

```go
// --- NAI-78 T3: tryInteract 4-branch dispatch ---

// TestTryInteract_OpFires_AdjacentNpc_Branch1 pins TS Player.ts:1123.
// Adjacent NPC + [opnpc1] registered + pre-step (allowOpScenery=false)
// → branch 1 fires (isPathing=true gates on op without allowOpScenery).
func TestTryInteract_OpFires_AdjacentNpc_Branch1(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpc1, npc.typeId, "branch1-fired"))

	// Place player adjacent to NPC.
	p.x, p.z = npc.x-1, npc.z

	got := p.tryInteract(false)

	if !got {
		t.Errorf("tryInteract: got false, want true (branch 1 OP fire)")
	}
	if !p.interactionFired {
		t.Errorf("p.interactionFired: got false, want true")
	}
	// Pin: OP script ran (npc say marker visible on wire).
	out := drainClientWrites(t, p)
	if !bytes.Contains(out, []byte("branch1-fired")) {
		t.Errorf("expected OP script marker on wire; got %q", out)
	}
}

// TestTryInteract_DoorSymptom_AdjacentLoc_OpOnly_Branch3to4 is THE
// regression test for the NAI-78 root cause. Adjacent Loc with [oploc1]
// registered but NO [aploc1]:
//   - Pre-step tryInteract(false): branch 1 fails (isPathing=false,
//     allowOpScenery=false), branch 2 fails (apTrigger=nil),
//     branch 3 FIRES (approach=true) → apRange=-1, return false.
//   - Post-step tryInteract(true) (after pathToTarget no-op since already
//     adjacent): branch 1 FIRES (isPathing=false, allowOpScenery=true,
//     operable=true) → OPLOC1 script executes.
//
// Pre-fix shape: pre-step's AP block returned true after fire-helper
// set apRange=-1; post-step skipped; auto-clear nuked the anchor.
// OPLOC1 never fired. Door symptom.
func TestTryInteract_DoorSymptom_AdjacentLoc_OpOnly_Branch3to4(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "door-fired"))
	// No TriggerApLoc1 registered — the door's [oploc1]-only shape.

	// Place player adjacent to loc.
	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-1, lz

	// Pre-step: branch 3 fires.
	preGot := p.tryInteract(false)
	if preGot {
		t.Errorf("pre-step tryInteract(false): got true, want false (TS branch 3 — apRange=-1, return false)")
	}
	if p.apRange != -1 {
		t.Errorf("p.apRange: got %d, want -1 (TS Player.ts:1174)", p.apRange)
	}
	if p.interactionFired {
		t.Errorf("p.interactionFired: got true, want false (branch 3 returned without firing)")
	}

	// Post-step: branch 1 fires (allowOpScenery=true).
	postGot := p.tryInteract(true)
	if !postGot {
		t.Errorf("post-step tryInteract(true): got false, want true (TS branch 1 OP fire)")
	}
	if !p.interactionFired {
		t.Errorf("p.interactionFired (post-step): got false, want true")
	}
	out := drainClientWrites(t, p)
	if !bytes.Contains(out, []byte("door-fired")) {
		t.Errorf("expected OPLOC1 script marker on wire (door symptom regression); got %q", out)
	}
}

// TestTryInteract_AdjacentLoc_BothScripts_Branch2 pins TS Player.ts:1139.
// Adjacent Loc with both [oploc1] AND [aploc1] registered + pre-step
// → branch 2 fires (apTrigger gates first since branch 1 fails for
// Loc+allowOpScenery=false).
func TestTryInteract_AdjacentLoc_BothScripts_Branch2(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApLoc1, loc.Type(), "ap-fired"))
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "op-fired"))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-1, lz

	got := p.tryInteract(false)

	if !got {
		t.Errorf("tryInteract: got false, want true (branch 2 AP fire)")
	}
	out := drainClientWrites(t, p)
	if !bytes.Contains(out, []byte("ap-fired")) {
		t.Errorf("expected AP script marker on wire; got %q", out)
	}
	if bytes.Contains(out, []byte("op-fired")) {
		t.Errorf("OP script should NOT fire when AP exists at this distance; got %q", out)
	}
}

// TestTryInteract_AdjacentNpc_NoScripts_Branch4 pins TS Player.ts:1179.
// Adjacent NPC with NO [opnpc1] AND NO [apnpc1] registered + pre-step
// (isPathing=true) → branch 1 fails (opTrigger=nil), branch 2 fails
// (apTrigger=nil), branch 3 fires first because approach=true →
// apRange=-1 + return false. Post-step (allowOpScenery=true): branch 4
// fires (operable=true, isPathing=true) → defaultOp NIH.
func TestTryInteract_AdjacentNpc_NoScripts_Branch4(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	// No scripts registered.

	p.x, p.z = npc.x-1, npc.z

	preGot := p.tryInteract(false)
	if preGot {
		t.Errorf("pre-step: got true, want false (branch 3 fires for NPC with no scripts)")
	}
	if p.apRange != -1 {
		t.Errorf("p.apRange: got %d, want -1", p.apRange)
	}

	flushClientWrites(t, p)
	postGot := p.tryInteract(true)
	if !postGot {
		t.Errorf("post-step: got false, want true (branch 4 NIH)")
	}
	out := drainClientWrites(t, p)
	if !bytes.Contains(out, []byte("Nothing interesting happens.")) {
		t.Errorf("expected branch 4 defaultOp NIH on wire; got %q", out)
	}
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (defaultOp clears)", p.waypointIndex)
	}
}

// TestTryInteract_NilTargetReturnsFalse — entry guard pin.
func TestTryInteract_NilTargetReturnsFalse(t *testing.T) {
	s, p, _, _ := makeOpLocTriggerFixture(t)
	_ = s
	p.target = nil

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract with nil target: got true, want false")
	}
}

// TestTryInteract_NAI69_AprangeRetry_PreservedInBranch2 pins NAI-69
// closure of NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED. AP script that
// calls p_aprange must cause tryInteract to return false (signaling
// same-tick walk-arm retry to processInteraction).
func TestTryInteract_NAI69_AprangeRetry_PreservedInBranch2(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	// Register an AP script that calls p_aprange(2) then returns.
	s.scriptProvider.Register(buildPApRangeScript(t, script.TriggerApLoc1, loc.Type(), 2))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-3, lz // out of operable, in approach

	got := p.tryInteract(false)

	// NAI-69 retry: apRangeCalled=true after AP fire → return false.
	if got {
		t.Errorf("tryInteract with apRangeCalled: got true, want false (NAI-69 retry)")
	}
	if !p.apRangeCalled {
		t.Errorf("p.apRangeCalled: got false, want true")
	}
	if p.interactionFired {
		t.Errorf("p.interactionFired: got true, want false (NAI-69 — reset for retry)")
	}
}

// TestTryInteract_OutOfRangeReturnsFalse — branches 1-4 all gated; out
// of approach AND out of operable → return false (no AP, no walk-arm
// signal beyond pathToTarget which runs in processInteraction's
// post-step branch).
func TestTryInteract_OutOfRangeReturnsFalse(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "op-fired"))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-50, lz // way out of range
	// p.apRange default 10 from SetInteraction in fixture.

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract out-of-range: got true, want false")
	}
}
```

`buildPApRangeScript` may not exist with that exact name — search for the existing test helper that registers an AP script that calls `p_aprange`. Likely candidates: `buildPApRangeScript`, `buildAprangeScript`, or inlined script construction in NAI-69 test files (`grep -rn "p_aprange\|OpPApRange\|TestTryInteract.*Aprange" modules/world/`). If none exists, construct inline using the `buildPOpLocScript` pattern from `interaction_trigger_nai68_test.go:25-39` but with `[push 2, OpPApRange, OpReturn]` body. Plan-author MUST verify the helper name before T3 dispatch.

- [ ] **Step 2: Run tests; verify FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestTryInteract_(OpFires_AdjacentNpc|DoorSymptom|AdjacentLoc_BothScripts|AdjacentNpc_NoScripts|NilTarget|NAI69_AprangeRetry|OutOfRange)" -count=1
```

Expected: 4-6 of 7 tests FAIL.
- `DoorSymptom` MUST FAIL: pre-step returns `true` (current 2-branch behavior) instead of `false` (TS branch 3). This is the smoking-gun assertion.
- `AdjacentNpc_NoScripts_Branch4` MUST FAIL: branch 4 doesn't exist yet.
- `AdjacentLoc_BothScripts` may PASS at HEAD (current behavior is correct for AP-script-exists case).
- `OpFires_AdjacentNpc` may PASS at HEAD (current branch-1 NPC path works).
- `NAI69_AprangeRetry_PreservedInBranch2` SHOULD PASS at HEAD (NAI-69 already in place).
- `NilTargetReturnsFalse` SHOULD PASS (current entry guard exists implicitly).
- `OutOfRangeReturnsFalse` SHOULD PASS (current AP block + OP block both gate on range).

If `DoorSymptom` PASSES at HEAD, the Bundle 0 verdict is wrong — STOP and re-investigate before T3 step 3.

- [ ] **Step 3: Implement tryInteract 4-branch rewrite**

Replace `modules/world/interaction.go:310-343` (the entire `tryInteract` function body — keep the func signature) with:

```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined. Mirrors
// LostCityRS/Engine-TS Player.tryInteract at Player.ts:1113-1184.
//
// 4-branch dispatch (after NAI-78). Resolves opTrigger/apTrigger via
// getOpTrigger/getApTrigger (interaction_trigger.go) at entry, then
// dispatches:
//
//	1. opTrigger != nil && (PathingEntity || allowOpScenery) && operable
//	   → fire OP, return true.
//	2. apTrigger != nil && approach
//	   → fire AP, return true (or false on NAI-69 same-tick retry).
//	3. approach (apTrigger nil)
//	   → apRange=-1, return false. Allows processInteraction's post-step
//	   to run pathToTarget + tryInteract(allowOpScenery=true).
//	4. (PathingEntity || allowOpScenery) && operable (opTrigger nil)
//	   → defaultOp NIH ("Nothing interesting happens." + clear waypoints),
//	   return true.
//
// Fixes NAI-78 root cause: pre-NAI-78, the 2-branch shape returned true
// from the AP block even when no AP script existed, which gated the
// post-step branch off and let the auto-clear nuke the anchor — the
// Tutorial Island RS Guide door symptom. Branch 3's `return false`
// closure is the load-bearing change.
//
// allowOpScenery gates branches 1 and 4 for non-PathingEntity targets
// (Loc, Obj). Mirrors TS Player.tryInteract(allowOpScenery: boolean).
// Callers:
//   - pre-step (always false): scenery OP blocked before movement
//   - post-step (stepsTaken==0): scenery OP allowed only if no walk
//
// NPC side equivalent: (*Npc).tryInteract(s, allowOpScenery bool)
// at npc_interaction.go:247.
func (p *Player) tryInteract(allowOpScenery bool) bool {
	if p.target == nil {
		return false
	}
	srv := p.client.server

	opTrigger := getOpTrigger(p, srv)
	apTrigger := getApTrigger(p, srv)

	tx, tz, _ := p.target.Coords()
	operable := inOperableDistance(p.x, p.z, tx, tz)
	approach := inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p))

	isPathing := false
	switch p.target.(type) {
	case *Npc, *Player:
		isPathing = true
	}

	// Branch 1 — OP fire (TS Player.ts:1123).
	if opTrigger != nil && (isPathing || allowOpScenery) && operable {
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return true
	}

	// Branch 2 — AP fire (TS Player.ts:1139).
	if apTrigger != nil && approach {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		// NAI-69 same-tick retry: apRangeCalled is set when the AP
		// script called p_aprange. Closes
		// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED. (TS L1163-1167.)
		if p.nextTarget == nil && p.apRangeCalled {
			p.interactionFired = false
			return false
		}
		return true
	}

	// Branch 3 — default-AP no-op (TS Player.ts:1173-1175).
	// Player is in approach distance but no [ap…] script exists.
	// Force apRange = -1 so the AP block can never re-enter on this
	// interaction, then return false to let processInteraction's
	// post-step branch run (pathToTarget → walktrigger → post-step
	// tryInteract with allowOpScenery=true so branch 1 can fire OP).
	//
	// THIS IS THE NAI-78 LOAD-BEARING FIX: pre-NAI-78 this branch
	// didn't exist; the AP block returned true unconditionally and
	// the auto-clear at processInteraction tail nuked the anchor
	// before OP could ever fire.
	if approach {
		p.apRange = -1
		return false
	}

	// Branch 4 — default-OP NIH (TS Player.ts:1179-1182).
	// Player is in operable distance but no [op…] script exists.
	// Emit "Nothing interesting happens." + clear waypoints.
	if (isPathing || allowOpScenery) && operable {
		defaultOp(p)
		return true
	}

	return false
}
```

Key invariants preserved:
- NAI-69 same-tick AP retry inside branch 2 (`apRangeCalled` + `interactionFired=false` + `return false`).
- Pre-step caller (`processInteraction:205`) passes `allowOpScenery=false` unchanged.
- Post-step caller (`processInteraction:228`) passes `stepsTaken==0` unchanged.
- `effectiveApRange(p)` call (S6l/NAI-69) routes through unchanged.

Fire-helper internal `if sf == nil { … }` branches (e.g. `interaction_trigger.go:79, 156, 341, 426, 540, 557, 605, 621` and `player_interaction_trigger.go:50, 64, 105, 115`) are NOT touched — they become defensive-only post-T3, unreachable on the happy path because tryInteract pre-gates on resolved-trigger-non-nil. Existing direct-call tests (`interaction_trigger_test.go`) continue to exercise those paths via the fire-helper-only entry, which is fine.

The previous 2-branch tryInteract body is fully replaced. The `// Loc/Obj + !allowOpScenery: fall through to AP check.` comment at the old line 322 is retired (no longer applicable — the new shape uses explicit branch gates).

- [ ] **Step 4: Run T3 tests; verify all PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestTryInteract_(OpFires_AdjacentNpc|DoorSymptom|AdjacentLoc_BothScripts|AdjacentNpc_NoScripts|NilTarget|NAI69_AprangeRetry|OutOfRange)" -count=1 -v
```

Expected: ALL 7 PASS. The `DoorSymptom` test is the load-bearing regression.

- [ ] **Step 5: Run full package suite + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1 -race
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS. Per `verify_implementer_claims.md`: re-run from a fresh shell to defeat stale IDE cache. Per `latent_bug_at_migration_boundary.md`: tryInteract is the engine path for EVERY interaction; pay close attention to any failure in `npc_interaction_test.go`, `interaction_trigger_test.go`, `tick_interactions_test.go`, `handler_oploc_test.go`, `handler_opnpc_test.go`, `handler_opobj_test.go`, `handler_op_player_test.go`, NAI-69 retry tests.

If a regression surfaces:
- **OPLOC/OPNPC/OPOBJ-related test failures**: likely a branch-1-vs-branch-2 mis-gating; verify `isPathing` switch covers the failing target type.
- **NAI-69 retry test failures**: branch 2's apRangeCalled handling has a gap; re-read TS Player.ts:1158-1168 for exact placement.
- **Any test that pre-NAI-78 passed in the AP-no-script path expecting `tryInteract` to return true**: that test was pinning the BUG. Update its assertions to match the new branch-3 return-false behavior.

- [ ] **Step 6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-78 T3 — tryInteract 4-branch port (RS Guide door fix)

Replaces the 2-branch tryInteract (modules/world/interaction.go:310-343)
with a TS-faithful 4-branch dispatch mirroring LostCityRS/Engine-TS
Player.tryInteract at Player.ts:1113-1184. Resolves opTrigger and
apTrigger up-front via getOpTrigger/getApTrigger (T1) and gates each
branch explicitly:

  1. OP fire — opTrigger && (PathingEntity || allowOpScenery) && operable
  2. AP fire — apTrigger && approach (NAI-69 retry preserved)
  3. default-AP no-op — approach (no apTrigger): apRange=-1, return false
  4. default-OP NIH — operable (no opTrigger): defaultOp() (T2)

Branch 3 is the load-bearing fix for the Tutorial Island RS Guide
door symptom: pre-NAI-78 the AP block returned true unconditionally
when player was in approach range, gating the post-step branch off
even when no [aploc1] script existed. The auto-clear at
processInteraction tail then nuked the anchor before OP could fire.
Post-NAI-78: branch 3 returns false → !interacted → post-step runs
pathToTarget + tryInteract(allowOpScenery=true) → branch 1 fires
OPLOC. Verified by TestTryInteract_DoorSymptom_AdjacentLoc_OpOnly_Branch3to4.

Fire-helper signatures and bodies UNCHANGED. Their internal
`if sf == nil { … }` branches become defensive-only post-fix
(unreachable on the happy path because tryInteract pre-gates on
resolved-trigger-non-nil). 31+ existing direct-call test sites
preserved.

NAI-69 same-tick AP retry preserved inside branch 2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Smoke handoff (no commit)

Per `smoke_test_server_handoff.md`. User-driven; goscape's sandboxed process is unreachable from the host Java client.

- [ ] **Step 1: Surface resume-to-user message**

Controller emits a paste-ready message to the user:

> NAI-78 T3 landed at HEAD `<commit-sha>`. Please launch the server with the latest binary and re-test the smoke matrix:
>
> 1. **Door click registers movement** — log in fresh; talk to RS Guide enough to advance `%tutorial` past `^newbie_basics_instructor_interact_with_scenery`; click wooden door at the RS Guide room exit. Pass = player walks toward door (path generation visible) AND OPLOC1 trigger fires (script execution evidence in server log or in-game effect).
> 2. **Door full path** — same as item 1 but assert `[oploc1,newbie_door1]` body completes: door visually opens, player walks through, `~tutorial_step_moving_around` chatbox appears, `%tutorial` advances.
> 3. **NIH fallback** — out-of-scope items (in-zone loc with no oploc/aploc registered) produce "Nothing interesting happens." chat instead of silent no-op.
>
> Reply with PASS/FAIL per item. If item 2 FAILS with a script error log line ("no handler for LOC_PARAM/LOC_COORD/…"), Bundle 3 conditional materializes per the spec §Bundle 3 template.

- [ ] **Step 2: Wait for user smoke result**

No commit at this step. Branch on user's report:
- **Items 1+2+3 all PASS** → proceed to Task 5 (close commit).
- **Item 1 PASS, item 2 FAIL with script-error log** → materialize Bundle 3 per the spec §Bundle 3 template (LOC opcode ports). The Bundle 3 template lives in the spec; controller drafts a concrete plan-doc-extension at smoke-fail time.
- **Item 1 PASS, item 2 FAIL with silent no-op** → Bundle 3 alternate path: runtime instrumentation per `investigation_subspec_cadence.md` Stage 3 template.
- **Item 1 FAIL** → T3 has a defect (signature mismatch, branch 3 not actually returning false, NAI-69 retry broken, mis-gating on isPathing). Re-investigate before close.
- **Item 3 FAIL alone** → branch 4 not wired correctly; in-scope-stretch fix per `smoke_surfaces_adjacent_divergences.md` (~5 LOC).

---

## Task 5: Close commit (conditional on smoke 1+2+3 PASS)

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — add NAI-78 close section
- Modify: any other memory entries surfaced by lessons learned this round

Empty-tree commit per `close_commit_memory_trailer.md`. Carries `Closes:` for the door symptom, `Closes memory:` for the followups-md updates.

- [ ] **Step 1: Update nai_followups.md**

Add to the top of `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (above the existing "## From NAI-77" section):

```markdown
## From NAI-78 (2026-05-03)

NAI-78 closed the Tutorial Island RS Guide door symptom by porting TS
`Player.tryInteract` 4-branch dispatch (Engine-TS Player.ts:1113-1184)
to goscape. Smoke at HEAD `<commit-sha>` confirmed all 3 items PASS.

Carry-forwards untouched by NAI-78:
- (existing list from NAI-77 close, plus any new items surfaced this round)
```

- [ ] **Step 2: Save any new lessons as memory entries**

Per `post_task_handoff.md`. Candidates:
- If smoke surfaced any adjacent untracked divergences (per `smoke_surfaces_adjacent_divergences.md`): document the routing decision.
- If T3 review caught a TS-fidelity gap that needed in-scope correction: save the lesson.
- If any controller pre-flight premise (P1-P16) was stale: re-confirm `controller_preflight.md` lesson and consider sharpening the rule.
- If the fire-helper internal-defensive-code-as-unreachable pattern is a recurrent goscape concern: consider opening a separate "defensive-only post-refactor" memory.

- [ ] **Step 3: Final review (Sonnet code-reviewer subagent)**

Per `superpowers_code_reviewer_model.md` — superpowers:code-reviewer agent on Sonnet. Review the full NAI-78 diff (T1+T2+T3 + tests) against the spec for TS-fidelity and any final-polish items.

```
Review NAI-78 (T1+T2+T3) against spec at
docs/superpowers/specs/2026-05-03-nai-78-tryinteract-4branch-port-design.md.
Diff: git diff <pre-T1-sha>..HEAD modules/world/.
Look for:
- TS-fidelity gaps in tryInteract 4-branch dispatch vs Player.ts:1113-1184
- defaultOp parity vs Player.ts:1072-1097 (skipped dev-only line is intentional)
- getOpTrigger/getApTrigger parity vs Player.ts:966-1032
- Any fire-helper internal branch that should ALSO be retired (vs left as
  defensive-only)
- Spec §7 deviation accounting accuracy
- Code-quality red flags per code-quality-red-flags.md
```

If reviewer flags an issue worth fixing, apply as a separate `polish(world): NAI-78 review — …` commit BEFORE the close commit.

- [ ] **Step 4: Empty-tree close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-78 — tryInteract 4-branch port closes RS Guide door

Stage 1 short-circuit + Stage 2 single-file refactor:
  T1: getOpTrigger/getApTrigger resolution helpers (LostCityRS/Engine-TS Player.ts:966-1032)
  T2: defaultOp NIH helper (LostCityRS/Engine-TS Player.ts:1072-1097)
  T3: tryInteract 4-branch rewrite (LostCityRS/Engine-TS Player.ts:1113-1184)

Door symptom resolved: smoke items 1+2+3 PASS at HEAD.

Investigation+fix cadence per investigation_subspec_cadence.md
(4th instance after NAI-31, NAI-75, NAI-76). Bundle 0 controller
pre-flight identified the smoking gun without subagent dispatch.

Net deviation tally: 15 → 14 (closes the untracked tryInteract
2-branch divergence; +0 new — defaultOp NIH ported in-scope).

Closes: tutorial-island-rs-guide-door-interaction
Closes memory: nai_followups.md NAI-78 close, lessons surfaced this round.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 3 conditional template (LOC opcode ports)

**Materializes only if smoke item 2 fails with a script-error log line at T4.** Pre-templated in the spec at §Bundle 3 template. Concrete tasks (B3-T1 through B3-T4) drafted there. Plan-author re-templates as plan-doc extension at smoke-fail time, NOT as part of NAI-78's main plan flow.

Likely missing opcodes the door's `[oploc1]` body + transitive procs require (per spec §Bundle 3 template):

| Opcode | Const | Required for |
|---|---|---|
| 3000 | OpLocAdd | `loc_add(...)` in `~open_and_close_door` |
| 3001 | OpLocAngle | `loc_angle` in `~check_axis` + `~open_and_close_door` |
| 3004 | OpLocChange | `loc_change(inviswall, 3)` in `~open_and_close_door` |
| 3005 | OpLocCoord | `loc_coord` in `~check_axis` + `~open_and_close_door` |
| 3011 | OpLocParam | `loc_param(next_loc_stage)` in door [oploc1] |
| 3012 | OpLocShape | `loc_shape` in `~open_and_close_door` |
| 2104 | OpSoundSynth | `sound_synth(door_open, 0, 0)` in `~open_and_close_door` |

Bundle 3 sized at ~250-300 LOC; may roll forward to NAI-79 if scope explodes per scope-gate discipline. Controller-level decision at smoke-fail time.

---

## Self-review checklist (controller, before T1 dispatch)

- [ ] Spec coverage: every Architecture section §A-§D has a corresponding T1/T2/T3 task. ✓ (§A→T1, §B→T3, §C→T2, §D NOT exercised — fire-helper signatures unchanged per pragmatic-minimal-change choice; spec §D's "retire no-script branches" downgrades to "becomes defensive-only post-refactor" per cleaner trade-off documented in T3 step 3)
- [ ] Placeholder scan: no TBD/TODO. All test code is concrete; fixture-helper names verified or have grep instructions. ✓
- [ ] Type consistency: `getOpTrigger`, `getApTrigger`, `defaultOp`, `triggerTypeAndCategory`, `apTriggerForTarget` consistently named across T1/T2/T3. ✓
- [ ] NAI-69 retry preserved: branch 2's `apRangeCalled` check matches HEAD interaction.go:336-339. ✓
- [ ] Pre-flight P1-P16 enumerate every premise the plan codifies. ✓
- [ ] Smoke decision tree (T4) covers all routing branches per spec §Smoke matrix. ✓

If any check fails, fix inline before T1 dispatch. Per `controller_preflight.md`: re-run grep verification of every cited line number against HEAD before each task dispatch (P1-P16 should already do this, but verify the specific snippet in the task body too).
