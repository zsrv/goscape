package world

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
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
	c.bufw.Write(seed.Bytes())
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

func BenchmarkClientSetup(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		c := newClient(nil, 30*time.Second, slog.Default())
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
	}
}
