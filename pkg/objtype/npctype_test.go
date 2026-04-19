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
