package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestIsMapBlocked_WalkBlockedMask pins worldVarsView.IsMapBlocked to the TS
// GameMap.isMapBlocked contract:
//
//	isFlagged(x, z, level, CollisionFlag.WALK_BLOCKED)   (GameMap.ts)
//
// rsmod CollisionFlag.WALK_BLOCKED = 2359552 = 0x240100 =
// LOC | FLOOR | FLOOR_DECORATION = goscape FlagWalkBlocked. It is NOT the lone
// FLOOR bit goscape calls FlagBlockWalk (0x200000). A tile blocked solely by a
// loc — e.g. a BlockWalk ground loc such as a Lumbridge courtyard bush, which
// writes only FlagLoc (0x100) — must report as map-blocked. Otherwise
// MAP_FINDSQUARE (teleport radius-2 scatter, all standard spellbook teleports)
// accepts the bush tile and drops the player onto it, which the TS engine never
// does.
//
// The off-map case pins the second half of the contract: TS isMapBlocked routes
// through isFlagged, which short-circuits an unallocated tile to false (rsmod:
// current == NULL → false; dee467c8 CollisionEngine.isFlagged: !!zone). goscape
// must therefore route through FlagMap.IsFlagged, not a raw Get()&mask read
// (FlagMap.Get returns FlagNull = 0x7FFFFFFF off-map, whose bits would otherwise
// read as blocked).
//
// Verified faithful against every revision pin: GameMap.ts isMapBlocked uses
// CollisionFlag.WALK_BLOCKED at e1dea19f / 9aadcec4 / 3c16994c / 2e3bcf43 /
// dee467c8 (rev-225 / 244 / 245.2 / 254 / 274).
func TestIsMapBlocked_WalkBlockedMask(t *testing.T) {
	const level, x, z = 0, 3221, 3218 // Lumbridge teleport destination tile

	cases := []struct {
		name     string
		flag     int
		allocate bool
		want     bool
	}{
		{"open tile is not map-blocked", collision.FlagOpen, true, false},
		{"loc-blocked tile (bush) is map-blocked", collision.FlagLoc, true, true},
		{"ground-decor-blocked tile is map-blocked", collision.FlagGroundDecor, true, true},
		{"floor-blocked tile is map-blocked", collision.FlagBlockWalk, true, true},
		{"off-map (unallocated) tile is not map-blocked", collision.FlagOpen, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.gamemap = gamemap.New(discardLogger())
			if tc.allocate {
				s.gamemap.Pathfinder.Flags.AllocateIfAbsent(x, z, level)
				if tc.flag != collision.FlagOpen {
					s.gamemap.Pathfinder.Flags.Add(x, z, level, tc.flag)
				}
			}

			got := worldVarsView{s: s}.IsMapBlocked(level, x, z)
			if got != tc.want {
				t.Errorf("IsMapBlocked(flag=0x%x, allocated=%v) = %v, want %v (TS WALK_BLOCKED=0x%x)",
					tc.flag, tc.allocate, got, tc.want, collision.FlagWalkBlocked)
			}
		})
	}
}

// TestIsIndoors_RoofFlag pins worldVarsView.IsIndoors to the TS isIndoors
// contract = isFlagged(x, z, level, CollisionFlag.ROOF) (GameMap.ts). Like
// IsMapBlocked it routes through FlagMap.IsFlagged, so an off-map/unallocated
// tile short-circuits to false. (Raw Get()&FlagRoof happens to agree off-map
// because FlagNull = 0x7FFFFFFF excludes bit 31, but the IsFlagged path makes
// the contract explicit and uniform with IsMapBlocked.)
func TestIsIndoors_RoofFlag(t *testing.T) {
	const level, x, z = 0, 3221, 3218

	cases := []struct {
		name     string
		flag     int
		allocate bool
		want     bool
	}{
		{"no roof flag is not indoors", collision.FlagOpen, true, false},
		{"roof flag is indoors", collision.FlagRoof, true, true},
		{"off-map (unallocated) tile is not indoors", collision.FlagOpen, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.gamemap = gamemap.New(discardLogger())
			if tc.allocate {
				s.gamemap.Pathfinder.Flags.AllocateIfAbsent(x, z, level)
				if tc.flag != collision.FlagOpen {
					s.gamemap.Pathfinder.Flags.Add(x, z, level, tc.flag)
				}
			}

			got := worldVarsView{s: s}.IsIndoors(x, z, level)
			if got != tc.want {
				t.Errorf("IsIndoors(flag=0x%x, allocated=%v) = %v, want %v (TS ROOF=0x%x)",
					tc.flag, tc.allocate, got, tc.want, collision.FlagRoof)
			}
		})
	}
}
