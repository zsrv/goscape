package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
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

	hunted := addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)
	s.npcs[1] = n

	// Run the full tick.
	n.turn(s)

	// NAI-13 (restored): PLAYERFOLLOW now dispatches to playerFollowMode
	// (TS Npc.ts:801-812) instead of the NAI-11 resetDefaults stub. The
	// target is preserved across the turn. This is the original pre-NAI-11
	// assertion shape, restored by NAI-13 Task 5.
	if n.target == nil {
		t.Errorf("target: got nil, want the hunted NPC (PLAYERFOLLOW should preserve target)")
	} else if n.target.Slot() != hunted.nid {
		t.Errorf("target: got nid %d, want %d (hunted NPC)", n.target.Slot(), hunted.nid)
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

// playerHuntFixture builds a server + a HuntModePlayer NPC registered in
// s.npcLoop with an in-range, line-of-sight-clear player, ready for
// processNpcHuntPlayers. Caller seeds the observer count.
func playerHuntFixture(t *testing.T) (*Server, *Npc) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	n.huntMode = 0
	n.huntClock = 0
	s.npcLoop = append(s.npcLoop, n)
	s.rsbuf.AddNpc(int32(n.nid), 0)

	_ = addPlayerToServer(t, s, 1, n.x, n.z+2, n.level)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:               objtype.HuntModePlayer,
				Rate:               1,
				CheckVis:           objtype.HuntVisLineOfSight,
				CheckNotCombat:     -1,
				CheckNotCombatSelf: -1,
				CheckInv:           -1,
			},
		},
	}
	return s, n
}

// TestProcessNpcHuntPlayers_AcquiresObservedPlayer pins the core aggression
// fix: the world-level player-hunt pass (TS World.processWorld,
// World.ts:577-589) gives an observed HuntModePlayer NPC a player huntTarget,
// which consumeHuntTarget then turns into an attack. Pre-fix this pass did
// not exist, so aggressive NPCs (ogres, wilderness monsters) never initiated.
func TestProcessNpcHuntPlayers_AcquiresObservedPlayer(t *testing.T) {
	s, n := playerHuntFixture(t)
	s.rsbuf.SetObserverForTest(int32(n.nid), 1)
	defer s.rsbuf.SetObserverForTest(int32(n.nid), 0)

	s.processNpcHuntPlayers()

	if n.huntTarget == nil {
		t.Fatal("huntTarget: got nil, want the observed in-range player (aggression must acquire)")
	}
}

// TestProcessNpcHuntPlayers_SkipsUnobservedNpc guards the observer gate
// (TS World.ts:581 getNpcObservers > 0): an NPC with no nearby observers
// does not scan for players, even with one in range.
func TestProcessNpcHuntPlayers_SkipsUnobservedNpc(t *testing.T) {
	s, n := playerHuntFixture(t)
	s.rsbuf.SetObserverForTest(int32(n.nid), 0)

	s.processNpcHuntPlayers()

	if n.huntTarget != nil {
		t.Error("huntTarget: got non-nil, want nil (unobserved NPC must not hunt players)")
	}
}

// TestProcessNpcHuntPlayers_SkipsDeadNpc guards the isActive gate
// (TS World.ts:607 npc.isActive → goscape `!n.dead`; n.IsValid() is
// intentionally NOT used here — see the processNpcHuntPlayers docstring).
func TestProcessNpcHuntPlayers_SkipsDeadNpc(t *testing.T) {
	s, n := playerHuntFixture(t)
	s.rsbuf.SetObserverForTest(int32(n.nid), 1)
	defer s.rsbuf.SetObserverForTest(int32(n.nid), 0)
	n.dead = true

	s.processNpcHuntPlayers()

	if n.huntTarget != nil {
		t.Error("huntTarget: got non-nil, want nil (dead NPC must not hunt)")
	}
}

func TestHuntPlayersCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServer(t, s, 1, n.x, n.z+2, n.level)

	// Task 1 fixture-ordering convention: seed entity, THEN AllocateIfAbsent.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight, CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntPlayersCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServer(t, s, 1, n.x, n.z+2, n.level)
	withBlockingWall(t, s, 3094, 3107, 0) // mid-tile blocker

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight, CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}

// TestHuntPlayersCheckVisArgumentOrderSwapQuirk guards the TS
// player-as-source swap at ScriptIterators.ts:88-94. TS huntPlayers
// uses player-as-source (opposite of the other three variants'
// NPC-as-source). An asymmetric directional wall blocks the TS-prescribed
// direction but would pass the un-swap — proving the Go call uses
// HasLineOfSight(p.x, p.z, n.x, n.z), NOT the swapped NPC-as-source order.
//
// Fixture rationale (player at n.z+2, NPC at n.z):
//
//	Player→NPC direction: travelSouth — ray checks FlagWallNorth-bit
//	when entering each new tile. FlagWallNorthProjBlocker at mid-tile
//	(3094, 3107) blocks this direction.
//	NPC→player (un-swap): travelNorth — checks FlagWallSouth-bit.
//	FlagWallNorthProjBlocker is NOT in the south mask, so un-swap
//	direction would pass. Test asserts player is FILTERED (want 0).
//
//	If implementer reverts to NPC-as-source, ray passes → player hunted
//	→ test flips red.
func TestHuntPlayersCheckVisArgumentOrderSwapQuirk(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServer(t, s, 1, n.x, n.z+2, n.level)
	// DIRECTIONAL blocker (not withBlockingWall which is bidirectional).
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagWallNorthProjBlocker)

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight, CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 — player-as-source LoS blocked; "+
			"if 1, the src/dest swap is reverted (bug)", len(hunted))
	}
}

// TestHuntPlayersCheckVars guards the CheckVars AND-chain filter at
// TS Npc.ts:950-957. Each entry passes if VarID==-1 OR
// CheckHuntCondition(p.Varp(VarID), Condition, Val). Any failing entry
// excludes the player. Nil/empty CheckVars → no-op.
func TestHuntPlayersCheckVars(t *testing.T) {
	setup := func(t *testing.T, varps []int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = varps
		return s, n, p
	}

	t.Run("nil-checkvars-no-filter", func(t *testing.T) {
		_, n, _ := setup(t, []int32{0})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1} // CheckVars nil
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (nil CheckVars → no filter)", len(hunted))
		}
	})

	t.Run("single-entry-passes", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1, CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (5 > 3 → pass)", len(hunted))
		}
	})

	t.Run("single-entry-fails", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1, CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 10},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (5 > 10 → fail)", len(hunted))
		}
	})

	t.Run("two-entries-both-pass", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5, 7})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1, CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3},
			{VarID: 1, Condition: "=", Val: 7},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (both pass)", len(hunted))
		}
	})

	t.Run("two-entries-second-fails", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5, 7})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1, CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3}, // pass
			{VarID: 1, Condition: "=", Val: 9}, // fail
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (AND-fail: first passes, second fails)", len(hunted))
		}
	})

	t.Run("varid-minus-one-short-circuits", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1, CheckVars: []objtype.HuntCheckVar{
			// VarID == -1 must pass without reading any varp, regardless of
			// the Condition/Val. TS Npc.ts:953 `checkVar.varId === -1 ||` short-circuit.
			{VarID: -1, Condition: ">", Val: 999},
			{VarID: 0, Condition: ">", Val: 3}, // real gate: 5 > 3 → pass
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (VarID=-1 entry skipped, second passes)", len(hunted))
		}
	})
}

// TestHuntPlayersCheckNotCombat guards the 8-tick combat-window filter
// at TS Npc.ts:943-945. When the outer guard applies (see
// TestHuntPlayersCombatGuard), a player whose last-combat varp was
// written within [currentTick-7, currentTick] is filtered; at
// currentTick-8 and earlier, they pass.
func TestHuntPlayersCheckNotCombat(t *testing.T) {
	// Helper: build a Server/Npc/Player with gamemap+non-multi guard so
	// the outer combat guard APPLIES (i.e., the checkNotCombat filter
	// actually runs). varpVal seeds p.varps[0].
	setup := func(t *testing.T, currentTick int, varpVal int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = currentTick
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil // guard applies (target != p) → filter fires
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = []int32{varpVal}
		return s, n, p
	}

	t.Run("default-minus-one-disables", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // varp written this tick
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (CheckNotCombat=-1 disables filter)", len(hunted))
		}
	})

	t.Run("varp-this-tick-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // 100+8 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: 0, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varp==currentTick → within 8-tick window)", len(hunted))
		}
	})

	t.Run("varp-minus-seven-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 93) // 93+8 = 101 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: 0, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varp==currentTick-7 → window-inclusive, filter fires)", len(hunted))
		}
	})

	t.Run("varp-minus-eight-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 92) // 92+8 = 100, 100 > 100 is false → pass
		hunt := &objtype.HuntType{CheckNotCombat: 0, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varp==currentTick-8 → exclusive boundary, filter passes)", len(hunted))
		}
	})

	t.Run("varp-zero-well-past-window-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 0) // fresh player, no combat recorded
		hunt := &objtype.HuntType{CheckNotCombat: 0, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varp==0, currentTick=100 → well past window)", len(hunted))
		}
	})
}

// TestHuntPlayersCombatGuard guards the outer multi-zone/target-equality
// guard at TS Npc.ts:942. When the guard is SKIPPED (target==p OR
// IsMulti(p) returns true), the inner combat filters (checkNotCombat
// and the deferred checkNotCombatSelf) do NOT run — even if they would
// otherwise fire. When the guard APPLIES, the combat filters run.
//
// All sub-cases use a CheckNotCombat setup that WOULD filter the player
// (varp written at currentTick) to make the guard's effect observable.
func TestHuntPlayersCombatGuard(t *testing.T) {
	setup := func(t *testing.T) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = 100
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = []int32{100} // varp==currentTick → filter would fire if guard applies
		return s, n, p
	}

	hunt := &objtype.HuntType{CheckNotCombat: 0, CheckNotCombatSelf: -1, CheckInv: -1}

	t.Run("target-equals-player-skips-guard", func(t *testing.T) {
		_, n, p := setup(t)
		n.target = p // guard SKIPPED (target == p)
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (target==p → guard skipped, filter does not fire)", len(hunted))
		}
	})

	t.Run("ismulti-true-skips-guard", func(t *testing.T) {
		s, n, p := setup(t)
		s.gamemap.SetMulti(p.x, p.z, p.level, true) // guard SKIPPED (multi-combat zone)
		hunted := n.huntPlayers(s, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (IsMulti=true → guard skipped, filter does not fire)", len(hunted))
		}
	})

	t.Run("target-nil-applies-guard", func(t *testing.T) {
		_, n, _ := setup(t)
		n.target = nil
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (target==nil → guard applies, filter fires)", len(hunted))
		}
	})

	t.Run("target-is-different-player-applies-guard", func(t *testing.T) {
		s, n, _ := setup(t)
		other := addPlayerToServer(t, s, 2, n.x+5, n.z+5, n.level)
		n.target = other
		hunted := n.huntPlayers(s, hunt)
		// Candidate (slot 1) filtered; the other (slot 2) is the NPC's
		// current target AND passes the combat guard-skip, so it bypasses
		// the combat filter. Expected: exactly slot 2 returned.
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (target=other → candidate filtered, other passes)", len(hunted))
		}
		if hunted[0].Slot() != other.slot {
			t.Errorf("hunted[0]: got slot %d, want slot %d (the target-player)", hunted[0].Slot(), other.slot)
		}
	})

	t.Run("gamemap-nil-applies-guard", func(t *testing.T) {
		_, n, _ := setup(t)
		n.server.gamemap = nil // fidelity: nil gamemap treats as not-multi
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (gamemap==nil → guard applies, filter fires)", len(hunted))
		}
	})
}

// TestHuntPlayersCheckNotCombatSelf guards the NPC-side 8-tick
// combat-window filter at TS Npc.ts:946-948. Symmetric to
// TestHuntPlayersCheckNotCombat but reads n.NpcVarN instead of p.Varp.
// When the outer guard applies, an NPC whose own combat-tracker varn
// was written within [currentTick-7, currentTick] skips the candidate;
// at currentTick-8 and earlier, the candidate passes.
func TestHuntPlayersCheckNotCombatSelf(t *testing.T) {
	// Helper mirrors TestHuntPlayersCheckNotCombat's setup. varnVal
	// seeds n.varns[0] via SetNpcVarN.
	setup := func(t *testing.T, currentTick int, varnVal int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = currentTick
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil // guard applies (target != p) → filter fires
		n.SetNpcVarN(0, varnVal)
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		return s, n, p
	}

	t.Run("default-minus-one-disables", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // varn written this tick
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (CheckNotCombatSelf=-1 disables filter)", len(hunted))
		}
	})

	t.Run("varn-this-tick-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // 100+8 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varn written this tick → filter fires)", len(hunted))
		}
	})

	t.Run("varn-minus-seven-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 93) // 93+8 = 101 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varn==currentTick-7 → window-inclusive, filter fires)", len(hunted))
		}
	})

	t.Run("varn-minus-eight-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 92) // 92+8 = 100, 100 > 100 is false → pass
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varn==currentTick-8 → exclusive boundary, filter passes)", len(hunted))
		}
	})

	t.Run("varn-zero-well-past-window-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 0) // fresh NPC, no combat recorded
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0, CheckInv: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varn==0, currentTick=100 → well past window)", len(hunted))
		}
	})
}

// TestHuntPlayersCheckNotCombatSelfOutsideGuard guards that the filter
// does NOT fire when the outer combat guard is skipped (target == p OR
// multi-combat zone). Mirrors TestHuntPlayersCombatGuard but with the
// self-side filter. The guard's other asymmetric cases
// (ismulti-true-skips-guard, gamemap-nil-applies-guard,
// target-differs-applies-guard) are covered transitively by NAI-15's
// TestHuntPlayersCombatGuard; this test only exercises the one case
// needed to prove the new filter shares the same guard.
func TestHuntPlayersCheckNotCombatSelfOutsideGuard(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	n.SetNpcVarN(0, 100) // varn==currentTick → filter would fire if guard applies
	p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	n.target = p // guard SKIPPED (target == p)

	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0, CheckInv: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("got %d, want 1 (target==p → guard skipped, filter does not fire)", len(hunted))
	}
}

// newHuntPlayersCheckInvFixture wires the minimum a CheckInv test needs:
// Server with empty objTypes/paramTypes registries and one in-range Player
// at the standard NPC coords. The HuntType is left to each test to build.
//
// Mirrors the in-package test idiom from npc_hunt_entities_test.go's
// addObjToZone (registry sized to 100; tests grow on demand).
func newHuntPlayersCheckInvFixture(t *testing.T) (*Server, *Npc, *Player) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 200)}
	s.paramTypes = &objtype.ParamTypeConfigs{Configs: make([]*objtype.ParamType, 300)}
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	return s, n, p
}

// TestHuntPlayersCheckInvDisabled pins NAI-22 Bundle 2: when CheckInv
// is -1 (the TS default), the filter is a no-op. Mirrors the implicit
// TS short-circuit at Npc.ts:959.
func TestHuntPlayersCheckInvDisabled(t *testing.T) {
	_, n, _ := newHuntPlayersCheckInvFixture(t)
	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           -1, // disabled
		CheckObj:           -1,
		CheckObjParam:      -1,
	}

	hunted := n.huntPlayers(n.server, hunt)

	if len(hunted) != 1 {
		t.Errorf("huntPlayers: got %d players, want 1 (CheckInv disabled, player must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckInvObjPasses pins NAI-22 Bundle 2: with CheckInv
// set, CheckObj branch evaluates inv.GetItemCount(obj) and compares
// against CheckInvVal via CheckHuntCondition. Player has 5 of obj X,
// hunt requires >=3 (encoded as ">" + Val=2 since CheckHuntCondition
// has no ">=" operator) → player included. Mirrors TS Npc.ts:961-962.
func TestHuntPlayersCheckInvObjPasses(t *testing.T) {
	_, n, p := newHuntPlayersCheckInvFixture(t)
	const invID, objID = 0, 100
	inv := inventory.New(invID, 28, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: objID, Count: 5}
	p.invs = map[int]*inventory.Inventory{invID: inv}

	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           invID,
		CheckObj:           objID,
		CheckObjParam:      -1,
		CheckInvCondition:  ">",
		CheckInvVal:        2,
	}

	hunted := n.huntPlayers(n.server, hunt)

	if len(hunted) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (5 > 2 must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckInvObjFails pins NAI-22 Bundle 2: condition fails
// → player excluded. 1 of obj X, hunt requires >2 → player NOT included.
func TestHuntPlayersCheckInvObjFails(t *testing.T) {
	_, n, p := newHuntPlayersCheckInvFixture(t)
	const invID, objID = 0, 100
	inv := inventory.New(invID, 28, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: objID, Count: 1}
	p.invs = map[int]*inventory.Inventory{invID: inv}

	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           invID,
		CheckObj:           objID,
		CheckObjParam:      -1,
		CheckInvCondition:  ">",
		CheckInvVal:        2,
	}

	hunted := n.huntPlayers(n.server, hunt)

	if len(hunted) != 0 {
		t.Errorf("huntPlayers: got %d, want 0 (1 > 2 must fail)", len(hunted))
	}
}

// TestHuntPlayersCheckInvObjParamPasses pins NAI-22 Bundle 2: CheckObjParam
// branch sums per-slot ObjType.Params[param] across non-empty slots,
// falling back to ParamType.DefaultInt for missing params. Mirrors TS
// Npc.ts:963-964 + Player.ts:1668-1697 (stack=false).
func TestHuntPlayersCheckInvObjParamPasses(t *testing.T) {
	s, n, p := newHuntPlayersCheckInvFixture(t)
	const invID, paramID = 0, 200
	objA, objB, objC := 100, 101, 102

	// Wire ParamType: DefaultInt = 0 (zero-value matches uint32 default).
	s.paramTypes.Configs[paramID] = &objtype.ParamType{
		ConfigType: objtype.ConfigType{ID: paramID},
		DefaultInt: 0,
	}
	// Wire 3 ObjTypes each with Params[paramID]=10. Mirrors handle-side
	// shape at pkg/script/handlers_inv.go:247-252 (uint32 value).
	for _, id := range []int{objA, objB, objC} {
		s.objTypes.Configs[id] = &objtype.ObjType{
			ConfigType: objtype.ConfigType{ID: id},
			Params:     objtype.ParamMap{uint32(paramID): uint32(10)},
		}
	}

	inv := inventory.New(invID, 28, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[1] = &inventory.Item{Id: objB, Count: 1}
	inv.Items[2] = &inventory.Item{Id: objC, Count: 1}
	p.invs = map[int]*inventory.Inventory{invID: inv}

	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           invID,
		CheckObj:           -1,
		CheckObjParam:      paramID,
		CheckInvCondition:  ">",
		CheckInvVal:        20,
	}

	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (sum=30 > 20 must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckInvObjParamFails pins NAI-22 Bundle 2: param-sum
// below threshold → player excluded.
func TestHuntPlayersCheckInvObjParamFails(t *testing.T) {
	s, n, p := newHuntPlayersCheckInvFixture(t)
	const invID, paramID = 0, 200
	objA := 100

	s.paramTypes.Configs[paramID] = &objtype.ParamType{
		ConfigType: objtype.ConfigType{ID: paramID},
		DefaultInt: 0,
	}
	s.objTypes.Configs[objA] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: objA},
		Params:     objtype.ParamMap{uint32(paramID): uint32(10)},
	}

	inv := inventory.New(invID, 28, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: objA, Count: 1}
	p.invs = map[int]*inventory.Inventory{invID: inv}

	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           invID,
		CheckObj:           -1,
		CheckObjParam:      paramID,
		CheckInvCondition:  ">",
		CheckInvVal:        20,
	}

	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("huntPlayers: got %d, want 0 (sum=10 > 20 must fail)", len(hunted))
	}
}

// TestHuntPlayersCheckInvMissingInvDefensive pins NAI-22 Bundle 2:
// when p.invs[CheckInv] is nil, quantity defaults to 0 and
// CheckHuntCondition decides. This is the goscape-vs-TS divergence
// (TS throws here; goscape iterates with quantity=0). Documented in
// code comment, no deviation tag — TS path is dead in practice.
func TestHuntPlayersCheckInvMissingInvDefensive(t *testing.T) {
	_, n, p := newHuntPlayersCheckInvFixture(t)
	const invID, objID = 0, 100
	p.invs = map[int]*inventory.Inventory{} // EMPTY — no inv at invID

	hunt := &objtype.HuntType{
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           invID,
		CheckObj:           objID,
		CheckObjParam:      -1,
		CheckInvCondition:  "=",
		CheckInvVal:        0,
	}

	hunted := n.huntPlayers(n.server, hunt)

	if len(hunted) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (missing inv → quantity=0; 0 == 0 must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckNotBusyFiltersBusyPlayer pins NAI-23 Bundle 2: when
// hunt.CheckNotBusy is true and the candidate player is busy (delayed or
// main/chat modal open), the player is filtered out. Mirrors TS
// Npc.ts:931-933.
func TestHuntPlayersCheckNotBusyFiltersBusyPlayer(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	// Busy player at (3203, 3200).
	pBusy := addPlayerToServer(t, s, 1, 3203, 3200, 0)
	pBusy.delayed = true
	// Fresh player at (3197, 3200).
	pFresh := addPlayerToServer(t, s, 2, 3197, 3200, 0)

	hunt := &objtype.HuntType{
		CheckNotBusy:       true,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (busy player must be filtered out)", len(hunted))
	}
	if hunted[0] != pFresh {
		t.Errorf("hunted[0]: got busy player, want fresh player")
	}
}

// TestHuntPlayersCheckNotBusyDisabled pins NAI-23 Bundle 2: when
// hunt.CheckNotBusy is false, busy players are NOT filtered (the filter
// is gated on the bool flag).
func TestHuntPlayersCheckNotBusyDisabled(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3200, 0, npcType)
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3203, 3200, 0)
	p.delayed = true

	hunt := &objtype.HuntType{
		CheckNotBusy:       false,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled — busy must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness pins
// NAI-23 Bundle 3: when CheckNotTooStrong is OutsideWilderness AND the
// player is outside wilderness AND combatLevel > vislevel*2, the player
// is filtered. Mirrors TS Npc.ts:939-941.
//
// NPC and player are both at z=3500 (outside south wilderness rect which
// starts at z=3520) and within huntRange=5 of each other.
func TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3500, 0, npcType)
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 100

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (strong player outside wilderness must be filtered)", len(hunted))
	}
}

func TestHuntPlayersCheckNotTooStrongIgnoresStrongPlayerInWilderness(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3000, 5000, 0, npcType) // NPC inside south wilderness
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3003, 5000, 0)
	p.combatLevel = 100

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled inside wilderness)", len(hunted))
	}
}

func TestHuntPlayersCheckNotTooStrongAllowsWeakPlayer(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3500, 0, npcType)
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 50 // NOT > 60

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (combatLevel <= vislevel*2 must pass)", len(hunted))
	}
}

func TestHuntPlayersCheckNotTooStrongDisabled(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3500, 0, npcType)
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 100

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOff,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (filter disabled — strong player must pass)", len(hunted))
	}
}

// TestHuntPlayersCheckNotTooStrongBoundaryComparison pins the strict-`>`
// comparison: combatLevel exactly equal to 2*vislevel passes (TS uses `>`,
// not `>=`).
func TestHuntPlayersCheckNotTooStrongBoundaryComparison(t *testing.T) {
	s := newTestServer(t)
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, 3200, 3500, 0, npcType)
	n.server = s
	n.huntRange = 5

	p := addPlayerToServer(t, s, 1, 3203, 3500, 0)
	p.combatLevel = 60 // exactly 2*vislevel

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckAfk:           false,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Errorf("hunted: got %d, want 1 (combatLevel == 2*vislevel must pass; > not >=)", len(hunted))
	}
}

func TestHuntPlayersUsesZoneSubscriptionExclusive(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, VisLevel: 50}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Seed s.players[99] only — do NOT subscribe to Zone. huntPlayers reads
	// from Zone, so this player must NOT be returned.
	c, _ := newTestClient(t)
	phantom := newPlayer(c)
	phantom.slot = 99
	phantom.x, phantom.z, phantom.level = hunter.x+1, hunter.z+1, hunter.level
	phantom.combatLevel = 50
	// active=true so the test catches a registry-fallback regression
	// regardless of where IsValid checks land in the filter chain.
	phantom.active = true
	s.players.set(99, phantom)
	hunt := &objtype.HuntType{
		CheckNpc:           -1,
		CheckVis:           objtype.HuntVisOff,
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOff,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           -1,
	}
	got := hunter.huntPlayers(s, hunt)
	for _, e := range got {
		if pl, ok := e.(*Player); ok && pl.slot == 99 {
			t.Error("huntPlayers returned non-Zone-subscribed player; should be Zone-exclusive")
		}
	}
}

// TestHuntPlayersRespectsIsValidFilter verifies that PlayersSafe's
// IsValid() gate propagates to huntPlayers — a Zone-subscribed but
// inactive player must NOT appear. Mirrors TS huntPlayers's reliance
// on Zone.getAllPlayersSafe.
func TestHuntPlayersRespectsIsValidFilter(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, VisLevel: 50}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Spawn a Zone-subscribed player at hunter's tile via addPlayerToServer
	// (post-NAI-28 it subscribes to Zone AND sets active=true).
	target := addPlayerToServer(t, s, 1, hunter.x, hunter.z, hunter.level)
	target.combatLevel = 50
	// Flip active=false — IsValid() returns false; PlayersSafe must skip.
	target.active = false
	hunt := &objtype.HuntType{
		CheckNpc:           -1,
		CheckVis:           objtype.HuntVisOff,
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOff,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckInv:           -1,
	}
	got := hunter.huntPlayers(s, hunt)
	for _, e := range got {
		if pl, ok := e.(*Player); ok && pl.slot == target.slot {
			t.Errorf("huntPlayers returned inactive player pid=%d; IsValid filter should skip it", target.slot)
		}
	}
}
