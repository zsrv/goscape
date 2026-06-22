# NAI-60 — component-registry cluster cleanup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire NAI-59's `s.lookupComponent` + `Player.IsComponentVisible` into 5 client-message handler families, retiring 10 deviation sites carrying 8 distinct tags.

**Architecture:** Three uniform gate templates (T-variant, U-variant, InvButton family) applied across handlers. Each task takes one cluster end-to-end (test → impl → existing-test seeding → comment retirement). Ports of canonical TS handlers under `Engine-TS/src/network/game/client/handler/`.

**Tech Stack:** Go 1.26+. Module: `modules/world`. Packages: `pkg/objtype` (ComponentType + ComActionTarget constants), `pkg/script` (script provider + triggers).

---

## §0. Pre-flight (one-time, controller verification)

Before dispatching any implementer, the controller must verify these premises against HEAD:

```bash
# 1. ComActionTarget bitmask values match TS (OBJ=1, NPC=2, LOC=4, PLAYER=8, HELD=16)
sed -n '36,43p' pkg/objtype/componenttype.go
# Expected: matches TS Component.ts:321-327

# 2. lookupComponent helper exists
sed -n '14,21p' modules/world/handler_interface.go

# 3. IsComponentVisible exists
grep -n "func (p \*Player) IsComponentVisible" modules/world/player_interface.go

# 4. seedComponentTypes test helper exists
grep -n "func seedComponentTypes" modules/world/handler_interface_test.go

# 5. runIfButtonProtectScript helper exists at handler_interface_test.go:345
grep -n "func runIfButtonProtectScript" modules/world/handler_interface_test.go

# 6. Build is clean
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...

# 7. All tests pass at HEAD
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

If any premise fails, halt and re-validate the spec before dispatching.

## §1. File structure

**New files:**
- `modules/world/handler_component_gate_test.go` — table-driven helper + driver tests for the 8 Op*T/U handlers; lifted `runProtectScript` helper used by InvButton family tests.

**Modified production files:**
- `modules/world/handler_opnpc.go` — gate in `handleOpNpcT` (T1) and `handleOpNpcU` (T2); retire deviation comments.
- `modules/world/handler_opobj.go` — gate in `handleOpObjT` (T1) and `handleOpObjU` (T2); retire deviation comments.
- `modules/world/handler_oploc.go` — gate in `handleOpLocT` (T1) and `handleOpLocU` (T2); retire deviation comments.
- `modules/world/handler_op_player.go` — gate in `handleOpPlayerT` (T1) and `handleOpPlayerU` (T2); retire deviation comments.
- `modules/world/handler_inv_button.go` — gate + protect in `handleInvButton` and `handleInvButtonD` (T3); retire deviation comments.

**Modified test files:**
- `modules/world/handler_opnpc_test.go` — seed components in `TestHandleOpNpcTSetsInteraction` (line 238), `TestHandleOpNpcUSetsInteraction` (line 335), and any T/U happy-path tests that reach SetInteraction.
- `modules/world/handler_opobj_test.go` — seed components in `TestHandleOpObjTSetsInteraction` (line 237), `TestHandleOpObjUSetsInteraction` (line 331), and downstream happy-paths.
- `modules/world/handler_oploc_test.go` — seed components in `TestHandleOpLocTSetsInteraction` (line 293), `TestHandleOpLocUSetsInteraction` (and other U happy-paths); retire stale narrative `(S6m-D2/D3)` at line 430.
- `modules/world/handler_op_player_test.go` — seed components in OpPlayerT/U happy-paths.
- `modules/world/handler_inv_button_test.go` — seed components in `TestHandleInvButtonSetsStateAndRunsScript` (line 121), `TestHandleInvButtonOpVariant`, `TestHandleInvButtonDSetsStateAndRunsScript` (line 264), `TestHandleInvButtonDDelayedRevert`, and other downstream happy-paths.

---

## Task 1: T-variant gates (4 sites)

**Goal:** Wire the §2.1 gate from the spec into `handleOpLocT`, `handleOpNpcT`, `handleOpObjT`, `handleOpPlayerT`. Retire deviation comments for `S6m-D1`, `S6o-D1`, `NAI-50-D1`, `NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED` (T-side).

**Files:**
- Create: `modules/world/handler_component_gate_test.go`
- Modify: `modules/world/handler_oploc.go`, `handler_opnpc.go`, `handler_opobj.go`, `handler_op_player.go`
- Modify: `modules/world/handler_oploc_test.go`, `handler_opnpc_test.go`, `handler_opobj_test.go`, `handler_op_player_test.go`

**Per-handler controller pre-flight:**

```bash
# Verify deviation tag positions at HEAD
grep -n "DEVIATION (S6m-D1)" modules/world/handler_oploc.go         # expect ~114
grep -n "DEVIATION S6o-D1" modules/world/handler_opnpc.go           # expect ~97
grep -n "DEVIATION NAI-50-D1" modules/world/handler_opobj.go        # expect ~95
grep -n "DEVIATION NAI-40-D-COMPONENT-REGISTRY" modules/world/handler_op_player.go  # expect ~74

# Verify happy-path test sites
grep -n "func TestHandleOpLocTSetsInteraction\|func TestHandleOpNpcTSetsInteraction\|func TestHandleOpObjTSetsInteraction" modules/world/*_test.go
grep -n "TestHandleOpPlayerT" modules/world/handler_op_player_test.go
```

### T1 Steps

- [ ] **Step 1.1: Create the shared gate-test file with the table-driven helper and 4 driver tests for T-variants.**

Write `modules/world/handler_component_gate_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// compGateCase drives the 4-scenario gate test for an Op*T/U handler.
//
// payloadOK is a payload that would otherwise pass all gates (entity exists,
// is visible, listener resolves, etc). The helper rewrites or relies on the
// payload's component-id field to drive the gate-failure scenarios.
type compGateCase struct {
	name        string
	handler     func(*Player, []byte) error
	setupOk     func(t *testing.T, s *Server, p *Player) // seeds prerequisite state for happy-path
	payloadOK   []byte
	rootLayer   int  // RootLayer for the test component; placed at p.tabs[0] to satisfy IsComponentVisible
	flagBits    int  // T-variant: ActionTarget bitmask. U-variant: 0.
	isUVariant  bool // U: gate Usable. T: gate ActionTarget bits.
	comId       int  // component id referenced by payloadOK
}

// runCompGate exercises 4 scenarios per handler:
//   1. nil component (registry empty for c.comId)
//   2. flag fail (T: ActionTarget=0; U: Usable=false)
//   3. not visible (component registered but RootLayer not in any tab/modal)
//   4. happy-path (all gates pass)
func runCompGate(t *testing.T, c compGateCase) {
	t.Helper()

	// Scenario 1: nil component (no registry seed).
	t.Run(c.name+"/nil_component_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		// no seedComponentTypes call → registry has no entry for comId

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "nil component should reject")
	})

	// Scenario 2: flag fail.
	t.Run(c.name+"/flag_fail_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = 0 // wrong bit cleared
		} else {
			ct.Usable = false
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		p.tabs[0] = c.rootLayer

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "flag fail should reject")
	})

	// Scenario 3: not visible (component exists with passing flag, but root not in any slot).
	t.Run(c.name+"/not_visible_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = c.flagBits
		} else {
			ct.Usable = true
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		// note: do NOT set p.tabs[0] — root invisible

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "not visible should reject")
	})

	// Scenario 4: happy-path.
	t.Run(c.name+"/happy_path_accepts", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = c.flagBits
		} else {
			ct.Usable = true
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		p.tabs[0] = c.rootLayer

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if !p.opcalled {
			t.Errorf("opcalled: got false, want true (gate should pass)")
		}
		if p.target == nil {
			t.Errorf("target: got nil, want non-nil entity (SetInteraction should fire)")
		}
	})
}

// assertGateRejected verifies the handler bailed without setting interaction
// state. opcalled and p.target are the load-bearing post-gate side effects.
func assertGateRejected(t *testing.T, p *Player, msg string) {
	t.Helper()
	if p.opcalled {
		t.Errorf("opcalled: got true, want false (%s)", msg)
	}
	if p.target != nil {
		t.Errorf("target: got non-nil, want nil (%s)", msg)
	}
}
```

Add the 4 driver tests to the same file (one per T-handler). Helper functions for fixture-creation reuse the existing `makeOpNpcFixture` etc. — the driver tests adapt those into `setupOk` callbacks that ONLY seed the prerequisites (npc, loc, obj, target player) without seeding components.

```go
func TestComponentGate_OpNpcT(t *testing.T) {
	const npcSlot = 0
	const spellCom = 4242
	const rootLayer = 4242
	runCompGate(t, compGateCase{
		name:      "OpNpcT",
		handler:   handleOpNpcT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetNpc,
		rootLayer: rootLayer,
		payloadOK: []byte{0, npcSlot, byte(spellCom >> 8), byte(spellCom)},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedNpcAtSlot(t, s, p, npcSlot)
		},
	})
}

func TestComponentGate_OpObjT(t *testing.T) {
	const x, z = 100, 100
	const objId = 42
	const spellCom = 4243
	const rootLayer = 4243
	runCompGate(t, compGateCase{
		name:      "OpObjT",
		handler:   handleOpObjT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetObj,
		rootLayer: rootLayer,
		payloadOK: []byte{
			byte(x >> 8), byte(x),
			byte(z >> 8), byte(z),
			byte(objId >> 8), byte(objId),
			byte(spellCom >> 8), byte(spellCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedObjAt(t, s, p, x, z, objId)
		},
	})
}

func TestComponentGate_OpLocT(t *testing.T) {
	const x, z = 100, 100
	const locId = 42
	const spellCom = 4244
	const rootLayer = 4244
	runCompGate(t, compGateCase{
		name:      "OpLocT",
		handler:   handleOpLocT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetLoc,
		rootLayer: rootLayer,
		payloadOK: []byte{
			byte(x >> 8), byte(x),
			byte(z >> 8), byte(z),
			byte(locId >> 8), byte(locId),
			byte(spellCom >> 8), byte(spellCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedLocAt(t, s, p, x, z, locId)
		},
	})
}

func TestComponentGate_OpPlayerT(t *testing.T) {
	const otherSlot = 1
	const spellCom = 4245
	const rootLayer = 4245
	runCompGate(t, compGateCase{
		name:      "OpPlayerT",
		handler:   handleOpPlayerT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetPlayer,
		rootLayer: rootLayer,
		payloadOK: []byte{0, otherSlot, byte(spellCom >> 8), byte(spellCom)},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedTargetPlayerAtSlot(t, s, p, otherSlot)
		},
	})
}

// seedNpcAtSlot, seedObjAt, seedLocAt, seedTargetPlayerAtSlot are
// minimal fixture helpers extracted from the per-handler test files.
// Implementer: copy the relevant bits from makeOpNpcFixture/makeOpObjFixture/
// makeOpLocFixture/makeOpPlayerFixture (handler_*_test.go) into these
// helpers. Each helper installs ONLY the prerequisite state for the gate
// test — npc visible to player, loc/obj at coordinates, target player
// logged in and rsbuf-visible. They MUST NOT seed component types.
func seedNpcAtSlot(t *testing.T, s *Server, p *Player, slot int) { /* see makeOpNpcFixture */ }
func seedObjAt(t *testing.T, s *Server, p *Player, x, z, objId int) { /* see makeOpObjFixture */ }
func seedLocAt(t *testing.T, s *Server, p *Player, x, z, locId int) { /* see makeOpLocFixture */ }
func seedTargetPlayerAtSlot(t *testing.T, s *Server, p *Player, slot int) { /* see makeOpPlayerFixture */ }
```

**Implementer note on the 4 seed helpers:** read each `make*Fixture` function in the corresponding `_test.go`, extract the prerequisite-seeding code (server config, npc/loc/obj registry, player rsbuf entry, but NOT component seeding), and inline into the helpers above. Keep them minimal — gate tests only need enough state to reach the gate.

- [ ] **Step 1.2: Run the new tests and verify they FAIL (gate doesn't exist yet).**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpNpcT -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpObjT -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpLocT -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpPlayerT -v
```

Expected: each test's `nil_component_rejects`, `flag_fail_rejects`, and `not_visible_rejects` subtests FAIL (handler still proceeds to SetInteraction since gate is missing). `happy_path_accepts` may PASS.

- [ ] **Step 1.3: Add the gate to `handleOpNpcT` (`modules/world/handler_opnpc.go:114`).**

Replace the deviation comment block (lines 86-113) and the body up through line 117 (`s := p.client.server`) so the gate slots in immediately after the `delayed` check, before `slot < 0` validation.

Replace this block:

```go
// (existing) DEVIATION S6o-D1 ... and surrounding comment + handler signature
// up through the line:
//     spellCom := int(r.G2())
```

With:

```go
// handleOpNpcT is the handler for OPNPCT (opcode 134, 4-byte payload).
// Spell-on-NPC: player drags a spell icon onto an NPC.
// Payload = (slot:G2, spellCom:G2).
//
// Gates per TS OpNpcTHandler.ts:
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. spellCom: nil component or ActionTarget&NPC == 0 → UnsetMapFlag
//  4. spellCom: !IsComponentVisible → UnsetMapFlag
//  5. slot out of range → UnsetMapFlag
//  6. NPC nil or dead → UnsetMapFlag
//  7. NPC delayed → UnsetMapFlag
//  8. NPC not rsbuf-visible → UnsetMapFlag
//  9. NpcType nil → UnsetMapFlag
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

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetNpc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	// (rest of body unchanged from current handler_opnpc.go:138-160)
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.delayed && s.currentTick < npc.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, spellCom)
	return nil
}
```

Add the import for `objtype` to the file's import block:

```go
import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

- [ ] **Step 1.4: Add the gate to `handleOpObjT` (`modules/world/handler_opobj.go:99`).**

Position: AFTER `delayed` + `len(payload) < 8` + decode (no entity lookup yet — TS positions the gate before viewport check too). Replace the deviation block + handler body to match TS OpObjTHandler.ts:

```go
// handleOpObjT is the handler for OPOBJT (opcode 138, 8-byte payload).
// Spell-on-obj: player casts a spell onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, spellCom:G2).
//
// Gates per TS OpObjTHandler.ts:
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. spellCom: nil or ActionTarget&OBJ == 0 → UnsetMapFlag
//  4. spellCom: !IsComponentVisible → UnsetMapFlag
//  5. coords outside viewport (52-tile half-extent) → UnsetMapFlag
//  6. Server.GetObj returns nil → UnsetMapFlag
//  7. ObjType not registered → UnsetMapFlag
func handleOpObjT(p *Player, payload []byte) error {
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
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())
	spellCom := int(r.G2())

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetObj) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, spellCom)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to imports.

- [ ] **Step 1.5: Add the gate to `handleOpLocT` (`modules/world/handler_oploc.go:124`).**

Same pattern as OpObjT — position gate after delayed+payload+decode, before viewport check:

```go
// handleOpLocT — gate added per TS OpLocTHandler.ts ActionTarget&LOC.
func handleOpLocT(p *Player, payload []byte) error {
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
	x := int(r.G2())
	z := int(r.G2())
	locId := int(r.G2())
	spellCom := int(r.G2())

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetLoc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	// (rest of body unchanged from current handler_oploc.go:146-178: viewport,
	// loc lookup, locType registration, ClearPendingAction, SetInteraction,
	// targetSubject snapshot.)
}
```

Update the doc comment to remove the `S6m-D1` block (replace with the new gate-explainer above the function). Add `"github.com/zsrv/goscape/pkg/objtype"` import.

- [ ] **Step 1.6: Add the gate to `handleOpPlayerT` (`modules/world/handler_op_player.go:84`).**

```go
// handleOpPlayerT — gate added per TS OpPlayerTHandler.ts ActionTarget&PLAYER.
func handleOpPlayerT(p *Player, payload []byte) error {
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

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetPlayer) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, spellCom)
	return nil
}
```

Replace the deviation comment block with the gate-explainer doc. Add `"github.com/zsrv/goscape/pkg/objtype"` import.

- [ ] **Step 1.7: Run the new driver tests — should now PASS.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_Op -v
```

Expected: all 16 subtests (4 handlers × 4 scenarios) PASS.

- [ ] **Step 1.8: Run the existing handler tests — they will start FAILING for happy-paths.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpNpcTSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpObjTSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpLocTSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpPlayerT -v
```

Expected: failures because spellCom in those tests is unregistered.

- [ ] **Step 1.9: Update existing happy-path tests to seed component types.**

For each of the 4 affected tests, add this stanza near the top of the test body (after fixture creation, before invoking the handler):

```go
// gate satisfaction: register spellCom with passing ActionTarget bit and visibility.
seedComponentTypes(t, s, map[int]*objtype.ComponentType{
    <spellComValue>: {RootLayer: <spellComValue>, ActionTarget: objtype.ComActionTarget<X>},
})
p.tabs[0] = <spellComValue>
```

Concrete edits:

**`modules/world/handler_opnpc_test.go:238` (TestHandleOpNpcTSetsInteraction):**
The current test passes `spellCom` via `p2x4NpcPayload`. Read the existing test, identify the spellCom value (e.g., 7777 or similar), and add seeding using `objtype.ComActionTargetNpc`. Add `"github.com/zsrv/goscape/pkg/objtype"` import to the file if not already present.

**`modules/world/handler_opobj_test.go:237` (TestHandleOpObjTSetsInteraction):**
Existing test uses `spellCom = 7777` (visible at line 240). Add:

```go
seedComponentTypes(t, s, map[int]*objtype.ComponentType{
    7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
})
p.tabs[0] = 7777
```

Note `s` is the first return of `makeOpObjFixture` (line 238: `s, p, obj, _ := makeOpObjFixture(t)` already returns `s`).

**`modules/world/handler_oploc_test.go:293` (TestHandleOpLocTSetsInteraction):**
Same pattern. Read the existing test for the spellCom value used and add seeding with `ComActionTargetLoc`.

**`modules/world/handler_op_player_test.go` (TestHandleOpPlayerT*):**
Same pattern with `ComActionTargetPlayer`.

For each file: ensure `"github.com/zsrv/goscape/pkg/objtype"` is in the imports.

- [ ] **Step 1.10: Run all package tests — verify GREEN.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 1.11: Verify deviation-tag retirement is complete for T-handlers.**

```bash
grep -n "S6m-D1\|S6o-D1\|NAI-50-D1\|NAI-40-D-COMPONENT-REGISTRY" modules/world/handler_oploc.go modules/world/handler_opnpc.go modules/world/handler_opobj.go modules/world/handler_op_player.go
```

Expected: only references to `:134` site in `handler_op_player.go` (OpPlayerU — closes in T2) remain. No references in the T-variant code paths.

If cluster-mention narrative ("bundle with S6m-D1", "Same cluster as S6m-D1, NAI-48-D1", etc.) appears in T-handler files but the gate is now wired, the narrative is stale — delete the sentence. Per memory `retire_deviation_grep_all_comments`, full-sentence retirement is required, not just tag-substring patching.

- [ ] **Step 1.12: Run full repo tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 1.13: Commit.**

```bash
git add modules/world/handler_component_gate_test.go \
        modules/world/handler_opnpc.go modules/world/handler_opnpc_test.go \
        modules/world/handler_opobj.go modules/world/handler_opobj_test.go \
        modules/world/handler_oploc.go modules/world/handler_oploc_test.go \
        modules/world/handler_op_player.go modules/world/handler_op_player_test.go

git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-60 T1 — component gate on Op*T handlers

Wires s.lookupComponent + IsComponentVisible into handleOpLocT,
handleOpNpcT, handleOpObjT, handleOpPlayerT per TS *THandler.ts.
Each gate rejects on nil component, ActionTarget bit unset, or
component not visible to player; UnsetMapFlag emitted on every
reject path.

Adds shared table-driven test helper (handler_component_gate_test.go)
that drives the 4-scenario gate test (nil / flag-fail / not-visible /
happy-path) for each handler.

Closes NAI-60 T1 — retires S6m-D1, S6o-D1, NAI-50-D1, NAI-40-D-
COMPONENT-REGISTRY-VALIDATION-SKIPPED (T-side).
EOF
)"
```

---

## Task 2: U-variant gates (4 sites)

**Goal:** Wire the §2.2 gate from the spec into `handleOpLocU`, `handleOpNpcU`, `handleOpObjU`, `handleOpPlayerU`. Position varies per handler (Npc/Player: pre-lookup; Obj/Loc: post-lookup, pre-listener). Retire deviation comments for `S6m-D2`, `S6o-D2`, `NAI-50-D2`, `NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED` (U-side).

**Files:**
- Modify: `modules/world/handler_opnpc.go` (handleOpNpcU)
- Modify: `modules/world/handler_opobj.go` (handleOpObjU)
- Modify: `modules/world/handler_oploc.go` (handleOpLocU)
- Modify: `modules/world/handler_op_player.go` (handleOpPlayerU)
- Modify: `modules/world/handler_component_gate_test.go` (add 4 U-driver tests)
- Modify: `modules/world/handler_opnpc_test.go`, `handler_opobj_test.go`, `handler_oploc_test.go`, `handler_op_player_test.go` (seed components in existing happy-paths)
- Modify: `modules/world/handler_oploc_test.go:430` (retire stale `(S6m-D2/D3)` narrative)

**Per-handler controller pre-flight:**

```bash
grep -n "DEVIATION (S6m-D2)" modules/world/handler_oploc.go         # expect ~195
grep -n "DEVIATION S6o-D2" modules/world/handler_opnpc.go           # expect ~177
grep -n "DEVIATION NAI-50-D2" modules/world/handler_opobj.go        # expect ~159
grep -n "DEVIATION NAI-40-D-COMPONENT-REGISTRY" modules/world/handler_op_player.go  # expect ~134

grep -n "S6m-D2/D3" modules/world/handler_oploc_test.go             # expect ~430 (stale narrative)
```

### T2 Steps

- [ ] **Step 2.1: Add the 4 U-variant driver tests to `handler_component_gate_test.go`.**

Append after the 4 T-driver tests:

```go
func TestComponentGate_OpNpcU(t *testing.T) {
	const npcSlot = 0
	const useObj = 1511
	const useSlot = 3
	const useCom = 4246
	const rootLayer = 4246
	runCompGate(t, compGateCase{
		name:       "OpNpcU",
		handler:    handleOpNpcU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			0, npcSlot,
			byte(useObj >> 8), byte(useObj),
			byte(useSlot >> 8), byte(useSlot),
			byte(useCom >> 8), byte(useCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedNpcAtSlot(t, s, p, npcSlot)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpObjU(t *testing.T) {
	const x, z = 100, 100
	const objId = 42
	const useObj = 1511
	const useSlot = 3
	const useCom = 4247
	const rootLayer = 4247
	runCompGate(t, compGateCase{
		name:       "OpObjU",
		handler:    handleOpObjU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			byte(x >> 8), byte(x),
			byte(z >> 8), byte(z),
			byte(objId >> 8), byte(objId),
			byte(useObj >> 8), byte(useObj),
			byte(useSlot >> 8), byte(useSlot),
			byte(useCom >> 8), byte(useCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedObjAt(t, s, p, x, z, objId)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpLocU(t *testing.T) {
	const x, z = 100, 100
	const locId = 42
	const useObj = 1511
	const useSlot = 3
	const useCom = 4248
	const rootLayer = 4248
	runCompGate(t, compGateCase{
		name:       "OpLocU",
		handler:    handleOpLocU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			byte(x >> 8), byte(x),
			byte(z >> 8), byte(z),
			byte(locId >> 8), byte(locId),
			byte(useObj >> 8), byte(useObj),
			byte(useSlot >> 8), byte(useSlot),
			byte(useCom >> 8), byte(useCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedLocAt(t, s, p, x, z, locId)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpPlayerU(t *testing.T) {
	const otherSlot = 1
	const useObj = 1511
	const useSlot = 3
	const useCom = 4249
	const rootLayer = 4249
	runCompGate(t, compGateCase{
		name:       "OpPlayerU",
		handler:    handleOpPlayerU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			0, otherSlot,
			byte(useObj >> 8), byte(useObj),
			byte(useSlot >> 8), byte(useSlot),
			byte(useCom >> 8), byte(useCom),
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedTargetPlayerAtSlot(t, s, p, otherSlot)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

// seedListenerWithItem registers an inv listener at useCom pointing at world-
// shared invType=93, populates that inv with useObj at useSlot.
func seedListenerWithItem(t *testing.T, s *Server, p *Player, useCom, useSlot, useObj int) {
	t.Helper()
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[useSlot] = &inventory.Item{Id: useObj, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, useCom, -1)
}
```

Add `"github.com/zsrv/goscape/pkg/inventory"` import to `handler_component_gate_test.go` if not already present.

- [ ] **Step 2.2: Run the 4 new U-driver tests — expect FAILS for reject scenarios.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpNpcU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpObjU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpLocU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpPlayerU -v
```

Expected: nil/flag-fail/not-visible scenarios FAIL (handler proceeds without gate).

- [ ] **Step 2.3: Add the gate to `handleOpNpcU` (`modules/world/handler_opnpc.go:189`).**

Position: AFTER `delayed` + `len(payload) < 8` + decode, BEFORE slot validation. Matches TS `OpNpcUHandler.ts:24-33`.

```go
// (after decoding slot, useObj, useSlot, useCom)
com := s.lookupComponent(useCom)
if com == nil || !com.Usable {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}

if slot < 0 || slot >= len(s.npcs) {
    sendUnsetMapFlag(p)
    return nil
}
// (rest of body unchanged)
```

Replace the deviation comment block (lines 162-188) with a clean gate-explainer doc that mirrors the TS handler's gate ordering. Add `"github.com/zsrv/goscape/pkg/objtype"` import (likely already added in T1 — verify).

- [ ] **Step 2.4: Add the gate to `handleOpPlayerU` (`modules/world/handler_op_player.go:141`).**

Position: AFTER `delayed` + `len(payload) < 8` + decode, BEFORE listener resolution. Matches TS `OpPlayerUHandler.ts:24-33`.

```go
// (after decoding slot, useObj, useSlot, useCom)
com := s.lookupComponent(useCom)
if com == nil || !com.Usable {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}

listener, ok := p.invListeners[useCom]
// (rest of body unchanged)
```

Replace the second deviation comment block (lines 121-138) with a clean gate-explainer doc.

- [ ] **Step 2.5: Add the gate to `handleOpObjU` (`modules/world/handler_opobj.go:163`).**

Position: AFTER coord viewport + obj lookup + objType registration, BEFORE listener resolution. Matches TS `OpObjUHandler.ts:39-48`.

```go
// (after the existing block: viewport check, GetObj, objTypes nil/range/nil-config check)
// At HEAD this ends around line 209.

com := s.lookupComponent(useCom)
if com == nil || !com.Usable {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}

listener, ok := p.invListeners[useCom]
// (rest of body unchanged: listener resolution, slot/item validation,
// members-only gate, lastUseItem/lastUseSlot, ClearPendingAction,
// SetInteraction, targetSubject snapshot)
```

Replace the deviation comment block (lines 155-162) with a clean gate-explainer doc.

- [ ] **Step 2.6: Add the gate to `handleOpLocU` (`modules/world/handler_oploc.go`, after line 195).**

Position: AFTER coord viewport + loc lookup + locType registration, BEFORE listener resolution. Mirrors TS `OpLocUHandler.ts` — read the TS file at `$HOME/Code/github.com/LostCityRS/Engine-TS/src/network/game/client/handler/OpLocUHandler.ts` and confirm the gate position before insertion.

```go
// (after the existing block: viewport check, GetLoc, locTypes nil/range/nil-config check)

com := s.lookupComponent(useCom)
if com == nil || !com.Usable {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}

listener, ok := p.invListeners[useCom]
// (rest of body unchanged)
```

Replace the deviation comment block (lines 180-200 area) with a clean gate-explainer doc.

- [ ] **Step 2.7: Run the 4 U-driver tests — expect PASS.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpNpcU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpObjU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpLocU -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestComponentGate_OpPlayerU -v
```

Expected: 16 subtests PASS.

- [ ] **Step 2.8: Run existing U-handler happy-path tests — expect FAILS until seeded.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpNpcUSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpObjUSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpLocUSetsInteraction -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpPlayerU -v
```

Plus all derived happy-path siblings (e.g., `TestHandleOpNpcUHappyPathWithOtherPlayerInv`, `TestHandleOpNpcUMembersOnFreeWorldRejected`, `TestHandleOpNpcUMembersOnMembersWorldAllowed`, `TestHandleOpObjUItemMismatchRejected`, etc.).

Expected: failures from the new gate.

- [ ] **Step 2.9: Update existing U-handler tests to seed components.**

For every U-handler happy-path or downstream-side-effect test that passes the gate (i.e., reaches listener resolution or beyond), add:

```go
seedComponentTypes(t, s, map[int]*objtype.ComponentType{
    <useComValue>: {RootLayer: <useComValue>, Usable: true},
})
p.tabs[0] = <useComValue>
```

Re-grep to enumerate every site:

```bash
grep -n "handleOpNpcU\|handleOpObjU\|handleOpLocU\|handleOpPlayerU" modules/world/handler_opnpc_test.go modules/world/handler_opobj_test.go modules/world/handler_oploc_test.go modules/world/handler_op_player_test.go
```

Each test that uses the handler AND expects the listener resolution / inv mismatch / members-only / SetInteraction code path needs the seeding. Tests that exercise pre-gate rejects (delayed, short payload) do NOT need seeding — those reject before the gate.

Per memory `enumerate_all_sites`, list every affected test function explicitly:

- `handler_opnpc_test.go`:
  - `TestHandleOpNpcUSetsInteraction`
  - `TestHandleOpNpcUMissingListenerRejected`
  - `TestHandleOpNpcUInvalidInvSlotRejected`
  - `TestHandleOpNpcUItemMismatchRejected`
  - `TestHandleOpNpcUHappyPathWithOtherPlayerInv`
  - `TestHandleOpNpcUMembersOnFreeWorldRejected`
  - `TestHandleOpNpcUMembersOnMembersWorldAllowed`
  - `TestHandleOpNpcUDelayedNpcRejected`
  - `TestHandleOpNpcUNpcNotVisibleRejected`
- `handler_opobj_test.go`:
  - `TestHandleOpObjUSetsInteraction`
  - `TestHandleOpObjUMissingListenerRejected`
  - `TestHandleOpObjUItemMismatchRejected`
- `handler_oploc_test.go`:
  - `TestHandleOpLocUSetsInteraction`
  - all U-side downstream tests
- `handler_op_player_test.go`:
  - `TestHandleOpPlayerU*`

For tests that target pre-listener gates (e.g., `*MissingListenerRejected`), seeding the component is still required — the listener-missing reject only fires once the gate has passed.

- [ ] **Step 2.10: Retire the stale `(S6m-D2/D3)` narrative at `handler_oploc_test.go:430`.**

Find the comment in `TestHandleOpLocUSetsInteraction` doc:

```go
// useObj and useSlot land on p.lastUseItem/lastUseSlot; useCom is discarded
// (S6m-D2/D3).
```

Replace with text reflecting current state (useCom is now read, gated, and used for listener lookup):

```go
// useObj and useSlot land on p.lastUseItem/lastUseSlot; useCom is gated
// against the component registry and used for listener lookup.
```

- [ ] **Step 2.11: Run all tests — expect GREEN.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

- [ ] **Step 2.12: Verify deviation-tag retirement is complete for U-handlers.**

```bash
grep -n "S6m-D2\|S6o-D2\|NAI-50-D2\|NAI-40-D-COMPONENT-REGISTRY" modules/world/handler_oploc.go modules/world/handler_opnpc.go modules/world/handler_opobj.go modules/world/handler_op_player.go modules/world/handler_oploc_test.go
```

Expected: zero matches.

Also re-grep for stale cluster-mention narrative across handler files:

```bash
grep -n "Same cluster as\|bundle with\|cluster-cleanup sub-spec\|component registry yet" modules/world/handler_*.go
```

Any remaining narrative referring to retired cluster siblings should be deleted (full sentence retirement per memory `retire_deviation_grep_all_comments`). Surviving references should only point to NAI-48-D1 (closes in T3) — no other cluster siblings remain open.

- [ ] **Step 2.13: Run full repo tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

- [ ] **Step 2.14: Commit.**

```bash
git add modules/world/handler_component_gate_test.go \
        modules/world/handler_opnpc.go modules/world/handler_opnpc_test.go \
        modules/world/handler_opobj.go modules/world/handler_opobj_test.go \
        modules/world/handler_oploc.go modules/world/handler_oploc_test.go \
        modules/world/handler_op_player.go modules/world/handler_op_player_test.go

git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-60 T2 — component gate on Op*U handlers

Wires s.lookupComponent + Usable check + IsComponentVisible into
handleOpLocU, handleOpNpcU, handleOpObjU, handleOpPlayerU per TS
*UHandler.ts. Gate position varies per handler:
  - OpNpcU/OpPlayerU: pre-lookup (TS:24-33)
  - OpObjU/OpLocU: post-entity-lookup, pre-listener (TS:39-48)

Adds 4 driver tests via shared compGateCase helper. Updates 16
existing happy-path / downstream tests to seed components. Retires
stale (S6m-D2/D3) narrative in handler_oploc_test.go:430.

Closes NAI-60 T2 — retires S6m-D2, S6o-D2, NAI-50-D2, NAI-40-D-
COMPONENT-REGISTRY-VALIDATION-SKIPPED (U-side).
EOF
)"
```

---

## Task 3: InvButton family (2 sites)

**Goal:** Wire the §2.3 gate + protect computation into `handleInvButton` and `handleInvButtonD`. Lift `runIfButtonProtectScript` into a generalised `runProtectScript` helper. Retire `NAI-48-D1` deviation comments.

**Files:**
- Modify: `modules/world/handler_inv_button.go` (handleInvButton, handleInvButtonD)
- Modify: `modules/world/handler_inv_button_test.go` (add reject tests, add protect tests, seed existing happy-paths)
- Modify: `modules/world/handler_component_gate_test.go` (add `runProtectScript` shared helper)

**Per-handler controller pre-flight:**

```bash
grep -n "DEVIATION NAI-48-D1" modules/world/handler_inv_button.go    # expect ~21 and ~74
grep -n "func runIfButtonProtectScript" modules/world/handler_interface_test.go  # expect ~345
```

### T3 Steps

- [ ] **Step 3.1: Add `runProtectScript` helper to `handler_component_gate_test.go`.**

Append:

```go
// runProtectScript registers a script for (trigger, comId) that runs
// P_DELAY (which requires Protect=true via requireProtectedActivePlayer),
// invokes handlerFn against a Server seeded with the rootLayer fixture,
// and reports whether the script suspended (handler computed protect=true)
// or aborted (handler computed protect=false).
//
// rootOverlay sets Overlay on the rootLayer component (when includeRoot).
// includeRoot=false omits the root component entirely → lookupComponent
// returns nil → protect should default to true.
func runProtectScript(
	t *testing.T,
	trigger script.ServerTriggerType,
	comId int,
	rootLayer int,
	rootOverlay bool,
	includeRoot bool,
	registerExtra func(*Server, *Player), // e.g., listener + inv setup
	invokeHandler func(*Server, *Player) error,
	componentExtras *objtype.ComponentType, // additional fields like Iop, Draggable
) bool {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	sf := &script.ScriptFile{
		Name:             "[trigger,com]",
		LookupKey:        script.LookupKeyForType(trigger, comId),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(sf)

	com := &objtype.ComponentType{RootLayer: rootLayer}
	if componentExtras != nil {
		com.Iop = componentExtras.Iop
		com.Draggable = componentExtras.Draggable
	}
	components := map[int]*objtype.ComponentType{comId: com}
	if includeRoot {
		components[rootLayer] = &objtype.ComponentType{RootLayer: rootLayer, Overlay: rootOverlay}
	}
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, components)
	p.tabs[0] = rootLayer

	if registerExtra != nil {
		registerExtra(s, p)
	}

	if err := invokeHandler(s, p); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return p.activeScript != nil && p.activeScript.Execution == script.Suspended
}
```

Add `"github.com/zsrv/goscape/pkg/script"` to imports if not already present.

- [ ] **Step 3.2: Add reject + protect tests for InvButton in `handler_inv_button_test.go`.**

Append:

```go
// TestHandleInvButton_NilComponentRejects pins that registry-empty for comId
// causes the handler to bail before reading lastItem/lastSlot.
func TestHandleInvButton_NilComponentRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	// no seedComponentTypes call → registry empty for com=149
	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil component should reject)", p.lastItem)
	}
}

// TestHandleInvButton_NoIopAtOpRejects pins that com.Iop[op-1]=="" or out
// of bounds rejects.
func TestHandleInvButton_NoIopAtOpRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Iop: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 149

	// op=2 → Iop[1]="" → reject
	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 2); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (Iop[1]=\"\" should reject)", p.lastItem)
	}
}

// TestHandleInvButton_NotVisibleRejects pins that root-not-in-tabs rejects.
func TestHandleInvButton_NotVisibleRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 999, Iop: []string{"option1", "", "", "", ""}},
	})
	// p.tabs left at default — 999 not visible

	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not visible should reject)", p.lastItem)
	}
}

// runInvButtonProtectScript wraps runProtectScript with InvButton specifics.
func runInvButtonProtectScript(t *testing.T, op int, rootOverlay, includeRoot bool) bool {
	t.Helper()
	const comId = 149
	const rootLayer = 100
	return runProtectScript(t,
		script.TriggerInvButton1+script.ServerTriggerType(op-1), comId,
		rootLayer, rootOverlay, includeRoot,
		func(s *Server, p *Player) {
			if s.invs == nil {
				s.invs = make(map[int]*inventory.Inventory)
			}
			inv := inventory.New(93, 28, inventory.StackNormal)
			inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
			s.invs[93] = inv
			p.invListenOnCom(93, comId, -1)
		},
		func(s *Server, p *Player) error {
			return s.handleInvButton(p, invButtonPayload(555, 3, comId), op)
		},
		&objtype.ComponentType{Iop: []string{"option1", "", "", "", ""}},
	)
}

func TestHandleInvButton_OverlayRootSetsProtectFalse(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, true, true); got {
		t.Errorf("script suspended: got true, want false (Overlay=true → protect=false)")
	}
}

func TestHandleInvButton_NonOverlayRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, false, true); !got {
		t.Errorf("script suspended: got false, want true (Overlay=false → protect=true)")
	}
}

func TestHandleInvButton_NilRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, false, false); !got {
		t.Errorf("script suspended: got false, want true (nil root → protect=true)")
	}
}
```

Add equivalent tests for `InvButtonD`:

```go
// TestHandleInvButtonD_NilComponentRejects, _NotDraggableRejects, _NotVisibleRejects
// — mirror InvButton variants with Draggable instead of Iop.

func TestHandleInvButtonD_NilComponentRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (nil component should reject)", p.lastSlot)
	}
}

func TestHandleInvButtonD_NotDraggableRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: false},
	})
	p.tabs[0] = 149
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (Draggable=false should reject)", p.lastSlot)
	}
}

func TestHandleInvButtonD_NotVisibleRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 999, Draggable: true},
	})
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not visible should reject)", p.lastSlot)
	}
}

func runInvButtonDProtectScript(t *testing.T, rootOverlay, includeRoot bool) bool {
	t.Helper()
	const comId = 149
	const rootLayer = 100
	return runProtectScript(t,
		script.TriggerInvButtonD, comId,
		rootLayer, rootOverlay, includeRoot,
		func(s *Server, p *Player) {
			if s.invs == nil {
				s.invs = make(map[int]*inventory.Inventory)
			}
			inv := inventory.New(93, 28, inventory.StackNormal)
			inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
			s.invs[93] = inv
			p.invListenOnCom(93, comId, -1)
		},
		func(s *Server, p *Player) error {
			return s.handleInvButtonD(p, invButtonDPayload(comId, 3, 5))
		},
		&objtype.ComponentType{Draggable: true},
	)
}

func TestHandleInvButtonD_OverlayRootSetsProtectFalse(t *testing.T) {
	if got := runInvButtonDProtectScript(t, true, true); got {
		t.Errorf("script suspended: got true, want false (Overlay=true → protect=false)")
	}
}

func TestHandleInvButtonD_NonOverlayRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonDProtectScript(t, false, true); !got {
		t.Errorf("script suspended: got false, want true (Overlay=false → protect=true)")
	}
}

func TestHandleInvButtonD_NilRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonDProtectScript(t, false, false); !got {
		t.Errorf("script suspended: got false, want true (nil root → protect=true)")
	}
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to `handler_inv_button_test.go` imports if not present.

- [ ] **Step 3.3: Run new tests — expect FAIL.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleInvButton_ -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleInvButtonD_ -v
```

Expected: rejects + protect tests FAIL (gate not yet wired).

- [ ] **Step 3.4: Wire the gate + protect into `handleInvButton`.**

Replace handler body in `modules/world/handler_inv_button.go:25-56`:

```go
// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS InvButtonHandler.ts:
//  1. delayed player → drop
//  2. payload < 6 bytes → drop
//  3. nil component or !IsComponentVisible → drop
//  4. com.Iop nil or Iop[op-1]=="" → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//
// On pass: set p.lastItem=obj, p.lastSlot=slot, look up
// [inv_button<op>,<comId>] via GetByTrigger and run with
// protect = !rootLayer.Overlay (rootLayer nil → protect=true).
func (s *Server) handleInvButton(p *Player, payload []byte, op int) error {
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
	if com == nil {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}
	if com.Iop == nil || op-1 < 0 || op-1 >= len(com.Iop) || com.Iop[op-1] == "" {
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

	trigger := script.TriggerInvButton1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, protect, nil, nil)
	return nil
}
```

- [ ] **Step 3.5: Wire the gate + protect into `handleInvButtonD`.**

Replace handler body in `modules/world/handler_inv_button.go:77-112`:

```go
// handleInvButtonD is the handler for INV_BUTTOND (opcode 159, 6-byte payload).
// Inventory drag-and-drop. Wire format: com:G2 | slot:G2 | targetSlot:G2.
//
// Gates per TS InvButtonDHandler.ts (note: visual-revert delayed-gate is
// AFTER inv-listener gates, matching TS):
//  1. payload < 6 bytes → drop
//  2. nil component or !Draggable → drop
//  3. !IsComponentVisible → drop
//  4. comId not in invListeners → drop
//  5. listener's inventory unresolved → drop
//  6. slot or targetSlot out of inv.Capacity bounds → drop
//  7. source slot empty (inv.Get(slot)==nil) → drop
//  8. player delayed → sendUpdateInvPartial to revert visual, then drop
//
// On pass: set p.lastSlot, p.lastTargetSlot, look up [inv_buttond,<comId>]
// and run with protect = !rootLayer.Overlay.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())

	com := s.lookupComponent(comId)
	if com == nil || !com.Draggable {
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
	if slot < 0 || slot >= inv.Capacity || targetSlot < 0 || targetSlot >= inv.Capacity {
		return nil
	}
	if inv.Get(slot) == nil {
		return nil
	}

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUpdateInvPartial(p, comId, inv, slot, targetSlot)
		return nil
	}

	p.lastSlot = slot
	p.lastTargetSlot = targetSlot

	sf := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, protect, nil, nil)
	return nil
}
```

- [ ] **Step 3.6: Run new InvButton tests — expect PASS.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleInvButton_ -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleInvButtonD_ -v
```

Expected: 12 new tests PASS (3 reject + 3 protect for each of InvButton/InvButtonD).

- [ ] **Step 3.7: Run existing InvButton tests — happy-paths will FAIL until seeded.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleInvButton -v
```

Failing tests (require seeding):
- `TestHandleInvButtonSetsStateAndRunsScript`
- `TestHandleInvButtonOpVariant`
- `TestHandleInvButtonDSetsStateAndRunsScript`
- `TestHandleInvButtonDDelayedRevert`

Tests that exercise pre-gate rejects (delayed, short payload, no-listener) may or may not require seeding — re-read each and add seeding only where the test expects the handler to reach beyond the new gate. For tests that currently expect a reject from a downstream gate (e.g., `TestHandleInvButtonNoListener`), seeding the component is required so the handler reaches the listener check.

- [ ] **Step 3.8: Update existing InvButton/InvButtonD tests to seed components.**

Per-test seeding additions (all in `modules/world/handler_inv_button_test.go`):

For tests that pass `comId=149` and expect to reach the gate or beyond:

```go
// Add immediately after the setupInvButtonServer call.
seedComponentTypes(t, s, map[int]*objtype.ComponentType{
    149: {
        RootLayer: 149,
        Iop:       []string{"option1", "", "", "", ""}, // for InvButton; satisfies any op=1
        Draggable: true, // for InvButtonD
    },
})
p.tabs[0] = 149
```

For `TestHandleInvButtonOpVariant` (op=2), set Iop[1] non-empty:

```go
seedComponentTypes(t, s, map[int]*objtype.ComponentType{
    149: {RootLayer: 149, Iop: []string{"o1", "o2", "", "", ""}},
})
p.tabs[0] = 149
```

For `TestHandleInvButtonDelayed` (test expects pre-gate reject from delayed): no seeding needed — delayed gate fires before component gate.

For `TestHandleInvButtonShortPayload`: no seeding needed.

For `TestHandleInvButtonNoListener` / `TestHandleInvButtonNilInv` / `TestHandleInvButtonItemMismatch`: seeding needed (handler must reach listener check, which is post-gate).

For `TestHandleInvButtonDNoListener` / `TestHandleInvButtonDNilInv` / `TestHandleInvButtonDSlotOOB` / `TestHandleInvButtonDSourceEmpty` / `TestHandleInvButtonDDelayedRevert`: seeding needed with `Draggable: true`.

Re-grep to enumerate every site:

```bash
grep -n "func TestHandleInvButton" modules/world/handler_inv_button_test.go
```

Walk each test top-to-bottom and decide: pre-gate reject (no seeding) or post-gate behavior (seeding required).

- [ ] **Step 3.9: Replace the deviation comment block in `handler_inv_button.go`.**

The handler bodies above already include the new gate-explainer doc comments. Confirm the original `DEVIATION NAI-48-D1` blocks (originally at lines 21-24 and 74-76) are fully removed.

- [ ] **Step 3.10: Run all tests — expect GREEN.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

- [ ] **Step 3.11: Verify NAI-48-D1 retirement.**

```bash
grep -rn "NAI-48-D1" modules/ pkg/ cmd/
```

Expected: zero matches in production / test code (historical specs in `docs/superpowers/specs/` are fine — those are not retired).

Also re-grep cluster narrative:

```bash
grep -n "Same cluster as\|component-registry sub-spec\|cluster-cleanup sub-spec\|component registry yet" modules/world/handler_*.go
```

Expected: zero matches across all handler files. All cluster narratives are stale post-T3 since every member of the cluster (S6m-D1/D2, S6o-D1/D2, NAI-50-D1/D2, NAI-40-D × 2, NAI-48-D1 × 2) has now closed.

- [ ] **Step 3.12: Run full repo tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

- [ ] **Step 3.13: Commit.**

```bash
git add modules/world/handler_inv_button.go \
        modules/world/handler_inv_button_test.go \
        modules/world/handler_component_gate_test.go

git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-60 T3 — component gate on InvButton family

Wires s.lookupComponent + IsComponentVisible + Iop/Draggable check
into handleInvButton and handleInvButtonD per TS InvButton(D)Handler.ts.
Replaces protect=true-always with TS root.overlay computation
(protect = !root.Overlay; nil root → protect=true).

Adds 6 reject tests + 6 protect tests via lifted runProtectScript
helper (handler_component_gate_test.go). Updates existing happy-path
and downstream-state tests in handler_inv_button_test.go to seed
components.

Closes NAI-60 T3 — retires NAI-48-D1 ×2.
EOF
)"
```

---

## Task 4: Close commit (narrative cleanup + sub-spec close)

**Goal:** Final pass to retire any surviving stale cluster narratives missed in T1-T3, then emit the NAI-60 close commit with `Closes:` and `Closes memory:` trailers.

**Files:**
- Modify (if surviving narrative found): any file under `modules/world/` flagged by the grep below.

### T4 Steps

- [ ] **Step 4.1: Final retirement grep.**

Run:

```bash
# All cluster tag references — should be zero in production/test code
grep -rn "S6m-D1\|S6m-D2\|S6o-D1\|S6o-D2\|NAI-50-D1\|NAI-50-D2\|NAI-48-D1\|NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED" modules/ pkg/ cmd/

# Stale cluster narrative — should be zero across handler files
grep -n "Same cluster as\|bundle with S6\|bundle with NAI-4\|bundle with NAI-5\|component-registry sub-spec\|cluster-cleanup sub-spec\|no component registry\|component registry yet" modules/world/handler_*.go
```

Expected: both empty. Any remaining hits are stale narrative — delete the full sentence (per memory `retire_deviation_grep_all_comments`).

- [ ] **Step 4.2: If narrative survived, fix and re-grep.**

For each surviving site, Read the surrounding 5-10 lines and delete the stale sentence. Do NOT mass-replace — narratives may share words with non-stale text.

- [ ] **Step 4.3: Run all tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 4.4: Verify deviation list under NAI-59 follow-up tracker is current.**

The NAI-59 spec doc at `docs/superpowers/specs/2026-05-01-nai-59-componenttype-config-port-design.md` lists deviations queued for NAI-60+ at lines 23-28. After NAI-60, those entries are historical; do NOT modify the historical spec. Tracker freshness for NAI-61+ is the next spec author's concern.

- [ ] **Step 4.5: Emit close commit.**

If T4 made any narrative-cleanup edits:

```bash
git add modules/world/  # only files actually touched
```

Compose close commit:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-60 — component-registry cluster cleanup

Closes 10 deviation sites carrying 8 distinct tags by wiring
NAI-59's lookupComponent + IsComponentVisible into 5 client-message
handler families. NAI-53-D-CLEARCOMLISTENERS-PER-SLOT explicitly
deferred — different mechanism (per-listener filter inside encodeOut).

Closes:
  S6m-D1 (handler_oploc.go OPLOCT) — T1
  S6m-D2 (handler_oploc.go OPLOCU) — T2
  S6o-D1 (handler_opnpc.go OPNPCT) — T1
  S6o-D2 (handler_opnpc.go OPNPCU) — T2
  NAI-50-D1 (handler_opobj.go OPOBJT) — T1
  NAI-50-D2 (handler_opobj.go OPOBJU) — T2
  NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED ×2 (handler_op_player.go OPPLAYERT/U) — T1+T2
  NAI-48-D1 ×2 (handler_inv_button.go INV_BUTTON/INV_BUTTOND) — T3

Closes memory: consume_reserved_constant
EOF
)"
```

If T4 made no edits, drop `--allow-empty` and use a final grep-verify-only commit (or skip entirely if T3 commit is the last with content; the close-commit is always emitted as the canonical NAI-60 closure record).

- [ ] **Step 4.6: Final repo state verification.**

```bash
git log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
git status
```

Expected: 4 NAI-60 commits at top of log, all tests green, working tree clean.

---

## Self-review notes

- **Spec coverage:** Every §1 retirement entry maps to a task (T1: 4 T-side; T2: 4 U-side; T3: 2 InvButton; T4: narrative cleanup). NAI-53-D explicitly deferred per spec §1.
- **Type consistency:** `compGateCase`, `runCompGate`, `runProtectScript` signatures kept consistent across T1, T2, T3 step definitions. `seedComponentTypes` used identically (existing helper, no signature change).
- **Placeholder scan:** Each step contains exact code. Where a step says "the rest of the body unchanged" or "see makeOpNpcFixture", surrounding context shows exactly what stays — implementer reads HEAD for those. Test fixture seed-helpers (`seedNpcAtSlot`, `seedObjAt`, `seedLocAt`, `seedTargetPlayerAtSlot`, `seedListenerWithItem`) are left as implementer-extracts-from-existing-make-fixture; spec is explicit about WHERE to extract from.
- **Per-handler positions:** Spec §2 enumerates per-handler gate position (T-variants pre-lookup, U-Npc/Player pre-lookup, U-Obj/Loc post-lookup-pre-listener, InvButtonD pre-delayed-revert). Plan §T1.3-1.6 and §T2.3-2.6 each codify the position with surrounding-line context.
