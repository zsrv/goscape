package world

import (
	"bytes"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
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
	p.interacted = true
	p.repathed = true
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
	if p.interacted {
		t.Error("interacted should be false")
	}
	if p.repathed {
		t.Error("repathed should be false")
	}
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
	if !p.repathed {
		t.Error("repathed should be true after first out-of-range tick")
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

	// Expect UnsetMapFlag (opcode 19, 0 payload = just the encrypted opcode byte).
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
	p.interactionFired = true
	npc := &Npc{nid: 0, typeId: 7}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.interactionFired {
		t.Error("SetInteraction: interactionFired should be reset to false")
	}
}

func TestClearInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.interactionFired = true
	p.ClearInteraction()
	if p.interactionFired {
		t.Error("ClearInteraction: interactionFired should be reset to false")
	}
}

// TestInOperableDistanceTable checks adjacency logic for various offsets.
func TestInOperableDistanceTable(t *testing.T) {
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
		got := inOperableDistance(0, 0, tc.dx, tc.dz)
		if got != tc.want {
			t.Errorf("inOperableDistance(0,0,%d,%d) = %v, want %v", tc.dx, tc.dz, got, tc.want)
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
// false (can't "approach" your own tile). Mirrors inOperableDistance
// (which also excludes same-tile).
func TestInApproachDistanceSameTile(t *testing.T) {
	if inApproachDistance(100, 100, 100, 100, 10) {
		t.Error("same tile: got true, want false")
	}
}

// TestInApproachDistanceAtRange verifies Chebyshev distance exactly
// apRange is accepted.
func TestInApproachDistanceAtRange(t *testing.T) {
	if !inApproachDistance(100, 100, 110, 100, 10) {
		t.Error("dx=10 apRange=10: got false, want true")
	}
	if !inApproachDistance(100, 100, 107, 107, 10) {
		t.Error("dx=dz=7 apRange=10: got false, want true")
	}
}

// TestInApproachDistanceBeyondRange verifies one tile past apRange
// is rejected.
func TestInApproachDistanceBeyondRange(t *testing.T) {
	if inApproachDistance(100, 100, 111, 100, 10) {
		t.Error("dx=11 apRange=10: got true, want false")
	}
	if inApproachDistance(100, 100, 105, 111, 10) {
		t.Error("dz=11 apRange=10: got true, want false")
	}
}

// TestInApproachDistanceZeroRange verifies apRange <= 0 is always
// rejected (even for adjacent tiles).
func TestInApproachDistanceZeroRange(t *testing.T) {
	if inApproachDistance(100, 100, 101, 100, 0) {
		t.Error("apRange=0: got true, want false")
	}
	if inApproachDistance(100, 100, 101, 100, -5) {
		t.Error("apRange=-5: got true, want false")
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

// TestProcessInteractionRoutesToApBranch verifies processInteraction
// fires the AP-branch (tryFireApTrigger → interactionFired=true) when
// the player is within apRange but not at contact. The full
// tryFireApTrigger impl requires zoneMap, locTypes, and targetSubject.
// No APLOC script is registered so fireApTriggerLoc falls through to
// the no-script path and sets interactionFired=true — the routing
// assertion still holds.
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
	p.interactionFired = false
	p.apRange = 10

	p.processInteraction()

	// NAI-44 T6 cascade: pre-T5 asserted interactionFired+interacted==true.
	// Post-T5 auto-clear (TS L1261-1263) fires when interacted && !apRangeCalled
	// (the no-script AP path does not set apRangeCalled), clearing both flags.
	// Observable proof of AP-branch routing: target==nil (auto-clear ran).
	if p.target != nil {
		t.Errorf("target: got %v, want nil (auto-clear at TS L1261-1263 after AP-branch fire)", p.target)
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

// fakeEntity is a minimal entity implementation for tests that need a
// non-nil, non-specific target.
type fakeEntity struct{ x, z, level int }

func (f fakeEntity) Slot() int                 { return -1 }
func (f fakeEntity) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeEntity) IsValid() bool             { return true }

// TestEffectiveApRangeNpcUsesTypeAttackrange verifies that when the
// player's target is an *Npc, effectiveApRange returns the NPC's
// per-type AttackRange — NOT the Player's mutable apRange field.
func TestEffectiveApRangeNpcUsesTypeAttackrange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 10 // Player-side mutable default — should be IGNORED for NPC

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
	}
	npc := NewNpc(0, 7, 100, 100, 0, npcType)
	p.target = npc

	if got := effectiveApRange(p); got != 5 {
		t.Errorf("effectiveApRange: got %d, want 5 (npc.typ.AttackRange)", got)
	}
}

// TestEffectiveApRangeLocUsesPlayerApRange verifies that for non-NPC
// targets (e.g. *Loc), effectiveApRange falls back to p.apRange.
func TestEffectiveApRangeLocUsesPlayerApRange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 7 // custom, simulating a p_aprange call

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	p.target = loc

	if got := effectiveApRange(p); got != 7 {
		t.Errorf("effectiveApRange: got %d, want 7 (p.apRange for Loc target)", got)
	}
}

// TestEffectiveApRangeNilNpcTypeReturnsZero verifies the defensive
// guard: an NPC with a nil typ pointer returns 0.
func TestEffectiveApRangeNilNpcTypeReturnsZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 10

	// Construct directly: NewNpc dereferences typ, so pass nil via struct literal.
	npc := &Npc{nid: 0, typeId: 7, typ: nil}
	p.target = npc

	if got := effectiveApRange(p); got != 0 {
		t.Errorf("effectiveApRange: got %d, want 0 (nil typ defensive)", got)
	}
}

// TestProcessInteractionNpcUsesAttackrange is an integration test:
// NPC with AttackRange=5 at dx=6 from the player, with p.apRange=10.
// Without the swap, processInteraction sees dx=6 <= p.apRange=10 and
// takes AP branch. With the swap, dx=6 > AttackRange=5 so pathing fires.
func TestProcessInteractionNpcUsesAttackrange(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.apRange = 10

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
	}
	npc := NewNpc(0, 7, 106, 100, 0, npcType) // dx=6
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	p.processInteraction()

	if p.interacted {
		t.Error("p.interacted: got true, want false — AP branch should NOT fire (dx=6 > AttackRange=5)")
	}
	if !p.repathed {
		t.Error("p.repathed: got false, want true — pathing branch should fire when out of AP range")
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
		t.Errorf("faceEntity: got %d, want %d (slot+32768)", p.faceEntity, wantFE)
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

// TestSetInteractionLocTargetDoesNotSetFaceEntity pins the deferred
// default branch: *Loc target leaves faceEntity untouched and
// MaskFaceEntity bit clear. Closes the spec's "deviation is intentional,
// not a partial port" contract for NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ.
func TestSetInteractionLocTargetDoesNotSetFaceEntity(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 100, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (default; *Loc branch must not write)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity != 0 {
		t.Error("MaskFaceEntity bit must NOT be set after SetInteraction with *Loc target")
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
// !this.protect via goscape's activeScript.Protect convergence.
func TestProcessWalktrigger_ProtectedScriptActiveNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 7
	p.activeScript = &script.ScriptState{Protect: true}

	p.processWalktrigger()

	if p.walktrigger != 7 {
		t.Errorf("walktrigger after protected-bail: got %d, want 7 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_ActiveScriptUnprotectedFires — NAI-52. Pins
// that activeScript != nil alone does NOT block the consumer; only
// activeScript.Protect == true does. activeScript with Protect=false
// must allow the walktrigger to fire and clear.
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
	p.activeScript = &script.ScriptState{Protect: false}

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
func TestFollowOpWaypointExhaustion(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3210, 3200, 0)

	clicker.SetInteraction(InteractionEngine, target, 3, -1)
	// Force waypoint exhaustion: set repathed=true to skip pathToTarget
	// and leave waypointIndex at -1 (no waypoints). This exercises the
	// TS L1237-1239 followOp + no-waypoints → ClearInteraction branch.
	clicker.waypointIndex = -1
	clicker.repathed = true

	clicker.processInteraction()

	if clicker.target != nil {
		t.Errorf("target: got %v, want nil (followOp + no waypoints must ClearInteraction)", clicker.target)
	}
}

// TestFollowOpContactFire — NAI-44 T5 / B4. OPPLAYER3 with target in
// operable distance: pre-step tryInteract fires the OP trigger. The
// auto-clear gate at TS L1261-1263 evaluates `interacted && !apRangeCalled`
// → ClearInteraction, wiping both target and interactionFired.
// followOp does NOT gate the auto-clear; it only gates post-step-interact.
//
// Note: interactionFired is NOT checked post-processInteraction because
// the auto-clear calls ClearInteraction() which resets interactionFired=false.
// The key invariant pinned here is target=nil (auto-clear fired).
func TestFollowOpContactFire(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3201, 3200, 0) // adjacent — operable distance

	clicker.SetInteraction(InteractionEngine, target, 3, -1)

	clicker.processInteraction()

	// Auto-clear gate fires (interacted && !apRangeCalled) → ClearInteraction.
	if clicker.target != nil {
		t.Errorf("target: got %v, want nil (auto-clear at TS L1261-1263)", clicker.target)
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
// fire the OP branch when allowOpScenery=false. AP branch fires instead
// if in approach range.
func TestTryInteractLocBlocksOpWhenSceneryFalse(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.apRange = 10 // wide AP range so AP branch fires

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// allowOpScenery=false + adjacent Loc → OP gated; AP fires instead (returns true).
	result := p.tryInteract(false)

	// AP branch fires (returns true) because the OP gate falls through to AP.
	if !result {
		t.Error("tryInteract(false) on adjacent Loc: got false, want true (AP fires)")
	}
}

// TestTryInteractLocAllowsOpWhenSceneryTrue pins that *Loc targets CAN fire
// the OP branch when allowOpScenery=true.
func TestTryInteractLocAllowsOpWhenSceneryTrue(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// allowOpScenery=true + adjacent Loc → OP fires.
	result := p.tryInteract(true)

	if !result {
		t.Error("tryInteract(true) on adjacent Loc: got false, want true (OP allowed)")
	}
}

// TestTryInteractProcessInteractionCallSites pins the two call-site semantics
// via processInteraction: pre-step always passes false, post-step passes
// stepsTaken==0 (true only when no movement this tick).
func TestTryInteractProcessInteractionCallSites(t *testing.T) {
	s := newTestServer(t)

	// Scenario: Loc target, player already adjacent (no movement needed),
	// so post-step call gets allowOpScenery=true (stepsTaken==0).
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.stepsTaken = 0 // no movement this tick

	loc := entitypkg.NewLoc(0, 101, 100, 1, 1, entitypkg.LifecycleForever, 42, 1, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	p.processInteraction()

	// OP or AP fired (interacted=true), and interaction was auto-cleared.
	if p.target != nil {
		t.Error("target should be nil after interaction auto-clear")
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
