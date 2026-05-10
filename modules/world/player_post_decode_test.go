package world

import (
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
