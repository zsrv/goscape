# Item-Icon Rasterizer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `goscape-cli icons` verb on all five rev branches that renders every item's 32×32 inventory icon from content-tree models/configs/textures — golden-pixel-verified against each branch's pinned reference client — plus a docsgen step that fills the docs site's reserved Icon column.

**Architecture:** Per branch, a new `pkg/render/` package ports the pinned client's icon pipeline (`ObjType.getSprite`/`getIcon` → Model lighting/projection → Pix3D software rasterizer → silhouette/shadow/cert passes). rev-274 is built first against Client-TS (line-by-line source) with Client-Java as cross-check, gated by goldens from a headless reference harness; 254/245.2/244 are copy-adapt ports (cosmetic pin deltas); 225 is a variant port (2-arg `getIcon`, hardcoded lighting, no resize/ambient/contrast). docsgen then renders per-revision icon sets into the overlays.

**Tech Stack:** Go (stdlib `image/png` only — no new deps), Node+tsx for the TS harness, `javac` for the Java harnesses, existing docsgen (Python).

**Spec:** `docs/superpowers/specs/2026-07-07-item-icon-rasterizer-design.md`.

## Global Constraints

- **Porting-task code convention:** for ported routines, the authoritative "code to write" is the pinned reference client source cited in the task (this repo's established port methodology). The plan gives exact reference file:line at the pin, the Go signature, and the gating golden test; the implementer transliterates the reference, never invents. Every ported function carries a provenance comment: `// Ported from <Repo> <path>:<lines> @<pin>` (PORTING-LESSONS §4).
- **Branches & pins (from `main:REFERENCES.md`):** rev-274 ↔ Client-TS branch `274` (primary) + Client-Java `32f30626` (cross-check); rev-254 ↔ Client-Java `2e629784`; rev-245.2 ↔ `176a85f7`; rev-244 ↔ `01f16088`; rev-225 ↔ `cc3781de` (`225-clean`). Reference repos: `/home/owner/Code/github.com/LostCityRS/Client-TS`, `/home/owner/Code/github.com/LostCityRS/Client-Java` — READ-ONLY; use `git -C <repo> show <pin>:<path>` or add temp worktrees at pins under `$TMPDIR`.
- **Worktrees:** rev-274 work in `/home/owner/Code/github.com/zsrv/goscape` (primary checkout; never stage its unrelated untracked files). Other branches in their existing worktrees `/home/owner/Code/github.com/zsrv/goscape-rev{225,244,245.2,254}`. Commits land directly on each rev branch (port-arc convention, no feature branches).
- **Argument-order trap:** TS `getSprite(id, count, outlineRgb)` vs Java-274 `getSprite(id, outline, count)` — match by call site, never positionally.
- **`render3ZClip` is out of scope** (unreachable: `objRender` → `render2(false, …)`); leave a comment at the `render3` port saying exactly this.
- **Icons variant:** outline=0 (silhouette + `0x301820` drop-shadow), count=1 per obj record; cert composition per client; true-alpha NRGBA PNG (palette 0 → transparent). 32×32.
- **Inputs:** content tree only — `<content>/models/obj/*.ob2`, `<content>/textures/*.png` (50), obj text configs. Content dirs per revision are in `main:tools/docsgen/revisions.toml`.
- **Go invocations:** `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Gates per branch when code changes: package tests + `go test -run '^$' ./...` compile-all; `-race` on touched packages (CGO_ENABLED=1) where gcc available.
- **Commits:** `git commit --no-gpg-sign`, trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`, `git status --short` first, stage only intended files.
- **Golden files:** committed under `pkg/render/testdata/` (Go) and produced by `tools/iconref/` harnesses (committed, rerunnable). Goldens are raw RGBA dumps (`.rgba`, 32*32*4 bytes) + PNGs for eyeballing; tests compare RGBA bytes exactly.
- **Harness fallback** (per spec): if a pin's harness proves impractical, STOP and ask the user before downgrading that branch to hand-verified goldens.
- **docsgen floor:** ≥95% of obj records rendered per revision, else abort. Determinism: rerun ⇒ byte-identical outputs.
- **Sandbox:** sibling-worktree writes and `npm install`/network need the override — expected, retry on denial.

---

## Phase 1 — rev-274 renderer (Tasks 1–10, in /home/owner/Code/github.com/zsrv/goscape)

### Task 1: Implementation-start verifications (spec gate)

**Files:** none committed (findings go in the task report; a throwaway probe under `$TMPDIR`).

**Interfaces:** Produces GO/NO-GO facts every later task relies on.

- [ ] **Step 1:** Verify content obj configs carry raw recolour lines: `grep -rn "recol1s=" /home/owner/Code/github.com/LostCityRS/Server274-ref/content/scripts --include='*.obj' | head -5` and same for `Server225_2`. Expected: hits with paired `recol1d=`. Record 3 sample stanzas.
- [ ] **Step 2:** Verify 225-era content models decode with the existing self-contained decoder: write `$TMPDIR/probe/main.go` importing `github.com/zsrv/goscape/pkg/unpack/internal/model` — cannot import `internal` from outside; instead add a temporary Go test in-tree (do NOT commit): `pkg/unpack/internal/model/probe_test.go` that reads `/home/owner/Code/github.com/LostCityRS/Server225_2/content/models/obj/<first .ob2>` and a rev-274 one, decodes both, asserts `VertexCount > 0 && FaceCount > 0` and prints counts. Run it; then `git checkout -- .`/delete the probe file. Expected: both decode.
- [ ] **Step 3:** Render-type census: extend the same throwaway probe to walk BOTH content trees' `models/obj/*.ob2`, decode, and tally `FaceInfo[i]&3` values (0/1/2/3) + models with `TexturedFaceCount>0`. Record the counts per revision (this tells us how much the flat/texture paths matter in practice; full tier is ported regardless).
- [ ] **Step 4:** Verify texture PNG shape: `file /home/owner/Code/github.com/LostCityRS/Server274-ref/content/textures/*.png | head -3` — record dimensions (expect 64×64 or 128×128) and confirm 50 files in both 274 and 225 trees.
- [ ] **Step 5:** Report findings (no commit). If Step 2 fails for 225, flag it loudly — Task 16's approach depends on it.

### Task 2: Promote the ob2 decoder to `pkg/render/model`

**Files:**
- Create: `pkg/render/model/model.go` (moved), `pkg/render/model/model_test.go` (moved)
- Modify: every importer of `pkg/unpack/internal/model` (find with `git grep -l 'unpack/internal/model'` — expect `pkg/unpack/config/*.go`), plus delete the old package dir.

**Interfaces:**
- Produces: `render/model.Model` struct exactly as today (`VertexCount, FaceCount, TexturedFaceCount int; VertexX/Y/Z, FaceVertexA/B/C, TexturedVertexA/B/C, FaceInfo, FacePriority, FaceAlpha, FaceColour, FaceColourA/B/C, VertexLabel, FaceLabel []int32; Priority int`) and its `Store`/decode API unchanged. Later tasks extend this package.

- [ ] **Step 1:** `git mv pkg/unpack/internal/model pkg/render/model` (create `pkg/render/` first). Update the package's own import path references if any.
- [ ] **Step 2:** Update importers: `grep -rl 'zsrv/goscape/pkg/unpack/internal/model' --include='*.go' .` → rewrite to `zsrv/goscape/pkg/render/model`. Keep alias `model` where used.
- [ ] **Step 3:** Gates: `go test ./pkg/render/... ./pkg/unpack/... -count=1` then compile-all `go test -run '^$' ./...`. Expected: green (pure move).
- [ ] **Step 4:** Commit: `refactor(render): promote ob2 decoder out of unpack/internal for the icon renderer`.

### Task 3: TS reference harness + goldens (tools/iconref)

**Files:**
- Create: `tools/iconref/README.md`, `tools/iconref/package.json`, `tools/iconref/dump.ts`, `tools/iconref/vendor/` (pinned Client-TS sources, verbatim copies with provenance headers), `tools/iconref/sample.json`
- Create (outputs, committed): `pkg/render/testdata/palette.bin`, `pkg/render/testdata/tri/*.rgba` (synthetic triangle goldens), `pkg/render/testdata/icons274/*.rgba` + `.png` (~40 icons), `pkg/render/testdata/lit/dagger.json` (lit-model intermediate golden)

**Interfaces:**
- Produces: the goldens every rasterizer task tests against, and `sample.json` (the curated ~40-item list: `[{"debugname":"bronze_dagger","id":1205}, …]` — chosen in Step 3 to cover: untextured gouraud, flat faces, textured faces, recoloured, resized, ambient/contrast≠0, certtemplate (noted), countobj stack-variant, a model with alpha faces).
- Consumes: Client-TS at branch `274` (vendored), reference cache `/home/owner/Code/github.com/LostCityRS/Server274-ref/unpack-ref/cache/`.

- [ ] **Step 1:** Vendor the pinned TS sources: from `/home/owner/Code/github.com/LostCityRS/Client-TS` (branch 274, commit b678942 — record exact SHA) copy verbatim into `tools/iconref/vendor/`: `src/config/ObjType.ts`, `src/dash3d/Model.ts`, `src/dash3d/Pix3D.ts`, `src/dash3d/PointNormal.ts`, `src/graphics/Pix2D.ts`, `src/graphics/Pix32.ts`, `src/graphics/Pix8.ts`, `src/datastruct/LruCache.ts` + its `Linkable`/`DoublyLinkable`/`Hashable` deps, `src/io/Packet.ts`, `src/jagfile/Jagfile.ts` (adjust import paths inside vendor only; add a one-line provenance header to each file; if a file drags in browser-only deps, stub ONLY those imports and document each stub in README.md).
- [ ] **Step 2:** `package.json` with `tsx` + `typescript` devDependencies; `npm install` (network → sandbox override).
- [ ] **Step 3:** Write `dump.ts`:
  - loads the reference cache: minimal `main_file_cache.dat/idx` reader (~60 lines: 520-byte blocks, 6-byte index entries — port from Client-TS's cache/`FileStore` if vendored, else implement per the format and validate by CRC against a known archive);
  - reads archive 0 (config jag → `ObjType.unpack`), archive 1 models on demand (feed `Model` the per-file bytes), textures jag (archive 0 file 6) → `Pix3D.unpackTextures` + `Pix3D.initColourTable(0.8)`;
  - dumps: (a) `palette.bin` — the 65536-entry `colourTable` as little-endian int32; (b) synthetic triangle goldens: for each case in a hardcoded list (gouraud small/large/degenerate, flat, textured with texture 1, alpha 128), set up a 32×32 `Pix2D` buffer, call the `Pix3D` routine directly with fixed coordinates/colors, dump RGBA; (c) `lit/dagger.json` — for `bronze_dagger`: the post-`calculateNormals` `faceColourA/B/C` arrays as JSON; (d) the ~40 sample icons via `ObjType.getSprite(id, 1, 0)` (TS arg order!), converting `Pix32.data` (0 ⇒ transparent) to RGBA.
  - Selection of the 40: query loaded ObjTypes programmatically (has recol / has certtemplate / model has TexturedFaceCount>0 etc.) and print the chosen list to `sample.json`.
- [ ] **Step 4:** Run `npx tsx dump.ts --cache /home/owner/Code/github.com/LostCityRS/Server274-ref/unpack-ref/cache --out ../../pkg/render/testdata`. Eyeball 5 PNGs (open or `feh`; note in report). Sanity: `palette.bin` is 262144 bytes; icons look like the real items.
- [ ] **Step 5:** Commit harness + goldens: `test(render): TS reference harness + rev-274 golden pixels`.
  Troubleshooting: if vendoring drags in half the client, STOP per the fallback rule and report — don't build a browser.

### Task 4: Palette (`initColourTable` + `gammaCorrect`)

**Files:**
- Create: `pkg/render/pix3d/palette.go`, `pkg/render/pix3d/palette_test.go`

**Interfaces:**
- Produces: `pix3d.State` struct (holds all former Pix3D globals: `ColourTable [65536]int32`, texture state added in Task 7, clip/raster fields added in Task 5) with `NewState() *State`; `(*State) InitColourTable(brightness float64, textures []*Texture)` — for this task textures may be nil; `gammaCorrect(rgb int32, gamma float64) int32`.
- Reference: Client-TS `vendor/Pix3D.ts` `initColourTable`/`gammaCorrect` (Java cross-check `Pix3D.java:271-363`).

- [ ] **Step 1 (failing test):**
```go
package pix3d

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestInitColourTableMatchesReference(t *testing.T) {
	raw, err := os.ReadFile("../testdata/palette.bin")
	if err != nil { t.Fatal(err) }
	if len(raw) != 65536*4 { t.Fatalf("palette.bin len = %d", len(raw)) }
	s := NewState()
	s.InitColourTable(0.8, nil)
	for i := 0; i < 65536; i++ {
		want := int32(binary.LittleEndian.Uint32(raw[i*4:]))
		if s.ColourTable[i] != want {
			t.Fatalf("colourTable[%d] = %#x, want %#x", i, s.ColourTable[i], want)
		}
	}
}
```
- [ ] **Step 2:** RED run (`go test ./pkg/render/pix3d/ -run ColourTable`). Expected: compile failure (no package) → after skeleton, value mismatch until ported.
- [ ] **Step 3:** Port `InitColourTable` + `gammaCorrect` from the vendored TS (float math: Go `float64` matches TS `number`; keep the exact loop structure and int truncations).
- [ ] **Step 4:** GREEN. **Step 5:** Commit (`feat(render): HSL colour table (ported Pix3D.initColourTable)`).

### Task 5: Framebuffer + sprite (`pix2d`, `pix32`) and raster state

**Files:**
- Create: `pkg/render/pix2d.go`, `pkg/render/pix32.go`, `pkg/render/pix2d_test.go`
- Modify: `pkg/render/pix3d/palette.go` (extend `State` with raster fields)

**Interfaces:**
- Produces: `render.Frame` (replaces Pix2D globals): `{Pixels []int32; Width, Height, ClipLeft, ClipRight, ClipTop, ClipBottom int}` with `NewFrame(w, h int) *Frame`, `(*Frame) FillRect(x, y, w, h int, colour int32)`, `(*Frame) SetClipping/ResetClipping`. `render.Sprite32` (Pix32 subset): `{Data []int32; Wi, Hi, OWi, OHi, Xof, Yof int}` + `PlotSprite(dst *Frame, x, y int)` (transparent-0 blit, ported from vendored Pix32.ts `plotSprite`/`quickPlotSprite`) + `(s *Sprite32) ToNRGBA() *image.NRGBA` (0 → alpha 0, else opaque RGB) + `WritePNG(path)`.
  `pix3d.State` gains: `Frame *Frame; CenterX, CenterY int; OffsetX []int32 (scanline offsets); LowDetail bool; Alpha int; ClipX bool` etc. per the TS fields, and `(*State) SetRenderClipping(f *Frame)` / `BindFrame`.
- Reference: vendored `Pix2D.ts` (subset), `Pix32.ts` (ctor/plotSprite/crop), `Pix3D.ts` clipping/init fields.

- [ ] **Step 1 (failing tests):** `FillRect` fills exactly [x,x+w)×[y,y+h); `PlotSprite` skips 0-pixels; `ToNRGBA` maps 0→transparent, `0xFF00FF`→opaque magenta. (Write the three table-driven tests concretely; ~40 lines.)
- [ ] **Step 2:** RED. **Step 3:** Port/implement. **Step 4:** GREEN + compile-all. **Step 5:** Commit.

### Task 6: Gouraud triangle path

**Files:**
- Create: `pkg/render/pix3d/gouraud.go`, `pkg/render/pix3d/tri_test.go` (shared by Tasks 6–8)

**Interfaces:**
- Produces: `(*State) GouraudTriangle(xA, xB, xC, yA, yB, yC, colourA, colourB, colourC int)` — writes into the bound `Frame` exactly as the reference.
- Reference: vendored `Pix3D.ts` `gouraudTriangle` + `gouraudRaster` (Java cross-check `Pix3D.java:366-981` for the per-pixel vs 4-batch `lowDetail` structure; icons force `lowDetail=false`, but port BOTH branches — `render2` is shared code and the flag is part of `State`).

- [ ] **Step 1 (failing test):** a table-driven test that, for each `testdata/tri/gouraud_*.rgba` golden, reconstructs the harness's exact setup (same 32×32 frame, same clip, same coordinates/colors — read them from a small `tri/manifest.json` the harness wrote alongside), runs `GouraudTriangle`, converts the frame to RGBA the same way, and byte-compares. Include the degenerate case.
- [ ] **Step 2:** RED. **Step 3:** Port (this is the big one — keep the reference's variable names in comments where the deob's structure is load-bearing; fixed-point `<<16`/`>>16` slope math must be transliterated exactly, incl. int overflow semantics: TS `|0` → Go `int32` conversions where the reference truncates). **Step 4:** GREEN. **Step 5:** Commit.

### Task 7: Flat + textured triangle paths, texture adapter

**Files:**
- Create: `pkg/render/pix3d/flat.go`, `pkg/render/pix3d/texture.go`, `pkg/render/pix3d/texture_test.go`
- Modify: `pkg/render/pix3d/palette.go` (`InitColourTable` texture-palette part), `tri_test.go` (flat/textured cases)

**Interfaces:**
- Produces: `(*State) FlatTriangle(xA…, yA…, colour int)`; `(*State) TextureTriangle(xA…, yA…, shadeA, shadeB, shadeC, originX, originY, originZ, txB, txC, tyB, tyC, texture int)`; `LoadTexturesFromPNGs(dir string) ([]*Texture, error)` — reads `<dir>/<i>.png` for i in 0..49 into the client's texel representation (128×128 palettized; build the per-texture palette exactly as `Pix8`+`initColourTable`'s `texPal` does — the adapter's output must be indistinguishable from the jag path, which the textured goldens prove).
- Reference: vendored `Pix3D.ts` `flatTriangle`/`flatRaster`, `textureTriangle`/`textureRaster`, `unpackTextures`/`getTexels`/`texPal`; `Pix8.ts` for the indexed-image semantics. Java cross-check `Pix3D.java:982-2432`.

- [ ] **Step 1:** Failing tests: flat + textured synthetic goldens from `testdata/tri/` (same manifest pattern as Task 6); plus a texel-level test: `LoadTexturesFromPNGs` on the content textures, then compare `texPal`-derived values for texture 1 against a `testdata/texpal1.bin` dump (add this dump to the harness in this task — one extra harness output line, rerun dump.ts, commit updated goldens).
- [ ] **Step 2:** RED. **Step 3:** Port. **Step 4:** GREEN + full pkg tests. **Step 5:** Commit.

### Task 8: Model lighting + projection + face dispatch

**Files:**
- Create: `pkg/render/model/light.go`, `pkg/render/model/render.go`, `pkg/render/model/light_test.go`
- Modify: `pkg/render/model/model.go` (add scratch/screen arrays + `Recolour`, `Resize` methods)

**Interfaces:**
- Produces on `*model.Model`: `Recolour(src, dst int32)`, `Resize(x, y, z int)`, `CalculateNormals(lightAmbient, lightAttenuation, lightSrcX, lightSrcY, lightSrcZ int, applyLighting bool)` (fills `FaceColourA/B/C` via `getColour`; computes bounds), `ObjRender(s *pix3d.State, pitch, yaw, roll, eyePitch, eyeX, eyeY, eyeZ int)` (rotate/project + `render2(false,…)` + `render3` dispatch to `s.GouraudTriangle`/`FlatTriangle`/`TextureTriangle` by `FaceInfo&3`; near-clip branch replaced by `// render3ZClip intentionally not ported: objRender always calls render2(false, …) so faces are never near-clipped on the icon path`).
- Reference: vendored `Model.ts` `calculateNormals`/`light`/`getColour`/`objRender`/`render2`/`render3`/`recolour`/`resize`/`calcBoundingCylinder` (Java cross-check `Model.java:1305-1825`); `PointNormal.ts`.

- [ ] **Step 1 (failing test):** load `bronze_dagger`'s ob2 from the rev-274 content tree, apply its config recolours (hardcode the values from `sample.json`/config for this one item in the test), `CalculateNormals(64+0, 768+0, -50, -10, -50, true)`, compare `FaceColourA/B/C` against `testdata/lit/dagger.json`.
- [ ] **Step 2:** RED. **Step 3:** Port. **Step 4:** GREEN + compile-all. **Step 5:** Commit.

### Task 9: Config parser + icon composition (`getSprite`)

**Files:**
- Create: `pkg/render/config.go`, `pkg/render/config_test.go`, `pkg/render/icon.go`, `pkg/render/icon_test.go`

**Interfaces:**
- Produces:
  - `render.ObjConfig` struct: `{Debugname string; Model string; Zoom2D, Xan2D, Yan2D, Zan2D, Xof2D, Yof2D int; RecolS, RecolD []int32; ResizeX, ResizeY, ResizeZ int; Ambient, Contrast int; CertLink, CertTemplate string; CountObj []string; CountCo []int}` with client defaults (`Zoom2D=2000`, `Resize*=128` — mirror `pkg/objtype.NewObjType`).
  - `render.ParseContentConfigs(scriptsDir string) (map[string]*ObjConfig, error)` — walks `*.obj` files, parses `[name]` + the icon-relevant `key=value` lines (incl. `recol1s=`…`recol9d=`, verified in Task 1); applies cert resolution: a record with `certtemplate=` copies model/zoom/angles/offsets/recols from the template record and keeps its own identity (port the semantics from `pkg/objtype.toCertificate`, cross-check vendored `ObjType.ts`).
  - `render.Renderer` : `NewRenderer(contentDir string) (*Renderer, error)` (loads configs, textures via `pix3d.LoadTexturesFromPNGs`, `InitColourTable(0.8, textures)`, model store rooted at `<content>/models`), `(*Renderer) Icon(debugname string) (*Sprite32, error)` — the full `getSprite(id, outline=0, count=1)` pipeline: getModelLit (load ob2 by config's `Model` name from `models/obj/<name>.ob2` — fall back to searching other `models/*/` subdirs before failing; recolour loop; resize; `CalculateNormals(Ambient+64, Contrast*5+768, -50,-10,-50, true)` — VERIFY the ×5 contrast scaling site in the vendored ObjType/decoder and match it), 32×32 frame, `LowDetail=false`, zoom/pitch camera math, `ObjRender(0, Yan2D, Zan2D, Xan2D, Xof2D, sinPitch*zoom>>16 + minY/2 + Yof2D, cosPitch*zoom>>16 + Yof2D)`, silhouette pass (transparent→1 next to >1... transliterate exactly), drop-shadow pass (`0x301820`), cert composition (recursive Icon of certlink at zoom×1.5/outline=-1 semantics per the reference — transliterate the actual TS flow, including the Pix32 plot order), `OWi=33` when stackable — keep, it's part of the reference contract even though docs crop to 32.
- Reference: vendored `ObjType.ts` `getSprite` (line 365) + `getModelLit` (324); Java-274 `ObjType.java:208-334,514-546` cross-check.

- [ ] **Step 1 (failing parser tests):** table-driven: a fixture stanza with recols/resize/ambient/cert fields parses to the exact struct; defaults applied when absent; cert record inherits template fields. (~50 lines, concrete fixtures.)
- [ ] **Step 2:** RED → implement parser → GREEN.
- [ ] **Step 3 (failing icon test — THE gate):**
```go
func TestIconsMatchReference(t *testing.T) {
	r, err := render.NewRenderer(contentDir()) // env GOSCAPE_TEST_CONTENT or skip
	if err != nil { t.Fatal(err) }
	sample := loadSampleJSON(t, "testdata/icons274/sample.json")
	for _, it := range sample {
		got, err := r.Icon(it.Debugname)
		if err != nil { t.Fatalf("%s: %v", it.Debugname, err) }
		want := readRGBA(t, "testdata/icons274/"+it.Debugname+".rgba")
		if !bytes.Equal(got.RGBA(), want) {
			writeFailurePNG(t, got, it.Debugname) // for eyeballing
			t.Errorf("%s: pixel mismatch", it.Debugname)
		}
	}
}
```
(`RGBA()` uses the same 0→transparent conversion as the harness; test skips with a clear message when the content dir env is unset so CI-less runs stay green — but the implementer MUST run it with the env set and paste output.)
- [ ] **Step 4:** RED → implement `icon.go` → GREEN for all ~40. Debugging aid: compare intermediate `FaceColour` sums / vertexScreen values against harness dumps when pixels mismatch (extend dump.ts if needed).
- [ ] **Step 5:** Full package tests + compile-all + `-race ./pkg/render/...`. Commit.

### Task 10: `goscape-cli icons` verb + full-content run

**Files:**
- Create: `cmd/goscape-cli/cmd_icons.go`
- Modify: `cmd/goscape-cli/main.go` (verbs slice: `{"icons", runIcons, "Render 32x32 item inventory icons from a content tree (PNG)."}`)
- Modify: `docs/PORTING.md` (EXTENSION row)

**Interfaces:**
- Produces: `goscape-cli icons -src-dir <content> -out-dir <dir> [-obj <debugname>] [-log.level ...]` → `<out-dir>/<debugname>.png` per renderable obj record; prints `rendered=N skipped=M total=T` summary; exit 0 (some skips OK), 1 on runtime error, 2 on usage.

- [ ] **Step 1:** Write `cmd_icons.go` following `cmd_rsa.go`'s shape (flagset, exit codes); iterate `ParseContentConfigs` output sorted by debugname (determinism); skip+log records with no/missing model; `-obj` renders one.
- [ ] **Step 2:** Full run: `goscape-cli icons -src-dir /home/owner/Code/github.com/LostCityRS/Server274-ref/content -out-dir $TMPDIR/icons274`. Expected: rendered ≥ 95% of records; paste the summary; eyeball 6 PNGs across categories. Re-run → byte-identical (hash the dir twice).
- [ ] **Step 3:** PORTING.md: one EXTENSION row (goscape-only tool; cite pinned reference sources; note render3ZClip exclusion).
- [ ] **Step 4:** Gates (pkg + cmd tests, compile-all). Commit.

## Phase 2 — docsgen integration, rev-274 first (Task 11, in a `main` worktree)

### Task 11: docsgen `icons` step + Icon column (rev-274 only)

**Files (recreate the docs worktree: `git worktree add ../goscape-docs main`, venv per README):**
- Modify: `tools/docsgen/__main__.py` (new `icons` step), `tools/docsgen/families.py` (`_obj_row` icon cell + icons dir knowledge), `tools/docsgen/revisions.toml` (per-revision `icons = true|false` — 274 true, others false until their ports land)
- Create: `tools/docsgen/tests/test_icons.py`
- Generated: `overlays/rev-274/player/items/icons/*.png`, regenerated item pages

**Interfaces:**
- Consumes: the branch CLI the step already builds per revision; new verb `icons` (Task 10 contract).
- Produces: `_obj_row` first cell = `![{name}](icons/{debugname}.png)` when `<overlay>/player/items/icons/<debugname>.png` exists else `""` — implemented by passing an `icons_present: set[str]` into `generate_config_families` (extend its signature: `generate_config_families(all_dir, overlay_docs, icons=None)`); floor: `rendered/total >= 0.95` for icon-enabled revisions else `SystemExit`.

- [ ] **Step 1 (failing test):** `test_obj_row_icon_cell` — with `icons={"knife"}`, the knife row's first cell is `![Knife](icons/knife.png)` and a non-member item's is `""` (exercise via `generate_config_families` on a 2-record fixture all.obj + tmp overlay).
- [ ] **Step 2:** RED → implement: icons step runs `<cli> icons -src-dir <content_dir> -out-dir <overlay>/player/items/icons` (CLI already built by the existing worktree lifecycle), parses the `rendered=/total=` summary for the floor, collects the PNG basenames as the `icons` set for the configs step (ORDERING: run icons before configs within `run_revision`; both need the same worktree). `revisions.toml` gate: only when `icons = true`.
- [ ] **Step 3:** `python -m tools.docsgen --revision 274` full run → icons in overlay + item pages updated. Determinism: rerun → clean diff. Strict build rev-274; eyeball a rendered page's icons in the built site.
- [ ] **Step 4:** pytest suite; commit on `main` (code + regenerated rev-274 overlay incl. PNGs); `python tools/build.py all` redeploy; verify an icon serves from gh-pages (`git show gh-pages:rev-274/player/items/icons/<name>.png | head -c8` = PNG magic).

## Phase 3 — ports to 254 / 245.2 / 244 (Tasks 12–14, one per branch, in their worktrees)

Each of these three tasks has the same shape (differences are cosmetic per the spec — verify, don't assume):

### Task 12: rev-254 port · Task 13: rev-245.2 port · Task 14: rev-244 port

**Files (per branch):**
- Create: `pkg/render/**` (copied from rev-274), `cmd/goscape-cli/cmd_icons.go`, `tools/iconref/java/` (harness `IconDump.java` + runner script), `pkg/render/testdata/**` (branch goldens)
- Modify: `cmd/goscape-cli/main.go` (verb entry), `docs/PORTING.md` (EXTENSION row)
- Note: these branches also have `pkg/unpack/internal/model` — apply the same Task-2 promotion (`git mv` + import rewrite) BEFORE copying the rest, so the tree matches rev-274's layout.

**Interfaces:** same as Tasks 2–10 produce on rev-274.

- [ ] **Step 1 — pin delta audit:** diff the icon closure between this branch's Client-Java pin and the 274 pin: `git -C /home/owner/Code/github.com/LostCityRS/Client-Java diff <pin>..32f30626 -- '*ObjType.java' '*Model.java' '*Pix3D.java' '*Pix2D.java' '*Pix32.java' '*Pix8.java'` (paths differ per lineage — locate by filename first). Record every SEMANTIC difference (expect: none for 254 beyond naming; none for 245.2/244; the deob lineages differ textually — compare logic, not text). If a semantic difference appears, STOP and report before porting.
- [ ] **Step 2 — Java harness:** `tools/iconref/java/IconDump.java` — a `main()` compiled against the pinned client sources in a `$TMPDIR` worktree of the pin (`javac -d out $(worktree)/src/main/java/jagex2/**/*.java IconDump.java`); it replicates the client's startup subset: load config jag + textures jag from this branch's reference cache (`Server<rev>-ref/unpack-ref/cache`), `Pix3D.initColourTable(0.8)`, then for the sample list call `getSprite`/`getIcon` per THIS pin's signature and dump RGBA+PNG. Reuse rev-274's `sample.json` items filtered to ids existing at this revision, topped up to cover all categories (write the branch's own `sample.json`). If the pinned client can't compile headlessly (AWT deps etc.), stub the minimum and document; if truly impractical → STOP (user sign-off rule).
- [ ] **Step 3 — copy-adapt:** run the Task-2 promotion on this branch; copy `pkg/render/**` + `cmd_icons.go` from rev-274 (`git -C <this-worktree> checkout rev-274 -- pkg/render cmd/goscape-cli/cmd_icons.go` is WRONG here — rev-274's tree may reference newer helpers; instead `cp -r` from the rev-274 worktree and fix imports/build errors); update provenance comments to THIS branch's pin; apply any Step-1 semantic deltas.
- [ ] **Step 4 — gates:** golden test vs this branch's `testdata/icons<rev>/` (content dir = this revision's pinned content), full pkg tests, compile-all, `-race pkg/render`. Full-content `icons` run ≥95% + determinism rerun.
- [ ] **Step 5:** PORTING.md EXTENSION row; commit.

## Phase 4 — rev-225 variant port (Task 15)

### Task 15: rev-225 renderer (2-arg `getIcon` semantics)

**Files (in /home/owner/Code/github.com/zsrv/goscape-rev225):**
- Create: `pkg/render/**` (model decoder INTRODUCED here — copy the rev-274 `pkg/render/model` port verbatim; it decodes 225 content models per Task 1 Step 2), `cmd/goscape-cli/cmd_icons.go`, `tools/iconref/java/`, `pkg/render/testdata/**`
- Modify: `cmd/goscape-cli/main.go`, `docs/PORTING.md`

**Interfaces:** same CLI contract; internal differences per the 225 pin.

- [ ] **Step 1 — pin study (225-clean `cc3781de`):** read this pin's `getIcon(int id, int count)` + its `getInterfaceModel`/`drawSimple` equivalents and the ObjType decode (NO opcodes 110–114). Enumerate the exact deltas to encode: hardcoded `calculateNormals(64, 768, -50,-10,-50, true)`; no resize; no ambient/contrast fields in the parser (reject/ignore them if present in content — they can't be, per format, but the parser is shared-shape); no ×1.5/×1.04 zoom variants; cert composition via `crop(22,5,22,5)` — transliterate THIS pin's flow, not 274's. Record `getIcon`'s full body in the report.
- [ ] **Step 2 — Java harness:** as Phase 3, but fed from `Server225_2`'s jag archives (this client loads jags, not main_file_cache — point the harness at the on-disk jag files; the 225 client's Jagfile loader works on raw files). Write this branch's `sample.json` (225-era items; include recoloured + noted + stack variants; textured if the Task-1 census found any at 225).
- [ ] **Step 3 — port:** copy rev-274 `pkg/render` then REMOVE/adapt per Step 1's delta list (delete resize/ambient/contrast plumbing from `icon.go`+`config.go` for this branch; keep the rasterizer tiers — the 225 client has the same triangle paths). Model container note: content per-file ob2 only (the 225 jag-stream container is NOT needed — models come from the content tree; say so in a comment).
- [ ] **Step 4 — gates:** goldens, ≥95% full run (content = `Server225_2/content`), determinism, pkg tests, compile-all, `-race`.
- [ ] **Step 5:** PORTING.md row; commit.

## Phase 5 — full rollout (Task 16, in the `main` worktree)

### Task 16: five-revision icons, comparison column, redeploy

**Files:**
- Modify: `tools/docsgen/revisions.toml` (all five `icons = true`), `tools/docsgen/__main__.py` (`write_comparison` gains an Icons column — extend `comparison_lines` and its tests), docs pages that still call icons a future project (grep `docs/` for "rasterizer"/"reserved"/"follow-up" and update: known sites = the GENERATED comment in `tools/docsgen/families.py:_obj_row`, `docs/index.md` or player pages if they mention it — fix what the grep finds), README if it mentions the pending icon column.
- Generated: `overlays/rev-{225,244,245.2,254}/player/items/icons/**` + regenerated pages + comparison page.

- [ ] **Step 1:** Enable all five; extend `write_comparison`/`comparison_lines` + `test_comparison.py` for the Icons column (counts from the summary dict key `icons`).
- [ ] **Step 2:** `python -m tools.docsgen --revision all` (builds each branch CLI incl. 225's new verb). Floors ≥95% ×5; counts non-decreasing-ish (report, don't gate — icon counts track item counts).
- [ ] **Step 3:** Determinism (stage → rerun → empty diff); strict builds ×5; pytest; commit main (code + all overlays).
- [ ] **Step 4:** `python tools/build.py all`; verify icons serve for all five revisions; report gh-pages size delta (`git count-objects -vH` before/after).
- [ ] **Step 5:** Remove the docs worktree; final report incl. per-revision icon counts and the size delta.

---

## Self-review notes (applied)

- Spec coverage: full tier (T6–7), five-branch ports (T12–15), harness+goldens per pin (T3, T12–15 Step 2), fallback stop-rule (Global Constraints), content-tree inputs incl. 225 (T9, T15), docsgen floor/determinism/Icons column (T11, T16), PORTING.md rows (T10, T12–15), render3ZClip exclusion comment (T8), out-of-scope items untouched.
- The verification probes (T1) precede all porting; T16's doc-mention grep closes the "reserved column" trail.
- Type consistency: `pix3d.State`/`render.Frame`/`Sprite32`/`ObjConfig`/`Renderer.Icon` used consistently across T4–T10; docsgen `icons` set parameter named in both T11 steps.
- Porting-task code blocks are intentionally reference-cited rather than transcribed (Global Constraints, first bullet) — the pinned client source is the code.
