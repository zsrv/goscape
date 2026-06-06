# rev-244 Bundle 7 — `goscape-cli unpack` — design

**Date:** 2026-06-06
**Status:** Approved
**Branch:** `rev-244`
**Umbrella:** [`2026-06-03-rev244-port-design.md`](2026-06-03-rev244-port-design.md) §B7 (the last bundle)

## Goal

Port the all-new TS `tools/unpack` (+3,793 lines at pin `9aadcec4`; every file
new in the cross-pin diff — read whole, not as diffs) as a `goscape-cli unpack`
command family. Work list:

```
git -C /home/owner/Code/github.com/LostCityRS/Server244-ref/engine \
  diff --stat e1dea19f..9aadcec4 -- tools/unpack
```

31 files. The tools read a client-format FileStream cache
(`main_file_cache.dat` + `.idx0-4` — exactly what B6's byte-parity pack
pipeline produces) and emit a Content-shaped source tree (aggregated
`scripts/_unpack/<rev>/all.<type>` configs, `.ob2`/`.mid`/`.synth`/sprite
extractions), **mutating** the content tree as they go (model renames, NameMap
pack-registry rewrites).

## Scope (user-approved: "core + debug aids")

**In scope** — the core unpack pipeline plus the two debug aids:

| TS file(s) | role |
|---|---|
| `config/Unpack.ts` (368) + `config/{Common,FloConfig,IdkConfig,LocConfig,NpcConfig,ObjConfig,SeqConfig,SpotAnimConfig,VarpConfig}.ts` | config archive → `all.<type>` + name registration + loc-model renames |
| `config/Compare.ts` (69) | debug aid: per-entry CRC diff of two config archives |
| `checksum.ts` (18) | debug aid: per-file CRC print + extraction for config/interface/synth jags |
| `interface/Unpack.ts` (875) | interface archive → interface sources + com model renames |
| `map/Unpack.ts` (264) | maps via versionlist → `maps/` |
| `midi/Unpack.ts` (43) | songs/jingles via versionlist → `songs/`, `jingles/` |
| `sound/Unpack.ts` (230) | sounds jag → `synth/` |
| `sprite/{media,textures,title}.ts` (17/19/39) | sprite jags → extracted assets (`binary/`, `title/`, `fonts/`, …) |
| `graphics/UnpackAnims.ts` (117), `graphics/UnpackModels.ts` (57) | anim bases/frames + models → `models/` (+ registries) |
| `versionlist/{anim_index,midi_index,model_index}.ts` (13/14/62) | versionlist index dumps/listings |
| `worldmap/Unpack.ts` (33) | debug print: floorcol table from `worldmap.jag` |

**NOT-PORTED** (decision rows in the B7 audit trail): the 6 synth-curation
one-offs — `sound/{Generate,Match,Reorganize,RenameFile,PrintDirectory,PrintOrderDirectory}.ts`
(~267 lines). They depend on artifacts goscape does not have: `Generate`
shells out to `java -cp data/pack/rs2client.jar jagex2.client.SoundSynth`
per file over a `data/pack/377-synth` dump; the rest read/curate a hand-made
`data/pack/synths.json`. Dead-end dev-machine utilities, same closure shape
as B1's `DoublyLinkList` row.

## CLI surface

One `unpack` verb in `cmd/goscape-cli`'s `verbs` slice, sub-dispatched by
family (the `jag` verb's `list|extract|dump` precedent):

```
goscape-cli unpack <family> [flags]

families: config | interface | map | midi | sound | models | anims |
          sprite-media | sprite-textures | sprite-title |
          versionlist-anim | versionlist-midi | versionlist-model |
          worldmap | checksum | compare

flags:    -cache-dir data/unpack   FileStream cache (TS-faithful default)
          -src-dir   data/src      content tree to write into / mutate
          -revision  244           config family's scripts/_unpack/<rev> dir
                                   (TS hardcodes '244' at the call site)
          -pack-dir  data/pack     second input where TS reads data/pack:
                                   compare's packed jag, worldmap's FloType.load,
                                   config's optional merge-compare cache
          -log.level / -log.format house style (cmd_pack.go)
```

Thin `cmd_unpack.go`: flag parse + family dispatch + exit codes 0/1/2 per the
existing convention. All logic library-side.

## Package layout

`pkg/unpack/` subpackages mirroring `tools/unpack` the way `pkg/pack` mirrors
`tools/pack`:

```
pkg/unpack/config          config/{Unpack,Common,Compare,*Config}.ts
pkg/unpack/clientinterface interface/Unpack.ts   (pkg/pack's reserved-word dodge)
pkg/unpack/maps            map/Unpack.ts
pkg/unpack/midi            midi/Unpack.ts
pkg/unpack/sound           sound/Unpack.ts
pkg/unpack/sprite          sprite/{media,textures,title}.ts
pkg/unpack/graphics        graphics/{UnpackAnims,UnpackModels}.ts
pkg/unpack/versionlist     versionlist/{anim,midi,model}_index.ts
pkg/unpack/worldmap        worldmap/Unpack.ts
pkg/unpack/checksum        checksum.ts
```

Each TS entrypoint script becomes an exported `Unpack(opts) error` (or
family-appropriate name). No `process.exit` / `printFatalError` analogs in
libraries — return errors; the cmd layer maps them to exit codes. Print-style
tools (worldmap, checksum, compare, versionlist listings) take an `io.Writer`
so parity tests can capture stdout.

**Existing deps reused:** `pkg/io/filestream` (B1), `pkg/io/jagfile`,
`pkg/io/packet`, `pkg/pack` NameMap registries + `listFilesExt` (`parse.go`),
`pkg/colorconv`, `pkg/objtype` FloType.

## New supporting ports (out-of-slice deps)

Two `src/` files are **unchanged across the pin gap** (empty cross-pin diff)
but were never ported because rev-225 had no consumer; B7's unpack is the
first Go consumer. They are formally part of B7's scope even though they are
invisible to the diff slice (the B6 "TS-invisible to diff slicing" lesson,
dependency edition):

- `src/cache/graphics/Model.ts` (354) — `Model.unpack` metadata parse +
  `modelsHaveTexture` (used by the config unpackers' recol/retex emission).
- `src/cache/graphics/Pix.ts` — sprite decode for `sprite/*`
  (`pkg/pack/sprites` is the encode side only).

Default home `pkg/unpack/internal/<name>`; the plan stage may pick a better
one. Full `// TS <File>.ts:<lines>` citations against the 244 pin as with any
ported region.

## Verification

**Primary gate (user-approved): TS-output byte parity** — pin Go output 1:1
to the reference implementation's output.

1. **One-time reference generation** (documented script, run from
   `Server244-ref/engine` with bun): scratch-copy the Content worktree
   (unpack MUTATES it), set `BUILD_SRC_DIR` to the scratch copy, copy the
   byte-parity client cache to `data/unpack`, run each in-scope TS unpack
   script, snapshot the post-run trees + captured stdout (print-only tools)
   into a reference directory under the Server244-ref umbrella. NOTE:
   "trees" plural — some tools write into the CACHE dir, not just
   `BUILD_SRC_DIR` (`versionlist/model_index.ts` saves
   `data/unpack/model_index{,.txt}`; `checksum.ts` extracts into
   `data/unpack/<jag>/<name>`), so the harness snapshots and diffs both.
2. **Go parity test** in `pkg/unpack`, gated on `GOSCAPE_REF244_DIR` (the B6
   `pkg/packall/parity_test.go` pattern): replay identical inputs (same cache,
   same pristine Content scratch copy) into `t.TempDir()`, byte-diff the full
   resulting tree and captured stdout against the snapshot.

Plus the standing gates: per-task pin tests against exact TS extraction
commands (B1–B6 process), `CGO_ENABLED=0 go build -trimpath ./...`,
`go vet ./...`, `go test ./...`, `-race` on touched packages.

**Determinism hazard (baked into every implementer prompt):** TS iterates
`fs.readdirSync` / `listFilesExt` results and object-map registries; any Go
output ordering derived from a map or directory listing needs explicit
ordering (Arc-26 fix-determinism-first rule). `listFilesExt`'s actual TS
iteration order is part of the contract — verify, don't assume.

## Tracker & conventions

- PORTING.md gains a §B7 audit-trail subsection: correspondence table mapping
  all 31 TS files → goscape commit / decision row (6 NOT-PORTED rows above).
- `rev244-b6-ondemand-zip` caveat honored: unpack consumes FileStream/jag
  **entry content**, never raw-zip bytes.
- `TestDecodeRealCacheBlob` (Arc-26 residual decoder bug,
  `pkg/script/file.go`) is B7-adjacent but **out of scope** unless unpack work
  trips over it — if it does, it gets its own commit and tracker row.
- Deviations → PORTING.md rows; accepted divergences →
  `PORTING-EXCEPTION (<id>, …)` markers (22 at B6 close).

## Process

B1–B6-proven loop: this spec (commit) → writing-plans plan (bite-sized TDD,
exact TS extraction commands as contracts) → subagent execution (implementer
sonnet → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct leaf tasks) → full-suite gate + PORTING.md
correspondence-audit → whole-bundle integration review. Every implementer
prompt carries the B2–B6 recurring-defect list (`cat -n`-verified citations
with FINAL-FILE numbers, reject-path prerequisite seeding, call-site wiring
grep, interface-fake cascades, modules/world suite when world contracts move,
sandbox heredoc `!=` mangling, consumer-first modeling, empty-side-of-gate
question).
