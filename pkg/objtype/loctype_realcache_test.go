package objtype

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadLocTypes_RealCache_CascadeBlockerLocs pins the NAI-80 fix:
// after the client-jagfile pass is wired, loc ids 3014 (RS Guide door),
// 380 (bookcase), and 350 (drawer) must all have non-empty Op[0].
//
// NAI-79 H4 re-smoke at HEAD 3cc043b captured 3/3 OPLOC1 clicks gating
// at op_slot_empty for these locs because the goscape decoder never
// loaded client/config's loc.dat entry. This test is the regression
// guard against re-introducing that gap.
func TestLoadLocTypes_RealCache_CascadeBlockerLocs(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "config")); err != nil {
		t.Skipf("data/pack/client/config unavailable: %v", err)
	}

	cfgs, err := LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	for _, tc := range []struct {
		id   int
		name string
	}{
		{3014, "RS Guide door"},
		{380, "bookcase"},
		{350, "drawer"},
	} {
		if tc.id >= len(cfgs.Configs) {
			t.Errorf("loc %d (%s): id out of range (configs len=%d)", tc.id, tc.name, len(cfgs.Configs))
			continue
		}
		cfg := cfgs.Configs[tc.id]
		if cfg == nil {
			t.Errorf("loc %d (%s): nil config", tc.id, tc.name)
			continue
		}
		if len(cfg.Op) < 1 || cfg.Op[0] == "" {
			t.Errorf("loc %d (%s): expected Op[0] non-empty (NAI-80 cascade-blocker pin); got DebugName=%q Op=%v",
				tc.id, tc.name, cfg.DebugName, cfg.Op)
		}
	}

	// ID-shift sanity probe: a known-name loc should resolve to its expected Op[0].
	// Implementer-derived probe — see commit body for which loc was pinned.
	if id, ok := cfgs.ConfigNames["oaktree"]; ok {
		cfg := cfgs.Configs[id]
		var got string
		if len(cfg.Op) > 0 {
			got = cfg.Op[0]
		}
		if got != "Chop down" {
			t.Errorf("ID-shift probe: ConfigNames[%q]=%d, Op[0]=%q, want %q",
				"oaktree", id, got, "Chop down")
		}
	} else {
		t.Logf("ID-shift probe: ConfigNames[%q] not found — skipping", "oaktree")
	}
}
