package collision

import "testing"

// TestFlagNullMatchesTSCollisionFlag pins FlagNull to TS
// CollisionFlag.NULL = 0x7FFFFFFF (@2004scape/rsmod-pathfinder): every flag
// bit set EXCEPT FlagRoof (bit 31). Guards the L43 fix from regressing back
// to -1 (which set bit 31 and wrongly made off-map tiles "indoors").
func TestFlagNullMatchesTSCollisionFlag(t *testing.T) {
	const tsCollisionFlagNull = 0x7FFFFFFF
	if FlagNull != tsCollisionFlagNull {
		t.Fatalf("FlagNull = %#x, want TS CollisionFlag.NULL %#x", FlagNull, tsCollisionFlagNull)
	}
	if FlagRoof != 1<<31 {
		t.Fatalf("FlagRoof = %#x, want 1<<31 (TS ROOF = 2147483648)", FlagRoof)
	}
	// FlagNull must carry the FlagRoof bit's complement and nothing above it:
	// it is exactly the low 31 bits, i.e. all flags except FlagRoof.
	if FlagNull&FlagRoof != 0 {
		t.Errorf("FlagNull (%#x) must NOT include FlagRoof bit (%#x)", FlagNull, FlagRoof)
	}
	// FlagNull together with FlagRoof spans the full 32-bit collision mask.
	if got := FlagNull | FlagRoof; got != 0xFFFFFFFF {
		t.Errorf("FlagNull | FlagRoof = %#x, want full 32-bit mask 0xFFFFFFFF", got)
	}
}

// TestFlagNullOffMapNotIndoors is the behavioral regression guard for L43:
// an off-map / unallocated tile (FlagMap.Get returns FlagNull) must NOT be
// reported as indoors, matching TS isIndoors which checks the ROOF bit that
// CollisionFlag.NULL deliberately leaves clear.
func TestFlagNullOffMapNotIndoors(t *testing.T) {
	if IsIndoors(FlagNull) {
		t.Errorf("IsIndoors(FlagNull=%#x) = true, want false (off-map is not indoors)", FlagNull)
	}
	// An empty FlagMap returns FlagNull for any tile; that tile is off-map,
	// hence not indoors.
	m := NewFlagMap()
	if got := m.Get(3200, 3200, 0); got != FlagNull {
		t.Fatalf("Get(unallocated) = %#x, want FlagNull %#x", got, FlagNull)
	}
	if IsIndoors(m.Get(3200, 3200, 0)) {
		t.Error("off-map tile reported as indoors")
	}
}

// TestFlagNullStillBlocksMovement confirms the value change preserves the
// other half of CollisionFlag.NULL's contract: every movement/wall/loc bit
// (bits 0..30) is still set, so off-map tiles remain impassable for all
// collision strategies, exactly as -1 did.
func TestFlagNullStillBlocksMovement(t *testing.T) {
	cases := []struct {
		name      string
		blockFlag int
		typ       Type
	}{
		{"normal-walk", FlagWalkBlocked, TypeNormal},
		{"normal-blockwalk", FlagBlockWalk, TypeNormal},
		// TypeBlocked strips FlagBlockWalk from the mask, so use a loc flag it
		// still honors; an off-map tile sets it and is impassable.
		{"blocked", FlagWalkBlocked, TypeBlocked},
		{"indoors", FlagBlockWalk, TypeIndoors},
		{"outdoors", FlagBlockWalk, TypeOutdoors},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if CanMove(FlagNull, tc.blockFlag, tc.typ) {
				t.Errorf("CanMove(FlagNull, %#x, %v) = true, want false (off-map blocks)", tc.blockFlag, tc.typ)
			}
		})
	}
}
