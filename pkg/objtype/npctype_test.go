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
				t.Errorf("%s: got %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}
