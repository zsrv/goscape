package worldmap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPack_NoMapsDir_NoOp(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// outDir/server/maps does not exist → TS parity early-return.
	if err := Pack(tmp, tmp); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "mapview", "worldmap.jag")); err == nil {
		t.Errorf("worldmap.jag created despite missing server/maps")
	}
}

// Coverage note: there is no unit test for "maps dir exists but is
// empty" because synthesising the type-loader inputs (flo/loc/npc
// dat + client jagfile), CSVs, fonts, sprites, and labels.txt for
// a goscape-only fixture is heavier than the path is worth in
// isolation. The env-gated TestPack_RealContent_Integration in
// Task 8 covers this end-to-end against real Content.

func TestPack_RealContent_Integration(t *testing.T) {
	if os.Getenv("GOSCAPE_WORLDMAP_INTEGRATION") != "1" {
		t.Skip("set GOSCAPE_WORLDMAP_INTEGRATION=1 to enable")
	}

	srcDir := os.Getenv("GOSCAPE_CONTENT_DIR")
	if srcDir == "" {
		srcDir = "/home/owner/Code/github.com/LostCityRS/content"
	}
	packDir := os.Getenv("GOSCAPE_PACK_DIR")
	if packDir == "" {
		t.Skip("set GOSCAPE_PACK_DIR to a directory containing server/maps/")
	}
	if _, err := os.Stat(filepath.Join(packDir, "server", "maps")); err != nil {
		t.Skipf("%s/server/maps missing: %v", packDir, err)
	}

	outDir := t.TempDir()
	if err := os.Symlink(filepath.Join(packDir, "server"), filepath.Join(outDir, "server")); err != nil {
		t.Fatalf("symlink server: %v", err)
	}
	if err := os.Symlink(filepath.Join(packDir, "client"), filepath.Join(outDir, "client")); err != nil {
		t.Fatalf("symlink client: %v", err)
	}

	if err := Pack(srcDir, outDir); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jagPath := filepath.Join(outDir, "mapview", "worldmap.jag")
	st, err := os.Stat(jagPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("output jag is empty")
	}

	jag, err := jagfile.LoadJagfile(jagPath)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}

	// f11-f30 font members re-added at rev-254 (TS Worldmap.ts:618-625
	// @ 2e3bcf43; they were removed at rev-244 / 9aadcec4).
	expectedNames := []string{
		"underlay.dat", "overlay.dat", "loc.dat", "obj.dat", "npc.dat",
		"multi.dat", "free.dat", "floorcol.dat",
		"mapscene.dat", "mapfunction.dat", "b12_full.dat",
		"f11.dat", "f12.dat", "f14.dat", "f17.dat",
		"f19.dat", "f22.dat", "f26.dat", "f30.dat",
		"mapdots.dat", "index.dat", "labels.dat",
	}
	for _, n := range expectedNames {
		p, err := jag.Read(n)
		if err != nil {
			t.Errorf("missing entry %q: %v", n, err)
			continue
		}
		switch n {
		case "underlay.dat", "overlay.dat", "loc.dat", "floorcol.dat", "labels.dat":
			if p.Length() == 0 {
				t.Errorf("entry %q is empty (expected non-zero)", n)
			}
		}
	}
}

func TestUnpackCoord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		level, x, z int
	}{
		{0, 0, 0},
		{0, 63, 63},
		{1, 30, 17},
		{3, 63, 63},
	} {
		packed := (tc.level << 12) | (tc.x << 6) | tc.z
		level, x, z := unpackCoord(packed)
		if level != tc.level || x != tc.x || z != tc.z {
			t.Errorf("unpackCoord(%#x) = (%d, %d, %d), want (%d, %d, %d)",
				packed, level, x, z, tc.level, tc.x, tc.z)
		}
	}
}

// TestPackWater_ByteLayout removed: packWater helper and all 16 call
// sites were commented out upstream at rev-244 (TS Worldmap.ts @ 9aadcec4)
// and deleted here. No test coverage needed for deleted code.

func TestProcessMap_EmptyLandFile_ProducesHeaderOnlyBytes(t *testing.T) {
	t.Parallel()

	land := packet2.Alloc(1)
	defer land.Release()
	for range 4 * 64 * 64 {
		land.P1(0)
	}

	loc := packet2.Alloc(1)
	defer loc.Release()
	loc.P1(0) // locIdOffset = 0 → loop exits immediately
	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	locTypes := &objtype.LocTypeConfigs{Configs: nil}
	npcTypes := &objtype.NPCTypeConfigs{Configs: nil}

	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: locTypes,
		npcTypes: npcTypes,
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 50, 50, 0, land, loc, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	if got, want := out.underlay.Length(), 2+4096; got != want {
		t.Errorf("underlay length = %d, want %d", got, want)
	}
	if got, want := out.overlay.Length(), 2+4096; got != want {
		t.Errorf("overlay length = %d, want %d", got, want)
	}
	if got, want := out.loc.Length(), 2+4096; got != want {
		t.Errorf("loc length = %d, want %d", got, want)
	}
	if got := out.obj.Length(); got != 0 {
		t.Errorf("obj length = %d, want 0 (empty input)", got)
	}
	if got := out.npc.Length(); got != 0 {
		t.Errorf("npc length = %d, want 0 (empty input)", got)
	}
}

// writeGSmartS writes v in the goscape GSmartS (= TS gsmarts) unsigned encoding:
// byte form if v < 0x80, otherwise 2-byte form (v | 0x8000).
func writeGSmartS(p *packet2.Packet, v int) {
	if v < 0x80 {
		p.P1(uint8(v))
	} else {
		p.P2(uint16(v) | 0x8000)
	}
}

func TestProcessMap_WallShapeDecoding(t *testing.T) {
	t.Parallel()

	land := packet2.Alloc(1)
	defer land.Release()
	for range 4 * 64 * 64 {
		land.P1(0)
	}

	type spec struct {
		shape  int
		angle  int
		active int
		x, z   int
		want   uint8 // expected walls byte
	}
	// All four locs at level=0 (the default selected level for mx=50, mz=50).
	// Each at a distinct (x, z) so they don't collide.
	locs := []spec{
		{shape: 0, angle: 2, active: 1, x: 1, z: 1, want: 7},  // WallStraight: 1+2+4
		{shape: 2, angle: 1, active: 0, x: 2, z: 2, want: 10}, // WallL: 9+1
		{shape: 4, angle: 3, active: 0, x: 3, z: 3, want: 20}, // WallDecorStraightNoOffset: 17+3
		{shape: 9, angle: 1, active: 0, x: 4, z: 4, want: 26}, // WallDiagonal: 25+(1%2)
	}

	locBuf := packet2.Alloc(1)
	defer locBuf.Release()
	prevId := -1
	for i, l := range locs {
		writeGSmartS(locBuf, i-prevId) // locId delta
		prevId = i
		coord := (0 << 12) | (l.x << 6) | l.z
		writeGSmartS(locBuf, coord+1)              // coordOffset; previous coord = 0
		locBuf.P1(uint8((l.shape << 2) | l.angle)) // info byte
		writeGSmartS(locBuf, 0)                    // coord inner-loop terminator
	}
	writeGSmartS(locBuf, 0) // locId outer-loop terminator

	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	locConfigs := []*objtype.LocType{
		{Active: 1, MapScene: -1, MapFunction: -1},
		{Active: 0, MapScene: -1, MapFunction: -1},
		{Active: 0, MapScene: -1, MapFunction: -1},
		{Active: 0, MapScene: -1, MapFunction: -1},
	}

	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{Configs: locConfigs},
		npcTypes: &objtype.NPCTypeConfigs{},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 50, 50, 0, land, locBuf, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	// Decode out.loc: header(2) + per-(x,z) slot. Each slot is 0..N
	// bytes terminated by a 0. We set no MapScene/MapFunction, so wall
	// slots have exactly (wall, 0); empty slots have just (0).
	out.loc.Pos = 2
	walls := make(map[[2]int]uint8)
	for x := range 64 {
		for z := range 64 {
			for {
				b := out.loc.G1()
				if b == 0 {
					break
				}
				walls[[2]int{x, z}] = b
			}
		}
	}

	for _, l := range locs {
		got, ok := walls[[2]int{l.x, l.z}]
		if !ok {
			t.Errorf("walls[%d,%d] missing; want %d", l.x, l.z, l.want)
			continue
		}
		if got != l.want {
			t.Errorf("walls[%d,%d] = %d, want %d (shape=%d angle=%d active=%d)",
				l.x, l.z, got, l.want, l.shape, l.angle, l.active)
		}
	}
}

func TestProcessMap_ObjPathEmitsPBoolMask(t *testing.T) {
	t.Parallel()

	land := packet2.Alloc(1)
	defer land.Release()
	for range 4 * 64 * 64 {
		land.P1(0)
	}
	locBuf := packet2.Alloc(1)
	defer locBuf.Release()
	locBuf.P1(0)
	npc := packet2.Alloc(1)
	defer npc.Release()

	// One obj entry at level=0, lx=5, lz=5.
	obj := packet2.Alloc(1)
	defer obj.Release()
	pos := (0 << 12) | (5 << 6) | 5
	obj.P2(uint16(pos))
	obj.P1(1)      // count
	obj.P2(0x1234) // objId
	obj.P1(1)      // objCount (discarded)

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{},
		npcTypes: &objtype.NPCTypeConfigs{},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 10, 10, 0, land, locBuf, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	if got, want := out.obj.Length(), 2+4096; got != want {
		t.Fatalf("obj length = %d, want %d", got, want)
	}
	out.obj.Pos = 0
	if h1, h2 := out.obj.G1(), out.obj.G1(); h1 != 10 || h2 != 10 {
		t.Errorf("obj header = (%d, %d), want (10, 10)", h1, h2)
	}
	for x := range 64 {
		for z := range 64 {
			got := out.obj.G1()
			wantTrue := x == 5 && z == 5
			if (got == 1) != wantTrue {
				t.Errorf("obj[%d,%d] = %d, want %v", x, z, got, wantTrue)
			}
		}
	}
}

func TestProcessMap_NpcPathEmitsPBoolMask(t *testing.T) {
	t.Parallel()

	land := packet2.Alloc(1)
	defer land.Release()
	for range 4 * 64 * 64 {
		land.P1(0)
	}
	locBuf := packet2.Alloc(1)
	defer locBuf.Release()
	locBuf.P1(0)
	obj := packet2.Alloc(1)
	defer obj.Release()

	// Two npc entries: id=5 with Minimap=true at (lx=10,lz=10),
	// id=7 with Minimap=false at (lx=20,lz=20). Only the former
	// should set its PBool bit.
	npc := packet2.Alloc(1)
	defer npc.Release()
	pos1 := (0 << 12) | (10 << 6) | 10
	npc.P2(uint16(pos1))
	npc.P1(1) // count
	npc.P2(5) // npcId
	pos2 := (0 << 12) | (20 << 6) | 20
	npc.P2(uint16(pos2))
	npc.P1(1) // count
	npc.P2(7) // npcId

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	npcConfigs := make([]*objtype.NpcType, 8)
	npcConfigs[5] = &objtype.NpcType{Minimap: true}
	npcConfigs[7] = &objtype.NpcType{Minimap: false}

	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{},
		npcTypes: &objtype.NPCTypeConfigs{Configs: npcConfigs},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 12, 13, 0, land, locBuf, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	if got, want := out.npc.Length(), 2+4096; got != want {
		t.Fatalf("npc length = %d, want %d", got, want)
	}
	out.npc.Pos = 0
	if h1, h2 := out.npc.G1(), out.npc.G1(); h1 != 12 || h2 != 13 {
		t.Errorf("npc header = (%d, %d), want (12, 13)", h1, h2)
	}
	for x := range 64 {
		for z := range 64 {
			got := out.npc.G1()
			wantTrue := x == 10 && z == 10 // (id=5, Minimap=true) only
			if (got == 1) != wantTrue {
				t.Errorf("npc[%d,%d] = %d, want %v", x, z, got, wantTrue)
			}
		}
	}
}

func TestProcessMap_MapScene22SkipsLoc(t *testing.T) {
	t.Parallel()

	land := packet2.Alloc(1)
	defer land.Release()
	for range 4 * 64 * 64 {
		land.P1(0)
	}

	// One loc at (level=0, x=5, z=5) with shape=0 (WallStraight),
	// angle=0. Its LocType has MapScene=22 → processMap continues
	// before emitting any wall/mapscene/mapfunction byte.
	locBuf := packet2.Alloc(1)
	defer locBuf.Release()
	writeGSmartS(locBuf, 1) // locId delta (locId: -1 → 0)
	coord := (0 << 12) | (5 << 6) | 5
	writeGSmartS(locBuf, coord+1) // coordOffset; previous coord = 0
	locBuf.P1(0)                  // info byte (shape=0, angle=0)
	writeGSmartS(locBuf, 0)       // coord inner-loop terminator
	writeGSmartS(locBuf, 0)       // locId outer-loop terminator

	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	locConfigs := []*objtype.LocType{
		{Active: 1, MapScene: 22, MapFunction: -1},
	}

	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{Configs: locConfigs},
		npcTypes: &objtype.NPCTypeConfigs{},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 50, 50, 0, land, locBuf, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	// header (2) + one terminator-0 per tile (4096) and no wall /
	// mapscene / mapfunction bytes — because MapScene==22 short-
	// circuits before any of those are recorded.
	if got, want := out.loc.Length(), 2+4096; got != want {
		t.Fatalf("loc length = %d, want %d", got, want)
	}
	out.loc.Pos = 2
	for x := range 64 {
		for z := range 64 {
			if b := out.loc.G1(); b != 0 {
				t.Fatalf("loc[%d,%d] body byte = %d, want 0", x, z, b)
			}
		}
	}
}

// TestProcessMap_UndergroundPassException254 pins the rev-254 change:
// the underground-pass override is RE-ADDED in new form (TS Worldmap.ts
// :109-113 + :185 @ 2e3bcf43; it was absent at 9aadcec4/244). Pack passes
// level=1 for mx==33 && mz 71..73; a non-bridged tile then reads
// underlayIds[0+1]: the level-1 underlay placed at (0,0) MUST surface
// in the output (the 244 contract pinned 0 here).
func TestProcessMap_UndergroundPassException254(t *testing.T) {
	t.Parallel()
	land := packet2.Alloc(1)
	defer land.Release()
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				if level == 1 && x == 0 && z == 0 {
					// Underlay opcode (>81) placed at level=1, tile (0,0).
					// Pre-244 this was the level used for underground-pass
					// squares; post-244 level-0 is used exclusively.
					land.P1(82) // underlay opcode (>81)
					land.P1(0)
				} else {
					land.P1(0)
				}
			}
		}
	}
	loc := packet2.Alloc(1)
	defer loc.Release()
	loc.P1(0)
	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{},
		npcTypes: &objtype.NPCTypeConfigs{},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}
	// level=1: what Pack computes for mx=33, mz=72 (Worldmap.ts:109-113).
	if err := processMap(ctx, out, 33, 72, 1, land, loc, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	// Non-bridged tile (0,0): actualLevel = (bridged?1:0)+level = 1, so the
	// level-1 underlay opcode 82 → id 82-81 = 1 must surface in the output.
	out.underlay.Pos = 2 // skip header
	if got := out.underlay.G1(); got != 1 {
		t.Errorf("underlay[0][0] = %d, want 1 (underground-pass level=1 re-added at 254, Worldmap.ts:185)", got)
	}
}
