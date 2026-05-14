package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_MesanimRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	// seq.pack: id→debugname mapping needed by .mesanim parser
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n1=walk\n2=death\n")
	writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=hero_chat\n")
	writeFile(t, filepath.Join(scripts, "test.mesanim"),
		"[hero_chat]\nlen0=walk\nlen2=death\n",
	)

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	cfgs, err := objtype.LoadMesanimTypes(outDir)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	id, ok := cfgs.ConfigNames["hero_chat"]
	if !ok {
		t.Fatalf("hero_chat not found in ConfigNames")
	}
	m := cfgs.Configs[id]
	// len0 → Len[0] = walk id (1); len2 → Len[1] = death id (2); others stay at -1
	if m.Len[0] != 1 {
		t.Errorf("Len[0] = %d, want 1", m.Len[0])
	}
	if m.Len[1] != 2 {
		t.Errorf("Len[1] = %d, want 2", m.Len[1])
	}
}
