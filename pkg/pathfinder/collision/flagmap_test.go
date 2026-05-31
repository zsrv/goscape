package collision

import "testing"

func TestAllocateIfAbsentSetAndGetFlag(t *testing.T) {
	m := NewFlagMap()
	m.AllocateIfAbsent(0, 0, 0)
	flags := m.flags[0]
	if flags == nil {
		t.Fatal("flags should not be nil")
	}
	flags[0] = 123456
	if flags[0] != m.Get(0, 0, 0) {
		t.Fatalf("flags[0] == %v, expected %v", flags[0], 123456)
	}
}

func TestIsZoneAllocatedGetDefaultFlagFromNonAllocatedZone(t *testing.T) {
	m := NewFlagMap()
	if m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone should not be allocated")
	}
	for x := 3200; x < 3208; x++ {
		for z := 3200; z < 3208; z++ {
			if m.Get(x, z, 0) != FlagNull {
				t.Fatalf("m.Get(%d, %d, 0) == %d, expected %d",
					x, z, m.Get(x, z, 0), FlagNull)
			}
		}
	}
}

func TestGetEmptyFlagFromAllocatedZone(t *testing.T) {
	m := NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)
	if !m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone should be allocated")
	}
	for x := 3200; x < 3208; x++ {
		for z := 3200; z < 3208; z++ {
			if m.Get(x, z, 0) != FlagOpen {
				t.Fatalf("m.Get(%d, %d, 0) == %d, expected %d",
					x, z, m.Get(x, z, 0), FlagOpen)
			}
		}
	}
}

func TestSetFlagsOnDifferentPlanes(t *testing.T) {
	m := NewFlagMap()

	if m.Get(3200, 3200, 0) != FlagNull {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), FlagNull)
	}
	if m.Get(3200, 3200, 1) != FlagNull {
		t.Fatalf("m.Get(3200, 3200, 1) == %d, expected %d",
			m.Get(3200, 3200, 1), FlagNull)
	}
	if m.Get(3200, 3200, 2) != FlagNull {
		t.Fatalf("m.Get(3200, 3200, 2) == %d, expected %d",
			m.Get(3200, 3200, 2), FlagNull)
	}

	m.Set(3200, 3200, 0, 0x800)
	m.Set(3200, 3200, 1, 0x200)
	m.Set(3200, 3200, 2, 0)

	if m.Get(3200, 3200, 0) != 0x800 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x800)
	}
	if m.Get(3200, 3200, 1) != 0x200 {
		t.Fatalf("m.Get(3200, 3200, 1) == %d, expected %d",
			m.Get(3200, 3200, 1), 0x200)
	}
	if m.Get(3200, 3200, 2) != 0 {
		t.Fatalf("m.Get(3200, 3200, 2) == %d, expected %d",
			m.Get(3200, 3200, 2), 0)
	}
}

func TestAddOntoExistingCoordinateFlags(t *testing.T) {
	m := NewFlagMap()

	m.AllocateIfAbsent(3200, 3200, 0)
	if m.Get(3200, 3200, 0) != FlagOpen {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), FlagOpen)
	}

	m.Add(3200, 3200, 0, 0x1000)
	if m.Get(3200, 3200, 0) != 0x1000 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x1000)
	}

	// add another collision bitflag
	m.Add(3200, 3200, 0, 0x400)

	// test that the initial bitflag has not been reset
	if m.Get(3200, 3200, 0)&0x1000 == 0 {
		t.Fatalf("m.Get(3200, 3200, 0) & 0x1000 == %d, expected non-zero",
			m.Get(3200, 3200, 0)&0x1000)
	}

	// test that the new bitflag has been set
	if m.Get(3200, 3200, 0)&0x400 == 0 {
		t.Fatalf("m.Get(3200, 3200, 0) & 0x400 == %d, expected non-zero",
			m.Get(3200, 3200, 0)&0x400)
	}

	// other tiles in the zone should not be affected
	for z := 3201; z < 3208; z++ {
		for x := 3201; x < 3208; x++ {
			if m.Get(x, z, 0) != FlagOpen {
				t.Fatalf("m.Get(%d, %d, 0) == %d, expected %d",
					x, z, m.Get(x, z, 0), FlagOpen)
			}
		}
	}
}

func TestAddFlagToUnallocatedZone(t *testing.T) {
	m := NewFlagMap()

	if m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone should not be allocated")
	}

	m.Add(3200, 3200, 0, 0x100)
	if m.Get(3200, 3200, 0) != 0x100 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x100)
	}
}

func TestRemoveSingleFlag(t *testing.T) {
	m := NewFlagMap()

	m.AllocateIfAbsent(3200, 3200, 0)

	m.Set(3200, 3200, 0, 0x1000)
	if m.Get(3200, 3200, 0) != 0x1000 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x1000)
	}

	m.Remove(3200, 3200, 0, 0x1000)
	if m.Get(3200, 3200, 0) != 0 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0)
	}
}

func TestRemoveSeparateFlags(t *testing.T) {
	m := NewFlagMap()

	m.AllocateIfAbsent(3200, 3200, 0)
	if m.Get(3200, 3200, 0) != 0 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0)
	}

	m.Add(3200, 3200, 0, 0x1000)
	m.Add(3200, 3200, 0, 0x400)

	if m.Get(3200, 3200, 0)&0x1000 == 0 {
		t.Fatalf("m.Get(3200, 3200, 0) & 0x1000 == %d, expected non-zero",
			m.Get(3200, 3200, 0)&0x1000)
	}
	if m.Get(3200, 3200, 0)&0x400 == 0 {
		t.Fatalf("m.Get(3200, 3200, 0) & 0x400 == %d, expected non-zero",
			m.Get(3200, 3200, 0)&0x400)
	}

	m.Remove(3200, 3200, 0, 0x1000)
	if m.Get(3200, 3200, 0) != 0x400 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x400)
	}

	m.Remove(3200, 3200, 0, 0x100)
	if m.Get(3200, 3200, 0) != 0x400 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0x400)
	}

	m.Remove(3200, 3200, 0, 0x400)
	if m.Get(3200, 3200, 0) != 0 {
		t.Fatalf("m.Get(3200, 3200, 0) == %d, expected %d",
			m.Get(3200, 3200, 0), 0)
	}
}

func TestAllocateSingleZone(t *testing.T) {
	m := NewFlagMap()

	if m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should not be allocated")
	}

	m.AllocateIfAbsent(3200, 3200, 0)
	if !m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should be allocated")
	}

	// test that neighboring zones did not get allocated
	if m.IsZoneAllocated(3192, 3192, 0) {
		t.Fatal("zone 3192, 3192 should not be allocated")
	}
	if m.IsZoneAllocated(3192, 3208, 0) {
		t.Fatal("zone 3192, 3208 should not be allocated")
	}
	if m.IsZoneAllocated(3208, 3208, 0) {
		t.Fatal("zone 3208, 3208 should not be allocated")
	}
	if m.IsZoneAllocated(3208, 3192, 0) {
		t.Fatal("zone 3208, 3192 should not be allocated")
	}
}

func TestDeallocateSingleZoneIfPresent(t *testing.T) {
	m := NewFlagMap()

	m.AllocateIfAbsent(3200, 3200, 0)
	if !m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should be allocated")
	}

	// test that deallocating neighboring zones won't affect the previous zone
	m.DeallocateIfPresent(3208, 3208, 0)
	if !m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should still be allocated after deallocating zone 3208, 3208")
	}
	m.DeallocateIfPresent(3196, 3196, 0)
	if !m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should still be allocated after deallocating zone 3196, 3196")
	}
	m.DeallocateIfPresent(3202, 3202, 0)
	if m.IsZoneAllocated(3200, 3200, 0) {
		t.Fatal("zone 3200, 3200 should not be allocated after deallocating zone 3202, 3202")
	}
}

// TestIsFlagged_OffMapTileReportsUnflagged pins pathfinder-3. Mirrors rsmod
// CollisionFlagMap::isFlagged (Engine/rsmod/collision.rs:58-65): on a tile
// whose Get returns FlagNull (unallocated zone / off-map), IsFlagged must
// return false regardless of the queried flag mask. Pre-fix RED because
// (FlagNull & flags) != FlagOpen evaluates to true for every non-zero
// mask, so every off-map probe reported "flagged" (= blocked) — the
// inverse of the canonical contract.
func TestIsFlagged_OffMapTileReportsUnflagged(t *testing.T) {
	m := NewFlagMap()

	// Sanity precondition: the tile is genuinely off-map.
	if got := m.Get(3200, 3200, 0); got != FlagNull {
		t.Fatalf("Get(off-map) = %#x, want FlagNull %#x", got, FlagNull)
	}

	cases := []struct {
		name  string
		flags int
	}{
		{"FlagWalkBlocked", FlagWalkBlocked},
		{"FlagBlockWalk", FlagBlockWalk},
		{"FlagWallWest", FlagWallWest},
		{"FlagLoc", FlagLoc},
		// Combined masks (LoS / LoW probes use these).
		{"FlagWalkBlocked|FlagBlockWalk", FlagWalkBlocked | FlagBlockWalk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if m.IsFlagged(3200, 3200, 0, tc.flags) {
				t.Errorf("IsFlagged(off-map, %s=%#x) = true, want false "+
					"(rsmod CollisionFlagMap::isFlagged short-circuits FlagNull → false)",
					tc.name, tc.flags)
			}
		})
	}
}

// TestIsFlagged_AllocatedOpenTileReportsUnflagged is the foil: an
// ALLOCATED zone with FlagOpen tiles must also report unflagged.
// Pre-fix this already worked; post-fix it must still work (the FlagNull
// short-circuit doesn't affect allocated tiles).
func TestIsFlagged_AllocatedOpenTileReportsUnflagged(t *testing.T) {
	m := NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)

	if got := m.Get(3200, 3200, 0); got != FlagOpen {
		t.Fatalf("Get(allocated-open) = %#x, want FlagOpen %#x", got, FlagOpen)
	}
	if m.IsFlagged(3200, 3200, 0, FlagWalkBlocked) {
		t.Errorf("IsFlagged(allocated-open, FlagWalkBlocked) = true, want false")
	}
}

// TestIsFlagged_AllocatedFlaggedTileReportsFlagged: the production
// happy-path. An allocated tile with the queried flag bit set reports
// flagged. Guards against an over-broad fix that accidentally returns
// false for all allocated tiles.
func TestIsFlagged_AllocatedFlaggedTileReportsFlagged(t *testing.T) {
	m := NewFlagMap()
	m.Add(3200, 3200, 0, FlagWallWest)

	if !m.IsFlagged(3200, 3200, 0, FlagWallWest) {
		t.Errorf("IsFlagged(WallWest-tile, FlagWallWest) = false, want true")
	}
	// A different flag must still report unflagged on the same tile.
	if m.IsFlagged(3200, 3200, 0, FlagWalkBlocked) {
		t.Errorf("IsFlagged(WallWest-tile, FlagWalkBlocked) = true, want false")
	}
}

func TestIfZoneAllocatedIsTrueForAllCoordinatesInZoneGrid(t *testing.T) {
	m := NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)

	for z := 3200; z < 3208; z++ {
		for x := 3200; x < 3208; x++ {
			if !m.IsZoneAllocated(x, z, 0) {
				t.Fatalf("IsZoneAllocated(%d, %d, %d) == %b, want true",
					x, z, 0, m.Get(x, z, 0))
			}
		}
	}

	for z := 3192; z < 3200; z++ {
		for x := 3192; x < 3200; x++ {
			if m.IsZoneAllocated(x, z, 0) {
				t.Fatalf("IsZoneAllocated(%d, %d, %d) == %b, want false",
					x, z, 0, m.Get(x, z, 0))
			}
		}
	}

	for z := 3208; z < 3216; z++ {
		for x := 3208; x < 3216; x++ {
			if m.IsZoneAllocated(x, z, 0) {
				t.Fatalf("IsZoneAllocated(%d, %d, %d) == %b, want false",
					x, z, 0, m.Get(x, z, 0))
			}
		}
	}
}
