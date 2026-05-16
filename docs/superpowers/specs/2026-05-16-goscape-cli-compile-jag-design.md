# `goscape-cli compile` + `goscape-cli jag` — two new verbs

**Date:** 2026-05-16
**Tech stack:** Go 1.26+ per `[[go_version]]`
**Cadence:** Standard (spec → plan → subagent-driven TDD). ~250 LOC across 4 new files + 1 small exporter addition in `pkg/pack/compiler`. Not compressed.
**Execution mode:** `subagent-driven-development` per `[[execution_mode_default]]`.
**TS canonical source:** none for the verb wiring; the underlying `runescript.Compile()` and `jagfile.LoadJagfile()` calls are TS-ported. No TS-parity gate applies to CLI surface.

---

## §1. Scope

Add two new verbs to `cmd/goscape-cli/`, the operational-tooling binary established in `[[goscape-cli-pack-close]]`:

- **`compile`** — runs the full runescript compiler pipeline on a single `.rs2` source file. Loads the same symbol set `PackAll` uses, so it catches type errors, unknown commands, and pointer issues — not just syntax. Two modes: write-output (default) or `--check` (diagnostics only).
- **`jag`** — parent verb with three sub-verbs (`list`, `extract`, `dump`) for inspecting `.jag` archives produced by the pack pipeline.

**In scope:**
- `cmd/goscape-cli/cmd_compile.go` + tests.
- `cmd/goscape-cli/cmd_jag.go` + tests (handles all three sub-verbs).
- Two new switch arms in `cmd/goscape-cli/main.go` dispatch + usage text update.
- One new exported helper in `pkg/pack/compiler` to expose the symbol-loading prep that's currently private inside `RunServerCompiler`.

**Out of scope:**
- Content migration verb (deferred per `[[goscape-cli-pack-close]]` follow-up #3).
- `compile`-style verb for other source types (`.obj`, `.npc`, etc.).
- A `jag write` / `jag pack` sub-verb (inspection-only this slice).
- Refactor of `dispatch` into a verb registry — three top-level verbs is still well within "linear switch is fine" per the original goscape-cli spec §4.
- `--force` override for `jag dump`'s non-empty-dir safety check.

---

## §2. Layout

| File | Status | Purpose |
|---|---|---|
| `cmd/goscape-cli/cmd_compile.go` | NEW | `runCompile` verb. |
| `cmd/goscape-cli/cmd_compile_test.go` | NEW | 3 tests for `runCompile`. |
| `cmd/goscape-cli/cmd_jag.go` | NEW | `runJag` parent + `runJagList`/`runJagExtract`/`runJagDump` sub-handlers. |
| `cmd/goscape-cli/cmd_jag_test.go` | NEW | 5 tests across the three sub-verbs. |
| `cmd/goscape-cli/main.go` | MOD | Two new switch arms + usage text update. |
| `cmd/goscape-cli/main_test.go` | MOD | Two dispatcher routing tests. |
| `pkg/pack/compiler/run_server_compiler.go` | MOD | Export `LoadCompilerSymbols(srcDir, dataPackDir)` factored from `RunServerCompiler`. |

No other files. No new packages.

---

## §3. Dispatcher changes

`cmd/goscape-cli/main.go` switch grows two arms:

```go
case "compile":
    return runCompile(rest, stderr)
case "jag":
    return runJag(rest, stdout, stderr)
```

`runCompile` takes only `(args, stderr)` — like `runPack`, all output is logger-style (goes to stderr per `[[goscape-cli-pack-close]]`).

`runJag` takes `(args, stdout, stderr)` — sub-verbs `list`, `extract`, and `dump` write content (entry listings, raw bytes) to stdout; only diagnostics/errors go to stderr.

`usage()` extends:

```
Verbs:
  pack       Build server-side packs (configs + compiled scripts).
  compile    Run the runescript compiler on a single .rs2 source file.
  jag        Inspect a .jag archive (list | extract | dump).
```

Trailing `Run goscape-cli <verb> -h for verb-specific flags.` line preserved.

---

## §4. `compile` verb

### Shape

```
goscape-cli compile [flags] <path>
```

Exactly one positional argument: path to a `.rs2` source file. Resolved as-given.

### Flags

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--src-dir` | string | `data/src` | Source content directory. Used to load entity-type symbols (.obj/.npc/.loc/...) and the script.pack symbol mapping. Same semantics as `pack`'s `--src-dir`. |
| `--datapack-dir` | string | `""` | Cache directory for binary entity-type loaders. Empty → defaults to `--src-dir`'s sibling `data/pack` per the same fallback `pack` uses (empty → `--out-dir`). For `compile`, empty → `data/pack` (not `--src-dir`). |
| `--check` | bool | `false` | Diagnostics-only mode. Writer still runs (mandatory in `runescript.Compile`) but output goes to a temp dir that's removed on exit. Without `--check`, writes to `--out-dir/server`. |
| `--out-dir` | string | `data/pack` | Output directory when `--check` is false. Ignored when `--check` is true. |
| `--log.level` | slog.Level | `info` | `debug|info|warn|error`. |
| `--log.format` | string | `text` | `text|json`. |

### Behavior

1. Parse flags + validate exactly one positional argument. Missing/multiple → exit 2 with "expected exactly one source path".
2. Build logger writing to `stderr`.
3. Call `compiler.LoadCompilerSymbols(srcDir, dataPackDir)` (new exporter; see §6). Returns `map[string]*runescript.CompilerTypeInfo`.
4. Decide writer output directory:
   - `--check` true → `os.MkdirTemp("", "goscape-cli-compile-*")`; defer `os.RemoveAll`.
   - else → `filepath.Join(outDir, "server")`.
5. Construct `runescript.Config{SourcePaths: []string{path}, Symbols: symbols, Writer: {Jag: {Output: writerOut}}}` and call `runescript.Compile(cfg)`.
6. Log "compile succeeded" / "compile failed" via logger; return exit code.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Compile succeeded (`-h`/`--help` also exits 0 via `flag.ErrHelp` short-circuit per `[[goscape-cli-pack-close]]`'s `pack -h` precedent). |
| 1 | Symbol load failed, compile failed, logger init failed, or temp-dir creation failed. |
| 2 | Flag parse error, missing/extra positional argument. |

### Why `--check` writes to a temp dir instead of skipping the writer

`runescript.Compile` requires a writer config — if both `Writer.Jag` and `Writer.Js5` are nil, it defaults to `JagWriterConfig{Output: "./data/pack/server"}`. Adding a `Writer.Discard` mode would touch upstream API for a small benefit. YAGNI: write-and-discard via temp dir adds <5 LOC in `runCompile` and zero upstream change.

---

## §5. `jag` verb

### Shape

```
goscape-cli jag list    <path.jag>
goscape-cli jag extract <path.jag> <entry> [--out <path>]
goscape-cli jag dump    <path.jag> --out <dir>
```

Sub-verb chosen because the three operations take different positional arguments. `runJag` is a mini-dispatcher; each sub-handler owns its own `flag.FlagSet`.

### `jag list`

Positional: exactly one `<path>`. No flags.

Loads via `jagfile.LoadJagfile(path)`, then for `i := 0; i < jf.FileCount; i++` writes one TAB-separated line to stdout:

```
<jf.FileName[i]>\t<jf.FileUnpackedSize[i]>\t<jf.FilePackedSize[i]>
```

No header. Pipeable to `column -t`, `sort`, `awk`.

### `jag extract`

Positional: exactly two — `<path>` then `<entry>`.

| Flag | Default | Notes |
|---|---|---|
| `--out` | `-` | Output path. `-` writes raw bytes to stdout (binary-safe). |

Reads via `jagfile.Read(name)` (hash-lookup by entry name). Missing entry → exit 1 with `"no such entry: <entry>"`. Existing `--out` path is overwritten (matches POSIX redirect semantics).

### `jag dump`

Positional: exactly one — `<path>`.

| Flag | Default | Notes |
|---|---|---|
| `--out` | (required) | Output directory. Must be either non-existent (will be created) or empty. Refuses to overwrite a non-empty dir. |

For each entry, writes its raw bytes to `<--out>/<jf.FileName[i]>`. Filenames are used as-given from `Jagfile.FileName[i]`; the Jagfile format uses simple alphanumeric+dot names (`hitmarks.dat`, `obj.idx`, etc.), so no path-traversal sanitization is needed in this slice. (If a future Jagfile contains a malicious entry name with `..` or `/`, this verb's safety would need re-evaluation; flagged as a non-blocking caveat.)

### Output channels

- All three sub-verbs write **content** to stdout, **diagnostics/errors** to stderr (via logger).
- `extract --out <path>` and `dump` write content to files instead of stdout; logger still goes to stderr.

### Exit codes (all three sub-verbs)

| Code | Meaning |
|---|---|
| 0 | Success (or `-h`/`--help`). |
| 1 | File not found, parse failure, missing entry, dir-not-empty, write error. |
| 2 | Flag parse error, missing/extra positional arg, missing/unknown sub-verb. |

---

## §6. `compiler.LoadCompilerSymbols` exporter

`pkg/pack/compiler/run_server_compiler.go` inlines the symbol-loading prep inside `RunServerCompiler`:

```go
loaders, err := loadConfigs(dataPackDir)
symbols, err := buildSymbolsCore(srcDir, loaders)
bridged := make(map[string]*runescript.CompilerTypeInfo, len(symbols))
for k, v := range symbols { bridged[k] = ToCompilerTypeInfo(v) }
translateCommandPointerNames(bridged["command"]) // NAI-212-D-POINTER-NAME-TRANSLATION
```

Expose this chain as a new exported helper. Implementation is additive — the existing `RunServerCompiler` / `runServerCompilerCore` flow stays unchanged because `runServerCompilerCore` has multiple test callers passing `*configLoaders` directly (see `pkg/pack/compiler/run_server_compiler_test.go`), and changing its signature would break them.

```go
// LoadCompilerSymbols assembles the symbol map the runescript compiler
// needs to type-check and codegen. Identical to the prep stages
// RunServerCompiler runs before invoking runescript.Compile.
//
// srcDir: directory containing scripts/ and pack/ subdirs.
// dataPackDir: cache directory with the 7 .dat/.idx pairs (read back
// the cache PackConfigs writes).
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

`RunServerCompiler` and `runServerCompilerCore` stay byte-identical. The minor prep-chain duplication (loaders + buildSymbolsCore + bridge + translate appears in both `LoadCompilerSymbols` and `RunServerCompiler`) is accepted for this slice — refactoring `RunServerCompiler` to delegate would shift `runServerCompilerCore`'s signature and break the test seam. A follow-up could collapse the duplication once the test seam is reconciled; not in scope here.

**No new tests** for `LoadCompilerSymbols` per se — it composes already-tested helpers. The new `compile` verb tests exercise it from the consumer side.

---

## §7. Testing

### `cmd_compile_test.go` — 3 tests

1. **`TestRunCompile_HappyPath`** — seed via `seedMinimalPackFixture` (already exists in `cmd_pack_test.go`, same package), call `runCompile([]string{"--src-dir", dir, "--check", filepath.Join(dir, "scripts", "helper.rs2")}, &stderr)`, assert exit 0 and stderr contains `"compile succeeded"`.
2. **`TestRunCompile_SourceError`** — seed minimal fixture but replace `helper.rs2` with a syntactically-invalid line (e.g., `[proc,broken]\nnot_a_command;\n`). Call with `--check`. Assert exit 1.
3. **`TestRunCompile_MissingPath`** — `runCompile([]string{}, &stderr)`. Assert exit 2 and stderr mentions "exactly one source path".

`seedMinimalPackFixture` is reused as-is from `cmd_pack_test.go` (same `package main`).

### `cmd_jag_test.go` — 5 tests

Test fixture: a small in-test helper that builds the same bytes `jagfile.MakeTestJagfile` builds (`hitmarks.dat` entry with `0xFF` payload), saves to `t.TempDir()/test.jag` via `jagfile.Jagfile.Save(...)`. Inlined — ~15 LOC. Not extracted to a public helper.

1. **`TestRunJag_List`** — fixture, call `runJag([]string{"list", path}, &stdout, &stderr)`, assert exit 0 and stdout matches `"hitmarks.dat\t1\t1\n"`.
2. **`TestRunJag_ExtractToStdout`** — call `runJag([]string{"extract", path, "hitmarks.dat"}, &stdout, &stderr)`, assert exit 0 and `stdout.Bytes()` equals `[]byte{0xFF}`.
3. **`TestRunJag_ExtractToFile`** — `--out` a temp path, assert exit 0 and file contents match.
4. **`TestRunJag_ExtractMissingEntry`** — extract `"nope.dat"`, assert exit 1 and stderr contains `"no such entry"`.
5. **`TestRunJag_Dump`** — `--out` an empty dir, assert exit 0 and `<dir>/hitmarks.dat` exists with correct bytes.

### `main_test.go` — 2 new dispatcher tests

6. **`TestDispatch_CompileRouting`** — `dispatch([]string{"compile", "--no-such-flag"}, ...)` returns 2 and stderr mentions `"no-such-flag"`. Mirrors `TestDispatch_PackRouting` shape.
7. **`TestDispatch_JagRouting`** — `dispatch([]string{"jag"}, ...)` returns 2 and stderr mentions the missing sub-verb.

Total new tests: 10. Coverage matches the original spec's §7 "light" stance and adds enough sub-verb coverage to pin `jag`'s internal mini-dispatcher.

---

## §8. Deviations

None vs. TS — neither verb has a TS counterpart. TS exposes equivalents through ad-hoc `npm run` scripts; goscape's CLI binary structure (already established) is naturally divergent.

`compile`'s `--check` writes to a temp dir instead of skipping the writer (justified in §4). This is a goscape-only ergonomic choice, not a behavioral divergence from any upstream.

`jag dump`'s refusal to overwrite non-empty `--out` is goscape-only safety; no equivalent guard exists in TS.

---

## §9. Follow-ups (not in this slice)

1. **Content migration verb** — original goscape-cli follow-up #3 from `[[goscape-cli-pack-close]]`. Still open. Would need its own brainstorm pass to define what migration means here.
2. **`jag` path-traversal hardening** — if a future Jagfile is sourced from an untrusted producer, `dump` should sanitize entry names to prevent writing outside `--out`. Current Jagfile sources are all goscape-produced, so this is non-blocking.
3. **`compile`-style verb for other source types** (`.obj`, `.npc`, `.loc`, etc.) — useful for config authors. Each would need its own loader path. Not requested.
4. **Verb registry / router library** — at 4+ verbs (we're at 3 after this slice), revisit a thin verb-registry refactor. Original spec §4 set the threshold at 3+.

---

## §10. Plan-author notes

- Project convention: prefix Go invocations with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per global `CLAUDE.md`.
- Project convention: commits use `git commit --no-gpg-sign`.
- `compiler.RunServerCompiler` signature confirmed at spec-write: `(srcDir, outDir, dataPackDir string) error`. Internal `loadConfigs`, `buildSymbolsCore`, `ToCompilerTypeInfo`, `translateCommandPointerNames` are unexported — the new `LoadCompilerSymbols` exporter is the entire public-API addition.
- `jagfile.LoadJagfile`, `Jagfile.Read`, `Jagfile.FileCount`, `Jagfile.FileName`, `Jagfile.FileUnpackedSize`, `Jagfile.FilePackedSize`, `Jagfile.Save` confirmed at spec-write per `pkg/io/jagfile/jagfile.go`.
- `MakeTestJagfile` lives in `_test.go` — invisible cross-package per `[[test_export_underscore_test_visibility]]`. The test fixture in `cmd_jag_test.go` duplicates the byte construction inline.
- `runescript.Compile` requires at least one of `Writer.Jag` / `Writer.Js5`. The `--check` mode writes to a temp dir and removes it on exit.
- `seedMinimalPackFixture` already exists in `cmd_pack_test.go` (same `package main`); `cmd_compile_test.go` reuses it directly.
- The new verbs follow the same flag-parsing pattern as `runPack`: `flag.ContinueOnError` + `fs.SetOutput(stderr)` + `errors.Is(err, flag.ErrHelp)` short-circuit for exit 0 on `-h`/`--help`.
