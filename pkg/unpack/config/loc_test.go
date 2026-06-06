package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildLocCfgIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildLocCfgIdx(body []byte) *ConfigIdx {
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

// TestLocShapeSuffix_Table spot-checks key entries in the LocShapeSuffix table.
// TS source: LocConfig.ts:131-155.
func TestLocShapeSuffix_Table(t *testing.T) {
	cases := []struct {
		shape  int
		suffix string
	}{
		{0, "_1"},   // wall_straight
		{1, "_2"},   // wall_diagonalcorner
		{4, "_q"},   // walldecor_straight_nooffset
		{5, "_w"},   // walldecor_straight_offset
		{9, "_5"},   // wall_diagonal
		{10, "_8"},  // centrepiece_straight
		{22, "_0"},  // grounddecor
		{18, "_z"},  // roofedge_straight
		{21, "_v"},  // roofedge_squarecorner
	}
	for _, tc := range cases {
		if LocShapeSuffix[tc.shape] != tc.suffix {
			t.Errorf("LocShapeSuffix[%d]: want %q got %q", tc.shape, tc.suffix, LocShapeSuffix[tc.shape])
		}
	}
}

// TestUnpackLocModels_Opcode1_CollectsModels verifies model-shape pairs are collected.
func TestUnpackLocModels_Opcode1_CollectsModels(t *testing.T) {
	// opcode 1, count=2, (model=100,shape=0), (model=101,shape=4), terminator
	body := []byte{1, 2, 0x00, 100, 0, 0x00, 101, 4, 0}
	cfg := buildLocCfgIdx(body)
	result := unpackLocModels(cfg, 0, nil)
	if len(result.Models) != 2 {
		t.Fatalf("want 2 models, got %d", len(result.Models))
	}
	if result.Models[0].Model != 100 || result.Models[0].Shape != 0 {
		t.Errorf("models[0]: want {100,0} got %+v", result.Models[0])
	}
	if result.Models[1].Model != 101 || result.Models[1].Shape != 4 {
		t.Errorf("models[1]: want {101,4} got %+v", result.Models[1])
	}
}

// TestUnpackLocModels_LdModels_AlwaysEmpty verifies ldModels is always empty.
func TestUnpackLocModels_LdModels_AlwaysEmpty(t *testing.T) {
	body := []byte{1, 1, 0x00, 5, 0, 0}
	cfg := buildLocCfgIdx(body)
	result := unpackLocModels(cfg, 0, nil)
	if len(result.LdModels) != 0 {
		t.Errorf("expected empty ldModels, got %d entries", len(result.LdModels))
	}
}

// TestUnpackLocModels_SkipAllOpcodes verifies all non-model opcodes are consumed.
func TestUnpackLocModels_SkipAllOpcodes(t *testing.T) {
	// Sequence of skip-read opcodes followed by terminator.
	body := []byte{
		14, 3,       // width=3
		15, 2,       // length=2
		17,          // no-op (blockwalk)
		18,          // no-op (blockrange)
		19, 1,       // active=true
		21,          // no-op (hillskew)
		22,          // no-op (sharelight)
		23,          // no-op (occlude)
		24, 0x00, 5, // seq anim
		25,          // no-op (hasalpha)
		28, 4,       // wallwidth=4
		29, 0xFE,    // ambient=-2 (g1b)
		39, 0x02,    // contrast=2 (g1b)
		30, 0x41, 0x0a, // op1 "A" + LF
		40, 1, 0x00, 10, 0x00, 20, // recol pair
		60, 0x00, 7, // mapfunction=7
		62,          // no-op (mirror)
		64,          // no-op (shadow)
		65, 0x00, 100, // resizex
		66, 0x00, 100, // resizey
		67, 0x00, 100, // resizez
		68, 0x00, 5,   // mapscene
		69, 0x0F,      // forceapproach flags
		70, 0x00, 5,   // offsetx
		71, 0x00, 5,   // offsety
		72, 0x00, 5,   // offsetz
		73,            // no-op (forcedecor)
		74,            // no-op
		75, 1,         // bool
		0,             // terminator
	}
	cfg := buildLocCfgIdx(body)
	// Should not panic; models should be empty.
	result := unpackLocModels(cfg, 0, nil)
	if len(result.Models) != 0 {
		t.Errorf("expected no models, got %d", len(result.Models))
	}
}

// TestUnpackLocModels_UnknownOpcode_BailsAndWarns verifies that an unknown opcode in
// unpackLocModels triggers warnf, returns models collected so far, and does not hang.
// TS has no default case and would loop forever; Go bails (no parity implication —
// no reference output exists for data that hangs TS).
func TestUnpackLocModels_UnknownOpcode_BailsAndWarns(t *testing.T) {
	// opcode 1 with one model, then unknown opcode 200, then a terminator that would
	// never be reached in TS (infinite loop). Go must return before the terminator.
	body := []byte{
		1, 1, 0x00, 42, 5, // opcode 1: count=1, model=42, shape=5
		200,               // unknown opcode — triggers bail
		0,                 // terminator (unreachable in TS; Go bails before this)
	}
	cfg := buildLocCfgIdx(body)

	var warns []string
	warnf := func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }

	result := unpackLocModels(cfg, 0, warnf)

	// warning must be fired
	if len(warns) == 0 {
		t.Fatal("expected warnf call for unknown opcode, got none")
	}
	if warns[0] != "unknown loc model code 200" {
		t.Errorf("warning mismatch: want %q got %q", "unknown loc model code 200", warns[0])
	}

	// models collected BEFORE the unknown opcode must be returned
	if len(result.Models) != 1 {
		t.Fatalf("want 1 model collected before bail, got %d", len(result.Models))
	}
	if result.Models[0].Model != 42 || result.Models[0].Shape != 5 {
		t.Errorf("model mismatch: want {42,5} got %+v", result.Models[0])
	}
}

// TestUnpackLoc_Opcode2_Name checks name= emission.
func TestUnpackLoc_Opcode2_Name(t *testing.T) {
	body := append([]byte{2}, []byte("Oak tree\x0a")...)
	body = append(body, 0)
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "oak"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[oak]", "name=Oak tree"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode14_15_Width_Length checks width/length.
func TestUnpackLoc_Opcode14_15_Width_Length(t *testing.T) {
	body := []byte{14, 3, 15, 5, 0} // width=3, length=5
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "door"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[door]", "width=3", "length=5"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode17_18_Block checks blockwalk=no and blockrange=no.
func TestUnpackLoc_Opcode17_18_Block(t *testing.T) {
	body := []byte{17, 18, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "decor"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[decor]", "blockwalk=no", "blockrange=no"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode19_Active checks active=yes and active=no.
func TestUnpackLoc_Opcode19_Active(t *testing.T) {
	// active=true
	body := []byte{19, 1, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "door"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "active=yes" {
		t.Errorf("want active=yes got %q", got[1])
	}

	// active=false (byte 0)
	body2 := []byte{19, 0, 0}
	cfg2 := buildLocCfgIdx(body2)
	got2, err := unpackLoc(cfg2, 0, makePackFile(0, "door"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got2[1] != "active=no" {
		t.Errorf("want active=no got %q", got2[1])
	}

	// active=false (byte 2): GBool uses ===1 semantics; value 2 must be treated as false.
	// This pins the fix vs the former g1 != 0 implementation (TS LocConfig.ts:213 gbool).
	body3 := []byte{19, 2, 0}
	cfg3 := buildLocCfgIdx(body3)
	got3, err := unpackLoc(cfg3, 0, makePackFile(0, "door"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got3[1] != "active=no" {
		t.Errorf("byte-value-2: want active=no got %q", got3[1])
	}
}

// TestUnpackLoc_Opcode69_ForceApproach checks all four direction flag combos.
// TS LocConfig.ts:267-279: each direction = bit CLEAR.
func TestUnpackLoc_Opcode69_ForceApproach(t *testing.T) {
	cases := []struct {
		flags byte
		want  string
	}{
		{0b1110, "north"}, // bit 0 clear
		{0b1101, "east"},  // bit 1 clear
		{0b1011, "south"}, // bit 2 clear
		{0b0111, "west"},  // bit 3 clear
		{0b1111, ""},      // all set → empty string
	}
	for _, tc := range cases {
		body := []byte{69, tc.flags, 0}
		cfg := buildLocCfgIdx(body)
		got, err := unpackLoc(cfg, 0, makePackFile(0, "door"), nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := "forceapproach=" + tc.want
		if got[1] != want {
			t.Errorf("flags=0b%04b: want %q got %q", tc.flags, want, got[1])
		}
	}
}

// TestUnpackLoc_Opcode70_72_Offsets checks signed i16 for offsetx/y/z.
func TestUnpackLoc_Opcode70_72_Offsets(t *testing.T) {
	// offsetx=-1, offsety=100, offsetz=0
	body := []byte{
		70, 0xFF, 0xFF, // offsetx = -1
		71, 0x00, 100,  // offsety = 100
		72, 0x00, 0x00, // offsetz = 0
		0,
	}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "wall"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[wall]", "offsetx=-1", "offsety=100", "offsetz=0"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode1_DuplicateNameSkip verifies consecutive duplicate names are skipped.
// TS LocConfig.ts:182-194: if lastName === name, skip (don't emit).
func TestUnpackLoc_Opcode1_DuplicateNameSkip(t *testing.T) {
	// Two models with same ID (same name after renameModelLoc) → only emit once.
	// Use a non-model_ name so renameModelLoc returns the name unchanged.
	modelPack := makeMultiPackFile(map[int]string{
		100: "mywall",
		101: "mywall", // same name after rename
	})

	body := []byte{1, 2, 0x00, 100, 0, 0x00, 101, 0, 0} // count=2, both shape=0
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "wall"), nil, nil, modelPack, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Only one model line should be emitted
	modelLines := 0
	for _, line := range got {
		if strings.HasPrefix(line, "model") {
			modelLines++
		}
	}
	if modelLines != 1 {
		t.Errorf("expected 1 model line (duplicate skip), got %d: %v", modelLines, got)
	}
}

// TestUnpackLoc_Opcode1_DifferentNames_BothEmitted verifies non-duplicate names both emit.
func TestUnpackLoc_Opcode1_DifferentNames_BothEmitted(t *testing.T) {
	modelPack := makeMultiPackFile(map[int]string{
		100: "wall_a",
		101: "wall_b",
	})

	// two models with different names
	body := []byte{1, 2, 0x00, 100, 0, 0x00, 101, 1, 0} // shape 0 and 1
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "wall"), nil, nil, modelPack, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should emit model= and model2=
	want := []string{"[wall]", "model=wall_a", "model2=wall_b"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode24_Anim_Fallback checks anim=seq_N fallback.
func TestUnpackLoc_Opcode24_Anim_Fallback(t *testing.T) {
	body := []byte{24, 0x00, 0x0A, 0} // seqId=10
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[loc]", "anim=seq_10"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode25_HasAlpha checks hasalpha=yes.
func TestUnpackLoc_Opcode25_HasAlpha(t *testing.T) {
	body := []byte{25, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[loc]", "hasalpha=yes"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode62_Mirror checks mirror=yes.
func TestUnpackLoc_Opcode62_Mirror(t *testing.T) {
	body := []byte{62, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[loc]", "mirror=yes"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode64_Shadow checks shadow=no.
func TestUnpackLoc_Opcode64_Shadow(t *testing.T) {
	body := []byte{64, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[loc]", "shadow=no"}
	assertLines(t, want, got)
}

// TestUnpackLoc_Opcode73_ForceDecor checks forcedecor=yes.
func TestUnpackLoc_Opcode73_ForceDecor(t *testing.T) {
	body := []byte{73, 0}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[loc]", "forcedecor=yes"}
	assertLines(t, want, got)
}

// TestUnpackLoc_UnknownOpcode_ReturnsError verifies unknown opcode triggers error return.
// TS: printFatalError → Go: return error.
func TestUnpackLoc_UnknownOpcode_ReturnsError(t *testing.T) {
	body := []byte{200, 0}
	cfg := buildLocCfgIdx(body)
	_, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown opcode, got nil")
	}
	if !strings.Contains(err.Error(), "unknown loc code 200") {
		t.Errorf("error message mismatch: %v", err)
	}
}

// TestUnpackLoc_Recol_Threshold100 checks recol threshold = 100 (not 50).
func TestUnpackLoc_Recol_Threshold100(t *testing.T) {
	// src=99 < 100 → check texture or recol path (NOT threshold path)
	// src=101 >= 100 → threshold path (recol)
	body := []byte{
		40, 1,
		0x00, 101, // recolSrc[0]=101 (>=100)
		0x00, 50,  // recolDst[0]=50
		0,
	}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundS, foundD := false, false
	for _, line := range got {
		if len(line) >= 8 && line[:8] == "recol1s=" {
			foundS = true
		}
		if len(line) >= 8 && line[:8] == "recol1d=" {
			foundD = true
		}
	}
	if !foundS || !foundD {
		t.Errorf("expected recol1s= and recol1d=, got: %v", got)
	}
}

// TestUnpackLoc_Recol_LocMode_RetexWhenReverseHslEmpty verifies retex when reverseHsl is empty.
// TS LocConfig.ts:318: typeof srcRgb === 'undefined' || typeof dstRgb === 'undefined'
// This is unique to loc vs idk/npc/obj which only check modelsHaveTexture.
func TestUnpackLoc_Recol_LocMode_RetexWhenReverseHslEmpty(t *testing.T) {
	// Find an HSL value that maps to empty reverseHsl
	var emptyHSL int = -1
	for hsl := 0; hsl < 100; hsl++ {
		if len(colorconvReverseHsl(hsl)) == 0 {
			emptyHSL = hsl
			break
		}
	}
	if emptyHSL == -1 {
		t.Skip("no HSL value with empty reverseHsl found in 0..99")
	}

	texPack := makePackFile(emptyHSL, "badtex")
	body := []byte{
		40, 1,
		byte(emptyHSL >> 8), byte(emptyHSL), // recolSrc[0] = emptyHSL (< 100)
		0x00, 3, // recolDst[0] = 3
		0,
	}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), texPack, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should emit retex1s= (because srcRgb is undefined/empty)
	found := false
	for _, line := range got {
		if len(line) >= 7 && line[:7] == "retex1s" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected retex1s= (locMode empty-hsl path), got: %v", got)
	}
}

// TestUnpackLoc_Recol_Dense_NotSparse verifies loc uses dense (not sparse) recol.
// All N recol pairs from opcode 40 are emitted without gaps.
func TestUnpackLoc_Recol_Dense_NotSparse(t *testing.T) {
	body := []byte{
		40, 2,     // count=2
		0x00, 110, // recolSrc[0]=110
		0x00, 50,  // recolDst[0]=50
		0x00, 120, // recolSrc[1]=120
		0x00, 60,  // recolDst[1]=60
		0,
	}
	cfg := buildLocCfgIdx(body)
	got, err := unpackLoc(cfg, 0, makePackFile(0, "loc"), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundS1, foundD1, foundS2, foundD2 := false, false, false, false
	for _, line := range got {
		switch {
		case len(line) >= 8 && line[:8] == "recol1s=":
			foundS1 = true
		case len(line) >= 8 && line[:8] == "recol1d=":
			foundD1 = true
		case len(line) >= 8 && line[:8] == "recol2s=":
			foundS2 = true
		case len(line) >= 8 && line[:8] == "recol2d=":
			foundD2 = true
		}
	}
	if !foundS1 || !foundD1 || !foundS2 || !foundD2 {
		t.Errorf("expected all 4 recol lines, got: %v", got)
	}
}

// TestRenameModelLoc_StripLdSuffix verifies _ld suffix is stripped.
func TestRenameModelLoc_StripLdSuffix(t *testing.T) {
	modelPack := makePackFile(5, "mywall_ld")
	result := renameModelLoc(5, 0, modelPack)
	// _ld stripped, then shape suffix "_1" checked (mywall doesn't end with _1)
	if result != "mywall" {
		t.Errorf("want mywall got %q", result)
	}
}

// TestRenameModelLoc_StripShapeSuffix verifies shape suffix is stripped.
func TestRenameModelLoc_StripShapeSuffix(t *testing.T) {
	// shape=0 → suffix="_1"; model "mywall_1" → "mywall"
	modelPack := makePackFile(5, "mywall_1")
	result := renameModelLoc(5, 0, modelPack)
	if result != "mywall" {
		t.Errorf("want mywall got %q", result)
	}
}

// TestRenameModelLoc_NoStrip verifies model with no matching suffix is returned as-is.
func TestRenameModelLoc_NoStrip(t *testing.T) {
	modelPack := makePackFile(5, "mywall")
	result := renameModelLoc(5, 0, modelPack)
	if result != "mywall" {
		t.Errorf("want mywall got %q", result)
	}
}
