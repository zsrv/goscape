package worldmap

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

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
