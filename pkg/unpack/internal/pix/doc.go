// Package pix decodes RS2 sprite cache entries (Jagfile-packed .dat + index.dat)
// and exports them as PNG images and optional .opt metadata files.
//
// This is a faithful 1:1 Go port of the decode/export path of the TypeScript
// class at:
//
//	Server244-ref/engine/src/cache/graphics/Pix.ts
//
// Ported entry points:
//   - [UnpackJag]  — TS Pix.ts:72-139  (binary sprite decode from jag)
//   - [UnpackFull] — TS Pix.ts:33-70   (PNG + .opt export, multi-sprite sheet)
//
// The encode half (packHeader, pack, packPng) is intentionally NOT ported here.
// Encoding is YAGNI for the unpack tool; the existing Go pack pipeline
// (pkg/pack/sprites) owns the encode side for production use.
//
// Dropped TS parameters and dead branches (no caller at the pinned revision uses them):
//
//   - unpackFull overrideName (TS Pix.ts:33, `overrideName?: string`): optional parameter
//     that renames the output files; ported as the outputName parameter of [UnpackFull]
//     (used by the textures unpack path which passes TexturePack names as output stems).
//
//   - unpackJagToPng explicit sheetWidth/sheetHeight args (TS Pix.ts:141,
//     `sheetWidth: number = 0, sheetHeight: number = 0`): the non-zero branch lets callers
//     supply fixed dimensions instead of using the auto-computed layout; all call sites use
//     the defaulted zeros so only the auto-layout path is ported.
//
//   - unpackJagToPng preferHorizontal=false branch (TS Pix.ts:182-186): the else branch of
//     the widen loop that decrements width / increments height; all call sites default to
//     preferHorizontal=true so only the increment-width path is ported.
package pix
