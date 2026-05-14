package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func newStructParamTypes() *objtype.ParamTypeConfigs {
	intParam := &objtype.ParamType{Type: objtype.ScriptVarTypeInt, DefaultInt: 0}
	intParam.ID = 5
	intParam.DebugName = "myint"
	strParam := &objtype.ParamType{Type: objtype.ScriptVarTypeString, DefaultString: ""}
	strParam.ID = 6
	strParam.DebugName = "mystr"
	return &objtype.ParamTypeConfigs{
		ConfigNames: map[string]int{"myint": 5, "mystr": 6},
		Configs:     []*objtype.ParamType{nil, nil, nil, nil, nil, intParam, strParam},
	}
}

func TestParseStructConfig_IntParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	v, ok, err := parse("param", "myint,42")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	pv := v.(ParamValue)
	if pv.ID != 5 || pv.Type != objtype.ScriptVarTypeInt || pv.Value.(int) != 42 {
		t.Fatalf("got %+v, want {5, Int, 42}", pv)
	}
}

func TestParseStructConfig_StringParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	v, ok, err := parse("param", "mystr,hello world")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	pv := v.(ParamValue)
	if pv.ID != 6 || pv.Type != objtype.ScriptVarTypeString || pv.Value.(string) != "hello world" {
		t.Fatalf("got %+v", pv)
	}
}

func TestParseStructConfig_UnknownParam(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	_, ok, err := parse("param", "doesnotexist,1")
	if !ok {
		t.Fatalf("ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseStructConfig_UnknownKey(t *testing.T) {
	pt := newStructParamTypes()
	parse := parseStructConfigFor(pt, &paramLookups{})

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v", ok, err)
	}
}

func TestPackStructConfigs_IntParam(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {
			{Key: "param", Value: ParamValue{ID: 5, Type: objtype.ScriptVarTypeInt, Value: 42}},
		},
	}
	pd := packStructConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x01,
		0xf9,                   // op249
		0x01,                   // p1(param count=1)
		0x00, 0x00, 0x05,       // p3(id=5)
		0x00,                   // pbool(false) — not STRING
		0x00, 0x00, 0x00, 0x2a, // p4(42)
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackStructConfigs_StringParam(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {
			{Key: "param", Value: ParamValue{ID: 6, Type: objtype.ScriptVarTypeString, Value: "hi"}},
		},
	}
	pd := packStructConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x01,
		0xf9,
		0x01,
		0x00, 0x00, 0x06, // p3(6)
		0x01,             // pbool(true)
		'h', 'i', 0x0a,   // pjstr("hi")
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackStructConfigs_EmptyParamList_NoOp249(t *testing.T) {
	pf := newTestPF("struct", map[int]string{0: "s"})
	cfgs := map[string][]ConfigLine{
		"s": {}, // present block but no params
	}
	pd := packStructConfigs(cfgs, pf)
	// No op249 emitted (TS L89 `if (params.length > 0)`). Just 250-trailer + Next.
	want := []byte{
		0x00, 0x01,
		0xfa, 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}
