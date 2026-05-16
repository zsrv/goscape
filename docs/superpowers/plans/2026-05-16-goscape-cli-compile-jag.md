# `goscape-cli compile` + `jag` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two verbs to `cmd/goscape-cli/`: `compile <path>.rs2` (full runescript pipeline on one file) and `jag` (parent verb with `list`/`extract`/`dump` sub-verbs for inspecting `.jag` archives).

**Architecture:** Two new verb files (`cmd_compile.go`, `cmd_jag.go`) parallel to the existing `cmd_pack.go`. Dispatcher gets two new switch arms. `runJag` does its own internal mini-dispatch over the three sub-verbs. One new exported helper `compiler.LoadCompilerSymbols` exposes the symbol-loading prep that's currently private inside `RunServerCompiler` — additive (the existing flow stays byte-identical to preserve the `runServerCompilerCore` test seam).

**Tech Stack:** Go 1.26+ per `[[go_version]]`. Stdlib `flag`, `log/slog`. Reuses `pkg/pack/compiler.LoadCompilerSymbols` (new), `pkg/pack/compiler/runescript.Compile`, `pkg/io/jagfile.LoadJagfile`. No new dependencies.

**Project conventions** (apply to every task):
- All `go` invocations: prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`.
- All commits: pass `--no-gpg-sign`.
- Spec: `docs/superpowers/specs/2026-05-16-goscape-cli-compile-jag-design.md`.

---

## Task 1: `compiler.LoadCompilerSymbols` exporter

**Files:**
- Modify: `pkg/pack/compiler/run_server_compiler.go` (add one exported function; existing code unchanged)

Pure additive change — exposes the symbol-loading prep chain that's currently inlined inside `RunServerCompiler`. Tests pass implicitly: the existing `RunServerCompiler` (and the full `PackAll` pipeline) stays byte-identical, and Task 2's compile-verb tests exercise `LoadCompilerSymbols` from the consumer side. No separate unit test is added here.

- [ ] **Step 1: Verify pre-Task baseline is green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`
Expected: all PASS.

- [ ] **Step 2: Add `LoadCompilerSymbols` to `pkg/pack/compiler/run_server_compiler.go`**

Append the following function at the end of the file (after the existing `translateCommandPointerNames` function):

```go
// LoadCompilerSymbols assembles the symbol map the runescript compiler
// needs to type-check and codegen. Identical to the prep stages
// RunServerCompiler runs before invoking runescript.Compile.
//
// srcDir: directory containing scripts/ and pack/ subdirs.
// dataPackDir: cache directory with the 7 .dat/.idx pairs (read back
// the cache PackConfigs writes).
//
// The NAI-212-D-POINTER-NAME-TRANSLATION translation is applied to the
// "command" entry so callers can invoke runescript.Compile directly
// with the returned map.
func LoadCompilerSymbols(srcDir, dataPackDir string) (map[string]*runescript.CompilerTypeInfo, error) {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadCompilerSymbols: %w", err)
	}
	symbols, err := buildSymbolsCore(srcDir, loaders)
	if err != nil {
		return nil, fmt.Errorf("LoadCompilerSymbols: %w", err)
	}
	bridged := make(map[string]*runescript.CompilerTypeInfo, len(symbols))
	for k, v := range symbols {
		bridged[k] = ToCompilerTypeInfo(v)
	}
	if cmd, ok := bridged["command"]; ok {
		translateCommandPointerNames(cmd)
	}
	return bridged, nil
}
```

The `fmt`, `path/filepath`, `strings`, and `github.com/zsrv/goscape/pkg/pack/compiler/runescript` imports already exist in the file — no import-block change needed.

- [ ] **Step 3: Verify existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`
Expected: all PASS (no test was added, no production behavior changed).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/...`
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/compiler/run_server_compiler.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/compiler): export LoadCompilerSymbols

Exposes the symbol-loading prep chain that's currently private inside
RunServerCompiler — loadConfigs + buildSymbolsCore + ToCompilerTypeInfo
bridge + translateCommandPointerNames. Returns the map of
runescript.CompilerTypeInfo that runescript.Compile accepts directly.

Additive: RunServerCompiler and runServerCompilerCore stay byte-
identical to preserve the existing *configLoaders test seam in
run_server_compiler_test.go. Minor prep-chain duplication accepted for
this slice.

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-compile-jag-design.md
EOF
)"
```

---

## Task 2: `cmd/goscape-cli/cmd_compile.go` — `runCompile` verb

**Files:**
- Create: `cmd/goscape-cli/cmd_compile.go`
- Create: `cmd/goscape-cli/cmd_compile_test.go`

`runCompile(args []string, stderr io.Writer) int` — parses flags, loads symbols via `compiler.LoadCompilerSymbols`, calls `runescript.Compile`, returns exit code. Mirrors `runPack`'s shape; same `stderr`-only writer contract (no stdout output).

- [ ] **Step 1: Write the failing tests**

Create `cmd/goscape-cli/cmd_compile_test.go` with EXACTLY this content:

```go
package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunCompile_HappyPath_Check seeds the same minimal fixture pack
// uses (via the seedMinimalPackFixture helper in cmd_pack_test.go),
// then compiles helper.rs2 in --check mode. Expects exit 0 and
// "compile succeeded" in the captured stderr (logger output).
func TestRunCompile_HappyPath_Check(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)

	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", dir,
		"--datapack-dir", dir,
		"--check",
		filepath.Join(dir, "scripts", "helper.rs2"),
	}, &stderr)

	if code != 0 {
		t.Fatalf("runCompile returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "compile succeeded") {
		t.Errorf("stderr %q missing success log", stderr.String())
	}
}

// TestRunCompile_SourceError seeds the minimal fixture but replaces
// helper.rs2 with an invalid source. Expects exit 1.
func TestRunCompile_SourceError(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)
	// Overwrite helper.rs2 with a clearly-broken source (unknown
	// command "not_a_command").
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nnot_a_command;\n")

	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", dir,
		"--datapack-dir", dir,
		"--check",
		filepath.Join(dir, "scripts", "helper.rs2"),
	}, &stderr)

	if code != 1 {
		t.Fatalf("runCompile returned %d, want 1; stderr=%q", code, stderr.String())
	}
}

// TestRunCompile_MissingPath expects exit 2 when no positional arg
// is provided.
func TestRunCompile_MissingPath(t *testing.T) {
	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", "irrelevant",
	}, &stderr)
	if code != 2 {
		t.Fatalf("runCompile returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "exactly one source path") {
		t.Errorf("stderr %q missing missing-path diagnostic", stderr.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunCompile -v`
Expected: build failure — `runCompile` undefined.

- [ ] **Step 3: Implement `runCompile`**

Create `cmd/goscape-cli/cmd_compile.go` with EXACTLY this content:

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runCompile implements the `compile` verb: parses flags, loads the
// compiler symbol set via compiler.LoadCompilerSymbols, and invokes
// runescript.Compile on a single .rs2 source file.
//
// stderr receives both flag-parse error output and slog logger
// output. The dispatcher in main.go passes os.Stderr.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` print)
//	1 — symbol load failed, compile failed, logger init failed, or
//	    temp-dir creation failed
//	2 — flag parse error or missing/extra positional argument
func runCompile(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory.")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: data/pack).")
	check := fs.Bool("check", false,
		"Diagnostics-only mode; discard compiler output.")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory (ignored when --check is set).")

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

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "compile: expected exactly one source path")
		return 2
	}
	path := rest[0]

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create logger: %v\n", err)
		return 1
	}

	dpd := *dataPackDir
	if dpd == "" {
		dpd = "data/pack"
	}

	writerOut := filepath.Join(*outDir, "server")
	if *check {
		tmp, err := os.MkdirTemp("", "goscape-cli-compile-*")
		if err != nil {
			logger.Error("failed to create temp dir", "err", err)
			return 1
		}
		defer os.RemoveAll(tmp)
		writerOut = tmp
	}

	logger.Info("loading compiler symbols",
		"src_dir", *srcDir,
		"datapack_dir", dpd,
	)
	symbols, err := compiler.LoadCompilerSymbols(*srcDir, dpd)
	if err != nil {
		logger.Error("load symbols failed", "err", err)
		return 1
	}

	logger.Info("compiling",
		"path", path,
		"check", *check,
		"writer_out", writerOut,
	)
	cfg := runescript.Config{
		SourcePaths: []string{path},
		Symbols:     symbols,
		Writer: runescript.WriterConfig{
			Jag: &runescript.JagWriterConfig{Output: writerOut},
		},
	}
	if err := runescript.Compile(cfg); err != nil {
		logger.Error("compile failed", "err", err)
		return 1
	}
	logger.Info("compile succeeded")
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunCompile -v`
Expected: all 3 tests PASS.

If `TestRunCompile_HappyPath_Check` fails with an unexpected `LoadCompilerSymbols` or `Compile` error, the minimal fixture's symbol coverage is the most likely cause. Re-read `pkg/pack/pack_all_test.go:TestPackAll_ThreeStageSmoke` — that's the reference fixture and it does drive the full PackAll pipeline (including `RunServerCompiler` which is what we mirror here). Reconcile any divergence.

If `TestRunCompile_SourceError` fails with exit 0, the runescript compiler may be more permissive about `not_a_command;` than expected. Try `not_a_command(broken);` (forces argument-resolution to fail) or `^bogus_constant` (unknown constant reference). Update the test to whatever syntax actually produces a non-zero exit.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_compile.go cmd/goscape-cli/cmd_compile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): T2 — compile verb (runCompile)

Single-file runescript compile. Loads the same symbol set PackAll
uses (via compiler.LoadCompilerSymbols), then calls runescript.Compile
with SourcePaths=[<path>]. --check mode writes to a temp dir and
discards (avoids upstream Writer-contract change).

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-compile-jag-design.md
EOF
)"
```

---

## Task 3: `cmd/goscape-cli/cmd_jag.go` — `runJag` verb (list/extract/dump)

**Files:**
- Create: `cmd/goscape-cli/cmd_jag.go`
- Create: `cmd/goscape-cli/cmd_jag_test.go`

`runJag(args []string, stdout, stderr io.Writer) int` is a mini-dispatcher over `list`/`extract`/`dump`. Each sub-handler owns its own flag set. Content goes to stdout (entry listings, raw bytes); diagnostics go to stderr.

- [ ] **Step 1: Write the failing tests**

Create `cmd/goscape-cli/cmd_jag_test.go` with EXACTLY this content:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// makeTestJagPath writes a small single-entry Jagfile (one file
// "hitmarks.dat" with payload []byte{0xFF}) to t.TempDir() and
// returns the path. Byte layout mirrors
// pkg/io/jagfile/jagfile_test.go:MakeTestJagfile, which lives in
// _test.go and is invisible cross-package.
func makeTestJagPath(t *testing.T) string {
	t.Helper()
	p := packet.NewPacket(make([]byte, 0, 19))
	p.P3(1)                        // UnpackedSize
	p.P3(1)                        // PackedSize
	p.P2(1)                        // FileCount
	p.P4(-1502153170 & 0xFFFFFFFF) // hash("hitmarks.dat")
	p.P3(1)                        // FileUnpackedSize[0]
	p.P3(1)                        // FilePackedSize[0]
	p.P1(255)                      // payload byte
	p.Pos = 0

	jf, err := jagfile.NewJagfile(p)
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	// NewJagfile reverse-resolves FileName[i] from FileHash[i] via
	// the package's knownNames table (which includes "hitmarks.dat"
	// at jagfile.go:432), so FileName is already populated here. No
	// manual injection needed.

	path := filepath.Join(t.TempDir(), "test.jag")
	if err := jf.Save(path, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return path
}

// TestRunJag_List verifies `jag list` writes one TAB-separated line
// per entry to stdout.
func TestRunJag_List(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"list", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	want := "hitmarks.dat\t1\t1\n"
	if stdout.String() != want {
		t.Errorf("stdout %q, want %q", stdout.String(), want)
	}
}

// TestRunJag_ExtractToStdout verifies extract writes raw entry bytes
// to stdout when --out is unset (or "-").
func TestRunJag_ExtractToStdout(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "hitmarks.dat"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), []byte{0xFF}) {
		t.Errorf("stdout bytes %v, want [255]", stdout.Bytes())
	}
}

// TestRunJag_ExtractToFile verifies --out <path> writes raw bytes
// to the file.
func TestRunJag_ExtractToFile(t *testing.T) {
	path := makeTestJagPath(t)
	out := filepath.Join(t.TempDir(), "out.bin")

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "hitmarks.dat", "--out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("file bytes %v, want [255]", got)
	}
}

// TestRunJag_ExtractMissingEntry expects exit 1 with a "no such
// entry" diagnostic in stderr.
func TestRunJag_ExtractMissingEntry(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "nope.dat"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runJag returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no such entry") {
		t.Errorf("stderr %q missing 'no such entry'", stderr.String())
	}
}

// TestRunJag_Dump writes every entry into --out <dir>. With a one-
// entry fixture, asserts <dir>/hitmarks.dat exists with the right
// bytes.
func TestRunJag_Dump(t *testing.T) {
	path := makeTestJagPath(t)
	outDir := filepath.Join(t.TempDir(), "dump")

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"dump", path, "--out", outDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(outDir, "hitmarks.dat"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("dumped bytes %v, want [255]", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunJag -v`
Expected: build failure — `runJag` undefined.

- [ ] **Step 3: Implement `runJag`**

Create `cmd/goscape-cli/cmd_jag.go` with EXACTLY this content:

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// runJag implements the `jag` verb: mini-dispatcher over the
// `list`, `extract`, and `dump` sub-verbs.
//
// stdout receives entry-listing or raw-byte content; stderr
// receives diagnostics and flag-parse errors.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` on the sub-verb)
//	1 — file-not-found, parse failure, missing entry, dir-not-empty,
//	    or write error
//	2 — flag parse error, missing/extra positional argument, or
//	    missing/unknown sub-verb
func runJag(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "jag: missing sub-verb (expected: list | extract | dump)")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return runJagList(rest, stdout, stderr)
	case "extract":
		return runJagExtract(rest, stdout, stderr)
	case "dump":
		return runJagDump(rest, stdout, stderr)
	case "-h", "--help", "help":
		jagUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "jag: unknown sub-verb %q\n\n", sub)
		jagUsage(stderr)
		return 2
	}
}

func jagUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli jag <sub-verb> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Sub-verbs:")
	fmt.Fprintln(w, "  list    <path.jag>                            List entries (name<TAB>unpacked<TAB>packed).")
	fmt.Fprintln(w, "  extract <path.jag> <entry> [--out <path>]     Extract one entry (default: stdout).")
	fmt.Fprintln(w, "  dump    <path.jag> --out <dir>                Extract every entry into <dir>.")
}

func runJagList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jag list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "jag list: expected exactly one path")
		return 2
	}
	jf, err := jagfile.LoadJagfile(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "jag list: %v\n", err)
		return 1
	}
	for i := 0; i < jf.FileCount; i++ {
		fmt.Fprintf(stdout, "%s\t%d\t%d\n",
			jf.FileName[i], jf.FileUnpackedSize[i], jf.FilePackedSize[i])
	}
	return 0
}

func runJagExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jag extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "-", "Output path (\"-\" for stdout).")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, "jag extract: expected <path.jag> <entry>")
		return 2
	}
	path, entry := rest[0], rest[1]

	jf, err := jagfile.LoadJagfile(path)
	if err != nil {
		fmt.Fprintf(stderr, "jag extract: %v\n", err)
		return 1
	}
	pkt, err := jf.Read(entry)
	if err != nil {
		fmt.Fprintf(stderr, "jag extract: no such entry: %s\n", entry)
		return 1
	}

	var sink io.Writer = stdout
	if *out != "-" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(stderr, "jag extract: %v\n", err)
			return 1
		}
		defer f.Close()
		sink = f
	}
	if _, err := sink.Write(pkt.Data); err != nil {
		fmt.Fprintf(stderr, "jag extract: write: %v\n", err)
		return 1
	}
	return 0
}

func runJagDump(args []string, stdout, stderr io.Writer) int {
	_ = stdout // unused; dump writes only files
	fs := flag.NewFlagSet("jag dump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out", "", "Output directory (required).")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "jag dump: expected exactly one path")
		return 2
	}
	if *outDir == "" {
		fmt.Fprintln(stderr, "jag dump: --out is required")
		return 2
	}

	// Empty-dir safety check: refuse non-empty existing dir. Create
	// the dir if missing.
	if entries, err := os.ReadDir(*outDir); err == nil {
		if len(entries) > 0 {
			fmt.Fprintf(stderr, "jag dump: --out dir %q is not empty\n", *outDir)
			return 1
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "jag dump: mkdir: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stderr, "jag dump: stat --out: %v\n", err)
		return 1
	}

	jf, err := jagfile.LoadJagfile(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "jag dump: %v\n", err)
		return 1
	}
	for i := 0; i < jf.FileCount; i++ {
		name := jf.FileName[i]
		pkt, err := jf.Get(i)
		if err != nil {
			fmt.Fprintf(stderr, "jag dump: %s: %v\n", name, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(*outDir, name), pkt.Data, 0o644); err != nil {
			fmt.Fprintf(stderr, "jag dump: write %s: %v\n", name, err)
			return 1
		}
	}
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunJag -v`
Expected: all 5 tests PASS.

If `TestRunJag_List` produces `\t1\t1\n` instead of `hitmarks.dat\t1\t1\n`, the `LoadJagfile` → `NewJagfile` reverse-hash-lookup didn't repopulate `FileName[0]` from the `knownNames` table. The expected behavior is that `nameToHash["hitmarks.dat"]` matches the stored hash, so `FileName[0]` gets set to `"hitmarks.dat"`. Re-read `pkg/io/jagfile/jagfile.go:NewJagfile` (around the `for k, v := range maps.All(nameToHash)` loop near line 329) to confirm.

If `TestRunJag_ExtractToStdout` produces empty stdout, `jf.Read(name)` is returning a `*packet.Packet` whose `Data` field doesn't contain the unpacked entry bytes. `Get` calls `packet.NewPacket(decompressed)` which sets `Data` to the full byte slice (with `Pos=0`); if Pos is non-zero, you'd want `pkt.Data[pkt.Pos:]` instead. Read `pkg/io/jagfile/jagfile.go:Get` and `pkg/io/packet/buffer.go:Packet` to confirm.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_jag.go cmd/goscape-cli/cmd_jag_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): T3 — jag verb (list | extract | dump)

Parent verb with internal mini-dispatch over three sub-verbs.
- list: TAB-separated entry name + unpacked + packed sizes.
- extract: one entry to --out (default stdout).
- dump: every entry to --out dir (refuses non-empty dir).

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-compile-jag-design.md
EOF
)"
```

---

## Task 4: Dispatcher wiring — `main.go` + `main_test.go`

**Files:**
- Modify: `cmd/goscape-cli/main.go` (two new switch arms + usage text update)
- Modify: `cmd/goscape-cli/main_test.go` (two new dispatcher routing tests)

- [ ] **Step 1: Write the failing tests**

Append the following two test functions at the end of `cmd/goscape-cli/main_test.go` (after the existing `TestDispatch_PackRouting`):

```go
// TestDispatch_CompileRouting verifies the `compile` verb is
// dispatched to runCompile (bad flags reach the compile flag set,
// surfacing exit code 2).
func TestDispatch_CompileRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"compile", "--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestDispatch_JagRouting verifies the `jag` verb is dispatched to
// runJag (bare `jag` returns 2 with missing-sub-verb diagnostic).
func TestDispatch_JagRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"jag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "missing sub-verb") {
		t.Errorf("stderr %q missing sub-verb diagnostic", stderr.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run 'TestDispatch_(CompileRouting|JagRouting)' -v`
Expected: tests FAIL — `dispatch` still routes `compile` and `jag` to the default arm (unknown verb), returning 2 with "unknown verb" instead of the verb-specific diagnostic. The flag-parse-error path / sub-verb-missing path is NOT being reached.

- [ ] **Step 3: Wire the dispatcher**

In `cmd/goscape-cli/main.go`, replace the existing `switch verb {` block:

```go
	switch verb {
	case "pack":
		return runPack(rest, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown verb: %q\n\n", verb)
		usage(stderr)
		return 2
	}
```

with:

```go
	switch verb {
	case "pack":
		return runPack(rest, stderr)
	case "compile":
		return runCompile(rest, stderr)
	case "jag":
		return runJag(rest, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown verb: %q\n\n", verb)
		usage(stderr)
		return 2
	}
```

Then update the `usage` function body. Replace the existing `usage` body:

```go
func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli <verb> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Verbs:")
	fmt.Fprintln(w, "  pack    Build server-side packs (configs + compiled scripts).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `goscape-cli <verb> -h` for verb-specific flags.")
}
```

with:

```go
func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli <verb> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Verbs:")
	fmt.Fprintln(w, "  pack       Build server-side packs (configs + compiled scripts).")
	fmt.Fprintln(w, "  compile    Run the runescript compiler on a single .rs2 source file.")
	fmt.Fprintln(w, "  jag        Inspect a .jag archive (list | extract | dump).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `goscape-cli <verb> -h` for verb-specific flags.")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -v`
Expected: all tests PASS — the four Task-1-era and 7-task-2-era tests plus the 3+5+2 added in T2/T3/T4.

- [ ] **Step 5: Sanity-check the binary builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./cmd/goscape-cli`
Expected: clean build, no output.

If a `./goscape-cli` artifact appears at repo root, remove it: `rm -f goscape-cli`.

Vet: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/main.go cmd/goscape-cli/main_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): T4 — wire `compile` and `jag` into dispatcher

Two new switch arms in dispatch + updated usage() text listing all
three verbs. Two dispatcher routing tests pin the wiring.

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-compile-jag-design.md
EOF
)"
```

---

## Final verification

After all four tasks land, run the full project test suite to confirm nothing regressed elsewhere:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS on both. The new cmd/goscape-cli/ verbs add 10 new test functions (3 `TestRunCompile_*` + 5 `TestRunJag_*` + 2 `TestDispatch_*Routing`). The `LoadCompilerSymbols` exporter in pkg/pack/compiler/ adds no new tests but is exercised transitively by the compile verb's happy-path test and (via the unchanged `RunServerCompiler` flow) by the existing `pkg/pack/pack_all_test.go:TestPackAll_ThreeStageSmoke`.

Quick CLI sanity check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache sh build.sh
./goscape-cli -h
./goscape-cli jag -h
rm -f goscape goscape-cli
```

Expected: `./goscape-cli -h` lists all three verbs (`pack`, `compile`, `jag`); `./goscape-cli jag -h` lists all three sub-verbs (`list`, `extract`, `dump`).
