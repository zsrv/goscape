package world

import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/grid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/zone"
)

// makeOpLocFixture creates a server + player + loc adjacent to the player,
// with a LocType registered, ready for handleOpLoc tests.
// Player at (99, 100, 0); loc at (100, 100, 0) — Chebyshev=1 (adjacent).
// Player originX/originZ = (100, 100) so viewport gate accepts coords
// within [-52, +52] of (100, 100).
// Returns (server, player, loc, clientConn) — pass clientConn to drainConn
// when the test needs to observe bytes written to the player.
func makeOpLocFixture(t *testing.T) (*Server, *Player, *entitypkg.Loc, net.Conn) {
	t.Helper()
	s := newTestServer(t)
	s.grid = grid.New()
	s.zoneMap = zone.NewZoneMap()

	// Register LocType 42.
	s.locTypes = &objtype.LocTypeConfigs{
		Configs: make([]*objtype.LocType, 43),
	}
	s.locTypes.Configs[42] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_loc"},
		Category:   7,
		// All 5 slots populated so pre-S6k tests (delayed, viewport,
		// boundary, etc.) don't regress under the S6k op-validation gate.
		// Tests that want to exercise the gate override individual slots.
		Op: []string{"op1", "op2", "op3", "op4", "op5"},
	}

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, loc)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	return s, p, loc, cc
}

// p2x3Payload encodes (x: u16, z: u16, locId: u16) into 6 bytes big-endian.
func p2x3Payload(x, z, locId int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
	}
}

// TestHandleOpLoc1SetsInteraction verifies a valid request sets interaction state.
func TestHandleOpLoc1SetsInteraction(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if p.targetSubject.typ != 42 {
		t.Errorf("targetSubject.typ: got %d, want 42", p.targetSubject.typ)
	}
	if p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject coords: got (%d, %d, %d), want (100, 100, 0)",
			p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}

// TestHandleOpLocDelayedPlayerRejected verifies delayed player gets UnsetMapFlag, no state change.
func TestHandleOpLocDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

// TestHandleOpLocShortPayloadRejected verifies < 6 byte payload emits UnsetMapFlag.
func TestHandleOpLocShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, []byte{0x01, 0x02, 0x03}) // only 3 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocOutOfViewportRejected verifies coords > 52 tiles from origin emits UnsetMapFlag.
func TestHandleOpLocOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t) // origin = (100, 100)

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(250, 100, 42)) // dx = 150 > 52
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport click, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport click")
	}
}

// TestHandleOpLocCoordValidationBoundary verifies exactly 52-tile distance accepted, 53 rejected.
func TestHandleOpLocCoordValidationBoundary(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t) // origin = (100, 100)

	// Extend locTypes to cover id 42 at index 152 is not needed — we reuse type 42.
	// Place a loc at (152, 100, 0), exactly 52 tiles from origin.
	boundaryLoc := entitypkg.NewLoc(0, 152, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	zn := s.zoneMap.Get(0, 152, 100)
	zn.Locs = append(zn.Locs, boundaryLoc)

	if err := handleOpLoc1(p, p2x3Payload(152, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1 at boundary: %v", err)
	}
	if p.target != boundaryLoc {
		t.Errorf("dx=52 should be accepted; target = %v, want boundaryLoc", p.target)
	}

	// Reset and try dx = 53 → reject.
	p.ClearInteraction()
	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(153, 100, 42))
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for dx=53")
	}
	if p.target != nil {
		t.Error("target should remain nil for dx=53")
	}
}

// TestHandleOpLocMissingLocRejected verifies Server.GetLoc returning nil emits UnsetMapFlag.
func TestHandleOpLocMissingLocRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 999)) // wrong locId
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocMissingLocTypeRejected verifies LocType not registered emits UnsetMapFlag.
func TestHandleOpLocMissingLocTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)

	// Place a second loc whose typeID has no LocType registered.
	// locTypes slice has length 43 (indices 0..42), so index 77 is out of range.
	missingTypeLoc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, missingTypeLoc)

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 77))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
	}
}

// TestHandleOpLocAllFiveOpsRouteIndependently runs op 1..5 and confirms targetOp matches.
func TestHandleOpLocAllFiveOpsRouteIndependently(t *testing.T) {
	type opCase struct {
		op   int
		fn   func(*Player, []byte) error
		name string
	}
	cases := []opCase{
		{1, handleOpLoc1, "OpLoc1"},
		{2, handleOpLoc2, "OpLoc2"},
		{3, handleOpLoc3, "OpLoc3"},
		{4, handleOpLoc4, "OpLoc4"},
		{5, handleOpLoc5, "OpLoc5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, p, _, _ := makeOpLocFixture(t)
			if err := c.fn(p, p2x3Payload(100, 100, 42)); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if p.targetOp != c.op {
				t.Errorf("targetOp: got %d, want %d", p.targetOp, c.op)
			}
		})
	}
}

// TestHandleOpLocRejectsEmptyOpSlot verifies that clicking an op slot
// whose Op string is "" emits UnsetMapFlag and leaves state untouched.
// Closes S6j-D1 coverage.
func TestHandleOpLocRejectsEmptyOpSlot(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	// Clear Op[0] so op=1 should reject.
	s.locTypes.Configs[42].Op[0] = ""

	received := drainConn(t, cc)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for empty Op slot, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil when Op slot is empty")
	}
}

// TestHandleOpLocAcceptsPopulatedOpSlot verifies that clicking a
// populated Op slot proceeds through the handler normally. Provides
// positive coverage for the S6k gate.
func TestHandleOpLocAcceptsPopulatedOpSlot(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	s.locTypes.Configs[42].Op[0] = "Chop"

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
}

// TestHandleOpLocClearsExistingInteraction verifies any pre-existing interaction is cleared.
func TestHandleOpLocClearsExistingInteraction(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)

	// Pre-set an interaction with a fake npc at slot 1.
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0, DebugName: "test"}}
	npc := NewNpc(1, 0, 100, 100, 0, typ)
	npc.nid = 1
	s.npcs[1] = npc
	p.SetInteraction(InteractionEngine, npc, 3, -1)
	if p.target != npc {
		t.Fatal("setup: pre-existing target should be npc")
	}

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc (existing npc interaction should be replaced)", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
}
