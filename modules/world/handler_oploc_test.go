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

// p2x4Payload encodes (x: u16, z: u16, locId: u16, com: u16) into 8 bytes big-endian.
// Used by OpLocT payload construction.
func p2x4Payload(x, z, locId, com int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
		byte(com >> 8), byte(com),
	}
}

// TestHandleOpLocTSetsInteraction verifies OpLocT decodes a valid payload
// and routes through SetInteraction with targetOp=targetOpLocT and
// targetSubject.com=spellCom.
func TestHandleOpLocTSetsInteraction(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)

	if err := handleOpLocT(p, p2x4Payload(100, 100, 42, 7777)); err != nil {
		t.Fatalf("handleOpLocT: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != targetOpLocT {
		t.Errorf("targetOp: got %d, want targetOpLocT (%d)", p.targetOp, targetOpLocT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d,level=%d), want (42,100,100,0)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
}

// TestHandleOpLocTDelayedPlayerRejected verifies delayed → UnsetMapFlag.
func TestHandleOpLocTDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpLocT(p, p2x4Payload(100, 100, 42, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

// TestHandleOpLocTShortPayloadRejected verifies <8 bytes → UnsetMapFlag.
func TestHandleOpLocTShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	_ = handleOpLocT(p, []byte{0x00, 0x64, 0x00, 0x64}) // only 4 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocTOutOfViewportRejected verifies dx > 52 → UnsetMapFlag.
func TestHandleOpLocTOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// origin is (100, 100); dx = 250-100 = 150 > 52.

	received := drainConn(t, cc)
	_ = handleOpLocT(p, p2x4Payload(250, 100, 42, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

// TestHandleOpLocTMissingLocRejected verifies Server.GetLoc nil → UnsetMapFlag.
func TestHandleOpLocTMissingLocRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	// locId 999 is not registered in the fixture zone.
	_ = handleOpLocT(p, p2x4Payload(100, 100, 999, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocTMissingLocTypeRejected verifies missing LocType → UnsetMapFlag.
func TestHandleOpLocTMissingLocTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)

	// Place a second loc at (100, 100) with typeID 77, but don't register
	// LocType 77 — Configs[77] stays nil (or out-of-bounds — either way nil check fires).
	extraLoc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, extraLoc)

	received := drainConn(t, cc)
	_ = handleOpLocT(p, p2x4Payload(100, 100, 77, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
	}
}

// p2x6Payload encodes (x, z, locId, useObj, useSlot, useCom) into 12 bytes big-endian.
// Used by OpLocU payload construction.
func p2x6Payload(x, z, locId, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// TestHandleOpLocUSetsInteraction verifies OpLocU decodes a valid payload
// and routes through SetInteraction with targetOp=targetOpLocU. useObj
// and useSlot land on p.lastUseItem/lastUseSlot; useCom is discarded
// (S6m-D2/D3).
func TestHandleOpLocUSetsInteraction(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)

	if err := handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpLocU: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != targetOpLocU {
		t.Errorf("targetOp: got %d, want targetOpLocU (%d)", p.targetOp, targetOpLocU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpLocU passes -1)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d,level=%d), want (42,100,100,0)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}

// TestHandleOpLocUDelayedPlayerRejected verifies delayed → UnsetMapFlag,
// and that lastUseItem is NOT clobbered (defensive: delayed rejection
// happens before any player-state mutation).
func TestHandleOpLocUDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged

	received := drainConn(t, cc)
	_ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

// TestHandleOpLocUShortPayloadRejected verifies <12 bytes → UnsetMapFlag.
func TestHandleOpLocUShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	_ = handleOpLocU(p, []byte{0x00, 0x64, 0x00, 0x64, 0x00, 0x2a, 0x05, 0xe7}) // 8 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocUOutOfViewportRejected verifies dx > 52 → UnsetMapFlag.
func TestHandleOpLocUOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// origin (100,100); dx = 250-100 = 150 > 52.

	received := drainConn(t, cc)
	_ = handleOpLocU(p, p2x6Payload(250, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

// TestHandleOpLocUMissingLocRejected verifies Server.GetLoc nil → UnsetMapFlag.
func TestHandleOpLocUMissingLocRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	received := drainConn(t, cc)
	// locId 999 is not registered in fixture zone.
	_ = handleOpLocU(p, p2x6Payload(100, 100, 999, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocUMissingLocTypeRejected verifies missing LocType → UnsetMapFlag.
func TestHandleOpLocUMissingLocTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)

	// Place a loc with typeID 77 but no registered LocType.
	extraLoc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, extraLoc)

	received := drainConn(t, cc)
	_ = handleOpLocU(p, p2x6Payload(100, 100, 77, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
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
