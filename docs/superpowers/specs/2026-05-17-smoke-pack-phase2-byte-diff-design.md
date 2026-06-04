# `goscape-cli smoke-pack` Phase 2 — byte-diff against an Engine-TS reference build

**Date:** 2026-05-17
**Tech stack:** Go 1.26+ per `[[go_version]]`.
**Cadence:** Compressed per `[[compressed_cadence]]` — ~250–350 LOC across one new file + companion test file + small `cmd_smoke_pack.go` edits. One spec doc, one short plan, light TDD via subagent-driven-development.
**Execution mode:** `subagent-driven-development` per `[[execution_mode_default]]`.
**TS canonical source:** none — the reference is the *byte output* of `bun tools/pack/PackAll.ts` against the same `Content` checkout. There is no TS code to mirror; this is a goscape-native diff harness.

---

## §1. Scope

Add a `--reference-dir <path>` flag to the existing `goscape-cli smoke-pack` verb. When set, after each PackAll stage runs, byte-diff every file the stage wrote (or modified) under `--out-dir` against the same relative path under `--reference-dir`. Report a per-stage `DIFF` count in the summary table and a bounded "Diff details" section listing the first non-matching byte for each diverged file.

**Why this matters now.** Phase 1 shipped 11 stages running best-effort against real Content. Phase 1 surfaced exactly one byte-level latent bug (the clientinterface pjstr-terminator fix at `6312712f`) by happenstance — a downstream stage panicked. Most stages emit data that no test currently inspects, so silent byte drift can accumulate. Phase 2 turns the smoke into a real fidelity check: every file is compared against the canonical TS output.

The reference is regenerated locally by the operator (`bun tools/pack/PackAll.ts` against the same `Content` checkout); it is gitignored in Engine-TS at `data/pack/`. Engine-TS and goscape both write to the same `{client,server}/...` layout, so the diff is a straightforward relative-path lookup.

**In scope:**
- New flag `--reference-dir <path>`; empty value preserves Phase 1 behavior exactly.
- Per-stage delta tracking: snapshot outDir before and after each stage; diff only files the stage added or modified.
- Per-stage byte-comparison against `<reference-dir>/<relpath>`; record file path + first-mismatch offset + got/want bytes.
- Four diff kinds: `DIFF` (sizes match, bytes differ), `SIZE` (sizes differ), `MISS` (file absent from reference), `ERR` (refDir-side read failure on a present file).
- New `DIFF` column on the summary table.
- Bounded "Diff details:" trailing section: first 10 diverged files per stage, then `... +K more` summary line.
- Unit tests for snapshot, delta, and diff routines.
- Integration test that runs the smoke against a fake content tree + matching reference and asserts 0 diffs, then perturbs one file and asserts 1 DIFF in the correct stage.

**Out of scope:**
- `--strict-diff` (exit non-zero when DIFFs exist). DIFFs are reported, not enforced. Add later if needed.
- Reverse direction (files present in reference but absent from outDir). The smoke audits what goscape produced, not what TS produced.
- Per-byte hex dumps or contextual windows around mismatches. The first-mismatch offset + 1 byte each side is the smallest useful pinpointing.
- CI integration. Reference regeneration is operator-driven.
- Refactoring stage attribution into `pkg/packall`. Stages are attributed via path-snapshot delta inside the smoke driver, not via a callback interface.

---

## §2. Layout

| File | Purpose |
|---|---|
| `cmd/goscape-cli/cmd_smoke_pack.go` (MODIFIED) | New `--reference-dir` flag; threading `refDir` into `runStages`; new `DIFF` column in `printSummary`; new "Diff details:" trailing block. |
| `cmd/goscape-cli/smoke_pack_diff.go` (NEW) | `stageSnapshot`, `snapshotOutDir`, `deltaFiles`, `fileDiff`, `diffAgainstReference`. |
| `cmd/goscape-cli/smoke_pack_diff_test.go` (NEW) | Unit tests for snapshot/delta/diff. |
| `cmd/goscape-cli/cmd_smoke_pack_test.go` (MODIFIED) | One integration test: matching reference → 0 diffs; perturbed reference → ≥1 DIFF attributed to a specific stage. |

No changes to `pkg/`, `internal/`, or `modules/`.

---

## §3. Invocation surface

```
goscape-cli smoke-pack --content-dir <dir> [--reference-dir <dir>] [flags...]
```

### New flag

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--reference-dir` | string | `""` | Absolute or relative path to a directory containing a reference pack output (typically `…/Engine-TS/data/pack`). Empty → byte-diff is disabled; Phase 1 behavior. Non-empty path must exist and be a directory or exit 3 (setup error). |

All Phase 1 flags retain their current semantics.

### Exit codes

Unchanged from Phase 1:
- `0` — all stages OK (DIFF count irrelevant).
- `1` — at least one stage ERR.
- `2` — flag parse error.
- `3` — setup error (incl. unreadable `--reference-dir`).

DIFFs do **not** affect exit code. Reasoned omission: the developer audits the report visually; mechanical strict-mode comes later.

---

## §4. Architecture

### Per-stage delta tracking

`runStages` already walks `outDir` after each stage to compute `(files, bytes)` for the report. Phase 2 promotes this to a richer snapshot.

```go
type stageSnapshot map[string]string // relpath → sha256 hex
```

After each *successful* stage (`stageOK`), the driver:
1. Calls `snapshotOutDir(outDir)` → `next`.
2. Computes `delta := deltaFiles(prev, next)` — relpaths present in `next` whose hash differs from `prev` (added or modified).
3. If `refDir != ""`, for each relpath in `delta`, calls `diffOneFile(filepath.Join(outDir, relpath), filepath.Join(refDir, relpath))` → optional `fileDiff`.
4. Stores resulting `[]fileDiff` on `stageResult.Diffs`.
5. Replaces `prev` with `next` for the next iteration.

Failed stages do not produce a diff list (`Diffs == nil`).

### snapshotOutDir

Walks `outDir` recursively. For each regular file, reads bytes, computes sha256, stores `relpath → sha256-hex` in the map. Symbolic links, directories, sockets, and special files are skipped. Errors reading individual files propagate as walk errors. Missing `outDir` returns an empty map and nil error.

Sha256 is the cheapest reliable change detector across stages; a full byte-by-byte file compare during snapshot would more than double the per-stage cost and isn't needed for delta detection.

### deltaFiles

```go
func deltaFiles(prev, next stageSnapshot) []string
```

Returns the slice of relpaths in `next` whose value differs from `prev[relpath]` (covers added: `prev[relpath] == ""`, and modified: hashes differ). Result is sorted alphabetically for deterministic per-stage diff output.

### diffOneFile

```go
func diffOneFile(outPath, refPath string) (*fileDiff, error)
```

Logic:
1. `os.Stat(refPath)` — if `ErrNotExist`, return `{Kind: kindMiss}`.
2. `os.ReadFile(outPath)`, `os.ReadFile(refPath)` — both as `[]byte`.
3. If sizes differ, return `{Kind: kindSize, OutSize, RefSize}`.
4. Iterate bytes; on first mismatch return `{Kind: kindDiff, Offset, Got, Want}`.
5. Otherwise return `nil` (match).

`os.ReadFile` is fine: largest individual pack outputs are <50MB (maps.jag tops the size chart in Phase 1 at ~18MB total, spread across hundreds of files). If a future stage produces a single huge file, swap to streaming compare; not now.

### fileDiff type

```go
type fileDiff struct {
    Path    string // relpath under outDir / refDir
    Kind    string // "DIFF" | "SIZE" | "MISS" | "ERR"
    Offset  int64  // first-mismatch offset (Kind=="DIFF"); 0 otherwise
    Got     byte   // outDir byte at Offset (Kind=="DIFF")
    Want    byte   // refDir byte at Offset (Kind=="DIFF")
    OutSize int64  // Kind=="SIZE"
    RefSize int64  // Kind=="SIZE"
    Note    string // Kind=="ERR"; human-readable reason (e.g. wrapped errno)
}
```

### Reporting

**Summary table** gains a `DIFF` column. When `refDir == ""`, the column is omitted (Phase 1-identical). When `refDir != ""`, the column shows `len(stageResult.Diffs)`; SKIP rows render `-`; ERR rows render `-` if Diffs==nil else the count.

```
STAGE              STATUS  ELAPSED  FILES  BYTES     DIFF  ERR
PackConfigs        OK      273ms    39     617147    0
ClientInterface    OK      42ms     41     866419    2
RunServerCompiler  ERR     3.936s   41     866419    -     check-pointers: …
...
```

**Trailing "Diff details:"** block printed after the Result line, only when any stage has a non-empty `Diffs`. One line per `fileDiff`:

```
Diff details:
  ClientInterface  DIFF   client/interface/inv.dat              offset=12 got=0x2a want=0x1f
  ClientInterface  SIZE   client/interface/lookup.dat           out=1024 ref=1100
  Texture          MISS   client/textures/missing.dat
  ... +3 more across 1 stage
```

**Truncation rule:** for each stage, emit up to 10 fileDiff lines; suppress the rest. Print one trailing `... +K more across N stage(s)` line aggregating any truncated diffs. Threshold 10 is a YAGNI default — tune later if the smoke gets noisier.

---

## §5. Test strategy

### Unit tests (`smoke_pack_diff_test.go`)

| Test | What it pins |
|---|---|
| `TestSnapshotOutDir_EmptyDir` | Missing directory → `(empty map, nil err)`. |
| `TestSnapshotOutDir_RegularFiles` | Hashes regular files; ignores subdirectory entries themselves; uses forward-slash relpaths (`filepath.ToSlash`). |
| `TestSnapshotOutDir_NestedDirs` | Recursive walk; nested file relpath joined correctly. |
| `TestDeltaFiles_AdditionsAndModifications` | Returns added files; returns files whose hash changed; ignores unchanged files; result is sorted. |
| `TestDeltaFiles_DeletionsIgnored` | Files present in `prev` but absent from `next` are NOT in the delta (we never diff deletions). |
| `TestDiffOneFile_MatchReturnsNil` | Identical bytes → `nil, nil`. |
| `TestDiffOneFile_ByteMismatch` | Same size, different bytes → `Kind=DIFF, Offset, Got, Want` correct. |
| `TestDiffOneFile_SizeMismatch` | Different sizes → `Kind=SIZE, OutSize, RefSize`; no offset spelunking. |
| `TestDiffOneFile_MissingReference` | refPath absent → `Kind=MISS`. |
| `TestDiffOneFile_MissingOutput` | outPath absent → propagated error (this case shouldn't happen because we only diff files we just observed in the snapshot — assert defensive error behavior anyway). |

### Integration test (`cmd_smoke_pack_test.go`)

Adds one test using the existing fake-content / minimal-smoke harness pattern:

`TestRunSmokePack_RefDir_CleanThenDiverged`:
1. Build a fake content directory + invoke smoke-pack with `--out-dir=A --reference-dir=A` (self-reference) → expect 0 diffs total.

   (Self-reference is the cheapest matching baseline; we don't have to maintain a separate handcrafted reference tree.)
2. Perturb one file in `A` (e.g. flip a byte in a known-stable output), invoke `smoke-pack --out-dir=B --reference-dir=A`, assert exactly 1 DIFF attributed to the stage whose path-prefix matches the perturbed file.

Test runs under `testing.Short()` if Content is unavailable — gracefully skip rather than fail. The existing Phase 1 tests already gate on this; reuse that gate.

---

## §6. Edge cases & decisions

| Case | Behaviour |
|---|---|
| `--reference-dir` empty | Phase 1 behavior; DIFF column omitted. |
| `--reference-dir` missing/not-a-dir | Exit 3 (setup error). |
| Reference file unreadable (permissions) | Propagated as `fileDiff{Kind: "ERR", Path: relpath, Note: <err string>}`. Counted in the DIFF column alongside DIFF/SIZE/MISS. |
| Output file >50MB (unlikely today) | `os.ReadFile` reads it all. If memory becomes an issue, swap to bufio.NewReader compare; YAGNI. |
| Stage panic (handled by `safeRun`) | Stage is ERR; `Diffs == nil`; DIFF column shows `-`. |
| Same file modified by two stages | Each stage gets attributed for the files it modified relative to the prior snapshot. Modification is detected by hash change. This is the right behavior: if `Texture` overwrites a file `Title` wrote, both stages contribute a diff for that path. |
| Newly-created subdirectory with zero files | Excluded by `d.Type().IsRegular()` filter. |
| Out-dir cleanup with `--keep=false` | Unaffected — cleanup runs after summary, after snapshots have been read into memory. |

---

## §7. Deviations / open questions

None at spec time. The diff harness has no TS counterpart; this is purely a goscape-native developer tool, so the "TS-faithful" gate doesn't apply.

---

## §8. Acceptance

- `go test ./cmd/goscape-cli/...` passes.
- `goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/Content --reference-dir /home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack` produces a per-stage DIFF count and a Diff details section. Specific divergence numbers are not pinned — Phase 2's job is to make divergences visible, not to fix them.
- Without `--reference-dir`, output is bit-identical to current Phase 1 (no new column, no trailing block).
