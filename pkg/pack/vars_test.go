package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarsConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarsConfig("type", "int")
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

func TestParseVarsConfig_UnknownKey(t *testing.T) {
	_, ok, err := parseVarsConfig("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true; want false for unknown key")
	}
}

func TestParseVarsConfig_UnknownType(t *testing.T) {
	_, _, err := parseVarsConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
}

func TestPackVarsConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.vars"),
		"[shared_quest]\ntype=int\n[shared_score]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n1=shared_score\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".vars", nil, parseVarsConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarsConfigs(cfgs, pf, nil)

	// Each entry body: 2 (type opcode+code) + 1 (250 opcode) + 13 (name+LF) + 1 (terminator) = 17 = 0x11
	wantDat := []byte{
		0x00, 0x02,
		0x01, 0x69,
		0xfa, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x5f, 0x71, 0x75, 0x65, 0x73, 0x74, 0x0a,
		0x00,
		0x01, 0x69,
		0xfa, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x5f, 0x73, 0x63, 0x6f, 0x72, 0x65, 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x\nwant % x", pd.Dat.Data, wantDat)
	}
	wantIdx := []byte{0x00, 0x02, 0x00, 0x11, 0x00, 0x11}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x\nwant % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackVarsConfigs_RoundtripThroughObjtypeLoader(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.vars"),
		"[shared_quest]\ntype=int\n[shared_score]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n1=shared_score\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".vars", nil, parseVarsConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "vars", nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := packVarsConfigs(cfgs, pf, nil)

	outDir := filepath.Join(dir, "out")
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pd.Save(filepath.Join(serverDir, "vars.dat"), filepath.Join(serverDir, "vars.idx")); err != nil {
		t.Fatal(err)
	}

	vc, err := objtype.LoadVarsTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vc.Configs) != 2 {
		t.Fatalf("Configs len=%d, want 2", len(vc.Configs))
	}
	if vc.Configs[0].DebugName != "shared_quest" {
		t.Fatalf("Configs[0].DebugName=%q", vc.Configs[0].DebugName)
	}
	if vc.Configs[0].Type != objtype.ScriptVarTypeInt {
		t.Fatalf("Configs[0].Type=%d", vc.Configs[0].Type)
	}
}
