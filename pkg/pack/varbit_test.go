package pack

import (
	"bytes"
	"path/filepath"
	"testing"
)

// newVarbitTestVarpPack builds a varp PackFile with the given names at
// sequential ids for basevar resolution.
func newVarbitTestVarpPack(t *testing.T, names ...string) *PackFile {
	t.Helper()
	dir := t.TempDir()
	var sb bytes.Buffer
	for i, n := range names {
		sb.WriteString(intToStr(i))
		sb.WriteByte('=')
		sb.WriteString(n)
		sb.WriteByte('\n')
	}
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"), sb.String())
	pf, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	return pf
}

func intToStr(i int) string {
	return string(rune('0' + i))
}

func TestParseVarbitConfig_StartbitDecimal(t *testing.T) {
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run"))
	v, ok, err := parse("startbit", "7")
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

func TestParseVarbitConfig_EndbitHex(t *testing.T) {
	// TS VarbitConfig.ts:12-18: 0x-prefixed hex accepted.
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run"))
	v, ok, err := parse("endbit", "0x1f")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 31 {
		t.Fatalf("v=%v, want 31", v)
	}
}

func TestParseVarbitConfig_StartbitNonNumericRejected(t *testing.T) {
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run"))
	_, ok, err := parse("startbit", "abc")
	if err == nil {
		t.Fatal("want err for non-numeric startbit")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarbitConfig_BasevarResolvesVarpName(t *testing.T) {
	// TS VarbitConfig.ts:33-39: basevar resolves through VarpPack.getByName.
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run", "quest_points"))
	v, ok, err := parse("basevar", "quest_points")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 1 {
		t.Fatalf("v=%v, want 1", v)
	}
}

func TestParseVarbitConfig_BasevarUnknownRejected(t *testing.T) {
	// TS VarbitConfig.ts:35-37: getByName -1 → null → invalid value.
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run"))
	_, ok, err := parse("basevar", "nope")
	if err == nil {
		t.Fatal("want err for unknown basevar")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarbitConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	parse := parseVarbitConfigFor(newVarbitTestVarpPack(t, "run"))
	v, ok, err := parse("not_a_key", "whatever")
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

// Byte-pin reference computation (id=0 = "vb" with basevar=run(0),
// startbit=0, endbit=3; id=1 = empty slot):
//
// Client dat (TS VarbitConfig.ts:70-75):
//   p2(size=2)                       → 00 02
//   id=0 body:
//     code 1                         → 01
//     p2(basevar=0)                  → 00 00
//     p1(startbit=0)                 → 00
//     p1(endbit=3)                   → 03
//   Next() terminator                → 00
//   id=1 (empty) terminator          → 00
//
// Client idx:
//   p2(size=2)                       → 00 02
//   id=0 entry length 6              → 00 06   (5 body + 1 terminator)
//   id=1 entry length 1              → 00 01
//
// Server dat (TS VarbitConfig.ts:78-81):
//   p2(size=2)                       → 00 02
//   id=0: code 250 + "vb"+LF         → fa 76 62 0a
//   Next() terminator                → 00
//   id=1 terminator                  → 00
//
// Server idx:
//   p2(size=2)                       → 00 02
//   id=0 entry length 5              → 00 05   (4 body + 1 terminator)
//   id=1 entry length 1              → 00 01

func TestPackVarbitConfigs_BytePin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varbit"),
		"[vb]\nbasevar=run\nstartbit=0\nendbit=3\n")
	writeFile(t, filepath.Join(dir, "pack", "varbit.pack"),
		"0=vb\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=run\n")
	ClearFsCache()

	varpPF, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	cfgs, err := ReadTypedConfigs(dir, ".varbit",
		[]string{"basevar", "startbit", "endbit"},
		parseVarbitConfigFor(varpPF), Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varbit", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Force Max=2 so the second slot's empty-terminator is emitted
	// (matches TS packVarbitConfigs which iterates [0, VarbitPack.max);
	// same fixture trick as TestPackVarpConfigs_BytePin).
	pf.Max = 2

	server, client := packVarbitConfigs(cfgs, pf, nil)

	wantClientDat := []byte{
		0x00, 0x02,
		0x01, 0x00, 0x00, 0x00, 0x03,
		0x00,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.dat=% x\nwant % x", client.Dat.Data, wantClientDat)
	}

	wantClientIdx := []byte{0x00, 0x02, 0x00, 0x06, 0x00, 0x01}
	if !bytes.Equal(client.Idx.Data, wantClientIdx) {
		t.Fatalf("client.idx=% x\nwant % x", client.Idx.Data, wantClientIdx)
	}

	wantServerDat := []byte{
		0x00, 0x02,
		0xfa, 0x76, 0x62, 0x0a,
		0x00,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}

	wantServerIdx := []byte{0x00, 0x02, 0x00, 0x05, 0x00, 0x01}
	if !bytes.Equal(server.Idx.Data, wantServerIdx) {
		t.Fatalf("server.idx=% x\nwant % x", server.Idx.Data, wantServerIdx)
	}
}

func TestPackVarbitConfigs_PartialTripleOmitsClientBody(t *testing.T) {
	// TS VarbitConfig.ts:70: client body requires ALL of
	// basevar/startbit/endbit. With only basevar+startbit declared the
	// client slot is terminator-only; the server trailer still lands.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varbit"),
		"[vb]\nbasevar=run\nstartbit=0\n")
	writeFile(t, filepath.Join(dir, "pack", "varbit.pack"),
		"0=vb\n")
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=run\n")
	ClearFsCache()

	varpPF, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	// No required-properties here: this exercises packVarbitConfigs'
	// own all-three guard, which TS keeps independently of readConfigs'
	// requiredProperties check.
	cfgs, err := ReadTypedConfigs(dir, ".varbit", nil,
		parseVarbitConfigFor(varpPF), Constants{})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := NewPackFile(dir, "varbit", nil)
	if err != nil {
		t.Fatal(err)
	}

	server, client := packVarbitConfigs(cfgs, pf, nil)

	wantClientDat := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.dat=% x\nwant % x", client.Dat.Data, wantClientDat)
	}
	wantServerDat := []byte{0x00, 0x01, 0xfa, 0x76, 0x62, 0x0a, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.dat=% x\nwant % x", server.Dat.Data, wantServerDat)
	}
}

// TestReadTypedConfigs_VarbitRequiredProperties pins the TS readConfigs
// requiredProperties wiring for .varbit: PackShared.ts:618-623 @ 2e3bcf43
// passes ['basevar', 'startbit', 'endbit'] — a block missing any of the
// three is a parse error.
func TestReadTypedConfigs_VarbitRequiredProperties(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varbit"),
		"[vb]\nbasevar=run\nstartbit=0\n") // endbit missing
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"),
		"0=run\n")
	ClearFsCache()

	varpPF, err := NewPackFile(dir, "varp", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReadTypedConfigs(dir, ".varbit",
		[]string{"basevar", "startbit", "endbit"},
		parseVarbitConfigFor(varpPF), Constants{})
	if err == nil {
		t.Fatal("want missing-required-property error for absent endbit")
	}
}
