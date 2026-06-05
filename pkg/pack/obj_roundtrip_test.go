package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackObjRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	// obj.pack is not provided by setupPackRoots (empty there); supply it
	// here along with the matching .obj source (244 invariant).
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=sword\n")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=sword_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=weapon\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=metal\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[damage]\ntype=int\ndefault=0\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.obj"),
		"[sword]\nname=Sword\ncost=10\nparam=damage,42\n")
	// seq.pack and loc.pack have entries that must match source (244 invariant):
	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"), "[idle]\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.loc"), "[table]\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatalf("LoadParamTypes: %v", err)
	}
	objs, err := objtype.LoadObjTypes(outDir, paramTypes)
	if err != nil {
		t.Fatal(err)
	}
	obj := objs.Configs[0]
	if obj.Name != "Sword" {
		t.Errorf("Name: got %q, want \"Sword\"", obj.Name)
	}
	if obj.Cost != 10 {
		t.Errorf("Cost: got %d, want 10", obj.Cost)
	}
	pid := paramTypes.ConfigNames["damage"]
	if v, ok := obj.Params[uint32(pid)]; !ok || v.(uint32) != 42 {
		t.Errorf("Params[damage=%d]: got %v, want uint32(42)", pid, obj.Params)
	}
}
