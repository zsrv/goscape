package script

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// -- mock active entity stubs (S6v) -------------------------------------

type mockActiveLoc struct {
	locType     int
	x, z, level int
	angle       int
	shape       int
	layer       int
	active      bool
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
func (m *mockActiveLoc) Shape() int                { return m.shape }
func (m *mockActiveLoc) Layer() int                { return m.layer }
func (m *mockActiveLoc) Active() bool              { return m.active }

type mockActiveNpc struct {
	typeId, x, z, level int
	stats               [8]int
}

func (m *mockActiveNpc) NpcType() int                            { return m.typeId }
func (m *mockActiveNpc) NpcX() int                               { return m.x }
func (m *mockActiveNpc) NpcZ() int                               { return m.z }
func (m *mockActiveNpc) NpcLevel() int                           { return m.level }
func (m *mockActiveNpc) NpcStat(stat int) int                    { return m.stats[stat] }
func (m *mockActiveNpc) NpcBaseStat(stat int) int                { return 0 }
func (m *mockActiveNpc) NpcCategory() int                        { return 0 }
func (m *mockActiveNpc) NpcWidth() int                           { return 1 }
func (m *mockActiveNpc) NpcLength() int                          { return 1 }
func (m *mockActiveNpc) NpcUID() int                             { return 0 }
func (m *mockActiveNpc) Nid() int                                { return 0 }
func (m *mockActiveNpc) LastMovement() int                       { return 0 }
func (m *mockActiveNpc) Respawnrate() int                        { return 0 }
func (m *mockActiveNpc) NpcVarN(id int) int32                    { return 0 }
func (m *mockActiveNpc) SetNpcVarN(id int, val int32)            {}
func (m *mockActiveNpc) NpcVarNString(id int) string             { return "" }
func (m *mockActiveNpc) SetNpcVarNString(id int, val string)     {}
func (m *mockActiveNpc) Say(text []byte)                         {}
func (m *mockActiveNpc) Animate(id, delay int)                   {}
func (m *mockActiveNpc) FaceCoord(x, z int)                      {}
func (m *mockActiveNpc) ChangeType(newType, duration int)        {}
func (m *mockActiveNpc) ChangeTypeKeepAll(newType, duration int) {}
func (m *mockActiveNpc) Damage(amount, dmgType int)              {}

func (m *mockActiveNpc) StoreActiveScript(_ *ScriptState)                      {}
func (m *mockActiveNpc) ClearActiveScript()                                    {}
func (m *mockActiveNpc) OnScriptFinishedOrAborted(_ *ScriptState)              {}
func (m *mockActiveNpc) SetDelayed(_ int)                                      {}
func (m *mockActiveNpc) EnqueueScriptForTrigger(_ ServerTriggerType, _, _ int) {}
func (m *mockActiveNpc) SetTimer(_ int)                                        {}
func (m *mockActiveNpc) SetHuntRange(_ int)                                    {}
func (m *mockActiveNpc) SetHuntMode(_ int)                                     {}
func (m *mockActiveNpc) SetWalkTrigger(_ int)                                  {}
func (m *mockActiveNpc) SetWalkTriggerArg(_ int)                               {}
func (m *mockActiveNpc) Teleport(_, _, _ int)                                  {}
func (m *mockActiveNpc) QueueWaypoint(_, _ int)                                {}
func (m *mockActiveNpc) TargetOp() int                                         { return 0 }
func (m *mockActiveNpc) ClearInteraction()                                     {}
func (m *mockActiveNpc) ResetDefaults()                                        {}
func (m *mockActiveNpc) ClearPatrol()                                          {}
func (m *mockActiveNpc) SetTargetOp(_ int)                                     {}
func (m *mockActiveNpc) SetInteractionScript(_ any, _ int)                     {}
func (m *mockActiveNpc) SetNpcStat(_, _ int)                                   {}
func (m *mockActiveNpc) PlaySpotAnim(_, _, _ int)                              {}
func (m *mockActiveNpc) AddHeroPoints(_, _ int)                                {}
func (m *mockActiveNpc) TopContributor() int                                   { return 0 }
func (m *mockActiveNpc) TargetWithinMaxRange() bool                            { return false }
func (m *mockActiveNpc) HeroPointsClear()                                      {}

// newSingleOp builds a single-opcode script plus its trailing OpReturn,
// so handler tests can run a handler in isolation and observe the state
// after.
func newSingleOp(name string, op Opcode) *ScriptFile {
	return &ScriptFile{
		Name:             name,
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// -- Stat read tests -----------------------------------------------------

func TestStatReadsSeededLevel(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 50

	sf := &ScriptFile{
		Name: "stat",
		Opcodes: []Opcode{
			OpPushConstantInt, // push stat id = 3
			OpStat,
			OpReturn,
		},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 50 {
		t.Errorf("STAT: got %d, want 50", got)
	}
}

func TestStatBaseReadsSeededBase(t *testing.T) {
	mp := &mockPlayer{}
	mp.baseLevels[0] = 7

	sf := &ScriptFile{
		Name: "stat_base",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpStatBase,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7 {
		t.Errorf("STAT_BASE: got %d, want 7", got)
	}
}

// TestStatTotalSumsAllBases was deleted: STAT_TOTAL removed from 244 enum.

// -- Stat mutation tests -------------------------------------------------

func TestStatAddFormula(t *testing.T) {
	// TS: added = current + constant + (base*percent)/100, capped at 255.
	// Seed: id=2, base=80, current=50, constant=10, percent=25
	// → 50 + (10 + 80*25/100) = 50 + (10 + 20) = 80
	mp := &mockPlayer{}
	mp.levels[2] = 50
	mp.baseLevels[2] = 80

	sf := &ScriptFile{
		Name: "stat_add",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat id
			OpPushConstantInt, // constant
			OpPushConstantInt, // percent (top)
			OpStatAdd,
			OpReturn,
		},
		IntOperands:      []int32{2, 10, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.setCurLevelCalls) != 1 {
		t.Fatalf("setCurLevelCalls: got %d, want 1", len(mp.setCurLevelCalls))
	}
	if got := mp.setCurLevelCalls[0]; got.id != 2 || got.level != 80 {
		t.Errorf("STAT_ADD: got %+v, want {id:2,level:80}", got)
	}
}

func TestStatAddCapsAt255(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[1] = 250
	mp.baseLevels[1] = 250

	sf := &ScriptFile{
		Name: "stat_add_cap",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{1, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 255 {
		t.Errorf("STAT_ADD cap: got %d, want 255", got)
	}
}

// TestStatAddOnHitpointsAtFullClearsHeroPoints pins STAT_ADD's HP-full
// heroPoints.clear() tail (TS PlayerOps.ts:513-515): when stat ==
// HITPOINTS and post-update levels[HITPOINTS] >= baseLevels[HITPOINTS],
// the player's hero-point ledger is cleared. The mock's
// SetCurLevel does not mutate m.levels, so we pre-seed m.levels[3] to
// the value the predicate should observe.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15     // PlayerStatHitpoints; predicate sees this as post-update
	mp.baseLevels[3] = 15 // HP-full: 15 >= 15

	sf := &ScriptFile{
		Name: "stat_add_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat id
			OpPushConstantInt, // constant
			OpPushConstantInt, // percent (top)
			OpStatAdd,
			OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0}, // stat=HITPOINTS, const=1, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_ADD HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}

// TestStatAddOnHitpointsNotFullSkipsClear pins the HP-not-full negative
// branch: predicate sees levels[3] < baseLevels[3] → no clear.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10     // post-update HP per predicate read
	mp.baseLevels[3] = 15 // not full: 10 < 15

	sf := &ScriptFile{
		Name: "stat_add_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_ADD HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}

// TestStatAddOnNonHitpointsStatSkipsClear pins the stat-gate: when
// stat != HITPOINTS, the clear NEVER fires even if HP happens to be
// full. Mirrors TS PlayerOps.ts:513 gate `stat === PlayerStat.HITPOINTS &&`.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15 // HP IS full
	mp.baseLevels[3] = 15
	mp.levels[0] = 50 // Attack; the stat being mutated
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_add_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{0, 10, 25, 0, 0}, // stat=ATTACK, const=10, pct=25
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_ADD non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}

// TestStatBoostOnHitpointsAtFullClearsHeroPoints pins STAT_BOOST's
// HP-full heroPoints.clear() tail (TS PlayerOps.ts:552-554).
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_boost_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{3, 0, 0, 0, 0}, // stat=HITPOINTS, const=0, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_BOOST HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}

// TestStatBoostOnHitpointsNotFullSkipsClear pins the HP-not-full
// negative branch for STAT_BOOST.
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_boost_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_BOOST HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}

// TestStatBoostOnNonHitpointsStatSkipsClear pins the stat-gate for
// STAT_BOOST. Mirrors TS PlayerOps.ts:552 gate.
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 5, 0, 0, 0}, // stat=ATTACK
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_BOOST non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}

// TestStatHealOnHitpointsAtFullClearsHeroPoints pins STAT_HEAL's
// HP-full heroPoints.clear() tail (TS PlayerOps.ts:609-611).
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_heal_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 5, 0, 0, 0}, // stat=HITPOINTS, const=5, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_HEAL HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}

// TestStatHealOnHitpointsNotFullSkipsClear pins the HP-not-full
// negative branch for STAT_HEAL.
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_heal_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_HEAL HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}

// TestStatHealOnNonHitpointsStatSkipsClear pins the stat-gate for
// STAT_HEAL. Mirrors TS PlayerOps.ts:609 gate.
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_heal_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{0, 5, 0, 0, 0}, // stat=ATTACK
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_HEAL non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}

// -- ChangeStat trigger fire tests --------------------------------------
//
// All 5 stat-mutation opcodes (STAT_ADD/SUB/BOOST/DRAIN/HEAL) fire the
// [changestat,<skill>] trigger after SetCurLevel when the PRE-CLAMP
// computed value differs from the prior current level. Mirrors TS
// PlayerOps.ts:516-518, :534-536, :555-557, :572-574, :613-615
// `if (added/subbed/boosted/healed !== current) player.changeStat(stat)`.
// Pre-clamp predicate means a 255→255 capped boost STILL fires if the
// unclamped value differs.

func TestStatAddFiresChangeStat(t *testing.T) {
	// cur=50, base=80, const=10, pct=25 → added=50+(10+20)=80; 80!=50 → fire.
	mp := &mockPlayer{}
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_add_fires_changestat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{0, 10, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 0 {
		t.Errorf("STAT_ADD changeStatCalls: got %v, want [0]", got)
	}
}

func TestStatAddCapped255StillFires(t *testing.T) {
	// Pre-clamp predicate: cur=255, const=1 → added=256, clamped to 255.
	// added (256) != cur (255) → fire. Mirrors TS pre-clamp `if (added !== current)`.
	mp := &mockPlayer{}
	mp.levels[0] = 255
	mp.baseLevels[0] = 255

	sf := &ScriptFile{
		Name: "stat_add_capped_still_fires",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{0, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 0 {
		t.Errorf("STAT_ADD capped-at-255 changeStatCalls: got %v, want [0] (pre-clamp predicate)", got)
	}
}

func TestStatAddNoopDoesNotFire(t *testing.T) {
	// True no-op: const=0, pct=0 → added=cur, no fire.
	mp := &mockPlayer{}
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_add_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 0 {
		t.Errorf("STAT_ADD no-op changeStatCalls: got %v, want []", got)
	}
}

func TestStatSubFiresChangeStat(t *testing.T) {
	// cur=60, base=50, const=5, pct=20 → subbed=60-(5+10)=45; 45!=60 → fire.
	mp := &mockPlayer{}
	mp.levels[4] = 60
	mp.baseLevels[4] = 50

	sf := &ScriptFile{
		Name: "stat_sub_fires_changestat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{4, 5, 20, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 4 {
		t.Errorf("STAT_SUB changeStatCalls: got %v, want [4]", got)
	}
}

func TestStatSubNoopDoesNotFire(t *testing.T) {
	// const=0, pct=0 → subbed=cur, no fire.
	mp := &mockPlayer{}
	mp.levels[4] = 60
	mp.baseLevels[4] = 50

	sf := &ScriptFile{
		Name: "stat_sub_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{4, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 0 {
		t.Errorf("STAT_SUB no-op changeStatCalls: got %v, want []", got)
	}
}

func TestStatBoostFiresChangeStat(t *testing.T) {
	// cur=50, base=80, const=10, pct=0 → boost=10, boosted=max(min(60,90),50)=60; 60!=50 → fire.
	mp := &mockPlayer{}
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_fires_changestat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 0 {
		t.Errorf("STAT_BOOST changeStatCalls: got %v, want [0]", got)
	}
}

func TestStatBoostNoopDoesNotFire(t *testing.T) {
	// cur>base with const=0/pct=0 → boost=0, boosted=max(min(cur,base),cur)=cur → no fire.
	mp := &mockPlayer{}
	mp.levels[0] = 120
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 0 {
		t.Errorf("STAT_BOOST no-op changeStatCalls: got %v, want []", got)
	}
}

func TestStatDrainFiresChangeStat(t *testing.T) {
	// cur=80, base=20, const=0, pct=25 → subbed=80-(0+80*25/100)=60; 60!=80 → fire.
	mp := &mockPlayer{}
	mp.levels[2] = 80
	mp.baseLevels[2] = 20

	sf := &ScriptFile{
		Name: "stat_drain_fires_changestat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatDrain, OpReturn,
		},
		IntOperands:      []int32{2, 0, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 2 {
		t.Errorf("STAT_DRAIN changeStatCalls: got %v, want [2]", got)
	}
}

func TestStatDrainNoopDoesNotFire(t *testing.T) {
	// const=0, pct=0 → subbed=cur, no fire.
	mp := &mockPlayer{}
	mp.levels[2] = 80
	mp.baseLevels[2] = 20

	sf := &ScriptFile{
		Name: "stat_drain_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatDrain, OpReturn,
		},
		IntOperands:      []int32{2, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 0 {
		t.Errorf("STAT_DRAIN no-op changeStatCalls: got %v, want []", got)
	}
}

func TestStatHealFiresChangeStat(t *testing.T) {
	// cur=10, base=50, const=20, pct=0 → healed=10+20=30; 30!=10 → fire.
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal_fires_changestat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 1 || got[0] != 3 {
		t.Errorf("STAT_HEAL changeStatCalls: got %v, want [3]", got)
	}
}

func TestStatHealNoopDoesNotFire(t *testing.T) {
	// const=0, pct=0 → healed=cur, no fire.
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.changeStatCalls; len(got) != 0 {
		t.Errorf("STAT_HEAL no-op changeStatCalls: got %v, want []", got)
	}
}

func TestStatSubFormula(t *testing.T) {
	// subbed = current - (constant + (base*percent)/100), clamped >=0.
	// id=4, current=60, base=50, constant=5, percent=20 → 60 - (5 + 10) = 45.
	mp := &mockPlayer{}
	mp.levels[4] = 60
	mp.baseLevels[4] = 50

	sf := &ScriptFile{
		Name: "stat_sub",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{4, 5, 20, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0]; got.id != 4 || got.level != 45 {
		t.Errorf("STAT_SUB: got %+v, want {id:4,level:45}", got)
	}
}

func TestStatSubFloorsAtZero(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[5] = 3
	mp.baseLevels[5] = 50

	sf := &ScriptFile{
		Name: "stat_sub_floor",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{5, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 0 {
		t.Errorf("STAT_SUB floor: got %d, want 0", got)
	}
}

func TestStatBoostClampsToBasePlusBoost(t *testing.T) {
	// TS: boost=10, boosted = max(min(cur+boost, base+boost), cur).
	// id=0, cur=50, base=80, constant=10, percent=0 → boost=10.
	// cur+boost=60; base+boost=90; min=60; max(60, 50)=60.
	mp := &mockPlayer{}
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 60 {
		t.Errorf("STAT_BOOST: got %d, want 60", got)
	}
}

func TestStatBoostNeverLowersCurrent(t *testing.T) {
	// If cur is already above base+boost, the max(cur,...) clamp keeps cur.
	// id=0, cur=120, base=80, boost=10 → cur+boost=130, base+boost=90,
	// min(130,90)=90, max(90, 120)=120.
	mp := &mockPlayer{}
	mp.levels[0] = 120
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 120 {
		t.Errorf("STAT_BOOST noop: got %d, want 120", got)
	}
}

func TestStatDrainUsesCurrentNotBase(t *testing.T) {
	// TS: drain uses current, not base.
	// id=2, cur=80, base=20, constant=0, percent=25 → 80 - (0 + 80*25/100) = 80 - 20 = 60.
	mp := &mockPlayer{}
	mp.levels[2] = 80
	mp.baseLevels[2] = 20 // deliberately different from cur to catch the bug

	sf := &ScriptFile{
		Name: "stat_drain",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatDrain, OpReturn,
		},
		IntOperands:      []int32{2, 0, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 60 {
		t.Errorf("STAT_DRAIN: got %d, want 60", got)
	}
}

func TestStatHealCapsAtBase(t *testing.T) {
	// healed = cur + (constant + (base*percent)/100), capped at base.
	// id=3, cur=10, base=50, constant=100, percent=0 → healed=110, capped to 50.
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 50 {
		t.Errorf("STAT_HEAL cap: got %d, want 50", got)
	}
}

func TestStatHealNeverLowersCurrent(t *testing.T) {
	// If cur > base (boosted), max(min(healed, base), cur) keeps cur.
	// id=3, cur=99, base=50 → min(99+const, 50)=50, max(50, 99)=99.
	mp := &mockPlayer{}
	mp.levels[3] = 99
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 99 {
		t.Errorf("STAT_HEAL noop: got %d, want 99", got)
	}
}

func TestStatAdvanceForwardsToAddXP(t *testing.T) {
	// TS popInts(2) = [stat, xp]; stack top = xp.
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "stat_advance",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat
			OpPushConstantInt, // xp (top)
			OpStatAdvance,
			OpReturn,
		},
		IntOperands:      []int32{7, 250, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.addXPCalls) != 1 {
		t.Fatalf("addXPCalls: got %d, want 1", len(mp.addXPCalls))
	}
	if got := mp.addXPCalls[0]; got.id != 7 || got.xp != 250 {
		t.Errorf("STAT_ADVANCE: got %+v, want {id:7,xp:250}", got)
	}
}

func TestStatRandomPushesZeroOrOne(t *testing.T) {
	// Can't assert the exact value without reseeding rand; just confirm
	// it's 0 or 1.
	mp := &mockPlayer{}
	mp.levels[6] = 50

	sf := &ScriptFile{
		Name: "stat_random",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat
			OpPushConstantInt, // low
			OpPushConstantInt, // high (top)
			OpStatRandom,
			OpReturn,
		},
		IntOperands:      []int32{6, 10, 200, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 && got != 1 {
		t.Errorf("STAT_RANDOM: got %d, want 0 or 1", got)
	}
}

// -- OOB stat id tests ---------------------------------------------------

// Covers STAT, STAT_BASE, STAT_ADD, STAT_SUB, STAT_BOOST, STAT_DRAIN,
// STAT_HEAL, STAT_ADVANCE, STAT_RANDOM with both -1 and NumStats=21.
func TestStatOpsRejectOOBStatID(t *testing.T) {
	type opCase struct {
		name      string
		op        Opcode
		intsBelow []int32 // additional ints pushed below the stat id
	}
	ops := []opCase{
		{"STAT", OpStat, nil},
		{"STAT_BASE", OpStatBase, nil},
		{"STAT_ADD", OpStatAdd, []int32{0, 0}}, // constant, percent
		{"STAT_SUB", OpStatSub, []int32{0, 0}},
		{"STAT_BOOST", OpStatBoost, []int32{0, 0}},
		{"STAT_DRAIN", OpStatDrain, []int32{0, 0}},
		{"STAT_HEAL", OpStatHeal, []int32{0, 0}},
		{"STAT_ADVANCE", OpStatAdvance, []int32{0}}, // xp
		// STAT_RANDOM intentionally absent: per h-player-4 it does NOT gate
		// on checkStatID (TS PlayerOps.ts:578-586 indexes the stats array
		// directly). OOB behaviour is pinned by
		// TestStatRandom_AcceptsOOBStatID_NoAbort below.
	}
	badIDs := []int32{-1, int32(NumStats)} // 21 is OOB

	for _, tc := range ops {
		for _, badID := range badIDs {
			t.Run(tc.name+"/id="+itoa(int(badID)), func(t *testing.T) {
				// Build a script: push stat id, push the "below" ints, then the op.
				pushes := 1 + len(tc.intsBelow)
				opcodes := make([]Opcode, 0, pushes+2)
				operands := make([]int32, 0, pushes+2)
				opcodes = append(opcodes, OpPushConstantInt)
				operands = append(operands, badID)
				for _, v := range tc.intsBelow {
					opcodes = append(opcodes, OpPushConstantInt)
					operands = append(operands, v)
				}
				opcodes = append(opcodes, tc.op, OpReturn)
				operands = append(operands, 0, 0)

				sf := &ScriptFile{
					Name:             "oob_" + tc.name,
					Opcodes:          opcodes,
					IntOperands:      operands,
					StringOperands:   make([]string, len(opcodes)),
					InstructionCount: uint32(len(opcodes)),
				}
				state := Init(sf, &mockPlayer{}, false, nil, nil)
				err := Execute(state)
				// L16: STAT_ADVANCE alone does NOT bound the stat id — it
				// forwards to addXp, where an OOB (>= NumStats) write is
				// silently ignored (TS TypedArray semantics; Go AddXP
				// bounds-guards), so it completes without error. The null
				// sentinel (-1) is still rejected by NumberNotNull. Every
				// other stat op rejects both bad ids.
				if tc.name == "STAT_ADVANCE" && badID == int32(NumStats) {
					if err != nil {
						t.Fatalf("%s id=%d: Execute returned %v, want nil (OOB no-op)", tc.name, badID, err)
					}
					if state.Execution != Finished {
						t.Errorf("%s id=%d: Execution = %v, want Finished", tc.name, badID, state.Execution)
					}
					return
				}
				if err == nil {
					t.Fatalf("%s id=%d: Execute returned nil, want error", tc.name, badID)
				}
				if state.Execution != Aborted {
					t.Errorf("%s id=%d: Execution = %v, want Aborted", tc.name, badID, state.Execution)
				}
			})
		}
	}
}

// TestStatRandomThreshold_MatchFloorSemantics pins h-player-4 (formula):
// `value = floor(low*(99-level)/98) + floor(high*(level-1)/98) + 1` uses
// Math.floor (round toward -∞) per JS, NOT Go's trunc-toward-zero. The
// divergence only fires when a numerator goes negative — which happens
// for boosted stats with level > 99 (the (99-level) factor in the low
// term flips sign).
//
// Toggle-revert RED proof: restore the pre-fix inline integer-division
// formula in handleStatRandom (`(low*(99-level))/98 + (high*(level-1))/98 + 1`)
// and remove statRandomThreshold; the boost subtests then read 11 and
// fail with the cited assertion message. Unboosted subtests stay GREEN.
func TestStatRandomThreshold_MatchFloorSemantics(t *testing.T) {
	tests := []struct {
		name             string
		low, high, level int
		want             int
		preFixGoTrunc    int
	}{
		{"level 50 (positive numerators)", 10, 10, 50, 11, 11},
		{"level 99 (low term zero)", 10, 10, 99, 11, 11},
		{"level 120 boost (negative numerator)", 10, 10, 120, 10, 11},
		{"level 200 extreme boost", 10, 10, 200, 10, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statRandomThreshold(tt.low, tt.high, tt.level)
			if got != tt.want {
				t.Errorf("statRandomThreshold(low=%d,high=%d,level=%d): got %d, want %d (pre-fix Go trunc-toward-zero would give %d); TS PlayerOps.ts:578-586 uses Math.floor not integer division (h-player-4)",
					tt.low, tt.high, tt.level, got, tt.want, tt.preFixGoTrunc)
			}
		})
	}
}

// TestStatRandom_AcceptsOOBStatID_NoAbort pins h-player-4 (gate): the
// handler does NOT abort the script for an out-of-range stat id. TS
// PlayerOps.ts:578-586 indexes `player.stats[id]` directly; an OOB index
// in JS returns undefined → NaN propagates through the formula →
// value=NaN → `value > chance` is false → pushes 0. goscape's pre-fix
// `checkStatID` raised "STAT_RANDOM: stat id out of range" instead;
// here we lean on (*Player).Stat returning 0 for OOB ids so the formula
// evaluates safely, the handler returns nil, and the pushed value
// remains within the 0/1 contract.
func TestStatRandom_AcceptsOOBStatID_NoAbort(t *testing.T) {
	for _, badID := range []int32{-1, int32(NumStats)} {
		t.Run("id="+itoa(int(badID)), func(t *testing.T) {
			sf := &ScriptFile{
				Name: "stat_random_oob",
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatRandom, OpReturn,
				},
				IntOperands:      []int32{badID, 10, 200, 0, 0},
				StringOperands:   []string{"", "", "", "", ""},
				InstructionCount: 5,
			}
			state := Init(sf, &mockPlayer{}, false, nil, nil)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: err=%v want nil — TS PlayerOps.ts:578-586 has no checkStatID gate (h-player-4)", err)
			}
			if state.Execution != Finished {
				t.Errorf("Execution=%v want Finished — STAT_RANDOM must not abort on OOB id (h-player-4)", state.Execution)
			}
			if got := state.PopInt(); got != 0 && got != 1 {
				t.Errorf("STAT_RANDOM OOB push: got %d want 0 or 1", got)
			}
		})
	}
}

// -- Coord / facing / teleport tests ------------------------------------

func TestCoordPushesPacked(t *testing.T) {
	mp := &mockPlayer{coordPacked: 0x1234_5678}
	state := Init(newSingleOp("coord", OpCoord), mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0x1234_5678 {
		t.Errorf("COORD: got %#x, want %#x", got, 0x1234_5678)
	}
}

func packCoord(level, x, z int) int {
	return ((level & 0x3) << 28) | ((x & 0x3fff) << 14) | (z & 0x3fff)
}

func TestPTeleJumpUnpacksCoord(t *testing.T) {
	// Lumbridge-style test: (3222, 3222, 0).
	mp := &mockPlayer{}
	packed := packCoord(0, 3222, 3222)
	sf := &ScriptFile{
		Name: "p_telejump",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleJump, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.teleJumpCalls != 1 {
		t.Fatalf("teleJumpCalls: got %d, want 1", mp.teleJumpCalls)
	}
	if mp.lastTeleJump != (struct{ x, z, level int }{3222, 3222, 0}) {
		t.Errorf("P_TELEJUMP: got %+v, want {3222, 3222, 0}", mp.lastTeleJump)
	}
}

func TestPTeleJumpRoundTripsLevel(t *testing.T) {
	// Level 3 (the 2-bit max) exercises the level mask.
	mp := &mockPlayer{}
	packed := packCoord(3, 3222, 3222)
	sf := &ScriptFile{
		Name: "p_telejump_level3",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleJump, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastTeleJump != (struct{ x, z, level int }{3222, 3222, 3}) {
		t.Errorf("P_TELEJUMP level=3: got %+v, want {3222, 3222, 3}", mp.lastTeleJump)
	}
}

func TestPTeleportUnpacksCoord(t *testing.T) {
	mp := &mockPlayer{}
	packed := packCoord(2, 1000, 2000)
	sf := &ScriptFile{
		Name: "p_teleport",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleport, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.teleportCalls != 1 {
		t.Fatalf("teleportCalls: got %d, want 1", mp.teleportCalls)
	}
	if mp.lastTeleport != (struct{ x, z, level int }{1000, 2000, 2}) {
		t.Errorf("P_TELEPORT: got %+v, want {1000, 2000, 2}", mp.lastTeleport)
	}
}

func TestFaceSquareIgnoresLevelComponent(t *testing.T) {
	// FaceSquare takes (x, z) only — the level bits are discarded.
	mp := &mockPlayer{}
	packed := packCoord(2, 3200, 3250)
	sf := &ScriptFile{
		Name: "facesquare",
		Opcodes: []Opcode{
			OpPushConstantInt, OpFaceSquare, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.faceSquareCalls != 1 {
		t.Fatalf("faceSquareCalls: got %d, want 1", mp.faceSquareCalls)
	}
	if mp.lastFaceSquare != (struct{ x, z int }{3200, 3250}) {
		t.Errorf("FACESQUARE: got %+v, want {3200, 3250}", mp.lastFaceSquare)
	}
}

// TestPTeleportRejectsBadCoord pins TS PlayerOps.ts:447-451 —
// check(state.popInt(), CoordValid) before any Teleport call.
// CoordValid (ScriptValidators.ts:109) rejects packed coords outside
// [0, 2147483647]; goscape's pre-fix unpackCoord(-1) silently
// produced garbage (x, z, level). Closes h-player-3 (audit row 248).
func TestPTeleportRejectsBadCoord(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_teleport_badcoord", OpPTeleport)
	state := Init(sf, mp, true, nil, nil) // protect=true
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for coord=-1, got nil (TS PlayerOps.ts:448 CoordValid must reject)")
	}
	if !strings.Contains(err.Error(), "P_TELEPORT: coord out of range") {
		t.Errorf("error: got %q, want substring 'P_TELEPORT: coord out of range'", err.Error())
	}
	if mp.teleportCalls != 0 {
		t.Errorf("teleportCalls: got %d, want 0 (must not teleport on rejected coord)", mp.teleportCalls)
	}
}

// TestPTeleJumpRejectsBadCoord pins TS PlayerOps.ts:439-443 —
// check(state.popInt(), CoordValid) before any TeleJump call.
// Closes h-player-3 (audit row 248).
func TestPTeleJumpRejectsBadCoord(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_telejump_badcoord", OpPTeleJump)
	state := Init(sf, mp, true, nil, nil) // protect=true
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for coord=-1, got nil (TS PlayerOps.ts:440 CoordValid must reject)")
	}
	if !strings.Contains(err.Error(), "P_TELEJUMP: coord out of range") {
		t.Errorf("error: got %q, want substring 'P_TELEJUMP: coord out of range'", err.Error())
	}
	if mp.teleJumpCalls != 0 {
		t.Errorf("teleJumpCalls: got %d, want 0 (must not teleJump on rejected coord)", mp.teleJumpCalls)
	}
}

// TestFaceSquareRejectsBadCoord pins TS PlayerOps.ts:239-243 —
// check(state.popInt(), CoordValid) before any FaceSquare call.
// Closes h-player-3 (audit row 248). FACESQUARE has no protected
// gate (ActivePlayer only), so protect=false.
func TestFaceSquareRejectsBadCoord(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("facesquare_badcoord", OpFaceSquare)
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for coord=-1, got nil (TS PlayerOps.ts:240 CoordValid must reject)")
	}
	if !strings.Contains(err.Error(), "FACESQUARE: coord out of range") {
		t.Errorf("error: got %q, want substring 'FACESQUARE: coord out of range'", err.Error())
	}
	if mp.faceSquareCalls != 0 {
		t.Errorf("faceSquareCalls: got %d, want 0 (must not face on rejected coord)", mp.faceSquareCalls)
	}
}

// -- Animation tests -----------------------------------------------------

func TestAnimCapturesSeqAndDelay(t *testing.T) {
	// TS pops (seq, delay); stack top is delay.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "anim",
		Opcodes: []Opcode{
			OpPushConstantInt, // seq
			OpPushConstantInt, // delay (top)
			OpAnim,
			OpReturn,
		},
		IntOperands:      []int32{808, 5, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.playAnimCalls != 1 {
		t.Fatalf("playAnimCalls: got %d, want 1", mp.playAnimCalls)
	}
	if mp.lastPlayAnim != (struct{ seqID, delay int }{808, 5}) {
		t.Errorf("ANIM: got %+v, want {seqID:808, delay:5}", mp.lastPlayAnim)
	}
}

func TestSpotAnimPlCapturesTriple(t *testing.T) {
	// TS pops (spotanim, height, delay); stack top is delay.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "spotanim_pl",
		Opcodes: []Opcode{
			OpPushConstantInt, // spotanim id
			OpPushConstantInt, // height
			OpPushConstantInt, // delay (top)
			OpSpotAnimPl,
			OpReturn,
		},
		IntOperands:      []int32{42, 100, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.playSpotAnimCalls != 1 {
		t.Fatalf("playSpotAnimCalls: got %d, want 1", mp.playSpotAnimCalls)
	}
	want := struct{ id, height, delay int }{42, 100, 3}
	if mp.lastPlaySpotAnim != want {
		t.Errorf("SPOTANIM_PL: got %+v, want %+v", mp.lastPlaySpotAnim, want)
	}
}

// Table-driven test covering every BAS setter. All seven take (seqID)
// and call the corresponding SetXxxAnim on mockPlayer. The id is
// validated against SeqTypeValid per TS PlayerOps.ts:935-966.
func TestBASSetters(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		get  func(*mockPlayer) int
	}{
		{"BAS_READYANIM", OpBasReadyAnim, func(m *mockPlayer) int { return m.lastReadyAnim }},
		{"BAS_TURNONSPOT", OpBasTurnOnSpot, func(m *mockPlayer) int { return m.lastTurnAnim }},
		{"BAS_WALK_F", OpBasWalkF, func(m *mockPlayer) int { return m.lastWalkAnim }},
		{"BAS_WALK_B", OpBasWalkB, func(m *mockPlayer) int { return m.lastWalkAnimB }},
		{"BAS_WALK_L", OpBasWalkL, func(m *mockPlayer) int { return m.lastWalkAnimL }},
		{"BAS_WALK_R", OpBasWalkR, func(m *mockPlayer) int { return m.lastWalkAnimR }},
		{"BAS_RUNNING", OpBasRunning, func(m *mockPlayer) int { return m.lastRunAnim }},
	}
	mc := &mockConfigs{
		seqs: map[int]*objtype.SeqType{
			1234: {ConfigType: objtype.ConfigType{ID: 1234}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, tc.op, OpReturn,
				},
				IntOperands:      []int32{1234, 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, mp, false, nil, nil)
			state.Configs = mc
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := tc.get(mp); got != 1234 {
				t.Errorf("%s: got %d, want 1234", tc.name, got)
			}
		})
	}
}

// TestBASSettersRejectInvalidSeq pins TS check(state.popInt(),
// SeqTypeValid) — every BAS opcode aborts the script when the popped
// seq id is not registered. RUNANIM is excluded here because -1 is
// special-cased; its rejection path is exercised by
// TestRunAnimRejectsInvalidSeq.
func TestBASSettersRejectInvalidSeq(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
	}{
		{"BAS_READYANIM", OpBasReadyAnim},
		{"BAS_TURNONSPOT", OpBasTurnOnSpot},
		{"BAS_WALK_F", OpBasWalkF},
		{"BAS_WALK_B", OpBasWalkB},
		{"BAS_WALK_L", OpBasWalkL},
		{"BAS_WALK_R", OpBasWalkR},
		{"BAS_RUNNING", OpBasRunning},
	}
	mc := &mockConfigs{seqs: map[int]*objtype.SeqType{}} // empty registry
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, tc.op, OpReturn,
				},
				// id=42 is not registered. RUNANIM uses 42
				// here (not -1) so the SeqTypeValid branch
				// fires; the -1 sentinel is tested
				// separately.
				IntOperands:      []int32{42, 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, mp, false, nil, nil)
			state.Configs = mc
			err := Execute(state)
			if err == nil {
				t.Fatalf("%s with unknown seq: Execute returned nil, want error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("%s: error %q does not mention opcode", tc.name, err.Error())
			}
			if !strings.Contains(err.Error(), "SeqType") {
				t.Errorf("%s: error %q does not mention SeqType", tc.name, err.Error())
			}
		})
	}
}

// TestBasRunningAcceptsMinusOne pins TS PlayerOps.ts:961-964 — -1 is a
// clear-sentinel that bypasses SeqTypeValid and is forwarded directly
// to SetRunAnim. Tested with an empty seq registry to confirm the -1
// branch does NOT consult Configs. (BAS_RUNNING renamed from RUNANIM in 244.)
func TestBasRunningAcceptsMinusOne(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "bas_running_clear",
		Opcodes: []Opcode{
			OpPushConstantInt, OpBasRunning, OpReturn,
		},
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{seqs: map[int]*objtype.SeqType{}}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastRunAnim != -1 {
		t.Errorf("BAS_RUNNING -1: got %d, want -1", mp.lastRunAnim)
	}
}

// TestBasRunningRejectsInvalidSeq pins TS PlayerOps.ts:965 — any non-(-1)
// seq id is validated against SeqTypeValid and aborts the script on
// miss. (BAS_RUNNING renamed from RUNANIM in 244.)
func TestBasRunningRejectsInvalidSeq(t *testing.T) {
	mp := &mockPlayer{lastRunAnim: -2} // sentinel to detect spurious write
	sf := &ScriptFile{
		Name: "bas_running_invalid",
		Opcodes: []Opcode{
			OpPushConstantInt, OpBasRunning, OpReturn,
		},
		IntOperands:      []int32{99, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{seqs: map[int]*objtype.SeqType{}}
	err := Execute(state)
	if err == nil {
		t.Fatal("BAS_RUNNING with unknown seq: Execute returned nil, want error")
	}
	if !strings.Contains(err.Error(), "BAS_RUNNING") {
		t.Errorf("error %q does not mention BAS_RUNNING", err.Error())
	}
	if mp.lastRunAnim != -2 {
		t.Errorf("SetRunAnim should not be called on validation failure (lastRunAnim=%d)", mp.lastRunAnim)
	}
}

// TestPRunDispatch verifies the P_RUN handler (opcode 2085) writes the
// popped int to SetRun and mirrors it to the varp id returned by
// RunVarpID() (the cache-resolved run-mode varp). Mirrors
// TS PlayerOps.ts:1204-1209. NAI-117 T1.
func TestPRunDispatch(t *testing.T) {
	for _, v := range []int{0, 1} {
		t.Run(fmt.Sprintf("v=%d", v), func(t *testing.T) {
			mp := &mockPlayer{lastSetRun: -1, runVarpID: 173, varps: map[int]int32{}}
			sf := &ScriptFile{
				Name: "p_run_dispatch",
				Opcodes: []Opcode{
					OpPushConstantInt, OpPRun, OpReturn,
				},
				IntOperands:      []int32{int32(v), 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, mp, true, nil, nil) // protect=true (P_RUN gate)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if mp.lastSetRun != v {
				t.Errorf("SetRun: got %d, want %d", mp.lastSetRun, v)
			}
			if got := mp.varps[173]; int(got) != v {
				t.Errorf("varp[173]: got %d, want %d", got, v)
			}
		})
	}
}

// TestRunEnergyDispatch verifies the RUNENERGY handler (opcode 2096)
// pushes the active player's runenergy onto the int stack. Mirrors TS
// PlayerOps.ts:1175-1178. NAI-117 T2.
func TestRunEnergyDispatch(t *testing.T) {
	mp := &mockPlayer{runenergyValue: 7250}
	sf := &ScriptFile{
		Name: "runenergy_dispatch",
		Opcodes: []Opcode{
			OpRunEnergy, OpReturn,
		},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7250 {
		t.Errorf("RUNENERGY: got %d, want 7250", got)
	}
}

// -- SAY tests -----------------------------------------------------------

// TestSay pins OpSay's body: pop the top-of-stack string and pass it to
// ActivePlayer.Say as a []byte. Mirrors TS PlayerOps.ts:462-464.
// NAI-160 T1.
func TestSay(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[say,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello world", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.sayCalls); got != 1 {
		t.Fatalf("sayCalls: got %d, want 1", got)
	}
	if got, want := string(mp.sayCalls[0]), "hello world"; got != want {
		t.Errorf("sayCalls[0]: got %q, want %q", got, want)
	}
}

// TestSayEmptyString pins TS semantics that an empty bubble is legal —
// matches the doc-comment at modules/world/player_masks.go:8-11 and
// the parallel TestNpcSay convention. NAI-160 T1.
func TestSayEmptyString(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[say_empty,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.sayCalls); got != 1 {
		t.Fatalf("sayCalls: got %d, want 1", got)
	}
	if got := len(mp.sayCalls[0]); got != 0 {
		t.Errorf("sayCalls[0]: got len=%d, want 0", got)
	}
}

// -- HEADICONS_GET tests -------------------------------------------------

// TestHeadIconsGet pins OpHeadIconsGet's body: read the player's headicons
// field and push it as an int. Mirrors TS PlayerOps.ts:980-982. NAI-160 T2.
func TestHeadIconsGet(t *testing.T) {
	mp := &mockPlayer{headiconsValue: 7}
	sf := &ScriptFile{
		Name:             "[headicons_get,test]",
		Opcodes:          []Opcode{OpHeadIconsGet, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7 {
		t.Errorf("HEADICONS_GET: got %d, want 7", got)
	}
}

// -- HEADICONS_SET tests -------------------------------------------------

// TestHeadIconsSet pins OpHeadIconsSet's body: pop an int, NumberNotNull
// check, write into headicons. Mirrors TS PlayerOps.ts:984-986. NAI-160 T3.
func TestHeadIconsSet(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[headicons_set,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpHeadIconsSet, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.setHeadIconsCalls); got != 1 {
		t.Fatalf("setHeadIconsCalls: got %d, want 1", got)
	}
	if got := mp.setHeadIconsCalls[0]; got != 42 {
		t.Errorf("setHeadIconsCalls[0]: got %d, want 42", got)
	}
	if got := mp.headiconsValue; got != 42 {
		t.Errorf("headiconsValue post-set: got %d, want 42", got)
	}
}

// TestHeadIconsSetRejectsNull pins the NumberNotNull check (goscape
// checkNotNull rejects -1; matches TS NumberNotNull). NAI-160 T3.
func TestHeadIconsSetRejectsNull(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[headicons_set_null,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpHeadIconsSet, OpReturn},
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want HEADICONS_SET: input number was null(-1)")
	}
	if got := err.Error(); !strings.Contains(got, "HEADICONS_SET: input number was null(-1)") {
		t.Errorf("err: got %q, want substring 'HEADICONS_SET: input number was null(-1)'", got)
	}
	if got := len(mp.setHeadIconsCalls); got != 0 {
		t.Errorf("setHeadIconsCalls: got %d, want 0 (write must NOT happen on validation failure)", got)
	}
}

// -- Active-player-required negative tests -------------------------------

// Every handler that dereferences Self must return an error when
// Self == nil (no active player). Runs one representative handler from
// each category.
func TestHandlersRequireActivePlayer(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
	}{
		{"STAT", OpStat},
		{"STAT_BASE", OpStatBase},
		// STAT_TOTAL deleted in 244.
		{"STAT_ADD", OpStatAdd},
		{"STAT_SUB", OpStatSub},
		{"STAT_BOOST", OpStatBoost},
		{"STAT_DRAIN", OpStatDrain},
		{"STAT_HEAL", OpStatHeal},
		{"STAT_ADVANCE", OpStatAdvance},
		{"STAT_RANDOM", OpStatRandom},
		{"COORD", OpCoord},
		{"FACESQUARE", OpFaceSquare},
		{"P_TELEPORT", OpPTeleport},
		{"P_TELEJUMP", OpPTeleJump},
		{"ANIM", OpAnim},
		{"SPOTANIM_PL", OpSpotAnimPl},
		{"BAS_READYANIM", OpBasReadyAnim},
		{"BAS_TURNONSPOT", OpBasTurnOnSpot},
		{"BAS_WALK_F", OpBasWalkF},
		{"BAS_WALK_B", OpBasWalkB},
		{"BAS_WALK_L", OpBasWalkL},
		{"BAS_WALK_R", OpBasWalkR},
		{"BAS_RUNNING", OpBasRunning},
		// NAI-117 T1.
		{"P_RUN", OpPRun},
		// NAI-117 T2.
		{"RUNENERGY", OpRunEnergy},
		// NAI-160 T1.
		{"SAY", OpSay},
		// NAI-160 T2.
		{"HEADICONS_GET", OpHeadIconsGet},
		// NAI-160 T3.
		{"HEADICONS_SET", OpHeadIconsSet},
		// NAI-160 T4.
		{"P_EXACTMOVE", OpPExactMove},
		// NAI-161 T4.
		{"CLEARQUEUE", OpClearQueue},
		// NAI-161 T5.
		{"GETQUEUE", OpGetQueue},
		// NAI-162 B1.
		{"LAST_LOGIN_INFO", OpLastLoginInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := Init(newSingleOp(tc.name, tc.op), nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Fatalf("%s with nil Self: Execute returned nil, want error", tc.name)
			}
		})
	}
}

// -- Small helpers -------------------------------------------------------

// itoa without importing strconv at test scope; just for sub-test names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestPStopAction(t *testing.T) {
	sf := &ScriptFile{
		Name:             "stop",
		Opcodes:          []Opcode{OpPStopAction, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("stopActionCalls: got %d, want 1", mp.stopActionCalls)
	}
}

func TestPClearPendingAction(t *testing.T) {
	sf := &ScriptFile{
		Name:             "clear",
		Opcodes:          []Opcode{OpPClearPendingAction, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.clearPendingActionCalls != 1 {
		t.Errorf("clearPendingActionCalls: got %d, want 1", mp.clearPendingActionCalls)
	}
}

// -- P_APRANGE tests -----------------------------------------------------

func TestHandlePApRangeSetsBothFields(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Pointers: PtrProtectedActivePlayer,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(5)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}

	if fake.lastApRange != 5 {
		t.Errorf("lastApRange: got %d, want 5", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true")
	}
	if fake.setApRangeCalls != 1 {
		t.Errorf("setApRangeCalls: got %d, want 1", fake.setApRangeCalls)
	}
}

func TestHandlePApRangeRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
	}
	s.PushInt(5)

	err := handlePApRange(s)
	if err == nil {
		t.Fatal("handlePApRange: expected error, got nil")
	}
	if got := err.Error(); got != "P_APRANGE: no active player" {
		t.Errorf("error: got %q, want \"P_APRANGE: no active player\"", got)
	}
}

func TestHandlePApRangeAcceptsNegative(t *testing.T) {
	// NAI-24 Bundle 1: TS NumberNotNull only rejects -1; other negatives
	// are accepted. Use -2 to verify negative-but-not-null still passes.
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Pointers: PtrProtectedActivePlayer,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(-2)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}
	if fake.lastApRange != -2 {
		t.Errorf("lastApRange: got %d, want -2", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true even for negative apRange")
	}
}

func TestHandlePApRangeAcceptsZero(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Pointers: PtrProtectedActivePlayer,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(0)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}
	if fake.lastApRange != 0 {
		t.Errorf("lastApRange: got %d, want 0", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true for zero apRange")
	}
}

// -- S6v: p_op* tests ----------------------------------------------------

// TestPOpLocAnchorsOnActiveLoc — happy path for P_OPLOC.
func TestPOpLocAnchorsOnActiveLoc(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	lt.Op = make([]string, 5)
	lt.Op[2] = "Operate" // op=3 → index 2
	mc.locs[42] = lt
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptLoc call, got %d", len(mp.lastSetInteractionScriptLoc))
	}
	got := mp.lastSetInteractionScriptLoc[0]
	if got.Loc != loc || got.Op != 3 {
		t.Errorf("args: got %+v, want {Loc:%p, Op:3}", got, loc)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction calls: got %d, want 1", mp.stopActionCalls)
	}
}

// TestPOpLocQueueWaypointOutOfRange pins TS PlayerOps.ts:396-398:
// when inOperableDistance returns false, P_OPLOC queues a waypoint to
// the active loc's coords before anchoring the script interaction.
func TestPOpLocQueueWaypointOutOfRange(t *testing.T) {
	mp := &mockPlayer{inOperableDistanceValue: false}
	loc := &mockActiveLoc{locType: 42, x: 3200, z: 3201, level: 0}

	sf := &ScriptFile{
		Name:             "p_op_loc_far",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	lt.Op = make([]string, 5)
	lt.Op[2] = "Operate"
	mc.locs[42] = lt
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.inOperableDistanceCalls) != 1 || mp.inOperableDistanceCalls[0] != loc {
		t.Fatalf("InOperableDistance: got %+v, want 1 call with active loc", mp.inOperableDistanceCalls)
	}
	want := struct{ x, z int }{x: 3200, z: 3201}
	if len(mp.queueWaypointCalls) != 1 || mp.queueWaypointCalls[0] != want {
		t.Errorf("QueueWaypoint args: got %+v, want [{3200, 3201}]", mp.queueWaypointCalls)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction calls: got %d, want 1", mp.stopActionCalls)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 || mp.lastSetInteractionScriptLoc[0].Op != 3 {
		t.Errorf("SetInteractionScriptLoc: got %+v, want 1 call op=3", mp.lastSetInteractionScriptLoc)
	}
}

// TestPOpLocSkipsQueueWaypointInRange pins TS PlayerOps.ts:396-398:
// when inOperableDistance returns true, P_OPLOC anchors the
// interaction without queueing a redundant waypoint.
func TestPOpLocSkipsQueueWaypointInRange(t *testing.T) {
	mp := &mockPlayer{inOperableDistanceValue: true}
	loc := &mockActiveLoc{locType: 42, x: 3200, z: 3201, level: 0}

	sf := &ScriptFile{
		Name:             "p_op_loc_close",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	lt.Op = make([]string, 5)
	lt.Op[2] = "Operate"
	mc.locs[42] = lt
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.inOperableDistanceCalls) != 1 {
		t.Fatalf("InOperableDistance: got %d calls, want 1", len(mp.inOperableDistanceCalls))
	}
	if len(mp.queueWaypointCalls) != 0 {
		t.Errorf("QueueWaypoint: got %+v, want no calls", mp.queueWaypointCalls)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction calls: got %d, want 1", mp.stopActionCalls)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 {
		t.Errorf("SetInteractionScriptLoc: want 1 call, got %d", len(mp.lastSetInteractionScriptLoc))
	}
}

// TestPOpLocNoActivePlayerErrors — requireActivePlayer gate fires.
func TestPOpLocNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("p_op_loc_no_player", OpPOpLoc)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active player" {
		t.Errorf("expected 'P_OPLOC: no active player', got %v", err)
	}
}

// TestPOpLocNoActiveLocErrors — nil ActiveLoc.
func TestPOpLocNoActiveLocErrors(t *testing.T) {
	mp := &mockPlayer{}

	sf := newSingleOp("p_op_loc_no_loc", OpPOpLoc)
	state := Init(sf, mp, true, nil, nil)
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active loc" {
		t.Errorf("expected 'P_OPLOC: no active loc', got %v", err)
	}
}

// TestPOpLocInvalidOpErrors — op out of [1,5] range.
func TestPOpLocInvalidOpErrors(t *testing.T) {
	// NAI-24 Bundle 1: -1 is now caught by the NumberNotNull wrap (TS
	// PlayerOps.ts:387) before reaching the [1..5] range check; covered
	// separately by TestHandlePOpLocNullRejected. Other out-of-range
	// values still produce "invalid op".
	for _, op := range []int32{0, 6, 100} {
		mp := &mockPlayer{}
		loc := &mockActiveLoc{locType: 42}

		sf := &ScriptFile{
			Name:             "p_op_loc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, true, nil, nil)
		state.ActiveLoc = loc
		state.Pointers |= PtrActiveLoc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
			continue
		}
		wantPrefix := "P_OPLOC: invalid op"
		if len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("op=%d: expected error starting with %q, got %v", op, wantPrefix, err)
		}
	}
}

// TestPOpNpcAnchorsOnActiveNpc — happy path for P_OPNPC.
func TestPOpNpcAnchorsOnActiveNpc(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}

	sf := &ScriptFile{
		Name:             "p_op_npc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptNpc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptNpc call, got %d", len(mp.lastSetInteractionScriptNpc))
	}
	got := mp.lastSetInteractionScriptNpc[0]
	if got.Npc != npc || got.Op != 2 {
		t.Errorf("args: got %+v, want {Npc:%p, Op:2}", got, npc)
	}
}

// TestPOpNpcInvalidOpErrors — op out of range.
func TestPOpNpcInvalidOpErrors(t *testing.T) {
	for _, op := range []int32{0, 6} {
		mp := &mockPlayer{}
		npc := &mockActiveNpc{typeId: 7}

		sf := &ScriptFile{
			Name:             "p_op_npc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, true, nil, nil)
		state.ActiveNpc = npc
		state.Pointers |= PtrActiveNpc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
		}
	}
}

// TestHandleP_OpNpcMissingOpEntryShortCircuits pins TS PlayerOps.ts:408-411
// `if (!npcType.op || !npcType.op[type]) { return; }`. NpcType id 0 in
// newTestConfigs has Op = {"Talk-to", "", "Pickpocket"}; op=2 selects the
// empty middle slot → silent skip (no StopAction, no SetInteractionScriptNpc).
func TestHandleP_OpNpcMissingOpEntryShortCircuits(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	s.ActiveNpc = &mockActiveNpc{typeId: 0} // "man" — Op[1] is ""
	s.Pointers |= PtrActiveNpc
	s.Configs = newTestConfigs()

	s.PushInt(2) // op = 2 → Op[1] empty
	if err := handleP_OpNpc(s); err != nil {
		t.Fatalf("P_OPNPC missing op entry: expected nil-error short-circuit, got %v", err)
	}
	if pl.stopActionCalls != 0 {
		t.Errorf("P_OPNPC missing op entry: expected 0 StopAction calls, got %d", pl.stopActionCalls)
	}
	if len(pl.lastSetInteractionScriptNpc) != 0 {
		t.Errorf("P_OPNPC missing op entry: expected 0 SetInteractionScriptNpc calls, got %d", len(pl.lastSetInteractionScriptNpc))
	}
}

// TestHandleP_OpNpcOpPresentFires verifies the converse — when the Op slot is
// populated, the interaction fires as before.
func TestHandleP_OpNpcOpPresentFires(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	npc := &mockActiveNpc{typeId: 0} // "man" — Op[0] = "Talk-to"
	s.ActiveNpc = npc
	s.Pointers |= PtrActiveNpc
	s.Configs = newTestConfigs()

	s.PushInt(1) // op = 1 → Op[0] = "Talk-to"
	if err := handleP_OpNpc(s); err != nil {
		t.Fatalf("P_OPNPC happy: unexpected error %v", err)
	}
	if pl.stopActionCalls != 1 {
		t.Errorf("P_OPNPC happy: stopActionCalls = %d, want 1", pl.stopActionCalls)
	}
	if len(pl.lastSetInteractionScriptNpc) != 1 {
		t.Fatalf("P_OPNPC happy: expected 1 SetInteractionScriptNpc call, got %d", len(pl.lastSetInteractionScriptNpc))
	}
	if got := pl.lastSetInteractionScriptNpc[0]; got.Npc != npc || got.Op != 1 {
		t.Errorf("P_OPNPC happy: got %+v, want {Npc:%p, Op:1}", got, npc)
	}
}

// TestHandleP_OpNpcNilNpcTypeShortCircuits pins TS `!npcType.op` half of the
// guard — when Configs has no entry for the active npc's type, the handler
// must short-circuit rather than panic on a nil deref.
func TestHandleP_OpNpcNilNpcTypeShortCircuits(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	s.ActiveNpc = &mockActiveNpc{typeId: 9999} // unregistered type
	s.Pointers |= PtrActiveNpc
	s.Configs = newTestConfigs()

	s.PushInt(1)
	if err := handleP_OpNpc(s); err != nil {
		t.Fatalf("P_OPNPC nil NpcType: expected nil-error short-circuit, got %v", err)
	}
	if pl.stopActionCalls != 0 {
		t.Errorf("P_OPNPC nil NpcType: expected 0 StopAction calls, got %d", pl.stopActionCalls)
	}
	if len(pl.lastSetInteractionScriptNpc) != 0 {
		t.Errorf("P_OPNPC nil NpcType: expected 0 SetInteractionScriptNpc calls, got %d", len(pl.lastSetInteractionScriptNpc))
	}
}

// TestPOpLocUnprotectedRejected verifies that a script started without
// protection (protect=false) gets an error from P_OPLOC. Matches TS
// checkedHandler(ProtectedActivePlayer, ...) semantics. Closes S6v-D1.
func TestPOpLocUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}
	sf := newSingleOp("p_op_loc_unprotected", OpPOpLoc)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: script not protected" {
		t.Errorf("expected 'P_OPLOC: script not protected', got %v", err)
	}
}

// TestHandlePOpLocMissingOpEntryShortCircuits pins TS PlayerOps.ts:391-394:
// when LocType.op[type-1] is empty (or the op slice is nil/too short), the
// handler silently returns without firing StopAction or
// SetInteractionScriptLoc. Mirrors the P_OPOBJ shape at
// PlayerOps.ts:997-999 / TestHandleP_OpObjMissingOpEntryShortCircuits.
func TestHandlePOpLocMissingOpEntryShortCircuits(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc_empty_op",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0}, // op=3 → index 2
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc

	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	lt.Op = make([]string, 5) // all slots empty → silent skip
	mc.locs[42] = lt
	state.Configs = mc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 0 {
		t.Errorf("SetInteractionScriptLoc: should not have been called, got %d", len(mp.lastSetInteractionScriptLoc))
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("StopAction: should not have been called, got %d", mp.stopActionCalls)
	}
}

// TestHandlePOpLocNilOpSliceShortCircuits pins the !locType.op branch
// (PlayerOps.ts:392): when LocType.Op is nil (never lazy-initialized),
// the handler silently returns. Mirrors the falsy-op-array half of the
// TS guard.
func TestHandlePOpLocNilOpSliceShortCircuits(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc_nil_op",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc

	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	// lt.Op left nil (never lazy-initialized)
	mc.locs[42] = lt
	state.Configs = mc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 0 {
		t.Errorf("SetInteractionScriptLoc: should not have been called, got %d", len(mp.lastSetInteractionScriptLoc))
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("StopAction: should not have been called, got %d", mp.stopActionCalls)
	}
}

// TestHandlePOpLocFiresWhenOpPresent pins the positive branch: when
// LocType.Op[op-1] is a non-empty string, StopAction + SetInteractionScriptLoc
// fire as before.
func TestHandlePOpLocFiresWhenOpPresent(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc_present",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{2, 0, 0}, // op=2 → index 1
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc

	mc := newTestConfigs()
	lt := objtype.NewLocType(42)
	lt.Op = make([]string, 5)
	lt.Op[1] = "Use"
	mc.locs[42] = lt
	state.Configs = mc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 {
		t.Fatalf("SetInteractionScriptLoc: got %d calls, want 1", len(mp.lastSetInteractionScriptLoc))
	}
	if got := mp.lastSetInteractionScriptLoc[0]; got.Loc != loc || got.Op != 2 {
		t.Errorf("args: got %+v, want {Loc:%p, Op:2}", got, loc)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction: got %d, want 1", mp.stopActionCalls)
	}
}

// TestPOpNpcUnprotectedRejected — symmetric.
func TestPOpNpcUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}
	sf := newSingleOp("p_op_npc_unprotected", OpPOpNpc)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	state.PushInt(2)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPNPC: script not protected" {
		t.Errorf("expected 'P_OPNPC: script not protected', got %v", err)
	}
}

// TestPTeleportUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_TELEPORT (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPTeleportUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_teleport_unprotected", OpPTeleport)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(123)

	err := Execute(state)
	if err == nil || err.Error() != "P_TELEPORT: script not protected" {
		t.Errorf("expected 'P_TELEPORT: script not protected', got %v", err)
	}
}

// TestPTeleJumpUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_TELEJUMP (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPTeleJumpUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_telejump_unprotected", OpPTeleJump)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(123)

	err := Execute(state)
	if err == nil || err.Error() != "P_TELEJUMP: script not protected" {
		t.Errorf("expected 'P_TELEJUMP: script not protected', got %v", err)
	}
}

// TestPRunUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Mirrors TS
// checkedHandler(ProtectedActivePlayer, ...) at PlayerOps.ts:1204.
// NAI-117 T1.
func TestPRunUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_run_unprotected", OpPRun)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(1)

	err := Execute(state)
	if err == nil || err.Error() != "P_RUN: script not protected" {
		t.Errorf("expected 'P_RUN: script not protected', got %v", err)
	}
}

// TestPApRangeUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_APRANGE (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPApRangeUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_aprange_unprotected", OpPApRange)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(5)

	err := Execute(state)
	if err == nil || err.Error() != "P_APRANGE: script not protected" {
		t.Errorf("expected 'P_APRANGE: script not protected', got %v", err)
	}
}

// TestPStopActionUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_STOPACTION (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPStopActionUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_stopaction_unprotected", OpPStopAction)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_STOPACTION: script not protected" {
		t.Errorf("expected 'P_STOPACTION: script not protected', got %v", err)
	}
}

// TestPClearPendingActionUnprotectedRejected verifies that a script started
// without protection gets the "script not protected" error. Closes S6l-D3
// for P_CLEARPENDINGACTION (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPClearPendingActionUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_clearpendingaction_unprotected", OpPClearPendingAction)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_CLEARPENDINGACTION: script not protected" {
		t.Errorf("expected 'P_CLEARPENDINGACTION: script not protected', got %v", err)
	}
}

// -- S7a FINDUID tests ---------------------------------------------------

// zoneKey indexes the byZone fixture below by (level, zoneX, zoneZ),
// matching the production-side ZonePlayers parameter shape (world coords,
// not zone indices). NAI-35-T2.
type zoneKey struct{ level, zoneX, zoneZ int }

// mockPlayerLookup resolves UIDs via a pre-seeded map. Introduced in S7a.
// NAI-35-T2 extends with byZone for the new ZonePlayers method.
type mockPlayerLookup struct {
	byUID  map[int]ActivePlayer
	byZone map[zoneKey][]ActivePlayer
	calls  int
}

func (m *mockPlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
	m.calls++
	if p, ok := m.byUID[uid]; ok {
		return p
	}
	// Mirror Server.LookupPlayerByUID: bottom-32-bits comparison so
	// callers using either the positive uint32 representation or the
	// int32-cast representation resolve to the same player. Tests that
	// seed byUID with the positive composeUID form (production-realistic)
	// keep working after the PushInt int32-cast fix landed.
	target := int32(uid)
	for storedUID, p := range m.byUID {
		if int32(storedUID) == target {
			return p
		}
	}
	return nil
}

// ZonePlayers satisfies the NAI-35-T2 PlayerLookup.ZonePlayers extension.
// Returns the slice keyed by (level, zoneX, zoneZ); nil/zero-value if
// unseeded. Mirrors the production semantics of "empty/nil slice on miss".
func (m *mockPlayerLookup) ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer {
	return m.byZone[zoneKey{level, zoneX, zoneZ}]
}

// TestPFindUID_HighBitUid_FastPathReacquire reproduces the player-
// controls regression: after the PushInt int32-cast fix landed (the
// "Someone else is fighting that." combat-bug fix), high-bit-set UIDs
// would round-trip on the script stack as their signed-int32 form
// (e.g. composeUID 0x80000001 = 2147483649 → toInt32 → -2147483647),
// but Player.UID() still returned the unsigned form. The if_button
// protected-script pattern `if (p_finduid(uid) = true) { ... }` then
// hit the fast-path comparison `s.Self.UID() == uid` with mismatched
// representations (2147483649 vs -2147483647) and fell through to the
// resync-only fallback — silently snapping toggle buttons back to
// their server-side state.
//
// TS sidesteps this in two ways: (a) toInt32 normalises both sides of
// the comparison automatically via JS bitwise-coercion semantics, and
// (b) getPlayerByUid decomposes uid into slot+hash for a
// representation-independent lookup (World.ts:1659-1673). Go needs
// explicit int32-cast normalisation at the comparison boundary.
func TestPFindUID_HighBitUid_FastPathReacquire(t *testing.T) {
	var positiveUID int = 0x80000001 // high bit set, matches composeUID output for ~50% of usernames
	self := &mockPlayer{username: "Self", uidValue: positiveUID, canAccessValue: true}

	sf := &ScriptFile{
		Name:             "pfinduid_highbit_fast_path",
		Opcodes:          []Opcode{OpUID, OpPFindUID, OpReturn},
		IntOperands:      []int32{0, 0, 0}, // PFINDUID intOperand=0 → slot 0
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	// protect=true → PtrProtectedActivePlayer set → fast-path eligible.
	state := Init(sf, self, true, nil, nil)
	// state.PlayerLookup deliberately nil — fast-path must succeed without
	// consulting the lookup. If the fast-path fails, P_FINDUID falls
	// through to the nil-lookup branch and pushes 0.

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got %v, want [1]\n"+
			"  p_finduid(uid) self-reacquire fast-path must succeed for high-bit UIDs",
			state.IntStack[:state.ISP])
	}
}

// TestPFindUID_HighBitUid_LookupPath covers the non-fast-path branch:
// state.Self is a different player than the lookup target, so the
// self-reacquire fast-path is skipped and the bug surfaces at the
// LookupPlayerByUID call. mockPlayerLookup mirrors production storage
// (positive Go int keys for high-bit UIDs); the script's PopInt-of-
// OpUID gives the int32-cast form. Production Server.LookupPlayerByUID
// has identical shape (server.go:1357 `p.uid == uid`); fixing the mock
// in lock-step is intentional — it documents that mocks must follow
// the production invariant.
func TestPFindUID_HighBitUid_LookupPath(t *testing.T) {
	var positiveUID int = 0x80000001
	target := &mockPlayer{username: "Target", uidValue: positiveUID, canAccessValue: true}
	other := &mockPlayer{username: "Other", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{positiveUID: target}}

	// Push uid manually (the value scripts would see after OpUID +
	// PushInt int32-cast): bottom 32 bits of positiveUID, sign-extended.
	pushedUID := int(int32(positiveUID))

	sf := newSingleOp("pfinduid_highbit_lookup", OpPFindUID)
	state := Init(sf, other, false, nil, nil) // Self != target; not protected → no fast-path
	state.PlayerLookup = lookup
	state.PushInt(pushedUID)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got %v, want [1]\n"+
			"  LookupPlayerByUID must find target whose stored uid round-trips through int32 to the pushed value",
			state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target after successful PFindUID", state.Self)
	}
}

// TestFindUIDFound: lookup returns a target → push 1, Self rebinds,
// PtrActivePlayer set, Protect stays false (FINDUID is unprotected).
func TestFindUIDFound(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("finduid_found", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set, pointers=%b", state.Pointers)
	}
	if state.Pointers&PtrProtectedActivePlayer != 0 {
		t.Errorf("PtrProtectedActivePlayer should remain unset for FINDUID, pointers=%b", state.Pointers)
	}
}

// TestFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("finduid_notfound", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged, got %v", state.Self)
	}
}

// TestFindUIDNoLookupConfigured: PlayerLookup nil → push 0.
// Host configurations that don't wire a lookup degrade to "not found"
// rather than erroring, matching the LAST_INT / LAST_COM precedent.
func TestFindUIDNoLookupConfigured(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig"}

	sf := newSingleOp("finduid_nolookup", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	// state.PlayerLookup left nil
	state.PushInt(1)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}

// TestFindUIDComposedUIDLookup pins NAI-113 cascade closure: with
// production-realistic composed uids (post-Server.addPlayer wiring),
// FINDUID resolves a registered other-player and rebinds Self. Pre-fix,
// every Player.uid was -1 → LookupPlayerByUID(any_uid) returned nil →
// FINDUID always pushed 0, dead code. Sentinel uid value 0xD005 =
// composeUID(0x1A, 5) = ((0x1A << 11) | 5) = 0xD005, documents the
// (username37 << 11 | slot) composed-uid shape.
func TestFindUIDComposedUIDLookup(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 1}
	target := &mockPlayer{username: "Target", uidValue: 0xD005}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{0xD005: target}}

	sf := newSingleOp("finduid_composed", OpFindUID)
	state := Init(sf, self, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(0xD005)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self should rebind to target")
	}
}

// TestPFindUIDSelfReacquire: script already runs protected on the target
// uid → push 1 with no state mutation, no lookup call (fast-path).
// Mirrors TS PlayerOps.ts:79-83.
func TestPFindUIDSelfReacquire(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_self", OpPFindUID)
	state := Init(sf, self, true, nil, nil) // protect=true
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != self {
		t.Errorf("Self should be unchanged on self-reacquire")
	}
	if lookup.calls != 0 {
		t.Errorf("fast-path should skip lookup, calls=%d", lookup.calls)
	}
	if state.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("PtrProtectedActivePlayer should remain set, pointers=%b", state.Pointers)
	}
}

// TestPFindUIDFoundCanAccess: target is reachable and CanAccess=true →
// push 1, Self rebinds, PtrProtectedActivePlayer set, PtrActivePlayer set.
func TestPFindUIDFoundCanAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: true}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_ok", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil) // protect=false initially
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set")
	}
	if state.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("PtrProtectedActivePlayer should be set after successful P_FINDUID, pointers=%b", state.Pointers)
	}
}

// TestPFindUIDFoundCannotAccess: target exists but CanAccess=false →
// push 0, Self unchanged, Protect unchanged.
func TestPFindUIDFoundCannotAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: false}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_busy", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged when CanAccess=false")
	}
	if state.Pointers&PtrProtectedActivePlayer != 0 {
		t.Errorf("PtrProtectedActivePlayer should remain unset, pointers=%b", state.Pointers)
	}
}

// TestPFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestPFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_notfound", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}

// TestPFindUIDSelfReacquireSkippedWhenUnprotected pins the inverse of
// the self-reacquire fast-path: when popped uid equals Self.UID() but
// the script is currently unprotected, P_FINDUID still consults
// PlayerLookup (no fast-path short-circuit). NAI-113 cascade context:
// pre-fix Self.UID() was always -1, so neither fast-path nor this
// branch fired meaningfully on uid match — both branches were dead
// code under the broken default. Mirrors TS PlayerOps.ts:79-83.
func TestPFindUIDSelfReacquireSkippedWhenUnprotected(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42, canAccessValue: true}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{42: self}}

	sf := newSingleOp("pfinduid_unprotected_self", OpPFindUID)
	state := Init(sf, self, false, nil, nil) // protect=false
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if lookup.calls != 1 {
		t.Errorf("lookup.calls: got %d, want 1 (fast-path must NOT fire when PtrProtectedActivePlayer unset)", lookup.calls)
	}
	if state.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("PtrProtectedActivePlayer should be set after successful unprotected lookup, pointers=%b", state.Pointers)
	}
}

// -- NAI-133 T2: FINDUID/P_FINDUID slot-1 routing --

// finduidSlotOp builds a one-instruction ScriptFile with the requested
// intOperand value (0 or 1). Sister to newSingleOp which always uses 0.
func finduidSlotOp(name string, op Opcode, operand int32) *ScriptFile {
	return &ScriptFile{
		Name:             name,
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// TestFindUID_Slot1_BindsSelf2 — operand=1, lookup hits → Self2 set,
// PtrActivePlayer2 set, Self UNTOUCHED. NAI-133 T2 closes the latent
// `.finduid` clobber bug.
func TestFindUID_Slot1_BindsSelf2(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("finduid_slot1", OpFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be UNCHANGED on slot-1 routing, got %v", state.Self)
	}
	if state.Self2 != target {
		t.Errorf("Self2: got %v, want target", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("PtrActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
	// Slot-0 isolation: Init set PtrActivePlayer for non-nil self; slot-1
	// routing must not clear it.
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer (slot-0) should remain set after slot-1 routing, pointers=%b", state.Pointers)
	}
}

// TestFindUID_Slot1_LookupMiss — operand=1, lookup miss → push 0,
// no state change.
func TestFindUID_Slot1_LookupMiss(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("finduid_slot1_miss", OpFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil on miss, got %v", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 != 0 {
		t.Errorf("PtrActivePlayer2 should remain unset on miss, pointers=%b", state.Pointers)
	}
}

// TestFindUID_InvalidOperand_Errors — operand=2 → error.
func TestFindUID_InvalidOperand_Errors(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("finduid_bad", OpFindUID, 2)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("expected error on intOperand=2, got nil")
	}
	if !strings.Contains(err.Error(), "FINDUID: invalid intOperand 2") {
		t.Errorf("err message: got %q, want containing %q", err.Error(), "FINDUID: invalid intOperand 2")
	}
}

// TestPFindUID_Slot1_Success — operand=1, lookup hits + CanAccess=true →
// Self2 set, PtrActivePlayer2 + PtrProtectedActivePlayer2 set, push 1.
func TestPFindUID_Slot1_Success(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: true}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("pfinduid_slot1", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self UNCHANGED: got %v, want %v", state.Self, origSelf)
	}
	if state.Self2 != target {
		t.Errorf("Self2: got %v, want target", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("PtrActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
	if state.Pointers&PtrProtectedActivePlayer2 == 0 {
		t.Errorf("PtrProtectedActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
	// Slot-0 isolation: Init was called with protect=false, so the slot-0
	// protect flag must NOT be set as a side effect of slot-1 P_FINDUID.
	if state.Pointers&PtrProtectedActivePlayer != 0 {
		t.Errorf("PtrProtectedActivePlayer (slot-0) should remain UNSET on slot-1 routing, pointers=%b", state.Pointers)
	}
}

// TestPFindUID_Slot1_SelfReacquire — slot-1 fast-path: Self2 already
// bound + PtrProtectedActivePlayer2 set + popped uid == Self2.UID() →
// push 1, no state mutation, no lookup call.
func TestPFindUID_Slot1_SelfReacquire(t *testing.T) {
	self2 := &mockPlayer{username: "Self2", uidValue: 42}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_slot1_reacquire", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2 | PtrProtectedActivePlayer2
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self2 != self2 {
		t.Errorf("Self2 should remain unchanged on fast-path")
	}
	if lookup.calls != 0 {
		t.Errorf("fast-path should skip lookup, calls=%d", lookup.calls)
	}
}

// TestPFindUID_Slot0_NoFastPathWhenSlot1Protected — only the matching
// slot's protect flag triggers the fast-path. Slot-0 P_FINDUID with
// PtrProtectedActivePlayer2 set (but PtrProtectedActivePlayer UNSET)
// must NOT fast-path; it must perform a real lookup.
func TestPFindUID_Slot0_NoFastPathWhenSlot1Protected(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42, canAccessValue: true}
	target := &mockPlayer{username: "Target", uidValue: 42, canAccessValue: true}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{42: target}}

	sf := finduidSlotOp("pfinduid_slot0_no_cross", OpPFindUID, 0)
	state := Init(sf, self, false, nil, nil)    // protect=false: slot-0 flag UNSET
	state.Pointers |= PtrProtectedActivePlayer2 // slot-1 protected (irrelevant)
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if lookup.calls != 1 {
		t.Errorf("expected real lookup, calls=%d (fast-path leaked from slot-1)", lookup.calls)
	}
	// Slot-0 protect flag set after success.
	if state.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("PtrProtectedActivePlayer should be set after success, pointers=%b", state.Pointers)
	}
}

// TestPFindUID_Slot1_LookupMiss — operand=1, lookup miss → push 0.
func TestPFindUID_Slot1_LookupMiss(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_slot1_miss", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil on miss")
	}
	if state.Pointers&PtrProtectedActivePlayer2 != 0 {
		t.Errorf("PtrProtectedActivePlayer2 should remain unset on miss")
	}
}

// TestPFindUID_Slot1_CanAccessFalse — operand=1, lookup hits but
// CanAccess=false → push 0, no state change.
func TestPFindUID_Slot1_CanAccessFalse(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: false}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("pfinduid_slot1_no_access", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil when CanAccess=false")
	}
}

// TestPFindUID_InvalidOperand_Errors — operand=-1 → error.
func TestPFindUID_InvalidOperand_Errors(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_bad", OpPFindUID, -1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("expected error on intOperand=-1, got nil")
	}
	if !strings.Contains(err.Error(), "P_FINDUID: invalid intOperand -1") {
		t.Errorf("err message: got %q", err.Error())
	}
}

// -- S7b: checkNotNull + handlePAnimProtect tests -------------------------

// TestCheckNotNull validates the shared NumberNotNull helper.
// Mirrors TS ScriptValidators.ts:36-41.
func TestCheckNotNull(t *testing.T) {
	cases := []struct {
		name    string
		v       int
		wantErr bool
	}{
		{"null sentinel", -1, true},
		{"zero", 0, false},
		{"positive", 1, false},
		{"min int32", math.MinInt32, false},
		{"max int32", math.MaxInt32, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkNotNull(tc.v, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkNotNull(%d): want error, got nil", tc.v)
				}
				if !strings.Contains(err.Error(), "OP: input number was null(-1)") {
					t.Errorf("error message: got %q, want contains %q", err.Error(), "OP: input number was null(-1)")
				}
			} else {
				if err != nil {
					t.Fatalf("checkNotNull(%d): want nil, got %v", tc.v, err)
				}
			}
		})
	}
}

// TestCheckSkinColour_Range pins the [0, 7] inclusive range check.
// Mirrors TS SkinColourValid (ScriptValidators.ts:137) —
// ScriptInputRangeValidator(0, 7, 'SkinColour').
func TestCheckSkinColour_Range(t *testing.T) {
	for _, v := range []int{0, 1, 4, 7} {
		if err := checkSkinColour(v, "TEST_OP"); err != nil {
			t.Errorf("checkSkinColour(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 8, 100, math.MinInt} {
		err := checkSkinColour(v, "TEST_OP")
		if err == nil {
			t.Errorf("checkSkinColour(%d): want error, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "TEST_OP") {
			t.Errorf("checkSkinColour(%d): error %q missing op name TEST_OP", v, err)
		}
	}
}

// TestCheckGender_Range pins the [0, 1] inclusive range check.
// Mirrors TS GenderValid (ScriptValidators.ts:136) —
// ScriptInputRangeValidator(0, 1, 'Gender').
func TestCheckGender_Range(t *testing.T) {
	for _, v := range []int{0, 1} {
		if err := checkGender(v, "TEST_OP"); err != nil {
			t.Errorf("checkGender(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 2, 100, math.MinInt, math.MaxInt} {
		err := checkGender(v, "TEST_OP")
		if err == nil {
			t.Errorf("checkGender(%d): want error, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "TEST_OP") {
			t.Errorf("checkGender(%d): error %q missing op name TEST_OP", v, err)
		}
	}
}

// TestPAnimProtectHappyPathZero — protect=true, push 0 → no error,
// animProtectValue set to 0.
func TestPAnimProtectHappyPathZero(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_zero", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(0)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.animProtectValue != 0 {
		t.Errorf("animProtectValue: got %d, want 0", player.animProtectValue)
	}
	if state.Self != player {
		t.Errorf("Self should be unchanged")
	}
}

// TestPAnimProtectHappyPathNonzero — protect=true, push 1 → no error,
// animProtectValue set to 1.
func TestPAnimProtectHappyPathNonzero(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_nonzero", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(1)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.animProtectValue != 1 {
		t.Errorf("animProtectValue: got %d, want 1", player.animProtectValue)
	}
}

// TestPAnimProtectNullRejected — protect=true, push -1 → error containing
// "P_ANIMPROTECT: input number was null(-1)"; animProtectValue unchanged.
func TestPAnimProtectNullRejected(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_null", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_ANIMPROTECT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// TestPAnimProtectNotProtected — protect=false, push 0 → error containing
// "P_ANIMPROTECT: script not protected"; animProtectValue unchanged.
func TestPAnimProtectNotProtected(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_notprotected", OpPAnimProtect)
	state := Init(sf, player, false, nil, nil) // protect=false
	state.PushInt(0)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_ANIMPROTECT: script not protected"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// TestPAnimProtectNoActivePlayer — Self=nil → error from requireActivePlayer
// chain containing "P_ANIMPROTECT"; animProtectValue unchanged.
func TestPAnimProtectNoActivePlayer(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel (not wired into state)
	sf := newSingleOp("panimprotect_noactive", OpPAnimProtect)
	state := Init(sf, nil, true, nil, nil) // Self=nil
	state.PushInt(0)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	if !strings.Contains(err.Error(), "P_ANIMPROTECT") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "P_ANIMPROTECT")
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// -- S7c: checkInvType + handleBuildAppearance tests ----------------------

// TestCheckInvType validates the state-aware InvType validator.
// Mirrors TS InvTypeValid (ScriptValidators.ts:122). Both the range check
// and the registry-present check collapse into a single Configs.InvType
// lookup per the Configs interface contract.
func TestCheckInvType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      5,
			setup:   func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{5: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkInvType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkInvType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkInvType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

// TestCheckIdkType validates the state-aware IdkType validator.
// Mirrors TS IDKTypeValid (ScriptValidators.ts:124). Both the range check
// and the registry-present check collapse into a single Configs.IdkType
// lookup per the Configs interface contract.
func TestCheckIdkType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      0,
			setup:   func() *mockConfigs { return &mockConfigs{idks: map[int]*objtype.IdkType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{idks: map[int]*objtype.IdkType{}} },
			wantErr:   true,
			wantSubst: "OP: no IdkType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{idks: map[int]*objtype.IdkType{}} },
			wantErr:   true,
			wantSubst: "OP: no IdkType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no IdkType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkIdkType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkIdkType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkIdkType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

// TestBuildAppearanceHappyPath — Self != nil, Configs.invs has id=5,
// push 5 → no error; lastAppearanceInv == 5, appearanceInvCalls == 1,
// appearanceMaskSet == true.
func TestBuildAppearanceHappyPath(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_happy", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{5: {}}}
	state.PushInt(5)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.lastAppearanceInv != 5 {
		t.Errorf("lastAppearanceInv: got %d, want 5", player.lastAppearanceInv)
	}
	if player.appearanceInvCalls != 1 {
		t.Errorf("appearanceInvCalls: got %d, want 1", player.appearanceInvCalls)
	}
	if !player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got false, want true")
	}
}

// TestBuildAppearanceInvalidInvRejected — Self != nil, Configs.invs empty,
// push 999 → error message contains "BUILDAPPEARANCE: no InvType with
// value (999) found"; appearanceInvCalls == 0, appearanceMaskSet == false.
func TestBuildAppearanceInvalidInvRejected(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_invalid", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{}}
	state.PushInt(999)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "BUILDAPPEARANCE: no InvType with value (999) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	if player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got true, want false")
	}
}

// TestBuildAppearanceNegativeIdRejected — Self != nil, Configs.invs empty,
// push -1 → error; setter not called. Covers the TS `input >= 0` branch
// via nil lookup since goscape collapses both checks.
func TestBuildAppearanceNegativeIdRejected(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_negative", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{}}
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "BUILDAPPEARANCE: no InvType with value (-1) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	if player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got true, want false")
	}
}

// TestBuildAppearanceNoActivePlayer — Self=nil → error from
// requireActivePlayer chain containing "BUILDAPPEARANCE". The gate runs
// before PopInt so the int stack should retain the pushed value.
func TestBuildAppearanceNoActivePlayer(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_noactive", OpBuildAppearance)
	state := Init(sf, nil, false, nil, nil) // Self=nil
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{5: {}}}
	state.PushInt(5)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	if !strings.Contains(err.Error(), "BUILDAPPEARANCE") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "BUILDAPPEARANCE")
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	// Gate runs before PopInt — the pushed value should still be on the stack.
	if got := state.PopInt(); got != 5 {
		t.Errorf("int stack top: got %d, want 5 (gate should run before PopInt)", got)
	}
}

// TestBuildAppearanceNotProtectedOK — Protect=false, Self != nil,
// Configs.invs has id=3, push 3 → no error. BUILDAPPEARANCE uses
// ActivePlayer (not ProtectedActivePlayer). Gate-regression guard:
// catches a future edit that copy-pastes requireProtectedActivePlayer.
func TestBuildAppearanceNotProtectedOK(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_unprotected", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil) // protect=false
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{3: {}}}
	state.PushInt(3)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: unexpected error %v (BUILDAPPEARANCE should not require Protect)", err)
	}
	if player.lastAppearanceInv != 3 {
		t.Errorf("lastAppearanceInv: got %d, want 3", player.lastAppearanceInv)
	}
	if player.appearanceInvCalls != 1 {
		t.Errorf("appearanceInvCalls: got %d, want 1", player.appearanceInvCalls)
	}
	if !player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got false, want true")
	}
}

// -- S7e: handleAllowDesign tests ------------------------------------------

// TestAllowDesign is a table-driven test covering the three value-coercion
// paths (5.1 true, 5.2 false, 5.3 non-one coerces to false). All three
// exercise the happy path: ActivePlayer set, valid int (not -1), setter
// called exactly once. Pins the exact v==1 coercion shape — a truthy
// v!=0 mistake would fail the 5.3 sub-case.
func TestAllowDesign(t *testing.T) {
	cases := []struct {
		name    string
		push    int
		wantVal bool
	}{
		{"True", 1, true},
		{"False", 0, false},
		{"NonOneCoercesToFalse_2", 2, false},
		{"NonOneCoercesToFalse_neg2", -2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			player := &mockPlayer{}
			sf := newSingleOp("allowdesign_"+tc.name, OpAllowDesign)
			state := Init(sf, player, false, nil, nil)
			state.PushInt(tc.push)

			if err := Execute(state); err != nil {
				t.Fatalf("Execute: unexpected error %v", err)
			}
			if player.allowDesignValue != tc.wantVal {
				t.Errorf("allowDesignValue: got %v, want %v", player.allowDesignValue, tc.wantVal)
			}
			if player.allowDesignCalls != 1 {
				t.Errorf("allowDesignCalls: got %d, want 1", player.allowDesignCalls)
			}
		})
	}
}

// TestAllowDesignNullInput — push -1 → checkNotNull rejects with
// "input number was null(-1)". Setter must NOT be called (S7e §5.4).
func TestAllowDesignNullInput(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("allowdesign_null", OpAllowDesign)
	state := Init(sf, player, false, nil, nil)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for null input, got nil")
	}
	if !strings.Contains(err.Error(), "input number was null(-1)") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "input number was null(-1)")
	}
	if player.allowDesignCalls != 0 {
		t.Errorf("allowDesignCalls: got %d, want 0 (setter must not be called on validator failure)", player.allowDesignCalls)
	}
}

// TestAllowDesignRequiresActivePlayer — Self=nil → error from
// requireActivePlayer containing "no active player". Setter must NOT
// be called (S7e §5.5). Gate is ActivePlayer (not Protected) — mirrors
// TestBuildAppearanceNoActivePlayer structure.
func TestAllowDesignRequiresActivePlayer(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("allowdesign_noactive", OpAllowDesign)
	state := Init(sf, nil, false, nil, nil) // Self=nil
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for missing active player, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "no active player")
	}
	if player.allowDesignCalls != 0 {
		t.Errorf("allowDesignCalls: got %d, want 0 (setter must not be called when gate fails)", player.allowDesignCalls)
	}
}

func TestCheckStringNotNullEmpty(t *testing.T) {
	err := checkStringNotNull("", "MIDI_SONG")
	if err == nil {
		t.Fatal("empty string: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error message %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestCheckStringNotNullNonEmpty(t *testing.T) {
	if err := checkStringNotNull("harmony1", "MIDI_SONG"); err != nil {
		t.Errorf("non-empty string: want nil, got %v", err)
	}
}

func TestMidiSongHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 1 {
		t.Fatalf("playSongCalls: got %d, want 1", len(mp.playSongCalls))
	}
	if mp.playSongCalls[0].name != "harmony1" {
		t.Errorf("playSongCalls[0].name: got %q, want %q", mp.playSongCalls[0].name, "harmony1")
	}
}

func TestMidiSongLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 0 {
		t.Errorf("lowMemory=true: playSongCalls=%d, want 0", len(mp.playSongCalls))
	}
}

func TestMidiSongNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestMidiSongNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0, // PtrActivePlayer unset
	}
	s.PushString("harmony1")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: no active player")
	}
}

func TestMidiJingleHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	// Pop order in handler: delay first (top-of-stack), then name.
	// Push order: name (deepest), delay (topmost).
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 1 {
		t.Fatalf("playJingleCalls: got %d, want 1", len(mp.playJingleCalls))
	}
	if mp.playJingleCalls[0].delay != 3 || mp.playJingleCalls[0].name != "fanfare" {
		t.Errorf("playJingleCalls[0]: got {delay:%d, name:%q}, want {delay:3, name:\"fanfare\"}",
			mp.playJingleCalls[0].delay, mp.playJingleCalls[0].name)
	}
}

func TestMidiJingleLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 0 {
		t.Errorf("lowMemory=true: playJingleCalls=%d, want 0", len(mp.playJingleCalls))
	}
}

func TestMidiJingleNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input string was null")
	}
}

func TestMidiJingleNullDelayRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(-1)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("delay=-1: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input number was null(-1)") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input number was null(-1)")
	}
}

func TestMidiJingleNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}
	s.PushString("fanfare")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: no active player")
	}
}

// -- NAI-24 Bundle 1: NumberNotNull audit null-pin tests -----------------
//
// Each test below corresponds to a popInt site in handlers_player.go where
// the TS counterpart (PlayerOps.ts) wraps with check(..., NumberNotNull).
// A value of -1 must be rejected before any side-effect occurs. Tests
// follow the TestHandle<OpName>NullRejected naming convention from
// handlers_interface_test.go.

// TestHandleStatAddNullRejected pins STAT_ADD: TS wraps both constant and
// percent with NumberNotNull (PlayerOps.ts:505-506). Stat id is wrapped
// with PlayerStatValid (separate gate via checkStatID); only constant and
// percent get the NumberNotNull pin here.
func TestHandleStatAddNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_ADD: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_ADD: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_add_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // stat id (bottom)
					OpPushConstantInt, // constant
					OpPushConstantInt, // percent (top)
					OpStatAdd,
					OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatSubNullRejected pins STAT_SUB: TS wraps both constant and
// percent with NumberNotNull (PlayerOps.ts:525-526).
func TestHandleStatSubNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_SUB: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_SUB: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_sub_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatSub, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatBoostNullRejected pins STAT_BOOST: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:542-543).
func TestHandleStatBoostNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_BOOST: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_BOOST: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_boost_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatBoost, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatDrainNullRejected pins STAT_DRAIN: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:565-566).
func TestHandleStatDrainNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_DRAIN: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_DRAIN: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_drain_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatDrain, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatHealNullRejected pins STAT_HEAL: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:600-601).
func TestHandleStatHealNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_HEAL: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_HEAL: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_heal_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatHeal, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatAdvanceNullRejected pins STAT_ADVANCE: TS wraps BOTH stat
// and xp with NumberNotNull (PlayerOps.ts:762-763) — NOT PlayerStatValid
// for stat (this is a TS asymmetry vs. sibling stat ops). Both ints are
// pinned here.
func TestHandleStatAdvanceNullRejected(t *testing.T) {
	tests := []struct {
		name       string
		statID, xp int32
		wantSubstr string
	}{
		{
			name:       "null_stat",
			statID:     -1,
			xp:         100,
			wantSubstr: "STAT_ADVANCE: input number was null(-1)",
		},
		{
			name:       "null_xp",
			statID:     2,
			xp:         -1,
			wantSubstr: "STAT_ADVANCE: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_advance_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // stat id (bottom)
					OpPushConstantInt, // xp (top)
					OpStatAdvance,
					OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.xp, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.addXPCalls) != 0 {
				t.Errorf("addXPCalls: should not have been called, got %d", len(mp.addXPCalls))
			}
		})
	}
}

// TestHandleSpotAnimPlNullRejected pins SPOTANIM_PL: TS wraps delay (top
// of stack) with NumberNotNull (PlayerOps.ts:589). height and spotanim
// are NOT wrapped; only delay is pinned here.
func TestHandleSpotAnimPlNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "spotanim_pl_null_delay",
		Opcodes: []Opcode{
			OpPushConstantInt, // spotanim (bottom)
			OpPushConstantInt, // height
			OpPushConstantInt, // delay (top) = -1
			OpSpotAnimPl,
			OpReturn,
		},
		IntOperands: []int32{100, 0, -1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "SPOTANIM_PL: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.playSpotAnimCalls != 0 {
		t.Errorf("playSpotAnimCalls: should not have been called, got %d", mp.playSpotAnimCalls)
	}
}

// TestHandlePApRangeNullRejected pins P_APRANGE: TS wraps with
// NumberNotNull (PlayerOps.ts:353).
func TestHandlePApRangeNullRejected(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Pointers: PtrProtectedActivePlayer,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(-1)

	err := handlePApRange(s)
	if err == nil {
		t.Fatal("handlePApRange: want error for n=-1, got nil")
	}
	want := "P_APRANGE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if fake.setApRangeCalls != 0 {
		t.Errorf("setApRangeCalls: should not have been called, got %d", fake.setApRangeCalls)
	}
}

// TestHandlePOpLocNullRejected pins P_OPLOC: TS wraps op with
// NumberNotNull (PlayerOps.ts:387). The wrap fires before the [1..5]
// range check, so -1 produces the NumberNotNull error.
func TestHandlePOpLocNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}
	sf := &ScriptFile{
		Name:        "p_op_loc_null",
		Opcodes:     []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for op=-1, got nil")
	}
	want := "P_OPLOC: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastSetInteractionScriptLoc) != 0 {
		t.Errorf("lastSetInteractionScriptLoc: should not have been called, got %d", len(mp.lastSetInteractionScriptLoc))
	}
}

// TestHandlePOpNpcNullRejected pins P_OPNPC: TS wraps op with
// NumberNotNull (PlayerOps.ts:404).
func TestHandlePOpNpcNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}
	sf := &ScriptFile{
		Name:        "p_op_npc_null",
		Opcodes:     []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for op=-1, got nil")
	}
	want := "P_OPNPC: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastSetInteractionScriptNpc) != 0 {
		t.Errorf("lastSetInteractionScriptNpc: should not have been called, got %d", len(mp.lastSetInteractionScriptNpc))
	}
}

// --- NAI-35-T4: HUNTALL handler tests ----------------------------------

// newHuntAllState pushes (coord, distance, huntvis) — popInts(3) order
// matching TS PlayerOps.ts:1215-1223. Mirrors handlers_npc_test.go's
// newNpcHuntAllState convention.
func newHuntAllState(t *testing.T, coord, distance, huntvis int, lookup *mockPlayerLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if lookup != nil {
		s.PlayerLookup = lookup
	}
	s.PushInt(coord)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestHandleHuntAll_StoresHuntAllPlayerIterator(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, objtype.HuntVisLineOfSight, &mockPlayerLookup{})
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	it, ok := s.huntIterator.(*PlayerIterator)
	if !ok || it == nil {
		t.Fatal("huntIterator should hold a *PlayerIterator after HUNTALL")
	}
	if it.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %v, want PlayerIteratorHuntAll", it.mode)
	}
	if it.huntvis != objtype.HuntVisLineOfSight {
		t.Errorf("huntvis: got %d, want HuntVisLineOfSight (%d)", it.huntvis, objtype.HuntVisLineOfSight)
	}
	if it.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)", it.creationTick)
	}
	if it.level != 2 || it.x != 3200 || it.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d), want (2, 3200, 3300)",
			it.level, it.x, it.z)
	}
	if it.distance != 10 {
		t.Errorf("distance: got %d, want 10", it.distance)
	}
	if s.ISP != 0 {
		t.Errorf("HUNTALL should not push; ISP=%d", s.ISP)
	}
}

func TestHandleHuntAll_NilLookupDegrades(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, objtype.HuntVisOff, nil)
	// PlayerLookup left nil.
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll with nil PlayerLookup: %v", err)
	}
	if s.huntIterator != nil {
		t.Error("huntIterator should remain nil when PlayerLookup is nil (degrades to HUNTNEXT push-0)")
	}
}

func TestHandleHuntAll_InvalidHuntVisRejected(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, 99, &mockPlayerLookup{})
	if err := handleHuntAll(s); err == nil {
		t.Fatal("expected validator error for invalid huntvis=99")
	} else if !strings.Contains(err.Error(), "HUNTALL") {
		t.Errorf("error should be tagged HUNTALL: %v", err)
	}
	if s.huntIterator != nil {
		t.Error("huntIterator should remain nil after validation error")
	}
}

// --- NAI-35-T5: HUNTNEXT handler tests ---------------------------------

// newHuntNextState mirrors newNpcFindNextState (handlers_npc_test.go:1860):
// builds a ScriptState with a pre-set huntIterator (*PlayerIterator) and
// configurable World tick. Tests use this for direct handler-level coverage.
func newHuntNextState(t *testing.T, tick int, iter *PlayerIterator) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if iter != nil {
		s.huntIterator = iter
	}
	return s
}

func TestHandleHuntNext_NilIteratorPushesZero(t *testing.T) {
	s := newHuntNextState(t, 0, nil)
	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.Self != nil {
		t.Error("Self should remain nil on nil iterator")
	}
	if s.Pointers&PtrActivePlayer != 0 {
		t.Error("PtrActivePlayer should NOT be set on nil iterator")
	}
}

func TestHandleHuntNext_StaleIteratorReturnsError(t *testing.T) {
	// Iterator created at tick=3, World now at tick=5 → stale.
	iter := NewHuntAllPlayerIterator(
		&mockPlayerLookup{}, nil, 3, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 5, iter)

	err := handleHuntNext(s)
	if err == nil {
		t.Fatal("stale iterator should return error")
	}
	if !strings.Contains(err.Error(), "HUNTNEXT") {
		t.Errorf("error should be tagged HUNTNEXT: %v", err)
	}
	if !strings.Contains(err.Error(), "tried to use an old iterator") {
		t.Errorf("error message should mention old iterator: %v", err)
	}
}

func TestHandleHuntNext_HitSetsSelfAndPushesOne(t *testing.T) {
	// HuntAll cursor for (level=0, x=3200, z=3200, distance=8):
	//   centerX = 3200>>3 = 400; radius = 1+8/8 = 2.
	//   curZoneX=curZoneZ=402 (max corner). First ZonePlayers lookup at
	//   world coords (402*8, 402*8) = (3216, 3216).
	// Player at (3204, 3204): DistanceToSW(3200,3200,3204,3204) = max(4,4) = 4 ≤ 8 → hit.
	target := &mockPlayer{username: "Hit", x: 3204, z: 3204}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, 3216, 3216}: {target},
		},
	}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", s.IntStack[:s.ISP])
	}
	if s.Self != target {
		t.Errorf("Self: got %v, want target %v", s.Self, target)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Error("PtrActivePlayer should be set on hit")
	}
}

func TestHandleHuntNext_ExhaustionPushesZero(t *testing.T) {
	// Empty PlayerLookup → iterator walks all zones in the radius and
	// finds nothing.
	lookup := &mockPlayerLookup{}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", s.IntStack[:s.ISP])
	}
	if s.Self != nil {
		t.Error("Self should remain nil on exhaustion (no hit to bind)")
	}
	if s.Pointers&PtrActivePlayer != 0 {
		t.Error("PtrActivePlayer should NOT be set on exhaustion")
	}
}

// TestHandleHuntNext_ExhaustionDoesNotClearIterator pins
// iterator_state_pattern.md element 7: exhaustion does NOT nil out
// s.huntIterator. Mirrors NPC parity at handlers_npc_test.go:1926.
func TestHandleHuntNext_ExhaustionDoesNotClearIterator(t *testing.T) {
	lookup := &mockPlayerLookup{}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("first handleHuntNext: %v", err)
	}
	_ = s.PopInt() // discard first push
	if s.huntIterator == nil {
		t.Fatal("huntIterator should NOT be cleared on exhaustion (TS parity)")
	}

	// Second call on the now-exhausted iterator must also push 0
	// without erroring (Stale check still passes — same tick).
	if err := handleHuntNext(s); err != nil {
		t.Fatalf("second handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("second exhaustion: got push %d, want 0", got)
	}
	if s.huntIterator == nil {
		t.Error("huntIterator should still be non-nil after second call")
	}
}

// --- HUNTNEXT intOperand slot-selection tests -------------------------------
//
// HUNTNEXT mirrors TS PlayerOps.ts:1236-1237 by routing the hit result to
// the slot selected by intOperand: 0 → Self + PtrActivePlayer, 1 → Self2 +
// PtrActivePlayer2. Pre-fix, the handler unconditionally bound Self regardless
// of intOperand (same-shape bug as the pre-NAI-133 FINDUID/P_FINDUID clobber).

// newHuntNextStateWithOperand mirrors newHuntNextState but lets the test pin
// IntOperands[0] for slot-routing coverage.
func newHuntNextStateWithOperand(t *testing.T, tick int, iter *PlayerIterator, operand int32) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{operand}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if iter != nil {
		s.huntIterator = iter
	}
	return s
}

// TestHandleHuntNext_Operand0_BindsPrimarySlot pins that intOperand=0 routes
// the hit to s.Self + PtrActivePlayer (and leaves Self2/PtrActivePlayer2
// untouched). Mirrors TS `ActivePlayer[0]` = ScriptPointer.ActivePlayer.
func TestHandleHuntNext_Operand0_BindsPrimarySlot(t *testing.T) {
	// Same fixture as TestHandleHuntNext_HitSetsSelfAndPushesOne.
	target := &mockPlayer{username: "Hit", x: 3204, z: 3204}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, 3216, 3216}: {target},
		},
	}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextStateWithOperand(t, 100, iter, 0)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", s.IntStack[:s.ISP])
	}
	if s.Self != target {
		t.Errorf("Self: got %v, want target %v", s.Self, target)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Error("PtrActivePlayer should be set on hit (operand=0)")
	}
	if s.Self2 != nil {
		t.Errorf("Self2 should remain nil on operand=0 hit, got %v", s.Self2)
	}
	if s.Pointers&PtrActivePlayer2 != 0 {
		t.Error("PtrActivePlayer2 should NOT be set on operand=0 hit")
	}
}

// TestHandleHuntNext_Operand1_BindsSecondarySlot pins that intOperand=1 routes
// the hit to s.Self2 + PtrActivePlayer2 (and leaves Self/PtrActivePlayer
// untouched). Mirrors TS `ActivePlayer[1]` = ScriptPointer.ActivePlayer2.
// Pre-fix, this case clobbered Self instead.
func TestHandleHuntNext_Operand1_BindsSecondarySlot(t *testing.T) {
	target := &mockPlayer{username: "Hit2", x: 3204, z: 3204}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, 3216, 3216}: {target},
		},
	}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextStateWithOperand(t, 100, iter, 1)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", s.IntStack[:s.ISP])
	}
	if s.Self2 != target {
		t.Errorf("Self2: got %v, want target %v", s.Self2, target)
	}
	if s.Pointers&PtrActivePlayer2 == 0 {
		t.Error("PtrActivePlayer2 should be set on hit (operand=1)")
	}
	if s.Self != nil {
		t.Errorf("Self should remain nil on operand=1 hit, got %v", s.Self)
	}
	if s.Pointers&PtrActivePlayer != 0 {
		t.Error("PtrActivePlayer should NOT be set on operand=1 hit")
	}
}

// TestHandleHuntNext_InvalidOperand_Errors pins that intOperand>1 (or <0)
// is rejected with a tagged error. Mirrors handleFindUID's invalid-operand
// branch at handlers_player.go:1033-1035.
func TestHandleHuntNext_InvalidOperand_Errors(t *testing.T) {
	lookup := &mockPlayerLookup{}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)

	for _, operand := range []int32{2, -1, 99} {
		s := newHuntNextStateWithOperand(t, 100, iter, operand)
		err := handleHuntNext(s)
		if err == nil {
			t.Errorf("operand=%d: expected error, got nil", operand)
			continue
		}
		if !strings.Contains(err.Error(), "HUNTNEXT") {
			t.Errorf("operand=%d: error should be tagged HUNTNEXT: %v", operand, err)
		}
		if !strings.Contains(err.Error(), "invalid intOperand") {
			t.Errorf("operand=%d: error should mention invalid intOperand: %v", operand, err)
		}
	}
}

// --- NAI-37 Task 6 / rev-244 B4: HINT_NPC handler unit tests --------------
//
// 244 contract (PlayerOps.ts:963-965):
//
//	state.activePlayer.hintNpc(check(state.popInt(), NumberNotNull))
//
// HINT_NPC now pops the nid from the int stack rather than reading
// state.activeNpc. The requireActiveNpc gate is gone from the handler
// (though the compiler-side pointer-table entry retains it). Removed tests
// that pinned the 225 activeNpc-based contract:
//   - TestHintNpc_NoActiveNpc_Errors (225 gate is gone in 244)
//   - TestHintNpc_Success_RecordsNid (pinned nid from activeNpc, not stack)

func TestHintNpc_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	s.PushInt(42)
	if err := handleHintNpc(s); err == nil {
		t.Fatalf("expected error for no active player")
	}
}

// TestHintNpc_PopsNidFromStack pins the 244 contract: nid is popped from the
// int stack (not read from state.activeNpc). TS PlayerOps.ts:963-965.
func TestHintNpc_PopsNidFromStack(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
		// ActiveNpc intentionally NOT set — handler must not require it.
	}
	s.PushInt(42)
	if err := handleHintNpc(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{42}; !slices.Equal(pl.hintNpcCalls, want) {
		t.Errorf("hintNpcCalls: got %v, want %v", pl.hintNpcCalls, want)
	}
}

// TestHintNpc_NullNid_Errors pins that a null(-1) nid is rejected via
// checkNotNull, mirroring TS check(state.popInt(), NumberNotNull).
func TestHintNpc_NullNid_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(-1) // null sentinel
	if err := handleHintNpc(s); err == nil {
		t.Fatal("expected error for null nid (-1)")
	}
	if len(pl.hintNpcCalls) != 0 {
		t.Errorf("hintNpcCalls: got %d, want 0 on validation failure", len(pl.hintNpcCalls))
	}
}

// --- NAI-39 Task 4: HINT_COORD / HINT_PL / HINT_STOP handler unit tests ---

func TestHintCoord_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintCoord_InvalidCoord_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// Push offset=3, coord=-1 (invalid), height=0. Pop order is height,
	// coord, offset — so push offset FIRST.
	s.PushInt(3)
	s.PushInt(-1)
	s.PushInt(0)
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for invalid coord")
	}
	if len(pl.hintCoordCalls) != 0 {
		t.Errorf("hintCoordCalls: got %d, want 0 on validation failure", len(pl.hintCoordCalls))
	}
}

func TestHintCoord_Success_RecordsArgs(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(level=0, x=100, z=200) = (0<<28)|(100<<14)|200
	coord := (100 << 14) | 200
	s.PushInt(3)     // offset
	s.PushInt(coord) // coord
	s.PushInt(42)    // height
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 3, x: 100, z: 200, height: 42}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

// TestHintCoord_PopOrderDistinctValues pins which popped value lands in
// which dispatch arg. Distinct values rule out symmetric off-by-one.
func TestHintCoord_PopOrderDistinctValues(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(0, 1, 2)
	coord := (1 << 14) | 2
	s.PushInt(2) // offset (push first, popped last)
	s.PushInt(coord)
	s.PushInt(99) // height (push last, popped first)
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 2, x: 1, z: 2, height: 99}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

// --- NAI-39 Task 4 / rev-244 B4: HINT_PLAYER handler unit tests -----------
//
// 244 contract (PlayerOps.ts:967-974):
//
//	const uid = check(state.popInt(), NumberNotNull)
//	const player = World.getPlayerByUid(uid)
//	if (!player) { return }
//	state.activePlayer.hintPlayer(player.pid)
//
// HINT_PLAYER now pops a uid, resolves via PlayerLookup, and hints by pid
// (Slot() in the ActivePlayer interface). activePlayer2 is gone. Removed
// tests that pinned the 225 activePlayer2-based contract:
//   - TestHintPl_NoActivePlayer2_Errors (requireActivePlayer2 gate is gone)
//   - TestHintPl_Success_RecordsSlot (pinned Self2.Slot(), not uid lookup)

func TestHintPl_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	s.PushInt(7)
	if err := handleHintPlayer(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

// TestHintPlayer_UidLookup_Hit pins: uid resolved → HintPlayer called with
// the target player's pid (Slot()). TS PlayerOps.ts:967-974.
func TestHintPlayer_UidLookup_Hit(t *testing.T) {
	pl := &mockPlayer{}
	target := &mockPlayer{slot: 3}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{7: target}}
	s := &ScriptState{
		IntStack:     make([]int, StackCapacity),
		StringStack:  make([]string, StackCapacity),
		Self:         pl,
		Pointers:     PtrActivePlayer,
		PlayerLookup: lookup,
	}
	s.PushInt(7) // uid
	if err := handleHintPlayer(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{3}; !slices.Equal(pl.hintPlayerCalls, want) {
		t.Errorf("hintPlayerCalls: got %v, want %v", pl.hintPlayerCalls, want)
	}
}

// TestHintPlayer_UidLookup_Miss pins the silent no-op on uid miss: TS
// PlayerOps.ts:970-972 `if (!player) { return; }` — no error, no hint call.
func TestHintPlayer_UidLookup_Miss(t *testing.T) {
	pl := &mockPlayer{}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}} // uid 9999 not present
	s := &ScriptState{
		IntStack:     make([]int, StackCapacity),
		StringStack:  make([]string, StackCapacity),
		Self:         pl,
		Pointers:     PtrActivePlayer,
		PlayerLookup: lookup,
	}
	s.PushInt(9999) // unknown uid
	if err := handleHintPlayer(s); err != nil {
		t.Fatalf("unexpected error on uid miss: %v", err)
	}
	if len(pl.hintPlayerCalls) != 0 {
		t.Errorf("hintPlayerCalls: got %d, want 0 on uid miss", len(pl.hintPlayerCalls))
	}
}

// TestHintPlayer_NullUid_Errors pins that a null(-1) uid is rejected via
// checkNotNull, mirroring TS check(state.popInt(), NumberNotNull).
func TestHintPlayer_NullUid_Errors(t *testing.T) {
	pl := &mockPlayer{}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}
	s := &ScriptState{
		IntStack:     make([]int, StackCapacity),
		StringStack:  make([]string, StackCapacity),
		Self:         pl,
		Pointers:     PtrActivePlayer,
		PlayerLookup: lookup,
	}
	s.PushInt(-1) // null sentinel
	if err := handleHintPlayer(s); err == nil {
		t.Fatal("expected error for null uid (-1)")
	}
	if len(pl.hintPlayerCalls) != 0 {
		t.Errorf("hintPlayerCalls: got %d, want 0 on validation failure", len(pl.hintPlayerCalls))
	}
}

func TestHintStop_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintStop(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintStop_Success_IncrementsCounter(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	if err := handleHintStop(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.hintStopCalls != 1 {
		t.Errorf("hintStopCalls: got %d, want 1", pl.hintStopCalls)
	}
}

// --- NAI-47: handleSetIdKit ---

func buildIdkTypeConfig(id, typ int) *objtype.IdkType {
	c := objtype.NewIdkType(id)
	c.Type = typ
	return c
}

func TestHandleSetIdKitRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{IntStack: make([]int, StackCapacity)}
	s.PushInt(0)
	s.PushInt(0)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for no active player, got nil")
	}
}

func TestHandleSetIdKitNilConfigs(t *testing.T) {
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}, IntStack: make([]int, StackCapacity)}
	s.PushInt(0) // idkit (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for nil Configs, got nil")
	}
}

func TestHandleSetIdKitInvalidIdkit(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{}}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(5) // idkit=5 — not in registry (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for unknown idkit, got nil")
	}
}

// TestHandleSetIdKitMaleHair: gender=0, idkType.Type=0 (hair) → body[0]=idkit,
// colors[0]=color (hair colorSlot).
func TestHandleSetIdKitMaleHair(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{3: buildIdkTypeConfig(3, 0)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(3) // idkit=3 (Type=0, male hair) — pushed first = below
	s.PushInt(7) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 3 {
		t.Errorf("bodyParts[0]: got %d, want 3 (idkit id)", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 7 {
		t.Errorf("colorParts[0]: got %d, want 7 (hair color)", mp.colorParts[0])
	}
}

// TestHandleSetIdKitFemaleSlotAdjust: gender=1, idkType.Type=7 (female hair).
// slot = 7 − 7 = 0, adjustedType = 0 → colorSlot=0.
func TestHandleSetIdKitFemaleSlotAdjust(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{9: buildIdkTypeConfig(9, 7)}}
	mp := &mockPlayer{genderValue: 1}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(9) // idkit=9 (Type=7 → female hair, slot=0) — pushed first = below
	s.PushInt(2) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 9 {
		t.Errorf("bodyParts[0]: got %d, want 9", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 2 {
		t.Errorf("colorParts[0]: got %d, want 2", mp.colorParts[0])
	}
}

// TestHandleSetIdKitSkinNoColorWrite: Type=4 (hands) has no color slot.
// colorParts must stay at zero defaults.
func TestHandleSetIdKitSkinNoColorWrite(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{4: buildIdkTypeConfig(4, 4)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(4)  // idkit=4 (Type=4, hands/skin) — pushed first = below
	s.PushInt(99) // color (should not be written) — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[4] != 4 {
		t.Errorf("bodyParts[4]: got %d, want 4", mp.bodyParts[4])
	}
	for i, v := range mp.colorParts {
		if v != 0 {
			t.Errorf("colorParts[%d]: got %d, want 0 (no color write for Type=4)", i, v)
		}
	}
}

// TestHandleSetIdKitLegs: Type=5 → colorSlot=2.
func TestHandleSetIdKitLegs(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{5: buildIdkTypeConfig(5, 5)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(5)  // idkit=5 (Type=5, legs) — pushed first = below
	s.PushInt(11) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.colorParts[2] != 11 {
		t.Errorf("colorParts[2]: got %d, want 11 (legs colorSlot=2)", mp.colorParts[2])
	}
}

// TestHandleWalkTrigger_PopsAndWrites verifies P_WALKTRIGGER (opcode
// 2128) pops one int and writes it via SetWalkTrigger on the active
// player. Mirrors TS PlayerOps.ts:1035-1037.
func TestHandleWalkTrigger_PopsAndWrites(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[walktrigger,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpWalkTrigger, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.walkTriggerSetCalls != 1 {
		t.Errorf("SetWalkTrigger calls: got %d, want 1", mp.walkTriggerSetCalls)
	}
	if mp.lastWalkTriggerSet != 42 {
		t.Errorf("SetWalkTrigger arg: got %d, want 42", mp.lastWalkTriggerSet)
	}
}

// TestHandleWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset, matching the requireActivePlayer
// contract.
func TestHandleWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{IntStack: make([]int, StackCapacity)}
	state.PushInt(42)
	err := handleWalkTrigger(state)
	if err == nil {
		t.Fatal("handleWalkTrigger: got nil, want no-active-player error")
	}
}

// TestHandleGetWalkTrigger_ReadsAndPushes verifies GETWALKTRIGGER (opcode
// 2023) reads p.walktrigger via WalkTrigger() and pushes the value.
// Mirrors TS PlayerOps.ts:1039-1042.
func TestHandleGetWalkTrigger_ReadsAndPushes(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: 99}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 {
		t.Fatalf("ISP after GETWALKTRIGGER: got %d, want 1", state.ISP)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("popped: got %d, want 99", got)
	}
}

// TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne pins the unset
// sentinel propagation through the handler.
func TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: -1}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != -1 {
		t.Errorf("popped: got %d, want -1", got)
	}
}

// TestHandleGetWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset.
func TestHandleGetWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{IntStack: make([]int, StackCapacity)}
	err := handleGetWalkTrigger(state)
	if err == nil {
		t.Fatal("handleGetWalkTrigger: got nil, want no-active-player error")
	}
}

// TestHandleSessionLog pins the SESSION_LOG opcode (TS PlayerOps.ts:1184-1189).
// Stack convention: pushString(event); pushInt(eventType_unshifted) →
// handler pops eventType+2, pops event, calls Self.AddSessionLog(eventType+2, event).
func TestHandleSessionLog(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:     make([]int, StackCapacity),
		StringStack:  make([]string, StackCapacity),
		IntLocals:    []int{},
		StringLocals: []string{},
		Pointers:     PtrActivePlayer,
		Self:         mp,
	}
	// Push string first (deeper), then int (top of int stack).
	s.PushString("hello")
	s.PushInt(0) // script-side 0 → engine-side MODERATOR (2)

	if err := handleSessionLog(s); err != nil {
		t.Fatalf("handleSessionLog: %v", err)
	}
	if got := len(mp.addSessionLogCalls); got != 1 {
		t.Fatalf("AddSessionLog calls: got %d, want 1", got)
	}
	call := mp.addSessionLogCalls[0]
	if call.eventType != 2 {
		t.Errorf("eventType: got %d, want 2 (script 0 → MODERATOR via +2 shift)", call.eventType)
	}
	if call.message != "hello" {
		t.Errorf("message: got %q, want %q", call.message, "hello")
	}
	if len(call.args) != 0 {
		t.Errorf("args: got %v, want empty", call.args)
	}
}

// TestHandleSessionLogModeratorAdventureMapping pins both script-side
// values: 0 → 2 (MODERATOR), 1 → 3 (ADVENTURE).
func TestHandleSessionLogModeratorAdventureMapping(t *testing.T) {
	cases := []struct {
		scriptVal int
		wantType  int
	}{
		{0, 2}, // MODERATOR
		{1, 3}, // ADVENTURE
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("script%d_eng%d", tc.scriptVal, tc.wantType), func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Pointers:    PtrActivePlayer,
				Self:        mp,
			}
			s.PushString("evt")
			s.PushInt(tc.scriptVal)

			if err := handleSessionLog(s); err != nil {
				t.Fatalf("handleSessionLog: %v", err)
			}
			if mp.addSessionLogCalls[0].eventType != tc.wantType {
				t.Errorf("eventType: got %d, want %d", mp.addSessionLogCalls[0].eventType, tc.wantType)
			}
		})
	}
}

// TestHandleSessionLogRequiresActivePlayer pins the gate.
func TestHandleSessionLogRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Pointers:    0, // no PtrActivePlayer
		Self:        nil,
	}
	if err := handleSessionLog(s); err == nil {
		t.Fatal("handleSessionLog: want error on missing ActivePlayer, got nil")
	}
}

// TestSoundSynthHappyPath pins NAI-87: SOUND_SYNTH dispatches to
// (*ActivePlayer).PlaySynth with the popped (synth, loops, delay)
// triple in TS argument order. Push order left-to-right matches
// TS popInts(3) at ScriptState.ts:325-331 (top-of-stack popped
// first, written into result[amount-1]).
func TestSoundSynthHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(42)  // synth
	s.PushInt(2)   // loops
	s.PushInt(100) // delay
	mp := s.Self.(*mockPlayer)

	if err := handleSoundSynth(s); err != nil {
		t.Fatalf("handleSoundSynth: %v", err)
	}
	if len(mp.playSynthCalls) != 1 {
		t.Fatalf("playSynthCalls: got %d, want 1", len(mp.playSynthCalls))
	}
	got := mp.playSynthCalls[0]
	if got.synth != 42 || got.loops != 2 || got.delay != 100 {
		t.Errorf("playSynthCalls[0] = %+v, want {synth:42, loops:2, delay:100}", got)
	}
}

// TestSoundSynthLowMemoryBails pins TS PlayerOps.ts:470-472 silent
// no-op gate. lowMemory=true → handler returns nil and PlaySynth is
// NOT called.
func TestSoundSynthLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(42)
	s.PushInt(2)
	s.PushInt(100)
	mp := s.Self.(*mockPlayer)

	if err := handleSoundSynth(s); err != nil {
		t.Fatalf("handleSoundSynth: %v", err)
	}
	if len(mp.playSynthCalls) != 0 {
		t.Errorf("lowMemory=true: playSynthCalls=%d, want 0", len(mp.playSynthCalls))
	}
}

// TestSoundSynthNoActivePlayerRejects pins the requireActivePlayer
// gate. Self=nil + Pointers=0 → error containing "SOUND_SYNTH: no
// active player".
func TestSoundSynthNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}
	s.PushInt(42)
	s.PushInt(2)
	s.PushInt(100)

	err := handleSoundSynth(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "SOUND_SYNTH: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "SOUND_SYNTH: no active player")
	}
}

// TestDisplayNameHappyPath pins NAI-103: DISPLAYNAME pushes the active
// player's display name onto the string stack.
func TestDisplayNameHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{displayName: "Alice Smith"},
		Pointers:    PtrActivePlayer,
	}

	if err := handleDisplayName(s); err != nil {
		t.Fatalf("handleDisplayName: %v", err)
	}
	if got := s.PopString(); got != "Alice Smith" {
		t.Errorf("PopString(): got %q, want %q", got, "Alice Smith")
	}
}

// TestDisplayNameNoActivePlayerRejects pins the requireActivePlayer gate.
// Self=nil + Pointers=0 → error containing "DISPLAYNAME: no active player".
func TestDisplayNameNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}

	err := handleDisplayName(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "DISPLAYNAME: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "DISPLAYNAME: no active player")
	}
}

// -- NAI-90 frame T tests ------------------------------------------------

// recordingHandler is a minimal slog handler that captures records for
// assertion. Used by NAI-90 frame T tests; not exported.
type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

// installRecordingLogger swaps slog.Default for a recording handler at
// INFO level for the duration of the test. Returns the handler so the
// test can read records; restoration is automatic via t.Cleanup.
func installRecordingLogger(t *testing.T) *recordingHandler {
	t.Helper()
	prev := slog.Default()
	h := &recordingHandler{}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func TestPTeleport_FrameT_EmittedWhenNodeDebugTrue(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "frame_t_emit"},
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Pointers:  PtrProtectedActivePlayer,
		NodeDebug: true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(packCoord(0, 3098, 3107))

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("frame T records: got %d, want 1", len(rec.records))
	}
	if rec.records[0].Message != "p_teleport" {
		t.Errorf("frame T message: got %q, want %q", rec.records[0].Message, "p_teleport")
	}
}

func TestPTeleport_FrameT_SuppressedWhenNodeDebugFalse(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{}
	s := &ScriptState{
		Script:   &ScriptFile{Name: "frame_t_silent"},
		IntStack: make([]int, StackCapacity),
		Self:     mp,
		Pointers: PtrProtectedActivePlayer,
		// NodeDebug zero-value = false
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(packCoord(0, 3098, 3107))

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 0 {
		t.Errorf("frame T records under NodeDebug=false: got %d, want 0", len(rec.records))
	}
}

func TestPTeleport_FrameT_FieldValues(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{coordPacked: packCoord(0, 3094, 3107)}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "open_and_close_door"},
		PC:        42,
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Pointers:  PtrProtectedActivePlayer,
		NodeDebug: true,
	}
	s.Pointers |= PtrActivePlayer
	argCoord := packCoord(0, 3098, 3107)
	s.PushInt(argCoord)

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("frame T records: got %d, want 1", len(rec.records))
	}
	got := map[string]any{}
	rec.records[0].Attrs(func(a slog.Attr) bool {
		got[a.Key] = a.Value.Any()
		return true
	})

	want := map[string]any{
		"script_name":    "open_and_close_door",
		"script_pc":      int64(42),
		"self_username":  "",
		"self_coord_pre": int64(packCoord(0, 3094, 3107)),
		"arg_coord":      int64(argCoord),
		"arg_x":          int64(3098),
		"arg_z":          int64(3107),
		"arg_level":      int64(0),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("frame T field %s: got %v, want %v", k, got[k], v)
		}
	}
}

// TestTextGenderMale: gender=0 → handler pushes the male string (the
// second-popped, i.e. the bottom of the two-string slice on entry).
// Mirrors TS PlayerOps.ts:787-794, gender===0 branch.
func TestTextGenderMale(t *testing.T) {
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("MALE")   // pushed first → below
	s.PushString("FEMALE") // pushed last → top
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "MALE" {
		t.Errorf("pushed string: got %q, want %q", got, "MALE")
	}
}

// TestTextGenderFemale: gender=1 → handler pushes the female string
// (the first-popped, i.e. top of stack on entry). Mirrors TS
// PlayerOps.ts:787-794, gender!==0 branch.
func TestTextGenderFemale(t *testing.T) {
	mp := &mockPlayer{genderValue: 1}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("MALE")   // pushed first → below
	s.PushString("FEMALE") // pushed last → top
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "FEMALE" {
		t.Errorf("pushed string: got %q, want %q", got, "FEMALE")
	}
}

// TestTextGenderNoActivePlayer: pointer-gate. Self=nil and/or
// PtrActivePlayer unset → handler returns the standard
// requireActivePlayer error and leaves the string stack untouched.
func TestTextGenderNoActivePlayer(t *testing.T) {
	s := &ScriptState{
		Pointers:    0, // no PtrActivePlayer
		Self:        nil,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("a")
	s.PushString("b")
	err := handleTextGender(s)
	if err == nil {
		t.Fatal("want error for no-active-player, got nil")
	}
	if !strings.Contains(err.Error(), "TEXT_GENDER: no active player") {
		t.Errorf("error: got %q, want substring %q", err.Error(), "TEXT_GENDER: no active player")
	}
	if s.SSP != 2 {
		t.Errorf("SSP: got %d, want 2 (stack must be untouched on guard reject)", s.SSP)
	}
}

// TestTextGenderEmptyStrings: TS does NOT call check(..., StringNotNull)
// on either argument (PlayerOps.ts:787-794 — destructure-and-push, no
// gate). Empty strings are valid input and pass through unchanged.
// Per ts_asymmetry_dual_pin memory: pin the absence of a null gate so
// the test escalates if upstream TS adds one.
func TestTextGenderEmptyStrings(t *testing.T) {
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("") // male (below)
	s.PushString("") // female (top)
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "" {
		t.Errorf("pushed string: got %q, want empty string", got)
	}
}

// --- NAI-115 T7: P_OPOBJ handler tests -----------------------------------

func TestHandleP_OpObjHappyPath(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{uidValue: 12345}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	active := &mockActiveObj{objType: 590, x: 3200, z: 3200, level: 0}
	s.ActiveObj = active

	mc := newTestConfigs()
	logs := objtype.NewObjType(590)
	logs.Op = make([]string, 5)
	logs.Op[0] = "Light"
	mc.objs[590] = logs
	s.Configs = mc

	s.PushInt(1) // op = 1

	if err := handleP_OpObj(s); err != nil {
		t.Fatalf("handleP_OpObj returned error: %v", err)
	}
	if pl.stopActionCalls != 1 {
		t.Errorf("expected 1 StopAction call, got %d", pl.stopActionCalls)
	}
	if len(pl.queueWaypointCalls) != 1 || pl.queueWaypointCalls[0] != (struct{ x, z int }{x: 3200, z: 3200}) {
		t.Errorf("QueueWaypoint args: got %+v, want [{3200, 3200}]", pl.queueWaypointCalls)
	}
	if len(pl.objOpCalls) != 1 || pl.objOpCalls[0].op != 1 {
		t.Errorf("SetInteractionScriptObj: got %+v, want 1 call op=1", pl.objOpCalls)
	}
}

func TestHandleP_OpObjOutOfRange(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{uidValue: 12345}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	s.ActiveObj = &mockActiveObj{objType: 590, x: 0, z: 0, level: 0}

	mc := newTestConfigs()
	logs := objtype.NewObjType(590)
	logs.Op = make([]string, 5)
	mc.objs[590] = logs
	s.Configs = mc

	s.PushInt(6) // op = 6 → out of range (1..5)
	if err := handleP_OpObj(s); err == nil {
		t.Errorf("P_OPOBJ op=6: expected error, got nil")
	}
}

func TestHandleP_OpObjMissingOpEntryShortCircuits(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{uidValue: 12345}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	s.ActiveObj = &mockActiveObj{objType: 590, x: 0, z: 0, level: 0}

	mc := newTestConfigs()
	logs := objtype.NewObjType(590)
	logs.Op = make([]string, 5) // empty op slots → silent skip
	mc.objs[590] = logs
	s.Configs = mc

	s.PushInt(3) // op = 3 → Op[2] empty
	if err := handleP_OpObj(s); err != nil {
		t.Fatalf("P_OPOBJ missing op entry: expected nil-error short-circuit, got %v", err)
	}
	if len(pl.objOpCalls) != 0 {
		t.Errorf("P_OPOBJ missing op entry: expected 0 SetInteractionScriptObj calls, got %d", len(pl.objOpCalls))
	}
}

// TestHandleP_OpObjUnregisteredTypeSilentSkips pins L17: TS ObjType.get
// returns a default (all-null-op) type for an unregistered id, so P_OPOBJ
// falls through to the `type.op[op] === null → return` silent skip rather than
// erroring. Go now matches — a nil ObjType (unregistered id) or a nil Configs
// is treated as an empty-op type, so the script continues without error.
func TestHandleP_OpObjUnregisteredTypeSilentSkips(t *testing.T) {
	newState := func(configs Configs) (*ScriptState, *mockPlayer) {
		s := newTestState(minimalScript(OpReturn))
		pl := &mockPlayer{uidValue: 12345}
		s.Self = pl
		s.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
		s.ActiveObj = &mockActiveObj{objType: 999, x: 0, z: 0, level: 0} // 999 unregistered
		s.Configs = configs
		s.PushInt(1)
		return s, pl
	}
	// Configs present but id 999 not registered → ObjType(999) == nil.
	s1, pl1 := newState(newTestConfigs())
	if err := handleP_OpObj(s1); err != nil {
		t.Fatalf("unregistered type: want nil (silent skip), got %v", err)
	}
	if len(pl1.objOpCalls) != 0 {
		t.Errorf("unregistered type: want 0 SetInteractionScriptObj calls, got %d", len(pl1.objOpCalls))
	}
	// Missing registry entirely (Configs nil).
	s2, pl2 := newState(nil)
	if err := handleP_OpObj(s2); err != nil {
		t.Fatalf("nil Configs: want nil (silent skip), got %v", err)
	}
	if len(pl2.objOpCalls) != 0 {
		t.Errorf("nil Configs: want 0 SetInteractionScriptObj calls, got %d", len(pl2.objOpCalls))
	}
}

func TestHandleP_OpObjRequiresProtect(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{uidValue: 12345}
	s.Self = pl
	s.Pointers |= PtrActivePlayer
	// not protected — gate must fire (Pointers zero-value lacks PtrProtectedActivePlayer)
	s.ActiveObj = &mockActiveObj{objType: 590, x: 0, z: 0, level: 0}

	mc := newTestConfigs()
	logs := objtype.NewObjType(590)
	logs.Op = make([]string, 5)
	logs.Op[0] = "Light"
	mc.objs[590] = logs
	s.Configs = mc

	s.PushInt(1)
	if err := handleP_OpObj(s); err == nil {
		t.Errorf("P_OPOBJ without PtrProtectedActivePlayer: expected error, got nil")
	}
}

// NAI-115 stretch — LOWMEM handler tests.

func TestHandleLowMemReturnsZeroWhenHighMem(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{}
	s.Pointers |= PtrActivePlayer

	if err := handleLowMemory(s); err != nil {
		t.Fatalf("LOWMEMORY returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LOWMEMORY high-mem: got %d, want 0", got)
	}
}

func TestHandleLowMemReturnsOneWhenLowMem(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	pl := &mockPlayer{lowMemoryValue: true}
	s.Self = pl
	s.Pointers |= PtrActivePlayer

	if err := handleLowMemory(s); err != nil {
		t.Fatalf("LOWMEMORY returned error: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("LOWMEMORY low-mem: got %d, want 1", got)
	}
}

func TestHandleLowMemNoActivePlayer(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if err := handleLowMemory(s); err == nil {
		t.Errorf("LOWMEMORY no active player: expected error, got nil")
	}
}

// -- BUSY tests (NAI-163 B0) --------------------------------------------------

func TestHandleBusy_NotBusy_NotLoggingOut_PushZero(t *testing.T) {
	mp := &mockPlayer{busyValue: false, loggingOutValue: false}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("BUSY neither: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("BUSY neither: got %d, want 0", got)
	}
}

func TestHandleBusy_Busy_PushOne(t *testing.T) {
	mp := &mockPlayer{busyValue: true, loggingOutValue: false}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("BUSY busy: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY busy: got %d, want 1", got)
	}
}

// TestHandleBusy_LoggingOut_PushOne pins the loggingOut arm — this is the
// conspicuous TS-asymmetry that distinguishes BUSY (opcode 2005) from BUSY2
// (opcode 2006): BUSY2 uses hasInteraction()||hasWaypoints() and does NOT
// gate on loggingOut. Per ts_asymmetry_dual_pin.md. NAI-163 B0.
func TestHandleBusy_LoggingOut_PushOne(t *testing.T) {
	mp := &mockPlayer{busyValue: false, loggingOutValue: true}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("BUSY loggingOut: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY loggingOut: got %d, want 1", got)
	}
}

func TestHandleBusy_NoActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy(s); err == nil {
		t.Error("BUSY with no active player: want error")
	}
}

// -- BUSY2 tests (NAI-120 Bundle 2B) -----------------------------------------

func TestBusy2_HasInteraction(t *testing.T) {
	mp := &mockPlayer{hasInteractionValue: true}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 hasInteraction: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY2 hasInteraction: got %d, want 1", got)
	}
}

func TestBusy2_HasWaypoints(t *testing.T) {
	mp := &mockPlayer{hasWaypointsValue: true}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 hasWaypoints: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY2 hasWaypoints: got %d, want 1", got)
	}
}

func TestBusy2_NeitherSet(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 neither: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("BUSY2 neither: got %d, want 0", got)
	}
}

func TestBusy2_NoActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err == nil {
		t.Error("BUSY2 with no active player: want error")
	}
}

// -- P_OPNPCT tests (NAI-120 Bundle 2B) --------------------------------------

func TestPOpNpcT_HappyPath(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234) // spellCom
	if err := handlePOpNpcT(s); err != nil {
		t.Fatalf("P_OPNPCT happy: unexpected error %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("P_OPNPCT happy: stopActionCalls = %d, want 1", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptNpcT); got != 1 {
		t.Fatalf("P_OPNPCT happy: SetInteractionScriptNpcT calls = %d, want 1", got)
	}
	call := mp.lastSetInteractionScriptNpcT[0]
	if call.npc != npc {
		t.Errorf("P_OPNPCT happy: npc = %v, want %v", call.npc, npc)
	}
	if call.spellCom != 1234 {
		t.Errorf("P_OPNPCT happy: spellCom = %d, want 1234", call.spellCom)
	}
}

func TestPOpNpcT_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc, // not protected — PtrProtectedActivePlayer absent
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234)
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT not-protected: want error")
	}
}

func TestPOpNpcT_NoActiveNpc(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234)
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT no active npc: want error")
	}
}

func TestPOpNpcT_NullSpellCom(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1) // null sentinel
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT spellCom=-1: want NumberNotNull error")
	}
}

// -- P_OPPLAYER tests (NAI-120 Bundle 2B) ------------------------------------

func TestPOpPlayer_HappyPath(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2 | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(2) // op
	if err := handlePOpPlayer(s); err != nil {
		t.Fatalf("P_OPPLAYER happy: unexpected error %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("P_OPPLAYER happy: stopActionCalls = %d, want 1", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptPlayer); got != 1 {
		t.Fatalf("P_OPPLAYER happy: SetInteractionScriptPlayer calls = %d, want 1", got)
	}
	call := mp.lastSetInteractionScriptPlayer[0]
	if call.player2 != mp2 {
		t.Errorf("P_OPPLAYER happy: player2 = %v, want %v", call.player2, mp2)
	}
	if call.op != 2 {
		t.Errorf("P_OPPLAYER happy: op = %d, want 2", call.op)
	}
}

func TestPOpPlayer_NoSelf2_SilentReturn(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       nil,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	if err := handlePOpPlayer(s); err != nil {
		t.Fatalf("P_OPPLAYER no Self2: want silent return, got error %v", err)
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("P_OPPLAYER no Self2: stopActionCalls = %d, want 0 (no-op)", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptPlayer); got != 0 {
		t.Errorf("P_OPPLAYER no Self2: should not call SetInteractionScriptPlayer, got %d calls", got)
	}
}

func TestPOpPlayer_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2, // not protected — PtrProtectedActivePlayer absent
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER not-protected: want error")
	}
}

func TestPOpPlayer_OpZero(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2 | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0) // 0 is NOT NumberNotNull-rejected; but op 0 → type=-1 → out of [0,5)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=0: want range error")
	}
}

func TestPOpPlayer_OpSix(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2 | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(6) // type = 5; type >= 5 fails
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=6: want range error")
	}
}

func TestPOpPlayer_OpNullSentinel(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2 | PtrProtectedActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=-1: want NumberNotNull error")
	}
}

// --- NAI-127 Bundle 1: FINDHERO (opcode 2018) ---

func newFindHeroState(self *mockPlayer, mw WorldVars, intOperand int) *ScriptState {
	s := &ScriptState{
		World:       mw,
		Self:        self,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.Script = &ScriptFile{IntOperands: []int32{int32(intOperand)}}
	return s
}

func TestFindHero_EmptyLedger(t *testing.T) {
	self := &mockPlayer{topContributor: 0}
	s := newFindHeroState(self, &mockWorld{}, 0)
	if err := handleFindHero(s); err != nil {
		t.Fatalf("FINDHERO empty: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("FINDHERO empty: pushed %d, want 0", got)
	}
	if s.Self2 != nil {
		t.Errorf("FINDHERO empty: Self2 must remain nil")
	}
}

// FINDHERO ALWAYS sets Self2 (secondary) regardless of IntOperand —
// pin TS asymmetry vs NPC_FINDHERO per ts_asymmetry_dual_pin.
// TestFindHero_OperandSelectsLedger pins M12 (TS-correct contract): FINDHERO
// reads the hero ledger from the OPERAND-RESOLVED active player (op 0 → Self,
// op 1 → Self2), and always writes the found player to the RAW secondary slot
// (s.Self2), mirroring TS `state.activePlayer.heroPoints.findHero()` then
// `state._activePlayer2 = player` (PlayerOps.ts:1138-1154).
//
// (This previously asserted FINDHERO always read the PRIMARY ledger — a bug;
// the int operand was wrongly treated as not selecting the subject.)
func TestFindHero_OperandSelectsLedger(t *testing.T) {
	other := &mockPlayer{uidValue: 7}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{7: other}}

	t.Run("op0_reads_primary_ledger", func(t *testing.T) {
		self := &mockPlayer{topContributor: 7} // primary holds the hero
		s := newFindHeroState(self, mw, 0)
		if err := handleFindHero(s); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := s.PopInt(); got != 1 {
			t.Errorf("pushed %d, want 1", got)
		}
		if s.Self2 != other {
			t.Errorf("Self2=%v, want other (raw secondary write)", s.Self2)
		}
		if s.Pointers&PtrActivePlayer2 == 0 {
			t.Error("PtrActivePlayer2 must be set")
		}
	})

	t.Run("op1_reads_secondary_ledger", func(t *testing.T) {
		self := &mockPlayer{topContributor: 0}  // primary ledger empty
		self2 := &mockPlayer{topContributor: 7} // secondary holds the hero
		s := newFindHeroState(self, mw, 1)
		s.Self2 = self2
		s.Pointers |= PtrActivePlayer2
		if err := handleFindHero(s); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := s.PopInt(); got != 1 {
			t.Errorf("pushed %d, want 1 (operand=1 reads Self2 ledger)", got)
		}
		if s.Self2 != other {
			t.Errorf("Self2=%v, want other (raw secondary write, not operand-swapped)", s.Self2)
		}
	})
}

func TestFindHero_LookupReturnsNil(t *testing.T) {
	self := &mockPlayer{topContributor: 99}
	mw := &mockWorld{} // empty
	s := newFindHeroState(self, mw, 0)
	if err := handleFindHero(s); err != nil {
		t.Fatalf("FINDHERO loggedout: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("FINDHERO loggedout: pushed %d, want 0", got)
	}
}

func TestFindHero_RequiresActivePlayer(t *testing.T) {
	mw := &mockWorld{}
	s := &ScriptState{
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no PtrActivePlayer
	if err := handleFindHero(s); err == nil {
		t.Fatalf("FINDHERO no-active-player: want error, got nil")
	}
}

// --- NAI-127 Bundle 1: BOTH_HEROPOINTS (opcode 2003) ---

// newBothHeroPointsState builds a state with both Self and Self2 set,
// IntOperand and damage configured, and PtrActivePlayer set. Optional
// nilSelf2 lets tests pin the nil-slot error path.
func newBothHeroPointsState(self, other *mockPlayer, intOperand, damage int, nilSelf2 bool) *ScriptState {
	s := &ScriptState{
		Self:        self,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.Script = &ScriptFile{IntOperands: []int32{int32(intOperand)}}
	if !nilSelf2 {
		s.Self2 = other
		// Bind the secondary pointer too: BOTH_HEROPOINTS is checkedHandler(
		// ActivePlayer) in TS, so at operand=1 requireActivePlayer validates
		// the PtrActivePlayer2 slot (production binds it whenever Self2 is set).
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(damage)
	return s
}

func TestBothHeroPoints_PrimaryToSecondary(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	to := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(from, to, 0, 5, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS primary: err=%v", err)
	}
	if got := len(to.addHeroPointsCalls); got != 1 {
		t.Fatalf("BOTH_HEROPOINTS primary: to.addHeroPointsCalls=%d, want 1", got)
	}
	if call := to.addHeroPointsCalls[0]; call.playerUID != 11 || call.amount != 5 {
		t.Errorf("BOTH_HEROPOINTS primary: call=%+v, want {11,5}", call)
	}
	if got := len(from.addHeroPointsCalls); got != 0 {
		t.Errorf("BOTH_HEROPOINTS primary: from.addHeroPointsCalls=%d, want 0", got)
	}
}

func TestBothHeroPoints_SecondaryToPrimary(t *testing.T) {
	primary := &mockPlayer{uidValue: 11}
	secondary := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(primary, secondary, 1, 9, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS secondary: err=%v", err)
	}
	if got := len(primary.addHeroPointsCalls); got != 1 {
		t.Fatalf("BOTH_HEROPOINTS secondary: primary.addHeroPointsCalls=%d, want 1", got)
	}
	if call := primary.addHeroPointsCalls[0]; call.playerUID != 22 || call.amount != 9 {
		t.Errorf("BOTH_HEROPOINTS secondary: call=%+v, want {22,9}", call)
	}
}

func TestBothHeroPoints_NilSlot(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	s := newBothHeroPointsState(from, nil, 0, 5, true) // Self2 nil
	if err := handleBothHeroPoints(s); err == nil {
		t.Fatalf("BOTH_HEROPOINTS nilSelf2: want error, got nil")
	}
}

// Pin that handler still calls AddHeroPoints with amount=0; ledger
// no-ops downstream per HeroPoints.AddHero `if amount < 1 return`.
func TestBothHeroPoints_AmountZero(t *testing.T) {
	from := &mockPlayer{uidValue: 11}
	to := &mockPlayer{uidValue: 22}
	s := newBothHeroPointsState(from, to, 0, 0, false)
	if err := handleBothHeroPoints(s); err != nil {
		t.Fatalf("BOTH_HEROPOINTS zero: err=%v", err)
	}
	if got := len(to.addHeroPointsCalls); got != 1 {
		t.Errorf("BOTH_HEROPOINTS zero: to.addHeroPointsCalls=%d, want 1 (mock records before ledger no-ops)", got)
	}
	if call := to.addHeroPointsCalls[0]; call.amount != 0 {
		t.Errorf("BOTH_HEROPOINTS zero: call.amount=%d, want 0", call.amount)
	}
}

// --- NAI-127 Bundle 2: DAMAGE (opcode 2015) ---

// newDamageState builds a state with World set + push order matching
// the handler's pop order: amount, hitType, uid (the handler pops
// amount first, then hitType, then uid).
func newDamageState(mw WorldVars, uid, hitType, amount int) *ScriptState {
	s := &ScriptState{
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(uid)
	s.PushInt(hitType)
	s.PushInt(amount)
	return s
}

func TestDamage_HappyPath(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	s := newDamageState(mw, 42, 1, 7)
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE happy: err=%v", err)
	}
	if got := len(target.applyDamageCalls); got != 1 {
		t.Fatalf("DAMAGE happy: applyDamageCalls=%d, want 1", got)
	}
	if call := target.applyDamageCalls[0]; call.amount != 7 || call.dmgType != 1 {
		t.Errorf("DAMAGE happy: call=%+v, want {amount:7,dmgType:1}", call)
	}
}

func TestDamage_UnknownUID(t *testing.T) {
	mw := &mockWorld{} // empty playersByUID
	s := newDamageState(mw, 99, 1, 7)
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE unknown: err=%v", err)
	}
	// no panic, no call recorded — silent no-op
}

// Pin TS quirk: DAMAGE uses raw `state =>` with no checkedHandler;
// goscape's handler must NOT call requireActivePlayer.
func TestDamage_NoPointerGate(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	s := newDamageState(mw, 42, 1, 5)
	// Pointers=0 — no PtrActivePlayer set.
	if err := handleDamage(s); err != nil {
		t.Fatalf("DAMAGE no-pointer: err=%v (pointer-gate must be absent)", err)
	}
	if got := len(target.applyDamageCalls); got != 1 {
		t.Errorf("DAMAGE no-pointer: applyDamageCalls=%d, want 1", got)
	}
}

// TestDamage_InvalidHitType pins that DAMAGE (P_DAMAGE) rejects
// hitType outside [0, HitTypeCount). Mirrors TS PlayerOps.ts:778 —
// check(state.popInt(), HitTypeValid). The validator short-circuits
// before the uid pop, so no UID lookup or ApplyDamage occurs.
func TestDamage_InvalidHitType(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	// uid=42, hitType=3 (out of range), amount=5
	s := newDamageState(mw, 42, 3, 5)
	err := handleDamage(s)
	if err == nil {
		t.Fatalf("handleDamage: want error for hitType=3, got nil")
	}
	want := "DAMAGE: hit type out of range (3)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if got := len(target.applyDamageCalls); got != 0 {
		t.Errorf("applyDamageCalls: got %d, want 0 (must not damage on rejection)", got)
	}
}

// TestDamage_NullAmountRejected pins TS PlayerOps.ts:769 —
// check(state.popInt(), NumberNotNull) on the amount slot. amount=-1
// (the script null sentinel) must abort before hitType is read, with
// no UID lookup or ApplyDamage. Closes h-player-2 (audit row 247).
func TestDamage_NullAmountRejected(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	// uid=42, hitType=1 (valid), amount=-1 (null sentinel)
	s := newDamageState(mw, 42, 1, -1)
	err := handleDamage(s)
	if err == nil {
		t.Fatalf("handleDamage: want error for amount=-1, got nil (TS PlayerOps.ts:769 NumberNotNull must reject null amount)")
	}
	want := "DAMAGE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if got := len(target.applyDamageCalls); got != 0 {
		t.Errorf("applyDamageCalls: got %d, want 0 (must not damage on null amount)", got)
	}
}

// TestDamage_NullUIDRejected pins TS PlayerOps.ts:771 —
// check(state.popInt(), NumberNotNull) on the uid slot. uid=-1 must
// abort after hitType passes, with no UID lookup or ApplyDamage.
// Closes h-player-2 (audit row 247).
func TestDamage_NullUIDRejected(t *testing.T) {
	target := &mockPlayer{uidValue: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: target}}
	// uid=-1 (null sentinel), hitType=1 (valid), amount=5 (valid)
	s := newDamageState(mw, -1, 1, 5)
	err := handleDamage(s)
	if err == nil {
		t.Fatalf("handleDamage: want error for uid=-1, got nil (TS PlayerOps.ts:771 NumberNotNull must reject null uid)")
	}
	want := "DAMAGE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if got := len(target.applyDamageCalls); got != 0 {
		t.Errorf("applyDamageCalls: got %d, want 0 (must not damage on null uid)", got)
	}
}

// --- NAI-127 Bundle 2: GENDER (opcode 2020) ---

// newGenderState builds a state with Self set; deliberately does NOT
// set PtrActivePlayer to pin TS quirk (no checkedHandler at
// PlayerOps.ts:968-970) per ts_asymmetry_dual_pin.
func newGenderState(self *mockPlayer) *ScriptState {
	return &ScriptState{
		Self:        self,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
}

func TestGender_Male(t *testing.T) {
	self := &mockPlayer{genderValue: 0}
	s := newGenderState(self)
	if err := handleGender(s); err != nil {
		t.Fatalf("GENDER male: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("GENDER male: pushed %d, want 0", got)
	}
}

func TestGender_Female(t *testing.T) {
	self := &mockPlayer{genderValue: 1}
	s := newGenderState(self)
	if err := handleGender(s); err != nil {
		t.Fatalf("GENDER female: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("GENDER female: pushed %d, want 1", got)
	}
}

// --- NAI-127 Bundle 2: P_PREVENTLOGOUT (opcode 2084) ---

// newPreventLogoutState builds a state with Self + protected flag +
// ticks/msg pre-pushed. Push order matches handler pop order: msg
// pushed FIRST (popped last) and ticks pushed LAST (popped first), so
// PopInt returns ticks and PopString returns msg.
func newPreventLogoutState(self *mockPlayer, mw WorldVars, msg string, ticks int, protect bool) *ScriptState {
	s := &ScriptState{
		World:       mw,
		Self:        self,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if protect {
		s.Pointers |= PtrProtectedActivePlayer
	}
	s.PushString(msg)
	s.PushInt(ticks)
	return s
}

func TestPPreventLogout_HappyPath(t *testing.T) {
	self := &mockPlayer{}
	mw := &mockWorld{tick: 100}
	s := newPreventLogoutState(self, mw, "Combat", 16, true)
	if err := handlePPreventLogout(s); err != nil {
		t.Fatalf("P_PREVENTLOGOUT happy: err=%v", err)
	}
	if self.preventLogoutMessage != "Combat" {
		t.Errorf("P_PREVENTLOGOUT happy: msg=%q, want %q", self.preventLogoutMessage, "Combat")
	}
	if self.preventLogoutUntil != 116 {
		t.Errorf("P_PREVENTLOGOUT happy: until=%d, want 116", self.preventLogoutUntil)
	}
}

func TestPPreventLogout_RequiresProtected(t *testing.T) {
	self := &mockPlayer{}
	mw := &mockWorld{tick: 100}
	s := newPreventLogoutState(self, mw, "Combat", 16, false) // not protected
	if err := handlePPreventLogout(s); err == nil {
		t.Fatalf("P_PREVENTLOGOUT not-protected: want error, got nil")
	}
}

func TestPPreventLogout_NoActivePlayer(t *testing.T) {
	mw := &mockWorld{tick: 100}
	s := &ScriptState{
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no PtrActivePlayer
	s.PushString("Combat")
	s.PushInt(16)
	if err := handlePPreventLogout(s); err == nil {
		t.Fatalf("P_PREVENTLOGOUT no-active-player: want error, got nil")
	}
}

// TestPPreventLogout_ValidatesArgs pins M13: TS validates the message with
// StringNotNull and the tick count with NumberNotNull (PlayerOps.ts
// P_PREVENTLOGOUT). goscape previously skipped both.
func TestPPreventLogout_ValidatesArgs(t *testing.T) {
	t.Run("null_message_rejected", func(t *testing.T) {
		self := &mockPlayer{}
		mw := &mockWorld{tick: 100}
		s := newPreventLogoutState(self, mw, "", 16, true) // empty string = null
		if err := handlePPreventLogout(s); err == nil {
			t.Fatal("null message: want error (StringNotNull), got nil")
		}
		if self.preventLogoutMessage != "" || self.preventLogoutUntil != 0 {
			t.Error("null message: handler must not mutate player state on validation failure")
		}
	})

	t.Run("null_ticks_rejected", func(t *testing.T) {
		self := &mockPlayer{}
		mw := &mockWorld{tick: 100}
		s := newPreventLogoutState(self, mw, "Combat", -1, true) // -1 = null number
		if err := handlePPreventLogout(s); err == nil {
			t.Fatal("null ticks: want error (NumberNotNull), got nil")
		}
	})
}

// TestHandlePlayerMember_PushesMembersFlag pins TS PlayerOps.ts:1211-1213
// (NAI-149). Both branches: members=true → 1, members=false → 0.
func TestHandlePlayerMember_PushesMembersFlag(t *testing.T) {
	cases := []struct {
		name string
		seed bool
		want int
	}{
		{"members=true pushes 1", true, 1},
		{"members=false pushes 0", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{membersValue: tc.seed}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			if err := handlePlayerMember(s); err != nil {
				t.Fatalf("handlePlayerMember: %v", err)
			}
			if s.ISP != 1 {
				t.Fatalf("ISP: got %d, want 1", s.ISP)
			}
			if got := s.IntStack[0]; got != tc.want {
				t.Errorf("top of int stack: got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestHandlePlayerMember_RequiresActivePlayer pins the ActivePlayer guard
// (TS checkedHandler(ActivePlayer, ...)).
func TestHandlePlayerMember_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	err := handlePlayerMember(s)
	if err == nil {
		t.Fatalf("handlePlayerMember: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "PLAYERMEMBER") {
		t.Errorf("error: got %q, want to contain \"PLAYERMEMBER\"", err.Error())
	}
}

// TestHandleAfkEvent_NodeDebugForcesEligible pins TS PlayerOps.ts:1058
// — the `Environment.NODE_DEBUG ||` arm short-circuits the staff-mod
// gate. Even with staffModLevel >= 2, NodeDebug=true + ready=true → push 1.
func TestHandleAfkEvent_NodeDebugForcesEligible(t *testing.T) {
	mp := &mockPlayer{
		staffModLevelValue: 5, // would normally suppress
		afkEventReadyValue: true,
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
		NodeDebug:   true,
	}
	if err := handleAfkEvent(s); err != nil {
		t.Fatalf("handleAfkEvent: %v", err)
	}
	if got := s.IntStack[0]; got != 1 {
		t.Errorf("top: got %d, want 1 (NodeDebug forces eligible)", got)
	}
	// Both branches must clear afkEventReady.
	if mp.afkEventReadyValue != false {
		t.Errorf("afkEventReadyValue: got true, want false (always cleared)")
	}
}

// TestHandleAfkEvent_StaffModSuppressesUnderProduction pins TS PlayerOps.ts:1058
// asymmetry — under NodeDebug=false, staffMod>=2 forces 0 even when ready=true.
func TestHandleAfkEvent_StaffModSuppressesUnderProduction(t *testing.T) {
	mp := &mockPlayer{
		staffModLevelValue: 2, // boundary — TS uses `< 2`
		afkEventReadyValue: true,
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
		NodeDebug:   false,
	}
	if err := handleAfkEvent(s); err != nil {
		t.Fatalf("handleAfkEvent: %v", err)
	}
	if got := s.IntStack[0]; got != 0 {
		t.Errorf("top: got %d, want 0 (staffMod=2 suppresses)", got)
	}
	// Asymmetry-pin: clearing happens REGARDLESS of eligibility.
	if mp.afkEventReadyValue != false {
		t.Errorf("afkEventReadyValue: got true, want false (cleared even when not eligible)")
	}
}

// TestHandleAfkEvent_NotReadyPushesZero pins the `&& afkEventReady`
// short-circuit at TS PlayerOps.ts:1058.
func TestHandleAfkEvent_NotReadyPushesZero(t *testing.T) {
	mp := &mockPlayer{
		staffModLevelValue: 0,     // eligible
		afkEventReadyValue: false, // but not ready
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
		NodeDebug:   true,
	}
	if err := handleAfkEvent(s); err != nil {
		t.Fatalf("handleAfkEvent: %v", err)
	}
	if got := s.IntStack[0]; got != 0 {
		t.Errorf("top: got %d, want 0 (not ready)", got)
	}
}

// TestHandleAfkEvent_RequiresActivePlayer pins the goscape-only
// defensive guard (TS skips this check; see defensive_gate_doc_comment_label).
func TestHandleAfkEvent_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	err := handleAfkEvent(s)
	if err == nil {
		t.Fatalf("handleAfkEvent: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "AFK_EVENT") {
		t.Errorf("error: got %q, want to contain \"AFK_EVENT\"", err.Error())
	}
}

// TestHandleWeight_PushesRunWeight pins TS PlayerOps.ts:1180-1182
// (NAI-149). Tests both zero and non-zero weights.
func TestHandleWeight_PushesRunWeight(t *testing.T) {
	cases := []struct {
		name string
		seed int
	}{
		{"zero weight", 0},
		{"non-zero weight (4500g)", 4500},
		{"negative weight (light items)", -1500}, // TS allows negative
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{runweightValue: tc.seed}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer | PtrProtectedActivePlayer,
			}
			if err := handleWeight(s); err != nil {
				t.Fatalf("handleWeight: %v", err)
			}
			if got := s.IntStack[0]; got != tc.seed {
				t.Errorf("top: got %d, want %d", got, tc.seed)
			}
		})
	}
}

// TestHandleWeight_RequiresProtected pins TS checkedHandler(ProtectedActivePlayer).
func TestHandleWeight_RequiresProtected(t *testing.T) {
	mp := &mockPlayer{runweightValue: 100}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer, // active, but NOT protected
	}
	err := handleWeight(s)
	if err == nil {
		t.Fatalf("handleWeight: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "WEIGHT") {
		t.Errorf("error: got %q, want to contain \"WEIGHT\"", err.Error())
	}
	if !strings.Contains(err.Error(), "not protected") {
		t.Errorf("error: got %q, want to contain \"not protected\"", err.Error())
	}
}

// TestHandleHealEnergy_AddsAndClamps pins TS PlayerOps.ts:1050-1054 —
// runenergy clamped to [0, 10000] after add. Mirrors TS Math.min/max.
func TestHandleHealEnergy_AddsAndClamps(t *testing.T) {
	cases := []struct {
		name        string
		startEnergy int
		amount      int
		want        int
	}{
		{"normal add", 5000, 1000, 6000},
		{"clamps to 10000 ceiling", 9500, 1000, 10000},
		{"clamps to 0 floor on negative", 200, -500, 0},
		{"max+max stays at 10000", 10000, 10000, 10000},
		{"exact 10000 from below", 9000, 1000, 10000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{runenergyValue: tc.startEnergy}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			s.PushInt(tc.amount)
			if err := handleHealEnergy(s); err != nil {
				t.Fatalf("handleHealEnergy: %v", err)
			}
			if mp.runenergyValue != tc.want {
				t.Errorf("runenergy: got %d, want %d", mp.runenergyValue, tc.want)
			}
		})
	}
}

// TestHandleHealEnergy_RejectsNullAmount pins TS check(amount, NumberNotNull).
// amount=-1 is the script null sentinel.
func TestHandleHealEnergy_RejectsNullAmount(t *testing.T) {
	mp := &mockPlayer{runenergyValue: 5000}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(-1)
	err := handleHealEnergy(s)
	if err == nil {
		t.Fatalf("handleHealEnergy: expected error for amount=-1")
	}
	if !strings.Contains(err.Error(), "HEAL_ENERGY") {
		t.Errorf("error: got %q, want to contain \"HEAL_ENERGY\"", err.Error())
	}
	// No write on error path.
	if mp.runenergyValue != 5000 {
		t.Errorf("runenergy: got %d, want 5000 (unchanged on error)", mp.runenergyValue)
	}
}

// TestHandleHealEnergy_RequiresActivePlayer pins the goscape-only
// defensive guard (TS skips this check).
func TestHandleHealEnergy_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(100) // amount, will be dead pop on error path
	err := handleHealEnergy(s)
	if err == nil {
		t.Fatalf("handleHealEnergy: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HEAL_ENERGY") {
		t.Errorf("error: got %q, want to contain \"HEAL_ENERGY\"", err.Error())
	}
}

// TestHandleSetSkinColour_WritesColors4 pins TS PlayerOps.ts:1121-1124
// — colors[4] = skin (inclusive [0, 7]).
func TestHandleSetSkinColour_WritesColors4(t *testing.T) {
	cases := []struct {
		name string
		skin int
	}{
		{"min boundary 0", 0},
		{"mid 3", 3},
		{"max boundary 7", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			s.PushInt(tc.skin)
			if err := handleSetSkinColour(s); err != nil {
				t.Fatalf("handleSetSkinColour: %v", err)
			}
			if got := mp.colorParts[4]; got != tc.skin {
				t.Errorf("colorParts[4]: got %d, want %d", got, tc.skin)
			}
			// Other slots must NOT be touched.
			for i, c := range mp.colorParts {
				if i == 4 {
					continue
				}
				if c != 0 {
					t.Errorf("colorParts[%d]: got %d, want 0 (only slot 4 written)", i, c)
				}
			}
		})
	}
}

// TestHandleSetSkinColour_RejectsOutOfRange pins TS check(skin, SkinColourValid)
// — inclusive [0, 7]. Tests both off-by-one boundaries.
func TestHandleSetSkinColour_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		skin int
	}{
		{"-1 below min", -1},
		{"8 above max", 8},
		{"large negative", -100},
		{"large positive", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			s.PushInt(tc.skin)
			err := handleSetSkinColour(s)
			if err == nil {
				t.Fatalf("handleSetSkinColour(%d): expected error, got nil", tc.skin)
			}
			if !strings.Contains(err.Error(), "SETSKINCOLOUR") {
				t.Errorf("error: got %q, want to contain \"SETSKINCOLOUR\"", err.Error())
			}
			// No write on error.
			if mp.colorParts[4] != 0 {
				t.Errorf("colorParts[4]: got %d, want 0 (no write on error)", mp.colorParts[4])
			}
		})
	}
}

// TestHandleSetSkinColour_RequiresActivePlayer pins the goscape-only
// defensive guard.
func TestHandleSetSkinColour_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(3)
	err := handleSetSkinColour(s)
	if err == nil {
		t.Fatalf("handleSetSkinColour: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "SETSKINCOLOUR") {
		t.Errorf("error: got %q, want to contain \"SETSKINCOLOUR\"", err.Error())
	}
}

// TestHandleSetGender_RequiresActivePlayer pins the goscape-only
// defensive active-player guard. TS skips this check; the guard
// follows the defensive_gate_doc_comment_label convention.
func TestHandleSetGender_RequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	err := handleSetGender(s)
	if err == nil {
		t.Fatalf("handleSetGender: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "SETGENDER") {
		t.Errorf("error: got %q, want to contain \"SETGENDER\"", err.Error())
	}
}

// TestHandleSetGender_RejectsOutOfRange pins TS check(gender, GenderValid)
// — inclusive [0, 1]. Tests both off-by-one boundaries.
func TestHandleSetGender_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name   string
		gender int
	}{
		{"-1 below min", -1},
		{"2 above max", 2},
		{"large negative", -100},
		{"large positive", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Self:        mp,
				Pointers:    PtrActivePlayer,
			}
			s.PushInt(tc.gender)
			err := handleSetGender(s)
			if err == nil {
				t.Fatalf("handleSetGender(%d): expected error, got nil", tc.gender)
			}
			if !strings.Contains(err.Error(), "SETGENDER") {
				t.Errorf("error: got %q, want to contain \"SETGENDER\"", err.Error())
			}
			if len(mp.setGenderCalls) != 0 {
				t.Errorf("setGenderCalls: got %v, want empty (no dispatch on validator error)", mp.setGenderCalls)
			}
		})
	}
}

// TestHandleSetGender_DispatchesToSetter pins the happy-path dispatch.
// PopInt + checkGender(1) + s.Self.SetGender(1).
func TestHandleSetGender_DispatchesToSetter(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(1)
	if err := handleSetGender(s); err != nil {
		t.Fatalf("handleSetGender: %v", err)
	}
	if got, want := mp.setGenderCalls, []int{1}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("setGenderCalls: got %v, want %v", got, want)
	}
	if s.ISP != 0 {
		t.Errorf("ISP: got %d, want 0 (stack should be fully drained)", s.ISP)
	}
}

// TestHandleSetGender_AcceptsZeroEdge pins the lower boundary of the
// inclusive [0, 1] range. Mirrors the predecessor slice's boundary-pin
// pattern (the *AcceptsZeroEdge / *RejectsTwenty test convention used
// across boundary-validator handler tests).
func TestHandleSetGender_AcceptsZeroEdge(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(0)
	if err := handleSetGender(s); err != nil {
		t.Fatalf("handleSetGender(0): %v", err)
	}
	if got, want := mp.setGenderCalls, []int{0}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("setGenderCalls: got %v, want %v", got, want)
	}
}

// TestPExactMove pins OpPExactMove's body: pop 5 ints (top-down: dir,
// endCycle, startCycle, end, start), unpack two coords via CoordValid,
// call UnsetMapFlag(), then ExactMove(sX, sZ, eX, eZ, begin, finish, dir).
// Mirrors TS PlayerOps.ts:881-890. NAI-160 T4.
//
// Per handler_pop_order_test_masking.md, the 5 push values are all
// distinct so a pop-order regression mis-binds at least one slot.
func TestPExactMove(t *testing.T) {
	mp := &mockPlayer{}
	startPacked := coordgrid.PackCoord(0, 3200, 3300)
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		// Push order matches TS popInts(5) source order:
		// [start, end, startCycle, endCycle, direction]
		IntOperands:      []int32{int32(startPacked), int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.unsetMapFlagCalls; got != 1 {
		t.Errorf("unsetMapFlagCalls: got %d, want 1", got)
	}
	if got := len(mp.exactMoveCalls); got != 1 {
		t.Fatalf("exactMoveCalls: got %d, want 1", got)
	}
	c := mp.exactMoveCalls[0]
	if c.sX != 3200 || c.sZ != 3300 {
		t.Errorf("start coord: got (sX=%d, sZ=%d), want (3200, 3300)", c.sX, c.sZ)
	}
	if c.eX != 3205 || c.eZ != 3308 {
		t.Errorf("end coord: got (eX=%d, eZ=%d), want (3205, 3308)", c.eX, c.eZ)
	}
	if c.begin != 11 || c.finish != 22 || c.dir != 3 {
		t.Errorf("cycle/dir: got (begin=%d, finish=%d, dir=%d), want (11, 22, 3)",
			c.begin, c.finish, c.dir)
	}
}

// TestPExactMoveRequiresProtected pins the ProtectedActivePlayer gate.
// NAI-160 T4.
func TestPExactMoveRequiresProtected(t *testing.T) {
	mp := &mockPlayer{}
	startPacked := coordgrid.PackCoord(0, 3200, 3300)
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove_unprotected,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		IntOperands:      []int32{int32(startPacked), int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_EXACTMOVE: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_EXACTMOVE") || !strings.Contains(got, "script not protected") {
		t.Errorf("err: got %q, want substrings 'P_EXACTMOVE' and 'script not protected'", got)
	}
	if got := mp.unsetMapFlagCalls; got != 0 {
		t.Errorf("unsetMapFlagCalls: got %d, want 0 (gate must fire before side effects)", got)
	}
}

// TestPExactMoveInvalidCoord pins checkCoord's rejection. NAI-160 T4.
func TestPExactMoveInvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove_badcoord,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		IntOperands:      []int32{-1, int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_EXACTMOVE: coord out of range")
	}
	if got := err.Error(); !strings.Contains(got, "P_EXACTMOVE") || !strings.Contains(got, "coord out of range") {
		t.Errorf("err: got %q, want substrings 'P_EXACTMOVE' and 'coord out of range'", got)
	}
	if got := mp.unsetMapFlagCalls; got != 0 {
		t.Errorf("unsetMapFlagCalls: got %d, want 0 (validation must precede side effects)", got)
	}
}

// TestGetQueueReturnsSeededCount pins OpGetQueue: pop a scriptID,
// push ActivePlayer.QueueCount(scriptID). Mirrors TS
// PlayerOps.ts:903-912. NAI-161 T5.
func TestGetQueueReturnsSeededCount(t *testing.T) {
	mp := &mockPlayer{
		queueCountByScript: map[int]int{7: 3},
	}
	sf := &ScriptFile{
		Name: "[getqueue,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=7
			OpGetQueue,
			OpReturn,
		},
		IntOperands:      []int32{7, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 3 {
		t.Errorf("GETQUEUE: got %d, want 3", got)
	}
}

// TestGetQueueNoMatchReturnsZero pins zero-result behavior: an
// unmapped scriptID returns the Go zero-value of int via the mock's
// nil-map read. Mirrors TS finding zero loop iterations. NAI-161 T5.
func TestGetQueueNoMatchReturnsZero(t *testing.T) {
	mp := &mockPlayer{} // queueCountByScript is nil
	sf := &ScriptFile{
		Name: "[getqueue_zero,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=99
			OpGetQueue,
			OpReturn,
		},
		IntOperands:      []int32{99, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("GETQUEUE no-match: got %d, want 0", got)
	}
}

// TestPOpHeldUnimplemented pins OpPOpHeld's TS-faithful
// 'unimplemented' error stub. Protected gate passes (both pointer
// flags set), then handler returns the unimplemented error.
// Mirrors TS PlayerOps.ts:381-383
// (`checkedHandler(ProtectedActivePlayer, () => { throw new Error('unimplemented'); })`).
// NAI-161 T6 — deviation NAI-161-D-POPHELD-STUB.
func TestPOpHeldUnimplemented(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("[p_opheld,test]", OpPOpHeld)
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_OPHELD: unimplemented")
	}
	if got := err.Error(); !strings.Contains(got, "P_OPHELD") || !strings.Contains(got, "unimplemented") {
		t.Errorf("err: got %q, want substrings 'P_OPHELD' and 'unimplemented'", got)
	}
}

// TestPOpHeldRequiresProtected pins gate-ordering: the
// ProtectedActivePlayer check fires BEFORE the unimplemented stub.
// Without the protect flag, the error is "script not protected",
// not "unimplemented". NAI-161 T6.
func TestPOpHeldRequiresProtected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("[p_opheld_unprotected,test]", OpPOpHeld)
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_OPHELD: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_OPHELD") || !strings.Contains(got, "script not protected") {
		t.Errorf("err: got %q, want substrings 'P_OPHELD' and 'script not protected'", got)
	}
	if got := err.Error(); strings.Contains(got, "unimplemented") {
		t.Errorf("err: got %q, must NOT contain 'unimplemented' — protected gate must fire first", got)
	}
}

// TestClearQueueDispatch pins OpClearQueue: pop the scriptID arg,
// delegate to ActivePlayer.UnlinkQueuedScript. Mirrors TS
// PlayerOps.ts:1045-1048. NAI-161 T4.
func TestClearQueueDispatch(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "[clearqueue,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=42
			OpClearQueue,
			OpReturn,
		},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.unlinkScriptCalls; len(got) != 1 || got[0] != 42 {
		t.Errorf("unlinkScriptCalls: got %v, want [42]", got)
	}
}

// TestHandlePOpPlayerT_Happy pins: protected gate set, Self2 present,
// spellId pops, StopAction + SetInteractionScriptPlayer fire.
// Mirrors TS PlayerOps.ts:1127-1135.
func TestHandlePOpPlayerT_Happy(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer | PtrActivePlayer2,
	}
	s.PushInt(1234) // spellId

	if err := handlePOpPlayerT(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("StopAction: got %d calls, want 1", mp.stopActionCalls)
	}
	if len(mp.lastSetInteractionScriptPlayer) != 1 {
		t.Fatalf("SetInteractionScriptPlayer: got %d, want 1", len(mp.lastSetInteractionScriptPlayer))
	}
	got := mp.lastSetInteractionScriptPlayer[0]
	if got.player2 != mp2 || got.op != 1234 {
		t.Errorf("call args: got {%v %d}, want {%v 1234}", got.player2, got.op, mp2)
	}
}

// TestHandlePOpPlayerT_NilSelf2 pins TS PlayerOps.ts:1130-1132 silent
// return: no error, no StopAction, no SetInteraction when Self2 absent.
func TestHandlePOpPlayerT_NilSelf2(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Self2:       nil,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer, // no PtrActivePlayer2
	}
	s.PushInt(1234)

	if err := handlePOpPlayerT(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("StopAction: got %d, want 0 (silent return)", mp.stopActionCalls)
	}
	if len(mp.lastSetInteractionScriptPlayer) != 0 {
		t.Errorf("SetInteraction calls: got %d, want 0", len(mp.lastSetInteractionScriptPlayer))
	}
}

// TestHandlePOpPlayerT_NotProtected pins the protected-gate error.
func TestHandlePOpPlayerT_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer, // not protected
	}
	s.PushInt(1234)
	if err := handlePOpPlayerT(s); err == nil {
		t.Fatal("expected error when not protected")
	}
}

// mergeLocWorld extends *mockWorld to record MergeLoc calls.
// Used by TestHandlePLocMerge_*.
type mergeLocWorld struct {
	*mockWorld
	calls []mergeLocCall
}

type mergeLocCall struct {
	loc        ActiveLoc
	player     ActivePlayer
	StartCycle int
	EndCycle   int
	South      int
	East       int
	North      int
	West       int
}

func (m *mergeLocWorld) MergeLoc(loc ActiveLoc, player ActivePlayer, startCycle, endCycle, south, east, north, west int) {
	m.calls = append(m.calls, mergeLocCall{
		loc:        loc,
		player:     player,
		StartCycle: startCycle,
		EndCycle:   endCycle,
		South:      south,
		East:       east,
		North:      north,
		West:       west,
	})
}

// packTestCoord packs (level, x, z) into the RS2 coord int used by checkCoord.
// Matches TS CoordGrid.packCoord: (level<<28)|(x<<14)|z.
func packTestCoord(level, x, z int) int {
	return (level << 28) | (x << 14) | z
}

// TestHandlePLocMerge_Happy pins World.MergeLoc dispatch with argument
// unpacking from TS popInts(4) → [startCycle, endCycle, se, nw].
func TestHandlePLocMerge_Happy(t *testing.T) {
	mp := &mockPlayer{}
	mw := &mergeLocWorld{mockWorld: newMockWorld()}
	loc := fakeActiveLoc{id: 1, x: 3200, z: 3200, level: 0}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		World:       mw,
		ActiveLoc:   loc,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer | PtrActiveLoc,
	}
	// LIFO push order: startCycle, endCycle, southEast, northWest
	// pop order: northWest, southEast, endCycle, startCycle
	s.PushInt(10)                           // startCycle (deepest)
	s.PushInt(50)                           // endCycle
	s.PushInt(packTestCoord(0, 3200, 3200)) // southEast: x=3200(east), z=3200(south)
	s.PushInt(packTestCoord(0, 3210, 3210)) // northWest: x=3210(west), z=3210(north)

	if err := handlePLocMerge(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mw.calls) != 1 {
		t.Fatalf("MergeLoc: got %d calls, want 1", len(mw.calls))
	}
	got := mw.calls[0]
	if got.StartCycle != 10 || got.EndCycle != 50 {
		t.Errorf("cycles: got {%d,%d}, want {10,50}", got.StartCycle, got.EndCycle)
	}
	// se.z=south=3200, se.x=east=3200, nw.z=north=3210, nw.x=west=3210
	if got.South != 3200 || got.East != 3200 || got.North != 3210 || got.West != 3210 {
		t.Errorf("rect: got s=%d e=%d n=%d w=%d, want s=3200 e=3200 n=3210 w=3210",
			got.South, got.East, got.North, got.West)
	}
}

// TestHandlePLocMerge_InvalidCoord pins checkCoord error on bad southEast.
func TestHandlePLocMerge_InvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	mw := &mergeLocWorld{mockWorld: newMockWorld()}
	loc := fakeActiveLoc{id: 1}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		World:       mw,
		ActiveLoc:   loc,
		Pointers:    PtrActivePlayer | PtrProtectedActivePlayer | PtrActiveLoc,
	}
	s.PushInt(10)
	s.PushInt(50)
	s.PushInt(-1) // bad southEast
	s.PushInt(packTestCoord(0, 3210, 3210))

	if err := handlePLocMerge(s); err == nil {
		t.Fatal("expected error on invalid southEast coord")
	}
}

// TestHandlePLocMerge_NotProtected pins the protected-gate error.
func TestHandlePLocMerge_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	mw := &mergeLocWorld{mockWorld: newMockWorld()}
	loc := fakeActiveLoc{id: 1}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		World:       mw,
		ActiveLoc:   loc,
		Pointers:    PtrActivePlayer, // not protected
	}
	s.PushInt(10)
	s.PushInt(50)
	s.PushInt(packTestCoord(0, 3200, 3200))
	s.PushInt(packTestCoord(0, 3210, 3210))

	if err := handlePLocMerge(s); err == nil {
		t.Fatal("expected error when not protected")
	}
}

// TestHandleWealthEvent_KnownObj pins the happy path: ObjByName resolves;
// AddWealthEvent called with assembled struct. Mirrors TS
// PlayerOps.ts:1191-1202.
func TestHandleWealthEvent_KnownObj(t *testing.T) {
	mp := &mockPlayer{}
	mc := newTestConfigs()
	whip := objtype.NewObjType(4151)
	whip.DebugName = "abyssal_whip"
	mc.objs[4151] = whip
	if mc.objsByName == nil {
		mc.objsByName = make(map[string]*objtype.ObjType)
	}
	mc.objsByName["abyssal_whip"] = whip

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Configs:     mc,
		Pointers:    PtrActivePlayer,
	}
	// Push order: name (string), eventType, count, value (ints).
	// Pop order LIFO: value, count, eventType → PopInt ×3; name → PopString.
	s.PushString("abyssal_whip")
	s.PushInt(WealthEventTypeDrop) // eventType
	s.PushInt(1)                   // count
	s.PushInt(120000)              // value

	if err := handleWealthEvent(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1", len(mp.addWealthEventCalls))
	}
	got := mp.addWealthEventCalls[0]
	if got.EventType != WealthEventTypeDrop {
		t.Errorf("EventType: got %d, want %d", got.EventType, WealthEventTypeDrop)
	}
	if got.AccountValue != 120000 {
		t.Errorf("AccountValue: got %d, want 120000", got.AccountValue)
	}
	if len(got.AccountItems) != 1 ||
		got.AccountItems[0].ID != 4151 ||
		got.AccountItems[0].Name != "abyssal_whip" ||
		got.AccountItems[0].Count != 1 {
		t.Errorf("AccountItems: got %+v, want [{ID:4151 Name:abyssal_whip Count:1}]", got.AccountItems)
	}
}

// TestHandleWealthEvent_UnknownObj pins ObjByName→nil path:
// AccountItems[0].ID == -1 (TS `objType?.id` undefined ≡ goscape -1).
func TestHandleWealthEvent_UnknownObj(t *testing.T) {
	mp := &mockPlayer{}
	mc := newTestConfigs()
	// no objsByName entry for "unknown_obj"

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Configs:     mc,
		Pointers:    PtrActivePlayer,
	}
	s.PushString("unknown_obj")
	s.PushInt(WealthEventTypeDrop)
	s.PushInt(1)
	s.PushInt(0)

	if err := handleWealthEvent(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1", len(mp.addWealthEventCalls))
	}
	got := mp.addWealthEventCalls[0]
	if len(got.AccountItems) != 1 || got.AccountItems[0].ID != -1 {
		t.Errorf("AccountItems[0].ID: got %v, want -1", got.AccountItems)
	}
}

// TestHandleLastLoginInfo pins the single-delegation pattern. No pop,
// no push — handler calls Self.LastLoginInfo and returns. Mirrors TS
// PlayerOps.ts:931-933.
func TestHandleLastLoginInfo(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	if err := handleLastLoginInfo(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.lastLoginInfoCalls != 1 {
		t.Errorf("LastLoginInfo: got %d calls, want 1", mp.lastLoginInfoCalls)
	}
}

// TestHandlePWalk_RequiresProtectedActivePlayer pins the
// ProtectedActivePlayer gate on P_WALK. Mirrors TS PlayerOps.ts:455
// checkedHandler(ProtectedActivePlayer, …).
func TestHandlePWalk_RequiresProtectedActivePlayer(t *testing.T) {
	mp := &mockPlayer{}
	packed := coordgrid.PackCoord(0, 3210, 3220)
	sf := &ScriptFile{
		Name: "[p_walk_unprotected,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_WALK: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_WALK") || !strings.Contains(got, "script not protected") {
		t.Errorf("err: got %q, want substrings 'P_WALK' and 'script not protected'", got)
	}
	if got := len(mp.walkCalls); got != 0 {
		t.Errorf("walkCalls: got %d, want 0 (gate should reject before dispatch)", got)
	}
}

// TestHandlePWalk_RejectsInvalidCoord pins that checkCoord rejects
// out-of-range packed coords before dispatch. Mirrors TS
// check(state.popInt(), CoordValid).
func TestHandlePWalk_RejectsInvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "[p_walk_badcoord,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		// -1 is outside the valid packed-coord range (level/x/z all OOB).
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want CoordValid rejection")
	}
	if got := err.Error(); !strings.Contains(got, "P_WALK") {
		t.Errorf("err: got %q, want substring 'P_WALK'", got)
	}
	if got := len(mp.walkCalls); got != 0 {
		t.Errorf("walkCalls: got %d, want 0 (coord rejection should precede dispatch)", got)
	}
}

// TestHandlePWalk_DispatchesWalkWithUnpackedXZ pins the happy path:
// gate satisfied + valid packed coord → Self.Walk(destX, destZ) called
// once with the unpacked X/Z. Critically pins that the packed coord's
// level component is NOT forwarded — TS uses player.level for the
// pathfinder call (PlayerOps.ts:459).
func TestHandlePWalk_DispatchesWalkWithUnpackedXZ(t *testing.T) {
	mp := &mockPlayer{}
	packed := coordgrid.PackCoord(0, 3210, 3220)
	sf := &ScriptFile{
		Name: "[p_walk,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.Execution; got != Finished {
		t.Errorf("Execution: got %v, want Finished", got)
	}
	if got := state.ISP; got != 0 {
		t.Errorf("ISP: got %d, want 0", got)
	}
	if got := len(mp.walkCalls); got != 1 {
		t.Fatalf("walkCalls: got %d, want 1", got)
	}
	c := mp.walkCalls[0]
	if c.destX != 3210 || c.destZ != 3220 {
		t.Errorf("Walk dispatch: got (destX=%d, destZ=%d), want (3210, 3220)", c.destX, c.destZ)
	}
}

// --- rev-244 B4 Task 3: HUNTALL/HUNTNEXT re-pointed to huntIterator ------

// TestHuntAll_StoresIntoHuntIterator pins that HUNTALL writes to
// s.huntIterator (not the old playerIterator field) after the rev-244
// unification. Mirrors TS ServerOps.ts:53-61 at pin 9aadcec4.
func TestHuntAll_StoresIntoHuntIterator(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3200
	s := newHuntAllState(t, coord, 10, objtype.HuntVisLineOfSight, &mockPlayerLookup{})
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	it, ok := s.huntIterator.(*PlayerIterator)
	if !ok || it == nil {
		t.Fatal("huntIterator should hold a *PlayerIterator after HUNTALL")
	}
	if it.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %v, want PlayerIteratorHuntAll", it.mode)
	}
}

// TestHuntNext_ConsumesHuntIterator verifies the HUNTALL→HUNTNEXT happy path
// against the unified huntIterator. A seeded player is expected to be yielded.
func TestHuntNext_ConsumesHuntIterator(t *testing.T) {
	target := &mockPlayer{username: "Hit2", x: 3204, z: 3204}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, 3216, 3216}: {target},
		},
	}
	coord := (0 << 28) | (3200 << 14) | 3200
	s := newHuntAllState(t, coord, 10, objtype.HuntVisOff, lookup)
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("HUNTNEXT = %d, want 1", got)
	}
	if s.Self != target {
		t.Fatalf("Self: got %v, want target %v", s.Self, target)
	}
}
