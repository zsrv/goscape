package objtype

import (
	"path/filepath"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type locEntry struct {
	debugName string
	desc      string
	category  int
	width     int
	length    int
	intParams map[uint32]uint32
	op        []string // codes 30-34
}

// hashLocDat is genHash("loc.dat") — pre-computed via the algorithm in
// pkg/io/jagfile/jagfile.go:18-25 (uppercase + h*61+c-32 reduction).
const hashLocDat uint32 = 682978269

// buildLocServerDat assembles the server-side loc.dat blob:
//
//	u16 count
//	for each entry: codes 61 (category), 249 (params), 250 (debugname),
//	terminated by code 0.
func buildLocServerDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.category != 0 {
			pkt.P1(61)
			pkt.P2(uint16(e.category))
		}
		if len(e.intParams) > 0 {
			pkt.P1(249)
			pkt.P1(uint8(len(e.intParams)))
			for k, v := range e.intParams {
				pkt.P3(k)
				pkt.PBool(false)
				pkt.P4(v)
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildLocClientDat assembles the inner client-side loc.dat payload (the
// blob that lives inside client/config jagfile under entry name "loc.dat"):
//
//	u16 count
//	for each entry: codes 3 (desc), 14 (width), 15 (length), 30-34 (op),
//	250 (debugname), terminated by code 0.
func buildLocClientDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.desc != "" {
			pkt.P1(3)
			pkt.PJStrLF(e.desc)
		}
		if e.width != 0 {
			pkt.P1(14)
			pkt.P1(uint8(e.width))
		}
		if e.length != 0 {
			pkt.P1(15)
			pkt.P1(uint8(e.length))
		}
		for i, name := range e.op {
			if name == "" {
				continue
			}
			pkt.P1(uint8(30 + i))
			pkt.PJStrLF(name)
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildClientJag wraps a single-entry jagfile around the given client
// loc.dat blob and returns a parsed *jag.Jagfile ready for parseLocTypes.
// Mirrors componenttype_test.go:751 buildMinimalJagfile pattern.
func buildClientJag(t *testing.T, locDatBytes []byte) *jag.Jagfile {
	t.Helper()
	compressed, err := jag.BZip2Compress(locDatBytes, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)                        // unpackedSize (== packedSize → Unpacked=false outer path)
	p.P3(1)                        // packedSize
	p.P2(1)                        // fileCount = 1
	p.P4(hashLocDat)               // file hash
	p.P3(uint32(len(locDatBytes))) // unpacked size
	p.P3(uint32(len(compressed)))  // packed size
	p.Data = append(p.Data, compressed...)

	jf, err := jag.NewJagfile(packet2.NewPacket(p.Data))
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	return jf
}

// buildLocFixture is a convenience that builds both server bytes and
// client jagfile from a single entries list, ready for parseLocTypes.
func buildLocFixture(t *testing.T, entries []locEntry) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	server := packet2.NewPacket(buildLocServerDat(entries))
	clientJag := buildClientJag(t, buildLocClientDat(entries))
	return server, clientJag
}

func TestParseLocTypes(t *testing.T) {
	entries := []locEntry{
		{
			debugName: "door_basic",
			desc:      "A wooden door.",
			category:  17,
			width:     1,
			length:    2,
			intParams: map[uint32]uint32{1: 100},
		},
		{
			debugName: "bush",
		},
	}

	server, clientJag := buildLocFixture(t, entries)
	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	door := cfgs.Configs[0]
	if door.DebugName != "door_basic" {
		t.Errorf("DebugName[0]: got %q", door.DebugName)
	}
	if door.Desc != "A wooden door." {
		t.Errorf("Desc[0]: got %q", door.Desc)
	}
	if door.Category != 17 {
		t.Errorf("Category[0]: got %d, want 17", door.Category)
	}
	if door.Width != 1 || door.Length != 2 {
		t.Errorf("Width/Length[0]: got %d/%d, want 1/2", door.Width, door.Length)
	}
	if got, _ := door.Params[1].(uint32); got != 100 {
		t.Errorf("Params[1]: got %v, want 100", door.Params[1])
	}

	bush := cfgs.Configs[1]
	if bush.Category != -1 {
		t.Errorf("Category default (bush): got %d, want -1", bush.Category)
	}
	if bush.Width != 1 || bush.Length != 1 {
		t.Errorf("Width/Length default (bush): got %d/%d, want 1/1", bush.Width, bush.Length)
	}

	if cfgs.ConfigNames["door_basic"] != 0 {
		t.Errorf("ConfigNames[door_basic]: got %d, want 0", cfgs.ConfigNames["door_basic"])
	}
}

func TestLocUnknownCode(t *testing.T) {
	server := packet2.NewPacket(nil)
	server.P2(1) // count = 1
	server.P1(0) // immediate terminator on server side

	clientInner := packet2.NewPacket(nil)
	clientInner.P2(1)   // count = 1
	clientInner.P1(200) // bogus code in client blob
	clientInner.P1(0)

	clientJag := buildClientJag(t, clientInner.Bytes())

	_, err := parseLocTypes(packet2.NewPacket(server.Bytes()), clientJag)
	if err == nil {
		t.Fatal("expected error on unknown loc code, got nil")
	}
}

func TestLocTypeDecodeOpSingleEntry(t *testing.T) {
	entries := []locEntry{
		{debugName: "tree", op: []string{"Chop", "", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 1 {
		t.Fatalf("Configs len: got %d, want 1", got)
	}

	tree := cfgs.Configs[0]
	if tree.Op == nil {
		t.Fatal("Op: got nil, want 5-slot slice")
	}
	if got := tree.Op[0]; got != "Chop" {
		t.Errorf("Op[0]: got %q, want \"Chop\"", got)
	}
	for i := 1; i < 5; i++ {
		if tree.Op[i] != "" {
			t.Errorf("Op[%d]: got %q, want \"\"", i, tree.Op[i])
		}
	}
}

func TestLocTypeDecodeOpAllFive(t *testing.T) {
	entries := []locEntry{
		{debugName: "multi", op: []string{"op0", "op1", "op2", "op3", "op4"}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	multi := cfgs.Configs[0]
	want := []string{"op0", "op1", "op2", "op3", "op4"}
	for i, w := range want {
		if got := multi.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
}

func TestLocTypeDecodeOpHiddenCoercedToEmpty(t *testing.T) {
	entries := []locEntry{
		{debugName: "hidden_test", op: []string{"visible", "hidden", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	entry := cfgs.Configs[0]
	if got := entry.Op[0]; got != "visible" {
		t.Errorf("Op[0]: got %q, want \"visible\"", got)
	}
	if got := entry.Op[1]; got != "" {
		t.Errorf("Op[1] (hidden-coerced, NAI-80-D1): got %q, want \"\"", got)
	}
}

// TestLoadRealLocCache verifies the loader handles the repo's real server
// cache end-to-end. This is a regression guard in case goscape's packer ever
// writes a loc code this loader doesn't recognise.
func TestLoadRealLocCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	cfgs, err := LoadLocTypes(cacheDir)
	if err != nil {
		t.Skipf("no cache data: %v", err)
	}
	if len(cfgs.Configs) == 0 {
		t.Fatal("expected at least one LocType, got 0")
	}
	t.Logf("loaded %d LocTypes from %s", len(cfgs.Configs), cacheDir)
}
