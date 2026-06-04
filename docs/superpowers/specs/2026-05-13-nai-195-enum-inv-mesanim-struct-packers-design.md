# NAI-195: `.enum` + `.inv` + `.mesanim` + `.struct` packer slice

**Date**: 2026-05-13
**Predecessor**: NAI-194 (`.param` packer slice; closed at 23e23c1)
**Cohort identity**: Server-only configs (no client jagfile, no CRC validator). Defers `.hunt` (545 TS LOC; isolated outlier) to NAI-196.

## 1. Goal

Port the next 4 per-config packer branches from TS `tools/pack/config/PackShared.ts:417/425/434/443` into goscape's `pkg/pack/PackConfigs`, so a goscape pack run produces 4 additional server-side `.dat`/`.idx` pairs (`enum`, `inv`, `mesanim`, `struct`) byte-identical to TS output for equivalent source inputs.

## 2. Scope

**In**: parsers + packers for `.enum`, `.inv`, `.mesanim`, `.struct`; orchestrator wiring; runtime `ParamType` registry load between `.param` save and `.struct` parse; pre-construction of `objPack`/`seqPack` name-maps consumed by `.inv` and `.mesanim` parsers; round-trip tests via existing `pkg/objtype.Load{Enum,Inv,Mesanim,Struct}Types` loaders; deviation-tag absence pins.

**Out**: `.hunt` (deferred to NAI-196); `.loc`/`.obj`/`.npc`/`.idk`/`.spotanim`/`.seq`/`.flo` (server+client+CRC family — different slice shape); `.category`/`.frame_del`/`.dbtable`+`.dbrow` (special-cased); BUILD_VERIFY/CRC validator callbacks (continues `VALIDATE-DEFERRED`); module-level pack singletons (continues `PACKFILE-SINGLETONS-DEFERRED`); retirement of `PARAM-AFTER-VARS` (requires `.loc/.obj/.npc` to land first).

## 3. Tech stack

- Go 1.26+ (per `go_version` memory)
- TS source: `LostCityRS/Engine-TS` (per `ts_source_canonical_path` memory). Specifically:
  - `tools/pack/config/PackShared.ts:417-450` (enum/inv/mesanim/struct branch gating)
  - `tools/pack/config/EnumConfig.ts:1-157`
  - `tools/pack/config/InvConfig.ts:1-197`
  - `tools/pack/config/MesAnimConfig.ts:1-92`
  - `tools/pack/config/StructConfig.ts:1-117`

## 4. Architecture

### 4.1 New files (in `pkg/pack/`)

| File | Contents |
|---|---|
| `enum.go` | `parseEnumConfig`, `packEnumConfigs` |
| `enum_test.go` | byte-pin tests for `packEnumConfigs` |
| `enum_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadEnumTypes` round-trip |
| `inv.go` | `parseInvConfigFor(objPack)`, `packInvConfigs` |
| `inv_test.go` | byte-pin tests for `packInvConfigs` (incl. error paths) |
| `inv_roundtrip_test.go` | round-trip via `objtype.LoadInvTypes` |
| `mesanim.go` | `parseMesAnimConfigFor(seqPack)`, `packMesAnimConfigs` |
| `mesanim_test.go` | byte-pin tests for `packMesAnimConfigs` |
| `mesanim_roundtrip_test.go` | round-trip via `objtype.LoadMesanimTypes` |
| `struct.go` | `parseStructConfigFor(paramTypes)`, `packStructConfigs` |
| `struct_test.go` | byte-pin tests for `packStructConfigs` |
| `struct_roundtrip_test.go` | round-trip via `objtype.LoadStructTypes` |
| `nai195_deviation_pins_test.go` | deviation tag presence/absence pins |

### 4.2 Modified file

`pkg/pack/pack_configs.go`:
- 4 new branches in TS-subset order (`enum` → `inv` → `mesanim` → `struct`), each gated on `GetLatestModified > 0` + `ShouldBuild` (matches NAI-194 pattern).
- Lazy construction of `objPack *PackFile`, `seqPack *PackFile`, `paramTypes *objtype.ParamTypeConfigs` — built only when first consumer branch fires.
- ParamType runtime load: after `.param` save (and lazily on first `.struct` consumer otherwise), call `objtype.LoadParamTypes(serverOut)` and thread the registry into `parseStructConfigFor`.
- Branch placement: after `.param`, before `clientJag.Save`. None of the 4 contributes to the client jagfile.
- **Hoist `lk *paramLookups` to `PackConfigs` function scope** (currently declared inside the `.param` `if` block). `.enum` and `.struct` reuse this value via `lookupParamValue`; both branches contain a `lk == nil` fallback that calls `loadParamLookups(srcDir, varpPack)` when `.param` did not rebuild this run.

### 4.3 Parser-closure pattern

Parsers that need name-map or registry context (`.inv`, `.mesanim`, `.struct`) expose factory functions returning the per-key `func(key, value string) (ConfigValue, bool, error)` shape consumed by `ReadTypedConfigs`. The factory captures dependencies in its closure:

```go
func parseInvConfigFor(objPack *PackFile) func(key, value string) (ConfigValue, bool, error) {
    return func(key, value string) (ConfigValue, bool, error) {
        // ... uses objPack.GetByName for stockN ...
    }
}
```

`.enum` does not require a parser closure: its parser pass-throughs `default`/`val` as raw strings; the param resolution happens in `packEnumConfigs` via the existing `lookupParamValue` (which already takes a `*paramLookups`). The `*paramLookups` value built for `.param` packing is reused for `.enum`.

## 5. Per-config design

### 5.1 `.enum`

**Parser** (`parseEnumConfig`):
- `inputtype` / `outputtype` → `objtype.ScriptVarTypeFromName(value)`; reject unknown chars with `ok=false` + `error`.
- `default`, `val` → return raw `string` (resolved at pack time).
- Empty `stringKeys`/`numberKeys`/`booleanKeys` arrays in TS (dead branches) are omitted per `NAI-192-D-DEADBRANCH-OMITTED`.
- Unknown key → `(nil, false, nil)`.

**Packer** (`packEnumConfigs(configs, pf, lk)`):
- Per id in `[0, pf.Max)`:
  - Pre-scan for `inputtype` and `outputtype` (both required for emitting val list). TS `.find(...)!.value` non-null-asserts; goscape returns `packStepError(debugname, "missing inputtype")` / `"missing outputtype"` when absent (no panic).
  - Walk config lines, emit:
    - `inputtype`: opcode 1, then `AUTOINT→INT` collapse before `p1(typeCode)`.
    - `outputtype`: opcode 2, `p1(typeCode)`.
    - `default`: if `outputtype == STRING` opcode 3 + `pjstr(lookupParamValue(...).(string))`; else opcode 4 + `p4(lookupParamValue(...).(int))`.
    - `val`: collect.
  - Val list trailer: opcode 5 (STRING) or 6 (other); `p2(len(val))`; for each val:
    - Key: if `inputtype == AUTOINT` write `p4(i)`; else split on `,`, resolve key via `lookupParamValue(inputtype, keyPart)`, `p4`.
    - Value: if `outputtype == AUTOINT` resolve whole val via `lookupParamValue(outputtype, valStr)`, `p4`; else resolve via `lookupParamValue(outputtype, valStr.after(','))` and `pjstr` (STRING) or `p4`.
  - Trailer (opcode 250 + `pjstr(debugname)`) emitted when `debugname != ""`.
  - `PackedData.Next()`.

### 5.2 `.inv`

**Parser** (`parseInvConfigFor(objPack)`):
- `size`: numeric `[0, 65535]`.
- Booleans: `stackall`, `restock`, `allstock`, `protect`, `runweight`, `dummyinv`.
- `scope`: `shared` → `objtype.InvTypeScopeShared` (2), `perm` → `InvTypeScopePerm` (1), `temp` → `InvTypeScopeTemp` (0).
- `stockN`: parts split on `,`; `parts[0]` → `objPack.GetByName(name)` (reject -1); `parts[1]` → int count; optional `parts[2]` → int respawn. Return `[]int{objId, count[, respawn]}`.
- Unknown key → `(nil, false, nil)`.

**Packer** (`packInvConfigs`):
- Pre-scan for `size`.
- Walk config lines:
  - `scope` → opcode 1 + `p1(value)`.
  - `size` → opcode 2 + `p2(value)`.
  - `stockN` → collect into `stock[index]` where `index = parseInt(key[5:])-1`; on duplicate or `index >= size` → `packStepError(debugname, ...)`.
  - `stackall`/`restock`/`allstock`/`runweight`/`dummyinv` → opcodes 3/5/6/8/9 fire only when value is `true`.
  - `protect` → opcode 7 fires only when value is `false` (TS asymmetry — pin in test).
- Stock-list trailer (when any stock present): opcode 4 + `p1(len(stock))` + per-slot:
  - Hole (`stock[i] == nil`): `p2(-1) + p2(0) + p4(0)`.
  - Present: `p2(id) + p2(count) + p4(respawn or 0)`.
- 250-trailer + `pjstr(debugname)` + `Next()`.

### 5.3 `.mesanim`

**Parser** (`parseMesAnimConfigFor(seqPack)`):
- `len*` keys: value → `seqPack.GetByName(value)` (reject -1).
- Empty `stringKeys`/`numberKeys`/`booleanKeys` arrays in TS — omitted per `NAI-192-D-DEADBRANCH-OMITTED`.
- Unknown key → `(nil, false, nil)`.

**Packer** (`packMesAnimConfigs`):
- Walk config lines:
  - `len*` keys: parse the suffix as int (`strconv.Atoi(key[3:])`); if non-numeric, skip (matches TS `isNaN` continue). Else `opcode = max(0, parsedLen-1)+1`, `p1(opcode) + p2(seqIdx)`.
- 250-trailer + `pjstr(debugname)` + `Next()`.

### 5.4 `.struct`

**Parser** (`parseStructConfigFor(paramTypes)`):
- `param=name,value`: split on first `,`; `paramTypes.ByName(name)` → reject unknown; use `param.Type` to call `lookupParamValue(type, valueStr)`. Return a `ParamValue` struct `{ID, Type, Value}` (define in `pkg/pack/config_value.go` if not already present; `lookupParamValue`-returned `Value` is `any`).
- Empty `stringKeys`/`numberKeys`/`booleanKeys` arrays — omitted per `NAI-192-D-DEADBRANCH-OMITTED`.
- Unknown key → `(nil, false, nil)`.

**Packer** (`packStructConfigs`):
- Per id, collect all `param=` values.
- If any present: opcode 249 + `p1(count)` + per-param:
  - `p3(id) + pbool(type == STRING)` then `pjstr(value)` (STRING) or `p4(value)` (else).
- 250-trailer + `pjstr(debugname)` + `Next()`.

## 6. Pipeline integration

Updated `PackConfigs` skeleton (post-`.param`, pre-`clientJag.Save`):

```go
// after the .param branch:
var (
    objPack    *PackFile
    seqPack    *PackFile
    paramTypes *objtype.ParamTypeConfigs
)

ensureObjPack := func() error {
    if objPack != nil { return nil }
    pf, err := NewPackFile(srcDir, "obj", nil)
    if err != nil { return err }
    objPack = pf
    return nil
}
ensureSeqPack := func() error {
    if seqPack != nil { return nil }
    pf, err := NewPackFile(srcDir, "seq", nil)
    if err != nil { return err }
    seqPack = pf
    return nil
}
ensureParamTypes := func() error {
    if paramTypes != nil { return nil }
    pt, err := objtype.LoadParamTypes(serverOut)
    if err != nil { return fmt.Errorf("load param types: %w", err) }
    paramTypes = pt
    return nil
}

// .enum branch — reuses paramLookups from .param branch (build if .param skipped)
if GetLatestModified(scriptsDir, ".enum") > 0 &&
    ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
    if lk == nil {
        lk, err = loadParamLookups(srcDir, varpPack)
        if err != nil { return err }
    }
    enumPack, err := NewPackFile(srcDir, "enum", nil)
    if err != nil { return err }
    if err := packAndSaveEnum(srcDir, serverOut, enumPack, lk, constants); err != nil {
        return err
    }
}

// .inv branch
if GetLatestModified(scriptsDir, ".inv") > 0 &&
    ShouldBuild(scriptsDir, ".inv", filepath.Join(serverOut, "inv.dat")) {
    if err := ensureObjPack(); err != nil { return err }
    invPack, err := NewPackFile(srcDir, "inv", nil)
    if err != nil { return err }
    if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {
        return err
    }
}

// .mesanim branch
if GetLatestModified(scriptsDir, ".mesanim") > 0 &&
    ShouldBuild(scriptsDir, ".mesanim", filepath.Join(serverOut, "mesanim.dat")) {
    if err := ensureSeqPack(); err != nil { return err }
    mesPack, err := NewPackFile(srcDir, "mesanim", nil)
    if err != nil { return err }
    if err := packAndSaveMesAnim(srcDir, serverOut, mesPack, seqPack, constants); err != nil {
        return err
    }
}

// .struct branch
if GetLatestModified(scriptsDir, ".struct") > 0 &&
    ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
    if err := ensureParamTypes(); err != nil { return err }
    if lk == nil {
        lk, err = loadParamLookups(srcDir, varpPack)
        if err != nil { return err }
    }
    structPack, err := NewPackFile(srcDir, "struct", nil)
    if err != nil { return err }
    if err := packAndSaveStruct(srcDir, serverOut, structPack, paramTypes, lk, constants); err != nil {
        return err
    }
}
```

`lk` (the `*paramLookups` built by `.param` branch) is hoisted to function scope; .enum reuses it for `lookupParamValue` calls in val-list resolution; .struct reuses it through `lookupParamValue` for the value-half of `param=` lines.

## 7. Deviations

### 7.1 Carryforward (unchanged)

- `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`
- `NAI-193-D-VALIDATE-DEFERRED`
- `NAI-193-D-FRESH-CLIENT-JAGFILE`
- `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL`
- `NAI-194-D-PARAM-AFTER-VARS`
- `NAI-192-D-NO-SRC-NO-OP` — applied to all 4 new branches

### 7.2 New

- `NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS` — TS interleaves `.enum`/`.inv`/`.mesanim`/`.struct` BEFORE `.varp` (`PackShared.ts:417/425/434/443`); goscape places all 4 AFTER `.param` (which is itself after `.varp` via `PARAM-AFTER-VARS`). Retire together with `PARAM-AFTER-VARS` when `.loc/.obj/.npc` force a full ordering rewrite.

### 7.3 Reused

- `NAI-192-D-DEADBRANCH-OMITTED` — applied to `.enum`/`.mesanim`/`.struct` parsers (empty `stringKeys`/`numberKeys`/`booleanKeys` arrays in TS).

### 7.4 None needed for ParamType runtime load

TS `ParamType.load('data/pack')` at `PackShared.ts:334` is mirrored by `objtype.LoadParamTypes(serverOut)`. The lazy-load fallback in goscape (only build when `.struct` fires and `.param` did not rebuild this run) is a goscape-internal optimization with identical observable behavior; no deviation tag required.

## 8. Tests

### 8.1 Byte-pin tests (one file per config)

Each `<config>_test.go` runs table-driven cases against `packXxxConfigs` and asserts byte-exact output. Cases enumerated in §5 per-config.

**Per `plan_runnable_test_fixtures` memory**: every fixture is mentally executable as written — no `ParamValue` shorthand without import path, no recorded fixture-only fields.

### 8.2 Round-trip tests (one file per config)

`<config>_roundtrip_test.go` writes source files (`.enum`/`.inv`/`.mesanim`/`.struct` plus any required `.param`/`.obj`/`.seq` for name-map resolution), runs `PackConfigs(srcDir, outDir)`, then loads via `objtype.Load<Type>Types(serverOut)` and asserts source-declared fields survive.

`struct_roundtrip_test.go` is the integration test for the ParamType runtime load: source includes a `.param` with `type=int default=42`, a `.struct` referencing `param=<param-name>,99`; after round-trip the struct's param map contains `{paramID → 99}`.

### 8.3 Integration test

`pack_configs_test.go` adds `TestPackConfigs_EightConfigsLand`: sources for `.varp`/`.varn`/`.vars`/`.param`/`.enum`/`.inv`/`.mesanim`/`.struct` all present; assert all 8 `serverOut/*.{dat,idx}` pairs exist and the client jagfile contains `varp.dat`/`varp.idx`/`param.dat`/`param.idx` only.

### 8.4 Deviation-tag pins

`nai195_deviation_pins_test.go`:
- **Presence pin** for `NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS` doc-comment.
- **Absence pin** for TS-source phrases that would self-trigger (e.g., the phrase `before varp` should NOT appear in production doc-comments per `pin_test_self_trigger_production_doc` memory). The deviation tag references the CONCEPT (`CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS`), not TS identifiers.

## 9. Risk register

| Risk | Likelihood | Mitigation | Verified at spec-write? |
|---|---|---|---|
| `ScriptVarTypeFromName` rejects valid TS-accepted chars | Low | Existing NAI-193 used the same function for `.varn`; coverage proven | ✅ scriptvartype.go:38-40 |
| `InvType.Scope*` constants don't match TS values | Med | TS: SHARED=2, PERM=1, TEMP=0; goscape: `InvTypeScopeShared=2`/`Perm=1`/`Temp=0` | ✅ invtype.go:11-13 |
| `LoadParamTypes(serverOut)` API expects different dir layout | Med | TS calls `ParamType.load('data/pack')`; goscape `LoadParamTypes` reads `<dir>/param.dat` directly | ⚠️ plan-author re-verify path semantics |
| `objPack.GetByName` returns negative for absent | Low | NAI-194 T3 confirmed `-1` sentinel on missing | ✅ packfile.go:192 |
| `paramTypes.ByName` API does not exist as named | Med | API name unverified; loader produces `*ParamTypeConfigs` but lookup signature TBD | ⚠️ plan-author re-grep and codify exact accessor name in T-`struct.go` |
| `ParamValue` struct already defined in `pkg/pack` | Med | NAI-194 T3 introduced `lookupParamValue` returning `any`; no struct named `ParamValue` likely exists | ⚠️ plan-author verifies; introduces struct in `pkg/pack/config_value.go` if absent |
| `pbool` operator on PackedData/Packet missing | Med | `.param` packer used `pbool`-equivalent via `p1(0/1)`; `.struct` needs same | ⚠️ plan-author verifies in packed_data.go |
| `.struct` `param=` value-half resolution requires `objPack`/`enumPack`/etc. | Low | `lookupParamValue` already accepts full `*paramLookups`; reuse `lk` | ✅ see §6 |

Per `risk_register_premise_grep` memory: the ⚠️ rows MUST be re-verified by the plan author against HEAD before codifying the affected task code blocks.

## 10. Out-of-scope follow-ups

- NAI-196: `.hunt` packer slice (545 TS LOC, isolated outlier in this cohort).
- NAI-N+1: first server+client+CRC slice (`.seq` recommended as smallest entry to that family).
- Long-tail: retire `PARAM-AFTER-VARS` + `NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS` together when `.loc/.obj/.npc` land (those packers consume param defaults at parse time and force `.param` before var-domain trio).

## 11. References

- `[[plan_ordering_deviation_preempt]]` — at plan-write, if `.param` ordering needs to flip, codify retire of `PARAM-AFTER-VARS` in spec §10 + verify cross-call-chain state before flipping. Out-of-scope for NAI-195.
- `[[pin_test_self_trigger_production_doc]]` — applies to §8.4 deviation pins.
- `[[plan_runnable_test_fixtures]]` — applies to §8.1/§8.2 fixtures.
- `[[risk_register_premise_grep]]` — applies to §9 ⚠️ rows.
- `[[mock_recorder_field_naming_check]]` — applies if plan introduces test recorders/mocks.
- `[[dead_param_from_literal_ts_port]]` — applies to §5 dead-branch omissions.
