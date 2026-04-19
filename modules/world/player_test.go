package world

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func newTestPlayer(t *testing.T) (*Player, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	c := newClient(serverConn, time.Second, discardLogger())
	t.Cleanup(func() { c.in.Release() })
	c.state = ClientStateGame
	p := newPlayer(c)
	c.player = p
	return p, clientConn
}

func TestReadPacketEmptyBufferReturnsFalse(t *testing.T) {
	_, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for empty buffer")
	}
	if opcode != -1 {
		t.Errorf("opcode: got %d, want -1", opcode)
	}
}

func TestReadPacketUnknownOpcodeReturnsErrCloseConn(t *testing.T) {
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// Opcode 0 is not registered in Ops.
	p.client.in.Write([]byte{encryptOpcode(enc, 0)})

	_, _, err := p.readPacket()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("unknown opcode: got %v, want errCloseConn", err)
	}
}

func TestReadPacketNoTimeoutConsumesAndResetsOpcode(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// NO_TIMEOUT: opcode 108, payload size 0
	p.client.in.Write([]byte{encryptOpcode(enc, 108)})

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 108 {
		t.Errorf("opcode: got %d, want 108", opcode)
	}
	if p.client.opcode != -1 {
		t.Errorf("client.opcode after dispatch: got %d, want -1", p.client.opcode)
	}
}

func TestReadPacketMoveGameClickFullPacket(t *testing.T) {
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// MOVE_GAMECLICK: opcode 181, 1-byte length prefix
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes
	payload := []byte{0, 0x0C, 0xA4, 0x0C, 0x8B}
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 181))
	buf = append(buf, byte(len(payload)))
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 181 {
		t.Errorf("opcode: got %d, want 181", opcode)
	}
	if p.client.opcode != -1 {
		t.Errorf("client.opcode after dispatch: got %d, want -1", p.client.opcode)
	}
}

func TestReadPacketPartialPayloadReturnsFalse(t *testing.T) {
	enc, dec := isaacPair([4]uint32{55, 66, 77, 88})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// MOVE_GAMECLICK claiming 10 payload bytes, only 3 arrive
	buf := []byte{encryptOpcode(enc, 181), 10, 0x01, 0x02, 0x03}
	p.client.in.Write(buf)

	_, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for partial payload")
	}
	// cursor must be preserved
	if p.client.opcode != 181 {
		t.Errorf("client.opcode preserved: got %d, want 181", p.client.opcode)
	}
	if p.client.waiting != 10 {
		t.Errorf("client.waiting preserved: got %d, want 10", p.client.waiting)
	}
}

func TestReadPacketEventTrackingTwoByteLenPrefix(t *testing.T) {
	enc, dec := isaacPair([4]uint32{99, 88, 77, 66})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING: opcode 81, -2 (2-byte length prefix)
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 81))
	buf = append(buf, 0x00, byte(len(payload))) // 2-byte big-endian length
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 81 {
		t.Errorf("opcode: got %d, want 81", opcode)
	}
}

func TestReadPacketOversizedTwoByteLenClosesConn(t *testing.T) {
	enc, dec := isaacPair([4]uint32{1, 1, 1, 1})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING with 2-byte length > 1600
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 81))
	buf = append(buf, 0x07, 0x00) // 0x0700 = 1792 > 1600
	p.client.in.Write(buf)

	_, _, err := p.readPacket()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("oversized packet: got %v, want errCloseConn", err)
	}
}

// Ensure io is used (io.ReadFull is used in later tasks in this file).
var _ = io.Discard
