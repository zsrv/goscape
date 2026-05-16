# `goscape-cli pack` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new `cmd/goscape-cli/` binary with one verb `pack` that invokes `pkg/pack.PackAll` and exits.

**Architecture:** Sibling binary to `cmd/goscape` (the daemon). Stdlib `flag`-based subcommand dispatcher; one verb file per subcommand. No dskit/service integration — pack is one-shot operational tooling, not a daemon. Build-script + Dockerfile updates ship both binaries.

**Tech Stack:** Go 1.26+ per `[[go_version]]`. Stdlib `flag`, `log/slog`. Reuses `pkg/util/log.NewLogger` for logger consistency with the daemon. No new dependencies.

**Project conventions** (apply to every task):
- All `go` invocations: prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`.
- All commits: pass `--no-gpg-sign`.
- Spec: `docs/superpowers/specs/2026-05-16-goscape-cli-pack-design.md`.

---

## Task 1: `cmd/goscape-cli/cmd_pack.go` — `runPack` verb

**Files:**
- Create: `cmd/goscape-cli/cmd_pack.go`
- Create: `cmd/goscape-cli/cmd_pack_test.go`

`runPack(args []string, stderr io.Writer) int` — parses pack-verb flags, calls `pack.PackAll`, returns an exit code. `stderr` is injected so the dispatcher test in Task 2 can capture flag-parse error output deterministically.

- [ ] **Step 1: Write the failing tests**

Create `cmd/goscape-cli/cmd_pack_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunPack -v`
Expected: build failure — `runPack` undefined. (Test compilation fails because the implementation file doesn't exist yet; this is the correct red state.)

- [ ] **Step 3: Implement `runPack`**

Create `cmd/goscape-cli/cmd_pack.go`:

```go
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runPack implements the `pack` verb: parses flags, calls
// pack.PackAll, returns an exit code.
//
// stderr receives flag-parse error output (for testability — the
// dispatcher in main.go passes os.Stderr).
//
// Exit codes:
//
//	0 — success
//	1 — logger init failed or pack.PackAll returned an error
//	2 — flag parse error
func runPack(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory.")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory.")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: --out-dir).")

	var logLevel slog.Level = slog.LevelInfo
	fs.TextVar(&logLevel, "log.level", logLevel,
		"Log severity (debug|info|warn|error).")
	logFormat := fs.String("log.format", "text",
		"Log format (text|json).")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger, err := log.NewLogger(logLevel, *logFormat)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create logger: %v\n", err)
		return 1
	}

	dpd := *dataPackDir
	if dpd == "" {
		dpd = *outDir
	}

	logger.Info("packing",
		"src_dir", *srcDir,
		"out_dir", *outDir,
		"datapack_dir", dpd,
	)
	if err := pack.PackAll(*srcDir, *outDir, dpd); err != nil {
		logger.Error("pack failed", "err", err)
		return 1
	}
	logger.Info("pack succeeded")
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestRunPack -v`
Expected: all 3 tests PASS.

If `TestRunPack_HappyPath` fails with an unexpected PackAll error, the fixture diverged from `pkg/pack/pack_all_test.go:TestPackAll_ThreeStageSmoke`. Re-read that test and reconcile.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_pack.go cmd/goscape-cli/cmd_pack_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): T1 — pack verb (runPack)

First verb for the new cmd/goscape-cli binary. Parses --src-dir /
--out-dir / --datapack-dir / --log.level / --log.format, calls
pack.PackAll, returns 0/1/2 exit code.

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-pack-design.md
EOF
)"
```

---

## Task 2: `cmd/goscape-cli/main.go` — dispatcher + `main`

**Files:**
- Create: `cmd/goscape-cli/main.go`
- Create: `cmd/goscape-cli/main_test.go`

`dispatch(args []string, stdout, stderr io.Writer) int` — pure routing helper, testable. `main()` is a thin wrapper that calls `dispatch(os.Args[1:], os.Stdout, os.Stderr)` and exits.

- [ ] **Step 1: Write the failing tests**

Create `cmd/goscape-cli/main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDispatch_NoArgs returns 2 and prints usage to stderr.
func TestDispatch_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr %q missing usage", stderr.String())
	}
}

// TestDispatch_UnknownVerb returns 2 and names the verb in stderr.
func TestDispatch_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"frobnicate"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "frobnicate") {
		t.Errorf("stderr %q does not mention unknown verb", stderr.String())
	}
}

// TestDispatch_HelpFlag returns 0 and prints usage to stdout.
func TestDispatch_HelpFlag(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := dispatch([]string{arg}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("dispatch(%q) returned %d, want 0", arg, code)
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Errorf("stdout %q missing usage", stdout.String())
			}
		})
	}
}

// TestDispatch_PackRouting verifies the `pack` verb is dispatched to
// runPack (not happy-path — just that bad flags reach the pack flag
// set, surfacing exit code 2).
func TestDispatch_PackRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"pack", "--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -run TestDispatch -v`
Expected: build failure — `dispatch` undefined.

- [ ] **Step 3: Implement dispatcher + main**

Create `cmd/goscape-cli/main.go`:

```go
// Command goscape-cli is the operational-tooling sibling of cmd/goscape.
// The daemon binary runs long-lived services; this binary runs one-shot
// utilities like `pack`.
//
// Layout mirrors grafana/tempo's cmd/tempo-cli: subcommand-dispatched,
// one verb per file. Today: `pack` only.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch routes args[0] to a verb handler. stdout receives help
// output; stderr receives errors and usage-on-failure.
//
// Exit codes:
//
//	0 — success (or help-flag print)
//	1 — verb returned a runtime error
//	2 — no verb, unknown verb, or verb flag-parse error
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	verb, rest := args[0], args[1:]
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
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli <verb> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Verbs:")
	fmt.Fprintln(w, "  pack    Build server-side packs (configs + compiled scripts).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `goscape-cli <verb> -h` for verb-specific flags.")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/... -v`
Expected: all Task 1 + Task 2 tests PASS.

- [ ] **Step 5: Sanity-check the binary builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./cmd/goscape-cli`
Expected: clean build, no output, no artifact left in repo root (default build behavior with package-path target).

Also vet:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./cmd/goscape-cli/...`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/main.go cmd/goscape-cli/main_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): T2 — dispatcher + main

dispatch(args, stdout, stderr) routes args[0] to a verb handler.
Today: pack only. -h/--help/help print usage to stdout (exit 0);
missing/unknown verb prints to stderr (exit 2).

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-pack-design.md
EOF
)"
```

---

## Task 3: Build wiring — `build.sh`, `Dockerfile`, `Makefile`

**Files:**
- Modify: `build.sh`
- Modify: `Dockerfile`
- Modify: `Makefile`

Add `goscape-cli` to the local build script, container image build, and Makefile run/build targets so the sibling binary ships alongside `goscape`.

- [ ] **Step 1: Update `build.sh`**

Current contents:

```sh
#!/bin/sh

go build -trimpath -ldflags '-s -w' -o goscape cmd/goscape/main.go
```

Replace with:

```sh
#!/bin/sh

go build -trimpath -ldflags '-s -w' -o goscape     ./cmd/goscape
go build -trimpath -ldflags '-s -w' -o goscape-cli ./cmd/goscape-cli
```

(Switched the existing `goscape` line from the single-file form `cmd/goscape/main.go` to the package-path form `./cmd/goscape` for consistency with the new line — also avoids breakage if `cmd/goscape` later grows additional `.go` files in `package main`.)

- [ ] **Step 2: Update `Dockerfile`**

Current build stage line:

```dockerfile
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /go/bin/goscape ./cmd/goscape
```

Add a sibling build line immediately after it:

```dockerfile
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /go/bin/goscape     ./cmd/goscape
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /go/bin/goscape-cli ./cmd/goscape-cli
```

In the final stage, current line:

```dockerfile
COPY --from=build /go/bin/goscape /goscape
```

Add the sibling COPY immediately after it:

```dockerfile
COPY --from=build /go/bin/goscape     /goscape
COPY --from=build /go/bin/goscape-cli /goscape-cli
```

Leave `ENTRYPOINT ["/goscape"]` unchanged — the container's default behavior is still to run the daemon. Operators invoke the CLI explicitly with `docker run --entrypoint /goscape-cli <image> pack ...`.

- [ ] **Step 3: Update `Makefile`**

Current target near the bottom of the file:

```makefile
run:
	CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
```

Leave `run` unchanged (it's a daemon convenience). Add a new `pack` target immediately after it for parity:

```makefile
pack:
	CGO_ENABLED=0 go run -trimpath ./cmd/goscape-cli pack
```

This invokes `goscape-cli pack` with all defaults (`--src-dir data/src`, `--out-dir data/pack`). Users override via standard `make` env-var passthrough or by calling `go run` directly.

- [ ] **Step 4: Sanity check — both binaries build via build.sh**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache sh build.sh
```

Expected: two artifacts at repo root, `./goscape` and `./goscape-cli`. Quick smoke:

```bash
./goscape-cli -h
```

Expected stdout:

```
Usage: goscape-cli <verb> [flags]

Verbs:
  pack    Build server-side packs (configs + compiled scripts).

Run `goscape-cli <verb> -h` for verb-specific flags.
```

Exit code 0.

Then:

```bash
./goscape-cli pack -h
```

Expected: pack-verb flag listing from `flag` package's default usage formatter (mentions `--src-dir`, `--out-dir`, `--datapack-dir`, `--log.level`, `--log.format`). Exit code 0 (pack `-h` exits 0 via flag package's stdlib behavior — actually `flag.ContinueOnError` causes `Parse` to return `flag.ErrHelp` for `-h`, which `runPack` currently maps to exit 2 via the generic parse-error path).

**Note on `pack -h` exit code:** the current `runPack` implementation returns 2 for all parse errors including `flag.ErrHelp`. This is acceptable but slightly nonstandard (most CLIs return 0 for `-h`). Leaving as 2 keeps Task 1 unchanged and matches the existing project's flag-handling style. If a follow-up wants `pack -h` to exit 0, it's a 2-line patch in `runPack`:

```go
if err := fs.Parse(args); err != nil {
    if errors.Is(err, flag.ErrHelp) {
        return 0
    }
    return 2
}
```

Not in scope for this slice. The sanity-check step accepts either exit code.

Cleanup:

```bash
rm -f goscape goscape-cli
```

- [ ] **Step 5: Commit**

```bash
git add build.sh Dockerfile Makefile
git commit --no-gpg-sign -m "$(cat <<'EOF'
build(goscape-cli): T3 — ship cli alongside daemon

build.sh, Dockerfile, and Makefile now build/include both binaries.
Container ENTRYPOINT still runs the daemon; operators invoke the cli
via `docker run --entrypoint /goscape-cli <image> pack ...`.

Spec: docs/superpowers/specs/2026-05-16-goscape-cli-pack-design.md
EOF
)"
```

---

## Final verification

After all three tasks land, run the full project test suite to confirm nothing regressed elsewhere:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS on both. The new `cmd/goscape-cli/` package adds 4 test functions (3 `TestRunPack_*` + `TestDispatch_*`); no existing test should be touched.
