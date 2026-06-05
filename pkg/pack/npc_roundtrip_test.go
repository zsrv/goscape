package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackNpcRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	// npc.pack is not provided by setupPackRoots (empty there); supply it
	// here along with the matching .npc source (244 invariant).
	writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=rat_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=monster\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=fur\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=aggression\n")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[aggression]\ntype=int\ndefault=0\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.npc"),
		"[rat]\nname=Giant Rat\nsize=2\nhuntmode=default_hunt\nparam=aggression,3\n")
	// seq.pack and loc.pack have entries that must match source (244 invariant):
	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"), "[walk]\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.loc"), "[table]\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	npcs, err := objtype.LoadNPCTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	npc := npcs.Configs[0]
	if npc.Name != "Giant Rat" {
		t.Errorf("Name: got %q, want \"Giant Rat\"", npc.Name)
	}
	if npc.Size != 2 {
		t.Errorf("Size: got %d, want 2", npc.Size)
	}
	if npc.HuntMode != 0 {
		t.Errorf("HuntMode: got %d, want 0 (default_hunt id=0)", npc.HuntMode)
	}
	pid := paramTypes.ConfigNames["aggression"]
	if v, ok := npc.Params[uint32(pid)]; !ok || v.(uint32) != 3 {
		t.Errorf("Params[aggression=%d]: got %v, want uint32(3)", pid, npc.Params)
	}
}
