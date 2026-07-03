package world

import (
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
	loginreq "github.com/zsrv/goscape/pkg/io/protocol/login/req"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
)

// TestHandleLogin_TruncatedRSABlock_RejectsWithClientOutOfDate drives a
// truncated-RSA OpReqInitGameConnection login packet through the real
// accept-loop -> handleTCPConn -> handleLogin read path (HandleConn over a
// net.Pipe, the same harness as TestHandleConn_ContainsPanic /
// TestHandleConn_QuitGate in conn_handler_test.go) and pins the single
// reject byte handleLogin writes back to the socket.
//
// The fixture is built the same way as
// TestUnmarshalBinary_TruncatedRSABlockReturnsError
// (pkg/io/protocol/login/req/req_test.go): a well-formed OUTER packet
// envelope (opcode + declared payload size of 39, matching what's actually
// written — so protocol.CheckPacketLength sees a complete packet and lets
// it through) whose payload is revision(1) + info(1) + 9*4 zero checksums
// + a single trailing numBytes=64 byte claiming a 64-byte RSA block that
// isn't there. The truncation is caught one layer deeper, inside
// Packet.RSADec's own bounds check (declared 64 > 0 remaining), not by the
// outer framing check.
//
// handleLogin's UnmarshalRSA error branch (server.go, the
// OpReqInitGameConnection/OpReqGameReconnect case) sends
// loginresp.OpClientOutOfDate.Opcode for this failure — the same reject
// used for a malformed cleartext header, a revision mismatch, and a CRC
// mismatch, all in that one case block. There is no separate
// "malformed RSA" reject opcode; OpClientOutOfDate is genuinely what a real
// client sees on this wire input, so that is what this test pins.
func TestHandleLogin_TruncatedRSABlock_RejectsWithClientOutOfDate(t *testing.T) {
	s := newHandleConnTestServer(t)
	client, server := net.Pipe()
	defer client.Close()

	p := packet.NewPacket(nil)
	p.P1(loginreq.OpReqInitGameConnection.Opcode)
	p.P1(39) // declared payload size: rev + info + 9*4 checksums + 1 (numBytes)
	p.P1(225)
	p.P1(0)
	for range 9 {
		p.P4(0)
	}
	p.P1(64) // RSA block length byte claiming 64 bytes that aren't present
	malformed := p.Bytes()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.HandleConn(server)
	}()

	if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if _, err := client.Write(malformed); err != nil {
		t.Fatalf("write malformed login packet: %v", err)
	}

	buf := make([]byte, 8)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("read reject byte: %v", err)
	}
	if n != 1 {
		t.Fatalf("reject reply length: got %d bytes (%v), want exactly 1", n, buf[:n])
	}
	if got, want := buf[0], loginresp.OpClientOutOfDate.Opcode; got != want {
		t.Fatalf("reject opcode: got %d, want %d (loginresp.OpClientOutOfDate)", got, want)
	}

	// handleLogin's sendLoginError returns errCloseConn, so handleTCPConn
	// closes the connection right after flushing the reject byte — a
	// second read must observe that closure, not more data or a hang.
	if _, err := client.Read(buf); err == nil {
		t.Error("expected connection closed after the reject byte; read succeeded instead")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleConn did not return after rejecting the truncated-RSA login")
	}
}
