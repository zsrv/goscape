package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewSpotanimTypeDefaults(t *testing.T) {
	s := NewSpotanimType(3)
	if s.ID != 3 {
		t.Errorf("ID: got %d, want 3", s.ID)
	}
	if s.Model != 0 {
		t.Errorf("Model: got %d, want 0", s.Model)
	}
	if s.Anim != -1 {
		t.Errorf("Anim: got %d, want -1", s.Anim)
	}
	if s.HasAlpha {
		t.Error("HasAlpha: got true, want false")
	}
	if s.Resizeh != 128 {
		t.Errorf("Resizeh: got %d, want 128", s.Resizeh)
	}
	if s.Resizev != 128 {
		t.Errorf("Resizev: got %d, want 128", s.Resizev)
	}
	if s.Orientation != 0 {
		t.Errorf("Orientation: got %d, want 0", s.Orientation)
	}
	if s.Ambient != 0 {
		t.Errorf("Ambient: got %d, want 0", s.Ambient)
	}
	if s.Contrast != 0 {
		t.Errorf("Contrast: got %d, want 0", s.Contrast)
	}
	var wantRecol [6]uint16
	if s.RecolS != wantRecol {
		t.Errorf("RecolS: got %v, want all zero", s.RecolS)
	}
	if s.RecolD != wantRecol {
		t.Errorf("RecolD: got %v, want all zero", s.RecolD)
	}
}

// decodeSpotanim builds a writer packet, appends a 0-terminator, flips to
// reader, and runs DecodeType on a fresh NewSpotanimType(0).
// Mirrors idktype_test.go's decodeIdk pattern.
func decodeSpotanim(build func(*packet.Packet)) (*SpotanimType, error) {
	w := packet.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet.NewPacket(w.Bytes())
	s := NewSpotanimType(0)
	err := DecodeType(r, s)
	return s, err
}

func TestSpotanimTypeDecode_Model(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(1); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Model != 0x0102 {
		t.Errorf("Model: got %d, want %d", s.Model, 0x0102)
	}
}

func TestSpotanimTypeDecode_Anim(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(2); p.P2(0x0203) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Anim != 0x0203 {
		t.Errorf("Anim: got %d, want %d", s.Anim, 0x0203)
	}
}

func TestSpotanimTypeDecode_HasAlpha(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !s.HasAlpha {
		t.Error("HasAlpha: got false, want true")
	}
}

func TestSpotanimTypeDecode_Resizeh(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(4); p.P2(0x0200) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Resizeh != 0x0200 {
		t.Errorf("Resizeh: got %d, want %d", s.Resizeh, 0x0200)
	}
}

func TestSpotanimTypeDecode_Resizev(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(5); p.P2(0x0100) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Resizev != 0x0100 {
		t.Errorf("Resizev: got %d, want %d", s.Resizev, 0x0100)
	}
}

func TestSpotanimTypeDecode_Orientation(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(6); p.P2(0x0090) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Orientation != 0x0090 {
		t.Errorf("Orientation: got %d, want %d", s.Orientation, 0x0090)
	}
}

func TestSpotanimTypeDecode_Ambient(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(7); p.P1(42) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Ambient != 42 {
		t.Errorf("Ambient: got %d, want 42", s.Ambient)
	}
}

func TestSpotanimTypeDecode_Contrast(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(8); p.P1(17) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.Contrast != 17 {
		t.Errorf("Contrast: got %d, want 17", s.Contrast)
	}
}

func TestSpotanimTypeDecode_RecolS(t *testing.T) {
	// code 40 → slot 0
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(40); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.RecolS[0] != 0x0102 {
		t.Errorf("RecolS[0]: got %d, want %d", s.RecolS[0], 0x0102)
	}

	// code 45 → slot 5 (last valid slot)
	s2, err := decodeSpotanim(func(p *packet.Packet) { p.P1(45); p.P2(0x0506) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s2.RecolS[5] != 0x0506 {
		t.Errorf("RecolS[5]: got %d, want %d", s2.RecolS[5], 0x0506)
	}
}

func TestSpotanimTypeDecode_RecolS_OutOfRange(t *testing.T) {
	// code 46 → slot 6, out-of-range for [6]uint16; G2 consumed, RecolS unchanged.
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(46); p.P2(0xBEEF) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	var want [6]uint16
	if s.RecolS != want {
		t.Errorf("RecolS after code 46: got %v, want all zero (out-of-range guard)", s.RecolS)
	}
}

func TestSpotanimTypeDecode_RecolD(t *testing.T) {
	// code 50 → slot 0
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(50); p.P2(0x0304) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.RecolD[0] != 0x0304 {
		t.Errorf("RecolD[0]: got %d, want %d", s.RecolD[0], 0x0304)
	}

	// code 55 → slot 5 (last valid slot)
	s2, err := decodeSpotanim(func(p *packet.Packet) { p.P1(55); p.P2(0x0708) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s2.RecolD[5] != 0x0708 {
		t.Errorf("RecolD[5]: got %d, want %d", s2.RecolD[5], 0x0708)
	}
}

func TestSpotanimTypeDecode_RecolD_OutOfRange(t *testing.T) {
	// code 56 → slot 6, out-of-range for [6]uint16; G2 consumed, RecolD unchanged.
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(56); p.P2(0xDEAD) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	var want [6]uint16
	if s.RecolD != want {
		t.Errorf("RecolD after code 56: got %v, want all zero (out-of-range guard)", s.RecolD)
	}
}

func TestSpotanimTypeDecode_DebugName(t *testing.T) {
	s, err := decodeSpotanim(func(p *packet.Packet) { p.P1(250); p.PJStrLF("test_spotanim") })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if s.DebugName != "test_spotanim" {
		t.Errorf("DebugName: got %q, want \"test_spotanim\"", s.DebugName)
	}
}

func TestSpotanimTypeDecode_UnknownCode(t *testing.T) {
	_, err := decodeSpotanim(func(p *packet.Packet) { p.P1(99) })
	if err == nil {
		t.Error("want error for unknown code 99, got nil")
	}
}

// TestLoadSpotanimTypes_MissingFileSilent pins that LoadSpotanimTypes returns
// an empty registry (not an error) when server/spotanim.dat is absent,
// matching TS SpotanimType.load's early-return on missing file.
func TestLoadSpotanimTypes_MissingFileSilent(t *testing.T) {
	dir := t.TempDir()
	// No server/spotanim.dat created — directory exists but file is absent.
	configs, err := LoadSpotanimTypes(dir)
	if err != nil {
		t.Fatalf("LoadSpotanimTypes: want nil error on missing file, got %v", err)
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

// TestLoadSpotanimTypes_FromPack loads SpotanimTypes from the real pack
// directory. Skipped when pack data is absent (CI / clean checkout).
func TestLoadSpotanimTypes_FromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "spotanim.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	configs, err := LoadSpotanimTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadSpotanimTypes: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one SpotanimType, got 0")
	}
}

func TestSpotanimTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs: []*SpotanimType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
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

func TestSpotanimTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs:     []*SpotanimType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestSpotanimTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *SpotanimTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestSpotanimTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs: []*SpotanimType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
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

func TestSpotanimTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs:     []*SpotanimType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
