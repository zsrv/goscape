package graphics

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// --- Helpers ---

// gzipBytes compresses data with gzip.  Models calls cache.Read(1,id,true)
// which decompresses, so model data stored in the cache must be pre-compressed.
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

// buildModelCache creates a minimal FileStream cache containing:
//   - archive 0 / file 5: versionlist jagfile with model_index member
//     (one flag byte per model).
//   - archive 1 / file i: gzip(modelData[i]) for non-nil entries.
func buildModelCache(t *testing.T, cacheDir string, modelFlags []byte, modelData [][]byte) {
	t.Helper()

	// Build versionlist jagfile.
	vl := jagfile.NewEmptyJagfile(false)
	vl.Write("model_index", packet.NewPacket(modelFlags))

	tmp := filepath.Join(t.TempDir(), "vl.jag")
	if err := vl.Save(tmp); err != nil {
		t.Fatalf("vl.Save: %v", err)
	}
	vlBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("readFile vl: %v", err)
	}

	fs2, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	if !fs2.Write(0, 5, vlBytes, 0) {
		t.Fatal("write versionlist failed")
	}
	// For nil entries we write a single non-gzip byte so that cache.Count(1)
	// includes those slots but Read(1, i, true) returns nil (decompression fails).
	for i, data := range modelData {
		var raw []byte
		if data != nil {
			raw = gzipBytes(t, data)
		} else {
			raw = []byte{0xFF} // corrupt: fails gunzip → Read returns nil
		}
		if !fs2.Write(1, i, raw, 0) {
			t.Fatalf("write model %d failed", i)
		}
	}
	fs2.Close()
}

// buildAnimSet builds a minimal raw anim-set buffer with:
//   - nFrames frames, each with a single label (flags driven by flagsList).
//   - A base skeleton with 1 joint, 1 label.
//   - The offset table at the last 8 bytes.
//
// frameIds[i] is the global frameId for frame i.
// flagsList[i] is the tran1 flags byte for that frame's single label.
// tran2Vals[i] contains the smart values to encode for flags bits that are set.
//
// The returned []byte can be passed directly to cache.Write(2, baseId, data, 0)
// (the filestream Read(2, baseId, true) path decompresses — but for tests we
// inject the raw bytes into archive 2 WITHOUT gzip because buildAnimSetCache
// stores them raw and the test overrides the decompress path by writing a
// pre-decompressed buffer directly to cache.Write which stores without
// compression.  Wait — the cache stores whatever bytes we give it; Read(true)
// will attempt to gunzip.  So we must gzip-compress the set bytes before storing.)
//
// Callers gzip-compress the returned bytes before writing to the cache.
func buildAnimSet(t *testing.T, frameIds []int, flagsList []byte, tran2Vals [][]int32) []byte {
	t.Helper()

	// We build each section as a packet, then assemble.
	headPkt := packet.NewPacket(nil)
	tran1Pkt := packet.NewPacket(nil)
	tran2Pkt := packet.NewPacket(nil)
	delPkt := packet.NewPacket(nil)
	basePkt := packet.NewPacket(nil)

	// base section: 1 joint.
	basePkt.P1(1) // length=1 joint
	basePkt.P1(0) // joint type
	basePkt.P1(1) // labelCount for joint 0
	basePkt.P1(0) // label 0

	// head section (frameCount g2 first, then per-frame records).
	headPkt.P2(uint16(len(frameIds)))
	for i, fid := range frameIds {
		headPkt.P2(uint16(fid))
		headPkt.P1(1) // labelCount=1

		flags := flagsList[i]
		tran1Pkt.P1(flags)

		var tran2Idx int
		for _, bit := range []byte{0x1, 0x2, 0x4} {
			if flags&bit != 0 {
				// encode as gsmart: value < 64 → 1 byte; value in [-16384,16383] use 2 bytes
				v := tran2Vals[i][tran2Idx]
				tran2Idx++
				// encode signed smart: 1 byte range is [-64, 63] represented as (v+64)
				if v >= -64 && v <= 63 {
					tran2Pkt.P1(uint8(v + 64))
				} else {
					tran2Pkt.P2(uint16(v + 49152))
				}
			}
		}
	}

	// del section: one delay byte per frame.
	for range len(frameIds) {
		delPkt.P1(1)
	}

	// Compute section lengths.
	headLen := len(headPkt.Data)
	tran1Len := len(tran1Pkt.Data)
	tran2Len := len(tran2Pkt.Data)
	delLen := len(delPkt.Data)

	// Assemble: head | tran1 | tran2 | del | base | offset-table(8 bytes)
	// offset table: g2(headLen-2), g2(tran1Len), g2(tran2Len), g2(delLen)
	// The head section length stored in the table is (headLen - 2) because
	// the Read path does offset += g2() + 2.
	var out []byte
	out = append(out, headPkt.Data...)
	out = append(out, tran1Pkt.Data...)
	out = append(out, tran2Pkt.Data...)
	out = append(out, delPkt.Data...)
	out = append(out, basePkt.Data...)

	// 8-byte offset table.
	offPkt := packet.NewPacket(nil)
	offPkt.P2(uint16(headLen - 2)) // stored as headLen-2 (read side adds +2)
	offPkt.P2(uint16(tran1Len))
	offPkt.P2(uint16(tran2Len))
	offPkt.P2(uint16(delLen))
	out = append(out, offPkt.Data...)

	return out
}

// buildAnimSetCache creates a minimal FileStream cache containing
// gzip-compressed anim-set data in archive 2.  Nil entries receive a single
// 0xFF byte so that cache.Count(2) counts those slots but Read(2, i, true)
// returns nil (decompression of 0xFF fails).
func buildAnimSetCache(t *testing.T, cacheDir string, sets [][]byte) {
	t.Helper()
	fs2, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	for i, s := range sets {
		var raw []byte
		if s != nil {
			raw = gzipBytes(t, s)
		} else {
			raw = []byte{0xFF} // corrupt: fails gunzip → Read returns nil
		}
		if !fs2.Write(2, i, raw, 0) {
			t.Fatalf("write anim set %d failed", i)
		}
	}
	fs2.Close()
}

// --- Models unit tests ---

// TestModels_FlagRouting verifies that:
//   - flag=1 (referenced) → printWarning on missing data.
//   - flag=0 (unreferenced) → printDebug on missing data.
//   - present data → written to _unpack/<name>.ob2.
func TestModels_FlagRouting(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// 3 models: id=0 present, id=1 missing+flag=1 (warn), id=2 missing+flag=0 (debug).
	flags := []byte{0, 1, 0}
	data0 := []byte("ob2-model-0")
	buildModelCache(t, cacheDir, flags, [][]byte{data0, nil, nil})

	var out bytes.Buffer
	if err := Models(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Models: %v", err)
	}

	// id=0 written.
	gotPath := filepath.Join(srcDir, "models", "_unpack", "model_0.ob2")
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Errorf("read model_0.ob2: %v", err)
	} else if !bytes.Equal(got, data0) {
		t.Errorf("model_0.ob2: got %q, want %q", got, data0)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")

	// Find warning and debug messages.
	var hasWarn, hasDebug bool
	for _, l := range lines {
		if l == "Missing model model_1" {
			hasWarn = true
		}
		if l == "Missing unreferenced model model_2" {
			hasDebug = true
		}
	}
	if !hasWarn {
		t.Errorf("expected 'Missing model model_1' warning, stdout: %q", out.String())
	}
	if !hasDebug {
		t.Errorf("expected 'Missing unreferenced model model_2' debug, stdout: %q", out.String())
	}
}

// TestModels_ExistingPathRouting verifies that a model with an existing .ob2
// file in the content tree is written to that path (not _unpack/).
func TestModels_ExistingPathRouting(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Create models/ dir with a pre-existing model file.
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models: %v", err)
	}
	existingPath := filepath.Join(modelsDir, "my_model.ob2")
	if err := os.WriteFile(existingPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	// Seed model.pack so id=0 → "my_model".
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "model.pack"), []byte("0=my_model\n"), 0o644); err != nil {
		t.Fatalf("write model.pack: %v", err)
	}

	newData := []byte("ob2-new-data")
	buildModelCache(t, cacheDir, []byte{1}, [][]byte{newData})

	if err := Models(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Models: %v", err)
	}

	// Should have overwritten the existing path.
	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Errorf("existing path: got %q, want %q", got, newData)
	}

	// Should NOT have created _unpack/my_model.ob2.
	unpackPath := filepath.Join(srcDir, "models", "_unpack", "my_model.ob2")
	if _, err := os.Stat(unpackPath); err == nil {
		t.Errorf("_unpack/my_model.ob2 should not exist")
	}
}

// TestModels_RegisterCondition verifies that a pre-registered name is used
// (not overwritten) when model.pack already has an entry for the id.
func TestModels_RegisterCondition(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "model.pack"), []byte("0=custom_name\n"), 0o644); err != nil {
		t.Fatalf("write model.pack: %v", err)
	}

	data := []byte("ob2-data")
	buildModelCache(t, cacheDir, []byte{1}, [][]byte{data})

	if err := Models(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Models: %v", err)
	}

	// File must be in _unpack/ under the custom name.
	gotPath := filepath.Join(srcDir, "models", "_unpack", "custom_name.ob2")
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Errorf("read custom_name.ob2: %v", err)
	} else if !bytes.Equal(got, data) {
		t.Errorf("custom_name.ob2: got %q, want %q", got, data)
	}

	// model_0.ob2 must NOT exist.
	if _, err := os.Stat(filepath.Join(srcDir, "models", "_unpack", "model_0.ob2")); err == nil {
		t.Errorf("model_0.ob2 should not exist — pre-registered name not used")
	}
}

// TestModels_InclusiveLoopBound verifies the rev-274 inclusive loop bound
// (`for id := 0; id <= len(models); id++`).  With 2 model_index flags and 2
// present model files, the loop must ALSO process id == 2 (one past the last
// flag), where the cache read is out of range and models[id] is undefined, so it
// emits the printDebug "Missing unreferenced model model_2" line.
//
// Under the pre-274 bound (`id < modelCount`) the loop stopped at id=1 and this
// extra iteration never happened — so this is a real behavioural assertion.
func TestModels_InclusiveLoopBound(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// 2 flags, 2 present models → modelCount == 2, len(models) == 2.
	flags := []byte{1, 1}
	data0 := []byte("ob2-model-0")
	data1 := []byte("ob2-model-1")
	buildModelCache(t, cacheDir, flags, [][]byte{data0, data1})

	var out bytes.Buffer
	if err := Models(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Models: %v", err)
	}

	// id=0 and id=1 written.
	for i, want := range map[int][]byte{0: data0, 1: data1} {
		p := filepath.Join(srcDir, "models", "_unpack", "model_"+string(rune('0'+i))+".ob2")
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read model_%d.ob2: %v", i, err)
		} else if !bytes.Equal(got, want) {
			t.Errorf("model_%d.ob2 mismatch", i)
		}
	}

	// id=2 (== len(models)) must emit the unreferenced-debug line and NOT a warning.
	if !strings.Contains(out.String(), "Missing unreferenced model model_2") {
		t.Errorf("expected inclusive-bound debug 'Missing unreferenced model model_2', stdout: %q", out.String())
	}
	if strings.Contains(out.String(), "Missing model model_2") {
		t.Errorf("id==len(models) must take the printDebug branch, not printWarning; stdout: %q", out.String())
	}

	// id=3 must NOT be processed (loop stops at id == len(models)).
	if strings.Contains(out.String(), "model_3") {
		t.Errorf("loop must stop at id == len(models); model_3 should not appear; stdout: %q", out.String())
	}
}

// TestModels_NilOut verifies no panic when Out=nil.
func TestModels_NilOut(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildModelCache(t, cacheDir, []byte{1}, [][]byte{nil})
	if err := Models(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: nil}); err != nil {
		t.Fatalf("Models with nil Out: %v", err)
	}
}

// --- Anims unit tests ---

// TestAnims_OffsetTable verifies the section-offset arithmetic by constructing
// a 2-frame anim set and confirming the section reads produce the expected
// registration and file writes.
func TestAnims_OffsetTable(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Frame 0: flags=0x3 (bits 0+1 set) → 2 tran2 smarts.
	// Frame 1: flags=0x0 → no tran2.
	set := buildAnimSet(t,
		[]int{10, 20},
		[]byte{0x3, 0x0},
		[][]int32{{5, 7}, {}},
	)
	buildAnimSetCache(t, cacheDir, [][]byte{set})

	if err := Anims(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Anims: %v", err)
	}

	// anim_0.anim must exist.
	animPath := filepath.Join(srcDir, "models", "anim_0.anim")
	got, err := os.ReadFile(animPath)
	if err != nil {
		t.Fatalf("read anim_0.anim: %v", err)
	}
	if !bytes.Equal(got, set) {
		t.Errorf("anim_0.anim bytes differ")
	}

	// anim.pack must have frame entries for id=10 (anim_10) and id=20 (anim_20).
	animPack := filepath.Join(srcDir, "pack", "anim.pack")
	packBytes, err := os.ReadFile(animPack)
	if err != nil {
		t.Fatalf("read anim.pack: %v", err)
	}
	packStr := string(packBytes)
	if !strings.Contains(packStr, "10=anim_10") {
		t.Errorf("anim.pack missing 10=anim_10; got:\n%s", packStr)
	}
	if !strings.Contains(packStr, "20=anim_20") {
		t.Errorf("anim.pack missing 20=anim_20; got:\n%s", packStr)
	}

	// base.pack must have entry for id=0 (base_0).
	basePack := filepath.Join(srcDir, "pack", "base.pack")
	baseBytes, err := os.ReadFile(basePack)
	if err != nil {
		t.Fatalf("read base.pack: %v", err)
	}
	if !strings.Contains(string(baseBytes), "0=base_0") {
		t.Errorf("base.pack missing 0=base_0; got:\n%s", string(baseBytes))
	}
}

// TestAnims_StaleFileDeletion verifies that pre-existing .base and .frame
// files whose names match the registered names are deleted during the run.
func TestAnims_StaleFileDeletion(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Pre-populate stale files.
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models: %v", err)
	}
	staleBase := filepath.Join(modelsDir, "base_0.base")
	staleFrame := filepath.Join(modelsDir, "anim_10.frame")
	if err := os.WriteFile(staleBase, []byte("stale-base"), 0o644); err != nil {
		t.Fatalf("write stale base: %v", err)
	}
	if err := os.WriteFile(staleFrame, []byte("stale-frame"), 0o644); err != nil {
		t.Fatalf("write stale frame: %v", err)
	}

	// 1-frame anim set, frameId=10.
	set := buildAnimSet(t, []int{10}, []byte{0x0}, [][]int32{{}})
	buildAnimSetCache(t, cacheDir, [][]byte{set})

	if err := Anims(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Anims: %v", err)
	}

	// Stale files must be gone.
	if _, err := os.Stat(staleBase); err == nil {
		t.Errorf("stale base_0.base should have been deleted")
	}
	if _, err := os.Stat(staleFrame); err == nil {
		t.Errorf("stale anim_10.frame should have been deleted")
	}
}

// TestAnims_MissingSet verifies that a missing cache entry emits a warning
// and does not crash.
func TestAnims_MissingSet(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// set id=0 present, set id=1 absent (nil → 0xFF placeholder, decompression fails).
	set0 := buildAnimSet(t, []int{0}, []byte{0}, [][]int32{{}})
	buildAnimSetCache(t, cacheDir, [][]byte{set0, nil})

	var out bytes.Buffer
	if err := Anims(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Anims: %v", err)
	}

	if !strings.Contains(out.String(), "Missing anim set 1") {
		t.Errorf("expected 'Missing anim set 1' warning, got: %q", out.String())
	}

	// id=0 .anim must still exist.
	if _, err := os.Stat(filepath.Join(srcDir, "models", "anim_0.anim")); err != nil {
		t.Errorf("anim_0.anim missing: %v", err)
	}
}

// TestAnims_Tran2Consumption verifies the tran2 consumption for all three flag
// bits (0x1, 0x2, 0x4) — each set bit consumes one gsmart from tran2.
// A desync here would cause a panic on the next read or incorrect frame parsing.
func TestAnims_Tran2Consumption(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// 3 frames: flags 0x7 (all bits), 0x0, 0x5 (bits 0+2).
	set := buildAnimSet(t,
		[]int{1, 2, 3},
		[]byte{0x7, 0x0, 0x5},
		[][]int32{{1, 2, 3}, {}, {4, 5}},
	)
	buildAnimSetCache(t, cacheDir, [][]byte{set})

	// Must not panic or error.
	if err := Anims(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Anims: %v", err)
	}

	// All three frame IDs must be registered in anim.pack.
	packBytes, _ := os.ReadFile(filepath.Join(srcDir, "pack", "anim.pack"))
	packStr := string(packBytes)
	for _, entry := range []string{"1=anim_1", "2=anim_2", "3=anim_3"} {
		if !strings.Contains(packStr, entry) {
			t.Errorf("anim.pack missing %q; got:\n%s", entry, packStr)
		}
	}
}
