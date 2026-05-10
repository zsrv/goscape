package world

import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
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

// TestProcessPostDecode_FaceEntityResetForLocTarget pins TS L619-622
// for *Loc target: faceEntity reset to -1, masks |= entitymask.
func TestProcessPostDecode_FaceEntityResetForLocTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{} // any *Loc satisfies the type-switch
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true // satisfies outer L613 gate

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForObjTarget pins same for *Obj.
func TestProcessPostDecode_FaceEntityResetForObjTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Obj{}
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Obj target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForNilTarget pins TS L619 nil
// target arm: nil target + faceEntity!=-1 → reset.
func TestProcessPostDecode_FaceEntityResetForNilTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = nil
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (nil target → reset)", p.faceEntity)
	}
}

// TestProcessPostDecode_FaceEntityPreservedForPlayerTarget pins the
// negative arm: PathingEntity targets (Player/Npc) do NOT trigger
// the faceEntity reset.
func TestProcessPostDecode_FaceEntityPreservedForPlayerTarget(t *testing.T) {
	s := newTestServer(t)
	other := newTestPlayerAt(t, s, 2, 3200, 3200, 0)
	p, _ := newPostDecodeTestPlayer(t)
	p.target = other
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (Player target → preserved)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (Player target → masks NOT touched)", p.masks)
	}
}

// TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne pins TS L620
// guard: when faceEntity is already -1, masks is NOT touched.
func TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{}
	p.faceEntity = -1 // guard: already cleared
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (no-op preserves)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (faceEntity already -1 → masks NOT touched)", p.masks)
	}
}

// TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled pins TS L624-625
// branch: !busy() && opcalled → moveClickRequest = false.
func TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = true // sentinel — must flip to false
	p.targetOp = -1           // disable pathToTarget branch (-1 ≠ 3, but opcalled=true → would fire)
	// Need to also block the pathToTarget branch from firing first:
	// !followingPlayer && opcalled && (len(userPath)==0 || !routefinder).
	// With routefinder=true (default) AND len(userPath)>0, the gate fails →
	// pathToTarget skipped. userPath is set in the fixture; routefinder=true default.

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (!Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_BusyOpcalled pins TS L626-627
// branch: Busy + opcalled → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_BusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.modalState = modalStateMain // Busy() = true
	p.delayed = false
	p.moveClickRequest = false // sentinel — must flip to true

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled pins TS
// L626-627 else-branch: !Busy + !opcalled + userPath set → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = false // sentinel

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (!Busy + !opcalled + userPath set)")
	}
}
