package world

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
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
// AND activeScript.Protect. Mirrors the convergence documented at
// CanAccess (player_script.go:232-238).
func TestPlayer_ProtectedScriptActive_TruthTable(t *testing.T) {
	cases := []struct {
		name   string
		active *script.ScriptState
		want   bool
	}{
		{"nil-active", nil, false},
		{"active-unprotected", &script.ScriptState{Protect: false}, false},
		{"active-protected", &script.ScriptState{Protect: true}, true},
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
	p.OpenTutorial(42)

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
		if p.lastModalTutorial != 42 {
			t.Errorf("lastModalTutorial: got %d, want 42", p.lastModalTutorial)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN")
	}
}

func TestEncodeOutTutorialNoChangeNoEmit(t *testing.T) {
	enc, _ := isaacPair([4]uint32{13, 14, 15, 16})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// First encodeOut after OpenTutorial(42): drain concurrently so
	// flushWrite does not block on net.Pipe's synchronous writes.
	p.OpenTutorial(42)
	p.encodeOut()
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		drain := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		io.ReadFull(clientConn, drain) //nolint:errcheck
	}()
	p.client.flushWrite()
	<-drainDone // wait until first emit bytes are consumed

	// Second pass — no field change.
	p.encodeOut()
	p.client.flushWrite()
	clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 16)
	n, _ := clientConn.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes from no-change encodeOut, got %d", n)
	}
}

// TestEncodeOutTutorialResetEmitsMinusOne pins the wire shape for
// future TUT_CLOSE: setting modalTutorial back to -1 emits
// OpTutOpen with payload [0xFF, 0xFF] (signed -1 → uint16 0xFFFF).
// Mirrors TS Player.closeTutorial Player.ts:716-726 which writes
// `new TutOpen(-1)`. closeTutorial itself is deferred; this test
// pins the diff-check emit path independently.
func TestEncodeOutTutorialResetEmitsMinusOne(t *testing.T) {
	enc, _ := isaacPair([4]uint32{17, 18, 19, 20})
	wantEnc, _ := isaacPair([4]uint32{17, 18, 19, 20})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// Establish lastModalTutorial = 42 via a first emit pass.
	// net.Pipe() is synchronous: start a concurrent reader so flushWrite
	// does not block, then signal when the drain bytes are consumed.
	drainDone := make(chan struct{})
	p.OpenTutorial(42)
	p.encodeOut()
	go func() {
		defer close(drainDone)
		drain := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		io.ReadFull(clientConn, drain) //nolint:errcheck
	}()
	p.client.flushWrite()
	<-drainDone                // wait until first emit bytes are consumed
	wantEnc.GetNext()          // consume the encryptor step for the first emit

	// Now reset.
	p.modalTutorial = -1

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_OPEN(reset) encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if got[1] != 0xFF || got[2] != 0xFF {
			t.Errorf("TUT_OPEN(reset) payload: got [%#x %#x], want [0xFF 0xFF]", got[1], got[2])
		}
		if p.lastModalTutorial != -1 {
			t.Errorf("lastModalTutorial: got %d, want -1", p.lastModalTutorial)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN(-1)")
	}
}

// TestEncodeOutTutorialIndependentOfMain pins that the tutorial emit
// branch is INDEPENDENT of the main/chat/side switch — opening
// main and tutorial in the same tick produces both packets.
func TestEncodeOutTutorialIndependentOfMain(t *testing.T) {
	enc, _ := isaacPair([4]uint32{21, 22, 23, 24})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	p.OpenMain(1234)
	p.OpenTutorial(42)

	// Read up to 6 bytes (3 for IF_OPENMAIN + 3 for TUT_OPEN).
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
		// Per encodeOut order, IF_OPENMAIN is emitted first (within
		// the refreshModal switch), then the tutorial branch.
		mainComp := int(got[1])<<8 | int(got[2])
		tutComp := int(got[4])<<8 | int(got[5])
		if mainComp != 1234 {
			t.Errorf("main component: got %d, want 1234", mainComp)
		}
		if tutComp != 42 {
			t.Errorf("tut component: got %d, want 42", tutComp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_OPENMAIN + TUT_OPEN")
	}
}
