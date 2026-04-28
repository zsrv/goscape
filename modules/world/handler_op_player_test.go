package world

import (
	"fmt"
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
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
