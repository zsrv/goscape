package worldmap

import (
	"os"
	"path/filepath"
	"testing"

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

func TestPackWater_ByteLayout(t *testing.T) {
	t.Parallel()

	flo := &objtype.FloTypeConfigs{
		ConfigNames: map[string]int{
			"muddygrass": 7,
			"water":      11,
		},
	}

	underlay := packet2.Alloc(1)
	defer underlay.Release()
	overlay := packet2.Alloc(1)
	defer overlay.Release()

	packWater(flo, underlay, overlay, 42, 56)

	if got, want := underlay.Length(), 2+4096; got != want {
		t.Errorf("underlay length = %d, want %d", got, want)
	}
	if got, want := overlay.Length(), 2+4096*2; got != want {
		t.Errorf("overlay length = %d, want %d", got, want)
	}

	underlay.Pos = 0
	if underlay.G1() != 42 || underlay.G1() != 56 {
		t.Errorf("underlay header bytes wrong")
	}
	for i := range 4096 {
		if got := underlay.G1(); got != 8 {
			t.Fatalf("underlay body byte %d = %d, want 8", i, got)
		}
	}

	overlay.Pos = 0
	if overlay.G1() != 42 || overlay.G1() != 56 {
		t.Errorf("overlay header bytes wrong")
	}
	for i := range 4096 {
		v := overlay.G1()
		z := overlay.G1()
		if v != 12 || z != 0 {
			t.Fatalf("overlay body pair %d = (%d, %d), want (12, 0)", i, v, z)
		}
	}
}

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

	if err := processMap(ctx, out, 50, 50, land, loc, obj, npc); err != nil {
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

// writeGSmart writes v in the goscape GSmart encoding (= TS gsmarts):
// byte form if v < 0x80, otherwise 2-byte form (v | 0x8000).
func writeGSmart(p *packet2.Packet, v int) {
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
		writeGSmart(locBuf, i-prevId) // locId delta
		prevId = i
		coord := (0 << 12) | (l.x << 6) | l.z
		writeGSmart(locBuf, coord+1)               // coordOffset; previous coord = 0
		locBuf.P1(uint8((l.shape << 2) | l.angle)) // info byte
		writeGSmart(locBuf, 0)                     // coord inner-loop terminator
	}
	writeGSmart(locBuf, 0) // locId outer-loop terminator

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

	if err := processMap(ctx, out, 50, 50, land, locBuf, obj, npc); err != nil {
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

	if err := processMap(ctx, out, 10, 10, land, locBuf, obj, npc); err != nil {
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

func TestProcessMap_UndergroundPassLevelOverride(t *testing.T) {
	t.Parallel()
	land := packet2.Alloc(1)
	defer land.Release()
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				if level == 1 && x == 0 && z == 0 {
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
	if err := processMap(ctx, out, 33, 72, land, loc, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	out.underlay.Pos = 2 // skip header
	if got := out.underlay.G1(); got != 1 {
		t.Errorf("underlay[0][0] = %d, want 1", got)
	}
}
