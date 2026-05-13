package objtype

import (
	"path/filepath"
	"testing"
)

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
