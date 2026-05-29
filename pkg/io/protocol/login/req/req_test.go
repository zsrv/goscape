package req

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildLoginHeader builds just the cleartext header of a login packet
// (opcode, payload size, revision, info byte, 9 archive checksums) with no
// RSA tail. The header payload is rev(1) + info(1) + 9*4 checksums = 38 bytes,
// so payload size is set to 38 and the packet ends after the checksums.
func buildLoginHeader(rev, info byte) []byte {
	p := packet.NewPacket(nil)
	p.P1(OpReqInitGameConnection.Opcode)
	p.P1(38) // payload size: rev + info + 9*4 checksums
	p.P1(rev)
	p.P1(info)
	for range 9 {
		p.P4(0)
	}
	return p.Bytes()
}

func TestUnmarshalHeader_LowMemoryMasksLowBit(t *testing.T) {
	// TS World.ts:2127 takes lowMemory from the info byte's low bit:
	// `(info & 0x1) !== 0`. A plain `== 1` (the old GBool decode) diverges for
	// odd values > 1, e.g. info=3. Pin the masking contract. L35.
	tests := []struct {
		name string
		info byte
		want bool
	}{
		{"info0_false", 0, false},
		{"info1_true", 1, true},
		{"info2_false", 2, false},
		{"info3_true_decisive", 3, true}, // GBool (==1) would give false here
		{"info255_true", 255, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q GameLogin
			r := packet.NewPacket(buildLoginHeader(225, tt.info))
			if err := q.UnmarshalHeader(r); err != nil {
				t.Fatalf("UnmarshalHeader: %v", err)
			}
			if q.LowMemory != tt.want {
				t.Errorf("LowMemory for info=%d: got %v, want %v", tt.info, q.LowMemory, tt.want)
			}
		})
	}
}

func TestUnmarshalHeader_DecodesWithoutRSABlock(t *testing.T) {
	// L37: TS gates rev (World.ts:2119) and CRC (2131) before rsadec (2139).
	// UnmarshalHeader must decode revision + checksums from the cleartext alone,
	// without requiring or consuming the RSA tail, so a caller can reject a
	// stale-revision / bad-CRC client before spending any RSA CPU.
	var q GameLogin
	r := packet.NewPacket(buildLoginHeader(225, 1))
	if err := q.UnmarshalHeader(r); err != nil {
		t.Fatalf("UnmarshalHeader: %v", err)
	}
	if q.Revision != 225 {
		t.Errorf("Revision: got %d, want 225", q.Revision)
	}
	for i, c := range q.ArchiveChecksums {
		if c != 0 {
			t.Errorf("ArchiveChecksums[%d]: got %d, want 0", i, c)
		}
	}
}

func TestUnmarshalBinary_TruncatedRSABlockPanics(t *testing.T) {
	// Root-cause reproduction for gap-login-wire-1: the RS2 packet read
	// methods (G1/GData) panic on under-read rather than returning errors, so
	// a login packet whose cleartext header is well-formed but whose RSA tail
	// is truncated drives RSADec -> GData into a slice-out-of-range panic.
	// UnmarshalRSA's `if err := r.RSADec(...); err != nil` guard never sees an
	// error because RSADec panics first. This panic is unauthenticated and
	// attacker-controllable, so the per-connection handler MUST contain it
	// (see TestServeConn_* in modules/world) — TS isolates per-connection via
	// try/catch -> client.terminate() (TcpServer.ts:29-41).
	//
	// Packet layout: opcode(16) + size(39) + rev(1) + info(1) +
	// 9*4 checksums(36) + numBytes(64). The header consumes rev+info+36 = 38
	// bytes, leaving the single numBytes=64 byte; RSADec reads it as the RSA
	// block length and then GData(rsax, 64) slices past the end of the buffer.
	p := packet.NewPacket(nil)
	p.P1(OpReqInitGameConnection.Opcode)
	p.P1(39) // payload size: rev + info + 9*4 checksums + 1 (numBytes)
	p.P1(225)
	p.P1(0)
	for range 9 {
		p.P4(0)
	}
	p.P1(64) // RSA block length byte claiming 64 bytes that aren't present
	malformed := p.Bytes()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("UnmarshalBinary on a truncated RSA block did not panic; " +
				"gap-login-wire-1 root cause may have changed — revisit the " +
				"per-connection recover() in modules/world/server.go")
		}
	}()

	var q GameLogin
	_ = q.UnmarshalBinary(malformed)
}

func TestGameLogin_RoundTrip(t *testing.T) {
	// Confirms the header/RSA split (L37) still round-trips end-to-end:
	// MarshalBinary (client, RSA-encrypts) → UnmarshalBinary (server), which
	// now decodes via UnmarshalHeader + UnmarshalRSA.
	orig := GameLogin{
		Username:         "tester",
		Password:         "secret",
		ArchiveChecksums: [9]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9},
		ISAACSeed:        [4]uint32{0xdead, 0xbeef, 0xcafe, 0xf00d},
		UID:              0x12345678,
		Revision:         225,
		LowMemory:        true,
	}
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	var got GameLogin
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if got.Username != orig.Username || got.Password != orig.Password {
		t.Errorf("creds: got %q/%q, want %q/%q", got.Username, got.Password, orig.Username, orig.Password)
	}
	if got.UID != orig.UID || got.Revision != orig.Revision || got.LowMemory != orig.LowMemory {
		t.Errorf("scalars: got UID=%#x rev=%d low=%v, want %#x/%d/%v",
			got.UID, got.Revision, got.LowMemory, orig.UID, orig.Revision, orig.LowMemory)
	}
	if got.ArchiveChecksums != orig.ArchiveChecksums {
		t.Errorf("checksums: got %v, want %v", got.ArchiveChecksums, orig.ArchiveChecksums)
	}
	if got.ISAACSeed != orig.ISAACSeed {
		t.Errorf("seed: got %v, want %v", got.ISAACSeed, orig.ISAACSeed)
	}
}
