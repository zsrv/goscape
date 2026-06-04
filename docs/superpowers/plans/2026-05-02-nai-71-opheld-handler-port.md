# NAI-71 — OPHELD handler family port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 7-opcode OPHELD handler family (OPHELD1-5, OPHELDT, OPHELDU) into `modules/world/`, mirroring TS `OpHeldHandler.ts` / `OpHeldTHandler.ts` / `OpHeldUHandler.ts` line-by-line. Closes the silent-discard gap in `gameHandlers[]` for inventory-side held-item interaction. Net deviation tally 12 → 13.

**Architecture:** Three free-function handlers (no Server-method wrapper) in a new `modules/world/handler_opheld.go`, modeled on the `handler_oploc.go` / `handler_opobj.go` shape but using the inv-listener / direct-script-fire pattern from `handler_inv_button.go`. Five 1-line wrappers `handleOpHeld1..5` dispatch into a shared `handleOpHeld(p, payload, op)` core. Direct registration in `handlers_game.go init()` — no adapter layer.

**Tech Stack:** Go 1.26+. TS source: `LostCityRS/Engine-TS`. Test idioms from `handler_inv_button_test.go` + `handler_opobj_test.go`.

**Spec:** `docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md`.

**Predecessor commit:** `5fd2a78` (NAI-71 spec). HEAD entering: `5fd2a78`.

---

## Pre-flight (controller, before each task dispatch)

Per `controller_preflight.md` and `spec_followup_tracker_freshness.md`, controller re-verifies before each task:

```bash
# Confirm spec premises still hold at HEAD
rg -n "TriggerOpHeld[1-5UT]\b" pkg/script/trigger.go
rg -n "Operable\b|ComActionTargetHeld\b|RootLayer\b" pkg/objtype/componenttype.go
rg -n "type ObjType struct" -A 50 pkg/objtype/objtype.go | grep -E "IOp|Category|Members"
rg -n "lastItem\b|moveClickRequest\b|modalMain\b|entitymask\b" modules/world/player.go
rg -n "func handleOpObj\b|func handleOpLoc\b" modules/world/  # template files
rg -n "handler_opheld\.go|handleOpHeld" modules/world/  # confirm absence pre-T1
```

Re-grep specifically before T2 + T3 (T1 may shift line numbers).

---

## Task 1 — OPHELD1-5 (shared core + 5 wrappers + tests)

**Files:**
- Create: `modules/world/handler_opheld.go`
- Modify: `modules/world/handlers_game.go` (add 5 init() lines)
- Create: `modules/world/handler_opheld_test.go`

### Step 1.1: Write failing reject-gate tests (delayed, short payload, nil component, !Operable, !visible, no-listener, nil-inv, slot-mismatch, ObjType-nil, IOp-empty)

- [ ] Create `modules/world/handler_opheld_test.go` with the test scaffolding:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// opHeldPayload encodes a 6-byte OPHELD1-5 payload (obj:G2 slot:G2 com:G2).
func opHeldPayload(obj, slot, com int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
	}
}

// setupOpHeldServer returns a Server + Player pre-wired with a world inv at
// invType=93, com=149, source=-1. ObjType 555 has IOp = ["op1","","","",""]
// so op=1 is allowed and op=2..5 reject. ComponentType 149 is Operable
// with RootLayer=149 (matches modalMain default).
//
// Item id=555 count=1 lives at inv slot 3.
func setupOpHeldServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	s.invs = make(map[int]*inventory.Inventory)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 600),
	}
	s.objTypes.Configs[555] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 555, DebugName: "test_held"},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
	})
	p.tabs[0] = 149
	p.modalMain = 149 // matches RootLayer ⇒ ClearPendingAction NOT called
	return s, p
}

// TestHandleOpHeld_Delayed pins that a delayed player drops the packet.
// Mirrors TS OpHeldHandler.ts:16-19.
func TestHandleOpHeld_Delayed(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_ShortPayload pins that <6 bytes drops.
func TestHandleOpHeld_ShortPayload(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s // silence unused
	if err := handleOpHeld1(p, []byte{0, 0, 0}); err != nil {
		t.Fatalf("handleOpHeld1: %v", err)
	}
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TestHandleOpHeld_NilComponent pins that a comId not in the registry rejects.
func TestHandleOpHeld_NilComponent(t *testing.T) {
	s, p := setupOpHeldServer(t)
	delete(p.invListeners, 149)         // drop the listener so the registry-empty path is exercised cleanly
	p.invListenOnCom(93, 999, -1)        // listener for com=999 (not seeded)
	p.tabs[0] = 999
	_ = s

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 999))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil component must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NotOperable pins that com.Operable=false rejects.
// Mirrors TS OpHeldHandler.ts:21-23.
func TestHandleOpHeld_NotOperable(t *testing.T) {
	s, p := setupOpHeldServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: false},
	})

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (Operable=false must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NotVisible pins !IsComponentVisible reject.
// Mirrors TS OpHeldHandler.ts:25-27.
func TestHandleOpHeld_NotVisible(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s
	p.tabs[0] = 0 // clear tab assignment so the component is not visible

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not visible must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NoListener pins that a comId without an inv listener rejects.
// Mirrors TS OpHeldHandler.ts:30-35.
func TestHandleOpHeld_NoListener(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s
	delete(p.invListeners, 149)

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no listener must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NilInv pins that resolveListenerInv→nil rejects.
// Mirrors TS OpHeldHandler.ts:30-35 second arm.
func TestHandleOpHeld_NilInv(t *testing.T) {
	s, p := setupOpHeldServer(t)
	delete(s.invs, 93)

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil inv must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_HasAtFalse pins inv.HasAt(slot, obj)=false reject.
// Mirrors TS OpHeldHandler.ts:37-43.
func TestHandleOpHeld_HasAtFalse(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s

	// Wrong slot — slot 3 has 555, slot 4 is empty.
	_ = handleOpHeld1(p, opHeldPayload(555, 4, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (HasAt false must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_ObjTypeNil pins that an obj id outside the loaded ObjType
// table rejects (goscape defensive; TS throws).
func TestHandleOpHeld_ObjTypeNil(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555] = nil

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil ObjType must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_IOpEmpty pins that objType.IOp[op-1] == "" rejects.
// Mirrors TS OpHeldHandler.ts:45-48.
func TestHandleOpHeld_IOpEmpty(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s

	// op=2 → IOp[1] is "" in the fixture.
	_ = handleOpHeld2(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (IOp[op-1]=='' must reject)", p.lastItem)
	}
}
```

- [ ] Run tests to verify failures:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeld_ -count=1
```

Expected: ALL FAIL with `undefined: handleOpHeld1` / `undefined: handleOpHeld2` (the production functions don't exist yet).

### Step 1.2: Implement `handleOpHeld` shared core + 5 wrappers

- [ ] Create `modules/world/handler_opheld.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleOpHeld is the shared implementation for OPHELD1..OPHELD5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS OpHeldHandler.ts:
//  1. p.delayed → drop
//  2. payload < 6 → drop
//  3. nil component or !Operable → drop
//  4. !IsComponentVisible → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//  8. ObjType not registered (goscape defensive; TS throws here) → drop
//  9. objType.IOp[op-1] == "" → drop
//
// On pass: p.lastItem/lastSlot snapshot → ClearPendingAction iff
// com.RootLayer != p.modalMain → moveClickRequest=false →
// faceEntity=-1 + emit entitymask (unconditional, matches TS) →
// fire [opheld<op>,<objId>] via GetByTrigger keyed on
// (objType.id, objType.Category) and runScript with protect=true.
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldHandler.ts:62-65
// calls addSessionLog(MODERATOR, ...) for op != 5. Skipped — no
// session-log subsystem in goscape. Closure path: future moderator-
// logging sub-spec ports LoggerEventType + session-log buffer.
func handleOpHeld(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 6 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	com := s.lookupComponent(comId)
	if com == nil || !com.Operable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) {
		return nil
	}
	objType := s.objTypes.Configs[obj]
	if objType == nil {
		return nil
	}
	if len(objType.IOp) < op || objType.IOp[op-1] == "" {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	if com.RootLayer != p.modalMain {
		p.ClearPendingAction()
	}

	p.moveClickRequest = false
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	trigger := script.TriggerOpHeld1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, obj, objType.Category)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }
```

- [ ] Add 5 registrations to `modules/world/handlers_game.go init()`. Insert after the existing OPOBJ block (around line 59, before MESSAGE_PUBLIC):

```go
	gameHandlers[195] = handleOpHeld1 // OPHELD1
	gameHandlers[71] = handleOpHeld2  // OPHELD2
	gameHandlers[133] = handleOpHeld3 // OPHELD3
	gameHandlers[157] = handleOpHeld4 // OPHELD4
	gameHandlers[211] = handleOpHeld5 // OPHELD5
```

### Step 1.3: Run reject-gate tests to verify they pass

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeld_ -count=1
```

Expected: PASS for all reject-gate tests from Step 1.1.

### Step 1.4: Write happy-path test (state mutation + script fire + mask emit)

- [ ] Append to `modules/world/handler_opheld_test.go`:

```go
// TestHandleOpHeld_HappyPath pins the success path: state mutated,
// trigger registered, script fired, mask emitted.
// Mirrors TS OpHeldHandler.ts:51-73.
func TestHandleOpHeld_HappyPath(t *testing.T) {
	s, p := setupOpHeldServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	// Sentinel pre-state.
	p.faceEntity = 7      // any non-(-1) value
	p.moveClickRequest = true
	p.masks = 0

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.moveClickRequest {
		t.Error("moveClickRequest: want false post-fire")
	}
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit not set (got %d)", p.masks)
	}
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}

// TestHandleOpHeld_Op2VariantFires pins that op=2 looks up TriggerOpHeld2.
// IOp[0]=="op1", IOp[1]=="op2" — both populated for this test.
func TestHandleOpHeld_Op2VariantFires(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "op2", "", "", ""}
	sf := &script.ScriptFile{
		Name:        "[opheld2,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld2, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeld2(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (op=2 dispatch must reach state mutation)", p.lastItem)
	}
}
```

- [ ] Run happy-path tests:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpHeld_HappyPath|TestHandleOpHeld_Op2VariantFires" -count=1
```

Expected: PASS.

### Step 1.5: Write modal-vs-rootLayer dual-pin tests

- [ ] Append to `modules/world/handler_opheld_test.go`:

```go
// TestHandleOpHeld_RootLayerMatchesModal_NoClearPending pins that when
// com.RootLayer == p.modalMain, ClearPendingAction is NOT called.
// Sentinel: pre-set p.activeScript to a non-nil ScriptFile and verify
// it survives the handler call. Mirrors TS OpHeldHandler.ts:54-56
// negative arm.
func TestHandleOpHeld_RootLayerMatchesModal_NoClearPending(t *testing.T) {
	s, p := setupOpHeldServer(t)
	// setupOpHeldServer already sets p.modalMain = 149 (matches com.RootLayer = 149)
	sf := &script.ScriptFile{
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	// Sentinel: a stale interaction target. ClearPendingAction would
	// null this; we verify it survives.
	p.target = p // any non-nil entity
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.target == nil {
		t.Error("ClearPendingAction was called but rootLayer matched modalMain (should be no-op)")
	}
}

// TestHandleOpHeld_RootLayerMismatch_ClearsPending pins that when
// com.RootLayer != p.modalMain, ClearPendingAction IS called.
// Mirrors TS OpHeldHandler.ts:54-56 positive arm.
func TestHandleOpHeld_RootLayerMismatch_ClearsPending(t *testing.T) {
	s, p := setupOpHeldServer(t)
	p.modalMain = 999 // != com.RootLayer (149)
	sf := &script.ScriptFile{
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.target != nil {
		t.Error("ClearPendingAction was NOT called but rootLayer mismatched modalMain (should clear)")
	}
}
```

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeld_RootLayer -count=1
```

Expected: PASS.

### Step 1.6: Run full package test suite

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: PASS. No regressions in adjacent handlers.

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS across all packages.

### Step 1.7: Commit

- [ ] Stage and commit:

```bash
git add modules/world/handler_opheld.go modules/world/handler_opheld_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-71 T1 — OPHELD1-5 handler port

Adds handleOpHeld shared core + 5 op-arity wrappers, registers them at
gameHandlers[195/71/133/157/211] in handlers_game.go init(). Mirrors
TS OpHeldHandler.ts line-by-line: 9 reject gates (delayed, short
payload, nil/non-operable/non-visible component, no listener, nil
inv, slot mismatch, nil ObjType, empty IOp slot), success path sets
p.lastItem/lastSlot, conditional ClearPendingAction on rootLayer
mismatch with p.modalMain, moveClickRequest=false, faceEntity=-1,
unconditional masks |= entitymask, then GetByTrigger keyed on
(objType.id, objType.Category) + runScript with protect=true.

DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: addSessionLog skipped
(no session-log subsystem in goscape).

Tests: 9 reject-gate pins + happy-path state/mask emit + op=2 variant
+ rootLayer-match/mismatch dual-pin.

Spec: docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 1.8: STOP for two-stage review

- [ ] Halt. Controller dispatches `superpowers:code-reviewer` (Sonnet) to audit T1's pattern (file-structure, naming convention, deviation tag wording, test coverage gaps) before T2/T3 mirror it. Resume only after review feedback is addressed.

---

## Task 2 — OPHELDT (handler + tests)

**Files:**
- Modify: `modules/world/handler_opheld.go` (append)
- Modify: `modules/world/handlers_game.go` (add 1 init() line)
- Modify: `modules/world/handler_opheld_test.go` (append)

### Step 2.1: Pre-flight re-grep

- [ ] Re-verify HEAD lines (T1 may have shifted them):

```bash
rg -n "func handleOpHeld\b|func handleOpHeld[1-5]" modules/world/handler_opheld.go
rg -n "OPHELD1|OPHELDT" modules/world/handlers_game.go
```

### Step 2.2: Write failing OPHELDT tests

- [ ] Append helpers + tests to `handler_opheld_test.go`:

```go
// opHeldTPayload encodes an 8-byte OPHELDT payload
// (obj:G2 slot:G2 com:G2 spellCom:G2).
func opHeldTPayload(obj, slot, com, spellCom int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
		byte(spellCom >> 8), byte(spellCom),
	}
}

// setupOpHeldTServer extends setupOpHeldServer with a spell component
// at id=200 that has ActionTarget&HELD set and is visible.
func setupOpHeldTServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s, p := setupOpHeldServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[1] = 200 // spell tab visible
	return s, p
}

func TestHandleOpHeldT_Delayed(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed reject)", p.lastItem)
	}
}

func TestHandleOpHeldT_ShortPayload(t *testing.T) {
	_, p := setupOpHeldTServer(t)
	_ = handleOpHeldT(p, []byte{0, 0, 0, 0, 0, 0, 0}) // 7 bytes
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TS OpHeldTHandler.ts:21-23 — spellCom: nil or actionTarget&HELD == 0.
func TestHandleOpHeldT_SpellComMissingHeldFlag(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: 0}, // HELD flag clear
	})
	p.tabs[1] = 200
	_ = s
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (spellCom missing HELD flag)", p.lastItem)
	}
}

// TS OpHeldTHandler.ts:30-32 — com: nil or !Usable.
func TestHandleOpHeldT_ComNotUsable(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: false}, // Usable cleared
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	_ = s
	p.tabs[1] = 200
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (com not Usable)", p.lastItem)
	}
}

// Happy path — script registered, fires, state mutated.
func TestHandleOpHeldT_HappyPath(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldt,200]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldT, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	p.faceEntity = 7
	p.target = p // sentinel for unconditional ClearPendingAction
	p.opcalled = true
	p.masks = 0

	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.target != nil {
		t.Error("ClearPendingAction must be unconditional in OPHELDT (target should be nil)")
	}
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Error("masks: entitymask bit not set")
	}
}

// TS OpHeldTHandler.ts:71-73 — no script ⇒ "Nothing interesting happens.".
func TestHandleOpHeldT_NoScript_NothingInteresting(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	_ = s
	cc := drainPlayerConn(t, p) // helper to drain conn — see note below
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	p.client.flushWrite()
	got := <-cc
	if !contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("want \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}
```

> **Note for implementer:** `drainPlayerConn` and `contains` helpers may not exist exactly under those names. Use the existing pattern from `handler_opobj_test.go:583-602` (`drainConn(t, cc)` returning a channel, `bytes.Contains(got, ...)`) — adapt the test to use whichever convention is in `modules/world/test_*.go`. Re-grep `rg "drainConn|drainPlayerConn|bytes\\.Contains" modules/world/` and follow whichever idiom lands in 5+ existing tests.

- [ ] Run failing tests:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeldT_ -count=1
```

Expected: ALL FAIL — `undefined: handleOpHeldT`.

### Step 2.3: Implement `handleOpHeldT`

- [ ] Append to `modules/world/handler_opheld.go`:

```go
// handleOpHeldT is the handler for OPHELDT (opcode 48, 8-byte payload).
// Spell-on-held-item: player drags a spell from the magic-book interface
// onto an inventory item.
// Wire format: obj:G2 | slot:G2 | com:G2 | spellCom:G2.
//
// Gates per TS OpHeldTHandler.ts:
//  1. p.delayed → drop
//  2. payload < 8 → drop
//  3. spellCom: nil or (ActionTarget & HELD) == 0 → drop
//  4. spellCom: !IsComponentVisible → drop
//  5. com: nil or !Usable → drop
//  6. com: !IsComponentVisible → drop
//  7. comId not in invListeners → drop
//  8. listener's inventory unresolved → drop
//  9. inv.HasAt(slot, obj) false → drop
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction
// (unconditional, contrast OPHELD1-5 conditional) → faceEntity=-1 +
// emit entitymask → fire [opheldt,<spellComId>] via
// GetByTrigger(typeID=spellComId, cat=-1). On no-script: emit
// "Nothing interesting happens.".
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldTHandler.ts:61
// addSessionLog skipped.
func handleOpHeldT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 8 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	spellComId := int(r.G2())

	spellCom := s.lookupComponent(spellComId)
	if spellCom == nil || (spellCom.ActionTarget&objtype.ComActionTargetHeld) == 0 {
		return nil
	}
	if !p.IsComponentVisible(spellCom) {
		return nil
	}

	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	p.ClearPendingAction()
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	sf := s.scriptProvider.GetByTrigger(script.TriggerOpHeldT, spellComId, -1)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
```

- [ ] Add the `objtype` import if not already present at top of file (T1 should have included `pkg/script`; objtype may need to be added). Confirm import block reads:

```go
import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] Add `gameHandlers[48] = handleOpHeldT // OPHELDT` to `handlers_game.go init()` after the OPHELD1-5 block.

### Step 2.4: Run all OPHELD tests

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeld -count=1
```

Expected: PASS for both T1 (`TestHandleOpHeld_*`) and new T2 (`TestHandleOpHeldT_*`) sets.

- [ ] Run full suite:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS.

### Step 2.5: Commit

- [ ] Stage and commit:

```bash
git add modules/world/handler_opheld.go modules/world/handler_opheld_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-71 T2 — OPHELDT handler port

Adds handleOpHeldT for spell-on-held-item interactions, registers it
at gameHandlers[48]. Mirrors TS OpHeldTHandler.ts: 9 reject gates
(delayed, short payload, nil/non-HELD-target spellCom, non-visible
spellCom, nil/non-usable com, non-visible com, no listener, nil inv,
slot mismatch); success path snapshots lastItem/lastSlot,
unconditional ClearPendingAction (vs OPHELD1-5 conditional),
faceEntity=-1, masks |= entitymask, then GetByTrigger keyed on
spellComId. No-script path emits "Nothing interesting happens."
matching goscape interaction_trigger convention.

DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG covers OpHeldTHandler.ts:61
addSessionLog skip.

Tests: 4 reject pins + happy path (state/mask + unconditional clear)
+ no-script "Nothing interesting" pin.

Spec: docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — OPHELDU (handler + 4-step trigger fallback + tests)

**Files:**
- Modify: `modules/world/handler_opheld.go` (append)
- Modify: `modules/world/handlers_game.go` (add 1 init() line)
- Modify: `modules/world/handler_opheld_test.go` (append)

### Step 3.1: Pre-flight re-grep

- [ ] Verify HEAD state:

```bash
rg -n "func handleOpHeld" modules/world/handler_opheld.go
rg -n "OPHELDU\b" modules/world/handlers_game.go pkg/io/protocol/game/client/prot.go
```

- [ ] Re-confirm `Inventory.HasAt` and `(*Server).cfg.NodeMembers` shape:

```bash
rg -n "func.*HasAt\b" pkg/inventory/
rg -n "NodeMembers\b" modules/world/server.go modules/world/handler_oploc.go
```

### Step 3.2: Write failing OPHELDU tests — reject gates + happy paths for each fallback arm

- [ ] Append to `handler_opheld_test.go`:

```go
// opHeldUPayload encodes a 12-byte OPHELDU payload.
func opHeldUPayload(obj, slot, com, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// setupOpHeldUServer extends setupOpHeldServer with a second item at
// slot 5 (id=777) so item-on-item swaps can be pinned. Both 555 and 777
// have Category=-1 so category-fallback arms inactive by default.
func setupOpHeldUServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s, p := setupOpHeldServer(t)
	s.invs[93].Items[5] = &inventory.Item{Id: 777, Count: 1}
	s.objTypes.Configs[777] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 777, DebugName: "test_held2"},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}
	return s, p
}

func TestHandleOpHeldU_Delayed(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed)", p.lastItem)
	}
}

func TestHandleOpHeldU_ShortPayload(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	_ = handleOpHeldU(p, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // 11 bytes
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TS OpHeldUHandler.ts:21-24 — comId !== useComId reject.
// Goscape uses != since both are int.
func TestHandleOpHeldU_ComMismatch(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (comId != useComId reject)", p.lastItem)
	}
}

// Happy path arm (a): [opheldu,objType.id] hits — no swap.
// TS OpHeldUHandler.ts:96-97 ("[opheldu,b]" in TS labelling but lookup
// is on objType.id which is the dragged item).
func TestHandleOpHeldU_ArmA_NoSwap(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldu,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldU, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem (a): got %d, want 555 (no swap)", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot (a): got %d, want 3", p.lastSlot)
	}
	if p.lastUseItem != 777 {
		t.Errorf("lastUseItem (a): got %d, want 777", p.lastUseItem)
	}
	if p.lastUseSlot != 5 {
		t.Errorf("lastUseSlot (a): got %d, want 5", p.lastUseSlot)
	}
}

// Arm (b): [opheldu,useObjType.id] hits when (a) misses — SWAP both pairs.
// TS OpHeldUHandler.ts:99-103.
func TestHandleOpHeldU_ArmB_SwapsItemAndSlot(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldu,777]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldU, 777),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	// Pre-swap: lastItem=555, lastSlot=3, lastUseItem=777, lastUseSlot=5
	// Post-swap: lastItem=777, lastSlot=5, lastUseItem=555, lastUseSlot=3
	if p.lastItem != 777 {
		t.Errorf("lastItem (b): got %d, want 777 (swapped)", p.lastItem)
	}
	if p.lastSlot != 5 {
		t.Errorf("lastSlot (b): got %d, want 5 (swapped)", p.lastSlot)
	}
	if p.lastUseItem != 555 {
		t.Errorf("lastUseItem (b): got %d, want 555 (swapped)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot (b): got %d, want 3 (swapped)", p.lastUseSlot)
	}
}

// Arm (c): [opheldu,-1,objType.Category] hits — no swap.
func TestHandleOpHeldU_ArmC_CategoryB_NoSwap(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.objTypes.Configs[555].Category = 100  // category set so category-fallback active
	sf := &script.ScriptFile{
		Name:        "[opheldu,_,100]",
		LookupKey:   script.LookupKeyForCategory(script.TriggerOpHeldU, 100),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem (c): got %d, want 555 (no swap)", p.lastItem)
	}
	if p.lastUseItem != 777 {
		t.Errorf("lastUseItem (c): got %d, want 777", p.lastUseItem)
	}
}

// Arm (d): [opheldu,-1,useObjType.Category] hits — SWAPS.
func TestHandleOpHeldU_ArmD_CategoryA_Swaps(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.objTypes.Configs[777].Category = 200
	sf := &script.ScriptFile{
		Name:        "[opheldu,_,200]",
		LookupKey:   script.LookupKeyForCategory(script.TriggerOpHeldU, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	if p.lastItem != 777 {
		t.Errorf("lastItem (d): got %d, want 777 (swapped)", p.lastItem)
	}
	if p.lastSlot != 5 {
		t.Errorf("lastSlot (d): got %d, want 5 (swapped)", p.lastSlot)
	}
	if p.lastUseItem != 555 {
		t.Errorf("lastUseItem (d): got %d, want 555 (swapped)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot (d): got %d, want 3 (swapped)", p.lastUseSlot)
	}
}

// All four arms miss ⇒ "Nothing interesting happens." emitted.
func TestHandleOpHeldU_AllMiss_NothingInteresting(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	_ = s
	cc := drainPlayerConn(t, p)
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	p.client.flushWrite()
	got := <-cc
	if !contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("want \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}

// TS OpHeldUHandler.ts:90-93 — members-only item on free world.
func TestHandleOpHeldU_MembersOnFreeWorld_Rejects(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.cfg.NodeMembers = false
	s.objTypes.Configs[555].Members = true
	cc := drainPlayerConn(t, p)
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	p.client.flushWrite()
	got := <-cc
	if !contains(got, []byte("To use this item please login to a members' server.")) {
		t.Errorf("want members-message in drained bytes, got %x", got)
	}
	// Per TS: returns false BEFORE script lookup ⇒ no script fired.
	// State mutation already happened pre-check (per TS:78-81); verify
	// lastItem still set to obj (= 555) but no script ran.
	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (TS sets state pre-members-check)", p.lastItem)
	}
}
```

> **Note for implementer:** `script.LookupKeyForCategory` may need verification — the spec assumes a category-keyed lookup helper exists alongside `LookupKeyForType`. Re-grep `pkg/script/provider.go` and `pkg/script/lookup.go` (or wherever) for the exact category-fallback key constructor. If only `LookupKeyForType` exists, the test fixture must construct the lookup key the same way `Provider.GetByTriggerSpecific` does internally — read `pkg/script/provider.go:145+` to confirm the key shape for the `(typeID=-1, categoryID=N)` arm.

> **Drain helper note (same as T2):** if `drainPlayerConn` doesn't exist verbatim, port the `drainConn(t, cc)` pattern from `handler_opobj_test.go`. Consider adding `drainPlayerConn(t, p) <-chan []byte` as a helper at the top of `handler_opheld_test.go` if multiple T2/T3 tests need it.

- [ ] Run failing tests:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeldU_ -count=1
```

Expected: ALL FAIL — `undefined: handleOpHeldU`.

### Step 3.3: Implement `handleOpHeldU`

- [ ] Append to `modules/world/handler_opheld.go`:

```go
// handleOpHeldU is the handler for OPHELDU (opcode 130, 12-byte payload).
// Item-on-held-item: player drags one inventory item onto another.
// Wire format: obj:G2 | slot:G2 | com:G2 | useObj:G2 | useSlot:G2 | useCom:G2.
//
// Gates per TS OpHeldUHandler.ts:
//  1. p.delayed → drop
//  2. payload < 12 → drop
//  3. comId != useComId → drop
//  4. com: nil or !Usable → drop
//  5. com: !IsComponentVisible → drop
//  6. useCom: nil or !Usable → drop
//  7. useCom: !IsComponentVisible → drop
//  8. comId not in invListeners → drop
//  9. listener's inventory unresolved → drop
//  10. inv.HasAt(slot, obj) false → moveClickRequest=false +
//      ClearPendingAction + drop (TS OpHeldUHandler.ts:54-58)
//  11. useComId not in invListeners → drop
//  12. listener's inventory unresolved → drop
//  13. useInv.HasAt(useSlot, useObj) false → moveClickRequest=false +
//      ClearPendingAction + drop (TS OpHeldUHandler.ts:71-75)
//
// On pass: lastItem/lastSlot/lastUseItem/lastUseSlot snapshot →
// ClearPendingAction (unconditional) → faceEntity=-1 + emit entitymask
// → members-only gate: free world + (objType.Members ||
// useObjType.Members) ⇒ MessageGame "To use this item please login..."
// + drop.
//
// Trigger fallback (4 arms; first hit wins):
//   (a) GetByTriggerSpecific(OPHELDU, objType.id, -1)    — no swap
//   (b) GetByTriggerSpecific(OPHELDU, useObjType.id, -1) — SWAP
//                                                          (lastItem,lastUseItem)
//                                                          AND
//                                                          (lastSlot,lastUseSlot)
//   (c) GetByTriggerSpecific(OPHELDU, -1, objType.Category)    — no swap
//                                                                (only if objType.Category != -1)
//   (d) GetByTriggerSpecific(OPHELDU, -1, useObjType.Category) — SWAP
//                                                                (only if useObjType.Category != -1)
//
// On miss across all 4: MessageGame "Nothing interesting happens.".
//
// Note on TS labelling: TS calls (a)/(b) "[opheldu,b]/[opheldu,a]"
// where 'b' = the inventory-listed (dragged-from) item and 'a' = the
// dragged-onto target. Goscape's implementation is byte-identical; the
// (a)/(b)/(c)/(d) labelling here is plan-local for clarity.
func handleOpHeldU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 12 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useComId := int(r.G2())

	if comId != useComId {
		return nil
	}

	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Usable {
		return nil
	}
	if !p.IsComponentVisible(useCom) {
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		// TS OpHeldUHandler.ts:54-58 — extra cleanup on this specific reject.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}

	useListener, ok := p.invListeners[useComId]
	if !ok {
		return nil
	}
	useInv := resolveListenerInv(s, useListener)
	if useInv == nil {
		return nil
	}
	if !useInv.HasAt(useSlot, useObj) {
		// TS OpHeldUHandler.ts:71-75.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}

	// State snapshot BEFORE members gate (matches TS:78-81 ordering).
	p.lastItem = obj
	p.lastSlot = slot
	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	// ObjType resolution for both objects.
	if s.objTypes == nil {
		return nil
	}
	if obj < 0 || obj >= len(s.objTypes.Configs) || s.objTypes.Configs[obj] == nil {
		return nil
	}
	if useObj < 0 || useObj >= len(s.objTypes.Configs) || s.objTypes.Configs[useObj] == nil {
		return nil
	}
	objType := s.objTypes.Configs[obj]
	useObjType := s.objTypes.Configs[useObj]

	p.ClearPendingAction()
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	// Members-only gate (TS OpHeldUHandler.ts:90-93).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}

	// 4-arm trigger lookup (TS OpHeldUHandler.ts:96-117).
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)
	if sf == nil {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
		if sf != nil {
			p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
			p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
		}
	}

	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
	}

	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		if sf != nil {
			p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
			p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
		}
	}

	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}

	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
```

- [ ] Add `gameHandlers[130] = handleOpHeldU // OPHELDU` to `handlers_game.go init()` after the OPHELDT registration.

### Step 3.4: Run all OPHELD tests

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpHeld -count=1
```

Expected: PASS for the full T1+T2+T3 suite.

- [ ] Run full suite + race detector:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

### Step 3.5: Commit

- [ ] Stage and commit:

```bash
git add modules/world/handler_opheld.go modules/world/handler_opheld_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-71 T3 — OPHELDU handler port (item-on-item, 4-arm trigger fallback)

Adds handleOpHeldU for item-on-held-item interactions, registers it
at gameHandlers[130]. Mirrors TS OpHeldUHandler.ts: 13 reject gates
(includes 2 special arms with extra cleanup on slot/item mismatch),
state snapshot, unconditional ClearPendingAction, faceEntity=-1,
masks |= entitymask, members-only gate, then 4-arm trigger lookup
with lastItem/lastSlot swap on the (b) and (d) arms (per
OpHeldUHandler.ts:101-102, 115-116). On no-script: emit "Nothing
interesting happens.".

Tests: 11 cases — 3 reject pins (delayed, short, comId mismatch),
4 fallback arms with explicit swap-pin (a/c no-swap, b/d swap),
all-miss "Nothing interesting" pin, members-only-on-free-world pin.

Spec: docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 3.6: STOP for two-stage review

- [ ] Halt. Controller dispatches `superpowers:code-reviewer` (Sonnet) for pre-close review of the full T1+T2+T3 commit chain. Reviewer focus areas:
  - 4-arm trigger fallback ordering matches TS OpHeldUHandler.ts:96-117 exactly
  - Both swap pairs (`lastItem↔lastUseItem` AND `lastSlot↔lastUseSlot`) execute together, not separately
  - `objType.Category != -1` guard mirrors TS `objCategory` truthy check (line 107) — neither (c) nor (d) arm runs when Category is -1
  - "(goscape defensive; TS throws here)" comment present at ObjType-nil branches
  - DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG inline tags present at the 2 sites enumerated in spec §3.3
  - Drain-conn helper convention consistent with `handler_opobj_test.go`
  - Resume only after review feedback is addressed.

---

## Task 4 — Close commit + memory entry

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-71 close section)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (no change unless new lessons surface)
- Create memory file: only if a new lesson surfaces during T1-T3. Default expectation: no new memory file.

### Step 4.1: Append NAI-71 close section to nai_followups.md

- [ ] Append the following block to `nai_followups.md` (use Edit, not Write — file is large):

```markdown

---

## NAI-71 — CLOSED 2026-05-02

**Scope:** OPHELD handler family port — adds 7 game-opcode handlers
(OPHELD1-5, OPHELDT, OPHELDU) closing the silent-discard gap in
`modules/world/handlers_game.go gameHandlers[]` for inventory-side
held-item interaction. Mirrors TS `OpHeldHandler` / `OpHeldTHandler`
/ `OpHeldUHandler` line-by-line.

**Cadence:** Full sub-spec, 3 implementation tasks + close. Two-stage
review at T1 (pattern-lock, Sonnet) and T3 (pre-close, Sonnet).

**Spec:** `docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-02-nai-71-opheld-handler-port.md`.

**Commit chain (HEAD → predecessor):**
T4: (this commit). T3: <T3-SHA>. T2: <T2-SHA>. T1: <T1-SHA>.

**Follow-ups closed:** none.

**Deviations opened:** `NAI-71-D-OPHELD-NO-SESSION-LOG` — TS
`addSessionLog(MODERATOR, ...)` calls at OpHeldHandler.ts:64 and
OpHeldTHandler.ts:61 are skipped pending session-log subsystem port.
Closure path: future moderator-logging sub-spec ports
LoggerEventType + session-log buffer; OPHELD is one of many call
sites that will activate.

**Deviations closed:** none.

**Net deviation tally:** -0 closure, +1 open = 12 → 13.

**Wire-behaviour delta at HEAD:**
- Inventory right-click on a held item (OPHELD1-5) now fires the
  registered `[opheld<op>,<objId>]` script. Pre-fix: silent discard.
- Spell-on-inventory-item drag (OPHELDT) now fires
  `[opheldt,<spellComId>]` or emits "Nothing interesting happens.".
- Inventory-item-on-inventory-item drag (OPHELDU) now fires the
  4-arm fallback chain or emits "Nothing interesting happens.".
- Members-only items on free worlds are rejected with the canonical
  "To use this item please login to a members' server." message.

**Lessons confirmed:**
- `runescript_cadence.md` — full 3-task cadence + two-stage review.
- `true_to_ts_gate.md` — every behavioural decision cited against TS
  source line.
- `controller_preflight.md` — pre-flight grep gates ran clean across
  T1/T2/T3 dispatch.
- `dead_api_polish.md` — drove rejection of original Scope A (anticheat
  no-op stubs with zero behavioral payoff) during brainstorm.
- `risk_register_premise_grep.md` — every spec §6 premise verified at
  spec-write with grep+Read.
- `defensive_gate_doc_comment_label.md` — ObjType-nil gates labeled
  "(goscape defensive; TS throws here)".
- `plan_grep_helper_patterns.md` — reuses `lookupComponent`,
  `IsComponentVisible`, `resolveListenerInv`, `inv.HasAt`,
  `runScript` instead of inlining.
- `enumerate_all_sites.md` — all 7 OPHELD opcode sites enumerated in
  spec §1 + §3 and registered in `handlers_game.go`.
- `superpowers_code_reviewer_model.md` — both review dispatches on
  Sonnet only.
- `execution_mode_default.md` — dispatch via subagent-driven-development.
- `close_commit_memory_trailer.md` — close commit carries
  `Opens memory: NAI-71-D-OPHELD-NO-SESSION-LOG`.
- `ts_asymmetry_dual_pin.md` — applied to OPHELDU 4-step fallback
  arms (every arm has a presence-pin AND the all-miss absence-pin).

**Lessons surfaced (saved as memory entries):**
- (TBD — implementer/reviewer fills this at close; default: none new.)

**Carry-forwards (still open after NAI-71):**
- `NAI-71-D-OPHELD-NO-SESSION-LOG` (new) — addSessionLog skipped pending
  session-log subsystem port.
- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — Player respawn/death sub-spec.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip.
- `NAI-35-T3-D1` op[1] operability gate audit.
- All other deferred carry-forwards from NAI-65 / NAI-66 / NAI-67 /
  NAI-68 / NAI-69 / NAI-70 unchanged.
```

### Step 4.2: Verify deviation tag is grep-able

- [ ] Run:

```bash
rg -n "NAI-71-D-OPHELD-NO-SESSION-LOG" modules/world/
```

Expected: 2+ hits (one in `handleOpHeld` doc-comment, one in `handleOpHeldT` doc-comment). If 0 hits, T1/T2 missed the deviation tag — fix before committing close.

### Step 4.3: Final full test run

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1
```

Expected: PASS.

### Step 4.4: Close commit

- [ ] Stage memory file changes (note: outside the goscape repo):

```bash
git -C /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory add nai_followups.md
git -C /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory commit --no-gpg-sign -m "memory: NAI-71 close — OPHELD handler family port"
```

- [ ] Stage close-commit in goscape repo:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-71 — OPHELD handler family port (7 opcodes)

Closes the silent-discard gap in modules/world/handlers_game.go for
opcodes 195/71/133/157/211 (OPHELD1-5), 48 (OPHELDT), and 130 (OPHELDU).
Mirrors TS OpHeldHandler / OpHeldTHandler / OpHeldUHandler line-by-line.

Net deviation tally:
  Spec claimed:  12 → 13 (close 0, open 1)
  Actual:        12 → 13

Implementation timeline:
  T1 OPHELD1-5 shared core + 5 wrappers + 11 tests
  T2 OPHELDT handler + 5 tests
  T3 OPHELDU handler + 4-arm trigger fallback + 11 tests

Wire-behaviour delta:
- Inventory right-click held items: silent discard → script fire
- Spell-on-inventory-item: silent discard → spell script fire +
  "Nothing interesting happens." fallback
- Item-on-item: silent discard → 4-arm trigger fallback with
  lastItem/lastSlot swap + members-gate + fallback message

Spec: docs/superpowers/specs/2026-05-02-nai-71-opheld-handler-port-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-71-opheld-handler-port.md

Opens memory: NAI-71-D-OPHELD-NO-SESSION-LOG

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

> **Note:** the close commit is `--allow-empty` because all production changes already landed in T1/T2/T3. The close is a narrative checkpoint, not a code change. (Alternative: skip the close commit entirely if T3's commit already serves as the implementation tail; this is project convention — verify against recent NAI-N close commits at memory entry time.)

---

## Self-review (controller pre-dispatch)

**Spec coverage check:**

| Spec section | Plan task |
|---|---|
| §3.1 OPHELD1-5 handler + 5 wrappers + handlers_game registrations | T1 |
| §3.1 OPHELDT handler + handlers_game registration | T2 |
| §3.1 OPHELDU handler + 4-arm fallback + handlers_game registration | T3 |
| §3.2 reject-gate test coverage | T1 (10 tests) + T2 (4) + T3 (3) |
| §3.2 happy-path script-fire pin | T1 + T2 + T3 (×4 fallback arms) |
| §3.2 OPHELD1-5 modal-vs-rootLayer dual-pin | T1 step 1.5 |
| §3.2 OPHELDU 4-arm fallback per-arm presence-pin + all-miss absence-pin | T3 step 3.2 |
| §3.2 OPHELDU members-only gate | T3 step 3.2 |
| §3.3 deviation tag NAI-71-D-OPHELD-NO-SESSION-LOG inline at 2 sites | T1 (1 site) + T2 (1 site) |
| §6 premise re-verification at HEAD | Pre-flight section + T2.1 + T3.1 |
| Memory close section | T4 |

**Placeholder scan:** No `TODO`, `TBD`, `placeholder`, or "fill in details" in implementation steps. Two implementer-side `Note for implementer:` blocks reference helper-name verification — these are intentional pre-flight checks, not unfilled placeholders.

**Type / signature consistency:**
- `handleOpHeld(p *Player, payload []byte, op int) error` — used across T1 step 1.2 and dispatch wrappers.
- `script.TriggerOpHeld1 + script.ServerTriggerType(op-1)` — used in T1 (matches `pkg/script/trigger.go:142-148` consts).
- `s.scriptProvider.GetByTrigger(trigger, typeID, categoryID)` — T1 (with category).
- `s.scriptProvider.GetByTriggerSpecific(trigger, typeID, categoryID)` — T3 (no category fallback within helper).
- `s.runScript(sf, p, nil, true, nil, nil)` — T1, T2, T3 all consistent.
- `Player` field names (`lastItem`, `lastSlot`, `lastUseItem`, `lastUseSlot`, `moveClickRequest`, `faceEntity`, `entitymask`, `masks`, `modalMain`) — verified against `modules/world/player.go:204, 212, 227` at spec-write.
- `objtype.ComActionTargetHeld` — verified `pkg/objtype/componenttype.go:42`.

No inconsistencies between tasks.

**Risk-register cross-check:** All §6 premises in the spec have a corresponding pre-flight grep step in either the plan's Pre-flight section or T2.1/T3.1 re-grep blocks.

---

## Post-review handoff

After T4 close, controller emits a paste-ready resume prompt for the
next NAI per `post_task_handoff.md`. Template:

```
Begin NAI-72. HEAD: <T4-SHA> (NAI-71 closed; net deviation tally 13).

Pre-brainstorm: grep for "NAI-N" deferred items in memory and recent
NAI close commits to surface candidates. Apply runescript_cadence
(brainstorm → spec → plan → subagent-driven TDD with two-stage review)
and plan_geometry_premise_pretrace (controller pre-traces
arithmetic/geometry premises before dispatch). Use Sonnet for any
superpowers:code-reviewer dispatches. /clear is recommended between
spec/plan and implementation per superpowers_clear_between_spec_and_impl.
```
