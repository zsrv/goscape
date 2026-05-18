package objtype

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// buildJagWithFloDat saves an empty jagfile with a single "flo.dat"
// entry to a temp file and re-loads it. Jagfile.Write only queues —
// data isn't readable until Save+LoadJagfile materialises a real
// serialized jagfile. This mirrors loctype_test's buildClientJag
// intent (in-memory parsed Jagfile) while staying on the high-level
// Save/LoadJagfile path.
func buildJagWithFloDat(t *testing.T, clientFlo *packet2.Packet) *jagfile.Jagfile {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(false)
	jf.Write("flo.dat", clientFlo)
	path := filepath.Join(t.TempDir(), "config")
	if err := jf.Save(path); err != nil {
		t.Fatalf("jagfile.Save: %v", err)
	}
	loaded, err := jagfile.LoadJagfile(path)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	return loaded
}

// buildFloFixture builds an in-memory (server, clientJag) pair as
// produced by TS FloConfig.packFloConfigs: server stream is just
// count + zero terminators, client jag's flo.dat carries the real
// opcode data, prefixed by a 2-byte count.
func buildFloFixture(t *testing.T, entries []string) (*packet2.Packet, *jagfile.Jagfile) {
	t.Helper()

	server := packet2.Alloc(1)
	server.P2(uint16(len(entries)))
	for range entries {
		server.P1(0) // server.next() per id
	}

	clientFlo := packet2.Alloc(1)
	clientFlo.P2(uint16(len(entries)))
	for _, name := range entries {
		// Only emit debugname (opcode 6) — matches the "no flo_ prefix"
		// path in FloConfig.packFloConfigs for named entries.
		if name != "" {
			clientFlo.P1(6)
			clientFlo.PJStrLF(name)
		}
		clientFlo.P1(0)
	}

	return server, buildJagWithFloDat(t, clientFlo)
}

func TestFloTypeConfigs_GetId_RoundTrip(t *testing.T) {
	t.Parallel()
	server, jag := buildFloFixture(t, []string{"water", "muddygrass", "grass"})
	defer server.Release()

	cfg, err := parseFloTypes(server, jag)
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

func TestFloTypeConfigs_AllOpcodes(t *testing.T) {
	t.Parallel()
	// One entry exercising every valid opcode (1,2,3,5,6) interleaved
	// on the client side. Server side is just one zero per id.
	server := packet2.Alloc(1)
	defer server.Release()
	server.P2(1)
	server.P1(0)

	clientFlo := packet2.Alloc(1)
	clientFlo.P2(1)
	clientFlo.P1(1) // rgb (G3)
	clientFlo.P3(0xaabbcc)
	clientFlo.P1(2) // texture (G1)
	clientFlo.P1(7)
	clientFlo.P1(3) // overlay = true (no payload)
	clientFlo.P1(5) // occlude = false (no payload)
	clientFlo.P1(6) // debugname
	clientFlo.PJStrLF("sandygrass")
	clientFlo.P1(0)

	jag := buildJagWithFloDat(t, clientFlo)

	cfg, err := parseFloTypes(server, jag)
	if err != nil {
		t.Fatalf("parseFloTypes: %v", err)
	}
	if got, want := cfg.GetId("sandygrass"), 0; got != want {
		t.Errorf("GetId(sandygrass) = %d, want %d", got, want)
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
