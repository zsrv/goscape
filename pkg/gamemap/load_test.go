package gamemap

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// mFileWithLand returns the bytes of an m-file where exactly one tile
// (level, localX, localZ) within mapsquare (mapX, mapZ) carries the given
// land value. All other tiles terminate with opcode 0 (no land).
//
// Encoding per loadGround's parser (load.go):
//   - opcode 0: terminate tile (no land set; lands[idx] stays 0)
//   - opcode 50+land: terminate tile with land = opcode - 49
//
// Iteration order is level outer, x middle, z inner — matches loadGround.
// The packCoord index is the same as the parser's packCoord helper.
func mFileWithLand(targetLevel, targetX, targetZ int, land byte) []byte {
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if level == targetLevel && x == targetX && z == targetZ {
					buf.WriteByte(49 + land) // opcode encoding land
				} else {
					buf.WriteByte(0) // empty tile
				}
			}
		}
	}
	return buf.Bytes()
}

// mFileWithLands writes multiple (level,x,z,land) entries; all other tiles empty.
func mFileWithLands(entries map[[3]int]byte) []byte {
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if land, ok := entries[[3]int{level, x, z}]; ok {
					buf.WriteByte(49 + land)
				} else {
					buf.WriteByte(0)
				}
			}
		}
	}
	return buf.Bytes()
}

func newTestGameMap() *GameMap {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestLoadGround_BlockMapSquare_WritesFloorBlock pins the BLOCK_MAP_SQUARE constant
// flip (NAI-96): a tile with land=0x1 must mark FlagBlockWalk via ChangeFloor.
// Pre-fix (gameMapBlockMapSquare=0x2): land=0x1 is silently ignored.
func TestLoadGround_BlockMapSquare_WritesFloorBlock(t *testing.T) {
	const mapX, mapZ = 50, 50 // arbitrary mapsquare; absolute = (mapX*64+x, mapZ*64+z)
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	// Pre-allocate the touched zone so flag reads return real values, not FlagNull.
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x1), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("tile (%d, %d, %d) land=0x1: flag=0x%x missing FlagBlockWalk (0x%x)",
			absX, absZ, targetLevel, flag, collision.FlagBlockWalk)
	}
}

// TestLoadGround_RemoveRoofs_WritesRoof pins the REMOVE_ROOFS=0x4 →
// Pathfinder.ChangeRoof write per TS GameMap.ts:200-202.
func TestLoadGround_RemoveRoofs_WritesRoof(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x4), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagRoof == 0 {
		t.Errorf("tile (%d, %d, %d) land=0x4: flag=0x%x missing FlagRoof (0x%x)",
			absX, absZ, targetLevel, flag, collision.FlagRoof)
	}
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("tile (%d, %d, %d) land=0x4: flag=0x%x unexpectedly has FlagBlockWalk",
			absX, absZ, targetLevel, flag)
	}
}

// TestLoadGround_BlockAndRemoveRoofs_BothWritten pins that land=0x5
// (BLOCK_MAP_SQUARE | REMOVE_ROOFS) writes both flags.
func TestLoadGround_BlockAndRemoveRoofs_BothWritten(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x5), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagRoof == 0 {
		t.Errorf("flag=0x%x missing FlagRoof", flag)
	}
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("flag=0x%x missing FlagBlockWalk", flag)
	}
}

// TestLoadGround_BridgedLevel0_DropsToLevelMinus1_Skipped pins that a level-0
// BLOCK_MAP_SQUARE with the level-1 land carrying LINK_BELOW becomes
// actualLevel=-1 and is silently skipped (TS GameMap.ts:208-212).
func TestLoadGround_BridgedLevel0_DropsToLevelMinus1_Skipped(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	// Level 0: BLOCK_MAP_SQUARE only; level 1 same (x,z): LINK_BELOW only.
	data := mFileWithLands(map[[3]int]byte{
		{0, localX, localZ}: 0x1,
		{1, localX, localZ}: 0x2,
	})
	gm.loadGround(data, mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("level 0 bridged tile: flag=0x%x unexpectedly has FlagBlockWalk (actualLevel=-1 should skip)", flag)
	}
}

// TestLoadGround_BridgedLevel1_WritesAtLevel0 pins TS GameMap.ts:208 — when
// level=1 land has both BLOCK_MAP_SQUARE and LINK_BELOW, write at level 0.
func TestLoadGround_BridgedLevel1_WritesAtLevel0(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 1)

	// Level 1: BLOCK_MAP_SQUARE | LINK_BELOW.
	data := mFileWithLand(1, localX, localZ, 0x3)
	gm.loadGround(data, mapX, mapZ)

	flag0 := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	flag1 := gm.Pathfinder.Flags.Get(absX, absZ, 1)
	if flag0&collision.FlagBlockWalk == 0 {
		t.Errorf("level 0 (post-bridge target): flag=0x%x missing FlagBlockWalk", flag0)
	}
	if flag1&collision.FlagBlockWalk != 0 {
		t.Errorf("level 1 (bridged origin): flag=0x%x unexpectedly has FlagBlockWalk", flag1)
	}
}

// TestLoadGround_NonBridgedLevel1_WritesAtLevel1 pins the inverse: level=1
// BLOCK_MAP_SQUARE without LINK_BELOW writes at level 1 (no bridge drop).
func TestLoadGround_NonBridgedLevel1_WritesAtLevel1(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 1)

	data := mFileWithLand(1, localX, localZ, 0x1) // BLOCK only, no LINK_BELOW
	gm.loadGround(data, mapX, mapZ)

	flag1 := gm.Pathfinder.Flags.Get(absX, absZ, 1)
	flag0 := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag1&collision.FlagBlockWalk == 0 {
		t.Errorf("level 1 (non-bridged): flag=0x%x missing FlagBlockWalk", flag1)
	}
	if flag0&collision.FlagBlockWalk != 0 {
		t.Errorf("level 0 (no bridge): flag=0x%x unexpectedly has FlagBlockWalk", flag0)
	}
}

// TestLoadGround_LinkBelowOnly_DoesNotBlock pins that LINK_BELOW alone (no
// BLOCK_MAP_SQUARE) does not write FlagBlockWalk. Confirms the constant flip
// distinguishes the two bits per TS GameMap.ts:204.
func TestLoadGround_LinkBelowOnly_DoesNotBlock(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	gm.loadGround(mFileWithLand(0, localX, localZ, 0x2), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("LINK_BELOW-only tile: flag=0x%x unexpectedly has FlagBlockWalk", flag)
	}
}

// lFileWithOneLoc returns the bytes of an l-file placing one loc.
//   - locId is the absolute loc id (delta = locId+1, since prevId starts at -1)
//   - level/localX/localZ encode coord
//   - shape, angle pack into info = (shape<<2)|angle
//
// Stream shape (per loadLocs):
//
//	gsmart(locDelta=locId+1)
//	gsmart(coordDelta = packedCoord+1)
//	g1(info)
//	gsmart(0)        // end of coords for this loc
//	gsmart(0)        // end of locs
func lFileWithOneLoc(locId, level, localX, localZ, shape, angle int) []byte {
	pw := packet.NewPacket(nil)
	pw.PSmart(int32(locId + 1))
	packed := (localZ & 0x3F) | ((localX & 0x3F) << 6) | ((level & 0x3) << 12)
	pw.PSmart(int32(packed + 1))
	pw.P1(uint8((shape << 2) | (angle & 0x3)))
	pw.PSmart(0) // end coords
	pw.PSmart(0) // end locs
	return pw.Data
}

// TestLoadLocs_BridgedLoc_PlacedAtActualLevel pins that a loc with the
// LINK_BELOW bit set on its corresponding lands tile is downshifted by one
// level on the staticLocs entity (TS GameMap.ts:242-246).
func TestLoadLocs_BridgedLoc_PlacedAtActualLevel(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const locId = 0
	const shape = int(loc.ShapeCentrepieceStraight) // LayerGround
	const angle = int(loc.AngleNorth)

	gm := newTestGameMap()

	// loadGround populates landsByMapSquare with level 1 LINK_BELOW set at (1,1,1).
	mData := mFileWithLand(1, localX, localZ, 0x2)
	gm.loadGround(mData, mapX, mapZ)

	// loadLocs places the loc at level 1 (request level), but lands[(1,1,1)]
	// has LINK_BELOW set, so actualLevel = 0.
	lData := lFileWithOneLoc(locId, 1, localX, localZ, shape, angle)
	gm.loadLocs(lData, mapX, mapZ)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("expected 1 static loc; got %d", len(gm.staticLocs))
	}
	got := gm.staticLocs[0]
	if got.Level != 0 {
		t.Errorf("bridged loc: level=%d, want 0 (actualLevel = level-1)", got.Level)
	}
}
