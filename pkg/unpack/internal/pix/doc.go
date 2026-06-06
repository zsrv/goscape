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
package pix
