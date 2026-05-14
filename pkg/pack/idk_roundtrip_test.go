package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackIdkRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "0=man_hair_default\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=hair_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.idk"),
		"[man_hair_default]\ntype=man_hair\nmodel1=hair_model\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	idks, err := objtype.LoadIdkTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	idk := idks.Configs[0]

	// type=man_hair → bodypart 0 (opcode 1)
	if idk.Type != 0 {
		t.Errorf("Type: got %d, want 0", idk.Type)
	}
	// model1=hair_model → Models[0] = 0 (opcode 2; hair_model is the only entry in model.pack)
	if len(idk.Models) == 0 || idk.Models[0] != 0 {
		t.Errorf("Models[0]: got %v, want 0", idk.Models)
	}
}
