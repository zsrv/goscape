# iconref — TS reference harness for the goscape item-icon rasterizer

This directory renders **golden pixels** for `pkg/render/testdata/` directly from
the **vendored, byte-for-byte** LostCityRS/Client-TS render code. Every later Go
porting task diffs its output against these goldens, so the goldens are the single
source of truth: they must come from the real client math, never a re-implementation.

## Provenance

- Vendored from: **LostCityRS/Client-TS**, branch `274-GOSCAPE`,
  commit **`b67894260fb06ae6162ed3a8adab506abcd7faa9`**.
- Reference cache (read-only): `LostCityRS/Server274-ref/unpack-ref/cache/`
  (`main_file_cache.dat` + `idx0..idx4`).
- debugname↔id map: `LostCityRS/Server274-ref/content/pack/obj.pack` (3894 entries).

Every file under `vendor/` is a verbatim copy of its Client-TS source with a
3-line provenance header prepended (source path + branch + commit + per-file blob
SHA). Nothing else in those files was edited: imports use the `#/...` alias, which
`package.json`'s `"imports": { "#/*": "./vendor/*" }` resolves to `vendor/`, so no
import rewriting was needed.

## Stubs (documented per the harness rule)

Exactly **one** file is not verbatim:

| file | why | effect on render math |
|------|-----|-----------------------|
| `vendor/graphics/Jpeg.ts` | Original decodes JPEG via `document.createElement('canvas')` + `<img>` + `URL.createObjectURL` — browser-only APIs absent under Node. | **None.** Its sole export `decodeJpeg` is used only by `Pix32.fromJpeg` (the JPEG photo loader). The item-icon path — `ObjType.getSprite`, `Pix3D.unpackTextures`/`Pix8.depack`, `Model` rendering — never calls it. The stub preserves the module surface and throws if ever reached, so it cannot silently alter math. |

The remaining 24 vendored files (`ObjType`, `Model`, `Pix3D`, `Pix2D`, `Pix8`,
`Pix32`, `JagFile`, `Packet`, `BZip2`, `Isaac`, `LruCache` + `Linkable`/`Linkable2`/
`HashTable`/`LinkList`/`LinkList2`, `AnimBase`, `AnimFrame`, `ModelSource`,
`PointNormal`, `Colour`, `Arrays`, `JsUtil`, `OnDemandProvider`) are the complete,
self-contained transitive closure and are unmodified. The only non-file "surface"
supplied by the harness is a no-op `OnDemandProvider` (`{ requestModel(){} }`)
passed to `Model.init`; it is never invoked because all archive-1 models are
pre-unpacked before any `Model.load`.

## Determinism override (critical)

`Pix3D.initColourTable(brightness)` jitters brightness by
`Math.random() * 0.03 - 0.015`. A golden must be reproducible bit-for-bit, so
`dump.ts` pins `Math.random = () => 0.5` **only** around that call:
`0.5 * 0.03 - 0.015 = 0` exactly, so `randomBrightness === brightness === 0.8`.
**The Go port must build its palette with the un-jittered `0.8` brightness** to
match `palette.bin`. This is the one intentional deviation from a literal run of
the client, and it removes randomness rather than adding behavior.

## Cache reader

`dump.ts` contains a ~55-line classic `.dat/.idx` reader (520-byte sectors, 6-byte
idx entries) mirroring `pkg/io/filestream/filestream.go`. Archive 0 (config jags)
is stored raw; archives 1-4 (models/anims/midi/maps) are gzip-per-file. Validation:
loading the config jag (archive 0, file 2) and running `ObjType.init` yields
**numDefinitions = 3894**, matching goscape's unpack and `obj.pack`'s line count.
The textures jag is archive 0, file 6 → `Pix3D.unpackTextures` (numTextures = 50).

## Init order

Mirrors `Client.ts:1152-1154`: `unpackTextures` → `initColourTable(0.8)` →
`initPool(20)`.

## Outputs (written to `../../pkg/render/testdata/`)

- `palette.bin` — 65536 × int32 **little-endian** (`Pix3D.colourTable`), 262144 bytes.
- `tri/*.rgba` + `tri/manifest.json` — synthetic single-triangle goldens
  (gouraud small/large/degenerate/alpha-128, flat, and three textured cases:
  uniform-shade texture 1, varying-shade opaque texture 2, varying-shade
  transparent texture 7). The manifest records every input (routine, coords,
  colours, `lowDetail`, `hclip`, `trans`, prefill, texture id/view-space verts)
  so each case is exactly replayable.

  Shade arithmetic note (traced; shade `s` is NOT "128 = full-bright"):
  `textureTriangle` shifts `s <<= 16` and hands the raster `s >> 8`; the raster
  then does `<<= 9` and derives `shadeShift = s >> 6` plus bank-select bits from
  `s & 0x30`. So `s=32` → shift 0 (full-bright), `s=128` → shift 2 (darkened),
  `s=200` → shift 3. Equal shades at all three vertices leave every
  shadeStep/shadeStrides term zero, hence the varying-shade cases.
- `lit/dagger.json` — `bronze_dagger` (id 1205) post-`calculateNormals`
  `faceColourA/B/C` (length == model FaceCount).
- `icons274/<debugname>.rgba` + `.png` — the curated sample icons via
  `ObjType.getSprite(id, 1, 0)` (note the TS arg order `id, count, outlineRgb`).
- `icons274/sample.json` (also copied to `./sample.json`) — the chosen items with
  the category each satisfies.

### RGBA conversion rule (implement identically in Go)

A `Pix32`/`Pix2D` buffer holds `0x00RRGGBB` ints; value `0` is the client's
"not drawn / transparent" sentinel (icon backgrounds are `fillRect`'d with
`Colour.BLACK === 0`). Conversion:

- `pix === 0` → RGBA `(0, 0, 0, 0)` — transparent.
- `pix !== 0` → RGBA `(R, G, B, 255)` — opaque, RGB from the low 24 bits.

## Sample categories

The ~40 sample targets these categories (queried over loaded `ObjType`s / model
metadata): `untextured_gouraud`, `flat_faces`, `textured_faces`, `recoloured`,
`ambient_contrast`, `certtemplate` (noted), `countobj_stack`, `alpha_faces`, and
`resized`. `bronze_dagger` and `knife` are always included.

**Note on `resized`:** a full sweep of all 3894 objs found **zero** with an
obj-level resize (decode codes 110/111/112 — `resizex/y/z != 128`); in rev-274,
model resizing lives on loc/npc configs, not obj configs. The category is
therefore legitimately empty in `sample.json`, not a harness gap.

## Run

```bash
npm install          # tsx + typescript (network; sandbox override expected)
npx tsx dump.ts --cache <ref cache dir> --out ../../pkg/render/testdata
```

`node_modules/` and `scratch/` are git-ignored; the committed artifacts are the
harness sources plus the goldens under `pkg/render/testdata/`.
