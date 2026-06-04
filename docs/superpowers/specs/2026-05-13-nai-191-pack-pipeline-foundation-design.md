# NAI-191 — Pack-pipeline source-side foundation

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/tools/pack/FsCache.ts`, `tools/pack/Parse.ts`, `tools/pack/NameMap.ts`, `tools/pack/PackFileBase.ts`, and the format-agnostic helpers in `tools/pack/PackFile.ts` (file-freshness cohort + generic `crawlConfigNames`).
**Predecessors:** NAI-190 (`World.reload()` port + `::reload` cheat). NAI-190 close commit `e7b2950` notes `::rebuild` as the lone remaining `ClientCheatHandler.ts` carryforward, blocked on the pack-pipeline arc opened here.
**HEAD at spec-write:** `e7b2950`

## §1 Goal

Open the native Go port of the source-side packing pipeline (`tools/pack/*.ts`) with a strictly format-agnostic foundation slice. NAI-191 ships a new `pkg/pack/` package containing five files of pure infrastructure: filesystem caches, source-file parsing, name-to-ID registry data structure, file-freshness helpers, and a generic `[header]`-block crawler. No per-config-format knowledge, no validator wiring, no production callsites, no user-visible behavior change.

The slice is the foundation on which every NAI-192+ per-config packer (`obj`, `loc`, `npc`, `seq`, `enum`, …) and eventually the `.rs2` bytecode compiler, `PackAll` orchestrator, `DevThread` file-watcher, and `::rebuild` cheat wiring will be built.

## §2 Out of scope

| Concern | TS location | Why deferred |
|---|---|---|
| All `validate*` functions | `tools/pack/PackFile.ts:8-228` (`validateFilesPack`, `validateImagePack`, `validateConfigPack`, `validateCategoryPack`, `validateInterfacePack`, `regenScriptPack`) | Encode per-config-format knowledge (`.flo`/`.obj`/`.npc`/`cert_` prefix, `engine.rs2` special-case, `interface.pack` file/component grammar). Land alongside their respective per-config packers in NAI-192+. |
| Module-level pack singletons (26 of them: `AnimPack`, `BasePack`, …, `VarsPack`) | `tools/pack/PackFile.ts:231-256` | Construct `PackFile(type, validator, args)` instances that wire format-specific validators. Land per packer in NAI-192+. |
| `revalidatePack()` orchestrator | `tools/pack/PackFile.ts:258-285` | Walks all 26 singletons. Lands when the last per-config packer slice closes. |
| `crawlConfigCategories` | `tools/pack/PackFile.ts:331-374` | Format-specific (`.loc`/`.npc`/`.obj`, `category=` line grammar). Lands with the first packer that needs categories. |
| `BUILD_VERIFY*` env-flag plumbing | TS `Environment.BUILD_VERIFY` / `BUILD_VERIFY_FOLDER` / `BUILD_VERIFY_PACK` | Read only by validators. Defers with them. |
| `.rs2` bytecode compiler | `tools/pack/Compiler.ts` (368 LOC) | Self-contained sub-spec on its own track. |
| `PackAll` orchestrator | `tools/pack/PackAll.ts` (52 LOC) | Calls every per-config packer; lands after they exist. |
| `DevThread` file watcher | `src/cache/DevThread.ts` (~100 LOC) | Runs in a Node worker thread; Go analogue (fsnotify + goroutine) lands as the penultimate arc slice. |
| `::rebuild` cheat handler wiring | `src/network/game/client/handler/ClientCheatHandler.ts:151-153` | Closes the arc; depends on the full pack pipeline + DevThread analogue. |
| JAG-format binary writer | n/a (already done) | `pkg/io/jagfile/jagfile.go` already has full read/write + BZip2. No work required in this arc. |
| Sprite/graphics/interface/map/midi/sound packers | `tools/pack/{PixPack,chat,graphics,interface,map,midi,sound,sprite}/*.ts` | Each is its own sub-spec under the NAI-192+ track. |

## §3 Pre-flight audit

Per `controller_preflight` and `risk_register_premise_grep`, every premise below was re-verified against HEAD `e7b2950`.

### §3.1 TS file scope

| TS file | LOC | Foundation in NAI-191? |
|---|---:|---|
| `tools/pack/FsCache.ts` | 79 | **Yes** — wholesale port. |
| `tools/pack/Parse.ts` | 179 | **Yes** — wholesale port. `Environment.BUILD_SRC_DIR` in `readConfigs` becomes a parameter. |
| `tools/pack/NameMap.ts` | 59 | **Yes** — wholesale port. |
| `tools/pack/PackFileBase.ts` | 141 | **Yes** — wholesale port of the `PackFile` class. |
| `tools/pack/PackFile.ts` | 408 | **Partial.** Port the freshness cohort (`getModified`, `getLatestModified`, `shouldBuild`, `shouldBuildFile`, `shouldBuildFileAny`) and the generic `crawlConfigNames`. Defer the 6 `validate*` functions, the 26 module-level singletons, `revalidatePack`, and `crawlConfigCategories`. |

**Total in scope:** ~580 LOC TS (full FsCache + Parse + NameMap + PackFileBase + ~120 LOC slice of PackFile) → projected ~500 LOC Go + tests.

### §3.2 `pkg/io/jagfile` already exists

Verified at HEAD `e7b2950`:

```
pkg/io/jagfile/
  jagfile.go       — Jagfile struct + Read/Write/Get/Save/Delete/Rename
  bzip2.go         — BZip2Compress / BZip2Decompress
```

`Jagfile.Save(path string, doNotCompressWhole bool) error` writes the full binary format (per-entry hash, unpacked size, packed size, BZip2 chunks). No work in NAI-191; downstream NAI-192+ packers will consume this directly.

### §3.3 No existing `pkg/pack/` package

`find pkg -name "pack*"` returns:
```
pkg/io/packet/...   (RS2 packet buffer — unrelated)
```
The new `pkg/pack/` namespace is free.

### §3.4 `Environment.BUILD_SRC_DIR` audit

TS references in scope-of-foundation:
- `Parse.ts:readConfigs` — `${Environment.BUILD_SRC_DIR}/scripts` is hardcoded.
- `PackFileBase.ts:PackFile.reload` — `${Environment.BUILD_SRC_DIR}/pack/${this.type}.pack` is hardcoded.
- `PackFileBase.ts:PackFile.save` — same path.

All three sites become explicit parameters (constructor on `PackFile`, function arg on `ReadConfigs`).

### §3.5 TS `printError`/`printFatalError` mode-switch

`PackFile.reload` catches errors and branches on `parentPort` presence (worker context → `printError` non-fatal vs main process → `printFatalError`). Goscape's foundation returns errors; callers handle logging policy. No `parentPort` analogue.

### §3.6 TS multi-line-comment counter behavior (`Parse.ts:loadFileFull`)

`loadFileFull` uses a `multiLineComments` counter with non-trivial semantics:

- The *outer* code path (counter = 0) only handles the first `/*` it sees on a line: if `*/` is on the same line, both are stripped in-place; otherwise the counter increments to 1.
- The *inner* code path (counter > 0) walks every `/*` and `*/` in the current line, incrementing / decrementing the counter accordingly.

Net result: `/* /* */ */` on a single line is **not** symmetric — only the first `/* */` pair is stripped, and a literal ` */` survives in the output. This is a TS quirk, not nesting-faithful comment parsing. Goscape pins the TS counter behavior byte-for-byte. The §8.1 `parse_test.go (e)` case enumerates the expected output for a representative quirky input.

### §3.7 `PackFile.refreshNames` rebuild semantics

`PackFile.refreshNames` rebuilds `names` (Set) from `pack.values()` and recomputes `max = max(pack.keys()) + 1` only when `names` is non-empty (TS guard: `if (this.names.size)`). It does **NOT** rebuild `nameToId` — that map is maintained incrementally by `register`/`delete`/`deleteByName`. Goscape must preserve this asymmetry; `nameToId` is not derived during `RefreshNames`.

### §3.8 `PackFile.save` output format

TS `save()` writes lines in `id=name\n` form, with entries sorted by id ascending, and a trailing newline (`Array.from(...).join('\n') + '\n'`). Creates `<srcDir>/pack/` directory recursively if absent. Goscape must produce byte-identical output to round-trip cleanly.

### §3.9 `PackFile` getter / delete API

TS `PackFile` also exposes `getById(id) → string` (`''` on missing), `getByName(name) → number` (`-1` on missing), `delete(id)`, and `deleteByName(name)`. All four are in foundation scope (cheap to port, downstream consumers will need them). `delete` / `deleteByName` call `refreshNames` after removing entries.

## §4 Architecture & components

### §4.1 Package layout

```
pkg/pack/
  fscache.go         — FsCache port (mutex-guarded module caches)
  fscache_test.go
  parse.go           — Parse port (LoadFile/LoadFileFull/LoadDir*/ReadConfigs)
  parse_test.go
  namemap.go         — NameMap port (LoadOrder/LoadPack/LoadDir/LoadDirExact)
  namemap_test.go
  packfile.go        — PackFileBase port (PackFile struct + methods)
  packfile_test.go
  freshness.go       — GetModified/GetLatestModified/ShouldBuild* cohort
  freshness_test.go
  crawl.go           — generic CrawlConfigNames
  crawl_test.go
  testdata/
    pack/obj.pack
    scripts/foo.obj
    scripts/comments.rs2
    scripts/dup.obj          (for ReadConfigs duplicate-error case)
    engine.rs2               (for CrawlConfigNames skip case)
```

### §4.2 Component summaries

Each component below has its full API in §5.

**`fscache.go`** — three module-level `sync.RWMutex`-guarded caches (`dirCache`, `existsCache`, `statsCache`). Public: `ClearFsCache`, `FileExists`, `FileStat`, `ListDir`, `ListFiles`. `ListDir` recurses and suffixes subdir entries with `/` to match TS output exactly (downstream callers depend on the suffix).

**`parse.go`** — `LoadFile` returns lines unchanged. `LoadFileFull` strips `//` single-line and `/* */` multi-line comments using TS's counter-based scheme (§3.6). Returns `error` on unclosed `/*`. `LoadDir`/`LoadDirFull`/`LoadDirExt`/`LoadDirExtFull` walk filesystem via `FsCache.ListFiles`; `ReadConfigs(srcDir, ext)` returns `map[string][]string` keyed by `[header]` name, returning error on duplicate keys.

**`namemap.go`** — `LoadOrder` reads numeric-only lines; `LoadPack` reads `id=name` lines into a sparse `[]string` indexed by id; `LoadDir` / `LoadDirExact` mirror TS callback shape, the latter NOT filtering empty lines.

**`packfile.go`** — `PackFile` struct + ctor + methods (§5). `NewPackFile` calls `Reload` immediately (TS parity) and returns the error.

**`freshness.go`** — `GetModified` returns mtime in ms (matching TS `mtimeMs`), 0 if missing. `ShouldBuild` / `ShouldBuildFile` / `ShouldBuildFileAny` compare against `dest` mtime; `ShouldBuildFileAny` recurses through directories.

**`crawl.go`** — `CrawlConfigNames(srcDir, ext, includeBrackets)` walks `<srcDir>/scripts/*.<ext>`, skips `<srcDir>/scripts/engine.rs2`, returns unique bracketed-header names in source order. The `BUILD_VERIFY_FOLDER` directory-structure check is documented as deferred (§7 deviation `NAI-191-D-VALIDATE-FLAGS-DEFERRED`).

### §4.3 Dependency graph

```
fscache.go              (no internal deps)
   ↑
parse.go                (uses fscache)
namemap.go              (uses fscache)
freshness.go            (uses fscache)
   ↑
packfile.go             (uses parse)
crawl.go                (uses parse)
```

No import cycles. No external deps beyond stdlib.

### §4.4 Concurrency

- `fscache.go`: module-level caches under a single `sync.RWMutex` (`RLock` on read, `Lock` on write/clear). TS is single-threaded; goscape's eventual `::rebuild` driver runs from a worker goroutine spawned by the cheat handler, so concurrency-safety is required up front.
- `packfile.go`: NOT safe for concurrent use. Document on struct. Callers serialize.
- `parse.go`, `namemap.go`, `freshness.go`, `crawl.go`: pure / stateless. Safe for concurrent use modulo `fscache.go`.

### §4.5 Errors and logging

- No logging in foundation code. All error paths return `error`.
- Sentinel error wording:
  - Unclosed multi-line comment: `"%s has an unclosed multi-line comment starting at line %d"` (TS message: `"${path} has an unclosed multi-line comment! Line: ${multiCommentStart}"` — wording adapted; line-number behavior preserved).
  - PackFile empty name: `"pack file has an empty name %s:%d"` (TS verbatim).
  - ReadConfigs duplicate: `"duplicate config found in %s: %s"` (TS verbatim).
- Missing-path short-circuit: `LoadFile`, `LoadOrder`, `LoadPack`, `ListDir`, `ListFilesExt`, `PackFile.Load` all return empty results (no error) when the path does not exist. TS parity.

## §5 API surface

### §5.1 `PackFile`

```go
package pack

type Validator func(pf *PackFile) error

// PackFile is a name↔id registry loaded from a .pack source file. Not safe
// for concurrent use; callers must serialize Reload/Load/Save/Register.
type PackFile struct {
    Type      string
    SrcDir    string
    Validator Validator
    Pack      map[int]string
    Names     map[string]struct{}
    NameToID  map[string]int
    Max       int
}

func NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error)
func (pf *PackFile) Reload() error
func (pf *PackFile) Load(path string) error
func (pf *PackFile) Save() error
func (pf *PackFile) Register(id int, name string)
func (pf *PackFile) Delete(id int)
func (pf *PackFile) DeleteByName(name string)
func (pf *PackFile) RefreshNames()
func (pf *PackFile) GetByID(id int) string       // "" if missing
func (pf *PackFile) GetByName(name string) int   // -1 if missing
func (pf *PackFile) Clear()
func (pf *PackFile) Size() int
```

`Reload` calls `Validator(pf)` if non-nil, else `Load("<SrcDir>/pack/<Type>.pack")`. `Save` writes sorted-by-id `<SrcDir>/pack/<Type>.pack` and creates the parent dir recursively. `NewPackFile` invokes `Reload` and surfaces the error (TS swallows it via `printError`/`printFatalError` based on `parentPort` — deviation `NAI-191-D-NO-PARENT-PORT`).

`RefreshNames` rebuilds `Names` from `Pack` values and recomputes `Max` only when `Names` is non-empty (TS-parity per §3.7); it does **not** rebuild `NameToID`. `Delete` / `DeleteByName` remove from both `Pack` and `NameToID`, then call `RefreshNames`.

### §5.2 `FsCache`

```go
func ClearFsCache()
func FileExists(path string) bool
func FileStat(path string) (os.FileInfo, error)
func ListDir(path string) []string    // recursive, "/"-suffixed subdir entries
func ListFiles(path string) []string  // recursive flat
```

### §5.3 `Parse`

```go
type LoadDirCallback func(lines []string, file string)

func LoadFile(path string) []string
func LoadFileFull(path string) ([]string, error)
func LoadDir(path string, cb LoadDirCallback)
func LoadDirFull(path string, cb LoadDirCallback) error  // error from LoadFileFull surfaces
func ListFilesExt(path, ext string) []string
func LoadDirExt(path, ext string, cb LoadDirCallback)
func LoadDirExtFull(path, ext string, cb LoadDirCallback) error
func ReadConfigs(srcDir, ext string) (map[string][]string, error)
```

Note: `LoadDirFull` / `LoadDirExtFull` return `error` (deviation `NAI-191-D-LOADFILEFULL-ERRORS-PROPAGATE`) — TS `loadFileFull` throws and the dir-walker propagates. In Go we return rather than panic.

### §5.4 `NameMap`

```go
type NameMapCallback func(src []string, file, path string)

func LoadOrder(path string) []int
func LoadPack(path string) []string
func LoadDir(path, ext string, cb NameMapCallback)
func LoadDirExact(path, ext string, cb NameMapCallback)
```

### §5.5 `Freshness`

```go
func GetModified(path string) int64           // mtime in milliseconds; 0 if missing
func GetLatestModified(path, ext string) int64
func ShouldBuild(srcPath, ext, outPath string) bool
func ShouldBuildFile(src, dest string) bool
func ShouldBuildFileAny(path, dest string) bool
```

### §5.6 `Crawl`

```go
func CrawlConfigNames(srcDir, ext string, includeBrackets bool) ([]string, error)
```

## §6 Data flow

NAI-191 has no production data flow — the package is library-only with no callers in production code at slice close. The end-state data flow for the eventual `::rebuild` pipeline is:

```
::rebuild cheat → World.rebuild() → DevThread channel
                                    ↓
                                  PackAll
                                    ↓
                          [per-config packer slices]
                                    ↓
                    revalidatePack() walks all PackFiles
                                    ↓
                       Parse.ReadConfigs() per format
                                    ↓
                       Compile + write jagfile entries
                                    ↓
                          jagfile.Jagfile.Save()
                                    ↓
                       DevThread posts "dev_reload"
                                    ↓
                       World.Reload(clearInvs=true)
```

NAI-191 ships **everything above `revalidatePack`**: the `PackFile` registry struct, the `Parse.ReadConfigs` source-walker, the file-freshness helpers that gate `shouldBuild`, the `Crawl` helper that scans for `[headers]`. The validators, the per-format packers, the compiler, `PackAll`, `DevThread`, and the `::rebuild` cheat all land in subsequent slices.

## §7 Deviations from TS

| Tag | Description |
|---|---|
| `NAI-191-D-CONCURRENCY` | `FsCache` module-level caches are `sync.RWMutex`-guarded. TS unguarded (single-threaded). Future-proofs for `::rebuild` worker goroutine. |
| `NAI-191-D-NO-PARENT-PORT` | Errors are returned, not branched on `parentPort` presence for `printError` vs `printFatalError`. Callers handle logging. |
| `NAI-191-D-NO-GLOBAL-SRCDIR` | `Environment.BUILD_SRC_DIR` becomes a constructor parameter on `PackFile` and an explicit function arg on `ReadConfigs` / `CrawlConfigNames`. No package-level setter. |
| `NAI-191-D-VALIDATE-FLAGS-DEFERRED` | `BUILD_VERIFY`, `BUILD_VERIFY_FOLDER`, `BUILD_VERIFY_PACK` env flags are read only by `validate*` functions. Deferred with them to NAI-192+. |
| `NAI-191-D-LOADFILEFULL-ERRORS-PROPAGATE` | `LoadDirFull` / `LoadDirExtFull` return `error`; TS throws synchronously from a callback. Same observable outcome (first error halts the walk); just the Go shape. |

All five deviations are intentional and documented per `true_to_ts_gate`. None are "TODO eventually fix" — they are permanent shape adjustments for Go.

## §8 Test strategy

### §8.1 Per-component unit tests

| File | Cases |
|---|---|
| `fscache_test.go` | (a) `ListDir` on a missing path returns `nil`; (b) recursion produces `"sub/"` suffix on subdir entry and lists nested files; (c) cache hit on second `ListDir` call (instrument via a write-then-read sequence that would fail if cache miss); (d) `ClearFsCache` invalidates. |
| `parse_test.go` | (a) `LoadFile` missing path returns `nil`; (b) `LoadFileFull` strips `//` single-line; (c) strips `/* */` same-line; (d) strips `/* */` multi-line; (e) **TS-parity-pinned**: `/* /* */ */` treated as `/* /* */ /* */` by counter — pin observable lines remaining; (f) unclosed `/*` returns error naming the start line; (g) `ReadConfigs` aggregates two files; (h) `ReadConfigs` duplicate key returns error naming file + key; (i) `LoadDirExtFull` propagates unclosed-comment error from one file. |
| `namemap_test.go` | (a) `LoadOrder` reads numeric lines, ignores empties; (b) `LoadPack` returns sparse `[]string` with gaps preserved; (c) `LoadDir` filters by extension, calls callback per file; (d) `LoadDirExact` does NOT filter empty lines (TS-parity pin). |
| `packfile_test.go` | (a) `NewPackFile` with missing file returns no error, empty registry; (b) `Load` on valid 3-line file populates `Pack`/`Names`/`NameToID`/`Max`; (c) `Load` on gap (`0=foo`, `5=bar`) sets `Max=6`; (d) `Load` empty-name line returns error naming file + line; (e) `Register` updates `NameToID` immediately; `RefreshNames` updates `Names` and `Max` from `Pack` but does NOT touch `NameToID` (§3.7 pin); (f) `Clear` empties all four fields; (g) round-trip: `Load`→`Save`→re-`Load` produces byte-identical file content (sorted id ascending, trailing `\n`); (h) `Validator` non-nil path: `Reload` calls validator, validator-error surfaces; (i) `Delete(id)` removes entry, updates `NameToID`, calls `RefreshNames`; (j) `DeleteByName` symmetrical; (k) `GetByID` missing returns `""`; (l) `GetByName` missing returns `-1`; (m) `Save` creates `<srcDir>/pack/` directory if absent. |
| `freshness_test.go` | (a) `GetModified` on missing returns 0; (b) `GetLatestModified` returns max across matching-extension files; (c) `ShouldBuild` true when out missing, true when out older, false when out newer; (d) `ShouldBuildFileAny` recurses through a 2-deep subdir to find a stale file. |
| `crawl_test.go` | (a) `CrawlConfigNames` walks `.obj` files, returns names in source order; (b) dedup across files (same `[name]` in two files appears once); (c) `engine.rs2` skip even when extension matches; (d) `includeBrackets=true` returns `"[name]"` form. |

### §8.2 Test fixtures

All fixtures < 30 lines. Examples:

`testdata/pack/obj.pack`:
```
0=coins
1=bronze_dagger
2=oak_log
```

`testdata/scripts/comments.rs2`:
```
[script,foo]    // first
mes("hi");      /* trailing */
/* multi
   line
   comment */
[script,bar]
```

`testdata/scripts/dup.obj`:
```
[coins]
model=coins
[coins]
model=other
```

(`ReadConfigs` over this file returns the duplicate error.)

### §8.3 No binary golden-file tests

This slice produces no `.jag` output; no binary equivalence needed. (Future NAI-192+ packer slices will add golden-file comparison against TS-produced reference binaries — out of scope here.)

### §8.4 Coverage target

≥ 90% line coverage on each of the six production files. The slice has no dead branches by design; gaps indicate untested error paths.

## §9 Risk register

| Risk | Likelihood | Mitigation |
|---|:-:|---|
| TS multi-line-comment counter behavior misported | Low | Test case §8.1 parse_test.go (e) pins the counter semantics with a hand-traced expected output. |
| `PackFile.Save` byte-output not identical to TS | Low | Round-trip test §8.1 packfile_test.go (g). If round-trip passes, the format is stable; whether it matches TS byte-for-byte is verified by a one-time human comparison against an existing TS-produced `.pack` file (plan task — fixture seed). |
| `mtimeMs` granularity differs between TS and Go | Negligible | TS uses `fs.Stats.mtimeMs` (float64 ms); Go uses `os.FileInfo.ModTime().UnixMilli()` (int64 ms). NAI-191 has no cross-runtime test; freshness comparisons are within-Go. |
| `FsCache` mutex contention under concurrent `::rebuild` | Negligible at this slice | No callers in NAI-191. Future slices that fan out across goroutines may want a sharded cache; deferred until measured contention exists. |
| `pkg/pack` namespace collides with future intent | Low | Verified at §3.3. `pkg/io/packet` is the only `pack*` namespace and is unrelated. |

## §10 Build / commit decomposition

Single feature commit recommended (foundation slice with no incremental dependencies between files beyond the §4.3 graph). Optional split into two commits if the diff is large enough to warrant review-surface tuning:

- **T1** `feat(pack): NAI-191 — FsCache + Parse + NameMap foundation`
  - `pkg/pack/fscache.go`, `parse.go`, `namemap.go` + tests + testdata fixtures.
- **T2** `feat(pack): NAI-191 — PackFile + Freshness + Crawl`
  - `pkg/pack/packfile.go`, `freshness.go`, `crawl.go` + tests + testdata fixtures.

Plan-author may collapse to a single commit if file count makes review easier.

## §11 The NAI-191 → NAI-?? arc

NAI-191 is slice 1 of a multi-slice arc that closes `::rebuild`. Provisional decomposition (not binding; later slices may merge/split):

| Slice | Scope | Status |
|---|---|---|
| **NAI-191** | This spec. Foundation (FsCache, Parse, NameMap, PackFile, Freshness, generic Crawl). | Active |
| NAI-192 .. NAI-19X | Per-config packers, one or two at a time. Each ports: validator function + module-level registry singleton + binary writer to a jagfile entry. Likely cohort order: small configs first (Varn, Vars, MesAnim, Param), then medium (Seq, SpotAnim, Flo, Idk, Inv, Enum, Struct, Hunt), then large (Loc, Npc, Obj, DbRow, DbTable). | Future |
| NAI-19Y | Sprite / texture / model / interface / map / midi / sound packers (`PixPack`, `chat/`, `graphics/`, `interface/`, `map/`, `midi/`, `sound/`, `sprite/`). | Future |
| NAI-19Z | `.rs2` bytecode compiler (`Compiler.ts`, ~370 LOC, plus its `Parse.ts` consumers). | Future |
| NAI-20A | `PackAll` orchestrator + `revalidatePack()`. | Future |
| NAI-20B | `DevThread` analogue (fsnotify + goroutine + worker channel). | Future |
| NAI-20C | `::rebuild` cheat handler wiring + close. | Future |

Total arc projected at ~10-15 sub-specs, several thousand LOC Go. NAI-191's clean foundation is what makes the per-slice cadence tractable.

## §12 Acceptance criteria

1. `pkg/pack/` exists with the six production files and six test files in §4.1.
2. `go test ./pkg/pack/...` passes (`-count=1 -race`).
3. `go build ./...` succeeds.
4. No new production callers — `rg "pkg/pack" modules/ cmd/` returns zero matches at slice close.
5. All five §7 deviations documented inline in the production files via `// DEVIATION-NAI-191-D-XXX` comments per `defensive_gate_doc_comment_label`.
6. Test coverage ≥ 90% per production file (best-effort; gaps explained).

## §13 References

- TS source: `LostCityRS/Engine-TS/tools/pack/{FsCache,Parse,NameMap,PackFileBase,PackFile}.ts`.
- jagfile backend (already ported): `pkg/io/jagfile/jagfile.go`.
- Predecessor close: NAI-190 (`e7b2950`) — `World.reload()` port + `::reload` cheat.
- Memory anchors: `controller_preflight`, `risk_register_premise_grep`, `true_to_ts_gate`, `defensive_gate_doc_comment_label`, `compressed_cadence` (does NOT apply — this slice is too large for compressed cadence; standard spec+plan).
