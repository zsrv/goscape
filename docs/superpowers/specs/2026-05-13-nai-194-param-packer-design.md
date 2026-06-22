# NAI-194 — .param packer slice

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/tools/pack/config/ParamConfig.ts` (~190 LOC body); `tools/pack/PackShared.ts` `packConfigs` param branch (continuation of varp/varn/vars pattern); `src/cache/config/ParamType.ts:64` (`autodisable=true` default).
**Predecessors:** NAI-191 (pack-pipeline foundation), NAI-192 (varn + vars), NAI-193 (.varp packer + jagfile writer fix + var-domain uniqueness check).
**HEAD at spec-write:** `2df5078`.

## §1 Goal

Port `tools/pack/config/ParamConfig.ts` onto the NAI-192/193 PackShared infrastructure. **First packer with cross-domain `*PackFile` typed-id lookups for default-value resolution** — establishes the multi-PackFile-in-`PackConfigs` pattern that NAI-195+ (.loc, .obj, .npc, .seq, .spotanim, .enum, .struct, .inv, .mesanim, .flo, .hunt, .idk, .dbtable, .dbrow) will inherit.

Dual-output, TS-faithful: server `.dat`/`.idx` written under `<outDir>/server/`; an **empty `param.dat`/`param.idx`** entry written into the client jagfile (TS `packParamConfigs` initializes a `client` PackedData but never `.p1()`'s into it — only `.next()` is called per slot, producing `p2(count) + count×0x00`).

Same slice fixes a pre-existing loader-side default-value bug in `pkg/objtype/paramtype.go`: `NewParamType` initializes `AutoDisable = false` (Go zero), but TS `ParamType.autodisable = true`. The new packer surfaces it via the round-trip test — fixing here is on the slice's critical path.

## §2 Out of scope

| Concern | TS location | Why deferred | Tag |
|---|---|---|---|
| `ParamPack` / 13 typed-id `*Pack` module-level singletons | `tools/pack/PackFile.ts:231-256` | Continuation of NAI-191 §2 / NAI-192/193 deferral of all 26 module-level pack singletons. `packParamConfigs` and `lookupParamValue` take explicit `*PackFile` arguments. | `NAI-194-D-PACKFILE-SINGLETONS-DEFERRED` |
| `BUILD_VERIFY` checksum validate callback | `PackShared.ts:251-253, 631-633` | Continuation of NAI-191 §2 / NAI-193 deferral. | `NAI-194-D-VALIDATE-DEFERRED` |
| `validateConfigPack` (auto-generated `<srcDir>/pack/param.pack`) | `tools/pack/PackFile.ts:8-228` | Continuation of NAI-191 §2 deferral. Tests hand-craft `pack/param.pack`. | (continuation) |
| Production callsite (build CLI / `::rebuild`) | `ClientCheatHandler.ts:151-153` | Closes the per-config arc. Standalone slice deferred to a post-cohort sub-spec. | (continuation) |
| TS-faithful empty `client` PackedData written to client jagfile | `ParamConfig.ts:188-194, 245-247` | TS-faithful: client is initialized as `new PackedData(ParamPack.max)` and only `.next()` is called per slot — produces `p2(count)+count×0x00`. Likely vestigial (older client formats had param.dat on the client side). Recorded for revival. | `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` |
| Other per-config packers (.inv/.seq/.spotanim/.loc/.obj/.npc/.enum/.struct/.mesanim/.flo/.hunt/.idk/.dbtable/.dbrow) | `tools/pack/config/*.ts` | Each is its own per-slice port in NAI-195+. | (per-slice) |

**Retired this slice:** none. The var-domain uniqueness retirement landed in NAI-193.

## §3 Pre-flight audit

Per `controller_preflight` + `risk_register_premise_grep`, every premise below was re-verified against HEAD `2df5078`.

### §3.1 TS schema (`ParamConfig.ts`)

`parseParamConfig(key, value)`:
- `stringKeys = []` (empty in TS) — placeholder for future string-typed metadata. Goscape mirrors with an empty `switch` arm.
- `numberKeys = []` (empty in TS) — placeholder.
- `booleanKeys = ['autodisable']` — via `isConfigBoolean` / `getConfigBoolean` (already ported in NAI-192 §4.2).
- `type` — `ScriptVarType.getTypeChar(value)`. Goscape `ScriptVarTypeFromName` returns `(ScriptVarType, bool)`; unknown → reject.
- `default` — returns `value` (raw string); resolution deferred to `packParamConfigs` after `type` is known. Mirrors TS comment `// defer lookup to pack callback`.
- Unknown key → `undefined` (TS) / `(nil, false, nil)` (Go, per NAI-192 `ParseFn` contract).

`lookupParamValue(type, value)` — `null`-sentinel early-return + 20 TS arms over `ScriptVarType` (NAMEDOBJ and OBJ share an arm):

| Branch | TS dispatch | Goscape dispatch | Notes |
|---|---|---|---|
| `null` sentinel | early-return: `-1` (non-STRING) or `""` (STRING) | same | applies before any type-specific branch |
| `INT` | hex (`0x` prefix) → `parseInt(_, 16)`; else dec | `strconv.ParseInt(value, 0, 64)` (single call covers both) | TS regex `/^-?[0-9a-fA-F]+$/` on hex slice + `/^-?[0-9]+$/` on dec; Go single-call rejects identical inputs |
| `STRING` | `value.length > 1000` → null; else passthrough | same | "arbitrary limit" per TS comment |
| `BOOLEAN` | `isConfigBoolean` + `getConfigBoolean` → 1/0 | `pkg/pack`-internal helpers from NAI-192 § | return value is `int` (0/1), not `bool` |
| `COORD` | 5-part `_`-split: `level_mX_mZ_lX_lZ`; bounds level≤3, mX/mZ≤255, lX/lZ≤63, all ≥0; bit-pack `z \| (x<<14) \| (level<<28)` where `x=(mX<<6)+lX`, `z=(mZ<<6)+lZ` | port via `coordgrid.PackCoord(level, x, z)` after computing `x = mX*64 + lX`, `z = mZ*64 + lZ` | `coordgrid.PackCoord` at `pkg/coordgrid/coordgrid.go:158`. Verify bit-layout parity at plan-author time — TS bit math is `z + (x<<14) + (level<<28)`; goscape `PackCoord` semantics must match exactly. |
| `ENUM` | `EnumPack.getByName` | `enumPack.GetByName` | -1 → null |
| `NAMEDOBJ`, `OBJ` | `ObjPack.getByName` | `objPack.GetByName` | TS uses same Pack for both type codes; mirror |
| `LOC` | `LocPack.getByName` | `locPack.GetByName` | |
| `COMPONENT` | `InterfacePack.getByName` | `interfacePack.GetByName` | TS routes `COMPONENT` through `InterfacePack`; current content uses neither but TS code-path mirrored |
| `STRUCT` | `StructPack.getByName` | `structPack.GetByName` | |
| `CATEGORY` | `CategoryPack.getByName` | `categoryPack.GetByName` | |
| `SPOTANIM` | `SpotAnimPack.getByName` | `spotanimPack.GetByName` | |
| `NPC` | `NpcPack.getByName` | `npcPack.GetByName` | |
| `INV` | `InvPack.getByName` | `invPack.GetByName` | |
| `SYNTH` | `SynthPack.getByName` | `synthPack.GetByName` | |
| `SEQ` | `SeqPack.getByName` | `seqPack.GetByName` | |
| `STAT` | `stats.indexOf(value)`, 21-entry hardcoded slice | port slice as `var paramStats = []string{…}`; `slices.Index(paramStats, value)` | order: attack, defence, strength, hitpoints, ranged, prayer, magic, cooking, woodcutting, fletching, fishing, firemaking, crafting, smithing, mining, herblore, agility, thieving, slayer, farming, runecraft |
| `NPC_STAT` | `npcStats.indexOf(value)`, 6-entry hardcoded slice | port slice as `var paramNpcStats = []string{…}`; `slices.Index(paramNpcStats, value)` | order: hitpoints, attack, strength, defence, magic, ranged |
| `VARP` | `VarpPack.getByName` | `varpPack.GetByName` | |
| `INTERFACE` | if `value.indexOf(':') !== -1` → -1; else `InterfacePack.getByName` | port colon-reject sentinel before Pack lookup | guards against component-path syntax `iface:component` |
| `DBROW` | `DbRowPack.getByName` | `dbrowPack.GetByName` | |
| default (no match) | `index = -1` | trailing `return -1, error("unknown ScriptVarType: …")` | |

Trailing return: `index !== -1 ? index : null` → `(value, nil) | (0, error)` shape in Go. TS callers throw `packStepError` on null; goscape callers wrap via `parseStepError`-shaped envelope per NAI-192 convention.

`packParamConfigs(configs, paramPF, lookups)`:
- For each slot `id ∈ [0, paramPF.Max)`:
  - debugname = `paramPF.GetByID(id)`.
  - If config-line list present:
    - **Pre-scan**: find the `type` line first (TS uses `config.find(({key}) => key === 'type')!.value`). This is needed before any `default` line can resolve.
    - Iterate lines in source order:
      - `type` → `server.P1(1); server.P1(uint8(value))`.
      - `default` → call `lookupParamValue(type, rawValue, lookups)`; on STRING type emit `server.P1(5); server.PJStr(value)`; otherwise emit `server.P1(2); server.P4(uint32(value))`.
      - `autodisable` → if value is `false`, emit `server.P1(4)` (no payload). Otherwise no opcode (default-true is implicit).
  - debugname trailer (when slot has a non-empty name): `server.P1(250); server.PJStr(debugname)`.
  - Both `client.Next()` and `server.Next()` — every slot terminates with 0x00, including empty slots. Client is initialized but never written to between `.Next()` calls (TS-faithful `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL`).
- Returns `(server, client *PackedData)`.

### §3.2 Loader parity target (`pkg/objtype/paramtype.go`)

`LoadParamTypes(dir)` (lines 33-72):
- Server side: `packet.Load(<dir>/server/param.dat)`.
- No client-side load (consistent with empty-client semantics).
- `NewParamType(id)` allocates a slot. **Pre-existing bug**: `AutoDisable` defaults to `false`. TS `ParamType.autodisable = true`. T1 fixes via explicit field init.
- `(pt *ParamType).Decode(code, dat)` (lines 86-103):
  - `case 1`: `Type = ScriptVarType(dat.G1())`.
  - `case 2`: `DefaultInt = int32(dat.G4())`.
  - `case 4`: `AutoDisable = false`.
  - `case 5`: `DefaultString = dat.GJStrLF()`.
  - `case 250`: `DebugName = dat.GJStrLF()`.

Binding: `TestParamPacker_LoaderRoundTrip` runs `PackConfigs(srcDir, outDir)` against a hand-crafted fixture covering primitive + typed-id + autodisable=true/false defaults, then asserts each ParamType's `Type`/`DefaultInt`/`DefaultString`/`AutoDisable`/`DebugName` round-trips.

### §3.3 `*PackFile` registries needed for lookup

Real `.param` content (208 entries across `$HOME/Code/github.com/LostCityRS/Content/scripts/**/*.param`) uses 13 ScriptVarType variants. TS `lookupParamValue` covers 16. Goscape `PackConfigs` must construct one `*PackFile` per typed-id reachable from TS code path:

| ScriptVarType | TS Pack | Hand-maintained `.pack` file location |
|---|---|---|
| ENUM | EnumPack | `Content/pack/enum.pack` |
| OBJ, NAMEDOBJ | ObjPack | `Content/pack/obj.pack` |
| LOC | LocPack | `Content/pack/loc.pack` |
| COMPONENT, INTERFACE | InterfacePack | `Content/pack/interface.pack` |
| STRUCT | StructPack | `Content/pack/struct.pack` |
| CATEGORY | CategoryPack | `Content/pack/category.pack` |
| SPOTANIM | SpotAnimPack | `Content/pack/spotanim.pack` |
| NPC | NpcPack | `Content/pack/npc.pack` |
| INV | InvPack | `Content/pack/inv.pack` |
| SYNTH | SynthPack | `Content/pack/synth.pack` |
| SEQ | SeqPack | `Content/pack/seq.pack` |
| VARP | VarpPack | `Content/pack/varp.pack` (reuses NAI-193 PackFile) |
| DBROW | DbRowPack | `Content/pack/dbrow.pack` |

13 distinct `*PackFile`s (VarpPack reused from existing trio; the other 12 are net-new constructions in `PackConfigs`). All 13 hand-maintained `.pack` files exist in `Content/pack/`.

To keep `PackConfigs` and `lookupParamValue` signatures legible, the 13 typed-id PackFiles are bundled into a struct:

```go
type paramLookups struct {
    enumPF, objPF, locPF, interfacePF, structPF, categoryPF *PackFile
    spotanimPF, npcPF, invPF, synthPF, seqPF, varpPF, dbrowPF *PackFile
}
```

Passed by pointer to `lookupParamValue(typ, value, lk *paramLookups)`. Constructor `loadParamLookups(srcDir string) (*paramLookups, error)` returns the 13-pack bundle; called from `PackConfigs` only when `.param` source is present (gated on `GetLatestModified(scriptsDir, ".param") > 0`).

### §3.4 Cross-domain uniqueness expansion

No expansion this slice. Param debugnames live in their own namespace (`param.pack`). The TS code path does not cross-check `param` names against any other domain. NAI-193's `checkVarNameUniqueness({varp, varn, vars})` remains the only cross-domain check.

### §3.5 Loader-side `AutoDisable` default bug

`pkg/objtype/paramtype.go:148-154`:

```go
func NewParamType(id int) *ParamType {
    return &ParamType{
        ConfigType: ConfigType{ID: id},
    }
}
```

`AutoDisable` not initialized → Go zero `false`. TS `ParamType.autodisable = true` (`src/cache/config/ParamType.ts:64`).

Impact in production code: `pkg/objtype/objtype.go:113` reads `ptc.Configs[k].AutoDisable` when filtering params on objs. With current goscape default `false`, **every param-by-default is autodisable-false** in goscape vs TS-true — a latent semantic divergence that the round-trip test would expose.

Grep coverage:
- `rg "AutoDisable" pkg/ modules/ cmd/` → 4 hits: `paramtype.go:82` (field decl), `paramtype.go:93` (case 4 → set false), `objtype.go:113` (read), `paramtype_test.go` (existing test fixture). All audited; no other writers.
- The fix is a 1-line addition in `NewParamType` (`AutoDisable: true`). Existing `paramtype_test.go` checks must be re-read for implicit-zero assumptions.

### §3.6 No existing `param.pack` source file in repo

`find` returns zero `.param` files in `$HOME/Code/github.com/zsrv/goscape`. Tests construct hand-crafted `pack/param.pack` + typed-id `pack/*.pack` files in `t.TempDir()`.

### §3.7 ScriptVarType `STAT` / `NPC_STAT` index resolution

TS hardcodes `stats` (21 entries) and `npcStats` (6 entries) inside `ParamConfig.ts`. These are NOT exported from `ScriptVarType.ts`. Goscape mirror lives in `pkg/pack/param.go` as package-private slices `paramStats` / `paramNpcStats` (not exported beyond `pkg/pack`, since they're a packer-specific encoding detail).

### §3.8 `parseStepError` envelope shape

NAI-192's `ReadTypedConfigs` wraps `parseFn` errors as `"Error during parsing - see %s:%d\n<inner>"`. Per-key validation errors from `parseParamConfig` use the same envelope shape (`"Invalid property value: ..."`). Default-value lookup errors from `lookupParamValue` are raised at pack-time (not parse-time) and use a `packStepError`-shaped wrapper: `"Error during pack step <debugname>: Invalid default value: %s"`.

NAI-192 introduced parse-stage error wrapping; NAI-194 introduces the first **pack-stage** error path. The wrapping helper goes in `pkg/pack/pack_errors.go` (or extends the existing parse-error helper if NAI-192 left a unified one).

## §4 Components

### §4.1 Loader fix: `NewParamType().AutoDisable = true` (`pkg/objtype/paramtype.go`)

```go
func NewParamType(id int) *ParamType {
    return &ParamType{
        ConfigType: ConfigType{ID: id},
        AutoDisable: true, // TS parity: ParamType.ts:64
    }
}
```

Existing `pkg/objtype/paramtype_test.go` callers must be audited; one or more may assume implicit-zero `false`. Adjust fixture inputs to encode explicit `false` where the test intent was the false case.

No deviation tag — TS-faithful fix to a goscape-only latent bug. Cite this slice in the field's doc-comment.

### §4.2 `parseParamConfig` (`pkg/pack/param.go`)

```go
func parseParamConfig(key, value string) (ConfigValue, bool, error) {
    switch key {
    case "autodisable":
        if !IsConfigBoolean(value) {
            return nil, true, fmt.Errorf("invalid boolean: %s", value)
        }
        return GetConfigBoolean(value), true, nil
    case "type":
        t, ok := objtype.ScriptVarTypeFromName(value)
        if !ok {
            return nil, true, fmt.Errorf("unknown script var type: %s", value)
        }
        return t, true, nil
    case "default":
        return value, true, nil // raw string; resolution deferred to pack stage
    }
    return nil, false, nil // unknown key
}
```

Notes:
- The TS `stringKeys = []` / `numberKeys = []` placeholders are omitted (no live branches). If TS ever populates them, the goscape switch adds them as cases.
- `default`'s deferred resolution matches the TS `// defer lookup to pack callback` comment — bound via a `Value: string` raw passthrough.

### §4.3 `lookupParamValue` (`pkg/pack/param.go`)

```go
var paramStats = []string{
    "attack", "defence", "strength", "hitpoints", "ranged", "prayer",
    "magic", "cooking", "woodcutting", "fletching", "fishing", "firemaking",
    "crafting", "smithing", "mining", "herblore", "agility", "thieving",
    "slayer", "farming", "runecraft",
}

var paramNpcStats = []string{
    "hitpoints", "attack", "strength", "defence", "magic", "ranged",
}

// lookupParamValue resolves a raw `default=` value string against a
// ScriptVarType. Returns the resolved scalar (int for indexed/primitive
// types, string for STRING) or an error if the lookup fails.
//
// TS source: tools/pack/config/ParamConfig.ts lookupParamValue (~30-180).
func lookupParamValue(typ objtype.ScriptVarType, value string, lk *paramLookups) (any, error) {
    if value == "null" {
        if typ == objtype.ScriptVarTypeString {
            return "", nil
        }
        return int(-1), nil
    }

    switch typ {
    case objtype.ScriptVarTypeInt:
        n, err := strconv.ParseInt(value, 0, 64)
        if err != nil {
            return nil, fmt.Errorf("invalid int default %q", value)
        }
        return int(n), nil

    case objtype.ScriptVarTypeString:
        if len(value) > 1000 {
            return nil, fmt.Errorf("string default exceeds 1000 chars")
        }
        return value, nil

    case objtype.ScriptVarTypeBoolean:
        if !IsConfigBoolean(value) {
            return nil, fmt.Errorf("invalid boolean default %q", value)
        }
        if GetConfigBoolean(value) {
            return int(1), nil
        }
        return int(0), nil

    case objtype.ScriptVarTypeCoord:
        return parseParamCoord(value)

    case objtype.ScriptVarTypeEnum:
        return paramIndexOrErr(lk.enumPF, value, "enum")
    case objtype.ScriptVarTypeNamedObj, objtype.ScriptVarTypeObj:
        return paramIndexOrErr(lk.objPF, value, "obj")
    case objtype.ScriptVarTypeLoc:
        return paramIndexOrErr(lk.locPF, value, "loc")
    case objtype.ScriptVarTypeComponent:
        return paramIndexOrErr(lk.interfacePF, value, "component")
    case objtype.ScriptVarTypeStruct:
        return paramIndexOrErr(lk.structPF, value, "struct")
    case objtype.ScriptVarTypeCategory:
        return paramIndexOrErr(lk.categoryPF, value, "category")
    case objtype.ScriptVarTypeSpotanim:
        return paramIndexOrErr(lk.spotanimPF, value, "spotanim")
    case objtype.ScriptVarTypeNPC:
        return paramIndexOrErr(lk.npcPF, value, "npc")
    case objtype.ScriptVarTypeInv:
        return paramIndexOrErr(lk.invPF, value, "inv")
    case objtype.ScriptVarTypeSynth:
        return paramIndexOrErr(lk.synthPF, value, "synth")
    case objtype.ScriptVarTypeSeq:
        return paramIndexOrErr(lk.seqPF, value, "seq")
    case objtype.ScriptVarTypeVarp:
        return paramIndexOrErr(lk.varpPF, value, "varp")
    case objtype.ScriptVarTypeDbrow:
        return paramIndexOrErr(lk.dbrowPF, value, "dbrow")

    case objtype.ScriptVarTypeStat:
        i := slices.Index(paramStats, value)
        if i < 0 {
            return nil, fmt.Errorf("unknown stat %q", value)
        }
        return i, nil

    case objtype.ScriptVarTypeNpcStat:
        i := slices.Index(paramNpcStats, value)
        if i < 0 {
            return nil, fmt.Errorf("unknown npc_stat %q", value)
        }
        return i, nil

    case objtype.ScriptVarTypeInterface:
        if strings.Contains(value, ":") {
            return nil, fmt.Errorf("interface default may not contain ':' (use component path elsewhere): %q", value)
        }
        return paramIndexOrErr(lk.interfacePF, value, "interface")
    }

    return nil, fmt.Errorf("unsupported default ScriptVarType %d (char %q)", typ, string(rune(typ)))
}

func paramIndexOrErr(pf *PackFile, value, kind string) (int, error) {
    if pf == nil {
        return 0, fmt.Errorf("%s pack not loaded", kind)
    }
    i := pf.GetByName(value)
    if i < 0 {
        return 0, fmt.Errorf("unknown %s %q", kind, value)
    }
    return i, nil
}

func parseParamCoord(value string) (int, error) {
    parts := strings.Split(value, "_")
    if len(parts) != 5 {
        return 0, fmt.Errorf("coord must be 5 parts (level_mX_mZ_lX_lZ): %q", value)
    }
    level, err := strconv.Atoi(parts[0])
    if err != nil {
        return 0, fmt.Errorf("coord level: %w", err)
    }
    mX, err := strconv.Atoi(parts[1])
    if err != nil {
        return 0, fmt.Errorf("coord mX: %w", err)
    }
    mZ, err := strconv.Atoi(parts[2])
    if err != nil {
        return 0, fmt.Errorf("coord mZ: %w", err)
    }
    lX, err := strconv.Atoi(parts[3])
    if err != nil {
        return 0, fmt.Errorf("coord lX: %w", err)
    }
    lZ, err := strconv.Atoi(parts[4])
    if err != nil {
        return 0, fmt.Errorf("coord lZ: %w", err)
    }
    if level < 0 || mX < 0 || mZ < 0 || lX < 0 || lZ < 0 {
        return 0, fmt.Errorf("coord parts must be non-negative")
    }
    if level > 3 || mX > 255 || mZ > 255 || lX > 63 || lZ > 63 {
        return 0, fmt.Errorf("coord part out of range (level≤3, m*≤255, l*≤63)")
    }
    x := mX*64 + lX
    z := mZ*64 + lZ
    return coordgrid.PackCoord(level, x, z), nil
}
```

**Plan-author verification gate (per `plan_geometry_premise_pretrace`):** confirm `coordgrid.PackCoord(level, x, z)` returns `z | (x<<14) | (level<<28)`. Read `pkg/coordgrid/coordgrid.go:158` and compare against TS `ParamConfig.ts:83-86`. If goscape `PackCoord` bit-layout differs, fall back to inline bit math + record deviation tag.

### §4.4 `packParamConfigs` (`pkg/pack/param.go`)

```go
func packParamConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (server, client *PackedData, err error) {
    server = NewPackedData(pf.Max)
    client = NewPackedData(pf.Max) // TS-faithful empty client (NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL)

    for id := range pf.Max {
        name := pf.GetByID(id)
        if cfg, ok := configs[name]; ok {
            // Pre-scan for type to enable default-value lookup.
            var typ objtype.ScriptVarType
            typFound := false
            for _, line := range cfg {
                if line.Key == "type" {
                    typ = line.Value.(objtype.ScriptVarType)
                    typFound = true
                    break
                }
            }
            if !typFound {
                return nil, nil, fmt.Errorf("param %q missing type", name)
            }

            for _, line := range cfg {
                switch line.Key {
                case "type":
                    server.P1(1)
                    server.P1(uint8(typ))
                case "default":
                    raw := line.Value.(string)
                    resolved, lookupErr := lookupParamValue(typ, raw, lk)
                    if lookupErr != nil {
                        return nil, nil, fmt.Errorf("param %q default: %w", name, lookupErr)
                    }
                    if typ == objtype.ScriptVarTypeString {
                        server.P1(5)
                        server.PJStr(resolved.(string))
                    } else {
                        server.P1(2)
                        server.P4(uint32(resolved.(int)))
                    }
                case "autodisable":
                    if !line.Value.(bool) {
                        server.P1(4)
                    }
                }
            }
        }
        if len(name) > 0 {
            server.P1(250)
            server.PJStr(name)
        }
        server.Next()
        client.Next()
    }
    return server, client, nil
}
```

Notes:
- `packParamConfigs` now returns `error` (unlike `packVarpConfigs` which is infallible). Pack-stage default-value lookup can fail; the err propagates through `packAndSaveParam` → `PackConfigs`.
- TS uses `config.find(({key}) => key === 'type')!.value` — the `!` non-null assertion implies TS assumes `type` is present. Goscape adds an explicit "missing type" error to keep the contract clearer at the pack-stage error site.

### §4.5 `PackConfigs` orchestrator extension (`pkg/pack/pack_configs.go`)

`PackConfigs(srcDir, outDir string) error` gains:

```go
// .param — server + (empty) client outputs.
if GetLatestModified(scriptsDir, ".param") > 0 &&
    ShouldBuild(scriptsDir, ".param", filepath.Join(serverOut, "param.dat")) {
    paramPack, err := NewPackFile(srcDir, "param", nil)
    if err != nil {
        return err
    }
    lk, err := loadParamLookups(srcDir, varpPack)
    if err != nil {
        return err
    }
    if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
        return err
    }
    clientJagDirty = true
}
```

Branch is freshness-gated and only constructs the 13-pack lookup bundle when `.param` source is present (cost-amortized for the no-source case).

`loadParamLookups(srcDir string, varpPF *PackFile) (*paramLookups, error)` builds the 13-pack bundle. `varpPF` is threaded in to avoid double-construction (already built up-front for the var-domain uniqueness check). The other 12 PackFiles are net-new per-call.

```go
func loadParamLookups(srcDir string, varpPF *PackFile) (*paramLookups, error) {
    lk := &paramLookups{varpPF: varpPF}
    for _, t := range []struct {
        name string
        dst  **PackFile
    }{
        {"enum", &lk.enumPF},
        {"obj", &lk.objPF},
        {"loc", &lk.locPF},
        {"interface", &lk.interfacePF},
        {"struct", &lk.structPF},
        {"category", &lk.categoryPF},
        {"spotanim", &lk.spotanimPF},
        {"npc", &lk.npcPF},
        {"inv", &lk.invPF},
        {"synth", &lk.synthPF},
        {"seq", &lk.seqPF},
        {"dbrow", &lk.dbrowPF},
    } {
        pf, err := NewPackFile(srcDir, t.name, nil)
        if err != nil {
            return nil, fmt.Errorf("load %s pack: %w", t.name, err)
        }
        *t.dst = pf
    }
    return lk, nil
}
```

`packAndSaveParam`:

```go
func packAndSaveParam(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants, clientJag *jagfile.Jagfile) error {
    cfgs, err := ReadTypedConfigs(srcDir, ".param", nil, parseParamConfig, c)
    if err != nil {
        return err
    }
    server, client, err := packParamConfigs(cfgs, pf, lk)
    if err != nil {
        return err
    }
    if err := server.Save(
        filepath.Join(serverOut, "param.dat"),
        filepath.Join(serverOut, "param.idx"),
    ); err != nil {
        return err
    }
    clientJag.Write("param.dat", client.Dat)
    clientJag.Write("param.idx", client.Idx)
    return nil
}
```

### §4.6 Tracker entries

Update `docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md` follow-up tracker (per NAI-191 carry-forward conventions) to mark `.param` packer landed in NAI-194; remaining packers tracked by extension count (`13 packers remaining: .inv, .seq, .spotanim, .loc, .obj, .npc, .enum, .struct, .mesanim, .flo, .hunt, .idk, .dbtable, .dbrow`).

## §5 Data flow

```
<srcDir>/scripts/**/*.constant       ──►  LoadConstants  ──►  Constants
<srcDir>/pack/{varp,varn,vars}.pack  ──►  NewPackFile × 3 ──►  PackFile × 3  ──►  checkVarNameUniqueness  (existing NAI-193 surface)
                                                                                       │
                                                                                       ▼
                                              fresh jagfile.Jagfile (NewJagfile(nil))
                                                                                       │
                                                                                       │ varp/varn/vars branches (existing) ──► server.Save + clientJag.Write
                                                                                       │
                                                                                       ▼
                                            (.param branch — NEW)
<srcDir>/pack/param.pack             ──►  NewPackFile  ──►  paramPF
<srcDir>/pack/{enum,obj,loc,interface,struct,category,
               spotanim,npc,inv,synth,seq,dbrow}.pack
                                     ──►  loadParamLookups ──►  paramLookups (13 PackFiles)
<srcDir>/scripts/**/*.param          ──►  ReadTypedConfigs  ──►  configs
                                                                  │
                                                                  ▼
                                          packParamConfigs(configs, paramPF, lk)
                                                                  │
                                              ┌───────────────────┴───────────────────┐
                                              ▼                                       ▼
                                          server                                  client (empty content)
                                              │                                       │
                                              ▼                                       ▼
                                          server.Save                            clientJag.Write
                                          param.{dat,idx}                        param.{dat,idx}
                                                                                       │
                                                                                       ▼ (at end of PackConfigs, if clientJagDirty)
                                                                                clientJag.Save(<outDir>/client/config)
```

## §6 Error handling

Inherits NAI-192/193 error envelope (parse-stage errors via `parseStepError` shape). New pack-stage error path:

| Site | Behavior | TS parity |
|---|---|---|
| `parseParamConfig` autodisable non-boolean | `"...Invalid property value: ..."` | ✅ |
| `parseParamConfig` type unknown | `"...Invalid property value: ..."` | ✅ |
| `packParamConfigs` missing `type` line | `"param %q missing type"` | ⚠ stricter than TS (TS asserts via `!`, would throw on undefined) |
| `lookupParamValue` "null" sentinel | returns `(-1, nil)` for non-STRING, `("", nil)` for STRING | ✅ |
| `lookupParamValue` INT non-numeric | `"invalid int default %q"` | ✅ (TS returns null → packStepError) |
| `lookupParamValue` STRING > 1000 chars | `"string default exceeds 1000 chars"` | ✅ |
| `lookupParamValue` BOOLEAN non-boolean | `"invalid boolean default %q"` | ✅ |
| `lookupParamValue` COORD malformed / OOB | `"coord must be 5 parts ..."` / `"coord part out of range ..."` | ✅ |
| `lookupParamValue` typed-id name not in pack | `"unknown %s %q"` | ✅ |
| `lookupParamValue` STAT/NPC_STAT name not in slice | `"unknown stat %q"` / `"unknown npc_stat %q"` | ✅ |
| `lookupParamValue` INTERFACE with `:` | `"interface default may not contain ':'..."` | ✅ |
| `lookupParamValue` unsupported ScriptVarType | `"unsupported default ScriptVarType ..."` | ✅ (TS falls through to `index = -1` → null → packStepError) |

## §7 Testing strategy

### §7.1 Loader fix (T1)

`pkg/objtype/paramtype_test.go` — audit existing tests for implicit-zero `AutoDisable` assumptions. Add an explicit positive test:

```go
func TestNewParamType_DefaultAutoDisableTrue(t *testing.T) {
    pt := NewParamType(0)
    if !pt.AutoDisable {
        t.Fatalf("AutoDisable default = false, want true (TS ParamType.ts:64)")
    }
}
```

Existing tests that pass through `Decode` opcode 4 still produce `AutoDisable = false` (the explicit-false case). Tests that exercised "no opcode 4" expected the goscape-buggy `false`; they must be updated to expect `true` (TS parity).

### §7.2 `parseParamConfig` per-key coverage (`pkg/pack/param_test.go`)

Table-driven:

| Input | Expected `(value, ok, err)` |
|---|---|
| `("autodisable", "yes")` | `(true, true, nil)` |
| `("autodisable", "no")` | `(false, true, nil)` |
| `("autodisable", "maybe")` | `(nil, true, non-nil)` |
| `("type", "int")` | `(ScriptVarTypeInt, true, nil)` |
| `("type", "loc")` | `(ScriptVarTypeLoc, true, nil)` |
| `("type", "bogus")` | `(nil, true, non-nil)` |
| `("default", "anything")` | `("anything", true, nil)` (raw passthrough) |
| `("unknownkey", "x")` | `(nil, false, nil)` |

### §7.3 `lookupParamValue` per-branch coverage (`pkg/pack/param_test.go`)

Each branch gets at least one happy-path case + at least one error case. Coordinated coverage:

| Branch | Happy-path | Error |
|---|---|---|
| `"null"` sentinel (non-STRING) | `(INT, "null") → (-1, nil)` | n/a |
| `"null"` sentinel (STRING) | `(STRING, "null") → ("", nil)` | n/a |
| INT decimal | `(INT, "42") → (42, nil)` | `(INT, "abc") → error` |
| INT hex | `(INT, "0xFF") → (255, nil)` | `(INT, "0xQQ") → error` |
| STRING | `(STRING, "hello") → ("hello", nil)` | `(STRING, strings.Repeat("a", 1001)) → error` |
| BOOLEAN | `(BOOLEAN, "yes") → (1, nil)`; `(BOOLEAN, "no") → (0, nil)` | `(BOOLEAN, "maybe") → error` |
| COORD | `(COORD, "0_50_50_32_32") → known pack int` | `(COORD, "0_50_50_32") → error`; `(COORD, "4_0_0_0_0") → error` (level OOB); `(COORD, "0_0_0_64_0") → error` (lX OOB) |
| ENUM | `(ENUM, "<known>") → id` | `(ENUM, "<missing>") → error` |
| OBJ, NAMEDOBJ | same as ENUM with `objPF` | same |
| LOC | same with `locPF` | same |
| COMPONENT | same with `interfacePF` | same |
| STRUCT, CATEGORY, SPOTANIM, NPC, INV, SYNTH, SEQ, VARP, DBROW | same pattern | same |
| STAT | `(STAT, "attack") → 0`; `(STAT, "runecraft") → 20` | `(STAT, "fakeskill") → error` |
| NPC_STAT | `(NPC_STAT, "hitpoints") → 0`; `(NPC_STAT, "ranged") → 5` | `(NPC_STAT, "magic") → 4`; `(NPC_STAT, "agility") → error` |
| INTERFACE no colon | `(INTERFACE, "<known>") → id` | `(INTERFACE, "<missing>") → error` |
| INTERFACE with colon | n/a | `(INTERFACE, "iface:component") → error` |
| Unsupported type | n/a | `(NpcUid, "x") → error` |

Test helper: each typed-id case constructs a minimal `paramLookups` with only the relevant PackFile populated (others nil) — verifies the nil-guard via `paramIndexOrErr`.

### §7.4 `packParamConfigs` byte-pin (`pkg/pack/param_test.go`)

Fixture: 2 param slots.
- Slot 0 = `health_param`: type=int, default=100, autodisable=no.
- Slot 1 = empty.

Expected server dat:
- `00 02` — count header
- Slot 0 body: `01 69` (type=int=105), `02 00 00 00 64` (default=p4(100)), `04` (autodisable=false), `fa 68 65 61 6c 74 68 5f 70 61 72 61 6d 0a` (debugname trailer = 0xfa + "health_param" + LF)
- Slot 0 terminator: `00`
- Slot 1 (empty): `00`

Expected server idx:
- `00 02` count header
- Slot 0 size: `p2(size)` — size = bytes between slot-0 marker and after `Next()`'s 0x00 emit. Per `pkg/pack/packed_data.go` (NAI-192), `Next()` writes `0x00` then advances marker — so size includes the terminator. Slot 0 = 1+1+1+4+1+1+12+1+1 = 23 bytes. `p2(23) = 00 17`.
- Slot 1 size: `p2(1) = 00 01`.

Expected client dat (empty client per NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL):
- `00 02` count header
- Slot 0 (only terminator): `00`
- Slot 1: `00`

Expected client idx:
- `00 02` count header
- Slot 0 size: `p2(1)` (just the terminator)
- Slot 1 size: `p2(1)`

**Plan-author note (per `rsbuf_roundtrip_tests`):** the exact slot-size accounting depends on whether `PackedData.Next()`'s `Dat.Length() - marker` includes the terminator. The NAI-192 varn byte-pin tests and NAI-193 varp byte-pin tests have already established the answer; this slice's pins must agree (re-derive numbers against `packed_data.go` HEAD).

Additional byte-pin sub-test: STRING default path. Slot 0 = `name_param`: type=string, default=`"hello"`, autodisable=yes (no opcode 4 emitted). Expected slot body: `01 73 05 68 65 6c 6c 6f 0a fa ...`.

Additional byte-pin sub-test: COORD default path. Slot 0 = `start_coord_param`: type=coord, default=`"0_50_50_32_32"`. Expected slot body emits opcode 2 + p4(`coordgrid.PackCoord(0, 50*64+32, 50*64+32)`).

### §7.5 `PackConfigs` integration (`pkg/pack/pack_configs_test.go` — extend)

- `TestPackConfigs_ParamOnly_PrimitiveDefault`: hand-crafted `.param` source with type=int + default=42. Asserts `<outDir>/server/param.{dat,idx}` exist; `<outDir>/client/config` jagfile contains `param.dat`+`param.idx`. Re-run idempotent.
- `TestPackConfigs_ParamWithTypedDefault`: source uses type=npc + default=`some_npc_name`. Hand-crafted `pack/npc.pack` with `some_npc_name`. Asserts round-trip via `LoadParamTypes` shows `DefaultInt = <expected_id>`.
- `TestPackConfigs_ParamMissingTypedPackFile`: source uses type=npc + default=`x` but no `pack/npc.pack` present. Asserts `PackConfigs` returns a non-nil error with "load npc pack" or equivalent in the message chain.
- `TestPackConfigs_ParamNoSrcNoOp`: no `.param` source. Asserts no `param.dat` written; no typed-id PackFiles loaded (verified by absence of side-effects in temp `pack/` dir — e.g., the test omits `pack/enum.pack`, and `PackConfigs` would error if it tried to load it).
- `TestPackConfigs_ParamUnknownTypedDefault`: source uses type=npc + default=`nonexistent_npc`. Hand-crafted `pack/npc.pack` does not include `nonexistent_npc`. Asserts error mentions both the param debugname and the missing npc.
- `TestPackConfigs_ParamMixedWithVarDomain`: param + varp + varn + vars all present. Asserts six server `.{dat,idx}` files + client jagfile (containing `varp.dat`+`varp.idx`+`param.dat`+`param.idx`).

### §7.6 Loader round-trip (`pkg/pack/param_test.go`)

```go
func TestParamPacker_LoaderRoundTrip(t *testing.T) {
    srcDir := t.TempDir()
    // hand-craft scripts/test.param with multiple slots:
    //   [int_p]   type=int     default=42
    //   [str_p]   type=string  default=hello
    //   [bool_p]  type=boolean default=yes  autodisable=no
    //   [coord_p] type=coord   default=0_50_50_32_32
    //   [npc_p]   type=npc     default=man     autodisable=yes
    // + pack/param.pack with 5 slots, pack/npc.pack with man=0
    outDir := t.TempDir()
    require.NoError(t, pack.PackConfigs(srcDir, outDir))

    cfgs, err := objtype.LoadParamTypes(outDir)
    require.NoError(t, err)
    require.Len(t, cfgs, 5)

    // assertions per slot — Type, DefaultInt, DefaultString, AutoDisable, DebugName
    require.Equal(t, objtype.ScriptVarTypeInt, cfgs[0].Type)
    require.Equal(t, int32(42), cfgs[0].DefaultInt)
    require.True(t, cfgs[0].AutoDisable) // implicit default-true
    require.Equal(t, "int_p", cfgs[0].DebugName)
    // ... etc for str_p (DefaultString="hello"), bool_p (DefaultInt=1, AutoDisable=false),
    //     coord_p (DefaultInt = PackCoord(0, 50*64+32, 50*64+32)),
    //     npc_p (DefaultInt=0, AutoDisable=true)
}
```

Binds end-to-end byte parity through the production loader for all 4 primitive types + 1 typed-id, including the AutoDisable default-true fix.

### §7.7 Deviation-tag pin tests

New file `pkg/pack/nai194_deviation_pins_test.go` per NAI-192 T9 / NAI-193 T7 pattern:

- `TestNAI194_PackFileSingletonsDeferred_NoModuleLevelParamPack` — `rg "^var ParamPack" pkg/` returns zero matches; constructors take explicit `*PackFile`.
- `TestNAI194_ValidateDeferred_NoBuildVerifyHook` — `rg "BUILD_VERIFY|705633567" pkg/` returns zero matches (continuation pin).
- `TestNAI194_ParamEmptyClientFaithful` — `packParamConfigs` returns a `client` whose `Dat.Data` is exactly `[hi, lo, 0x00 × count]` for an N-slot pack — no opcodes ever emitted to client.

## §8 File inventory

```
pkg/objtype/
  paramtype.go                            MODIFY (NewParamType: AutoDisable = true)
  paramtype_test.go                       MODIFY (audit + adjust existing tests; add positive default-true test)

pkg/pack/
  param.go                                NEW (parseParamConfig + lookupParamValue + parseParamCoord
                                               + paramIndexOrErr + paramLookups + paramStats/paramNpcStats
                                               + packParamConfigs + packAndSaveParam + loadParamLookups)
  param_test.go                           NEW (parse + lookup + byte-pin + round-trip tests)
  pack_configs.go                         MODIFY (param branch — gated PackFile + lookups construction
                                                  + packAndSaveParam + clientJagDirty flip)
  pack_configs_test.go                    MODIFY (add 6 param integration tests per §7.5)
  nai194_deviation_pins_test.go           NEW

docs/superpowers/specs/
  2026-05-13-nai-194-param-packer-design.md  NEW (this file)
```

Outside `pkg/objtype/paramtype{,_test}.go` (one production file + tests) and `pkg/pack/` (the per-slice surface), no production code changes. No new exported API on `pkg/objtype`. No new top-level `PackConfigs` parameters.

## §9 Risk register

| # | Risk | Mitigation | Verified |
|---|---|---|---|
| R1 | Goscape `NewParamType().AutoDisable = false` default contradicts TS (`autodisable = true`). Fixing it may break unrelated tests downstream of `pkg/objtype/objtype.go:113`. | T1 fix is 1-line; pre-flight grep enumerates all `AutoDisable` readers (4 sites). `paramtype_test.go` re-audited per `test_sut_vs_setup_distinction`. | §3.5 grep coverage. |
| R2 | `coordgrid.PackCoord` may not produce TS `z \| (x<<14) \| (level<<28)` exactly — silently wrong COORD defaults. | Plan-author gate per `plan_geometry_premise_pretrace`: read `pkg/coordgrid/coordgrid.go:158` body, compare bits against TS `ParamConfig.ts:83-86`. If divergent, fall back to inline math + tag the deviation. | §4.3 verification gate. |
| R3 | 13 typed-id PackFiles construction in `loadParamLookups` is verbose and easy to mis-key. | Construction loops over a struct slice — same constructor path NAI-191/192/193 already validated. Test §7.5 `TestPackConfigs_ParamMissingTypedPackFile` binds the failure mode. | Pattern match against NAI-193 `PackConfigs`. |
| R4 | Param `value=="null"` sentinel for non-STRING returns `-1`; downstream loader reads as `DefaultInt = -1` (uint32 → int32 cast). | T6 round-trip includes a `default=null` slot to bind. Confirm `int32(uint32(int(-1))) == -1`. | Go integer-cast semantics. |
| R5 | TS `packParamConfigs` does not validate `type` is present — uses `!` non-null assertion. Goscape adds explicit "missing type" error (§3.1). | Behavior strictly safer than TS (TS would `throw` on `undefined.value`). No tag needed. | §6 row. |
| R6 | INTERFACE colon-reject sentinel is easy to miss. | §7.3 includes explicit `(INTERFACE, "iface:component") → error` case. | §3.1 table. |
| R7 | TS-faithful empty `client` PackedData written to client jagfile may surprise loader code. Loader does NOT load client-side param (confirmed §3.2). | `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` tag; pin test §7.7 binds. | §3.2 read of `paramtype.go:39`. |
| R8 | Pack-stage error envelope (`packStepError` shape) is new this slice — wrapping helper location may not exist yet. | §3.8 lands a `pkg/pack/pack_errors.go` if absent; otherwise extends NAI-192's existing parse-error helper. Plan-author audit at T-step start. | grep `parseStepError\|packStepError` in `pkg/pack/`. |
| R9 | `paramIndexOrErr` rejects nil PackFile with `"%s pack not loaded"` — TS would let it crash with `undefined.getByName`. Goscape stricter. | `TestPackConfigs_ParamMissingTypedPackFile` binds the goscape-stricter path. Tag if reviewer flags: `NAI-194-D-NIL-PACKFILE-EXPLICIT-ERROR`. | §3.1 table + §4.3 helper. |
| R10 | `ScriptVarType.STAT/NPC_STAT` slice ordering must match TS exactly — index leaks into the pack as `DefaultInt`. | §7.3 covers happy + boundary indices (0, 5, 20). Test names cite the TS line. | §3.1 table notes. |

## §10 Deviations from TS source

| Tag | What | Why |
|---|---|---|
| `NAI-194-D-PACKFILE-SINGLETONS-DEFERRED` | No module-level `ParamPack` or other 12 typed-id `*Pack` singletons. `packParamConfigs`/`lookupParamValue` take explicit `*PackFile` / `*paramLookups`. | Continuation of NAI-191 §2 / NAI-192/193 deferral. |
| `NAI-194-D-VALIDATE-DEFERRED` | No `BUILD_VERIFY` CRC validate callback (TS magic preserved in NAI-191/193 tag bodies). | Continuation of NAI-191 §2 deferral. |
| `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` | `client` PackedData is initialized and `.Next()`'d per slot but never `.p1()`'d — produces `p2(count)+count×0x00`. Written to client jagfile as `param.dat`/`param.idx`. | TS-faithful: `ParamConfig.ts:188-194, 245-247`. Likely vestigial (pre-#225 client format). |

**Optional tags** (apply only if reviewer flags the corresponding goscape behavior as a divergence):
- `NAI-194-D-NIL-PACKFILE-EXPLICIT-ERROR` — `paramIndexOrErr` emits a typed error when its PackFile arg is nil, vs. TS's implicit `undefined.getByName` crash.
- `NAI-194-D-PARAM-MISSING-TYPE-EXPLICIT-ERROR` — `packParamConfigs` emits "param %q missing type" when `type` line is absent, vs. TS's implicit `!`-assertion crash.

**Retired this slice:** none.

## §11 References

- TS source: `LostCityRS/Engine-TS/tools/pack/config/ParamConfig.ts` (~190 LOC), `tools/pack/PackShared.ts:261-669` (orchestrator), `src/cache/config/ParamType.ts:62-72` (`autodisable=true` default).
- Goscape predecessors: NAI-191 spec (`docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md`), NAI-192 spec (`...-nai-192-varn-vars-packers-design.md`), NAI-193 spec (`...-nai-193-varp-packer-design.md`).
- Existing loader (round-trip target): `pkg/objtype/paramtype.go`.
- Coord bit-pack: `pkg/coordgrid/coordgrid.go:158`.
- Memories: `controller_preflight`, `risk_register_premise_grep`, `plan_geometry_premise_pretrace`, `rsbuf_roundtrip_tests`, `plan_helper_coverage`, `plan_runnable_test_fixtures`, `test_sut_vs_setup_distinction`, `defensive_gate_doc_comment_label`, `jagfile_write_save_latent_bugs`, `for_range_slice_mutation_in_body`, `packet_rw_pointer_gotcha`, `true_to_ts_gate`, `retire_deviation_grep_all_comments`.

## §12 Acceptance criteria

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... ./pkg/objtype/... -count=1 -race` — PASS.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — clean.
- `gofmt -l pkg/objtype pkg/pack` — empty.
- `rg "NAI-194-D-" pkg/` — at least three matches (PACKFILE-SINGLETONS-DEFERRED, VALIDATE-DEFERRED, PARAM-EMPTY-CLIENT-FAITHFUL); plus any optional tags chosen at review.
- All pin tests in `nai194_deviation_pins_test.go` green.
- `LoadParamTypes(<outDir>)` round-trips correctly after `PackConfigs` runs against a hand-crafted multi-type fixture (binds end-to-end byte-format parity through the production loader, including the AutoDisable default-true fix).
- No production callsite added; `PackConfigs` remains test-only wired (NAI-195+ continues the per-config arc; production `cmd/` entry point lands when the cohort closes).
