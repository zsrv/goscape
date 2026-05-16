# `goscape-cli pack` — CLI wiring for `pack.PackAll`

**Date:** 2026-05-16
**Tech stack:** Go 1.26+ per `[[go_version]]`
**Cadence:** Compressed per `[[compressed_cadence]]` — ~100 LOC across two new files + small build-script diffs. One spec doc, one short plan, no formal sub-spec review cycle.
**Execution mode:** `subagent-driven-development` per `[[execution_mode_default]]`.
**TS canonical source:** none — TS `packAll()` is invoked from `tools/pack/PackAll.ts` (already ported in NAI-212). This slice is goscape-native CLI surface only; no TS-parity gate applies.

---

## §1. Scope

Add a new sibling binary `cmd/goscape-cli/` that exposes operational tooling for goscape. The first verb is `pack`, which invokes `pkg/pack.PackAll(srcDir, outDir, dataPackDir)` and exits.

Modeled on `/home/owner/Code/github.com/grafana/tempo/cmd/tempo-cli/` (separate binary, subcommand-dispatched, distinct from the daemon binary). Goscape's daemon stays `cmd/goscape`; ops/inspection tooling lives under `cmd/goscape-cli`.

**In scope:**
- New binary `cmd/goscape-cli/` with subcommand dispatcher and one verb `pack`.
- Flags: `--src-dir`, `--out-dir`, `--datapack-dir`, `--log.level`, `--log.format`.
- Build-script + Dockerfile updates so both binaries are built and shipped.

**Out of scope:**
- kong/cobra (single verb today; reconsider at 3+ verbs).
- Shared `config.yaml` `pack:` block — pack inputs are operational arguments, not daemon configuration. Daemon config stays focused.
- Module/dskit integration — pack is not a service.
- Additional verbs (::rebuild equivalent, jagfile inspection, single-script compile, content migration). The dispatcher is shaped to accept them, but none are scaffolded.
- Wiring `pack` into `--target` on the daemon binary.

---

## §2. Layout

| File | Purpose |
|---|---|
| `cmd/goscape-cli/main.go` (NEW) | Entry point; usage; subcommand dispatch. |
| `cmd/goscape-cli/cmd_pack.go` (NEW) | `pack` verb: flag parsing, logger init, `pack.PackAll` call, exit. |
| `Makefile` (MODIFIED) | Build target for `goscape-cli`. |
| `build.sh` (MODIFIED) | Build invocation for `goscape-cli`. |
| `Dockerfile` (MODIFIED) | Build + COPY `goscape-cli` into the final image. |

No changes to `cmd/goscape/`, `pkg/pack/`, or `cmd/goscape/app/`. No `modules/pack/`. No `config.yaml` change.

---

## §3. Invocation surface

```
goscape-cli                              # prints usage + list of verbs, exit 2
goscape-cli -h | --help                  # prints usage + list of verbs, exit 0
goscape-cli <verb> -h                    # prints verb-specific usage, exit 0
goscape-cli pack [flags]                 # run PackAll, exit 0 on success
```

### `pack` flags

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--src-dir` | string | `data/src` | Source content directory. Passed to `PackAll` as `srcDir`. |
| `--out-dir` | string | `data/pack` | Output directory. Passed to `PackAll` as `outDir`. |
| `--datapack-dir` | string | `""` | Cache directory for entity-type loaders. Empty → defaults to `--out-dir` at runtime (matches `PackAll` doc'd common case at `pkg/pack/pack_all.go:24-26`). |
| `--log.level` | string | `info` | One of `debug`, `info`, `warn`, `error`. Parsed via `slog.Level.UnmarshalText`. |
| `--log.format` | string | `text` | One of `text`, `json`. Passed to `pkg/util/log.NewLogger`. |

All paths are resolved as-given (no cwd-rewriting, no symlink chasing); callers are responsible for invoking from a consistent working directory.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | `PackAll` returned an error, or logger init failed. |
| 2 | Flag parse error, unknown verb, or `goscape-cli` invoked with no verb. |

---

## §4. Dispatcher shape

```go
// cmd/goscape-cli/main.go (sketch)
package main

import (
    "fmt"
    "os"
)

func main() {
    if len(os.Args) < 2 {
        usage(os.Stderr)
        os.Exit(2)
    }
    verb := os.Args[1]
    args := os.Args[2:]

    switch verb {
    case "pack":
        os.Exit(runPack(args))
    case "-h", "--help", "help":
        usage(os.Stdout)
        os.Exit(0)
    default:
        fmt.Fprintf(os.Stderr, "unknown verb: %q\n\n", verb)
        usage(os.Stderr)
        os.Exit(2)
    }
}

func usage(w io.Writer) {
    fmt.Fprintln(w, "Usage: goscape-cli <verb> [flags]")
    fmt.Fprintln(w)
    fmt.Fprintln(w, "Verbs:")
    fmt.Fprintln(w, "  pack    Build server-side packs (configs + compiled scripts).")
}
```

Adding a future verb is one case-arm + one `cmd_<verb>.go` file. Stays linear until verb count justifies a router library.

---

## §5. `pack` verb shape

```go
// cmd/goscape-cli/cmd_pack.go (sketch)
package main

import (
    "flag"
    "fmt"
    "log/slog"
    "os"

    "github.com/zsrv/goscape/pkg/pack"
    "github.com/zsrv/goscape/pkg/util/log"
)

func runPack(args []string) int {
    fs := flag.NewFlagSet("pack", flag.ContinueOnError)
    srcDir := fs.String("src-dir", "data/src", "Source content directory.")
    outDir := fs.String("out-dir", "data/pack", "Output directory.")
    dataPackDir := fs.String("datapack-dir", "", "Entity-type cache directory (default: --out-dir).")
    var logLevel slog.Level = slog.LevelInfo
    fs.TextVar(&logLevel, "log.level", logLevel, "Log severity (debug|info|warn|error).")
    logFormat := fs.String("log.format", "text", "Log format (text|json).")

    if err := fs.Parse(args); err != nil {
        return 2
    }

    logger, err := log.NewLogger(logLevel, *logFormat)
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
        return 1
    }

    dpd := *dataPackDir
    if dpd == "" {
        dpd = *outDir
    }

    logger.Info("packing", "src_dir", *srcDir, "out_dir", *outDir, "datapack_dir", dpd)
    if err := pack.PackAll(*srcDir, *outDir, dpd); err != nil {
        logger.Error("pack failed", "err", err)
        return 1
    }
    logger.Info("pack succeeded")
    return 0
}
```

Notes on the sketch:
- `flag.ContinueOnError` so `fs.Parse` returns instead of calling `os.Exit`. Lets `runPack` own the exit code.
- `slog.Level` implements `encoding.TextMarshaler`/`TextUnmarshaler`, so `fs.TextVar` parses level strings directly.
- No defaulting logic for `--datapack-dir` beyond "empty → outDir" so the doc'd `PackAll` common case is the zero-config one. Logged before invocation so users can verify which directory was actually used.
- No path validation here — `PackAll` and its stages already surface clear errors for missing/unreadable directories. Adding pre-checks would be redundant and could mask future genuine errors from the pipeline.

---

## §6. Build/Dockerfile updates

### Makefile

Existing target builds `cmd/goscape`. Add a sibling target and include both in any `build`/`build-image`/CI roll-up targets.

```makefile
goscape-cli: # build the cli binary
    CGO_ENABLED=0 go build -trimpath -o ./bin/goscape-cli ./cmd/goscape-cli
```

(Exact form follows the existing `goscape` target's style — see `Makefile` for the pattern this slice matches.)

### build.sh

Add a second `go build` invocation for `cmd/goscape-cli` alongside the existing `cmd/goscape` build.

### Dockerfile

In the builder stage, add a second `go build` for `goscape-cli` and a corresponding `COPY --from=builder ... /go/bin/goscape-cli /usr/local/bin/goscape-cli` in the runtime stage. No `ENTRYPOINT`/`CMD` change — the default container behavior continues to run `goscape` (the daemon).

---

## §7. Testing

Light. The dispatcher is glue; `pack.PackAll` and every stage it calls have their own unit + smoke coverage (NAI-208 through NAI-212).

| Test | Location | Purpose |
|---|---|---|
| `runPack` happy path | `cmd/goscape-cli/cmd_pack_test.go` (NEW) | With a `t.TempDir()` srcDir + outDir minimally seeded (one empty `scripts/` subdir; no configs to pack), invoke `runPack([]string{"--src-dir", srcDir, "--out-dir", outDir})` and assert it returns 0. |
| `runPack` propagates PackAll error | same file | Invoke `runPack` with `--src-dir` pointing at a path that does not exist; assert it returns 1. |
| Dispatcher unknown verb | `cmd/goscape-cli/main_test.go` (NEW) | Cannot easily test `main()` directly; instead extract the switch into a `dispatch(args []string, stdout, stderr io.Writer) int` helper and assert `dispatch([]string{"frobnicate"}, &b1, &b2)` returns 2 and writes the verb to stderr. |
| Dispatcher `-h` | same file | `dispatch([]string{"-h"}, ...)` returns 0 and writes "Usage:" to stdout. |

No integration test wires the binary end-to-end through `os/exec` — overkill for this slice, and `pack.PackAll` already has cross-package smoke coverage in `pkg/pack/`.

---

## §8. Deviations

None vs. TS — this slice has no TS counterpart. TS's pack entry point is `npm run pack` invoking `tools/pack/PackAll.ts` directly; goscape's binary structure (single daemon + sibling cli, both compiled) is naturally divergent and not subject to the parity gate.

Tempo-cli precedent informed the structure but isn't a parity target; goscape-cli is allowed to diverge freely from tempo-cli.

---

## §9. Follow-ups (not in this slice)

1. **`--target pack` daemon integration** — if there's ever a need to run pack from inside the running daemon (e.g., wiring ::rebuild cheat to in-process pack), revisit by hooking the cheat handler at `modules/world/handlers_game.go:handleClientCheat` directly to `pack.PackAll`. Does not require this binary.
2. **Further verbs** — jagfile inspection, single-script compile, content migration. Each lands as `cmd/goscape-cli/cmd_<verb>.go` + one `switch` arm. Reconsider router library at 3+ verbs.
3. **Shared `config.yaml`** — if users want pack paths in YAML, add a `goscape-cli`-specific config-loading helper later. Don't expand the daemon's `app.Config`.

---

## §10. Plan-author notes

- Project convention: prefix Go invocations with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per global `CLAUDE.md`.
- Project convention: commits use `git commit --no-gpg-sign`.
- `pkg/util/log.NewLogger(level slog.Level, format string) (*slog.Logger, error)` — signature confirmed at spec-write against `pkg/util/log/log.go:10`. §5 sketch matches.
- `Makefile` / `build.sh` / `Dockerfile` current shapes — read them before writing the plan task; the spec's §6 sketches are illustrative.
