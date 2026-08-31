# Item-Icon Rasterizer Implementation Plan v2 (goscape-client reuse)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Supersedes:** `2026-07-07-item-icon-rasterizer.md` per the spec's Amendment 1 (reuse `~/Code/github.com/zsrv/goscape-client` instead of porting ~4,450 lines). Tasks R1–R3 of the v1 plan are DONE and carry over: R1 verifications, R2 decoder promotion in goscape (to be reverted by V1), R3 harness + goldens on goscape rev-274 (to be migrated by V1).

**Goal:** A headless `cmd/icondump` in goscape-client on all five branches — golden-pixel-verified against R3's reference goldens and per-pin Java harnesses — plus the docsgen step that fills the docs site's Icon column for all five revisions.

**Architecture:** goscape-client already contains the audited per-revision icon closure (`objtype.GetSprite`/`GetIcon`, `Model`, `Pix3D` incl. textured paths, `Pix32`), pure CPU integer rasterization. Net-new per branch: a deterministic palette hook, `cmd/icondump` (native cache/jag loading → `GetSprite` loop → PNG + index.tsv), and golden tests. docsgen builds each branch's icondump and maps ids→debugnames.

**Tech Stack:** Go 1.26 (goscape-client), R3's committed goldens, Node/tsx harness (already committed), javac harnesses for 254/245.2/244/225 pins, docsgen (Python).

## Global Constraints

- **Repos & branches:** goscape-client at `~/Code/github.com/zsrv/goscape-client` (rev-274 checked out; create sibling worktrees `../goscape-client-rev{225,244,245.2,254}` as needed). goscape server repo at `~/Code/github.com/zsrv/goscape` (rev-274) — V1 reverts its rasterizer-era commits; after V1 it is READ-ONLY for this project except docsgen work on `main` via a docs worktree. Reference repos Client-TS/Client-Java: READ-ONLY. Reference data: `Server{244,245.2,254,274}-ref/unpack-ref/cache/` (main_file_cache), `Server225_2` jag archives (locate at V7).
- **goscape-client conventions:** read its `CLAUDE.md` before first commit on each branch and follow it (build/test commands, doc conventions). Provenance comments cite Java `File.java:line @<pin>` like the surrounding code. Its pins per branch are authoritative in `docs/shared/REFERENCES.md` on its `main`.
- **Fidelity rule for golden mismatches:** R3 goldens are ground truth (generated from verbatim pinned Client-TS with `Math.random` pinned to 0.5 → brightness exactly 0.8). A goscape-client mismatch is a REAL fidelity bug: diagnose against the vendored TS in goscape rev-274's `tools/iconref/vendor/` (until V1 migrates it) and fix in goscape-client citing the reference line. If a fix would be non-trivial (>~20 lines or touching shared render state), STOP and report.
- **Determinism:** icondump must be byte-deterministic (deterministic palette hook, sorted outputs, no timestamps). The faithful jittered `InitColourTable` stays untouched for the real client.
- **Icon variant:** `GetSprite(id, 0, 1)` on 274/254 (arg order per branch! 254: `GetSprite(outlineRgb, count, id)` = `(0, 1, id)`; 244/245.2: `GetIcon(0, 1, id)`; 225: `GetIcon(id, 1)`). Output: 32×32 NRGBA PNG, pixel 0 → alpha 0 else opaque — the SAME conversion as R3's dump.ts.
- **Go invocations:** `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix (unless goscape-client's CLAUDE.md says otherwise). Gates per branch when code changes: touched-package tests + `go test -run '^$' ./...` compile-all; `-race` on touched packages where possible.
- **Commits:** `--no-gpg-sign`, trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`, `git status --short` first, stage only intended files.
- **Floors:** icons rendered ≥95% of docsgen item records per revision; rev-225 ≥90% (name-based mapping). Harness fallback + pin-delta stop rules from v1 remain.
- **Sandbox:** sibling-repo writes need the override (expected).

---

### Task 1: V1 — Migrate harness + goldens to goscape-client; revert goscape

**Files:**
- goscape-client rev-274 — Create: `tools/iconref/**` (moved harness incl. vendor/), `cmd/icondump/testdata/**` (palette.bin, tri/, lit/, icons274/ + sample.json)
- goscape rev-274 — Revert commits `32718d5e` (decoder promotion), `3934a4f1`, `101dfce0`, `d4dcfb85` (harness + goldens) via `git revert --no-commit` of the range then one commit, OR a single commit deleting `tools/iconref` + `pkg/render/testdata` and `git mv`-ing `pkg/render/model` back to `pkg/unpack/internal/model` with import restore. Either way: goscape's tree returns to pre-R2 state.

**Interfaces:**
- Produces: goldens at goscape-client rev-274 `cmd/icondump/testdata/` (paths every later test uses); harness rerunnable from its new home (update its README paths).

- [ ] **Step 1:** Copy `tools/iconref/**` and `pkg/render/testdata/**` from goscape rev-274 into goscape-client rev-274 at the paths above (adjust dump.ts's default `--out` and README paths to the new testdata location). `npm install` in the new location; rerun `npx tsx dump.ts --cache ~/Code/github.com/LostCityRS/Server274-ref/unpack-ref/cache --out ../../cmd/icondump/testdata` and verify byte-identical outputs vs the copied goldens (`git status` clean after rerun = proof the move didn't break rerunnability).
- [ ] **Step 2:** Commit on goscape-client rev-274: `test(icondump): import reference harness + rev-274 golden pixels (from goscape rev-274)` — note the source commits in the body.
- [ ] **Step 3:** In goscape rev-274: revert per above; gates `go test ./pkg/unpack/... -count=1` + compile-all green (decoder back home, importers restored). Commit: `revert(render): rasterizer moves to goscape-client (spec Amendment 1)` citing the amendment.
- [ ] **Step 4:** Verify goscape tree has no `pkg/render` and no `tools/iconref`; `git grep -c 'pkg/render/model'` = 0.

### Task 2: V2 — Deterministic palette hook + palette/tri golden tests (goscape-client rev-274)

**Files:**
- Modify: `pkg/jagex2/graphics/pix3d/pix3d.go` (add `InitColourTableDeterministic`)
- Create: `pkg/jagex2/graphics/pix3d/pix3d_golden_test.go`

**Interfaces:**
- Produces: `pix3d.InitColourTableDeterministic(brightness float64)` — identical to `InitColourTable` with the `rand.Float64()*0.03-0.015` term replaced by 0 (factor the shared body into an unexported `initColourTable(brightness float64)` both call; the faithful function passes the jittered value). Extension comment: reference clients jitter with `Math.random()`; expectation = brightness; goldens pin 0.8 exactly.

- [ ] **Step 1 (failing tests):** (a) palette test — `InitColourTableDeterministic(0.8)` then byte-compare `ColourTable` (packed as LE int32, low 32 bits of each int) against `cmd/icondump/testdata/palette.bin`; (b) tri tests — for each case in `testdata/tri/manifest.json`: reset state, `pix2d.SetPixels(32, buf, 32)` + prefill per manifest, call the routine named in the manifest with its recorded args (gouraud/flat direct; textured cases need `UnpackTextures`+`InitPool` first — load the textures jag from the reference cache via the repo's own `io` package, path from env `ICONDUMP_TEST_CACHE`, skip-with-message when unset), convert with the 0→transparent rule, byte-compare to the `.rgba`. Write the manifest-reader helper once; it is reused by nothing else (test-local).
- [ ] **Step 2:** RED (hook missing). **Step 3:** implement the hook (pure refactor + one new exported func). **Step 4:** GREEN with `ICONDUMP_TEST_CACHE` set — paste output. ANY tri mismatch → fidelity-bug rule from Global Constraints. **Step 5:** package tests + compile-all; commit.

### Task 3: V3 — `cmd/icondump` (rev-274) + icon golden test + full run

**Files:**
- Create: `cmd/icondump/main.go`, `cmd/icondump/icondump_test.go`

**Interfaces:**
- Produces: `icondump -cache <dir> -out <dir> [-id N]` → `<out>/<id>.png` per obj id with a renderable model, plus `<out>/index.tsv` (`id<TAB>name`, name from `ObjType.Name`, empty allowed) and a final stdout line `rendered=N skipped=M total=T`. Exit 0 (skips allowed), 1 runtime, 2 usage.
- Consumes (call sequence verified by exploration; adapt to real signatures): read config jag (`FileStore`/`Cache.Read(0, 2)` → `io.NewJagFile`) → `objtype.Init(jagConfig)`; textures jag (`Read(0, 6)`) → `pix3d.UnpackTextures(...)`, `pix3d.InitColourTableDeterministic(0.8)`, `pix3d.InitPool(20)`; models: `model.Init(count, provider)` with a provider serving archive-1 blobs from the cache (mirror the `stubProvider` pattern in model_test.go); loop id 0..total: era call `GetSprite(id, 0, 1)`; nil → skip; `Pix32.Pixels` → NRGBA (0→transparent) → PNG.

- [ ] **Step 1 (failing test):** `TestIcondumpMatchesGoldens` — for each entry in `testdata/icons274/sample.json`, run the in-process dump pipeline (factor `main.go` so the pipeline is a callable `run(cfg) error` — test calls it once into a t.TempDir) and byte-compare `<id>.png`-decoded RGBA (or dump raw RGBA alongside) against `testdata/icons274/<debugname>.rgba`. Skip-with-message without `ICONDUMP_TEST_CACHE`.
- [ ] **Step 2:** RED. **Step 3:** implement. **Step 4:** GREEN for all 39 — paste. Mismatch → fidelity-bug rule.
- [ ] **Step 5:** Full run against the reference cache: paste `rendered=/skipped=/total=` (expect total 3894, rendered well ≥95%); run twice → identical dir hashes. Eyeball 5 PNGs (describe).
- [ ] **Step 6:** Gates (pkg + cmd tests, compile-all, -race on touched); document the tool per goscape-client's doc conventions (README section or docs/ file per its CLAUDE.md). Commit.

### Task 4: V4 — docsgen icons step (rev-274 only, goscape `main` docs worktree)

**Files:**
- Modify: `tools/docsgen/revisions.toml` (add per-revision `[revisions.N.icons]`: `client_worktree` path, `client_branch`, `cache_dir` or `jag_dir`, `enabled` — 274 true only), `tools/docsgen/__main__.py` (icons step: build icondump from the client worktree, run, map, floor), `tools/docsgen/families.py` (icon cell via `icons` set param)
- Create: `tools/docsgen/tests/test_icons.py`
- Generated: `overlays/rev-274/player/items/icons/*.png` + regenerated item pages

**Interfaces:**
- Mapping (274-era): verify then rely on record `_index` == obj id — assert `len(records) == total` from icondump's summary and spot-assert 3 known ids; then icon file for a record = `<out>/<record._index>.png`, copied to `overlays/rev-N/player/items/icons/<debugname>.png` (debugname-keyed in the site, id-keyed only internally). `_obj_row` first cell: `![{name}](icons/{debugname}.png)` when present else `""` (same contract as plan v1 Task 11). Floor ≥95%.
- Icons step builds icondump: `git -C <client_worktree> worktree add $TMPDIR/... <client_branch>`? NO — use the client worktree directly (it's a persistent sibling like the server rev worktrees; docsgen builds `go build ./cmd/icondump` from it read-only into $TMPDIR).

- [ ] **Step 1 (failing test):** `test_obj_row_icon_cell` (icons={"knife"} → `![Knife](icons/knife.png)`; absent → `""`) + `test_icon_mapping_dense` (fixture: 3 records, summary total=3 → mapping by index; total mismatch → SystemExit).
- [ ] **Step 2:** RED → implement → GREEN.
- [ ] **Step 3:** Full `--revision 274` run; determinism rerun; strict build; eyeball a page. Commit on `main` (code + rev-274 icons overlay); `python tools/build.py all`; verify an icon serves from gh-pages.

### Task 5: V5 — rev-254 icondump · Task 6: V6 — rev-245.2 + rev-244 icondump

**Files (per branch, in `../goscape-client-rev{254,245.2,244}` worktrees):** `cmd/icondump/**` (copy-adapt from rev-274: arg order `GetSprite(0, 1, id)` on 254 / `GetIcon(0, 1, id)` on 245.2/244; verify each branch's actual signature first), the pix3d deterministic hook (same factoring — verify `InitColourTable` is textually identical first; if it differs, STOP and report), `tools/iconref/java/IconDump.java` harness + this branch's goldens `cmd/icondump/testdata/icons<rev>/` + sample.json, golden tests.

- [ ] **Step 1 — pin delta audit** (per v1 plan: diff this branch's Client-Java pin vs the 274 pin across the icon closure; goscape-client's audit-274 covers 254↔274 — read its relevant units first; semantic delta → STOP).
- [ ] **Step 2 — Java harness + goldens:** compile the pinned Client-Java in a $TMPDIR worktree with an `IconDump.java` main (load config+textures jags from `Server<rev>-ref/unpack-ref/cache`, `initColourTable(0.8)` with `Math.random` neutralized — subclass/shim or a tiny source patch in the $TMPDIR copy, documented; dump the branch sample per THIS pin's icon signature). ~40 items per the category list; reuse 274's sample where ids exist.
- [ ] **Step 3 — copy-adapt + tests + full run + gates** (as V2/V3 compressed: hook, tri goldens optional — palette golden REQUIRED via the branch's own harness dump or verified-identical `palette.bin`; icon goldens required; ≥95% full run; determinism ×2).
- [ ] **Step 4:** Docs row per goscape-client conventions; commit per branch. (V6 = two branches, one task each half; commit separately.)

### Task 7: V7 — rev-225 icondump (jag era)

**Files (`../goscape-client-rev225`):** as V5 but: locate the jag archives in `Server225_2` first (find config/models/textures jags; record paths in revisions.toml later); model loading via `model.Unpack(*io.Jagfile)` (whole-archive, no provider); icon call `GetIcon(id, 1)`; goldens from a `cc3781de`-pinned Client-Java harness fed the same jags. Name-based docsgen mapping happens in V8 — here just ensure index.tsv carries names.

### Task 8: V8 — five-revision rollout (goscape `main`)

**Files:** `tools/docsgen/revisions.toml` (all enabled; 225 entry uses jag paths + floor 0.90), `__main__.py` (name-based mapping for `config_source == "content-tree"` revisions: exact-name match in record order against index.tsv, each tsv row consumed at most once; unmatched → no icon), `write_comparison` Icons column + tests, doc-page mentions of the pending rasterizer (grep and fix), regenerate all five, determinism, strict ×5, pytest, commit, `build.py all`, gh-pages size delta report, remove docs worktree.

> **As-built addendum (2026-07-07):** the 225 mapping shipped DEBUGNAME-based, not
> the index.tsv display-name design above — controller-directed at execution:
> content-tree records carry debugnames, and `<content>/pack/obj.pack`
> (`id=debugname`, unique keys, zero collisions) maps record.debugname → id
> directly, avoiding the display-name-collision/greedy-order machinery entirely.
> Result: 1869/1869 matched. index.tsv remains emitted by icondump but is not
> consumed by docsgen. The docs worktree was also deliberately LEFT in place
> (not removed) for follow-up work.

---

## Self-review notes (applied)

- Spec Amendment 1 coverage: migration/revert (V1), deterministic hook + golden gating of reused code (V2–V3), icondump per branch with era signatures (V3, V5–V7), native data incl. 225 jags (V3/V7), docsgen id- and name-based mapping with floors 95/90 (V4, V8), per-pin Java harnesses retained (V5–V7), goscape-client conventions + provenance (Global Constraints).
- v1 leftovers intentionally dropped: goscape-cli `icons` verb, content-tree config parser, `pkg/render` in the server repo — all superseded; the v1 plan file remains for history with R1–R3's record.
- Type consistency: `InitColourTableDeterministic` (V2) used by V3/V5–V7; `run(cfg)`/index.tsv/`rendered= skipped= total=` contract shared V3→V4/V8.
