package world

import (
	"testing"

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
