package pack

import (
	"bytes"
	"strings"
	"testing"
)

// buildParamLookupsForDbTableTest constructs a paramLookups with empty PackFiles for
// the table types not exercised by the test.
func buildParamLookupsForDbTableTest(t *testing.T) *paramLookups {
	t.Helper()
	lk := &paramLookups{}
	for _, dst := range []**PackFile{
		&lk.enumPF, &lk.objPF, &lk.locPF, &lk.interfacePF, &lk.structPF,
		&lk.categoryPF, &lk.spotanimPF, &lk.npcPF, &lk.invPF, &lk.synthPF,
		&lk.seqPF, &lk.varpPF, &lk.dbrowPF,
	} {
		*dst = newTestPF("dummy", map[int]string{})
	}
	return lk
}

func TestPackDbTableConfigs_EmptyConfigDebugnameOnly(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_empty"})
	configs := map[string][]ConfigLine{
		"t_empty": {},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Layout: 2-byte size header | opcode 250 | "t_empty"\n | 0x00 terminator (Next)
	want := []byte{
		0x00, 0x01, // count header
		250, 't', '_', 'e', 'm', 'p', 't', 'y', 0x0a, // 250 + PJStrLF
		0x00, // Next() terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackDbTableConfigs_SingleColumnNoDefault(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_one"})
	configs := map[string][]ConfigLine{
		"t_one": {
			{Key: "column", Value: "id,int"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1: count=1 | flags=0x00(col0,no-default) | type-count=1 | 'i'(105) | end=255
	// opcode 251: count=1 | "id"\n
	// opcode 252: count=1 | props=0
	// opcode 250: "t_one"\n
	// Next() terminator
	want := []byte{
		0x00, 0x01, // count header
		1, 1, 0x00, 1, 105, 255,
		251, 1, 'i', 'd', 0x0a,
		252, 1, 0,
		250, 't', '_', 'o', 'n', 'e', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackDbTableConfigs_SingleColumnWithIntDefault(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_def"})
	configs := map[string][]ConfigLine{
		"t_def": {
			{Key: "column", Value: "score,int"},
			{Key: "default", Value: "score,42"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1: count=1 | flags=0x80(col0,has-default) | type-count=1 | 'i'(105) |
	//           field-count=1 | P4(42)=0x00,0x00,0x00,0x2a | end=255
	// opcode 251: count=1 | "score"\n
	// opcode 252: count=1 | props=0
	// opcode 250: "t_def"\n
	// Next() terminator
	want := []byte{
		0x00, 0x01, // count header
		1, 1, 0x80, 1, 105, 1, 0, 0, 0, 42, 255,
		251, 1, 's', 'c', 'o', 'r', 'e', 0x0a,
		252, 1, 0,
		250, 't', '_', 'd', 'e', 'f', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackDbTableConfigs_AllPropertyBits(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_props"})
	configs := map[string][]ConfigLine{
		"t_props": {
			{Key: "column", Value: "key,int,INDEXED,REQUIRED,LIST,CLIENTSIDE"},
		},
	}
	pd, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pd.Dat.Data, []byte{252, 1, 0x0F}) {
		t.Fatalf("expected opcode 252 + count=1 + props=0x0F, got % x", pd.Dat.Data)
	}
}

func TestPackDbTableConfigs_IndexedWithoutRequiredErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int,INDEXED"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for INDEXED without REQUIRED, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") || !strings.Contains(err.Error(), "INDEXED") {
		t.Fatalf("err=%q, want substrings 't_bad' and 'INDEXED'", err)
	}
}

func TestPackDbTableConfigs_DefaultOnRequiredErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int,REQUIRED"},
			{Key: "default", Value: "x,7"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for default on REQUIRED, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") || !strings.Contains(err.Error(), "REQUIRED") {
		t.Fatalf("err=%q, want substrings 't_bad' and 'REQUIRED'", err)
	}
}

func TestPackDbTableConfigs_UnknownDefaultColumnErrors(t *testing.T) {
	pf := newTestPF("dbtable", map[int]string{0: "t_bad"})
	configs := map[string][]ConfigLine{
		"t_bad": {
			{Key: "column", Value: "x,int"},
			{Key: "default", Value: "z,7"},
		},
	}
	_, err := packDbTableConfigs(configs, pf, buildParamLookupsForDbTableTest(t))
	if err == nil {
		t.Fatal("want error for unknown default column, got nil")
	}
	if !strings.Contains(err.Error(), "t_bad") {
		t.Fatalf("err=%q, want debugname 't_bad'", err)
	}
}

func TestParseDbTableConfig_AcceptsColumnAndDefault(t *testing.T) {
	v, claimed, err := parseDbTableConfig("column", "id,int")
	if err != nil || !claimed {
		t.Fatalf("column key: v=%v claimed=%v err=%v", v, claimed, err)
	}
	if s, ok := v.(string); !ok || s != "id,int" {
		t.Fatalf("column value=%v, want raw string", v)
	}
	v, claimed, err = parseDbTableConfig("default", "x,42")
	if err != nil || !claimed {
		t.Fatalf("default key: v=%v claimed=%v err=%v", v, claimed, err)
	}
	if s, ok := v.(string); !ok || s != "x,42" {
		t.Fatalf("default value=%v, want raw string", v)
	}
}

func TestParseDbTableConfig_UnknownKey(t *testing.T) {
	v, claimed, err := parseDbTableConfig("foo", "bar")
	if claimed || err != nil || v != nil {
		t.Fatalf("got v=%v claimed=%v err=%v, want all-zero", v, claimed, err)
	}
}

