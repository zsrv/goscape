package world

import (
	"fmt"
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeOpPlayerFixture builds a server with two logged-in players (clicker
// at slot 1 and "other" at slot 2). The clicker has a real net.Conn for
// drainConn-based UnsetMapFlag assertions; "other" is wired with the
// minimum needed to satisfy LookupPlayerBySlot + slot indexing.
//
// Returns (server, clicker, otherPlayer, clickerConn).
func makeOpPlayerFixture(t *testing.T) (*Server, *Player, *Player, net.Conn) {
	t.Helper()
	s := newTestServer(t)

	clicker, cc := newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	clicker.slot = 1
	s.players[1] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	other, _ := newTestPlayer(t)
	other.client.server = s
	other.slot = 2
	s.players[2] = other
	s.rsbuf.AddPlayer(int32(other.slot))

	return s, clicker, other, cc
}

// rsbufSeesPlayer makes s.rsbuf.HasPlayer(observer, target) return true
// by inserting target into observer's BuildArea.Players tracking set
// directly (test-only path; production code goes through ComputePlayer).
func rsbufSeesPlayer(t *testing.T, s *Server, observerSlot, targetSlot int) {
	t.Helper()
	bp := s.rsbuf.PlayerForTest(int32(observerSlot))
	if bp == nil {
		t.Fatalf("rsbuf has no player at observer slot %d", observerSlot)
	}
	bp.Build.Players.Insert(int32(targetSlot))
}

// TestHandleOpPlayer_HappyPath_AllOps — for each of op 1..4, the handler
// sets target = other, targetOp = op, targetSubject.com = -1, kind =
// InteractionEngine.
func TestHandleOpPlayer_HappyPath_AllOps(t *testing.T) {
	for op := 1; op <= 4; op++ {
		t.Run(fmt.Sprintf("op=%d", op), func(t *testing.T) {
			s, clicker, other, _ := makeOpPlayerFixture(t)
			rsbufSeesPlayer(t, s, clicker.slot, other.slot)

			if err := handleOpPlayer(clicker, p2Payload(other.slot), op); err != nil {
				t.Fatalf("handleOpPlayer: %v", err)
			}

			if clicker.target != other {
				t.Errorf("target: got %v, want other (%p)", clicker.target, other)
			}
			if clicker.targetOp != op {
				t.Errorf("targetOp: got %d, want %d", clicker.targetOp, op)
			}
			if clicker.targetSubject.com != -1 {
				t.Errorf("targetSubject.com: got %d, want -1", clicker.targetSubject.com)
			}
			if clicker.interactionKind != InteractionEngine {
				t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
			}
		})
	}
}

// TestHandleOpPlayer_DelayedSendsUnsetMapFlag — when the player is
// delayed, handler skips interaction setup and writes UnsetMapFlag.
func TestHandleOpPlayer_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(other.slot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayer_TargetNotLoggedIn — LookupPlayerBySlot returns nil
// (slot empty) → UnsetMapFlag, no interaction set.
func TestHandleOpPlayer_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(missingSlot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayer_NotVisibleViaRsbuf — target exists but not visible
// to local player per rsbuf.HasPlayer → UnsetMapFlag, no interaction set.
func TestHandleOpPlayer_NotVisibleViaRsbuf(t *testing.T) {
	_, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(other.slot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayer_TruncatedPayload — payload < 2 bytes → UnsetMapFlag.
func TestHandleOpPlayer_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, []byte{0x01}, 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// p2x2Payload encodes (a: u16, b: u16) into 4 bytes big-endian.
// Used by OpPlayerT payload construction: slot + spellCom.
func opPlayerTPayload(slot, spellCom int) []byte {
	return []byte{
		byte(slot >> 8), byte(slot),
		byte(spellCom >> 8), byte(spellCom),
	}
}

// TestHandleOpPlayerT_HappyPath — valid OPPLAYERT request sets target,
// targetOp = targetOpPlayerT, targetSubject.com = spellCom, kind = Engine.
func TestHandleOpPlayerT_HappyPath(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const spellCom = 7777
	if err := handleOpPlayerT(clicker, opPlayerTPayload(other.slot, spellCom)); err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetOp != targetOpPlayerT {
		t.Errorf("targetOp: got %d, want targetOpPlayerT (%d)", clicker.targetOp, targetOpPlayerT)
	}
	if clicker.targetSubject.com != spellCom {
		t.Errorf("targetSubject.com: got %d, want %d (spellCom)", clicker.targetSubject.com, spellCom)
	}
	if clicker.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
	}
}

// TestHandleOpPlayerT_DelayedSendsUnsetMapFlag — delayed clicker →
// UnsetMapFlag, no interaction set.
func TestHandleOpPlayerT_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(other.slot, 7777))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerT_TargetNotLoggedIn — LookupPlayerBySlot returns nil →
// UnsetMapFlag, no interaction set.
func TestHandleOpPlayerT_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(missingSlot, 7777))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerT_TargetNotVisible — target exists but rsbuf.HasPlayer
// is false → UnsetMapFlag, no interaction set.
func TestHandleOpPlayerT_TargetNotVisible(t *testing.T) {
	_, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(other.slot, 7777))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerT_TruncatedPayload — payload < 4 bytes → UnsetMapFlag.
func TestHandleOpPlayerT_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, []byte{0x00, 0x02, 0x01})
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// opPlayerUPayload encodes (slot:u16, useObj:u16, useSlot:u16, useCom:u16)
// into 8 bytes big-endian. Used by OpPlayerU payload construction.
func opPlayerUPayload(slot, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(slot >> 8), byte(slot),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// seedOpPlayerUInv populates s.invs[invType] with `useObj` at slot
// `useSlot`, and registers a player-local inv listener at component
// `useCom` pointing at world-shared inv (Source = -1). Mirrors the
// fixture pattern from handler_opnpc_test.go's TestHandleOpNpcUSetsInteraction.
func seedOpPlayerUInv(t *testing.T, s *Server, p *Player, invType, useCom, useObj, useSlot int) {
	t.Helper()
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(invType, 28, inventory.StackNormal)
	inv.Items[useSlot] = &inventory.Item{Id: useObj, Count: 1}
	s.invs[invType] = inv
	p.invListenOnCom(invType, useCom, -1)
}

// TestHandleOpPlayerU_HappyPath — valid OPPLAYERU request sets target,
// targetOp = targetOpPlayerU, targetSubject.com = -1 (useCom discarded),
// lastUseItem = useObj, lastUseSlot = useSlot, kind = Engine.
func TestHandleOpPlayerU_HappyPath(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const (
		invType = 93
		useCom  = 149
		useObj  = 1511
		useSlot = 3
	)
	seedOpPlayerUInv(t, s, clicker, invType, useCom, useObj, useSlot)

	if err := handleOpPlayerU(clicker, opPlayerUPayload(other.slot, useObj, useSlot, useCom)); err != nil {
		t.Fatalf("handleOpPlayerU: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetOp != targetOpPlayerU {
		t.Errorf("targetOp: got %d, want targetOpPlayerU (%d)", clicker.targetOp, targetOpPlayerU)
	}
	if clicker.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1", clicker.targetSubject.com)
	}
	if clicker.lastUseItem != useObj {
		t.Errorf("lastUseItem: got %d, want %d (useObj)", clicker.lastUseItem, useObj)
	}
	if clicker.lastUseSlot != useSlot {
		t.Errorf("lastUseSlot: got %d, want %d", clicker.lastUseSlot, useSlot)
	}
	if clicker.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
	}
}

// TestHandleOpPlayerU_DelayedSendsUnsetMapFlag — delayed clicker →
// UnsetMapFlag, no interaction set, lastUseItem unmodified.
func TestHandleOpPlayerU_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0
	clicker.lastUseItem = 42 // sentinel: must stay unchanged on rejection

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	if clicker.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_TargetNotLoggedIn — LookupPlayerBySlot returns nil →
// UnsetMapFlag, no interaction set.
func TestHandleOpPlayerU_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(missingSlot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_TargetNotVisible — target exists but rsbuf.HasPlayer
// is false → UnsetMapFlag, no interaction set.
func TestHandleOpPlayerU_TargetNotVisible(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_TruncatedPayload — payload < 8 bytes → UnsetMapFlag.
func TestHandleOpPlayerU_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, []byte{0x00, 0x02, 0x05, 0xE7}) // only 4 bytes
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_InvListenerMissing — no invListener registered for
// useCom → UnsetMapFlag, lastUseItem unmodified.
func TestHandleOpPlayerU_InvListenerMissing(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// NO invListenOnCom.
	clicker.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	if clicker.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_ItemNotInSlot — registered listener resolves an
// inv where the claimed slot does NOT hold the claimed useObj →
// UnsetMapFlag, lastUseItem unmodified.
func TestHandleOpPlayerU_ItemNotInSlot(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	clicker.invListenOnCom(93, 149, -1)
	clicker.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	if clicker.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", clicker.lastUseItem)
	}
}

// TestHandleOpPlayer1SetsOpcalled verifies success path sets p.opcalled=true.
func TestHandleOpPlayer1SetsOpcalled(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	if err := handleOpPlayer1(clicker, p2Payload(other.slot)); err != nil {
		t.Fatalf("handleOpPlayer1: %v", err)
	}

	if !clicker.opcalled {
		t.Error("opcalled: want true after successful handleOpPlayer1, got false")
	}
}

// TestHandleOpPlayerU_MembersOnNonMembersServer — useObj is members-only
// and NodeMembers is false → MessageGame + UnsetMapFlag, no interaction.
// Mirrors TestHandleOpNpcUMembersOnFreeWorldRejected fixture.
func TestHandleOpPlayerU_MembersOnNonMembersServer(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	s.cfg.NodeMembers = false
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
	}
	s.objTypes.Configs[1511] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
		Members:    true,
	}
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame + UnsetMapFlag for members-on-free, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil for members-on-free rejection; got %v", clicker.target)
	}
}
