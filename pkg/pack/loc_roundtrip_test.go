package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackLocRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=table_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=furniture\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=wood\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=flammable\n")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[flammable]\ntype=int\ndefault=0\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.loc"),
		"[table]\nname=Table\nwidth=2\nlength=3\nparam=flammable,1\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	pid, ok := paramTypes.ConfigNames["flammable"]
	if !ok {
		t.Fatalf("flammable param not registered")
	}

	locs, err := objtype.LoadLocTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	loc := locs.Configs[0]
	if loc.Name != "Table" {
		t.Errorf("Name: got %q, want \"Table\"", loc.Name)
	}
	if loc.Width != 2 {
		t.Errorf("Width: got %d, want 2", loc.Width)
	}
	if loc.Length != 3 {
		t.Errorf("Length: got %d, want 3", loc.Length)
	}
	if v, ok := loc.Params[uint32(pid)]; !ok || v.(uint32) != 1 {
		t.Errorf("Params[flammable=%d]: got %v, want uint32(1)", pid, loc.Params)
	}
}

// setupPackRoots is a shared helper used by all three roundtrip tests.
// Creates scripts/ and pack/ directories, then writes minimal stub .pack
// files for every registry that PackConfigs (and loadParamLookups) touches.
func setupPackRoots(t *testing.T, srcDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	// var-domain trio (required by PackConfigs up-front uniqueness check)
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=login_msg\n")
	// entity packs (required by loadParamLookups and pack*For helpers)
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=sword\n")
	writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=default_hunt\n")
	// remaining loadParamLookups stubs (not provided per-test)
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "interface.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "synth.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "dbrow.pack"), "")
}
