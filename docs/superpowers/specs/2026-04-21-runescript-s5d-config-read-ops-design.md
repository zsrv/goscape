# Sub-spec RuneScript S5d: Config-Read Opcodes — Design

**Status:** Draft → ready for plan
**Scope:** 33 config-read handlers across `EnumOps` (2), `StructOps` (1), `LocConfigOps` (7), `NpcConfigOps` (8), and `ObjConfigOps` (15). Three new cache-config loaders: `EnumType`, `StructType`, `LocType`. A single `Configs` interface on `ScriptState` gives handlers the lookup surface they need. No new server state, no wire traffic — these are stateless reads against already-loaded cache data.
**Out of scope:** Loc/npc/obj `_OP` / `_IOP` variants beyond the 33 below (TS handler files don't implement them for every config type — e.g. `OC_OP` exists but isn't in `ObjConfigOps.ts`; skip for S5d). Client-side config fields (model id, icon, etc.) — we load server.dat only. Writeable struct/enum setters — all reads only.

---

## Goal

After S5d:

- Scripts can look up any loaded ObjType field (name, cost, weight, members flag, wearpos, tradeable, stackable, etc.) via the 15 `OC_*` handlers.
- Scripts can look up NpcType fields (name, category, op list, size, vislevel) via the 8 `NC_*` handlers.
- Scripts can look up LocType fields (name, category, width, length) via the 7 `LC_*` handlers.
- Scripts can resolve enum lookups (e.g. `stat_name_enum[3] → "Magic"`) via `ENUM` and `ENUM_GETOUTPUTCOUNT`.
- Scripts can read `STRUCT_PARAM` values.
- `OC_PARAM` / `NC_PARAM` / `LC_PARAM` / `STRUCT_PARAM` share a single lookup path with correct int/string return-type dispatch via `ParamType`.
- Demo: a script `mes(oc_name(995))` prints `"Coins"` on the wire (asserting real cache data is readable).

## Architecture

Three new config loaders + one handler-file split by TS category + one new `Configs` interface on `ScriptState`.

```
pkg/objtype/
├── enumtype.go          (new) EnumType + LoadEnumTypes
├── enumtype_test.go     (new)
├── structtype.go        (new) StructType + LoadStructTypes
├── structtype_test.go   (new)
├── loctype.go           (new) LocType + LoadLocTypes (server-side only)
└── loctype_test.go      (new)

pkg/script/
├── configs.go           (new) Configs interface + paramLookup helper
├── handlers_config.go   (new) all 33 handlers
├── handlers_config_test.go (new)
├── state.go             + Configs field on ScriptState
└── handlers.go          + 33 map entries

modules/world/
├── server.go            + enumTypes, structTypes, locTypes fields + NewServer loads + Configs impl via serverConfigsView
├── server_configs.go    (new) serverConfigsView struct implementing script.Configs
├── script.go            runScript sets state.Configs
└── script_test.go       + E2E OC_NAME round-trip using a seeded fixture
```

## Components

### 1. Config type loaders — `pkg/objtype/{enumtype,structtype,loctype}.go`

Follow the existing `ParamType` / `VarPlayerType` pattern exactly (a `ConfigType` embed, a `Decode(code, dat)` switch, a `LoadXxxTypes(dir)` entrypoint that reads server/`<name>`.dat, and a parse helper that produces a `XxxTypeConfigs { ConfigNames map[string]int; Configs []*XxxType }` struct).

**EnumType fields** (from TS EnumType.ts — implementer verifies exact code ids):
```go
type EnumType struct {
    ConfigType
    InputType     ScriptVarType
    OutputType    ScriptVarType
    DefaultInt    int32
    DefaultString string
    Values        map[int32]any // int32 → int32 OR string, dispatched by OutputType
}
```

**StructType fields**:
```go
type StructType struct {
    ConfigType
    Params ParamMap // only stored field; decoded at code 249
}
```

**LocType fields** (server-side subset only — TS decodes many more from client jagfile which we skip):
```go
type LocType struct {
    ConfigType
    Category int
    Desc     string
    Width    int
    Length   int
    Params   ParamMap
}
```

Implementer verifies each `Decode(code, dat)` switch against the TS source for the three types (EnumType.ts, StructType.ts, LocType.ts in `/Engine-TS/src/cache/config/`).

### 2. `script.Configs` interface + `state.go` addition

```go
// Configs is the config-type lookup surface that pkg/script needs for
// OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM handlers. Implementations
// return nil when the type isn't loaded or the id is out of range.
type Configs interface {
    ObjType(id int) *objtype.ObjType
    NpcType(id int) *objtype.NPCType
    LocType(id int) *objtype.LocType
    EnumType(id int) *objtype.EnumType
    StructType(id int) *objtype.StructType
    ParamType(id int) *objtype.ParamType
}
```

Add `Configs Configs` field to `ScriptState`. Wire it in `modules/world/script.go` next to `state.Provider` and `state.World`.

### 3. Shared `paramLookup` helper — `pkg/script/configs.go`

```go
// paramLookup reads params[paramID] (or ParamType defaults) and pushes
// onto the appropriate stack based on the ParamType's type.
// Shared by OC_PARAM, NC_PARAM, LC_PARAM, STRUCT_PARAM.
func paramLookup(s *ScriptState, params objtype.ParamMap, paramID int) error {
    pt := s.Configs.ParamType(paramID)
    if pt == nil {
        return fmt.Errorf("param %d not found", paramID)
    }
    isString := pt.Type == objtype.ScriptVarTypeString
    if v, ok := params[uint32(paramID)]; ok {
        if isString {
            s.PushString(v.(string))
        } else {
            s.PushInt(int(v.(uint32)))
        }
        return nil
    }
    // Fall through to ParamType defaults.
    if isString {
        s.PushString(pt.DefaultString)
    } else {
        s.PushInt(int(pt.DefaultInt))
    }
    return nil
}
```

### 4. Handlers — `pkg/script/handlers_config.go`

All 33 handlers follow a small number of shapes. The implementer writes each one, using the TS handlers as canonical reference. Key shapes:

**Simple int/string field reader** (most OC_/NC_/LC_ handlers):
```go
func handleOcName(s *ScriptState) error {
    id := s.PopInt()
    ot := s.Configs.ObjType(id)
    if ot == nil {
        return fmt.Errorf("OC_NAME: unknown obj id %d", id)
    }
    s.PushString(ot.Name)
    return nil
}
```

**Boolean field reader (push 0/1)**:
```go
func handleOcMembers(s *ScriptState) error {
    id := s.PopInt()
    ot := s.Configs.ObjType(id)
    if ot == nil {
        return fmt.Errorf("OC_MEMBERS: unknown obj id %d", id)
    }
    if ot.Members {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

**Param handler (uses shared helper)**:
```go
func handleOcParam(s *ScriptState) error {
    paramID := s.PopInt()
    objID := s.PopInt()
    ot := s.Configs.ObjType(objID)
    if ot == nil {
        return fmt.Errorf("OC_PARAM: unknown obj id %d", objID)
    }
    return paramLookup(s, ot.Params, paramID)
}
```

**NC_OP** is special — ops are indexed `1..5` in TS (1-based) and may be empty. Handler pops `(npcID, op)`, 1-based op index into `NpcType.Op[op-1]`, pushes empty string for OOB.

**ENUM** is the most complex:
```go
// TS pops [inputtype, outputtype, enumId, key] via popInts(4).
// Stack top = key. Returns enum.values[key] or defaults, dispatched
// by OutputType (int vs string).
```

**ENUM_GETOUTPUTCOUNT**: pops enumId, pushes `len(enum.Values)` (int).

**OC_CERT / OC_UNCERT**: these have pairwise item-cert logic (cert items are noted tradable versions of standard items). TS uses `certlink` / `certtemplate` fields. Implementer checks TS `OC_CERT.ts` and either full-implements or stubs with the source id passed through — document either way.

### 5. `modules/world/server_configs.go` — interface impl

Thin wrapper exposing the server's loaded configs:
```go
type serverConfigsView struct{ s *Server }

func (c serverConfigsView) ObjType(id int) *objtype.ObjType {
    if c.s == nil || c.s.objTypes == nil { return nil }
    if id < 0 || id >= len(c.s.objTypes.Configs) { return nil }
    return c.s.objTypes.Configs[id]
}
// ... same pattern for NpcType, LocType, EnumType, StructType, ParamType
```

### 6. Server + tick wiring

`NewServer` gains:
```go
enumTypes, err := objtype.LoadEnumTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load enum types: %w", err) }
structTypes, err := objtype.LoadStructTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load struct types: %w", err) }
locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load loc types: %w", err) }
s.enumTypes = enumTypes
s.structTypes = structTypes
s.locTypes = locTypes
s.configsView = serverConfigsView{s: s}
```

`runScript` sets `state.Configs = s.configsView`.

### 7. Testing

**Per-loader tests** (enumtype_test.go, structtype_test.go, loctype_test.go) — round-trip a synthetic blob through LoadXxxTypes and assert field decode. Follow the exact pattern of `varptype_test.go`.

**Handler tests** (`handlers_config_test.go`) — small `mockConfigs` struct that implements the `Configs` interface, seeded with a handful of ObjType/NpcType/LocType/EnumType/StructType/ParamType entries. Table-driven coverage of each of the 33 opcodes, including param int vs string dispatch via ENUM and `*_PARAM`.

**E2E test** (`modules/world/script_test.go`) — seed a fake `ObjType` named "Coins" at id 995, wire `s.configsView`, run a 1-instruction `OC_NAME` script, pop the string result, assert "Coins".

### 8. LOC estimate

| File | LOC |
|---|---|
| `pkg/objtype/enumtype.go` | 90 |
| `pkg/objtype/structtype.go` | 50 |
| `pkg/objtype/loctype.go` | 100 |
| `pkg/objtype/*_test.go` (3 files) | 180 |
| `pkg/script/configs.go` | 60 |
| `pkg/script/handlers_config.go` | 350 |
| `pkg/script/handlers_config_test.go` | 260 |
| `pkg/script/state.go` (diff) | +3 |
| `pkg/script/handlers.go` (diff) | +40 (register 33) |
| `modules/world/server.go` (diff) | +25 |
| `modules/world/server_configs.go` | 60 |
| `modules/world/script.go` (diff) | +1 |
| `modules/world/script_test.go` (diff) | +40 |
| **Total** | **~1260** |

Larger than prior sub-specs but mechanically repetitive (33 similar handlers). Manageable in one pass because there's no new runtime behavior — just config plumbing.

## Key design calls

- **Single `handlers_config.go` file** (~350 LOC) rather than splitting into 5 per-category files. Trade-off: single file is easier to grep and review; split is more diffable against TS upstream. One file wins on ergonomics at this size.
- **`Configs` interface on `ScriptState`** matches the `Provider`/`World` pattern. Keeps pkg/script decoupled from modules/world.
- **Shared `paramLookup` helper** absorbs the OC_PARAM / NC_PARAM / LC_PARAM / STRUCT_PARAM duplication. One failure mode if ParamType isn't loaded: return error (not silent default).
- **LocType loads server-side only.** TS decodes both server and client jagfiles; we skip client fields. If a handler needs a client-only field (e.g. LC_DESC relies on client decode), it returns "" and we document — implementer flags this if found.
- **No writes.** Every handler is a read. This sub-spec adds zero server mutations. Write ops (`OC_PARAM_SET`, etc.) aren't real TS ops; don't speculate.

## Gotchas

- **ENUM output dispatch**: implementer must check `enum.OutputType == ScriptVarTypeString` and push to the correct stack. Getting this wrong causes a type-mismatch at the next pop.
- **ParamType.DefaultInt is a `uint32`**, not int. Check `pkg/objtype/paramtype.go`. Handler does `int(pt.DefaultInt)` — OK as long as it fits. Document if any cache params exceed int32.
- **NC_OP / LC_OP / OC_OP**: TS handlers only exist for NC_OP (npc interaction verbs). `OC_OP` and `LC_OP` opcodes exist in `opcode.go` but aren't in S5d scope — don't register them. Flag for future.
- **Cert chains**: `OC_CERT` and `OC_UNCERT` follow item-cert pairs (`certlink` / `certtemplate` fields on ObjType). Implementer checks if those fields exist in Go's ObjType already; if not, add + decode + include in these handlers.
- **Heredoc `!=` bug**: Same as prior sub-specs — use Edit/Write tool for test code with `!=`, not `cat <<EOF` Bash heredocs.
- **loc.dat size**: real caches have thousands of locs. `LoadLocTypes` should tolerate a large count (4-byte length prefix) — check TS loader for the count width before coding.
