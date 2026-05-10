package world

import (
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestProcessIn_DecodedThisTickResetAtStart pins T1a: at the top of
// processIn, decodedThisTick is reset to false BEFORE the decode loop
// runs. Sentinel: pre-set to true; a no-read processIn must reset.
func TestProcessIn_DecodedThisTickResetAtStart(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.decodedThisTick = true // poison from prior tick
	// No bytes in c.in → decode loop reads zero packets.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false after no-read processIn (must reset before decode loop)")
	}
}

// TestProcessIn_DecodedThisTickStaysFalseOnNoRead pins T1c: after a
// processIn tick that read zero packets, decodedThisTick is false.
// Equivalent intent to TS decodeIn() returning false.
func TestProcessIn_DecodedThisTickStaysFalseOnNoRead(t *testing.T) {
	p, _ := newTestPlayer(t)
	// No bytes in c.in.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false on no-read tick")
	}
}

// TestProcessIn_DecodedThisTickSetAfterRead pins T1b: after processIn
// reads ≥1 packet, decodedThisTick is true. Uses NO_TIMEOUT (op 108,
// 0-payload) — same pattern as TestReadPacketNoTimeoutConsumesAndResetsOpcode.
func TestProcessIn_DecodedThisTickSetAfterRead(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// Op 108 = NO_TIMEOUT, payload 0.
	p.client.in.Write([]byte{encryptOpcode(enc, 108)})

	p.processIn(0)

	if !p.decodedThisTick {
		t.Error("decodedThisTick: want true after reading ≥1 packet")
	}
}

// TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket pins T2: the
// new helper bundles clearWaypoints (waypointIndex=-1) + the
// OpUnsetMapFlag wire write. Mirrors TS Player.unsetMapFlag
// (Player.ts:2169-2172). Sibling pattern to
// TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket.
func TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.waypointIndex = 5

	// Sibling decoder seeded from same key for opcode comparison.
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	// Start drain BEFORE the action; drainConn requires this ordering.
	received := drainConn(t, cc)
	p.unsetMapFlag()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (clearWaypoints arm of bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
}

// newPostDecodeTestPlayerWithConn wires a Player with the minimum
// scaffolding required to drive (p *Player) processPostDecode
// end-to-end:
//   - p.client.server (with default cfg: PLAYERPACKET, routefinder=true)
//   - p.decodedThisTick = true       (outer gate satisfied)
//   - p.userPath set to one packed coord (outer gate satisfied)
//   - p.faceEntity = -1              (faceEntity branch no-op by default)
//   - p.moveClickRequest = false     (sentinel)
//
// Returns (player, server, conn). Most branch tests don't need the
// conn (use the wrapper newPostDecodeTestPlayer). Wire-asserting
// branches (T3b delayed) drain the conn directly.
func newPostDecodeTestPlayerWithConn(t *testing.T) (*Player, *Server, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	s := &Server{
		log: discardLogger(),
		cfg: Config{
			NodeWalktriggerSetting: WalkTriggerSettingPlayerpacket,
			NodeClientRoutefinder:  true,
		},
	}
	p.client.server = s
	p.decodedThisTick = true
	p.userPath = []int{0x12345}
	p.faceEntity = -1
	p.moveClickRequest = false
	return p, s, cc
}

// newPostDecodeTestPlayer is a wrapper that discards the conn for
// branch tests that don't drain wire output.
func newPostDecodeTestPlayer(t *testing.T) (*Player, *Server) {
	t.Helper()
	p, s, _ := newPostDecodeTestPlayerWithConn(t)
	return p, s
}

// TestProcessPostDecode_OuterGateSkipsWhenNotDecoded pins TS L611
// `decodeIn()` short-circuit. With decodedThisTick=false the entire
// block is skipped — moveClickRequest is NOT set even when userPath
// AND opcalled would otherwise satisfy L613.
func TestProcessPostDecode_OuterGateSkipsWhenNotDecoded(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.decodedThisTick = false
	p.opcalled = true // would otherwise satisfy L613

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !decodedThisTick)")
	}
}

// TestProcessPostDecode_OuterGateSkipsWhenIdle pins TS L613 outer
// gate. With userPath empty AND !opcalled, the block returns early.
// moveClickRequest stays at its sentinel.
func TestProcessPostDecode_OuterGateSkipsWhenIdle(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.userPath = nil
	p.opcalled = false

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !userPath && !opcalled)")
	}
}

// TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns pins TS
// L614-617: when delayed AND outer gate satisfied (userPath set),
// unsetMapFlag fires (waypointIndex=-1 + OpUnsetMapFlag) and the
// block returns BEFORE the faceEntity reset / moveClickRequest setter.
//
// newPostDecodeTestPlayer (defined below) returns the conn alongside
// the player so this test can drainConn the wire output.
func TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns(t *testing.T) {
	p, _, cc := newPostDecodeTestPlayerWithConn(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	p.delayed = true
	p.waypointIndex = 5    // would clear under unsetMapFlag bundle
	p.faceEntity = 42      // would reset if delayed branch DIDN'T return
	p.moveClickRequest = true // sentinel — must NOT be touched after return

	received := drainConn(t, cc)
	p.processPostDecode()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (delayed → unsetMapFlag bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (delayed branch must return BEFORE faceEntity reset)", p.faceEntity)
	}
	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (sentinel preserved — delayed branch must return BEFORE setter)")
	}
}
