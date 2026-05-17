# `goscape-cli smoke-pack` — real-datapack survival smoke for `packall.PackAll`

**Date:** 2026-05-17
**Tech stack:** Go 1.26+ per `[[go_version]]`.
**Cadence:** Compressed per `[[compressed_cadence]]` — ~150–200 LOC across one new verb file + companion test file. One spec doc, one short plan, no formal sub-spec review cycle.
**Execution mode:** `subagent-driven-development` per `[[execution_mode_default]]`.
**TS canonical source:** none — TS `packAll()` is the production pipeline (already ported). The smoke verb is a goscape-native developer tool; no TS-parity gate applies.

---

## §1. Scope

Add a new verb `smoke-pack` to `cmd/goscape-cli` that drives `packall.PackAll` against an arbitrary content directory (in practice: `/home/owner/Code/github.com/LostCityRS/Content`, ~200MB) and produces a per-stage outcome report. The verb runs all 10 PackAll stages best-effort — i.e., logs and continues past stage errors rather than failing fast — so that one developer-driven run surfaces every stage that breaks against the real datapack, not just the first.

**Why this matters now.** Until NAI-213, PackAll had only been exercised against unit-test fixtures. Expected divergences from MEMORY against real Content include the pixpack tiled-sheet semantic (`opt`-file x/y coordinate space) and the BUILDVERIFY-INTERFACE CRC residual (`NAI-213-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE`). Additional gaps are likely. This smoke is the first end-to-end exercise of the full PackAll pipeline against unseen input.

**In scope:**
- New verb `smoke-pack` registered in `cmd/goscape-cli/main.go` `verbs` slice.
- Continue-on-error driver that re-lists the 10 PackAll stages inline.
- Per-stage structured logging (`stage_start`, `stage_done`, `stage_err`) with elapsed time, output file count, output byte total.
- End-of-run human-readable summary table on stdout.
- Optional preservation of `--out-dir`.
- Unit tests for flag parsing, exit codes, and the summary-formatter against a synthetic fixture.

**Out of scope (Phase 2 deferred follow-up):**
- Byte-diff against an Engine-TS reference build (`bun tools/pack/PackAll.ts` output).
  Reference output is gitignored in Engine-TS and must be regenerated locally; byte-diff scaffolding is large enough to deserve its own spec, and is meaningful only once Phase 1 survival is clean enough that most stages produce output. A `--reference-dir` flag is **not** added in this spec — Phase 2 will add it.
- Refactoring `packall.PackAll` to expose a stage list or progress hook. Stage order is duplicated inline in the smoke driver. If we keep the smoke long-term, a follow-up can DRY this.
- Continuous-integration hookup. The smoke is intentionally manual/developer-driven; it depends on a 200MB checkout not in the goscape repo.
- Real-Content invocation as part of `go test ./...`. Unit tests use a tiny synthetic fixture; the operator runs the verb manually against Content.

---

## §2. Layout

| File | Purpose |
|---|---|
| `cmd/goscape-cli/cmd_smoke_pack.go` (NEW) | `smoke-pack` verb: flag parsing, logger init, stage driver, summary table. |
| `cmd/goscape-cli/cmd_smoke_pack_test.go` (NEW) | Flag-parse + exit-code + summary-formatter tests. |
| `cmd/goscape-cli/main.go` (MODIFIED) | Register `smoke-pack` in `verbs` slice. |

No changes to `pkg/packall/`, `pkg/pack/`, or other packages. The smoke driver imports each stage subpackage directly, mirroring the import set of `pkg/packall/packall.go`.

---

## §3. Invocation surface

```
goscape-cli smoke-pack --content-dir <dir> [flags]
```

### Flags

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--content-dir` | string | (required) | Source content directory. Passed to stages as `srcDir`. If empty or path does not exist, exit 3 (setup error). |
| `--out-dir` | string | `""` | Output directory. Empty → driver creates a fresh `os.MkdirTemp("", "goscape-smoke-pack-*")` directory and deletes it on exit unless `--keep`. Non-empty path is used as-given; the driver does not create parents and does not delete on exit. |
| `--datapack-dir` | string | `""` | Cache directory for entity-type loaders. Empty → defaults to the effective `--out-dir` (same convention as `pack` verb). |
| `--keep` | bool | `false` | When set, do not delete an auto-created `--out-dir` on exit. Logged at end so the operator can inspect. No effect on operator-provided `--out-dir`. |
| `--stop-on-error` | bool | `false` | When set, exit at the first failing stage with exit code 1 (matches `pack` verb fail-fast semantic). Default: best-effort — log and continue past stage errors. |
| `--log.level` | `slog.Level` | `info` | Same shape as `pack` verb (`debug`/`info`/`warn`/`error`). |
| `--log.format` | string | `text` | Same shape as `pack` verb (`text`/`json`). |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | All 10 stages succeeded. |
| 1 | At least one stage failed in best-effort mode, OR the first failing stage in `--stop-on-error` mode. |
| 2 | Flag parse error. |
| 3 | Setup error: `--content-dir` missing/unreadable, logger init failed, or auto-out-dir mkdtemp failed. |

The 0/1 split aligns the verb with `go test` semantics: green run = green exit.

---

## §4. Stage list (canonical, mirrors `packall.PackAll`)

The driver runs these in order. Each stage runs against the effective `srcDir`, `outDir`, and (where used) `dataPackDir`. The `reg` produced by `PackConfigs` is the same `*pack.Registry` consumed by `ClientInterface`, `Texture`, `Sound`, and `Graphics`.

| # | Stage name (report key) | Call |
|---|---|---|
| 0 | (pre) | `pack.ClearFsCache()` — not a reported stage; no error path. |
| 1 | `PackConfigs` | `pack.PackConfigsForRegistry(srcDir, outDir)` → returns `*pack.Registry` |
| 2 | `ClientInterface` | `clientinterface.Pack(reg, srcDir, outDir)` |
| 3 | `RunServerCompiler` | `compiler.RunServerCompiler(srcDir, outDir, dataPackDir)` |
| 4 | `Title` | `sprites.PackTitle(srcDir, outDir)` |
| 5 | `Media` | `sprites.PackMedia(srcDir, outDir)` |
| 6 | `Texture` | `sprites.PackTexture(reg, srcDir, outDir)` |
| 7 | `Wordenc` | `wordenc.Pack(srcDir, outDir)` |
| 8 | `Sound` | `audio.PackSound(reg, srcDir, outDir)` |
| 9 | `Graphics` | `graphics.Pack(reg, srcDir, outDir)` |
| 10 | `Midi` | `audio.PackMidi(srcDir, outDir)` |
| 11 | `Maps` | `maps.Pack(srcDir, outDir)` |

**`PackConfigs` is special.** If it fails, the driver records the error in the report and skips all downstream stages (every subsequent stage either uses `reg` directly or expects PackConfigs to have prepared the output dir). The summary table still prints; downstream stages render with status `SKIP`.

**SKIP semantics summary.** A stage renders as `SKIP` when, and only when, the driver decided not to call it. Three cases produce SKIPs:

1. `PackConfigs` failed (any mode) — every downstream stage is SKIP.
2. `--stop-on-error` is set and an earlier stage failed — every stage after the first ERR is SKIP.
3. (Reserved for Phase 2; no other SKIP triggers in Phase 1.)

In default best-effort mode, a non-`PackConfigs` ERR does **not** trigger SKIPs; downstream stages still run. The §6 example illustrates this: `RunServerCompiler ERR` is followed by `Title OK`, etc.

The driver does **not** invoke `packall.PackAll` because PackAll is fail-fast. Stage order is duplicated inline; an accompanying doc-comment on the verb references `pkg/packall/packall.go` as the production source-of-truth. A `nai_213_smoke_stage_parity` pin test (against `packall.PackAll` body or a comment-fenced stage list) is **not** added — drift is acceptable given the smoke is exploratory; we will rediscover the order if it ever changes. If this turns out to be load-bearing, the follow-up to DRY into a shared stage list (see §1 Out of scope) addresses it.

---

## §5. Per-stage telemetry

Each stage's outcome is captured as:

```go
type stageResult struct {
    Name        string        // "PackConfigs", etc.
    Status      stageStatus   // OK | ERR | SKIP
    Elapsed     time.Duration // wall time from immediately-before to immediately-after the call
    OutputFiles int           // count of regular files under outDir at end of stage (cumulative; see below)
    OutputBytes int64         // sum of regular-file sizes under outDir at end of stage (cumulative)
    Err         error         // nil unless Status == ERR
}
```

**Cumulative vs per-stage output measurement.** Stages share the same `outDir` and each writes a subset of files. Computing the *delta* would require walking `outDir` before and after each stage. For Phase 1 we take the simpler approach: walk `outDir` once at end of each stage and report cumulative totals. The summary table shows growth at each row, which gives the operator an at-a-glance picture of which stages produced output and which were no-ops/errors. Phase 2's byte-diff will provide per-file resolution.

**Logging.** Each stage emits one structured log line at start and one at end (info-level for OK, error-level for ERR). Fields: `stage`, `elapsed_ms`, `files`, `bytes`, and on err, `err`. The operator can `--log.format=json` to feed jq.

---

## §6. Summary table

After all stages run (or after the first ERR if `--stop-on-error`), the driver prints to **stdout**:

```
STAGE              STATUS  ELAPSED   FILES   BYTES
PackConfigs        OK      1.21s     30      1245678
ClientInterface    OK      0.42s     32      1689320
RunServerCompiler  ERR     0.08s     32      1689320   pointer error: ...
Title              OK      0.15s     33      1701234
Media              OK      0.51s     34      1820000
Texture            OK      0.66s     35      1900000
Wordenc            OK      0.03s     36      1903456
Sound              OK      0.22s     37      2100000
Graphics           OK      0.31s     38      2200000
Midi               OK      0.18s     39      2300000
Maps               OK      4.87s     78      52000000

Result: 9 OK, 1 ERR, 0 SKIP    total elapsed: 8.74s    out-dir: /tmp/goscape-smoke-pack-12345 (will be kept; --keep)
```

Column-width heuristic: each column is fixed-width per its widest stage-name / formatted value, with a 2-space gutter. `BYTES` is rendered raw (no MB/KB conversion) for grep/diff stability. The trailing `out-dir:` line is unconditional; the `(will be kept; --keep)` suffix only appears when the driver auto-created the dir AND `--keep` was set. When `--out-dir` was operator-supplied, the suffix is just the path.

**Stdout vs stderr split.** Slog output goes to **stderr** (matches `pack` verb). The summary table goes to **stdout** so that operators can `2>/dev/null` to see just the table, or `>summary.txt` to capture it.

---

## §7. Test plan

Unit tests live in `cmd_smoke_pack_test.go` and exercise:

1. **Flag parse — `-h` exits 0** with usage on stdout.
2. **Flag parse — unknown flag** exits 2 with usage on stderr.
3. **Missing `--content-dir`** exits 3 with a clear error on stderr.
4. **Non-existent `--content-dir`** exits 3 with a clear error on stderr.
5. **Happy path against a tiny synthetic fixture.** Build a minimal fixture dir under `t.TempDir()` containing the smallest inputs each of the 10 stages will accept (or accept-and-no-op). Most stages will produce zero output but should exit OK against an empty-but-valid input dir. Expected behavior: exit 0, summary table on stdout, slog `stage_done` lines on stderr.
   - If some stages crash on an empty-but-valid input dir, those become known-deviations of the SUT itself, not of the smoke; the test asserts the smoke surfaces them rather than crashing. Concretely: the test expects exit 1 if any stage fails, exit 0 if all pass, and asserts the summary table is well-formed in either case.
6. **Synthetic fixture + `--stop-on-error`** — when the first stage is induced to fail (e.g., by deleting a required input subdir after fixture setup), exit code is 1 and downstream stages appear as `SKIP` in the summary.
7. **`--keep` preserves auto-out-dir.** After the verb returns, the auto-created out-dir still exists. (Cleanup is the test's responsibility via `t.TempDir`-rooted parent.)
8. **Operator-supplied `--out-dir` is never deleted.** Run verb with explicit out-dir under `t.TempDir()`; assert directory still exists after return.

Real-Content runs are **not** tested in CI — they require the 200MB Content checkout. The verb is exercised manually by the operator. The MEMORY entry for this close will record the operator's first-run divergence list.

---

## §8. Non-goals / deferred

- **Byte-diff against Engine-TS reference** — Phase 2, separate spec. Likely flag: `--reference-dir`.
- **Per-file output deltas** — also Phase 2.
- **In-process `::rebuild` wiring** — separate initiative; the smoke verb runs out-of-process.
- **Smoke against `packWorldmap`** — TS `map/Worldmap.ts` (682 LOC) is not in `packAll`; tracked separately.
- **CI integration** — the verb is local-developer-driven for now.

---

## §9. Acceptance criteria

- `goscape-cli smoke-pack -h` prints usage and exits 0.
- `goscape-cli smoke-pack` (no flags) exits 3 (missing `--content-dir`) with a clear error.
- `goscape-cli smoke-pack --content-dir <Content>` against `/home/owner/Code/github.com/LostCityRS/Content`:
  - Completes without panicking the binary regardless of per-stage outcomes.
  - Prints a summary table with one row per stage.
  - Exits 0 if all 10 stages succeed, 1 otherwise.
- All listed unit tests pass under `go test ./cmd/goscape-cli/...` and `go test -race ./cmd/goscape-cli/...`.
- A MEMORY entry is written at close summarizing the first real-Content run: which stages survived, which diverged, which need follow-up sub-specs. This entry is what makes the smoke valuable.

---

## §10. Deviation accounting

No anticipated deviation tags. The smoke verb is goscape-native; there is no TS counterpart to diverge from.

If the operator's first real-Content run surfaces gaps in the SUT (the packers themselves), those are tracked as their own follow-up sub-specs, not as deviations in this spec.
