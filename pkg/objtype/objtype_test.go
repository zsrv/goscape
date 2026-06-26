package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// TestObjTypeDecodeOpStoredVerbatim pins L32: the decoder stores the op
// string verbatim, including the "hidden" keyword (no coercion to "").
// TS ObjType.ts:226-227 does the same; "hidden" is gated at the op-click
// handler, while OC_OP/P_OPOBJ read it as a present string.
func TestObjTypeDecodeOpStoredVerbatim(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(30)
	pkt.PJStrLF("visible")
	pkt.P1(31)
	pkt.PJStrLF("hidden")
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if got := ot.Op[0]; got != "visible" {
		t.Errorf("Op[0]: got %q, want \"visible\"", got)
	}
	if got := ot.Op[1]; got != "hidden" {
		t.Errorf("Op[1] (verbatim): got %q, want \"hidden\"", got)
	}
}

// TestObjTypeDecodeCode200Rejected pins L32: opcode 200 is not a valid obj
// config code (TS ObjType has no such case; tradeable defaults true and
// code 15 sets false). It must fall through to the unrecognized-code error.
func TestObjTypeDecodeCode200Rejected(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(200)
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err == nil {
		t.Fatal("DecodeType: want error for unrecognized obj code 200, got nil")
	}
}

func TestNewObjTypeOpDefaults(t *testing.T) {
	ot := NewObjType(0)

	if got, want := len(ot.Op), 5; got != want {
		t.Fatalf("len(Op): got %d, want %d", got, want)
	}
	wantOp := []string{"", "", "Take", "", ""}
	for i, w := range wantOp {
		if got := ot.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}

	if got, want := len(ot.IOp), 5; got != want {
		t.Fatalf("len(IOp): got %d, want %d", got, want)
	}
	wantIOp := []string{"", "", "", "", "Drop"}
	for i, w := range wantIOp {
		if got := ot.IOp[i]; got != w {
			t.Errorf("IOp[%d]: got %q, want %q", i, got, w)
		}
	}
}

func TestObjTypeDecodeSilentCachePreservesDefaults(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(0) // terminator only — no codes

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if got := ot.Op[2]; got != "Take" {
		t.Errorf("Op[2] (silent cache): got %q, want \"Take\"", got)
	}
	if got := ot.IOp[4]; got != "Drop" {
		t.Errorf("IOp[4] (silent cache): got %q, want \"Drop\"", got)
	}
}

func TestObjTypeDecodeCode32OverridesDefault(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(32)
	pkt.PJStrLF("Whatever")
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if got := ot.Op[2]; got != "Whatever" {
		t.Errorf("Op[2] (code 32 override): got %q, want \"Whatever\"", got)
	}
	// Non-overridden slots still default.
	if got := ot.IOp[4]; got != "Drop" {
		t.Errorf("IOp[4]: got %q, want \"Drop\"", got)
	}
}

func TestApplyPostDecodeFixupsF2PMembersResetsOpToTakeOnly(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Op[0] = "Wear" // simulates cache code 30 ("Wear")
	ot.IOp[0] = "Examine"

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	wantOp := []string{"", "", "Take", "", ""}
	for i, w := range wantOp {
		if got := ot.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
	wantIOp := []string{"", "", "", "", "Drop"}
	for i, w := range wantIOp {
		if got := ot.IOp[i]; got != w {
			t.Errorf("IOp[%d]: got %q, want %q", i, got, w)
		}
	}
	if ot.Tradeable != false {
		t.Errorf("Tradeable: got %v, want false", ot.Tradeable)
	}
}

func TestApplyPostDecodeFixupsF2PMembersZeroesCategory(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Category = 42 // simulates cache code 94

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Category != -1 {
		t.Errorf("Category: got %d, want -1", ot.Category)
	}
}

func TestApplyPostDecodeFixupsNonF2PMembersUnchanged(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true")

	ot := NewObjType(0)
	ot.Members = true
	ot.Op[0] = "Wear"
	ot.Category = 42

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	// F2P branch must not fire when NODE_MEMBERS=true.
	if ot.Op[0] != "Wear" {
		t.Errorf("Op[0]: got %q, want \"Wear\" (F2P branch must not fire)", ot.Op[0])
	}
	if ot.Category != 42 {
		t.Errorf("Category: got %d, want 42 (F2P branch must not fire)", ot.Category)
	}
}

func TestApplyPostDecodeFixupsDummyItemForcesTradeableFalse(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true") // disable F2P branch

	ot := NewObjType(0)
	ot.DummyItem = 1
	ot.Tradeable = true // simulates cache code 200

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Tradeable != false {
		t.Errorf("Tradeable: got %v, want false (DummyItem != 0)", ot.Tradeable)
	}
}

func TestApplyPostDecodeFixupsDummyItemZeroPreservesTradeable(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true")

	ot := NewObjType(0)
	ot.DummyItem = 0
	ot.Tradeable = true

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Tradeable != true {
		t.Errorf("Tradeable: got %v, want true (DummyItem == 0)", ot.Tradeable)
	}
}

// TestApplyPostDecodeFixupsF2P_OutOfRangeParam_NoPanic pins cfg-onl-2 (a): TS
// ObjType.ts:73 uses ParamType.get(key)?.autodisable — optional-chain silently
// no-ops on lookup miss. goscape's pre-fix code did a raw ptc.Configs[k] slice
// index that PANICS with "index out of range" when k >= len(ptc.Configs).
// Post-fix the loop quietly skips the param, leaving it in place — matching
// TS's miss-is-no-op semantics.
func TestApplyPostDecodeFixupsF2P_OutOfRangeParam_NoPanic(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Params = ParamMap{99: "stays"}

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	// ptc.Configs has length 2 — key 99 is out-of-range.
	ptc := &ParamTypeConfigs{Configs: []*ParamType{
		{AutoDisable: false},
		{AutoDisable: false},
	}}

	// Pre-fix this call panics (index out of range [99]); post-fix returns
	// normally. The test's pass condition is "did not panic"; if it did
	// panic the Go test runner reports it as a FAIL.
	applyPostDecodeFixups(otc, ptc)

	if _, ok := ot.Params[99]; !ok {
		t.Errorf("Params[99]: got dropped, want preserved (TS ?.autodisable miss must no-op, not delete)")
	}
}

// TestApplyPostDecodeFixupsF2P_NilParamTypeSlot_NoPanic pins cfg-onl-2 (b):
// ptc.Configs[k] can be nil for a sparse cache. Pre-fix the .AutoDisable read
// nil-derefs; post-fix the loop quietly skips matching TS's ?.autodisable
// short-circuit on null.
func TestApplyPostDecodeFixupsF2P_NilParamTypeSlot_NoPanic(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Params = ParamMap{1: "stays"}

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	// In-range but nil slot.
	ptc := &ParamTypeConfigs{Configs: []*ParamType{
		{AutoDisable: false},
		nil,
	}}

	applyPostDecodeFixups(otc, ptc)

	if _, ok := ot.Params[1]; !ok {
		t.Errorf("Params[1]: got dropped, want preserved (TS ?.autodisable on nil ParamType must no-op)")
	}
}

// TestApplyPostDecodeFixupsF2P_AutoDisableTrue_DeletesParam control test:
// the existing TS-faithful delete path still fires when ParamType is present
// AND AutoDisable=true. Ensures the new guards are not over-broad.
func TestApplyPostDecodeFixupsF2P_AutoDisableTrue_DeletesParam(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Params = ParamMap{
		0: "drop_me",
		1: "keep_me",
	}

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{Configs: []*ParamType{
		{AutoDisable: true},  // key 0 — should be deleted
		{AutoDisable: false}, // key 1 — should be preserved
	}}

	applyPostDecodeFixups(otc, ptc)

	if _, ok := ot.Params[0]; ok {
		t.Errorf("Params[0]: got preserved, want deleted (AutoDisable=true must fire)")
	}
	if _, ok := ot.Params[1]; !ok {
		t.Errorf("Params[1]: got deleted, want preserved (AutoDisable=false must not fire)")
	}
}

// newTestObjTypeConfigs builds a minimal ObjTypeConfigs from a debugname→id map.
// Used by TestObjTypeConfigs_ByName.
func newTestObjTypeConfigs(ids map[int]string) *ObjTypeConfigs {
	maxID := 0
	for id := range ids {
		if id > maxID {
			maxID = id
		}
	}
	cfg := &ObjTypeConfigs{
		ConfigNames: make(map[string]int, len(ids)),
		Configs:     make([]*ObjType, maxID+1),
	}
	for id, name := range ids {
		ot := NewObjType(id)
		ot.DebugName = name
		cfg.Configs[id] = ot
		cfg.ConfigNames[name] = id
	}
	return cfg
}

// TestObjTypeConfigs_ByName pins (*ObjTypeConfigs).ByName lookup by debugname.
// Mirrors TS ObjType.getByName.
func TestObjTypeConfigs_ByName(t *testing.T) {
	cfg := newTestObjTypeConfigs(map[int]string{
		558:  "mind_rune",
		4151: "abyssal_whip",
	})
	got := cfg.ByName("abyssal_whip")
	if got == nil {
		t.Fatal("ByName(\"abyssal_whip\"): got nil, want id=4151")
	}
	if got.ID != 4151 {
		t.Errorf("got ID=%d, want 4151", got.ID)
	}
}

// TestObjTypeConfigs_ByName_Unknown pins nil return for unknown debugname.
func TestObjTypeConfigs_ByName_Unknown(t *testing.T) {
	cfg := newTestObjTypeConfigs(map[int]string{558: "mind_rune"})
	if got := cfg.ByName("unknown_obj"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestNewObjType_TradeableDefaultsTrue(t *testing.T) {
	ot := NewObjType(123)
	if !ot.Tradeable {
		t.Fatalf("NewObjType(123).Tradeable: got false, want true (TS ObjType.ts:177 class-field default)")
	}
}

func TestObjTypeDecode_Code15FlipsTradeableFalse(t *testing.T) {
	ot := NewObjType(0)
	if !ot.Tradeable {
		t.Fatalf("precondition: NewObjType.Tradeable expected true")
	}
	if err := ot.Decode(15, packet2.NewPacket(nil)); err != nil {
		t.Fatalf("Decode(15): unexpected error: %v", err)
	}
	if ot.Tradeable {
		t.Fatalf("after Decode(15): Tradeable: got true, want false (TS ObjType.ts:211)")
	}
}

func TestLoadObjTypesFromPack(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")

	params, err := LoadParams(cacheDir)
	if err != nil {
		t.Skipf("no cache data (skipping): %v", err)
	}

	objs, err := LoadObjTypes(cacheDir, params)
	if err != nil {
		t.Skipf("LoadObjTypes (cross-revision data/pack?): %v", err)
	}
	if len(objs.Configs) == 0 {
		t.Fatal("expected at least one ObjType, got 0")
	}

	invs, err := LoadInvTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadInvTypes: %v", err)
	}
	if len(invs.Configs) == 0 {
		t.Fatal("expected at least one InvType, got 0")
	}

	if _, ok := invs.ConfigNames["worn"]; !ok {
		t.Error("expected invs.ConfigNames to contain 'worn'")
	}
}
