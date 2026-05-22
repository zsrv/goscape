package world

import "testing"

// TestNextPmIdCounterMonotone pins the low 16 bits of pmId as a
// monotonically increasing counter, and confirms pmCount advances by 1
// per call. Random byte (bits 16-23) is masked before assertion per
// memory:no_rng_seam_cascade_probe_bypass.md — pkg/script and friends
// use math/rand/v2 globally with no test seam.
//
// pmCount is explicitly zeroed here to test the counter mechanic in
// isolation. Production NewServer (and newTestServer) inits pmCount to 1
// per TS World.ts:167.
func TestNextPmIdCounterMonotone(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0
	s.pmCount.Store(0)
	got := []uint32{s.nextPmId(), s.nextPmId(), s.nextPmId()}
	// Mask out the random byte (bits 16-23); compare NodeID byte + counter.
	const randMask = uint32(0xff00ffff)
	want := []uint32{0x00000000, 0x00000001, 0x00000002}
	for i, g := range got {
		if g&randMask != want[i] {
			t.Errorf("nextPmId[%d]: got %08x masked %08x, want %08x",
				i, g, g&randMask, want[i])
		}
	}
	if got := s.pmCount.Load(); got != 3 {
		t.Errorf("pmCount: got %d, want 3", got)
	}
}

// TestNextPmIdNodeIDByte pins the high 8 bits to cfg.NodeID & 0xff.
// Mirrors TS World.sendPrivateMessage (World.ts:1641) where
// Environment.NODE_ID populates bits 24-31.
func TestNextPmIdNodeIDByte(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0x42
	pm := s.nextPmId()
	if (pm>>24)&0xff != 0x42 {
		t.Errorf("NodeID byte: got %02x, want 0x42 (pm=%08x)", (pm>>24)&0xff, pm)
	}
}

// TestNextPmIdRandByteInRange pins the TS off-by-one from
// Math.random()*0xff producing values in [0, 254]. The Go port uses
// rand.IntN(0xff) which yields [0, 254], NOT rand.IntN(256).
func TestNextPmIdRandByteInRange(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0
	for i := range 64 {
		// Reset pmCount each iteration so it doesn't pollute bits 0-15
		// when the counter wraps into bit 16+ (would take 65536 calls
		// in practice; defensive reset keeps the test independent).
		s.pmCount.Store(0)
		pm := s.nextPmId()
		randByte := (pm >> 16) & 0xff
		if randByte > 0xfe {
			t.Errorf("iter %d: rand byte %d > 254 (pm=%08x)", i, randByte, pm)
		}
	}
}
