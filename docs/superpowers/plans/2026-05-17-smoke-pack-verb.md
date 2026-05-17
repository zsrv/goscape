# `goscape-cli smoke-pack` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `smoke-pack` verb in `cmd/goscape-cli` that runs all 10 `packall.PackAll` stages best-effort against an arbitrary content dir, with per-stage logging and an end-of-run summary table on stdout, so the operator can surface byte-faithfulness gaps against real datapack content in one shot.

**Architecture:** Single-file verb under `cmd/goscape-cli/cmd_smoke_pack.go`, registered in the existing `verbs` slice of `main.go`. The smoke driver duplicates the stage list from `pkg/packall/packall.go` inline (no refactor of `packall`), wraps each call with a timer + recorder, and after the run prints a fixed-width summary table. Synthetic-fixture unit tests live in `cmd_smoke_pack_test.go` and mirror the test patterns of `cmd_pack_test.go`. No production code outside `cmd/goscape-cli/` is touched.

**Tech Stack:** Go 1.26+; stdlib (`flag`, `os`, `time`, `path/filepath`, `text/tabwriter`, `errors`, `log/slog`); existing helpers from `pkg/pack`, `pkg/pack/{clientinterface,compiler,sprites,wordenc,audio,graphics,maps}`, and `pkg/util/log`.

**Reference files (canonical patterns):**
- `cmd/goscape-cli/cmd_pack.go` — flag-set + logger init + exit-code style
- `cmd/goscape-cli/cmd_pack_test.go` — `seedMinimalPackFixture`, table-style flag tests
- `cmd/goscape-cli/main.go` — `verbs` slice + `verbHandler` type
- `pkg/packall/packall.go` — canonical stage order + signatures

---

## File Plan

| File | Action | Responsibility |
|---|---|---|
| `cmd/goscape-cli/cmd_smoke_pack.go` (NEW) | create | `runSmokePack` verb handler: flag parse, out-dir lifecycle, stage driver, summary printer |
| `cmd/goscape-cli/cmd_smoke_pack_test.go` (NEW) | create | flag/exit-code/summary/cleanup tests; reuses `writeFile` from `cmd_pack_test.go` (same package) |
| `cmd/goscape-cli/main.go` (MODIFIED) | edit | add `{"smoke-pack", runSmokePack, "Run all PackAll stages best-effort against a content dir and report per-stage outcomes."}` to `verbs` slice |

---

## Task 1: Verb skeleton — flag parsing, registration, setup-error exit codes

**Files:**
- Create: `cmd/goscape-cli/cmd_smoke_pack.go`
- Create: `cmd/goscape-cli/cmd_smoke_pack_test.go`
- Modify: `cmd/goscape-cli/main.go` (verbs slice)

This task lands the verb registration, flag parsing, and the three setup-error exit paths (missing `--content-dir`, non-existent `--content-dir`, logger-init failure). No stage execution yet; on success the function logs "would run stages" and returns 0.

- [ ] **Step 1: Write failing tests in `cmd/goscape-cli/cmd_smoke_pack_test.go`**

```go
package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

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
```

- [ ] **Step 2: Run tests to verify they fail to compile (undefined `runSmokePack`)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
```

Expected: build error `undefined: runSmokePack`.

- [ ] **Step 3: Implement `cmd/goscape-cli/cmd_smoke_pack.go` skeleton**

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/zsrv/goscape/pkg/util/log"
)

// runSmokePack implements the `smoke-pack` verb: a best-effort driver
// over packall.PackAll's 10 stages with per-stage logging and an
// end-of-run summary table. See
// docs/superpowers/specs/2026-05-17-smoke-pack-verb-design.md for the
// design contract. Stage order mirrors pkg/packall/packall.go.
//
// Exit codes:
//
//	0 — all stages succeeded (or `-h`/`--help`)
//	1 — at least one stage failed (best-effort) or first failing stage
//	    in --stop-on-error mode
//	2 — flag parse error
//	3 — setup error (missing/unreadable --content-dir, logger init, etc.)
func runSmokePack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smoke-pack", flag.ContinueOnError)
	fs.SetOutput(stderr)

	contentDir := fs.String("content-dir", "",
		"Source content directory (required).")
	outDir := fs.String("out-dir", "",
		"Output directory. Empty → auto-create temp dir (deleted on exit unless --keep).")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: effective --out-dir).")
	keep := fs.Bool("keep", false,
		"Preserve auto-created --out-dir on exit.")
	stopOnError := fs.Bool("stop-on-error", false,
		"Exit at the first failing stage (default: log and continue).")

	var logLevel slog.Level = slog.LevelInfo
	fs.TextVar(&logLevel, "log.level", logLevel,
		"Log severity (debug|info|warn|error).")
	logFormat := fs.String("log.format", "text",
		"Log format (text|json).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *contentDir == "" {
		fmt.Fprintln(stderr, "smoke-pack: --content-dir is required")
		return 3
	}
	if info, err := os.Stat(*contentDir); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "smoke-pack: --content-dir %q is not a readable directory\n", *contentDir)
		return 3
	}

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "smoke-pack: logger init failed: %v\n", err)
		return 3
	}

	// Out-dir lifecycle, stage driver, summary printer land in later tasks.
	_ = outDir
	_ = dataPackDir
	_ = keep
	_ = stopOnError
	_ = stdout
	logger.Info("smoke-pack skeleton — stage driver lands in subsequent tasks")
	return 0
}
```

- [ ] **Step 4: Register the verb in `cmd/goscape-cli/main.go`**

Modify the `verbs` slice to add the entry:

```go
var verbs = []verb{
	{"pack", runPack, "Build server-side packs (configs + compiled scripts)."},
	{"compile", runCompile, "Run the runescript compiler on a single .rs2 source file."},
	{"jag", runJag, "Inspect a .jag archive (list | extract | dump)."},
	{"smoke-pack", runSmokePack, "Run all PackAll stages best-effort against a content dir and report per-stage outcomes."},
}
```

- [ ] **Step 5: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all four TestRunSmokePack_* tests PASS. `go vet` clean.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go cmd/goscape-cli/main.go
git commit --no-gpg-sign -m "feat(goscape-cli): scaffold smoke-pack verb (flags + setup-error exit codes)"
```

---

## Task 2: Stage driver — run all 10 stages best-effort, record OK/ERR

**Files:**
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` (add `stageResult`, `stageStatus`, stage runner)
- Modify: `cmd/goscape-cli/cmd_smoke_pack_test.go` (add fixture + best-effort tests)

Drive all 10 stages inline, mirroring `pkg/packall/packall.go`. Continue past per-stage errors in best-effort mode. Return 0 if all stages OK, 1 if any failed. No telemetry, no summary, no out-dir lifecycle yet — operator-supplied `--out-dir` only (an empty `--out-dir` is treated as a setup error returning 3 for now; Task 5 makes it auto-mkdtemp).

- [ ] **Step 1: Write failing tests**

Add to `cmd/goscape-cli/cmd_smoke_pack_test.go`:

```go
import "os" // ensure imported

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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack_AllStagesRunBestEffort -v
```

Expected: FAIL (no stages run yet; stderr won't contain stage names).

- [ ] **Step 3: Implement stage driver in `cmd/goscape-cli/cmd_smoke_pack.go`**

Add types and driver below the existing skeleton. Replace the `_ = outDir`, `_ = dataPackDir`, `_ = keep`, `_ = stopOnError` block with real wiring.

```go
import (
	// ... existing imports plus:
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pack/audio"
	"github.com/zsrv/goscape/pkg/pack/clientinterface"
	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/graphics"
	"github.com/zsrv/goscape/pkg/pack/maps"
	"github.com/zsrv/goscape/pkg/pack/sprites"
	"github.com/zsrv/goscape/pkg/pack/wordenc"
)

type stageStatus int

const (
	stageOK stageStatus = iota
	stageErr
	stageSkip
)

func (s stageStatus) String() string {
	switch s {
	case stageOK:
		return "OK"
	case stageErr:
		return "ERR"
	case stageSkip:
		return "SKIP"
	}
	return "?"
}

type stageResult struct {
	Name   string
	Status stageStatus
	Err    error
}

// runStages drives the 10 PackAll stages best-effort. PackConfigs is
// special: if it fails, all downstream stages render as SKIP because
// they consume the *pack.Registry it produces.
func runStages(srcDir, outDir, dataPackDir string, logger *slog.Logger) []stageResult {
	pack.ClearFsCache()

	results := make([]stageResult, 0, 11)

	logger.Info("stage_start", "stage", "PackConfigs")
	reg, err := pack.PackConfigsForRegistry(srcDir, outDir)
	if err != nil {
		logger.Error("stage_err", "stage", "PackConfigs", "err", err)
		results = append(results, stageResult{Name: "PackConfigs", Status: stageErr, Err: err})
		for _, name := range []string{
			"ClientInterface", "RunServerCompiler", "Title", "Media", "Texture",
			"Wordenc", "Sound", "Graphics", "Midi", "Maps",
		} {
			results = append(results, stageResult{Name: name, Status: stageSkip})
		}
		return results
	}
	logger.Info("stage_done", "stage", "PackConfigs")
	results = append(results, stageResult{Name: "PackConfigs", Status: stageOK})

	type stage struct {
		name string
		run  func() error
	}
	rest := []stage{
		{"ClientInterface", func() error { return clientinterface.Pack(reg, srcDir, outDir) }},
		{"RunServerCompiler", func() error { return compiler.RunServerCompiler(srcDir, outDir, dataPackDir) }},
		{"Title", func() error { return sprites.PackTitle(srcDir, outDir) }},
		{"Media", func() error { return sprites.PackMedia(srcDir, outDir) }},
		{"Texture", func() error { return sprites.PackTexture(reg, srcDir, outDir) }},
		{"Wordenc", func() error { return wordenc.Pack(srcDir, outDir) }},
		{"Sound", func() error { return audio.PackSound(reg, srcDir, outDir) }},
		{"Graphics", func() error { return graphics.Pack(reg, srcDir, outDir) }},
		{"Midi", func() error { return audio.PackMidi(srcDir, outDir) }},
		{"Maps", func() error { return maps.Pack(srcDir, outDir) }},
	}
	for _, st := range rest {
		logger.Info("stage_start", "stage", st.name)
		if err := st.run(); err != nil {
			logger.Error("stage_err", "stage", st.name, "err", err)
			results = append(results, stageResult{Name: st.name, Status: stageErr, Err: err})
			continue
		}
		logger.Info("stage_done", "stage", st.name)
		results = append(results, stageResult{Name: st.name, Status: stageOK})
	}
	return results
}
```

Replace the skeleton tail (the `_ = outDir`, ..., `logger.Info("skeleton...")`, `return 0`) with:

```go
	// Out-dir lifecycle lands in Task 5; for now, --out-dir is required.
	if *outDir == "" {
		fmt.Fprintln(stderr, "smoke-pack: --out-dir is required (auto-create lands in a later task)")
		return 3
	}
	effectiveOut := *outDir
	effectiveDataPack := *dataPackDir
	if effectiveDataPack == "" {
		effectiveDataPack = effectiveOut
	}
	_ = keep
	_ = stopOnError
	_ = stdout

	results := runStages(*contentDir, effectiveOut, effectiveDataPack, logger)

	anyErr := false
	for _, r := range results {
		if r.Status == stageErr {
			anyErr = true
			break
		}
	}
	if anyErr {
		return 1
	}
	return 0
```

- [ ] **Step 4: Update Task 1 setup-error test that now fails**

`TestRunSmokePack_NonExistentContentDirReturns3` and `TestRunSmokePack_MissingContentDirReturns3` should still pass (they short-circuit before the new code). But there is no existing test that exercises an absent `--out-dir`; that path returns 3 now (intentional for this task, removed in Task 5). No test edits needed.

- [ ] **Step 5: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all TestRunSmokePack_* tests PASS. If `TestRunSmokePack_AllStagesRunBestEffort` exits 1, that is acceptable (Some stages may legitimately fail against the minimal fixture); the assertion is `code != 0 && code != 1`.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go
git commit --no-gpg-sign -m "feat(goscape-cli): smoke-pack runs 10 stages best-effort with OK/ERR/SKIP results"
```

---

## Task 3: Per-stage telemetry — elapsed time + cumulative file/byte counts

**Files:**
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` (extend `stageResult`, add walk helper, wire into driver)
- Modify: `cmd/goscape-cli/cmd_smoke_pack_test.go` (add telemetry-assertion test)

After each stage, walk `outDir` once and record cumulative regular-file count + total bytes. Wrap each stage call with a wall-clock timer.

- [ ] **Step 1: Write failing test**

Add to `cmd_smoke_pack_test.go`:

```go
import "time" // ensure imported

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
	// Suppress unused-import warning during fail-first run.
	_ = time.Now()
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack_TelemetryPopulated -v
```

Expected: FAIL — no `elapsed_ms`/`files`/`bytes` in current log output.

- [ ] **Step 3: Extend `stageResult` and add walk helper**

In `cmd_smoke_pack.go`, replace the existing `stageResult` definition with:

```go
import (
	// ... existing imports plus:
	"path/filepath"
	"time"
)

type stageResult struct {
	Name        string
	Status      stageStatus
	Elapsed     time.Duration
	OutputFiles int
	OutputBytes int64
	Err         error
}

// walkOutDir returns (fileCount, totalBytes) for regular files under
// dir. Missing dir → (0, 0, nil). Other errors propagate so the driver
// can log them but still record the stage.
func walkOutDir(dir string) (int, int64, error) {
	var files int
	var bytes int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			files++
			bytes += info.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return files, bytes, err
	}
	return files, bytes, nil
}
```

- [ ] **Step 4: Wire telemetry into `runStages`**

Update each stage path in `runStages` to record elapsed + walk after each call. Replace the existing `runStages` body to use this pattern:

```go
func runStages(srcDir, outDir, dataPackDir string, logger *slog.Logger) []stageResult {
	pack.ClearFsCache()

	results := make([]stageResult, 0, 11)

	// PackConfigs (special — produces the *Registry).
	logger.Info("stage_start", "stage", "PackConfigs")
	pcStart := time.Now()
	reg, pcErr := pack.PackConfigsForRegistry(srcDir, outDir)
	pcElapsed := time.Since(pcStart)
	pcFiles, pcBytes, _ := walkOutDir(outDir)
	if pcErr != nil {
		logger.Error("stage_err", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes, "err", pcErr)
		results = append(results, stageResult{Name: "PackConfigs", Status: stageErr, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes, Err: pcErr})
		for _, name := range []string{
			"ClientInterface", "RunServerCompiler", "Title", "Media", "Texture",
			"Wordenc", "Sound", "Graphics", "Midi", "Maps",
		} {
			results = append(results, stageResult{Name: name, Status: stageSkip})
		}
		return results
	}
	logger.Info("stage_done", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes)
	results = append(results, stageResult{Name: "PackConfigs", Status: stageOK, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes})

	type stage struct {
		name string
		run  func() error
	}
	rest := []stage{
		{"ClientInterface", func() error { return clientinterface.Pack(reg, srcDir, outDir) }},
		{"RunServerCompiler", func() error { return compiler.RunServerCompiler(srcDir, outDir, dataPackDir) }},
		{"Title", func() error { return sprites.PackTitle(srcDir, outDir) }},
		{"Media", func() error { return sprites.PackMedia(srcDir, outDir) }},
		{"Texture", func() error { return sprites.PackTexture(reg, srcDir, outDir) }},
		{"Wordenc", func() error { return wordenc.Pack(srcDir, outDir) }},
		{"Sound", func() error { return audio.PackSound(reg, srcDir, outDir) }},
		{"Graphics", func() error { return graphics.Pack(reg, srcDir, outDir) }},
		{"Midi", func() error { return audio.PackMidi(srcDir, outDir) }},
		{"Maps", func() error { return maps.Pack(srcDir, outDir) }},
	}
	for _, st := range rest {
		logger.Info("stage_start", "stage", st.name)
		start := time.Now()
		err := st.run()
		elapsed := time.Since(start)
		files, bytesSum, _ := walkOutDir(outDir)
		if err != nil {
			logger.Error("stage_err", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum, "err", err)
			results = append(results, stageResult{Name: st.name, Status: stageErr, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum, Err: err})
			continue
		}
		logger.Info("stage_done", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum)
		results = append(results, stageResult{Name: st.name, Status: stageOK, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum})
	}
	return results
}
```

- [ ] **Step 5: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all TestRunSmokePack_* tests PASS, including the new telemetry test.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go
git commit --no-gpg-sign -m "feat(goscape-cli): smoke-pack per-stage elapsed + cumulative file/byte telemetry"
```

---

## Task 4: Summary table renderer — stdout

**Files:**
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` (add `printSummary` + call it before return)
- Modify: `cmd/goscape-cli/cmd_smoke_pack_test.go` (add summary-shape test)

Print a fixed-width table to stdout: one row per stage, plus a result line. The trailing `out-dir:` line lands in Task 5.

- [ ] **Step 1: Write failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack_SummaryTableShape -v
```

Expected: FAIL — stdout currently empty.

- [ ] **Step 3: Add `printSummary` to `cmd_smoke_pack.go`**

```go
import (
	// ... existing imports plus:
	"text/tabwriter"
)

// printSummary renders the per-stage report + result line to w.
// elapsed is the whole-run wall clock for the Result line.
func printSummary(w io.Writer, results []stageResult, elapsed time.Duration) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STAGE\tSTATUS\tELAPSED\tFILES\tBYTES\tERR")
	var ok, errCount, skip int
	for _, r := range results {
		switch r.Status {
		case stageOK:
			ok++
		case stageErr:
			errCount++
		case stageSkip:
			skip++
		}
		elapsedStr := "-"
		filesStr := "-"
		bytesStr := "-"
		errStr := ""
		if r.Status != stageSkip {
			elapsedStr = r.Elapsed.Round(time.Millisecond).String()
			filesStr = fmt.Sprintf("%d", r.OutputFiles)
			bytesStr = fmt.Sprintf("%d", r.OutputBytes)
		}
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Status, elapsedStr, filesStr, bytesStr, errStr)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nResult: %d OK, %d ERR, %d SKIP\ttotal elapsed: %s\n", ok, errCount, skip, elapsed.Round(time.Millisecond))
}
```

- [ ] **Step 4: Call `printSummary` from `runSmokePack`**

Replace the existing `results := runStages(...)` + anyErr loop with:

```go
	runStart := time.Now()
	results := runStages(*contentDir, effectiveOut, effectiveDataPack, logger)
	totalElapsed := time.Since(runStart)
	printSummary(stdout, results, totalElapsed)

	anyErr := false
	for _, r := range results {
		if r.Status == stageErr {
			anyErr = true
			break
		}
	}
	if anyErr {
		return 1
	}
	return 0
```

- [ ] **Step 5: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all TestRunSmokePack_* tests PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go
git commit --no-gpg-sign -m "feat(goscape-cli): smoke-pack stdout summary table (STAGE/STATUS/ELAPSED/FILES/BYTES)"
```

---

## Task 5: Out-dir lifecycle — auto-mkdtemp + --keep

**Files:**
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` (out-dir resolution + cleanup + trailing line)
- Modify: `cmd/goscape-cli/cmd_smoke_pack_test.go` (lifecycle tests)

Resolve `--out-dir`: empty → `os.MkdirTemp("", "goscape-smoke-pack-*")`. If auto-created and `--keep` is false, delete on exit. Append a trailing `out-dir:` line to the summary so the operator knows where output went.

- [ ] **Step 1: Write failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run "TestRunSmokePack_(AutoOutDir|OperatorOutDir)" -v
```

Expected: FAIL — empty `--out-dir` currently returns 3, no out-dir line in stdout.

- [ ] **Step 3: Implement out-dir resolution + cleanup**

Replace the Task-2 placeholder block:

```go
	// Out-dir lifecycle lands in Task 5; for now, --out-dir is required.
	if *outDir == "" {
		fmt.Fprintln(stderr, "smoke-pack: --out-dir is required (auto-create lands in a later task)")
		return 3
	}
	effectiveOut := *outDir
```

with:

```go
	effectiveOut := *outDir
	autoCreated := false
	if effectiveOut == "" {
		tmp, mkErr := os.MkdirTemp("", "goscape-smoke-pack-*")
		if mkErr != nil {
			fmt.Fprintf(stderr, "smoke-pack: mkdtemp failed: %v\n", mkErr)
			return 3
		}
		effectiveOut = tmp
		autoCreated = true
		defer func() {
			if !*keep {
				_ = os.RemoveAll(effectiveOut)
			}
		}()
	}
```

After `printSummary(stdout, results, totalElapsed)` add the trailing out-dir line:

```go
	suffix := ""
	if autoCreated {
		if *keep {
			suffix = " (kept; --keep)"
		} else {
			suffix = " (auto-deleted)"
		}
	}
	fmt.Fprintf(stdout, "out-dir: %s%s\n", effectiveOut, suffix)
```

Note: `(auto-deleted)` is printed BEFORE the deferred `RemoveAll`; the line is operator-facing intent, not a verification of completed cleanup.

- [ ] **Step 4: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all TestRunSmokePack_* tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go
git commit --no-gpg-sign -m "feat(goscape-cli): smoke-pack --out-dir auto-mkdtemp + --keep + out-dir summary line"
```

---

## Task 6: `--stop-on-error` + SKIP rendering for downstream stages

**Files:**
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` (thread `stopOnError` into `runStages`)
- Modify: `cmd/goscape-cli/cmd_smoke_pack_test.go` (induced-failure test)

When `--stop-on-error` is set, the first failing stage causes every subsequent stage to render as `SKIP`. PackConfigs failure already triggers SKIPs in both modes (Task 2).

- [ ] **Step 1: Write failing test**

```go
// TestRunSmokePack_StopOnError verifies --stop-on-error causes every
// stage after the first ERR to render as SKIP. We force a failure by
// pointing --datapack-dir at a path that exists but is unwritable, so
// RunServerCompiler (which depends on the cache dir) fails. We then
// assert that Title, Media, etc. appear as SKIP rows.
//
// Simpler: induce PackConfigs ERR by writing a deliberately-malformed
// pack file that causes PackConfigsForRegistry to error. Then all
// downstream stages SKIP regardless of mode (covered by Task 2 SKIPs).
// For --stop-on-error specifically, induce a NON-PackConfigs failure:
// we delete varn.pack AFTER seedSmokeFixture, then re-seed corrupted
// varn so PackConfigs still succeeds but a later stage diverges.
//
// Pragmatic approach: assert the flag exists and is respected by
// running with --stop-on-error against a corrupted fixture and checking
// for SKIP rows after the first ERR, OR by checking that the exit code
// is 1 (which is the same in both modes) AND that at least one SKIP
// row appears in stdout when an ERR is present.
func TestRunSmokePack_StopOnError(t *testing.T) {
	dir := t.TempDir()
	seedSmokeFixture(t, dir)
	// Corrupt obj.pack so PackConfigs ERRs — produces SKIPs for ALL
	// downstream stages in both modes (PackConfigs-special path).
	// To distinguish --stop-on-error from best-effort, we instead need
	// a fixture where PackConfigs OKs but a later stage ERRs.
	//
	// Use --datapack-dir pointing at a non-directory path so
	// RunServerCompiler will fail when it tries to read the cache.
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
	if !strings.Contains(out, "ERR") {
		t.Errorf("stdout missing ERR row; got:\n%s", out)
	}
	if !strings.Contains(out, "SKIP") {
		t.Errorf("stdout missing SKIP row(s) after ERR; got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack_StopOnError -v
```

Expected: FAIL — `--stop-on-error` is currently ignored; either the run continues past the failure (no SKIPs) or the assertion finds no SKIP rows.

- [ ] **Step 3: Thread `stopOnError` into `runStages`**

Change the signature and the loop body:

```go
func runStages(srcDir, outDir, dataPackDir string, stopOnError bool, logger *slog.Logger) []stageResult {
	// ... existing PackConfigs block unchanged ...

	// Inside the `for _, st := range rest` loop, after the err branch:
	for i, st := range rest {
		logger.Info("stage_start", "stage", st.name)
		start := time.Now()
		err := st.run()
		elapsed := time.Since(start)
		files, bytesSum, _ := walkOutDir(outDir)
		if err != nil {
			logger.Error("stage_err", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum, "err", err)
			results = append(results, stageResult{Name: st.name, Status: stageErr, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum, Err: err})
			if stopOnError {
				// Mark every remaining stage as SKIP and return.
				for _, remaining := range rest[i+1:] {
					results = append(results, stageResult{Name: remaining.name, Status: stageSkip})
				}
				return results
			}
			continue
		}
		logger.Info("stage_done", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum)
		results = append(results, stageResult{Name: st.name, Status: stageOK, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum})
	}
	return results
}
```

Update the call site in `runSmokePack`:

```go
	results := runStages(*contentDir, effectiveOut, effectiveDataPack, *stopOnError, logger)
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunSmokePack -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./cmd/goscape-cli/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...
```

Expected: all TestRunSmokePack_* tests PASS under both `-run TestRunSmokePack` and `-race`. `go vet` clean.

If the induced-failure test in Step 1 turns out NOT to trigger a stage error (e.g., RunServerCompiler tolerates a not-a-directory `--datapack-dir`), pick a different induction:
- Make `outDir` a file (chmod 0644) before running — `clientinterface.Pack` should fail trying to mkdir under it. Set the file path as `--out-dir`.
- Or write a malformed `interface/` source that `clientinterface.Pack` rejects.

Pick whichever induces an ERR while leaving PackConfigs OK. Document the choice in the test doc-comment.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_smoke_pack.go cmd/goscape-cli/cmd_smoke_pack_test.go
git commit --no-gpg-sign -m "feat(goscape-cli): smoke-pack --stop-on-error short-circuits with SKIP rows for downstream stages"
```

---

## Task 7: Real-Content shakedown + memory close

**Files:** none (manual operator run)

This task is non-coding: drive the new verb against real `LostCityRS/Content` and capture the divergence list for the close-memory entry. The unit tests cover mechanics; this exercise covers value.

- [ ] **Step 1: Build the binary**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o ./goscape-cli ./cmd/goscape-cli
```

- [ ] **Step 2: Run against real Content with --keep**

```bash
./goscape-cli smoke-pack \
  --content-dir /home/owner/Code/github.com/LostCityRS/Content \
  --keep \
  --log.level info \
  2> smoke-stderr.log
```

The summary table is on stdout; slog output on stderr.

- [ ] **Step 3: Capture the divergence list**

Record, for each stage:
- Status (OK / ERR)
- Elapsed
- Output file count + bytes
- For ERR: the err message

Also note the out-dir path so artifacts are inspectable.

- [ ] **Step 4: Write the close-memory entry**

Save to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/smoke_pack_phase1_close.md` with frontmatter:

```markdown
---
name: smoke-pack-phase1-close
description: smoke-pack Phase 1 verb shipped YYYY-MM-DD; first real-Content run surfaced N stage divergences (list); BUILDVERIFY-INTERFACE/pixpack-tiled-sheet anticipated divergences confirmed (or not); follow-up sub-specs queued per divergence
metadata:
  type: project
---

# Body
- Short of TS-parity gaps (e.g. pixpack tiled-sheet, BUILDVERIFY-INTERFACE) confirmed or refuted by the real-Content run.
- Per-stage outcome table.
- Open follow-up sub-specs (one per surfaced gap that needs work).
- Phase 2 (byte-diff vs Engine-TS reference) status.
```

Add a one-line pointer in `MEMORY.md`:

```markdown
- [smoke-pack Phase 1 close](smoke_pack_phase1_close.md) — smoke-pack verb shipped YYYY-MM-DD; first real-Content run surfaced N divergences; M follow-ups queued
```

- [ ] **Step 5: Final commit (memory + close)**

The memory file is outside the repo. Commit any in-repo cleanup if applicable; otherwise close with:

```bash
git log --oneline HEAD~6..HEAD
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all green. If a stage error in the real-Content run identifies a regression in a previously-shipped package (rather than a known-divergent expected gap), file it as a sub-spec; do not block this close on it.

---

## Self-Review Checklist (already applied)

- ✅ Spec §3 flags covered: T1 (all flags parsed), T2 (--out-dir required path → T5 auto), T5 (--keep), T6 (--stop-on-error). Log flags wired in T1.
- ✅ Spec §3 exit codes 0/1/2/3 covered: T1 (2/3 paths), T2 (0/1 wiring), implicit through T6.
- ✅ Spec §4 stage list reproduced verbatim in T2 (`PackConfigs` special + 10 in `rest`).
- ✅ Spec §5 telemetry: T3 elapsed + cumulative files/bytes via `walkOutDir`.
- ✅ Spec §6 summary table: T4 via `tabwriter`; trailing out-dir line in T5.
- ✅ Spec §7 tests: -h (T1), unknown flag (T1), missing --content-dir (T1), non-existent --content-dir (T1), happy-path against synthetic fixture (T2), --stop-on-error (T6), --keep (T5), operator-supplied --out-dir not deleted (T5).
- ✅ No placeholders. All code blocks complete.
- ✅ Type/method consistency: `stageResult`, `stageStatus`, `runStages`, `walkOutDir`, `printSummary`, `runSmokePack` named identically across tasks.
- ✅ SKIP semantics from §5 covered: PackConfigs failure (T2), stop-on-error (T6).
- ✅ No production-package edits outside `cmd/goscape-cli/`.
- ✅ All `go` invocations prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per CLAUDE.md.
- ✅ All commits use `--no-gpg-sign` per CLAUDE.md.
