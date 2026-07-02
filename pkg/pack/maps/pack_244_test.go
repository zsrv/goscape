package maps

// Tests for the rev-244 observable deltas ported in this task:
//  1. Client m/l files compressed with gzip (not bzip2).
//  2. Server m/l files remain raw (not compressed).
//  3. cache.Write(4, mapPack.GetByName("m<XZ>"), on-disk gzip bytes, 1).
//  4. NPC/OBJ emission: level 0..3 → x 0..63 → z 0..63 ascending order.
//  5. Unknown NPC type → return error containing "NPC type does not exist".
//  6. modelFlags[model] |= 0x4 for each NpcType.Models/Heads entry.
//
// TS citations: tools/pack/map/Pack.js @ 9aadcec4.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/pack"
)

// writeFile244 creates parent directories and writes content to path.
func writeFile244(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %q: %v", path, err)
	}
}

// buildNpcDatIntoOutDir runs PackConfigs to produce a valid npc.dat/idx
// and client Jagfile in outDir. Each npcFixtureEntry has an id, name, and
// optional model IDs. The function populates srcDir with the minimal
// pack infrastructure needed and calls pack.PackConfigs.
func buildNpcDatIntoOutDir(t *testing.T, outDir string, entries []npcFixtureEntry) {
	t.Helper()
	srcDir := t.TempDir()

	// npc.pack.
	npcPackLines := ""
	for _, e := range entries {
		npcPackLines += fmt.Sprintf("%d=%s\n", e.id, e.name)
	}
	writeFile244(t, filepath.Join(srcDir, "pack", "npc.pack"), npcPackLines)

	// model.pack: collect all model IDs.
	modelPack := "0=dummy_model\n"
	modelSeen := map[int]bool{}
	for _, e := range entries {
		for _, mid := range e.models {
			if !modelSeen[mid] {
				modelSeen[mid] = true
				modelPack += fmt.Sprintf("%d=npc_m%d\n", mid, mid)
			}
		}
	}
	writeFile244(t, filepath.Join(srcDir, "pack", "model.pack"), modelPack)

	// All other required pack files (empty).
	for _, name := range []string{
		"loc", "seq", "category", "texture", "param", "obj", "varp", "varn",
		"vars", "inv", "spotanim", "idk", "flo", "enum", "struct", "mesanim",
		"dbtable", "dbrow", "interface", "animset", "anim", "base", "synth",
		"map", "midi",
	} {
		writeFile244(t, filepath.Join(srcDir, "pack", name+".pack"), "")
	}
	writeFile244(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=default_hunt\n")

	// NPC source scripts.
	npcSrc := ""
	for _, e := range entries {
		npcSrc += "[" + e.name + "]\n"
		for i, mid := range e.models {
			_ = mid
			// model1=, model2=, ... referencing model pack names.
			npcSrc += fmt.Sprintf("model%d=npc_m%d\n", i+1, e.models[i])
		}
	}
	writeFile244(t, filepath.Join(srcDir, "scripts", "test.npc"), npcSrc)
	writeFile244(t, filepath.Join(srcDir, "scripts", "test.hunt"), "[default_hunt]\n")

	pack.ClearFsCache()
	if err := pack.PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("buildNpcDatIntoOutDir: PackConfigs: %v", err)
	}
}

type npcFixtureEntry struct {
	id     int
	name   string
	models []int // model pack IDs (model1=, model2=, ...)
}

// TestPack_ClientFilesAreGzip pins TS Pack.js:275,356 @ 9aadcec4:
//
//	fs.writeFileSync(mapFile, compressGz(data))   // land
//	fs.writeFileSync(locFile, compressGz(data))   // loc
//
// Client m/l files must start with gzip magic 0x1f 0x8b.
// Server m/l files must remain raw (not gzip-compressed).
func TestPack_ClientFilesAreGzip(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile244(t, filepath.Join(mapsDir, "m5050.jm2"), strings.Join([]string{
		"==== MAP ====", "0 5 7: h10 o2 u3",
		"==== LOC ====", "0 5 7: 100",
	}, "\n"))

	out := filepath.Join(tmp, "out")
	if err := Pack(src, out, nil, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Client files must be gzip-compressed.
	for _, name := range []string{"m5050", "l5050"} {
		path := filepath.Join(out, "client", "maps", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("client %q: %v", name, err)
			continue
		}
		if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
			t.Errorf("client %q: want gzip magic 0x1f 0x8b, got %#x", name, data[:min(2, len(data))])
		}
	}

	// Server files must NOT be gzip-compressed.
	for _, name := range []string{"m5050", "l5050"} {
		path := filepath.Join(out, "server", "maps", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("server %q: %v", name, err)
			continue
		}
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			t.Errorf("server %q: must NOT be gzip-compressed", name)
		}
	}
}

// TestPack_CacheWritesArchive4 pins TS Pack.js:449-450 @ 9aadcec4:
//
//	cache.write(4, MapPack.getByName(`m${mapX}_${mapZ}`), fs.readFileSync(mapFile), 1)
//	cache.write(4, MapPack.getByName(`l${mapX}_${mapZ}`), fs.readFileSync(locFile), 1)
//
// The writes happen unconditionally at the end of the per-file loop.
// Written bytes are the on-disk client-file content (gzip).
func TestPack_CacheWritesArchive4(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile244(t, filepath.Join(mapsDir, "m5050.jm2"), strings.Join([]string{
		"==== MAP ====", "0 5 7: h10 o2 u3",
		"==== LOC ====", "0 5 7: 100",
	}, "\n"))

	// MapPack: m5050 → id=0, l5050 → id=1.
	// The jm2 filename m5050.jm2 yields mapXZ="5050"; cache key is
	// mapPack.GetByName("m5050") = 0 and GetByName("l5050") = 1.
	mapPack := &pack.PackFile{
		SrcDir:   tmp,
		Type:     "map",
		Pack:     map[int]string{0: "m5050", 1: "l5050"},
		NameToID: map[string]int{"m5050": 0, "l5050": 1},
		Names:    map[string]struct{}{"m5050": {}, "l5050": {}},
		Max:      2,
	}

	cacheDir := t.TempDir()
	cache := filestream.New(cacheDir, true, false)
	defer cache.Close()

	out := filepath.Join(tmp, "out")
	if err := Pack(src, out, mapPack, cache, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Both cache entries must contain gzip data (client file bytes).
	for _, tc := range []struct {
		archive, file int
		label         string
	}{
		{4, 0, "m5050"},
		{4, 1, "l5050"},
	} {
		data := cache.Read(tc.archive, tc.file, false)
		if len(data) < 2 {
			t.Errorf("cache(%d,%d) %s: empty or missing", tc.archive, tc.file, tc.label)
			continue
		}
		if data[0] != 0x1f || data[1] != 0x8b {
			t.Errorf("cache(%d,%d) %s: want gzip magic, got %#x %#x",
				tc.archive, tc.file, tc.label, data[0], data[1])
		}
	}
}

// TestPack_NpcObjLevelXZOrder pins TS Pack.js:366-407, 416-447 @ 9aadcec4:
// NPC and OBJ entries are emitted in level 0..3 → x 0..63 → z 0..63
// ascending order, regardless of source-line insertion order.
//
// Pos encoding: p2((level<<12)|(x<<6)|z).
// NPC output: p2(pos) p1(count) p2(id)...
// OBJ output: p2(pos) p1(count) [p2(id) p1(count)]...
func TestPack_NpcObjLevelXZOrder(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// NPC entries inserted in REVERSE order: (1,5,7) then (0,3,4).
	// Expected emission order: (0,3,4) pos=196, then (1,5,7) pos=4423.
	// OBJ entries in REVERSE: (2,10,5) then (0,1,2).
	// Expected emission: (0,1,2) pos=66, then (2,10,5) pos=8837.
	writeFile244(t, filepath.Join(mapsDir, "m5050.jm2"), strings.Join([]string{
		"==== NPC ====",
		"1 5 7: 42",
		"0 3 4: 7",
		"==== OBJ ====",
		"2 10 5: 99 3",
		"0 1 2: 11 1",
	}, "\n"))

	out := filepath.Join(tmp, "out")
	if err := Pack(src, out, nil, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	const (
		pos034  = (0 << 12) | (3 << 6) | 4  // 196  = 0x00C4
		pos157  = (1 << 12) | (5 << 6) | 7  // 4423 = 0x1147
		pos012  = (0 << 12) | (1 << 6) | 2  // 66   = 0x0042
		pos2105 = (2 << 12) | (10 << 6) | 5 // 8837 = 0x2285
	)

	// NPC file: (pos034, count=1, npc-id=7), (pos157, count=1, npc-id=42).
	wantNpc := []byte{
		byte(pos034 >> 8), byte(pos034 & 0xFF), 0x01, 0x00, 0x07,
		byte(pos157 >> 8), byte(pos157 & 0xFF), 0x01, 0x00, 0x2A,
	}
	// OBJ file: (pos012, count=1, obj-id=11, stack=1), (pos2105, count=1, obj-id=99, stack=3).
	wantObj := []byte{
		byte(pos012 >> 8), byte(pos012 & 0xFF), 0x01, 0x00, 0x0B, 0x01,
		byte(pos2105 >> 8), byte(pos2105 & 0xFF), 0x01, 0x00, 0x63, 0x03,
	}

	gotNpc, err := os.ReadFile(filepath.Join(out, "server", "maps", "n5050"))
	if err != nil {
		t.Fatalf("n5050: %v", err)
	}
	if string(gotNpc) != string(wantNpc) {
		t.Errorf("n5050:\n got  %v\n want %v", gotNpc, wantNpc)
	}

	gotObj, err := os.ReadFile(filepath.Join(out, "server", "maps", "o5050"))
	if err != nil {
		t.Fatalf("o5050: %v", err)
	}
	if string(gotObj) != string(wantObj) {
		t.Errorf("o5050:\n got  %v\n want %v", gotObj, wantObj)
	}
}

// TestPack_UnknownNpcTypeError pins TS Pack.js:390-393 @ 9aadcec4:
//
//	const type = NpcType.get(npcs[i]);
//	if (!type) { printFatalError(`m${mapX}_${mapZ}: NPC type does not exist: ${id}`); }
//
// goscape returns an error (not os.Exit). Error contains map name and id.
func TestPack_UnknownNpcTypeError(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// NPC id=999 — not in the packed npc.dat (only id=0 exists).
	writeFile244(t, filepath.Join(mapsDir, "m5050.jm2"), strings.Join([]string{
		"==== NPC ====",
		"0 5 7: 999",
	}, "\n"))

	// Build outDir with only NPC id=0 ("rat"). NPC 999 is absent.
	out := filepath.Join(tmp, "out")
	buildNpcDatIntoOutDir(t, out, []npcFixtureEntry{{id: 0, name: "rat"}})

	err := Pack(src, out, nil, nil, nil)
	if err == nil {
		t.Fatal("Pack: want error for unknown NPC id=999, got nil")
	}
	for _, want := range []string{"NPC type does not exist", "999"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	}
}

// TestPack_ModelFlagsSetFor0x4 pins TS Pack.js:394-403 @ 9aadcec4:
//
//	if (type.models) { for (const model of type.models) { modelFlags[model] |= 0x4; } }
//	if (type.heads)  { for (const model of type.heads)  { modelFlags[model] |= 0x4; } }
//
// After packing a map that references NPC 0 (which has model ID 3),
// modelFlags[3] must have bit 0x4 set.
func TestPack_ModelFlagsSetFor0x4(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile244(t, filepath.Join(mapsDir, "m5050.jm2"), strings.Join([]string{
		"==== NPC ====",
		"0 5 7: 0",
	}, "\n"))

	// NPC id=0 has model ID 3.
	out := filepath.Join(tmp, "out")
	buildNpcDatIntoOutDir(t, out, []npcFixtureEntry{
		{id: 0, name: "rat", models: []int{3}},
	})

	modelFlags := make([]int, 16)
	if err := Pack(src, out, nil, nil, modelFlags); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// modelFlags[3] must have bit 0x4 set (TS Pack.js:394-398 @ 9aadcec4).
	if modelFlags[3]&0x4 == 0 {
		t.Errorf("modelFlags[3] = 0x%x, want bit 0x4 set", modelFlags[3])
	}
}
