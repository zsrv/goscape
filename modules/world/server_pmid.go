package world

import "math/rand/v2"

// nextPmId mirrors the pmId computation inside TS
// World.sendPrivateMessage (World.ts:1641):
//
//	(Environment.NODE_ID << 24) + ((Math.random() * 0xff) << 16)
//	  + this.pmCount++
//
// Bit layout (MSB to LSB):
//
//	bits 24-31: cfg.NodeID & 0xff
//	bits 16-23: random byte in [0, 254]
//	bits 0-15:  pmCount (post-increment)
//
// pmCount is initialized to 1 in NewServer (not 0); see TS World.ts:167
// "can't be 0 as clients will ignore the pm, their array is filled
// with 0 as default".
//
// Uses rand.IntN(0xff) (range [0, 254]) — NOT rand.IntN(256) — to match
// the TS off-by-one from Math.random()*0xff yielding strictly less than
// 0xff. Test seam absent per memory:no_rng_seam_cascade_probe_bypass.md;
// tests mask bits 16-23 to assert deterministic parts.
func (s *Server) nextPmId() uint32 {
	randByte := uint32(rand.IntN(0xff))
	// R4 (Arc 18): atomic.Uint32 — Add returns the post-increment value,
	// so subtract 1 to bake the pre-increment counter into pmId (matching
	// the prior `s.pmCount; s.pmCount++` order).
	post := s.pmCount.Add(1)
	pre := post - 1
	pm := uint32(s.cfg.NodeID&0xff)<<24 | randByte<<16 | pre
	return pm
}
