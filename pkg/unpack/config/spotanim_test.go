package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildSpotAnimCfgIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildSpotAnimCfgIdx(body []byte) *ConfigIdx {
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

// TestUnpackSpotAnim_Opcode1_ModelRename checks model rename to spot/ dir.
func TestUnpackSpotAnim_Opcode1_ModelRename(t *testing.T) {
	srcDir := setupModelTree(t, "model_7")
	modelPack := makePackFile(7, "model_7")

	body := []byte{1, 0x00, 0x07, 0} // modelId=7
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "myspot"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[myspot]", "model=spot_myspot"}
	assertLines(t, want, got)

	dest := filepath.Join(srcDir, "models", "spot", "spot_myspot.ob2")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}
}

// TestUnpackSpotAnim_Opcode2_Anim_Fallback checks anim=seq_N fallback.
func TestUnpackSpotAnim_Opcode2_Anim_Fallback(t *testing.T) {
	body := []byte{2, 0x00, 0x0F, 0} // seqId=15
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[spot]", "anim=seq_15"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Opcode2_Anim_Pack checks anim= with pack lookup.
func TestUnpackSpotAnim_Opcode2_Anim_Pack(t *testing.T) {
	seqPack := makePackFile(15, "myanim")
	body := []byte{2, 0x00, 0x0F, 0} // seqId=15
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, seqPack, nil, nil, "", nil, nil)
	want := []string{"[spot]", "anim=myanim"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Opcode3_HasAlpha checks hasalpha=yes.
func TestUnpackSpotAnim_Opcode3_HasAlpha(t *testing.T) {
	body := []byte{3, 0}
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[spot]", "hasalpha=yes"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Opcode4_5_ResizeH_V checks resizeh/resizev.
func TestUnpackSpotAnim_Opcode4_5_ResizeH_V(t *testing.T) {
	body := []byte{4, 0x00, 200, 5, 0x01, 0x00, 0}
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[spot]", "resizeh=200", "resizev=256"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Opcode6_Angle checks angle=N.
func TestUnpackSpotAnim_Opcode6_Angle(t *testing.T) {
	body := []byte{6, 0x00, 0x5A, 0} // angle=90
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[spot]", "angle=90"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Opcode7_8_AmbientContrast checks ambient/contrast signed bytes.
func TestUnpackSpotAnim_Opcode7_8_AmbientContrast(t *testing.T) {
	body := []byte{7, 0xFE, 8, 0x02, 0} // ambient=-2, contrast=2
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[spot]", "ambient=-2", "contrast=2"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_Recol_Threshold50 verifies spotanim uses threshold >= 50.
// TS comment: "texture ids cap at 50, so we can save time knowing it's not a texture id"
func TestUnpackSpotAnim_Recol_Threshold50(t *testing.T) {
	// srcRaw=60 >= 50 → recol path (NOT retex even with model texture)
	body := []byte{
		40, 0x00, 0x3C, // recolSrc[0]=60
		50, 0x00, 0x03, // recolDst[0]=3
		0,
	}
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
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

// TestUnpackSpotAnim_Recol_Threshold50_Below checks values below 50 use retex/recol logic.
func TestUnpackSpotAnim_Recol_Threshold50_Below(t *testing.T) {
	// srcRaw=49 < 50 → check texture or recol
	body := []byte{
		40, 0x00, 49, // recolSrc[0]=49
		50, 0x00, 3, // recolDst[0]=3
		0,
	}
	cfg := buildSpotAnimCfgIdx(body)
	// No model with texture → goes to recol path
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	// Should be recol1s or retex1s depending on model texture — no model → recol
	found := false
	for _, line := range got {
		if len(line) >= 7 && (line[:7] == "recol1s" || line[:7] == "retex1s") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recol1s= or retex1s= in output: %v", got)
	}
}

// TestUnpackSpotAnim_Recol_SparseSkip verifies sparse index skip in spotanim.
func TestUnpackSpotAnim_Recol_SparseSkip(t *testing.T) {
	// Only set index 1 (opcode 41/51), skip index 0.
	body := []byte{
		41, 0x00, 0x0A, // recolSrc[1]=10
		51, 0x00, 0x05, // recolDst[1]=5
		0,
	}
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "spot"), nil, nil, nil, nil, "", nil, nil)
	// index 0 never set → recol1s should NOT appear; recol2s SHOULD
	foundRecol1 := false
	foundRecol2 := false
	for _, line := range got {
		if len(line) >= 8 && line[:8] == "recol1s=" {
			foundRecol1 = true
		}
		if len(line) >= 8 && line[:8] == "recol2s=" {
			foundRecol2 = true
		}
	}
	if foundRecol1 {
		t.Errorf("recol1s= should not appear (index 0 was never set): %v", got)
	}
	if !foundRecol2 {
		t.Errorf("recol2s= should appear (index 1 was set): %v", got)
	}
}

// TestUnpackSpotAnim_RenameSpot_CollisionSuffix_Underscore tests collision uses _2 suffix.
// SpotAnimConfig collision format: spot_name_2 (with underscore before number).
func TestUnpackSpotAnim_RenameSpot_CollisionSuffix_Underscore(t *testing.T) {
	srcDir := setupModelTree(t, "model_9")
	modelPack := makeMultiPackFile(map[int]string{
		9:  "model_9",
		10: "spot_magic", // collision
	})

	body := []byte{1, 0x00, 0x09, 0} // modelId=9
	cfg := buildSpotAnimCfgIdx(body)
	got := unpackSpotAnim(cfg, 0, makePackFile(0, "magic"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[magic]", "model=spot_magic_2"}
	assertLines(t, want, got)
}

// TestUnpackSpotAnim_UnknownOpcode checks warning emission.
func TestUnpackSpotAnim_UnknownOpcode(t *testing.T) {
	body := []byte{99, 0}
	cfg := buildSpotAnimCfgIdx(body)
	var warns []string
	unpackSpotAnim(cfg, 0, nil, nil, nil, nil, nil, "", captureWarnings(&warns), nil)
	if len(warns) == 0 || warns[0] != "unknown spotanim code 99" {
		t.Errorf("want [\"unknown spotanim code 99\"], got %v", warns)
	}
}
