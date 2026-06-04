# PackAll client-stages arc — design

**Date:** 2026-05-17
**Scope:** Port the 9 deferred TS `packAll` client-side stages plus the PixPack image codec they depend on. Retires `NAI-212-D-CLIENT-PACKERS-DEFERRED`.
**Reference:** `/home/owner/Code/github.com/LostCityRS/Engine-TS/tools/pack/`
**Tech stack:** Go 1.26+, stdlib only (no new third-party deps).

## Context

`pkg/pack.PackAll` (shipped at NAI-212, `4ba1a76..efa6a43`) currently runs three stages: `ClearFsCache → PackConfigs → compiler.RunServerCompiler`. TS `PackAll.ts:17-52` runs nine additional stages between/after those three, each producing a client-side artifact under `data/pack/client/...` or (for `packMaps`) `data/pack/server/maps/`. Those nine stages are tagged at `pkg/pack/pack_all.go:32` as `NAI-212-D-CLIENT-PACKERS-DEFERRED`.

This spec covers the entire arc in a single design rather than per-stage NAI sub-specs (per user direction). Implementation will be a single multi-task plan, dispatched subagent-driven.

## Stages in scope (TS-faithful execution order)

| # | TS function | TS file | TS LOC | Output |
|---|---|---|---|---|
| 1 | `packClientInterface` | `interface/PackClient.ts` + `PackShared.ts` | 27 + 597 | Jagfile `<outDir>/client/interface` + `<outDir>/server/interface.dat` |
| — | `runServerCompiler` (already ported) | — | — | — |
| 2 | `packClientTitle` | `sprite/title.ts` | 30 | Jagfile `<outDir>/client/title` |
| 3 | `packClientMedia` | `sprite/media.ts` | 34 | Jagfile `<outDir>/client/media` |
| 4 | `packClientTexture` | `sprite/textures.ts` | 21 | Jagfile `<outDir>/client/textures` |
| 5 | `packClientWordenc` | `chat/pack.ts` | 106 | Jagfile `<outDir>/client/wordenc` |
| 6 | `packClientSound` | `sound/pack.ts` | 53 | Jagfile `<outDir>/client/sounds` |
| 7 | `packClientGraphics` | `graphics/pack.ts` | 338 | Multiple Jagfiles under `<outDir>/client/models` |
| 8 | `packClientMidi` | `midi/pack.ts` | 35 | Raw bzip2 files under `<outDir>/client/{jingles,songs}/` |
| 9 | `packMaps` | `map/Pack.js` | 302 | Raw bzip2 zone files under `<outDir>/server/maps/` |

**Total TS LOC in scope:** ~1,517 (plus 214 LOC PixPack codec dep).

**Out of scope:** `packWorldmap` (TS `map/Worldmap.ts`, 682 LOC) — separate function, not called by `packAll`, tracked elsewhere.

## Design decisions (settled in brainstorm)

| # | Decision | Rationale |
|---|---|---|
| D1 | One big arc spec, single multi-task plan | User direction. Saves per-NAI close ceremony; single retirement of `NAI-212-D-CLIENT-PACKERS-DEFERRED`. |
| D2 | Synthetic-fixture byte-pin tests only | No dep on real LostCityRS datapack on test machines. Mirrors NAI-209/210/211 testing approach. |
| D3 | PixPack codec uses stdlib `image/png` + custom RGBA buffer | No new third-party deps. RGBA layout mirrors Jimp `bitmap.data` for byte-faithful palette/RLE logic. |
| D4 | `PackConfigs` returns a `*Registry` of named `*PackFile`s; client stages read from it | Avoids module-level singletons (un-Go-like) and per-stage re-construction (wasteful). |
| D5 | Inline `.land`/`.loc` parser in new `pkg/pack/maps` | Self-contained, ~80 LOC; no other caller justifies a separate parser package. |
| D6 | Port `BUILD_VERIFY` CRC checks now via `pkg/io/packet.CheckCRC` | Locks output-byte stability as a hard requirement. Retires `BUILD_VERIFY` portion of NAI-191/194/195 deferrals for stages in scope. |
| D7 | TS-faithful stage order inside `PackAll` | Trivial deviation audit. |
| D8 | Cluster subpackages: `pkg/pixpack`, `pkg/pack/clientinterface`, `pkg/pack/sprites`, `pkg/pack/wordenc`, `pkg/pack/audio`, `pkg/pack/graphics`, `pkg/pack/maps` | Mirrors TS dir shape; isolates 597-LOC interface and 338-LOC graphics from existing 19k-LOC `pkg/pack/` root. PixPack is top-level because it's a reusable codec. |

## Architecture

```
cmd/goscape-cli/pack
        │
        ▼
pkg/pack.PackAll(srcDir, outDir, dataPackDir) error
        │
        ├── ClearFsCache                                          (existing)
        ├── reg, _ := PackConfigsForRegistry(srcDir, outDir)      (modified)
        ├── clientinterface.Pack(reg, srcDir, outDir)             (NEW)
        ├── compiler.RunServerCompiler(srcDir, outDir, dataDir)   (existing)
        ├── sprites.PackTitle(srcDir, outDir)                     (NEW, → pkg/pixpack)
        ├── sprites.PackMedia(srcDir, outDir)                     (NEW, → pkg/pixpack)
        ├── sprites.PackTexture(reg, srcDir, outDir)              (NEW, → pkg/pixpack)
        ├── wordenc.Pack(srcDir, outDir)                          (NEW)
        ├── audio.PackSound(reg, srcDir, outDir)                  (NEW)
        ├── graphics.Pack(reg, srcDir, outDir)                    (NEW)
        ├── audio.PackMidi(srcDir, outDir)                        (NEW)
        └── maps.Pack(srcDir, outDir)                             (NEW)
```

### `pkg/pack.Registry`

Promoted from local vars in `pack_configs.go:107-127`:

```go
// Registry holds the *PackFile singletons that PackConfigs builds
// while packing. Client stages read from it after PackConfigs returns.
// Each accessor lazily constructs on first call and memoizes;
// nil-safe ConfigsOnly construction (no save).
//
// Field names match the TS singleton names (InterfacePack → Interface).
//
// Only Interface, Obj, Seq, Varp, Anim, Base, Model, Synth, and
// Texture are consumed by the new client stages; the remaining
// fields are exposed because PackConfigs already constructs them
// internally and surfacing them is free.
type Registry struct {
    srcDir string

    // Lazy-constructed; access via accessor methods, not field reads.
    Interface, Obj, Seq, Loc, Npc, Model, Anim, Base,
    Synth, Texture, Varp, Varn, Vars, Inv, SpotAnim, Idk,
    Flo, Category, Hunt, Param, DbTable, DbRow, MesAnim, Struct *PackFile
}

// PackConfigsForRegistry is the new entry point; PackConfigs is kept
// as a wrapper that discards the registry for non-PackAll callers.
func PackConfigsForRegistry(srcDir, outDir string) (*Registry, error) { ... }

func PackConfigs(srcDir, outDir string) error {
    _, err := PackConfigsForRegistry(srcDir, outDir)
    return err
}
```

The existing `ensure<Pack>` closures in `pack_configs.go` lift into `Registry` methods (or, equivalently, the function stays the same but the locals get hoisted to a `Registry` constructed up-front and returned). Either shape is acceptable; implementation chooses based on diff size.

### `pkg/pixpack`

```go
// ConvertImage reads <srcDir>/<name>.png, optionally reads
// <srcDir>/meta/<name>.opt for spritesheet metadata, encodes the
// image into the RS sprite format using either row-major or
// column-major pixel order (chosen by GeneratePixelOrder), and
// appends a frame to index. Returns the per-sprite payload Packet.
func ConvertImage(index *packet.Packet, srcDir, name string) (*packet.Packet, error)

// WriteImage emits the sprite payload bytes given a decoded RGBA
// bitmap, a destination data Packet, the shared index Packet, the
// palette colors, and optional spritesheet metadata.
func WriteImage(img *Bitmap, data, index *packet.Packet, colors []int32, meta *SpriteMeta)

// GeneratePixelOrder returns 0 (column-major) or 1 (row-major) by
// summing absolute RGB-delta scores across both traversal orders
// and picking the lower one. Matches TS PixPack.ts:7-32.
func GeneratePixelOrder(img *Bitmap) int

// Bitmap is the RGBA buffer shim. Layout: len = width*height*4,
// byte order R, G, B, A per pixel, row-major.
type Bitmap struct { Width, Height int; Data []uint8 }

// SpriteMeta is the parsed <srcDir>/meta/<name>.opt sidecar.
type SpriteMeta struct { ... }  // shape per TS Sprite interface
```

Internal helpers: `generatePalette(img *Bitmap) []int32`, `loadSpriteMeta(srcDir, name string) (*SpriteMeta, error)`, `decodePNG(path string) (*Bitmap, error)`.

### Per-stage packages

Each per-stage package exports exactly one (sometimes two) `Pack...` function returning `error`. Internal helpers (parsers, type-int dispatchers, name normalizers) stay unexported. Stage-specific notes:

- **`pkg/pack/clientinterface`** — Single `Pack(reg, srcDir, outDir) error` that internally runs the `packInterface` workhorse returning `(client, server *packet.Packet)`. `nameToType`/`nameToButtonType`/`nameToComparator`/`nameToScript`/`nameToStat`/`nameToFont` six dispatchers ported as unexported funcs. BUILD_VERIFY CRC `-2146838800` enforced via `pkg/pack.BuildVerify`.
- **`pkg/pack/sprites`** — Three public funcs (`PackTitle`, `PackMedia`, `PackTexture`). `PackMedia` sorts spritesheets last via `.opt`-sidecar existence test (TS `media.ts:16-20`).
- **`pkg/pack/wordenc`** — Single `Pack(srcDir, outDir) error`. Reads 4 text files (`badenc.txt`, `fragmentsenc.txt`, `tldlist.txt`, `domainenc.txt`), each parsed into its own packet, then bundled into one Jagfile.
- **`pkg/pack/audio`** — Two public funcs. `PackSound` reads `synth.order` + per-file `.synth` data, gates by `reg.Synth.GetByName`. `PackMidi` is a per-file mtime/existence-gated bzip2 copy under `jingles/` and `songs/`.
- **`pkg/pack/graphics`** — Single `Pack(reg, srcDir, outDir) error`. Reads model `.ob2`, anim `.frame`, base `.base` source files; emits 21 named `.dat` streams in fixed order (TS `graphics/pack.ts:16-29`).
- **`pkg/pack/maps`** — Single `Pack(srcDir, outDir) error`. Inline `readMap(lines []string) (land, loc, npc, obj map)`, `packKey(level, x, z int) int`. Reads `<srcDir>/maps/*.land` and `<srcDir>/maps/*.loc`; emits one bzip2 file per `(level, mx, mz)` zone under `<outDir>/server/maps/`.

### Cross-cutting additions to `pkg/pack`

```go
// BuildVerify checks a CRC and returns an error on mismatch.
// Used by clientinterface (active) and audio (commented in TS,
// constant retained as `soundCRCMagic`).
func BuildVerify(data []uint8, length int, expected int32) error
```

A new banned-identifier-pin file `pkg/pack/nai_N_buildverify_pins_test.go` asserts:
- `BUILD_VERIFY` (literal env var name) does NOT appear in any new packager file (no env-driven gating; CRC is unconditional).
- The two CRC magic numbers (`-2146838800` for interface, `-1570057128` for sound) appear exactly once each, in `clientinterface/pack.go` and `audio/sound.go` respectively.

## Output paths

All paths anchor on the `outDir` parameter (matches existing `pkg/pack` convention; TS hardcodes `data/pack/...` relative paths):

| Stage | Output |
|---|---|
| Interface | `<outDir>/client/interface` (Jagfile) + `<outDir>/server/interface.dat` (raw) |
| Title | `<outDir>/client/title` (Jagfile) |
| Media | `<outDir>/client/media` (Jagfile) |
| Texture | `<outDir>/client/textures` (Jagfile) |
| Wordenc | `<outDir>/client/wordenc` (Jagfile) |
| Sound | `<outDir>/client/sounds` (Jagfile) |
| Graphics | `<outDir>/client/models` (Jagfile, multiple writes) |
| Midi | `<outDir>/client/jingles/<f>` + `<outDir>/client/songs/<f>` (raw bzip2 files) |
| Maps | `<outDir>/server/maps/<level>_<mx>_<mz>.{land,loc,npc,obj}` (raw bzip2 files) |

## Error handling

- Each stage returns `error`; `PackAll` wraps with `"PackAll: <Stage>: %w"`.
- `shouldBuild*` no-op gates return cleanly (no error, no output) — matches `NAI-192-D-NO-SRC-NO-OP`.
- `BuildVerify` mismatch surfaces as `error`, wrapped at the stage boundary.
- No `os.Exit`/`panic` paths — matches `NAI-211-D-NO-PROCESS-EXIT`.
- File I/O errors propagate verbatim; no silent swallowing.

## Testing strategy

### Per-package unit tests (synthetic fixtures only — D2)

| Pkg | Test surface |
|---|---|
| `pkg/pixpack` | `TestGeneratePixelOrder_RowMajorPreferred`, `TestGeneratePixelOrder_ColumnMajorPreferred`, `TestGeneratePalette_Dedup`, `TestWriteImage_RowMajorWithMeta`, `TestWriteImage_ColumnMajorNoMeta`, `TestConvertImage_RoundTripBytePin` (helper `writeSyntheticPNG(t, w, h, pixels)` using stdlib `image/png`) |
| `clientinterface` | `TestPack_BytePinned` with a ≤3-component-per-type fixture (layer + inv + rect + text + graphic + model + invtext); `TestPack_BuildVerifyMismatchSurfaces` (force a one-byte mutation, assert error) |
| `sprites` | `TestPackTitle_BytePinned` (8 fixed-name PNG fixtures + title.jpg + fonts); `TestPackMedia_SpritesheetsSortedLast`; `TestPackMedia_BytePinned`; `TestPackTexture_BytePinned` |
| `wordenc` | `TestPack_BytePinned` (4 minimal text fixtures, 1–2 lines each, including a badenc entry with combinations) |
| `audio` | `TestPackSound_BytePinned`; `TestPackSound_RespectsOrder`; `TestPackMidi_CompressesNew`; `TestPackMidi_SkipsExisting` |
| `graphics` | `TestPack_BytePinned` (minimal model/anim/base source set covering at least one of each of the 21 output streams) |
| `maps` | `TestReadMap_LandLocNpcObjSections`; `TestPackKey_Encoding`; `TestPack_BytePinned` for `(0, 50, 50)` zone |
| `pkg/pack.Registry` | `TestRegistry_LazyConstruct`; `TestRegistry_IdempotentAccess`; `TestRegistry_AllFieldsAccessibleAfterPackConfigs` |

### Integration tests in `pkg/pack`

- Extend existing `TestPackAll_ThreeStageSmoke` (`pack_all_test.go`) — rename to `TestPackAll_TwelveStageSmoke`; fixture grows by `.if`, 8 fixed-name PNGs, `.synth`, `synth.order`, `.land`, `.loc`, 4 wordenc text files; assert presence and minimum byte size for each `<outDir>/client/...` artifact and `<outDir>/server/maps/` zone files.
- Add `TestPackAll_ClientInterfaceErrorPropagates` — induce a CRC mismatch (mutate a fixture byte) and assert wrapped error string.

### Banned-identifier pins

- New `pkg/pack/nai_N_buildverify_pins_test.go` asserting the two CRC magic numbers exist exactly once at their expected locations.
- Existing NAI-191/194/195 pins kept as-is (they pin server-side validator absence; this arc doesn't change that).

## Anticipated deviation tags

| Tag | Location | Retire when |
|---|---|---|
| `NAI-N-D-REGISTRY-RETURN` | `pkg/pack/pack_configs.go` | Permanent (structural shape change from TS module-level singletons). |
| `NAI-N-D-PIXPACK-RGBA-LAYOUT` | `pkg/pixpack/bitmap.go` | Permanent (Go uses stdlib `image/png` decode; bitmap layout documented for byte-faithfulness check). |
| `NAI-N-D-PACKMAPS-PRINTWARN-LOG` | `pkg/pack/maps/pack.go` | Permanent (TS `printWarning` → goscape standard log). |
| `NAI-N-D-PACKMIDI-MTIME-CHECK-MIRROR-TS-TODO` | `pkg/pack/audio/midi.go` | Both TS and goscape implement mtime check (TS comment `// TODO: mtime-based check`). |
| `NAI-N-D-SOUND-CRC-DISABLED-MIRROR-TS` | `pkg/pack/audio/sound.go` | TS un-comments the CRC check. |
| `NAI-N-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE` *(provisional)* | `pkg/pack/clientinterface/` | Empirically resolved at T1 — if our output bytes match TS CRC, this tag is dropped before commit. If they diverge (due to upstream NAI-209/210 byte-deferrals), tag stays and pin test asserts CRC-fail-only. |

Per-stage T2-emergent deviations are expected (NAI-212 saw 2 emergent tags during driver wiring) and will be added during implementation.

## What's not in this spec

- **`cmd/goscape-cli/pack` CLI surface** — already shipped at NAI-212-FU. New stages run automatically once `PackAll` calls them; no CLI changes needed.
- **`::rebuild` cheat in-process wiring** — separate future NAI per NAI-212 close.
- **`packWorldmap`** — separate function not called by `packAll`; tracked elsewhere.
- **Server-side BUILD_VERIFY_FOLDER validators** — covered by existing NAI-191/194/195 deferrals; this arc only ports the client-output CRC variant.
- **Server-side packer macros** — `NAI-211-D-MACRO-LOOKUP-DEFERRED` stays live.
- **`legends_guard.rs2` pointer-checker residual** — orthogonal; tracked elsewhere.

## Plan slicing preview

The follow-up plan will use these tasks (subagent-driven, per-task TDD + two-stage review):

1. **T1** — `pkg/pack.Registry` extraction + `PackConfigsForRegistry` (+ `BuildVerify` helper). Backward-compatible `PackConfigs` wrapper. Tests cover lazy access + idempotency.
2. **T2** — `pkg/pixpack` codec (decode, bitmap, palette, write, convertImage). All tests synthetic-PNG byte-pinned.
3. **T3** — `pkg/pack/wordenc` (smallest TS-source-faithful stage, validates byte-pin discipline).
4. **T4** — `pkg/pack/audio` (PackSound + PackMidi).
5. **T5** — `pkg/pack/maps` (inline parser + writer).
6. **T6** — `pkg/pack/sprites` (PackTitle + PackMedia + PackTexture).
7. **T7** — `pkg/pack/graphics` (largest non-interface stage).
8. **T8** — `pkg/pack/clientinterface` (largest stage; benefits from all earlier work).
9. **T9** — `pkg/pack.PackAll` wiring (all 9 new stage calls + integration smoke) + retire `NAI-212-D-CLIENT-PACKERS-DEFERRED`.

T1–T8 are individually unit-testable and order-flexible after T1+T2; T9 is the integration cap.
