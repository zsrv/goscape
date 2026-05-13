package world

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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
	p.client.encryptor = enc
	s := newTestServer(t)
	p.client.server = s

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

func TestProcessInIncrementsPlaytime(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.playtime = 0
	p.processIn(0)
	if p.playtime != 1 {
		t.Errorf("playtime: got %d, want 1", p.playtime)
	}
}

func TestProcessInUserEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// CLOSE_MODAL: opcode 231, USER_EVENT, 0-byte payload — just the opcode byte
	var buf []byte
	for range 6 {
		buf = append(buf, encryptOpcode(enc, 231))
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.userLimit != 5 {
		t.Errorf("userLimit: got %d, want 5", p.userLimit)
	}
	// 6th packet remains in the buffer
	if p.client.in.Len() != 1 {
		t.Errorf("remaining bytes: got %d, want 1", p.client.in.Len())
	}
}

func TestProcessInClientEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// NO_TIMEOUT: opcode 108, CLIENT_EVENT, 0-byte payload
	var buf []byte
	for range 21 {
		buf = append(buf, encryptOpcode(enc, 108))
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.clientLimit != 20 {
		t.Errorf("clientLimit: got %d, want 20", p.clientLimit)
	}
	if p.client.in.Len() != 1 {
		t.Errorf("remaining bytes: got %d, want 1", p.client.in.Len())
	}
}

func TestProcessInRestrictedEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{55, 44, 33, 22})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING: opcode 81, RESTRICTED_EVENT, -2 (2-byte length prefix), 0 payload bytes
	var buf []byte
	for range 3 {
		buf = append(buf, encryptOpcode(enc, 81))
		buf = append(buf, 0x00, 0x00) // 2-byte length = 0
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.restrictedLimit != 2 {
		t.Errorf("restrictedLimit: got %d, want 2", p.restrictedLimit)
	}
	// 3rd packet (3 bytes) remains
	if p.client.in.Len() != 3 {
		t.Errorf("remaining bytes: got %d, want 3", p.client.in.Len())
	}
}

func TestProcessInSkipsDisconnectedClient(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.state = ClientStateClosed

	p.processIn(0)

	if p.playtime != 1 {
		t.Errorf("playtime: got %d, want 1", p.playtime)
	}
	if p.userLimit != 0 {
		t.Errorf("userLimit: got %d, want 0", p.userLimit)
	}
}

func TestEncodeOutNoopWhenModalUnchanged(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	p.encodeOut()
	p.client.flushWrite()

	clientConn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	buf := make([]byte, 16)
	n, _ := clientConn.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes from encodeOut no-op, got %d", n)
	}
}

func TestEncodeOutSendsIfCloseOnRefreshModalClose(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModalClose = true

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1) // IF_CLOSE has no payload
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpIfClose.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("IF_CLOSE encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if p.refreshModalClose {
			t.Error("refreshModalClose should be false after encodeOut")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_CLOSE")
	}
}

func TestEncodeOutSendsIfOpenMain(t *testing.T) {
	enc, _ := isaacPair([4]uint32{5, 6, 7, 8})
	wantEnc, _ := isaacPair([4]uint32{5, 6, 7, 8})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModal = true
	p.modalState = modalStateMain
	p.modalMain = 1234

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3) // 1 encrypted opcode + 2 payload bytes
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpIfOpenMain.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("IF_OPENMAIN encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		component := int(got[1])<<8 | int(got[2])
		if component != 1234 {
			t.Errorf("IF_OPENMAIN component: got %d, want 1234", component)
		}
		if p.refreshModal {
			t.Error("refreshModal should be false after encodeOut")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_OPENMAIN")
	}
}

func TestWriteOutFixedSize(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 42, PayloadSize: 2}
	payload := []byte{0xAB, 0xCD}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		if got[1] != 0xAB || got[2] != 0xCD {
			t.Errorf("payload: got [%d %d], want [0xAB 0xCD]", got[1], got[2])
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for writeOut bytes")
	}
}

func TestWriteOutOneByteLenPrefix(t *testing.T) {
	enc, _ := isaacPair([4]uint32{2, 3, 4, 5})
	wantEnc, _ := isaacPair([4]uint32{2, 3, 4, 5})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 50, PayloadSize: -1}
	payload := []byte{0x01, 0x02, 0x03}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 5) // 1 opcode + 1 len-prefix + 3 payload
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		if got[1] != byte(len(payload)) {
			t.Errorf("length prefix: got %d, want %d", got[1], len(payload))
		}
		if got[2] != 0x01 || got[3] != 0x02 || got[4] != 0x03 {
			t.Errorf("payload: got %v, want [1 2 3]", got[2:])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}

func TestWriteOutTwoByteLenPrefix(t *testing.T) {
	enc, _ := isaacPair([4]uint32{3, 4, 5, 6})
	wantEnc, _ := isaacPair([4]uint32{3, 4, 5, 6})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 60, PayloadSize: -2}
	payload := []byte{0xDE, 0xAD}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 5) // 1 opcode + 2 len-prefix + 2 payload
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		if got[1] != 0x00 || got[2] != 0x02 {
			t.Errorf("length prefix: got [%d %d], want [0 2]", got[1], got[2])
		}
		if got[3] != 0xDE || got[4] != 0xAD {
			t.Errorf("payload: got [%d %d], want [0xDE 0xAD]", got[3], got[4])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}

func TestNewPlayerCopiesLowMemoryFromClient(t *testing.T) {
	// lowMemory=true case
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	c.lowMemory = true
	p := newPlayer(c)
	if !p.lowMemory {
		t.Errorf("lowMemory=true on client: want p.lowMemory=true, got false")
	}

	// lowMemory=false (default) case
	serverConn2, clientConn2 := net.Pipe()
	defer serverConn2.Close()
	defer clientConn2.Close()
	c2 := newClient(serverConn2, time.Second, discardLogger())
	defer c2.in.Release()
	c2.state = ClientStateGame
	// c2.lowMemory defaults to false
	p2 := newPlayer(c2)
	if p2.lowMemory {
		t.Errorf("lowMemory=false on client: want p.lowMemory=false, got true")
	}
}

func TestNewPlayerCopiesReconnectingFromClient(t *testing.T) {
	// reconnecting=true case
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	c.reconnecting = true
	p := newPlayer(c)
	if !p.reconnecting {
		t.Errorf("reconnecting=true on client: want p.reconnecting=true, got false")
	}

	// reconnecting=false (default) case
	serverConn2, clientConn2 := net.Pipe()
	defer serverConn2.Close()
	defer clientConn2.Close()
	c2 := newClient(serverConn2, time.Second, discardLogger())
	defer c2.in.Release()
	c2.state = ClientStateGame
	// c2.reconnecting defaults to false
	p2 := newPlayer(c2)
	if p2.reconnecting {
		t.Errorf("reconnecting=false on client: want p.reconnecting=false, got true")
	}
}

func TestNewPlayer_OrientationXZ_DefaultMinusOne(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	p := newPlayer(c)
	if p.OrientationX != -1 {
		t.Errorf("OrientationX default: got %d, want -1", p.OrientationX)
	}
	if p.OrientationZ != -1 {
		t.Errorf("OrientationZ default: got %d, want -1", p.OrientationZ)
	}
}

func TestNewPlayer_LastAppearance_DefaultMinusOne(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	p := newPlayer(c)
	if p.lastAppearance != -1 {
		t.Errorf("lastAppearance default: got %d, want -1", p.lastAppearance)
	}
}

// TestNewPlayer_WalkTrigger_DefaultMinusOne pins the NAI-51 default for the
// new walktrigger field. -1 sentinel = "no script queued"; default 0 would
// silently fire script id 0 on every walktrigger consumer entry.
func TestNewPlayer_WalkTrigger_DefaultMinusOne(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	p := newPlayer(c)
	if p.walktrigger != -1 {
		t.Errorf("walktrigger default: got %d, want -1", p.walktrigger)
	}
}

func TestPlayerIsValid(t *testing.T) {
	base := func() *Player {
		return &Player{
			active:     true,
			visibility: rsbuf.VisibilityDefault,
			// loggingOut defaults false
		}
	}

	if !base().IsValid() {
		t.Error("active + default-visibility + not-logging-out: IsValid = false, want true")
	}

	p := base()
	p.loggingOut = true
	if p.IsValid() {
		t.Error("loggingOut=true: IsValid = true, want false")
	}

	p = base()
	p.visibility = rsbuf.VisibilityHard
	if p.IsValid() {
		t.Error("non-default visibility: IsValid = true, want false")
	}

	p = base()
	p.active = false
	if p.IsValid() {
		t.Error("active=false: IsValid = true, want false")
	}
}

func TestPlayerBusyNotDelayedNoModal(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.Busy() {
		t.Error("Busy: got true, want false (fresh player has neither delayed nor modal)")
	}
}

func TestPlayerBusyDelayedOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	if !p.Busy() {
		t.Error("Busy: got false, want true (delayed=true)")
	}
}

func TestPlayerBusyModalMainOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	if !p.Busy() {
		t.Error("Busy: got false, want true (modalStateMain set)")
	}
}

func TestPlayerBusyModalChatOnly(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	if !p.Busy() {
		t.Error("Busy: got false, want true (modalStateChat set)")
	}
}

func TestPlayerBusyModalSideOnlyNotBusy(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateSide
	if p.Busy() {
		t.Error("Busy: got true, want false (modalStateSide alone — TS containsModalInterface excludes side per Player.ts:796-799)")
	}
}

func TestPlayerBusyDelayedAndModalCombined(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	p.modalState = modalStateMain
	if !p.Busy() {
		t.Error("Busy: got false, want true (both delayed and modal)")
	}
}

func TestPlayerIsInWildernessSouthRectInside(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 5000
	if !p.IsInWilderness() {
		t.Error("IsInWilderness: got false, want true (3000,5000 inside south rect)")
	}
}

func TestPlayerIsInWildernessNorthRectInside(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 11000
	if !p.IsInWilderness() {
		t.Error("IsInWilderness: got false, want true (3000,11000 inside north rect)")
	}
}

func TestPlayerIsInWildernessOutsideAllRects(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z = 3000, 3500
	if p.IsInWilderness() {
		t.Error("IsInWilderness: got true, want false (3000,3500 outside south rect)")
	}
}

// TestPlayerIsInWildernessBoundaries pins TS Player.ts:2082-2090 boundary
// asymmetry: lower-edge inclusive (>=), upper-edge exclusive (<). A
// future "fix" to <= would flip these red.
func TestPlayerIsInWildernessBoundaries(t *testing.T) {
	cases := []struct {
		name string
		x, z int
		want bool
	}{
		{"south_lower_corner_inclusive", 2944, 3520, true},
		{"south_just_below_x_lower", 2943, 5000, false},
		{"south_upper_x_exclusive", 3392, 5000, false},
		{"south_upper_z_exclusive", 3000, 6400, false},
		{"north_lower_z_inclusive", 3000, 9920, true},
		{"north_upper_z_exclusive", 3000, 12800, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newTestPlayer(t)
			p.x, p.z = tc.x, tc.z
			got := p.IsInWilderness()
			if got != tc.want {
				t.Errorf("IsInWilderness(%d,%d): got %v, want %v", tc.x, tc.z, got, tc.want)
			}
		})
	}
}

// TestPlayer_ProtectedScriptActive_TruthTable pins the goscape mapping
// of TS Player.protect: protectedScriptActive iff activeScript != nil
// AND activeScript.Pointers&PtrProtectedActivePlayer != 0. Mirrors the
// convergence documented at CanAccess (player_script.go:232-238).
func TestPlayer_ProtectedScriptActive_TruthTable(t *testing.T) {
	cases := []struct {
		name   string
		active *script.ScriptState
		want   bool
	}{
		{"nil-active", nil, false},
		{"active-unprotected", &script.ScriptState{}, false},
		{"active-protected", &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{activeScript: tc.active}
			if got := p.protectedScriptActive(); got != tc.want {
				t.Errorf("protectedScriptActive: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProcessInCallsInputTrackingOnCycle pins that the last line of
// Player.processIn dispatches to InputTracking.OnCycle (TS World.ts:646
// placement parity).
func TestProcessInCallsInputTrackingOnCycle(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.input = &InputTracking{player: p}
	// Position the window so OnCycle should fire enable() this tick.
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.input.enabled = false

	p.processIn(0)

	if !p.input.enabled {
		t.Error("input.enabled: must be true after processIn → OnCycle → enable()")
	}
}

func TestEncodeOutSendsTutOpen(t *testing.T) {
	enc, _ := isaacPair([4]uint32{9, 10, 11, 12})
	wantEnc, _ := isaacPair([4]uint32{9, 10, 11, 12})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// NAI-112 Stage 2.2: OpenTutorial writes TUT_OPEN directly (no encodeOut
	// deferred path). Start receiver before calling OpenTutorial.
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3) // 1 encrypted opcode + 2 payload bytes
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.OpenTutorial(42)
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_OPEN encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		component := int(got[1])<<8 | int(got[2])
		if component != 42 {
			t.Errorf("TUT_OPEN component: got %d, want 42", component)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN")
	}
}

// TestNAI_112_OpenTutorialUnconditionalReEmit pins that OpenTutorial
// writes TUT_OPEN UNCONDITIONALLY on every call — even when com matches
// the prior modalTutorial. Mirrors TS Player.openTutorial at
// Engine-TS/src/engine/entity/Player.ts:1999-2003 which writes
// `new TutOpen(com)` directly without any state diff.
//
// goscape pre-NAI-112 deferred to encodeOut's diff-check
// (modalTutorial != lastModalTutorial), suppressing duplicate-com calls.
// That regressed the Java client's overlay redraw after IF_SETTEXT
// updates: tutorial_step_view_inventory's first OpenTutorial(6179)
// emitted, but tutorial_step_cut_tree's second OpenTutorial(6179) was
// silently suppressed (smoke 2026-05-06).
func TestNAI_112_OpenTutorialUnconditionalReEmit(t *testing.T) {
	enc, _ := isaacPair([4]uint32{21, 22, 23, 24})
	wantEnc, _ := isaacPair([4]uint32{21, 22, 23, 24})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	readPacket := func() ([]byte, error) {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		_, err := io.ReadFull(clientConn, buf)
		return buf, err
	}

	// First open: must emit immediately.
	received1 := make(chan []byte, 1)
	go func() {
		if got, err := readPacket(); err == nil {
			received1 <- got
		}
	}()
	p.OpenTutorial(42)
	p.client.flushWrite()

	expectedByte1 := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)
	select {
	case got := <-received1:
		if got[0] != expectedByte1 {
			t.Errorf("first OpenTutorial(42) opcode: got %d, want %d", got[0], expectedByte1)
		}
		if int(got[1])<<8|int(got[2]) != 42 {
			t.Errorf("first OpenTutorial(42) component: got %d, want 42", int(got[1])<<8|int(got[2]))
		}
	case <-time.After(time.Second):
		t.Fatal("first OpenTutorial(42) did not emit TUT_OPEN")
	}

	// Second open with SAME com: must STILL emit (not diff-suppressed).
	received2 := make(chan []byte, 1)
	go func() {
		if got, err := readPacket(); err == nil {
			received2 <- got
		}
	}()
	p.OpenTutorial(42)
	p.client.flushWrite()

	expectedByte2 := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)
	select {
	case got := <-received2:
		if got[0] != expectedByte2 {
			t.Errorf("duplicate OpenTutorial(42) opcode: got %d, want %d", got[0], expectedByte2)
		}
		if int(got[1])<<8|int(got[2]) != 42 {
			t.Errorf("duplicate OpenTutorial(42) component: got %d, want 42", int(got[1])<<8|int(got[2]))
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate OpenTutorial(42) was diff-suppressed (H6.c divergence) — expected unconditional re-emit per TS Player.ts:1999-2003")
	}
}

// TestPlayerFlashTutorialWireBytes pins the TUT_FLASH wire shape:
// (*Player).FlashTutorial(tab) → OpTutFlash (126, 1) with 1-byte
// payload = byte(tab). Mirrors TS TutFlashEncoder.ts:9-11
// (buf.p1(message.tab)).
func TestPlayerFlashTutorialWireBytes(t *testing.T) {
	enc, _ := isaacPair([4]uint32{17, 18, 19, 20})
	wantEnc, _ := isaacPair([4]uint32{17, 18, 19, 20})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2) // 1 encrypted opcode + 1 payload byte
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.FlashTutorial(7)
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutFlash.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_FLASH encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if got[1] != 7 {
			t.Errorf("TUT_FLASH tab payload: got %d, want 7", got[1])
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_FLASH")
	}
}

// TestEncodeOutTutorialNoChangeNoEmit and TestEncodeOutTutorialResetEmitsMinusOne
// were retired by NAI-112 Stage 2.2. Both tests pinned the encodeOut
// diff-suppression (modalTutorial != lastModalTutorial) as intentional behavior.
// H6.c retired that diff: OpenTutorial/CloseTutorial now write TUT_OPEN
// unconditionally (per TS Player.ts:1999-2003 + 716-726).
// Coverage for the close path moved to TestCloseTutorialEmitsMinusOne.
// The unconditional re-emit is pinned by TestNAI_112_OpenTutorialUnconditionalReEmit.

// TestCloseTutorialEmitsMinusOne pins that calling (*Player).CloseTutorial()
// writes TUT_OPEN with payload [0xFF, 0xFF] (signed -1 → uint16 0xFFFF)
// DIRECTLY — no encodeOut deferred path. Mirrors TS Player.closeTutorial
// Player.ts:716-726 → `this.write(new TutOpen(-1))`.
// NAI-112 Stage 2.2: CloseTutorial inlines the wire write.
func TestCloseTutorialEmitsMinusOne(t *testing.T) {
	enc, _ := isaacPair([4]uint32{17, 18, 19, 20})
	wantEnc, _ := isaacPair([4]uint32{17, 18, 19, 20})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// Establish modalTutorial = 42 via a first open (drains the first emit).
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		drain := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		io.ReadFull(clientConn, drain) //nolint:errcheck
	}()
	p.OpenTutorial(42)
	p.client.flushWrite()
	<-drainDone       // wait until first emit bytes are consumed
	wantEnc.GetNext() // consume the encryptor step for the first emit

	// Now close via CloseTutorial — must emit TUT_OPEN(-1) immediately.
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.CloseTutorial()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_OPEN(close) encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if got[1] != 0xFF || got[2] != 0xFF {
			t.Errorf("TUT_OPEN(close) payload: got [%#x %#x], want [0xFF 0xFF]", got[1], got[2])
		}
		if p.modalTutorial != -1 {
			t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN(-1) via CloseTutorial")
	}
}

// TestEncodeOutTutorialIndependentOfMain pins that the tutorial emit
// is INDEPENDENT of the main/chat/side switch — opening main and tutorial
// in the same tick produces both packets.
// NAI-112 Stage 2.2: OpenTutorial writes TUT_OPEN immediately (before
// encodeOut). Wire order: TUT_OPEN (bytes 0-2) then IF_OPENMAIN (bytes 3-5).
func TestEncodeOutTutorialIndependentOfMain(t *testing.T) {
	enc, _ := isaacPair([4]uint32{21, 22, 23, 24})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	p.OpenMain(1234)
	p.OpenTutorial(42) // writes TUT_OPEN immediately to bufw

	// Read 6 bytes (3 for TUT_OPEN + 3 for IF_OPENMAIN).
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 6)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if n, err := io.ReadFull(clientConn, buf); err == nil && n == 6 {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	select {
	case got := <-received:
		if len(got) != 6 {
			t.Fatalf("expected 6 bytes (3+3), got %d: %#v", len(got), got)
		}
		// TUT_OPEN is emitted first (OpenTutorial direct write),
		// then IF_OPENMAIN is emitted by encodeOut's refreshModal switch.
		tutComp := int(got[1])<<8 | int(got[2])
		mainComp := int(got[4])<<8 | int(got[5])
		if tutComp != 42 {
			t.Errorf("tut component: got %d, want 42", tutComp)
		}
		if mainComp != 1234 {
			t.Errorf("main component: got %d, want 1234", mainComp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN + IF_OPENMAIN")
	}
}

func TestPlayer_BlockWalkFlag_Unconditional(t *testing.T) {
	// TS Player.blockWalkFlag (Player.ts:706-708) is unconditional —
	// returns CollisionFlag.PLAYER regardless of moveRestrict. Pin that
	// goscape behaves identically across all MoveRestrict variants.
	cases := []MoveRestrict{
		MoveRestrictNormal,
		MoveRestrictBlocked,
		MoveRestrictIndoors,
		MoveRestrictOutdoors,
		MoveRestrictNoMove,
		MoveRestrictPassthru,
	}
	for _, mr := range cases {
		t.Run(fmt.Sprintf("MR%d", mr), func(t *testing.T) {
			p := &Player{}
			p.moveRestrict = mr
			if got := p.blockWalkFlag(); got != collision.FlagBlockPlayers {
				t.Errorf("blockWalkFlag(%v) = %d, want FlagBlockPlayers (%d)", mr, got, collision.FlagBlockPlayers)
			}
		})
	}
}

func TestPlayer_GetCollisionStrategy_PerMoveRestrict(t *testing.T) {
	// Mirrors TS PathingEntity.getCollisionStrategy (PathingEntity.ts:558-575).
	// goscape MoveRestrict has no BLOCKED_NORMAL — that branch is skipped.
	cases := []struct {
		mr   MoveRestrict
		want *collision.Type
	}{
		{MoveRestrictNormal, ptrType(collision.TypeNormal)},
		{MoveRestrictBlocked, ptrType(collision.TypeBlocked)},
		{MoveRestrictIndoors, ptrType(collision.TypeIndoors)},
		{MoveRestrictOutdoors, ptrType(collision.TypeOutdoors)},
		{MoveRestrictNoMove, nil},
		{MoveRestrictPassthru, ptrType(collision.TypeNormal)},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("MR%d", tc.mr), func(t *testing.T) {
			p := &Player{}
			p.moveRestrict = tc.mr
			got := p.getCollisionStrategy()
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("getCollisionStrategy(%v) nil-mismatch: got %v want %v", tc.mr, got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Errorf("getCollisionStrategy(%v) = %v, want %v", tc.mr, *got, *tc.want)
			}
		})
	}
}

func ptrType(t collision.Type) *collision.Type { return &t }

// TestPlayerSetVisibilityDefault pins TS Player.setVisibility(DEFAULT) at
// Engine-TS/src/engine/entity/Player.ts:1875-1891. DEFAULT arm sets
// visibility=Default, blockWalk=Npc, calls ChangeNPCCollision(...,true)
// at player coords, and emits MessageGame "vis: 0".
func TestPlayerSetVisibilityDefault(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3200, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Start from Hard so we can observe the transition into Default.
	p.visibility = rsbuf.VisibilityHard
	p.blockWalk = BlockWalkNone

	received := drainConn(t, cc)

	p.SetVisibility(rsbuf.VisibilityDefault)

	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want VisibilityDefault", p.visibility)
	}
	if p.blockWalk != BlockWalkNpc {
		t.Errorf("blockWalk: got %v, want BlockWalkNpc", p.blockWalk)
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Error("FlagBlockNPCs: must be set at player tile after SetVisibility(Default)")
	}
	if !bytes.Contains(out, []byte("vis: 0")) {
		t.Errorf("MessageGame: out missing 'vis: 0'; got %q", out)
	}
}

// TestPlayerSetVisibilitySoftStub pins TS Player.setVisibility(SOFT) early
// return at Engine-TS/src/engine/entity/Player.ts:1876-1879. SOFT is a
// message-only stub: no state change to visibility, blockWalk, or
// collision flags. Pinning both presence-of-message AND absence-of-state-
// change per memory ts_asymmetry_dual_pin.
func TestPlayerSetVisibilitySoftStub(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3201, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Initial state: defaults (visibility=Default, blockWalk=BlockWalkNpc per
	// modules/world/player.go:556+523).
	if p.visibility != rsbuf.VisibilityDefault {
		t.Fatalf("preflight: visibility should default to Default")
	}
	if p.blockWalk != BlockWalkNpc {
		t.Fatalf("preflight: blockWalk should default to BlockWalkNpc")
	}

	received := drainConn(t, cc)
	p.SetVisibility(rsbuf.VisibilitySoft)
	p.client.flushWrite()
	out := <-received

	// State pins: unchanged.
	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default)", p.visibility)
	}
	if p.blockWalk != BlockWalkNpc {
		t.Errorf("blockWalk: got %v, want unchanged (BlockWalkNpc)", p.blockWalk)
	}

	// Message pin: includes "vis: 1 (not implemented - you are still on vis: 0)".
	if !bytes.Contains(out, []byte("vis: 1 (not implemented - you are still on vis: 0)")) {
		t.Errorf("MessageGame: missing TS-faithful SOFT stub string; got %q", out)
	}
}

// TestPlayerSetVisibilityHard pins TS Player.setVisibility(HARD) at
// Engine-TS/src/engine/entity/Player.ts:1885-1890. HARD arm sets
// visibility=Hard, blockWalk=None, calls ChangeNPCCollision(...,false)
// AND ChangePlayerCollision(...,false), and emits "vis: 2".
func TestPlayerSetVisibilityHard(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 3200, 3202, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)

	// Seed FlagBlockNPCs at the tile so we can observe the clear.
	s.gamemap.Pathfinder.Flags.Add(p.x, p.z, p.level, collision.FlagBlockNPCs|collision.FlagBlockPlayers)

	received := drainConn(t, cc)
	p.SetVisibility(rsbuf.VisibilityHard)
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityHard {
		t.Errorf("visibility: got %d, want VisibilityHard", p.visibility)
	}
	if p.blockWalk != BlockWalkNone {
		t.Errorf("blockWalk: got %v, want BlockWalkNone", p.blockWalk)
	}
	// After HARD: both FlagBlockNPCs and FlagBlockPlayers must be cleared.
	if s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Error("FlagBlockNPCs: must be cleared at player tile after SetVisibility(Hard)")
	}
	if s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockPlayers) {
		t.Error("FlagBlockPlayers: must be cleared at player tile after SetVisibility(Hard)")
	}
	if !bytes.Contains(out, []byte("vis: 2")) {
		t.Errorf("MessageGame: missing 'vis: 2'; got %q", out)
	}
}
