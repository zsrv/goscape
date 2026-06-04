package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// TestNpcTypeDecodeRegenRate214 pins the opcode-214 (regenrate) decoder case,
// mirroring TS NpcType.ts:223-224 (`this.regenrate = dat.g2()`). The packer
// emits P1(214)+P2(value) for any `regenrate=` line (pkg/pack/npc.go:525), so a
// missing case here aborts the whole NPC config load with "unrecognized npc
// config code 214" — a latent CRITICAL until any Content .npc sets regenrate.
func TestNpcTypeDecodeRegenRate214(t *testing.T) {
	typ := NewNpcType(0)
	if typ.RegenRate != 100 {
		t.Fatalf("default RegenRate: got %d, want 100", typ.RegenRate)
	}
	if err := typ.Decode(214, packet2.NewPacket([]byte{0x12, 0x34})); err != nil {
		t.Fatalf("Decode(214): %v", err)
	}
	if typ.RegenRate != 0x1234 {
		t.Errorf("RegenRate after opcode 214: got %d, want %d", typ.RegenRate, 0x1234)
	}
}

// TestNpcTypeDecodeOpCodes30To39NoPanic pins L31: NpcType funnels all of
// codes 30-39 into one `op` array (TS NpcType.ts:141-146, `code >= 30 && < 40`,
// a 5-slot array JS grows on demand). The packer only emits op1-5 → codes
// 30-34, but a foreign cache with codes 35-39 must grow the slice rather than
// panic on the fixed 5-slot index. "hidden" is stored verbatim (no coercion).
func TestNpcTypeDecodeOpCodes30To39NoPanic(t *testing.T) {
	typ := NewNpcType(0)
	// code 30 → op[0]; code 31 → op[1] "hidden"; code 39 → op[9].
	if err := typ.Decode(30, packet2.NewPacket([]byte("Talk-to\n"))); err != nil {
		t.Fatalf("Decode(30): %v", err)
	}
	if err := typ.Decode(31, packet2.NewPacket([]byte("hidden\n"))); err != nil {
		t.Fatalf("Decode(31): %v", err)
	}
	if err := typ.Decode(39, packet2.NewPacket([]byte("Examine\n"))); err != nil {
		t.Fatalf("Decode(39): %v (op slice should grow, not panic)", err)
	}
	if got := typ.Op[0]; got != "Talk-to" {
		t.Errorf("Op[0]: got %q, want \"Talk-to\"", got)
	}
	if got := typ.Op[1]; got != "hidden" {
		t.Errorf("Op[1] (verbatim, not coerced): got %q, want \"hidden\"", got)
	}
	if len(typ.Op) < 10 {
		t.Fatalf("len(Op): got %d, want >= 10 (grown for code 39)", len(typ.Op))
	}
	if got := typ.Op[9]; got != "Examine" {
		t.Errorf("Op[9]: got %q, want \"Examine\"", got)
	}
}

func TestLoadNPCTypesFromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	configs, err := LoadNPCTypes(cacheDir)
	if err != nil {
		t.Skipf("no cache data: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one NpcType, got 0")
	}
}

func TestNPCModeQueueConstants(t *testing.T) {
	// QUEUE1..QUEUE20 mirror TS NpcMode.ts:76-95 (values 47..66).
	// Used by consumeHuntTarget (NAI-10) to detect the direct-dispatch
	// branch via arithmetic: trigger = TriggerAiQueue1 + (mode - QUEUE1).
	if NPCModeQueue1 != 47 {
		t.Errorf("NPCModeQueue1: got %d, want 47", NPCModeQueue1)
	}
	if NPCModeQueue20 != 66 {
		t.Errorf("NPCModeQueue20: got %d, want 66", NPCModeQueue20)
	}
	if NPCModeQueue20-NPCModeQueue1 != 19 {
		t.Errorf("range: got %d, want 19 (20 consecutive values)",
			NPCModeQueue20-NPCModeQueue1)
	}
}

func TestNPCModeFullEnum(t *testing.T) {
	// Mirrors Engine-TS/src/engine/entity/NpcMode.ts:1-96.
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"NPCModeNull", NPCModeNull, -1},
		{"NPCModeNone", NPCModeNone, 0},
		{"NPCModeWander", NPCModeWander, 1},
		{"NPCModePatrol", NPCModePatrol, 2},
		{"NPCModePlayerEscape", NPCModePlayerEscape, 3},
		{"NPCModePlayerFollow", NPCModePlayerFollow, 4},
		{"NPCModePlayerFace", NPCModePlayerFace, 5},
		{"NPCModePlayerFaceClose", NPCModePlayerFaceClose, 6},
		{"NPCModeOpPlayer1", NPCModeOpPlayer1, 7},
		{"NPCModeOpPlayer5", NPCModeOpPlayer5, 11},
		{"NPCModeApPlayer1", NPCModeApPlayer1, 12},
		{"NPCModeApPlayer5", NPCModeApPlayer5, 16},
		{"NPCModeOpLoc1", NPCModeOpLoc1, 17},
		{"NPCModeOpLoc5", NPCModeOpLoc5, 21},
		{"NPCModeApLoc1", NPCModeApLoc1, 22},
		{"NPCModeApLoc5", NPCModeApLoc5, 26},
		{"NPCModeOpObj1", NPCModeOpObj1, 27},
		{"NPCModeOpObj5", NPCModeOpObj5, 31},
		{"NPCModeApObj1", NPCModeApObj1, 32},
		{"NPCModeApObj5", NPCModeApObj5, 36},
		{"NPCModeOpNpc1", NPCModeOpNpc1, 37},
		{"NPCModeOpNpc5", NPCModeOpNpc5, 41},
		{"NPCModeApNpc1", NPCModeApNpc1, 42},
		{"NPCModeApNpc5", NPCModeApNpc5, 46},
		{"NPCModeQueue1", NPCModeQueue1, 47},
		{"NPCModeQueue20", NPCModeQueue20, 66},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				// t.Run already prefixes failures with tc.name.
				t.Errorf("got %d, want %d", tc.got, tc.want)
			}
		})
	}
}

func TestNPCTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "first"}}, {ConfigType: ConfigType{ID: 1, DebugName: "second"}}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := nc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestNPCTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := nc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestNPCTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var nc *NPCTypeConfigs
	if got := nc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestNPCTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "other"}}, {ConfigType: ConfigType{ID: 1, DebugName: "fresh"}}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := nc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestNPCTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := nc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}

// TestNpcTypeDecodeCode99AlwaysOnTop pins the code-99 decoder (TS NpcType.ts:193-194).
// Code 99 has no operand — it sets alwaysontop=true by presence alone.
// The default must be false; after decode it must be true; buffer must advance
// zero bytes (no operand consumed).
func TestNpcTypeDecodeCode99AlwaysOnTop(t *testing.T) {
	typ := NewNpcType(0)
	if typ.AlwaysOnTop != false {
		t.Fatalf("default AlwaysOnTop: got true, want false")
	}
	// No operand bytes — empty packet is correct.
	pkt := packet2.NewPacket(nil)
	if err := typ.Decode(99, pkt); err != nil {
		t.Fatalf("Decode(99): %v", err)
	}
	if !typ.AlwaysOnTop {
		t.Errorf("AlwaysOnTop after code 99: got false, want true")
	}
	// Must not have consumed any bytes.
	if pkt.Pos != 0 {
		t.Errorf("Decode(99) consumed %d bytes, want 0 (no operand)", pkt.Pos)
	}
}

// TestNpcTypeDecodeCode100Ambient pins the code-100 decoder (TS NpcType.ts:195-196).
// ambient = g1b() — SIGNED byte. Default 0; feed 0x05 → 5; feed 0xFF → -1.
func TestNpcTypeDecodeCode100AmbientPositive(t *testing.T) {
	typ := NewNpcType(0)
	if typ.Ambient != 0 {
		t.Fatalf("default Ambient: got %d, want 0", typ.Ambient)
	}
	if err := typ.Decode(100, packet2.NewPacket([]byte{0x05})); err != nil {
		t.Fatalf("Decode(100, 0x05): %v", err)
	}
	if typ.Ambient != 5 {
		t.Errorf("Ambient after 0x05: got %d, want 5", typ.Ambient)
	}
}

func TestNpcTypeDecodeCode100AmbientSigned(t *testing.T) {
	typ := NewNpcType(0)
	if err := typ.Decode(100, packet2.NewPacket([]byte{0xFF})); err != nil {
		t.Fatalf("Decode(100, 0xFF): %v", err)
	}
	if typ.Ambient != -1 {
		t.Errorf("Ambient after 0xFF: got %d, want -1 (signed byte)", typ.Ambient)
	}
}

// TestNpcTypeDecodeCode101Contrast pins the code-101 decoder (TS NpcType.ts:197-198).
// contrast = g1b() — SIGNED byte. Default 0; feed 0x0A → 10; feed 0xFF → -1.
func TestNpcTypeDecodeCode101ContrastPositive(t *testing.T) {
	typ := NewNpcType(0)
	if typ.Contrast != 0 {
		t.Fatalf("default Contrast: got %d, want 0", typ.Contrast)
	}
	if err := typ.Decode(101, packet2.NewPacket([]byte{0x0A})); err != nil {
		t.Fatalf("Decode(101, 0x0A): %v", err)
	}
	if typ.Contrast != 10 {
		t.Errorf("Contrast after 0x0A: got %d, want 10", typ.Contrast)
	}
}

func TestNpcTypeDecodeCode101ContrastSigned(t *testing.T) {
	typ := NewNpcType(0)
	if err := typ.Decode(101, packet2.NewPacket([]byte{0xFF})); err != nil {
		t.Fatalf("Decode(101, 0xFF): %v", err)
	}
	if typ.Contrast != -1 {
		t.Errorf("Contrast after 0xFF: got %d, want -1 (signed byte)", typ.Contrast)
	}
}

// TestNpcTypeDecodeCode102HeadIcon pins the code-102 decoder (TS NpcType.ts:199-200).
// headicon = g2() — unsigned 2-byte. Default -1; feed 0x00 0x07 → 7.
func TestNpcTypeDecodeCode102HeadIcon(t *testing.T) {
	typ := NewNpcType(0)
	if typ.HeadIcon != -1 {
		t.Fatalf("default HeadIcon: got %d, want -1", typ.HeadIcon)
	}
	if err := typ.Decode(102, packet2.NewPacket([]byte{0x00, 0x07})); err != nil {
		t.Fatalf("Decode(102, 0x00 0x07): %v", err)
	}
	if typ.HeadIcon != 7 {
		t.Errorf("HeadIcon after 0x00 0x07: got %d, want 7", typ.HeadIcon)
	}
}
