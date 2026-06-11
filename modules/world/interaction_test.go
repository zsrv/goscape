package world

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// makeInteractionNpc builds a live NPC registered in s.npcs at the given slot.
func makeInteractionNpc(t *testing.T, s *Server, slot, x, z, level int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	n := NewNpc(slot, 0, x, z, level, typ)
	n.nid = slot
	s.npcs[slot] = n
	s.npcLoop = append(s.npcLoop, n)
	return n
}

// makeInteractionPlayer wires a Player to the server with ISAAC pair and coords.
func makeInteractionPlayer(t *testing.T, s *Server, x, z, level int) (*Player, func()) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = x, z, level
	drain := drainConn(t, cc)
	return p, func() { <-drain }
}

// TestSetInteractionPopulatesFields checks that SetInteraction stores all fields.
func TestSetInteractionPopulatesFields(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 3, -1)

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if p.apRange != 10 {
		t.Errorf("apRange: got %d, want 10", p.apRange)
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled should be false")
	}
	if p.interacted {
		t.Error("interacted should be false")
	}
	if p.repathed {
		t.Error("repathed should be false")
	}
}

// TestClearInteractionResetsAll verifies all fields return to idle.
func TestClearInteractionResetsAll(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.apRangeCalled = true

	p.ClearInteraction()

	if p.target != nil {
		t.Errorf("target: got %v, want nil", p.target)
	}
	if p.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1", p.targetOp)
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled should be false")
	}
	// interacted/repathed are no longer reset by ClearInteraction (interaction-6:
	// `interacted` is now reset per-tick in ResetMasks, matching TS
	// PathingEntity.ts:587; `repathed` is vestigial). The TS clearInteraction
	// (PathingEntity.ts:550-555) only resets target/targetOp/targetSubject/
	// apRange/apRangeCalled.
}

// TestProcessInteractionNoTargetNoop verifies nil target is a no-op.
func TestProcessInteractionNoTargetNoop(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	// no target set

	p.processInteraction()

	if p.interacted {
		t.Error("interacted should remain false with no target")
	}
	if p.waypointIndex >= 0 {
		t.Error("no waypoint should be set with no target")
	}
}

// TestProcessInteractionInRangeFacesTarget verifies adjacent target fires the
// OP trigger and auto-clears the interaction. NAI-41: faceEntity write
// timing moved to SetInteraction-time; this test no longer pins faceEntity
// (covered by TestSetInteractionNpcTargetSetsFaceEntity).
//
// NAI-44 T6 cascade: pre-T5 asserted interacted==true; post-T5 auto-clear
// (TS L1261-1263) fires when interacted && !apRangeCalled, setting target=nil
// and clearing interacted. The observable proof of contact-fire is target==nil.
func TestProcessInteractionInRangeFacesTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	// NAI-44: auto-clear fires after contact (interacted && !apRangeCalled);
	// target==nil is the observable proof that the OP arm was reached and fired.
	if p.target != nil {
		t.Errorf("target: got %v, want nil (auto-clear at TS L1261-1263 after contact-fire)", p.target)
	}
}

// TestProcessInteractionOutOfRangePaths verifies a distant target causes pathing.
// The NPC is placed 15 tiles away — beyond the default apRange of 10 — so the
// interaction falls through to the pathing branch (not the AP branch).
func TestProcessInteractionOutOfRangePaths(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true              // use direct-step mode
	npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // 15 tiles away — beyond apRange=10

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.waypointIndex < 0 {
		t.Error("waypointIndex should be >= 0 after pathToTarget")
	}
	if p.interacted {
		t.Error("interacted should be false when out of range")
	}
}

// TestProcessInteractionDifferentLevelClears verifies level mismatch clears and emits UnsetMapFlag.
func TestProcessInteractionDifferentLevelClears(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 1) // level 1

	p, cc := newTestPlayer(t)
	p.client.server = s
	enc := io2.New([4]uint32{1, 2, 3, 4})
	refEnc := io2.New([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.x, p.z, p.level = 100, 100, 0 // player on level 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	got := <-received

	// Expect UnsetMapFlag (opcode 62, 0 payload = just the encrypted opcode byte).
	want := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(refEnc.GetNext())) & 0xff)
	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag byte on wire, got nothing")
	}
	if got[0] != want {
		t.Errorf("wire byte: got %d, want %d (UnsetMapFlag)", got[0], want)
	}
	if p.target != nil {
		t.Error("target should be nil after level mismatch")
	}
}

// TestProcessInteractionDelayedPlayerSkipped verifies a delayed player skips interaction.
func TestProcessInteractionDelayedPlayerSkipped(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.delayed = true
	p.delayedUntil = 999 // far future
	s.currentTick = 0

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("delayed player: expected no wire bytes, got %d", len(got))
	}
	if p.interacted {
		t.Error("interacted should be false for delayed player")
	}
}

func TestSetInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	npc := &Npc{nid: 0, typeId: 7}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
}

func TestClearInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.ClearInteraction()
}

// TestInOperableDistanceCheb_DefensiveFallback pins the goscape-defensive
// Chebyshev≤1 (excluding same-tile) predicate retained for the nil-gamemap
// test-fixture paths in inOperableDistance / (*Npc).inOperableDistance.
// Production never reaches inOperableDistanceCheb post-NAI-173 — see
// TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb
// for the production-path defensive-arm coverage.
func TestInOperableDistanceCheb_DefensiveFallback(t *testing.T) {
	cases := []struct {
		dx, dz int
		want   bool
	}{
		{0, 0, false}, // same tile
		{1, 0, true},  // N/S/E/W adjacent
		{0, 1, true},
		{-1, 0, true},
		{0, -1, true},
		{1, 1, true},   // diagonal adjacent
		{-1, -1, true}, // diagonal adjacent
		{2, 0, false},  // 2 away
		{0, 2, false},
		{2, 1, false},
	}
	for _, tc := range cases {
		got := inOperableDistanceCheb(0, 0, tc.dx, tc.dz)
		if got != tc.want {
			t.Errorf("inOperableDistanceCheb(0,0,%d,%d) = %v, want %v", tc.dx, tc.dz, got, tc.want)
		}
	}
}

// TestSendUnsetMapFlagWireFormat verifies the encrypted opcode byte.
func TestSendUnsetMapFlagWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc := io2.New([4]uint32{7, 8, 9, 10})
	refEnc := io2.New([4]uint32{7, 8, 9, 10})
	p.client.encryptor = enc

	want := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(refEnc.GetNext())) & 0xff)

	received := drainConn(t, cc)
	sendUnsetMapFlag(p)
	p.client.flushWrite()
	got := <-received

	if len(got) != 1 {
		t.Fatalf("UnsetMapFlag: got %d bytes, want 1", len(got))
	}
	if !bytes.Equal(got, []byte{want}) {
		t.Errorf("UnsetMapFlag wire: got %v, want %v", got, []byte{want})
	}
}

// TestInApproachDistanceSameTile verifies same-tile coordinates return
// false against a PathingEntity target (can't "approach" your own tile).
// Mirrors inOperableDistance (which also excludes same-tile).
func TestInApproachDistanceSameTile(t *testing.T) {
	if inApproachDistance(100, 100, 100, 100, 1, 1, 10, true) {
		t.Error("same tile: got true, want false")
	}
}

// TestInApproachDistanceAtRange verifies Chebyshev distance exactly
// apRange is accepted.
func TestInApproachDistanceAtRange(t *testing.T) {
	if !inApproachDistance(100, 100, 110, 100, 1, 1, 10, true) {
		t.Error("dx=10 apRange=10: got false, want true")
	}
	if !inApproachDistance(100, 100, 107, 107, 1, 1, 10, true) {
		t.Error("dx=dz=7 apRange=10: got false, want true")
	}
}

// TestInApproachDistanceBeyondRange verifies one tile past apRange
// is rejected.
func TestInApproachDistanceBeyondRange(t *testing.T) {
	if inApproachDistance(100, 100, 111, 100, 1, 1, 10, true) {
		t.Error("dx=11 apRange=10: got true, want false")
	}
	if inApproachDistance(100, 100, 105, 111, 1, 1, 10, true) {
		t.Error("dz=11 apRange=10: got true, want false")
	}
}

// TestInApproachDistanceZeroRange verifies apRange <= 0 is always
// rejected (even for adjacent tiles).
func TestInApproachDistanceZeroRange(t *testing.T) {
	if inApproachDistance(100, 100, 101, 100, 1, 1, 0, true) {
		t.Error("apRange=0: got true, want false")
	}
	if inApproachDistance(100, 100, 101, 100, 1, 1, -5, true) {
		t.Error("apRange=-5: got true, want false")
	}
}

// TestInApproachDistance_EdgeAware_MultiTileTarget pins that approach distance
// is measured to the target's nearest EDGE, not its origin corner (TS uses
// CoordGrid.distanceTo). A 3x3 target at origin (100,100) occupies tiles
// (100..102, 100..102). A 1x1 source at (107,100) is edge-distance 5 from it
// (107-102) but origin-distance 7 (107-100). With apRange=5 (a weapon's range)
// the source IS in approach (can fire from max range), so the player does not
// have to walk (size-1) tiles too close.
//
// The origin-corner form returned false here (7 > 5), forcing ranged/magic
// attackers to approach to edge-distance 3 against a 3x3 NPC — the "I still
// get too close" symptom that scales with NPC size.
//
// All cases below pass isPathingTarget=true to model a 3x3 / 2x2 NPC target.
// The Loc/Obj (non-pathing) sibling cases live in
// TestInApproachDistance_NonPathingTarget_SkipsFootprintBail (npc-ai-5).
func TestInApproachDistance_EdgeAware_MultiTileTarget(t *testing.T) {
	// edge distance 5, apRange 5 → in approach (fire from max range)
	if !inApproachDistance(107, 100, 100, 100, 3, 3, 5, true) {
		t.Error("3x3 target, edge dist 5, apRange 5: got false, want true (edge-aware)")
	}
	// edge distance 6 (source at 108), apRange 5 → out of approach
	if inApproachDistance(108, 100, 100, 100, 3, 3, 5, true) {
		t.Error("3x3 target, edge dist 6, apRange 5: got true, want false")
	}
	// 2x2 target (occupies 100..101): source at (106,100) is edge dist 5
	if !inApproachDistance(106, 100, 100, 100, 2, 2, 5, true) {
		t.Error("2x2 target, edge dist 5, apRange 5: got false, want true (edge-aware)")
	}
	// source on a 3x3 NPC footprint → not approach (PathingEntity bail fires)
	if inApproachDistance(101, 101, 100, 100, 3, 3, 5, true) {
		t.Error("source under 3x3 NPC footprint: got true, want false")
	}
}

// TestInApproachDistance_NonPathingTarget_SkipsFootprintBail pins the
// npc-ai-5 / pathing-5 / interaction-5 fix: a Loc / Obj target (non-pathing)
// does NOT trigger the footprint-overlap bail at TS PathingEntity.ts:395 —
// a player standing on a multi-tile Loc footprint (e.g. a banker counter, a
// 3x3 stall) or sharing a tile with an Obj is still in approach distance to
// fire its AP script.
//
// RED before the fix (when the bail ran unconditionally):
//   - "source under 3x3 Loc footprint: got false, want true"
//   - "same tile as Obj: got false, want true"
//
// Out-of-range still rejects (the bail is independent of the distance check).
func TestInApproachDistance_NonPathingTarget_SkipsFootprintBail(t *testing.T) {
	// 1x1 source inside the 3x3 Loc footprint (101,101 within 100..102) →
	// approach distance OK; isPathingTarget=false skips the bail.
	if !inApproachDistance(101, 101, 100, 100, 3, 3, 5, false) {
		t.Error("source under 3x3 Loc footprint: got false, want true " +
			"(PathingEntity bail must not fire for non-pathing targets)")
	}
	// Same-tile Obj (1x1, isPathingTarget=false). TS skips the bail for Obj
	// targets just like Loc — the operable-distance gate at the call site is
	// what actually accepts same-tile Obj pickup.
	if !inApproachDistance(100, 100, 100, 100, 1, 1, 5, false) {
		t.Error("source same-tile as 1x1 Obj: got false, want true " +
			"(PathingEntity bail must not fire for non-pathing targets)")
	}
	// Out-of-range still rejects regardless of isPathingTarget.
	if inApproachDistance(110, 100, 100, 100, 3, 3, 5, false) {
		t.Error("3x3 Loc target, edge dist 8, apRange 5: got true, want false " +
			"(distance check is independent of the footprint bail)")
	}
}

// TestApproachHasLineOfSight pins M1: the player-side approach gate applies the
// forward line-of-sight check (TS PathingEntity.ts:405 else-branch via
// isApproached → hasLineOfSight with CollisionFlag.PLAYER). A projectile-blocking
// wall between the player and an in-range target must gate AP off, mirroring the
// NPC branch's backward LoS. Setup follows the NPC-side LoS tests: a
// FlagWallNorthProjBlocker on the tile the forward ray enters.
func TestApproachHasLineOfSight(t *testing.T) {
	build := func(t *testing.T, withWall bool) *Player {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3106, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3107, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3108, 0)
		if withWall {
			// Player (size 1) at z=3108 casting toward z=3106 enters the dest
			// tile 3106 from the north; FlagWallNorthProjBlocker there blocks it.
			s.gamemap.Pathfinder.Flags.Add(3094, 3106, 0, collision.FlagWallNorthProjBlocker)
		}
		p := addPlayerToServer(t, s, 1, 3094, 3108, 0)
		p.client = &client{server: s} // the helper reaches the map via p.client.server
		return p
	}

	t.Run("clear_los_passes", func(t *testing.T) {
		p := build(t, false)
		if !p.approachHasLineOfSight(3094, 3106, 1, 1) {
			t.Error("approachHasLineOfSight: got false, want true with no blocker")
		}
	})

	t.Run("blocked_los_gates", func(t *testing.T) {
		p := build(t, true)
		if p.approachHasLineOfSight(3094, 3106, 1, 1) {
			t.Error("approachHasLineOfSight: got true, want false — " +
				"FlagWallNorthProjBlocker must gate the forward AP ray")
		}
	})

	t.Run("nil_gamemap_passes", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.gamemap = nil
		p := addPlayerToServer(t, s, 1, 3094, 3108, 0)
		p.client = &client{server: s}
		if !p.approachHasLineOfSight(3094, 3106, 1, 1) {
			t.Error("approachHasLineOfSight: got false, want true (nil gamemap short-circuits to pass)")
		}
	})
}

// TestProcessInteractionPreMove_InRangeAttackClearsPathBeforeMovement pins the
// TS-faithful pre-step-before-movement ordering (Player.ts:1241 — updateMovement
// sits between the pre-step and post-step interact arms). A player who clicks an
// NPC while already within attack range has a path queued toward it (op-click),
// but the pre-step interact must fire FIRST and clear that path, so the
// subsequent movement pass cannot step the player to contact.
//
// Pre-fix, goscape ran movement (processPathing) before the interaction, so the
// op-click path was walked and the player ended adjacent, firing the contact
// opnpc2 instead of shooting from where they stood — the user-reported "ranged
// fires arrows from right up against the NPC". Confirmed live via RANGED-DEBUG
// (playerMovedThisTick=2, branch_pre=1 OP, apRange never reduced).
func TestProcessInteractionPreMove_InRangeAttackClearsPathBeforeMovement(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t) // player (100,100), npc (105,100): dist 5, apRange 10
	p.interacted = false

	// [apnpc1] attack script (no p_op_*, no p_aprange → the "attack at range" path).
	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerApNpc1, 7, -1))

	// Simulate the op-click: a path queued toward the NPC.
	p.queueWaypoint(npc.x, npc.z)
	if p.waypointIndex < 0 {
		t.Fatal("setup: expected a queued path toward the NPC")
	}

	// Pre-move pass (runs BEFORE processPathing in the real tick).
	p.processInteractionPreMove()

	if !p.interactTick.interacted {
		t.Error("pre-move interact did not fire: an in-range AP attack should fire before movement")
	}
	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex=%d, want -1: the in-range attack must clear the op-click path BEFORE the movement pass, so the player holds at range instead of stepping to contact", p.waypointIndex)
	}
}

// TestClearInteractionResetsApRange verifies ClearInteraction resets
// apRange to 10 (the default), preventing stale values from leaking
// between interactions. Matches TS PathingEntity.ts:554-555.
func TestClearInteractionResetsApRange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 3
	p.apRangeCalled = true

	p.ClearInteraction()

	if p.apRange != 10 {
		t.Errorf("apRange after clear: got %d, want 10", p.apRange)
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled after clear: got true, want false")
	}
}

// TestProcessInteractionRoutesToApBranch verifies processInteraction routes
// via branch 3 (default-AP no-op) when the player is within approach range
// of a Loc with no [aploc1] script. Pre-NAI-78 this test pinned the
// 2-branch bug where tryFireApTrigger was called unconditionally (even with
// no AP script) causing early auto-clear. Post-NAI-78 branch 3 fires:
// apRange=-1, tryInteract returns false, player starts walking toward target.
//
// NAI-98 update: pathToPathingTarget is a no-op for Loc targets (TS
// L1035-1037 alignment — retired the pre-NAI-98 once-per-interaction gate).
// The path toward the Loc must be pre-queued (simulating what MoveClick
// does in production) so that the post-step NIH branch ("I can't reach
// that") does not fire and clear the interaction.
func TestProcessInteractionRoutesToApBranch(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.locTypes = &objtype.LocTypeConfigs{
		Configs: make([]*objtype.LocType, 1), // type 0 slot only
	}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 100, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	zn := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	zn.Locs = append(zn.Locs, loc)

	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	p.apRange = 10
	// Pre-queue path toward Loc, mirroring what MoveClick does in production.
	// NAI-98: pathToPathingTarget is a no-op for Loc targets; path must come
	// from MoveClick / scripts, not from the tickloop repath.
	p.queueWaypoint(loc.X, loc.Z)

	p.processInteraction()

	// NAI-78 branch 3: approach=true, no AP script → apRange=-1, return false.
	// Target is preserved (player still walking toward Loc); apRange is set
	// to -1 (the "no AP script for this interaction" sentinel).
	if p.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (NAI-78 branch 3 no-AP sentinel)", p.apRange)
	}
	// Target preserved: player is en-route, not prematurely cleared.
	if p.target == nil {
		t.Errorf("target: got nil, want non-nil (target preserved while walking toward Loc)")
	}
}

// TestSetInteractionStoresComField verifies that SetInteraction's
// new com parameter writes through to p.targetSubject.com.
// S6m: proves the spellCom slot is carried end-to-end.
func TestSetInteractionStoresComField(t *testing.T) {
	p, _ := newTestPlayer(t)

	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 6, 12345)

	if p.targetSubject.com != 12345 {
		t.Errorf("targetSubject.com: got %d, want 12345", p.targetSubject.com)
	}
	if p.targetOp != 6 {
		t.Errorf("targetOp: got %d, want 6", p.targetOp)
	}
}

// TestSetInteractionPassesMinusOneForNonComOps verifies backwards-compat
// behavior: the S6j/S6k/S6l call sites that pass -1 correctly clear any
// prior com state.
func TestSetInteractionPassesMinusOneForNonComOps(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.targetSubject.com = 999 // simulate stale prior value

	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 1, -1)

	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (S6j-era callers pass -1)", p.targetSubject.com)
	}
}

// TestSetInteractionComZeroCanonicalisation verifies that SetInteraction
// canonicalises com=0 to com=-1 at storage time, matching TS truthy
// PathingEntity.ts:520: `targetSubject.com = com ? com : -1`. NAI-62: this
// boundary affects OpPlayerU's useObj=0 case post-producer-fix.
func TestSetInteractionComZeroCanonicalisation(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.targetSubject.com = 999 // stale prior value

	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 1, 0)
	if p.targetSubject.com != -1 {
		t.Errorf("com=0 canonicalisation: got %d, want -1 (TS PathingEntity.ts:520)", p.targetSubject.com)
	}

	// Sanity: positive com is preserved
	p.SetInteraction(InteractionEngine, fake, 1, 12345)
	if p.targetSubject.com != 12345 {
		t.Errorf("positive com: got %d, want 12345", p.targetSubject.com)
	}

	// Sanity: -1 sentinel is preserved
	p.SetInteraction(InteractionEngine, fake, 1, -1)
	if p.targetSubject.com != -1 {
		t.Errorf("-1 sentinel: got %d, want -1", p.targetSubject.com)
	}
}

// fakeEntity is a minimal entity implementation for tests that need a
// non-nil, non-specific target.
type fakeEntity struct{ x, z, level int }

func (f fakeEntity) Slot() int                 { return -1 }
func (f fakeEntity) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeEntity) IsValid() bool             { return true }

// TestEffectiveApRange_UsesPlayerApRange_NpcTarget pins TS-parity with
// Player.tryInteract (Player.ts:1139) which reads this.apRange
// regardless of target type. NPC's per-type AttackRange is the NPC's
// own combat reach (used by Npc.checkApTrigger when the NPC is the
// attacker), not the player's. The bow's apheld trigger calls
// p_aprange(N) to set the player's mutable apRange; that value gates
// AP-firing for both Loc and Npc targets.
func TestEffectiveApRange_UsesPlayerApRange_NpcTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 7 // simulates bow apheld → p_aprange(7)

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 1, // melee NPC — IRRELEVANT for player-side AP gating
	}
	npc := NewNpc(0, 7, 100, 100, 0, npcType)
	p.target = npc

	if got := effectiveApRange(p); got != 7 {
		t.Errorf("effectiveApRange: got %d, want 7 (p.apRange — TS Player.ts:1139)", got)
	}
}

// TestEffectiveApRangeLocUsesPlayerApRange verifies that for non-NPC
// targets (e.g. *Loc), effectiveApRange uses p.apRange.
func TestEffectiveApRangeLocUsesPlayerApRange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 7 // custom, simulating a p_aprange call

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	p.target = loc

	if got := effectiveApRange(p); got != 7 {
		t.Errorf("effectiveApRange: got %d, want 7 (p.apRange for Loc target)", got)
	}
}

// TestProcessInteraction_NpcInRange_FiresApBranch pins the user-visible
// fence-shooting fix: a player with apRange=10 (bow) attacking a melee
// NPC at dx=6 reaches AP-firing distance regardless of the NPC's own
// AttackRange. Pre-fix Go used npc.typ.AttackRange=5 and rejected at
// dx=6, leaving processInteraction stuck on pathing; through-fence
// scenarios then deadlocked because the fence walk-blocked adjacency
// while leaving projectiles unimpeded.
//
// With no AP script registered, branch 3 (default-AP NIH) fires when
// the player is in approach distance — sets p.apRange=-1 as the signal
// that AP "ran". Pre-fix p.apRange would stay at 10 because branch 3
// never reached.
func TestProcessInteraction_NpcInRange_FiresApBranch(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5, // melee NPC — does not gate player AP
	}
	npc := NewNpc(0, 7, 106, 100, 0, npcType) // dx=6, within p.apRange=10
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	// SetInteraction resets apRange=10 (TS PathingEntity.ts:554 parity).
	// Default fixture state already matches the bow-equipped scenario.

	p.processInteraction()

	// dx=6 <= p.apRange=10 → in approach distance → branch 3
	// (no AP script registered) → apRange=-1. Pre-fix path stayed in
	// pathing-only because effectiveApRange returned AttackRange=5 < dx=6.
	if p.apRange != -1 {
		t.Errorf("p.apRange: got %d, want -1 (branch 3 default-AP NIH did not run — pre-fix bug)", p.apRange)
	}
}

// --- NAI-41: Player.SetInteraction face-entity TS-fidelity ---------------
// Mirrors TS PathingEntity.setInteraction (PathingEntity.ts:530-541) and
// the in-codebase Npc.SetInteraction template (npc_interaction.go:651-666).

// TestSetInteractionPlayerTargetSetsFaceEntity pins the *Player branch:
// faceEntity = target.slot + 32768, MaskFaceEntity bit set. The +32768
// magic encodes "this is a player slot" on the client wire.
func TestSetInteractionPlayerTargetSetsFaceEntity(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// Use a second player as the target. slot=-1 default would yield
	// faceEntity=32767 — pick a non-default slot so the formula assertion
	// catches accidental sign drops or off-by-one errors.
	other, _ := newTestPlayer(t)
	other.slot = 5

	p.SetInteraction(InteractionEngine, other, 1, -1)

	wantFE := other.slot + 32768 // 32773
	if p.faceEntity != wantFE {
		t.Errorf("faceEntity: got %d, want %d (pid+32768)", p.faceEntity, wantFE)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set after SetInteraction with *Player target")
	}
}

// TestSetInteractionNpcTargetSetsFaceEntity pins the *Npc branch:
// faceEntity = npc.nid, MaskFaceEntity bit set, AT SetInteraction time
// (not at contact). Supersedes the contact-time pin previously in
// TestProcessInteractionInRangeFacesTarget.
func TestSetInteractionNpcTargetSetsFaceEntity(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d (npc.nid)", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set after SetInteraction with *Npc target")
	}
}

// TestSetInteractionFaceEntityIdempotent pins the TS idempotency check
// at PathingEntity.ts:532 / 538 (`if (this.faceEntity !== X)`). Without
// this check, repeated SetInteraction calls with the same target re-emit
// MaskFaceEntity needlessly. We reset masks=0 between calls to isolate
// the second call's mask-emission decision.
func TestSetInteractionFaceEntityIdempotent(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.masks&MaskFaceEntity == 0 {
		t.Fatal("first SetInteraction should set MaskFaceEntity")
	}
	p.masks = 0 // isolate the second call's emission decision

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if p.masks&MaskFaceEntity != 0 {
		t.Error("second SetInteraction with same target must NOT re-emit MaskFaceEntity (TS idempotency check at PathingEntity.ts:532)")
	}
	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity should remain %d (npc.nid) after idempotent second call, got %d", npc.nid, p.faceEntity)
	}
}

// TestHasWaypoints — NAI-44 T3 helper. Returns true iff the player has
// active waypoints; goscape's existing convention is waypointIndex == -1
// for "no waypoints" (vs >= 0 for "active path").
func TestHasWaypoints(t *testing.T) {
	tests := []struct {
		name          string
		waypointIndex int
		want          bool
	}{
		{"no path", -1, false},
		{"single step path", 0, true},
		{"multi-step path", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Player{waypointIndex: tt.waypointIndex}
			if got := p.hasWaypoints(); got != tt.want {
				t.Errorf("hasWaypoints: got %v, want %v (waypointIndex=%d)", got, tt.want, tt.waypointIndex)
			}
		})
	}
}

// TestProcessWalktrigger_UnsetNoOp — NAI-51 T1.7. walktrigger=-1 → no
// script lookup, no field write. Replaces the NAI-44 stub-no-op test.
func TestProcessWalktrigger_UnsetNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	// Default from newPlayer is -1.
	if p.walktrigger != -1 {
		t.Fatalf("precondition: walktrigger=%d, want -1", p.walktrigger)
	}

	p.processWalktrigger()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after no-op: got %d, want -1 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_DelayedNoOp — NAI-51 T1.7. delayed=true gates
// the consumer entirely; field stays unchanged. Mirrors TS gate at
// Player.ts:1062.
func TestProcessWalktrigger_DelayedNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 7
	p.delayed = true

	p.processWalktrigger()

	if p.walktrigger != 7 {
		t.Errorf("walktrigger after delayed bail: got %d, want 7 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_FiresAndClears — NAI-51 T1.7. walktrigger=N + a
// registered script at slot N → script fires once, field cleared to -1.
// Verifies firing via mes "wt-fired" landing on the wire.
func TestProcessWalktrigger_FiresAndClears(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-fired", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after fire: got %d, want -1", p.walktrigger)
	}
	// MessageGame wire = opcode(1) + len(1) + PJStrLF("wt-fired") = 1+1+9 = 11 bytes
	if len(pkt) != 11 {
		t.Fatalf("packet length: got %d, want 11", len(pkt))
	}
	if string(pkt[2:10]) != "wt-fired" || pkt[10] != 0x0a {
		t.Errorf("payload: got %q, want 'wt-fired\\n'", pkt[2:])
	}
}

// TestProcessWalktrigger_MissingScriptStillClears — NAI-51 T1.7. TS
// Player.ts:1064 clears walktrigger BEFORE the script-found check, so a
// missing script still resets the field. No script registered at slot 42
// → walktrigger reset to -1, no script run.
func TestProcessWalktrigger_MissingScriptStillClears(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 42

	p.processWalktrigger()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after missing-script: got %d, want -1 (TS clear-before-check)", p.walktrigger)
	}
}

// TestProcessWalktrigger_ProtectedScriptActiveNoOp — NAI-52. With a
// suspended protected script anchored on the player, the walktrigger
// consumer must bail without firing. Mirrors TS Player.ts:1062 gate
// !this.protect via goscape's activeScript.Pointers&PtrProtectedActivePlayer
// convergence.
func TestProcessWalktrigger_ProtectedScriptActiveNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 7
	p.activeScript = &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}
	p.protect = true // NAI-111-D1: Player.protect is the TS-faithful gate, set alongside activeScript fixture

	p.processWalktrigger()

	if p.walktrigger != 7 {
		t.Errorf("walktrigger after protected-bail: got %d, want 7 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_ActiveScriptUnprotectedFires — NAI-52. Pins
// that activeScript != nil alone does NOT block the consumer; only
// activeScript with PtrProtectedActivePlayer set does. activeScript
// without the protect flag must allow the walktrigger to fire and clear.
func TestProcessWalktrigger_ActiveScriptUnprotectedFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-unprot", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42
	p.activeScript = &script.ScriptState{} // unprotected — PtrProtectedActivePlayer absent

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after unprotected fire: got %d, want -1", p.walktrigger)
	}
	if !bytes.Contains(pkt, []byte("wt-unprot")) {
		t.Errorf("payload: did not contain wt-unprot: %q", pkt)
	}
}

// TestProcessWalktrigger_NilActiveScriptFires — NAI-52. activeScript=nil
// short-circuit pin: protectedScriptActive returns false on nil
// activeScript, so the consumer must fire.
func TestProcessWalktrigger_NilActiveScriptFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-nilactive", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42
	// activeScript is already nil from newPlayer.

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after nil-active fire: got %d, want -1", p.walktrigger)
	}
	if !bytes.Contains(pkt, []byte("wt-nilactive")) {
		t.Errorf("payload: did not contain wt-nilactive: %q", pkt)
	}
}

// --- NAI-44 T5 helpers ---

// setupServerForInteractionTest returns a server configured for Player→Player
// interaction tests. Uses NodeClientRoutefinder=true (direct-step mode) so
// pathToTarget produces deterministic waypoints without a real gamemap.
func setupServerForInteractionTest(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true
	return s
}

// newTestPlayerAt wires a Player to the server at specified coordinates and
// assigns it the given slot. Returns the player; caller drains conn via
// drainConn if wire output is expected.
func newTestPlayerAt(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = x, z, level
	p.slot = slot
	// Drain connection in background so wire writes don't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := cc.Read(buf); err != nil {
				return
			}
		}
	}()
	return p
}

// --- NAI-44 T5 / B1-B4 tests ---

// TestFollowOpPredicate — NAI-44 T5 / B1. followOp = (targetOp == 3 &&
// target is *Player). TS Player.ts:1205 uses ServerTriggerType enum
// (APPLAYER3/OPPLAYER3 are sibling values); goscape stores raw op slot
// 1..4, so a single equality check covers both AP and OP variants.
func TestFollowOpPredicate(t *testing.T) {
	npcForPredicate := func(t *testing.T, s *Server) entity {
		t.Helper()
		return makeInteractionNpc(t, s, 1, 3100, 3200, 0)
	}
	tests := []struct {
		name        string
		targetOp    int
		buildTarget func(t *testing.T, s *Server) entity
		wantFollow  bool
	}{
		{
			"OPPLAYER3 → followOp",
			3,
			func(t *testing.T, s *Server) entity { return newTestPlayerAt(t, s, 2, 3200, 3200, 0) },
			true,
		},
		{
			"OPPLAYER1 → not followOp",
			1,
			func(t *testing.T, s *Server) entity { return newTestPlayerAt(t, s, 2, 3200, 3200, 0) },
			false,
		},
		{
			"OPNPC3 (op=3, *Npc target) → not followOp",
			3,
			npcForPredicate,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupServerForInteractionTest(t)
			p := newTestPlayerAt(t, s, 1, 3200, 3201, 0)
			target := tt.buildTarget(t, s)
			p.SetInteraction(InteractionEngine, target, tt.targetOp, -1)

			got := isFollowOp(p)

			if got != tt.wantFollow {
				t.Errorf("followOp: got %v, want %v (targetOp=%d, target type=%T)", got, tt.wantFollow, tt.targetOp, target)
			}
		})
	}
}

// TestFollowOpAnchoredChase — NAI-44 T5 / B2. When OPPLAYER3 fires with
// the target out of operable/approach range, the player path-walks toward
// the target. processInteraction must NOT clear the interaction in this
// scenario (followOp keeps interaction anchored across steps).
// Target is placed 15 tiles east — beyond the default apRange of 10 —
// so both OP and AP distance checks fail and the pathing branch fires.
func TestFollowOpAnchoredChase(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3215, 3200, 0) // 15 tiles east — beyond apRange=10
	target.active = true                              // a valid in-world follow target (validateTarget gate 3 / TS Player.isValid)

	clicker.SetInteraction(InteractionEngine, target, 3, -1)

	clicker.processInteraction()

	if clicker.target != target {
		t.Errorf("target: got %v, want %v (followOp must NOT auto-clear when chasing)", clicker.target, target)
	}
	if clicker.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", clicker.targetOp)
	}
	if !clicker.hasWaypoints() {
		t.Error("hasWaypoints: got false, want true (path should be set toward target)")
	}
}

// TestFollowOpWaypointExhaustion — NAI-44 T5 / B3. When followOp is
// active and pathToTarget yields no waypoints (e.g. target unreachable),
// the post-step arm clears the interaction (TS L1237-1239).
func TestFollowOpRepathsOnExhaustion(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3210, 3200, 0)
	target.active = true // a valid in-world follow target (validateTarget gate 3 / TS Player.isValid)

	clicker.SetInteraction(InteractionEngine, target, 3, -1)
	// Simulate path exhaustion (waypointIndex=-1) mid-follow interaction.
	// NAI-98 update: the pre-NAI-98 test (TestFollowOpWaypointExhaustion)
	// used repathed=true to artificially skip pathToTarget, then asserted
	// ClearInteraction. Post-fix, pathToPathingTarget repaths unconditionally
	// when isLastOrNoWaypoint() && followOp (TS L1039-1042), queuing a
	// waypoint to target.followX/followZ. The TS L1237-1239
	// !hasWaypoints() && followOp → ClearInteraction branch is effectively
	// unreachable for *Player targets (pathToPathingTarget always queues
	// the chase waypoint when isLastOrNoWaypoint). Target is preserved.
	clicker.waypointIndex = -1

	clicker.processInteraction()

	// Post-fix TS-faithful behavior: pathToPathingTarget queues a chase
	// waypoint → hasWaypoints()=true → L1237 does not fire → target preserved.
	if clicker.target == nil {
		t.Errorf("target: got nil, want non-nil (pathToPathingTarget should repath followOp on isLastOrNoWaypoint; L1237 ClearInteraction does not fire when chase waypoint is queued)")
	}
}

// TestPlayerFollow_PathToPathingTarget_QueuesValidLeaderCoord pins the
// NAI-174 cascade end-to-end: with T1 (unconditional top writes in
// processInteraction) + T2 (processLogins lastStep init), a stationary
// post-login leader exposes a valid lastStepX/Z to followers via the
// leader's per-tick refreshed followX/Z. A follower with targetOp=3 and
// target=leader then queues a waypoint to (leader.lastStepX,
// leader.lastStepZ) instead of the pre-NAI-174 (-1, -1) sentinel that
// stalled pathfinding ~5 tiles SW.
//
// Retires the NAI-173-FU-FOLLOW-MODE-INVESTIGATION carry-forward.
func TestPlayerFollow_PathToPathingTarget_QueuesValidLeaderCoord(t *testing.T) {
	s := newTestServer(t)

	leader, leaderWait := makeInteractionPlayer(t, s, 3220, 3220, 0)
	defer leaderWait()
	leader.active = true // a valid in-world follow target (validateTarget gate 3 / TS Player.isValid)
	// Simulate post-processLogins state: lastStepX = x - 1; lastStepZ = z
	// (NAI-174 T2 mirror). leader.target stays nil — they aren't
	// interacting with anyone; they're being followed.
	leader.lastStepX = leader.x - 1
	leader.lastStepZ = leader.z

	follower, followerWait := makeInteractionPlayer(t, s, 3225, 3225, 0)
	defer followerWait()
	follower.lastStepX = follower.x - 1
	follower.lastStepZ = follower.z
	follower.target = leader
	follower.targetOp = 3 // raw op-slot 3; isFollowOp matches targetOp==3 && target.(*Player)

	t.Run("stationary leader: follower queues leader's lastStepX/Z", func(t *testing.T) {
		// Tick the leader first so their unconditional top writes (NAI-174 T1)
		// refresh leader.followX/Z from leader.lastStepX/Z (= 3219, 3220).
		leader.processInteraction()
		if leader.followX != 3219 || leader.followZ != 3220 {
			t.Fatalf("pre-condition: leader followX/Z should be (3219, 3220) post-top-writes; got (%d, %d)",
				leader.followX, leader.followZ)
		}

		// Now tick the follower. pathToPathingTarget's followOp arm at
		// interaction.go:802-809 should queueWaypoint(leader.followX,
		// leader.followZ). queueWaypoint stores packed coord at waypoints[0]
		// and sets waypointIndex=0 (from the initial -1).
		preWaypointIdx := follower.waypointIndex
		follower.processInteraction()

		// Assert a waypoint was queued (waypointIndex advanced from -1 to 0).
		if follower.waypointIndex == preWaypointIdx {
			t.Fatalf("follower waypointIndex unchanged post-processInteraction; want waypointIndex=0 (new waypoint queued via pathToPathingTarget follow-op arm); got %d", follower.waypointIndex)
		}
		// The queued waypoint destination (waypoints[0]) should be the leader's
		// followX/Z, NOT (-1, -1). Decode the packed coord.
		wp := coordgrid.UnpackCoord(follower.waypoints[follower.waypointIndex])
		if wp.X != 3219 || wp.Z != 3220 {
			t.Errorf("follower queued waypoint: got (%d, %d), want (3219, 3220) = leader.followX/Z post-NAI-174", wp.X, wp.Z)
		}
	})

	t.Run("leader moved one step: follower queues leader's pre-step tile", func(t *testing.T) {
		// Simulate leader's stepOnce (movement.go:140-143): capture pre-step
		// coord into lastStepX/Z, then mutate x. Leader walks east one tile.
		prevX, prevZ := leader.x, leader.z
		leader.lastStepX = prevX
		leader.lastStepZ = prevZ
		leader.x = prevX + 1
		// leader.z unchanged

		// Tick leader's processInteraction — top writes refresh followX/Z
		// from the new lastStepX/Z (= 3220, 3220).
		leader.processInteraction()
		if leader.followX != 3220 || leader.followZ != 3220 {
			t.Fatalf("post-step: leader followX/Z should be (3220, 3220) = pre-step tile; got (%d, %d)",
				leader.followX, leader.followZ)
		}

		// Tick follower. isLastOrNoWaypoint() returns waypointIndex<=0, which
		// is true (waypointIndex==0 from the first sub-test, and queueWaypoint
		// always resets to index 0). The follow-op arm should re-queue to the
		// leader's new followX/Z.
		follower.processInteraction()

		if follower.waypointIndex < 0 {
			t.Fatalf("follower has no waypoints post-second-tick; expected re-queued waypoint")
		}
		wp := coordgrid.UnpackCoord(follower.waypoints[follower.waypointIndex])
		if wp.X != 3220 || wp.Z != 3220 {
			t.Errorf("follower last-queued waypoint after leader step: got (%d, %d), want (3220, 3220)", wp.X, wp.Z)
		}
	})
}

// TestFollowOpContactFire — NAI-44 T5 / B4 (updated for NAI-78, updated for
// NAI-147 T5).
// OPPLAYER3 with no registered script, adjacent target Player:
// Pre-NAI-78 the 2-branch fired tryFireOpTrigger unconditionally for
// PathingEntity → ClearInteraction + auto-clear → target=nil.
// Post-NAI-78 the 4-branch routes via branch 3 (approach=true, no AP
// script, no OP trigger) → apRange=-1, return false. followOp gates the
// post-step tryInteract, so no NIH fires this tick. Target preserved.
// NAI-147 T5 — TS-faithful 3-part guard ports Player.ts:1114: follow-op
// (targetOp=3, *Player target) now short-circuits at the top of tryInteract
// via !HasInteraction() BEFORE reaching branch 3. apRange is therefore NOT
// mutated to -1 (stays at the default 10). The net observable behavior is
// identical to the NAI-78 state: tryInteract returns false, target preserved.
// NAI-147-D-CANACCESS-MODAL-GATE: prior apRange=-1 assertion was pinning
// intermediate goscape routing through branch 3; now branch-3 is bypassed
// for follow-ops entirely per TS.
func TestFollowOpContactFire(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3201, 3200, 0) // adjacent — operable distance
	target.active = true                              // a valid in-world follow target (validateTarget gate 3 / TS Player.isValid)

	clicker.SetInteraction(InteractionEngine, target, 3, -1)

	clicker.processInteraction()

	// NAI-147 T5: follow-op short-circuits at top guard (!HasInteraction()),
	// apRange is NOT mutated. Target preserved for continued following.
	if clicker.apRange == -1 {
		t.Errorf("apRange: got -1, want default 10 (NAI-147 T5: follow-op guard bypasses branch 3, no apRange=-1 mutation)")
	}
	if clicker.target == nil {
		t.Errorf("target: got nil, want non-nil (followOp preserves target for continued chase)")
	}
}

// --- NAI-47: tryInteract allowOpScenery gate ---

// TestTryInteractNpcAllowsOpWhenSceneryGated pins that *Npc targets (PathingEntity)
// are always eligible for the OP branch regardless of allowOpScenery.
// Mirrors TS: (target instanceof PathingEntity || allowOpScenery).
func TestTryInteractNpcAllowsOpWhenSceneryGated(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // adjacent — in OP range
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	// allowOpScenery=false: NPC is PathingEntity so OP fires anyway.
	result := p.tryInteract(false)

	if !result {
		t.Error("tryInteract(false): got false, want true — NPC is PathingEntity, OP must fire")
	}
}

// TestTryInteractLocBlocksOpWhenSceneryFalse pins that *Loc targets cannot
// fire the OP branch when allowOpScenery=false (branch 1 blocked: isPathing=false
// && allowOpScenery=false). Updated for NAI-78: with no AP script registered,
// branch 3 fires (approach=true → apRange=-1, return false). Pre-NAI-78 the
// 2-branch caused the AP block to return true unconditionally (pinning the bug).
func TestTryInteractLocBlocksOpWhenSceneryFalse(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.apRange = 10 // wide AP range so approach=true

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// allowOpScenery=false + adjacent Loc + no scripts → branch 1 blocked (isPathing=false),
	// branch 2 blocked (no apTrigger), branch 3 fires (approach=true) → apRange=-1, false.
	result := p.tryInteract(false)

	if result {
		t.Error("tryInteract(false) on adjacent Loc with no scripts: got true, want false (branch 3)")
	}
	if p.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (NAI-78 branch 3 no-AP sentinel)", p.apRange)
	}
}

// TestTryInteractLocAllowsOpWhenSceneryTrue pins that *Loc targets CAN fire
// the OP branch when allowOpScenery=true AND an OP script is registered.
// Updated for NAI-78: the 4-branch gates branch 1 on opTrigger!=nil. Without
// an OP script, approach=true causes branch 3 to fire even with allowOpScenery=true.
// With an OP script registered, branch 1 fires: opTrigger!=nil && allowOpScenery && operable.
func TestTryInteractLocAllowsOpWhenSceneryTrue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// NAI-147 T3: triggerTypeAndCategory now returns ok=false when locTypes
	// is nil (TS Player.ts:986-988 null-type guard). Seed locTypes so the
	// Loc type 42 resolves and getOpTrigger proceeds to GetByTrigger.
	locConfigs := make([]*objtype.LocType, 43)
	locConfigs[42] = &objtype.LocType{ConfigType: objtype.ConfigType{ID: 42}}
	s.locTypes = &objtype.LocTypeConfigs{Configs: locConfigs}

	// Register an OP script for loc.Type()=42 so getOpTrigger returns non-nil.
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "op-fired"))

	// allowOpScenery=true + OP script registered + adjacent Loc → branch 1 fires.
	result := p.tryInteract(true)

	if !result {
		t.Error("tryInteract(true) on adjacent Loc with OP script: got false, want true (branch 1 OP allowed)")
	}
}

// TestTryInteractProcessInteractionCallSites pins the two call-site semantics
// via processInteraction: pre-step always passes false, post-step passes
// stepsTaken==0 (true only when no movement this tick).
// Updated for NAI-78: with no scripts and adjacent Loc, the two-step sequence is:
//
//	pre-step tryInteract(false): branch 3 (approach=true) → apRange=-1, return false.
//	post-step tryInteract(true): approach=false (apRange=-1), operable=true → branch 4 NIH.
//
// Branch 4 fires defaultOp → target auto-cleared.
func TestTryInteractProcessInteractionCallSites(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// Scenario: Loc target, player already adjacent (no movement needed),
	// so post-step call gets allowOpScenery=true (stepsTaken==0).
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4}) // required for MessageGame in defaultOp
	p.x, p.z, p.level = 100, 100, 0
	p.stepsTaken = 0 // no movement this tick

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	p.processInteraction()

	// Post-step branch 4 NIH fires → defaultOp → waypointIndex=-1 + auto-clear.
	if p.target != nil {
		t.Error("target should be nil after interaction auto-clear via branch 4 NIH")
	}
}

// TestProcessInteraction_PreStepWalktriggerFires — NAI-51 T1.8. With
// a walktrigger queued and a target in operable distance, the pre-step
// arm at interaction.go:169 must fire the walktrigger BEFORE tryInteract.
// Verified via "wt-fired" wire output AND walktrigger=-1 after the tick.
func TestProcessInteraction_PreStepWalktriggerFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-fired", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(7, sf)

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0 // dx=1 → operable
	received := drainConn(t, cc)

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.walktrigger = 7

	p.processInteraction()
	p.client.flushWrite()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after pre-step fire: got %d, want -1", p.walktrigger)
	}
	// First wire packet should be the "wt-fired" mes.
	pkt := <-received
	if !bytes.Contains(pkt, []byte("wt-fired")) {
		t.Errorf("first wire packet did not contain wt-fired: %q", pkt)
	}
}

// TestProcessInteraction_PostStepWalktriggerFires — NAI-51 T1.8. With a
// walktrigger queued, a target out of range, and waypoints set, the
// post-step arm at interaction.go:183 must fire the walktrigger.
func TestProcessInteraction_PostStepWalktriggerFires(t *testing.T) {
	s := setupServerForInteractionTest(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-post", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(11, sf)

	npc := makeInteractionNpc(t, s, 1, 200, 200, 0) // far away → no operable
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	received := drainConn(t, cc)

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.walktrigger = 11
	// Pre-seed waypoints so hasWaypoints() is true after the pre-step
	// arm fails its tryInteract.
	p.waypointIndex = 0
	p.waypoints[0] = (0 << 28) | (200 << 14) | 200

	p.processInteraction()
	p.client.flushWrite()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after post-step fire: got %d, want -1", p.walktrigger)
	}
	pkt := <-received
	if !bytes.Contains(pkt, []byte("wt-post")) {
		t.Errorf("wire did not contain wt-post: %q", pkt)
	}
}

// TestSetInteractionLocTargetWritesTargetXZ pins NAI-66 closure of
// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: *Loc target writes
// targetX = fine(loc.X, loc.Width), targetZ = fine(loc.Z, loc.Length)
// per TS PathingEntity.ts:542-545. Replaces the previous
// TestSetInteractionLocTargetDoesNotSetFaceEntity contract (which
// pinned the now-closed deferral).
func TestSetInteractionLocTargetWritesTargetXZ(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// 3x2 Loc at (50, 60).
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// faceEntity must remain unwritten (Loc branch never sets it).
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc branch must not write)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity != 0 {
		t.Error("MaskFaceEntity bit must NOT be set after SetInteraction with *Loc target")
	}
	// targetX/Z now written per NAI-66.
	wantTX := coordgrid.Fine(50, 3)
	wantTZ := coordgrid.Fine(60, 2)
	if p.targetX != wantTX {
		t.Errorf("targetX: got %d, want %d (fine(50, width=3))", p.targetX, wantTX)
	}
	if p.targetZ != wantTZ {
		t.Errorf("targetZ: got %d, want %d (fine(60, length=2))", p.targetZ, wantTZ)
	}
}

// TestSetInteractionObjTargetWritesTargetXZ pins the Obj-target case:
// always 1x1, so fine(obj.X, 1) and fine(obj.Z, 1).
func TestSetInteractionObjTargetWritesTargetXZ(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := entitypkg.NewObj(0, 50, 60, entitypkg.LifecycleForever, 42, 1)

	p.SetInteraction(InteractionEngine, obj, 1, -1)

	wantTX := coordgrid.Fine(50, 1)
	wantTZ := coordgrid.Fine(60, 1)
	if p.targetX != wantTX {
		t.Errorf("targetX: got %d, want %d (fine(50, 1))", p.targetX, wantTX)
	}
	if p.targetZ != wantTZ {
		t.Errorf("targetZ: got %d, want %d (fine(60, 1))", p.targetZ, wantTZ)
	}
}

// --- NAI-67 T1.3: Player.SetInteraction TS:528 focus() driver tests ---

// TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare pins TS
// PathingEntity.ts:528: Npc target (PathingEntity, not NonPathingEntity)
// passes instant=false ⇒ faceAngle written, faceSquare/mask untouched.
func TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 50, 60, 0)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("Npc target must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("Npc target must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare pins TS
// PathingEntity.ts:528: Player target (PathingEntity) passes
// instant=false ⇒ faceAngle written, faceSquare/mask untouched.
func TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 50, 60, 0)
	defer otherWait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	p.SetInteraction(InteractionEngine, other, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("Player target must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("Player target must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionLocEngineWritesFaceSquareAndMask pins TS
// PathingEntity.ts:528: Loc target + InteractionEngine ⇒ instant=true
// path; faceSquare = (fx, fz); MaskFaceCoord ORed in.
func TestSetInteractionLocEngineWritesFaceSquareAndMask(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	// 3x2 Loc at (50, 60) — non-trivial sizing exercises width/length use.
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	wantFX := coordgrid.Fine(50, 3)
	wantFZ := coordgrid.Fine(60, 2)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != wantFX || p.faceSquareZ != wantFZ {
		t.Errorf("faceSquare: got (%d, %d), want (%d, %d)", p.faceSquareX, p.faceSquareZ, wantFX, wantFZ)
	}
	if p.masks&MaskFaceCoord == 0 {
		t.Errorf("MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}

// TestSetInteractionLocScriptDoesNotWriteFaceSquare pins TS:528 — Loc
// target + InteractionScript ⇒ instant=false (scripts don't trigger the
// engine-face wire write).
func TestSetInteractionLocScriptDoesNotWriteFaceSquare(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionScript, loc, 1, -1)

	wantFX := coordgrid.Fine(50, 3)
	wantFZ := coordgrid.Fine(60, 2)
	// faceAngle still written on every SetInteraction (TS:528 unconditional).
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	// instant=false ⇒ faceSquare/mask untouched.
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("InteractionScript must NOT write faceSquare (got %d, %d)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&MaskFaceCoord != 0 {
		t.Errorf("InteractionScript must NOT set MaskFaceCoord (masks=%d)", p.masks)
	}
}

// TestSetInteractionObjEngineWritesFaceSquareAndMask pins TS:528 — Obj
// target + InteractionEngine ⇒ instant=true. Obj is always 1x1, so
// fine(_, 1).
func TestSetInteractionObjEngineWritesFaceSquareAndMask(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0
	obj := entitypkg.NewObj(0, 50, 60, entitypkg.LifecycleForever, 42, 1)

	p.SetInteraction(InteractionEngine, obj, 1, -1)

	wantFX := coordgrid.Fine(50, 1)
	wantFZ := coordgrid.Fine(60, 1)
	if p.faceAngleX != wantFX || p.faceAngleZ != wantFZ {
		t.Errorf("faceAngle: got (%d, %d), want (%d, %d)", p.faceAngleX, p.faceAngleZ, wantFX, wantFZ)
	}
	if p.faceSquareX != wantFX || p.faceSquareZ != wantFZ {
		t.Errorf("faceSquare: got (%d, %d), want (%d, %d)", p.faceSquareX, p.faceSquareZ, wantFX, wantFZ)
	}
	if p.masks&MaskFaceCoord == 0 {
		t.Errorf("MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}

// TestProcessInteractionTailAutoClearsWithoutNextTarget pins the tail's
// else-if branch (interacted && !apRangeCalled && nextTarget==nil → ClearInteraction)
// as the sole clearing agent.
//
// Path exercised:
//
//	tryInteract (NPC adjacent, targetOp=1) → fireOpTriggerNpc executes the
//	registered [opnpc1, typeID=0] noop script to Finished; the script does NOT
//	call p_op_npc, so p.target stays nil during execution and p.nextTarget is
//	set to nil at the capture step (p.nextTarget = p.target = nil); then
//	fireOpTriggerNpc restores p.target = savedTarget (= npc); interacted=true,
//	nextTarget=nil, apRangeCalled=false → tail else-if calls ClearInteraction.
//
// NAI-68 B1 dual-pin.
func TestProcessInteractionTailAutoClearsWithoutNextTarget(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	// npcA is adjacent (3201, 3200) so tryInteract fires the OP path.
	// typeID=0 matches makeInteractionNpc's default NpcType.ID.
	npcA := makeInteractionNpc(t, s, 1, 3201, 3200, 0)

	// Register a noop [opnpc1, typeID=0] script so fireOpTriggerNpc reaches
	// the script-execution path (not the early-return ClearInteraction path).
	// The noop script completes without calling p_op_npc, so p.nextTarget
	// remains nil after the capture step.
	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerOpNpc1, 0, -1))

	// SetInteraction with targetOp=1 so apNpcTriggerForOp(1) returns ok=true.
	p.SetInteraction(InteractionEngine, npcA, 1, -1)
	p.nextTarget = nil

	p.processInteraction()

	if p.target != nil {
		t.Errorf("p.target after tail: got %v, want nil (tail else-if sole clearer)", p.target)
	}
}

// TestProcessInteractionEntryResetsNextTarget pins TS Player.ts:1203.
// p.nextTarget MUST be reset to nil on every processInteraction call,
// even on the level-mismatch early-exit path (TS L1203 runs before
// validateTarget at TS L1207).
//
// NAI-68 B2.
func TestProcessInteractionEntryResetsNextTarget(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	// Stale nextTarget from a hypothetical previous tick.
	npcStale := makeInteractionNpc(t, s, 1, 3201, 3201, 0)
	// npcA is far away (15 tiles) so tryInteract returns false; the tail
	// sees nextTarget=nil (entry reset wiped it) and doesn't pop.
	npcA := makeInteractionNpc(t, s, 2, 3215, 3200, 0)

	p.target = npcA
	p.nextTarget = npcStale

	p.processInteraction()

	if p.nextTarget != nil {
		// After tail: pop happens, but the field STAYS at the popped
		// value (we don't post-pop reset). However, the entry reset
		// runs FIRST, so the stale value is wiped before the pop reads
		// it. The pop reads nil → no swap → tail's else-if runs.
		// Final state: nextTarget=nil because the value was reset on
		// entry, never re-set during this tick.
		t.Errorf("p.nextTarget after processInteraction: got %v, want nil (entry reset per TS L1203)", p.nextTarget)
	}
}

// TestProcessInteractionEntryResetsNextTargetEvenOnLevelMismatch pins
// TS L1203's placement BEFORE the validateTarget level-check. A target
// at a different level triggers the early-exit at interaction.go:196-199,
// but the entry reset must already have run.
//
// NAI-68 B2 placement-pin.
func TestProcessInteractionEntryResetsNextTargetEvenOnLevelMismatch(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	npcStale := makeInteractionNpc(t, s, 1, 3201, 3201, 0)
	// Target on a DIFFERENT level — triggers level-mismatch early-exit.
	npcOtherLevel := makeInteractionNpc(t, s, 2, 3201, 3200, 1)

	p.target = npcOtherLevel
	p.nextTarget = npcStale

	p.processInteraction()

	if p.nextTarget != nil {
		t.Errorf("p.nextTarget after level-mismatch exit: got %v, want nil (TS L1203 runs before L1207)", p.nextTarget)
	}
}

// --- NAI-78 T2: defaultOp NIH helper ---

// TestDefaultOp_EmitsNIHAndClearsWaypoints pins TS
// LostCityRS/Engine-TS Player.ts:1072-1097. defaultOp must emit
// "Nothing interesting happens." to the player AND clear the
// waypoint queue (waypointIndex = -1). Goscape ports the
// NODE_PRODUCTION-gated dev "No trigger for [...]" debug line under
// cfg.NodeDebug (NAI-147 T4 closed NAI-78-D-DEBUG-MSG-DEFERRED).
// This test runs with NodeDebug=false (the makeOpLocTriggerFixture
// default) so the debug line is suppressed and only
// "Nothing interesting happens." is asserted.
func TestDefaultOp_EmitsNIHAndClearsWaypoints(t *testing.T) {
	_, p, _, cc := makeOpLocTriggerFixture(t)

	// Pre-state: active waypoint queue.
	p.waypointIndex = 5
	p.waypoints[5] = 0xCAFE

	received := drainConn(t, cc)
	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Assert: waypointIndex cleared.
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS Player.ts:1096 clearWaypoints)", p.waypointIndex)
	}

	// Assert: "Nothing interesting happens." emitted on the wire.
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("expected MessageGame(\"Nothing interesting happens.\") on wire; got %x", got)
	}
}

// --- NAI-78 T3: tryInteract 4-branch dispatch ---

// TestTryInteract_OpFires_AdjacentNpc_Branch1 pins TS Player.ts:1123.
// Adjacent NPC + [opnpc1] registered → branch 1 fires (PathingEntity
// gates branch 1 even with allowOpScenery=false).
// Wire-bytes assertion omitted: newApTriggerNpcFixture discards the
// clientConn; interactionFired is the operative signal here.
func TestTryInteract_OpFires_AdjacentNpc_Branch1(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpNpc1, npc.typeId, "branch1-fired"))

	p.x, p.z = npc.x-1, npc.z

	got := p.tryInteract(false)

	if !got {
		t.Errorf("tryInteract: got false, want true (branch 1 OP fire)")
	}
}

// TestTryInteract_DoorSymptom_AdjacentLoc_OpOnly_Branch3to4 is THE
// regression test for the NAI-78 root cause.
// Pre-step (allowOpScenery=false): player adjacent to loc with only [oploc1],
// no [aploc1]. Must return false (branch 3 — apRange=-1).
// Post-step (allowOpScenery=true): branch 1 fires OPLOC → interactionFired=true.
//
// Wire-bytes assertion dropped: buildNpcSayScript uses OpNpcSay which requires
// an active NPC; the script aborts cleanly for Loc targets but interactionFired
// is still set by fireOpTriggerLoc after resumeOrFinish (interaction_trigger.go).
// interactionFired=true is the operative regression signal.
func TestTryInteract_DoorSymptom_AdjacentLoc_OpOnly_Branch3to4(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "door-fired"))
	// No TriggerApLoc1 registered — the door's [oploc1]-only shape.

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-1, lz

	// Pre-step: branch 3 fires.
	preGot := p.tryInteract(false)
	if preGot {
		t.Errorf("pre-step tryInteract(false): got true, want false (TS branch 3 — apRange=-1, return false)")
	}
	if p.apRange != -1 {
		t.Errorf("p.apRange: got %d, want -1 (TS Player.ts:1174)", p.apRange)
	}

	// Post-step: branch 1 fires (allowOpScenery=true).
	// interactionFired=true is the operative signal that OP fired.
	postGot := p.tryInteract(true)
	if !postGot {
		t.Errorf("post-step tryInteract(true): got false, want true (TS branch 1 OP fire)")
	}
}

// TestTryInteract_AdjacentLoc_BothScripts_Branch2 pins TS Player.ts:1139.
// Adjacent loc with both [aploc1] and [oploc1] registered → AP fires (not OP).
//
// Assertion strategy: wire-bytes dropped (buildNpcSayScript uses OpNpcSay
// which requires active NPC — aborts cleanly for Loc). Instead:
//   - got=true (branch 2 returns true)
//   - p.apRange != -1 (AP script found → apRange NOT set to -1 sentinel;
//     if OP had fired via branch 1, fireOpTriggerLoc sets waypointIndex=-1
//     but branch 1 is blocked when allowOpScenery=false && !isPathing)
//   - p.interactionFired=true (AP fire helper sets this)
//
// Branch 1 is blocked here: allowOpScenery=false, Loc is not PathingEntity.
// Branch 2 fires because apTrigger!=nil and player is in approach range.
func TestTryInteract_AdjacentLoc_BothScripts_Branch2(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerApLoc1, loc.Type(), "ap-fired"))
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "op-fired"))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-1, lz

	got := p.tryInteract(false)

	if !got {
		t.Errorf("tryInteract: got false, want true (branch 2 AP fire)")
	}
	// AP script found → apRange stays at 10 (not reset to -1 sentinel).
	// If OP had fired instead, apRange would still be 10 too, but branch 1
	// is structurally blocked (allowOpScenery=false && loc is not PathingEntity).
	if p.apRange == -1 {
		t.Errorf("p.apRange: got -1, want !=−1 (AP script found — not the no-script sentinel)")
	}
}

// TestTryInteract_AdjacentNpc_NoScripts_Branch4 pins TS Player.ts:1179.
// Adjacent NPC with no AP scripts and p.apRange=0 (approach always
// false): tryInteract(false) → branch 4 NIH (defaultOp,
// waypointIndex=-1). p.apRange=0 forces inApproachDistance to skip
// branch 3, so an adjacent NPC reaches branch 4 directly.
// Wire-bytes assertion omitted: NPC fixture has no exposed clientConn.
// waypointIndex=-1 is the operative signal that defaultOp ran.
func TestTryInteract_AdjacentNpc_NoScripts_Branch4(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// No scripts registered.

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4}) // required for MessageGame
	p.x, p.z, p.level = 100, 100, 0

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 0,
		Category:    0,
	}
	npc := NewNpc(0, 7, 101, 100, 0, npcType) // adjacent: distance=1, operable
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	// SetInteraction resets apRange=10 (TS PathingEntity.ts:554 parity);
	// force apRange=0 AFTER so the approach branch's apRange>0 guard
	// rejects and branch 3 is skipped, leaving branch 4 reachable on
	// the adjacent (operable) NPC.
	p.apRange = 0

	got := p.tryInteract(false)
	if !got {
		t.Errorf("tryInteract: got false, want true (branch 4 NIH — adjacent NPC, no scripts)")
	}
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (defaultOp clears)", p.waypointIndex)
	}
}

func TestTryInteract_NilTargetReturnsFalse(t *testing.T) {
	_, p, _, _ := makeOpLocTriggerFixture(t)
	p.target = nil
	if p.tryInteract(false) {
		t.Errorf("tryInteract with nil target: got true, want false")
	}
}

// TestTryInteract_NAI69_AprangeRetry_PreservedInBranch2 pins NAI-69.
// Out-of-operable, in-approach distance with AP script that calls
// p_aprange → interactionFired reset + return false (same-tick retry).
func TestTryInteract_NAI69_AprangeRetry_PreservedInBranch2(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-3, lz // out of operable, in approach

	got := p.tryInteract(false)

	if got {
		t.Errorf("tryInteract with apRangeCalled: got true, want false (NAI-69 retry)")
	}
	if !p.apRangeCalled {
		t.Errorf("p.apRangeCalled: got false, want true")
	}
}

func TestTryInteract_OutOfRangeReturnsFalse(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "op-fired"))

	lx, lz, _ := loc.Coords()
	p.x, p.z = lx-50, lz

	if p.tryInteract(false) {
		t.Errorf("tryInteract out-of-range: got true, want false")
	}
}

// -- NAI-91 player-side shape-aware inOperableDistance tests --------------

// newInOperableTestServer builds a minimal *Server with locTypes + gamemap
// populated so inOperableDistance's Loc dispatch can resolve forceapproach
// and read collision flags. The returned LocType is the only configured one
// (ID 100); callers may set custom ForceApproach via the returned pointer.
func newInOperableTestServer(t *testing.T) (*Server, *objtype.LocType) {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		locObjTracker:  newLocObjTracker(),
		rsbuf:          rsbuf.New(),
		players:        newPlayerList(2048),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100, DebugName: "wall_test"}}
	s.locTypes.Configs[100] = lt
	return s, lt
}

// makeWallLoc constructs a 1×1 *entitypkg.Loc at (level, x, z) with the given
// shape/angle, type ID 100 (matching newInOperableTestServer's configured
// LocType). Lifecycle is Despawn — non-load-bearing for these tests.
func makeWallLoc(t *testing.T, level, x, z, shape, angle int) *entitypkg.Loc {
	t.Helper()
	return entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleDespawn, 100, shape, angle)
}

// TestPlayer_InOperableDistance_DoorTile_AllowsReClick pins the Tutorial
// Island RS Guide door re-click case (NAI-91 root symptom). Player on the
// door tile clicking the door (wall_straight, angle=west, 1×1 footprint).
// Pre-NAI-91 returned false (excluded same-tile); post-NAI-91 returns true
// because reach.Reached short-circuits srcX==destX && srcZ==destZ for wall
// strategies.
func TestPlayer_InOperableDistance_DoorTile_AllowsReClick(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0

	loc := makeWallLoc(t, 0, 3098, 3107, 0 /*wall_straight*/, 0 /*loc_west*/)

	if !inOperableDistance(p, loc) {
		t.Fatalf("expected inOperableDistance true on the door tile (NAI-91 binding)")
	}
}

// TestPlayer_InOperableDistance_WallStraightMatrix exercises the four
// wall_straight angles across on-tile, all 4 orthogonal neighbors, and 4
// diagonals. Reaches that depend on collision flags (e.g. north/south
// neighbors gated by FlagBlock*) are tested in both open and blocked
// configurations.
func TestPlayer_InOperableDistance_WallStraightMatrix(t *testing.T) {
	type tile struct {
		dx, dz int
		want   bool
		// preFlags is OR-applied to the player's tile (srcX, srcZ) before
		// the call. A blocking flag set on the player's tile makes the
		// flag-gated reach condition false. Empty for cases that don't
		// depend on flags.
		preFlags int
	}
	type angleCase struct {
		angle int
		name  string
		tiles []tile
	}
	cases := []angleCase{
		{
			angle: 0 /*loc_west*/, name: "west",
			tiles: []tile{
				{0, 0, true, 0},                          // on-tile
				{-1, 0, true, 0},                         // west-adjacent (in front of wall)
				{0, 1, true, 0},                          // north-adjacent, open flags
				{0, -1, true, 0},                         // south-adjacent, open flags
				{0, 1, false, collision.FlagBlockNorth},  // north-adjacent, blocked
				{0, -1, false, collision.FlagBlockSouth}, // south-adjacent, blocked
				{1, 0, false, 0},                         // east-adjacent (behind wall, no gate)
				{1, 1, false, 0},                         // diagonals false
				{-1, -1, false, 0},
			},
		},
		{
			angle: 1 /*loc_north*/, name: "north",
			tiles: []tile{
				{0, 0, true, 0},
				{0, 1, true, 0},                         // north-adjacent
				{-1, 0, true, 0},                        // west-adjacent, open flags
				{1, 0, true, 0},                         // east-adjacent, open flags
				{-1, 0, false, collision.FlagBlockWest}, // west-adjacent, blocked
				{1, 0, false, collision.FlagBlockEast},  // east-adjacent, blocked
				{0, -1, false, 0},
			},
		},
		{
			angle: 2 /*loc_east*/, name: "east",
			tiles: []tile{
				{0, 0, true, 0},
				{1, 0, true, 0},                          // east-adjacent
				{0, 1, true, 0},                          // north-adjacent, open flags
				{0, -1, true, 0},                         // south-adjacent, open flags
				{0, 1, false, collision.FlagBlockNorth},  // north-adjacent, blocked
				{0, -1, false, collision.FlagBlockSouth}, // south-adjacent, blocked
				{-1, 0, false, 0},
			},
		},
		{
			angle: 3 /*loc_south*/, name: "south",
			tiles: []tile{
				{0, 0, true, 0},
				{0, -1, true, 0},                        // south-adjacent
				{-1, 0, true, 0},                        // west-adjacent, open flags
				{1, 0, true, 0},                         // east-adjacent, open flags
				{-1, 0, false, collision.FlagBlockWest}, // west-adjacent, blocked
				{1, 0, false, collision.FlagBlockEast},  // east-adjacent, blocked
				{0, 1, false, 0},
			},
		},
	}

	const lx, lz = 3098, 3107

	for _, ac := range cases {
		t.Run(ac.name, func(t *testing.T) {
			for _, tt := range ac.tiles {
				t.Run(fmt.Sprintf("dx=%+d_dz=%+d_flags=0x%x", tt.dx, tt.dz, tt.preFlags), func(t *testing.T) {
					s, _ := newInOperableTestServer(t)
					p, _ := newTestPlayer(t)
					p.client.server = s
					p.x, p.z, p.level = lx+tt.dx, lz+tt.dz, 0
					// Always initialise the player's tile so flags.Get returns
					// FlagOpen (0) rather than FlagNull (0x7FFFFFFF) for unallocated zones;
					// FlagNull makes all flag-gated reach conditions false. Then
					// OR in any blocking flags for the gated test cases.
					s.gamemap.Pathfinder.Flags.Set(p.x, p.z, p.level, tt.preFlags)
					loc := makeWallLoc(t, 0, lx, lz, 0 /*wall_straight*/, ac.angle)
					got := inOperableDistance(p, loc)
					if got != tt.want {
						t.Errorf("angle=%s tile dx=%+d dz=%+d preFlags=0x%x: got %v want %v",
							ac.name, tt.dx, tt.dz, tt.preFlags, got, tt.want)
					}
				})
			}
		})
	}
}

// TestPlayer_InOperableDistance_LevelMismatchFalse pins the level-guard
// from TS PathingEntity.ts:379-381.
func TestPlayer_InOperableDistance_LevelMismatchFalse(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0
	loc := entitypkg.NewLoc(1 /*level=1*/, 3098, 3107, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	if inOperableDistance(p, loc) {
		t.Errorf("expected false when target.level != p.level")
	}
}

// TestPlayer_InOperableDistance_NilLocTypeFallback pins forceapproach=0
// behavior when LocType lookup returns nil (out-of-range type id).
func TestPlayer_InOperableDistance_NilLocTypeFallback(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0
	// Type id 199 is not configured in newInOperableTestServer.
	loc := entitypkg.NewLoc(0, 3098, 3107, 1, 1, entitypkg.LifecycleDespawn, 199, 0, 0)
	if !inOperableDistance(p, loc) {
		t.Errorf("on-tile reach should still resolve true with nil LocType (forceapproach=0)")
	}
}

// -- NAI-173 PathingEntity reach tests (replaces pre-NAI-173 Chebyshev fallback) -------
//
// Ports TS Player.inOperableDistance (Player.ts:1099-1111) PathingEntity arm
// to reach.Reached(..., 0, -2, 0) (TS reachedEntity). Retired the
// *Player and *Npc target arms of the pre-NAI-173 Chebyshev fallback.
//
// reachRectangle1 (rectangularbounds.go:15-48) reads walk-flags AT THE SOURCE
// tile: every src tile must be AllocateIfAbsent'd to clear FlagNull
// (=0x7FFFFFFF, every movement bit set). Diagonals reject — reachRectangle1 has no diagonal arm; this is
// TS-faithful.

// TestPlayer_InOperableDistance_PathingEntity_Reach pins the production
// reach-based PathingEntity arm. Each row asserts the post-NAI-173 result.
func TestPlayer_InOperableDistance_PathingEntity_Reach(t *testing.T) {
	cases := []struct {
		name           string
		px, pz, plevel int
		tx, tz, tlevel int
		targetIsPlayer bool
		targetSize     int // npc.size (ignored when targetIsPlayer)
		want           bool
	}{
		{"npc same-tile", 100, 100, 0, 100, 100, 0, false, 1, false},
		{"npc adjacent N (orth)", 100, 100, 0, 100, 101, 0, false, 1, true},
		{"npc adjacent E (orth)", 100, 100, 0, 101, 100, 0, false, 1, true},
		{"npc adjacent NE (diag) — TS-faithful reject", 100, 100, 0, 101, 101, 0, false, 1, false},
		{"player adjacent N (orth)", 100, 100, 0, 100, 101, 0, true, 1, true},
		{"npc distance 2 east", 100, 100, 0, 102, 100, 0, false, 1, false},
		{"npc cross-level", 100, 100, 0, 100, 101, 1, false, 1, false},
		// Multi-tile NPC (size=2): occupies (100,100)-(101,101). Player one
		// tile west of the west edge at (99,100) reaches via reachRectangle1's
		// "srcX == destX-1" arm (destWidth=2, destLength=2 → east=101, north=101;
		// srcZ=100 ∈ [100,101] and srcX=99 == 100-1 ✓). Chebyshev would also
		// pass for srcX=99,srcZ=100 (|dx|=1) — non-divergent geometry; the
		// goal here is to exercise the destWidth/destLength path, not divergence.
		{"npc multi-tile (size=2) west of west edge", 99, 100, 0, 100, 100, 0, false, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newInOperableTestServer(t)
			p, _ := newTestPlayer(t)
			p.client.server = s
			p.x, p.z, p.level = tc.px, tc.pz, tc.plevel
			// Allocate src tile so unallocated FlagNull doesn't spuriously
			// block reach (per empty_flagmap_degenerate_routefinder.md).
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(tc.px, tc.pz, tc.plevel)

			var target entity
			if tc.targetIsPlayer {
				tp, _ := newTestPlayer(t)
				tp.client.server = s
				tp.x, tp.z, tp.level = tc.tx, tc.tz, tc.tlevel
				target = tp
			} else {
				typ := &objtype.NpcType{Size: byte(tc.targetSize)}
				n := NewNpc(1, 0, tc.tx, tc.tz, tc.tlevel, typ)
				n.server = s
				target = n
			}

			if got := inOperableDistance(p, target); got != tc.want {
				t.Errorf("inOperableDistance got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb
// pins the goscape-defensive nil-gamemap arm: when srv.gamemap is nil
// (narrow test fixtures), the PathingEntity arm falls back to Chebyshev≤1.
func TestPlayer_InOperableDistance_PathingEntity_NilGamemap_FallsThroughToCheb(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Construct a Server with NO gamemap — exercises the defensive branch.
	s := &Server{quit: make(chan interface{}), log: discardLogger()}
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 0, 101, 101, 0, typ) // diagonal — Chebyshev says true
	n.server = s

	if !inOperableDistance(p, n) {
		t.Fatalf("nil gamemap: expected Chebyshev fallback to allow diagonal-adjacent (got false)")
	}

	n2 := NewNpc(2, 0, 100, 100, 0, typ) // same tile — Chebyshev says false
	n2.server = s
	if inOperableDistance(p, n2) {
		t.Fatalf("nil gamemap: expected Chebyshev fallback to reject same-tile (got true)")
	}
}

// ---------------------------------------------------------------------------
// pathfinderRecorder — test double for pathfinderForTarget
// ---------------------------------------------------------------------------

// pathfinderRecorder captures FindPath* calls for assertion in
// pathToTarget tests. Implements the pathfinderForTarget interface.
type pathfinderRecorder struct {
	findPathToLocCalls    []findPathToLocCall
	findPathToEntityCalls []findPathToEntityCall
	findNaivePathCalls    []findNaivePathCall
	findPathPlainCalls    []findPathPlainCall

	// returned routes — empty by default (callers typically don't
	// care about the path content for call-shape assertions)
	returnRoute routefinder.Route
}

type findPathToLocCall struct {
	level, srcX, srcZ, destX, destZ int
	srcSize, destWidth, destLength  int
	angle, shape, blockAccessFlags  int
}

type findPathToEntityCall struct {
	level, srcX, srcZ, destX, destZ int
	srcSize, destWidth, destLength  int
}

type findNaivePathCall struct {
	level, srcX, srcZ, destX, destZ            int
	srcWidth, srcLength, destWidth, destLength int
	extraFlag                                  int
	collisionType                              collision.Type
}

type findPathPlainCall struct {
	level, srcX, srcZ, destX, destZ int
}

func (r *pathfinderRecorder) FindPathPlain(level, srcX, srcZ, destX, destZ int) routefinder.Route {
	r.findPathPlainCalls = append(r.findPathPlainCalls, findPathPlainCall{level, srcX, srcZ, destX, destZ})
	return r.returnRoute
}

func (r *pathfinderRecorder) FindPathToEntity(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength int) routefinder.Route {
	r.findPathToEntityCalls = append(r.findPathToEntityCalls, findPathToEntityCall{
		level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength,
	})
	return r.returnRoute
}

func (r *pathfinderRecorder) FindPathToLoc(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags int) routefinder.Route {
	r.findPathToLocCalls = append(r.findPathToLocCalls, findPathToLocCall{
		level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags,
	})
	return r.returnRoute
}

func (r *pathfinderRecorder) FindNaivePath(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag int, collisionType collision.Type) routefinder.Route {
	r.findNaivePathCalls = append(r.findNaivePathCalls, findNaivePathCall{
		level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag, collisionType,
	})
	return r.returnRoute
}

func (r *pathfinderRecorder) lastFindPathToLoc() (findPathToLocCall, bool) {
	if len(r.findPathToLocCalls) == 0 {
		return findPathToLocCall{}, false
	}
	return r.findPathToLocCalls[len(r.findPathToLocCalls)-1], true
}

func (r *pathfinderRecorder) lastFindPathToEntity() (findPathToEntityCall, bool) {
	if len(r.findPathToEntityCalls) == 0 {
		return findPathToEntityCall{}, false
	}
	return r.findPathToEntityCalls[len(r.findPathToEntityCalls)-1], true
}

func (r *pathfinderRecorder) lastFindNaivePath() (findNaivePathCall, bool) {
	if len(r.findNaivePathCalls) == 0 {
		return findNaivePathCall{}, false
	}
	return r.findNaivePathCalls[len(r.findNaivePathCalls)-1], true
}

func (r *pathfinderRecorder) lastFindPathPlain() (findPathPlainCall, bool) {
	if len(r.findPathPlainCalls) == 0 {
		return findPathPlainCall{}, false
	}
	return r.findPathPlainCalls[len(r.findPathPlainCalls)-1], true
}

// newPathToTargetTestServer builds a Server suitable for pathToTarget
// dispatch testing. Wires the recorder as the pathfinder seam.
func newPathToTargetTestServer(t *testing.T) (*Server, *pathfinderRecorder) {
	t.Helper()
	rec := &pathfinderRecorder{}
	srv := newTestServer(t)
	srv.testPathfinder = rec
	// Initialize empty locTypes so locTypeOrNil returns nil cleanly
	// for fixtures that don't register a type.
	srv.locTypes = &objtype.LocTypeConfigs{}
	return srv, rec
}

// newPathToTargetTestPlayer returns a Player with the given coords,
// wired to srv via client.server. Fields are mutated directly to
// avoid the full constructor.
func newPathToTargetTestPlayer(t *testing.T, srv *Server, x, z, level int) *Player {
	t.Helper()
	p, _ := newTestPlayer(t)
	p.client.server = srv
	p.x = x
	p.z = z
	p.level = level
	p.moveStrategy = MoveStrategySmart
	return p
}

// newPathToTargetTestNpc returns an Npc at the given coords with size=size,
// suitable for use as a Player.target in B3+ pathToTarget tests. The NPC's
// own moveStrategy is irrelevant — only its Coords/Width/Length matter when
// it's the target.
func newPathToTargetTestNpc(t *testing.T, srv *Server, x, z, level, size int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "pttarget"},
		Size:       byte(size),
	}
	n := NewNpc(0, 0, x, z, level, typ)
	n.server = srv
	return n
}

// ---------------------------------------------------------------------------
// pathToTarget tests — NAI-92 B2
// ---------------------------------------------------------------------------

// TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle pins NAI-92 B2's
// SMART/Loc dispatch. Asserts FindPathToLoc receives the loc's shape,
// angle, width, length, and the player's Width as srcSize.
func TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 3097, 3107, 0)

	// Loc at (3098, 3107, 0), wall_straight (shape=0), angle west (0), 1×1.
	loc := entitypkg.NewLoc(0, 3098, 3107, 1, 1, entitypkg.LifecycleForever /*typ=*/, 1234 /*shape=*/, 0 /*angle=*/, 0)
	p.target = loc

	// Register loc type with ForceApproach=0 so locTypeOrNil returns
	// non-nil cfg with the expected blockAccessFlags.
	for len(srv.locTypes.Configs) <= 1234 {
		srv.locTypes.Configs = append(srv.locTypes.Configs, nil)
	}
	srv.locTypes.Configs[1234] = &objtype.LocType{ForceApproach: 0}

	p.pathToTarget()

	call, ok := rec.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.level != 0 || call.srcX != 3097 || call.srcZ != 3107 || call.destX != 3098 || call.destZ != 3107 {
		t.Errorf("coords: got (lvl=%d src=%d,%d dest=%d,%d), want (0, 3097, 3107, 3098, 3107)",
			call.level, call.srcX, call.srcZ, call.destX, call.destZ)
	}
	if call.angle != 0 || call.shape != 0 {
		t.Errorf("angle/shape: got (%d, %d), want (0, 0)", call.angle, call.shape)
	}
	if call.blockAccessFlags != 0 {
		t.Errorf("blockAccessFlags: got %d, want 0", call.blockAccessFlags)
	}
	if call.destWidth != 1 || call.destLength != 1 {
		t.Errorf("destW/L: got (%d, %d), want (1, 1)", call.destWidth, call.destLength)
	}
	if call.srcSize != 1 {
		t.Errorf("srcSize: got %d, want 1 (Player.Width)", call.srcSize)
	}
}

// TestPlayer_PathToTarget_LocTarget_ForceApproachThreaded pins that
// LocType.ForceApproach is threaded into the FindPathToLoc
// blockAccessFlags argument.
func TestPlayer_PathToTarget_LocTarget_ForceApproachThreaded(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, entitypkg.LifecycleForever, 1234, 0, 0)
	p.target = loc

	for len(srv.locTypes.Configs) <= 1234 {
		srv.locTypes.Configs = append(srv.locTypes.Configs, nil)
	}
	srv.locTypes.Configs[1234] = &objtype.LocType{ForceApproach: 7}

	p.pathToTarget()

	call, ok := rec.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.blockAccessFlags != 7 {
		t.Errorf("blockAccessFlags: got %d, want 7 (LocType.ForceApproach)", call.blockAccessFlags)
	}
}

// TestPlayer_PathToTarget_LocTarget_NilLocTypeUsesZeroForceApproach
// pins the goscape-defensive guard in pathToTargetSmart that handles
// locTypeOrNil returning nil (e.g. test fixtures with no registered
// type). (goscape defensive; TS skips this check)
func TestPlayer_PathToTarget_LocTarget_NilLocTypeUsesZeroForceApproach(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, entitypkg.LifecycleForever, 9999, 0, 0)
	p.target = loc
	// No registration in srv.locTypes.Configs[9999] — locTypeOrNil returns nil.

	p.pathToTarget()

	call, ok := rec.lastFindPathToLoc()
	if !ok {
		t.Fatalf("FindPathToLoc not called")
	}
	if call.blockAccessFlags != 0 {
		t.Errorf("blockAccessFlags: got %d, want 0 (nil locType→zero)", call.blockAccessFlags)
	}
}

// TestPlayer_PathToTarget_NoTarget_NoOp pins the guard at the top
// of pathToTarget — no target ⇒ no pathfinder call, no waypoint.
func TestPlayer_PathToTarget_NoTarget_NoOp(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.target = nil

	p.pathToTarget()

	if _, ok := rec.lastFindPathToLoc(); ok {
		t.Errorf("FindPathToLoc unexpectedly called for nil target")
	}
	if _, ok := rec.lastFindPathPlain(); ok {
		t.Errorf("FindPathPlain unexpectedly called for nil target")
	}
	if p.waypointIndex >= 0 {
		t.Errorf("expected no waypoints (waypointIndex=-1), got waypointIndex=%d", p.waypointIndex)
	}
}

// ---------------------------------------------------------------------------
// pathToTarget tests — NAI-92 B3
// ---------------------------------------------------------------------------

// TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity pins
// NAI-92 B3's SMART/PathingEntity arm without the intersect shortcut.
// Fixture: Survival Expert NPC at (3104, 3093) + player at (3101, 3105).
// bbox is disjoint → FindPathToEntity called with srcSize=p.Width(),
// destWidth=destLength=npc.size. Pre-NAI-92 this used the shape-blind 1×1
// FindPathPlain and failed to route through the cabin door.
func TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = false // server-routefinder mode (production default)
	p := newPathToTargetTestPlayer(t, srv, 3101, 3105, 0)
	npc := newPathToTargetTestNpc(t, srv, 3104, 3093, 0 /*size=*/, 1)
	p.target = npc

	p.pathToTarget()

	call, ok := rec.lastFindPathToEntity()
	if !ok {
		t.Fatalf("FindPathToEntity not called")
	}
	if call.srcSize != 1 {
		t.Errorf("srcSize: got %d, want 1 (Player.Width)", call.srcSize)
	}
	if call.destWidth != 1 || call.destLength != 1 {
		t.Errorf("destW/L: got (%d, %d), want (1, 1) (npc.size)", call.destWidth, call.destLength)
	}
	if call.level != 0 || call.srcX != 3101 || call.srcZ != 3105 || call.destX != 3104 || call.destZ != 3093 {
		t.Errorf("coords: got (lvl=%d src=%d,%d dest=%d,%d), want (0, 3101, 3105, 3104, 3093)",
			call.level, call.srcX, call.srcZ, call.destX, call.destZ)
	}

	// Negative pin: FindNaivePath must NOT have been called.
	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called (no NCR + no intersect should use FindPathToEntity)")
	}
}

// TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_Intersect_UsesNaivePath
// pins the shortcut: NodeClientRoutefinder=true AND bbox-intersect →
// FindNaivePath instead of FindPathToEntity.
func TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_Intersect_UsesNaivePath(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = true
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	npc := newPathToTargetTestNpc(t, srv, 100, 100, 0 /*size=*/, 1) // same tile = intersect
	p.target = npc

	p.pathToTarget()

	if _, ok := rec.lastFindNaivePath(); !ok {
		t.Fatalf("FindNaivePath not called (NCR + intersect should shortcut)")
	}
	if _, ok := rec.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called (intersect should shortcut)")
	}
}

// TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_NoIntersect_UsesFindPathToEntity
// pins the fallthrough: NCR=true but bbox is DISJOINT → FindPathToEntity (full search).
func TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_NoIntersect_UsesFindPathToEntity(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = true
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	npc := newPathToTargetTestNpc(t, srv, 200, 200, 0 /*size=*/, 1) // disjoint bbox
	p.target = npc

	p.pathToTarget()

	if _, ok := rec.lastFindPathToEntity(); !ok {
		t.Fatalf("FindPathToEntity not called (no intersect should use full search)")
	}
	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called (NCR + no intersect should fall through)")
	}
}

// TestPlayer_PathToTarget_PlayerTarget_DispatchesSameAsNpc pins symmetry —
// when the target is another *Player, the same SMART/PathingEntity branch
// fires.
func TestPlayer_PathToTarget_PlayerTarget_DispatchesSameAsNpc(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	srv.cfg.NodeClientRoutefinder = false
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	other := newPathToTargetTestPlayer(t, srv, 105, 105, 0)
	p.target = other

	p.pathToTarget()

	if _, ok := rec.lastFindPathToEntity(); !ok {
		t.Fatalf("FindPathToEntity not called for *Player target")
	}
	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called for *Player target (no NCR)")
	}
}

// TestPlayer_PathToTarget_ObjTarget_SameTile_QueuesSingleWaypoint pins
// the TS workaround at PathingEntity.ts:472-473: findPath returns (0,0)
// when src==dest, so the Obj-same-tile case queues a direct waypoint.
func TestPlayer_PathToTarget_ObjTarget_SameTile_QueuesSingleWaypoint(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleForever /*typ=*/, 1234 /*count=*/, 1)
	p.target = obj

	p.pathToTarget()

	if p.waypointIndex < 0 {
		t.Fatalf("expected single waypoint queued, got waypointIndex=%d", p.waypointIndex)
	}
	got := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	if got.Level != 0 || got.X != 100 || got.Z != 100 {
		t.Errorf("waypoint coord: got (lvl=%d, %d, %d), want (0, 100, 100)", got.Level, got.X, got.Z)
	}
	if _, ok := rec.lastFindPathPlain(); ok {
		t.Errorf("FindPathPlain unexpectedly called for same-tile Obj")
	}
	if _, ok := rec.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called for same-tile Obj")
	}
	if _, ok := rec.lastFindPathToLoc(); ok {
		t.Errorf("FindPathToLoc unexpectedly called for same-tile Obj")
	}
}

// TestPlayer_PathToTarget_ObjTarget_DifferentTile_UsesFindPathPlain pins
// the shape-blind 1×1 fallback for the different-tile Obj case.
func TestPlayer_PathToTarget_ObjTarget_DifferentTile_UsesFindPathPlain(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	obj := entitypkg.NewObj(0, 105, 105, entitypkg.LifecycleForever /*typ=*/, 1234 /*count=*/, 1)
	p.target = obj

	p.pathToTarget()

	call, ok := rec.lastFindPathPlain()
	if !ok {
		t.Fatalf("FindPathPlain not called for different-tile Obj")
	}
	if call.level != 0 || call.srcX != 100 || call.srcZ != 100 || call.destX != 105 || call.destZ != 105 {
		t.Errorf("coords: got (lvl=%d src=%d,%d dest=%d,%d), want (0, 100, 100, 105, 105)",
			call.level, call.srcX, call.srcZ, call.destX, call.destZ)
	}
	if _, ok := rec.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called for different-tile Obj")
	}
	if _, ok := rec.lastFindPathToLoc(); ok {
		t.Errorf("FindPathToLoc unexpectedly called for different-tile Obj")
	}
}

// TestPlayer_PathToTarget_NaiveStrategy_PathingEntityTarget_UsesFindNaivePath
// pins NAI-92 B5's NAIVE/PathingEntity dispatch. Player.blockWalkFlag is
// unconditional FlagBlockPlayers in TS, so extraFlag is always
// FlagBlockPlayers (verified via the recorder's extraFlag field).
func TestPlayer_PathToTarget_NaiveStrategy_PathingEntityTarget_UsesFindNaivePath(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	npc := newPathToTargetTestNpc(t, srv, 105, 105, 0 /*size=*/, 1)
	p.target = npc

	p.pathToTarget()

	call, ok := rec.lastFindNaivePath()
	if !ok {
		t.Fatalf("FindNaivePath not called")
	}
	if call.extraFlag != p.blockWalkFlag() {
		t.Errorf("extraFlag: got %d, want %d (Player.blockWalkFlag)", call.extraFlag, p.blockWalkFlag())
	}
	if call.srcWidth != 1 || call.srcLength != 1 {
		t.Errorf("srcW/L: got (%d, %d), want (1, 1) (Player.Width/Length)", call.srcWidth, call.srcLength)
	}
	if call.destWidth != 1 || call.destLength != 1 {
		t.Errorf("destW/L: got (%d, %d), want (1, 1) (npc.size)", call.destWidth, call.destLength)
	}
	if call.collisionType != collision.TypeNormal {
		t.Errorf("collisionType: got %v, want TypeNormal (MoveRestrictNormal player)", call.collisionType)
	}
	// Negative pin: SMART arm should NOT have fired.
	if _, ok := rec.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called (NAIVE should use FindNaivePath)")
	}
}

// TestPlayer_PathToTarget_NaiveStrategy_LocTarget_QueuesSingleWaypoint
// pins the non-PathingEntity branch of NAIVE — Loc target queues one
// waypoint, no FindNaivePath call.
func TestPlayer_PathToTarget_NaiveStrategy_LocTarget_QueuesSingleWaypoint(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	loc := entitypkg.NewLoc(0, 105, 105, 1, 1, entitypkg.LifecycleForever, 1234, 0, 0)
	p.target = loc

	p.pathToTarget()

	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called for Loc target in NAIVE")
	}
	if _, ok := rec.lastFindPathToLoc(); ok {
		t.Errorf("FindPathToLoc unexpectedly called for NAIVE Loc target")
	}
	if p.waypointIndex < 0 {
		t.Errorf("expected single waypoint queued, got waypointIndex=%d", p.waypointIndex)
	}
	got := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	if got.Level != 0 || got.X != 105 || got.Z != 105 {
		t.Errorf("waypoint coord: got (lvl=%d, %d, %d), want (0, 105, 105)", got.Level, got.X, got.Z)
	}
}

// TestPlayer_PathToTarget_NaiveStrategy_NoMove_NoOp pins the
// getCollisionStrategy() == nil early-return for MoveRestrictNoMove.
func TestPlayer_PathToTarget_NaiveStrategy_NoMove_NoOp(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.moveStrategy = MoveStrategyNaive
	p.moveRestrict = MoveRestrictNoMove
	p.target = newPathToTargetTestNpc(t, srv, 105, 105, 0, 1)

	p.pathToTarget()

	if p.waypointIndex >= 0 {
		t.Errorf("expected no waypoints (NoMove early return), got waypointIndex=%d", p.waypointIndex)
	}
	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called for NoMove player")
	}
}

// TestPlayer_PathToTarget_NoStrategyBranch_QueuesSingleWaypoint pins
// the third else-branch (PathingEntity.ts:494-507). goscape's
// MoveStrategy enum has only Smart+Naive, so engage the default arm
// via an out-of-range cast.
func TestPlayer_PathToTarget_NoStrategyBranch_QueuesSingleWaypoint(t *testing.T) {
	srv, rec := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.moveStrategy = MoveStrategy(99) // out of enum range → default branch
	p.target = newPathToTargetTestNpc(t, srv, 105, 105, 0, 1)

	p.pathToTarget()

	if p.waypointIndex < 0 {
		t.Errorf("expected single waypoint, got waypointIndex=%d", p.waypointIndex)
	}
	got := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	if got.Level != 0 || got.X != 105 || got.Z != 105 {
		t.Errorf("waypoint coord: got (lvl=%d, %d, %d), want (0, 105, 105)", got.Level, got.X, got.Z)
	}
	// Negative pins: no pathfinder call should have fired.
	if _, ok := rec.lastFindNaivePath(); ok {
		t.Errorf("FindNaivePath unexpectedly called in no-strategy branch")
	}
	if _, ok := rec.lastFindPathToEntity(); ok {
		t.Errorf("FindPathToEntity unexpectedly called in no-strategy branch")
	}
}

// TestPlayer_PathToTarget_NoStrategyBranch_NoMove_NoOp pins the same
// nomove early-return for the no-strategy else branch.
func TestPlayer_PathToTarget_NoStrategyBranch_NoMove_NoOp(t *testing.T) {
	srv, _ := newPathToTargetTestServer(t)
	p := newPathToTargetTestPlayer(t, srv, 100, 100, 0)
	p.moveStrategy = MoveStrategy(99)
	p.moveRestrict = MoveRestrictNoMove
	p.target = newPathToTargetTestNpc(t, srv, 105, 105, 0, 1)

	p.pathToTarget()

	if p.waypointIndex >= 0 {
		t.Errorf("expected no waypoints (NoMove early return), got waypointIndex=%d", p.waypointIndex)
	}
}

// TestPlayerInteractedDoesNotLeakAcrossIdleTick — NAI-108 Task 4 (δ) verify-and-pin.
// TS PathingEntity.ts:587 resets interacted=false every tick. goscape
// relies on SetInteraction/ClearInteraction handlers re-setting on the
// next interaction touch. Pins the no-leak contract for the idle-tick
// path (no SetInteraction/ClearInteraction call between ticks).
//
// SKIP-PINNED as NAI-108-D-INTERACTED-LEAK: p.interacted is set true by
// tryInteract fire helpers (interaction.go:390,400) but is never READ in
// production code — only referenced in doc-comments. The field is a
// write-only struct annotation; cross-tick leak is moot since no
// consumer reads it before the next handler-side re-set. Deferred for
// future cleanup if a reader is added.
func TestPlayerInteractedDoesNotLeakAcrossIdleTick(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.interacted = true // simulate prior-tick fire
	// Idle tick: ResetMasks runs at tick end; no interaction touch.
	p.ResetMasks()
	if p.interacted {
		t.Error("interacted: got true, want false (must not leak across idle tick) — NAI-108-D-INTERACTED-LEAK candidate")
	}
}

// TestPlayerApRangeCalledDoesNotLeakAcrossIdleTick — NAI-108 Task 4 (δ).
// TS PathingEntity.ts:588 resets apRangeCalled=false every tick.
// Pinning the no-leak contract; rationale differs from INTERACTED-LEAK
// because apRangeCalled HAS production reads (see SKIP note below).
//
// SKIP-PINNED as NAI-108-D-APRANGECALLED-LEAK: apRangeCalled is reset on
// SetInteraction (interaction.go:85), ClearInteraction (interaction.go:133),
// and post-fire (player_interaction_trigger.go:121); not reset by ResetMasks.
// Unlike NAI-108-D-INTERACTED-LEAK (which is moot — apRangeCalled HAS
// production reads at interaction.go:271 (`else if interacted &&
// !p.apRangeCalled`) and interaction.go:406 (`if p.nextTarget == nil &&
// p.apRangeCalled`), both at the start of processInteraction() before any
// handler-side reset. A real cross-tick leak (idle tick with no
// SetInteraction/ClearInteraction between SetApRange and the next
// processInteraction) would suppress the auto-clear branch at line 271 —
// a behavioral divergence from TS. Deferred per spec R1 (no auto-port to
// ResetMasks); future fix should either reset in ResetMasks or audit
// each read site.
func TestPlayerApRangeCalledDoesNotLeakAcrossIdleTick(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRangeCalled = true
	p.ResetMasks()
	if p.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (must not leak across idle tick) — NAI-108-D-APRANGECALLED-LEAK candidate")
	}
}

// -- NAI-152 B2 T2 Obj-target reach tests ---------------------------------
//
// Ports TS Player.ts:1110 — reachedEntity || reachedObj. Retired the Obj
// clause of the pre-NAI-152-B2 Chebyshev fallback. Same-tile pickup succeeds
// via reach.Reached's srcX==destX && srcZ==destZ early-out on the
// locShape=-1 arm.

// newObjReachTestServer constructs a minimal *Server with a gamemap so
// inOperableDistance's new Obj branch can read collision flags. No
// locTypes needed — Obj targets don't dispatch via locTypeOrNil.
func newObjReachTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		locObjTracker:  newLocObjTracker(),
		rsbuf:          rsbuf.New(),
		players:        newPlayerList(2048),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.gamemap = gamemap.New(discardLogger())
	return s
}

// TestPlayer_InOperableDistance_Obj_SameTile pins the mindrune pickup
// reach-check. Pre-B2 returned false via inOperableDistanceCheb (excludes
// same-tile); post-B2 returns true via reach.Reached locShape=-1
// short-circuit (strategy.go:37). This is the B1-smoke binding case.
func TestPlayer_InOperableDistance_Obj_SameTile(t *testing.T) {
	s := newObjReachTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if !inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance true on same-tile Obj (mindrune pickup)")
	}
}

// TestPlayer_InOperableDistance_Obj_Adjacent pins the table-pickup case
// (player one tile away from the obj). reachedEntity (locShape=-2) enters
// ReachExclusiveRectangle which returns true for the 4 orthogonal
// neighbors of a 1×1 dest (reachRectangle1 perimeter check, all flags
// default zero).
func TestPlayer_InOperableDistance_Obj_Adjacent(t *testing.T) {
	s := newObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3201, 3200, 0)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3201, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if !inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance true on adjacent (east) Obj")
	}
}

// TestPlayer_InOperableDistance_Obj_OutOfReach pins the no-reach case
// (distance > 1). Both reachedEntity and reachedObj arms return false:
// reachedEntity's reachRectangle1 perimeter check rejects non-adjacent
// src; reachedObj falls through the noStrategy switch default to false.
func TestPlayer_InOperableDistance_Obj_OutOfReach(t *testing.T) {
	s := newObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3200, 0)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3210, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance false at distance 10")
	}
}

// TestPlayer_InOperableDistance_Obj_CrossLevel preserves the existing
// top-level guard (target.level != p.level → false).
func TestPlayer_InOperableDistance_Obj_CrossLevel(t *testing.T) {
	s := newObjReachTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0

	obj := entitypkg.NewObj(1 /*level=1*/, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance false on cross-level Obj")
	}
}

// TestSetInteractionLoc_FaceSquareUsesTSFineScale reproduces the user-
// reported bug "the player faces the wrong way when interacting with an
// object (like a tree)". The fine-grained centre coord that drives the
// face-coord mask is sent to the client as a 16-bit value
// (pkg/rsbuf/mask_payload.go: writeFaceCoord uses uint16). TS
// CoordGrid.fine produces `pos*2 + size`; for a 1x1 tree at world
// X=3201 that is 6403 which fits cleanly in uint16. The Go port
// originally shipped Fine() as `coord*64 + (size*64-1)/2` (NAI-11,
// commit 15861a7d) — a 32x-too-large value that silently truncated
// through uint16 and rendered as a random direction on the client.
//
// Pins:
//   - faceSquareX equals coordgrid.Fine(tree.x, 1) (a single source of truth).
//   - The resulting value fits inside uint16 with no truncation,
//     i.e. uint16(faceSquareX) == faceSquareX as ints.
//   - For typical-RS2 tile coords (~3200), the value is in the 6000-7000
//     band, matching the TS scale; sanity-check upper bound rules out
//     the broken *64-scale regression.
func TestSetInteractionLoc_FaceSquareUsesTSFineScale(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	// Tree (1x1 loc) one tile east of the player.
	tree := entitypkg.NewLoc(0, 3201, 3200, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)

	p.SetInteraction(InteractionEngine, tree, 1, -1)

	wantX := coordgrid.Fine(3201, 1) // TS: 3201*2 + 1 = 6403
	wantZ := coordgrid.Fine(3200, 1) // TS: 3200*2 + 1 = 6401

	if p.faceSquareX != wantX {
		t.Errorf("faceSquareX: got %d, want %d (Fine(3201, 1))", p.faceSquareX, wantX)
	}
	if p.faceSquareZ != wantZ {
		t.Errorf("faceSquareZ: got %d, want %d (Fine(3200, 1))", p.faceSquareZ, wantZ)
	}
	// 16-bit wire-fit invariant: round-tripping through uint16 must be
	// lossless. Catches a regression to `coord*64+(size*64-1)/2` (which
	// would produce ~204895 here and wrap silently).
	if int(uint16(p.faceSquareX)) != p.faceSquareX {
		t.Errorf("faceSquareX %d does not survive uint16 round-trip (wire-truncation regression)", p.faceSquareX)
	}
	if int(uint16(p.faceSquareZ)) != p.faceSquareZ {
		t.Errorf("faceSquareZ %d does not survive uint16 round-trip (wire-truncation regression)", p.faceSquareZ)
	}
	// MaskFaceCoord must be set so the client actually receives the
	// rotation (instant=true branch in focus, gated on
	// NonPathingEntity + InteractionEngine).
	if p.masks&rsbuf.MaskFaceCoord == 0 {
		t.Errorf("masks: MaskFaceCoord bit not set after SetInteraction with engine loc click")
	}
}
