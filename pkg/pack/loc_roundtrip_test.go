package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackLocRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=table_model\n")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=furniture\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=wood\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=flammable\n")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[flammable]\ntype=int\ndefault=0\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "test.loc"),
		"[table]\nname=Table\nwidth=2\nlength=3\nparam=flammable,1\n")
	// seq.pack has entries that must match source (244 invariant):
	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"), "[idle]\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	pid, ok := paramTypes.ConfigNames["flammable"]
	if !ok {
		t.Fatalf("flammable param not registered")
	}

	locs, err := objtype.LoadLocTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	loc := locs.Configs[0]
	if loc.Name != "Table" {
		t.Errorf("Name: got %q, want \"Table\"", loc.Name)
	}
	if loc.Width != 2 {
		t.Errorf("Width: got %d, want 2", loc.Width)
	}
	if loc.Length != 3 {
		t.Errorf("Length: got %d, want 3", loc.Length)
	}
	if v, ok := loc.Params[uint32(pid)]; !ok || v.(uint32) != 1 {
		t.Errorf("Params[flammable=%d]: got %v, want uint32(1)", pid, loc.Params)
	}
}

// setupPackRoots is a shared helper used by all roundtrip tests.
// Creates scripts/ and pack/ directories, then writes minimal stub .pack
// files for every registry that PackConfigs (and loadParamLookups) touches.
//
// Pack entries that are subject to the rev-244 universal config-name
// verification (ValidateConfigPackNames) require a matching source file
// in scripts/. Entries that are NOT needed as cross-reference lookups in
// the roundtrip fixtures are written as empty stubs to avoid 244-invariant
// violations. Per-test overrides (e.g. npc.pack=0=rat in TestPackNpcRoundTrip)
// must write both the pack entry AND a matching source stub.
//
// Freshness-gated packs (varn, vars, hunt) may carry entries safely because
// their packAndSave* branches only fire when GetLatestModified > 0, which
// requires at least one matching source file in scripts/ — absent source
// means the branch is skipped entirely and no validation runs.
func setupPackRoots(t *testing.T, srcDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	// var-domain trio (required by PackConfigs up-front uniqueness check).
	// varp is unconditional → pack must be empty (no source stubs here) so
	// ValidateConfigPackNames has nothing to check. Per-test fixtures that
	// need varp names write both the pack entry and a .varp source stub.
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	// varn/vars are freshness-gated → safe to carry entries without source.
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=login_msg\n")
	// entity packs (loadParamLookups + pack*For helpers).
	// npc and obj are unconditional → empty stubs; per-test overrides must
	// supply both pack entries and matching source stubs.
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "")
	// hunt is freshness-gated → safe to carry entries without source.
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=default_hunt\n")
	// remaining loadParamLookups stubs (not provided per-test)
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "interface.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "synth.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "dbrow.pack"), "")
}
