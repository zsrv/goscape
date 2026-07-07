// dump.ts — TS reference harness for the goscape item-icon rasterizer (pkg/render).
//
// Renders rev-274 golden pixels straight from the VENDORED, byte-for-byte
// LostCityRS/Client-TS render code (tools/iconref/vendor/, pinned at commit
// b67894260fb06ae6162ed3a8adab506abcd7faa9, branch 274-GOSCAPE). Every golden
// this file writes is the single source of truth that later Go porting tasks
// diff against, so this harness must call the vendored routines EXACTLY as the
// real client does and must never re-implement any render math.
//
// Outputs (all under --out, default ../../pkg/render/testdata):
//   palette.bin              65536 × int32 LE  (Pix3D.colourTable)
//   tri/*.rgba + manifest    synthetic single-triangle goldens
//   lit/dagger.json          bronze_dagger post-calculateNormals faceColourA/B/C
//   icons274/*.rgba + *.png  ~40 item icons via ObjType.getSprite(id, 1, 0)
//   icons274/sample.json     the curated sample (also copied to ./sample.json)
//
// Run:
//   npx tsx dump.ts --cache <ref cache dir> --out ../../pkg/render/testdata
//
// DETERMINISM NOTE (critical): Pix3D.initColourTable() jitters brightness by
// `Math.random() * 0.03 - 0.015`. A golden must be reproducible, so before
// calling it we pin Math.random to 0.5 → jitter = 0.5*0.03 - 0.015 = 0 exactly,
// making randomBrightness === brightness (0.8) to the bit. The Go port must use
// the un-jittered 0.8 brightness to match palette.bin. See README.md.

import * as fs from 'node:fs';
import * as path from 'node:path';
import * as zlib from 'node:zlib';

import ObjType from '#/config/ObjType.js';
import Model from '#/dash3d/Model.js';
import Pix3D from '#/dash3d/Pix3D.js';
import Pix2D from '#/graphics/Pix2D.js';
import Pix32 from '#/graphics/Pix32.js';
import JagFile from '#/io/JagFile.js';

// ---------------------------------------------------------------------------
// args
// ---------------------------------------------------------------------------
function argVal(name: string, def: string): string {
    const i = process.argv.indexOf(name);
    return i >= 0 && i + 1 < process.argv.length ? process.argv[i + 1] : def;
}
const HERE = path.dirname(new URL(import.meta.url).pathname);
const CACHE_DIR = path.resolve(argVal('--cache', '/home/owner/Code/github.com/LostCityRS/Server274-ref/unpack-ref/cache'));
const OUT_DIR = path.resolve(HERE, argVal('--out', '../../pkg/render/testdata'));
const OBJ_PACK = argVal('--objpack', '/home/owner/Code/github.com/LostCityRS/Server274-ref/content/pack/obj.pack');

function ensureDir(p: string): void {
    fs.mkdirSync(p, { recursive: true });
}

// ---------------------------------------------------------------------------
// Minimal classic .dat/.idx cache reader (520-byte sectors, 6-byte idx entries).
// Format mirrors goscape pkg/io/filestream/filestream.go (which itself ports the
// 244 engine FileStream): idx entry = size(3 BE) + firstSector(3 BE); each dat
// sector = 8-byte header [file(2), part(2), nextSector(3), archive+1(1)] + up to
// 512 payload bytes. Archive 0 (config jags) is stored raw; archives 1-4 (models,
// anims, midi, maps) are gzip-compressed per file.
// ---------------------------------------------------------------------------
const SECTOR = 520;
const SECTOR_DATA = 512;
class Cache {
    dat: Buffer;
    idx: Buffer[] = [];
    constructor(dir: string) {
        this.dat = fs.readFileSync(path.join(dir, 'main_file_cache.dat'));
        for (let i = 0; i <= 4; i++) {
            this.idx.push(fs.readFileSync(path.join(dir, `main_file_cache.idx${i}`)));
        }
    }
    count(archive: number): number {
        if (archive < 0 || archive > 4) return 0;
        return (this.idx[archive].length / 6) | 0;
    }
    // Sector-walk a file into its raw (still-compressed for archives 1-4) bytes.
    readRaw(archive: number, file: number): Uint8Array | null {
        if (archive < 0 || archive > 4) return null;
        const idx = this.idx[archive];
        if (file < 0 || file * 6 + 6 > idx.length) return null;
        const size = (idx[file * 6] << 16) | (idx[file * 6 + 1] << 8) | idx[file * 6 + 2];
        let sector = (idx[file * 6 + 3] << 16) | (idx[file * 6 + 4] << 8) | idx[file * 6 + 5];
        if (size <= 0 || size > 2_000_000) return null;
        const maxSector = (this.dat.length / SECTOR) | 0;
        if (sector <= 0 || sector > maxSector) return null;

        const out = new Uint8Array(size);
        let pos = 0;
        for (let part = 0; pos < size; part++) {
            if (sector === 0) break;
            const base = sector * SECTOR;
            const sectorFile = (this.dat[base] << 8) | this.dat[base + 1];
            const sectorPart = (this.dat[base + 2] << 8) | this.dat[base + 3];
            const nextSector = (this.dat[base + 4] << 16) | (this.dat[base + 5] << 8) | this.dat[base + 6];
            const sectorIndex = this.dat[base + 7];
            if (file !== sectorFile || part !== sectorPart || archive !== sectorIndex - 1) {
                throw new Error(`cache: sector header mismatch archive=${archive} file=${file} part=${part}`);
            }
            if (nextSector < 0 || nextSector > maxSector) return null;
            const avail = Math.min(SECTOR_DATA, size - pos);
            out.set(this.dat.subarray(base + 8, base + 8 + avail), pos);
            pos += avail;
            sector = nextSector;
        }
        return out;
    }
    // Full read: gunzip archives 1-4, return archive 0 raw.
    read(archive: number, file: number): Uint8Array | null {
        const raw = this.readRaw(archive, file);
        if (!raw) return null;
        if (archive === 0) return raw;
        return new Uint8Array(zlib.gunzipSync(raw));
    }
}

// ---------------------------------------------------------------------------
// RGBA conversion — ONE definition, mirrored verbatim by the Go side (pkg/render).
//
// A Pix32/Pix2D pixel buffer holds 0x00RRGGBB ints. The value 0 is the client's
// "not drawn / transparent" sentinel (icon backgrounds are fillRect'd with
// Colour.BLACK === 0x0 and only overwritten where the model rasterizes). So:
//     pix === 0  ->  RGBA (0,   0,   0,   0)     fully transparent
//     pix !== 0  ->  RGBA (R,   G,   B,   255)   fully opaque, RGB = low 24 bits
// This exact rule is what the Go port must apply when turning its int32 buffer
// into the .rgba goldens.
// ---------------------------------------------------------------------------
function toRGBA(pix: Int32Array, w: number, h: number): Buffer {
    const out = Buffer.alloc(w * h * 4);
    for (let i = 0; i < w * h; i++) {
        const p = pix[i] | 0;
        if (p === 0) continue; // leaves (0,0,0,0)
        out[i * 4] = (p >>> 16) & 0xff;
        out[i * 4 + 1] = (p >>> 8) & 0xff;
        out[i * 4 + 2] = p & 0xff;
        out[i * 4 + 3] = 0xff;
    }
    return out;
}

// ---------------------------------------------------------------------------
// Minimal PNG (RGBA8888) encoder — for human eyeballing only; .rgba is golden.
// ---------------------------------------------------------------------------
const CRC_TABLE: number[] = (() => {
    const t: number[] = [];
    for (let n = 0; n < 256; n++) {
        let c = n;
        for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
        t[n] = c >>> 0;
    }
    return t;
})();
function crc32(buf: Buffer): number {
    let c = 0xffffffff;
    for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
}
function pngChunk(type: string, data: Buffer): Buffer {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length, 0);
    const typeBuf = Buffer.from(type, 'ascii');
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])), 0);
    return Buffer.concat([len, typeBuf, data, crc]);
}
function encodePNG(rgba: Buffer, w: number, h: number): Buffer {
    const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
    const ihdr = Buffer.alloc(13);
    ihdr.writeUInt32BE(w, 0);
    ihdr.writeUInt32BE(h, 4);
    ihdr[8] = 8; // bit depth
    ihdr[9] = 6; // colour type RGBA
    ihdr[10] = 0; ihdr[11] = 0; ihdr[12] = 0; // deflate / adaptive filter / no interlace
    // raw scanlines, each prefixed with filter byte 0
    const raw = Buffer.alloc(h * (w * 4 + 1));
    for (let y = 0; y < h; y++) {
        raw[y * (w * 4 + 1)] = 0;
        rgba.copy(raw, y * (w * 4 + 1) + 1, y * w * 4, (y + 1) * w * 4);
    }
    const idat = zlib.deflateSync(raw);
    return Buffer.concat([sig, pngChunk('IHDR', ihdr), pngChunk('IDAT', idat), pngChunk('IEND', Buffer.alloc(0))]);
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------
function main(): void {
    ensureDir(OUT_DIR);
    const cache = new Cache(CACHE_DIR);
    console.log(`cache: ${CACHE_DIR}`);
    console.log(`idx file counts: idx0=${cache.count(0)} idx1=${cache.count(1)} idx2=${cache.count(2)} idx3=${cache.count(3)} idx4=${cache.count(4)}`);

    // --- load config jag (archive 0, file 2) + validate via ObjType.init ---
    const configBytes = cache.read(0, 2);
    if (!configBytes) throw new Error('cannot read config jag (0,2)');
    const configJag = new JagFile(configBytes);
    ObjType.init(configJag, true);
    console.log(`ObjType.init OK — numDefinitions=${ObjType.numDefinitions} (expect ~3894)`);
    if (ObjType.numDefinitions < 3800 || ObjType.numDefinitions > 4000) {
        throw new Error(`unexpected obj count ${ObjType.numDefinitions}; cache reader likely wrong`);
    }

    // --- textures (archive 0, file 6) then colour table (Client.ts:1152-1154 order) ---
    const texBytes = cache.read(0, 6);
    if (!texBytes) throw new Error('cannot read textures jag (0,6)');
    const texJag = new JagFile(texBytes);
    Pix3D.unpackTextures(texJag);
    console.log(`Pix3D.unpackTextures OK — numTextures=${Pix3D.numTextures}`);
    // Pin randomness so the palette golden is reproducible (see header note).
    const realRandom = Math.random;
    Math.random = () => 0.5;
    Pix3D.initColourTable(0.8);
    Math.random = realRandom;
    Pix3D.initPool(20);
    console.log('Pix3D.initColourTable(0.8) [Math.random pinned to 0.5 → zero jitter] + initPool(20) OK');

    // --- pre-unpack every model in archive 1 so Model.load never hits the provider ---
    const numModels = cache.count(1);
    Model.init(numModels, { requestModel(_id: number): void {} } as never);
    let unpacked = 0;
    for (let id = 0; id < numModels; id++) {
        let raw: Uint8Array | null;
        try {
            raw = cache.read(1, id);
        } catch {
            raw = null;
        }
        if (!raw) continue;
        try {
            Model.unpack(id, raw);
            unpacked++;
        } catch {
            // leave meta unset; Model.load(id) will return null for this id
        }
    }
    console.log(`models: unpacked ${unpacked}/${numModels} archive-1 files`);

    // debugname <-> id map from the RuneScript obj.pack symbol table.
    const nameById = new Map<number, string>();
    for (const line of fs.readFileSync(OBJ_PACK, 'utf8').split('\n')) {
        const t = line.trim();
        if (!t) continue;
        const eq = t.indexOf('=');
        if (eq < 0) continue;
        nameById.set(parseInt(t.slice(0, eq), 10), t.slice(eq + 1));
    }

    dumpPalette();
    dumpTriangles();
    dumpLitDagger();
    dumpIcons(nameById);
    console.log('DONE');
}

// (a) palette.bin — 65536 int32 little-endian
function dumpPalette(): void {
    const buf = Buffer.alloc(65536 * 4);
    for (let i = 0; i < 65536; i++) buf.writeInt32LE(Pix3D.colourTable[i] | 0, i * 4);
    fs.writeFileSync(path.join(OUT_DIR, 'palette.bin'), buf);
    console.log(`palette.bin: ${buf.length} bytes (expect 262144)`);
}

// (b) synthetic single-triangle goldens.
//
// Each case sets up a fresh 32×32 Pix2D buffer, sets the Pix3D render state the
// icon path uses (lowDetail=false, hclip=false), sets Pix3D.trans, then calls the
// rasterizer with the SAME argument domain Model.render3 uses:
//   gouraud  -> colour args are colourTable indices (0..65535)
//   flat     -> colour arg is a resolved 0xRRGGBB (Model passes colourTable[idx])
//   textured -> shadeA/B/C, view-space A/B/C coords, texture id (as Model does)
// manifest.json captures EVERY input so the Go port can replay each case exactly.
function dumpTriangles(): void {
    const dir = path.join(OUT_DIR, 'tri');
    ensureDir(dir);
    const W = 32, H = 32;
    const manifest: unknown[] = [];

    interface TriCase {
        name: string;
        routine: 'gouraud' | 'flat' | 'texture';
        prefill: number; // background fill value before the draw
        lowDetail: boolean;
        hclip: boolean;
        trans: number;
        run: () => void;
        args: Record<string, unknown>;
    }

    const setup = (c: TriCase): Int32Array => {
        const px = new Int32Array(W * H).fill(c.prefill);
        Pix2D.setPixels(px, W, H);
        Pix3D.setRenderClipping();
        Pix3D.lowDetail = c.lowDetail;
        Pix3D.hclip = c.hclip;
        Pix3D.trans = c.trans;
        c.run();
        return px;
    };

    const cases: TriCase[] = [
        {
            name: 'gouraud_small', routine: 'gouraud', prefill: 0, lowDetail: false, hclip: false, trans: 0,
            args: { xA: 12, xB: 22, xC: 15, yA: 10, yB: 12, yC: 24, colourA: 0x107f, colourB: 0x287f, colourC: 0x407f },
            run: () => Pix3D.gouraudTriangle(12, 22, 15, 10, 12, 24, 0x107f, 0x287f, 0x407f),
        },
        {
            name: 'gouraud_large', routine: 'gouraud', prefill: 0, lowDetail: false, hclip: false, trans: 0,
            args: { xA: 1, xB: 30, xC: 4, yA: 1, yB: 6, yC: 30, colourA: 0x087f, colourB: 0x307f, colourC: 0x607f },
            run: () => Pix3D.gouraudTriangle(1, 30, 4, 1, 6, 30, 0x087f, 0x307f, 0x607f),
        },
        {
            // Collinear vertices → zero-area triangle; exercises the degenerate guard.
            name: 'gouraud_degenerate', routine: 'gouraud', prefill: 0, lowDetail: false, hclip: false, trans: 0,
            args: { xA: 4, xB: 16, xC: 28, yA: 4, yB: 16, yC: 28, colourA: 0x107f, colourB: 0x307f, colourC: 0x507f },
            run: () => Pix3D.gouraudTriangle(4, 16, 28, 4, 16, 28, 0x107f, 0x307f, 0x507f),
        },
        {
            name: 'flat', routine: 'flat', prefill: 0, lowDetail: false, hclip: false, trans: 0,
            args: { xA: 4, xB: 28, xC: 12, yA: 6, yB: 4, yC: 30, colour: 0xff8040 },
            run: () => Pix3D.flatTriangle(4, 28, 12, 6, 4, 30, 0xff8040),
        },
        {
            // Gouraud with alpha blend over a non-zero grey background.
            name: 'gouraud_alpha128', routine: 'gouraud', prefill: 0x303030, lowDetail: false, hclip: false, trans: 128,
            args: { xA: 12, xB: 22, xC: 15, yA: 10, yB: 12, yC: 24, colourA: 0x107f, colourB: 0x287f, colourC: 0x407f },
            run: () => Pix3D.gouraudTriangle(12, 22, 15, 10, 12, 24, 0x107f, 0x287f, 0x407f),
        },
        {
            // Textured, texture id 1. Screen triangle + a front-facing view-space
            // triangle (A/B/C view coords) so the texture actually samples.
            name: 'textured_tex1', routine: 'texture', prefill: 0, lowDetail: false, hclip: false, trans: 0,
            args: {
                xA: 6, xB: 26, xC: 10, yA: 6, yB: 8, yC: 28,
                shadeA: 128, shadeB: 128, shadeC: 128,
                originX: -50, originY: -50, originZ: 240,
                txB: 50, tyB: -50, tzB: 240,
                txC: -50, tyC: 50, tzC: 240,
                texture: 1,
                // screen origin used by textureRaster = Pix3D.originX/Y after
                // setRenderClipping() on a 32×32 buffer:
                screenOriginX: 16, screenOriginY: 16,
            },
            run: () => Pix3D.textureTriangle(
                6, 26, 10, 6, 8, 28,
                128, 128, 128,
                -50, -50, 240,
                50, -50, 240,
                -50, 50, 240,
                1,
            ),
        },
    ];

    for (const c of cases) {
        // Textured raster reads Pix3D.originX/originY; setRenderClipping sets them to 16.
        const px = setup(c);
        const rgba = toRGBA(px, W, H);
        fs.writeFileSync(path.join(dir, `${c.name}.rgba`), rgba);
        let nonZero = 0;
        for (let i = 0; i < px.length; i++) if (px[i] !== 0) nonZero++;
        manifest.push({
            name: c.name, routine: c.routine, width: W, height: H,
            prefill: c.prefill, lowDetail: c.lowDetail, hclip: c.hclip, trans: c.trans,
            args: c.args,
            palette_dependent: c.routine !== 'texture',
            texture_dependent: c.routine === 'texture',
            nonZeroPixels: nonZero,
        });
        console.log(`tri/${c.name}.rgba: ${rgba.length} bytes, ${nonZero} non-transparent px`);
        if (c.name !== 'gouraud_degenerate' && nonZero === 0) {
            throw new Error(`triangle case ${c.name} produced an empty image`);
        }
    }
    // Restore lowDetail default the icon path expects to toggle itself.
    Pix3D.lowDetail = true;
    fs.writeFileSync(path.join(dir, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
    console.log(`tri/manifest.json: ${manifest.length} cases`);
}

// (c) lit/dagger.json — bronze_dagger post-calculateNormals face colours.
const BRONZE_DAGGER = 1205;
const KNIFE = 946;
function dumpLitDagger(): void {
    const dir = path.join(OUT_DIR, 'lit');
    ensureDir(dir);
    const obj = ObjType.list(BRONZE_DAGGER);
    const model = obj.getModelLit(1); // recolour + resize + calculateNormals(ambient+64, contrast+768, -50,-10,-50, true)
    if (!model) throw new Error('bronze_dagger getModelLit returned null');
    const out = {
        id: BRONZE_DAGGER,
        debugname: 'bronze_dagger',
        modelId: obj.model,
        numFaces: model.numFaces,
        numPoints: model.numPoints,
        ambient: obj.ambient,
        contrast: obj.contrast,
        faceColourA: Array.from(model.faceColourA ?? []),
        faceColourB: Array.from(model.faceColourB ?? []),
        faceColourC: Array.from(model.faceColourC ?? []),
    };
    if (out.faceColourA.length !== model.numFaces) throw new Error('faceColourA length != numFaces');
    fs.writeFileSync(path.join(dir, 'dagger.json'), JSON.stringify(out, null, 2) + '\n');
    console.log(`lit/dagger.json: numFaces=${model.numFaces}, faceColourA/B/C lengths=${out.faceColourA.length}/${out.faceColourB.length}/${out.faceColourC.length}`);
}

// (d) icons + sample selection.
interface ObjRec {
    id: number;
    model: number;
    recolCount: number;
    resized: boolean;
    ambient: number;
    contrast: number;
    certtemplate: number;
    hasCountobj: boolean;
    numT: number;      // model textured-face count (from meta)
    hasAlpha: boolean; // model has per-face alpha (from meta)
    numFaces: number;
    hasFlat: boolean;  // model has ≥1 flat face (renderType & 3 === 1)
    allGouraud: boolean;
}

function readObj(id: number): ObjRec | null {
    const o = ObjType.list(id);
    const meta = Model.meta[o.model];
    if (!meta || meta.numFaces === 0) return null;
    // Load once to classify gouraud vs flat.
    let hasFlat = false, allGouraud = true;
    const m = Model.load(o.model);
    if (m) {
        const rt = m.faceRenderType;
        if (rt) {
            for (let f = 0; f < m.numFaces; f++) {
                const type = rt[f] & 0x3;
                if (type === 1) hasFlat = true;
                if (type !== 0) allGouraud = false;
            }
        }
    }
    return {
        id,
        model: o.model,
        recolCount: o.recol_s ? o.recol_s.length : 0,
        resized: o.resizex !== 128 || o.resizey !== 128 || o.resizez !== 128,
        ambient: o.ambient,
        contrast: o.contrast,
        certtemplate: o.certtemplate,
        hasCountobj: !!o.countobj,
        numT: meta.numT,
        hasAlpha: meta.faceAlphaOffset >= 0,
        numFaces: meta.numFaces,
        hasFlat,
        allGouraud,
    };
}

function dumpIcons(nameById: Map<number, string>): void {
    const dir = path.join(OUT_DIR, 'icons274');
    ensureDir(dir);

    // Build category buckets. Each obj may satisfy several; we keep a per-id set.
    const cats = new Map<number, Set<string>>();
    const addCat = (id: number, c: string) => {
        if (!cats.has(id)) cats.set(id, new Set());
        cats.get(id)!.add(c);
    };
    const PER_CAT = 5;
    const bucketCount = new Map<string, number>();
    const want = (c: string) => (bucketCount.get(c) ?? 0) < PER_CAT;
    const bump = (c: string) => bucketCount.set(c, (bucketCount.get(c) ?? 0) + 1);

    const CATS = ['untextured_gouraud', 'flat_faces', 'textured_faces', 'recoloured', 'resized', 'ambient_contrast', 'certtemplate', 'countobj_stack', 'alpha_faces'];

    // Seed the two mandatory items first.
    const mandatory = [BRONZE_DAGGER, KNIFE];

    const chosen = new Map<number, ObjRec>();
    const consider = (rec: ObjRec, tagsForced?: string[]): boolean => {
        // Icon must render and be non-empty.
        const spr = ObjType.getSprite(rec.id, 1, 0);
        if (!spr) return false;
        let nonZero = 0;
        for (let i = 0; i < spr.data.length; i++) if (spr.data[i] !== 0) nonZero++;
        if (nonZero === 0) return false;
        chosen.set(rec.id, rec);
        const tags = tagsForced ?? categoriesOf(rec);
        for (const t of tags) { addCat(rec.id, t); }
        return true;
    };

    const categoriesOf = (rec: ObjRec): string[] => {
        const t: string[] = [];
        if (rec.numT > 0) t.push('textured_faces');
        if (rec.hasAlpha) t.push('alpha_faces');
        if (rec.recolCount > 0) t.push('recoloured');
        if (rec.resized) t.push('resized');
        if (rec.ambient !== 0 || rec.contrast !== 0) t.push('ambient_contrast');
        if (rec.certtemplate !== -1) t.push('certtemplate');
        if (rec.hasCountobj) t.push('countobj_stack');
        if (rec.hasFlat) t.push('flat_faces');
        if (rec.numT === 0 && rec.allGouraud) t.push('untextured_gouraud');
        return t;
    };

    for (const id of mandatory) {
        const rec = readObj(id);
        if (!rec) throw new Error(`mandatory obj ${id} has no renderable model`);
        if (!consider(rec)) throw new Error(`mandatory obj ${id} icon failed to render`);
        for (const c of categoriesOf(rec)) bump(c);
    }

    // Sweep all objs, filling under-full category buckets. Stop once every
    // category has PER_CAT and we have a healthy total (~40).
    const allFull = () => CATS.every((c) => (bucketCount.get(c) ?? 0) >= PER_CAT);
    const N = ObjType.numDefinitions;
    for (let id = 0; id < N; id++) {
        if (chosen.has(id)) continue;
        if (chosen.size >= 44 || allFull()) break;
        const rec = readObj(id);
        if (!rec) continue;
        const tags = categoriesOf(rec);
        // Only take this obj if it advances at least one still-wanted bucket.
        const advancing = tags.filter(want);
        if (advancing.length === 0) continue;
        if (consider(rec, tags)) {
            for (const c of advancing) bump(c);
        }
    }

    // Emit the sample list, sorted by id.
    const sample = [...chosen.keys()].sort((a, b) => a - b).map((id) => ({
        id,
        debugname: nameById.get(id) ?? `obj_${id}`,
        categories: [...(cats.get(id) ?? new Set())].sort(),
    }));

    // Render + write each icon (.rgba golden + .png for eyeballing).
    for (const s of sample) {
        const spr = ObjType.getSprite(s.id, 1, 0) as Pix32;
        const rgba = toRGBA(spr.data, 32, 32);
        if (rgba.length !== 32 * 32 * 4) throw new Error(`icon ${s.id} wrong rgba size ${rgba.length}`);
        fs.writeFileSync(path.join(dir, `${s.debugname}.rgba`), rgba);
        fs.writeFileSync(path.join(dir, `${s.debugname}.png`), encodePNG(rgba, 32, 32));
    }

    const sampleJson = JSON.stringify(sample, null, 2) + '\n';
    fs.writeFileSync(path.join(OUT_DIR, 'icons274', 'sample.json'), sampleJson);
    fs.writeFileSync(path.join(HERE, 'sample.json'), sampleJson);

    // Coverage report.
    const coverage: Record<string, number> = {};
    for (const c of CATS) coverage[c] = 0;
    for (const s of sample) for (const c of s.categories) coverage[c] = (coverage[c] ?? 0) + 1;
    console.log(`icons274: ${sample.length} icons written (.rgba + .png)`);
    console.log(`category coverage: ${JSON.stringify(coverage)}`);
    const missing = CATS.filter((c) => (coverage[c] ?? 0) === 0);
    if (missing.length) console.log(`WARNING: categories with zero coverage: ${missing.join(', ')}`);
}

main();
