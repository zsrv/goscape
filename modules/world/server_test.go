package world

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestClient(t *testing.T) (*client, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	c := newClient(serverConn, time.Second, discardLogger())
	t.Cleanup(func() { c.in.Release() })
	return c, clientConn
}

// Seed packet size

func TestSeedPacketIsExactly8Bytes(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 32) // read more than needed to catch oversized packets
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := clientConn.Read(buf)
		received <- buf[:n]
	}()

	seed := packet.NewPacket(make([]byte, 0, 8))
	seed.P4(0xDEADBEEF)
	seed.P4(0xCAFEBABE)
	c.write(seed.Bytes())
	c.flushWrite()

	select {
	case got := <-received:
		if len(got) != 8 {
			t.Errorf("seed packet: got %d bytes, want 8", len(got))
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for seed packet")
	}
}

// Fix 3: incoming buffer overflow protection

func TestBufferDataRejectsWhenAtMaxSize(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize))

	if c.bufferData([]byte{1}) {
		t.Error("expected bufferData to reject data when buffer is full")
	}
	if c.in.Len() != maxClientInBufSize {
		t.Errorf("buffer should be unchanged: got %d, want %d", c.in.Len(), maxClientInBufSize)
	}
}

func TestBufferDataAcceptsDataWithinLimit(t *testing.T) {
	c, _ := newTestClient(t)

	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !c.bufferData(data) {
		t.Error("expected bufferData to accept data within limit")
	}
	if c.in.Len() != len(data) {
		t.Errorf("buffer length: got %d, want %d", c.in.Len(), len(data))
	}
}

func TestBufferDataRejectsExactOverflow(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize-2))

	// 2 bytes remaining — 3 bytes should overflow
	if c.bufferData(make([]byte, 3)) {
		t.Error("expected bufferData to reject data that would overflow by 1 byte")
	}
}

func TestBufferDataAcceptsUpToExactLimit(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize-3))

	// Exactly 3 bytes remaining
	if !c.bufferData(make([]byte, 3)) {
		t.Error("expected bufferData to accept data that fills buffer exactly")
	}
}

// Fix 4+5: login error byte is sent and errCloseConn is returned

func TestSendLoginErrorWritesCodeToClient(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	_ = c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)

	select {
	case got := <-received:
		if got != loginresp.OpClientOutOfDate.Opcode {
			t.Errorf("wrong response byte: got %d, want %d", got, loginresp.OpClientOutOfDate.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for response byte")
	}
}

func TestSendLoginErrorReturnsErrCloseConn(t *testing.T) {
	c, clientConn := newTestClient(t)
	go io.Copy(io.Discard, clientConn) // drain so flush doesn't block

	err := c.sendLoginError(loginresp.OpIPLimit.Opcode)
	if !errors.Is(err, errCloseConn) {
		t.Errorf("expected errCloseConn, got %v", err)
	}
}

func TestSendLoginErrorVariousCodes(t *testing.T) {
	codes := []byte{
		loginresp.OpInvalidUsernameOrPassword.Opcode,
		loginresp.OpBanned.Opcode,
		loginresp.OpDuplicate.Opcode,
		loginresp.OpClientOutOfDate.Opcode,
		loginresp.OpServerFull.Opcode,
		loginresp.OpLoginServerOffline.Opcode,
		loginresp.OpIPLimit.Opcode,
	}

	for _, code := range codes {
		t.Run("code"+string(rune('0'+code)), func(t *testing.T) {
			c, clientConn := newTestClient(t)

			received := make(chan byte, 1)
			go func() {
				buf := make([]byte, 1)
				clientConn.SetReadDeadline(time.Now().Add(time.Second))
				if _, err := io.ReadFull(clientConn, buf); err == nil {
					received <- buf[0]
				}
			}()

			if err := c.sendLoginError(code); !errors.Is(err, errCloseConn) {
				t.Errorf("expected errCloseConn for code %d", code)
			}

			select {
			case got := <-received:
				if got != code {
					t.Errorf("wrong byte: got %d, want %d", got, code)
				}
			case <-time.After(time.Second):
				t.Errorf("timed out waiting for code %d", code)
			}
		})
	}
}

func TestSendLoginOKSendsOpOKAndTransitionsState(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		if got != loginresp.OpOK.Opcode {
			t.Errorf("login OK byte: got %d, want %d", got, loginresp.OpOK.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for login OK byte")
	}

	if c.state != ClientStateGame {
		t.Errorf("state after sendLoginOK: got %v, want ClientStateGame", c.state)
	}
}

func TestSendLoginOKStaffSendsRightsByte(t *testing.T) {
	c, clientConn := newTestClient(t)
	c.staffModLevel = 1

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		if got != loginresp.OpLoginOKWithRights.Opcode {
			t.Errorf("staff login OK byte: got %d, want %d", got, loginresp.OpLoginOKWithRights.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for staff login OK byte")
	}
}

func TestGameProtTableHasExpectedOpcodes(t *testing.T) {
	cases := []struct {
		opcode      int
		name        string
		payloadSize int
	}{
		{108, "NO_TIMEOUT", 0},
		{70, "IDLE_TIMER", 0},
		{181, "MOVE_GAMECLICK", -1},
		{93, "MOVE_OPCLICK", -1},
		{165, "MOVE_MINIMAPCLICK", -1},
		{150, "REBUILD_GETMAPS", -1},
		{81, "EVENT_TRACKING", -2},
	}
	for _, tc := range cases {
		op := gameclient.Ops[tc.opcode]
		if op.Name != tc.name {
			t.Errorf("Ops[%d].Name = %q, want %q", tc.opcode, op.Name, tc.name)
		}
		if op.PayloadSize != tc.payloadSize {
			t.Errorf("Ops[%d].PayloadSize = %d, want %d", tc.opcode, op.PayloadSize, tc.payloadSize)
		}
	}
}

// isaacPair returns two independent ISAAC instances with identical initial state.
// Use enc to encrypt opcodes in the test, dec to give to the client under test.
func isaacPair(seed [4]uint32) (enc, dec *io2.Isaac) {
	return io2.New(seed), io2.New(seed)
}

// encryptOpcode produces the wire byte the Java client sends for realOpcode.
func encryptOpcode(enc *io2.Isaac, realOpcode byte) byte {
	return byte((int(realOpcode) + int(enc.GetNext())) & 0xff)
}

func TestHandleGameEmptyBufferReturnsErrPayloadTooSmall(t *testing.T) {
	_, dec := isaacPair([4]uint32{1, 2, 3, 4})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	err := c.handleGame()
	if !errors.Is(err, protocol.ErrPayloadTooSmall) {
		t.Errorf("empty buffer: got %v, want ErrPayloadTooSmall", err)
	}
}

func TestHandleGameUnknownOpcodeReturnsErrCloseConn(t *testing.T) {
	// Opcode 0 is not registered in the Ops table.
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	c.in.Write([]byte{encryptOpcode(enc, 0)})

	err := c.handleGame()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("unknown opcode: got %v, want errCloseConn", err)
	}
}

func BenchmarkClientSetup(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		c := newClient(nil, 30*time.Second, slog.Default())
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
	}
}
