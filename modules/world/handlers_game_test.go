package world

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
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

// --- NAI-188: ::speed dev-block cheat ---
//
// TS ClientCheatHandler.ts:154-167 ports to the dev block at
// modules/world/handlers_game.go. Branch matrix:
//   args == ""       → "Usage: ::speed <ms>"; no state change.
//   parsed < 20      → "::speed input was too low."; no state change.
//   parsed >= 20     → "World speed was changed to {ms}ms"; s.tickRate update.
// Per spec §7.1, non-numeric arg traces to default 20 via parseIntOr
// (TS tryParseInt fallback), which is >= floor → success at 20ms.
// Negative numeric arg (-5) parses to -5, which is < 20 → "too low".

func TestHandleClientCheat_Speed_EmptyArgs_EmitsUsageMessage(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("Usage: ::speed <ms>")) {
		t.Errorf("missing 'Usage: ::speed <ms>' in emitted bytes")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on empty args)", s.tickRate, priorRate)
	}
}

func TestHandleClientCheat_Speed_BelowFloor_EmitsTooLow(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 19")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("::speed input was too low.")) {
		t.Errorf("missing '::speed input was too low.' in emitted bytes")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on too-low input)", s.tickRate, priorRate)
	}
}

func TestHandleClientCheat_Speed_AtFloor_SetsTickRate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 20")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 20ms")) {
		t.Errorf("missing 'World speed was changed to 20ms' in emitted bytes")
	}
	want := 20 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed 20: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_AboveFloor_SetsTickRate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 100")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 100ms")) {
		t.Errorf("missing 'World speed was changed to 100ms' in emitted bytes")
	}
	want := 100 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed 100: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_NonNumeric_DefaultsTo20ms(t *testing.T) {
	// Per spec §7.1: TS tryParseInt("banana", 20) returns 20 (the
	// default), and 20 < 20 is false → success branch at 20ms.
	// parseIntOr mirrors this exactly.
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed banana")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 20ms")) {
		t.Errorf("missing 'World speed was changed to 20ms' in emitted bytes (non-numeric traces to default 20)")
	}
	want := 20 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed banana: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_Negative_EmitsTooLow(t *testing.T) {
	// Per spec §7.1: parseIntOr("-5", 20) == -5; -5 < 20 → "too low".
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed -5")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("::speed input was too low.")) {
		t.Errorf("missing '::speed input was too low.' in emitted bytes for negative input")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on negative input)", s.tickRate, priorRate)
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

// setvarTestFixture extends teleTestPlayer with a populated VarpTypeConfigs
// containing two varps: id=0 "transmit_only" (Transmit=true, Protect=false),
// id=1 "protect_var" (Transmit=true, Protect=true). Returns the same
// (player, conn, server) tuple as teleTestPlayer.
func setvarTestFixture(t *testing.T) (*Player, net.Conn, *Server) {
	t.Helper()
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: []*objtype.VarPlayerType{
			{ConfigType: objtype.ConfigType{ID: 0, DebugName: "transmit_only"}, Transmit: true, Protect: false},
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "protect_var"}, Transmit: true, Protect: true},
		},
		ConfigNames: map[string]int{"transmit_only": 0, "protect_var": 1},
	}
	// Player.varps must be sized for SetVarp to be in-range.
	p.varps = make([]int32, len(s.varpTypes.Configs))
	return p, cc, s
}

func TestHandleClientCheat_SetVar_HappyPath_SetsVarpAndMessagesCaller(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only 42")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 42 {
		t.Errorf("varps[0] after setvar: got %d, want 42", p.varps[0])
	}
	if !bytes.Contains(emitted, []byte("set transmit_only: to 42")) {
		t.Errorf("expected 'set transmit_only: to 42' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_MissingValueArg_Rejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 0 {
		t.Errorf("varps[0] after setvar with missing value: got %d, want 0 (unchanged)", p.varps[0])
	}
	if bytes.Contains(emitted, []byte("set ")) {
		t.Errorf("unexpected 'set ' MessageGame on missing-value reject; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_UnknownName_SilentReject(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar no_such_var 99")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[0] != 0 || p.varps[1] != 0 {
		t.Errorf("unknown-name setvar mutated varps: %v, want all 0", p.varps)
	}
	if len(emitted) != 0 {
		t.Errorf("unknown-name setvar emitted %d bytes; want silent reject", len(emitted))
	}
}

func TestHandleClientCheat_SetVar_ClampHigh(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only 2147483648") // INT32_MAX + 1
	_ = drainAfterTele(t, p, cc)

	if p.varps[0] != 0x7fffffff {
		t.Errorf("varps[0] after clamp-high setvar: got %d, want %d", p.varps[0], int32(0x7fffffff))
	}
}

func TestHandleClientCheat_SetVar_ClampLow(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "setvar transmit_only -2147483649") // INT32_MIN - 1
	_ = drainAfterTele(t, p, cc)

	if p.varps[0] != -0x80000000 {
		t.Errorf("varps[0] after clamp-low setvar: got %d, want %d", p.varps[0], int32(-0x80000000))
	}
}

func TestHandleClientCheat_SetVar_ProtectVarp_HappyPath_ClearsInteraction(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.modalState = modalStateMain
	p.waypointIndex = 5

	dispatchTeleCheat(t, p, "setvar protect_var 7")
	_ = drainAfterTele(t, p, cc)

	if p.modalState != modalStateNone {
		t.Errorf("modalState after protect-setvar: got %d, want modalStateNone", p.modalState)
	}
	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex after protect-setvar: got %d, want -1 (UnsetMapFlag)", p.waypointIndex)
	}
	if p.varps[1] != 7 {
		t.Errorf("varps[1] after protect-setvar: got %d, want 7", p.varps[1])
	}
}

func TestHandleClientCheat_SetVar_ProtectVarp_CanAccessFalse_MessagesAndRejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.delayed = true // forces CanAccess() = false

	dispatchTeleCheat(t, p, "setvar protect_var 99")
	emitted := drainAfterTele(t, p, cc)

	if p.varps[1] != 0 {
		t.Errorf("CanAccess=false should have rejected setvar: varps[1] = %d, want 0", p.varps[1])
	}
	if !bytes.Contains(emitted, []byte("Please finish what you are doing first.")) {
		t.Errorf("expected busy-message in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVarOther_HappyPath_SetsTargetVarpAndMessagesCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only 13")
	emitted := drainAfterTele(t, p, cc)

	if other.varps[0] != 13 {
		t.Errorf("other.varps[0] after setvarother: got %d, want 13", other.varps[0])
	}
	if !bytes.Contains(emitted, []byte("set transmit_only: to 13 on target")) {
		t.Errorf("expected 'set transmit_only: to 13 on target' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVarOther_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only 13")
	_ = drainAfterTele(t, p, cc)

	if other.varps[0] != 0 {
		t.Errorf("setvarother under NP=false mutated target: %d, want 0", other.varps[0])
	}
}

func TestHandleClientCheat_SetVarOther_MissingArgsRejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	dispatchTeleCheat(t, p, "setvarother target transmit_only") // 2 tokens, need 3
	_ = drainAfterTele(t, p, cc)

	if other.varps[0] != 0 {
		t.Errorf("len(args)<3 setvarother mutated target: %d, want 0", other.varps[0])
	}
}

func TestHandleClientCheat_SetVarOther_UnknownUser_MessagesCallerAndRejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "setvarother no_such_user transmit_only 5")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_SetVarOther_UnknownVarp_SilentReject(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	emittedBefore := drainAfterTele(t, p, cc)

	dispatchTeleCheat(t, p, "setvarother target no_such_var 5")
	emittedAfter := drainAfterTele(t, p, cc)
	emitted := append(emittedBefore, emittedAfter...)

	if bytes.Contains(emitted, []byte("set ")) {
		t.Errorf("unknown-varp setvarother emitted 'set '; want silent reject. bytes=%d", len(emitted))
	}
}

// TestHandleClientCheat_SetVarOther_BusyMessageGoesToCaller pins
// DEVIATION-NAI-185-D3: when the TARGET fails CanAccess, the
// "<arg0> is busy right now." message is sent to the CALLER, not the
// target. Mirrors TS L242.
func TestHandleClientCheat_SetVarOther_BusyMessageGoesToCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.delayed = true // target's CanAccess() = false

	dispatchTeleCheat(t, p, "setvarother target protect_var 99")
	emittedCaller := drainAfterTele(t, p, cc)

	if other.varps[1] != 0 {
		t.Errorf("busy-target setvarother mutated target: %d, want 0 (rejected)", other.varps[1])
	}
	if !bytes.Contains(emittedCaller, []byte("target is busy right now.")) {
		t.Errorf("expected 'target is busy right now.' on CALLER's conn; got %d bytes", len(emittedCaller))
	}
}

func TestHandleClientCheat_GetVar_HappyPath_MessagesValue(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)
	p.varps[0] = 42

	dispatchTeleCheat(t, p, "getvar transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("get transmit_only: 42")) {
		t.Errorf("expected 'get transmit_only: 42' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVar_MissingArg_Rejects(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "getvar")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("missing-arg getvar emitted 'get '; want silent reject. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVar_UnknownName_SilentReject(t *testing.T) {
	p, cc, _ := setvarTestFixture(t)

	dispatchTeleCheat(t, p, "getvar no_such_var")
	emitted := drainAfterTele(t, p, cc)

	if len(emitted) != 0 {
		t.Errorf("unknown-name getvar emitted %d bytes; want silent reject", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_HappyPath_MessagesValueOnTarget(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.varps[0] = 77

	dispatchTeleCheat(t, p, "getvarother target transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("get transmit_only: 77 on target")) {
		t.Errorf("expected 'get transmit_only: 77 on target' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))
	other.varps[0] = 77

	dispatchTeleCheat(t, p, "getvarother target transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("getvarother under NP=false emitted 'get '; want dead. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_MissingArgs_Rejects(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "getvarother target") // 1 token, need 2
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("len(args)<2 getvarother emitted 'get '; want reject. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_UnknownUser_MessagesCaller(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "getvarother no_such_user transmit_only")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GetVarOther_UnknownVarp_SilentReject(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "getvarother target no_such_var")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("get ")) {
		t.Errorf("unknown-varp getvarother emitted 'get '; want silent reject. bytes=%d", len(emitted))
	}
}

// giveotherFixtureCommon wires the shared inv/obj infra for the
// ::giveother test cohort. Returns (caller, callerConn, server, invID, objID).
// objID=1277, debugName="test_obj", non-stackable so each unit fills a slot.
//
// Note: does NOT start a discard goroutine on `cc` — exactly one test
// (UnknownUser_MessagesCaller) calls drainAfterTele(t, p, cc) and
// must own the read side. State-only tests don't emit on `cc`.
func giveotherFixtureCommon(t *testing.T) (*Player, net.Conn, *Server, int, int) {
	t.Helper()
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupNamedObj(t, s, 1277, "test_obj", /*stackable=*/ false)
	s.invTypes.Inv = invID
	return p, cc, s, invID, objID
}

func TestHandleClientCheat_GiveOther_HappyPath_AddsToTarget(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(other, invID) = nil")
	}
	if got := countSlots(inv, objID); got != 5 {
		t.Errorf("after giveother target test_obj 5: countSlots(target, 1277) = %d, want 5", got)
	}
}

func TestHandleClientCheat_GiveOther_NoOpWhenNotProduction(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = false
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv != nil {
		if got := countSlots(inv, objID); got != 0 {
			t.Errorf("giveother under NP=false: countSlots(target, 1277) = %d, want 0", got)
		}
	}
}

func TestHandleClientCheat_GiveOther_UnknownUser_MessagesCaller(t *testing.T) {
	p, cc, s, _, _ := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "giveother no_such_user test_obj 5")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("no_such_user is not logged in.")) {
		t.Errorf("expected 'no_such_user is not logged in.' in emitted bytes; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_GiveOther_UnknownItem_SilentReject(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target no_such_obj 5")

	inv := s.invLookup.Get(other, invID)
	if inv != nil {
		if got := countSlots(inv, objID); got != 0 {
			t.Errorf("unknown-item giveother added items: countSlots = %d, want 0", got)
		}
	}
}

func TestHandleClientCheat_GiveOther_MissingCountDefaultsToOne(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj")

	inv := s.invLookup.Get(other, invID)
	if got := countSlots(inv, objID); got != 1 {
		t.Errorf("after giveother target test_obj (no count): countSlots = %d, want 1", got)
	}
}

func TestHandleClientCheat_GiveOther_CountClampsToMin1(t *testing.T) {
	p, _, s, invID, objID := giveotherFixtureCommon(t)
	s.cfg.NodeProduction = true
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "giveother target test_obj 0") // 0 → clamps to 1

	inv := s.invLookup.Get(other, invID)
	if got := countSlots(inv, objID); got != 1 {
		t.Errorf("after giveother target test_obj 0: countSlots = %d, want 1 (count clamp)", got)
	}
}

// --- NAI-185 T7: ::givecrap admin cheat ---

// givecrapFixture seeds objTypes with a controlled pool that exercises
// every filter branch. Pool composition:
//   id=0  nil (filter must skip)
//   id=1  pass (members=false, dummy=0, cert=-1)
//   id=2  pass (members=false, dummy=0, cert=-1)
//   id=3  fail-members (members=true)
//   id=4  fail-dummy   (dummyitem=1)
//   id=5  fail-cert    (certtemplate=10)
//
// Non-stackable so each invocation occupies a fresh slot.
func givecrapFixture(t *testing.T, nodeMembers bool) (*Player, *Server, int) {
	t.Helper()
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeMembers = nodeMembers
	invID := mustSetupTestInv(t, s, 0, 28)
	s.invTypes.Inv = invID
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: []*objtype.ObjType{
			nil,
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "pass1"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 2, DebugName: "pass2"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 3, DebugName: "members"}, Members: true, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 4, DebugName: "dummy"}, DummyItem: 1, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 5, DebugName: "cert"}, CertTemplate: 10},
		},
	}
	return p, s, invID
}

func TestHandleClientCheat_GiveCrap_AddsTwentyEightFilteredItems(t *testing.T) {
	p, s, invID := givecrapFixture(t, false /* NodeMembers=false */)

	dispatchTeleCheat(t, p, "givecrap")

	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	// 28 non-stackable items → 28 occupied slots.
	occupied := 0
	for _, it := range inv.Items {
		if it == nil {
			continue
		}
		occupied++
		// With NodeMembers=false, only id=1 or id=2 should appear.
		if it.Id != 1 && it.Id != 2 {
			t.Errorf("givecrap (F2P) slot has filtered-out id=%d", it.Id)
		}
	}
	if occupied != 28 {
		t.Errorf("givecrap occupied slots = %d, want 28", occupied)
	}
}

func TestHandleClientCheat_GiveCrap_MembersWorld_NoCertOrDummy(t *testing.T) {
	p, s, invID := givecrapFixture(t, true /* NodeMembers=true */)

	dispatchTeleCheat(t, p, "givecrap")

	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	// Members items (id=3) become eligible; dummy (id=4) and cert (id=5)
	// stay filtered. Pin invariant: no dummy/cert slots.
	for _, it := range inv.Items {
		if it == nil {
			continue
		}
		if it.Id == 4 || it.Id == 5 {
			t.Errorf("givecrap (members world) has dummy/cert id=%d", it.Id)
		}
	}
}

func TestHandleClientCheat_GiveCrap_SmallPoolOnePassingItem_NoInfiniteLoop(t *testing.T) {
	// Pool with exactly one passing item among 5. The retry loop must
	// terminate within a 2s budget. A real infinite loop would hang
	// past the deadline and t.Fatal the run.
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeMembers = false
	invID := mustSetupTestInv(t, s, 0, 28)
	s.invTypes.Inv = invID
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: []*objtype.ObjType{
			{ConfigType: objtype.ConfigType{ID: 0, DebugName: "pass"}, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 1, DebugName: "members"}, Members: true, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 2, DebugName: "dummy"}, DummyItem: 1, CertTemplate: -1},
			{ConfigType: objtype.ConfigType{ID: 3, DebugName: "cert"}, CertTemplate: 10},
			{ConfigType: objtype.ConfigType{ID: 4, DebugName: "members2"}, Members: true, CertTemplate: -1},
		},
	}

	done := make(chan struct{})
	go func() {
		dispatchTeleCheat(t, p, "givecrap")
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("givecrap did not terminate within 2s on small-pool fixture")
	}
	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("invLookup.Get(p, invID) = nil")
	}
	occupied := 0
	for _, it := range inv.Items {
		if it != nil {
			occupied++
		}
	}
	if occupied != 28 {
		t.Errorf("givecrap small-pool: occupied = %d, want 28", occupied)
	}
}

// --- NAI-185 T8: ::broadcast admin cheat ---

func TestHandleClientCheat_Broadcast_FansOutToAllPlayers(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true
	addOtherTestPlayer(t, s, "second_user", 3220, 3220, 0)

	dispatchTeleCheat(t, p, "broadcast hello world")
	emitted := drainAfterTele(t, p, cc)

	if !bytes.Contains(emitted, []byte("hello world")) {
		t.Errorf("caller did not receive broadcast 'hello world'; got %d bytes", len(emitted))
	}
}

func TestHandleClientCheat_Broadcast_NoOpWhenNotProduction(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = false

	dispatchTeleCheat(t, p, "broadcast hello world")
	emitted := drainAfterTele(t, p, cc)

	if bytes.Contains(emitted, []byte("hello world")) {
		t.Errorf("broadcast under NP=false reached caller; want dead. bytes=%d", len(emitted))
	}
}

func TestHandleClientCheat_Broadcast_EmptyArgs_StillBroadcastsEmpty(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	s.cfg.NodeProduction = true

	dispatchTeleCheat(t, p, "broadcast")
	emitted := drainAfterTele(t, p, cc)

	// MESSAGE_GAME with empty body = framed opcode + 0x0a terminator.
	// At minimum, a non-zero byte count should land on the caller's conn.
	if len(emitted) == 0 {
		t.Errorf("::broadcast with empty args produced zero bytes; expected framed empty-MES")
	}
}

// --- NAI-185 T9: dispatch wiring smoke ---

// TestHandleClientCheat_DispatchesToNAI185Arms drives one representative
// arm of each NAI-185 shape (varp, cross-player varp, cross-player inv,
// fan-out broadcast) end-to-end through handleClientCheat.
// Pins:
//   - parts[0] dispatch reaches each arm
//   - existing staffModLevel >= 3 outer guard is honored
func TestHandleClientCheat_DispatchesToNAI185Arms(t *testing.T) {
	p, cc, s := setvarTestFixture(t)
	s.cfg.NodeProduction = true
	go io.Copy(io.Discard, cc)
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupNamedObj(t, s, 1277, "test_obj", false)
	s.invTypes.Inv = invID
	other := addOtherTestPlayer(t, s, "target", 3220, 3220, 0)
	other.varps = make([]int32, len(s.varpTypes.Configs))

	t.Run("setvar", func(t *testing.T) {
		dispatchTeleCheat(t, p, "setvar transmit_only 1")
		if p.varps[0] != 1 {
			t.Errorf("setvar dispatch failed: varps[0] = %d", p.varps[0])
		}
	})
	t.Run("setvarother", func(t *testing.T) {
		dispatchTeleCheat(t, p, "setvarother target transmit_only 2")
		if other.varps[0] != 2 {
			t.Errorf("setvarother dispatch failed: other.varps[0] = %d", other.varps[0])
		}
	})
	t.Run("giveother", func(t *testing.T) {
		dispatchTeleCheat(t, p, "giveother target test_obj 1")
		inv := s.invLookup.Get(other, invID)
		if inv == nil || countSlots(inv, objID) != 1 {
			t.Errorf("giveother dispatch failed: target missing test_obj")
		}
	})
	t.Run("broadcast_no_panic", func(t *testing.T) {
		// Wire smoke only — content assertions are in T8.
		dispatchTeleCheat(t, p, "broadcast smoke")
	})

	// givecrap covered by T7 dedicated tests; getvar/getvarother by T4/T5.
}

// TestHandleClientCheat_Locadd_SpawnsLoc pins TS L441-452. Resolves
// LocType by debugname, spawns a CENTREPIECE_STRAIGHT loc with
// angle=WEST=0, duration=500. Emits "Loc Added: <name> (ID: <id>)".
func TestHandleClientCheat_Locadd_SpawnsLoc(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	const locName = "test_dialogue_box"
	const locID = 42

	s.locTypes = &objtype.LocTypeConfigs{
		Configs: []*objtype.LocType{{
			ConfigType: objtype.ConfigType{ID: locID, DebugName: locName},
			Width:      1,
			Length:     1,
		}},
		ConfigNames: map[string]int{locName: 0},
	}
	s.zonesTracking = map[*zone.Zone]struct{}{}

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "locadd "+locName)
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	// Verify the loc was appended to z.Locs (z.AddLoc on a DESPAWN
	// lifecycle appends to z.Locs at pkg/zone/zone.go:164).
	z := s.zoneMap.Get(p.level, p.x, p.z)
	found := false
	for _, l := range z.Locs {
		if l.Type() == locID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Loc type=%d at (%d,%d,%d); zone had %d locs",
			locID, p.x, p.z, p.level, len(z.Locs))
	}

	wantMsg := []byte("Loc Added: " + locName + " (ID: " + fmt.Sprintf("%d", locID) + ")")
	if !bytes.Contains(all, wantMsg) {
		t.Errorf("missing MessageGame %q in emitted bytes (got %d bytes)", string(wantMsg), len(all))
	}
}

// TestHandleClientCheat_Locadd_UnknownName_NoOp pins the TS L448 nil
// guard: an unknown debugname → no spawn, no MessageGame.
func TestHandleClientCheat_Locadd_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 0)}

	dispatchTeleCheat(t, p, "locadd absent_name")

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("expected zero locs after unknown ::locadd; got %d", len(z.Locs))
	}
}

// TestHandleClientCheat_Locadd_EmptyArgs_NoOp pins TS L443-445 args.length<1.
func TestHandleClientCheat_Locadd_EmptyArgs_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 0)}

	dispatchTeleCheat(t, p, "locadd")

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("expected zero locs after empty-args ::locadd; got %d", len(z.Locs))
	}
}

// TestHandleClientCheat_Npcadd_SpawnsNpc pins TS L453-463. Resolves
// NpcType by debugname, constructs a DESPAWN npc at the player's
// tile with duration=500; nid allocated inside addNpc. TS has no
// MessageGame.
func TestHandleClientCheat_Npcadd_SpawnsNpc(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const npcName = "test_chicken"
	const npcID = 41

	// NPC registry needs s.npcTypes populated; teleTestPlayer leaves it nil.
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs:     make([]*objtype.NpcType, 100),
		ConfigNames: map[string]int{npcName: npcID},
	}
	s.npcTypes.Configs[npcID] = &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: npcID, DebugName: npcName},
		Size:         1,
		RespawnRate:  100,
		Timer:        0,
		RegenRate:    0,
		HuntMode:     -1,
		HuntRange:    0,
		BlockWalk:    objtype.BlockWalkNone,
		MoveRestrict: 0,
	}

	startNpcCount := len(s.npcLoop)
	dispatchTeleCheat(t, p, "npcadd "+npcName)

	if len(s.npcLoop) != startNpcCount+1 {
		t.Fatalf("after ::npcadd: npcLoop len = %d, want %d", len(s.npcLoop), startNpcCount+1)
	}
	added := s.npcLoop[len(s.npcLoop)-1]
	if added.typeId != npcID {
		t.Errorf("spawned npc.typeId = %d, want %d", added.typeId, npcID)
	}
	if added.x != p.x || added.z != p.z || added.level != p.level {
		t.Errorf("spawned npc coord = (%d,%d,%d), want (%d,%d,%d)",
			added.x, added.z, added.level, p.x, p.z, p.level)
	}
	if added.lifecycle != NpcLifecycleDespawn {
		t.Errorf("spawned npc.lifecycle = %d, want NpcLifecycleDespawn (%d)",
			added.lifecycle, NpcLifecycleDespawn)
	}
}

// TestHandleClientCheat_Npcadd_UnknownName_NoOp pins the TS L460 nil
// guard: an unknown debugname → no spawn.
func TestHandleClientCheat_Npcadd_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 0)}
	startNpcCount := len(s.npcLoop)

	dispatchTeleCheat(t, p, "npcadd absent_name")

	if len(s.npcLoop) != startNpcCount {
		t.Errorf("unknown ::npcadd should not change npcLoop; len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
}

// TestHandleClientCheat_Npcadd_EmptyArgs_NoOp pins TS L455-457
// args.length<1.
func TestHandleClientCheat_Npcadd_EmptyArgs_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 0)}
	startNpcCount := len(s.npcLoop)

	dispatchTeleCheat(t, p, "npcadd")

	if len(s.npcLoop) != startNpcCount {
		t.Errorf("empty-args ::npcadd should not change npcLoop; len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
}

// TestHandleClientCheat_Openmain_OpensMainModal pins TS L464-476.
// Resolves ComponentType by debugname; gate type.rootLayer === type.id
// passes only for root layers; routes through p.OpenMain which sets
// modalMain, clears modalChat/Side, sets modalState=modalStateMain,
// sets refreshModal=true.
func TestHandleClientCheat_Openmain_OpensMainModal(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const comName = "test_dialogue_root"
	const comID = 100

	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  comID, // root: rootLayer == id passes the gate
	}

	// Seed an open chat/side modal so we can verify OpenMain clears them.
	p.modalChat = 999
	p.modalSide = 888
	p.modalState = modalStateChat | modalStateSide
	p.refreshModal = false

	dispatchTeleCheat(t, p, "openmain "+comName)

	if p.modalMain != comID {
		t.Errorf("modalMain = %d, want %d", p.modalMain, comID)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat = %d, want -1 (cleared by OpenMain)", p.modalChat)
	}
	if p.modalSide != -1 {
		t.Errorf("modalSide = %d, want -1 (cleared by OpenMain)", p.modalSide)
	}
	if p.modalState != modalStateMain {
		t.Errorf("modalState = %d, want modalStateMain (%d)", p.modalState, modalStateMain)
	}
	if !p.refreshModal {
		t.Error("refreshModal = false, want true")
	}
}

// TestHandleClientCheat_Openmain_NonRoot_NoOp pins TS L472 rootLayer
// guard: a component whose rootLayer != id (i.e. a child layer) is
// rejected without opening any modal.
func TestHandleClientCheat_Openmain_NonRoot_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const comName = "test_child_layer"
	const comID = 101

	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  50, // child: rootLayer != id fails the gate
	}

	startMain := p.modalMain
	startState := p.modalState

	dispatchTeleCheat(t, p, "openmain "+comName)

	if p.modalMain != startMain {
		t.Errorf("non-root ::openmain mutated modalMain: %d → %d", startMain, p.modalMain)
	}
	if p.modalState != startState {
		t.Errorf("non-root ::openmain mutated modalState: %d → %d", startState, p.modalState)
	}
}

// TestHandleClientCheat_Openmain_UnknownName_NoOp pins TS L472 nil
// guard: an unknown debugname → no modal change.
func TestHandleClientCheat_Openmain_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.componentTypes = &objtype.ComponentTypeConfigs{Configs: make([]*objtype.ComponentType, 0)}
	startMain := p.modalMain

	dispatchTeleCheat(t, p, "openmain absent_name")

	if p.modalMain != startMain {
		t.Errorf("unknown ::openmain mutated modalMain: %d → %d", startMain, p.modalMain)
	}
}

// TestHandleClientCheat_AdminSpawn_StaffGateRejects pins the NAI-187
// admin-tier gate: at p.staffModLevel = 2 (mod tier), none of the
// three NAI-187 cheats fire. Mirrors the NAI-185 _Give_AdminGate
// pattern. Three sub-assertions, one fixture.
func TestHandleClientCheat_AdminSpawn_StaffGateRejects(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // below admin tier
	go io.Copy(io.Discard, cc)

	// Seed all three config tables with valid named entries so the
	// gate is the only thing that can reject. If the gate were absent,
	// each cheat would mutate world state with these fixtures.
	const locName = "gate_test_loc"
	const npcName = "gate_test_npc"
	const comName = "gate_test_com"
	const locID, npcID, comID = 42, 41, 100

	s.locTypes = &objtype.LocTypeConfigs{
		Configs: []*objtype.LocType{{
			ConfigType: objtype.ConfigType{ID: locID, DebugName: locName},
			Width:      1,
			Length:     1,
		}},
		ConfigNames: map[string]int{locName: 0},
	}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs:     make([]*objtype.NpcType, 100),
		ConfigNames: map[string]int{npcName: npcID},
	}
	s.npcTypes.Configs[npcID] = &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: npcID, DebugName: npcName},
		Size:        1,
		RespawnRate: 100,
		HuntMode:    -1,
		BlockWalk:   objtype.BlockWalkNone,
	}
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  comID,
	}

	startNpcCount := len(s.npcLoop)
	startModalMain := p.modalMain

	dispatchTeleCheat(t, p, "locadd "+locName)
	dispatchTeleCheat(t, p, "npcadd "+npcName)
	dispatchTeleCheat(t, p, "openmain "+comName)

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("staff<3 ::locadd should not spawn; zone had %d locs", len(z.Locs))
	}
	if len(s.npcLoop) != startNpcCount {
		t.Errorf("staff<3 ::npcadd should not spawn; npcLoop len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
	if p.modalMain != startModalMain {
		t.Errorf("staff<3 ::openmain should not open modal; modalMain = %d, want %d",
			p.modalMain, startModalMain)
	}
}

// stageDebugprocScript registers a [debugproc,<name>] runscript with the
// given ParamTypes onto s.scriptProvider. The script body is a no-op
// (single OpReturn) — debugproc tests assert against the marshaled
// intArgs/stringArgs, not against script execution side-effects.
//
// Replaces s.scriptProvider entirely (rather than appending to the default
// provider) so debugproc-specific fixtures don't compose with the
// catch-all [opnpc1,_default] script seeded by newTestServer's
// defaultTestProvider.
func stageDebugprocScript(t *testing.T, s *Server, name string, paramTypes []byte) *script.ScriptFile {
	t.Helper()
	if s.scriptProvider == nil || s.scriptProvider.Count() == 0 {
		s.scriptProvider = script.NewProvider()
	}
	sf := &script.ScriptFile{
		Name:             "[debugproc," + name + "]",
		LookupKey:        0xFFFFFFFF,
		ParamTypes:       paramTypes,
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)
	return sf
}

// --- NAI-189: dispatchDebugproc arg marshalling ---

func TestMarshalDebugprocArgs_String_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeString)})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "hello", "~test hello")
	if len(intArgs) != 0 {
		t.Errorf("intArgs len = %d, want 0", len(intArgs))
	}
	if len(stringArgs) != 1 || stringArgs[0] != "hello" {
		t.Errorf("stringArgs = %+v, want [\"hello\"]", stringArgs)
	}
}

func TestMarshalDebugprocArgs_String_Missing(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeString)})
	_, stringArgs := s.marshalDebugprocArgs(sf, "", "~test")
	if len(stringArgs) != 1 || stringArgs[0] != "" {
		t.Errorf("missing-arg stringArgs = %+v, want [\"\"]", stringArgs)
	}
}

func TestMarshalDebugprocArgs_Int_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "42", "~test 42")
	if len(intArgs) != 1 || intArgs[0] != 42 {
		t.Errorf("intArgs = %+v, want [42]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Int_NonNumeric(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "banana", "~test banana")
	if len(intArgs) != 1 || intArgs[0] != 0 {
		t.Errorf("non-numeric intArgs = %+v, want [0]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Int_Missing(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "", "~test")
	if len(intArgs) != 1 || intArgs[0] != 0 {
		t.Errorf("missing intArgs = %+v, want [0]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Obj_Hit(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 946, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "knife", "~test knife")
	if len(intArgs) != 1 || intArgs[0] != 946 {
		t.Errorf("Obj_Hit intArgs = %+v, want [946]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Obj_Miss(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{},
		ConfigNames: map[string]int{},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "unknown", "~test unknown")
	if len(intArgs) != 1 || intArgs[0] != -1 {
		t.Errorf("Obj_Miss intArgs = %+v, want [-1]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Namedobj_Hit(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 7, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeNamedObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "knife", "~test knife")
	if len(intArgs) != 1 || intArgs[0] != 7 {
		t.Errorf("Namedobj_Hit intArgs = %+v, want [7]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Npc_Hit(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs:     []*objtype.NpcType{{ConfigType: objtype.ConfigType{ID: 11, DebugName: "man"}}},
		ConfigNames: map[string]int{"man": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeNPC)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "man", "~test man")
	if len(intArgs) != 1 || intArgs[0] != 11 {
		t.Errorf("Npc_Hit intArgs = %+v, want [11]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Loc_Hit(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{
		Configs:     []*objtype.LocType{{ConfigType: objtype.ConfigType{ID: 33, DebugName: "table_basic"}}},
		ConfigNames: map[string]int{"table_basic": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeLoc)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "table_basic", "~test table_basic")
	if len(intArgs) != 1 || intArgs[0] != 33 {
		t.Errorf("Loc_Hit intArgs = %+v, want [33]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Seq_Hit(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = &objtype.SeqTypeConfigs{
		Configs:     []*objtype.SeqType{{ConfigType: objtype.ConfigType{ID: 13, DebugName: "human_walk"}}},
		ConfigNames: map[string]int{"human_walk": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeSeq)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "human_walk", "~test human_walk")
	if len(intArgs) != 1 || intArgs[0] != 13 {
		t.Errorf("Seq_Hit intArgs = %+v, want [13]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Stat_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeStat)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "attack", "~test attack")
	if len(intArgs) != 1 || intArgs[0] != objtype.PlayerStatAttack {
		t.Errorf("Stat_Hit intArgs = %+v, want [%d]", intArgs, objtype.PlayerStatAttack)
	}
}

func TestMarshalDebugprocArgs_Stat_Miss(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeStat)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "unknown", "~test unknown")
	if len(intArgs) != 1 || intArgs[0] != -1 {
		t.Errorf("Stat_Miss intArgs = %+v, want [-1]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Inv_Hit(t *testing.T) {
	s := newTestServer(t)
	s.invTypes = &objtype.InvTypeConfigs{
		Configs:     []*objtype.InvType{{ConfigType: objtype.ConfigType{ID: 93, DebugName: "inv"}}},
		ConfigNames: map[string]int{"inv": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInv)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "inv", "~test inv")
	if len(intArgs) != 1 || intArgs[0] != 93 {
		t.Errorf("Inv_Hit intArgs = %+v, want [93]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Interface_Hit(t *testing.T) {
	s := newTestServer(t)
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     []*objtype.ComponentType{{ConfigType: objtype.ConfigType{ID: 137, DebugName: "welcome_screen"}}},
		ConfigNames: map[string]int{"welcome_screen": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInterface)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "welcome_screen", "~test welcome_screen")
	if len(intArgs) != 1 || intArgs[0] != 137 {
		t.Errorf("Interface_Hit intArgs = %+v, want [137]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Spotanim_Hit(t *testing.T) {
	s := newTestServer(t)
	s.spotanimTypes = &objtype.SpotanimTypeConfigs{
		Configs:     []*objtype.SpotanimType{{ConfigType: objtype.ConfigType{ID: 70, DebugName: "air_strike"}}},
		ConfigNames: map[string]int{"air_strike": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeSpotanim)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "air_strike", "~test air_strike")
	if len(intArgs) != 1 || intArgs[0] != 70 {
		t.Errorf("Spotanim_Hit intArgs = %+v, want [70]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Idkit_Hit(t *testing.T) {
	s := newTestServer(t)
	s.idkTypes = &objtype.IdkTypeConfigs{
		Configs:     []*objtype.IdkType{{ConfigType: objtype.ConfigType{ID: 256, DebugName: "arms"}}},
		ConfigNames: map[string]int{"arms": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeIdkit)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "arms", "~test arms")
	if len(intArgs) != 1 || intArgs[0] != 256 {
		t.Errorf("Idkit_Hit intArgs = %+v, want [256]", intArgs)
	}
}

func TestMarshalDebugprocArgs_MultipleArgsMixed(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 946, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "mix", []byte{
		byte(objtype.ScriptVarTypeString),
		byte(objtype.ScriptVarTypeInt),
		byte(objtype.ScriptVarTypeObj),
	})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "hello 42 knife", "~mix hello 42 knife")
	if len(stringArgs) != 1 || stringArgs[0] != "hello" {
		t.Errorf("stringArgs = %+v, want [\"hello\"]", stringArgs)
	}
	if len(intArgs) != 2 || intArgs[0] != 42 || intArgs[1] != 946 {
		t.Errorf("intArgs = %+v, want [42, 946]", intArgs)
	}
}

func TestMarshalDebugprocArgs_EmptyParamTypes(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "noargs", []byte{})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "ignored", "~noargs ignored")
	if len(intArgs) != 0 {
		t.Errorf("intArgs = %+v, want []", intArgs)
	}
	if len(stringArgs) != 0 {
		t.Errorf("stringArgs = %+v, want []", stringArgs)
	}
}

func TestMarshalDebugprocArgs_Coord_OneToken(t *testing.T) {
	// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE: TS's slice(6) on
	// args2[0] produces an empty/non-digit string for all reasonable
	// debugproc names, making level coerce to -1 (Go) / NaN (TS). The
	// (mx<<6)+lx components parse correctly. Pin the result verbatim.
	//
	// rawCheat "~coord_0_50_50_32_32" splits on '_' as:
	//   parts[0]="~coord" (len=6, slice(6)="" → level=-1)
	//   parts[1]="0"  → mx=0
	//   parts[2]="50" → mz=50
	//   parts[3]="50" → lx=50
	//   parts[4]="32" → lz=32
	// x=(0<<6)+50=50, z=(50<<6)+32=3232 → PackCoord(-1,50,3232).
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "coord_0_50_50_32_32", []byte{byte(objtype.ScriptVarTypeCoord)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "", "~coord_0_50_50_32_32")
	wantLevel := -1
	wantX, wantZ := 50, (50<<6)+32
	want := coordgrid.PackCoord(wantLevel, wantX, wantZ)
	if len(intArgs) != 1 || intArgs[0] != want {
		t.Errorf("OneToken intArgs = %+v, want [%d] (PackCoord(%d,%d,%d))",
			intArgs, want, wantLevel, wantX, wantZ)
	}
}

func TestMarshalDebugprocArgs_Coord_TwoToken(t *testing.T) {
	// Same DEVIATION as OneToken; args2[0] is now "~setpos coord" (13 chars).
	// slice(6) = "s coord"; parseInt → NaN (TS) / Atoi → err → -1 (goscape).
	//
	// rawCheat "~setpos coord_0_50_50_32_32" splits on '_' as:
	//   parts[0]="~setpos coord" (slice(6)="s coord" → level=-1)
	//   parts[1]="0"  → mx=0
	//   parts[2]="50" → mz=50
	//   parts[3]="50" → lx=50
	//   parts[4]="32" → lz=32
	// x=(0<<6)+50=50, z=(50<<6)+32=3232 → PackCoord(-1,50,3232).
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "setpos", []byte{byte(objtype.ScriptVarTypeCoord)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "coord_0_50_50_32_32", "~setpos coord_0_50_50_32_32")
	wantLevel := -1
	wantX, wantZ := 50, (50<<6)+32
	want := coordgrid.PackCoord(wantLevel, wantX, wantZ)
	if len(intArgs) != 1 || intArgs[0] != want {
		t.Errorf("TwoToken intArgs = %+v, want [%d] (PackCoord(%d,%d,%d))",
			intArgs, want, wantLevel, wantX, wantZ)
	}
}
