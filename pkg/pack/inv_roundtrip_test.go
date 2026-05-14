package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_InvRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=bank\n")
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=egg\n1=bone\n")
	writeFile(t, filepath.Join(scripts, "test.inv"),
		"[bank]\n"+
			"scope=shared\n"+
			"size=2\n"+
			"stackall=yes\n"+
			"stock1=egg,5,100\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadInvTypes(outDir)
	if err != nil {
		t.Fatalf("LoadInvTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["bank"]
	if !ok {
		t.Fatalf("bank not found")
	}
	inv := cfgs.Configs[id]
	if inv.Scope != objtype.InvTypeScopeShared {
		t.Errorf("Scope = %d, want %d", inv.Scope, objtype.InvTypeScopeShared)
	}
	if inv.Size != 2 {
		t.Errorf("Size = %d, want 2", inv.Size)
	}
	if !inv.StackAll {
		t.Errorf("StackAll = false, want true")
	}
	if len(inv.StockObj) != 1 || inv.StockObj[0] != 0 || inv.StockCount[0] != 5 || inv.StockRate[0] != 100 {
		t.Errorf("stock: obj=%v count=%v rate=%v", inv.StockObj, inv.StockCount, inv.StockRate)
	}
}
