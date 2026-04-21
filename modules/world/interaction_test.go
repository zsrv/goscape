package world

import (
	"bytes"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/grid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
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

// TestProcessInteractionInRangeFacesTarget verifies adjacent target triggers face + interacted.
func TestProcessInteractionInRangeFacesTarget(t *testing.T) {
	s := newTestServer(t)
	s.grid = grid.New()
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

	if !p.interacted {
		t.Error("interacted should be true when adjacent to target")
	}
	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set")
	}
}

// TestProcessInteractionOutOfRangePaths verifies a distant target causes pathing.
// The NPC is placed 15 tiles away — beyond the default apRange of 10 — so the
// interaction falls through to the pathing branch (not the AP branch).
func TestProcessInteractionOutOfRangePaths(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true // use direct-step mode
	s.grid = grid.New()
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
	s.grid = grid.New()
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
	s.grid = grid.New()
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
	s.grid = grid.New()
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

	if !p.interactionFired {
		t.Error("interactionFired after AP-branch: got false, want true")
	}
	if !p.interacted {
		t.Error("interacted after AP-branch: got false, want true")
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
