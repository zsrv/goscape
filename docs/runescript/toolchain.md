# Toolchain

Turning a `.rs2` source tree into a cache the server can serve is the job of
**`goscape-cli`**, the offline tooling binary that ships alongside the `goscape`
server (`make all` builds both). This page is the reference for its three
content-facing verbs:

- **`compile`** — type-check and (with `-check`) get fast diagnostics on the
  script tree without producing a cache.
- **`pack`** — run the full pipeline that compiles the scripts and packs every
  other archive into the game cache.
- **`smoke-pack`** — a diagnostic driver that runs every pack stage
  individually, reports per-stage results, and can byte-diff its output against
  a reference cache.

For the shape of a script and a first compile-checked example, read
[Writing scripts](writing-scripts.md); for getting a freshly packed cache onto a
running server, read [Deploying scripts](deploying.md). The operational side of
packing — `make pack`, the `CACHE_*` variables, and when to repack — lives in
the [Operations runbook](../admin/operations.md#cache-packing).

## The RuneScript compiler

goscape does not shell out to an external compiler. It compiles RuneScript with
its own Go implementation (`pkg/pack/compiler`, with the `.dat`/`.idx` writer in
`pkg/pack/compiler/runescript`) — a faithful port of the LostCity reference
compiler. What that port tracks differs by revision, because the reference
compiler itself changed across the game's history: the earliest revisions used
the TypeScript **RuneScriptTS** (`@lostcityrs/runescript`), the middle revisions
switched to the Kotlin **RuneScriptKt** (a downloaded `RuneScriptCompiler.jar`),
and the later revisions returned to an advanced RuneScriptTS. goscape's compiler
output is byte-checked against the cache the matching upstream compiler packs, so
the reference is pinned per revision in
[`REFERENCES.md`](../contributor/references-pins.md).

This build documents revision **{{ revision }}**. The full lineage:

| Revision | Reference compiler | Version pin | Compiled cache version |
|---|---|---|---|
| rev-225 | RuneScriptTS — `@lostcityrs/runescript` (npm) | `^0.9.4` | 26 |
| rev-244 | RuneScriptKt — `RuneScriptCompiler.jar` | release tag `26` (`COMPILER_VERSION = 26`) | 26 |
| rev-245.2 | RuneScriptKt — `RuneScriptCompiler.jar` | release tag `26` (`COMPILER_VERSION = 26`) | 26 |
| rev-254 | RuneScriptTS — `@lostcityrs/runescript` (npm) | `^0.9.6` (`COMPILER_VERSION = 27`) | 27 |
| rev-274 | RuneScriptTS — `@lostcityrs/runescript` (npm) | `0.9.6` (unchanged from rev-254) | 27 |

The **compiled cache version** is the version byte written into the script
archive header (`script.dat`). It stepped from 26 to 27 at rev-254, when the
reference compiler advanced past the upstream version bump; goscape's server
reads that byte on load and refuses a cache whose version does not match the
engine it was built for. The value is a build product of the revision, not
something you set.

!!! note "You do not install a compiler"
    Because the compiler is built into `goscape-cli`, there is no separate npm
    package or `.jar` to fetch. The table above is the *reference* each
    revision's Go port was translated from and is byte-compared against — see
    the [byte-parity philosophy](deploying.md#byte-parity) on the Deploying
    page. Pack with the `goscape-cli` built from the **same revision** as your
    server binary.

## `compile` — check the script tree

`goscape-cli compile -check` type-checks the `.rs2` tree and reports diagnostics
without writing a cache. It is the fast inner-loop tool: it catches unresolved
commands, unknown entities, and type errors in a second or two, before you spend
the time to pack. The [Writing scripts](writing-scripts.md#compile-checking-it)
page walks through it with a worked example and a sample error; the essentials:

```console
$ goscape-cli compile -check \
    -src-dir data/src \
    -datapack-dir data/pack \
    data/src/scripts
```

- **`-check`** discards the compiled output — diagnostics only. Without it,
  `compile` writes only the script archive (`script.dat` / `script.idx`) into
  `<out-dir>/server`, which is not a complete cache; to produce a runnable cache
  use [`pack`](#pack-build-the-cache).
- **`-src-dir`** (default `data/src`) is the content root — the directory
  holding `scripts/` and `pack/` — from which the compiler loads its entity
  symbols.
- **`-datapack-dir`** (default `data/pack`) points at the packed entity-type
  cache the compiler resolves ids against.
- **`-out-dir`** (default `data/pack`) is the output directory; it is ignored
  when `-check` is set.
- The final positional argument is the source path to compile.

!!! warning "Compile the whole tree, not a lone file"
    Engine commands such as `mes` are declared in `scripts/engine.rs2`, and any
    procs a script calls are defined elsewhere in the tree. The compiler only
    sees files under the path you give it, so a single-file compile fails to
    resolve those references. Point the positional argument at the whole
    `scripts/` directory (`data/src/scripts`), which the compiler walks
    recursively. See
    [Compile-checking it](writing-scripts.md#compile-checking-it).

A clean run logs `compile succeeded` and exits `0`. A failure prints the file,
line, and column of the first error and exits non-zero.

## `pack` — build the cache

`goscape-cli pack` runs the **whole** content pipeline: it compiles the scripts
(one stage among many) and packs configs, interfaces, sprites, textures, sounds,
music, models, maps, and the word-encoding table into the game cache the client
downloads and the world reads maps from. In practice you invoke it through the
Makefile:

```bash
make pack
```

`make pack` calls `goscape-cli pack` with three directories, each overridable as
a make variable ([Operations runbook](../admin/operations.md#packing-the-cache)):

| `pack` flag | Make variable | Default | Contents |
|---|---|---|---|
| `--src-dir` | `CACHE_SRC_DIR` | `data/src` | Source content to pack (`scripts/`, `pack/`, …) |
| `--out-dir` | `CACHE_OUT_DIR` | `data/pack` | Packed output — point `world.cache_path` and OnDemand here |
| `--raw-dir` | `CACHE_RAW_DIR` | `data/raw` | Engine-owned raw blobs (the `wordenc` Jagfile) |
| `--datapack-dir` | — | falls back to `--out-dir` | Entity-type cache directory |

`pack` writes the RS2 client cache files (`main_file_cache.*`) plus the
server-side archives (`server/script.dat`, `server/*.dat`, …) under `--out-dir`.
The packed cache is a build product of the source: editing content under
`--src-dir` has no effect on a running server until you repack. When to repack —
after content changes, and when upgrading to a revision whose engine expects a
newer cache — is covered in the
[Operations runbook](../admin/operations.md#when-to-repack).

## `smoke-pack` — per-stage report and byte-diff

`goscape-cli smoke-pack` runs the same pipeline as `pack`, but stage by stage and
best-effort: every stage is timed and reported independently, a crashing stage is
caught rather than aborting the run, and — when you point it at a reference cache
— each stage's output is byte-diffed against that reference. It is the tool for
answering "did my change break packing, and if so, where?" and "does my cache
still match the upstream reference byte-for-byte?".

```console
$ goscape-cli smoke-pack -content-dir data/src
```

Key flags:

- **`-content-dir`** (required) — the source content directory.
- **`-out-dir`** — where to write. Empty auto-creates a temp directory that is
  deleted on exit unless `-keep` is set.
- **`-datapack-dir`** — entity-type cache directory used by the server compiler
  stage. Empty falls back to the effective `-out-dir`.
- **`-reference-dir`** — a reference pack output (typically the upstream
  TS-packed `data/pack`) to byte-diff each stage against. Empty disables the
  diff.
- **`-keep`** — preserve an auto-created `-out-dir` on exit.
- **`-stop-on-error`** — exit at the first failing stage instead of logging it
  and continuing.

The run ends with a per-stage table and a summary line:

```text
STAGE              STATUS  ELAPSED  FILES  BYTES     ERR
PackConfigs        OK      412ms    128    3187204
ClientInterface    OK      36ms     129    3201884
...
Worldmap           OK      21ms     140    5127713

Result: 14 OK, 0 ERR, 0 SKIP  total elapsed: 1.2s
```

The stages run in pipeline order: `PackConfigs`, `ClientInterface`,
`CompilerSymbols`, `RunServerCompiler` (this is where the scripts compile),
`Title`, `Media`, `Texture`, `Wordenc`, `Sound`, `Graphics`, `Midi`, `Maps`,
`VersionList`, and a standalone `Worldmap` re-run. `PackConfigs` is special: it
produces the registry every later stage consumes, so if it fails every
downstream stage reports `SKIP`.

The exit code encodes the outcome: `0` all stages succeeded, `1` at least one
stage failed, `2` a flag-parse error, `3` a setup error (missing or unreadable
`-content-dir`, logger init, etc.).

### Byte-diffing against a reference

With `-reference-dir` set, the table gains a `DIFF` column counting per-stage
byte-divergences, and a `Diff details:` block lists them. Each divergence is one
of four kinds:

| Kind | Meaning |
|---|---|
| `DIFF` | Bytes differ — reported with `offset`, `got`, and `want`. |
| `SIZE` | The output and reference file are different lengths. |
| `MISS` | The file is present in the output but absent from the reference. |
| `ERR` | The comparison itself failed (e.g. an unreadable file). |

A run with `0` diffs across all stages is the byte-parity proof the pack pipeline
is designed to produce — see [byte-parity philosophy](deploying.md#byte-parity).

!!! note "Reference diffs are sensitive to the content path"
    The packed `server/script.dat` embeds the source path exactly as passed via
    `-content-dir` (both goscape and the reference are case-faithful to their
    build environment). For a byte-faithful diff against a reference packed from
    the upstream default path, point `-content-dir` at a directory whose
    absolute path matches the reference build's — `goscape-cli smoke-pack -h`
    documents the exact caveat.

## Where to go next

- **[Deploying scripts](deploying.md)** — the edit → compile → pack → reload dev
  loop and the production procedure.
- **[Writing scripts](writing-scripts.md)** — file layout and a worked
  compile-checked first script.
- **[Operations runbook](../admin/operations.md#cache-packing)** — `make pack`,
  the `CACHE_*` variables, and when to repack.
- **[References & pins](../contributor/references-pins.md)** — the per-revision
  upstream compiler and content pins.
