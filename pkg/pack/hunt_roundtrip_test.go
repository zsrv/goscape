package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_HuntRoundTrip(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "h.hunt"),
		"[h_test]\ntype=npc\ncheck_vis=lineofsight\nrate=5\nfind_newmode=opobj2\n")
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=h_test\n")
	writeAllOtherEmptyPacks_NAI198(t, srcDir, "hunt")

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadHuntTypes(outDir)
	if err != nil {
		t.Fatalf("LoadHuntTypes: %v", err)
	}
	if len(cfgs.Configs) != 1 {
		t.Fatalf("got %d hunt configs, want 1", len(cfgs.Configs))
	}
	h := cfgs.Configs[0]
	if h.Type != objtype.HuntModeNpc {
		t.Errorf("Type=%d, want %d", h.Type, objtype.HuntModeNpc)
	}
	if h.CheckVis != objtype.HuntVisLineOfSight {
		t.Errorf("CheckVis=%d, want %d", h.CheckVis, objtype.HuntVisLineOfSight)
	}
	if h.Rate != 5 {
		t.Errorf("Rate=%d, want 5", h.Rate)
	}
	// NAI-198-D-HUNT-OPOBJ2-TS-BUG round-trip: find_newmode=opobj2 in
	// the source resolves to NPCModeOpObj1 (= 27) in the decoded type,
	// NOT NPCModeOpObj2 (= 28).
	if h.FindNewMode != objtype.NPCModeOpObj1 {
		t.Errorf("FindNewMode=%d, want NPCModeOpObj1=%d (TS bug ported per NAI-198-D-HUNT-OPOBJ2-TS-BUG)",
			h.FindNewMode, objtype.NPCModeOpObj1)
	}
}
