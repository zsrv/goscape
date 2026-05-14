package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseEnumConfig_InputType(t *testing.T) {
	v, ok, err := parseEnumConfig("inputtype", "int")
	if err != nil || !ok {
		t.Fatalf("inputtype: ok=%v err=%v", ok, err)
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("inputtype got %v, want Int", v)
	}
}

func TestParseEnumConfig_OutputType(t *testing.T) {
	v, ok, err := parseEnumConfig("outputtype", "string")
	if err != nil || !ok {
		t.Fatalf("outputtype: ok=%v err=%v", ok, err)
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeString {
		t.Fatalf("outputtype got %v, want String", v)
	}
}

func TestParseEnumConfig_UnknownScriptVarType(t *testing.T) {
	_, ok, err := parseEnumConfig("inputtype", "notatype")
	if !ok {
		t.Fatalf("inputtype unknown: ok=false, want true")
	}
	if err == nil || !strings.Contains(err.Error(), "notatype") {
		t.Fatalf("inputtype unknown: err=%v", err)
	}
}

func TestParseEnumConfig_DefaultAndVal_PassThrough(t *testing.T) {
	v, ok, err := parseEnumConfig("default", "raw_string_value")
	if err != nil || !ok || v.(string) != "raw_string_value" {
		t.Fatalf("default: ok=%v err=%v v=%v", ok, err, v)
	}
	v, ok, err = parseEnumConfig("val", "1,foo")
	if err != nil || !ok || v.(string) != "1,foo" {
		t.Fatalf("val: ok=%v err=%v v=%v", ok, err, v)
	}
}

func TestParseEnumConfig_UnknownKey(t *testing.T) {
	_, ok, err := parseEnumConfig("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v, want (false, nil)", ok, err)
	}
}

// helper: build a paramLookups with a single enumPF for AUTOINT outputtype tests.
func newEnumLk(t *testing.T) *paramLookups {
	t.Helper()
	return &paramLookups{
		enumPF: newTestPF("enum", map[int]string{0: "first", 1: "second"}),
		objPF:  newTestPF("obj", map[int]string{0: "egg", 1: "bone"}),
	}
}

func TestPackEnumConfigs_IntOutputType_DefaultAndOneVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "test_enum"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"test_enum": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "default", Value: "42"},
			{Key: "val", Value: "7,99"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	// dat: [size=1] [op1 INT] [op2 INT] [op4 p4(42)] [op6 p2(1) p4(7) p4(99)] [op250 pjstr] [Next 0x00]
	want := []byte{
		0x00, 0x01, // size=1
		0x01, 105,  // op1 inputtype=INT(105)
		0x02, 105,  // op2 outputtype=INT(105)
		0x04, 0x00, 0x00, 0x00, 0x2a, // op4 default=p4(42)
		0x06,                         // op6 (non-STRING trailer)
		0x00, 0x01,                   // p2(val count=1)
		0x00, 0x00, 0x00, 0x07,       // p4 key=7
		0x00, 0x00, 0x00, 0x63,       // p4 value=99
		0xfa, 't', 'e', 's', 't', '_', 'e', 'n', 'u', 'm', 0x0a, // op250 + pjstr LF
		0x00, // Next terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat:\n got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_StringOutputType_StringDefaultAndVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeString},
			{Key: "default", Value: "hi"},
			{Key: "val", Value: "1,abc"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x01, 105,
		0x02, 115,              // outputtype=STRING(115)
		0x03, 'h', 'i', 0x0a,  // op3 default=pjstr("hi")
		0x05,                   // op5 (STRING trailer)
		0x00, 0x01,             // count=1
		0x00, 0x00, 0x00, 0x01, // p4 key=1
		'a', 'b', 'c', 0x0a,   // pjstr("abc")
		0xfa, 'e', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_AutoIntInputType_RequiresCommaInVal(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			{Key: "inputtype", Value: objtype.ScriptVarTypeAutoInt},
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
			{Key: "val", Value: "ignored,555"},
		},
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		t.Fatalf("packEnumConfigs: %v", err)
	}
	// AUTOINT inputtype → key = p4(0). outputtype INT (not AUTOINT) → value = p4(555).
	want := []byte{
		0x00, 0x01,
		0x01, 105, // collapsed AUTOINT→INT
		0x02, 105,
		0x06,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, // p4(loop index 0)
		0x00, 0x00, 0x02, 0x2b, // p4(555)
		0xfa, 'e', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackEnumConfigs_MissingInputType_Errors(t *testing.T) {
	pf := newTestPF("enum", map[int]string{0: "e"})
	lk := newEnumLk(t)
	cfgs := map[string][]ConfigLine{
		"e": {
			// inputtype missing
			{Key: "outputtype", Value: objtype.ScriptVarTypeInt},
		},
	}
	_, err := packEnumConfigs(cfgs, pf, lk)
	if err == nil || !strings.Contains(err.Error(), "inputtype") {
		t.Fatalf("missing inputtype: err=%v", err)
	}
}
