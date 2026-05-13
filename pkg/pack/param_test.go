package pack

import (
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
