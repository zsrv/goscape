package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildNpcCfgIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildNpcCfgIdx(body []byte) *ConfigIdx {
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

// TestUnpackNpc_Opcode2_Name checks name= emission.
func TestUnpackNpc_Opcode2_Name(t *testing.T) {
	body := append([]byte{2}, []byte("Hans\x0a")...)
	body = append(body, 0)
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "hans"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[hans]", "name=Hans"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode3_Desc checks desc= emission.
func TestUnpackNpc_Opcode3_Desc(t *testing.T) {
	body := append([]byte{3}, []byte("A goblin.\x0a")...)
	body = append(body, 0)
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "goblin"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[goblin]", "desc=A goblin."}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode12_Size checks size= with signed byte.
func TestUnpackNpc_Opcode12_Size(t *testing.T) {
	body := []byte{12, 0xFF, 0} // -1 as signed byte
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "size=-1"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode13_ReadyAnimFallback verifies the odd 'seq_ N' spacing (with space).
// TS NpcConfig.ts:83 uses 'seq_ ' + readyanimId (note the space before the number).
func TestUnpackNpc_Opcode13_ReadyAnimFallback_OddSpace(t *testing.T) {
	body := []byte{13, 0x00, 42, 0} // readyanimId=42, not in SeqPack
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "readyanim=seq_ 42"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode14_WalkAnimFallback verifies the odd 'seq_ N' spacing (with space).
// TS NpcConfig.ts:88 uses 'seq_ ' + walkanimId (note the space before the number).
func TestUnpackNpc_Opcode14_WalkAnimFallback_OddSpace(t *testing.T) {
	body := []byte{14, 0x00, 99, 0} // walkanimId=99, not in SeqPack
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "walkanim=seq_ 99"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode17_WalkAnimCommaJoined tests 4-id comma-joined walkanim.
// Fallback for opcode 17 uses 'seq_' + id (NO space — NpcConfig.ts:98).
func TestUnpackNpc_Opcode17_WalkAnim_FallbackNoSpace(t *testing.T) {
	body := []byte{
		17,
		0x00, 10, // walkanimId=10 (not in pack)
		0x00, 11, // walkanim_b=11
		0x00, 12, // walkanim_l=12
		0x00, 13, // walkanim_r=13
		0,
	}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "walkanim=seq_10,seq_11,seq_12,seq_13"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode17_WalkAnim_RegistryHit tests 4-id comma-joined with pack lookup.
func TestUnpackNpc_Opcode17_WalkAnim_RegistryHit(t *testing.T) {
	seqPack := makeMultiPackFile(map[int]string{
		10: "walk_f", 11: "walk_b", 12: "walk_l", 13: "walk_r",
	})
	body := []byte{
		17,
		0x00, 10, 0x00, 11, 0x00, 12, 0x00, 13,
		0,
	}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, seqPack, nil, nil, "", nil, nil)
	want := []string{"[npc]", "walkanim=walk_f,walk_b,walk_l,walk_r"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode16_HasAlpha checks hasalpha=yes.
func TestUnpackNpc_Opcode16_HasAlpha(t *testing.T) {
	body := []byte{16, 0}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "hasalpha=yes"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode95_Vislevel0_Hide checks vislevel=hide when value is 0.
func TestUnpackNpc_Opcode95_Vislevel0_Hide(t *testing.T) {
	body := []byte{95, 0x00, 0x00, 0} // vislevel=0
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "vislevel=hide"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode95_Vislevel_Nonzero checks vislevel=N when value != 0.
func TestUnpackNpc_Opcode95_Vislevel_Nonzero(t *testing.T) {
	body := []byte{95, 0x00, 0x05, 0} // vislevel=5
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "vislevel=5"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode1_ModelRename_CollisionSuffix_i2 tests npc collision format: namei2.
// NpcConfig uses 'npc_' + name + 'i' + i (NO underscore before i).
func TestUnpackNpc_RenameModel_CollisionSuffix_i2(t *testing.T) {
	srcDir := setupModelTree(t, "model_1")
	modelPack := makeMultiPackFile(map[int]string{
		1: "model_1",
		2: "npc_goblin", // collision
	})

	body := []byte{1, 1, 0x00, 0x01, 0} // count=1, modelId=1
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "goblin"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[goblin]", "model1=npc_goblini2"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode60_HeadModels checks head model emission.
func TestUnpackNpc_Opcode60_HeadModels(t *testing.T) {
	srcDir := setupModelTree(t, "model_20")
	modelPack := makePackFile(20, "model_20")

	body := []byte{60, 1, 0x00, 0x14, 0} // count=1, modelId=20
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "hans"), nil, nil, modelPack, nil, srcDir, nil, nil)
	want := []string{"[hans]", "head1=npc_hans_head"}
	assertLines(t, want, got)

	dest := filepath.Join(srcDir, "models", "npc", "npc_hans_head.ob2")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}
}

// TestUnpackNpc_Opcode93_Minimap checks minimap=no.
func TestUnpackNpc_Opcode93_Minimap(t *testing.T) {
	body := []byte{93, 0}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "minimap=no"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode99_AlwaysOnTop checks alwaysontop=yes.
func TestUnpackNpc_Opcode99_AlwaysOnTop(t *testing.T) {
	body := []byte{99, 0}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "alwaysontop=yes"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Opcode102_Headicon checks headicon=N.
func TestUnpackNpc_Opcode102_Headicon(t *testing.T) {
	body := []byte{102, 0x00, 0x07, 0} // headicon=7
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
	want := []string{"[npc]", "headicon=7"}
	assertLines(t, want, got)
}

// TestUnpackNpc_Recol_DenseThreshold100 checks that dense recol uses >= 100 threshold.
func TestUnpackNpc_Recol_DenseThreshold100(t *testing.T) {
	// src=150 >= 100 → recol path
	body := []byte{
		40, 1, // count=1
		0x00, 150, // recolSrc[0]=150
		0x00, 50, // recolDst[0]=50
		0,
	}
	cfg := buildNpcCfgIdx(body)
	got := unpackNpc(cfg, 0, makePackFile(0, "npc"), nil, nil, nil, nil, "", nil, nil)
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

// TestUnpackNpc_UnknownOpcode checks warning emission.
func TestUnpackNpc_UnknownOpcode(t *testing.T) {
	body := []byte{77, 0}
	cfg := buildNpcCfgIdx(body)
	var warns []string
	unpackNpc(cfg, 0, nil, nil, nil, nil, nil, "", captureWarnings(&warns), nil)
	if len(warns) == 0 || warns[0] != "unknown npc code 77" {
		t.Errorf("want [\"unknown npc code 77\"], got %v", warns)
	}
}
