package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackSpotAnimRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "0=flame\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=flame_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=flame_anim\n")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.spotanim"),
		"[flame]\nmodel=flame_model\nanim=flame_anim\nangle=180\n")
	// seq.pack has entries that must match source (244 invariant):
	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"), "[flame_anim]\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	spots, err := objtype.LoadSpotanimTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	spot := spots.Configs[0]

	// model=flame_model → model id 0 (the only entry in model.pack)
	if spot.Model != 0 {
		t.Errorf("Model: got %d, want 0", spot.Model)
	}
	// anim=flame_anim → seq id 0 (the only entry in seq.pack)
	if spot.Anim != 0 {
		t.Errorf("Anim: got %d, want 0", spot.Anim)
	}
	// angle=180 → Orientation field (opcode 6)
	if spot.Angle != 180 {
		t.Errorf("Orientation: got %d, want 180", spot.Angle)
	}
}
