package world

import (
	"bytes"
	"errors"
	"testing"

	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	"github.com/zsrv/goscape/pkg/tapper"
)

// rampBytes returns n bytes with distinct values, so a test can tell which
// slice of a frame the tap recorded.
func rampBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i + 1)
	}
	return b
}

// TestRejectedInboundTap_UnknownOpcode pins that the packet which got the
// connection closed is captured before the teardown. Both rejections used to
// return errCloseConn ahead of the inbound tap, so the one frame a consumer
// most wants — an opcode absent from that revision's packet table — was the
// one frame never recorded.
func TestRejectedInboundTap_UnknownOpcode(t *testing.T) {
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	p.accountID = 4242
	tp := attachTap(p.client)

	// Opcode 3 is not in this revision's Ops table.
	payload := rampBytes(200)
	p.client.in.Write([]byte{encryptOpcode(enc, 3)})
	p.client.in.Write(payload)

	if _, _, _, err := p.readPacket(); err == nil {
		t.Fatal("preflight: readPacket accepted an unknown opcode")
	}

	got := tp.tapped()
	if len(got) != 1 {
		t.Fatalf("tapped %d packets, want exactly 1", len(got))
	}
	if got[0].dir != tapper.DirIn {
		t.Errorf("direction = %v, want inbound", got[0].dir)
	}
	if got[0].opcode != 3 {
		t.Errorf("opcode = %d, want the DECRYPTED 3", got[0].opcode)
	}
	if want := payload[:maxRejectedTapBytes]; !bytes.Equal(got[0].payload, want) {
		t.Errorf("payload = %v, want the first %d buffered bytes %v",
			got[0].payload, maxRejectedTapBytes, want)
	}

	wantCloseReason(t, p.client, tp, tapper.CloseReasonProtocol)
}

// TestRejectedInboundTap_UnknownOpcodeRecordsOnlyWhatIsBuffered pins that the
// tap never waits for more bytes: the connection is ending, so it records the
// short tail that has already arrived and nothing else.
func TestRejectedInboundTap_UnknownOpcodeRecordsOnlyWhatIsBuffered(t *testing.T) {
	enc, dec := isaacPair([4]uint32{9, 10, 11, 12})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	tp := attachTap(p.client)

	payload := rampBytes(3)
	p.client.in.Write([]byte{encryptOpcode(enc, 3)})
	p.client.in.Write(payload)

	if _, _, _, err := p.readPacket(); err == nil {
		t.Fatal("preflight: readPacket accepted an unknown opcode")
	}

	got := tp.tapped()
	if len(got) != 1 {
		t.Fatalf("tapped %d packets, want exactly 1", len(got))
	}
	if !bytes.Equal(got[0].payload, payload) {
		t.Errorf("payload = %v, want the 3 buffered bytes %v", got[0].payload, payload)
	}
}

// TestRejectedInboundTap_OversizedFrame covers the other rejection.
//
// It drives the decoder directly from the state a 2-byte-length op leaves it
// in — opcode read, length prefix pending — so the branch is pinned whatever
// this revision's packet table happens to contain.
// TestRejectedInboundTap_OversizedFrameFromRealFrame is the same rejection
// reached the ordinary way, over the wire.
func TestRejectedInboundTap_OversizedFrame(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)

	p.client.opcode = int(gameclient.OpcMessagePublic)
	p.client.waiting = -2

	payload := rampBytes(100)
	p.client.in.Write([]byte{0x07, 0xD0}) // declared length 2000 > the 1600 cap
	p.client.in.Write(payload)

	if _, _, _, err := p.readPacket(); err == nil {
		t.Fatal("preflight: readPacket accepted an oversized frame")
	}

	got := tp.tapped()
	if len(got) != 1 {
		t.Fatalf("tapped %d packets, want exactly 1", len(got))
	}
	if got[0].dir != tapper.DirIn {
		t.Errorf("direction = %v, want inbound", got[0].dir)
	}
	if got[0].opcode != gameclient.OpcMessagePublic {
		t.Errorf("opcode = %d, want %d", got[0].opcode, gameclient.OpcMessagePublic)
	}
	if want := payload[:maxRejectedTapBytes]; !bytes.Equal(got[0].payload, want) {
		t.Errorf("payload = %v, want the first %d buffered bytes %v",
			got[0].payload, maxRejectedTapBytes, want)
	}

	wantCloseReason(t, p.client, tp, tapper.CloseReasonProtocol)
}

// TestRejectedInboundTap_OversizedFrameFromRealFrame reaches the oversized
// rejection the ordinary way: a real EVENT_TRACKING frame — this revision's
// one 2-byte-length opcode — whose declared length is over the 1600 cap. It
// pins that readPacket taps what a peer actually sent, opcode decryption and
// length decoding included, rather than only the hand-placed decoder state its
// sibling above starts from.
func TestRejectedInboundTap_OversizedFrameFromRealFrame(t *testing.T) {
	enc, dec := isaacPair([4]uint32{13, 14, 15, 16})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	p.accountID = 909
	tp := attachTap(p.client)

	payload := rampBytes(100)
	p.client.in.Write([]byte{encryptOpcode(enc, gameclient.OpcEventTracking)})
	p.client.in.Write([]byte{0x07, 0xD0}) // declared length 2000 > the 1600 cap
	p.client.in.Write(payload)

	if _, _, _, err := p.readPacket(); !errors.Is(err, errCloseConn) {
		t.Fatalf("preflight: readPacket returned %v, want errCloseConn", err)
	}

	got := tp.tapped()
	if len(got) != 1 {
		t.Fatalf("tapped %d packets, want exactly 1", len(got))
	}
	if got[0].dir != tapper.DirIn {
		t.Errorf("direction = %v, want inbound", got[0].dir)
	}
	if got[0].opcode != gameclient.OpcEventTracking {
		t.Errorf("opcode = %d, want the DECRYPTED %d", got[0].opcode, gameclient.OpcEventTracking)
	}
	if want := payload[:maxRejectedTapBytes]; !bytes.Equal(got[0].payload, want) {
		t.Errorf("payload = %v, want the first %d buffered bytes %v",
			got[0].payload, maxRejectedTapBytes, want)
	}

	wantCloseReason(t, p.client, tp, tapper.CloseReasonProtocol)
}

// TestRejectedInboundTap_DisabledCostsNothing pins the disabled path: with the
// public no-op tapper the rejection tap allocates nothing and does no more
// work than having no tap at all.
//
// It measures the tap helper rather than the whole readPacket rejection,
// because that rejection already emits a Warn whose variadic arguments are
// built at the call site whatever the log level is — an allocation that
// predates this change and that no tap guard can remove. The helper is
// exactly what Task 4 adds to the path.
func TestRejectedInboundTap_DisabledCostsNothing(t *testing.T) {
	c, _ := newTestClient(t)
	c.sessionID = "11111111-2222-3333-4444-555555555555"
	buf := rampBytes(200)

	c.tap = tapper.NoopTapper()
	noop := testing.AllocsPerRun(100, func() {
		c.tapRejectedInbound(1, 3, buf)
	})
	if noop != 0 {
		t.Errorf("no-op tapper allocated %v times per rejection, want 0", noop)
	}

	c.tap = nil
	if untapped := testing.AllocsPerRun(100, func() {
		c.tapRejectedInbound(1, 3, buf)
	}); noop != untapped {
		t.Errorf("no-op tapper allocated %v times, an untapped build %v: the disabled path must cost the same",
			noop, untapped)
	}
}
