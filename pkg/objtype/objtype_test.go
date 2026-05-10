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
