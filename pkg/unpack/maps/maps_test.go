package maps

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// --- Low-level encode helpers ---

// buildLandPacket encodes a [4][64][64][]byte tile code table into the flat
// land-format byte stream expected by readLand.  Each entry is the raw code
// bytes for that tile; a 0-terminator is appended automatically.
func buildLandPacket(codes [4][64][64][]byte) []byte {
	var b []byte
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				if seq := codes[level][x][z]; len(seq) > 0 {
					b = append(b, seq...)
				}
				b = append(b, 0) // terminator
			}
		}
	}
	return b
}

// buildLocPacket encodes a flat list of loc entries into the gsmarts-delta
// format expected by readLocs.  Entries must be provided in ascending id order;
// entries with the same id may be in any tile order.
func buildLocPacket(entries []locEntry) []byte {
	if len(entries) == 0 {
		return []byte{0} // outer terminator only
	}

	type group struct {
		id      int
		members []locEntry
	}
	var groups []group
	for _, e := range entries {
		if len(groups) == 0 || groups[len(groups)-1].id != e.id {
			groups = append(groups, group{id: e.id})
		}
		groups[len(groups)-1].members = append(groups[len(groups)-1].members, e)
	}

	var b []byte
	prevId := -1
	for _, g := range groups {
		deltaId := g.id - prevId
		b = appendGSmartS(b, deltaId)
		prevId = g.id

		prevData := 0
		for _, e := range g.members {
			packed := e.z | (e.x << 6) | (e.level << 12)
			deltaData := packed - prevData + 1
			b = appendGSmartS(b, deltaData)
			prevData = packed
			b = append(b, byte((e.shape<<2)|e.angle))
		}
		b = append(b, 0) // inner terminator
	}
	b = append(b, 0) // outer terminator
	return b
}

// locEntry is a helper struct for building test loc packets.
type locEntry struct {
	id, x, z, level, shape, angle int
}

// appendGSmartS appends a gsmarts-encoded unsigned value to b.
// Values 0-127 are single-byte; 128-32767 use 2-byte form (val+32768).
func appendGSmartS(b []byte, val int) []byte {
	if val < 128 {
		return append(b, byte(val))
	}
	v := uint16(val + 32768)
	return append(b, byte(v>>8), byte(v))
}

// newTestPacket wraps data in a packet.Packet for decoding.
func newTestPacket(data []byte) *packet.Packet {
	return packet.NewPacket(data)
}

// --- readLand unit tests ---

func TestReadLand_Code0Break(t *testing.T) {
	// All tiles get code=0 → all values stay -1.
	var codes [4][64][64][]byte
	d := readLand(newTestPacket(buildLandPacket(codes)))
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				if d.heightmap[level][x][z] != -1 {
					t.Fatalf("level=%d x=%d z=%d: heightmap should be -1, got %d", level, x, z, d.heightmap[level][x][z])
				}
			}
		}
	}
}

func TestReadLand_Code1Heightmap(t *testing.T) {
	// code=1 at [0][3][5] → heightmap=42, loop breaks; overlay stays -1.
	var codes [4][64][64][]byte
	codes[0][3][5] = []byte{1, 42}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.heightmap[0][3][5] != 42 {
		t.Fatalf("heightmap[0][3][5]: want 42 got %d", d.heightmap[0][3][5])
	}
	if d.overlayIds[0][3][5] != -1 {
		t.Fatal("overlayIds should be -1 after code=1")
	}
}

func TestReadLand_Code2Overlay(t *testing.T) {
	// code=2 (min overlay): shape=(2-2)/4=0, rot=(2-2)&3=0, overlayId=g1b=5.
	var codes [4][64][64][]byte
	codes[0][0][0] = []byte{2, 5}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.overlayIds[0][0][0] != 5 {
		t.Fatalf("overlayIds: want 5 got %d", d.overlayIds[0][0][0])
	}
	if d.overlayShape[0][0][0] != 0 {
		t.Fatalf("overlayShape: want 0 got %d", d.overlayShape[0][0][0])
	}
	if d.overlayRotation[0][0][0] != 0 {
		t.Fatalf("overlayRotation: want 0 got %d", d.overlayRotation[0][0][0])
	}
}

func TestReadLand_Code49Overlay(t *testing.T) {
	// code=49 (max overlay): shape=(49-2)/4=11, rot=(49-2)&3=3.
	var codes [4][64][64][]byte
	codes[1][2][3] = []byte{49, 7}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.overlayIds[1][2][3] != 7 {
		t.Fatalf("overlayIds: want 7 got %d", d.overlayIds[1][2][3])
	}
	if d.overlayShape[1][2][3] != 11 {
		t.Fatalf("overlayShape: want 11 got %d", d.overlayShape[1][2][3])
	}
	if d.overlayRotation[1][2][3] != 3 {
		t.Fatalf("overlayRotation: want 3 got %d", d.overlayRotation[1][2][3])
	}
}

func TestReadLand_Code50Flags(t *testing.T) {
	// code=50 (min flags): flags=50-49=1.
	var codes [4][64][64][]byte
	codes[0][1][1] = []byte{50}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.flags[0][1][1] != 1 {
		t.Fatalf("flags: want 1 got %d", d.flags[0][1][1])
	}
}

func TestReadLand_Code81Flags(t *testing.T) {
	// code=81 (max flags): flags=81-49=32.
	var codes [4][64][64][]byte
	codes[2][5][5] = []byte{81}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.flags[2][5][5] != 32 {
		t.Fatalf("flags: want 32 got %d", d.flags[2][5][5])
	}
}

func TestReadLand_Code82Underlay(t *testing.T) {
	// code=82 (min underlay): underlay=82-81=1.
	var codes [4][64][64][]byte
	codes[3][7][7] = []byte{82}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.underlay[3][7][7] != 1 {
		t.Fatalf("underlay: want 1 got %d", d.underlay[3][7][7])
	}
}

func TestReadLand_Code255Underlay(t *testing.T) {
	// code=255: underlay=255-81=174.
	var codes [4][64][64][]byte
	codes[0][0][1] = []byte{255}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.underlay[0][0][1] != 174 {
		t.Fatalf("underlay: want 174 got %d", d.underlay[0][0][1])
	}
}

func TestReadLand_SignedOverlayId(t *testing.T) {
	// g1b=0xFF → signed byte = -1 (value -1 stored in overlayIds).
	var codes [4][64][64][]byte
	codes[0][0][0] = []byte{2, 0xFF}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.overlayIds[0][0][0] != -1 {
		t.Fatalf("overlayIds: want -1 for g1b=0xFF, got %d", d.overlayIds[0][0][0])
	}
}

func TestReadLand_MultipleCodesNoBreak(t *testing.T) {
	// codes 50 and 82 in one tile (no break for these) → both flags and underlay set.
	var codes [4][64][64][]byte
	codes[0][0][0] = []byte{50, 82}
	d := readLand(newTestPacket(buildLandPacket(codes)))
	if d.flags[0][0][0] != 1 {
		t.Fatalf("flags: want 1 got %d", d.flags[0][0][0])
	}
	if d.underlay[0][0][0] != 1 {
		t.Fatalf("underlay: want 1 got %d", d.underlay[0][0][0])
	}
}

// --- readLocs unit tests ---

func TestReadLocs_Empty(t *testing.T) {
	locs := readLocs(newTestPacket([]byte{0}))
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				if len(locs[level][x][z]) != 0 {
					t.Fatalf("expected empty locs at [%d][%d][%d]", level, x, z)
				}
			}
		}
	}
}

func TestReadLocs_SingleEntry(t *testing.T) {
	// id=5, x=3, z=7, level=0, shape=2, angle=1.
	entries := []locEntry{{id: 5, x: 3, z: 7, level: 0, shape: 2, angle: 1}}
	locs := readLocs(newTestPacket(buildLocPacket(entries)))

	if len(locs[0][3][7]) != 1 {
		t.Fatalf("expected 1 loc at [0][3][7], got %d", len(locs[0][3][7]))
	}
	l := locs[0][3][7][0]
	if l.id != 5 || l.shape != 2 || l.angle != 1 {
		t.Fatalf("loc: want {5,2,1} got {%d,%d,%d}", l.id, l.shape, l.angle)
	}
}

func TestReadLocs_MultipleIds(t *testing.T) {
	entries := []locEntry{
		{id: 0, x: 0, z: 0, level: 0, shape: 0, angle: 0},
		{id: 5, x: 10, z: 20, level: 1, shape: 3, angle: 2},
	}
	locs := readLocs(newTestPacket(buildLocPacket(entries)))

	if len(locs[0][0][0]) != 1 || locs[0][0][0][0].id != 0 {
		t.Fatalf("expected id=0 at [0][0][0]")
	}
	if len(locs[1][10][20]) != 1 || locs[1][10][20][0].id != 5 {
		t.Fatalf("expected id=5 at [1][10][20]")
	}
}

// --- Full Unpack integration tests ---

// regionSpec describes one region for building a minimal test cache.
type regionSpec struct {
	mapX, mapZ int
	landCodes  func(codes *[4][64][64][]byte) // nil = all zero (all -1 results)
	locEntries []locEntry                     // nil/empty = no locs
}

func TestUnpack_LandOnly(t *testing.T) {
	scratch := t.TempDir()
	cacheDir := buildMinimalCache(t, []regionSpec{
		{
			mapX: 50, mapZ: 50,
			landCodes: func(codes *[4][64][64][]byte) {
				codes[0][1][2] = []byte{1, 5} // height=5
				codes[0][0][0] = []byte{82}   // underlay=1
			},
		},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(scratch, "maps", "m50_50.jm2"))
	if err != nil {
		t.Fatalf("read jm2: %v", err)
	}
	s := string(content)
	if !strings.HasPrefix(s, "==== MAP ====\n") {
		t.Fatalf("expected MAP header, got prefix: %q", s[:min(len(s), 50)])
	}
	if !strings.Contains(s, "0 1 2: h5\n") {
		t.Fatalf("expected '0 1 2: h5', content:\n%s", s)
	}
	if !strings.Contains(s, "0 0 0: u1\n") {
		t.Fatalf("expected '0 0 0: u1', content:\n%s", s)
	}
}

func TestUnpack_OverlayTokens(t *testing.T) {
	// Tests all three overlay token forms based on shape/rotation values.
	scratch := t.TempDir()
	cacheDir := buildMinimalCache(t, []regionSpec{
		{
			mapX: 51, mapZ: 51,
			landCodes: func(codes *[4][64][64][]byte) {
				// code=7: shape=(7-2)/4=1, rot=(7-2)&3=1 → both non-zero → o10;1;1
				codes[0][0][0] = []byte{7, 10}
				// code=4: shape=(4-2)/4=0, rot=(4-2)&3=2 → shape==0 → o3
				codes[0][1][0] = []byte{4, 3}
				// code=10: shape=(10-2)/4=2, rot=(10-2)&3=0 → rot==0 → o9;2
				codes[0][3][0] = []byte{10, 9}
			},
		},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	s, err := readJM2(t, scratch, "m51_51.jm2")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(s, "o10;1;1") {
		t.Fatalf("expected o10;1;1 (shape+rot), content:\n%s", s)
	}
	if strings.Contains(s, "o3;") {
		t.Fatalf("unexpected semicolon for o3 (shape==0), content:\n%s", s)
	}
	if !strings.Contains(s, "o3 ") && !strings.Contains(s, "o3\n") {
		t.Fatalf("expected plain 'o3', content:\n%s", s)
	}
	if !strings.Contains(s, "o9;2") {
		t.Fatalf("expected o9;2 (shape only), content:\n%s", s)
	}
	if strings.Contains(s, "o9;2;") {
		t.Fatalf("unexpected third field for o9;2 (rot==0), content:\n%s", s)
	}
}

func TestUnpack_LocAngleOmission(t *testing.T) {
	scratch := t.TempDir()
	cacheDir := buildMinimalCache(t, []regionSpec{
		{
			mapX: 52, mapZ: 52,
			locEntries: []locEntry{
				{id: 5, x: 10, z: 20, level: 0, shape: 3, angle: 0}, // angle=0 → omit
				{id: 7, x: 11, z: 21, level: 1, shape: 1, angle: 2}, // angle=2 → include
			},
		},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	s, err := readJM2(t, scratch, "m52_52.jm2")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(s, "\n==== LOC ====\n") {
		t.Fatalf("expected LOC section, content:\n%s", s)
	}
	if !strings.Contains(s, "0 10 20: 5 3\n") {
		t.Fatalf("expected '0 10 20: 5 3' (no angle), content:\n%s", s)
	}
	if !strings.Contains(s, "1 11 21: 7 1 2\n") {
		t.Fatalf("expected '1 11 21: 7 1 2', content:\n%s", s)
	}
}

func TestUnpack_NpcObjPreservation(t *testing.T) {
	scratch := t.TempDir()

	// Pre-seed .jm2 with all four sections.
	mapsDir := filepath.Join(scratch, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "==== MAP ====\n0 0 0: u1\n\n==== LOC ====\n0 0 0: 5 3\n\n==== NPC ====\n0 0 0: 999\n\n==== OBJ ====\n0 0 0: 100 1\n"
	if err := os.WriteFile(filepath.Join(mapsDir, "m53_53.jm2"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := buildMinimalCache(t, []regionSpec{
		{
			mapX: 53, mapZ: 53,
			landCodes: func(codes *[4][64][64][]byte) {
				codes[0][0][0] = []byte{82} // underlay=1
			},
		},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	s, err := readJM2(t, scratch, "m53_53.jm2")
	if err != nil {
		t.Fatal(err)
	}

	mapIdx := strings.Index(s, "==== MAP ====")
	locIdx := strings.Index(s, "==== LOC ====")
	npcIdx := strings.Index(s, "==== NPC ====")
	objIdx := strings.Index(s, "==== OBJ ====")

	if mapIdx == -1 || locIdx == -1 || npcIdx == -1 || objIdx == -1 {
		t.Fatalf("missing sections, content:\n%s", s)
	}
	if !(mapIdx < locIdx && locIdx < npcIdx && npcIdx < objIdx) {
		t.Fatalf("section order wrong: MAP=%d LOC=%d NPC=%d OBJ=%d\ncontent:\n%s",
			mapIdx, locIdx, npcIdx, objIdx, s)
	}
	if !strings.Contains(s, "0 0 0: 999") {
		t.Fatalf("NPC data not preserved, content:\n%s", s)
	}
	if !strings.Contains(s, "0 0 0: 100 1") {
		t.Fatalf("OBJ data not preserved, content:\n%s", s)
	}
}

// TestUnpack_NpcObjPreservation_LineSplit254 pins the 254 split semantics for
// the pre-existing .jm2 read: CRLF line endings split cleanly, but a lone \r
// NOT followed by \n stays INSIDE its line (TS map/Unpack.ts:176 @2e3bcf43
// switched from the old strip-all-\r-then-split-on-\n to .split(/\r?\n/)).
func TestUnpack_NpcObjPreservation_LineSplit254(t *testing.T) {
	scratch := t.TempDir()

	mapsDir := filepath.Join(scratch, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// CRLF endings throughout + one lone \r embedded in an NPC line.
	existing := "==== MAP ====\r\n0 0 0: u1\r\n\r\n==== NPC ====\r\n0 0 0: 9\r9 9\r\n"
	if err := os.WriteFile(filepath.Join(mapsDir, "m53_53.jm2"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := buildMinimalCache(t, []regionSpec{
		{
			mapX: 53, mapZ: 53,
			landCodes: func(codes *[4][64][64][]byte) {
				codes[0][0][0] = []byte{82} // underlay=1
			},
		},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	s, err := readJM2(t, scratch, "m53_53.jm2")
	if err != nil {
		t.Fatal(err)
	}

	// The CRLF \r must be stripped from the section header line…
	if !strings.Contains(s, "==== NPC ====\n") {
		t.Fatalf("NPC header not split cleanly from CRLF, content:\n%q", s)
	}
	// …while the lone \r inside the NPC data line survives.
	if !strings.Contains(s, "0 0 0: 9\r9 9\n") {
		t.Fatalf("lone \\r inside the NPC line must be preserved at 254, content:\n%q", s)
	}
}

func TestUnpack_MissingMapFileWarning(t *testing.T) {
	scratch := t.TempDir()
	cacheDir := buildMinimalCacheNoData(t, []regionSpec{
		{mapX: 60, mapZ: 60},
	})

	var out strings.Builder
	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch, Out: &out}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	if !strings.Contains(out.String(), "Missing map file for 60_60") {
		t.Fatalf("expected warning, got: %q", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(scratch, "maps", "m60_60.jm2")); !os.IsNotExist(statErr) {
		t.Fatal("jm2 file should not exist when land/loc missing")
	}
}

func TestUnpack_NoNpcObjSection_NotAppended(t *testing.T) {
	scratch := t.TempDir()
	mapsDir := filepath.Join(scratch, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "==== MAP ====\n\n==== LOC ====\n"
	if err := os.WriteFile(filepath.Join(mapsDir, "m54_54.jm2"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := buildMinimalCache(t, []regionSpec{
		{mapX: 54, mapZ: 54},
	})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: scratch}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	s, err := readJM2(t, scratch, "m54_54.jm2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "==== NPC ====") || strings.Contains(s, "==== OBJ ====") {
		t.Fatalf("unexpected NPC/OBJ sections, content:\n%s", s)
	}
}

// --- Test infrastructure ---

// buildMinimalCache creates a synthetic cache with versionlist + tile data.
func buildMinimalCache(t *testing.T, specs []regionSpec) string {
	t.Helper()
	cacheDir := t.TempDir()
	buildCacheImpl(t, cacheDir, specs, true)
	return cacheDir
}

// buildMinimalCacheNoData creates a cache with map_index entries but no tile data.
func buildMinimalCacheNoData(t *testing.T, specs []regionSpec) string {
	t.Helper()
	cacheDir := t.TempDir()
	buildCacheImpl(t, cacheDir, specs, false)
	return cacheDir
}

func buildCacheImpl(t *testing.T, cacheDir string, specs []regionSpec, writeData bool) {
	t.Helper()

	// Build map_index: 7 bytes per record.
	var mapIndexBytes []byte
	for i, spec := range specs {
		region := uint16((spec.mapX << 8) | spec.mapZ)
		landFile := i*2 + 10
		locFile := i*2 + 11
		mapIndexBytes = binary.BigEndian.AppendUint16(mapIndexBytes, region)
		mapIndexBytes = binary.BigEndian.AppendUint16(mapIndexBytes, uint16(landFile))
		mapIndexBytes = binary.BigEndian.AppendUint16(mapIndexBytes, uint16(locFile))
		mapIndexBytes = append(mapIndexBytes, 0) // members=false
	}

	// Build versionlist jagfile containing map_index.
	versionlistData := buildJagfileWithEntry(t, "map_index", mapIndexBytes)

	// Initialise FileStream cache files.
	if err := os.WriteFile(filepath.Join(cacheDir, "main_file_cache.dat"), nil, 0o666); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		p := filepath.Join(cacheDir, "main_file_cache.idx"+string(rune('0'+i)))
		if err := os.WriteFile(p, nil, 0o666); err != nil {
			t.Fatal(err)
		}
	}

	// Write archive 0 / file 5 (versionlist).
	writeToCacheDir(t, cacheDir, 0, 5, versionlistData, false)

	if !writeData {
		return
	}

	// Write land and loc data for each spec.
	for i, spec := range specs {
		landFile := i*2 + 10
		locFile := i*2 + 11

		var codes [4][64][64][]byte
		if spec.landCodes != nil {
			spec.landCodes(&codes)
		}
		landBytes := buildLandPacket(codes)

		locBytes := buildLocPacket(spec.locEntries)

		writeToCacheDir(t, cacheDir, 4, landFile, landBytes, true)
		writeToCacheDir(t, cacheDir, 4, locFile, locBytes, true)
	}
}

func writeToCacheDir(t *testing.T, cacheDir string, archive, file int, data []byte, gzipCompress bool) {
	t.Helper()
	// For archive 4 entries, FileStream.Read(4, n, true) runs gzip decompression.
	// Write the data pre-gzipped (with a 2-byte version trailer, as the real packer does)
	// so that Read can decompress it successfully.
	payload := data
	if gzipCompress {
		payload = gzipBytes(t, data)
	}
	fs, err := filestream.New(cacheDir, false, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	defer fs.Close()
	if ok := fs.Write(archive, file, payload, 0); !ok {
		t.Fatalf("Write(%d, %d) failed", archive, file)
	}
}

// gzipBytes gzip-compresses data for use as an archive-4 cache entry.
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// buildJagfileWithEntry builds a minimal Jagfile byte slice (per-file bzip2,
// CompressWhole=false) containing one entry with the given name and raw data.
// The individual file entry is bzip2-compressed (header stripped) to match the
// format NewJagfile expects when unpackedSize == packedSize (CompressWhole=false).
func buildJagfileWithEntry(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	// Compress the entry data (prefixLength=false, removeHeader=true, blockSize=1).
	compressed, err := jagfile.BZip2Compress(data, false, true, 1, 0)
	if err != nil {
		t.Fatalf("buildJagfileWithEntry BZip2Compress: %v", err)
	}

	hash := jagfileHash(name)
	fileCount := 1
	// Offset of first file data relative to the body start (after the 6-byte outer header).
	// Body layout: 2-byte count + fileCount*10 (table) + file data.
	offsetInBody := 2 + fileCount*10

	var body []byte
	body = append(body, byte(fileCount>>8), byte(fileCount))
	// file table: 4-byte hash, 3-byte uncompressed size, 3-byte compressed size
	body = append(body,
		byte(hash>>24), byte(hash>>16), byte(hash>>8), byte(hash),
		byte(len(data)>>16), byte(len(data)>>8), byte(len(data)),
		byte(len(compressed)>>16), byte(len(compressed)>>8), byte(len(compressed)),
	)
	body = append(body, compressed...)
	_ = offsetInBody // used for documentation; actual offset is implied by table layout

	sz := len(body)
	var result []byte
	// Outer header: 3-byte uncompressed = 3-byte compressed (same → CompressWhole=false).
	result = append(result, byte(sz>>16), byte(sz>>8), byte(sz))
	result = append(result, byte(sz>>16), byte(sz>>8), byte(sz))
	result = append(result, body...)
	return result
}

// jagfileHash mirrors the hash function in pkg/io/jagfile (not exported, so we
// re-implement the identical formula here for test-only use).
func jagfileHash(name string) uint32 {
	hash := uint32(0)
	for _, c := range strings.ToUpper(name) {
		hash = (hash*61 + uint32(c) - 32) | 0
	}
	return hash
}

// readJM2 is a convenience helper to read the content of a .jm2 file.
func readJM2(t *testing.T, dir, name string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "maps", name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
