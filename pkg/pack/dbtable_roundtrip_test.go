package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// writeAllOtherEmptyPacks_NAI198 writes empty .pack stubs for every config
// type that PackConfigs expects, skipping those listed in except. Place the
// populated .pack for the config under test before calling this helper.
func writeAllOtherEmptyPacks_NAI198(t *testing.T, srcDir string, except ...string) {
	t.Helper()
	isExcept := map[string]bool{}
	for _, e := range except {
		isExcept[e] = true
	}
	all := []string{
		"varp", "varn", "vars", "param", "enum", "inv", "mesanim", "struct",
		"dbtable", "dbrow", "loc", "npc", "obj", "seq", "flo", "spotanim", "idk", "hunt",
		"model", "category", "texture", "anim", "interface", "synth",
	}
	for _, p := range all {
		if isExcept[p] {
			continue
		}
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}
}

func TestPackConfigs_DbTableRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// Minimal fixture: one dbtable with two columns, one of them
	// REQUIRED+LIST.
	writeFile(t, filepath.Join(srcDir, "scripts", "t.dbtable"),
		"[t_demo]\ncolumn=hp,int\ncolumn=loot,obj,REQUIRED,LIST\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbtable.pack"), "0=t_demo\n")
	writeAllOtherEmptyPacks_NAI198(t, srcDir, "dbtable")

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadDbTableTypes(outDir)
	if err != nil {
		t.Fatalf("LoadDbTableTypes: %v", err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("got %d dbtable configs, want 1", len(cfgs.Configs))
	}
	dt := cfgs.Configs[0]
	if dt.DebugName != "t_demo" {
		t.Errorf("DebugName=%q, want %q", dt.DebugName, "t_demo")
	}
	if len(dt.ColumnNames) != 2 || dt.ColumnNames[0] != "hp" || dt.ColumnNames[1] != "loot" {
		t.Errorf("ColumnNames=%v, want [hp loot]", dt.ColumnNames)
	}
	if len(dt.Props) != 2 || dt.Props[0] != 0 || dt.Props[1] != (objtype.DbTableFlagRequired|objtype.DbTableFlagList) {
		t.Errorf("Props=%v, want [0 0x06]", dt.Props)
	}
}
