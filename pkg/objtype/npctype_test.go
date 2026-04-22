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
