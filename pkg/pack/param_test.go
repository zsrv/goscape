package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseParamConfig(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantVal ConfigValue
		wantOK  bool
		wantErr bool
	}{
		{"autodisable yes", "autodisable", "yes", true, true, false},
		{"autodisable no", "autodisable", "no", false, true, false},
		{"autodisable true", "autodisable", "true", true, true, false},
		{"autodisable false", "autodisable", "false", false, true, false},
		{"autodisable invalid", "autodisable", "maybe", nil, true, true},
		{"type int", "type", "int", objtype.ScriptVarTypeInt, true, false},
		{"type loc", "type", "loc", objtype.ScriptVarTypeLoc, true, false},
		{"type string", "type", "string", objtype.ScriptVarTypeString, true, false},
		{"type bogus", "type", "bogus", nil, true, true},
		{"default raw passthrough", "default", "anything", "anything", true, false},
		{"default null", "default", "null", "null", true, false},
		{"unknown key", "unknownkey", "x", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseParamConfig(tt.key, tt.value)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err: got %v, want error=%v", err, tt.wantErr)
			}
			if err == nil && got != tt.wantVal {
				t.Errorf("value: got %#v (%T), want %#v (%T)", got, got, tt.wantVal, tt.wantVal)
			}
		})
	}
}

// newTestPF builds an in-memory PackFile fixture for lookup tests.
// Avoids touching the filesystem.
func newTestPF(packType string, entries map[int]string) *PackFile {
	pack := make(map[int]string, len(entries))
	names := make(map[string]struct{}, len(entries))
	nameToID := make(map[string]int, len(entries))
	maxID := -1
	for id, name := range entries {
		pack[id] = name
		names[name] = struct{}{}
		nameToID[name] = id
		if id > maxID {
			maxID = id
		}
	}
	return &PackFile{
		Type:     packType,
		Pack:     pack,
		Names:    names,
		NameToID: nameToID,
		Max:      maxID + 1,
	}
}

func TestLookupParamValue_NullSentinel(t *testing.T) {
	lk := &paramLookups{}
	got, err := lookupParamValue(objtype.ScriptVarTypeInt, "null", lk)
	if err != nil {
		t.Fatalf("INT null: %v", err)
	}
	if got != int(-1) {
		t.Errorf("INT null: got %#v, want -1", got)
	}
	got, err = lookupParamValue(objtype.ScriptVarTypeString, "null", lk)
	if err != nil {
		t.Fatalf("STRING null: %v", err)
	}
	if got != "" {
		t.Errorf("STRING null: got %#v, want \"\"", got)
	}
}

func TestLookupParamValue_Int(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		val     string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"-5", -5, false},
		{"0xFF", 255, false},
		{"0x10", 16, false},
		{"abc", 0, true},
		{"0xQQ", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeInt, c.val, lk)
		if (err != nil) != c.wantErr {
			t.Errorf("INT %q: err=%v, wantErr=%v", c.val, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("INT %q: got %#v, want %d", c.val, got, c.want)
		}
	}
}

func TestLookupParamValue_String(t *testing.T) {
	lk := &paramLookups{}
	got, err := lookupParamValue(objtype.ScriptVarTypeString, "hello", lk)
	if err != nil {
		t.Fatalf("STRING hello: %v", err)
	}
	if got != "hello" {
		t.Errorf("STRING hello: got %#v, want \"hello\"", got)
	}

	long := strings.Repeat("a", 1001)
	if _, err := lookupParamValue(objtype.ScriptVarTypeString, long, lk); err == nil {
		t.Errorf("STRING %d chars: want error, got nil", len(long))
	}

	// 1000 exactly is accepted.
	at := strings.Repeat("a", 1000)
	if _, err := lookupParamValue(objtype.ScriptVarTypeString, at, lk); err != nil {
		t.Errorf("STRING 1000 chars: unexpected error %v", err)
	}
}

func TestLookupParamValue_Boolean(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		val     string
		want    int
		wantErr bool
	}{
		{"yes", 1, false},
		{"true", 1, false},
		{"1", 1, false},
		{"no", 0, false},
		{"false", 0, false},
		{"0", 0, false},
		{"maybe", 0, true},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeBoolean, c.val, lk)
		if (err != nil) != c.wantErr {
			t.Errorf("BOOL %q: err=%v, wantErr=%v", c.val, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("BOOL %q: got %#v, want %d", c.val, got, c.want)
		}
	}
}

func TestLookupParamValue_Coord(t *testing.T) {
	lk := &paramLookups{}
	// 0_50_50_32_32 → x = 50*64+32 = 3232, z = 50*64+32 = 3232, level = 0
	// pack = z | (x<<14) | (level<<28) = 3232 | (3232<<14) = 3232 | 52953088 = 52956320
	got, err := lookupParamValue(objtype.ScriptVarTypeCoord, "0_50_50_32_32", lk)
	if err != nil {
		t.Fatalf("COORD 0_50_50_32_32: %v", err)
	}
	want := 3232 | (3232 << 14)
	if got != want {
		t.Errorf("COORD 0_50_50_32_32: got %d, want %d", got, want)
	}

	// level=3 high bit set
	got, err = lookupParamValue(objtype.ScriptVarTypeCoord, "3_0_0_0_0", lk)
	if err != nil {
		t.Fatalf("COORD 3_0_0_0_0: %v", err)
	}
	if got != (3 << 28) {
		t.Errorf("COORD 3_0_0_0_0: got %d, want %d", got, 3<<28)
	}

	// Error cases
	errCases := []string{
		"0_50_50_32",       // 4 parts
		"0_50_50_32_32_99", // 6 parts
		"a_b_c_d_e",        // non-numeric
		"4_0_0_0_0",        // level > 3
		"0_256_0_0_0",      // mX > 255
		"0_0_256_0_0",      // mZ > 255
		"0_0_0_64_0",       // lX > 63
		"0_0_0_0_64",       // lZ > 63
		"-1_0_0_0_0",       // level < 0
	}
	for _, c := range errCases {
		if _, err := lookupParamValue(objtype.ScriptVarTypeCoord, c, lk); err == nil {
			t.Errorf("COORD %q: want error, got nil", c)
		}
	}
}

func TestLookupParamValue_TypedIDs(t *testing.T) {
	lk := &paramLookups{
		enumPF:      newTestPF("enum", map[int]string{0: "myenum"}),
		objPF:       newTestPF("obj", map[int]string{7: "myobj"}),
		locPF:       newTestPF("loc", map[int]string{3: "myloc"}),
		interfacePF: newTestPF("interface", map[int]string{42: "myiface"}),
		structPF:    newTestPF("struct", map[int]string{1: "mystruct"}),
		categoryPF:  newTestPF("category", map[int]string{5: "mycat"}),
		spotanimPF:  newTestPF("spotanim", map[int]string{2: "myspot"}),
		npcPF:       newTestPF("npc", map[int]string{99: "mynpc"}),
		invPF:       newTestPF("inv", map[int]string{4: "myinv"}),
		synthPF:     newTestPF("synth", map[int]string{6: "mysynth"}),
		seqPF:       newTestPF("seq", map[int]string{8: "myseq"}),
		varpPF:      newTestPF("varp", map[int]string{0: "myvarp"}),
		dbrowPF:     newTestPF("dbrow", map[int]string{10: "mydbrow"}),
	}
	cases := []struct {
		typ  objtype.ScriptVarType
		name string
		want int
	}{
		{objtype.ScriptVarTypeEnum, "myenum", 0},
		{objtype.ScriptVarTypeObj, "myobj", 7},
		{objtype.ScriptVarTypeNamedObj, "myobj", 7}, // NAMEDOBJ shares ObjPack
		{objtype.ScriptVarTypeLoc, "myloc", 3},
		{objtype.ScriptVarTypeComponent, "myiface", 42}, // COMPONENT → interfacePF
		{objtype.ScriptVarTypeStruct, "mystruct", 1},
		{objtype.ScriptVarTypeCategory, "mycat", 5},
		{objtype.ScriptVarTypeSpotanim, "myspot", 2},
		{objtype.ScriptVarTypeNPC, "mynpc", 99},
		{objtype.ScriptVarTypeInv, "myinv", 4},
		{objtype.ScriptVarTypeSynth, "mysynth", 6},
		{objtype.ScriptVarTypeSeq, "myseq", 8},
		{objtype.ScriptVarTypeVarp, "myvarp", 0},
		{objtype.ScriptVarTypeDbrow, "mydbrow", 10},
	}
	for _, c := range cases {
		got, err := lookupParamValue(c.typ, c.name, lk)
		if err != nil {
			t.Errorf("type=%d name=%q: %v", c.typ, c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("type=%d name=%q: got %#v, want %d", c.typ, c.name, got, c.want)
		}
	}

	// Missing name → error
	if _, err := lookupParamValue(objtype.ScriptVarTypeNPC, "nonexistent", lk); err == nil {
		t.Errorf("NPC nonexistent: want error, got nil")
	}

	// Nil PackFile → error (defensive — TS would crash on undefined.getByName)
	emptyLk := &paramLookups{}
	if _, err := lookupParamValue(objtype.ScriptVarTypeLoc, "myloc", emptyLk); err == nil {
		t.Errorf("LOC with nil locPF: want error, got nil")
	}
}

func TestLookupParamValue_Stat(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		name string
		want int
	}{
		{"attack", 0},
		{"defence", 1},
		{"hitpoints", 3},
		{"runecraft", 20},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeStat, c.name, lk)
		if err != nil {
			t.Errorf("STAT %q: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("STAT %q: got %#v, want %d", c.name, got, c.want)
		}
	}
	if _, err := lookupParamValue(objtype.ScriptVarTypeStat, "fakeskill", lk); err == nil {
		t.Errorf("STAT fakeskill: want error, got nil")
	}
}

func TestLookupParamValue_NpcStat(t *testing.T) {
	lk := &paramLookups{}
	cases := []struct {
		name string
		want int
	}{
		{"hitpoints", 0},
		{"attack", 1},
		{"strength", 2},
		{"defence", 3},
		{"magic", 4},
		{"ranged", 5},
	}
	for _, c := range cases {
		got, err := lookupParamValue(objtype.ScriptVarTypeNpcStat, c.name, lk)
		if err != nil {
			t.Errorf("NPC_STAT %q: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("NPC_STAT %q: got %#v, want %d", c.name, got, c.want)
		}
	}
	// Player-only stat is NOT in npcStats.
	if _, err := lookupParamValue(objtype.ScriptVarTypeNpcStat, "agility", lk); err == nil {
		t.Errorf("NPC_STAT agility: want error, got nil")
	}
}

func TestLookupParamValue_InterfaceColonReject(t *testing.T) {
	lk := &paramLookups{
		interfacePF: newTestPF("interface", map[int]string{42: "myiface"}),
	}
	// No colon → resolves via interfacePF
	got, err := lookupParamValue(objtype.ScriptVarTypeInterface, "myiface", lk)
	if err != nil {
		t.Fatalf("INTERFACE myiface: %v", err)
	}
	if got != 42 {
		t.Errorf("INTERFACE myiface: got %#v, want 42", got)
	}
	// Colon → reject before lookup
	if _, err := lookupParamValue(objtype.ScriptVarTypeInterface, "myiface:component", lk); err == nil {
		t.Errorf("INTERFACE with ':' want error, got nil")
	}
}

func TestLookupParamValue_UnsupportedType(t *testing.T) {
	lk := &paramLookups{}
	// PlayerUid is a valid ScriptVarType but not a default-resolvable param type.
	if _, err := lookupParamValue(objtype.ScriptVarTypePlayerUid, "anything", lk); err == nil {
		t.Errorf("PlayerUid: want unsupported-type error, got nil")
	}
}

// TestPackParamConfigs_IntDefaultAutodisableFalse pins one slot with
// type=int, default=100, autodisable=no (opcode 4 emitted).
func TestPackParamConfigs_IntDefaultAutodisableFalse(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "health_param", 1: ""})
	cfgs := map[string][]ConfigLine{
		"health_param": {
			{Key: "type", Value: objtype.ScriptVarTypeInt},
			{Key: "default", Value: "100"},
			{Key: "autodisable", Value: false},
		},
	}
	server, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}

	// Server dat:
	//   00 02              count header (size=2)
	//   slot 0 body: 01 69 (type=int=105), 02 00 00 00 64 (default p4(100)),
	//                04 (autodisable=false),
	//                fa <"health_param" + LF> (debugname trailer)
	//   slot 0 terminator: 00
	//   slot 1 (empty, no name): 00
	wantServerDat := []byte{
		0x00, 0x02,
		0x01, 0x69, // type=int (105)
		0x02, 0x00, 0x00, 0x00, 0x64, // default=p4(100)
		0x04, // autodisable=false
		0xfa,
		'h', 'e', 'a', 'l', 't', 'h', '_', 'p', 'a', 'r', 'a', 'm', '\n',
		0x00, // slot 0 terminator
		0x00, // slot 1 terminator
	}
	if !bytes.Equal(server.Dat.Data, wantServerDat) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, wantServerDat)
	}

	// Server idx: 00 02 (count) | 00 <slot 0 byte count incl. terminator>
	// | 00 01 (slot 1: terminator only).
	// Slot 0 byte count = 2 (type) + 5 (default p4) + 1 (autodisable=false)
	//                   + 14 (0xfa + 12-byte name + 0x0a LF) + 1 (terminator) = 23 = 0x17
	wantServerIdx := []byte{
		0x00, 0x02,
		0x00, 0x17,
		0x00, 0x01,
	}
	if !bytes.Equal(server.Idx.Data, wantServerIdx) {
		t.Fatalf("server.Idx:\n  got: % x\n  want: % x", server.Idx.Data, wantServerIdx)
	}

	// Client dat: empty content per NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL.
	// 00 02 (count) | 00 (slot 0 terminator) | 00 (slot 1 terminator)
	wantClientDat := []byte{0x00, 0x02, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClientDat) {
		t.Fatalf("client.Dat:\n  got: % x\n  want: % x", client.Dat.Data, wantClientDat)
	}
	wantClientIdx := []byte{0x00, 0x02, 0x00, 0x01, 0x00, 0x01}
	if !bytes.Equal(client.Idx.Data, wantClientIdx) {
		t.Fatalf("client.Idx:\n  got: % x\n  want: % x", client.Idx.Data, wantClientIdx)
	}
}

// TestPackParamConfigs_StringDefault pins the STRING path: opcode 5
// instead of opcode 2; payload is pjstr instead of p4.
func TestPackParamConfigs_StringDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "name_param"})
	cfgs := map[string][]ConfigLine{
		"name_param": {
			{Key: "type", Value: objtype.ScriptVarTypeString},
			{Key: "default", Value: "hello"},
			// no autodisable → default true → no opcode 4
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}

	// Slot 0 body:
	//   01 73                                type=string (115)
	//   05 'h' 'e' 'l' 'l' 'o' 0x0a          default=pjstr("hello")
	//   fa "name_param" 0x0a                 debugname trailer
	//   00                                   slot terminator
	wantSlot0 := []byte{
		0x01, 0x73,
		0x05, 'h', 'e', 'l', 'l', 'o', '\n',
		0xfa, 'n', 'a', 'm', 'e', '_', 'p', 'a', 'r', 'a', 'm', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_CoordDefault pins the COORD path: opcode 2 with
// the packed coord integer.
func TestPackParamConfigs_CoordDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "start"})
	cfgs := map[string][]ConfigLine{
		"start": {
			{Key: "type", Value: objtype.ScriptVarTypeCoord},
			{Key: "default", Value: "0_50_50_32_32"},
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	packed := uint32((3232) | (3232 << 14))
	wantSlot0 := []byte{
		0x01, 0x63, // type=coord (99)
		0x02,
		byte(packed >> 24), byte(packed >> 16), byte(packed >> 8), byte(packed),
		0xfa, 's', 't', 'a', 'r', 't', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_TypedDefaultViaPackFile pins NPC default
// resolution via lookupParamValue + paramIndexOrErr.
func TestPackParamConfigs_TypedDefaultViaPackFile(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "boss"})
	lk := &paramLookups{
		npcPF: newTestPF("npc", map[int]string{42: "kalphite_queen"}),
	}
	cfgs := map[string][]ConfigLine{
		"boss": {
			{Key: "type", Value: objtype.ScriptVarTypeNPC},
			{Key: "default", Value: "kalphite_queen"},
		},
	}
	server, _, err := packParamConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	wantSlot0 := []byte{
		0x01, 0x6e, // type=npc (110)
		0x02, 0x00, 0x00, 0x00, 0x2a, // default=p4(42)
		0xfa, 'b', 'o', 's', 's', '\n',
		0x00,
	}
	want := append([]byte{0x00, 0x01}, wantSlot0...)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server.Dat:\n  got: % x\n  want: % x", server.Dat.Data, want)
	}
}

// TestPackParamConfigs_MissingType errors with a clear message —
// goscape stricter than TS's implicit `!`-assertion crash.
func TestPackParamConfigs_MissingType(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "broken"})
	cfgs := map[string][]ConfigLine{
		"broken": {
			{Key: "default", Value: "42"},
		},
	}
	_, _, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err == nil {
		t.Fatalf("missing type: want error, got nil")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the param: got %v", err)
	}
}

// TestPackParamConfigs_UnknownTypedDefault propagates lookupParamValue
// errors with the param debugname in scope.
func TestPackParamConfigs_UnknownTypedDefault(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "boss"})
	lk := &paramLookups{
		npcPF: newTestPF("npc", map[int]string{42: "kalphite_queen"}),
	}
	cfgs := map[string][]ConfigLine{
		"boss": {
			{Key: "type", Value: objtype.ScriptVarTypeNPC},
			{Key: "default", Value: "nonexistent_npc"},
		},
	}
	_, _, err := packParamConfigs(cfgs, pf, lk)
	if err == nil {
		t.Fatalf("unknown npc default: want error, got nil")
	}
	if !strings.Contains(err.Error(), "boss") {
		t.Errorf("error should name the param: got %v", err)
	}
}

// TestPackParamConfigs_EmptyClientFaithful pins NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL:
// regardless of payload, client.Dat must be exactly count-header + N×0x00.
func TestPackParamConfigs_EmptyClientFaithful(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "a", 1: "b", 2: "c"})
	cfgs := map[string][]ConfigLine{
		"a": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "1"}},
		"b": {{Key: "type", Value: objtype.ScriptVarTypeString}, {Key: "default", Value: "x"}},
		"c": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "99"}},
	}
	_, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	want := []byte{0x00, 0x03, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("empty-client violated: got % x, want % x", client.Dat.Data, want)
	}
}

// TestParamPacker_LoaderRoundTrip binds end-to-end byte-format parity
// through the production loader for 4 primitives + 1 typed-id, plus
// the AutoDisable default-true fix (T1).
func TestParamPacker_LoaderRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scriptsDir := filepath.Join(srcDir, "scripts")
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .param source — 5 slots.
	writeFile(t, filepath.Join(scriptsDir, "test.param"), `[int_p]
type=int
default=42

[str_p]
type=string
default=hello

[bool_p]
type=boolean
default=yes
autodisable=no

[coord_p]
type=coord
default=0_50_50_32_32

[npc_p]
type=npc
default=man
autodisable=yes
`)

	// param.pack — slot order is load-bearing.
	writeFile(t, filepath.Join(packDir, "param.pack"), `0=int_p
1=str_p
2=bool_p
3=coord_p
4=npc_p
`)

	// npc.pack — single entry.
	writeFile(t, filepath.Join(packDir, "npc.pack"), "0=man\n")

	// Stub the other 11 typed packs + var-domain packs.
	for _, kind := range []string{"varp", "varn", "vars", "enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(packDir, kind+".pack"), "")
	}

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	ptc, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatalf("LoadParamTypes: %v", err)
	}
	if got, want := len(ptc.Configs), 5; got != want {
		t.Fatalf("len(Configs): got %d, want %d", got, want)
	}

	// Slot 0: int_p — type=INT, default=42, autodisable default-true.
	c0 := ptc.Configs[0]
	if got, want := c0.DebugName, "int_p"; got != want {
		t.Errorf("c0.DebugName: got %q, want %q", got, want)
	}
	if got, want := c0.Type, objtype.ScriptVarTypeInt; got != want {
		t.Errorf("c0.Type: got %d, want %d", got, want)
	}
	if got, want := c0.DefaultInt, int32(42); got != want {
		t.Errorf("c0.DefaultInt: got %d, want %d", got, want)
	}
	if !c0.AutoDisable {
		t.Errorf("c0.AutoDisable: got false, want true (default-true per T1)")
	}

	// Slot 1: str_p — type=STRING, default="hello", autodisable default-true.
	c1 := ptc.Configs[1]
	if got, want := c1.Type, objtype.ScriptVarTypeString; got != want {
		t.Errorf("c1.Type: got %d, want %d", got, want)
	}
	if got, want := c1.DefaultString, "hello"; got != want {
		t.Errorf("c1.DefaultString: got %q, want %q", got, want)
	}
	if !c1.AutoDisable {
		t.Errorf("c1.AutoDisable: got false, want true")
	}

	// Slot 2: bool_p — type=BOOLEAN, default=yes→1, autodisable=no → AutoDisable=false.
	c2 := ptc.Configs[2]
	if got, want := c2.Type, objtype.ScriptVarTypeBoolean; got != want {
		t.Errorf("c2.Type: got %d, want %d", got, want)
	}
	if got, want := c2.DefaultInt, int32(1); got != want {
		t.Errorf("c2.DefaultInt: got %d, want %d", got, want)
	}
	if c2.AutoDisable {
		t.Errorf("c2.AutoDisable: got true, want false (opcode 4 emitted)")
	}

	// Slot 3: coord_p — type=COORD, default=PackCoord(0, 50*64+32, 50*64+32).
	c3 := ptc.Configs[3]
	if got, want := c3.Type, objtype.ScriptVarTypeCoord; got != want {
		t.Errorf("c3.Type: got %d, want %d", got, want)
	}
	wantCoord := int32((3232) | (3232 << 14))
	if got := c3.DefaultInt; got != wantCoord {
		t.Errorf("c3.DefaultInt: got %d, want %d", got, wantCoord)
	}

	// Slot 4: npc_p — type=NPC, default="man"→0, autodisable=yes → AutoDisable=true.
	c4 := ptc.Configs[4]
	if got, want := c4.Type, objtype.ScriptVarTypeNPC; got != want {
		t.Errorf("c4.Type: got %d, want %d", got, want)
	}
	if got, want := c4.DefaultInt, int32(0); got != want {
		t.Errorf("c4.DefaultInt: got %d, want %d", got, want)
	}
	if !c4.AutoDisable {
		t.Errorf("c4.AutoDisable: got false, want true")
	}
}
