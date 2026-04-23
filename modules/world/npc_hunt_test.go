package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// newConsumeHuntTargetFixture builds a Server + Npc ready to exercise
// consumeHuntTarget: s.huntTypes is populated with a single hunt config
// at slot 0, n.huntMode = 0, and n.huntTarget is left for the caller to
// set (a nil-target test should leave it nil).
//
// The returned hunt config has Type=HuntModeNpc, Rate=1, FindKeepHunting=true,
// FindNewMode=NPCModeNone (interaction-branch default). Tests mutate these
// fields in place to exercise specific branches.
func newConsumeHuntTargetFixture(t *testing.T) (*Server, *Npc, *objtype.HuntType) {
	t.Helper()
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	hunt := &objtype.HuntType{
		ConfigType:      objtype.ConfigType{ID: 0},
		Type:            objtype.HuntModeNpc,
		Rate:            1,
		FindKeepHunting: true,
		FindNewMode:     objtype.NPCModeNone,
	}
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{hunt},
	}
	n.huntMode = 0
	return s, n, hunt
}

func TestConsumeHuntTargetNilHuntTargetNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	n.huntTarget = nil
	n.target = nil
	n.targetOp = 99
	n.huntClock = 7
	n.huntMode = 5

	s.consumeHuntTarget(n)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (no-op expected)", n.target)
	}
	if n.targetOp != 99 {
		t.Errorf("targetOp: got %d, want 99 (unchanged)", n.targetOp)
	}
	if n.huntClock != 7 {
		t.Errorf("huntClock: got %d, want 7 (unchanged)", n.huntClock)
	}
	if n.huntMode != 5 {
		t.Errorf("huntMode: got %d, want 5 (unchanged)", n.huntMode)
	}
}

func TestConsumeHuntTargetHuntModeOffNoOp(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	n.huntClock = 3
	hunt.Type = objtype.HuntModeOff

	s.consumeHuntTarget(n)

	if n.huntTarget != other {
		t.Errorf("huntTarget: got %v, want unchanged (HuntModeOff gate)", n.huntTarget)
	}
	if n.huntClock != 3 {
		t.Errorf("huntClock: got %d, want 3 (unchanged)", n.huntClock)
	}
}

func TestConsumeHuntTargetInvalidHuntModeNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)

	// Case 1: huntMode = -1 (below lower bound).
	n.huntTarget = other
	n.huntMode = -1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=-1: huntTarget should be unchanged, got %v", n.huntTarget)
	}

	// Case 2: huntMode = len(Configs) (above upper bound).
	n.huntTarget = other
	n.huntMode = len(s.huntTypes.Configs) // == 1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=OOB: huntTarget should be unchanged, got %v", n.huntTarget)
	}
}

func TestConsumeHuntTargetInteractionBranchSetsTarget(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = 4 // PLAYERFOLLOW — not in QUEUE1..20 range

	s.consumeHuntTarget(n)

	if n.target != other {
		t.Errorf("target: got %v, want %v (interaction branch)", n.target, other)
	}
	if n.targetOp != 4 {
		t.Errorf("targetOp: got %d, want 4 (PLAYERFOLLOW)", n.targetOp)
	}
}

func TestConsumeHuntTargetInteractionBranchClearsHuntState(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	n.huntClock = 5
	hunt.FindNewMode = 4

	s.consumeHuntTarget(n)

	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (cleared after consume)", n.huntTarget)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset after consume)", n.huntClock)
	}
}

// newQueueBranchFixture extends the consumeHuntTarget fixture with
// a scriptProvider wired for AI_QUEUE dispatch. Returns (s, n, hunt,
// provider) so the test can register specific trigger scripts.
//
// The NPC's type (created by newNpcForLifecycleTest) has typeId=0
// and category=-1; scripts must be registered at (trigger, 0)
// specifically for GetByTrigger to find them.
func newQueueBranchFixture(t *testing.T) (
	*Server, *Npc, *objtype.HuntType, *script.Provider,
) {
	t.Helper()
	s, n, hunt := newConsumeHuntTargetFixture(t)
	s.scriptProvider = script.NewProvider()
	return s, n, hunt, s.scriptProvider
}

// buildTimerSetScript creates a script that sets n.timerInterval to a
// specific value — used as a dispatch observer. Post-run, asserting
// n.timerInterval equals `val` proves the script fired (and therefore
// that the correct TriggerAiQueueN was dispatched).
//
// Body: OpPushConstantInt (push val), OpNpcSetTimer (pop, set timer),
// OpReturn. Registered at (trigger, typeID) via LookupKeyForType.
func buildTimerSetScript(t *testing.T, trigger script.ServerTriggerType, typeID int, val int32) *script.ScriptFile {
	t.Helper()
	return &script.ScriptFile{
		Name:             "[queue_branch_observer]",
		LookupKey:        script.LookupKeyForType(trigger, typeID),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpNpcSetTimer, script.OpReturn},
		IntOperands:      []int32{val, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

func TestConsumeHuntTargetQueueBranchFiresScript(t *testing.T) {
	s, n, hunt, provider := newQueueBranchFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue3 // 49 → should fire TriggerAiQueue3

	const observerVal int32 = 12345
	provider.Register(buildTimerSetScript(t, script.TriggerAiQueue3, n.typeId, observerVal))

	s.consumeHuntTarget(n)

	if int32(n.timerInterval) != observerVal {
		t.Errorf("timerInterval: got %d, want %d (script should have fired)",
			n.timerInterval, observerVal)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (common-tail cleanup)", n.huntTarget)
	}
}

func TestConsumeHuntTargetQueueBranchDoesNotSetTarget(t *testing.T) {
	s, n, hunt, _ := newQueueBranchFixture(t)
	preexisting := newNpcForLifecycleTest(t)
	other := newNpcForLifecycleTest(t)
	n.target = preexisting
	n.targetOp = 999
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue3

	// Note: no script registered for TriggerAiQueue3 — runNpcScript is
	// a no-op on nil sf. This is the "happy-path with no registered
	// handler" case; huntTarget cleanup still runs.

	s.consumeHuntTarget(n)

	if n.target != preexisting {
		t.Errorf("target: got %v, want %v (QUEUE branch must NOT set target)",
			n.target, preexisting)
	}
	if n.targetOp != 999 {
		t.Errorf("targetOp: got %d, want 999 (unchanged)", n.targetOp)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (common tail)", n.huntTarget)
	}
}

func TestConsumeHuntTargetQueueBranchBoundaryQueue20(t *testing.T) {
	s, n, hunt, provider := newQueueBranchFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue20 // 66 → should fire TriggerAiQueue20

	const observerVal int32 = 77777
	provider.Register(buildTimerSetScript(t, script.TriggerAiQueue20, n.typeId, observerVal))

	s.consumeHuntTarget(n)

	if int32(n.timerInterval) != observerVal {
		t.Errorf("timerInterval: got %d, want %d (Queue20 dispatch)",
			n.timerInterval, observerVal)
	}
}

func TestConsumeHuntTargetFindKeepHuntingFalseClearsHuntMode(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindKeepHunting = false
	hunt.FindNewMode = 4
	n.huntMode = 0 // pointing at Configs[0], valid entry

	s.consumeHuntTarget(n)

	if n.huntMode != -1 {
		t.Errorf("huntMode: got %d, want -1 (!FindKeepHunting clears it)", n.huntMode)
	}
}

func TestConsumeHuntTargetFindKeepHuntingTrueKeepsHuntMode(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindKeepHunting = true
	hunt.FindNewMode = 4
	n.huntMode = 0

	s.consumeHuntTarget(n)

	if n.huntMode != 0 {
		t.Errorf("huntMode: got %d, want 0 (FindKeepHunting preserves it)", n.huntMode)
	}
}

func TestNpcTurnHuntAndConsumeSetsTarget(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)

	// Prepare the NPC to run a full turn() and reach consumeHuntTarget:
	//   - Avoid the Events block by setting lifecycle to Forever.
	//   - Place the NPC at a known coord with a configured huntRange.
	n.lifecycle = NpcLifecycleForever
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Configure the hunt for NPC-type hunt.
	hunt.Type = objtype.HuntModeNpc
	hunt.Rate = 1
	hunt.FindNewMode = 4 // PLAYERFOLLOW (interaction branch)
	hunt.FindKeepHunting = true
	hunt.NobodyNear = objtype.HuntNobodyNearKeepHunting
	hunt.CheckNpc = -1
	hunt.CheckCategory = -1

	addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)
	s.npcs[1] = n

	// Run the full tick.
	n.turn(s)

	// NAI-11 shift: consumeHuntTarget still sets target+targetOp=4
	// intermediately, but processMovementInteraction (now wired via
	// npc_ai.go:turn) immediately routes PLAYERFOLLOW through the
	// deferred-PLAYER*-mode branch → resetDefaults → target cleared.
	// So the observable post-turn state is target==nil + targetOp back
	// at defaultMode (Wander for a WanderRange-only NpcType).
	//
	// Tracked: nai_followups.md — when PLAYER* modes are implemented,
	// this assertion should flip back to "target is the hunted NPC".
	if n.target != nil {
		t.Errorf("target: got %v, want nil (PLAYER* mode is deferred → resetDefaults)", n.target)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (cleared by consumeHuntTarget)", n.huntTarget)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset by consumeHuntTarget)", n.huntClock)
	}
	if n.huntMode != 0 {
		t.Errorf("huntMode: got %d, want 0 (FindKeepHunting=true preserves it)", n.huntMode)
	}
}

// TestConsumeHuntTargetInteractionBranchCallsSetInteraction verifies the
// NAI-11 closure: consumeHuntTarget's interaction branch now dispatches
// through SetInteraction, writing all seven previously-deferred fields.
func TestConsumeHuntTargetInteractionBranchCallsSetInteraction(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)

	// Use an OP NPC mode so no PLAYER* deferral is triggered downstream.
	hunt.FindNewMode = objtype.NPCModeOpNpc1

	target := &Npc{nid: 7, typeId: 99, x: 105, z: 105, level: 0}
	n.huntTarget = target

	s.consumeHuntTarget(n)

	if n.target != target {
		t.Error("target: not set via SetInteraction")
	}
	if n.targetOp != objtype.NPCModeOpNpc1 {
		t.Errorf("targetOp: got %d, want NPCModeOpNpc1", n.targetOp)
	}
	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (NAI-10 deferral #1 closed)", n.apRange)
	}
	if n.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (NAI-10 deferral #2 closed)")
	}
	if n.targetSubject.typ != target.typeId {
		t.Errorf("targetSubject.typ: got %d, want %d (NAI-10 deferral #3 closed)",
			n.targetSubject.typ, target.typeId)
	}
	if n.faceEntity != target.nid {
		t.Errorf("faceEntity: got %d, want %d (NAI-10 deferral #5 closed)",
			n.faceEntity, target.nid)
	}
}
