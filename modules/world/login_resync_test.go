package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdatePid_EmitsExactByteSequence pins the wire bytes of sendUpdatePid. NAI-182 B1.
func TestSendUpdatePid_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
		0x12, 0x34,
	}

	received := drainConn(t, cc)
	p.slot = 0x1234
	sendUpdatePid(p, p.slot)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetClientVarCache_EmitsOpcodeOnly pins the wire bytes of sendResetClientVarCache. NAI-182 B1.
func TestSendResetClientVarCache_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetClientVarCache.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetClientVarCache(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetAnims_EmitsOpcodeOnly pins the wire bytes of sendResetAnims. NAI-182 B1.
func TestSendResetAnims_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetAnims.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetAnims(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestProcessLogins_FreshLogin_EmitsOpcodeOrder pins UPDATE_PID →
// RESET_CLIENT_VARCACHE → RESET_ANIMS emit order on fresh login.
// Verifies the sequence wired into processLogins at NAI-182 B3.
func TestProcessLogins_FreshLogin_EmitsOpcodeOrder(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Walk the byte stream: each packet is an encrypted opcode byte followed
	// by its fixed-length payload. Verify opcodes in order.
	// addPlayer assigns slot 1 (first available slot), so UPDATE_PID carries 0x0001.
	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
	}
	// UPDATE_PID payload: 2 bytes (slot assigned by addPlayer — first free slot = 1).
	want = append(want, 0x00, byte(p.slot))
	want = append(want,
		byte((int(gameserver.OpResetClientVarCache.Opcode)+int(enc.GetNext()))&0xff),
	)
	// RESET_CLIENT_VARCACHE: 0-byte payload
	want = append(want,
		byte((int(gameserver.OpResetAnims.Opcode)+int(enc.GetNext()))&0xff),
	)
	// RESET_ANIMS: 0-byte payload

	if len(got) < len(want) {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), len(want))
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer verifies
// that UPDATE_REBOOT_TIMER is emitted after the 3 standard fresh-login
// packets when s.shutdownTick != -1. NAI-182 B3.
func TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.shutdownTick = s.currentTick + 25

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Consume the first 3 packets: UPDATE_PID (3 bytes), RESET_CLIENT_VARCACHE
	// (1 byte), RESET_ANIMS (1 byte) = 5 bytes total.
	enc.GetNext() // UPDATE_PID opcode key
	enc.GetNext() // RESET_CLIENT_VARCACHE opcode key
	enc.GetNext() // RESET_ANIMS opcode key
	offset := 3 + 1 + 1

	// 4th packet: UPDATE_REBOOT_TIMER opcode + 2-byte payload (25 == 0x0019).
	wantOpcode := byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff)
	wantPayload := []byte{0x00, 0x19}

	if len(got) < offset+3 {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), offset+3)
	}
	if got[offset] != wantOpcode {
		t.Errorf("UPDATE_REBOOT_TIMER opcode byte: got 0x%02x, want 0x%02x", got[offset], wantOpcode)
	}
	if got[offset+1] != wantPayload[0] || got[offset+2] != wantPayload[1] {
		t.Errorf("UPDATE_REBOOT_TIMER payload: got [0x%02x 0x%02x], want [0x%02x 0x%02x]",
			got[offset+1], got[offset+2], wantPayload[0], wantPayload[1])
	}
}

// TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer asserts that no
// UPDATE_REBOOT_TIMER opcode appears in the stream when shutdownTick == -1.
// NAI-182 B3.
func TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// shutdownTick defaults to -1 in newTestServer; leave it unchanged.

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Compute the expected UPDATE_REBOOT_TIMER encrypted opcode if it were
	// sent. We consume 3 ISAAC values for the 3 standard packets first.
	enc.GetNext() // UPDATE_PID
	enc.GetNext() // RESET_CLIENT_VARCACHE
	enc.GetNext() // RESET_ANIMS
	forbiddenOpcode := byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff)

	// The stream should be exactly 5 bytes (UPDATE_PID=3, RCV=1, RA=1).
	// Anything beyond that, including a byte matching the reboot opcode, is a bug.
	const wantLen = 5
	if len(got) != wantLen {
		t.Errorf("stream length: got %d, want %d (no reboot timer packet)", len(got), wantLen)
	}
	// Additionally verify no byte in the observed stream matches the forbidden opcode.
	for i, b := range got {
		if b == forbiddenOpcode {
			t.Errorf("byte[%d] matches UPDATE_REBOOT_TIMER encrypted opcode 0x%02x — should not be present",
				i, forbiddenOpcode)
		}
	}
}
