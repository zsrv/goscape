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

// TestPackInvConfigs_StockDense_GapBecomesOneEntry pins the 244 dense behavior:
// stock3=... alone (no stock1/2) packs ONE entry — the N in stockN is ignored,
// push order is file-scan order.
// Previously (225) this produced 3 entries with 2 filler slots (0xffff,0,0).
// TS source: tools/pack/config/InvConfig.ts:115-116 (stock.push(value)) +
// tools/pack/config/InvConfig.ts:148-158 (dense emit, no filler branch).
func TestPackInvConfigs_StockDense_GapBecomesOneEntry(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "shop"})
	cfgs := map[string][]ConfigLine{
		"shop": {
			{Key: "size", Value: 3},
			// Only stock3 declared — 244 dense: emits 1 entry (no fillers for 1/2).
			{Key: "stock3", Value: []int{99, 5}}, // no respawn
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x02, 0x00, 0x03, // size=3
		0x04,                                           // op4 stock block
		0x01,                                           // p1(count=1)
		0x00, 0x63, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, // entry: id=99, count=5, rate=0
		0xfa, 's', 'h', 'o', 'p', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

// TestPackInvConfigs_StockDense_DuplicatesBothEmit pins the 244 behavior:
// duplicate stockN lines both emit as separate dense entries (no error).
// Previously (225) this returned a pack error.
// TS source: tools/pack/config/InvConfig.ts:115-116 (unconditional stock.push).
func TestPackInvConfigs_StockDense_DuplicatesBothEmit(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "stock1", Value: []int{0, 1}},
			{Key: "stock1", Value: []int{0, 2}},
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("duplicate stock should not error in 244: %v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x04,                                           // op4 stock block
		0x02,                                           // p1(count=2)
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, // entry 0: id=0, count=1, rate=0
		0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, // entry 1: id=0, count=2, rate=0
		0xfa, 'i', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

// TestPackInvConfigs_StockDense_NoBoundsCheck pins the 244 behavior:
// stockN with N >= size produces no error.
// Previously (225) this returned a pack error.
// TS source: tools/pack/config/InvConfig.ts:115-116 (no index check, just push).
func TestPackInvConfigs_StockDense_NoBoundsCheck(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "size", Value: 1},
			{Key: "stock2", Value: []int{0, 1}}, // N=2 >= size=1, but no error in 244
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("stockN>=size should not error in 244: %v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x02, 0x00, 0x01, // size=1
		0x04,                                           // op4 stock block
		0x01,                                           // p1(count=1)
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, // entry: id=0, count=1, rate=0
		0xfa, 'i', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

// TestPackInvConfigs_StockDense_DeclarationOrder pins the 244 behavior:
// stock entries emit in file-scan (declaration) order, not index-N order.
// A stock2 line appearing before stock1 emits stock2's value first.
// TS source: tools/pack/config/InvConfig.ts:115-116 (push appends in scan order).
func TestPackInvConfigs_StockDense_DeclarationOrder(t *testing.T) {
	pf := newTestPF("inv", map[int]string{0: "i"})
	cfgs := map[string][]ConfigLine{
		"i": {
			{Key: "stock2", Value: []int{7, 3}}, // appears first in file → emits first
			{Key: "stock1", Value: []int{5, 1}}, // appears second → emits second
		},
	}
	pd, err := packInvConfigs(cfgs, pf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte{
		0x00, 0x01,
		0x04,                                           // op4 stock block
		0x02,                                           // p1(count=2)
		0x00, 0x07, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, // entry 0: id=7, count=3, rate=0 (stock2 first)
		0x00, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, // entry 1: id=5, count=1, rate=0 (stock1 second)
		0xfa, 'i', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}
