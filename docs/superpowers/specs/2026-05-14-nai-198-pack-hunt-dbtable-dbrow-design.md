# NAI-198: `.hunt` + `.dbtable` + `.dbrow` packer slice (final config cohort)

**Date**: 2026-05-14
**Predecessor**: NAI-197 (`.seq` + `.flo` + `.spotanim` + `.idk` packer slice; closed at `6713144`)
**Cohort identity**: Final three configs in the `tools/pack/config/*.ts` family. With this slice, the `pkg/pack/` per-config layer is complete (all 18 TS configs ported). Two novel dispatch shapes vs NAI-192..197:
1. **DbTable → DbRow chronological coupling**: TS PackShared.ts:393-414 packs `.dbtable` first, calls `DbTableType.load('data/pack')` to populate the runtime cache, then packs `.dbrow` (which calls `DbTableType.get(value)` at pack time for typed-column schema resolution). Goscape mirrors this by loading `*DbTableTypeConfigs` from the just-written `dbtable.dat` between the two packers.
2. **Hunt's 9-registry fan-out**: largest cross-coupling of any config — `parseHuntConfig` resolves names against `CategoryPack`, `InvPack`, `LocPack`, `NpcPack`, `ObjPack`, `ParamPack`, `VarnPack`, `VarpPack`, plus iterating `HuntPack` itself. All registries already have lazy `ensureFooPack` helpers in `pack_configs.go`.

## 1. Goal

Port the final three per-config packer branches from TS `tools/pack/config/PackShared.ts`:

- `.dbtable` (`PackShared.ts:393-405`)
- `.dbrow` (`PackShared.ts:408-413`, with `DbTableType.load` at `:406`)
- `.hunt` (`PackShared.ts:638-645`)

After this slice:
- A goscape pack run produces three additional `<serverOut>/<type>.{dat,idx}` pairs (`dbtable`, `dbrow`, `hunt`) byte-identical to TS output for equivalent source inputs. All three are server-only (no client jagfile contribution; TS calls `noOp` for the client writer at lines 399/408/639).
- The `pkg/pack/` config layer is complete: 18/18 TS config types ported.
- The integration test extends from `_FifteenConfigsLand` → `_EighteenConfigsLand`.
- A NAI-191 carry-forward decision lands (`LoadFile` nil-vs-empty — see §7.3).

## 2. Scope

**In**:

- Parsers + packers for `.dbtable`, `.dbrow`, `.hunt`
- New `parseCsv` helper in `pkg/pack/` (TS-equivalent: `DbTableConfig.ts:6-26` and `DbRowConfig.ts:7-27` — identical CSV-with-quote-handling routine, currently duplicated in TS). Port once; share between `dbtable.go` and `dbrow.go`.
- Two new lazy registry helpers: `ensureDbTablePack`, `ensureDbRowPack`. `ensureCategoryPack`, `ensureHuntPack`, `ensureLocPack`, `ensureNpcPack`, `ensureObjPack`, `ensureModelPack`, `ensureSeqPack`, `ensureTexturePack`, `ensureSpotAnimPack` are reused. `ensureInvPack` and `ensureParamPack` are NEW lazy promotions (currently `invPack` is inline-only inside the `.inv` branch and `paramPack` is eager at top-of-function — see §9 R2 for the promotion decision). `varpPack`/`varnPack`/`varsPack`/`paramPack` are eager (constructed at top of `PackConfigs`); reused as-is.
- `PackConfigs` re-ordering: insert three new branches in TS-canonical positions (`.dbtable` + `.dbrow` paired, between `.struct` and `.seq`; `.hunt` between `.varp` and `.varn`). The dbtable/dbrow pair runs under a joint freshness gate (TS PackShared.ts:394-397 ORs two `shouldBuild` calls for the two `.dat` files), with a mid-pipeline `LoadDbTableTypes` between them.
- Round-trip tests for all three configs (`pkg/objtype.LoadDbTableTypes`, `LoadDbRowTypes`, `LoadHuntTypes` all confirmed at spec-write — see §9 R3).
- 18-config integration test (renames `TestPackConfigs_FifteenConfigsLand` → `_EighteenConfigsLand`; extends fixture-builder by three configs + three `.pack` registries).
- New deviation-tag pin tests in `nai198_deviation_pins_test.go`.
- NAI-191 carry-forward follow-up #2 decision (`LoadFile` nil-vs-empty): the `.dbtable` and `.dbrow` parsers do not distinguish nil from empty in any new way. Document as no-action-needed in §7.2, retiring the follow-up from the NAI-191 tracker.

**Out**:

- `category.pack → category.dat` writer + `frame_del.dat` writer (TS PackShared.ts:281-389; non-`.<ext>`-source pipelines). Deferred to a "specials slice" — distinct dispatch shape.
- `regenScriptPack` / `.rs2` script-name pack regeneration (TS PackFile.ts:180-191). Defers with the bytecode-compiler arc.
- `validate*` cohort (`validateFilesPack`, `validateImagePack`, `validateConfigPack`, `validateCategoryPack`, `validateInterfacePack`) — deferred per `NAI-191-D-VALIDATE-FLAGS-DEFERRED` and `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`.
- Module-level pack singletons — continues `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`.
- BUILD_VERIFY/CRC validator callbacks — continues `NAI-191-D-VALIDATE-FLAGS-DEFERRED`.
- `PackAll.ts` / `Build.ts` / `Clean.ts` orchestrators — deferred until per-config layer is complete (this slice) AND specials/script layers land.
- NAI-191 follow-ups #1 (`TrimLeft` Unicode narrowing) and #3 (`ShouldBuildFileAny` `ReadDir` failure) — not on this slice's hot paths.

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]` memory)
- TS source: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]` memory). Specifically:
  - `tools/pack/config/PackShared.ts:280-669` (canonical order reference for full `packConfigs` body)
  - `tools/pack/config/HuntConfig.ts:1-545`
  - `tools/pack/config/DbTableConfig.ts:1-224`
  - `tools/pack/config/DbRowConfig.ts:1-185`
  - `tools/pack/PackFile.ts:193-218` (registry declarations — confirms each config's consumer set)
  - `src/engine/entity/hunt/{HuntCheckNotTooStrong,HuntModeType,HuntNobodyNear,HuntVis}.ts` (referenced enums — already ported in goscape `pkg/objtype/hunttype.go`)
  - `src/engine/entity/NpcMode.ts` (referenced enum — already ported in goscape `pkg/objtype/npctype.go` as `NPCMode*`)
  - `src/cache/config/DbTableType.ts` (runtime registry — already ported in goscape `pkg/objtype/dbtabletype.go`)

Per `[[true_to_ts_gate]]` and the NAI-196/197 retrospectives: this spec does NOT codify opcode tables, NPCMode string→enum tables, or HuntConfig opcode tables. Each per-config task block references the TS file + line range and instructs the implementer to read TS directly. Plan-author follows the same discipline.

## 4. Architecture

### 4.1 New files (in `pkg/pack/`)

| File | Contents |
|---|---|
| `csv.go` | `parseCsv(s string) []string` — shared helper, mirrors TS DbTableConfig.ts:6-26 / DbRowConfig.ts:7-27 (identical bodies). Single export, no other contents. |
| `csv_test.go` | Table-driven tests covering: empty string, single field, comma-separated fields, quoted field with embedded comma, escaped-quote edge cases, trailing comma. |
| `hunt.go` | `parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)`, `packHuntConfigs(configs, huntPack) (server *PackedData, err error)`. Server-only — TS returns `{client, server}` but `client` is unused; goscape returns `*PackedData` (server only). |
| `hunt_test.go` | byte-pin tests for `packHuntConfigs` (per opcode branch). |
| `hunt_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadHuntTypes(outDir)` round-trip. |
| `dbtable.go` | `parseDbTableConfig(key, value)` (no registry deps — TS parser at `DbTableConfig.ts:28-76` consults zero PackFiles), `packDbTableConfigs(configs, dbtablePack, lk) (server *PackedData, err error)`. Server-only. Consumes `paramLookups` for typed-default value resolution via `lookupParamValue` (TS line 167-173). |
| `dbtable_test.go` | byte-pin tests covering: empty config (no columns), single-column non-default, single-column with default, multi-column with mixed defaults, INDEXED-without-REQUIRED error path (per `DbTableConfig.ts:116-118`), CLIENTSIDE/REQUIRED/LIST property bit flags (props code 252). |
| `dbtable_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadDbTableTypes(outDir)` round-trip. |
| `dbrow.go` | `parseDbRowConfigFor(dbtablePack)`, `packDbRowConfigs(configs, dbrowPack, dbtableTypes, lk) (server *PackedData, err error)`. Server-only. Consumes the just-loaded `*DbTableTypeConfigs` (NOT `dbtablePack`) for schema lookup at pack time. |
| `dbrow_test.go` | byte-pin tests covering: row with single typed column, row with LIST column having multiple data values, REQUIRED-column-missing-data error path (per `DbRowConfig.ts:141-143`), non-LIST-column-with-multiple-data error path (per `DbRowConfig.ts:145-147`), invalid-data-reference error path (per `DbRowConfig.ts:156-158`). |
| `dbrow_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadDbRowTypes(outDir)` round-trip. |
| `nai198_deviation_pins_test.go` | New presence pins (see §7.4); 0 retirements; 1 NAI-196-D-UNCONDITIONAL-CLIENT-PACK scope-extension pin update (re-asserts that `.hunt`/`.dbtable`/`.dbrow` are NOT in the unconditional-client cohort — they're server-only, gated). |

### 4.2 Modified files

`pkg/pack/pack_configs.go` — body extension:

- Two new lazy `ensureFooPack` helpers (`ensureDbTablePack`, `ensureDbRowPack`) added alongside the twelve existing. Optional further additions per §9 R2: promote `invPack` and `paramPack` from inline/eager to lazy `ensureFooPack` (Hunt needs both at its branch site).
- Three new `packAndSaveHunt` / `packAndSaveDbTable` / `packAndSaveDbRow` functions following the NAI-196/197 `packAndSaveLoc`/`Seq`/`Npc` shape, but with `clientJag` parameter OMITTED (server-only configs).
- Three new branches:
  - `.dbtable` + `.dbrow` (paired): inserted between `.struct` and `.seq` (TS canonical: PackShared.ts:393-414 sits between line 388 `frame_del.save` and line 416 `.enum`; in goscape's ordering this maps to "after `.struct`, before `.seq`"). Gated by joint freshness check matching TS:
    ```
    shouldBuild(scriptsDir, ".dbrow", filepath.Join(serverOut, "dbrow.dat")) ||
    shouldBuild(scriptsDir, ".dbtable", filepath.Join(serverOut, "dbtable.dat"))
    ```
    Within the branch: pack `.dbtable` → call `objtype.LoadDbTableTypes(outDir)` → pack `.dbrow` using the loaded types.
  - `.hunt`: inserted between `.varp` and `.varn` (TS canonical: PackShared.ts:638-645 sits between `.varp` (615-636) and `.varn` (647-654)). Freshness-gated server-only branch matching the existing `.varn`/`.vars` pattern.
- The `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment on `PackConfigs` stays unchanged in scope (the three new branches are server-only-freshness-gated, NOT unconditional-client). The pin in `nai198_deviation_pins_test.go` re-asserts this.
- The `NAI-192-D-NO-SRC-NO-OP` doc-comment scope grows from 6 → 9 server-only freshness-gated branches (`.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars` + new: `.dbtable`-or-`.dbrow` joint, `.hunt`). Plan-author updates the count phrasing per `[[adjacent_doc_paragraph_count_drift]]` — audit ADJACENT paragraphs for stale counts when extending one.

### 4.3 Per-config registry consumer table

| Config | Iterated by | Resolves refs in (parser) | Reads at pack time | New helpers |
|---|---|---|---|---|
| `.dbtable` | `DbTablePack` | (none — parser is registry-free) | `paramLookups` (`lookupParamValue` for typed defaults) | `ensureDbTablePack` |
| `.dbrow` | `DbRowPack` | `DbTablePack` (for `table=NAME` → id) | `*DbTableTypeConfigs` (loaded mid-pipeline), `paramLookups` | `ensureDbRowPack` |
| `.hunt` | `HuntPack` | `CategoryPack`, `InvPack`, `LocPack`, `NpcPack`, `ObjPack`, `ParamPack`, `VarnPack`, `VarpPack` | (none beyond parser) | (none — all 8 + Hunt itself already have helpers) |

Verified at spec-write — see `grep -nE "Pack\.getByName\|Pack\.getById" tools/pack/config/{HuntConfig,DbTableConfig,DbRowConfig}.ts`. Hunt's 8-registry dependency is by FAR the largest; DbTable's parser is unique among 18 configs in resolving zero registries (its packer does call `lookupParamValue` for defaults).

### 4.4 Cross-config coupling: DbTableType cache load

TS pattern (`PackShared.ts:406`):
```
DbTableType.load('data/pack'); // dbrow needs to access it
```

This is a runtime cache populated from the just-written `dbtable.dat`. Goscape equivalent:
```go
dbtableTypes, err := objtype.LoadDbTableTypes(outDir)
if err != nil {
    return fmt.Errorf("LoadDbTableTypes after pack: %w", err)
}
```
Then passed to `packDbRowConfigs(configs, dbrowPack, dbtableTypes, lk)`. Per `[[load_param_types_dir_arg]]`: `LoadDbTableTypes` signature confirmed at spec-write as `func(dir string) (*DbTableTypeConfigs, error)` reading `dir/server/dbtable.dat` — `dir` is parent-of-`server/`, NOT `serverOut/server` itself. Plan-author re-verifies at HEAD before codifying the call.

## 5. Per-config design

For each per-config task, plan-author writes a code-block instructing the implementer to:

1. Read the cited TS file end-to-end.
2. Port the parser. If TS parser has zero registry references (DbTable), the Go function takes no `*PackFile` deps; if it has registry refs (DbRow, Hunt), the Go function returns a closure via `parseFooConfigFor(deps...)`. Returns the per-key parser shape matching `parseLocConfigFor`/`parseNpcConfigFor` (NAI-196).
3. Port the packer as `packFooConfigs(configs, ...) (server *PackedData, err error)` — server-only signature differs from NAI-196/197's `(server, client *PackedData, err error)`. Where TS returns `{client, server}`, goscape returns only the server (the TS client is uniformly empty across all three configs — verified at spec-write: lines 384, 79, 384 of Hunt/DbTable/DbRow respectively allocate `client = new PackedData(...)` and emit `client.next()` per id, no `client.p<N>` calls).
4. Omit dead TS branches per `NAI-195-D-DEADBRANCH-OMITTED`. Verify per config:
   - HuntConfig: `stringKeys: []` (TS line 10) + `booleanKeys: []` (TS line 15) are dead. Apply.
   - DbTableConfig: `stringKeys: []` (TS line 29) + `numberKeys: []` (TS line 30) + `booleanKeys: []` (TS line 31) ALL dead — DbTable parser has no primitive-typed keys. Apply.
   - DbRowConfig: `stringKeys: []` + `numberKeys: []` + `booleanKeys: []` ALL dead. Apply.

### 5.1 `.hunt`

**TS source**: `HuntConfig.ts:1-545` — parser at `:9-381` (372 LOC), packer at `:383-545` (163 LOC).

- **Iterated by**: `HuntPack` (per id in `[0, huntPack.Max())`).
- **Reference registries (8)**: `CategoryPack`, `InvPack`, `LocPack`, `NpcPack`, `ObjPack`, `ParamPack`, `VarnPack`, `VarpPack`.
- **Notable parser branches**:
  - `type` → one of `HuntModeOff/Player/Npc/Obj/Scenery` (per `HuntConfig.ts:58-73`; goscape constants in `pkg/objtype/hunttype.go:14-19`).
  - `check_vis` → `HuntVisOff/LineOfSight/LineOfWalk` (`pkg/objtype/hunttype.go:23-26`).
  - `check_nottoostrong` → `HuntCheckNotTooStrongOff/OutsideWilderness` (`pkg/objtype/hunttype.go:36-38`).
  - `find_newmode` → one of 60+ `NpcMode.*` cases. Goscape constants are `NPCMode*` (all-caps) per `pkg/objtype/npctype.go:43+`. Plan-author writes a per-task code-block instructing implementer to port the TS chain to a Go `switch` statement; do NOT codify the 60-line table in the plan body (per `[[true_to_ts_gate]]`).
  - `check_notcombat` / `check_notcombat_self` → `VarpPack.getByName` / `VarnPack.getByName` (must strip leading `%`).
  - `check_category` / `check_npc` / `check_obj` / `check_loc` → registry name → id.
  - `check_inv` / `check_invparam` / `extracheck_var` → CSV-style multi-field values; return struct types (`HuntCheckInv` / `HuntCheckInvParam` / `HuntCheckVar` from TS PackShared.ts). Goscape introduces three private struct types in `pkg/pack/hunt.go` (or shared `pkg/pack/types.go` if reused — verify at plan-write).
- **Packer opcodes** (per `HuntConfig.ts:383-545`): 1–17 covering each parser branch, plus 18+extracheckVarsCount for repeated `extracheck_var` (max 3, opcodes 18/19/20 — see TS line 519). 250-trailer with `pjstr(debugname)` per id.
- **Mutual exclusion gates**: opcodes 12–17 enforce that `check_category` cannot co-exist with `check_npc`/`check_obj`/`check_loc`/`check_inv`/`check_invparam` (and analogous gates per arm — see TS lines 451-507). Each gate also requires a matching `type=...` to be present. Plan-author code-block instructs implementer to port the `config.every(x => ...)` predicates literally (per `[[asymmetric_predicate_or_chain]]` — TS arms use slightly different predicate sets per opcode; don't refactor to one helper).
- **Default-value emission gates**: many opcodes only emit when value differs from a default (e.g., opcode 7 `nobodynear` only emits when `value !== HuntNobodyNear.PAUSEHUNT` — see TS line 426). Plan-author transcribes per-arm defaults faithfully.
- **TS bug at line 201-202**: `case 'opobj2': return NpcMode.OPOBJ1;` — typo in TS source (should be `OPOBJ2`). Port faithfully per `[[true_to_ts_gate]]`; add NEW deviation tag `NAI-198-D-HUNT-OPOBJ2-TS-BUG` documenting the literal port (mapping `'opobj2'` → `NPCModeOpObj1`) with a TODO to coordinate fix-or-deviate decision with upstream. See §7.3.
- **Server side**: 250-trailer + `pjstr(debugname)` when `debugname.length > 0` (`HuntConfig.ts:535-538`).
- **Client side**: empty per id (`client.next()` only). Server-only — does NOT contribute to `clientJag`.

### 5.2 `.dbtable`

**TS source**: `DbTableConfig.ts:1-224` — parser at `:28-76` (49 LOC), packer at `:78-224` (147 LOC).

- **Iterated by**: `DbTablePack` (per id in `[0, dbtablePack.Max())`).
- **Reference registries (parser)**: NONE (DbTable parser is the ONLY config parser among 18 with zero registry deps).
- **Parser branches**: `column` → returns raw string (unparsed); `default` → returns raw string. (Parse-time deferral — the packer parses CSV at emit time. This is unique among 18 configs.)
- **Packer flow** (per `DbTableConfig.ts:78-224`):
  1. Walk config lines collecting columns (parse CSV via shared `parseCsv`, classify parts as `properties` if UPPERCASE else `types` via `ScriptVarType.getTypeChar` — goscape equivalent in `pkg/objtype/scriptvartype.go` — verify at plan-write).
  2. Walk again collecting defaults (parse CSV; index by column name; resolve typed values via `lookupParamValue`).
  3. Emit three sections per id:
     - Opcode 1: total columns + per-column (flag byte with bit 0x80 for "has default", type-array bytes, optional default-value bytes per typed-field). End with 255.
     - Opcode 251: column-name list (debugname-style `pjstr` per column).
     - Opcode 252: per-column property bits (INDEXED=0x1, REQUIRED=0x2, LIST=0x4, CLIENTSIDE=0x8).
  4. 250-trailer + `pjstr(debugname)` per id.
- **Validation errors** (per `DbTableConfig.ts:116-118, 132-138`): INDEXED-without-REQUIRED, unknown-default-column, default-on-REQUIRED-column. All throw via `packStepError`. Goscape returns error per `[[plan_grep_helper_patterns]]` (existing `packStepError` already exists — see `pkg/pack/config_value.go` or equivalent; plan-author verifies at HEAD).
- **Server side**: opcodes 1/251/252 + 250-trailer.
- **Client side**: empty per id. Server-only.

### 5.3 `.dbrow`

**TS source**: `DbRowConfig.ts:1-185` — parser at `:29-82` (54 LOC), packer at `:84-185` (102 LOC).

- **Iterated by**: `DbRowPack`.
- **Reference registries (parser)**: `DbTablePack` (`table=NAME` → id).
- **Reference data (packer)**: `*DbTableTypeConfigs` from just-loaded `dbtable.dat` — looked up via `table.id` (the parsed `table` value).
- **Parser branches**: `table` → `DbTablePack.getByName` → id; `data` → returns raw string (CSV deferred to packer, like DbTable).
- **Packer flow** (per `DbRowConfig.ts:84-185`):
  1. First pass: find the row's `table` line, resolve to `*DbTableType` via `dbtableTypes.Get(value)` (goscape signature TBD by plan-author — verify `pkg/objtype/dbtabletype.go` exposes `*DbTableTypeConfigs.Get(id) *DbTableType` or equivalent).
  2. Throw `packStepError(debugname, 'No table defined for dbrow')` if absent.
  3. Second pass: collect `data=column,value,value,...` entries via CSV.
  4. Emit opcode 3 (data block): for each table column (in column-index order), emit column-id + type-array + field-count + per-field typed values (`lookupParamValue` per type). Validate REQUIRED-column-must-have-data and non-LIST-column-must-have-≤1-data. End with 255.
  5. Emit opcode 4 + `p2(table.id)`.
  6. 250-trailer + `pjstr(debugname)` per id.
- **Validation errors** (per `DbRowConfig.ts:103, 142, 146, 156-158`): no-table-defined, REQUIRED-column-missing, non-LIST-column-multi-value, invalid-data-reference. All return error.
- **Server side**: opcodes 3 (if data) + 4 (always) + 250-trailer.
- **Client side**: empty per id. Server-only.

## 6. Pipeline integration

Full new branch insertions in `PackConfigs`. Other branches unchanged from NAI-197.

```go
// (existing NAI-196 branches: .param → ParamType load → .enum → .inv → .mesanim → .struct)

// NEW: .dbtable + .dbrow — paired server-only joint freshness-gated.
// TS PackShared.ts:393-414 — joint shouldBuild gate, DbTableType.load between packers.
if GetLatestModified(scriptsDir, ".dbrow") > 0 || GetLatestModified(scriptsDir, ".dbtable") > 0 {
    if ShouldBuild(scriptsDir, ".dbrow", filepath.Join(serverOut, "dbrow.dat")) ||
        ShouldBuild(scriptsDir, ".dbtable", filepath.Join(serverOut, "dbtable.dat")) {
        if err := ensureDbTablePack(); err != nil { return err }
        if err := packAndSaveDbTable(srcDir, serverOut, dbtablePack, lk, constants); err != nil {
            return err
        }

        dbtableTypes, err := objtype.LoadDbTableTypes(outDir)
        if err != nil {
            return fmt.Errorf("LoadDbTableTypes between dbtable/dbrow packers: %w", err)
        }

        if err := ensureDbRowPack(); err != nil { return err }
        if err := packAndSaveDbRow(srcDir, serverOut, dbrowPack, dbtablePack, dbtableTypes, lk, constants); err != nil {
            return err
        }
    }
}

// (existing NAI-197 branches: .seq → .loc → .flo → .spotanim → .npc → .obj → .idk → .varp)

// NEW: .hunt — server-only, freshness-gated.
// TS PackShared.ts:638-645.
if GetLatestModified(scriptsDir, ".hunt") > 0 &&
    ShouldBuild(scriptsDir, ".hunt", filepath.Join(serverOut, "hunt.dat")) {
    if err := ensureCategoryPack(); err != nil { return err }
    if err := ensureHuntPack(); err != nil { return err }
    if err := ensureInvPack(); err != nil { return err }
    if err := ensureLocPack(); err != nil { return err }
    if err := ensureNpcPack(); err != nil { return err }
    if err := ensureObjPack(); err != nil { return err }
    if err := packAndSaveHunt(srcDir, serverOut, categoryPack, huntPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack, lk, constants); err != nil {
        return err
    }
}

// (existing .varn / .vars freshness-gated, then clientJag.Save)
```

Plan-author notes:
- **Joint freshness gate** (`[[plan_geometry_premise_pretrace]]`): TS PackShared.ts:393-397 uses `||` of four `shouldBuild` calls (two scripts-dir-vs-dat-file pairs). Goscape collapses the `tools/pack/config/*.ts`-vs-dat freshness check (always-false because we don't watch our own source — same elision as elsewhere in goscape per `NAI-191-D-VALIDATE-FLAGS-DEFERRED`). The remaining two `shouldBuild` calls become the goscape `||` pair. Plus the `GetLatestModified > 0` per-extension guard mirrors the `.enum`/`.inv`/`.mesanim`/`.struct`/`.varn`/`.vars` pattern (NAI-192-D-NO-SRC-NO-OP) — applied to BOTH `.dbrow` and `.dbtable` via `||`.
- **`ensureInvPack`** doesn't exist yet — `.inv` packing constructs `invPack` inline (see `pack_configs.go:294`). Hunt needs Inv for `check_inv` resolution. Plan-author adds `ensureInvPack` helper as part of T5 wiring task. Same caveat may apply to `ensureParamPack` — `paramPack` is eager (top-of-function). Plan-author verifies all 9 of Hunt's registry deps are accessible at the wiring site.
- **Variable shadowing** (`[[plan_var_name_collision]]`): `packAndSaveHunt`'s 9 `*PackFile` parameters shadow the outer `PackConfigs`-scoped lazy vars; intentional. Mentally compile.
- **Adjacent doc-comment count drift** (`[[adjacent_doc_paragraph_count_drift]]`): `NAI-192-D-NO-SRC-NO-OP` doc-comment phrasing currently says "the six server-only freshness-gated branches" (NAI-197 spec §4.2 confirms). After this slice it becomes nine (six existing + dbtable/dbrow joint + hunt). Audit ADJACENT paragraphs in the same doc-comment block for stale counts/enumerations.

Resulting TS-canonical order across 18 implemented configs:
`.param` → `.enum` → `.inv` → `.mesanim` → `.struct` → **`.dbtable` → `.dbrow`** → `.seq` → `.loc` → `.flo` → `.spotanim` → `.npc` → `.obj` → `.idk` → `.varp` → **`.hunt`** → `.varn` → `.vars`.

## 7. Deviations

### 7.1 Retired (0)

None this slice.

### 7.2 Carryforward (6 — three unchanged, one extended in scope, two effectively-resolved-this-slice)

- `NAI-191-D-VALIDATE-FLAGS-DEFERRED` — unchanged.
- `NAI-191-D-LOADFILEFULL-ERRORS-PROPAGATE` — unchanged.
- `NAI-191-D-NO-PARENT-PORT` — unchanged.
- `NAI-191-D-NO-GLOBAL-SRCDIR` — unchanged.
- `NAI-191-D-CONCURRENCY` — unchanged.
- `NAI-192-D-NO-SRC-NO-OP` — **scope extended in-place** from 6 branches to 9. Doc-comment list grows by `.dbtable`-or-`.dbrow` (joint) and `.hunt`. Plan-author updates the count phrasing AND audits adjacent paragraphs per `[[adjacent_doc_paragraph_count_drift]]`.
- `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` — unchanged. The two new `*PackFile` constructions (dbtable, dbrow) follow the existing lazy per-call pattern.
- `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` — unchanged. Tag is `.param`-specific.
- `NAI-195-D-DEADBRANCH-OMITTED` — applies to all three configs (every parser in this slice has at least one of `stringKeys: []` / `numberKeys: []` / `booleanKeys: []` dead). Verify per config.
- `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` — unchanged. The three new branches are server-only (not unconditional-client); the existing 9-branch doc-comment list (post-NAI-197) is unaffected. Pin in `nai198_deviation_pins_test.go` re-asserts the count remains 9 (no growth into hunt/dbtable/dbrow territory).

**NAI-191 follow-up #2 resolution**: NAI-191's "`LoadFile` returns nil on missing vs TS `[]`" carry-forward — per the NAI-191 spec, "behaviorally identical for all iterating consumers; flag if a future consumer distinguishes nil from empty." The three configs in this slice all use `range over config` patterns (TS `for (let j = 0; j < config.length; j++)`, identical to NAI-192..197), so no nil-vs-empty distinguishing consumer is added. **Decision**: retire this follow-up from the NAI-191 carry-forward list at NAI-198 close (it has now survived 7 slices without surfacing; further deferral is theatre). Document retirement in close memory entry.

### 7.3 New (1)

- **`NAI-198-D-HUNT-OPOBJ2-TS-BUG`** (in `hunt.go` adjacent to the `'opobj2'` case): port the TS bug at `HuntConfig.ts:201-202` faithfully (the string `'opobj2'` maps to `NPCModeOpObj1`, the constant for `'opobj1'`, NOT `NPCModeOpObj2` as a reader would expect). Per `[[true_to_ts_gate]]`, mirror the divergence rather than silently correct it; the tag pins the deviation for future reconciliation with upstream. **Follow-up**: open an upstream issue at LostCityRS/Engine-TS reporting the typo, then either (a) backport their fix and retire the tag, or (b) confirm intentional and re-classify the tag as `goscape-permanent`. Tracked in `[[nai_followups]]` at NAI-198 close.

### 7.4 New presence pin (1 tag-extension)

- **`NAI-192-D-NO-SRC-NO-OP` scope-extension pin**: doc-comment in `pack_configs.go` now lists 9 branches, NOT 6 (the previous count). Pin: regex assertion that the doc-comment matches `nine|9` AND contains all 9 type names (`.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars`, `.dbtable`, `.dbrow`, `.hunt`) as substrings within the doc-block.

### 7.5 New absence pin (1)

- **`NAI-196-D-UNCONDITIONAL-CLIENT-PACK` non-growth pin**: regex assertion that the doc-comment listing the unconditional-client cohort does NOT contain `.hunt`, `.dbtable`, or `.dbrow` (those are server-only). Pin protects against future accidental promotion into the unconditional cohort.

## 8. Tests

### 8.1 Byte-pin tests (one file per config — 3 files)

`hunt_test.go`, `dbtable_test.go`, `dbrow_test.go` — table-driven cases against `packFooConfigs` asserting byte-exact output.

Notable test cases:

- `hunt_test.go`:
  - Each of the 18 opcodes (1–17 plus 18+extracheckVarsCount) gets at least one positive case + at least one "default value, no emit" negative case per `[[ts_asymmetry_dual_pin]]` (e.g., `nobodynear=pausehunt` → no emit; `nobodynear=keephunting` → opcode 7 emit).
  - Mutual exclusion error path: a config with both `check_category` and `check_npc` → packer returns error.
  - `extracheck_var` overflow: 4+ `extracheck_var` lines → error per TS line 519-520.
  - **TS bug pin** (`NAI-198-D-HUNT-OPOBJ2-TS-BUG`): parse `find_newmode=opobj2`, then pack; assert the emitted byte for opcode 6 equals `NPCModeOpObj1` (not `NPCModeOpObj2`). The pin is BOTH the bug-port faithfulness check AND the deviation-tag presence asserter (assert tag appears in the file as a comment).
  - 250-trailer present-vs-absent pin (debugname empty vs non-empty).
- `dbtable_test.go`:
  - Empty config (no columns) → no opcode 1/251/252 emission per TS line 144/181/190 (`if (columns.length)` gate); 250-trailer present iff debugname non-empty.
  - Single-column non-default: opcode 1 with flags `i|0x80=0` (no default), type-byte, end-marker 255 + opcode 251 (column-name list) + opcode 252 (property bits = 0) + 250-trailer.
  - Single-column with default: opcode 1 with `flags=0x80`, type, `1` field, value bytes per type (`pjstr` for STRING via `lookupParamValue`).
  - INDEXED-without-REQUIRED → packer error per TS line 116-118.
  - REQUIRED-with-default → packer error per TS line 136-138.
  - All property bits: column with INDEXED+REQUIRED+LIST+CLIENTSIDE → opcode 252 emits `0x0F`.
- `dbrow_test.go`:
  - Row referencing a table with one column → opcode 3 with `types.length=1`, opcode 4 + `p2(table.id)`, 250-trailer.
  - Row with REQUIRED column missing data → packer error per TS line 142-143.
  - Row with non-LIST column having 2 data values → packer error per TS line 145-147.
  - Row with invalid data reference (`lookupParamValue` returns nil) → packer error per TS line 156-158.
  - No-table-defined → packer error per TS line 102-104.

Per `[[plan_runnable_test_fixtures]]`: every fixture must be mentally executable as written.
Per `[[pin_test_self_trigger_production_doc]]`: the `NAI-198-D-HUNT-OPOBJ2-TS-BUG` pin asserts against production source containing the TS-identifier-bearing comment `'opobj2'`. To avoid pin self-trigger, the test reads the `.go` file as bytes and greps; the test file itself uses the deviation-tag identifier `NAI-198-D-HUNT-OPOBJ2-TS-BUG` but NOT the bare string `opobj2` (or, if needed, splits it across two adjacent string literals to defeat substring grep).

### 8.2 Round-trip tests (3 of 3 configs)

All three configs have runtime loaders in `pkg/objtype/` (verified at spec-write: `dbtabletype.go:155 LoadDbTableTypes`, `dbrowtype.go LoadDbRowTypes`, `hunttype.go` has loader — plan-author re-verifies signatures).

- `hunt_roundtrip_test.go` — sources for `.hunt`; run `PackConfigs`; load via `objtype.LoadHuntTypes(outDir)` (or actual exposed signature); assert per-hunt fields (`Type`, `CheckVis`, `NobodyNear`, `Rate`, `CheckInv`, `ExtraCheckVar` array, etc.). Asserts the TS-bug behaviour: a source with `find_newmode=opobj2` round-trips to `NPCModeOpObj1`.
- `dbtable_roundtrip_test.go` — sources for `.dbtable`; run `PackConfigs`; load via `objtype.LoadDbTableTypes(outDir)`; assert `*DbTableType` for each id has correct `Types`, `ColumnNames`, `Props` arrays, and that `GetDefault(column int)` returns the expected typed values.
- `dbrow_roundtrip_test.go` — sources for both `.dbtable` AND `.dbrow` (DbRow can't round-trip without a defining DbTable); run `PackConfigs`; load via `objtype.LoadDbRowTypes(outDir)`; assert per-dbrow fields including the table-id back-reference.

### 8.3 Integration test

Extend `pack_configs_test.go`'s `TestPackConfigs_FifteenConfigsLand` → `TestPackConfigs_EighteenConfigsLand`:

- Sources for all 18 implemented configs (adds `.dbtable`, `.dbrow`, `.hunt` to the existing 15).
- Required `.pack` registry files. The current `writeEmptyTypedPacks` helper at `pack_configs_test.go:285-290` writes 12 typed packs (`enum, obj, loc, interface, struct, category, spotanim, npc, inv, synth, seq, dbrow`). Add `dbtable` and `hunt` (the existing list already includes `dbrow` — kept consistent for `loadParamLookups`).
- Assert all 18 `<serverOut>/<type>.{dat,idx}` pairs exist.
- Assert client jagfile entry count remains 18 (9 configs × 2 files — `.hunt`/`.dbtable`/`.dbrow` are SERVER-ONLY; no growth). This is a regression guard against accidentally adding the three new configs to the client cohort.
- Assert config server-side emission order: `param → enum → inv → mesanim → struct → dbtable → dbrow → seq → loc → flo → spotanim → npc → obj → idk → varp → hunt → varn → vars`.

**NAI-197 T6 carry-forward**: rename existing `TestPackConfigs_FifteenConfigsLand` atomically to `_EighteenConfigsLand` (single source of truth — matches NAI-196 T5 / NAI-197 T6 pattern). Plan-author chooses atomic rename vs additive new test; recommendation is atomic rename.

### 8.4 Deviation-tag pins (`nai198_deviation_pins_test.go`)

- **Presence pin (1, scope-extension)**: `NAI-192-D-NO-SRC-NO-OP` doc-comment lists 9 branches (regex / substring contains all of: `.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars`, `.dbtable`, `.dbrow`, `.hunt`).
- **Presence pin (1, new tag)**: `NAI-198-D-HUNT-OPOBJ2-TS-BUG` appears as a comment in `pkg/pack/hunt.go` adjacent to the `'opobj2'` case-mapping line.
- **Absence pin (1)**: `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment does NOT list `.hunt`, `.dbtable`, or `.dbrow`.
- **Absence pin (1)**: NAI-191 follow-up #2 (`LoadFile` nil-vs-empty) does NOT appear as an active TODO/deviation in `pkg/pack/parse.go` — retirement confirmation per §7.2.

Per `[[pin_test_self_trigger_production_doc]]`: the OPOBJ2 pin reads `pkg/pack/hunt.go` as bytes and greps for `NAI-198-D-HUNT-OPOBJ2-TS-BUG` only. It does NOT also assert the literal string `'opobj2'` in production source (which would self-trigger when the implementer adds the case-mapping).

## 9. Risk register

| Risk | Likelihood | Mitigation | Verified at spec-write? |
|---|---|---|---|
| R1: `LoadDbTableTypes`/`LoadDbRowTypes`/`LoadHuntTypes` signature drift from NAI-197 expectations | Med | `pkg/objtype/dbtabletype.go:155` confirms `LoadDbTableTypes(dir string) (*DbTableTypeConfigs, error)`. Plan-author re-greps `pkg/objtype/{dbtabletype,dbrowtype,hunttype}.go` at HEAD for current signatures before codifying §6 calls. | ⚠️ plan-author re-verifies |
| R2: `paramPack` is eager (constructed at top of `PackConfigs`), `invPack` is inline-only (only built inside `.inv` branch) | High | Hunt needs both. Plan-author EITHER (a) promotes `invPack` and `paramPack` to lazy `ensureFooPack` helpers as part of T5, OR (b) routes through the existing eager `paramPack` and ensures `invPack` is constructed before Hunt's branch fires. Recommend (a) for parallelism. | ⚠️ plan-author chooses + audits |
| R3: TS bug `opobj2 → OPOBJ1` (HuntConfig.ts:201-202) | High | Port faithfully per `[[true_to_ts_gate]]` with new `NAI-198-D-HUNT-OPOBJ2-TS-BUG` deviation tag. Byte-pin test asserts the TS-faithful behavior; deviation pin asserts the tag comment exists. Follow-up: upstream issue + reconcile. | ✅ verified — see §7.3 |
| R4: `ScriptVarType.getTypeChar` equivalent in goscape | Med | `pkg/objtype/scriptvartype.go` exists per general repo structure (referenced by NAI-179 spec). Plan-author re-greps `pkg/objtype/scriptvartype.go` at HEAD for `getTypeChar` / `GetTypeChar` / equivalent. | ⚠️ plan-author re-verifies |
| R5: `DbTableType.Get(id)` accessor on `*DbTableTypeConfigs` | Med | `pkg/objtype/dbtabletype.go:121` shows `(t *DbTableType) GetDefault`, suggesting per-table methods exist. The plural-configs accessor pattern (e.g., `*DbTableTypeConfigs.Get(id)` or `.Configs[id]`) must be confirmed. Plan-author re-greps `dbtabletype.go` + `dbtableindex.go` for the accessor. | ⚠️ plan-author re-verifies |
| R6: HuntConfig parser's 60-line `find_newmode` switch (NpcMode strings) — naming-convention divergence | Med | TS uses `NpcMode.OPPLAYER1` (SCREAMING) goscape uses `NPCModeOpPlayer1` (Pascal-with-NPC-cap). Plan-author task code-block instructs implementer to port the switch literally; provide a CONCRETE example of one or two mappings but NOT the full table (per `[[true_to_ts_gate]]`). Implementer follows the TS file end-to-end. | ⚠️ plan-author writes per-task code-block reflecting this |
| R7: `parseCsv` already exists somewhere in goscape under a different name | Low | `[[plan_grep_helper_patterns]]`: grep `func parseCsv\|csv\.` in `pkg/pack/` + `pkg/io/` + `pkg/script/` before adding the new `pkg/pack/csv.go`. If it exists, reuse; if it doesn't, the new file is fine. | ⚠️ plan-author greps at HEAD |
| R8: `packStepError` helper exists in `pkg/pack/`? | High | TS uses `packStepError(debugname, message)` for typed errors. Goscape likely has equivalent (used by NAI-194 `.param` packer for similar typed-error reporting). Plan-author greps `func packStepError\|PackStepError` in `pkg/pack/` + `pkg/io/` before codifying. | ⚠️ plan-author greps at HEAD |
| R9: Joint freshness gate semantics — when one of `.dbtable`/`.dbrow` is fresh but the other is not | Med | TS PackShared.ts:393-414 packs BOTH whenever EITHER is dirty (single `if` block). Goscape mirrors. Risk: if `.dbtable` is fresh but `dbrow.dat` doesn't exist yet, the joint gate still fires — `.dbtable` packs (with current data), DbTableType loads, `.dbrow` packs (also with current data). This is correct; the `||` matches TS. Plan-author plus byte-pin test pins the four corner cases (neither dirty, only dbtable dirty, only dbrow dirty, both dirty) → both pack iff either dirty. | ⚠️ plan-author writes 4-case integration test |
| R10: `LoadDbTableTypes` reads `outDir/server/dbtable.dat` — is outDir always parent-of-server? | Med | NAI-195 T7 lesson (`[[load_param_types_dir_arg]]`): plan codified `LoadParamTypes(serverOut)` causing double `/server/` path. Plan-author MUST pass `outDir`, NOT `serverOut` (= `outDir/server`), to `LoadDbTableTypes`. Pre-flight table per `[[plan_runnable_test_fixtures]]`. | ⚠️ plan-author re-verifies |
| R11: HuntConfig parser branches that use `varp` / `varn` references via leading `%` prefix | Low | `check_notcombat` parser strips `%` then calls `VarpPack.getByName`; `check_notcombat_self` strips then calls `VarnPack.getByName`. Goscape mirrors. Plan-author writes per-branch byte-pin test that exercises both `%` prefix paths. | ✅ verified at TS-read |
| R12: Existing pin tests rely on `nai197_deviation_pins_test.go` doc-comment count remaining at "six server-only" | Low | After this slice the count phrasing in `NAI-192-D-NO-SRC-NO-OP` changes to "nine server-only". Plan-author updates the NAI-197 pin's expected substring as part of T6 (atomic doc-comment edit + atomic pin-test edit). | ⚠️ plan-author audits NAI-197 pin file at T6 |
| R13: `pkg/objtype/dbtabletype.go` and `pkg/objtype/dbrowtype.go` Decode methods may have fields not covered by the round-trip assertion set | Med | NAI-152/NAI-179 pattern: assert 2-4 representative fields per round-trip, not exhaustive. Plan-author picks 3 fields per config based on what the byte-pin tests already cover (avoid duplicating assertions). | ⚠️ plan-author picks fields at T6 |
| R14: PackFile registry `Max` semantics for an empty .pack file | Low | Verified at NAI-196 (NAI-196 spec §9 R10): `*PackFile.Max()` returns size+1 matching TS `PackFile.max`. Pattern reused. | ✅ verified |
| R15: NAI-191 follow-up #2 retirement decision premature | Low | The follow-up has survived NAI-192..197 without a distinguishing consumer. The three new configs in NAI-198 also use `range` iteration only. Retiring at NAI-198 close per §7.2 is the right time. If a future config DOES distinguish, re-open as a new tag. | ✅ verified |

Per `[[risk_register_premise_grep]]`: every ⚠️ row MUST be re-verified by the plan author against HEAD before codifying affected task code blocks. Per `[[plan_geometry_premise_pretrace]]`: R9 flags joint-gate semantics; plan-author pre-traces the four corner cases.

## 10. Out-of-scope follow-ups

Tracked for subsequent NAI sub-specs:

- **NAI-199+ "specials slice"**: `category.pack → category.dat` writer + `frame_del.dat` writer (TS PackShared.ts:281-389). Non-`.<ext>`-source pipelines; distinct dispatch shape.
- **NAI-200+ ".rs2 bytecode compiler" arc opener**: ports `tools/pack/Compiler.ts:1-368` (lexer/parser/typechecker scaffolding). Foundational slice opening a multi-sub-spec arc that closes `regenScriptPack` → `script.pack` → `script.dat` end-to-end.
- **NAI-201+ "PackAll wiring"**: ports `tools/pack/PackAll.ts:1-52` end-to-end orchestration (currently goscape's `PackConfigs` covers only the per-config layer). Blocks on specials + bytecode arcs.
- **NAI-202+ "PackFile.ts validate cohort"**: ports `validateFilesPack` / `validateImagePack` / `validateConfigPack` / `validateCategoryPack` / `validateInterfacePack` / `regenScriptPack` from PackFile.ts:11-191. Resolves `NAI-191-D-VALIDATE-FLAGS-DEFERRED` and `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`.
- **NAI-198 follow-up: upstream HuntConfig.ts:201-202 bug**: open issue at LostCityRS/Engine-TS; reconcile `NAI-198-D-HUNT-OPOBJ2-TS-BUG` based on response.
- **Retire NAI-191 follow-up #2** (`LoadFile` nil-vs-empty) at NAI-198 close per §7.2.

## 11. References

- `[[runescript_cadence]]` — applied: this spec is brainstorm → spec; plan + subagent-driven TDD follow.
- `[[true_to_ts_gate]]` — applied: this spec does NOT codify opcode tables, NpcMode string-tables, or the 60+ HuntConfig case branches; per-config task blocks instruct implementers to read TS directly. New `NAI-198-D-HUNT-OPOBJ2-TS-BUG` tag is the canonical example of preserving TS-fidelity over silently-correct-but-divergent behavior.
- `[[ts_source_canonical_path]]` — applied: TS references all point at `LostCityRS/Engine-TS`.
- `[[pin_test_self_trigger_production_doc]]` — applies to §8.4: OPOBJ2 deviation pin reads production source without substring-matching the bare `'opobj2'` literal.
- `[[plan_runnable_test_fixtures]]` — applies to §8.1/§8.2/§8.3 fixtures.
- `[[risk_register_premise_grep]]` — applies to §9 ⚠️ rows.
- `[[dead_param_from_literal_ts_port]]` — applies to §5 dead-branch omissions per-config.
- `[[ts_asymmetry_dual_pin]]` — applies to §8.1 negative-emit cases (e.g., `nobodynear=pausehunt`).
- `[[load_param_types_dir_arg]]` — applies to §6 R10: `LoadDbTableTypes(outDir)` NOT `LoadDbTableTypes(serverOut)`.
- `[[adjacent_doc_paragraph_count_drift]]` — applies to §6/§7.2: when extending `NAI-192-D-NO-SRC-NO-OP` doc-comment from "six" to "nine" branches, audit adjacent paragraphs for stale counts.
- `[[plan_sibling_site_guard_audit]]` — applies to §6: re-grep sibling `ensureFooPack` sites for shared guards before adding 2 new helpers.
- `[[plan_var_name_collision]]` — applies to §6: mentally compile each new `packAndSave*` body, especially Hunt's 9-parameter signature.
- `[[plan_doc_replaceall_timeline]]` — applies to §7.2: doc-comment scope-extension uses per-instance Edits, not `replace_all`.
- `[[asymmetric_predicate_or_chain]]` — applies to §5.1 Hunt mutex predicates: each opcode arm's `config.every(x => ...)` predicate is sliced differently per arm (e.g., opcode 12 `check_category` excludes 5 keys; opcode 16 `check_inv` excludes 5 different keys). DO NOT refactor to a single helper.
- `[[plan_grep_helper_patterns]]` — applies to R7/R8: grep `parseCsv` and `packStepError` before authoring.
- `[[plan_geometry_premise_pretrace]]` — applies to R9: pre-trace joint freshness-gate corner cases.
- `[[spec_library_capability_match]]` — applied to §8.2: all three configs HAVE loaders, so all three get round-trip tests.
- `[[close_commit_memory_trailer]]` — applies at NAI-198 close commit.
- `[[nai_followups]]` — applies at NAI-198 close: add NAI-198 entry with upstream-bug-report follow-up and NAI-191-followup-#2 retirement.

## 12. Task-count estimate

Mirroring NAI-196/197's 8-task shape:

| Task | Scope | Est. commits |
|---|---|---|
| T1 | Shared helpers: `csv.go` + `csv_test.go`; verify `packStepError` exists or add. Add 2 lazy `ensureFooPack` helpers (`ensureDbTablePack`, `ensureDbRowPack`). Optionally promote `invPack`/`paramPack` to lazy helpers per R2. | 1–2 |
| T2 | `.dbtable` parser + packer + byte-pin tests. Stand-alone; no DbRow dependency. | 1–2 |
| T3 | `.dbrow` parser + packer + byte-pin tests. Depends on T2's `*DbTableTypeConfigs` shape. | 1–2 |
| T4 | `.hunt` parser + packer + byte-pin tests (largest single task — 545 TS LOC). | 2 |
| T5 | `PackConfigs` wiring: insert paired `.dbtable`+`.dbrow` branch with mid-pipeline `LoadDbTableTypes` call; insert `.hunt` branch. Atomically rename `TestPackConfigs_FifteenConfigsLand` → `_EighteenConfigsLand` and extend its assertions. Update `NAI-192-D-NO-SRC-NO-OP` doc-comment count phrasing (6 → 9) + sibling doc-paragraph audit. | 1 |
| T6 | Round-trip tests for all three configs. | 1–2 |
| T7 | Deviation-tag pins (`nai198_deviation_pins_test.go`): NAI-192-D scope-extension pin update, NAI-198-D-HUNT-OPOBJ2-TS-BUG presence pin, NAI-196-D non-growth absence pin. Update NAI-197's existing pin for the new "9 branches" wording. | 1 |
| T8 | NAI-191 follow-up #2 retirement: remove from `nai_followups` tracker, document in NAI-198 close memory entry. Add the absence pin (§8.4 bullet 4) to `nai198_deviation_pins_test.go` asserting no active `LoadFile nil-vs-empty` TODO/deviation comment in `pkg/pack/parse.go`. Confirm no production-side code change needed (the `parse.go:15-23` body is unchanged). | 1 |

**Total**: 8 tasks, ~10–13 commits, ~954 LOC TS ported (vs NAI-196's 1389 LOC / NAI-197's 670 LOC — sits between them; Hunt at 545 LOC is the largest single config of the arc, balanced by DbTable/DbRow at 224+185 = small).

After this slice the per-config layer of the pack pipeline is complete: 18/18 TS configs ported. Arc momentum shifts to specials (NAI-199) → bytecode (NAI-200+) → PackAll (NAI-201+) → validate cohort (NAI-202+) → `::rebuild` wiring (final).
