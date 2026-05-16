package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackAll_ThreeStageSmoke pins NAI-212 spec §7 PackAll test 1.
//
// Drives a fixture with one .obj source + one .rs2 script through the
// full three-stage pipeline. Asserts each stage produced its expected
// artifact:
//   - Stage A (PackConfigs server): <outDir>/server/obj.dat exists.
//   - Stage B (PackConfigs client): <outDir>/client/config jagfile exists.
//   - Stage C (RunServerCompiler): <outDir>/server/script.dat exists.
//
// dataPackDir is passed as outDir so RunServerCompiler reads back the
// cache PackConfigs just wrote.
//
// The fixture seeds the four freshness-gated config types whose .dat
// files are required by compiler.RunServerCompiler's loadConfigs:
// .inv, .varn, .vars, .dbtable. Without at least one source file of
// each type, PackConfigs skips the freshness-gated branch and leaves
// the .dat absent, causing loadConfigs to fail with ErrNotExist.
// The other three loadConfigs entries need no seeding: component
// (LoadComponentTypes is lenient on ErrNotExist), param and varp
// (+ client/config jagfile, always written by PackConfigs).
func TestPackAll_ThreeStageSmoke(t *testing.T) {
	dir := t.TempDir()

	// Minimal .obj fixture mirrors pkg/pack/obj_test.go shape.
	writeFile(t, filepath.Join(dir, "scripts", "o.obj"),
		"[bronze_sword]\nname=Bronze sword\n")
	writeFile(t, filepath.Join(dir, "pack", "obj.pack"),
		"0=bronze_sword\n")

	// Minimal .rs2 script: a single empty proc.
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")
	writeFile(t, filepath.Join(dir, "pack", "script.pack"),
		"0=[proc,helper]\n")

	// Freshness-gated configs required by loadConfigs (inv, varn, vars,
	// dbtable). Each needs at least one source file so GetLatestModified
	// returns > 0 and PackConfigs writes the corresponding .dat.
	writeFile(t, filepath.Join(dir, "scripts", "i.inv"), "[backpack]\n")
	writeFile(t, filepath.Join(dir, "pack", "inv.pack"), "0=backpack\n")
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"), "[npc_hp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=npc_hp\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"), "[shared_xp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "0=shared_xp\n")
	writeFile(t, filepath.Join(dir, "scripts", "d.dbtable"), "[records]\n")
	writeFile(t, filepath.Join(dir, "pack", "dbtable.pack"), "0=records\n")

	outDir := filepath.Join(dir, "out")
	if err := PackAll(dir, outDir, outDir); err != nil {
		t.Fatalf("PackAll: %v", err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "obj.dat"),
		filepath.Join(outDir, "server", "script.dat"),
		filepath.Join(outDir, "client", "config"),
	} {
		if fi, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		} else if fi.Size() == 0 {
			// Header-only files (8 bytes) are acceptable; truly-empty
			// (0 bytes) is not.
			t.Errorf("%s is 0 bytes", p)
		}
	}
}

// TestPackAll_PackConfigsErrorPropagates pins NAI-212 spec §7 PackAll
// test 2: error from a stage is wrapped with the stage name.
//
// We trigger a PackConfigs failure by writing a .varn / .vars name
// collision (cross-domain uniqueness check at pack_configs.go:625
// returns an error). The PackAll wrapper must prefix "PackAll:" or
// "PackConfigs:" so the caller can identify which stage failed.
func TestPackAll_PackConfigsErrorPropagates(t *testing.T) {
	dir := t.TempDir()

	// Cross-domain collision: same name "duplicate_name" registered as
	// both varn and vars triggers checkVarNameUniqueness.
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=duplicate_name\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=duplicate_name\n")

	outDir := filepath.Join(dir, "out")
	err := PackAll(dir, outDir, outDir)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "PackConfigs") {
		t.Errorf("error %q does not name the failing stage (\"PackConfigs\")",
			err.Error())
	}
}
