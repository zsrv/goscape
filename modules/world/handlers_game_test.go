package world

import (
	"bytes"
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// TestHandleMessagePublic_SetsMaskChat pins the wire-format decode for the
// MESSAGE_PUBLIC opcode handler. After processing, the player must carry
// MaskChat in its mask set and the chat fields (colour, effect, rights,
// bytes) must match what the handler decoded.
//
// Pre-fix, opcode 158 had no handler — readPacket silently discarded the
// packet and other tracked clients never saw the chat. NAI-32 Bundle 3
// Stage 6 added the handler.
func TestHandleMessagePublic_SetsMaskChat(t *testing.T) {
	p := &Player{}
	// Wire format: [color=2, effect=1, text=0x12 0x34 0x56]
	payload := []byte{2, 1, 0x12, 0x34, 0x56}

	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic returned error: %v", err)
	}

	if p.masks&rsbuf.MaskChat == 0 {
		t.Errorf("p.masks: MaskChat bit not set; got 0x%x", p.masks)
	}
	if p.chatColour != 2 {
		t.Errorf("chatColour: got %d, want 2", p.chatColour)
	}
	if p.chatEffect != 1 {
		t.Errorf("chatEffect: got %d, want 1", p.chatEffect)
	}
	if p.chatRights != 0 {
		t.Errorf("chatRights: got %d, want 0 (no staff)", p.chatRights)
	}
	want := []byte{0x12, 0x34, 0x56}
	if !bytes.Equal(p.chatBytes, want) {
		t.Errorf("chatBytes: got %v, want %v", p.chatBytes, want)
	}
}

// TestHandleMessagePublic_RightsFromStaffModLevel pins that the player's
// staffModLevel propagates as the rights field, so staff get the priority
// chat (var15 > 1 routes to client.java:10520's addMessage type 1).
func TestHandleMessagePublic_RightsFromStaffModLevel(t *testing.T) {
	p := &Player{staffModLevel: 2}
	payload := []byte{0, 0, 0xaa}
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic error: %v", err)
	}
	if p.chatRights != 2 {
		t.Errorf("chatRights: got %d, want 2 (staff mod level passthrough)", p.chatRights)
	}
}

// TestHandleMessagePublic_ShortPayloadIsNoop pins that a payload shorter
// than 2 bytes is dropped silently — no MaskChat, no panic.
func TestHandleMessagePublic_ShortPayloadIsNoop(t *testing.T) {
	p := &Player{}
	payload := []byte{0} // missing effect byte
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic error: %v", err)
	}
	if p.masks&rsbuf.MaskChat != 0 {
		t.Errorf("p.masks should not have MaskChat for short payload; got 0x%x", p.masks)
	}
}

// buildMovePayload encodes a minimal MOVE_GAMECLICK/MOVE_OPCLICK payload with
// no extra waypoints: [ctrlHeld(1), startX(2), startZ(2)].
func buildMovePayload(ctrlHeld int, startX, startZ int) []byte {
	return []byte{
		byte(ctrlHeld),
		byte(startX >> 8), byte(startX),
		byte(startZ >> 8), byte(startZ),
	}
}

// TestHandleMoveGameClickClosesChatModal pins symptom-2: a plain walk click
// (MOVE_GAMECLICK) must close an open chat modal via ClearPendingAction.
// Mirrors TS MoveClickHandler.ts:!opClick → clearPendingAction().
func TestHandleMoveGameClickClosesChatModal(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Open a chat modal.
	p.modalChat = 100
	p.modalState |= modalStateChat

	payload := buildMovePayload(0, p.x, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (closed by ClearPendingAction)", p.modalChat)
	}
	if p.modalState&modalStateChat != modalStateNone {
		t.Errorf("modalState chat bit still set: got 0x%x", p.modalState)
	}
}

// TestHandleMoveOpClickPreservesChatModal pins that MOVE_OPCLICK (opClick=true)
// skips ClearPendingAction, leaving an open chat modal untouched.
func TestHandleMoveOpClickPreservesChatModal(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Open a chat modal.
	p.modalChat = 100
	p.modalState |= modalStateChat

	payload := buildMovePayload(0, p.x, p.z)
	if err := handleMoveOpClick(p, payload); err != nil {
		t.Fatalf("handleMoveOpClick: %v", err)
	}

	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (unchanged — op click skips ClearPendingAction)", p.modalChat)
	}
	if p.modalState&modalStateChat == modalStateNone {
		t.Errorf("modalState chat bit cleared unexpectedly")
	}
}

// TestHandleMoveClickDelayedSendsUnsetMapFlag pins that a delayed player receives
// UnsetMapFlag and the handler returns early without touching modal state.
func TestHandleMoveClickDelayedSendsUnsetMapFlag(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Open a chat modal — must remain open.
	p.modalChat = 100
	p.modalState |= modalStateChat

	// Mark player as delayed.
	s.currentTick = 0
	p.delayed = true
	p.delayedUntil = 5

	payload := buildMovePayload(0, p.x, p.z)
	received := drainConn(t, cc)
	_ = handleMoveGameClick(p, payload)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag packet for delayed player, got nothing")
	}
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (unchanged — early return)", p.modalChat)
	}
}

// TestHandleMoveClickInvalidCtrlHeldRejects pins that ctrlHeld outside [0,1]
// triggers UnsetMapFlag and early return without modifying modal state.
func TestHandleMoveClickInvalidCtrlHeldRejects(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Open a chat modal — must remain open.
	p.modalChat = 100
	p.modalState |= modalStateChat

	// ctrlHeld=5 is out of [0,1].
	payload := buildMovePayload(5, p.x, p.z)
	received := drainConn(t, cc)
	_ = handleMoveGameClick(p, payload)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for invalid ctrlHeld, got nothing")
	}
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (unchanged — early return)", p.modalChat)
	}
}

// TestHandleMoveClickStartTooFarRejects pins that a destination more than 104
// tiles away (Chebyshev) triggers UnsetMapFlag and early return.
func TestHandleMoveClickStartTooFarRejects(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Open a chat modal — must remain open.
	p.modalChat = 100
	p.modalState |= modalStateChat

	// startX = p.x + 200 is 200 tiles away — exceeds 104.
	payload := buildMovePayload(0, p.x+200, p.z)
	received := drainConn(t, cc)
	_ = handleMoveGameClick(p, payload)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for too-far destination, got nothing")
	}
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (unchanged — early return)", p.modalChat)
	}
}

// TestHandleMoveGameClickSetsTempRunFromCtrlHeld pins that ctrlHeld=1 with
// sufficient runenergy sets tempRun=1.
func TestHandleMoveGameClickSetsTempRunFromCtrlHeld(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Default runenergy=10000 (>=100), so tempRun should follow ctrlHeld.
	payload := buildMovePayload(1, p.x, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.tempRun != 1 {
		t.Errorf("tempRun: got %d, want 1 (ctrlHeld=1, runenergy high)", p.tempRun)
	}
}

// TestHandleMoveGameClickRunenergyLowSuppressesTempRun pins that ctrlHeld=1
// with runenergy<100 overrides tempRun to 0 (can't run on empty energy).
func TestHandleMoveGameClickRunenergyLowSuppressesTempRun(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	p.runenergy = 50 // below 100 threshold

	payload := buildMovePayload(1, p.x, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.tempRun != 0 {
		t.Errorf("tempRun: got %d, want 0 (runenergy<100 overrides ctrlHeld)", p.tempRun)
	}
}

// TestHandleMoveGameClickFiresWalktriggerWhenPlayerpacket pins that
// processWalktrigger is called when WalkTriggerSetting==PLAYERPACKET and the
// player has waypoints. Observable via p.walktrigger being reset to -1.
func TestHandleMoveGameClickFiresWalktriggerWhenPlayerpacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Precondition: default cfg has WalkTriggerSetting=PLAYERPACKET (value 0).
	if s.cfg.NodeWalktriggerSetting != WalkTriggerSettingPlayerpacket {
		t.Fatalf("precondition: NodeWalktriggerSetting=%v, want PLAYERPACKET", s.cfg.NodeWalktriggerSetting)
	}

	// Set a walktrigger so processWalktrigger has something to clear.
	p.walktrigger = 42

	// Dest one tile away so hasWaypoints() returns true after pathToMoveClick.
	payload := buildMovePayload(0, p.x+1, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (cleared by processWalktrigger)", p.walktrigger)
	}
}

// TestHandleMoveGameClickSkipsWalktriggerWhenSettingNotPlayerpacket pins that
// processWalktrigger is NOT called when WalkTriggerSetting!=PLAYERPACKET.
// walktrigger remains at its preset value.
func TestHandleMoveGameClickSkipsWalktriggerWhenSettingNotPlayerpacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Override setting to PLAYERSETUP.
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup

	p.walktrigger = 42

	payload := buildMovePayload(0, p.x+1, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (unchanged — setting is not PLAYERPACKET)", p.walktrigger)
	}
}

// ensure net is used (drainConn uses net.Conn)
var _ net.Conn
