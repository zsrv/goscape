package objtype

import (
	"os"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type varpEntry struct {
	debugName  string
	scope      int
	transmit   bool
	clientCode uint16
}

// hashVarpDat is genHash("varp.dat") — pre-computed via the algorithm in
// pkg/io/jagfile/jagfile.go:18-25 (uppercase + h*61+c-32 reduction).
const hashVarpDat uint32 = 383739196

// buildVarpServerDat assembles the server-side varp.dat blob:
//
//	u16 count
//	for each entry: codes 1 (scope), 6 (transmit), 250 (debugname),
//	terminated by code 0.
func buildVarpServerDat(entries []varpEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.scope != 0 {
			pkt.P1(1)
			pkt.P1(uint8(e.scope))
		}
		if e.transmit {
			pkt.P1(6)
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0) // terminator
	}
	return pkt.Bytes()
}

// buildVarpClientDat assembles the inner client-side varp.dat payload (the
// blob that lives inside client/config jagfile under entry name "varp.dat"):
//
//	u16 count
//	for each entry: code 5 (clientcode), terminated by code 0.
func buildVarpClientDat(entries []varpEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.clientCode != 0 {
			pkt.P1(5)
			pkt.P2(e.clientCode)
		}
		pkt.P1(0) // terminator
	}
	return pkt.Bytes()
}

// buildClientJag wraps a single-entry jagfile around the given client
// varp.dat blob and returns a parsed *jag.Jagfile ready for parseVarpTypes.
// Mirrors loctype_test.go:97-117 buildClientJag pattern.
func buildVarpClientJag(t *testing.T, varpDatBytes []byte) *jag.Jagfile {
	t.Helper()
	compressed, err := jag.BZip2Compress(varpDatBytes, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)                         // unpackedSize (== packedSize → CompressWhole=false outer path)
	p.P3(1)                         // packedSize
	p.P2(1)                         // fileCount = 1
	p.P4(hashVarpDat)               // file hash
	p.P3(uint32(len(varpDatBytes))) // unpacked size
	p.P3(uint32(len(compressed)))   // packed size
	p.Data = append(p.Data, compressed...)

	jf, err := jag.NewJagfile(packet2.NewPacket(p.Data))
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	return jf
}

// buildVarpFixture is a convenience that builds both server bytes and
// client jagfile from a single entries list, ready for parseVarpTypes.
func buildVarpFixture(t *testing.T, entries []varpEntry) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	server := packet2.NewPacket(buildVarpServerDat(entries))
	clientJag := buildVarpClientJag(t, buildVarpClientDat(entries))
	return server, clientJag
}

func TestParseVarpTypes(t *testing.T) {
	entries := []varpEntry{
		{debugName: "coins", scope: 0, transmit: true},
		{debugName: "quest_state", scope: 1},
		{debugName: "anon"},
	}

	server, clientJag := buildVarpFixture(t, entries)

	cfgs, err := parseVarpTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if len(cfgs.Configs) != 3 {
		t.Fatalf("configs: got %d, want 3", len(cfgs.Configs))
	}
	if cfgs.Configs[0].DebugName != "coins" || !cfgs.Configs[0].Transmit {
		t.Errorf("coins: got %+v", cfgs.Configs[0])
	}
	if cfgs.Configs[1].Scope != VarpScopePerm {
		t.Errorf("quest_state scope: got %d, want %d", cfgs.Configs[1].Scope, VarpScopePerm)
	}
	if cfgs.ConfigNames["coins"] != 0 {
		t.Errorf("ConfigNames[coins]: got %d, want 0", cfgs.ConfigNames["coins"])
	}
}

func TestVarpProtectDefaultTrue(t *testing.T) {
	// No code 4 → Protect stays true.
	entries := []varpEntry{{"x", 0, false, 0}}
	server, clientJag := buildVarpFixture(t, entries)
	cfgs, err := parseVarpTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if !cfgs.Configs[0].Protect {
		t.Errorf("Protect default: got false, want true")
	}
}

// TestParseVarpTypes_RunIDDefaultsZeroWhenNoClientCode7 pins the TS-faithful
// default-0 fallback (VarPlayerType.ts:18) when no clientcode-7 config exists.
// clientcodes are placed in the CLIENT stream (production-faithful).
func TestParseVarpTypes_RunIDDefaultsZeroWhenNoClientCode7(t *testing.T) {
	serverEntries := []varpEntry{
		{debugName: "alpha"},
		{debugName: "beta"},
	}
	clientEntries := []varpEntry{
		{clientCode: 1},
		{clientCode: 3},
	}

	server := packet2.NewPacket(buildVarpServerDat(serverEntries))
	clientJag := buildVarpClientJag(t, buildVarpClientDat(clientEntries))

	cfgs, err := parseVarpTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if cfgs.RunID != 0 {
		t.Errorf("RunID: got %d, want 0 (default)", cfgs.RunID)
	}
}

// TestParseVarpTypes_DiscoversRunIDFromClientStream is the new production-faithful
// test: clientcode opcode lives ONLY in the client stream (never the server stream).
// Asserts that RunID is discovered correctly when the client stream carries clientcode=7.
func TestParseVarpTypes_DiscoversRunIDFromClientStream(t *testing.T) {
	const runVarpID = 2
	serverEntries := []varpEntry{
		{debugName: "varp_a", scope: 0, transmit: false},
		{debugName: "varp_b", scope: 1, transmit: true},
		{debugName: "option_run", scope: 0, transmit: true}, // id=2, NO clientcode here
		{debugName: "varp_c", scope: 0, transmit: false},
	}
	clientEntries := []varpEntry{
		{clientCode: 0},
		{clientCode: 0},
		{clientCode: 7}, // id=2 — run varp, clientcode ONLY in client stream
		{clientCode: 0},
	}

	server := packet2.NewPacket(buildVarpServerDat(serverEntries))
	clientJag := buildVarpClientJag(t, buildVarpClientDat(clientEntries))

	cfgs, err := parseVarpTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if cfgs.RunID != runVarpID {
		t.Errorf("RunID: got %d, want %d", cfgs.RunID, runVarpID)
	}
	if cfgs.Configs[runVarpID].ClientCode != 7 {
		t.Errorf("Configs[%d].ClientCode: got %d, want 7", runVarpID, cfgs.Configs[runVarpID].ClientCode)
	}
	if cfgs.Configs[runVarpID].DebugName != "option_run" {
		t.Errorf("Configs[%d].DebugName: got %q, want %q", runVarpID, cfgs.Configs[runVarpID].DebugName, "option_run")
	}
	// Verify clientcode did NOT bleed in from server stream (server had no opcode 5)
	if cfgs.Configs[0].ClientCode != 0 {
		t.Errorf("Configs[0].ClientCode: got %d, want 0 (no client-stream clientcode)", cfgs.Configs[0].ClientCode)
	}
}

func TestVarpTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ID: 0, DebugName: "first"}, {ID: 1, DebugName: "second"}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := vtc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestVarpTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ID: 0, DebugName: "only"}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := vtc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestVarpTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var vtc *VarpTypeConfigs
	if got := vtc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestVarpTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	// ConfigNames points "fresh" at id=5 but Configs is only length 2.
	// Lookup must NOT panic and must fall through to the linear scan,
	// which finds "fresh" at id=1 by DebugName equality.
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ID: 0, DebugName: "other"}, {ID: 1, DebugName: "fresh"}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := vtc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestVarpTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	// Some test fixtures construct Configs without populating ConfigNames.
	// ByName must still resolve by DebugName.
	vtc := &VarpTypeConfigs{
		Configs:     []*VarPlayerType{{ID: 0, DebugName: "scan_me"}},
		ConfigNames: nil,
	}
	got := vtc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}

// TestLoadVarpTypes_ProductionCacheRunIDIs173 binds the two-stream fix to the
// actual production cache. Skipped when data/pack/client/config is absent.
func TestLoadVarpTypes_ProductionCacheRunIDIs173(t *testing.T) {
	const cacheDir = "../../data/pack"
	if _, err := os.Stat(cacheDir + "/client/config"); err != nil {
		t.Skipf("production cache not present (%s/client/config absent): %v", cacheDir, err)
	}

	cfgs, err := LoadVarpTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarpTypes: %v", err)
	}
	if cfgs.RunID != 173 {
		t.Errorf("RunID: got %d, want 173", cfgs.RunID)
	}
	if cfgs.Configs[173].ClientCode != 7 {
		t.Errorf("Configs[173].ClientCode: got %d, want 7", cfgs.Configs[173].ClientCode)
	}
	if got, ok := cfgs.ConfigNames["option_run"]; !ok || got != 173 {
		t.Errorf("ConfigNames[option_run]: got %d (ok=%v), want 173", got, ok)
	}
}
