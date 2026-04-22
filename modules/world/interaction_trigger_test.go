package world

import (
	"bytes"
	"fmt"
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
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
	if p.target != nil {
		t.Error("target: expected cleared after Finished")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true after dispatch")
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
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
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
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
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
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
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
	if p.interactionFired {
		t.Error("interactionFired: expected false so next tick retries")
	}
}

func TestTryFireOpTrigger_ReClickResetsFired(t *testing.T) {
	_, p, _ := newTriggerFixture(t)
	tryFireOpTrigger(p)
	if !p.interactionFired {
		t.Fatalf("pre: expected interactionFired true")
	}
	npc2Type := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Op: []string{"Talk-to"}}
	npc2 := NewNpc(1, 8, p.x, p.z, p.level, npc2Type)
	p.SetInteraction(InteractionEngine, npc2, 1, -1)
	if p.interactionFired {
		t.Error("interactionFired: expected false after SetInteraction")
	}
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

// TestProcessInteractionInteractionScriptKindSkipsDispatch verifies that the
// processInteraction hook gates tryFireOpTrigger on InteractionEngine. A future
// sub-spec introducing InteractionScript anchors should not see engine-style
// trigger dispatch.
func TestProcessInteractionInteractionScriptKindSkipsDispatch(t *testing.T) {
	_, p, npc := newTriggerFixture(t)
	// Re-anchor as script-kind instead of engine-kind. SetInteraction resets
	// interactionFired to false so the gate's other condition matches.
	p.SetInteraction(InteractionScript, npc, 1, -1)
	p.processInteraction()
	if len(npc.sayText) != 0 {
		t.Errorf("sayText: expected empty (dispatch skipped for script-kind), got %q", npc.sayText)
	}
	if p.interactionFired {
		t.Error("interactionFired: expected false (dispatch skipped, not consumed)")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after default-op clear")
	}
	// Assert the message text appears in the drained bytes. The
	// wire format is [opcode][length][text+10]; we check the text
	// substring to stay robust to framing details.
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("drained bytes: expected \"Nothing interesting happens.\" substring, got %x", got)
	}
}

// TestTryFireOpTriggerLocScriptFires verifies a registered [oploc1,<typeID>]
// script fires, ActiveLoc is set, and ClearInteraction runs after Finished.
func TestTryFireOpTriggerLocScriptFires(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	// Register a no-op script for [oploc1, locType=42].
	sf := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
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
	if p.interactionFired {
		t.Error("interactionFired: want false (deferred)")
	}
}

// TestTryFireOpTriggerLocTypeChanged verifies in-place type mutation
// (loc.Info changed via packLocInfo) clears interaction silently.
func TestTryFireOpTriggerLocTypeChanged(t *testing.T) {
	_, p, loc, _ := makeOpLocTriggerFixture(t)

	// Mutate the loc's type in-place by overwriting Info. New type 99
	// differs from p.targetSubject.typ (42).
	loc.Info = (99 & 0x3FFF) | (10&0x1F)<<14 | (0&0x3)<<19

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (type changed)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after type-change clear")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after removal clear")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after invalid-op clear")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-AP-script mark")
	}
}

// TestTryFireApTriggerLocScriptFiresNoApRangeCalled verifies an APLOC
// script that runs but doesn't call p_aprange causes ClearInteraction
// per TS Player.ts:1261 (if interacted && !apRangeCalled: clear).
func TestTryFireApTriggerLocScriptFiresNoApRangeCalled(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register a no-op APLOC1 script for locType=42.
	sf := newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after clear")
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
// that calls p_aprange sets apRangeCalled=true, which causes the
// interaction to PERSIST past the tick (no ClearInteraction). repathed
// is reset to force a fresh path on the next tick.
func TestTryFireApTriggerLocScriptCallsPApRange(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register an APLOC1 script that calls p_aprange(5).
	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 5)
	s.scriptProvider.Register(sf)

	p.repathed = true // verify it gets reset to false post-fire

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
	if p.repathed {
		t.Error("repathed: want false (reset post-p_aprange for fresh path)")
	}
	if p.interactionFired {
		t.Error("interactionFired: want false (allow re-fire next tick)")
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
	if p.interactionFired {
		t.Error("interactionFired: want false (deferred)")
	}
}

// TestTryFireApTriggerLocTypeChanged verifies in-place type mutation
// (loc.Info changed) clears interaction silently.
func TestTryFireApTriggerLocTypeChanged(t *testing.T) {
	_, p, loc, _ := makeApTriggerFixture(t)

	// Mutate loc.Info to a different type (99 ≠ 42 recorded in targetSubject).
	loc.Info = (99 & 0x3FFF) | (10&0x1F)<<14 | (0&0x3)<<19

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (lifecycle gate)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after lifecycle clear")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after removal clear")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after invalid-op clear")
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

	sf := newNoopScriptFile(t, script.TriggerOpLocT, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPLOCT fire")
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

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPLOCU fire")
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

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APLOCT fire")
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

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APLOCU fire")
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

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
		Category:    0,
	}
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
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
	}
}

// TestFireApTriggerNpcScriptFires verifies that with an APNPC1 script
// registered at (TriggerApNpc1, typeID=7, categoryID=-1), fireApTriggerNpc
// runs the script, binds ActiveNpc, and clears the interaction after
// Finished (no apRangeCalled persistence — TS divergence #3).
func TestFireApTriggerNpcScriptFires(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApNpc1, 7, -1)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after dead-clear")
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
	if p.interactionFired {
		t.Error("interactionFired: expected false so next tick retries")
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
	if !p.interactionFired {
		t.Error("interactionFired: want true after silent clear")
	}
}

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
// APNPCT dispatch at approach distance.
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
	if inApproachDistance(100, 100, 101, 100, -1) {
		t.Error("inApproachDistance should return false when apRange=-1 (sentinel)")
	}

	// Control: with apRange=5, same positions should return true.
	if !inApproachDistance(100, 100, 101, 100, 5) {
		t.Error("control: inApproachDistance should return true when apRange=5 and distance=1")
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
