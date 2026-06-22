# rev-244 B6 — pack pipeline re-baseline — design

**Date:** 2026-06-05
**Status:** Approved
**Branch:** rev-244 (umbrella: `2026-06-03-rev244-port-design.md` §B6; resume
context: `docs/superpowers/handoffs/2026-06-05-RESUME-rev244-port-b6.md`)

## Goal

Port the pack-pipeline slice of the 225→244 Engine-TS delta and re-baseline
goscape's pack output against a reference cache produced by the upstream 244
toolchain. B6 is where ALL deferred windows close (B3 user decision): B1
format window, B2 map-delivery, B3 midi live-verification, B4 script.dat
opcode numbering — plus the umbrella's end-to-end 244-client login smoke
(amended from post-B2+B3 to B6).

**Definition of done:**

1. The 244 reference cache exists, packed from the EXACT recorded pins, with
   a sha256 manifest committed to goscape.
2. The `tools/pack` delta (31 files, +1,392/−1,266) plus the two deferred
   externals (`src/cache/DevThread.ts` packAll signature — B1;
   `src/app.ts` BUILD_STARTUP_UPDATE / `packAll(modelFlags)` — B3) are
   ported hunk-for-hunk with PORTING.md §B6 correspondence rows.
3. `goscape-cli pack` over Content@pin is deterministic (two runs,
   byte-identical) AND **full-tree byte-identical** to the reference
   (user decision): `server/*` + client jag archives + `main_file_cache.*`
   + `ondemand.zip` — everything Build.ts produces under `data/pack`.
4. All four deferred windows closed; the two B1 skip-gated tests
   (`TestLoadSeqTypes_FromPack`, `TestNewServer_LoadsWordencFilter`)
   un-skipped and green.
5. End-to-end 244 client login smoke (Client-Java `01f16088`, user-driven)
   against goscape serving the new cache.
6. Gates green: build / vet / full suite / `-race` on touched packages;
   bundle-end correspondence audit + integration review.

## User decisions (recorded 2026-06-05)

1. **Keep the recorded pins.** Upstream's 244 branch moved/rebased after the
   pins were captured (engine `9aadcec4` → `3c16994c`; content `e5d0282e` →
   `cbcfe670`, 502 tree entries apart — later-revision work, per user). The
   Jun-4 cache in `LostCityRS/Server/` was packed from those newer commits
   and is **NOT usable as the B6 reference** (differing files include
   `tools/pack/config/PackShared.ts`, `LocConfig.ts`, `interface/Pack*.ts`,
   `ScriptOpcode*.ts`). B6 packs a fresh reference from the pins, checked
   out via git worktrees. No re-pin; upstream movement is a separate later
   decision.
2. **Reference layout: durable sibling dir** —
   `$HOME/Code/github.com/LostCityRS/Server244-ref/` (mirrors the
   Server225_2 precedent the Arc-26 byte-diff loop used).
3. **Parity scope: full tree.** Not just `server/*`; the login smoke needs
   the client cache + ondemand correct anyway.

## Verified provenance facts (2026-06-05)

- Engine-TS pin `9aadcec4` = local branches `244` and `244-GOSCAPE`
  (identical SHA) = `origin/244` as of the 2026-06-03 fetch. Working copy
  sits on `254-GOSCAPE` — do not move it; use a worktree.
- Content pin `e5d0282e` = local branch `244` tip = `origin/244` likewise.
- `ScriptProvider.COMPILER_VERSION = 26` at the engine pin
  (ScriptProvider.ts:12).
- RuneScriptKt-26 jar already on disk:
  `LostCityRS/Server/engine/RuneScriptCompiler.jar`, sha256
  `38e16e2c375cfdb0179cce1cab9c06d279cc7c30b0cbc298c97a37c4dca1851a`.
  Upstream's own `updateCompiler()` (RuneScriptCompiler.ts) sha256-verifies
  the jar against the release-26 published checksum on download, so this
  file IS release 26. Record this sha in `main:REFERENCES.md` §rev-244
  (fills its "record when fetched" placeholder). No network required.
- Local RuneScriptKt checkout is at tag `22` — irrelevant once the jar is
  pinned by sha; do not build from source.
- Toolchain present: bun, node, java 25 (Temurin).
- goscape `pkg/packall.PackAll` stage list mirrors TS `PackAll.ts` 1:1
  (configs → clientinterface → server compiler → title/media/texture →
  wordenc → sound → graphics → midi → maps), so the delta maps stage-wise.

## Phase 0 — reference cache (PREREQUISITE; nothing else verifiable without it)

```
git -C LostCityRS/Engine-TS worktree add ../Server244-ref/engine 9aadcec4
git -C LostCityRS/Content   worktree add ../Server244-ref/content e5d0282e
cp LostCityRS/Server/engine/RuneScriptCompiler.jar Server244-ref/engine/
# verify sha256 == 38e16e2c…
cd Server244-ref/engine && bun install   # pin's bun.lock; offline cache first
bun run build                            # tools/pack/Build.ts → data/pack/
```

- Worktrees are read-only reference material — nothing is ever committed
  there.
- How Build.ts locates the content dir (env var / `content/` sibling /
  config) is confirmed from the pin's source during planning; the layout
  above adjusts to whatever the pin expects (Server225_2 used
  `engine/` + `content/` siblings).
- `bun install` may need network if bun's offline cache can't satisfy the
  pin's lockfile (sandbox-bypass prompt). All Phase-0 writes land outside
  the goscape repo → run sandbox-bypassed with user permission prompts.
- Output: `Server244-ref/engine/data/pack/` = THE reference cache.
- Deliverable into goscape: a sha256 **manifest** of every produced file
  (small text file, not binaries), committed as `testdata` next to the
  parity acceptance test — drift detection + the test's expected values.
- The dependency set matters for byte parity (`@2004scape/rsbuf ^244.1.0`):
  the pin's own `bun.lock` is authoritative; do NOT copy `node_modules`
  from `Server/engine` (its lockfile differs from the pin's).

## Phase 1 — port the TS delta

Scope slice (extraction commands; all citations refer to the 244 pin):

```
git -C ../Engine-TS diff e1dea19f..9aadcec4 -- tools/pack
git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/cache/DevThread.ts src/app.ts src/util/RuneScriptCompiler.ts
```

### Compiler swap (umbrella RISK #1)

244 deletes the in-process npm compiler driver (`Compiler.ts` −368;
`@lostcityrs/runescript` dependency dropped) and replaces it with
`CompilerSymbols.ts` (+411 — symbol-file generation feeding the jar) +
`RuneScriptCompiler.ts` (+47 — jar download/exec). **goscape keeps its
native Go compiler** (`pkg/pack/compiler/`) — no java/jar runtime
dependency (single-static-binary philosophy; the `pkg/rsbuf`
reimplementation precedent). Consequences:

- `CompilerSymbols.ts` maps onto goscape's symbol-table source
  (`pkg/pack/compiler/symbols.go` and friends) — the symbol *content* the
  jar consumes must match what the Go compiler derives, since symbol
  content shapes emission.
- `Compiler.ts` deletion maps onto whatever Go scaffolding mirrored the
  old npm driver — audit, don't assume.
- The real contract is **emission parity with RuneScriptKt-26**: the Go
  compiler's `script.dat`/`script.idx` must byte-match the jar's output
  over Content@pin. Differences vs the RuneScriptTS lineage are the
  bundle's biggest unknown; expect a multi-bug stack (Arc-26 precedent)
  and budget Phase 2 iterations accordingly.
- `RuneScriptCompiler.ts` (downloader/exec glue) is expected
  **NOT-PORTED, platform-inapplicable** — goscape compiles natively;
  decision row records it.

### Everything else (stage-wise mapping)

| TS surface | Go target |
|---|---|
| `Build.ts` (12/12), `PackAll.ts` (+76), `NameMap.ts`, `Parse.ts`, `PackFile.ts`, `PixPack.ts` | `pkg/packall`, `pkg/pack` (crawl/parse/packfile/registry) |
| `config/*.ts` (Idk/Inv/Loc/Npc/Obj/Seq/SpotAnim + `PackShared.ts` 89-line churn) | `pkg/pack/{idk,inv,loc,npc,obj,seq,spotanim}.go`, `pack_configs.go` |
| `chat/pack.ts` (105-line, deletion-heavy) | `pkg/pack/mesanim.go` / chat stage |
| `graphics/pack.ts` (353-line, deletion-heavy), `sprite/{media,textures,title}.ts` | `pkg/pack/graphics`, `pkg/pack/sprites` |
| `interface/{PackClient,PackShared}.ts` | `pkg/pack/clientinterface` — **minus the B1 pulled-forward hunks (below)** |
| `map/Pack.js` (+526 churn) | `pkg/pack/maps` |
| `midi/pack.ts`, `sound/pack.ts` | `pkg/pack/audio` |
| `versionlist/pack.ts` (+133, NEW) | NEW Go stage (client versionlist / ondemand support) |
| `Worldmap.ts` (156-line churn) | `pkg/pack/worldmap` |
| `src/cache/DevThread.ts` (+3/−2, B1-deferred) | `pkg/packall.PackAll` signature |
| `src/app.ts` BUILD_STARTUP_UPDATE + `packAll(modelFlags)` (B3-deferred) | world/ondemand startup path — **apply the B5 "what does TS's CONSUMER do?" check first** |

Per-file hunk dispositions (PORT / NO-OP / NOT-PORTED / already-applied)
are produced at planning time from numbered listings — the B4-spec
discipline. Two pins are non-negotiable:

**B6 must NOT double-apply:**

- The B1 clientinterface pull-forward (`pkg/pack/clientinterface/pack.go`):
  Component trans P1 + layer-childCount g1→g2 (TS PackShared.ts:267-274,
  428-431) — landed in `e4e881d8`.
- `jagFileVersion` stays **26** unless the upstream meta-repo moves its
  runescript pin past `750291c` (PORTING-LESSONS §3).

## Phase 2 — verify & close windows (strict order)

1. **Determinism re-baseline FIRST** (Arc-26 lesson #188): pack Content@pin
   twice into separate dirs, `diff -r`. Any nondeterminism is fixed before
   a single reference byte is compared (Go map iteration is the usual
   suspect; the Arc-26 sorted-iteration inventory is prior art).
2. **Full-tree byte-diff loop** vs `Server244-ref/engine/data/pack/`,
   iterating per-file (the Arc-26 ~5-10 min/cycle loop). `server/script.dat`
   is expected to be the hard one (jar emission semantics).
3. **Close the windows:** un-skip `TestLoadSeqTypes_FromPack`
   (pkg/objtype/seqtype_test.go) + `TestNewServer_LoadsWordencFilter`
   (modules/world/server_wordenc_test.go); remove/close
   `rev244-b1-format-window` marker (pkg/objtype/seqtype.go:98) +
   the B2 map-delivery, B3 midi-live, and B4 script.dat-numbering rows in
   PORTING.md.
4. **Login smoke LAST:** install the cache into goscape's data dir
   (empty/absent cache → empty CrcTable → every login rejected
   out-of-date, rev244-b3-crc-compare row), start goscape, user drives
   Client-Java `01f16088` through login + basic play.
5. Gates + PORTING.md §B6 correspondence audit + B7 handoff.

## Testing

- Per-stage roundtrip/pin tests updated RED→GREEN per hunk (existing
  `pkg/pack/*_test.go` pattern).
- New determinism + full-tree parity acceptance test reusing the
  `smoke_pack_diff` snapshot machinery (`cmd/goscape-cli/smoke_pack_diff.go`
  sha256 walker), env-var-gated on the ref dir
  (`GOSCAPE_REF_CACHE_DIR`-style; skip when absent — the
  `GOSCAPE_WORLDMAP_INTEGRATION` pattern), asserting against the committed
  sha256 manifest.
- `TestDecodeRealCacheBlob` "bad trailer position" is a known
  **pre-existing DECODER bug** in `pkg/script/file.go` (Arc-26 residual) —
  do NOT chase it through the packer; fix or re-defer it explicitly as its
  own item if it blocks a B6 test.

## Gates & process

B1-B5-proven cycle: plan via writing-plans (bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution (implementer
sonnet → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks) → full-suite gate + PORTING.md §B6
correspondence audit → final whole-bundle integration review.

Implementer-prompt mandates (recurring defects, B2-B5-proven): citations
verified against `cat -n` numbered listings BEFORE writing; reject-path
tests seed earlier-gate prerequisites; final-review "missing X" findings
verified before fixing; interface-method additions cascade into test fakes
(§NEW-INTERFACE-METHOD-COMPILE-CASCADE); run the modules/world suite when a
world-exercised contract changes; **write Go test code with Write/Edit
tools, never bash heredocs** (sandbox mangles `!=`); ask "what does TS's
CONSUMER do?" before modeling any producer.

Gates: `CGO_ENABLED=0 go build -trimpath ./...`; `go vet ./...`; full
`go test ./... -count=1` (real exit codes); `-race` (CGO_ENABLED=1) on
pkg/pack + pkg/pack/compiler + pkg/packall + modules/world (if touched).
Commits on `rev-244` only, `--no-gpg-sign` + Claude trailer; subagents
warned about phantom `??` dotfiles (never `git add -A`).

## Risks

1. **RuneScriptKt-26 emission delta** (umbrella risk #1) — unknown until
   the byte-diff runs; mitigated by reference-first ordering,
   determinism-first discipline, and per-file iteration. If the delta
   proves structural (not a bug-stack but a different format), that is a
   design checkpoint back to the user, not a grind.
2. **map/Pack.js churn** (+526, and it's `.js` — the sloppiest source in
   the slice) — budget extra citation-verification care.
3. **bun install / Build.ts environment drift** — the pin's Build.ts may
   expect env/config not present in a bare worktree; resolved during
   Phase 0 before any Go work depends on it.
4. **Full-tree scope creep** — client-side stages (graphics, sprites,
   versionlist, main_file_cache assembly) get equal gate status; if one
   client artifact stalls the bundle, descoping it is a user decision,
   not a silent skip.

## Housekeeping

- Delete the stale 23 MB `script.test` binary at the goscape repo root
  (untracked `go test -c` leftover).
- Fill the REFERENCES.md §rev-244 RuneScriptKt jar-sha placeholder
  (sha256 `38e16e2c…`) — lands on `main` (docs hub owns REFERENCES.md).

## Non-goals

- `tools/unpack` (+3,793) — B7.
- Re-pinning to upstream's moved 244 branch (`3c16994c` / `cbcfe670`) —
  separate future decision.
- Worker/multiworld architecture — closed in B5.
