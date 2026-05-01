package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestParseSeqFrames_EmptyBuffer(t *testing.T) {
	configs := parseSeqFrames(packet.NewPacket(nil))
	if configs == nil {
		t.Fatal("parseSeqFrames: got nil")
	}
	if len(configs.Instances) != 0 {
		t.Errorf("Instances len: got %d, want 0", len(configs.Instances))
	}
}

func TestParseSeqFrames_DelaysSequential(t *testing.T) {
	configs := parseSeqFrames(packet.NewPacket([]byte{1, 2, 3, 4}))
	if len(configs.Instances) != 4 {
		t.Fatalf("Instances len: got %d, want 4", len(configs.Instances))
	}
	for i, want := range []int{1, 2, 3, 4} {
		if configs.Instances[i].Delay != want {
			t.Errorf("Instances[%d].Delay: got %d, want %d", i, configs.Instances[i].Delay, want)
		}
	}
}

func TestLoadSeqFrames_MissingFile(t *testing.T) {
	dir := t.TempDir()
	configs, err := LoadSeqFrames(dir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Instances) != 0 {
		t.Errorf("Instances: want empty, got %d entries", len(configs.Instances))
	}
}

func TestLoadSeqFrames_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "frame_del.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	configs, err := LoadSeqFrames(cacheDir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: %v", err)
	}
	if len(configs.Instances) == 0 {
		t.Fatal("expected at least one SeqFrame, got 0")
	}
}
