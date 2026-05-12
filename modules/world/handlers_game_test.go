package world

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
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

// TestHandleMoveClickGate2ClearsWaypoints pins that gate-2 (ctrlHeld out
// of range OR start > 104 tiles away) clears any in-flight waypoints,
// matching TS Player.unsetMapFlag (Player.ts:2169 = clearWaypoints + write).
// Goscape's sendUnsetMapFlag is the wire write only, so moveClickInner
// resets waypointIndex inline at the gate-2 reject site.
func TestHandleMoveClickGate2ClearsWaypoints(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	_ = cc

	// Seed in-flight movement: waypointIndex >= 0 simulates a prior click
	// that's still being walked.
	p.waypointIndex = 0
	p.userPath = []int{42}

	// Trip gate-2 via ctrlHeld=5 (out of [0,1]).
	payload := buildMovePayload(5, p.x, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (gate-2 must clear)", p.waypointIndex)
	}
	if p.userPath != nil {
		t.Errorf("userPath: got %v, want nil (gate-2 must clear)", p.userPath)
	}
}

// ensure net is used (drainConn uses net.Conn)
var _ net.Conn

// --- NAI-93 Bundle 2: ::tele cheat-handler tightening ---

// teleTestPlayer creates a Player wired into a Server with the conn
// handle preserved, an encryptor seeded from a fixed key (for stable
// wire-byte assertions), and the gamemap initialized.
func teleTestPlayer(t *testing.T) (*Player, net.Conn, *Server) {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	enc, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.client.decryptor = dec
	p.x, p.z, p.level = 3094, 3106, 0
	p.originX, p.originZ = 3094, 3106
	p.lastTickX, p.lastTickZ, p.lastLevel = 3094, 3106, 0
	p.slot = 1
	s.players[1] = p
	s.playerLoop = append(s.playerLoop, p)
	p.active = true
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(1))
	}
	return p, cc, s
}

// dispatchTeleCheat sends a `::<cmd>` cheat through handleClientCheat.
// Builds the payload as G1(ctrlHeld) + PJStrLF(cmd), matching the
// rev-225 wire format consumed at handlers_game.go:334.
func dispatchTeleCheat(t *testing.T, p *Player, cheat string) {
	t.Helper()
	pkt := packet.NewPacket(nil)
	pkt.P1(0) // ctrlHeld byte (unused)
	pkt.PJStrLF(cheat)
	if err := handleClientCheat(p, pkt.Data); err != nil {
		t.Fatalf("handleClientCheat: %v", err)
	}
}

// drainAfterTele flushes p.client.bufw and reads everything that lands
// on the test conn.
func drainAfterTele(t *testing.T, p *Player, cc net.Conn) []byte {
	t.Helper()
	received := drainConn(t, cc)
	p.client.flushWrite()
	return <-received
}

// TestTeleCheat_CallsCloseModal — pins that ::tele invokes p.CloseModal,
// closing any open modal. Mirrors TS ClientCheatHandler.ts:504.
func TestTeleCheat_CallsCloseModal(t *testing.T) {
	p, _, _ := teleTestPlayer(t)
	p.staffModLevel = 2
	p.modalState = modalStateMain

	dispatchTeleCheat(t, p, "tele 0,50,50,32,32")

	if p.modalState != modalStateNone {
		t.Errorf("modalState after ::tele: got %d, want %d (modalStateNone)",
			p.modalState, modalStateNone)
	}
}

// TestTeleCheat_CanAccessGate_RejectsAndMessagesGame — pins that the
// canAccess gate rejects with the TS message and DOES NOT teleport.
// Mirrors TS ClientCheatHandler.ts:506-509.
func TestTeleCheat_CanAccessGate_RejectsAndMessagesGame(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 2
	p.delayed = true // forces CanAccess() = false (player_script.go:284)

	startX, startZ := p.x, p.z
	emitted := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "tele 0,50,50,32,32")
	emitted2 := drainAfterTele(t, p, cc)
	emitted = append(emitted, emitted2...)

	if p.x != startX || p.z != startZ {
		t.Errorf("position changed despite CanAccess=false: (%d, %d) → (%d, %d)",
			startX, startZ, p.x, p.z)
	}
	want := []byte("Please finish what you are doing first.")
	if !bytes.Contains(emitted, want) {
		t.Errorf("expected MessageGame body %q in emitted bytes; got %d bytes (none containing target)", string(want), len(emitted))
	}
}

// TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket — pins that
// the cheat handler clears p.waypointIndex AND emits OpUnsetMapFlag,
// matching the TS Player.unsetMapFlag bundle (Player.ts:2169-2172):
// clearWaypoints + write(UnsetMapFlag).
func TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 2
	p.waypointIndex = 5

	// Sibling decoder seeded from same key → can decrypt opcodes in order.
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	dispatchTeleCheat(t, p, "tele 0,50,50,32,32")
	emitted := drainAfterTele(t, p, cc)

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex after ::tele: got %d, want -1", p.waypointIndex)
	}

	// modalState was None pre-dispatch, so CloseModal early-returns and
	// emits no packets. ClearInteraction is state-only. The first
	// emitted byte should therefore be the encrypted OpUnsetMapFlag opcode.
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)",
			gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag opcode %d)",
			emitted[0], wantEnc, gameserver.OpUnsetMapFlag.Opcode)
	}
}

// TestTeleCheat_BoundsCheck_RejectsAfterCleanup — pins TS-faithful
// ordering: closeModal/clearInteraction/unsetMapFlag run BEFORE the
// numeric bounds check. An invalid coord still triggers cleanup side
// effects but does NOT teleport. Matches TS lines 504-522 ordering.
func TestTeleCheat_BoundsCheck_RejectsAfterCleanup(t *testing.T) {
	p, _, _ := teleTestPlayer(t)
	p.staffModLevel = 2
	p.modalState = modalStateMain
	p.waypointIndex = 5
	startX, startZ := p.x, p.z

	// Level 9 is OOB; bounds check at the end of the case rejects.
	dispatchTeleCheat(t, p, "tele 9,50,50,32,32")

	// Cleanup side effects fire BEFORE bounds check (TS-faithful).
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %d, want modalStateNone (cleanup runs before bounds check)", p.modalState)
	}
	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (cleanup runs before bounds check)", p.waypointIndex)
	}
	// Bounds check rejects: position unchanged.
	if p.x != startX || p.z != startZ {
		t.Errorf("position changed despite OOB level: (%d, %d) → (%d, %d)",
			startX, startZ, p.x, p.z)
	}
}

// --- NAI-184 T3: ::fly / ::naive / ::random dev-block cheats ---

// TestHandleClientCheat_Fly_TogglesStrategy pins TS L168-175: ::fly
// toggles between MoveStrategyFly and MoveStrategySmart, emitting a
// MessageGame describing the new state each invocation.
func TestHandleClientCheat_Fly_TogglesStrategy(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 4
	p.moveStrategy = MoveStrategySmart

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "fly")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)
	if p.moveStrategy != MoveStrategyFly {
		t.Errorf("after ::fly: moveStrategy = %v, want MoveStrategyFly", p.moveStrategy)
	}
	if !bytes.Contains(all, []byte("Changed move strategy: fly")) {
		t.Errorf("missing 'Changed move strategy: fly' in emitted bytes")
	}

	dispatchTeleCheat(t, p, "fly")
	emitted3 := drainAfterTele(t, p, cc)
	if p.moveStrategy != MoveStrategySmart {
		t.Errorf("after second ::fly: moveStrategy = %v, want MoveStrategySmart", p.moveStrategy)
	}
	if !bytes.Contains(emitted3, []byte("Changed move strategy: smart")) {
		t.Errorf("missing 'Changed move strategy: smart' in second emit")
	}
}

// TestHandleClientCheat_Naive_TogglesStrategy pins TS L176-183: ::naive
// toggles between MoveStrategyNaive and MoveStrategySmart, emitting a
// MessageGame each invocation.
func TestHandleClientCheat_Naive_TogglesStrategy(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 4
	p.moveStrategy = MoveStrategySmart

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "naive")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)
	if p.moveStrategy != MoveStrategyNaive {
		t.Errorf("after ::naive: moveStrategy = %v, want MoveStrategyNaive", p.moveStrategy)
	}
	if !bytes.Contains(all, []byte("Naive move strategy: naive")) {
		t.Errorf("missing 'Naive move strategy: naive' in emitted bytes")
	}

	dispatchTeleCheat(t, p, "naive")
	emitted3 := drainAfterTele(t, p, cc)
	if p.moveStrategy != MoveStrategySmart {
		t.Errorf("after second ::naive: moveStrategy = %v, want MoveStrategySmart", p.moveStrategy)
	}
	if !bytes.Contains(emitted3, []byte("Naive move strategy: smart")) {
		t.Errorf("missing 'Naive move strategy: smart' in second emit")
	}
}

// TestHandleClientCheat_Random_SetsAfkEventReady pins TS L184-186: ::random
// primes afkEventReady for the next tick.
func TestHandleClientCheat_Random_SetsAfkEventReady(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 4
	p.afkEventReady = false
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "random")

	if !p.afkEventReady {
		t.Errorf("after ::random: afkEventReady = false, want true")
	}
}

// --- NAI-183: ::reboot / ::slowreboot dev-block dead-code pins ---

// TS ClientCheatHandler.ts:360-373 places ::reboot and ::slowreboot
// under `if (!Environment.NODE_PRODUCTION && staffModLevel >= 4)` with
// inner `&& Environment.NODE_PRODUCTION` clauses. Inside that outer
// block NODE_PRODUCTION is always false, so those inner clauses are
// dead. goscape preserves the TS-faithful structure verbatim, so
// ::reboot / ::slowreboot do NOT fire under default config
// (cfg.NodeProduction=false). NAI-183.

// TestHandleClientCheat_Reboot_DeadUnderDefaultConfig pins the TS-faithful
// dead-code semantics for ::reboot at staffModLevel=4 with the default
// cfg.NodeProduction=false: the inner `&& NodeProduction` clause blocks
// the rebootTimer call, so shutdownTick stays at its newTestServer
// initial value (-1). Mirrors TS ClientCheatHandler.ts:360-364.
func TestHandleClientCheat_Reboot_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc) // keep pipe unblocked

	dispatchTeleCheat(t, p, "reboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::reboot under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig — same
// dead-code pin for ::slowreboot with no args. Mirrors TS L365-373.
func TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig — same
// dead-code pin for ::slowreboot with a seconds arg. NAI-183.
func TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot 60")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot 60 under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig —
// same dead-code pin for the tryParseInt-fallback arg path. NAI-183.
func TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot abc")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot abc under !NodeProduction: got %d, want -1 (TS-faithful dead code)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_NoArgsUnderProductionRejects pins the
// TS L367-371 args-empty rejection: under NodeProduction=true at admin
// tier, ::slowreboot with no argument returns false (no rebootTimer call)
// rather than silently defaulting to 30 seconds. NAI-184 T2 fix.
func TestHandleClientCheat_SlowReboot_NoArgsUnderProductionRejects(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	s.cfg.NodeProduction = true
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::slowreboot (no args, NP=true): got %d, want -1 (args-empty rejection per TS L367-371)", s.shutdownTick)
	}
}

// TestHandleClientCheat_SlowReboot_WithArgUnderProductionSchedulesReboot
// pins the happy path: NP=true at admin tier with a valid seconds arg
// schedules a reboot via rebootTimer(ceil(seconds * 1000 / 600)).
// 60 seconds → ceil(100000/600) = 167 ticks. NAI-184 T2 fix.
func TestHandleClientCheat_SlowReboot_WithArgUnderProductionSchedulesReboot(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	s.cfg.NodeProduction = true
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "slowreboot 60")

	// rebootTimer sets s.shutdownTick to currentTick + ticks. Pre-flight:
	// confirm s.shutdownTick != -1 after the call (the test pins
	// "scheduled" rather than the exact tick value, to avoid coupling to
	// the tick counter).
	if s.shutdownTick == -1 {
		t.Errorf("shutdownTick after ::slowreboot 60 (NP=true): got -1, want non-(-1) (rebootTimer should have fired)")
	}
}

// TestHandleClientCheat_ServerDrop_ClosesConn pins that ::serverdrop
// closes the TCP connection but leaves the player in s.players so that
// the next reconnect hits the same slot (onReconnect path).
// Mirrors TS ClientCheatHandler.ts:374-376 player.terminate(). Lives
// in the TS L189 admin block (staffModLevel >= 3) with no NP guard,
// so it fires under any NodeProduction value. NAI-184 T2 relocated
// this arm from the L56 dev block (NAI-183's misclassification).
func TestHandleClientCheat_ServerDrop_ClosesConn(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	slotBefore := p.slot
	_ = cc

	dispatchTeleCheat(t, p, "serverdrop")

	if s.players[slotBefore] != p {
		t.Errorf("player removed from slot %d after ::serverdrop; should remain for reconnect", slotBefore)
	}
	if _, err := p.client.conn.Write([]byte{0}); err == nil {
		t.Error("p.client.conn.Write succeeded after ::serverdrop; expected closed-conn error")
	}
}

// TestHandleClientCheat_RebootCheats_StaffGate pins that ::reboot is
// silently rejected (shutdownTick unchanged) when p.staffModLevel < 3
// (the TS L189 admin gate). NAI-184 T2.
func TestHandleClientCheat_RebootCheats_StaffGate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // below the admin gate (>=3)
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "reboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::reboot with staffModLevel=2: got %d, want -1 (gate blocked)", s.shutdownTick)
	}
}

// --- NAI-183: outer-guard restructure tests ---

// TestHandleClientCheat_AddsSessionLogAtModLevel2 pins the TS L52-54
// `if (staffModLevel >= 2) addSessionLog(MODERATOR, 'Ran cheat', cheat)`
// tier. Dispatches an unrecognized cheat ("foo") so no arm body fires
// and the test isolates the L52 tier. Below modLevel 2, no entry is
// pushed. Join semantics: `message + " " + strings.Join(args, " ")` →
// "Ran cheat foo" (cheat is the lowercased input WITHOUT the stripped
// "::" prefix per handlers_game.go:345-347). NAI-183.
func TestHandleClientCheat_AddsSessionLogAtModLevel2(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	go io.Copy(io.Discard, cc)

	// At staffModLevel=2 the L52 tier fires.
	p.staffModLevel = 2
	dispatchTeleCheat(t, p, "foo")

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs after L52-tier dispatch at staffModLevel=2: got %d, want 1", got)
	}
	if got := s.sessionLogs[0].EventType; got != LoggerEventTypeModerator {
		t.Errorf("EventType: got %d, want %d (LoggerEventTypeModerator)", got, LoggerEventTypeModerator)
	}
	if got := s.sessionLogs[0].Event; got != "Ran cheat foo" {
		t.Errorf("Event: got %q, want %q", got, "Ran cheat foo")
	}

	// Below the gate: no entry pushed.
	s.sessionLogs = s.sessionLogs[:0]
	p.staffModLevel = 1
	dispatchTeleCheat(t, p, "foo")

	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs at staffModLevel=1: got %d, want 0 (below L52 gate)", got)
	}
}

// TestHandleClientCheat_ServerDrop_StaffGate pins that ::serverdrop is
// silently rejected when p.staffModLevel < 3 (TS L189 admin gate).
// Sibling of TestHandleClientCheat_RebootCheats_StaffGate. NAI-184 T2.
func TestHandleClientCheat_ServerDrop_StaffGate(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 2 // below the admin gate (>=3)
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "serverdrop")

	if _, err := p.client.conn.Write([]byte{0}); err != nil {
		t.Errorf("p.client.conn.Write failed after ::serverdrop at staffModLevel=2: %v; want success (gate blocked, conn still open)", err)
	}
}

// TestHandleClientCheat_SetStat_SetsBaseCurAndXP pins TS L401-414.
// ::setstat <skill> <level> writes baseLevels/levels/stats via
// Player.SetStat (which clamps to [1, 99]).
func TestHandleClientCheat_SetStat_SetsBaseCurAndXP(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	dispatchTeleCheat(t, p, "setstat attack 50")
	if p.baseLevels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("baseLevels[ATTACK] = %d, want 50", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("levels[ATTACK] = %d, want 50", p.levels[objtype.PlayerStatAttack])
	}
	wantXP := int32(objtype.GetExpByLevel(50))
	if p.stats[objtype.PlayerStatAttack] != wantXP {
		t.Errorf("stats[ATTACK] = %d, want %d", p.stats[objtype.PlayerStatAttack], wantXP)
	}

	// Unknown stat name: no mutation (TS L410-412 returns false).
	p.baseLevels[objtype.PlayerStatDefence] = 11
	dispatchTeleCheat(t, p, "setstat fake_stat 99")
	if p.baseLevels[objtype.PlayerStatDefence] != 11 {
		t.Errorf("unknown stat mutated DEFENCE: got %d, want 11", p.baseLevels[objtype.PlayerStatDefence])
	}
}

// TestHandleClientCheat_AdvanceStat_ZerosThenAddsXP pins TS L415-431.
// ::advancestat <skill> <level> resets stats[skill]/baseLevels/levels
// to 0/1/1 then calls addXp(skill, getExpByLevel(level)).
func TestHandleClientCheat_AdvanceStat_ZerosThenAddsXP(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	// Pre-populate to verify the L428-431 zero-reset before AddXP.
	p.stats[objtype.PlayerStatAttack] = 999999
	p.baseLevels[objtype.PlayerStatAttack] = 30
	p.levels[objtype.PlayerStatAttack] = 30

	dispatchTeleCheat(t, p, "advancestat attack 50")

	wantXP := int32(objtype.GetExpByLevel(50))
	if p.stats[objtype.PlayerStatAttack] != wantXP {
		t.Errorf("stats[ATTACK] after advancestat = %d, want %d (= GetExpByLevel(50))",
			p.stats[objtype.PlayerStatAttack], wantXP)
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("baseLevels[ATTACK] after advancestat = %d, want 50", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 50 {
		t.Errorf("levels[ATTACK] after advancestat = %d, want 50", p.levels[objtype.PlayerStatAttack])
	}
}

// TestHandleClientCheat_MinMe_AllStatsSetTo1ExceptHitpoints pins TS
// L432-440 verbatim: iterates indices 0..PlayerStatEnabled.length (21),
// sets each to 1 except HITPOINTS which goes to 10. TS does NOT filter
// by PlayerStatEnabled value — STAT18/19 ARE set even though they're
// reserved/unused. Note: SetStat clamps to [1, 99], so the call still
// goes through on disabled slots.
func TestHandleClientCheat_MinMe_AllStatsSetTo1ExceptHitpoints(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 99
		p.levels[i] = 99
		p.stats[i] = int32(objtype.GetExpByLevel(99))
	}

	dispatchTeleCheat(t, p, "minme")

	for i := 0; i < objtype.PlayerStatCount; i++ {
		want := uint8(1)
		if i == objtype.PlayerStatHitpoints {
			want = 10
		}
		if p.baseLevels[i] != want {
			t.Errorf("stat %d after minme: baseLevels = %d, want %d (TS L432-440 sets ALL 21 stats; no PlayerStatEnabled filter)", i, p.baseLevels[i], want)
		}
		if p.levels[i] != want {
			t.Errorf("stat %d after minme: levels = %d, want %d", i, p.levels[i], want)
		}
	}
}

// --- NAI-184 T6: ::give / ::givemany admin-block cheats ---

// TestHandleClientCheat_Give_AddsToInv pins TS L288-302 — ::give <obj> [count].
// Resolves obj by name via ObjTypeConfigs.ByName, count clamps to
// [1, 0x7fffffff] (default 1 when missing), routes through Player.InvAdd
// with assureFullInsertion=false (TS L302).
func TestHandleClientCheat_Give_AddsToInv(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const objName = "test_coin"
	const objID = 995

	// Build the inv slot, the obj (stackable=true, debug name="test_coin"),
	// and wire ByName via ConfigNames.
	invID := mustSetupTestInv(t, s, 0, 28)
	mustSetupNamedObj(t, s, objID, objName, /*stackable=*/ true)
	s.invTypes.Inv = invID

	dispatchTeleCheat(t, p, "give "+objName+" 5")

	inv := s.invLookup.Get(p, invID)
	if got := totalUnits(inv, objID); got != 5 {
		t.Errorf("after ::give test_coin 5: total = %d, want 5", got)
	}

	// Missing count → defaults to 1.
	dispatchTeleCheat(t, p, "give "+objName)
	if got := totalUnits(inv, objID); got != 6 {
		t.Errorf("after ::give test_coin (no count): total = %d, want 6 (5+1)", got)
	}

	// Unknown obj name → no mutation.
	dispatchTeleCheat(t, p, "give fake_obj 7")
	if got := totalUnits(inv, objID); got != 6 {
		t.Errorf("after ::give fake_obj 7: total = %d, want 6 (unknown obj)", got)
	}

	// Empty args → reject (TS L290-294 args.length<1).
	dispatchTeleCheat(t, p, "give")
	if got := totalUnits(inv, objID); got != 6 {
		t.Errorf("after ::give (no args): total = %d, want 6 (rejected)", got)
	}
}

// TestHandleClientCheat_GiveMany_Adds1000 pins TS L339-352 — fixed count of 1000.
func TestHandleClientCheat_GiveMany_Adds1000(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const objName = "test_coin"
	const objID = 995

	invID := mustSetupTestInv(t, s, 0, 28)
	mustSetupNamedObj(t, s, objID, objName, true)
	s.invTypes.Inv = invID

	dispatchTeleCheat(t, p, "givemany "+objName)

	inv := s.invLookup.Get(p, invID)
	if got := totalUnits(inv, objID); got != 1000 {
		t.Errorf("after ::givemany test_coin: total = %d, want 1000", got)
	}
}

// TestHandleClientCheat_Give_AdminGate pins that staffModLevel < 3 silently
// rejects (admin tier gate).
func TestHandleClientCheat_Give_AdminGate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // below admin tier
	go io.Copy(io.Discard, cc)
	const objID = 995
	invID := mustSetupTestInv(t, s, 0, 28)
	mustSetupNamedObj(t, s, objID, "test_coin", true)
	s.invTypes.Inv = invID

	dispatchTeleCheat(t, p, "give test_coin 5")

	inv := s.invLookup.Get(p, invID)
	if got := totalUnits(inv, objID); got != 0 {
		t.Errorf("after ::give at modLevel=2: total = %d, want 0 (admin gate rejected)", got)
	}
}

// TestHandleClientCheat_Snapshot_WritesHeapFile pins TS L477-480
// (functional analog): ::snapshot at admin tier writes a heap-*.pprof
// file under TMPDIR. TS uses v8 heap snapshot; goscape uses
// runtime/pprof.WriteHeapProfile — different format, same dispatch
// behavior. NAI-184 T7.
func TestHandleClientCheat_Snapshot_WritesHeapFile(t *testing.T) {
	p, cc, _ := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	preFiles, _ := filepath.Glob(filepath.Join(os.TempDir(), "heap-*.pprof"))

	dispatchTeleCheat(t, p, "snapshot")

	postFiles, _ := filepath.Glob(filepath.Join(os.TempDir(), "heap-*.pprof"))
	if len(postFiles) <= len(preFiles) {
		t.Errorf("snapshot did not write a heap-*.pprof file (pre=%d, post=%d)", len(preFiles), len(postFiles))
	}

	// Cleanup newly-created files.
	for _, f := range postFiles {
		found := false
		for _, pf := range preFiles {
			if pf == f {
				found = true
				break
			}
		}
		if !found {
			os.Remove(f)
		}
	}
}

// addOtherTestPlayer adds a second active Player to s.playerLoop with
// the given username and coord. Used by teleother/teleto tests.
// Wires the minimum state for LookupPlayerByUsername + TeleJump to work:
// active=true, username, x/z/level/originX/originZ/lastTickX/Z/lastLevel,
// slot assignment, playerLoop append.
func addOtherTestPlayer(t *testing.T, s *Server, username string, x, z, level int) *Player {
	t.Helper()
	other, otherConn := newTestPlayer(t)
	other.client.server = s
	// Seed encryptor so any writeOut path (e.g. sendUnsetMapFlag for
	// teleother) doesn't NPE on ISAAC.GetNext. Drain the conn so writes
	// don't block.
	enc, dec := isaacPair([4]uint32{9, 9, 9, 9})
	other.client.encryptor = enc
	other.client.decryptor = dec
	go io.Copy(io.Discard, otherConn)
	other.username = username
	other.x, other.z, other.level = x, z, level
	other.originX, other.originZ = x, z
	other.lastTickX, other.lastTickZ, other.lastLevel = x, z, level
	// Find next free slot. teleTestPlayer uses slot 1.
	slot := 2
	for slot < len(s.players) && s.players[slot] != nil {
		slot++
	}
	if slot >= len(s.players) {
		t.Fatalf("addOtherTestPlayer: no free player slot")
	}
	other.slot = slot
	s.players[slot] = other
	s.playerLoop = append(s.playerLoop, other)
	other.active = true
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(slot))
	}
	return other
}

// TestHandleClientCheat_TeleOther_MovesTargetToSource pins TS L377-400
// happy path: target player teleports to the caller's coord.
func TestHandleClientCheat_TeleOther_MovesTargetToSource(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true
	p.x, p.z, p.level = 3200, 3200, 0
	go io.Copy(io.Discard, cc)

	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3200 || other.z != 3200 || other.level != 0 {
		t.Errorf("after ::teleother: other at (%d, %d, %d), want (3200, 3200, 0)", other.x, other.z, other.level)
	}
}

// TestHandleClientCheat_TeleOther_NoOpWhenNotProduction pins the
// `&& NodeProduction` outer-arm-selector gate from TS L377.
func TestHandleClientCheat_TeleOther_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = false
	go io.Copy(io.Discard, cc)
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3220 || other.z != 3220 {
		t.Errorf("teleother under NP=false moved other: at (%d, %d), want (3220, 3220)", other.x, other.z)
	}
}

// TestHandleClientCheat_TeleOther_UnknownUserMessagesCaller pins TS L385-388:
// when LookupPlayerByUsername returns nil, message the caller and reject.
func TestHandleClientCheat_TeleOther_UnknownUserMessagesCaller(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "teleother no_such_user")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes")
	}
}

// TestHandleClientCheat_TeleOther_AdminGate pins that staffModLevel < 3
// silently rejects (admin tier gate).
func TestHandleClientCheat_TeleOther_AdminGate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // below admin tier
	s.cfg.NodeProduction = true
	go io.Copy(io.Discard, cc)
	other := addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleother other_user")

	if other.x != 3220 {
		t.Errorf("teleother at staffModLevel=2 moved other: at x=%d, want 3220", other.x)
	}
}

// TestHandleClientCheat_TeleTo_MovesSourceToTarget pins TS L525-548
// happy path: caller teleports to target's coord.
func TestHandleClientCheat_TeleTo_MovesSourceToTarget(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // super-mod tier
	s.cfg.NodeProduction = true
	p.x, p.z, p.level = 3200, 3200, 0
	go io.Copy(io.Discard, cc)
	addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleto other_user")

	if p.x != 3220 || p.z != 3220 {
		t.Errorf("after ::teleto: caller at (%d, %d), want (3220, 3220)", p.x, p.z)
	}
}

// TestHandleClientCheat_TeleTo_NoOpWhenNotProduction pins NP gate on teleto.
func TestHandleClientCheat_TeleTo_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2
	p.x, p.z = 3200, 3200
	s.cfg.NodeProduction = false
	go io.Copy(io.Discard, cc)
	addOtherTestPlayer(t, s, "other_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "teleto other_user")

	if p.x != 3200 || p.z != 3200 {
		t.Errorf("teleto under NP=false moved caller: at (%d, %d), want (3200, 3200)", p.x, p.z)
	}
}

