# NAI-196: `.loc` + `.obj` + `.npc` packer slice + TS-canonical ordering rewrite

**Date**: 2026-05-13
**Predecessor**: NAI-195 (`.enum`/`.inv`/`.mesanim`/`.struct` packer slice; closed at `ff80283`)
**Cohort identity**: First server+client+CRC config family — `.loc`, `.obj`, `.npc` all write to both `<serverOut>/<type>.{dat,idx}` AND the shared `<clientOut>/config` jagfile. Forcing function for retiring three accumulated ordering/jagfile deviations.

## 1. Goal

Port the next three per-config packer branches from TS `tools/pack/config/PackShared.ts:477-588` (`.loc`/`.npc`/`.obj`) into goscape's `pkg/pack.PackConfigs`, AND restructure `PackConfigs` to the TS-canonical config order (`PackShared.ts:261-669`). After this slice, a goscape pack run produces three additional server-side `.dat`/`.idx` pairs (`loc`, `npc`, `obj`) byte-identical to TS output for equivalent source inputs, contributes their client-side counterparts to a complete client jagfile (alongside `varp.dat/.idx` and `param.dat/.idx`), and orders all eleven implemented configs identically to TS.

## 2. Scope

**In**:
- Parsers + packers for `.loc`, `.obj`, `.npc`
- Six new lazy `*PackFile` registry helpers (`ensureLocPack`, `ensureNpcPack`, `ensureModelPack`, `ensureCategoryPack`, `ensureHuntPack`, `ensureTexturePack`)
- `PackConfigs` re-ordering to TS-canonical layout for all eleven implemented configs (`.param`/`.enum`/`.inv`/`.mesanim`/`.struct`/`.loc`/`.npc`/`.obj`/`.varp`/`.varn`/`.vars`)
- Eager `objtype.LoadParamTypes(serverOut)` immediately after `.param` save (TS-verbatim; replaces lazy fallback)
- Unconditional execution of all five client+server branches (`.param`, `.loc`, `.npc`, `.obj`, `.varp`) — drops `GetLatestModified > 0 && ShouldBuild` guards (TS-verbatim per `rebuildClient = true`)
- Unconditional `clientJag.Save` at end of `PackConfigs` (drops `clientJagDirty` flag)
- Round-trip tests via existing `pkg/objtype.LoadLocTypes`, `LoadNPCTypes`, `LoadObjTypes`
- 11-config integration test
- Deviation-tag absence pins for the three retired tags + presence pin for one new tag

**Out**:
- TS interleaved specials between `.param` and `.enum`: `category.pack→category.dat` writer, `frame_del.dat` writer (sub-spec deferred — both produce `<serverOut>/<name>.dat` from non-`.<ext>`-source pipelines; structurally distinct)
- `.dbtable`/`.dbrow` (server-only middle group; sub-spec deferred)
- `.seq`/`.flo`/`.spotanim`/`.idk` (client+server middle group; sub-spec deferred)
- `.hunt` (server-only tail; sub-spec deferred — 545 TS LOC isolated outlier)
- BUILD_VERIFY/CRC validator callbacks (continues `NAI-191-D-VALIDATE-FLAGS-DEFERRED`)
- Module-level pack singletons (continues `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`)
- NAI-191 followup #1 (TrimLeft Unicode narrowing) — not hot path for `.loc`/`.obj`/`.npc`; defer to a slice that touches `parse.go`
- NAI-191 followup #3 (`ShouldBuildFileAny` `ReadDir` failure) — not exercised by this slice's pack-flow paths

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]` memory)
- TS source: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]` memory). Specifically:
  - `tools/pack/config/PackShared.ts:261-669` (full `packConfigs` body — canonical order reference)
  - `tools/pack/config/LocConfig.ts:1-434`
  - `tools/pack/config/ObjConfig.ts:1-444`
  - `tools/pack/config/NpcConfig.ts:1-511`
  - `tools/pack/PackFile.ts:193-218` (singleton declarations — confirms which registries each config consumes)

## 4. Architecture

### 4.1 New files (in `pkg/pack/`)

| File | Contents |
|---|---|
| `loc.go` | `parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)`, `packLocConfigs(configs, locPack, modelPack)` |
| `loc_test.go` | byte-pin tests for `packLocConfigs` (per opcode branch) |
| `loc_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadLocTypes` round-trip |
| `obj.go` | `parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)`, `packObjConfigs(configs, objPack)` |
| `obj_test.go` | byte-pin tests for `packObjConfigs` (per opcode branch, incl. `param=`) |
| `obj_roundtrip_test.go` | round-trip via `objtype.LoadObjTypes(serverOut, paramTypes)` |
| `npc.go` | `parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)`, `packNpcConfigs(configs, npcPack)` |
| `npc_test.go` | byte-pin tests for `packNpcConfigs` (per opcode branch, incl. `param=`) |
| `npc_roundtrip_test.go` | round-trip via `objtype.LoadNPCTypes(serverOut)` |
| `nai196_deviation_pins_test.go` | absence pins for 3 retired tags + presence pin for new tag |

### 4.2 Modified file

`pkg/pack/pack_configs.go` — body rewrite:
- TS-canonical branch order (see §6)
- Six new lazy `ensureFooPack` helpers (loc/npc/model/category/hunt/texture); existing `ensureObjPack`/`ensureSeqPack` reused
- Eager `objtype.LoadParamTypes(serverOut)` call immediately after `.param` save block (replaces the lazy `ensureParamTypes` re-entry-guard; helper variable may be retained as a no-op idempotency shim if any later branch still references it, otherwise deleted)
- Five client+server branches (`.param`, `.loc`, `.npc`, `.obj`, `.varp`) drop their `GetLatestModified > 0 && ShouldBuild(...)` gates entirely (TS-verbatim — `rebuildClient = true`)
- Six server-only branches (`.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars`) retain freshness gates (TS uses `shouldBuild(...)` for these in `PackShared.ts:399/408/417/425/434/444/648/657`)
- `clientJagDirty` boolean removed; `clientJag.Save(filepath.Join(clientOut, "config"), false)` runs unconditionally at function tail
- Three new `packAndSaveLoc`/`packAndSaveNpc`/`packAndSaveObj` functions following the existing `packAndSaveVarp`/`packAndSaveParam` shape (signature: `(srcDir, serverOut string, pf *PackFile, /* deps */, c Constants, clientJag *jagfile.Jagfile) error`; body: parse → pack → server save → client jag.Write).

### 4.3 New `objtype.LoadObjTypes` consumer dependency

`obj_roundtrip_test.go` must pre-load a `*objtype.ParamTypeConfigs` (or use the one threaded through `PackConfigs`) and pass it to `LoadObjTypes`. The integration test in §8.3 must arrange the same.

## 5. Per-config design

### 5.1 `.loc`

**Parser** (`parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)`):

Closure-captures six dependencies. Returns the per-key parser shape. Accepted keys (per `LocConfig.ts:1-170`):
- `name` → string
- `desc` → string
- `width` / `length` → `p1` numeric, `[0, 255]`
- `model{N}=<name>` or `model_8/...` (shape suffix variants) → resolve via `modelPack.GetByName(value)`; collect into per-shape slot
- `recol{N}{s,d}` → `p2` numeric
- `retex{N}{s,d}` → `texturePack.GetByName(value)` for `d`; `p2` numeric `s`
- `category` → `categoryPack.GetByName(value)`
- `anim` → `seqPack.GetByName(value)`
- Boolean keys: `blockwalk`, `blockrange`, `active`, `hillskew`, `shadow`, `forcedecor` (mapped per TS table)
- `mapscene`, `mapfunction` → `p1` numeric
- `forceapproach{N}=<dir>` → directional enum
- Op keywords (`op1..op5`) → string array
- `param={name},{value}` → split on first `,`; `paramTypes.ByName(name)`; `lookupParamValue(paramType.Type, valueStr)` via `lk`; return `ParamValue{ID, Type, Value}` — same shape as `.obj`/`.npc`/`.struct` (LocConfig.ts:131-148)
- Unknown → `(nil, false, nil)`

**Packer** (`packLocConfigs(configs, locPack, modelPack)`):

Per id in `[0, locPack.Max())`:
- Emit per-opcode byte stream per `LocConfig.ts:165-432` (opcodes 1, 2, 14, 15, 17, 18, 19, 21, 22, 23, 24, 27, 28, 29, 39, 40, 60, 62, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 77, 78, 79, 81, 82, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 249, 250).
- Shape-suffix model synthesis: when `model{N}` matches per-shape suffix (`LocShapeSuffix` table — see Risk R2), use the synthesized name in `modelPack.GetByName`. Port the suffix table as a Go const-slice in `loc.go`.
- Param trailer (opcode 249): emitted when any `param=` collected, identical shape to `.obj`/`.npc`/`.struct` (LocConfig.ts:406-422).
- 250-trailer + `pjstr(debugname)` when `debugname != ""`.
- `PackedData.Next()`.

LocConfig DOES accept `param=` (verified at spec-write — `LocConfig.ts:131,212` parser-side and `:406-422` packer-side). So `parseLocConfigFor` takes the same `lk *paramLookups` + `paramTypes *objtype.ParamTypeConfigs` as `.obj` and `.npc`.

### 5.2 `.obj`

**Parser** (`parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)`):

Closure-captures six dependencies. Accepted keys (per `ObjConfig.ts:1-200`):
- `name`, `desc` → string
- `model` → `modelPack.GetByName(value)`
- `2dzoom`, `2dxan`, `2dyan`, `2dzan`, `2dxof`, `2dyof` → signed numeric
- `code{N}={objName},...` → multi-part parse, resolves via `objPack.GetByName(parts[0])`
- `recol{N}{s,d}` → `p2`
- `cert` → `objPack.GetByName(value)`
- `category` → `categoryPack.GetByName(value)`
- `manwear`/`womanwear` etc. (anim-tied wear keys) → `seqPack.GetByName(value)` for anim half
- `op{N}` → string array
- `cost`, `weight` → numeric
- `stackable`, `members`, `tradeable` etc. (booleans) per TS table
- `param={name},{value}` → split on first `,`; `paramTypes.ByName(name)`; `lookupParamValue(paramType.Type, valueStr)` via `lk`; return `ParamValue{ID, Type, Value}` (struct already defined in `pkg/pack/config_value.go` per NAI-195 T1)
- Unknown → `(nil, false, nil)`

**Packer** (`packObjConfigs(configs, objPack)`):

Per id in `[0, objPack.Max())`:
- Emit per-opcode byte stream per `ObjConfig.ts:196-440`.
- Cert pairing: when `debugname` starts with `cert_`, look up uncert via `objPack.GetByName(debugname[len("cert_"):])`. When `debugname` does NOT start with `cert_`, look up `cert_<debugname>` via `objPack.GetByName`. Pin the asymmetry via §5.2 test.
- Param trailer (opcode 249): emitted when any `param=` collected. Format: `p1(count) + per-param: p3(id) + pbool(type == STRING) + pjstr(value)` (STRING) or `p4(value)` (else). Identical to `.struct` 249-trailer.
- 250-trailer + `pjstr(debugname)`.

### 5.3 `.npc`

**Parser** (`parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)`):

Closure-captures six dependencies. Accepted keys (per `NpcConfig.ts:1-260`):
- `name`, `desc` → string
- `size` → `p1` numeric `[1, 7]`
- `model{N}` → `modelPack.GetByName(value)`; collect into model slice
- `head{N}` → `modelPack.GetByName(value)`; collect into head slice
- `recol{N}{s,d}` → `p2`
- `wanderrange`, `attackrange`, `maxrange` → `p1`
- `huntmode` → `huntPack.GetByName(value)`
- `category` → `categoryPack.GetByName(value)`
- Anim keys (`readyanim`, `walkanim`, `idlewalkanim`, `defendanim`, etc.) → `seqPack.GetByName(value)`
- Op keys (`op1..op5`) → string
- Stat keys (`att`, `def`, `str`, `hp`, `mage`, `range`) → numeric
- `param=…` → same shape as `.obj` §5.2
- Booleans (`visonmap`, `members`, `aggressive`, etc.) per TS table
- Unknown → `(nil, false, nil)`

**Packer** (`packNpcConfigs(configs, npcPack)`):

Per id in `[0, npcPack.Max())`:
- Emit per-opcode byte stream per `NpcConfig.ts:265-509`.
- Param trailer (opcode 249): identical shape to `.obj`/`.struct`.
- 250-trailer + `pjstr(debugname)`.

## 6. Pipeline integration

Full new `PackConfigs` body (post-eager-construction, after var-uniqueness check):

```go
serverOut := filepath.Join(outDir, "server")
clientOut := filepath.Join(outDir, "client")

clientJag, err := jagfile.NewJagfile(nil)
if err != nil {
    return err
}

// Lazy registries reused across multiple branches.
var (
    lk           *paramLookups
    objPack      *PackFile
    seqPack      *PackFile
    modelPack    *PackFile
    categoryPack *PackFile
    huntPack     *PackFile
    texturePack  *PackFile
    locPack      *PackFile
    npcPack      *PackFile
    paramTypes   *objtype.ParamTypeConfigs
)
// ensureFooPack helpers defined here (one per registry) — see NAI-195 pattern.

// 1. .param — first per TS PackShared.ts:315; unconditional per Q2 ruling
//    (see §9 R5 — TS technically keeps a shouldBuild gate around .param; this spec
//    recommends dropping it for consistency with the Q2 "all client+server branches
//    unconditional" directive, but plan-author must reconcile).
paramPack, err := NewPackFile(srcDir, "param", nil)
if err != nil { return err }
if err := ensureLk(); err != nil { return err }
if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
    return err
}

// 2. Eager ParamType load (TS PackShared.ts:334)
paramTypes, err = objtype.LoadParamTypes(outDir)
if err != nil { return fmt.Errorf("load param types: %w", err) }

// 3. .enum  — server only; freshness-gated (TS uses shouldBuild)
// 4. .inv   — server only; freshness-gated
// 5. .mesanim — server only; freshness-gated
// 6. .struct — server only; freshness-gated
//    (Existing NAI-195 branches, unchanged behavior; reuse lk + paramTypes.)

// 7. .loc — server+client jag; unconditional
if err := ensureLocPack(); err != nil { return err }
if err := ensureModelPack(); err != nil { return err }
if err := ensureCategoryPack(); err != nil { return err }
if err := ensureSeqPack(); err != nil { return err }
if err := ensureTexturePack(); err != nil { return err }
if err := packAndSaveLoc(srcDir, serverOut, locPack, modelPack, categoryPack, seqPack, texturePack, lk, paramTypes, constants, clientJag); err != nil {
    return err
}

// 8. .npc — server+client jag; unconditional
if err := ensureNpcPack(); err != nil { return err }
if err := ensureModelPack(); err != nil { return err }
if err := ensureCategoryPack(); err != nil { return err }
if err := ensureSeqPack(); err != nil { return err }
if err := ensureHuntPack(); err != nil { return err }
if err := packAndSaveNpc(srcDir, serverOut, npcPack, modelPack, categoryPack, seqPack, huntPack, lk, paramTypes, constants, clientJag); err != nil {
    return err
}

// 9. .obj — server+client jag; unconditional
if err := ensureObjPack(); err != nil { return err }
if err := ensureModelPack(); err != nil { return err }
if err := ensureCategoryPack(); err != nil { return err }
if err := ensureSeqPack(); err != nil { return err }
if err := packAndSaveObj(srcDir, serverOut, objPack, modelPack, categoryPack, seqPack, lk, paramTypes, constants, clientJag); err != nil {
    return err
}

// 10. .varp — server+client jag; unconditional (moved from position 1)
if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
    return err
}

// 11. .varn — server only; freshness-gated (moved from position 2)
// 12. .vars — server only; freshness-gated (moved from position 3)
//    (Existing branches retained from NAI-193; gating unchanged.)

// 13. Unconditional client jagfile save (was gated by clientJagDirty)
return clientJag.Save(filepath.Join(clientOut, "config"), false)
```

Plan-author note: `ensureModelPack`/`ensureCategoryPack`/`ensureSeqPack` are called from multiple branches; the lazy guard inside each ensures the underlying `NewPackFile` runs at most once per `PackConfigs` invocation.

## 7. Deviations

### 7.1 Retired (3)

- `NAI-194-D-PARAM-AFTER-VARS` — `.param` now runs first per TS; no longer "after vars".
- `NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS` — accompanies `PARAM-AFTER-VARS`; the four `.enum`/`.inv`/`.mesanim`/`.struct` branches now sit in their TS-canonical position (after `.param`+ParamType.load+specials, before client+server group).
- `NAI-193-D-FRESH-CLIENT-JAGFILE` — client jagfile is now always re-built with all client+server branches every run (TS-verbatim), so the "subset rebuild truncates pre-existing entries" risk no longer exists. Save runs unconditionally.

### 7.2 Carryforward (5 — unchanged)

- `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`
- `NAI-191-D-VALIDATE-FLAGS-DEFERRED`
- `NAI-193-D-PARAM-EMPTY-CLIENT-FAITHFUL`
- `NAI-192-D-NO-SRC-NO-OP` — still applies to the six server-only freshness-gated branches (`.enum`/`.inv`/`.mesanim`/`.struct`/`.varn`/`.vars`). NO LONGER applies to `.param`/`.loc`/`.npc`/`.obj`/`.varp` (unconditional now).
- `NAI-195-D-DEADBRANCH-OMITTED` — applies to `.loc`/`.obj`/`.npc` parsers if any of them carry empty `stringKeys`/`numberKeys`/`booleanKeys` arrays in TS. Plan-author verifies at port time.

### 7.3 New (1)

- `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` — labels each of the five client+server branches (`.param`, `.loc`, `.npc`, `.obj`, `.varp`) at their `packAndSaveFoo` call sites. Documents that they ignore source-file freshness because TS does (`const rebuildClient = true;` at `PackShared.ts:337`). Acts as an anchor for future reconsideration if pack-run performance becomes a concern. Placed as a single doc-comment on `PackConfigs` referencing all five branches, mirroring how `PARAM-AFTER-VARS` was placed.

## 8. Tests

### 8.1 Byte-pin tests (one file per config)

`loc_test.go`, `obj_test.go`, `npc_test.go` — table-driven cases against `packLocConfigs`/`packObjConfigs`/`packNpcConfigs` asserting byte-exact output. Each opcode branch listed in §5 gets at least one positive case. Notable test cases:

- `loc_test.go`: opcode emission per TS LocConfig branches; shape-suffix synthesis; per-shape model slot population; texture refs.
- `obj_test.go`: `param=` round-trip (int and string); cert/uncert pairing asymmetry (per `[[ts_asymmetry_dual_pin]]` memory); op-key array emission.
- `npc_test.go`: `param=` round-trip; `huntPack`-resolved `huntmode`; head-vs-model slot independence.

Per `[[plan_runnable_test_fixtures]]`: every fixture must be mentally executable as written.

### 8.2 Round-trip tests (one file per config)

- `loc_roundtrip_test.go` — sources for `.param` + `.loc` (`.param` required because the new canonical order runs `.param` first AND `parseLocConfigFor` now consumes `lk`+`paramTypes` for `param=` resolution); run `PackConfigs(srcDir, outDir)`; load via `objtype.LoadLocTypes(serverOut)`; assert 2-3 loc-type fields per config including at least one `param=` round-trip.
- `obj_roundtrip_test.go` — sources for `.param` + `.obj`; run `PackConfigs`; load `paramTypes` via `objtype.LoadParamTypes(serverOut)`; pass to `objtype.LoadObjTypes(serverOut, paramTypes)`; assert per-obj `Param` map contains the source-declared entries.
- `npc_roundtrip_test.go` — sources for `.param` + `.npc` + minimal `.hunt`-name registry source (`.hunt.pack` file under `<srcDir>/pack`, NOT a `.hunt` config file — `.hunt` config packer is deferred); run `PackConfigs`; load via `objtype.LoadNPCTypes(serverOut)`; assert per-npc fields including `param=…` resolution and `huntmode`.

### 8.3 Integration test

`pack_configs_test.go` extends to `TestPackConfigs_ElevenConfigsLand`:
- Sources for `.varp`, `.varn`, `.vars`, `.param`, `.enum`, `.inv`, `.mesanim`, `.struct`, `.loc`, `.npc`, `.obj` all present
- Required `.pack` registry files for: varp, varn, vars, param, enum, inv, mesanim, struct, loc, npc, obj, model, category, hunt, texture, seq
- Assert all 11 `serverOut/<type>.{dat,idx}` pairs exist
- Assert client jagfile at `clientOut/config` contains exactly 10 entries: `param.dat/idx`, `loc.dat/idx`, `npc.dat/idx`, `obj.dat/idx`, `varp.dat/idx` (5 configs × 2 files)
- Assert config emission order via a sentinel byte ordering recorded against `clientJag.Write` call order (or via inspection of jagfile entry order if the format preserves it)

**NAI-195 T8 carry-forward**: the existing `TestPackConfigs_EightConfigsLand` (NAI-195 T8) asserts the OLD goscape ordering (varp first, param fourth). T5 of this slice MUST update that test in the same commit to assert the new TS-canonical ordering, then T7 extends to 11 configs. Plan §0 cross-checks both.

### 8.4 Deviation-tag pins

`nai196_deviation_pins_test.go`:
- **Absence pins** (3): `rg "NAI-194-D-PARAM-AFTER-VARS" pkg/` returns 0 hits; `rg "NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS" pkg/` returns 0 hits; `rg "NAI-193-D-FRESH-CLIENT-JAGFILE" pkg/` returns 0 hits.
- **Presence pin** (1): `rg "NAI-196-D-UNCONDITIONAL-CLIENT-PACK" pkg/` returns ≥1 hit.
- **Sanity pin**: `clientJagDirty` identifier no longer appears in `pkg/pack/pack_configs.go` (the bool was the gate for the now-retired FRESH-CLIENT-JAGFILE behavior).

Per `[[pin_test_self_trigger_production_doc]]` memory: the new `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment uses goscape's own terms ("unconditional client pack"), not the TS-side phrase `rebuildClient = true`. The deviation tag references the CONCEPT, not TS identifiers.

## 9. Risk register

| Risk | Likelihood | Mitigation | Verified at spec-write? |
|---|---|---|---|
| R1: All three configs require `lk` + `paramTypes` for `param=` resolution | Low | `LocConfig.ts:131,212`, `ObjConfig.ts:165,262`, `NpcConfig.ts:169,304` — all parser-side; all three packers emit opcode 249. (Spec-write self-review caught the brainstorm-time misdiagnosis of `.loc` as param-free.) | ✅ verified at spec-write |
| R2: `LocShapeSuffix` table values | Med | TS `LocConfig.ts` exports a const array indexed by shape; plan-author must transcribe values verbatim into `loc.go` | ⚠️ plan-author transcribe + pin shape→suffix mapping in `loc_test.go` |
| R3: NAI-195 T8 `TestPackConfigs_EightConfigsLand` ordering assertion | High | This test asserts the OLD order; T5 MUST rewrite it in the same commit as the ordering surgery | ⚠️ plan §0 grep + plan code-block calls out the in-place test update |
| R4: `PackedData(NpcPack.max)` boundary semantics | Med | TS uses `PackFile.max` (size+1); goscape `PackFile.Max()` returns same. Pre-trace at plan-write | ⚠️ plan-author re-grep `packfile.go` for `Max()` semantics |
| R5: `.param` `shouldBuild` gate vs unconditional | Med | TS PackShared.ts:316 wraps `.param` in `if (shouldBuild(...))`. Q2 answer "TS-verbatim" says client+server unconditional. `.param` IS client+server in TS (writes to jag at PackShared.ts:325). But TS DOES gate `.param` on shouldBuild while leaving the rest of client+server group unconditional via `rebuildClient`. Plan-author re-traces and decides whether `.param` is the one exception (keep ShouldBuild) OR also unconditional (preserve consistency under Q2 ruling) | ⚠️ plan-author re-trace TS lines 315-336 + 460+ ; reconcile with Q2 |
| R6: `objtype.LoadObjTypes(dir, ptc)` already exists with this signature | Low | grep confirmed at spec-write | ✅ verified |
| R7: `objtype.LoadNPCTypes` capitalized as `NPC` not `Npc` | Low | grep confirmed; round-trip test uses `LoadNPCTypes` not `LoadNpcTypes` | ✅ verified |
| R8: Required `.pack` registry source files (`model.pack`, `category.pack`, `hunt.pack`, `texture.pack`, `loc.pack`, `npc.pack`) for integration test | Med | Integration test fixture builder must create stub `.pack` files; `NewPackFile` reads from `<srcDir>/pack/<type>.pack` per existing pattern | ⚠️ plan-author confirms `NewPackFile` source path conventions |
| R9: `ParamValue` struct location | Low | NAI-195 T1 introduced in `pkg/pack/config_value.go` | ✅ verified |
| R10: `LocConfig`/`ObjConfig`/`NpcConfig` empty `stringKeys`/`numberKeys`/`booleanKeys` arrays (dead branches) | Med | Per `[[dead_param_from_literal_ts_port]]`, omit if empty — applies `NAI-195-D-DEADBRANCH-OMITTED` | ⚠️ plan-author verifies per-config |

Per `[[risk_register_premise_grep]]`: every ⚠️ row MUST be re-verified by the plan author against HEAD before codifying affected task code blocks.

R5 is the highest-priority unresolved item — Q2's "TS-verbatim unconditional client pack" was the user's directive, but TS itself keeps the `shouldBuild` gate around `.param`. The plan-author must reconcile: option (i) drop `.param`'s ShouldBuild to honor the directive's spirit (treat `.param` as part of the unconditional client+server group); option (ii) keep `.param`'s ShouldBuild to honor TS literally, and narrow `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` scope to the four other client+server branches. This spec recommends (i) for internal consistency with the Q2 directive — plan-author confirms or escalates.

## 10. Out-of-scope follow-ups

Tracked for subsequent NAI sub-specs:

- **NAI-197+ "specials slice"**: `category.pack → category.dat` writer + `frame_del.dat` writer (the two non-`.<ext>`-source pipelines TS interleaves between `.param` and `.enum`). Currently goscape's `PackConfigs` skips both; landing them closes another small gap.
- **NAI-198+ "dbtable/dbrow slice"**: paired server-only configs in the server-only middle group; `.dbrow` depends on `DbTableType.load` runtime registry mirroring the `.struct` ↔ `.param` pattern.
- **NAI-199+ "client+server middle group"**: `.seq`, `.flo`, `.spotanim`, `.idk` packers (per TS PackShared.ts:460-619). All share the same shape as `.loc`/`.npc`/`.obj` (server save + jag.Write + CRC validator). `.seq` recommended first (208 TS LOC, smallest).
- **NAI-200+ "hunt slice"**: `.hunt` server-only packer (545 TS LOC isolated outlier from NAI-195 deferral).
- **Long-tail**: retire `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` once all per-config packers exist (eliminates the per-call `NewPackFile` constructor cost); retire `NAI-191-D-VALIDATE-FLAGS-DEFERRED` once a BUILD_VERIFY-equivalent surface lands.

## 11. References

- `[[plan_ordering_deviation_preempt]]` — applied: this spec codifies the retirement of `PARAM-AFTER-VARS` per NAI-195's earmark.
- `[[pin_test_self_trigger_production_doc]]` — applies to §8.4 deviation pins; new tag uses CONCEPT names not TS phrases.
- `[[plan_runnable_test_fixtures]]` — applies to §8.1/§8.2/§8.3 fixtures.
- `[[risk_register_premise_grep]]` — applies to §9 ⚠️ rows.
- `[[dead_param_from_literal_ts_port]]` — applies to §5 dead-branch omissions per-config.
- `[[ts_asymmetry_dual_pin]]` — applies to §8.1 cert/uncert asymmetry pin in `obj_test.go`.
- `[[load_param_types_dir_arg]]` — applies to §6 / §8.2: `LoadParamTypes(outDir)` arg is the parent of `server/`, not `serverOut` itself.
- `[[mock_recorder_field_naming_check]]` — applies if plan-author introduces test recorders for §8.1.
- `[[plan_geometry_premise_pretrace]]` — applies to §9 R2 (shape-suffix table) if plan flags it RED.
- `[[plan_sibling_site_guard_audit]]` — applies to §6: when adding new `packAndSaveFoo` call sites, re-grep all sibling sites for shared guards.
- `[[plan_var_name_collision]]` — applies to §6: mentally compile each function body to catch `:=` parameter-shadow bugs.
- `[[file_scoped_audits_miss_cross_file_ts]]` — applies to §9 R3: the NAI-195 T8 ordering assertion is cross-file from this slice's primary work but must be updated atomically.
