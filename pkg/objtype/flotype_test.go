package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestFloTypeConfigs_GetId_RoundTrip(t *testing.T) {
	t.Parallel()

	dat := packet2.Alloc(1)
	defer dat.Release()
	dat.P2(3)
	dat.P1(2)
	dat.PJStrLF("water")
	dat.P1(0)
	dat.P1(2)
	dat.PJStrLF("muddygrass")
	dat.P1(0)
	dat.P1(2)
	dat.PJStrLF("grass")
	dat.P1(0)

	cfg, err := parseFloTypes(dat)
	if err != nil {
		t.Fatalf("parseFloTypes: %v", err)
	}
	if got, want := len(cfg.Configs), 3; got != want {
		t.Errorf("len(Configs) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("water"), 0; got != want {
		t.Errorf("GetId(water) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("muddygrass"), 1; got != want {
		t.Errorf("GetId(muddygrass) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("nope"), -1; got != want {
		t.Errorf("GetId(nope) = %d, want %d", got, want)
	}
}

func TestFloTypeConfigs_SkipsUnknownOpcodes(t *testing.T) {
	t.Parallel()

	dat := packet2.Alloc(1)
	defer dat.Release()
	dat.P2(1)
	dat.P1(1)
	dat.P3(0xaabbcc)
	dat.P1(2)
	dat.PJStrLF("sandygrass")
	dat.P1(5)
	dat.P1(1)
	dat.P1(6)
	dat.P2(0xdead)
	dat.P1(7)
	dat.P3(0x112233)
	dat.P1(3)
	dat.PJStrLF("planks")
	dat.P1(8)
	dat.P3(0x445566)
	dat.P1(0)

	cfg, err := parseFloTypes(dat)
	if err != nil {
		t.Fatalf("parseFloTypes: %v", err)
	}
	if got, want := cfg.GetId("sandygrass"), 0; got != want {
		t.Errorf("GetId = %d, want %d", got, want)
	}
}

func TestLoadFloTypes_RealContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	dir := "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	if _, err := filepath.Abs(dir); err != nil {
		t.Skipf("real flo.dat not available: %v", err)
	}
	cfg, err := LoadFloTypes(dir)
	if err != nil {
		t.Skipf("LoadFloTypes(%s): %v", dir, err)
	}
	if len(cfg.Configs) == 0 {
		t.Fatalf("len(Configs) = 0")
	}
	if cfg.GetId("water") < 0 {
		t.Errorf("GetId(water) = -1 (expected >= 0)")
	}
	if cfg.GetId("muddygrass") < 0 {
		t.Errorf("GetId(muddygrass) = -1 (expected >= 0)")
	}
}
