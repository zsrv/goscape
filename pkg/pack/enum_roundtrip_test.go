package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_EnumRoundTrip_IntInt(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=days_per_month\n")
	// Empty .pack files for the other 13 paramLookups slots so loadParamLookups succeeds.
	for _, p := range []string{"obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "varp", "dbrow", "param", "midi"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
	writeFile(t, filepath.Join(scripts, "test.enum"),
		"[days_per_month]\n"+
			"inputtype=int\n"+
			"outputtype=int\n"+
			"default=30\n"+
			"val=1,31\n"+
			"val=2,28\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadEnumTypes(outDir)
	if err != nil {
		t.Fatalf("LoadEnumTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["days_per_month"]
	if !ok {
		t.Fatalf("days_per_month not found")
	}
	e := cfgs.Configs[id]
	if e.InputType != objtype.ScriptVarTypeInt || e.OutputType != objtype.ScriptVarTypeInt {
		t.Errorf("types: in=%v out=%v", e.InputType, e.OutputType)
	}
	if e.DefaultInt != 30 {
		t.Errorf("DefaultInt = %d, want 30", e.DefaultInt)
	}
	if v, ok := e.Values[1]; !ok || v.(int32) != 31 {
		t.Errorf("Values[1] = %v, want 31", v)
	}
	if v, ok := e.Values[2]; !ok || v.(int32) != 28 {
		t.Errorf("Values[2] = %v, want 28", v)
	}
}
