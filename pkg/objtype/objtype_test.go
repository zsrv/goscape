package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestObjTypeDecodeOpHiddenCoercedToEmpty(t *testing.T) {
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
	if got := ot.Op[1]; got != "" {
		t.Errorf("Op[1] (hidden-coerced): got %q, want \"\"", got)
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
