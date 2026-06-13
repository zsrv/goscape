package world

// login_revision_test.go pins the rev-274 login revision gate.
//
// TS contracts verified against dee467c8:
//
//	World.ts:2136-2138 — the op-16/18 revision is g1; 0xff escapes to a g2
//	extended read (274 does not fit in one byte). The 274 Java client
//	transmits p1(255) + p2(274) (Client.java:3586-3587 @32f30626), sizing
//	the payload with the extra two bytes (Client.java:3585).
//	World.ts:2140-2144 — rev !== Environment.engine.revision → send([6])
//	("RuneScape has been updated!") + close. WorldConfig.ts:91 sets the
//	engine revision to 274.

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
)

// buildRawLoginRequest builds a complete, well-formed op-16 login request
// with raw control over the revision wire bytes. It mirrors
// loginreq.GameLogin.MarshalBinary byte-for-byte (info byte, 9 checksums,
// RSA-encrypted credential block) so the ONLY divergence between test cases
// is the revision encoding — a reply other than [6] therefore proves the
// payload passed the revision gate.
func buildRawLoginRequest(revBytes []byte, checksums [9]uint32) []byte {
	b := packet.NewPacket(nil)
	b.P1(16) // OpReqInitGameConnection
	b.P1(0)  // payload length placeholder
	start := b.Len()

	for _, rb := range revBytes {
		b.P1(rb)
	}
	b.P1(0) // info byte: low-memory false
	for _, cs := range checksums {
		b.P4(cs)
	}

	plaintext := packet.NewPacket(nil)
	plaintext.P1(10) // RSA magic number
	for i := range 4 {
		plaintext.P4(uint32(i + 1)) // ISAAC seed
	}
	plaintext.P4(0x1234) // UID
	plaintext.PJStrLF("test")
	plaintext.PJStrLF("pw")
	plaintext.RSAEnc(protocol.Modulus, protocol.PublicExponent)
	b.PData(plaintext.Bytes())

	b.PSize1(b.Len() - start)
	return b.Bytes()
}

// TestHandleLogin_RevisionGate274 pins the rev-274 revision gate at the
// handleLogin level. The CRC table is seeded to match the request and the
// RSA block is valid, so a request that passes the revision gate runs all
// the way to the "login server not configured" reply (OpTryAgain, byte 1) —
// wire-distinguishable from the revision reject (OpClientOutOfDate, byte 6).
func TestHandleLogin_RevisionGate274(t *testing.T) {
	prev := cache.CRC()
	sentinel := [9]uint32{11, 22, 33, 44, 55, 66, 77, 88, 99}
	cache.SetCRCForTest(&cache.CRCSnapshot{Table: sentinel[:]})
	t.Cleanup(func() { cache.SetCRCForTest(prev) })

	cases := []struct {
		name      string
		revBytes  []byte
		wantReply byte
	}{
		// 0xff escape + u2 274 → passes the gate (TS World.ts:2136-2140).
		{"escaped_274_passes_gate", []byte{0xff, 0x01, 0x12}, loginresp.OpTryAgain.Opcode},
		// Hypothetical single-byte 254 client → survives the read but fails
		// the equality check → [6] + close (World.ts:2140-2143).
		{"single_byte_254_rejected", []byte{0xfe}, loginresp.OpClientOutOfDate.Opcode},
		// Escaped u2 254 → decodes to 254, fails equality → [6] + close.
		{"escaped_254_rejected", []byte{0xff, 0x00, 0xfe}, loginresp.OpClientOutOfDate.Opcode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, clientConn := newTestClient(t)
			c.bufferData(buildRawLoginRequest(tc.revBytes, sentinel))

			received := make(chan byte, 1)
			go func() {
				buf := make([]byte, 1)
				clientConn.SetReadDeadline(time.Now().Add(time.Second))
				if _, err := io.ReadFull(clientConn, buf); err == nil {
					received <- buf[0]
				}
			}()

			// Both outcomes end in a single reply byte + connection close.
			if err := c.handleLogin(); !errors.Is(err, errCloseConn) {
				t.Errorf("handleLogin: got err %v, want errCloseConn", err)
			}

			select {
			case got := <-received:
				if got != tc.wantReply {
					t.Errorf("reply byte: got %d, want %d", got, tc.wantReply)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for login reply byte")
			}
		})
	}
}
