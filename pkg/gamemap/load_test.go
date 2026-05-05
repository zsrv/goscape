package gamemap

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// mFileWithLand returns the bytes of an m-file where exactly one tile
// (level, localX, localZ) within mapsquare (mapX, mapZ) carries the given
// land value. All other tiles terminate with opcode 0 (no land).
//
// Encoding per loadGround's parser (load.go):
//   - opcode 0: terminate tile (no land set; lands[idx] stays 0)
//   - opcode (49+land): set land = opcode - 49; CONTINUES the tile loop
//   - opcode 0: terminate tile after the land opcode
//
// Each non-empty tile emits two bytes: the land opcode followed by a
// terminator (opcode 0), matching TS's while-loop semantics where opcodes
// 50..81 continue rather than break.
//
// Iteration order is level outer, x middle, z inner — matches loadGround.
// The packCoord index is the same as the parser's packCoord helper.
func mFileWithLand(targetLevel, targetX, targetZ int, land byte) []byte {
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if level == targetLevel && x == targetX && z == targetZ {
					buf.WriteByte(49 + land) // opcode encoding land (continues loop)
					buf.WriteByte(0)         // terminator
				} else {
					buf.WriteByte(0) // empty tile
				}
			}
		}
	}
	return buf.Bytes()
}

// mFileWithLands writes multiple (level,x,z,land) entries; all other tiles empty.
// Each non-empty tile emits a land opcode followed by a terminator (opcode 0).
func mFileWithLands(entries map[[3]int]byte) []byte {
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if land, ok := entries[[3]int{level, x, z}]; ok {
					buf.WriteByte(49 + land) // land opcode (continues loop)
					buf.WriteByte(0)         // terminator
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

// TestLoadGround_OverlayThenLand pins TS GameMap.ts:173-177 — opcode in
// 2..49 (overlay) consumes 1 trailing byte and continues; a subsequent
// opcode 50+ on the same tile sets lands.
func TestLoadGround_OverlayThenLand(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	// Build a stream where tile (1,1,0) has [overlay=10, overlayTag=0xAA, land=50, terminator=0].
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if level == 0 && x == localX && z == localZ {
					buf.WriteByte(10)   // overlay opcode (2..49)
					buf.WriteByte(0xAA) // overlay tag (1 byte skipped)
					buf.WriteByte(50)   // land=1 (BLOCK_MAP_SQUARE)
					buf.WriteByte(0)    // terminator
				} else {
					buf.WriteByte(0) // empty tile
				}
			}
		}
	}
	gm.loadGround(buf.Bytes(), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("overlay-then-land tile: flag=0x%x missing FlagBlockWalk; parser likely misaligned by overlay byte skip", flag)
	}
}

// TestLoadGround_ReservedOpcode_NoOp pins TS GameMap.ts:175 — opcodes
// 82..255 are no-ops within a tile (no lands write, no break).
func TestLoadGround_ReservedOpcode_NoOp(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	// Build a stream where tile (1,1,0) has [reserved=100, land=50, terminator=0].
	// Pre-fix Go: opcode=100 sets lands[idx]=51 then BREAKs, leaving int8 with bits
	//   that include BLOCK_MAP_SQUARE (51 & 1 == 1), but the test asserts the
	//   POST-fix behavior where opcode 100 is ignored and the subsequent opcode 50
	//   (land=1) sets the bit.
	// Post-fix Go: opcode=100 no-op continues; opcode=50 sets lands[idx]=1.
	var buf bytes.Buffer
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				if level == 0 && x == localX && z == localZ {
					buf.WriteByte(100) // reserved (>=82)
					buf.WriteByte(50)  // land=1 (BLOCK_MAP_SQUARE)
					buf.WriteByte(0)   // terminator
				} else {
					buf.WriteByte(0)
				}
			}
		}
	}
	gm.loadGround(buf.Bytes(), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("reserved-then-land tile: flag=0x%x missing FlagBlockWalk; reserved opcode may have terminated tile prematurely or set wrong land", flag)
	}
}

// TestLoadLocs_UsesLocTypeWidthLength pins the post-NAI-100 behavior:
// when SetLocTypes is called before loadLocs, the resulting *entity.Loc
// carries the LocType's W×L (not the legacy hardcoded 1×1).
func TestLoadLocs_UsesLocTypeWidthLength(t *testing.T) {
	const mapX, mapZ = 50, 50
	const locId = 42
	const shape = 10 // LayerGround centrepiece
	const angle = 0

	gm := newTestGameMap()

	cfgs := &objtype.LocTypeConfigs{
		Configs: make([]*objtype.LocType, 100),
	}
	cfgs.Configs[locId] = &objtype.LocType{Width: 2, Length: 3}
	gm.SetLocTypes(cfgs)

	data := lFileWithOneLoc(locId, 0, 0, 0, shape, angle)
	gm.loadLocs(data, mapX, mapZ)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("staticLocs: got %d, want 1", len(gm.staticLocs))
	}
	l := gm.staticLocs[0]
	if l.Type() != locId {
		t.Errorf("Type: got %d, want %d", l.Type(), locId)
	}
	if l.Width != 2 {
		t.Errorf("Width: got %d, want 2 (lt.Width)", l.Width)
	}
	if l.Length != 3 {
		t.Errorf("Length: got %d, want 3 (lt.Length)", l.Length)
	}
	wantX := mapX*mapSquareSize + 0
	wantZ := mapZ*mapSquareSize + 0
	if l.X != wantX || l.Z != wantZ || l.Level != 0 {
		t.Errorf("coords: got (%d,%d,%d), want (%d,%d,0)", l.X, l.Z, l.Level, wantX, wantZ)
	}
}

// TestLoadLocs_NilLocTypesFallback pins the test-fixture path:
// when SetLocTypes was never called, loadLocs falls back to 1×1 and
// does not log any "LocType" warnings (the warnings only fire when
// gm.locTypes != nil but the entry is missing/out-of-range).
func TestLoadLocs_NilLocTypesFallback(t *testing.T) {
	const mapX, mapZ = 50, 50
	const locId = 42
	const shape = 10
	const angle = 0

	gm := newTestGameMap()
	// Note: no SetLocTypes call.

	data := lFileWithOneLoc(locId, 0, 0, 0, shape, angle)
	gm.loadLocs(data, mapX, mapZ)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("staticLocs: got %d, want 1", len(gm.staticLocs))
	}
	l := gm.staticLocs[0]
	if l.Type() != locId {
		t.Errorf("Type: got %d, want %d", l.Type(), locId)
	}
	if l.Width != 1 {
		t.Errorf("Width: got %d, want 1 (nil-locTypes fallback)", l.Width)
	}
	if l.Length != 1 {
		t.Errorf("Length: got %d, want 1 (nil-locTypes fallback)", l.Length)
	}
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
