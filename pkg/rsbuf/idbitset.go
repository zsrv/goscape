package rsbuf

// idBitSet pairs a bit-array for O(1) containment with an ordered ID list
// for iteration. Mirrors upstream build.rs IdBitSet (2004scape/rsbuf
// branch 225, src/build.rs:8-62).
//
// Used by BuildArea to track per-player observed players + npcs.
//
// Concurrency: tick-goroutine-owned. No internal synchronization
// (matches upstream's WASM single-threaded model — see lib.rs static-mut
// model).
type idBitSet struct {
	bits []uint32 // bits[id>>5] & (1 << (id & 0x1f)) tests containment
	ids  []int32  // insertion-ordered list of contained ids
}

// newIdBitSet returns an empty idBitSet with bit-array sized to address
// ids in [0, maxID) and ids slice pre-allocated to capacity. capacity is
// only an initial backing-array hint; the slice grows as needed.
//
// maxID must be a power-of-two-multiple of 32; pass 2048 (player slot
// count) or 16384 (npc nid count).
func newIdBitSet(maxID, capacity int) *idBitSet {
	return &idBitSet{
		bits: make([]uint32, maxID/32),
		ids:  make([]int32, 0, capacity),
	}
}

// Contains reports whether id is in the set. O(1).
func (s *idBitSet) Contains(id int32) bool {
	return s.bits[id>>5]&(1<<(id&0x1f)) != 0
}

// Insert adds id to the set. No-op if id is already present (the insertion
// order list is preserved exactly once per id).
func (s *idBitSet) Insert(id int32) {
	if s.Contains(id) {
		return
	}
	s.bits[id>>5] |= 1 << (id & 0x1f)
	s.ids = append(s.ids, id)
}

// Remove takes id out of the set. No-op if id is not present.
func (s *idBitSet) Remove(id int32) {
	if !s.Contains(id) {
		return
	}
	s.bits[id>>5] &^= 1 << (id & 0x1f)
	for i, v := range s.ids {
		if v == id {
			s.ids = append(s.ids[:i], s.ids[i+1:]...)
			return
		}
	}
}

// Len returns the number of ids in the set.
func (s *idBitSet) Len() int {
	return len(s.ids)
}

// Iter returns a copy of the contained ids in insertion order. Caller-
// owned: mutating the returned slice does not affect the set. Mirrors
// upstream IdBitSet::iter at build.rs:53-55 which clones the Vec.
func (s *idBitSet) Iter() []int32 {
	out := make([]int32, len(s.ids))
	copy(out, s.ids)
	return out
}

// Clear empties the set: zeros all bit-words and truncates the ids
// slice. Capacity of bits + ids is preserved.
func (s *idBitSet) Clear() {
	for i := range s.bits {
		s.bits[i] = 0
	}
	s.ids = s.ids[:0]
}
