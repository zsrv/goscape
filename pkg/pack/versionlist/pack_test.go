// Package versionlist tests ports the TS versionlist/pack.ts contract at 9aadcec4.
package versionlist_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pack/versionlist"
)

// buildCache creates a fresh FileStream in dir and pre-populates it with
// synthetic entries for tests.
//
// Fixture layout:
//   - archive 1 (models): file 0 = [0x01, 0x02, 0x03, 0x00, 0x00] (3 payload bytes + 2-byte version trailer)
//     file 1 is absent (ModelPack.max=2, id=1 missing)
//   - archive 2 (animset): file 0 = [0xAA, 0x00, 0x00] (1 payload byte + 2-byte trailer)
//     AnimSetPack.max=1
//   - archive 3 (midi):    file 0 = [0xBB, 0x00, 0x00] (1 payload byte + 2-byte trailer)
//     MidiPack.max=1
//   - archive 4 (maps):    file 0 = [0xCC, 0x00, 0x00] (1 payload byte + 2-byte trailer)
//     MapPack.max=1
func buildCache(t *testing.T, cacheDir string) *filestream.FileStream {
	t.Helper()
	fs, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}

	// archive 1, file 0: 3 payload bytes + 2-byte version trailer.
	fs.Write(1, 0, []byte{0x01, 0x02, 0x03, 0x00, 0x01}, 0)
	// archive 2, file 0: 1 payload byte + 2-byte version trailer.
	fs.Write(2, 0, []byte{0xAA, 0x00, 0x01}, 0)
	// archive 3, file 0: 1 payload byte + 2-byte version trailer.
	fs.Write(3, 0, []byte{0xBB, 0x00, 0x01}, 0)
	// archive 4, file 0: 1 payload byte + 2-byte version trailer.
	fs.Write(4, 0, []byte{0xCC, 0x00, 0x01}, 0)

	return fs
}

// buildRegistry sets up a minimal Registry pointing at srcDir, with:
//   - model.pack: 0=model0, 1=model1 (max=2)
//   - animset.pack: 0=anim0 (max=1)
//   - anim.pack: 0=seq0 (max=1)
//   - midi.pack: 0=jingle0 (max=1)
//   - map.pack: 0=m50_100, 1=l50_100 (max=2)
func buildRegistry(t *testing.T, srcDir string) *pack.Registry {
	t.Helper()

	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}

	files := map[string]string{
		"model.pack":   "0=model0\n1=model1\n",
		"animset.pack": "0=anim0\n",
		"anim.pack":    "0=seq0\n",
		"midi.pack":    "0=jingle0\n",
		"map.pack":     "0=m50_100\n1=l50_100\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(packDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Since Engine-TS 8139461a, anim_index is derived by parsing each packed
	// animset's .anim source, so every animset present in the cache needs one
	// (versionlist/pack.ts:101-121 @1d25566c uses a non-null assertion on the
	// lookup, so a missing source is a hard error there too).
	//
	// anim0 declares one frame, frame id 0: p2(frameCount=1), p2(frame=0),
	// p1(groupCount) — the group byte is skipped, not read.
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models: %v", err)
	}
	anim0 := []byte{0x00, 0x01, 0x00, 0x00, 0x00}
	if err := os.WriteFile(filepath.Join(modelsDir, "anim0.anim"), anim0, 0o644); err != nil {
		t.Fatalf("write anim0.anim: %v", err)
	}

	return &pack.Registry{SrcDir: srcDir}
}

// TestPack_NilCacheNoOp verifies that a nil cache causes Pack to no-op without error.
func TestPack_NilCacheNoOp(t *testing.T) {
	dir := t.TempDir()
	reg := buildRegistry(t, dir)
	if err := versionlist.Pack(reg, dir, dir, nil, nil); err != nil {
		t.Fatalf("expected no error with nil cache, got: %v", err)
	}
	// No output file should be created.
	if _, err := os.Stat(filepath.Join(dir, "client", "versionlist")); !os.IsNotExist(err) {
		t.Errorf("expected no output file with nil cache")
	}
}

// TestPack_ModelSection verifies the model_version/model_crc/model_index members.
//
// Fixture: ModelPack.max=2; file 0 present = [payload:3 bytes, trailer:2 bytes];
// file 1 absent. modelFlags = [0x42, 0] (only id 0 has a flag).
//
// Expected:
//   - model_version: p2(1), p2(0)       = [0x00,0x01, 0x00,0x00]
//   - model_crc:     p4(CRC(payload[0:3])), p4(0)
//   - model_index:   p1(0x42), p1(0)
func TestPack_ModelSection(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	// Build maps/free2play.csv (empty — no prefetch entries).
	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte("// comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create jingles dir (no files).
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	modelFlags := []int{0x42, 0}
	if err := versionlist.Pack(reg, srcDir, outDir, modelFlags, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	// --- model_version ---
	mvPkt, err := jag.Read("model_version")
	if err != nil {
		t.Fatalf("model_version: %v", err)
	}
	wantMV := []byte{0x00, 0x01, 0x00, 0x00}
	if !bytes.Equal(mvPkt.Data, wantMV) {
		t.Errorf("model_version: got %v, want %v", mvPkt.Data, wantMV)
	}

	// --- model_crc ---
	// The stored file for id=0 is [0x01, 0x02, 0x03, 0x00, 0x01].
	// CRC excludes the trailing 2 bytes → CRC over [0x01, 0x02, 0x03].
	payload := []byte{0x01, 0x02, 0x03}
	wantCRC := packet.GetCRC(payload, 0, len(payload))
	mcPkt, err := jag.Read("model_crc")
	if err != nil {
		t.Fatalf("model_crc: %v", err)
	}
	gotCRC := binary.BigEndian.Uint32(mcPkt.Data[0:4])
	if gotCRC != wantCRC {
		t.Errorf("model_crc[0]: got 0x%08x, want 0x%08x", gotCRC, wantCRC)
	}
	// id=1 absent → CRC = 0.
	gotCRC1 := binary.BigEndian.Uint32(mcPkt.Data[4:8])
	if gotCRC1 != 0 {
		t.Errorf("model_crc[1]: got 0x%08x, want 0", gotCRC1)
	}

	// --- model_index ---
	miPkt, err := jag.Read("model_index")
	if err != nil {
		t.Fatalf("model_index: %v", err)
	}
	if miPkt.Data[0] != 0x42 {
		t.Errorf("model_index[0]: got 0x%02x, want 0x42", miPkt.Data[0])
	}
	if miPkt.Data[1] != 0x00 {
		t.Errorf("model_index[1]: got 0x%02x, want 0x00", miPkt.Data[1])
	}
}

// TestPack_ModelFlagsOutOfBounds verifies that modelFlags shorter than
// ModelPack.max is handled safely (missing entries treated as 0).
func TestPack_ModelFlagsOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Only 1 flag for 2 models — id=1 is out of bounds.
	modelFlags := []int{0x10}
	if err := versionlist.Pack(reg, srcDir, outDir, modelFlags, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	miPkt, err := jag.Read("model_index")
	if err != nil {
		t.Fatalf("model_index: %v", err)
	}
	if miPkt.Data[0] != 0x10 {
		t.Errorf("model_index[0]: got 0x%02x, want 0x10", miPkt.Data[0])
	}
	if miPkt.Data[1] != 0x00 {
		t.Errorf("model_index[1] (OOB should be 0): got 0x%02x, want 0", miPkt.Data[1])
	}
}

// TestPack_CRCExcludesTrailer verifies the core CRC semantics: the CRC
// is computed over all bytes except the trailing 2-byte version field.
//
// We write a known payload to archive 1 file 0 and check that:
//   - CRC(full_data[0 : len-2]) == what Pack writes into model_crc
//   - CRC(full_data[0 : len])   != what Pack writes (confirms trailer exclusion)
func TestPack_CRCExcludesTrailer(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Custom cache: single model with distinct payload.
	fullData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x05} // 4 payload + 2 trailer

	fs, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	defer fs.Close()
	fs.Write(1, 0, fullData, 0) // write WITHOUT version appended (already in fullData)
	// Write dummy entries for other archives so the jag build succeeds.
	fs.Write(2, 0, []byte{0xAA, 0x00, 0x00}, 0)
	fs.Write(3, 0, []byte{0xBB, 0x00, 0x00}, 0)
	fs.Write(4, 0, []byte{0xCC, 0x00, 0x00}, 0)

	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	packFiles := map[string]string{
		"model.pack":   "0=model0\n",
		"animset.pack": "0=anim0\n",
		"anim.pack":    "0=seq0\n",
		"midi.pack":    "0=jingle0\n",
		"map.pack":     "0=m50_100\n1=l50_100\n",
	}
	for name, content := range packFiles {
		if err := os.WriteFile(filepath.Join(packDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// anim_index needs the animset's .anim source (see buildRegistry).
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "anim0.anim"), []byte{0x00, 0x01, 0x00, 0x00, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: srcDir}
	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	mcPkt, err := jag.Read("model_crc")
	if err != nil {
		t.Fatalf("model_crc: %v", err)
	}
	gotCRC := binary.BigEndian.Uint32(mcPkt.Data[0:4])

	// Payload = all bytes except last 2.
	payloadOnly := fullData[:len(fullData)-2]
	wantCRC := packet.GetCRC(payloadOnly, 0, len(payloadOnly))
	if gotCRC != wantCRC {
		t.Errorf("CRC: got 0x%08x, want 0x%08x (trailer NOT excluded)", gotCRC, wantCRC)
	}

	// Confirm full-data CRC would differ.
	fullCRC := packet.GetCRC(fullData, 0, len(fullData))
	if gotCRC == fullCRC {
		t.Errorf("CRC matches full-data CRC — trailer was NOT excluded as required")
	}
}

// TestPack_AnimSection verifies anim_version/anim_crc have 1 entry and
// anim_index is AnimPack.max p2 entries carrying the 1-based owning animset.
func TestPack_AnimSection(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	// anim_version: AnimSetPack.max=1, file 0 present → [0x00, 0x01]
	avPkt, err := jag.Read("anim_version")
	if err != nil {
		t.Fatalf("anim_version: %v", err)
	}
	if len(avPkt.Data) != 2 {
		t.Errorf("anim_version: got %d bytes, want 2", len(avPkt.Data))
	}
	wantAV := []byte{0x00, 0x01}
	if !bytes.Equal(avPkt.Data, wantAV) {
		t.Errorf("anim_version: got %v, want %v", avPkt.Data, wantAV)
	}

	// anim_crc: 4 bytes for 1 animset.
	acPkt, err := jag.Read("anim_crc")
	if err != nil {
		t.Fatalf("anim_crc: %v", err)
	}
	if len(acPkt.Data) != 4 {
		t.Errorf("anim_crc: got %d bytes, want 4", len(acPkt.Data))
	}

	// anim_index: AnimPack.max=1 entry. Frame 0 is declared by animset 0
	// (anim0.anim), so the value is the 1-based animset id = 1.
	aiPkt, err := jag.Read("anim_index")
	if err != nil {
		t.Fatalf("anim_index: %v", err)
	}
	if len(aiPkt.Data) != 2 {
		t.Errorf("anim_index: got %d bytes, want 2 (AnimPack.max×p2)", len(aiPkt.Data))
	}
	if aiPkt.Data[0] != 0 || aiPkt.Data[1] != 1 {
		t.Errorf("anim_index: got %v, want [0x00, 0x01]", aiPkt.Data)
	}
}

// TestPack_MidiJinglePrefetch verifies that midi_index.pbool reflects
// jingle file presence in <srcDir>/jingles/<name>.mid.
//
// Fixture: midi name = "jingle0"; a real .mid file is created.
func TestPack_MidiJinglePrefetch(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	// Create the jingles directory WITH the .mid file.
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jinglesDir, "jingle0.mid"), []byte{0x4D, 0x54, 0x68, 0x64}, 0o644); err != nil {
		t.Fatal(err)
	}

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	miPkt, err := jag.Read("midi_index")
	if err != nil {
		t.Fatalf("midi_index: %v", err)
	}
	// MidiPack.max=1, file present → pbool(true) = 1.
	if miPkt.Data[0] != 1 {
		t.Errorf("midi_index[0] (jingle present): got %d, want 1", miPkt.Data[0])
	}
}

// TestPack_MidiJingleAbsent verifies midi_index.pbool is 0 when the .mid file
// does not exist.
func TestPack_MidiJingleAbsent(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	// Create empty jingles dir — NO .mid file.
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	miPkt, err := jag.Read("midi_index")
	if err != nil {
		t.Fatalf("midi_index: %v", err)
	}
	if miPkt.Data[0] != 0 {
		t.Errorf("midi_index[0] (jingle absent): got %d, want 0", miPkt.Data[0])
	}
}

// TestPack_MapIndex verifies the map_index encoding:
//   - p2(region) p2(mapId) p2(locMapId) pbool(prefetch)
//
// Registry: map.pack has "m50_100" → 0, "l50_100" → 1.
// free2play.csv: one entry "50_100_50_100" (mx=100, mz=50 → no,
// actually parse as: _y=50,mx=100,mz=50 — wait, let's use the TS field
// order: [_y, mx, mz, _lx, _lz] = line.split('_').map(Number)).
//
// To get region=(mapX<<8)|mapZ = (50<<8)|100 = 0x3264 = 12900,
// the csv line must contain "_50_100_" so that split('_') gives
// [_y, mx=50, mz=100, _lx, _lz]. Use "0_50_100_0_0" as the line.
//
// map_index for mapX=50, mapZ=100:
//
//	p2(0x3264) p2(0) p2(1) pbool(true) → [0x32,0x64, 0x00,0x00, 0x00,0x01, 0x01]
func TestPack_MapIndex(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	// free2play.csv: one valid line placing region (50,100) in prefetch.
	// Line format: "_y_mx_mz_lx_lz" → split('_') → ["", y, mx, mz, lx, lz].
	// TS: [_y, mx, mz, _lx, _lz] = line.split('_').map(Number)
	// so for "0_50_100_0_0": split gives ["0","50","100","0","0"] → mx=50, mz=100.
	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	csvContent := "0_50_100_0_0\n"
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(csvContent), 0o644); err != nil {
		t.Fatal(err)
	}

	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	miPkt, err := jag.Read("map_index")
	if err != nil {
		t.Fatalf("map_index: %v", err)
	}

	// Expect 7 bytes: p2(region) p2(mapId) p2(locMapId) pbool(prefetch).
	// region = (50<<8)|100 = 12900 = 0x3264
	// mapId = 0 (m50_100 → id 0)
	// locMapId = 1 (l50_100 → id 1)
	// prefetch = true (region 0x3264 is in free2play.csv)
	want := []byte{0x32, 0x64, 0x00, 0x00, 0x00, 0x01, 0x01}
	if !bytes.Equal(miPkt.Data, want) {
		t.Errorf("map_index: got %v, want %v", miPkt.Data, want)
	}
}

// TestPack_MapNoPrefetch verifies that a map region NOT in free2play.csv
// gets pbool(false) = 0.
func TestPack_MapNoPrefetch(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	// Empty free2play.csv — no prefetch entries.
	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte("// no entries\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	miPkt, err := jag.Read("map_index")
	if err != nil {
		t.Fatalf("map_index: %v", err)
	}

	// Last byte (pbool) should be 0 (not prefetched).
	if len(miPkt.Data) < 7 {
		t.Fatalf("map_index too short: %d bytes", len(miPkt.Data))
	}
	if miPkt.Data[6] != 0 {
		t.Errorf("map_index prefetch byte: got %d, want 0", miPkt.Data[6])
	}
}

// TestPack_JagWrittenToCacheFile5 verifies that the jag file bytes written to
// disk exactly match what cache.Read(0, 5, false) returns.
func TestPack_JagWrittenToCacheFile5(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Read on-disk file.
	diskBytes, err := os.ReadFile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("read disk: %v", err)
	}

	// Read from cache(0, 5).
	cacheBytes := fs.Read(0, 5, false)
	if cacheBytes == nil {
		t.Fatal("cache.Read(0,5,false) returned nil")
	}

	if !bytes.Equal(diskBytes, cacheBytes) {
		t.Errorf("disk vs cache bytes differ: disk=%d bytes, cache=%d bytes", len(diskBytes), len(cacheBytes))
	}
}

// TestPack_AllMembersPresent verifies all 12 expected members are in the jag.
func TestPack_AllMembersPresent(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fs := buildCache(t, cacheDir)
	defer fs.Close()
	reg := buildRegistry(t, srcDir)

	mapsDir := filepath.Join(srcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	jinglesDir := filepath.Join(srcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := versionlist.Pack(reg, srcDir, outDir, nil, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "versionlist"))
	if err != nil {
		t.Fatalf("load jag: %v", err)
	}

	members := []string{
		"model_version", "model_crc", "model_index",
		"anim_version", "anim_crc", "anim_index",
		"midi_version", "midi_crc", "midi_index",
		"map_version", "map_crc", "map_index",
	}
	for _, name := range members {
		if _, err := jag.Read(name); err != nil {
			t.Errorf("missing jag member %q: %v", name, err)
		}
	}
}
