package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// TestObjTypeDecodeOpStoredVerbatim pins L32: the decoder stores the op
// string verbatim, including the "hidden" keyword (no coercion to "").
// TS ObjType.ts:228-231 (244): lazy-inits Op on first code-30..34, then sets
// the slot. "hidden" is gated at the op-click handler, while OC_OP/P_OPOBJ
// read it as a present string.
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

	if ot.Op == nil {
		t.Fatal("Op after codes 30/31: got nil, want lazy-inited")
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

// TestNewObjTypeOpDefaults pins TS ObjType.ts:147-148 (244): op and iop class
// fields default to null (nil in Go). The packer lazy-inits them on first
// op/iop cache code (30-34 / 35-39). Items that have no op/iop in the cache
// have nil slices, NOT a pre-populated ["","","Take","",""] / ["","","","","Drop"].
func TestNewObjTypeOpDefaults(t *testing.T) {
	ot := NewObjType(0)

	if ot.Op != nil {
		t.Errorf("Op default: got %v, want nil (TS ObjType.ts:147 op = null)", ot.Op)
	}
	if ot.IOp != nil {
		t.Errorf("IOp default: got %v, want nil (TS ObjType.ts:148 iop = null)", ot.IOp)
	}
}

// TestObjTypeDecodeSilentCachePreservesNilOp pins TS ObjType.ts:147-148 (244):
// an item with no op/iop codes in the cache leaves Op/IOp as nil — there is no
// "Take"/"Drop" default injected at decode time. The nil slice is the correct
// representation of "no operations defined".
func TestObjTypeDecodeSilentCachePreservesNilOp(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(0) // terminator only — no codes

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if ot.Op != nil {
		t.Errorf("Op (silent cache): got %v, want nil (no op codes → no lazy-init)", ot.Op)
	}
	if ot.IOp != nil {
		t.Errorf("IOp (silent cache): got %v, want nil (no iop codes → no lazy-init)", ot.IOp)
	}
}

// TestObjTypeDecodeCode32LazyInit pins the lazy-init path (TS ObjType.ts:228-231):
// code 32 (op[2]) lazy-inits Op to a 5-slot null array and sets the slot.
// IOp remains nil (no iop codes present).
func TestObjTypeDecodeCode32LazyInit(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(32)
	pkt.PJStrLF("Whatever")
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if ot.Op == nil {
		t.Fatal("Op after code 32: got nil, want lazy-inited 5-slot slice")
	}
	if got := ot.Op[2]; got != "Whatever" {
		t.Errorf("Op[2] (code 32): got %q, want \"Whatever\"", got)
	}
	// Other Op slots are empty strings (fill(null) → "" in Go).
	if got := ot.Op[0]; got != "" {
		t.Errorf("Op[0] (unset slot): got %q, want \"\"", got)
	}
	// IOp remains nil because no iop codes appeared.
	if ot.IOp != nil {
		t.Errorf("IOp: got %v, want nil (no iop codes)", ot.IOp)
	}
}

// TestApplyPostDecodeFixupsF2PMembersNilsOpAndIop pins TS ObjType.ts:62-63 (244):
// when NODE_MEMBERS=false and config.members=true, the F2P gating branch sets
// op=null and iop=null (NOT the old [null,null,'Take',null,null] arrays).
// Tradeable is still forced false. Category is NOT touched (244 removed that line).
func TestApplyPostDecodeFixupsF2PMembersNilsOpAndIop(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	// Simulate an item that had op/iop set from cache codes.
	ot.Op = []string{"Wear", "", "", "", ""}
	ot.IOp = []string{"Examine", "", "", "", ""}

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Op != nil {
		t.Errorf("Op: got %v, want nil (TS ObjType.ts:62 config.op = null)", ot.Op)
	}
	if ot.IOp != nil {
		t.Errorf("IOp: got %v, want nil (TS ObjType.ts:63 config.iop = null)", ot.IOp)
	}
	if ot.Tradeable != false {
		t.Errorf("Tradeable: got %v, want false", ot.Tradeable)
	}
}

// TestApplyPostDecodeFixupsF2PMembersCategoryUnchanged pins TS ObjType.ts:59-66 (244):
// the 244 diff removed the `config.category = -1` line from the F2P gating branch.
// Category is now LEFT UNCHANGED when NODE_MEMBERS=false — the old "auto-ignore
// category triggers on f2p" zeroing is gone.
func TestApplyPostDecodeFixupsF2PMembersCategoryUnchanged(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Category = 42 // simulates cache code 94

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	// 244 no longer resets category in the F2P branch — category stays 42.
	if ot.Category != 42 {
		t.Errorf("Category: got %d, want 42 (TS ObjType.ts:244 removed category=-1 from F2P branch)", ot.Category)
	}
}

func TestApplyPostDecodeFixupsNonF2PMembersUnchanged(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true")

	ot := NewObjType(0)
	ot.Members = true
	ot.Op = []string{"Wear", "", "", "", ""}
	ot.Category = 42

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	// F2P branch must not fire when NODE_MEMBERS=true.
	if ot.Op == nil || ot.Op[0] != "Wear" {
		t.Errorf("Op[0]: got %v, want \"Wear\" (F2P branch must not fire)", ot.Op)
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
		t.Fatalf("LoadObjTypes: %v", err)
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

// TestObjTypeDecodeCode110ResizeX pins code-110 decoder (TS ObjType.ts:274-275 @ 244).
// resizex = g2(). Default 128; feed 0x00 0xC8 → 200.
func TestObjTypeDecodeCode110ResizeX(t *testing.T) {
	ot := NewObjType(0)
	if ot.ResizeX != 128 {
		t.Fatalf("default ResizeX: got %d, want 128", ot.ResizeX)
	}
	if err := ot.Decode(110, packet2.NewPacket([]byte{0x00, 0xC8})); err != nil {
		t.Fatalf("Decode(110): %v", err)
	}
	if ot.ResizeX != 200 {
		t.Errorf("ResizeX after 0x00 0xC8: got %d, want 200", ot.ResizeX)
	}
}

// TestObjTypeDecodeCode111ResizeY pins code-111 decoder (TS ObjType.ts:276-277 @ 244).
func TestObjTypeDecodeCode111ResizeY(t *testing.T) {
	ot := NewObjType(0)
	if ot.ResizeY != 128 {
		t.Fatalf("default ResizeY: got %d, want 128", ot.ResizeY)
	}
	if err := ot.Decode(111, packet2.NewPacket([]byte{0x01, 0x00})); err != nil {
		t.Fatalf("Decode(111): %v", err)
	}
	if ot.ResizeY != 256 {
		t.Errorf("ResizeY after 0x01 0x00: got %d, want 256", ot.ResizeY)
	}
}

// TestObjTypeDecodeCode112ResizeZ pins code-112 decoder (TS ObjType.ts:278-279 @ 244).
func TestObjTypeDecodeCode112ResizeZ(t *testing.T) {
	ot := NewObjType(0)
	if ot.ResizeZ != 128 {
		t.Fatalf("default ResizeZ: got %d, want 128", ot.ResizeZ)
	}
	if err := ot.Decode(112, packet2.NewPacket([]byte{0x00, 0x40})); err != nil {
		t.Fatalf("Decode(112): %v", err)
	}
	if ot.ResizeZ != 64 {
		t.Errorf("ResizeZ after 0x00 0x40: got %d, want 64", ot.ResizeZ)
	}
}

// TestObjTypeDecodeCode113AmbientSigned pins code-113 decoder (TS ObjType.ts:280-281 @ 244).
// ambient = g1b() — SIGNED byte. Default 0; 0xFF → -1.
func TestObjTypeDecodeCode113AmbientPositive(t *testing.T) {
	ot := NewObjType(0)
	if ot.Ambient != 0 {
		t.Fatalf("default Ambient: got %d, want 0", ot.Ambient)
	}
	if err := ot.Decode(113, packet2.NewPacket([]byte{0x0A})); err != nil {
		t.Fatalf("Decode(113, 0x0A): %v", err)
	}
	if ot.Ambient != 10 {
		t.Errorf("Ambient after 0x0A: got %d, want 10", ot.Ambient)
	}
}

func TestObjTypeDecodeCode113AmbientSigned(t *testing.T) {
	ot := NewObjType(0)
	if err := ot.Decode(113, packet2.NewPacket([]byte{0xFF})); err != nil {
		t.Fatalf("Decode(113, 0xFF): %v", err)
	}
	if ot.Ambient != -1 {
		t.Errorf("Ambient after 0xFF: got %d, want -1 (signed byte)", ot.Ambient)
	}
}

// TestObjTypeDecodeCode114ContrastSigned pins code-114 decoder (TS ObjType.ts:282-283 @ 244).
// contrast = g1b() — SIGNED byte. Default 0; 0xFF → -1.
func TestObjTypeDecodeCode114ContrastPositive(t *testing.T) {
	ot := NewObjType(0)
	if ot.Contrast != 0 {
		t.Fatalf("default Contrast: got %d, want 0", ot.Contrast)
	}
	if err := ot.Decode(114, packet2.NewPacket([]byte{0x05})); err != nil {
		t.Fatalf("Decode(114, 0x05): %v", err)
	}
	if ot.Contrast != 5 {
		t.Errorf("Contrast after 0x05: got %d, want 5", ot.Contrast)
	}
}

func TestObjTypeDecodeCode114ContrastSigned(t *testing.T) {
	ot := NewObjType(0)
	if err := ot.Decode(114, packet2.NewPacket([]byte{0xFF})); err != nil {
		t.Fatalf("Decode(114, 0xFF): %v", err)
	}
	if ot.Contrast != -1 {
		t.Errorf("Contrast after 0xFF: got %d, want -1 (signed byte)", ot.Contrast)
	}
}

// TestObjTypeDecodeIopLazyInit pins TS ObjType.ts:233-236 (244): code 35 (iop[0])
// lazy-inits IOp to a 5-slot array and sets the slot. Op remains nil.
func TestObjTypeDecodeIopLazyInit(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(39)
	pkt.PJStrLF("Drop")
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if ot.IOp == nil {
		t.Fatal("IOp after code 39: got nil, want lazy-inited 5-slot slice")
	}
	if got := ot.IOp[4]; got != "Drop" {
		t.Errorf("IOp[4] (code 39): got %q, want \"Drop\"", got)
	}
	if ot.Op != nil {
		t.Errorf("Op: got %v, want nil (no op codes)", ot.Op)
	}
}
