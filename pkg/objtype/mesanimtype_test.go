package objtype

import (
	"os"
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewMesanimType_LenInitMinusOne(t *testing.T) {
	m := NewMesanimType(3)
	for i, v := range m.Len {
		if v != -1 {
			t.Errorf("Len[%d]: got %d, want -1", i, v)
		}
	}
	if m.ID != 3 {
		t.Errorf("ID: got %d, want 3", m.ID)
	}
}

// flipReader builds a writer Packet via build, then returns a reader
// Packet over the bytes written. Mirrors decodeIdk's pattern in
// idktype_test.go but exposes the raw reader for direct Decode calls.
func flipReader(build func(*packet2.Packet)) *packet2.Packet {
	w := packet2.NewPacket(nil)
	build(w)
	return packet2.NewPacket(w.Bytes())
}

func TestMesanimDecode_Code1WritesLen0(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.P2(42) })
	if err := m.Decode(1, r); err != nil {
		t.Fatalf("Decode(1): %v", err)
	}
	if m.Len[0] != 42 {
		t.Errorf("Len[0]: got %d, want 42", m.Len[0])
	}
}

func TestMesanimDecode_Code4WritesLen3(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.P2(7) })
	if err := m.Decode(4, r); err != nil {
		t.Fatalf("Decode(4): %v", err)
	}
	if m.Len[3] != 7 {
		t.Errorf("Len[3]: got %d, want 7", m.Len[3])
	}
}

func TestMesanimDecode_Code250WritesDebugName(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.PJStrLF("neutral") })
	if err := m.Decode(250, r); err != nil {
		t.Fatalf("Decode(250): %v", err)
	}
	if m.DebugName != "neutral" {
		t.Errorf("DebugName: got %q, want %q", m.DebugName, "neutral")
	}
}

func TestMesanimDecode_UnknownCodeErrors(t *testing.T) {
	m := NewMesanimType(0)
	r := packet2.NewPacket(nil)
	err := m.Decode(5, r)
	if err == nil {
		t.Fatalf("Decode(5): expected error, got nil")
	}
}

func TestLoadMesanimTypes_MissingFileEmptyRegistry(t *testing.T) {
	tmp := t.TempDir()
	cfgs, err := LoadMesanimTypes(tmp)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	if len(cfgs.Configs) != 0 {
		t.Errorf("Configs len: got %d, want 0", len(cfgs.Configs))
	}
	if len(cfgs.ConfigNames) != 0 {
		t.Errorf("ConfigNames len: got %d, want 0", len(cfgs.ConfigNames))
	}
}

func TestLoadMesanimTypes_RealCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "mesanim.dat")); err != nil {
		t.Skipf("data/pack/server/mesanim.dat unavailable: %v", err)
	}
	cfgs, err := LoadMesanimTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	if len(cfgs.Configs) == 0 {
		t.Fatalf("real cache produced zero configs")
	}
	// At least one config should have a non-empty DebugName.
	gotName := false
	for _, c := range cfgs.Configs {
		if c != nil && c.DebugName != "" {
			gotName = true
			break
		}
	}
	if !gotName {
		t.Errorf("no config has a non-empty DebugName")
	}
	// ConfigNames map should mirror the named configs.
	for name, id := range cfgs.ConfigNames {
		if id < 0 || id >= len(cfgs.Configs) {
			t.Errorf("ConfigNames[%q] = %d: out of range", name, id)
			continue
		}
		if cfgs.Configs[id] == nil || cfgs.Configs[id].DebugName != name {
			t.Errorf("ConfigNames[%q] = %d: roundtrip mismatch", name, id)
		}
	}
}
