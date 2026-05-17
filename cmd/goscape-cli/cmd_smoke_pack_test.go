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
// synthetic fixture, the driver runs all 11 stages (no early return)
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
	// smoke surfaces. We DO pin: the driver ran all 11 stages and exit
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

// TestRunSmokePack_TelemetryPopulated pins that telemetry fields appear
// in per-stage log lines (elapsed_ms, files, bytes).
func TestRunSmokePack_TelemetryPopulated(t *testing.T) {
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
		"--log.format", "json",
	}, io.Discard, &stderr)
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}
	for _, field := range []string{`"elapsed_ms"`, `"files"`, `"bytes"`} {
		if !strings.Contains(stderr.String(), field) {
			t.Errorf("stderr missing telemetry field %s; got:\n%s", field, stderr.String())
		}
	}
}

// TestRunSmokePack_SummaryTableShape pins the structural properties of
// the summary table on stdout: header row, one row per stage, a Result
// line, and OK/ERR/SKIP status values only.
func TestRunSmokePack_SummaryTableShape(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	var stdout bytes.Buffer
	code := runSmokePack([]string{
		"--content-dir", dir,
		"--out-dir", outDir,
	}, &stdout, io.Discard)
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}

	out := stdout.String()
	for _, want := range []string{"STAGE", "STATUS", "ELAPSED", "FILES", "BYTES", "PackConfigs", "Maps", "Result:"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q; got:\n%s", want, out)
		}
	}
	// Status column must only contain OK / ERR / SKIP — surface unexpected tokens.
	for _, bad := range []string{"PANIC", "?", "FAIL"} {
		if strings.Contains(out, " "+bad+" ") {
			t.Errorf("stdout contains unexpected status %q; got:\n%s", bad, out)
		}
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

// TestRunSmokePack_AutoOutDirCleanup verifies that when --out-dir is
// empty and --keep is unset, the auto-created out-dir is deleted on exit.
// We discover the path by parsing the stdout "out-dir:" line.
func TestRunSmokePack_AutoOutDirCleanup(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)

	var stdout bytes.Buffer
	code := runSmokePack([]string{"--content-dir", dir}, &stdout, io.Discard)
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}

	path := extractOutDirPath(t, stdout.String())
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("auto out-dir %q should have been deleted; stat err=%v", path, err)
	}
}

// TestRunSmokePack_AutoOutDirKept verifies --keep preserves auto-created out-dir.
func TestRunSmokePack_AutoOutDirKept(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)

	var stdout bytes.Buffer
	code := runSmokePack([]string{"--content-dir", dir, "--keep"}, &stdout, io.Discard)
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}

	path := extractOutDirPath(t, stdout.String())
	defer os.RemoveAll(path)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("auto out-dir %q should be preserved with --keep; stat err=%v", path, err)
	}
}

// TestRunSmokePack_OperatorOutDirPreserved verifies operator-supplied
// --out-dir is never deleted, even without --keep.
func TestRunSmokePack_OperatorOutDirPreserved(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	code := runSmokePack([]string{
		"--content-dir", dir,
		"--out-dir", outDir,
	}, io.Discard, io.Discard)
	if code != 0 && code != 1 {
		t.Fatalf("runSmokePack returned %d, want 0 or 1", code)
	}
	if info, err := os.Stat(outDir); err != nil || !info.IsDir() {
		t.Errorf("operator out-dir %q should be preserved; stat err=%v", outDir, err)
	}
}

// TestRunSmokePack_StopOnError verifies --stop-on-error causes every
// stage after the first ERR to render as SKIP. We induce a
// non-PackConfigs failure by passing a regular file as --datapack-dir,
// which causes RunServerCompiler (stage 3, after PackConfigs +
// ClientInterface) to fail when loadConfigs tries to read
// <datapack-dir>/server/inv.dat. PackConfigs writes to outDir
// independently, so it still succeeds — exercising the cascade path
// distinct from the PackConfigs special-case SKIPs.
func TestRunSmokePack_StopOnError(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)
	notADir := filepath.Join(dir, "file-not-dir")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write notADir: %v", err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	var stdout bytes.Buffer
	code := runSmokePack([]string{
		"--content-dir", dir,
		"--out-dir", outDir,
		"--datapack-dir", notADir,
		"--stop-on-error",
	}, &stdout, io.Discard)
	if code != 1 {
		t.Fatalf("runSmokePack returned %d, want 1 (induced ERR)", code)
	}
	out := stdout.String()
	// Find the Result line and assert SKIP count > 0. The Result line
	// has the form "Result: N OK, M ERR, K SKIP ..." where a working
	// --stop-on-error must produce K >= 1 (every stage after the
	// RunServerCompiler ERR — 8 downstream stages — should SKIP).
	var resultLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Result:") {
			resultLine = line
			break
		}
	}
	if resultLine == "" {
		t.Fatalf("stdout missing Result line; got:\n%s", out)
	}
	if strings.Contains(resultLine, "0 SKIP") {
		t.Errorf("Result line shows 0 SKIP; --stop-on-error should cascade SKIPs; got: %q\nfull stdout:\n%s", resultLine, out)
	}
	// Cross-check: at least one row in the table must be a non-PackConfigs
	// stage rendered as SKIP. We look for "SKIP" preceded by a stage name
	// other than the result-line context.
	skipRowFound := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Result:") {
			continue
		}
		// Match a stage row by leading non-space then "SKIP" as STATUS column.
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "SKIP" {
			skipRowFound = true
			break
		}
	}
	if !skipRowFound {
		t.Errorf("stdout missing SKIP table row after ERR; got:\n%s", out)
	}
}

// extractOutDirPath scans the summary line of the form
// "out-dir: <path>" (optionally followed by " (kept; --keep)") and
// returns the path. Fails the test if no such line is present.
func extractOutDirPath(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		const prefix = "out-dir:"
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len(prefix):])
		// Strip optional " (kept; --keep)" or " (auto-deleted)" suffix.
		if paren := strings.Index(rest, " ("); paren >= 0 {
			rest = rest[:paren]
		}
		return rest
	}
	t.Fatalf("stdout missing 'out-dir:' line; got:\n%s", stdout)
	return ""
}
