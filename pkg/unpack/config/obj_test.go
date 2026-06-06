package config

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildObjCfgIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildObjCfgIdx(body []byte) *ConfigIdx {
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

// TestUnpackObj_Opcode2_Name checks name= emission.
func TestUnpackObj_Opcode2_Name(t *testing.T) {
	body := append([]byte{2}, []byte("Bronze sword\x0a")...)
	body = append(body, 0)
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword_bronze"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword_bronze]", "name=Bronze sword"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode4_2dZoom checks 2dzoom= emission.
func TestUnpackObj_Opcode4_2dZoom(t *testing.T) {
	body := []byte{4, 0x04, 0xD2, 0} // zoom2d=1234
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "2dzoom=1234"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode7_2dxof_Signed checks signed g2s for 2dxof.
func TestUnpackObj_Opcode7_2dxof_Signed(t *testing.T) {
	// g2s of 0xFFFF = -1 (signed i16)
	body := []byte{7, 0xFF, 0xFF, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "2dxof=-1"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode9_Code9 checks code9=yes.
func TestUnpackObj_Opcode9_Code9(t *testing.T) {
	body := []byte{9, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "code9=yes"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode10_Code10_Fallback checks seq_N fallback.
func TestUnpackObj_Opcode10_Code10_Fallback(t *testing.T) {
	body := []byte{10, 0x00, 0x07, 0} // seqId=7
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "code10=seq_7"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode11_Stackable checks stackable=yes.
func TestUnpackObj_Opcode11_Stackable(t *testing.T) {
	body := []byte{11, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "coins"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[coins]", "stackable=yes"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode12_Cost_Negative checks signed g4s for cost.
func TestUnpackObj_Opcode12_Cost_Negative(t *testing.T) {
	// g4s of 0xFFFFFFFF = -1
	body := []byte{12, 0xFF, 0xFF, 0xFF, 0xFF, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "cost=-1"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode16_Members checks members=yes.
func TestUnpackObj_Opcode16_Members(t *testing.T) {
	body := []byte{16, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "members=yes"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode23_ManWear_WithOffset checks manwear=model,offset.
func TestUnpackObj_Opcode23_ManWear_WithOffset(t *testing.T) {
	srcDir := setupModelTree(t, "model_50")
	modelPack := makePackFile(50, "model_50")

	// opcode 23: g2=50 (modelId), g1b=-2 (signed offset)
	body := []byte{23, 0x00, 0x32, 0xFE, 0} // offset = -2 (signed byte 0xFE)
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword_bronze"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[sword_bronze]", "manwear=obj_sword_bronze_manwear,-2"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode25_WomanWear_WithOffset checks womanwear=model,offset.
func TestUnpackObj_Opcode25_WomanWear_WithOffset(t *testing.T) {
	srcDir := setupModelTree(t, "model_51")
	modelPack := makePackFile(51, "model_51")

	body := []byte{25, 0x00, 0x33, 0x05, 0} // offset=5
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "robe"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[robe]", "womanwear=obj_robe_womanwear,5"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode30_35_Op checks op1..op5 emission.
func TestUnpackObj_Opcode30_35_Op(t *testing.T) {
	body := append([]byte{30}, []byte("Wield\x0a")...)
	body = append(body, 0)
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "op1=Wield"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode35_40_Iop checks iop1..iop5 emission.
func TestUnpackObj_Opcode35_40_Iop(t *testing.T) {
	body := append([]byte{35}, []byte("Wear\x0a")...)
	body = append(body, 0)
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "iop1=Wear"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode97_CertLink_Fallback checks certlink=obj_N fallback.
func TestUnpackObj_Opcode97_CertLink_Fallback(t *testing.T) {
	body := []byte{97, 0x00, 0x64, 0} // objId=100
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "certlink=obj_100"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode97_CertLink_Pack checks certlink with pack lookup.
func TestUnpackObj_Opcode97_CertLink_Pack(t *testing.T) {
	body := []byte{97, 0x00, 0x03, 0} // objId=3
	cfg := buildObjCfgIdx(body)
	objPack := makeMultiPackFile(map[int]string{0: "sword_bronze", 3: "cert_sword"})
	got := unpackObj(cfg, 0, objPack, nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword_bronze]", "certlink=cert_sword"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode100_109_CountN checks countN=objName,count.
func TestUnpackObj_Opcode100_109_CountN(t *testing.T) {
	body := []byte{100, 0x00, 0x05, 0x00, 0x0A, 0} // code=100→count1; objId=5; count=10
	cfg := buildObjCfgIdx(body)
	objPack := makeMultiPackFile(map[int]string{0: "rune_platebody", 5: "rune_legs"})
	got := unpackObj(cfg, 0, objPack, nil, nil, nil, nil, "", nil, nil)
	want := []string{"[rune_platebody]", "count1=rune_legs,10"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode100_109_CountN_Fallback checks countN=obj_N,count fallback.
func TestUnpackObj_Opcode100_109_CountN_Fallback(t *testing.T) {
	body := []byte{101, 0x00, 0x09, 0x00, 0x05, 0} // code=101→count2; objId=9; count=5
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "count2=obj_9,5"}
	assertLines(t, want, got)
}

// TestUnpackObj_Opcode110_113_Resize checks resizex/y/z.
func TestUnpackObj_Opcode110_113_Resize(t *testing.T) {
	body := []byte{110, 0x00, 200, 111, 0x01, 0x00, 112, 0x00, 128, 0}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[sword]", "resizex=200", "resizey=256", "resizez=128"}
	assertLines(t, want, got)
}

// TestUnpackObj_RenameModel_CollisionSuffix_i2 tests obj collision format: namei2.
func TestUnpackObj_RenameModel_CollisionSuffix_i2(t *testing.T) {
	srcDir := setupModelTree(t, "model_2")
	modelPack := makeMultiPackFile(map[int]string{
		2: "model_2",
		3: "obj_sword", // collision
	})

	body := []byte{1, 0x00, 0x02, 0} // modelId=2
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[sword]", "model=obj_swordi2"}
	assertLines(t, want, got)
}

// TestUnpackObj_Recol_Dense checks dense recol with threshold >= 100.
func TestUnpackObj_Recol_Dense(t *testing.T) {
	body := []byte{
		40, 1, // count=1
		0x00, 120, // recolSrc[0]=120 (>=100)
		0x00, 80, // recolDst[0]=80
		0,
	}
	cfg := buildObjCfgIdx(body)
	got := unpackObj(cfg, 0, makePackFile(0, "sword"), nil, nil, nil, nil, "", nil, nil)
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

// TestUnpackObj_UnknownOpcode checks warning emission.
func TestUnpackObj_UnknownOpcode(t *testing.T) {
	body := []byte{55, 0}
	cfg := buildObjCfgIdx(body)
	var warns []string
	unpackObj(cfg, 0, nil, nil, nil, nil, nil, "", captureWarnings(&warns), nil)
	if len(warns) == 0 || warns[0] != "unknown obj code 55" {
		t.Errorf("want [\"unknown obj code 55\"], got %v", warns)
	}
}
