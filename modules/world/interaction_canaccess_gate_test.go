package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction pins
// TS Player.ts:1244 fidelity: when CanAccess()=false due to modalState&Chat,
// the post-step interact block (including "I can't reach!" + ClearInteraction)
// must NOT fire. The interaction is preserved across the tick.
//
// Pre-NAI-155: the missing canAccess gate at interaction.go:267 let goscape
// reach ClearInteraction() when TS would skip the whole block. This test
// fails (p.target==nil) before the fix and passes (p.target!=nil) after.
func TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3105, 3096, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 3105, 3095, 0) // cheb=1 adjacent

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.modalState = modalStateChat // residue from prior chatnpc dialog

	if p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be false with modalStateChat")
	}

	p.processInteraction()

	// Load-bearing assertion: interaction must be preserved (target non-nil).
	// TS L1244 gates the entire post-step block on canAccess(); when false,
	// clearInteraction() is not called, so p.target stays set.
	if p.target == nil {
		t.Fatal("interaction destroyed under modalChat residue; TS L1244 preserves interaction when canAccess()=false")
	}
}

// TestProcessInteraction_CanAccessGate_ProtectedScript_PreservesInteraction pins
// the same invariant for protectedScriptActive()=true (TS Player.protect path,
// memory protect_over_clear / NAI-111-D1).
//
// Pre-NAI-155: same root cause — missing canAccess gate at processInteraction.
func TestProcessInteraction_CanAccessGate_ProtectedScript_PreservesInteraction(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3105, 3096, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 3105, 3095, 0) // cheb=1 adjacent

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.activeScript = &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}
	p.protect = true // NAI-111-D1: Player.protect is the TS-faithful gate, set alongside activeScript fixture

	if p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be false with protectedScriptActive")
	}

	p.processInteraction()

	// Load-bearing assertion: interaction must be preserved (target non-nil).
	// TS L1244 skips the entire post-step block when canAccess()=false;
	// ClearInteraction() is not called.
	if p.target == nil {
		t.Fatal("interaction destroyed under protected-script residue; TS L1244 preserves interaction when canAccess()=false")
	}
}

// TestProcessInteraction_CanAccessGate_HappyPath_OpFires regression-pins the
// success case: CanAccess()=true, target at cheb=1, opnpc1 script registered →
// pre-step arm fires branch 1, auto-clear runs, p.target becomes nil.
//
// Pattern mirrors TestTryInteract_NotFollowOp_NotShortCircuited in
// interaction_tryinteract_guard_test.go but exercises the full
// processInteraction path (TS L1209-1224 pre-step arm).
func TestProcessInteraction_CanAccessGate_HappyPath_OpFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[opnpc1,_]",
		LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
	})
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // cheb=1 adjacent
	npc.typeId = 7
	npc.typ = &objtype.NpcType{ID: 7}

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if !p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be true for happy path")
	}

	p.processInteraction()

	// After a successful branch-1 fire the auto-clear at interaction.go:289
	// calls ClearInteraction() → p.target = nil. Presence of nil target is the
	// observable proof that the interaction fired rather than being preserved
	// (which would leave target non-nil).
	if p.target != nil {
		t.Fatal("happy-path OPNPC1 did not fire and auto-clear; p.target non-nil after processInteraction")
	}
}

// TestProcessInteraction_CanAccessGate_Delayed_PreservesInteraction pins TS
// Player.ts:1210/1244 fidelity: when delayed, both the pre-step interact arm
// (L1210) and the post-step interact arm (L1244) skip via the canAccess gate,
// so ClearInteraction() never fires and the target stays set across the tick.
//
// Pre-interaction-7 a goscape-only `delayed && currentTick<delayedUntil`
// short-circuit at the top of processInteractionPreMove returned ahead of the
// post-step HEAD entirely (skipping pathToPathingTarget, the followOp
// exhaustion clear, and the tail mapflag clear). The CanAccess() call-site
// gates on the pre-step arm (interaction.go:351) and post-step arm
// (interaction.go:417) already cover the "interaction must be preserved"
// invariant; this test continues to lock that in.
//
// Sibling test TestProcessInteraction_CanAccessGate_Delayed_FollowOp_PathRecomputed
// pins the load-bearing TS L1227-1239 head that the removed short-circuit
// previously skipped.
func TestProcessInteraction_CanAccessGate_Delayed_PreservesInteraction(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 100
	p, wait := makeInteractionPlayer(t, s, 3105, 3096, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 3105, 3095, 0) // cheb=1 adjacent

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.delayed = true
	p.delayedUntil = 105 // s.currentTick (100) < delayedUntil (105) → CanAccess()=false via p.delayed

	p.processInteraction()

	// Target must be preserved: the CanAccess gates on both interact arms
	// (pre-step at L351, post-step at L417) skip the validateTarget-clear
	// and the "I can't reach!"-clear arms. Mirrors TS L1210/L1244 fidelity.
	if p.target == nil {
		t.Fatal("delayed processInteraction cleared interaction; TS L1210/L1244 preserve target when canAccess()=false")
	}
}

// TestProcessInteraction_CanAccessGate_Delayed_FollowOp_PathRecomputed pins
// TS Player.ts:1227-1239 fidelity for the delayed-follower case: even when
// canAccess()=false, the post-step HEAD still runs pathToPathingTarget, and
// the followOp branch at TS Player.ts:1039-1042 queues a waypoint to the
// leader's followX/Z. A delayed follower must keep chasing the leader.
//
// Closes interaction-7. Pre-fix, the goscape-only short-circuit at
// processInteractionPreMove returned ahead of the post-step HEAD, so a delayed
// follower stopped re-pathing and stalled at their previous tile.
//
// The followOp branch of pathToPathingTarget (interaction.go:979-986) runs
// BEFORE the local CanAccess gate (interaction.go:988) and queues
// unconditionally, mirroring TS L1039-1042 (the canAccess check at TS L1044
// only blocks the non-followOp arms).
func TestProcessInteraction_CanAccessGate_Delayed_FollowOp_PathRecomputed(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 100

	leader, leaderWait := makeInteractionPlayer(t, s, 3220, 3220, 0)
	defer leaderWait()
	leader.active = true // valid follow target (validateTarget gate 3 / TS Player.isValid)
	// Simulate post-processLogins state so the leader's per-tick top writes
	// refresh followX/Z from lastStepX/Z = (3219, 3220) — mirrors the
	// TestPlayerFollow_PathToPathingTarget_QueuesValidLeaderCoord fixture.
	leader.lastStepX = leader.x - 1
	leader.lastStepZ = leader.z
	leader.processInteraction() // top writes refresh followX/Z
	if leader.followX != 3219 || leader.followZ != 3220 {
		t.Fatalf("pre-condition: leader followX/Z = (%d, %d), want (3219, 3220)", leader.followX, leader.followZ)
	}

	follower, followerWait := makeInteractionPlayer(t, s, 3225, 3225, 0)
	defer followerWait()
	follower.target = leader
	follower.targetOp = 3 // raw op-slot 3; isFollowOp matches targetOp==3 && target.(*Player)
	follower.delayed = true
	follower.delayedUntil = 105 // s.currentTick (100) < 105 → canAccess()=false via p.delayed

	if follower.CanAccess() {
		t.Fatal("test setup invalid: follower.CanAccess() should be false with delayed=true")
	}

	follower.processInteraction()

	// pathToPathingTarget's followOp arm (interaction.go:979-986, TS L1039-1042)
	// fires BEFORE the local CanAccess gate and queues a waypoint to the
	// leader's followX/Z. Pre-interaction-7 the short-circuit short-returned
	// from processInteractionPreMove and waypointIndex stayed at -1.
	if follower.waypointIndex < 0 {
		t.Fatalf("delayed follower has no waypoints post-processInteraction; want a re-queued chase waypoint via TS L1039-1042 (waypointIndex=%d)",
			follower.waypointIndex)
	}
	wp := coordgrid.UnpackCoord(follower.waypoints[follower.waypointIndex])
	if wp.X != 3219 || wp.Z != 3220 {
		t.Errorf("delayed follower queued waypoint: got (%d, %d), want (3219, 3220) = leader.followX/Z (TS Player.ts:1040)",
			wp.X, wp.Z)
	}
	// Target must still be preserved across the tick — the post-step interact
	// arm (L1244 / interaction.go:417) is canAccess-gated and never fires.
	if follower.target == nil {
		t.Fatal("delayed follower's interaction cleared; TS L1244 preserves target when canAccess()=false")
	}
}

// TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped negatively pins
// the post-step block: when target is nil at entry, processInteraction returns
// at the p.target == nil guard AFTER the NAI-174 top writes (followX/Z and
// nextTarget — see TestProcessInteraction_TopWritesFireUnconditionally for that
// coverage). Branch counters do not fire — they live inside the target-having
// path. No panic. (TS L1244 also gates on this.target.)
func TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3105, 3096, 0)
	defer wait()

	// p.target is nil; processInteraction short-circuits at the p.target == nil guard after the top writes.
	// No interaction set — mirrors a mid-tick clear scenario.
	p.processInteraction()

	// No panic is the primary invariant. Also: branch counters are not
	// reset (the Frame B reset at L218-220 is skipped when target==nil
	// at entry).
	if p.lastInteractBranchPre != 0 || p.lastInteractBranchPost != 0 {
		t.Fatalf("branch counters mutated with nil target at entry; pre=%d post=%d",
			p.lastInteractBranchPre, p.lastInteractBranchPost)
	}
}

// TestProcessInteraction_TopWritesFireUnconditionally pins TS
// Player.ts:1200-1203 — the followX/Z and nextTarget writes at the top
// of processInteraction fire EVERY tick for EVERY player, regardless
// of whether the player has a target. Required for player-follow:
// a leader without a target must still refresh followX/Z each tick so
// followers can queue a valid waypoint via pathToPathingTarget at
// interaction.go:802-809. Pre-NAI-174 the writes sat after the
// p.target == nil early-return and never fired for targetless players.
func TestProcessInteraction_TopWritesFireUnconditionally(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3105, 3096, 0)
	defer wait()

	// Pre-conditions: target nil; lastStepX/Z set to a real coord
	// (production sets via processLogins or per-step movement.go); stale
	// followX/Z + a non-nil nextTarget to verify both writes fire.
	p.lastStepX = 3200
	p.lastStepZ = 3210
	p.followX = -1
	p.followZ = -1
	// nextTarget is a *entity; using p itself as a non-nil sentinel.
	p.nextTarget = p

	p.processInteraction()

	if p.followX != 3200 {
		t.Errorf("followX: got %d, want 3200 (= lastStepX, unconditional top write)", p.followX)
	}
	if p.followZ != 3210 {
		t.Errorf("followZ: got %d, want 3210 (= lastStepZ, unconditional top write)", p.followZ)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (unconditional top write)", p.nextTarget)
	}
	// The existing branch-counter invariant from TestProcessInteraction_
	// CanAccessGate_NilTarget_PostStepSkipped — preserved post-NAI-174.
	if p.lastInteractBranchPre != 0 || p.lastInteractBranchPost != 0 {
		t.Fatalf("branch counters mutated with nil target at entry; pre=%d post=%d",
			p.lastInteractBranchPre, p.lastInteractBranchPost)
	}
}

// TestPathToPathingTarget_ModalChat_SkipsPathing pins TS Player.ts:1044
// fidelity: when CanAccess()=false due to modalState&Chat (no delay, no
// protected script), pathToPathingTarget must skip the waypoint-queue
// arm entirely. Pre-NAI-170 the local gate p.delayed || p.protectedScriptActive()
// passed through modal-only players, leaking a path queue mid-dialog.
//
// Fixture mirrors TestProcessInteractionRepathsAfterPathExhaustion (H8):
// NodeClientRoutefinder=true + NPC at cheb=15 forces the pathToTarget
// arm to fire and queue waypoints when the gate passes.
func TestPathToPathingTarget_ModalChat_SkipsPathing(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true
	npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // cheb=15 from player

	p, cc := newTestPlayer(t)
	_ = cc
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.modalState = modalStateChat
	p.waypointIndex = -1

	if p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be false with modalStateChat")
	}
	if p.delayed || p.protectedScriptActive() {
		t.Fatal("test setup invalid: only modal-Chat should be active; narrow local gate must pass pre-fix")
	}

	p.pathToPathingTarget()

	if p.waypointIndex >= 0 {
		t.Fatalf("modalChat player got waypoints queued (waypointIndex=%d); TS L1044 canAccess gate must skip pathing", p.waypointIndex)
	}
}
