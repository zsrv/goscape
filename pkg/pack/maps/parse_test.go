package maps

import (
	"reflect"
	"testing"
)

// TestPackKey pins (level << 12) | (x << 6) | z encoding from TS
// map/Pack.js:13-15.
func TestPackKey(t *testing.T) {
	tests := []struct {
		level, x, z int
		want        int
	}{
		{0, 0, 0, 0},
		{1, 0, 0, 4096},
		{0, 1, 0, 64},
		{0, 0, 1, 1},
		{3, 0x3f, 0x3f, (3 << 12) | (0x3f << 6) | 0x3f},
	}
	for _, tc := range tests {
		got := packKey(tc.level, tc.x, tc.z)
		if got != tc.want {
			t.Errorf("packKey(%d,%d,%d) = %d, want %d", tc.level, tc.x, tc.z, got, tc.want)
		}
	}
}

// TestReadMap_MapSection pins the MAP-section parser. Single tile
// at (level=0, x=5, z=7): height=10, overlayId=2 with shape=4 rot=1,
// flags=8, underlay=3.
func TestReadMap_MapSection(t *testing.T) {
	lines := []string{
		"==== MAP ====",
		"0 5 7: h10 o2;4;1 f8 u3",
	}
	got := readMap(lines)
	key := packKey(0, 5, 7)
	tile, ok := got.Land[key]
	if !ok {
		t.Fatalf("Land[%d] missing", key)
	}
	want := landTile{H: 10, OverlayID: 2, OverlayShape: 4, OverlayRot: 1, Flags: 8, Underlay: 3}
	if tile != want {
		t.Errorf("tile = %+v, want %+v", tile, want)
	}
}

// TestReadMap_LocSection pins the LOC parser with defaulted optional
// fields (shape=10, angle=0).
func TestReadMap_LocSection(t *testing.T) {
	lines := []string{"==== LOC ====", "1 4 4: 100", "1 4 4: 200 5 2"}
	got := readMap(lines)
	key := packKey(1, 4, 4)
	entries, ok := got.Loc[key]
	if !ok || len(entries) != 2 {
		t.Fatalf("Loc[%d] = %v, want 2 entries", key, entries)
	}
	want0 := locEntry{ID: 100, Shape: 10, Angle: 0}
	want1 := locEntry{ID: 200, Shape: 5, Angle: 2}
	if entries[0] != want0 || entries[1] != want1 {
		t.Errorf("entries = %+v, want [%+v %+v]", entries, want0, want1)
	}
}

// TestReadMap_NpcAndObj pins NPC + OBJ sections.
func TestReadMap_NpcAndObj(t *testing.T) {
	lines := []string{
		"==== NPC ====",
		"0 1 2: 42",
		"==== OBJ ====",
		"0 3 4: 99 10",
	}
	got := readMap(lines)
	if ids, ok := got.Npc[packKey(0, 1, 2)]; !ok || len(ids) != 1 || ids[0] != 42 {
		t.Errorf("Npc = %v, want [42]", ids)
	}
	if objs, ok := got.Obj[packKey(0, 3, 4)]; !ok || len(objs) != 1 || objs[0] != (objEntry{ID: 99, Count: 10}) {
		t.Errorf("Obj = %v, want [{99 10}]", objs)
	}
}

// TestReadMap_CommentLineSkipped pins the '/'-prefixed comment-line
// skip: a "// comment" line parses identically to its absence, in every
// section. The pre-restructure TS shape had the explicit skip
// (map/Pack.js:31-34 @ 43e02957); the restructured parser @ 2e3bcf43
// dropped it but stays observably equivalent because the resulting
// negative packKey only feeds JS Int16Array writes, which are silent
// no-ops at negative indices. Go needs the explicit skip.
func TestReadMap_CommentLineSkipped(t *testing.T) {
	clean := []string{
		"==== MAP ====",
		"0 5 7: h10 o2;4;1 f8 u3",
		"==== LOC ====",
		"1 4 4: 100",
		"==== NPC ====",
		"0 3 3: 9",
		"==== OBJ ====",
		"0 2 2: 995 50",
	}
	commented := []string{
		"// header comment",
		"==== MAP ====",
		"// map comment",
		"0 5 7: h10 o2;4;1 f8 u3",
		"==== LOC ====",
		"// loc comment",
		"1 4 4: 100",
		"==== NPC ====",
		"// npc comment",
		"0 3 3: 9",
		"==== OBJ ====",
		"// obj comment",
		"0 2 2: 995 50",
	}

	got := readMap(commented)
	want := readMap(clean)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("readMap with comments = %+v\nwant (identical to no comments) %+v", got, want)
	}
}
