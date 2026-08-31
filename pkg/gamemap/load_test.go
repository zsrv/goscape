package gamemap

import (
	"bytes"
	"fmt"
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
	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Default to a members world so load/collision parse-tests aren't gated
	// out by the F2P/members map gate (TS GameMap loadGround/loadLocs/loadNPCs).
	// F2P-gating tests override with gm.SetMembers(false) + setFreemapAt.
	gm.SetMembers(true)
	return gm
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
//	gsmarts(locDelta=locId+1)
//	gsmarts(coordDelta = packedCoord+1)
//	g1(info)
//	gsmarts(0)        // end of coords for this loc
//	gsmarts(0)        // end of locs
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

// --- NAI-151: static-obj parser tests ---

// objFileHeader builds the 3-byte record header for a single tile:
// G2(packed coord) + G1(tileCount).
// packed = (level<<12) | (localX<<6) | localZ.
func objFileHeader(level, localX, localZ, tileCount int) []byte {
	packed := (level << 12) | (localX << 6) | localZ
	return []byte{byte(packed >> 8), byte(packed & 0xFF), byte(tileCount)}
}

// objFileEntry builds a 3-byte obj entry: G2(typeID) + G1(count).
func objFileEntry(typeID, count int) []byte {
	return []byte{byte(typeID >> 8), byte(typeID & 0xFF), byte(count)}
}

// setFreemapAt flags the zone containing (x, z) as F2P. Mirrors the
// encoding used by gm.IsFreeToPlay → zoneIndex(x, z, 0).
func setFreemapAt(gm *GameMap, x, z int) {
	gm.freemap[zoneIndex(x, z, 0)] = true
}

// TestLoadNPCs_F2PGate pins H13: on a non-members world, NPC spawns on
// member-only tiles are gated out; F2P tiles load. Mirrors TS GameMap.ts:122-124.
func TestLoadNPCs_F2PGate(t *testing.T) {
	// One record at local (10,20) level 0, count=1, typeID=100.
	// packed = (0<<12)|(10<<6)|20 = 0x0294.
	nData := []byte{0x02, 0x94, 0x01, 0x00, 0x64}
	const mx, mz = 50, 50
	absX, absZ := mx*64+10, mz*64+20

	gm := newTestGameMap()
	gm.SetMembers(false) // non-members world, no F2P data → gated out
	gm.loadNPCs(nData, mx, mz)
	if got := len(gm.NpcSpawns()); got != 0 {
		t.Errorf("non-members world without F2P: got %d spawns, want 0", got)
	}

	gm2 := newTestGameMap()
	gm2.SetMembers(false)
	setFreemapAt(gm2, absX, absZ) // tile is F2P → spawn loads
	gm2.loadNPCs(nData, mx, mz)
	if got := len(gm2.NpcSpawns()); got != 1 {
		t.Errorf("F2P tile: got %d spawns, want 1", got)
	}
}

// TestBordersFreeToPlay pins the orthogonal-adjacency helper used by the
// loadGround/loadLocs gates (TS GameMap.ts:295-297). Uses a zone-boundary
// tile so x+1 crosses into a genuinely different (F2P) zone while the tile
// itself stays members-only.
func TestBordersFreeToPlay(t *testing.T) {
	gm := newTestGameMap()
	const x, z = 3215, 3220 // 3215>>3=401 (last tile of its zone); 3216>>3=402
	if gm.bordersFreeToPlay(x, z) {
		t.Fatal("no F2P data: bordersFreeToPlay must be false")
	}
	setFreemapAt(gm, x+1, z) // flag the east-adjacent zone (402)
	if gm.IsFreeToPlay(x, z) {
		t.Fatal("the tile's own zone must NOT be F2P (only the neighbour is)")
	}
	if !gm.bordersFreeToPlay(x, z) {
		t.Error("east-neighbour zone F2P: bordersFreeToPlay must be true")
	}
}

// objTypeConfigs builds an ObjTypeConfigs slice with len entries; each
// non-nil index has Members=members[i].
func objTypeConfigs(members map[int]bool) *objtype.ObjTypeConfigs {
	maxID := 0
	for id := range members {
		if id > maxID {
			maxID = id
		}
	}
	cfgs := make([]*objtype.ObjType, maxID+1)
	for id, m := range members {
		cfgs[id] = &objtype.ObjType{Members: m}
	}
	return &objtype.ObjTypeConfigs{Configs: cfgs}
}

func TestLoadObjs_Empty(t *testing.T) {
	gm := newTestGameMap()
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	gm.loadObjs([]byte{}, 0, 0)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("empty input: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_SingleTileSingleObj(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 10, 20
	const level = 0
	const typeID = 100
	const count = 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ

	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{typeID: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(level, localX, localZ, 1), objFileEntry(typeID, count)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 1 {
		t.Fatalf("got %d spawns, want 1", len(spawns))
	}
	got := spawns[0]
	want := ObjSpawn{TypeID: typeID, Count: count, X: absX, Z: absZ, Level: level}
	if got != want {
		t.Errorf("spawn: got %+v, want %+v", got, want)
	}
}

func TestLoadObjs_SingleTileMultiObj(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 10, 20
	absX, absZ := mapX*64+localX, mapZ*64+localZ

	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false, 2: false, 3: false}))
	setFreemapAt(gm, absX, absZ)

	data := objFileHeader(0, localX, localZ, 3)
	data = append(data, objFileEntry(1, 10)...)
	data = append(data, objFileEntry(2, 20)...)
	data = append(data, objFileEntry(3, 30)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 3 {
		t.Fatalf("got %d spawns, want 3", len(spawns))
	}
	for i, want := range []struct{ id, c int }{{1, 10}, {2, 20}, {3, 30}} {
		if spawns[i].TypeID != want.id || spawns[i].Count != want.c {
			t.Errorf("spawn[%d]: got TypeID=%d Count=%d, want TypeID=%d Count=%d",
				i, spawns[i].TypeID, spawns[i].Count, want.id, want.c)
		}
	}
}

func TestLoadObjs_MultiTile(t *testing.T) {
	const mapX, mapZ = 50, 50
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false, 2: false}))
	setFreemapAt(gm, mapX*64+5, mapZ*64+5)
	setFreemapAt(gm, mapX*64+10, mapZ*64+10)

	data := append(objFileHeader(0, 5, 5, 1), objFileEntry(1, 7)...)
	data = append(data, objFileHeader(0, 10, 10, 1)...)
	data = append(data, objFileEntry(2, 8)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 2 {
		t.Fatalf("got %d spawns, want 2", len(spawns))
	}
	if spawns[0].TypeID != 1 || spawns[0].X != mapX*64+5 || spawns[0].Z != mapZ*64+5 {
		t.Errorf("spawn[0]: got %+v", spawns[0])
	}
	if spawns[1].TypeID != 2 || spawns[1].X != mapX*64+10 || spawns[1].Z != mapZ*64+10 {
		t.Errorf("spawn[1]: got %+v", spawns[1])
	}
}

func TestLoadObjs_LevelEncoding(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 0, 0
	for level := range 4 {
		t.Run(fmt.Sprintf("level=%d", level), func(t *testing.T) {
			gm := newTestGameMap()
			gm.SetMembers(false)
			gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
			setFreemapAt(gm, mapX*64+localX, mapZ*64+localZ)

			data := append(objFileHeader(level, localX, localZ, 1), objFileEntry(1, 1)...)
			gm.loadObjs(data, mapX, mapZ)
			spawns := gm.ObjSpawns()
			if len(spawns) != 1 {
				t.Fatalf("got %d spawns, want 1", len(spawns))
			}
			if spawns[0].Level != level {
				t.Errorf("Level: got %d, want %d", spawns[0].Level, level)
			}
		})
	}
}

func TestLoadObjs_TruncatedTrailing(t *testing.T) {
	const mapX, mapZ = 50, 50
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, mapX*64+5, mapZ*64+5)

	// Header claims 2 entries but only 1 entry's bytes follow.
	data := append(objFileHeader(0, 5, 5, 2), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 1 {
		t.Fatalf("truncated: got %d spawns, want 1 (loop should stop at p.Len()<3)", len(spawns))
	}
}

func TestLoadObjs_TileGate_F2POnlyServer_NonF2PTile_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	gm := newTestGameMap()
	gm.SetMembers(false) // F2P-only server
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	// NOTE: do NOT call setFreemapAt — tile is non-F2P.

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("F2P-only server, members tile: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TileGate_MembersWorld_NonF2PTile_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	gm := newTestGameMap()
	gm.SetMembers(true) // members world
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	// NOTE: tile is non-F2P, but members=true bypasses tile gate.

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("members world, non-F2P tile: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TileGate_F2POnlyServer_F2PTile_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("F2P server + F2P tile: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_F2POnlyServer_MembersObj_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: true})) // members-only obj
	setFreemapAt(gm, absX, absZ)                          // F2P tile passes tile gate

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("F2P server + members-only obj: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_MembersWorld_MembersObj_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: true}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("members world + members obj: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_F2POnlyServer_NonMembersObj_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("F2P server + non-members obj: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypesNil_DropsAll(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	// NOTE: SetObjTypes NOT called — objTypes==nil
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("nil objTypes: got %d spawns, want 0 (nil-guard)", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TypeIDOutOfRange_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{0: false})) // len(Configs)=1
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(99, 7)...) // typeID=99 > len-1
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("typeID OOB: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

// mFileAllEmpty returns the bytes of an m-file where every tile is
// empty (opcode 0 terminator only — no land, no roof, no block).
func mFileAllEmpty() []byte {
	var buf bytes.Buffer
	for range mapLevels {
		for range mapSquareSize {
			for range mapSquareSize {
				buf.WriteByte(0)
			}
		}
	}
	return buf.Bytes()
}

// TestLoadGround_EmptyZones_AreAllocatedForOpenMovement reproduces the
// "vertical line in mapsquare 38_52" bug: a mapsquare with no
// BLOCK_MAP_SQUARE / REMOVE_ROOFS flags (i.e., entirely open ground)
// must leave every tile walkable, not "all blocked".
//
// Without a per-zone AllocateIfAbsent matching TS GameMap.ts:193-196,
// zones whose 64 tiles all carry no flag stay unallocated, and
// FlagMap.Get returns FlagNull=0x7FFFFFFF (every movement bit set) — which the
// StepValidator treats as fully blocked. Symptom in-game: at the
// boundary between an allocated zone (containing the player) and an
// adjacent unallocated zone, the player cannot move across the
// boundary; teleported into the unallocated zone, all 8 directions
// are blocked.
//
// Mirrors TS GameMap.ts:193-196 (`x % 7 === 0 && z % 7 === 0` hits
// every 8x8 zone at least once).
func TestLoadGround_EmptyZones_AreAllocatedForOpenMovement(t *testing.T) {
	const mapX, mapZ = 38, 52

	gm := newTestGameMap()
	gm.loadGround(mFileAllEmpty(), mapX, mapZ)

	// Every zone in every level should be allocated even though no tile
	// flag triggered an explicit ChangeFloor / ChangeRoof.
	for level := range mapLevels {
		for zx := range 8 {
			for zz := range 8 {
				absX := mapX*mapSquareSize + zx*8
				absZ := mapZ*mapSquareSize + zz*8
				if !gm.Pathfinder.Flags.IsZoneAllocated(absX, absZ, level) {
					t.Errorf("zone (zx=%d,zz=%d,level=%d) not allocated", zx, zz, level)
				}
			}
		}
	}

	// Source: localX=55,localZ=19 → zone (6,2). Dest: localX=56,localZ=19 →
	// zone (7,2). The 55→56 step crosses an internal zone boundary —
	// exactly the user-reported "vertical line" case.
	srcX := mapX*mapSquareSize + 55
	srcZ := mapZ*mapSquareSize + 19
	if !gm.CanTravel(0, srcX, srcZ, 1, 0, 1, 0, collision.TypeNormal) {
		t.Errorf("cannot move east from (%d,%d) → (%d,%d) in all-empty mapsquare",
			srcX, srcZ, srcX+1, srcZ)
	}

	// Teleport-east case: from inside the formerly-unallocated zone,
	// every direction should be open.
	teleX := mapX*mapSquareSize + 60
	teleZ := mapZ*mapSquareSize + 19
	for _, d := range []struct {
		name       string
		offX, offZ int
	}{
		{"east", 1, 0}, {"west", -1, 0}, {"north", 0, 1}, {"south", 0, -1},
	} {
		if !gm.CanTravel(0, teleX, teleZ, d.offX, d.offZ, 1, 0, collision.TypeNormal) {
			t.Errorf("cannot move %s from (%d,%d) after teleport into empty zone",
				d.name, teleX, teleZ)
		}
	}
}

func TestLoadObjs_TypeIDNilEntry_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	// Configs[1] = nil
	cfgs := &objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{nil, nil}}
	gm.SetObjTypes(cfgs)
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("nil Configs entry: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}
