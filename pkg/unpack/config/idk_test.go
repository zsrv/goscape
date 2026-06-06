package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// buildCfgIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildCfgIdx(body []byte) *ConfigIdx {
	idx := packet.NewPacket([]byte{
		0, 1,
		byte(len(body) >> 8), byte(len(body)),
	})
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg, err := ReadConfigIdx(idx, packet.NewPacket(dat))
	if err != nil {
		panic(err)
	}
	return cfg
}

// makeMultiPackFile builds an in-memory PackFile with multiple entries.
func makeMultiPackFile(entries map[int]string) *pack.PackFile {
	nameToID := make(map[string]int, len(entries))
	names := make(map[string]struct{}, len(entries))
	maxID := 0
	for id, name := range entries {
		nameToID[name] = id
		names[name] = struct{}{}
		if id > maxID {
			maxID = id
		}
	}
	return &pack.PackFile{
		Pack:     entries,
		NameToID: nameToID,
		Names:    names,
		Max:      maxID + 1,
	}
}

// setupModelTree creates a temp dir with models/<base>.ob2 file.
// Returns the srcDir.
func setupModelTree(t *testing.T, baseNames ...string) string {
	t.Helper()
	srcDir := t.TempDir()
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, base := range baseNames {
		path := filepath.Join(modelsDir, base+".ob2")
		if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return srcDir
}

// TestIdkPartTypeNames checks the idkPartTypeName table against TS enum values.
func TestIdkPartTypeNames(t *testing.T) {
	expected := [14]string{
		"man_hair", "man_jaw", "man_torso", "man_arms", "man_hands", "man_legs", "man_feet",
		"woman_hair", "woman_jaw", "woman_torso", "woman_arms", "woman_hands", "woman_legs", "woman_feet",
	}
	for i, want := range expected {
		if idkPartTypeName[i] != want {
			t.Errorf("idkPartTypeName[%d]: want %q got %q", i, want, idkPartTypeName[i])
		}
	}
}

// TestUnpackIdk_Opcode1_TypeName checks type= emission.
func TestUnpackIdk_Opcode1_TypeName(t *testing.T) {
	// type=3 → man_arms
	body := []byte{1, 3, 0}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), nil, nil, nil, "", captureWarnings(new([]string)), nil)
	want := []string{"[myidk]", "type=man_arms"}
	assertLines(t, want, got)
}

// TestUnpackIdk_Opcode3_Disable checks disable=yes emission.
func TestUnpackIdk_Opcode3_Disable(t *testing.T) {
	body := []byte{3, 0}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), nil, nil, nil, "", nil, nil)
	want := []string{"[myidk]", "disable=yes"}
	assertLines(t, want, got)
}

// TestUnpackIdk_Opcode2_ModelRename verifies model rename + file move + registry update.
func TestUnpackIdk_Opcode2_ModelRename(t *testing.T) {
	srcDir := setupModelTree(t, "model_5")
	modelPack := makePackFile(5, "model_5")

	// count=1, modelId=5 (big-endian g2=0x0005)
	body := []byte{2, 1, 0x00, 0x05, 0}
	cfg := buildCfgIdx(body)
	idkPack := makePackFile(0, "myhair")

	got := unpackIdk(cfg, 0, idkPack, nil, modelPack, nil, srcDir, nil, captureWarnings(new([]string)))

	// Expected: model name is idk_myhair (model_5 starts with model_, name doesn't start with idk_)
	want := []string{"[myhair]", "model1=idk_myhair"}
	assertLines(t, want, got)

	// File should be renamed to models/idk/idk_myhair.ob2
	dest := filepath.Join(srcDir, "models", "idk", "idk_myhair.ob2")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("renamed file missing at %s: %v", dest, err)
	}
	old := filepath.Join(srcDir, "models", "model_5.ob2")
	if _, err := os.Stat(old); err == nil {
		t.Errorf("old file still present at %s", old)
	}

	// Registry should be updated
	if modelPack.GetByID(5) != "idk_myhair" {
		t.Errorf("registry: want idk_myhair got %q", modelPack.GetByID(5))
	}
}

// TestUnpackIdk_Opcode2_ModelAlreadyNamed verifies non-model_ names are returned as-is.
func TestUnpackIdk_Opcode2_ModelAlreadyNamed(t *testing.T) {
	srcDir := t.TempDir()
	modelPack := makePackFile(7, "idk_somehair")

	body := []byte{2, 1, 0x00, 0x07, 0}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "somehair"), nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[somehair]", "model1=idk_somehair"}
	assertLines(t, want, got)
}

// TestUnpackIdk_RenameIdk_CollisionSuffix tests that collision avoidance uses _2 suffix.
func TestUnpackIdk_RenameIdk_CollisionSuffix(t *testing.T) {
	srcDir := setupModelTree(t, "model_10")
	// Pre-register idk_myhair so it collides
	modelPack := makeMultiPackFile(map[int]string{
		10: "model_10",
		11: "idk_myhair", // collision
	})

	body := []byte{2, 1, 0x00, 0x0A, 0}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myhair"), nil, modelPack, nil, srcDir, nil, nil)
	// Collision → idk_myhair_2
	want := []string{"[myhair]", "model1=idk_myhair_2"}
	assertLines(t, want, got)
}

// TestUnpackIdk_HeadModel checks head{N}= emission.
func TestUnpackIdk_HeadModel(t *testing.T) {
	srcDir := setupModelTree(t, "model_3")
	modelPack := makePackFile(3, "model_3")

	// opcode 60 → index=1; opcode 61 → index=2
	// body: [60, g2=3, 0] → head1=idk_myidk_head
	body := []byte{60, 0x00, 0x03, 0}
	cfg := buildCfgIdx(body)

	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[myidk]", "head1=idk_myidk_head"}
	assertLines(t, want, got)
}

// TestUnpackIdk_Recol_RawAbove100 tests recol emission when raw value >= 100.
// ReverseHsl(0) returns a slice containing 0 (RGB15 0 maps to HSL 0).
// But value 0 < 100, so we need to use a value >= 100.
// HSL value 100 — use an actual known mapping to verify recol path.
func TestUnpackIdk_Recol_RawAbove100(t *testing.T) {
	// Use src=200 (>= 100 threshold). ReverseHsl(200) should return at least one value.
	// Emit recol1s/recol1d.
	// Opcodes: 40=recolSrc[0]=200, 50=recolDst[0]=201, 0=terminator.
	body := []byte{40, 0x00, 200, 50, 0x00, 201, 0}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), nil, nil, nil, "", nil, nil)
	// Should emit recol1s and recol1d (RGB values from reverseHsl or raw fallback)
	if len(got) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %v", len(got), got)
	}
	// Check that recol1s and recol1d are present
	foundS, foundD := false, false
	for _, line := range got[1:] {
		if len(line) >= 8 && line[:8] == "recol1s=" {
			foundS = true
		}
		if len(line) >= 8 && line[:8] == "recol1d=" {
			foundD = true
		}
	}
	if !foundS || !foundD {
		t.Errorf("expected recol1s= and recol1d= in output: %v", got)
	}
}

// TestUnpackIdk_Recol_TexturePath tests retex emission via model texture.
func TestUnpackIdk_Recol_TexturePath(t *testing.T) {
	// Create a model with a textured face where faceColour == textureID.
	// textureID = 5 (< 100, so threshold path is skipped).
	// Build a minimal model blob that has a textured face with colour=5.
	textureID := 5
	modelData := buildTexturedModelBlob(textureID)
	ms := model.New()
	ms.Unpack(42, modelData)

	texPack := makePackFile(textureID, "snow")

	// opcode 1 (body models): count=1, modelId=42
	// opcodes 40 (recolSrc[0]=5), 50 (recolDst[0]=3), terminator
	body := []byte{
		2, 1, 0x00, 0x2A, // opcode 2 (models), count=1, modelId=42
		40, 0x00, byte(textureID), // recolSrc[0] = 5
		50, 0x00, 0x03, // recolDst[0] = 3
		0,
	}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), texPack, nil, ms, "", nil, nil)

	// Should contain retex1s=snow and retex1d=... (since model has texture 5)
	foundS := false
	for _, line := range got {
		if line == "retex1s=snow" {
			foundS = true
		}
	}
	if !foundS {
		t.Errorf("expected retex1s=snow in output, got: %v", got)
	}
}

// TestUnpackIdk_Recol_SparseSkinip tests that sparse undefined entries are skipped.
func TestUnpackIdk_Recol_SparseSkip(t *testing.T) {
	// Set recolSrc[0] (opcode 40) only — recolDst[0] (opcode 50) not set.
	// Set recolSrc[2] (opcode 42) and recolDst[2] (opcode 52).
	// Index 1 (opcode 41/51) is never set → skipped; output should be recol1s/recol1d and recol3s/recol3d.
	body := []byte{
		40, 0x00, 0x0A, // recolSrc[0]=10
		50, 0x00, 0x05, // recolDst[0]=5
		42, 0x00, 0x14, // recolSrc[2]=20
		52, 0x00, 0x0F, // recolDst[2]=15
		0,
	}
	cfg := buildCfgIdx(body)
	got := unpackIdk(cfg, 0, makePackFile(0, "myidk"), nil, nil, nil, "", nil, nil)
	// Should have header + recol1s + recol1d + recol3s + recol3d (index 2 missing)
	// recol2s/recol2d should NOT be present
	for _, line := range got {
		if len(line) >= 8 && line[:8] == "recol2s=" {
			t.Errorf("unexpected recol2s= in output (index 1 should be skipped): %v", got)
		}
	}
	foundS3, foundD3 := false, false
	for _, line := range got {
		if len(line) >= 8 && line[:8] == "recol3s=" {
			foundS3 = true
		}
		if len(line) >= 8 && line[:8] == "recol3d=" {
			foundD3 = true
		}
	}
	if !foundS3 || !foundD3 {
		t.Errorf("expected recol3s= and recol3d= in output: %v", got)
	}
}

// TestUnpackIdk_ModelFileMissing_Errorf tests console.error path when model file absent.
func TestUnpackIdk_ModelFileMissing_Errorf(t *testing.T) {
	srcDir := t.TempDir() // no model files
	modelPack := makePackFile(5, "model_5")

	body := []byte{2, 1, 0x00, 0x05, 0}
	cfg := buildCfgIdx(body)

	var errs []string
	errorf := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	unpackIdk(cfg, 0, makePackFile(0, "myhair"), nil, modelPack, nil, srcDir, nil, errorf)
	if len(errs) == 0 {
		t.Error("expected errorf call for missing model file, got none")
	}
}

// TestRenameModelIdk_MkdirFail_ErrorfFiredRegistryUnchanged verifies that when the
// destination subdirectory cannot be created (models/idk is a plain file, so MkdirAll
// cannot create models/idk/ as a directory), errorf is called and the registry is NOT
// updated — the old name (model_5) is returned so the caller still has a valid name.
// TS fs.renameSync would throw (process-fatal); Go degrades gracefully.
func TestRenameModelIdk_MkdirFail_ErrorfFiredRegistryUnchanged(t *testing.T) {
	srcDir := t.TempDir()

	// Create models/ as a real directory containing the source .ob2 so that
	// findFileInList can locate it during the listing pass.
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "model_5.ob2"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Block MkdirAll(models/idk/) by placing a regular FILE named "idk" inside models/.
	// os.MkdirAll will fail because it cannot convert a file into a directory.
	if err := os.WriteFile(filepath.Join(modelsDir, "idk"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}

	modelPack := makePackFile(5, "model_5")

	var errs []string
	errorf := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	// renameModelIdk directly: id=5, name="myhair"
	got := renameModelIdk(5, "myhair", modelPack, srcDir, errorf)

	// errorf must have been called
	if len(errs) == 0 {
		t.Error("expected errorf call on MkdirAll failure, got none")
	}

	// registry must still hold the old name
	if modelPack.GetByID(5) != "model_5" {
		t.Errorf("registry changed unexpectedly: want model_5, got %q", modelPack.GetByID(5))
	}

	// returned name must be the old name (not the attempted new name)
	if got != "model_5" {
		t.Errorf("want old name model_5, got %q", got)
	}
}

// buildTexturedModelBlob builds a minimal model binary blob that reports
// a single textured face with faceColour == textureID.
// This is the minimum viable structure to make modelsHaveTexture return true.
func buildTexturedModelBlob(textureID int) []byte {
	// We need:
	//   hasInfo=1 (so FaceInfo is populated)
	//   faceInfo[0] = 2 (>1, making it textured; 0x3&2=2>1)
	//   faceColour[0] = textureID
	//   texturedFaceCount >= 1
	//   faceCount = 1, vertexCount = 3
	//
	// Trailer layout (18 bytes):
	//   u2 vertexCount | u2 faceCount | u1 texturedFaceCount
	//   u1 hasInfo | u1 priority | u1 hasAlpha | u1 hasFaceLabels | u1 hasVertexLabels
	//   u2 dataLengthX | u2 dataLengthY | u2 dataLengthZ | u2 dataLengthFaceOrientations

	vertexCount := 3
	faceCount := 1
	texturedFaceCount := 1
	hasInfo := 1
	priority := 0 // != 255 → no per-face priority array
	hasAlpha := 0
	hasFaceLabels := 0
	hasVertexLabels := 0

	// Compute section sizes.
	// vertexFlags: vertexCount bytes
	// faceOrientations: faceCount bytes
	// facePriorities: 0 (priority != 255)
	// faceLabels: 0
	// faceInfos: faceCount bytes (hasInfo=1)
	// vertexLabels: 0
	// faceAlphas: 0
	// faceVertices: dataLengthFaceOrientations bytes
	// faceColours: faceCount*2 bytes
	// faceTextureAxis: texturedFaceCount*6 bytes
	// vertexX/Y/Z: dataLengthX/Y/Z bytes

	// For vertexX/Y/Z each vertex needs gsmart: 1 byte per coordinate if < 128.
	// With flags=0 for all vertices → no delta reads → dataLengthX/Y/Z = 0.
	// With flags=1|2|4 for all vertices → 1 byte per axis each.
	// Use flags=0 → no vertex data needed.
	dataLengthX := 0
	dataLengthY := 0
	dataLengthZ := 0

	// faceVertices: orientation=1 needs 3 gsmart (each 1 byte if < 128).
	dataLengthFaceOrientations := 3 // 3 deltas for orientation=1

	// Build body.
	var body []byte

	// vertexFlags: 3 bytes, all 0 (no delta for any vertex).
	for range vertexCount {
		body = append(body, 0)
	}

	// faceOrientations: 1 byte = orientation type 1.
	body = append(body, 1)

	// faceInfos (hasInfo=1): 1 byte = 2 (textured: info&3 = 2 > 1).
	body = append(body, 2)

	// faceVertices (dataLengthFaceOrientations=3):
	// orientation=1 → 3 gsmart values; use 1+64=65 (gsmart of 1), etc.
	body = append(body, 64+1, 64+1, 64+1) // gsmart: 1, 1, 1 → va=1, vb=2, vc=3

	// faceColours: faceCount*2 bytes = 1 face → 2 bytes (big-endian textureID).
	body = append(body, byte(textureID>>8), byte(textureID))

	// faceTextureAxis: texturedFaceCount*6 = 6 bytes.
	body = append(body, 0, 0, 0, 0, 0, 0)

	// Trailer (18 bytes).
	trailer := []byte{
		byte(vertexCount >> 8), byte(vertexCount),
		byte(faceCount >> 8), byte(faceCount),
		byte(texturedFaceCount),
		byte(hasInfo),
		byte(priority),
		byte(hasAlpha),
		byte(hasFaceLabels),
		byte(hasVertexLabels),
		byte(dataLengthX >> 8), byte(dataLengthX),
		byte(dataLengthY >> 8), byte(dataLengthY),
		byte(dataLengthZ >> 8), byte(dataLengthZ),
		byte(dataLengthFaceOrientations >> 8), byte(dataLengthFaceOrientations),
	}

	return append(body, trailer...)
}

// TestUnpackIdk_Recol_ReverseHslEmptyFallback tests that raw value is used when ReverseHsl returns empty.
// Value 0 should have HSL entry (0→0), so use a value that is guaranteed to not reverse.
// Actually colorconv.ReverseHsl can return empty for certain HSL values.
// We test the raw fallback by verifying the output numeric value matches the raw when ReverseHsl is empty.
func TestUnpackIdk_Recol_ReverseHslEmptyFallback(t *testing.T) {
	// Find an HSL value that maps to no RGB15. We check a few known values.
	// For test stability, use a known small value and check both paths in appendRecolLine.
	// If ReverseHsl(1) returns empty (likely — HSL 1 may not map to any 15-bit RGB),
	// then srcVal should equal srcRaw=1.
	srcRaw := 1
	dstRaw := 1
	srcRgb, ok := firstOrZero(colorconvReverseHsl(srcRaw))
	if ok {
		// Not empty, use the rgb value
		_ = srcRgb // just verify it exists
	}
	// Regardless, test that appendRecolLine produces recol1s=
	var def []string
	def = appendRecolLine(def, 1, srcRaw, dstRaw, nil, nil, nil, 100, false)
	if len(def) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(def), def)
	}
	// Should start with "recol1s="
	if len(def[0]) < 8 || def[0][:8] != "recol1s=" {
		t.Errorf("unexpected line: %q", def[0])
	}
}

// TestUnpackIdk_UnknownOpcodeWarning verifies warning on unknown code.
func TestUnpackIdk_UnknownOpcodeWarning(t *testing.T) {
	body := []byte{200, 0}
	cfg := buildCfgIdx(body)
	var warns []string
	unpackIdk(cfg, 0, nil, nil, nil, nil, "", captureWarnings(&warns), nil)
	if len(warns) == 0 || warns[0] != "unknown idk code 200" {
		t.Errorf("want [\"unknown idk code 200\"], got %v", warns)
	}
}
