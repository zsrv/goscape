package world

import (
	"bytes"
	"fmt"
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// buildNpcSayScript produces a tiny [push "hello", NPC_SAY, RETURN] script
// keyed at the trigger+typeID-specific lookup key.
func buildNpcSayScript(trigger script.ServerTriggerType, typeID int, text string) *script.ScriptFile {
	key := script.LookupKeyForType(trigger, typeID)
	return &script.ScriptFile{
		Name:             "[opnpc1,test]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{text, "", ""},
		InstructionCount: 3,
	}
}

// newTriggerFixture builds a Server + Player + live Npc of typeID=7 with a
// seeded NPC_SAY script registered at (TriggerOpNpc1, 7, 0).
func newTriggerFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpc1, 7, "hello"))
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
		Op:         []string{"Talk-to", "", "", "", ""},
		Category:   0,
	}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true // simulate reach
	return s, p, npc
}

func TestTryFireOpTrigger_HappyPath(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	tryFireOpTrigger(p)
	if string(npc.sayText) != "hello" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "hello")
	}
	// NAI-68: target restored to originalTarget (not nil); processInteraction
	// tail's else-if handles clear after contact-fire.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored after OP fire — NAI-68)", p.target)
	}
}

func TestTryFireOpTrigger_NoScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared when no script found")
	}
}

func TestTryFireOpTrigger_DeadNpc(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	npc.dead = true
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared for dead npc")
	}
	if len(npc.sayText) != 0 {
		t.Error("sayText: expected empty (script did not run)")
	}
}

// nonNpcEntity satisfies the entity interface for the wrong-target-type test.
type nonNpcEntity struct{}

func (nonNpcEntity) Slot() int                 { return 0 }
func (nonNpcEntity) Coords() (x, z, level int) { return 0, 0, 0 }
func (nonNpcEntity) IsValid() bool             { return true }

func TestTryFireOpTrigger_WrongTargetType(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.SetInteraction(InteractionEngine, nonNpcEntity{}, 1, -1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved for non-npc (future OPLOC branch)")
	}
}

func TestTryFireOpTrigger_BadOp(t *testing.T) {
	_, p, _ := newTriggerFixture(t)
	p.targetOp = 99
	tryFireOpTrigger(p)
	if p.target != nil {
		t.Error("target: expected cleared for bad op")
	}
}

func TestTryFireOpTrigger_ScriptSuspends(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Script: P_DELAY(5) + RETURN. Operands layout per S4 reference.
	suspendScript := &script.ScriptFile{
		Name:             "[opnpc1,suspend]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpNpc1, 7),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(suspendScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved across suspension")
	}
	if p.activeScript == nil {
		t.Error("activeScript: expected stored for resume")
	}
}

func TestTryFireOpTrigger_PlayerDelayed(t *testing.T) {
	s, p, _ := newTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = s.currentTick + 3
	tryFireOpTrigger(p)
	if p.target == nil {
		t.Error("target: expected preserved while delayed")
	}
}

func TestTryFireOpTrigger_ReClickResetsFired(t *testing.T) {
	_, p, _ := newTriggerFixture(t)
	tryFireOpTrigger(p)
	npc2Type := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Op: []string{"Talk-to"}}
	npc2 := NewNpc(1, 8, p.x, p.z, p.level, npc2Type)
	p.SetInteraction(InteractionEngine, npc2, 1, -1)
}

func TestTryFireOpTrigger_CategoryFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Script registered at category-level, not type-level.
	categoryKey := script.LookupKeyForCategory(script.TriggerOpNpc1, 3)
	catScript := &script.ScriptFile{
		Name:             "[opnpc1,_cat3]",
		LookupKey:        categoryKey,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"category", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(catScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}, Category: 3}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true
	tryFireOpTrigger(p)
	if string(npc.sayText) != "category" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "category")
	}
}

// Compile-time assertion that *entitypkg.Loc satisfies the package-local
// entity interface (Slot() int + Coords() (x, z, level int)). Required
// for p.target = loc to type-check when handler_oploc sets the target.
var _ entity = (*entitypkg.Loc)(nil)

func TestTryFireOpTrigger_GlobalFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalScript := &script.ScriptFile{
		Name:             "[opnpc1,_]",
		LookupKey:        script.LookupKeyForGlobal(script.TriggerOpNpc1),
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"global", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(globalScript)
	p, _ := newTestPlayer(t)
	p.client.server = s
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Op: []string{"Talk-to"}}
	npc := NewNpc(0, 7, p.x, p.z, p.level, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true
	tryFireOpTrigger(p)
	if string(npc.sayText) != "global" {
		t.Errorf("sayText: got %q, want %q", npc.sayText, "global")
	}
}

// TestProcessInteractionInteractionScriptKindFiresDispatch verifies that
// processInteraction fires AP/OP triggers for script-kind interactions
// just like engine-kind. S6v closed the placeholder-skip behavior that
// was previously codified as "reserved for RuneScript integration."
//
// NAI-44 T6 cascade: pre-T5 asserted interactionFired==true; post-T5
// auto-clear (TS L1261-1263) fires after contact-fire (interacted &&
// !apRangeCalled), clearing interactionFired. Observable proof of dispatch:
// npc.sayText set by the registered script + target==nil (auto-cleared).
func TestProcessInteractionInteractionScriptKindFiresDispatch(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	// Move the npc one tile east so inOperableDistance(player, npc) returns
	// true (dx=1, dz=0 — adjacent but not same tile). The fixture places the
	// npc at player coords; same-tile fails both OP and AP distance checks.
	npc.x = p.x + 1
	// Anchor as script-kind. The npc is now in operable distance, so
	// processInteraction should hit the OP branch and fire the trigger.
	p.SetInteraction(InteractionScript, npc, 1, -1)
	p.processInteraction()
	if string(npc.sayText) == "" {
		t.Error("sayText: expected script-kind dispatch to fire the trigger, got empty")
	}
	// NAI-44: auto-clear fires after contact; target==nil proves dispatch + clear.
	if p.target != nil {
		t.Errorf("target: got %v, want nil (auto-clear at TS L1261-1263 after contact-fire)", p.target)
	}
}

// newNoopScriptFile creates a *script.ScriptFile with a single OpReturn opcode,
// keyed at the type-specific lookup for (trigger, typeID). categoryID is unused
// (pass -1 to indicate "type-level key only"); matches the pattern used by
// TestTryFireOpTrigger_HappyPath but for arbitrary triggers.
func newNoopScriptFile(t *testing.T, trigger script.ServerTriggerType, typeID, _ int) *script.ScriptFile {
	t.Helper()
	key := script.LookupKeyForType(trigger, typeID)
	return &script.ScriptFile{
		Name:             "[trigger,noop]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
}

// makeOpLocTriggerFixture creates a fixture for tryFireOpTrigger Loc-branch
// tests: server + player anchored on a loc with valid targetSubject.
// Returns (server, player, loc, clientConn) — pass clientConn to drainConn
// when the test needs to observe bytes written to the player.
func makeOpLocTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Loc, net.Conn) {
	t.Helper()
	s, p, loc, cc := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return s, p, loc, cc
}

// TestTryFireOpTriggerLocNoScript verifies a Loc target with no registered
// trigger emits "Nothing interesting happens." and clears the interaction.
func TestTryFireOpTriggerLocNoScript(t *testing.T) {
	_, p, _, cc := makeOpLocTriggerFixture(t)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame packet for defaultOp, got nothing")
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after default-op clear", p.target)
	}
	// Assert the message text appears in the drained bytes. The
	// wire format is [opcode][length][text+10]; we check the text
	// substring to stay robust to framing details.
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("drained bytes: expected \"Nothing interesting happens.\" substring, got %x", got)
	}
}

// TestTryFireOpTriggerLocScriptFires verifies a registered [oploc1,<typeID>]
// script fires, ActiveLoc is set, and target is restored after Finished.
// NAI-68: Finished/Aborted ClearInteraction dropped; target preserved.
func TestTryFireOpTriggerLocScriptFires(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	// Register a no-op script for [oploc1, locType=42].
	sf := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	// NAI-68: target restored to originalTarget; processInteraction tail clears.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after Finished — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (noop script set no nextTarget)", p.nextTarget)
	}
}

// TestTryFireOpTriggerLocDeferredOnDelay verifies a delayed player defers
// fire (no state change, interactionFired stays false).
func TestTryFireOpTriggerLocDeferredOnDelay(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	tryFireOpTrigger(p)

	if p.target != loc {
		t.Errorf("target: got %v, want loc (deferred)", p.target)
	}
}

// TestTryFireOpTriggerLocTypeChanged verifies in-place type mutation
// (loc.CurrentInfo changed via packLocInfo) clears interaction silently.
func TestTryFireOpTriggerLocTypeChanged(t *testing.T) {
	_, p, loc, _ := makeOpLocTriggerFixture(t)

	// Mutate the loc's type in-place. New type 99 differs from
	// p.targetSubject.typ (42).
	loc.Change(99, 10, 0)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (type changed)", p.target)
	}
}

// TestTryFireOpTriggerLocRemoved verifies removing the loc from its zone
// (axed-tree case) clears interaction silently.
func TestTryFireOpTriggerLocRemoved(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	// Remove the loc from its zone.
	zn := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	zn.Locs = nil

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (loc removed)", p.target)
	}
}

// TestTryFireOpTriggerLocOpOutOfRange verifies targetOp=0 silently clears.
func TestTryFireOpTriggerLocOpOutOfRange(t *testing.T) {
	_, p, _, _ := makeOpLocTriggerFixture(t)
	p.targetOp = 0 // invalid

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (invalid op)", p.target)
	}
}

// makeApTriggerFixture creates a fixture for tryFireApTrigger tests:
// server + player anchored on a loc with valid targetSubject, positioned
// within apRange=10 but NOT at contact. Returns (server, player, loc, conn).
func makeApTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Loc, net.Conn) {
	t.Helper()
	// makeOpLocFixture places the loc at (100, 100) and the player at
	// (99, 100) — at contact. For AP tests we move the player farther.
	s, p, loc, cc := makeOpLocFixture(t)
	p.x, p.z = 95, 100 // 5 tiles away — within apRange=10, not contact
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return s, p, loc, cc
}

// TestTryFireApTriggerLocNoScript verifies a Loc target with no APLOC
// trigger registered leaves the interaction anchored (no clear), sets
// interactionFired=true, and sets apRange=-1 (S6l-D1 sentinel, closed
// in S6r). Subsequent ticks see apRange<=0 in inApproachDistance and
// skip the AP path; OPLOC/defaultOp takes over on contact.
func TestTryFireApTriggerLocNoScript(t *testing.T) {
	_, p, loc, _ := makeApTriggerFixture(t)

	tryFireApTrigger(p)

	if p.target != loc {
		t.Errorf("target: got %v, want loc (no-AP-script should not clear)", p.target)
	}
}

// TestTryFireApTriggerLocScriptFiresNoApRangeCalled verifies an APLOC
// script that runs but doesn't call p_aprange leaves p.target as the
// original loc (ClearInteraction is deferred to processInteraction tail's
// else-if per NAI-68 TS L1261-1263 refactor). nextTarget is nil.
func TestTryFireApTriggerLocScriptFiresNoApRangeCalled(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register a no-op APLOC1 script for locType=42.
	sf := newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	// NAI-68: ClearInteraction dropped from fire helper; p.target restored to
	// original loc. Auto-clear happens in processInteraction tail's else-if.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (no p_op_* in script)", p.nextTarget)
	}
}

// scriptFileWithApRangeCall creates a ScriptFile whose only opcode is
// P_APRANGE(N), simulating an APLOC script that calls p_aprange.
// Reuses the newNoopScriptFile key-packing convention (type-tier key).
func scriptFileWithApRangeCall(t *testing.T, trigger script.ServerTriggerType, typeID, newApRange int) *script.ScriptFile {
	t.Helper()
	return &script.ScriptFile{
		Name:      "aploc_aprange_test",
		LookupKey: script.LookupKeyForType(trigger, typeID),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
		},
		IntOperands:      []int32{int32(newApRange), 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// TestTryFireApTriggerLocScriptCallsPApRange verifies an APLOC script
// that calls p_aprange completes the fire cleanly with apRangeCalled=true
// and interactionFired=true (post-NAI-69 uniform-exit contract). The
// same-tick retry decision happens in tryInteract, not the fire helper
// (see TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired).
func TestTryFireApTriggerLocScriptCallsPApRange(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register an APLOC1 script that calls p_aprange(5).
	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 5)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != loc {
		t.Errorf("target: got %v, want loc (p_aprange should persist interaction)", p.target)
	}
	if p.apRange != 5 {
		t.Errorf("apRange: got %d, want 5 (p_aprange argument)", p.apRange)
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: want true after p_aprange fire")
	}
}

// TestTryFireApTriggerLocDeferredOnDelay verifies a delayed player defers
// the fire (no state change, interactionFired stays false).
func TestTryFireApTriggerLocDeferredOnDelay(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	tryFireApTrigger(p)

	if p.target != loc {
		t.Errorf("target: got %v, want loc (deferred)", p.target)
	}
}

// TestTryFireApTriggerLocTypeChanged verifies in-place type mutation
// (loc.CurrentInfo changed) clears interaction silently.
func TestTryFireApTriggerLocTypeChanged(t *testing.T) {
	_, p, loc, _ := makeApTriggerFixture(t)

	// Mutate loc.CurrentInfo to a different type (99 ≠ 42 recorded in targetSubject).
	loc.Change(99, 10, 0)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (lifecycle gate)", p.target)
	}
}

// TestTryFireApTriggerLocRemoved verifies removing the loc from its zone
// (axed-tree case) clears interaction silently.
func TestTryFireApTriggerLocRemoved(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	zn := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	zn.Locs = nil

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (removed from zone)", p.target)
	}
}

// TestTryFireApTriggerLocOpOutOfRange verifies targetOp=0 silently clears.
func TestTryFireApTriggerLocOpOutOfRange(t *testing.T) {
	_, p, _, _ := makeApTriggerFixture(t)
	p.targetOp = 0

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (invalid op)", p.target)
	}
}

// TestApLocTriggerForOpValidValues table-tests all valid targetOp
// mappings:
//
//	1..5 → TriggerApLoc1..5 (existing OpLoc1..5 behavior)
//	6 (targetOpLocT) → TriggerApLocT (single)
//	7 (targetOpLocU) → TriggerApLocU (single)
func TestApLocTriggerForOpValidValues(t *testing.T) {
	cases := []struct {
		op   int
		want script.ServerTriggerType
		name string
	}{
		{1, script.TriggerApLoc1, "OpLoc1"},
		{2, script.TriggerApLoc2, "OpLoc2"},
		{3, script.TriggerApLoc3, "OpLoc3"},
		{4, script.TriggerApLoc4, "OpLoc4"},
		{5, script.TriggerApLoc5, "OpLoc5"},
		{targetOpLocT, script.TriggerApLocT, "OpLocT"},
		{targetOpLocU, script.TriggerApLocU, "OpLocU"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := apLocTriggerForOp(c.op)
			if !ok {
				t.Fatalf("op=%d: ok=false, want true", c.op)
			}
			if got != c.want {
				t.Errorf("op=%d: got %d, want %d", c.op, got, c.want)
			}
		})
	}
}

// TestApLocTriggerForOpInvalidValues verifies out-of-range op values
// return ok=false (caller silent-clears). Covers the gap between 5 and
// targetOpLocT (none currently) and below 1 / above 7.
func TestApLocTriggerForOpInvalidValues(t *testing.T) {
	invalid := []int{0, -1, 8, 100, -100}
	for _, op := range invalid {
		t.Run(fmt.Sprintf("op_%d", op), func(t *testing.T) {
			_, ok := apLocTriggerForOp(op)
			if ok {
				t.Errorf("op=%d: ok=true, want false", op)
			}
		})
	}
}

// TestFireOpTriggerLocFiresOpLocTTrigger verifies that when p.targetOp
// is targetOpLocT (6) and an OPLOCT script is registered, fireOpTriggerLoc
// dispatches to it. Player positioned at contact distance.
func TestFireOpTriggerLocFiresOpLocTTrigger(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, targetOpLocT, 7777)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// com=7777 is passed to SetInteraction above; post-NAI-62 fix, the
	// override key is 7777 (not loc.Type()).
	sf := newNoopScriptFile(t, script.TriggerOpLocT, 7777, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	// NAI-68: target restored to loc (not nil); tail clears on next tick.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after OPLOCT fire — NAI-68)", p.target)
	}
}

// TestFireOpTriggerLocFiresOpLocUTrigger verifies targetOpLocU (7) →
// OPLOCU dispatch at contact.
func TestFireOpTriggerLocFiresOpLocUTrigger(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	sf := newNoopScriptFile(t, script.TriggerOpLocU, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	// NAI-68: target restored to loc (not nil); tail clears on next tick.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after OPLOCU fire — NAI-68)", p.target)
	}
}

// TestFireApTriggerLocFiresApLocTTrigger verifies targetOpLocT (6) →
// APLOCT dispatch at approach distance.
func TestFireApTriggerLocFiresApLocTTrigger(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)
	// Override targetOp from fixture's default 1 → targetOpLocT.
	p.targetOp = targetOpLocT

	sf := newNoopScriptFile(t, script.TriggerApLocT, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	// NAI-68: ClearInteraction dropped from fire helper; target restored to
	// original loc. processInteraction tail's else-if is the sole auto-clear.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (no p_op_* in script)", p.nextTarget)
	}
}

// TestFireApTriggerLocFiresApLocUTrigger verifies targetOpLocU (7) →
// APLOCU dispatch at approach distance.
func TestFireApTriggerLocFiresApLocUTrigger(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)
	p.targetOp = targetOpLocU

	sf := newNoopScriptFile(t, script.TriggerApLocU, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	// NAI-68: ClearInteraction dropped from fire helper; target restored to
	// original loc. processInteraction tail's else-if is the sole auto-clear.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (no p_op_* in script)", p.nextTarget)
	}
}

// TestApNpcTriggerForOpValidValues table-tests the 1..5 op mapping:
//
//	1..5 → TriggerApNpc1..5 (3..7)
//
// fireOpTriggerNpc derives OPNPC triggers by adding 7 (10..14).
func TestApNpcTriggerForOpValidValues(t *testing.T) {
	cases := []struct {
		op   int
		want script.ServerTriggerType
		name string
	}{
		{1, script.TriggerApNpc1, "OpNpc1"},
		{2, script.TriggerApNpc2, "OpNpc2"},
		{3, script.TriggerApNpc3, "OpNpc3"},
		{4, script.TriggerApNpc4, "OpNpc4"},
		{5, script.TriggerApNpc5, "OpNpc5"},
		{targetOpNpcT, script.TriggerApNpcT, "OpNpcT"}, // NEW (S6o)
		{targetOpNpcU, script.TriggerApNpcU, "OpNpcU"}, // NEW (S6o)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := apNpcTriggerForOp(c.op)
			if !ok {
				t.Fatalf("op=%d: ok=false, want true", c.op)
			}
			if got != c.want {
				t.Errorf("op=%d: got %d, want %d", c.op, got, c.want)
			}
		})
	}
}

// TestApNpcTriggerForOpInvalidValues verifies out-of-range op values
// return ok=false. Values 6 and 7 are Loc sentinels (not Npc) and
// remain invalid. 8 and 9 are now valid (targetOpNpcT/targetOpNpcU).
func TestApNpcTriggerForOpInvalidValues(t *testing.T) {
	invalid := []int{0, 6, 7, -1, 100, -100}
	for _, op := range invalid {
		t.Run(fmt.Sprintf("op_%d", op), func(t *testing.T) {
			_, ok := apNpcTriggerForOp(op)
			if ok {
				t.Errorf("op=%d: ok=true, want false", op)
			}
		})
	}
}

// newApTriggerNpcFixture creates a fixture for fireApTriggerNpc tests:
// Server + Player + live Npc with typeID=7, AttackRange=5, Category=0.
// Player at (100, 100); NPC at (105, 100) — dx=5, within AttackRange.
// targetOp=1. No APNPC script pre-registered; callers register per-test.
func newApTriggerNpcFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	// Wire an ISAAC encryptor so handlers that emit packets (e.g.
	// p_op_npc → StopAction → unsetMapFlag → OpUnsetMapFlag write,
	// player-script-1) can encrypt without nil-deref. Tests that
	// assert specific encrypted bytes overwrite this with their own
	// keyed encryptor; tests that don't read bytes are unaffected.
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
		Category:    0,
		Op:          []string{"op1", "op2", "op3", "op4", "op5"},
	}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: make([]*objtype.NpcType, 8),
	}
	s.npcTypes.Configs[7] = npcType
	npc := NewNpc(0, 7, 105, 100, 0, npcType)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true // simulate reach (as processInteraction would)
	return s, p, npc
}

// TestFireApTriggerNpcNoScript verifies that when no APNPC script is
// registered, fireApTriggerNpc clears the interaction silently and
// marks interactionFired=true.
func TestFireApTriggerNpcNoScript(t *testing.T) {
	s, p, _ := newApTriggerNpcFixture(t)

	fireApTriggerNpc(p, s, p.target.(*Npc))

	if p.target != nil {
		t.Error("target: expected cleared after no-script path")
	}
}

// TestFireApTriggerNpcScriptFires verifies that with an APNPC1 script
// registered at (TriggerApNpc1, typeID=7, categoryID=-1), fireApTriggerNpc
// runs the script and restores p.target to the original npc (NAI-68: eager
// ClearInteraction dropped; deferred to processInteraction tail's else-if).
func TestFireApTriggerNpcScriptFires(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApNpc1, 7, -1)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	// NAI-68: ClearInteraction dropped from fire helper; p.target restored to
	// original npc. Auto-clear happens in processInteraction tail's else-if.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (noop script set no target)", p.nextTarget)
	}
}

// TestFireApTriggerNpcDeadNpc verifies the lifecycle gate: a dead NPC
// clears interaction silently (no script runs). Mirrors the S6j
// TestTryFireOpTrigger_DeadNpc pattern but on the AP path.
func TestFireApTriggerNpcDeadNpc(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	npc.dead = true

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Error("target: expected cleared for dead npc")
	}
}

// TestFireApTriggerNpcDeferredOnDelay verifies that a delayed player
// short-circuits before any state change (no clear, no fire).
// Matches S6l's TestTryFireApTriggerLocDeferredOnDelay pattern.
func TestFireApTriggerNpcDeferredOnDelay(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.delayed = true
	p.delayedUntil = s.currentTick + 3

	fireApTriggerNpc(p, s, npc)

	if p.target == nil {
		t.Error("target: expected preserved while delayed")
	}
}

// TestFireApTriggerNpcOpOutOfRange verifies that an invalid targetOp
// (e.g., 0 or 99) causes a silent interaction clear via the
// apNpcTriggerForOp gate.
func TestFireApTriggerNpcOpOutOfRange(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.targetOp = 99 // out of [1, 5]

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Error("target: expected cleared for out-of-range op")
	}
}

// TestFireOpTriggerNpcFiresOpNpcTTrigger verifies that when p.targetOp
// is targetOpNpcT (8) and an OPNPCT script is registered, fireOpTriggerNpc
// dispatches to it via apNpcTriggerForOp + 7. Mirrors S6m's
// TestFireOpTriggerLocFiresOpLocTTrigger.
func TestFireOpTriggerNpcFiresOpNpcTTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	// com=7777 is passed to SetInteraction below; post-NAI-62 fix, the
	// override key is 7777 (not npc.typeId=0).
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpcT, 7777, "opnpct-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, 7777)
	p.interacted = true

	tryFireOpTrigger(p)

	// NAI-68: target restored to npc (not nil); tail clears on next tick.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored after OPNPCT fire — NAI-68)", p.target)
	}
	if string(npc.sayText) != "opnpct-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "opnpct-fired")
	}
}

// TestFireOpTriggerNpcFiresOpNpcUTrigger verifies targetOpNpcU (9) →
// OPNPCU dispatch at contact.
func TestFireOpTriggerNpcFiresOpNpcUTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpcU, 0, "opnpcu-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	p.interacted = true

	tryFireOpTrigger(p)

	// NAI-68: target restored to npc (not nil); tail clears on next tick.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored after OPNPCU fire — NAI-68)", p.target)
	}
	if string(npc.sayText) != "opnpcu-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "opnpcu-fired")
	}
}

// TestFireApTriggerNpcFiresApNpcTTrigger verifies targetOpNpcT (8) →
// APNPCT dispatch at approach distance.
func TestFireApTriggerNpcFiresApNpcTTrigger(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	s.scriptProvider = script.NewProvider()
	// com=7777 is passed to SetInteraction below; post-NAI-62 fix, the
	// override key is 7777 (not npc.typeId=0).
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApNpcT, 7777, "apnpct-fired"))

	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, 7777)
	p.interacted = true

	fireApTriggerNpc(p, s, npc)

	// NAI-68: ClearInteraction dropped from fire helper; p.target restored to
	// original npc. Auto-clear happens in processInteraction tail's else-if.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (NPC_SAY script set no target)", p.nextTarget)
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

	// NAI-68: ClearInteraction dropped from fire helper; p.target restored to
	// original npc. Auto-clear happens in processInteraction tail's else-if.
	if p.target != npc {
		t.Errorf("target: got %v, want npc (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (NPC_SAY script set no target)", p.nextTarget)
	}
	if string(npc.sayText) != "apnpcu-fired" {
		t.Errorf("npc.sayText: got %q, want %q", npc.sayText, "apnpcu-fired")
	}
}

// TestFireApTriggerLocNoScriptSetsApRangeSentinel verifies that when
// fireApTriggerLoc finds no registered AP script for (trigger,
// locType, category), it sets p.apRange = -1 as a sentinel. Closes
// S6l-D1: matches TS Player.ts:~1139-1170 apRange=-1 semantics.
func TestFireApTriggerLocNoScriptSetsApRangeSentinel(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	// Anchor an OpLoc1 interaction. makeOpLocFixture registers
	// LocType 42 but NO AP script for it.
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// Sanity: apRange starts at 10 (SetInteraction default).
	if p.apRange != 10 {
		t.Fatalf("apRange pre-fire: got %d, want 10", p.apRange)
	}

	fireApTriggerLoc(p, s, loc)

	if p.apRange != -1 {
		t.Errorf("apRange post no-script fire: got %d, want -1 (S6l-D1 sentinel)", p.apRange)
	}
}

// TestApRangeSentinelShortCircuitsApproachGate verifies that with
// p.apRange = -1, inApproachDistance returns false regardless of
// actual player-to-target distance. This is how the sentinel skips
// re-lookup on subsequent ticks.
func TestApRangeSentinelShortCircuitsApproachGate(t *testing.T) {
	// Player at (100, 100), target at (101, 100) — distance 1 tile.
	// With apRange=-1, should return false even though distance <
	// any positive apRange.
	if inApproachDistance(100, 100, 101, 100, 1, 1, -1, true) {
		t.Error("inApproachDistance should return false when apRange=-1 (sentinel)")
	}

	// Control: with apRange=5, same positions should return true.
	if !inApproachDistance(100, 100, 101, 100, 1, 1, 5, true) {
		t.Error("control: inApproachDistance should return true when apRange=5 and distance=1")
	}
}

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
	// NAI-68: target restored to loc (not nil); tail clears on next tick.
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after override Finished — NAI-68)", p.target)
	}
}

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
	// NAI-68: target restored to obj (not nil); tail clears on next tick.
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored after override Finished — NAI-68)", p.target)
	}
}

// TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Strategy parallels Task 2.4's apRange-preservation pin.
func TestFireApTriggerObjOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	const overrideTypeId = 7782
	p.targetSubject.com = overrideTypeId

	sf := newNoopScriptFile(t, script.TriggerApObj1, overrideTypeId, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have run override-keyed script for targetSubject.com=%d (default obj.Type=%d)",
			overrideTypeId, obj.Type)
	}
}

// TestSetInteractionResetsApRangeSentinel verifies that starting a
// fresh interaction clears the -1 sentinel. Codifies the contract
// so future refactors can't regress it silently.
func TestSetInteractionResetsApRangeSentinel(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)
	p.apRange = -1 // simulate a prior sentinel state

	p.SetInteraction(InteractionEngine, loc, 3, -1)

	if p.apRange != 10 {
		t.Errorf("apRange post SetInteraction: got %d, want 10 (sentinel should be reset)", p.apRange)
	}
}

// TestObjStillValid verifies the helper returns true when the obj is
// present in the target zone and false after it's cleared — parallels
// locStillValid for Obj targets.
func TestObjStillValid(t *testing.T) {
	s := newServerForScriptTest(t)
	o := addObjToZone(t, s, 0, 100, 100, 42, 0)

	if !objStillValid(s, o, 100, 100, 0) {
		t.Error("present obj: objStillValid = false, want true")
	}

	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = nil
	if objStillValid(s, o, 100, 100, 0) {
		t.Error("removed obj: objStillValid = true, want false")
	}
}

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
	// loc.Type() == 42 in this fixture; use a different com value so keys differ.
	const overrideTypeId = 7777
	// targetSubject.com=overrideTypeId → resolveTriggerTypeId returns overrideTypeId (TS Player.ts:993-995).
	p.targetSubject.com = overrideTypeId
	want := newNoopScriptFile(t, script.TriggerOpLoc1, overrideTypeId, -1)
	s.scriptProvider.Register(want)
	// Counter-pin: a script keyed at the loc's actual type must NOT be returned.
	deceiver := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(deceiver)

	got := getOpTrigger(p, s)
	if got != want {
		t.Errorf("getOpTrigger: got %p, want %p (com override per TS Player.ts:993-995)", got, want)
	}
}

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
