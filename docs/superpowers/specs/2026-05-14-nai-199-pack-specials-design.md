# NAI-199: specials slice — `category.dat` + `frame_del.dat` writers

**Date**: 2026-05-14
**Predecessor**: NAI-198 (`.hunt` + `.dbtable` + `.dbrow` packer slice; closed at `3040889`; per-config layer 18/18 complete)
**Cohort identity**: The two PackShared writers that sit OUTSIDE the per-config dispatch table (TS `tools/pack/config/PackShared.ts:340-388`). Both are server-only `.dat` outputs with no `.idx` sibling, no client-jagfile contribution, and no per-config `readConfigs(...)` plumbing. They consume already-loaded `PackFile` registries (`CategoryPack`, `AnimPack`) plus — for `frame_del` — raw `.frame` model files. Distinct dispatch shape from NAI-192..198.

**Pre-context correction**: the user's dispatch note framed `frame_del` as "writes to client jag". Audit of `PackShared.ts:354-388` confirms `frame_del.dat` saves to `data/pack/server/frame_del.dat` only (no `jag.write` call). Both specials are server-only. Spec proceeds with TS-faithful server-only routing for both.

## 1. Goal

Port the two specials writers from TS `tools/pack/config/PackShared.ts`:

- `category.dat` writer (`PackShared.ts:340-352`)
- `frame_del.dat` writer (`PackShared.ts:354-388`)

After this slice:
- A goscape pack run produces two additional `<serverOut>/<type>.dat` files (`category.dat`, `frame_del.dat`) byte-identical to TS output for equivalent source inputs. Both are server-only; neither contributes to the client jagfile.
- The 18-configs integration test extends to assert presence of the two additional outputs (20 server-side `.dat` files total when fixtures supply the relevant sources).
- The PackShared port surface advances: per-config layer (NAI-191..198) plus specials writers (this slice) = full TS `packConfigs` body modulo the validate cohort, the `BUILD_VERIFY` callbacks, and the bytecode/PackAll orchestrators (each scope-gated to its own future arc).

## 2. Scope

**In**:

- Two new free functions in a new file `pkg/pack/pack_specials.go`:
  - `packAndSaveCategoryDat(serverOut string, categoryPack *PackFile) error`
  - `packAndSaveFrameDel(srcDir, serverOut string, animPack *PackFile) error`
- Two new branches in `PackConfigs` (TS-canonical positions):
  - Category branch inserted after `LoadParamTypes` and before the `.enum` branch (TS PackShared.ts:341 — sits between `ParamType.load` at :334 and the `.dbtable/.dbrow` pair at :393).
  - Frame_del branch inserted immediately after the category branch (TS PackShared.ts:355).
  - Both reuse existing lazy `ensureCategoryPack` / `ensureAnimPack` helpers (already present in `pack_configs.go` from prior slices; AnimPack already populated for `.seq` and others).
- Byte-pin tests for both writers (small fixtures with hand-computed expected bytes).
- Negative-path tests (missing source: empty registry → `p2(0)`-only output for category; `GetLatestModified == 0` for frame_del → branch skipped).
- Round-trip / integration test: extend the 18-configs integration test to cover the two new outputs.
- One new deviation tag pin test (`nai199_deviation_pins_test.go`).

**Out**:

- Strict re-ordering of the existing `.enum`/`.inv`/`.mesanim`/`.struct`/`.dbtable`/`.dbrow` branches toward TS canonical layout. Current goscape order (`param → enum → inv → mesanim → struct → dbtable/dbrow → ...`) was set in NAI-196/197/198 and is not regressed by NAI-199's insertions. TS canonical for the affected window is `param → category → frame_del → dbtable/dbrow → enum → inv → mesanim → struct → ...`. The category/frame_del insertion point matches TS; the `.dbtable/.dbrow`-vs-`.enum/.inv/.mesanim/.struct` swap is a pre-existing inconsistency unrelated to NAI-199. Leave for a future spec if it ever matters (it does not at run-time — none of these configs cross-reference each other in ways order-sensitive within this window).
- `validate*` cohort (`validateFilesPack`, `validateImagePack`, `validateConfigPack`, `validateCategoryPack`, `validateInterfacePack`) — continues `NAI-191-D-VALIDATE-FLAGS-DEFERRED` / `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`.
- `regenScriptPack` / bytecode compiler / `.rs2` (TS `PackFile.ts:180-191`) — deferred to NAI-200+ bytecode arc.
- `PackAll.ts` / `Build.ts` / `Clean.ts` orchestrators — deferred to NAI-201+.
- BUILD_VERIFY CRC validators — continues `NAI-191-D-VALIDATE-FLAGS-DEFERRED`.
- NAI-198 #1 OPOBJ2 upstream reconciliation — upstream-engagement track, not NAI-199 scope.
- NAI-191 #1 `LoadFile` `TrimLeft` Unicode narrowing — not on either NAI-199 hot path (category uses `PackFile.Load`'s regex-gated line reader; frame_del reads binary via `packet.Load`). Leave deferred.
- NAI-191 #3 `ShouldBuildFileAny` `ReadDir` failure — not used by either special (category uses `ShouldBuildFile`; frame_del uses `ShouldBuild` + `GetLatestModified`). Leave deferred.

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]` memory)
- TS source: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]` memory). Specifically:
  - `tools/pack/config/PackShared.ts:340-388` (specials writers — full body of both)
  - `tools/pack/PackFile.ts:193` (AnimPack declaration: `new PackFile('anim', validateFilesPack, [...models], '.frame', false)`)
  - `tools/pack/PackFile.ts:195` (CategoryPack declaration: `new PackFile('category', validateCategoryPack)`)
  - `tools/pack/PackFileBase.ts:11-141` (PackFile.size = Map.size, PackFile.max = max-id+1; goscape mirrors via `(*PackFile).Size()` returning `len(pf.Pack)` and `.Max` field)
  - `tools/pack/Parse.ts:114-122` (`listFilesExt` — recursive `.frame` scan; goscape mirror is `pack.ListFilesExt`)

Per `[[true_to_ts_gate]]` and prior NAI-19N retrospectives: this spec does NOT codify the `.frame` binary trailer format in prose — the implementer reads TS `PackShared.ts:373-383` directly. The frame-trailer protocol is: last 8 bytes of the file are 4 × u16 BE (head/tran1/tran2/del segment lengths); the first byte of the `del` segment is what gets written to `frame_del.dat`. Plan-author follows the same discipline.

## 4. Non-goals

- Cross-pipeline orchestration (PackAll, watcher loop, etc.).
- Performance: TS reads each `.frame` file once per pack run, no caching. Goscape mirrors. `AnimPack.Max` is typically <50 in fixture; <2000 in full content. No optimization called for.
- Concurrency: TS is single-threaded. Goscape mirrors — both writers run serially within `PackConfigs`.

## 5. Architecture

### 5.1 File layout

```
pkg/pack/
├── pack_configs.go           (modified — two new branches inserted)
├── pack_configs_test.go      (modified — _EighteenConfigsLand → _TwentyConfigsLand)
├── pack_specials.go          (NEW — packAndSaveCategoryDat + packAndSaveFrameDel)
├── pack_specials_test.go     (NEW — byte-pin tests for both writers)
└── nai199_deviation_pins_test.go  (NEW — TS-CODE-STALENESS-GATE tag pin)
```

No new pkg-level helpers. Both writers use existing primitives (`packet.Alloc`, `packet.Load`, `(*Packet).P1/P2/PJStrLF/G1/G2/Save`, `pack.ShouldBuildFile`, `pack.ShouldBuild`, `pack.GetLatestModified`, `pack.ListFilesExt`).

### 5.2 `packAndSaveCategoryDat` (TS PackShared.ts:340-352)

Reads the in-memory `categoryPack *PackFile` (already populated via the existing `ensureCategoryPack` helper); writes `<serverOut>/category.dat`.

Per-byte protocol:
- `p2(categoryPack.Size())` — registered name count (TS `CategoryPack.size`)
- For `i in 0..categoryPack.Size()`:
  - `p1(1)` — record marker
  - `pjstr(categoryPack.GetByID(i))` — LF-terminated name (TS `pjstr` = goscape `PJStrLF`)
  - `p1(0)` — record terminator

Initial allocation: `packet.Alloc(1)` (TS uses `Packet.alloc(1)` = ~5 KB pool). Save via `(*Packet).Save(path, p.Pos, 0)`.

Dense-ID assumption: TS iterates `0..size-1` and calls `getById(i)` — if the source `category.pack` has gaps in ID space, `getById` returns `''` and the record is still emitted (with empty `pjstr`). Goscape mirrors. NOT a deviation — preserves TS behaviour on (rare) sparse category packs.

### 5.3 `packAndSaveFrameDel` (TS PackShared.ts:354-388)

Reads `animPack *PackFile` (in-memory, populated by existing `ensureAnimPack` helper) plus `<srcDir>/models/**/*.frame` files; writes `<serverOut>/frame_del.dat`.

Per-byte protocol:
- Resolve `files := pack.ListFilesExt(<srcDir>/models, ".frame")` once.
- For `i in 0..animPack.Max`:
  - `name := animPack.GetByID(i)`
  - If `name == ""` → `p1(0)`; continue.
  - Find first `f` in `files` ending with `<name>.frame` (TS uses `files.find(f => f.endsWith(...))` — full string match including `/`; goscape mirrors via `strings.HasSuffix(f, "/"+name+".frame")` OR `f == name+".frame"` for the no-parent case — see §9 R2).
  - If no match → `p1(0)`; continue.
  - `p, err := packet.Load(f, false)` — uncompressed binary read. Propagate err.
  - `p.Pos = len(p.Data) - 8`
  - `headLen := p.G2()`; `tran1Len := p.G2()`; `tran2Len := p.G2()` — discard the fourth u16 (del segment length) implicitly by not reading it.
  - `p.Pos = 0; p.Pos += int(headLen) + int(tran1Len) + int(tran2Len)`
  - `frame_del.P1(p.G1())` — first byte of the del segment.

Initial allocation: `packet.Alloc(3)` (TS uses `Packet.alloc(3)` = ~30 KB pool). Save via `(*Packet).Save(path, p.Pos, 0)`.

`AnimPack.Max` source: when `<srcDir>/pack/anim.pack` is absent, `ensureAnimPack` returns a `PackFile` with `Max == 0` (no registered names), and the for-loop body never executes — output is the empty allocation `Pos == 0`, which `(*Packet).Save` writes as a zero-byte file. TS matches: `Packet.alloc(3)` returns an empty packet, the loop runs 0 times because `AnimPack.max == 0`, and `frame_del.save(...)` writes 0 bytes. This is the no-source path.

### 5.4 `PackConfigs` wiring (insertion in `pkg/pack/pack_configs.go`)

Insertion point: between the `paramTypes, err := objtype.LoadParamTypes(outDir)` block (currently lines 320-323) and the `.enum` branch (currently starting line 326).

```go
// .category — server-only special. TS PackShared.ts:341-352.
// Reads <srcDir>/pack/category.pack (already loaded by ensureCategoryPack).
// Gate: ShouldBuildFile on the .pack source. NAI-199-D-TS-CODE-STALENESS-GATE
// drops TS's second arm (`shouldBuild('tools/pack/config', '.ts', ...)`).
if ShouldBuildFile(filepath.Join(srcDir, "pack", "category.pack"), filepath.Join(serverOut, "category.dat")) {
    if err := ensureCategoryPack(); err != nil {
        return err
    }
    if err := packAndSaveCategoryDat(serverOut, categoryPack); err != nil {
        return err
    }
}

// .frame_del — server-only special. TS PackShared.ts:355-388.
// Reads AnimPack + <srcDir>/models/**/*.frame. Server-only.
// Gate: ShouldBuild + GetLatestModified >0 (NAI-192-D-NO-SRC-NO-OP).
// NAI-199-D-TS-CODE-STALENESS-GATE drops TS's second arm.
if GetLatestModified(filepath.Join(srcDir, "models"), ".frame") > 0 &&
    ShouldBuild(filepath.Join(srcDir, "models"), ".frame", filepath.Join(serverOut, "frame_del.dat")) {
    if err := ensureAnimPack(); err != nil {
        return err
    }
    if err := packAndSaveFrameDel(srcDir, serverOut, animPack); err != nil {
        return err
    }
}
```

`ensureCategoryPack` and `ensureAnimPack` already exist in `pack_configs.go` (lines 182-225). No new lazy helpers needed.

### 5.5 Data flow

```
            ┌───────────────────────────────────────┐
            │  PackConfigs (pkg/pack/pack_configs.go)│
            └────────────────┬──────────────────────┘
                             │
              ┌──────────────┴───────────────┐
              ▼                              ▼
  ensureCategoryPack()          ensureAnimPack()
   loads <src>/pack/             loads <src>/pack/
   category.pack                 anim.pack
              │                              │
              ▼                              ▼
  packAndSaveCategoryDat        packAndSaveFrameDel
              │                              │
              │                              ├─ ListFilesExt(<src>/models, ".frame")
              │                              ├─ for i in 0..AnimPack.Max:
              │                              │    packet.Load(f, false)
              │                              │    read trailer, write del byte
              ▼                              ▼
   write <serverOut>/             write <serverOut>/
   category.dat                   frame_del.dat
```

## 6. Error handling

Per `[[true_to_ts_gate]]` and consistent with the per-config layer: parse / pack functions propagate errors via Go's standard `error` return. No panics on bad inputs. Specifically:

- `packet.Load` failure on a `.frame` file → propagated.
- Truncated `.frame` (`len < 8`) → `p.G2()` panic per `pkg/io/packet` semantics. Acceptable: TS also throws on out-of-bounds reads (uncaught). Not a deviation — both runtimes treat malformed input as a fatal pack error.
- Missing `<srcDir>/pack/category.pack` → `ensureCategoryPack` constructs an empty `PackFile` (default `Load` returns nil on missing file). `categoryPack.Size() == 0` → writer emits `p2(0)` only. TS matches.
- Missing `<srcDir>/pack/anim.pack` → same as above; `animPack.Max == 0` → writer emits 0 bytes. TS matches.
- Empty `category.pack` file → same as missing.

## 7. Testing

### 7.1 `packAndSaveCategoryDat` byte-pin test

Fixture:
- `<src>/pack/category.pack` containing `0=alpha\n1=bravo\n2=charlie\n` (3 dense entries).
- Call `ensureCategoryPack()`-equivalent (`NewPackFile(src, "category", nil)`).
- Call `packAndSaveCategoryDat(<serverOut>, categoryPack)`.

Expected `<serverOut>/category.dat` bytes (hex):
```
00 03                                  // p2(3)
01 'a' 'l' 'p' 'h' 'a' 0a 00          // record 0: marker, pjstr(alpha)+LF, terminator
01 'b' 'r' 'a' 'v' 'o' 0a 00          // record 1
01 'c' 'h' 'a' 'r' 'l' 'i' 'e' 0a 00  // record 2
```

Total: 2 + 8 + 8 + 11 = 29 bytes. Assert exact byte equality.

(`PJStrLF` writes the string raw followed by `\n` (`0x0a`). Per `pkg/io/packet/packet.go:395`.)

### 7.2 `packAndSaveCategoryDat` empty-source test

Fixture: empty `categoryPack` (no `<src>/pack/category.pack` file).
Expected `<serverOut>/category.dat`: `00 00` (just `p2(0)`).

### 7.3 `packAndSaveFrameDel` byte-pin test

Fixture:
- `<src>/pack/anim.pack`: `0=foo\n2=bar\n` (ID 1 is a gap; Max = 3).
- `<src>/models/foo.frame`: synthetic 32-byte body with hand-crafted trailer.
  - `head[0..5]` (6 bytes): `aa bb cc dd ee ff`
  - `tran1[0..3]` (4 bytes): `11 22 33 44`
  - `tran2[0..1]` (2 bytes): `55 66`
  - `del[0..N-1]` (N bytes, N>=1): first byte `0x42` followed by `0x99 0x99 ... 0x99` (need at least 1 del byte). Let N = 12 so total before trailer = 6+4+2+12 = 24, plus trailer 8 = 32.
  - trailer: `00 06 00 04 00 02 00 0c` (headLen=6, tran1Len=4, tran2Len=2, delLen=12)
- No `<src>/models/bar.frame` (bar will fall through to "file not found" arm).

Expected `<serverOut>/frame_del.dat` bytes (hex):
```
42 00 00
```
- `42`: foo's del[0] byte
- `00`: ID 1 (gap, empty name → p1(0))
- `00`: bar (no .frame file → p1(0))

Total: 3 bytes. Assert exact byte equality.

### 7.4 `packAndSaveFrameDel` no-models-dir test

Fixture: `<src>` with no `models/` subdir.
Expected: `GetLatestModified(<src>/models, ".frame") == 0` → branch skipped in `PackConfigs` (NOT in the per-function test — this is wiring behaviour). Tested at the `PackConfigs` integration level by asserting `<serverOut>/frame_del.dat` does NOT exist when no `.frame` sources are supplied.

### 7.5 `PackConfigs` integration test extension

Modify `pack_configs_test.go::TestPackConfigs_EighteenConfigsLand` (current name from NAI-198) → `_TwentyConfigsLand`:
- Add `<src>/pack/category.pack` to fixture-builder with N entries.
- Add `<src>/pack/anim.pack` + ≥1 `<src>/models/<name>.frame` file (synthetic minimal valid trailer).
- Assert two new outputs land: `<out>/server/category.dat`, `<out>/server/frame_del.dat`.
- Existing assertions for the 18 prior outputs unchanged.

### 7.6 Deviation tag pin test (`nai199_deviation_pins_test.go`)

Mirror the pattern from `nai192_deviation_pins_test.go` and `nai194_deviation_pins_test.go`:
- `TestNAI199Deviation_TSCodeStalenessGate_Pinned`: grep `pkg/pack/` and `modules/` for the string `NAI-199-D-TS-CODE-STALENESS-GATE`; assert ≥2 matches (one per writer's doc-comment in `pack_configs.go`). Optionally also pin a non-zero count for the rationale comment in `pack_specials.go`.

Per `[[pin_test_self_trigger_production_doc]]`: pin matches the tag identifier only, not the literal phrase "TS source staleness" or similar — to avoid self-trigger on adjacent doc text.

## 8. Open questions

Resolved at spec-write — no open items pending. See §9.

## 9. Resolved risks

**R1 — Pre-context "frame_del writes to client jag" vs TS reality**
*Risk*: dispatch note framed frame_del as client-jag-bound. If the implementer trusts pre-context without re-reading TS, client jag entries leak.
*Resolution*: TS PackShared.ts:386 calls `frame_del.save('data/pack/server/frame_del.dat')` only. No `jag.write` for frame_del anywhere in PackShared.ts. Confirmed at spec-write by full re-read of lines 354-388. Spec explicitly notes the correction in the cohort identity (top). Per `[[true_to_ts_gate]]` — server-only is the TS-faithful answer.

**R2 — `.frame` file-match string form: full path suffix vs basename**
*Risk*: TS `files.find(f => f.endsWith(`${name}.frame`))` — `name.frame` as a suffix would falsely match `xname.frame` (e.g. `name="oo"` matching `foo.frame`).
*Resolution*: TS HAS this bug — `endsWith('foo.frame')` matches `bigfoo.frame`. Per `[[true_to_ts_gate]]`, goscape mirrors literally: use `strings.HasSuffix(f, name+".frame")` (no leading `/`). Document as `NAI-199-D-FRAME-SUFFIX-MATCH-TS-PARITY`? — No: this is too speculative as a deviation tag (the bug would only surface with carefully crafted anim names sharing a suffix substring of a real frame file). Pre-empt via a parser doc-comment referencing TS line 365 and noting "endsWith suffix-match per TS" without a formal tag. If `[[true_to_ts_gate]]` requires a tag for every literal-port-of-a-bug, escalate to tag; otherwise inline-comment-only. Spec resolves to inline-comment-only at plan-author discretion. Per `[[plan_author_ordering_deviation_preempt]]` and `[[emergent_deviation_mid_impl]]`: if the implementer feels this rises to deviation-tag level, flag at review.

**R3 — TS source-staleness gate (second arm of TS shouldBuild)**
*Risk*: TS's second arm `shouldBuild('tools/pack/config', '.ts', dest)` rebuilds when TS pipeline source files are newer than output. Goscape has no equivalent (Go source mtime isn't a content-change signal at runtime; the binary is the source of truth for runtime).
*Resolution*: Drop the second arm. Pin via `NAI-199-D-TS-CODE-STALENESS-GATE` doc-comment in both branches. Same shape as `NAI-191-D-VALIDATE-FLAGS-DEFERRED` and others — TS-only gate semantics that don't translate.

**R4 — `AnimPack` already loaded by upstream branches (`.seq`, `.spotanim`, `.idk`)?**
*Risk*: if AnimPack must be loaded EARLIER than the `.seq` branch (which is currently the first goscape consumer), inserting frame_del between `LoadParamTypes` and `.enum` might trigger `ensureAnimPack` for the first time. Verify ensureAnimPack works at that position.
*Resolution*: confirmed via `pack_configs.go:215-225` — `ensureAnimPack` is a pure lazy `NewPackFile(srcDir, "anim", nil)` call with no dependencies on prior branches. Safe to call at any position. AnimPack will be FIRST consumed by frame_del at the new insertion point, but the lazy-init invariant is unchanged.

**R5 — Order: should `.dbtable/.dbrow` be moved to TS-canonical position (before `.enum`)?**
*Risk*: NAI-199 doesn't fix it; future spec might. Sticking with current goscape order leaves a subtle TS divergence.
*Resolution*: Out of scope per §2. The pre-existing order divergence does not affect any cross-config dependency at run-time (none of these configs cross-reference each other in ways order-sensitive within this window — `.dbrow` depends on `.dbtable` only, both via the mid-pipeline `LoadDbTableTypes` call; the enum/inv/mesanim/struct group doesn't consume dbtable/dbrow outputs at pack time). Document via a small note in the cohort identity. If a future arc requires strict TS order, a separate spec lands it.

**R6 — `packet.Save(path, length, start)` signature parity**
*Risk*: `(*Packet).Save` takes three args, not just a path. Verify usage shape.
*Resolution*: confirmed via `pkg/io/packet/packet.go:108` — signature is `(p *Packet) Save(filePath string, length int, start int)`. Pass `p.Pos, 0` to write everything from the start up to the write cursor. The doc-comment on `Save` (line 110) confirms default length = `p.Pos`. Both writers will call `p.Save(<path>, p.Pos, 0)`.

**R7 — Should `packAndSaveFrameDel` factor out the `.frame` trailer reader to a helper?**
*Risk*: dead-API YAGNI per `[[dead_api_polish]]` — if no other caller uses the trailer parser, keep it inline.
*Resolution*: keep inline. The trailer parse is 4 lines; no other call site in `pkg/pack/`. If a future model-frame-format consumer lands (NAI-2NN+ model converter?), extract then.

**R8 — Test fixture: synthetic `.frame` body is contrived; does it matter?**
*Risk*: byte-pin test asserts goscape's parser behaviour against a known-good trailer; doesn't validate against a real RuneScape `.frame` file.
*Resolution*: synthetic is appropriate for unit-test scope per `[[rsbuf_roundtrip_tests]]` (decode in canonical reader order). Real-frame validation lands at smoke-time post-close per `[[cascade_theory_smoke_binding]]`. The byte-pin proves the GO parser reproduces the TS algorithm; a follow-up smoke would validate against real `.frame` files when content is in place.

## 10. Deviations enumerated

- **`NAI-199-D-TS-CODE-STALENESS-GATE`**: both branches drop TS's `shouldBuild('tools/pack/config', '.ts', dest)` second-arm gate. Rationale: TS-only semantic (rebuilds when TS pipeline source changes); doesn't translate to a compiled Go binary where source mtime is irrelevant at runtime. Pinned by deviation-tag grep in `nai199_deviation_pins_test.go`.

(Optional, plan-author discretion per R2: `NAI-199-D-FRAME-SUFFIX-MATCH-TS-PARITY` if the implementer judges the literal `endsWith` suffix-match warrants a formal tag rather than an inline comment. Default: inline-only.)

## 11. Carry-forward (from prior NAI sub-specs)

Per `[[nai_followups]]` audit at spec-write:
- **NAI-191 #1 `LoadFileFull` `TrimLeft` Unicode narrowing** — not on either NAI-199 hot path (category uses `PackFile.Load` regex-gated line reader; frame_del reads binary). Leave deferred.
- **NAI-191 #3 `ShouldBuildFileAny` `ReadDir` failure returns false** — not used by either special. Leave deferred.
- **NAI-198 #1 OPOBJ2 upstream reconciliation** — upstream-engagement track. Out of NAI-199 scope.

No new NAI-199-bound carry-forwards introduced.

## 12. Arc next step

After NAI-199 close, the per-config + specials layer of PackShared is fully ported. Remaining PackShared-adjacent work:
- **NAI-200+**: bytecode compiler arc (`PackScript.ts`, `.rs2` opcode tables, `regenScriptPack`).
- **NAI-201+**: `PackAll.ts` / `Build.ts` / `Clean.ts` orchestrators — full pack-CLI parity.
- **NAI-202+**: validate cohort (`validateFilesPack`, `validateImagePack`, `validateConfigPack`, `validateCategoryPack`, `validateInterfacePack`) + `BUILD_VERIFY` CRC callbacks.

NAI-199 unblocks NAI-201 (PackAll needs the specials to land before it can be a TS-equivalent driver). NAI-200 (bytecode) is independent and could land in parallel.
