# rev-244 B6 — pack pipeline re-baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 225→244 `tools/pack` delta (cache-architecture rework: FileStream client cache, GZip, versionlist, modelFlags, compiler-symbols swap) and re-baseline goscape's pack output to full-tree byte parity against a reference cache packed by the upstream 244 toolchain from the EXACT recorded pins.

**Architecture:** Spec `docs/superpowers/specs/2026-06-05-rev244-b6-pack-pipeline-design.md` (commit `30461f78`). Phase 0 (reference cache) first — nothing else is verifiable without it. Then foundation plumbing (registries, modelFlags, FileStream cache handle), then per-stage ports in PackAll order, then the compiler-symbols swap, then the Phase-2 verification ladder (determinism → byte-diff loop → window closures → login smoke). All TS citations refer to the 244 pin `9aadcec4` in `$HOME/Code/github.com/LostCityRS/Engine-TS`.

**Tech Stack:** Go (pkg/pack, pkg/packall, pkg/io/filestream, pkg/io/gziputil, archive/zip), upstream toolchain for Phase 0 only (bun + RuneScriptKt-26 jar + java 25).

**Worker ground rules (B2-B5-proven, bake into every implementer prompt):**
- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Build with `CGO_ENABLED=0 go build -trimpath ./...`; `-race` needs `CGO_ENABLED=1`.
- Every commit: `git commit --no-gpg-sign` + `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer.
- Verify every `// TS <File>.ts:<lines>` citation against `git -C ../Engine-TS show 9aadcec4:<file> | cat -n` BEFORE writing.
- **Write Go test code with the Write/Edit tools, never bash heredocs** (sandbox mangles `!=`).
- Sandbox `git status` shows phantom `??` dotfiles — never stage them; never `git add -A`. Stage explicit paths only.
- Stale LSP diagnostics routinely false-alarm whole files after interface changes — trust real build/vet/test runs only.
- Ask "what does TS's CONSUMER do?" before modeling a producer (the `modelFlags` out-param and `updateCompiler` tasks below are pre-resolved examples).
- Run the modules/world suite in any task that touches a contract world tests exercise (cmd_smoke_pack stages, packall signature).
- The byte-parity contract for every stage is **the reference cache**, not the 225 goscape output. Tests that pin 225 output bytes update with the stage port (a test can pin a bug / an old format — verify against 244 TS first).

**Key shared facts (verified 2026-06-05, controller):**
- Engine-TS pin `9aadcec4` = local branch `244`; Content pin `e5d0282e` = local branch `244` tip. Working copies sit on `254-GOSCape`-line branches — NEVER check out branches in those repos; use the worktrees from Task 1.
- `BUILD_SRC_DIR` defaults to `../content` at the pin (Environment.ts:109) — the engine/content sibling layout is native.
- RuneScriptKt-26 jar is at `LostCityRS/Server/engine/RuneScriptCompiler.jar`, sha256 `38e16e2c375cfdb0179cce1cab9c06d279cc7c30b0cbc298c97a37c4dca1851a` (upstream-verified against the release-26 published checksum at download).
- Go FileStream (B1): `filestream.New(dir, createNew, readOnly)`, `Write(archive, file, data, version) bool`, `Read(archive, file, decompress) []byte`, `Count(archive) int`, `Has(archive, file) bool`. Go GZip (B1): `gziputil.CompressGz(src, off, length)`.
- 244 cache layout written by packAll (PackAll.ts:35-90): archive 0 = jags (1=title, 2=config, 3=interface, 4=media, 5=versionlist, 6=textures, 7=wordenc, 8=sounds), archive 1 = models (gzip, version 1), archive 2 = animsets (gzip, version 1), archive 3 = midis (gzip, version 1), archive 4 = maps (gzip client m/l files, version 1).
- **Parity exemptions (built into Task 17's gate, each gets a PORTING.md row):** `data/pack/server/build` is `p4(Date.now()/1000)` — a wall-clock timestamp, never byte-comparable; `data/pack/ondemand.zip` is fflate `zipSync` whose per-entry mtimes are the pack wall-clock — compare ENTRY CONTENTS, not raw bytes.

---

### Task 0: Housekeeping — stale test binary

**Files:**
- Delete: `script.test` (repo root)

- [ ] **Step 1: Verify and delete**

```bash
file script.test   # expect: ELF ... Go BuildID ... (a stray `go test -c` artifact, 23 MB, untracked)
rm script.test
git status --short # expect: script.test gone; do NOT stage anything else
```

No commit (the file was untracked).

### Task 1: Phase 0 — pinned reference checkouts + upstream pack run

**Files (all OUTSIDE goscape; sandbox-bypass with user permission):**
- Create: `$HOME/Code/github.com/LostCityRS/Server244-ref/engine` (worktree @ `9aadcec4`)
- Create: `$HOME/Code/github.com/LostCityRS/Server244-ref/content` (worktree @ `e5d0282e`)
- Create: `Server244-ref/engine/RuneScriptCompiler.jar` (copied), `Server244-ref/engine/.env`

This task is **controller-direct** (interactive permission prompts; not subagent work).

- [ ] **Step 1: Create the worktrees**

```bash
cd $HOME/Code/github.com/LostCityRS
mkdir -p Server244-ref
git -C Engine-TS worktree add ../Server244-ref/engine 9aadcec4
git -C Content   worktree add ../Server244-ref/content e5d0282e
git -C Server244-ref/engine log -1 --format='%H'   # expect 9aadcec4e9560b810b5e5eee31aadc67f3b206cd
git -C Server244-ref/content log -1 --format='%H'  # expect e5d0282e03b383efd3b2a81e63090e703ffb5399
```

- [ ] **Step 2: Install the compiler jar + verify its sha**

```bash
cp Server/engine/RuneScriptCompiler.jar Server244-ref/engine/
sha256sum Server244-ref/engine/RuneScriptCompiler.jar
# expect 38e16e2c375cfdb0179cce1cab9c06d279cc7c30b0cbc298c97a37c4dca1851a
```

- [ ] **Step 3: Write `.env` (skip the network jar check; pin java path)**

Create `Server244-ref/engine/.env`:

```
# B6 reference pack — goscape rev-244 (spec 2026-06-05).
# BUILD_SRC_DIR defaults to ../content (sibling worktree) — do not override.
BUILD_STARTUP_UPDATE=false
BUILD_VERIFY=true
```

`BUILD_STARTUP_UPDATE=false` stops Build.ts calling `updateCompiler()` (axios → github.com — sandboxed; the jar is already in place and sha-verified). `BUILD_JAVA_PATH` default `java` resolves to Temurin 25 on PATH.

- [ ] **Step 4: bun install (pin's lockfile)**

```bash
cd Server244-ref/engine && bun install --frozen-lockfile
```

Try sandboxed first (bun's global cache may satisfy it); on network failure re-run with the sandbox bypass (user prompt). Do NOT copy `node_modules` from `Server/engine` — its lockfile differs from the pin's (`@2004scape/rsbuf` version matters for byte parity).

- [ ] **Step 5: Run the pack**

```bash
cd Server244-ref/engine && bun run build 2>&1 | tee /tmp/claude/b6-ref-pack.log
```

Expected: `pack: <time>` timing line, exit 0. **CRITICAL POST-CHECK** — PackAll.ts:48-55 SWALLOWS a jar failure outside a worker (`catch { if (parentPort) throw }`): verify the compiler actually ran:

```bash
ls -la data/pack/server/script.dat data/pack/server/script.idx   # must exist, fresh mtime
ls data/symbols/ | head                                          # *.sym files, fresh
ls data/pack/ # expect: client/ server/ main_file_cache.dat main_file_cache.idx0..4 ondemand.zip
```

If script.dat is missing/stale: read the jar's stderr in the log, fix (likely cwd/env), re-run. Do not proceed on a half-packed tree.

- [ ] **Step 6: Record the reference manifest into goscape**

```bash
cd $HOME/Code/github.com/LostCityRS/Server244-ref/engine
{ find data/pack data/symbols -type f ! -name build ! -name ondemand.zip -print0 | sort -z | xargs -0 sha256sum; } > $HOME/Code/github.com/zsrv/goscape/pkg/packall/testdata/ref244_manifest.txt
# entry list for the content-level ondemand.zip check:
python3 - <<'EOF'  # (or `unzip -l`) — list ondemand.zip entries + sizes
import zipfile
z = zipfile.ZipFile('data/pack/ondemand.zip')
for i in sorted(z.namelist()):
    print(i, len(z.read(i)))
EOF
```

Save the zip entry listing as `pkg/packall/testdata/ref244_ondemand_entries.txt`. Record in both files' headers (hand-edit a `#` comment line): engine pin, content pin, jar sha, pack date.

- [ ] **Step 7: Commit (goscape side only)**

```bash
cd $HOME/Code/github.com/zsrv/goscape
git add pkg/packall/testdata/ref244_manifest.txt pkg/packall/testdata/ref244_ondemand_entries.txt
git commit --no-gpg-sign -m "test(packall): 244 reference-cache sha256 manifest (Engine-TS 9aadcec4 + Content e5d0282e + RuneScriptKt-26) [rev-244 B6]" -m "Packed via Server244-ref worktrees; server/build and ondemand.zip raw bytes excluded (timestamp / fflate mtimes — see plan Task 17 exemptions). data/symbols included as the compiler-parity diff anchor." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 8: Record the jar sha in REFERENCES.md (main branch, via worktree)**

```bash
git worktree add $TMPDIR/goscape-main main
# edit $TMPDIR/goscape-main/REFERENCES.md — fill the RuneScriptKt row's
# "(record when fetched ...)" placeholder with:
#   release tag `26` | jar sha256 `38e16e2c375cfdb0179cce1cab9c06d279cc7c30b0cbc298c97a37c4dca1851a` (captured 2026-06-05 from the auto-downloaded Server/engine jar)
git -C $TMPDIR/goscape-main add REFERENCES.md
git -C $TMPDIR/goscape-main commit --no-gpg-sign -m "docs(references): pin RuneScriptKt-26 jar sha256 for rev-244" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
git worktree remove $TMPDIR/goscape-main
```

### Task 2: Reference-run analysis (controller-direct, no code)

Read-only investigation; output = facts appended to this plan file (edit in place under this task) for downstream tasks.

- [ ] **Step 1: Capture the jar's input/output contract**

```bash
cd $HOME/Code/github.com/LostCityRS/Server244-ref/engine
ls data/symbols/                              # full .sym inventory
head -5 data/symbols/commands.sym data/symbols/varp.sym data/symbols/interface.sym
xxd data/pack/server/script.dat | head -5     # header shape
xxd data/pack/server/script.dat | tail -5     # trailer shape
xxd data/pack/server/script.idx | head -3
cat data/pack/server/script.dat | wc -c; cat data/pack/server/script.idx | wc -c
```

- [ ] **Step 2: Compare script.dat container format vs the 225 baseline**

```bash
xxd $HOME/Code/github.com/LostCityRS/Server225_2/engine/data/pack/server/script.dat | head -5
xxd $HOME/Code/github.com/LostCityRS/Server225_2/engine/data/pack/server/script.dat | tail -5
```

Record: same/different trailer layout, version field value, count fields. This decides whether Task 14's compiler re-baseline is bug-stack-shaped (iterate) or structural (STOP — user checkpoint per spec Risk 1).

- [x] **Step 3: Inventory the content pin's pack-registry inputs**

```bash
ls $HOME/Code/github.com/LostCityRS/Server244-ref/content/pack/
# record whether map.pack / midi.pack / animset.pack exist as committed inputs
# or were regenerated by the run (check `git -C ../content status --short`).
```

**FINDINGS (controller, 2026-06-05, from the live reference run):**

1. **Reference pack run:** `bun run build` = 9.59s, exit 0, BUILD_VERIFY=true,
   rsbuf `244.1.0` / rsmod-pathfinder `5.0.4` resolved from the pin's frozen
   lockfile (bun global cache, no network). Jar ran visibly (netty warnings).
   Three upstream `missing model` warnings (model_111_npc_head,
   model_434_npc, model_2925_npc) — expected modelFlags>0 posture, goscape
   should reproduce them. 2,641 files under data/pack + 32 .sym files.
2. **244 client/ tree confirms the architecture move:** no `client/models`,
   no `client/wordenc`, no jingles/songs dirs — only
   config/interface/maps/media/sounds/textures/title/versionlist. wordenc
   exists ONLY as cache(0,7).
3. **script.dat container is STRUCTURALLY IDENTICAL to 225** (Risk-1
   de-risked from "structural" to "byte-level"): same `p2(count)` header
   (8120 scripts vs 8032), same per-script record framing starting
   `p4(len) [name] \0 <abs source path>`, same trailer shape, same idx
   format (`p2(count)` + per-script `p4(len)`).
   - **Embedded absolute source paths**: script.dat embeds the resolved
     content path (`$HOME/Code/github.com/LostCityRS/Server244-ref/
     content/...`) — byte parity REQUIRES packing from the same src dir
     (same as the Arc-26/225 posture).
   - **Uniform +2 bytes per script** in the first idx entries (e6/e4,
     1496/1494, 084a/0848, 06fc/06fa…) — suggests ONE new 2-byte field per
     script in RuneScriptKt-26 emission vs RuneScriptTS. Task 17 pins it.
4. **commands.sym leads with `0\tpush_constant_int`** — the jar's command
   table is keyed by the RUNTIME opcode ids (B4's renumbered table).
   varp.sym rows `<id>\t<name>\t<type>\t<protect>`; writeinv rows
   `<id>\t<name>\tnone\t<bool>`; interface.sym is the post-split file (root
   names only), component.sym carries the `if:com` names.
5. **animset.pack / map.pack / midi.pack are COMMITTED Content inputs at the
   pin** (content worktree clean after the run — PackFile regen did not fire);
   the 225-era `model.order`/`anim.order`/`base.order` files are GONE from
   content/pack (graphics no longer order-driven). Go Task 4 loads these
   packs as ordinary committed registries.

### Task 3: NameMap/Parse — \r normalization

**Files:**
- Modify: `pkg/pack/namemap.go` (loadOrder/loadPack/loadDir analogs), `pkg/pack/parse.go`
- Test: `pkg/pack/parse_test.go`, `pkg/pack/namemap_test.go`

TS contract: NameMap.ts:8-60 + Parse.ts:6-30 — every text read switches `split(/\r?\n/)` → `replace(/\r/g, '').split('\n')`, and Parse.ts gains exported `readTextNormalize(path)`. Observable delta: a STRAY `\r` not followed by `\n` (or mid-line) is now stripped instead of preserved.

- [ ] **Step 1: Write the failing test** — a fixture line `"a\rb=1\r\nc=2"` through the Go loadPack/loadFile analogs must yield `ab=1` / `c=2` (mid-line `\r` stripped).
- [ ] **Step 2: Run to verify it fails** (`go test ./pkg/pack/ -run 'CR' -v` — name the tests `*_CRNormalization`).
- [ ] **Step 3: Implement** — centralize a `readTextNormalize(path string) (string, error)` in parse.go mirroring Parse.ts:6-12; route the loaders through it (loadFile, loadFileFull, the namemap loaders, PixPack meta read — grep `\r?\n`-equivalent handling: `grep -rn 'splitLines\|\\r' pkg/pack --include='*.go' | grep -v _test`).
- [ ] **Step 4: Run package tests** (`go test ./pkg/pack/... -count=1`). PASS.
- [ ] **Step 5: Commit** `feat(pack): normalize CR handling across text loaders (TS NameMap.ts/Parse.ts 244) [rev-244 B6]`.

### Task 4: PackFile — animset/map/midi registries + validateConfigPack gate removal

**Files:**
- Modify: `pkg/pack/packfile.go`, `pkg/pack/registry.go`
- Test: `pkg/pack/packfile_test.go`, `pkg/pack/registry_test.go`

TS contract (PackFile.ts:114-210):
1. `validateConfigPack` drops the `if (transmitted)` gate — the names-in-files check now runs for ALL config packs under BUILD_VERIFY (PackFile.ts:117-121).
2. Three NEW PackFiles: `AnimSetPack` (`'animset'`, validateFilesPack, models dir, `.anim`, regen=default(true)), `MapPack` (`'map'`, validateFilesPack, maps dir, `.jm2`, regen=false), `MidiPack` (`'midi'`, validateFilesPack, jingles+songs dirs, `.mid`) — note MidiPack takes TWO source dirs (PackFile.ts:191,205-206).

Go: extend `Registry` with `AnimSet, Map, Midi *PackFile` + `EnsureAnimSet/EnsureMap/EnsureMidi` and teach the `ensure` machinery the multi-dir source case if it can't already express it (check `pkg/pack/packfile.go` validateFilesPack analog first — cite the Go lines in the commit).

- [ ] **Step 1: Failing tests** — (a) registry resolves `animset`/`map`/`midi` types with the right extensions/dirs; (b) a config pack with a name missing from src files now fails validation even when not transmitted (seed a fixture; assert error contains the TS message shape).
- [ ] **Step 2: Verify RED.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: `go test ./pkg/pack/... -count=1` PASS.** Pre-existing tests that relied on non-transmitted packs skipping the name check update WITH a TS citation in the test comment.
- [ ] **Step 5: Commit** `feat(pack): animset/map/midi pack registries + universal config-name verification (TS PackFile.ts 244) [rev-244 B6]`.

### Task 5: modelFlags plumbing + readConfigs threading

**Files:**
- Modify: `pkg/pack/pack_configs.go` (readConfigs analog + packConfigs), `pkg/pack/registry.go` if needed
- Modify: signatures only in `pkg/pack/{idk,inv,loc,npc,obj,seq,spotanim,...}.go` pack funcs
- Test: `pkg/pack/pack_configs_test.go`

TS contract (PackShared.ts:137-141 + the per-config signatures): `ConfigPackCallback` gains `modelFlags: number[]`; `readConfigs(...)` gains and forwards it; `packConfigs(cache, modelFlags)`. Go: thread `modelFlags []int` through the config-pack pipeline. The slice is allocated by PackAll (Task 13) sized `ModelPack.max`; pass it down. Config packers that don't use it take `_ []int`.

**modelFlags is write-only below PackAll** — flag values land in Tasks 6-12; this task is the mechanical signature thread + compile-green + one pin test that flags written by a stub callback reach the caller's slice (shared backing array).

- [ ] Steps: failing signature-level test → RED → thread → `go test ./pkg/pack/... && go build ./...` → commit `refactor(pack): thread modelFlags through config packing (TS PackShared.ts:137-141) [rev-244 B6]`.

### Task 6: SeqConfig — preanim_move / postanim_move / duplicatebehavior

**Files:**
- Modify: `pkg/pack/seq.go`
- Test: `pkg/pack/seq_test.go` (+ roundtrip)

TS contract (SeqConfig.ts:113-143 parse, :200-211 pack): three new keys.
- parse: `preanim_move`: delaymove→0, delayanim→1, merge→2, else null. `postanim_move`: delaymove→0, abortanim→1, merge→2, else null. `duplicatebehavior`: '0'→0, reset→1, reset_loop→2, else null.
- pack (client side): preanim_move → `p1(9); p1(v)`; postanim_move → `p1(10); p1(v)`; duplicatebehavior → `p1(11); p1(v)`.

- [ ] Steps: failing parse+pack tests (each enum value + an invalid value → null/error per goscape's parse convention; packed bytes `09 vv`/`0A vv`/`0B vv` in the client stream) → RED → implement → package tests → commit `feat(pack): seq preanim_move/postanim_move/duplicatebehavior (TS SeqConfig.ts 244) [rev-244 B6]`.

### Task 7: NpcConfig — ambient/contrast/headicon/alwaysontop + modelFlags

**Files:**
- Modify: `pkg/pack/npc.go`
- Test: `pkg/pack/npc_test.go`

TS contract (NpcConfig.ts:22-33 parse keys; :293-297 modelFlags; :379-392 pack):
- numberKeys += `ambient`, `contrast`, `headicon`; booleanKeys += `alwaysontop`.
- model*N* values: `modelFlags[v] |= 0x2`; head*N*: `modelFlags[v] |= 0x2`.
- client opcodes: alwaysontop → `p1(99)` (presence-only, like minimap); ambient → `p1(100); p1(v)`; contrast → `p1(101); p1(v)`; headicon → `p1(102); p2(v)`.

- [ ] Steps: failing tests (parse accepts the new keys; packed bytes; modelFlags bits set for model1/head1 fixture values) → RED → implement → tests → commit `feat(pack): npc ambient/contrast/headicon/alwaysontop + model flags (TS NpcConfig.ts 244) [rev-244 B6]`.

### Task 8: ObjConfig — resize/ambient/contrast + model_index flag tracking

**Files:**
- Modify: `pkg/pack/obj.go`
- Test: `pkg/pack/obj_test.go`

TS contract (ObjConfig.ts:17-26 keys; :247-252 + :397-430 flags; :397-414 pack):
- numberKeys += `resizex/resizey/resizez/ambient/contrast`.
- client opcodes: resizex `p1(110);p2`, resizey `p1(111);p2`, resizez `p1(112);p2`, ambient `p1(113);p1`, contrast `p1(114);p1`.
- Track during pack: `model` values → `model[]`; `manwear`/`manwear2`/`womanwear`/`womanwear2`/`manwear3`/`womanwear3` first values → `worn[]`; `members===true` → flag. After the key loop: members ? (model |= 0x40, worn |= 0x10) : (model |= 0x20, worn |= 0x08). `manhead/womanhead/manhead2/womanhead2` → `modelFlags[v] |= 0x80` inline.
- The cert-template restructure (ObjConfig.ts:207-243): `configs.get(debugname)!` + name-fill move — in Go this is the existing nil-check shape; verify goscape's current branch structure against the 244 listing and adjust ONLY if the observable changes (the name-fill condition is unchanged; expect NO-OP, record as decision row if so).

- [ ] Steps: failing tests (new opcodes' bytes; flag matrix: members obj w/ model+manwear → 0x40/0x10; f2p obj → 0x20/0x08; manhead → 0x80) → RED → implement → tests → commit `feat(pack): obj resize/ambient/contrast + model_index flags (TS ObjConfig.ts 244) [rev-244 B6]`.

### Task 9: InvConfig — stock simplification

**Files:**
- Modify: `pkg/pack/inv.go`
- Test: `pkg/pack/inv_test.go`

TS contract (InvConfig.ts:103-152): the `size`-prescan, duplicate-stockN error, stockN>size error, and the `undefined`-slot filler (`p2(-1);p2(0);p4(0)`) are ALL deleted; stock lines simply `stock.push(value)` in file order and emit densely. Observable: sparse `stock3` without `stock1/2` now packs 1 entry, not 3; duplicate stockN no longer errors.

- [ ] Steps: failing tests pinning the NEW contract (sparse stock packs densely in line order; duplicate stock lines both emit) — UPDATE any existing test pinning the 225 sparse-fill bytes (cite InvConfig.ts:103-152 in the test comment) → RED → implement (delete the Go analogs) → tests → commit `feat(pack): inv stock packs densely in declaration order (TS InvConfig.ts 244) [rev-244 B6]`.

### Task 10: Idk/Loc/SpotAnim — modelFlags writes + PackShared CRC/frame_del

**Files:**
- Modify: `pkg/pack/idk.go` (model→0x80, head→0x80; IdkConfig.ts:149-156), `pkg/pack/loc.go` (all three ModelPack-hit sites → `|= 0x4`; LocConfig.ts:334-360), `pkg/pack/spotanim.go` (model → `|= 0x2`; SpotAnimConfig.ts:104-107)
- Modify: `pkg/pack/pack_configs.go` — (a) DELETE the frame_del.dat block (PackShared.ts 225:355-390 deleted at 244); (b) update the six client-CRC pin constants (PackShared.ts 244): seq `1405403166`, loc `1195428820`, spotanim `117013845`, npc `-997428438`, obj `1589810970`, varp `-1961744050`; (c) `packConfigs` tail gains `cache.Write(0, 2, <client/config bytes>)` — wire the cache handle param from Task 5.
- Test: respective `*_test.go` + `pack_configs_test.go`

**frame_del consumer check (do FIRST):** `grep -rn "frame_del" modules pkg --include='*.go'` — if the goscape RUNTIME reads `server/frame_del.dat`, check what 244 TS runtime reads instead (`git -C ../Engine-TS grep -n frame_del 9aadcec4 -- src/`) and port that reader change in THIS task; if nothing reads it, it's a clean delete + decision row.

- [ ] Steps: failing tests (flag bits per stage fixture; frame_del absent from output; CRC constants — the CRC pins only PASS against real 244 content, so assert the CONSTANT value in a unit test and leave enforcement to BUILD_VERIFY at pack time) → RED → implement → `go test ./pkg/pack/... -count=1` → commit `feat(pack): idk/loc/spotanim model flags, 244 client CRC pins, frame_del removal [rev-244 B6]`.

### Task 11: wordenc + sound + sprites/title/media/textures — cache wiring

**Files:**
- Modify: `pkg/pack/wordenc/pack.go` — REPLACE the four-txt jag build with the raw blob: TS chat/pack.ts 244 is 7 lines — `cache.write(0, 7, fs.readFileSync('data/raw/wordenc'))`. Source the blob from the engine-owned raw dir (new `data/raw/wordenc` in goscape — copy from `git -C ../Engine-TS show 9aadcec4:data/raw/wordenc > data/raw/wordenc`, committed as binary engine data with a README line citing its origin).
- Modify: `pkg/pack/audio/sound.go` — re-enable the synth CRC check with `-1415586973` (sound/pack.ts:47-49) + `cache.Write(0, 8, <client/sounds bytes>)`.
- Modify: `pkg/pack/sprites/sprites.go` — PackTitle/PackMedia/PackTexture gain the cache param and write archive-0 files 1/4/6 (title.ts:33, media.ts:36, textures.ts:24).
- Modify: `pkg/pack/graphics/` or wherever PixPack's Go analog lives — the `meta/<name>.pal.png` palette workaround (PixPack.ts:185-192): if the pal png exists, palette comes from IT (CRC-preserving), else the generated palette.
- Test: respective `*_test.go`

**wordenc consumer check (do FIRST):** modules/world loads the wordenc filter (`TestNewServer_LoadsWordencFilter` is skip-gated on the B1 window). The 244 SERVER still reads the packed wordenc jag out of the cache — confirm via `git -C ../Engine-TS grep -n "wordenc" 9aadcec4 -- src/ | head` which path the engine reads, and keep goscape's runtime reader pointed at the matching artifact.

- [ ] Steps: failing tests (wordenc stage output == raw blob bytes at cache(0,7); sound CRC constant; title/media/texture archive-0 writes present; pal.png branch) → RED → implement → package tests + modules/world suite → commit `feat(pack): raw wordenc blob, synth CRC re-pin, client jags into cache archive 0 (TS 244) [rev-244 B6]`.

### Task 12: graphics rewrite + midi rewrite (cache archives 1/2/3)

**Files:**
- Rewrite: `pkg/pack/graphics/pack.go`
- Rewrite: `pkg/pack/audio/midi.go`
- Test: `pkg/pack/graphics/*_test.go`, `pkg/pack/audio/midi_test.go`

TS contract (graphics/pack.ts 244 — the whole file is 41 lines; midi/pack.ts 244 — 20 lines):

```
packClientGraphics(cache, modelFlags):
  for each src models/*.ob2: id = ModelPack.getByName(basename-sans-ext); if data non-empty → cache.write(1, id, compressGz(data), version=1)
  for id in [0, ModelPack.max): if !cache.has(1, id) && modelFlags[id] > 0 → warn "missing model <name> (<id>)"
  for each src models/*.anim: id = AnimSetPack.getByName(...) → cache.write(2, id, compressGz(data), version=1)

packClientMidi(cache):
  for each {jingles,songs}/*.mid: id = MidiPack.getByName(...) → cache.write(3, id, compressGz(data), version=1)
```

The ENTIRE 225 jag-aggregation body (ob_head/frame_*/base_* parts → client/models jag; bzip2 jingles/songs dirs) is deleted upstream — delete the Go analogs in the same commit. Iteration order: `listFilesExt` analog must be deterministically sorted in Go (filepath.WalkDir is lexical — cite it).

- [ ] Steps: failing tests (fixture .ob2/.anim/.mid → gzip'd cache entries at the right archive/file/version; empty file skipped; missing-model warning gated on modelFlags>0; client/models jag + jingles/songs dirs NOT produced) → RED → implement → package tests → commit `feat(pack): per-file gzip model/animset/midi cache archives, drop 225 jag aggregation (TS graphics+midi pack.ts 244) [rev-244 B6]`.

### Task 13: maps rewrite (archive 4 + gzip + npc validation)

**Files:**
- Rewrite: `pkg/pack/maps/pack.go`
- Test: `pkg/pack/maps/*_test.go`

TS contract (map/Pack.js 244 — full-file listing is the contract, `git -C ../Engine-TS show 9aadcec4:tools/pack/map/Pack.js | cat -n`):
- Client m/l files: `compressGz(data)` (was BZip2) — server copies stay raw.
- After both encodes: `cache.write(4, MapPack.getByName('m<x>_<z>'), <client m bytes>, 1)` and same for `l<x>_<z>` (Pack.js:407-408).
- **npc/obj section byte-order change:** 225 emitted in source-line (Map insertion) order; 244 iterates `level→x→z` ascending and emits `pos=(level<<12)|(x<<6)|z` per occupied tile (Pack.js:286-352). Land/loc encode logic is byte-identical to 225 (representational rewrite only) — verify by reading both listings side-by-side, then port the npc/obj ordering change.
- NPC validation: `NpcType.load('data/pack'); type missing → printFatalError` + models/heads → `modelFlags |= 0x4` (Pack.js:295-318). Go analog: the runtime npc-type loader (`pkg/objtype`) reads the packed npc.dat — note the ordering constraint: maps stage runs AFTER configs pack (PackAll order) so npc.dat exists.
- Per-artifact rebuild conditions become per-file `shouldBuildFile` triplets (Pack.js:120-135) — map onto goscape's freshness.go analogs.
- Go internal representation: keep goscape's existing struct-based readMap if output bytes match; the TS nested-array rewrite is representational. Cite the equivalence in a comment; the npc/obj ORDER change is the behavioral piece.

- [ ] Steps: failing tests (client m/l gzip'd + cache(4,id,version 1); npc/obj server files emit level→x→z ascending — fixture with two tiles inserted in reverse source order; unknown npc id → error; modelFlags 0x4 from npc models) → RED → implement → package tests → commit `feat(pack): maps to gzip + cache archive 4, level/x/z npc-obj emission, npc-type validation (TS map/Pack.js 244) [rev-244 B6]`.

### Task 14: versionlist (NEW stage)

**Files:**
- Create: `pkg/pack/versionlist/pack.go`
- Test: `pkg/pack/versionlist/pack_test.go`

TS contract (versionlist/pack.ts — full 133-line file quoted in the B6 diff at $TMPDIR/b6-diff.txt:3639-3777; re-extract with `git -C ../Engine-TS show 9aadcec4:tools/pack/versionlist/pack.ts | cat -n`):
- Jag `data/pack/client/versionlist` with members `model_version/model_crc/model_index`, `anim_version/anim_crc/anim_index`, `midi_version/midi_crc/midi_index`, `map_version/map_crc/map_index`; then `cache.write(0, 5, <jag bytes>)`.
- model: per id < ModelPack.max — present in cache(1,id) ? `p2(1) / p4(crc(data[0:len-2])) / p1(modelFlags[id])` : `p2(0)/p4(0)/p1(0)`. **CRC excludes the 2-byte version trailer** (`data.length - 2`).
- anim: same over AnimSetPack/archive 2 (no index flags); then `AnimPack.max` × `p2(0)` for anim_index (TS todo posture — port as-is).
- midi: same over MidiPack/archive 3; midi_index = `pbool(jingle file exists)` per id (prefetch flag).
- map: version/crc over MapPack/archive 4; map_index iterates mapX 0..99 × mapZ 0..254: if `m<x>_<z>` in MapPack → `p2((x<<8)|z) p2(mapId) p2(locMapId) pbool(prefetch)`, prefetch = region in free2play.csv set (parse: skip `//`+empty, split `_`, `(mx<<8)|mz`).
- Jagfile flavor: `Jagfile.new(true)` (same flavor the interface packer uses — check the Go jagfile API for the flag's analog).

- [ ] Steps: failing tests (synthetic cache with 2 models/1 anim/1 midi/1 map pair → exact member bytes; absent ids zero-filled; crc range excludes trailer; free2play prefetch bit) → RED → implement → tests → commit `feat(pack): client versionlist stage (TS versionlist/pack.ts 244) [rev-244 B6]`.

### Task 15: PackAll orchestration + build stamp + ondemand.zip + DevThread/app.ts rows

**Files:**
- Modify: `pkg/packall/packall.go`
- Test: `pkg/packall/packall_test.go`
- Modify: `cmd/goscape-cli/cmd_smoke_pack.go` stage list if it enumerates stages (check first)

TS contract (PackAll.ts:31-90):
1. Open `cache := filestream.New(outDir, true /*createNew — truncates*/, false)` after ClearFsCache/revalidate.
2. `modelFlags := make([]int, ModelPack.max)` (TS zeroes 0..ModelPack.max).
3. Stage order: packConfigs(cache, modelFlags) → clientinterface(cache, modelFlags) → **compiler** (Go: WriteCompilerSymbols (Task 16) + RunServerCompiler — native, replacing TS's jar exec) → title → media → texture → wordenc → sound → graphics(cache, modelFlags) → midi(cache) → maps(cache, modelFlags) → versionlist(cache, modelFlags).
4. `server/build`: `p4(unix-seconds)` (PackAll.ts:74-76). Parity-exempt artifact.
5. `ondemand.zip`: entries named `<archive>.<file>` for archives 1-4, file 0..count-1, skipping nil reads; STORED (level 0). Go: `archive/zip` with `zip.Store` method and a FIXED ModTime (e.g. `time.Unix(0,0)`) — goscape's zip is deterministic even though upstream's isn't; the parity check is content-level (PORTING-EXCEPTION row `rev244-b6-ondemand-zip`).
6. **Signature decision (pre-resolved consumer check):** TS `packAll(modelFlags)` takes an out-param that NO caller reads at the pin (app.ts:28-29, DevThread.ts:24-25, Build.ts:163-166 — all pass a fresh empty array and never read it). Go keeps `PackAll(srcDir, outDir, dataPackDir string) error` and allocates modelFlags internally. Decision row: `rev244-b6-packall-modelflags` (NO-OP at the call boundary; the B1-deferred DevThread row and B3-deferred app.ts packAll row CLOSE here).
7. `updateCompiler()`/BUILD_STARTUP_UPDATE (app.ts:18-20, Build.ts:158-160, RuneScriptCompiler.ts): **NOT-PORTED, platform-inapplicable** — goscape compiles natively, no jar to download. Decision row. The remaining app.ts hunks (createWorker, printError(err.message), uncaughtException-handler removal) are worker/dskit NO-OPs (B5 worker-eval taxonomy).

- [ ] Steps: failing tests (PackAll over the minimal fixture produces main_file_cache.dat/idx0-4 + archive-0 entries 1-8 present per stage + server/build 4 bytes + ondemand.zip with stored entries matching archives 1-4) → RED → implement → `go test ./pkg/packall/... ./cmd/goscape-cli/... -count=1` (smoke_pack stage list updates here too) → commit `feat(packall): 244 orchestration — client cache, build stamp, ondemand.zip; closes B1 DevThread + B3 app.ts deferrals [rev-244 B6]`.

### Task 16: Compiler symbols swap (CompilerSymbols.ts → Go)

**Files:**
- Create: `pkg/pack/compiler/symbols_export.go` (generateCompilerSymbols port — writes `data/symbols/*.sym`)
- Modify: `pkg/pack/compiler/bridge.go` / `symbols.go` (symbol-table construction semantics)
- Test: `pkg/pack/compiler/symbols_export_test.go`

Two halves:

**(a) Port `generateCompilerSymbols` faithfully** (CompilerSymbols.ts full listing — it IS packAll output and the compiler-parity diff anchor; byte-diff vs `Server244-ref/engine/data/symbols/`). Formats (verify each against the numbered listing):
- Simple packs (`constant/npc/obj/seq/idk/spotanim/loc/struct/enum/hunt/mesanim/synth/category/runescript/dbrow.sym`): `<id>\t<name>\n` (some skip empty names, some don't — struct/enum/dbrow iterate raw, mirror exactly).
- `inv.sym` `<id>\t<debugname>`; `writeinv.sym` `<id>\t<debugname>\tnone\t<protect>`.
- `component/interface/overlayinterface.sym` three-way split: name contains `:` → component.sym; `com.overlay` → overlayinterface.sym; else interface.sym; PLUS the "temporary: until compiler updates" rule — overlay entries ALSO duplicated into interface.sym (CompilerSymbols.ts:124-130).
- `varp.sym` `<id>\t<debugname>\t<type>\t<protect>`; `varn/vars.sym` `<id>\t<debugname>\t<type>`; `param.sym` `<id>\t<debugname>\t<paramtype>`.
- `commands.sym` (CompilerSymbols.ts:243-303): `<opcode>\t<name>` then if pointers: `\t<require[,..][:require2,..]|none>\t<[CONDITIONAL:]set[,..][:set2,..]|none>\t<corrupt[,..][:corrupt2,..]|none>`; no-pointer rows end after the name.
- `dbtable.sym`/`dbcolumn.sym`: column key `(table&0xffff)<<12 | (col&0x7f)<<4 [| tuple+1&0xf]`, types comma-joined.
- `stat/npc_stat/npc_mode.sym`: map entries sorted by value, `\n`-joined + trailing `\n`. `locshape.sym`/`fontmetrics.sym`: fixed arrays.

**(b) Re-point the Go compiler's own symbol-table construction** at the same semantics where the OLD Compiler.ts (which bridge.go mirrors) differed — the known deltas: the interface/component/overlay three-way classification (old code fed `interfaceInfo` ALL components; new classification is split + overlay-duplication), and `writeinv` vartype `none`. Diff `bridge.go`'s loading against CompilerSymbols.ts semantics field by field; every change cites the CompilerSymbols.ts lines. (The old Compiler.ts had a real bug — `varnInfo` loop reads `varpInfo.map` (Compiler.ts:427) — check whether goscape inherited it and whether the new .sym path fixes it; record either way.)

- [ ] **Step 1: Failing test** — fixture-driven: pack the minimal fixture, run the exporter, assert exact .sym contents for a curated subset (commands.sym row with require/set/conditional; writeinv row; the component/interface/overlay split + duplication).
- [ ] **Step 2: RED.**
- [ ] **Step 3: Implement exporter + bridge updates.**
- [ ] **Step 4: Real-data check (not yet the gate):** `goscape-cli pack` is not byte-ready yet, but the exporter can run against the REFERENCE's own data/pack inputs — defer the full diff to Task 17; unit tests PASS now.
- [ ] **Step 5: Commit** `feat(pack/compiler): CompilerSymbols port — .sym export + symbol-table semantics rebaseline (TS CompilerSymbols.ts 244) [rev-244 B6]`.

### Task 16.5: Bit-exact gzip — Cloudflare-zlib deflate L6 port (USER DECISION 2026-06-05)

**Discovery (controller, during T12):** the reference cache's gzip streams are
NOT reproducible by Go stdlib flate, stock madler zlib, zlib-ng, or libdeflate
at any level. Bun's `node:zlib.gzipSync` routes to its vendored **Cloudflare
zlib fork** (process.versions zlib = `886098f3f339617b4243b286f5ed364b9989e245`).
Empirically verified: CF-zlib@886098f3 deflate level 6 / memLevel 8 /
Z_DEFAULT_STRATEGY / gzip wrapper / OS-byte zeroed reproduces **4,764/4,764**
ondemand.zip entries byte-identically (the full reference corpus). libdeflate
(also vendored by bun) is a red herring — used by other bun APIs, not
zlib.gzipSync. Stock zlib L6 = 237 bytes vs CF 238 on the probe entry —
CF's match-finder genuinely differs.

**User decision:** bit-exact port (full-tree byte parity preserved; no
content-level gzip exemption).

**References on disk:** `$HOME/Code/github.com/cloudflare/cf-zlib` @
`886098f3` (pin to record in REFERENCES.md at T20); probe harness
`/tmp/claude-1000/cfztest.c` + corpus checker `corpus_check.sh`.

**Files:**
- Create: `pkg/io/gziputil/deflate_cf.go` (+ helpers) — CF-zlib deflate
  level-6 path in pure Go: deflate_slow lazy loop, CF longest_match, CF hash
  (crc-folding hash where output-affecting — CRC-32C is reproducible in
  software), fill_window, trees.c bit emission (lit/dist/code-length huffman,
  block split on flush logic at Z_FINISH for one-shot gzipSync input).
- Modify: `pkg/io/gziputil/gzip.go` — CompressGz routes through the new
  encoder (same signature; stdlib stays for Decompress).
- Test: corpus oracle — env-gated (`GOSCAPE_REF244_DIR`) test recompressing
  every ondemand.zip entry + every client/maps file in the reference and
  asserting byte-equality; plus small deterministic unit fixtures pinning
  known CF-vs-stock divergence cases (the 2.0 probe entry's bytes as
  testdata).

Port discipline: work from the C source at the pin
(`lib/deflate.c`, `trees.c`, `deflate.h`); cite C function/line landmarks in
comments (`// cf-zlib deflate.c:NNNN`) the way TS ports cite TS. The corpus
test is the acceptance gate. One-shot semantics only (gzipSync = single
deflate(Z_FINISH) call) — do NOT port streaming/flush states beyond what
Z_FINISH one-shot exercises, YAGNI.

- [ ] Steps: corpus harness test (RED — stdlib fails it) → port → iterate
  against corpus → GREEN → commit(s).

### Task 17: Phase 2 — determinism, then the full-tree byte-diff loop

Controller-led diagnostic loop (Arc-26 method, ~5-10 min/cycle); subagents for fixes it surfaces. **Strict order.**

- [ ] **Step 1: Determinism gate.** Pack twice, compare:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape-cli pack \
  --src-dir=$HOME/Code/github.com/LostCityRS/Server244-ref/content --out-dir=$TMPDIR/b6-det1
# again into $TMPDIR/b6-det2, then:
diff -r $TMPDIR/b6-det1 $TMPDIR/b6-det2   # expect ONLY server/build (timestamp) and (if mtime not fixed) nothing else
```

Any other diff = nondeterminism: fix FIRST (map iteration → `slices.Sorted(maps.Keys(...))`; the Arc-26 inventory in `docs/superpowers/plans/2026-05-23-pack-determinism.md` lists known-correct sites). One commit per fix with a regression test.

- [ ] **Step 2: Symbols diff (compiler input parity).**

```bash
diff -r $TMPDIR/b6-det1/../symbols-or-wherever data/symbols  # vs $HOME/Code/github.com/LostCityRS/Server244-ref/engine/data/symbols/
```

Iterate Task 16's exporter until `diff -r` is clean. Symbols clean ⇒ the Go compiler and the jar agreed on every input id/name/type/pointer — script.dat deltas after this point are EMISSION deltas.

- [ ] **Step 3: Full-tree diff vs the reference.**

```bash
cd $TMPDIR/b6-det1 && find . -type f ! -name build ! -name ondemand.zip -print0 | sort -z | xargs -0 sha256sum > /tmp/claude/b6-go.sums
diff /tmp/claude/b6-go.sums <(sed 's#data/pack/#./#' .../ref244_manifest.txt | sort -k2)  # adjust paths to taste
```

Work the mismatches file-by-file, easiest-first (configs → jags → cache → maps → script.dat). For each: locate the first divergent byte (`cmp -l | head`), decode both sides with the relevant Go reader or `xxd`, find the responsible stage, fix with a pin test, commit. **script.dat:** if the divergence is structural (different container format, not byte-level slips) — STOP and checkpoint with the user (spec Risk 1). Expect a multi-bug stack; do not batch fixes — one commit each with the diagnostic in the message (Arc-26 #193).

- [ ] **Step 4: ondemand.zip content check** — entry names + per-entry sha vs `ref244_ondemand_entries.txt` (Go test or `unzip -p` loop). PORTING-EXCEPTION `rev244-b6-ondemand-zip` marker lands in packall.go where the zip is written.

- [ ] **Step 5: Parity acceptance test.** Add `pkg/packall/parity_test.go`: env-gated (`GOSCAPE_REF244_DIR` unset → `t.Skip`), packs from `GOSCAPE_REF244_DIR/content` into `t.TempDir()`, hashes the tree (exempting build/ondemand.zip raw), compares against `testdata/ref244_manifest.txt`, and content-checks the zip against `testdata/ref244_ondemand_entries.txt`. Commit `test(packall): full-tree 244 byte-parity acceptance gate [rev-244 B6]`.

### Task 18: Window closures

**Files:**
- Modify: `pkg/objtype/seqtype_test.go` (un-skip `TestLoadSeqTypes_FromPack`), `modules/world/server_wordenc_test.go` (un-skip `TestNewServer_LoadsWordencFilter`), `pkg/objtype/seqtype.go:98` (remove/replace the `rev244-b1-format-window` marker)
- Modify: `PORTING.md` (+ `docs/PORTING-CLOSED.md`)

- [ ] **Step 1:** Un-skip both tests; point their fixtures at 244-format pack data (they may need a packed fixture regenerated by the NEW packer — produce it from the minimal fixture, or the ref cache via env gate; follow each test's existing fixture convention).
- [ ] **Step 2:** Run them + the world suite: `go test ./pkg/objtype/... ./modules/world/... -count=1`. PASS.
- [ ] **Step 3:** PORTING.md closures: `rev244-b1-format-window` (marker out, row → PORTING-CLOSED with the parity-gate citation), B2 map-delivery window row (the 244 cache now feeds REBUILD_GETMAPS/DATA_LAND — verify the world map-delivery path reads the new artifacts in a quick live check during Task 19), B3 midi live-verification row, B4 script.dat numbering window row (closed by Task 17 script.dat parity).
- [ ] **Step 4:** Commit `test+docs: close B1 format / B2 map / B3 midi / B4 script.dat windows at B6 parity [rev-244 B6]`.

### Task 19: End-to-end 244 client login smoke (user-driven)

- [ ] **Step 1:** Install the cache for the world/ondemand modules: run `goscape-cli pack` into goscape's configured `data/pack` (or point config at a packed dir). Confirm CrcTable is non-empty at startup (log line) — empty cache ⇒ every login rejected out-of-date (rev244-b3-crc-compare).
- [ ] **Step 2:** Start `goscape --target all`; ask the USER to run Client-Java `01f16088` against it: login, walk, open a shop/bank, hear midi, hop between two map squares (exercises map delivery + midi + versionlist prefetch).
- [ ] **Step 3:** Triage anything the live client surfaces (get the client debug log EARLY — Arc-29 lesson). Fixes are their own TDD commits.
- [ ] **Step 4:** Record the smoke result in PORTING.md §B6 audit trail (umbrella definition-of-done (b)).

### Task 20: Gates, correspondence audit, handoff

- [ ] **Step 1: Full gates.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... ; \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 ; echo "exit: $?"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./pkg/pack/... ./pkg/packall/... ./modules/world/... -count=1 ; echo "exit: $?"
```

Real exit codes captured; vet failures must be pre-existing-only (compare against a pre-B6 run).

- [ ] **Step 2: PORTING.md §B6 correspondence audit** — every file in the Task-2 scope slice (31 tools/pack files + DevThread.ts + app.ts + RuneScriptCompiler.ts) maps to a commit SHA or decision row (PORT / NO-OP / NOT-PORTED / already-applied-B1). Include: the B1 clientinterface pull-forward (NOT double-applied — audit-listed), jagFileVersion=26 unchanged, `rev244-b6-ondemand-zip` + `rev244-b6-packall-modelflags` rows, the worldmap.jag note (packWorldmap is NOT in packAll — its delta ports with `cmd_worldmap`; parity for `mapview/worldmap.jag` is checked only by the worldmap integration env-gate, record as a row. The Worldmap.ts hunks — packWater commented out, underground-pass level exception removed, +10 floorcol entries, f11-f30 fonts dropped from the jag, jag write-order interleave — port into `pkg/pack/worldmap` in whichever task touches it; if none did, they are a REQUIRED port here before the audit closes).
- [ ] **Step 3: Final whole-bundle integration review** (subagent: read the full `git diff <pre-B6>..HEAD`, the spec, and the audit table; verdict READY/NOT-READY; verify "missing X" findings before acting).
- [ ] **Step 4: B7 handoff** — `docs/superpowers/handoffs/<date>-RESUME-rev244-port-b7.md` (tools/unpack → `goscape-cli unpack`, +3,793 new TS; carry forward: parity test env vars, Server244-ref location, any Task-17 residuals).
- [ ] **Step 5: Commit docs** `docs(porting): B6 audit trail + final gates + B7 handoff [rev-244 B6]`.

---

## Self-review notes (controller, written at plan time)

- **Spec coverage:** Phase 0 → Tasks 1-2; port → Tasks 3-16 (Worldmap.ts explicitly safety-netted in Task 20 Step 2); Phase 2 → Tasks 17-19; gates/audit → Task 20; housekeeping → Tasks 0 and 1 Step 8.
- **Worldmap gap fixed in review:** the Worldmap.ts delta had no dedicated task — Task 20 Step 2 now hard-requires it before the audit closes; implementers hitting `pkg/pack/worldmap` earlier should pull it forward into its own commit.
- **Type consistency:** `modelFlags []int` everywhere; cache handle is `*filestream.FileStream`; stage signatures gain `(cache, modelFlags)` only where TS does.
- **Open contracts deliberately deferred to extraction commands:** per-line Go code for the graphics/maps/versionlist rewrites — the numbered TS listings are the contract; the plan pins formats, orders, archive/file ids, and test fixtures instead of transcribing whole files.
