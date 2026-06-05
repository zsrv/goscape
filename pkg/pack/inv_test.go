package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseInvConfig_Size(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("size", "28")
	if err != nil || !ok || v.(int) != 28 {
		t.Fatalf("size: ok=%v err=%v v=%v", ok, err, v)
	}
}

func TestParseInvConfig_SizeOutOfRange(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	if _, _, err := parse("size", "-1"); err == nil {
		t.Errorf("size=-1: want error")
	}
	if _, _, err := parse("size", "70000"); err == nil {
		t.Errorf("size=70000: want error")
	}
}

func TestParseInvConfig_Scope(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	for in, want := range map[string]int{
		"shared": objtype.InvTypeScopeShared,
		"perm":   objtype.InvTypeScopePerm,
		"temp":   objtype.InvTypeScopeTemp,
	} {
		v, ok, err := parse("scope", in)
		if err != nil || !ok || v.(int) != want {
			t.Errorf("scope=%q: ok=%v err=%v v=%v want %d", in, ok, err, v, want)
		}
	}
	if _, _, err := parse("scope", "bad"); err == nil {
		t.Errorf("scope=bad: want error")
	}
}

func TestParseInvConfig_Booleans(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	for _, key := range []string{"stackall", "restock", "allstock", "protect", "runweight", "dummyinv"} {
		v, ok, err := parse(key, "yes")
		if err != nil || !ok || v.(bool) != true {
			t.Errorf("%s=yes: ok=%v err=%v v=%v", key, ok, err, v)
		}
		v, ok, err = parse(key, "no")
		if err != nil || !ok || v.(bool) != false {
			t.Errorf("%s=no: ok=%v err=%v v=%v", key, ok, err, v)
		}
	}
}

func TestParseInvConfig_Stock_WithoutRespawn(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg", 1: "bone"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("stock1", "bone,5")
	if err != nil || !ok {
		t.Fatalf("stock1: ok=%v err=%v", ok, err)
	}
	parts := v.([]int)
	if len(parts) != 2 || parts[0] != 1 || parts[1] != 5 {
		t.Fatalf("stock1: got %v, want [1, 5]", parts)
	}
}

func TestParseInvConfig_Stock_WithRespawn(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg", 1: "bone"})
	parse := parseInvConfigFor(objPF)

	v, ok, err := parse("stock2", "egg,3,100")
	if err != nil || !ok {
		t.Fatalf("stock2: ok=%v err=%v", ok, err)
	}
	parts := v.([]int)
	if len(parts) != 3 || parts[0] != 0 || parts[1] != 3 || parts[2] != 100 {
		t.Fatalf("stock2: got %v, want [0, 3, 100]", parts)
	}
}

func TestParseInvConfig_Stock_UnknownObj(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	_, ok, err := parse("stock1", "ghost,2")
	if !ok {
		t.Fatalf("stock1: ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("stock1 unknown obj: err=%v", err)
	}
}

func TestParseInvConfig_UnknownKey(t *testing.T) {
	objPF := newTestPF("obj", map[int]string{0: "egg"})
	parse := parseInvConfigFor(objPF)

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v", ok, err)
	}
}

func TestPackInvConfigs_AllOpcodes(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "bank"})
	cfgs := map[string][]ConfigLine{
		"bank": {
			{Key: "scope", Value: objtype.InvTypeScopeShared}, // op1, p1(2)
			{Key: "size", Value: 28},                          // op2, p2(28)
			{Key: "stackall", Value: true},                    // op3
			{Key: "restock", Value: true},                     // op5
			{Key: "allstock", Value: true},                    // op6
			{Key: "protect", Value: false},                    // op7 (only fires on false)
			{Key: "runweight", Value: true},                   // op8
			{Key: "dummyinv", Value: true},                    // op9
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("packInvConfigs: %v", err)
	}
	want := []byte{
		0x00, 0x01, // size=1
		0x01, 0x02, // scope shared (2)
		0x02, 0x00, 0x1c, // size=28
		0x03, // stackall
		0x05, // restock
		0x06, // allstock
		0x07, // protect=false
		0x08, // runweight
		0x09, // dummyinv
		0xfa, 'b', 'a', 'n', 'k', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_ProtectTrueDoesNotEmit(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {{Key: "protect", Value: true}}, // op7 should NOT fire
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := []byte{0x00, 0x01, 0xfa, 'i', 0x0a, 0x00}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_StockListWithHoles(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "shop"})
	cfgs := map[string][]ConfigLine{
		"shop": {
			{Key: "size", Value: 3},
			{Key: "stock1", Value: []int{42, 10, 200}}, // index 0 → present
			// stock2 absent → hole
			{Key: "stock3", Value: []int{99, 5}}, // index 2 → present, no respawn
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x02, 0x00, 0x03, // size=3
		0x04,                                           // op4 stock trailer
		0x03,                                           // p1(stock.length=3)
		0x00, 0x2a, 0x00, 0x0a, 0x00, 0x00, 0x00, 0xc8, // slot 0: 42, 10, 200
		0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // slot 1: hole (-1, 0, 0)
		0x00, 0x63, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, // slot 2: 99, 5, 0
		0xfa, 's', 'h', 'o', 'p', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackInvConfigs_DuplicateStockErrors(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "size", Value: 2},
			{Key: "stock1", Value: []int{0, 1}},
			{Key: "stock1", Value: []int{0, 2}},
		},
	}
	_, err := packInvConfigs(cfgs, pf, nil)
	if err == nil || !strings.Contains(err.Error(), "stock1") {
		t.Fatalf("duplicate stock: err=%v", err)
	}
}

func TestPackInvConfigs_StockBeyondSizeErrors(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "size", Value: 1},
			{Key: "stock2", Value: []int{0, 1}}, // index 1 >= size 1
		},
	}
	_, err := packInvConfigs(cfgs, pf, nil)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("stock beyond size: err=%v", err)
	}
}
