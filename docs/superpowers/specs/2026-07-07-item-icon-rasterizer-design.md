# Item-icon rasterizer — design

**Date:** 2026-07-07
**Status:** approved by user (brainstorming session); **AMENDED same day — see
"Amendment 1" at the end, which supersedes the Architecture section's renderer
location, inputs, and phasing.** The fidelity tier, per-branch fidelity intent,
and harness+golden verification decisions stand.
**Prereq:** versioned docs site (merged to `main`, spec 2026-07-06); item tables reserve an empty Icon column.

## Problem

The docs site's item reference tables have an empty Icon column: goscape has no
model→2D renderer. RS inventory icons are software-rasterized 32×32 sprites
produced by the client from the item's 3D model plus obj-config parameters
(`model`, `2dzoom/2dxan/2dyan/2dzan/2dxof/2dyof`, recolours, `resize*`,
`ambient`/`contrast`, cert links). Port that renderer so docsgen can emit one
PNG per item per revision.

## Decisions made (user-confirmed)

1. **Fidelity tier: FULL** — gouraud + flat + textured triangle paths
   (~4,450 Java-deob lines transitive closure). Textured faces (render type
   2/3) are reachable for real item models; no per-cache gamble.
2. **Architecture: port to ALL FIVE rev branches** — each branch gets a
   revision-appropriate renderer matching *its* pinned reference client
   (user chose per-branch fidelity over a single rev-274-semantics tool).
3. **Verification: reference harness + golden pixels** — per-pin headless
   harness dumps reference icons; byte-exact comparisons become committed
   golden tests per branch. Fallback for an impractical pin: hand-verified
   goldens, only with explicit user sign-off.

## Key facts (from exploration; verified against local repos)

- **The routine:** rev-274 `ObjType.getSprite(id, outlineRgb, count)`
  (Client-Java `jagex2/config/ObjType.java:208-334`) → 32×32 `Pix32`.
  Pipeline: count-variant substitution → `getModelLit` (model load +
  `recolour` + `resize` + `calculateNormals(ambient+64, contrast+768,
  -50,-10,-50, true)`) → save/rebind global `Pix2D`/`Pix3D` state,
  `lowDetail=false` → camera from obj config (`zoom2d`; ×1.5 when
  outline==-1, ×1.04 when outline>0; pitch via sin/cos tables) →
  `objRender(0, yan2d, zan2d, xan2d, xof2d, sinPitch·zoom>>16 + minY/2 +
  yof2d, cosPitch·zoom>>16 + yof2d)` → silhouette pass (transparent pixel
  adjacent to rendered → 1) → outline==0: diagonal drop-shadow `0x301820`;
  outline>0: colored outline → cert overlay composition when
  `certtemplate != -1` (recursive `getSprite(certlink, -1, 10)`) → cache,
  restore globals. No fonts, no text, no near-clip: `objRender` calls
  `render2(false, …)`, so **`render3ZClip` (~154 lines) is provably
  unreachable — out of scope, with a comment saying why.**
- **Transitive closure (rev-274):** Model (unpack/decode ctor/getColour/
  calculateNormals/light/objRender/render2/render3/recolour/resize/bounding
  ≈ 915 lines) · Pix3D (initColourTable 65536-entry palette + gammaCorrect +
  gouraudTriangle/Raster + flatTriangle/Raster + textureTriangle/Raster +
  texture mgmt + clipping + trig ≈ 2,215) · Pix2D subset ≈ 90 · Pix32 subset
  ≈ 160 · LruCache ≈ 77 (optional in an offline tool — render once per obj,
  cache only needed for cert recursion reuse) · buffer-reader subset ≈ 60.
- **Porting references (pins in `main:REFERENCES.md`, all present locally):**
  - rev-274: **Client-TS branch `274`** (`src/config/ObjType.ts:365`
    `getSprite(id, count, outlineRgb)`, `src/dash3d/{Model,Pix3D}.ts`,
    `src/graphics/{Pix2D,Pix32,Pix8}.ts`) as the line-by-line source —
    named params, `Int32Array`→`[]int32`; **Client-Java pin 32f30626** as
    fidelity cross-check for the unrolled raster inner loops.
    ⚠ Argument-order trap: TS `(id, count, outlineRgb)` vs Java-274
    `(id, outline, count)` — match by call site, never positionally.
  - rev-254 pin `2e629784`, rev-245.2 `176a85f7`, rev-244 `01f16088`
    (Client-Java): same 3-arg pipeline as 274 (254 renamed
    getIcon→getSprite, iconCache→spriteCache — cosmetic).
  - rev-225 pin `cc3781de` (`225-clean`): **2-arg `getIcon(id, count)`** —
    no colored-outline param, no resize, lighting hardcoded
    `calculateNormals(64, 768, -50,-10,-50, true)` (obj opcodes 110-114
    don't exist at 225); silhouette + shadow + cert crop(22,5,22,5) only.
- **Format stability:** the per-model ob2 triangle encoding is identical
  225↔274 (no version byte). What differs is the container: 225's client
  loads a `models` jag with parallel streams; 244+ load self-contained
  per-file models (18-byte trailer). The content trees hold the
  self-contained per-file form for every revision, which is what the
  renderer consumes.
- **Existing goscape assets to reuse (per branch):**
  `pkg/unpack/internal/model/model.go` — full ob2 geometry decoder (1:1
  port of engine Model.ts; currently only used for texture detection;
  it is `internal`, so promote it or place the renderer under
  `pkg/unpack/`) — note rev-225 has no pkg/unpack: the decoder must be
  introduced there (copy of the same port). `pkg/colorconv` — HSL16↔RGB.
  `pkg/unpack/internal/pix` — `png.Encode` pattern (`writePNG`).
  `pkg/objtype` — typed obj fields incl. cert auto-resolution (reference
  for semantics; the renderer parses content text configs directly).
- **Inputs per revision (content tree; exists for all five incl. cache-less
  225):** `content/models/obj/*.ob2` (obj `model=` names map 1:1 to
  filenames — verified: `obj_3142`, `model_2837_obj`, `model_2559_obj`),
  `content/textures/*.png` (all 50), obj text configs.
- **Docs integration points (main):** `tools/docsgen/families.py:_obj_row`
  first cell is the reserved icon column; `tools/build.py assemble()` copies
  overlay files verbatim (skips `_`-prefixed names) — PNGs under
  `overlays/rev-N/player/items/icons/` ride the existing pipeline; table
  cell becomes `![<name>](icons/<debugname>.png)` (passes `md_escape`
  untouched — no `|`/newline).
- **Volume:** ≈16,500 icons across five revisions at 32×32 ≈ 8–33 MiB —
  roughly doubles the ~22 MiB gh-pages pack. Accepted; **no dedup (YAGNI)**.

## Architecture

### Per-branch renderer: new goscape-cli verb `icons`

`goscape-cli icons -src-dir <content> -out-dir <dir> [-obj <debugname>]
[-log.level …]` — top-level verb (the `rsa` precedent; `unpack` doesn't
exist on 225), one entry in `main.go`'s `verbs` slice, `cmd_icons.go`.

New package `pkg/render/` per branch, mirroring the client:
- `pix3d/` — palette (`initColourTable(0.8)` + gamma), triangle rasterizers
  (gouraud, flat, textured + their raster inner loops), texture load from
  the content-tree PNGs (replacing the client's jag/`Pix8` load with a
  PNG→texel-palette adapter that produces identical texel data — the
  adapter must be validated against the harness like everything else),
  clipping, sin/cos/div tables.
- `pix2d.go` / `pix32.go` — framebuffer + sprite subsets (explicit struct
  state instead of the client's globals; the save/rebind dance disappears).
- `icon.go` — `getSprite`/`getIcon` composition per that branch's client
  semantics, silhouette/shadow/cert passes, 32×32 `image.NRGBA` output with
  true alpha (palette 0 = transparent), PNG write.
- `config.go` — content obj-text parser for the icon-relevant fields
  (model, 2d transforms, raw `recol1s/d…`, resize/ambient/contrast where
  the revision has them, certlink/certtemplate resolved by debugname,
  countobj/countco).
- Model decode: reuse/promote the existing ob2 decoder (274-lineage
  branches); introduce the same port on rev-225.

Comment/reference conventions follow PORTING-LESSONS §4 (cite the pinned
client file:line at every ported routine).

**Implementation-start verifications (before any porting):**
(a) content `.obj` files carry raw `recol1s=`/`recol1d=` lines (the unpack
text emitter resolves them, but the renderer parses *content* sources);
(b) 225-era content `.ob2` files decode with the self-contained-trailer
decoder; (c) sample a few models per revision for render types present.

### Icon variant policy

Docs icons are the plain inventory rendering: outline=0 (silhouette +
drop-shadow), count=1 per obj record (stack variants are separate records),
cert records composed per client logic. Missing model file / unparseable
record → warn, skip, empty cell; the count feeds the docsgen floor.

### Verification: per-pin reference harness + golden tests

For each branch pin, a minimal headless harness dumps reference icons for a
curated sample (~40 items: untextured gouraud, flat, textured, recoloured,
resized, ambient/contrast extremes, noted, stack-variant, zero-model edge):
- rev-274: node script in a scratch dir driving Client-TS (`branch 274`)
  `ObjType.getSprite` with data fed from the reference cache
  (`Server274-ref/unpack-ref/cache`).
- rev-254/245.2/244: small `main()` compiled against the pinned Client-Java
  checkout (worktree at the pin), fed from that revision's reference cache.
- rev-225: Client-Java `225-clean` pin fed from `Server225_2`'s jag
  archives.
Harness outputs (PNG + raw RGBA) are committed as goldens; per-branch Go
tests render the same sample from the same inputs and byte-compare.
Harness scripts live under `tools/iconref/` on each branch — committed,
rerunnable. **Fallback** (only with user sign-off, per pin):
hand-verified goldens.

### docsgen integration (main)

New `icons` step in `tools/docsgen`:
- per revision: build that branch's `goscape-cli` (all five — 225 included
  now that the verb exists there; the existing worktree lifecycle is
  reused), run `icons -src-dir <content_dir> -out-dir
  overlays/rev-N/player/items/icons`, deterministic output (stable file
  set, no timestamps — PNG encode of identical pixels is byte-stable).
- `_obj_row` first cell: `![<display name>](icons/<debugname>.png)` when
  the PNG exists, else empty string.
- Sanity floor: ≥95% of obj records rendered per revision, else abort.
- Determinism gate, strict builds ×5, comparison page gains an Icons count
  column, redeploy gh-pages.

## Phasing (implementation plan structure)

1. rev-274 renderer end-to-end (decoder promotion, pix3d, icon composition,
   CLI verb) + TS harness + goldens.
2. docsgen `icons` step proven on rev-274 alone (site shows icons for one
   revision).
3. Ports to rev-254, rev-245.2, rev-244 (+ per-pin Java harnesses +
   goldens). PORTING.md EXTENSION rows per branch.
4. rev-225 variant port (2-arg getIcon semantics, decoder introduction,
   jag-fed harness).
5. Full five-revision regen, determinism, strict ×5, redeploy; docs-site
   pages that mention the reserved column / follow-up project updated.

## Out of scope

- Colored-outline / enlarged (×1.5) icon variants (client supports; docs
  don't need them — the code paths exist for fidelity but aren't exposed
  as CLI options).
- NPC/loc/spotanim rendering, animation frames, player kit.
- Icon dedup/content-hashing; remote hosting.
- `render3ZClip` (unreachable from the icon path — documented).

## Testing summary

Golden-pixel tests per branch (the fidelity gate) + unit tests for parser
and palette math + docsgen determinism/floor gates + strict site builds.

## Amendment 1 (2026-07-07): reuse goscape-client — supersedes Architecture/inputs/phasing

**Trigger:** the user surfaced `~/Code/github.com/zsrv/goscape-client`
— an existing MIT-licensed Go port of the RS2 client with the SAME five-branch
model (rev-225 … rev-274), per-revision faithful ports with inline Java `@pin`
provenance, and a completed `audit-274` correctness audit. Exploration verified
it contains the complete icon closure on every branch (`objtype.GetSprite` /
2-arg `GetIcon` on 225, full `Model`, full `Pix3D` incl. textured paths,
`Pix2D`/`Pix32`), that triangle rasterization is pure CPU integer math (GL only
uploads the final framebuffer — byte-exact reuse holds), that `pkg/` has no
`internal/` walls, and that the sole nondeterminism is the faithful
`math/rand` brightness jitter in `InitColourTable` (same jitter as the
reference clients). User approved modifying goscape-client.

**Superseding decisions:**
1. **No fresh rasterizer port.** The ~4,450-line port is replaced by reuse of
   goscape-client's render packages per branch.
2. **The icon tool lives in goscape-client** (client functionality belongs in
   the client): new `cmd/icondump` per branch — headless driver that loads the
   revision's native data (config jag + textures jag + models from
   `main_file_cache` on 244+; **jag archives on 225**, which `Server225_2`
   provides — the "no 225 cache" problem does not exist on the client data
   path), calls `GetSprite(id, 0, 1)` / era-appropriate `GetIcon`, converts
   `Pix32.Pixels` (0 → transparent) to NRGBA, writes `<id>.png` + an
   `index.tsv` of `id<TAB>name`. The goscape SERVER repo gains nothing; the
   Task-2 decoder promotion and Task-3 harness commits on goscape rev-274 are
   reverted/migrated (harness + goldens move to goscape-client rev-274).
3. **Deterministic palette hook:** goscape-client gains
   `pix3d.InitColourTableDeterministic(brightness)` (no jitter) used only by
   icondump; the faithful jittered function is untouched. Documented as an
   extension: the reference's jitter has expectation exactly `brightness`, and
   the R3 harness pinned `Math.random = 0.5` (zero jitter) so goldens match.
4. **Goldens gate the reused code:** the R3 goldens (palette.bin, 8 synthetic
   triangles, lit/dagger.json, 39 icons) become goscape-client tests —
   synthetic tris against `Pix3D` directly, palette against the deterministic
   hook, icons end-to-end through icondump. Any mismatch = a REAL
   goscape-client fidelity bug → fix there under its own conventions
   (escalate if a fix would be non-trivial). Per-branch Java harnesses for
   254/245.2/244/225 goldens remain as originally decided.
5. **docsgen mapping:** icondump emits ids; docsgen maps id→debugname. For
   244+ the unpack-derived records are id-ordered and dense (verify at
   implementation: record `_index` == obj id — client numDefinitions 3894 ==
   record count on 274). For rev-225 (content-tree records, no ids) mapping is
   by exact NAME in record order via icondump's index.tsv, unmatched → no
   icon; the 225 icon floor is 90% (others stay 95%).
6. **goscape-client work protocol:** worktrees per branch (like the server
   repo), same commit conventions; goldens/harness live on its branches.

**What survives unchanged from the original spec:** full fidelity tier
(already implemented in goscape-client, gated by our goldens), per-branch
fidelity (goscape-client's branches ARE the per-revision ports), harness +
golden-pixel verification, docs integration shape (overlays icons dir, Icon
cell, floors, determinism), out-of-scope list, icon variant policy.
