package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_StructRoundTrip_ExercisesParamRuntimeLoad(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "varp", "dbrow", "enum"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=goblin_loot\n")

	writeFile(t, filepath.Join(scripts, "params.param"),
		"[damage]\ntype=int\ndefault=10\n",
	)
	writeFile(t, filepath.Join(scripts, "structs.struct"),
		"[goblin_loot]\nparam=damage,99\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	// Verify .param landed (precondition for ParamType.load).
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); err != nil {
		t.Fatalf("param.dat missing: %v", err)
	}

	cfgs, err := objtype.LoadStructTypes(outDir)
	if err != nil {
		t.Fatalf("LoadStructTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["goblin_loot"]
	if !ok {
		t.Fatalf("goblin_loot not found")
	}
	s := cfgs.Configs[id]
	if len(s.Params) != 1 {
		t.Fatalf("Params count = %d, want 1", len(s.Params))
	}
	// Param key is `damage` → param id 0. Value is uint32(99) per DecodeParams.
	v, ok := s.Params[0]
	if !ok {
		t.Fatalf("Params[0] missing")
	}
	if v.(uint32) != 99 {
		t.Errorf("Params[0] = %v, want uint32(99)", v)
	}
}
