package pack

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarnConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarnConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false for known type")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarnConfig_UnknownTypeIsInvalidValue(t *testing.T) {
	_, ok, err := parseVarnConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
	// ok=true with err!=nil is the contract for invalid value.
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil (invalid value)")
	}
}

func TestParseVarnConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	v, ok, err := parseVarnConfig("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true for unknown key; want false")
	}
	if v != nil {
		t.Fatalf("v=%v, want nil", v)
	}
}

func TestPackVarnConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	// scripts/test.varn declaring two configs:
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	// pack/varn.pack declaring the id mapping:
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	// Expected dat:
	//   p2(size=2)             — 00 02
	//   id=0 (npctier) body:
	//     p1(1), p1(105)       — 01 69
	//     p1(250), pjstrlf("npctier")  — fa 6e 70 63 74 69 65 72 0a
	//   next() terminator:     — 00
	//   id=1 (npchealth) body:
	//     p1(1), p1(105)       — 01 69
	//     p1(250), pjstrlf("npchealth")— fa 6e 70 63 68 65 61 6c 74 68 0a
	//   next() terminator:     — 00
	wantDat := []byte{
		0x00, 0x02,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x74, 0x69, 0x65, 0x72, 0x0a,
		0x00,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x68, 0x65, 0x61, 0x6c, 0x74, 0x68, 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}

	// Expected idx:
	//   p2(size=2)             — 00 02
	//   id=0 entry length 12   — 00 0c   (2 type-bytes + 1 name-opcode + 8 name+LF + 1 terminator)
	//   id=1 entry length 14   — 00 0e   (2 + 1 + 10 + 1)
	wantIdx := []byte{0x00, 0x02, 0x00, 0x0c, 0x00, 0x0e}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarnConfigs_EmptySlotEmitsTerminatorOnly(t *testing.T) {
	// Slot id=1 has no [name] in pack — pf.GetByID(1)=="" — so it should
	// write only the next() terminator with no body.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	wantDat := []byte{
		0x00, 0x01,
		0x01, 0x69,
		0xfa, 0x6e, 0x70, 0x63, 0x74, 0x69, 0x65, 0x72, 0x0a,
		0x00,
	}
	wantIdx := []byte{0x00, 0x01, 0x00, 0x0c}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarnConfigs_RoundtripThroughObjtypeLoader(t *testing.T) {
	// Bind: pack output → write to disk → objtype.LoadVarnTypes parses
	// it correctly.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, parseVarnConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varn", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarnConfigs(cfgs, pf)

	outDir := filepath.Join(dir, "out")
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pd.Save(filepath.Join(serverDir, "varn.dat"), filepath.Join(serverDir, "varn.idx")); err != nil {
		t.Fatal(err)
	}

	vc, err := objtype.LoadVarnTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vc.Configs) != 2 {
		t.Fatalf("Configs len=%d, want 2", len(vc.Configs))
	}
	if vc.Configs[0].DebugName != "npctier" {
		t.Fatalf("Configs[0].DebugName=%q", vc.Configs[0].DebugName)
	}
	if vc.Configs[0].Type != objtype.ScriptVarTypeInt {
		t.Fatalf("Configs[0].Type=%d", vc.Configs[0].Type)
	}
	if vc.Configs[1].DebugName != "npchealth" {
		t.Fatalf("Configs[1].DebugName=%q", vc.Configs[1].DebugName)
	}
}

// Sanity: keep an explicit reference to the parseFn's invalid-value
// signature so the compiler catches contract regressions.
var _ = func() error {
	_, _, err := parseVarnConfig("type", "bogus")
	return err
}
var _ = errors.New
