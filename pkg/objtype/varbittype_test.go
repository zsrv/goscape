package objtype

import (
	"os"
	"path/filepath"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type varbitEntry struct {
	debugName string
	// hasRange gates emission of code 1 (basevar/startbit/endbit).
	hasRange bool
	basevar  int
	startbit int
	endbit   int
}

// hashVarbitDat is genHash("varbit.dat") — pre-computed via the algorithm
// in pkg/io/jagfile/jagfile.go:34-41 (uppercase + h*61+c-32 reduction).
const hashVarbitDat uint32 = 3780097711

// buildVarbitDat assembles a varbit.dat-shaped blob (used for BOTH the
// server stream and the inner client jagfile payload — the wire format is
// identical, only the code distribution differs in production):
//
//	u16 count
//	for each entry: code 1 (basevar g2, startbit g1, endbit g1),
//	code 250 (debugname gjstr), terminated by code 0.
func buildVarbitDat(entries []varbitEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.hasRange {
			pkt.P1(1)
			pkt.P2(uint16(e.basevar))
			pkt.P1(uint8(e.startbit))
			pkt.P1(uint8(e.endbit))
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0) // terminator
	}
	return pkt.Bytes()
}

// buildVarbitClientJag wraps a single-entry jagfile around the given client
// varbit.dat blob, mirroring varptype_test.go's buildVarpClientJag.
func buildVarbitClientJag(t *testing.T, varbitDatBytes []byte) *jag.Jagfile {
	t.Helper()
	compressed, err := jag.BZip2Compress(varbitDatBytes, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)                           // unpackedSize (== packedSize → CompressWhole=false outer path)
	p.P3(1)                           // packedSize
	p.P2(1)                           // fileCount = 1
	p.P4(hashVarbitDat)               // file hash
	p.P3(uint32(len(varbitDatBytes))) // unpacked size
	p.P3(uint32(len(compressed)))     // packed size
	p.Data = append(p.Data, compressed...)

	jf, err := jag.NewJagfile(packet2.NewPacket(p.Data))
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	return jf
}

// emptyVarbitClientDat builds a client stream of bare terminators for the
// given entry count (production puts code 1 in the server stream).
func emptyVarbitClientDat(count int) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(count))
	for range count {
		pkt.P1(0)
	}
	return pkt.Bytes()
}

func TestParseVarBitTypes(t *testing.T) {
	entries := []varbitEntry{
		{debugName: "prayer_thick_skin", hasRange: true, basevar: 83, startbit: 0, endbit: 0},
		{debugName: "bank_insert_mode", hasRange: true, basevar: 0, startbit: 4, endbit: 7},
		{debugName: "name_only"},
	}

	server := packet2.NewPacket(buildVarbitDat(entries))
	clientJag := buildVarbitClientJag(t, emptyVarbitClientDat(len(entries)))

	cfgs, err := parseVarBitTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarBitTypes: %v", err)
	}
	if len(cfgs.Configs) != 3 {
		t.Fatalf("configs: got %d, want 3", len(cfgs.Configs))
	}
	if got := cfgs.Configs[0]; got.Basevar != 83 || got.Startbit != 0 || got.Endbit != 0 || got.DebugName != "prayer_thick_skin" {
		t.Errorf("config 0: got %+v", got)
	}
	if got := cfgs.Configs[1]; got.Basevar != 0 || got.Startbit != 4 || got.Endbit != 7 {
		t.Errorf("config 1: got %+v", got)
	}
	// Code-1 absent → TS field-initializer defaults -1/-1/-1
	// (VarBitType.ts:68-70).
	if got := cfgs.Configs[2]; got.Basevar != -1 || got.Startbit != -1 || got.Endbit != -1 || got.DebugName != "name_only" {
		t.Errorf("config 2 (defaults): got %+v", got)
	}
	if cfgs.ConfigNames["bank_insert_mode"] != 1 {
		t.Errorf("ConfigNames[bank_insert_mode]: got %d, want 1", cfgs.ConfigNames["bank_insert_mode"])
	}
}

// TestParseVarBitTypes_ClientStreamDecoded pins the dual-pass: a code-1
// range carried ONLY by the client stream must still populate the config
// (TS parse decodes server then client into the same VarBitType,
// VarBitType.ts:32-42).
func TestParseVarBitTypes_ClientStreamDecoded(t *testing.T) {
	serverEntries := []varbitEntry{{debugName: "from_client"}}
	clientEntries := []varbitEntry{{hasRange: true, basevar: 12, startbit: 2, endbit: 5}}

	server := packet2.NewPacket(buildVarbitDat(serverEntries))
	clientJag := buildVarbitClientJag(t, buildVarbitDat(clientEntries))

	cfgs, err := parseVarBitTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseVarBitTypes: %v", err)
	}
	got := cfgs.Configs[0]
	if got.Basevar != 12 || got.Startbit != 2 || got.Endbit != 5 || got.DebugName != "from_client" {
		t.Errorf("config 0: got %+v", got)
	}
}

// TestVarBitTypeDecode_UnknownCode pins the package convention shared with
// varp/varn: unknown codes log a warning and decoding continues (TS
// VarBitType.ts:80 printError is non-fatal); no error is returned.
func TestVarBitTypeDecode_UnknownCode(t *testing.T) {
	cfg := NewVarBitType(0)
	dat := packet2.NewPacket([]byte{0xFF})
	if err := cfg.Decode(99, dat); err != nil {
		t.Fatalf("Decode(99): got err %v, want nil (printError convention)", err)
	}
	if cfg.Basevar != -1 || cfg.Startbit != -1 || cfg.Endbit != -1 {
		t.Errorf("unknown code mutated fields: %+v", cfg)
	}
}

// TestLoadVarBitTypes_MissingFileReturnsEmpty pins the TS early-return:
// VarBitType.load:14-16 skips loading entirely when server/varbit.dat is
// absent. A 245.2-era cache (no varbit.dat) must still boot with an empty
// registry, not an error.
func TestLoadVarBitTypes_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgs, err := LoadVarBitTypes(dir)
	if err != nil {
		t.Fatalf("LoadVarBitTypes(empty dir): %v", err)
	}
	if cfgs == nil {
		t.Fatal("LoadVarBitTypes(empty dir) = nil registry, want non-nil empty")
	}
	if len(cfgs.Configs) != 0 {
		t.Errorf("Configs: got %d entries, want 0", len(cfgs.Configs))
	}
	if cfgs.Get(0) != nil {
		t.Errorf("Get(0) on empty registry: got non-nil")
	}
}

// TestLoadVarBitTypes_RoundTrip writes server/varbit.dat + client/config
// to disk and loads through the public entry point.
func TestLoadVarBitTypes_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "client"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []varbitEntry{{debugName: "rt", hasRange: true, basevar: 7, startbit: 1, endbit: 3}}
	if err := os.WriteFile(filepath.Join(dir, "server", "varbit.dat"), buildVarbitDat(entries), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-assemble the raw client/config jagfile bytes (same layout as
	// buildVarbitClientJag, written to disk instead of parsed in-memory).
	clientDat := emptyVarbitClientDat(len(entries))
	compressed, err := jag.BZip2Compress(clientDat, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)
	p.P3(1)
	p.P2(1)
	p.P4(hashVarbitDat)
	p.P3(uint32(len(clientDat)))
	p.P3(uint32(len(compressed)))
	p.Data = append(p.Data, compressed...)
	if err := os.WriteFile(filepath.Join(dir, "client", "config"), p.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs, err := LoadVarBitTypes(dir)
	if err != nil {
		t.Fatalf("LoadVarBitTypes: %v", err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("Configs: got %d, want 1", len(cfgs.Configs))
	}
	got := cfgs.Get(0)
	if got == nil || got.Basevar != 7 || got.Startbit != 1 || got.Endbit != 3 || got.DebugName != "rt" {
		t.Errorf("Get(0): got %+v", got)
	}
}

func TestVarBitTypeConfigs_Get_NilAndOOBSafe(t *testing.T) {
	var nilCfgs *VarBitTypeConfigs
	if nilCfgs.Get(0) != nil {
		t.Error("nil-receiver Get(0): got non-nil")
	}
	cfgs := &VarBitTypeConfigs{Configs: []*VarBitType{NewVarBitType(0)}}
	if cfgs.Get(-1) != nil {
		t.Error("Get(-1): got non-nil")
	}
	if cfgs.Get(1) != nil {
		t.Error("Get(1) OOB: got non-nil")
	}
	if cfgs.Get(0) == nil {
		t.Error("Get(0): got nil, want config")
	}
}

// TestVarBitTypeConfigs_ByName pins the debugname lookup mirroring TS
// VarBitType.getByName (VarBitType.ts:53-60 @43e02957): index hit,
// linear-scan fallback for unindexed fixtures, miss → nil, and
// nil-receiver tolerance (245.2-era cache with no varbit registry).
func TestVarBitTypeConfigs_ByName(t *testing.T) {
	cfgs := &VarBitTypeConfigs{
		Configs: []*VarBitType{
			{ConfigType: ConfigType{ID: 0, DebugName: "alpha"}, Basevar: 0, Startbit: 0, Endbit: 3},
			{ConfigType: ConfigType{ID: 1, DebugName: "beta"}, Basevar: 1, Startbit: 4, Endbit: 7},
		},
		ConfigNames: map[string]int{"alpha": 0, "beta": 1},
	}
	if got := cfgs.ByName("beta"); got == nil || got.ID != 1 {
		t.Errorf("ByName(beta): got %+v, want config id 1", got)
	}
	if got := cfgs.ByName("no_such"); got != nil {
		t.Errorf("ByName(no_such): got %+v, want nil", got)
	}

	// Linear-scan fallback when ConfigNames is unpopulated (test fixtures).
	unindexed := &VarBitTypeConfigs{
		Configs: []*VarBitType{
			{ConfigType: ConfigType{ID: 0, DebugName: "gamma"}, Basevar: 0, Startbit: 0, Endbit: 3},
		},
	}
	if got := unindexed.ByName("gamma"); got == nil || got.ID != 0 {
		t.Errorf("ByName(gamma) without index: got %+v, want config id 0", got)
	}

	var nilCfgs *VarBitTypeConfigs
	if got := nilCfgs.ByName("alpha"); got != nil {
		t.Errorf("nil-receiver ByName: got %+v, want nil", got)
	}
}
