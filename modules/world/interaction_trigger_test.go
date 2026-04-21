package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// buildNpcSayScript produces a tiny [push "hello", NPC_SAY, RETURN] script
// keyed at the trigger+typeID-specific lookup key.
func buildNpcSayScript(trigger script.ServerTriggerType, typeID int, text string) *script.ScriptFile {
	key := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
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
	p.SetInteraction(InteractionEngine, npc, 1)
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
	p.SetInteraction(InteractionEngine, npc, 1)
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
	p.SetInteraction(InteractionEngine, nonNpcEntity{}, 1)
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
		LookupKey:        uint32(script.TriggerOpNpc1) | (0x2 << 8) | (uint32(7) << 10),
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
	p.SetInteraction(InteractionEngine, npc, 1)
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
	p.SetInteraction(InteractionEngine, npc2, 1)
	if p.interactionFired {
		t.Error("interactionFired: expected false after SetInteraction")
	}
}

func TestTryFireOpTrigger_CategoryFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Script registered at category-level, not type-level.
	categoryKey := uint32(script.TriggerOpNpc1) | (0x1 << 8) | (uint32(3) << 10)
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
	p.SetInteraction(InteractionEngine, npc, 1)
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
		LookupKey:        uint32(script.TriggerOpNpc1),
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
	p.SetInteraction(InteractionEngine, npc, 1)
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
	p.SetInteraction(InteractionScript, npc, 1)
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
	key := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
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
// Returns (server, player, loc).
func makeOpLocTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Loc) {
	t.Helper()
	s, p, loc, _ := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, 1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return s, p, loc
}

// TestTryFireOpTriggerLocNoScript verifies a Loc target with no registered
// trigger silently clears the interaction.
func TestTryFireOpTriggerLocNoScript(t *testing.T) {
	_, p, _ := makeOpLocTriggerFixture(t)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after silent clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-script clear")
	}
}

// TestTryFireOpTriggerLocScriptFires verifies a registered [oploc1,<typeID>]
// script fires, ActiveLoc is set, and ClearInteraction runs after Finished.
func TestTryFireOpTriggerLocScriptFires(t *testing.T) {
	s, p, loc := makeOpLocTriggerFixture(t)

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
	s, p, loc := makeOpLocTriggerFixture(t)
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
	_, p, loc := makeOpLocTriggerFixture(t)

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
	s, p, loc := makeOpLocTriggerFixture(t)

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
	_, p, _ := makeOpLocTriggerFixture(t)
	p.targetOp = 0 // invalid

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (invalid op)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after invalid-op clear")
	}
}
