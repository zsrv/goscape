package objtype

import (
	"os"
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewIdkTypeDefaults(t *testing.T) {
	idk := NewIdkType(5)
	if idk.ID != 5 {
		t.Errorf("ID: got %d, want 5", idk.ID)
	}
	if idk.Type != -1 {
		t.Errorf("Type: got %d, want -1", idk.Type)
	}
	if idk.Disable {
		t.Error("Disable: got true, want false")
	}
	if idk.Models != nil {
		t.Errorf("Models: got %v, want nil", idk.Models)
	}
	want := [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}
	if idk.Heads != want {
		t.Errorf("Heads: got %v, want %v", idk.Heads, want)
	}
	for i, v := range idk.RecolS {
		if v != 0 {
			t.Errorf("RecolS[%d]: got %d, want 0", i, v)
		}
	}
	for i, v := range idk.RecolD {
		if v != 0 {
			t.Errorf("RecolD[%d]: got %d, want 0", i, v)
		}
	}
}

// decodeIdk builds a writer packet, appends a 0-terminator, flips to reader,
// and runs DecodeType on a fresh NewIdkType(0). Mirrors hunttype_test.go style.
func decodeIdk(build func(*packet2.Packet)) (*IdkType, error) {
	w := packet2.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet2.NewPacket(w.Bytes())
	idk := NewIdkType(0)
	err := DecodeType(r, idk)
	return idk, err
}

func TestIdkTypeDecode_Type(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(1); p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.Type != 3 {
		t.Errorf("Type: got %d, want 3", idk.Type)
	}
}

func TestIdkTypeDecode_Models(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) {
		p.P1(2)
		p.P1(2)      // count = 2
		p.P2(0x0100) // model[0] = 256
		p.P2(0x0200) // model[1] = 512
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if len(idk.Models) != 2 {
		t.Fatalf("Models len: got %d, want 2", len(idk.Models))
	}
	if idk.Models[0] != 256 {
		t.Errorf("Models[0]: got %d, want 256", idk.Models[0])
	}
	if idk.Models[1] != 512 {
		t.Errorf("Models[1]: got %d, want 512", idk.Models[1])
	}
}

func TestIdkTypeDecode_Disable(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !idk.Disable {
		t.Error("Disable: got false, want true")
	}
}

func TestIdkTypeDecode_RecolS(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(40); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.RecolS[0] != 0x0102 {
		t.Errorf("RecolS[0]: got %d, want %d", idk.RecolS[0], 0x0102)
	}
}

func TestIdkTypeDecode_RecolS_OutOfRange(t *testing.T) {
	// code 46 → slot 6, out-of-range for [6]uint16; G2 consumed, RecolS unchanged.
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(46); p.P2(0xBEEF) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	var want [6]uint16
	if idk.RecolS != want {
		t.Errorf("RecolS after code 46: got %v, want all zero (out-of-range guard)", idk.RecolS)
	}
}

func TestIdkTypeDecode_RecolD(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(50); p.P2(0x0304) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.RecolD[0] != 0x0304 {
		t.Errorf("RecolD[0]: got %d, want %d", idk.RecolD[0], 0x0304)
	}
}

func TestIdkTypeDecode_Heads(t *testing.T) {
	// code 60 → Heads[0]
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(60); p.P2(0x0506) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.Heads[0] != 0x0506 {
		t.Errorf("Heads[0]: got %d, want %d", idk.Heads[0], 0x0506)
	}

	// code 64 → Heads[4]
	idk2, err := decodeIdk(func(p *packet2.Packet) { p.P1(64); p.P2(0x0708) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk2.Heads[4] != 0x0708 {
		t.Errorf("Heads[4]: got %d, want %d", idk2.Heads[4], 0x0708)
	}

	// code 65 → out-of-range, Heads unchanged (guard: slot=5 >= 5)
	idk3, err := decodeIdk(func(p *packet2.Packet) { p.P1(65); p.P2(0x090A) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	want := [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}
	if idk3.Heads != want {
		t.Errorf("Heads after code 65: got %v, want all 0xFFFF (out-of-range guard)", idk3.Heads)
	}
}

func TestIdkTypeDecode_DebugName(t *testing.T) {
	idk, err := decodeIdk(func(p *packet2.Packet) { p.P1(250); p.PJStrLF("test_idk") })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if idk.DebugName != "test_idk" {
		t.Errorf("DebugName: got %q, want \"test_idk\"", idk.DebugName)
	}
}

func TestIdkTypeDecode_UnknownCode(t *testing.T) {
	_, err := decodeIdk(func(p *packet2.Packet) { p.P1(99) })
	if err == nil {
		t.Error("want error for unknown code 99, got nil")
	}
}

// TestLoadIdkTypes_MissingServerDat pins that LoadIdkTypes returns an empty
// registry (not an error) when server/idk.dat is absent, matching TS
// IdkType.load's early-return on missing file.
func TestLoadIdkTypes_MissingServerDat(t *testing.T) {
	dir := t.TempDir()
	// No server/idk.dat created — directory exists but file is absent.
	configs, err := LoadIdkTypes(dir)
	if err != nil {
		t.Fatalf("LoadIdkTypes: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Configs) != 0 {
		t.Errorf("Configs: want empty slice, got %d entries", len(configs.Configs))
	}
	if len(configs.ConfigNames) != 0 {
		t.Errorf("ConfigNames: want empty map, got %d entries", len(configs.ConfigNames))
	}
}

// TestLoadIdkTypes_FromPack loads IdkTypes from the real pack directory.
// Skipped when the pack data is absent (CI / clean checkout).
func TestLoadIdkTypes_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "idk.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	configs, err := LoadIdkTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadIdkTypes: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one IdkType, got 0")
	}
}

func TestIdkTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs: []*IdkType{
			{ID: 0, DebugName: "first"},
			{ID: 1, DebugName: "second"},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestIdkTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs:     []*IdkType{{ID: 0, DebugName: "only"}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestIdkTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *IdkTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestIdkTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs: []*IdkType{
			{ID: 0, DebugName: "other"},
			{ID: 1, DebugName: "fresh"},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestIdkTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs:     []*IdkType{{ID: 0, DebugName: "scan_me"}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
