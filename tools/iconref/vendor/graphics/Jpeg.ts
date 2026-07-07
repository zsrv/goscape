// STUB (NOT verbatim) — replaces LostCityRS/Client-TS src/graphics/Jpeg.ts
// (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9).
//
// The original file is browser-only: it decodes JPEG via
// `document.createElement('canvas')` + `<img>` + `URL.createObjectURL`. Those
// APIs do not exist under Node. Its single export `decodeJpeg` is imported by
// Pix32.ts and used ONLY inside `Pix32.fromJpeg` (the JPEG background/photo
// loader). The item-icon rasterizer path — ObjType.getSprite, Pix3D texture
// unpack (Pix8.depack), Model rendering — never touches JPEG decoding, so this
// stub is never invoked. It preserves the module surface and throws loudly if
// anything ever reaches it, guaranteeing it cannot silently alter render math.
export async function decodeJpeg(_data: Uint8Array): Promise<never> {
    throw new Error('Jpeg.decodeJpeg is stubbed in the iconref harness: JPEG decoding (Pix32.fromJpeg) is browser-only and is never used by the item-icon rasterizer path.');
}
