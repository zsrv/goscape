package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack"
)

// writeFile is a test helper mirroring the shape used in pkg/pack tests:
// creates parent dirs and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// seedMinimalPackFixture writes the minimum srcDir layout that lets
// pack.PackAll succeed end-to-end. Mirrors pkg/pack/pack_all_test.go
// TestPackAll_TwelveStageSmoke fixture (.obj + .rs2 + freshness-gated
// inv/varn/vars/dbtable).
func seedMinimalPackFixture(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "scripts", "o.obj"),
		"[bronze_sword]\nname=Bronze sword\n")
	writeFile(t, filepath.Join(dir, "pack", "obj.pack"),
		"0=bronze_sword\n")
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")
	writeFile(t, filepath.Join(dir, "pack", "script.pack"),
		"0=[proc,helper]\n")
	writeFile(t, filepath.Join(dir, "scripts", "i.inv"), "[backpack]\n")
	writeFile(t, filepath.Join(dir, "pack", "inv.pack"), "0=backpack\n")
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"), "[npc_hp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=npc_hp\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"), "[shared_xp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "0=shared_xp\n")
	writeFile(t, filepath.Join(dir, "scripts", "d.dbtable"), "[records]\n")
	writeFile(t, filepath.Join(dir, "pack", "dbtable.pack"), "0=records\n")
	// VersionList: versionlist.Pack reads <srcDir>/maps/free2play.csv for the
	// map_index section. Rev-244 B6: required by PackAll pipeline.
	if err := os.MkdirAll(filepath.Join(dir, "maps"), 0o755); err != nil {
		t.Fatalf("MkdirAll maps: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "maps", "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile free2play.csv: %v", err)
	}
}

// seedWorldmapJagStub pre-creates <outDir>/mapview/worldmap.jag so the
// 254 packMaps worldmap gate stays closed for synthetic fixtures:
// TS Pack.js:189 @ 2e3bcf43 seeds rebuildWorldmap = !exists(worldmap.jag)
// and only a map rebuild re-opens it. These fixtures pack no .jm2 maps
// and carry none of the worldmap inputs (flo.dat, sprites, fonts,
// labels.txt), so a worldmap rebuild would fail exactly as the TS
// toolchain would on the same tree.
//
// PSG: the stub alone is no longer sufficient. An output tree with no packer
// format stamp is treated as stale and forces a full rebuild, which reopens
// the worldmap gate. Stamping the current format keeps these fixtures on the
// incremental path they are written to exercise. See pack.FormatVersion.
func seedWorldmapJagStub(t *testing.T, outDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(outDir, "mapview"), 0o755); err != nil {
		t.Fatalf("MkdirAll mapview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "mapview", "worldmap.jag"), []byte{0}, 0o644); err != nil {
		t.Fatalf("WriteFile worldmap.jag stub: %v", err)
	}
	if err := pack.WriteFormatStamp(outDir); err != nil {
		t.Fatalf("WriteFormatStamp: %v", err)
	}
}

// makeRawDir creates a fixture rawDir containing a synthetic wordenc blob.
// Rev-244: wordenc.Pack reads rawDir/wordenc as an opaque blob; any content works.
func makeRawDir(t *testing.T, dir string) string {
	t.Helper()
	rawDir := filepath.Join(dir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("MkdirAll rawDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "wordenc"), []byte{0x01, 0x02, 0x03, 0x04}, 0o644); err != nil {
		t.Fatalf("WriteFile wordenc: %v", err)
	}
	return rawDir
}

// TestRunPack_HappyPath verifies runPack returns 0 when PackAll succeeds.
// Implicitly covers --datapack-dir empty → --out-dir fallback.
// Also pins that the logger writes its output to the injected stderr
// writer (not os.Stdout).
func TestRunPack_HappyPath(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)
	rawDir := makeRawDir(t, dir)
	outDir := filepath.Join(dir, "out")
	seedWorldmapJagStub(t, outDir)

	var stderr bytes.Buffer
	code := runPack([]string{
		"--src-dir", dir,
		"--out-dir", outDir,
		"--raw-dir", rawDir,
	}, io.Discard, &stderr)

	if code != 0 {
		t.Fatalf("runPack returned %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "obj.dat")); err != nil {
		t.Errorf("expected pack output missing: %v", err)
	}
	if !strings.Contains(stderr.String(), "pack succeeded") {
		t.Errorf("stderr %q missing logger output", stderr.String())
	}
}

// TestRunPack_PackAllErrorReturns1 verifies runPack returns 1 when
// pack.PackAll fails. Uses the cross-domain varn/vars name collision
// fixture from pkg/pack/pack_all_test.go.
func TestRunPack_PackAllErrorReturns1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=duplicate_name\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=duplicate_name\n")
	rawDir := makeRawDir(t, dir)
	outDir := filepath.Join(dir, "out")

	code := runPack([]string{
		"--src-dir", dir,
		"--out-dir", outDir,
		"--raw-dir", rawDir,
	}, io.Discard, io.Discard)

	if code != 1 {
		t.Fatalf("runPack returned %d, want 1", code)
	}
}

// TestRunPack_FlagParseErrorReturns2 verifies runPack returns 2 on
// flag parse failure (unknown flag).
func TestRunPack_FlagParseErrorReturns2(t *testing.T) {
	var stderr bytes.Buffer
	code := runPack([]string{"--no-such-flag"}, io.Discard, &stderr)
	if code != 2 {
		t.Fatalf("runPack returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestRunPack_HelpFlagReturns0 verifies runPack returns 0 (not 2) when
// invoked with -h or --help, and writes the flag-set usage to stderr.
func TestRunPack_HelpFlagReturns0(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runPack([]string{arg}, io.Discard, &stderr)
			if code != 0 {
				t.Fatalf("runPack(%q) returned %d, want 0", arg, code)
			}
			if !strings.Contains(stderr.String(), "src-dir") {
				t.Errorf("stderr %q missing pack flag listing", stderr.String())
			}
		})
	}
}
