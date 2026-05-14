package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_DbRowRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "t.dbtable"),
		"[t_demo]\ncolumn=score,int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "r.dbrow"),
		"[r_one]\ntable=t_demo\ndata=score,42\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbtable.pack"), "0=t_demo\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbrow.pack"), "0=r_one\n")
	writeAllOtherEmptyPacks_NAI198(t, srcDir, "dbtable", "dbrow")

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	rcfgs, err := objtype.LoadDbRowTypes(outDir)
	if err != nil {
		t.Fatalf("LoadDbRowTypes: %v", err)
	}
	if len(rcfgs.Configs) != 1 {
		t.Fatalf("got %d dbrow configs, want 1", len(rcfgs.Configs))
	}
	row := rcfgs.Configs[0]
	if row.DebugName != "r_one" {
		t.Errorf("DebugName=%q, want %q", row.DebugName, "r_one")
	}
	if row.TableID != 0 {
		t.Errorf("TableID=%d, want 0", row.TableID)
	}
	if len(row.IntValues) == 0 || len(row.IntValues[0]) == 0 || row.IntValues[0][0] != 42 {
		t.Errorf("IntValues=%v, want [[42]]", row.IntValues)
	}
}
