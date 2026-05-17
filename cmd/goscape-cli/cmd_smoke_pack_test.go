package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedSmokeFixture mirrors seedMinimalPackFixture (cmd_pack_test.go) and
// adds synth.pack + anim/base/model.pack so audio/graphics stages don't
// fail their reg.Ensure* lookups. All other stages' src subdirs are
// absent; per NAI-192-D-NO-SRC-NO-OP, those stages no-op cleanly.
func seedSmokeFixture(t *testing.T, dir string) {
	t.Helper()
	// Configs (PackConfigs inputs).
	writeFile(t, filepath.Join(dir, "scripts", "o.obj"), "[bronze_sword]\nname=Bronze sword\n")
	writeFile(t, filepath.Join(dir, "pack", "obj.pack"), "0=bronze_sword\n")
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"), "[proc,helper]\nreturn;\n")
	writeFile(t, filepath.Join(dir, "pack", "script.pack"), "0=[proc,helper]\n")
	writeFile(t, filepath.Join(dir, "scripts", "i.inv"), "[backpack]\n")
	writeFile(t, filepath.Join(dir, "pack", "inv.pack"), "0=backpack\n")
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"), "[npc_hp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), "0=npc_hp\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"), "[shared_xp]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), "0=shared_xp\n")
	writeFile(t, filepath.Join(dir, "scripts", "d.dbtable"), "[records]\n")
	writeFile(t, filepath.Join(dir, "pack", "dbtable.pack"), "0=records\n")
	// Registry inputs for stages that call reg.Ensure*.
	writeFile(t, filepath.Join(dir, "pack", "synth.pack"), "")
	writeFile(t, filepath.Join(dir, "pack", "anim.pack"), "")
	writeFile(t, filepath.Join(dir, "pack", "base.pack"), "")
	writeFile(t, filepath.Join(dir, "pack", "model.pack"), "")
}

// TestRunSmokePack_AllStagesRunBestEffort verifies that against the
// synthetic fixture, the driver runs all 10 stages (no early return)
// and returns 0 if all stages succeed.
func TestRunSmokePack_AllStagesRunBestEffort(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	var stderr bytes.Buffer
	code := runSmokePack([]string{
		"--content-dir", dir,
		"--out-dir", outDir,
	}, io.Discard, &stderr)

	// We don't pin "all stages pass" — that depends on stage-specific
	// behavior against a minimal fixture, which is exactly what the
	// smoke surfaces. We DO pin: the driver ran all 10 stages and exit
	// is 0 or 1 (not panic, not 3).
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}
	// Stage-start log for each stage must appear (one per stage).
	for _, name := range []string{
		"PackConfigs", "ClientInterface", "RunServerCompiler",
		"Title", "Media", "Texture", "Wordenc", "Sound", "Graphics", "Midi", "Maps",
	} {
		if !strings.Contains(stderr.String(), name) {
			t.Errorf("stderr missing stage %q; got:\n%s", name, stderr.String())
		}
	}
}

// TestRunSmokePack_HelpFlagReturns0 pins -h/--help → exit 0 with flag listing on stderr.
func TestRunSmokePack_HelpFlagReturns0(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runSmokePack([]string{arg}, io.Discard, &stderr)
			if code != 0 {
				t.Fatalf("runSmokePack(%q) returned %d, want 0", arg, code)
			}
			if !strings.Contains(stderr.String(), "content-dir") {
				t.Errorf("stderr %q missing flag listing", stderr.String())
			}
		})
	}
}

// TestRunSmokePack_UnknownFlagReturns2 pins flag-parse error → exit 2.
func TestRunSmokePack_UnknownFlagReturns2(t *testing.T) {
	var stderr bytes.Buffer
	code := runSmokePack([]string{"--no-such-flag"}, io.Discard, &stderr)
	if code != 2 {
		t.Fatalf("runSmokePack returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestRunSmokePack_MissingContentDirReturns3 pins required-flag → exit 3.
func TestRunSmokePack_MissingContentDirReturns3(t *testing.T) {
	var stderr bytes.Buffer
	code := runSmokePack(nil, io.Discard, &stderr)
	if code != 3 {
		t.Fatalf("runSmokePack returned %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "content-dir") {
		t.Errorf("stderr %q missing content-dir mention", stderr.String())
	}
}

// TestRunSmokePack_NonExistentContentDirReturns3 pins setup error → exit 3.
func TestRunSmokePack_NonExistentContentDirReturns3(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	var stderr bytes.Buffer
	code := runSmokePack([]string{"--content-dir", missing}, io.Discard, &stderr)
	if code != 3 {
		t.Fatalf("runSmokePack returned %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "content-dir") && !strings.Contains(stderr.String(), missing) {
		t.Errorf("stderr %q missing path or content-dir mention", stderr.String())
	}
}
