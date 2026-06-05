package pack

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarpConfig_ClientCodeDecimal(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 7 {
		t.Fatalf("v=%v, want 7", v)
	}
}

func TestParseVarpConfig_ClientCodeHex(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "0x42")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 66 {
		t.Fatalf("v=%v, want 66", v)
	}
}

func TestParseVarpConfig_ClientCodeNegative(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "-5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != -5 {
		t.Fatalf("v=%v, want -5", v)
	}
}

func TestParseVarpConfig_ClientCodeNonNumericRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("clientcode", "abc")
	if err == nil {
		t.Fatal("want err for non-numeric clientcode")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_ProtectBoolean(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"yes", true},
		{"no", false},
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	} {
		v, ok, err := parseVarpConfig("protect", tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if !ok {
			t.Fatalf("%s: ok=false", tc.in)
		}
		if v.(bool) != tc.want {
			t.Fatalf("%s: v=%v, want %v", tc.in, v, tc.want)
		}
	}
}

func TestParseVarpConfig_ProtectInvalidRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("protect", "maybe")
	if err == nil {
		t.Fatal("want err for non-boolean protect")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TransmitBoolean(t *testing.T) {
	v, ok, err := parseVarpConfig("transmit", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(bool) != true {
		t.Fatalf("v=%v, want true", v)
	}
}

func TestParseVarpConfig_ScopePerm(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "perm")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopePerm {
		t.Fatalf("v=%v, want VarpScopePerm=%d", v, objtype.VarpScopePerm)
	}
}

func TestParseVarpConfig_ScopeTemp(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "temp")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopeTemp {
		t.Fatalf("v=%v, want VarpScopeTemp=%d", v, objtype.VarpScopeTemp)
	}
}

func TestParseVarpConfig_ScopeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("scope", "global")
	if err == nil {
		t.Fatal("want err for unknown scope")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarpConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarpConfig_TypeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	v, ok, err := parseVarpConfig("not_a_key", "whatever")
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

// Byte-pin reference computation (id=0 = "run" with scope=temp, type=int,
// transmit=yes, clientcode=7; id=1 = empty slot):
//
// Server dat:
//   p2(size=2)                  → 00 02
//   id=0 body:
//     scope opcode + value      → 01 00     (scope=temp=0)
//     type opcode + value       → 02 69     (type=int=105=0x69)
//     transmit opcode (no val)  → 06        (only when value==true)
//     debugname opcode + LFstr  → fa 72 75 6e 0a   ("run" + LF)
//   Next() terminator           → 00
//   id=1 (empty) terminator     → 00
//
// Server idx:
//   p2(size=2)                  → 00 02
//   id=0 entry length 11        → 00 0b     (2+2+1+5 body + 1 terminator)
//   id=1 entry length 1         → 00 01     (terminator only)
//
// Client dat:
//   p2(size=2)                  → 00 02
//   id=0 body:
//     clientcode + p2 value     → 05 00 07
//   Next() terminator           → 00
//   id=1 terminator             → 00
//
// Client idx:
//   p2(size=2)                  → 00 02
//   id=0 entry length 4         → 00 04     (3 body + 1 terminator)
//   id=1 entry length 1         → 00 01

func TestPackVarpConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[run]\nscope=temp\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=run\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	// pf.Max is len(pf.Pack) which excludes empty trailing slots; force
	// Max=2 so the second slot's empty-terminator is emitted (matches
	// TS packVarpConfigs which iterates [0, VarpPack.max)). The cleanest
	// way is to bump pf.Max directly for the test fixture.
	pf.Max = 2

	server, client := packVarpConfigs(cfgs, pf, nil)

	wantServerDat := []byte{
		0x00, 0x02,
		0x01, 0x00,
		0x02, 0x69,
		0x06,
		0xfa, 0x72, 0x75, 0x6e, 0x0a,
		0x00,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}

	wantServerIdx := []byte{0x00, 0x02, 0x00, 0x0b, 0x00, 0x01}
	if !bytes.Equal(server.Idx.Data, wantServerIdx) {
		t.Fatalf("server.idx=% x\nwant % x", server.Idx.Data, wantServerIdx)
	}

	wantClientDat := []byte{
		0x00, 0x02,
		0x05, 0x00, 0x07,
		0x00,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.dat=% x\nwant % x", client.Dat.Data, wantClientDat)
	}

	wantClientIdx := []byte{0x00, 0x02, 0x00, 0x04, 0x00, 0x01}
	if !bytes.Equal(client.Idx.Data, wantClientIdx) {
		t.Fatalf("client.idx=% x\nwant % x", client.Idx.Data, wantClientIdx)
	}
}

func TestPackVarpConfigs_ProtectFalseEmitsOpcode(t *testing.T) {
	// protect=true is the DEFAULT (NewVarPlayerType). TS pack emits
	// opcode 4 ONLY when value==false. Verify the asymmetry.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\nprotect=no\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf, nil)

	// Server dat for id=0 with protect=false:
	//   p2(1)               → 00 01
	//   protect opcode 4    → 04
	//   debugname trailer   → fa 76 0a   ("v" + LF)
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0x04, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}

func TestPackVarpConfigs_ProtectTrueOmitsOpcode(t *testing.T) {
	// protect=true should NOT emit opcode 4. Only the debugname trailer
	// + terminator are written.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\nprotect=yes\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf, nil)

	// Server dat for id=0 with protect=true (no opcode emitted):
	//   p2(1)               → 00 01
	//   debugname trailer   → fa 76 0a
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}

func TestPackVarpConfigs_TransmitFalseOmitsOpcode(t *testing.T) {
	// transmit=false is the DEFAULT. TS pack emits opcode 6 ONLY when
	// value==true. Inverse asymmetry vs protect.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varp"),
		"[v]\ntransmit=no\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=v\n")
	ClearFsCache()

	cfgs, err := ReadTypedConfigs(dir, ".varp", nil, parseVarpConfig, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, _ := packVarpConfigs(cfgs, pf, nil)

	// Server dat for id=0 with transmit=false (no opcode emitted):
	//   p2(1)               → 00 01
	//   debugname trailer   → fa 76 0a
	//   Next() terminator   → 00
	wantServerDat := []byte{0x00, 0x01, 0xfa, 0x76, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}
