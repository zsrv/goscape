package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
// TestPackAll_ThreeStageSmoke fixture (.obj + .rs2 + freshness-gated
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
}

// TestRunPack_HappyPath verifies runPack returns 0 when PackAll succeeds.
// Implicitly covers --datapack-dir empty → --out-dir fallback.
func TestRunPack_HappyPath(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)
	outDir := filepath.Join(dir, "out")

	code := runPack([]string{
		"--src-dir", dir,
		"--out-dir", outDir,
	}, io.Discard)

	if code != 0 {
		t.Fatalf("runPack returned %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "obj.dat")); err != nil {
		t.Errorf("expected pack output missing: %v", err)
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
	outDir := filepath.Join(dir, "out")

	code := runPack([]string{
		"--src-dir", dir,
		"--out-dir", outDir,
	}, io.Discard)

	if code != 1 {
		t.Fatalf("runPack returned %d, want 1", code)
	}
}

// TestRunPack_FlagParseErrorReturns2 verifies runPack returns 2 on
// flag parse failure (unknown flag).
func TestRunPack_FlagParseErrorReturns2(t *testing.T) {
	var stderr bytes.Buffer
	code := runPack([]string{"--no-such-flag"}, &stderr)
	if code != 2 {
		t.Fatalf("runPack returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}
