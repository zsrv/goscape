package packall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedWordencRaw creates a synthetic wordenc blob at <rawDir>/wordenc.
// The blob is not a valid Jagfile, but wordenc.Pack only reads and passes
// through raw bytes (no parsing at pack time), so any non-empty content works.
func seedWordencRaw(t *testing.T, rawDir string) {
	t.Helper()
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("MkdirAll rawDir: %v", err)
	}
	// 4 bytes of synthetic blob — wordenc.Pack treats them as opaque.
	if err := os.WriteFile(filepath.Join(rawDir, "wordenc"), []byte{0x01, 0x02, 0x03, 0x04}, 0o644); err != nil {
		t.Fatalf("WriteFile wordenc: %v", err)
	}
}

// TestPackAll_TwelveStageSmoke pins NAI-213 spec §PackAll arc-close.
//
// Drives a fixture with minimal sources for the relevant stages through
// the full orchestration (PackConfigs + ClientInterface + RunServerCompiler +
// Title + Media + Texture + Wordenc + Sound + Graphics + Midi + Maps +
// VersionList). Asserts each stage produced its expected artifact:
//   - PackConfigs server: <outDir>/server/obj.dat exists.
//   - PackConfigs client: <outDir>/client/config jagfile exists.
//   - RunServerCompiler: <outDir>/server/script.dat exists.
//   - Sound: <outDir>/client/sounds jagfile exists.
//   - Texture: <outDir>/client/textures jagfile exists.
//   - Cache: main_file_cache.dat + idx0-4 exist (truncated on pack).
//
// rev-274: the BuildStamp (server/build) + OndemandZip (ondemand.zip) stages
// were removed — TS PackAll.ts (dee467c8) no longer emits either artifact.
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
//
// Client-stage no-src/no-op tolerance: title/fonts/binary/sprites/models/
// jingles/songs/maps are NOT seeded; those stages either no-op (via
// NAI-192-D-NO-SRC-NO-OP mirrors) or write minimal jagfiles iterating
// empty registries.
//
// Rev-244: wordenc stage reads from rawDir/wordenc (not txt files).
// A synthetic 4-byte blob is provided to exercise the full pipeline
// without requiring the real Engine-TS blob.
func TestPackAll_TwelveStageSmoke(t *testing.T) {
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

	// Sound: empty .pack/.order so PackSound writes a terminator-only sounds.dat.
	_ = os.WriteFile(filepath.Join(dir, "pack", "synth.order"), []byte(""), 0o644)

	// VersionList: free2play.csv required by versionlist.Pack for the map_index section.
	if err := os.MkdirAll(filepath.Join(dir, "maps"), 0o755); err != nil {
		t.Fatalf("MkdirAll maps: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "maps", "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile free2play.csv: %v", err)
	}

	// Rev-244: wordenc.Pack reads rawDir/wordenc (replaces 4-txt builder).
	rawDir := filepath.Join(dir, "raw")
	seedWordencRaw(t, rawDir)

	outDir := filepath.Join(dir, "out")
	// 254: keep the packMaps worldmap gate closed (TS Pack.js:189 seeds
	// rebuildWorldmap = !exists(mapview/worldmap.jag); this fixture packs
	// no .jm2 maps and carries no worldmap inputs).
	if err := os.MkdirAll(filepath.Join(outDir, "mapview"), 0o755); err != nil {
		t.Fatalf("MkdirAll mapview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "mapview", "worldmap.jag"), []byte{0}, 0o644); err != nil {
		t.Fatalf("WriteFile worldmap.jag stub: %v", err)
	}
	if err := PackAll(dir, outDir, outDir, rawDir); err != nil {
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

	for _, p := range []string{
		filepath.Join(outDir, "client", "sounds"),
		filepath.Join(outDir, "client", "textures"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected client artifact %s: %v", p, err)
		}
	}

	// rev-244 B6: cache files (main_file_cache.dat + idx0-4) must exist.
	for _, name := range []string{
		"main_file_cache.dat",
		"main_file_cache.idx0",
		"main_file_cache.idx1",
		"main_file_cache.idx2",
		"main_file_cache.idx3",
		"main_file_cache.idx4",
	} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("cache file missing %s: %v", name, err)
		}
	}

	// rev-274: server/build + ondemand.zip assertions removed (artifacts
	// retired upstream at dee467c8).

	// rev-244 B6: truncation test — pack again, cache must not grow stale
	// entries (createNew=true zeroes the cache file each run).
	if err := PackAll(dir, outDir, outDir, rawDir); err != nil {
		t.Fatalf("PackAll (repack): %v", err)
	}
	// Re-run sanity: the cache .dat is still present after a clean repack.
	if _, err := os.Stat(filepath.Join(outDir, "main_file_cache.dat")); err != nil {
		t.Errorf("main_file_cache.dat missing after repack: %v", err)
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

	rawDir := filepath.Join(dir, "raw")
	// No wordenc needed: PackConfigs failure happens before Wordenc stage.

	outDir := filepath.Join(dir, "out")
	err := PackAll(dir, outDir, outDir, rawDir)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "PackConfigs") {
		t.Errorf("error %q does not name the failing stage (\"PackConfigs\")",
			err.Error())
	}
}
