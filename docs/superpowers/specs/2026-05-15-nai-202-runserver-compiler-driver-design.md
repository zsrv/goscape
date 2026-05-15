# NAI-202: bytecode compiler arc — `runServerCompiler` driver port

**Date**: 2026-05-15
**Predecessor**: NAI-201 (`compiler-foundation registries`; closed at `c465948`). NAI-201 §12 enumerated the four prerequisite data registries this slice consumes (`NpcStatMap`, `NpcModeMap`, `ScriptOpcodeMap`, `ScriptOpcodePointers`). All four are present at HEAD.
**Arc step**: Third sub-spec of the bytecode compiler arc. NAI-200 shipped the symbol-table data type (`CompilerTypeInfo` → `TypeInfo`). NAI-201 shipped the four input registries. NAI-202 ships the driver that orchestrates them.

## 0. Pre-context: what NAI-202 ports and stops short of

TS source: `tools/pack/Compiler.ts:109-367` (the `runServerCompiler` function body).

Lines 109-329 ("build the `symbols` dict") are in scope. The final `CompileServerScript({ symbols })` call at lines 330-367 is **out of scope**: it hands the symbol table to the external `@lostcityrs/runescript` lexer/parser/typechecker/bytecode-emitter, which is its own multi-sub-spec arc (NAI-203+).

NAI-202 ships a single public function:

```go
func BuildSymbols(srcDir, dataPackDir string) (map[string]*TypeInfo, error)
```

Returning the 32-key symbol table the (deferred) `CompileServerScript` would consume. No `RunServerCompiler` wrapper, no compile hook — both deferred per design Q2 ("API shape").

## 1. Goal

Port `runServerCompiler`'s symbol-table assembly from TS to goscape in one slice. After this:

- `pkg/pack/compiler.BuildSymbols(srcDir, dataPackDir)` returns the same 32-category symbol table TS Compiler.ts:330-365 hands to `CompileServerScript`.
- All 22 `.pack` files are loaded via `compiler.Load` (NAI-200 surface).
- All 7 entity-type loaders contribute their enrichments: `InvType.Protect` → `writeinv.Protect`; `Component.ComName`/`Overlay` → `interface`/`overlayinterface`; `VarPlayerType.Type`/`Protect` → `varp.VarType`/`Protect`; `VarNpcType.Type` → `varn.VarType`; `VarSharedType.Type` → `vars.VarType`; `ParamType.Type` → `param.VarType`; `DbTableType` → `dbcolumn` synthesis (bitfield-encoded ids).
- The three meta `LoadMap` sources (`PlayerStatMap`, `NpcStatMap`, `NpcModeMap`) populate `stat` / `npc_stat` / `npc_mode` with `valueAsKey=true`.
- The two `LoadArray` sources (`fontmetrics` 4-entry, `locshape` 23-entry) populate `fontmetrics` / `locshape`.
- The inline `.constant` loader (TS Compiler.ts:152-173) ports as an unexported helper with TS-faithful loose-parsing semantics distinct from `pkg/pack.LoadConstants`.
- Two TS-source bugs (Compiler.ts:147 corrupt2-field typo; Compiler.ts:247 varn-loop-guard typo) are fixed with deviation tags + pin tests.
- Two NAI-201 carryforwards close: `PointerGroupFind` hardened against caller mutation; reverse-coverage test for `Op*` constants vs `ScriptOpcodeMap`.

After NAI-202, only the external `@lostcityrs/runescript` compiler itself blocks end-to-end script compilation.

## 2. Scope

**In**:

- New file `pkg/pack/compiler/symbols.go` — `BuildSymbols` + unexported helpers.
- New file `pkg/pack/compiler/symbols_test.go` — end-to-end + per-enrichment + parser unit tests.
- New file `pkg/pack/compiler/nai202_deviation_pins_test.go` — pins for the four NAI-202 deviation tags.
- Edit `pkg/script/opcode_pointers.go` — unexport `PointerGroupFind` to `pointerGroupFind [5]string`; expose `func PointerGroupFind() []string` returning a fresh copy. Update `corruptExceptActive` callsite.
- Edit `pkg/script/opcode_pointers_test.go` — adjust the existing `PointerGroupFind` references to use the accessor.
- New test in `pkg/script/opcode_map_test.go` — `TestScriptOpcodeMap_ReverseCoverage`: every `Op*` constant in `pkg/script/opcode.go` either appears as a value in `ScriptOpcodeMap` or is on an explicit `excludedOpcodes` allowlist with rationale.

**Out** (deferred):

- The external `@lostcityrs/runescript` `CompileServerScript` port — NAI-203+ arc (lexer/parser/typechecker/bytecode-emitter).
- A `RunServerCompiler` wrapper or compile-hook signature — premature; arrives with NAI-203.
- Producer-side packing for `script.pack` / `interface.pack` / `synth.pack` — these `.pack` files are consumed by `BuildSymbols` but produced upstream (outside goscape's current pack pipeline). Already consumed-only via `paramLookups` in `pkg/pack/pack_configs.go:693,699`; no producer needed in NAI-202.
- Adding a reverse `ScriptVarType` name lookup as a public method (e.g., `ScriptVarType.GetType()`). The TS callsite at Compiler.ts:241, 252, 262 calls `ScriptVarType.getType(varp.type)` which converts a type-code int back to its name string. `ParamType` already has a `GetType()` instance method (`pkg/objtype/paramtype.go:105`). Goscape mirrors that for `VarPlayerType` / `VarNpcType` / `VarSharedType` by reusing a single internal helper rather than adding methods — see §5.4.
- A CLI entry point. `BuildSymbols` is library API only; the eventual `pack` command lives outside this slice.

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]`).
- TS source canon: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]`). Specifically:
  - `tools/pack/Compiler.ts:109-329` — `runServerCompiler` body up to `CompileServerScript` call.
  - `tools/pack/Compiler.ts:152-173` — inline `.constant` loader.
  - `src/cache/config/ScriptVarType.ts:85-170` — `getType` reverse-lookup (referenced for vartype-string emission).
- Stdlib only; reuses existing goscape packages (`pkg/objtype`, `pkg/script`, `pkg/pack/compiler` from NAI-200).

## 4. Non-goals

- Wiring `BuildSymbols` into a production command. Zero new callers in `cmd/` or `modules/`.
- Generating `dbcolumn` ids via a `binary/encoding` helper. The TS expression `((table.id & 0xffff) << 12) | ((column & 0x7f) << 4) | ((tuple + 1) & 0xf)` ports as a literal Go expression mirroring the bit layout. Helper introduction deferred until a second site needs it.
- Parallelizing the loader phase. TS Compiler.ts runs strictly sequentially. Goscape mirrors that; the loaders are small fixed-cost reads (the entire pack-CLI startup phase is single-digit ms for current cache sizes).
- Introducing a `Symbols` wrapper struct around `map[string]*TypeInfo`. TS uses a plain object literal; goscape mirrors with a plain map. Strongly-typed accessors deferred until NAI-203 has a real consumer that benefits from them.
- Coverage of `excludedOpcodes` semantics for the reverse-coverage test. The allowlist starts empty; rationales accrue if specific opcodes prove TS-internal-only. NAI-201 spotchecks confirmed all 393 `ScriptOpcodeMap` entries map to existing `Op*` constants — see §7.10 expected starting allowlist size of 0.

## 5. Architecture

### 5.1 File layout

```
pkg/pack/compiler/
├── symbols.go                       (NEW)
├── symbols_test.go                  (NEW)
└── nai202_deviation_pins_test.go    (NEW)

pkg/script/
├── opcode_pointers.go               (EDIT — unexport PointerGroupFind)
├── opcode_pointers_test.go          (EDIT — adjust references)
└── opcode_map_test.go               (EDIT — add reverse-coverage test)
```

`pkg/pack/compiler/typeinfo.go` is **not** edited — NAI-200's `TypeInfo` surface is consumed as-is.

### 5.2 `BuildSymbols` orchestration

```go
// BuildSymbols ports TS runServerCompiler (Compiler.ts:109-329) up to the
// CompileServerScript call (deferred to NAI-203+). Returns the 32-key
// symbol-category dict consumed by the bytecode compiler's typechecker.
//
// srcDir: contains scripts/ and pack/ subdirs.
// dataPackDir: contains client/ and server/ subdirs with cache .dat/.idx.
func BuildSymbols(srcDir, dataPackDir string) (map[string]*TypeInfo, error) {
    symbols := map[string]*TypeInfo{}

    // 1. commandInfo from ScriptOpcodeMap + ScriptOpcodePointers
    // 2. constantInfo from <srcDir>/scripts/**/*.constant
    // 3. 22 .pack file Load calls
    // 4. writeinv synth (InvType.Protect)
    // 5. interface/overlay synth (Component.ComName / Overlay)
    // 6. varp/varn/vars/param vartype + protect enrichments
    // 7. dbcolumn synth (DbTableType columns + bitfield ids)
    // 8. stat / npc_stat / npc_mode via LoadMap valueAsKey=true
    // 9. fontmetrics / locshape via LoadArray
    // 10. assemble the 32-key map

    return symbols, nil
}
```

Implementation strategy: each numbered phase is a contiguous block in the function body. The function is long (~200 LOC) but flat — no abstraction beyond unexported helpers for the four passes that warrant them (`populateCommandInfo`, `populateInterfaceOverlay`, `populateDbColumns`, `loadCompilerConstants`). This mirrors TS Compiler.ts:109-329's flat layout and supports side-by-side review per `[[flat_arg_signature_for_cross_lang_parity]]`.

Error handling: any sub-step error short-circuits with `return nil, fmt.Errorf("BuildSymbols: <phase>: %w", err)`. Missing optional `.pack` files are handled by NAI-200's `Load` (returns empty `*TypeInfo`, nil err); missing cache `.dat/.idx` files are loader-specific (some return empty, some error) — `BuildSymbols` propagates whatever the loader returns.

### 5.3 commandInfo build (TS Compiler.ts:110-150)

```go
// populateCommandInfo iterates ScriptOpcodeMap in ascending opcode-value
// order and applies ScriptOpcodePointers enrichments. Mirrors TS
// Compiler.ts:110-150 (allCommands + commandInfo build).
//
// NAI-202-D-CORRUPT2-FIELD: TS Compiler.ts:147 assigns the corrupt2 string
// to commandInfo.corrupt[opcode] instead of commandInfo.corrupt2[opcode]
// (typo: same array name on lines 144 and 147). Goscape fixes this; pin
// test in nai202_deviation_pins_test.go anchors the corrected behaviour.
func populateCommandInfo(info *TypeInfo) {
    type entry struct {
        name   string
        opcode script.Opcode
    }
    entries := make([]entry, 0, len(script.ScriptOpcodeMap))
    for n, op := range script.ScriptOpcodeMap {
        entries = append(entries, entry{n, op})
    }
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].opcode < entries[j].opcode
    })

    for _, e := range entries {
        info.Add(int(e.opcode), strings.ToLower(e.name), true)
        ptrs, hasPtrs := script.ScriptOpcodePointers[e.opcode]
        if !hasPtrs {
            continue
        }
        op := int(e.opcode)
        if len(ptrs.Require) > 0 {
            info.Require[op] = strings.Join(ptrs.Require, ",")
            if len(ptrs.Require2) > 0 {
                info.Require2[op] = strings.Join(ptrs.Require2, ",")
            }
        }
        if len(ptrs.Set) > 0 {
            if ptrs.Conditional {
                info.Conditional[op] = true
            }
            info.Set[op] = strings.Join(ptrs.Set, ",")
            if len(ptrs.Set2) > 0 {
                info.Set2[op] = strings.Join(ptrs.Set2, ",")
            }
        }
        if len(ptrs.Corrupt) > 0 {
            info.Corrupt[op] = strings.Join(ptrs.Corrupt, ",")
            if len(ptrs.Corrupt2) > 0 {
                info.Corrupt2[op] = strings.Join(ptrs.Corrupt2, ",")
            }
        }
    }
}
```

Truthiness convention: TS uses `if (pointers.require)` (truthy on non-undefined). Goscape uses `len(ptrs.Require) > 0`. NAI-201's `ScriptOpcodePointers` map uses the `nil = absent` convention (no entry uses an explicit `[]string{}`), so `len > 0` and `!= nil` give identical results on current data. `len > 0` is preferred for robustness.

### 5.4 Constant loader (TS Compiler.ts:152-173)

```go
// loadCompilerConstants walks <scriptsDir> recursively for *.constant
// files and returns a flat map of constant name → raw textual value.
// Mirrors TS Compiler.ts:152-173 exactly — a TS-faithful loose parser
// distinct from pkg/pack.LoadConstants (which is stricter: dedup-error,
// no surrounding-quote strip, splits on first '=' via strings.Cut).
//
// NAI-202-D-CONSTANT-LOOSE-PARSER: this is goscape's second .constant
// parser. TS has two semantically different parsers (PackShared.ts:262
// strict; Compiler.ts:152 loose). Goscape mirrors that two-parser shape
// rather than collapsing them.
//
// Per-line rules (TS-faithful, in order):
//   - empty line                       → skip
//   - line starts with "//"            → skip
//   - split on "=", take first segment as name (parts[0]),
//     second as value (parts[1]). Anything past the second "=" is
//     discarded (mirrors TS const [name, value] = line.split('=')).
//   - trim name and value (whitespace)
//   - if name starts with "^", strip the leading "^"
//   - if value starts with `"` AND ends with `"`, strip both
//   - assign m[name] = value (last writer wins; no dedup error)
//
// A line with no "=" produces an out-of-range index on parts[1] in TS
// (throws). Goscape matches by returning an error wrapping the file path
// and offending line.
func loadCompilerConstants(scriptsDir string) (map[string]string, error)
```

This parser is callsite-only for `BuildSymbols`. Not exported.

`loadCompilerConstants` does not reuse `pack.LoadConstants` because:
- `LoadConstants` errors on duplicate name; Compiler.ts allows last-writer-wins.
- `LoadConstants` does not strip surrounding `"` from values; Compiler.ts does.
- `LoadConstants` uses `strings.Cut` (concatenates everything after first `=`); Compiler.ts uses unbounded `split` then discards parts past second segment.

Walking the directory uses the existing `pack.LoadDirExtFull(scriptsDir, ".constant", cb)` (per `[[plan_grep_helper_patterns]]` — the helper is in place at `pkg/pack/parse.go:162`).

### 5.5 Enrichment loops (TS Compiler.ts:203-273)

Five loops share the same shape:

```go
for id := 0; id <= info.Max; id++ {
    if _, ok := info.Map[id]; !ok {
        continue
    }
    // enrich info.<aux>[id] from the corresponding entity-type loader
}
```

Mapping:

| TS line range | Source loader (goscape) | TypeInfo enrichment |
|---|---|---|
| 203-212 | `objtype.LoadInvTypes(dataPackDir)` | `writeinv.Protect[id] = inv.Protect` |
| 214-232 | `objtype.LoadComponentTypes(dataPackDir)` | `interface.Map[id] = name`; if `com.Overlay`, `overlay.Map[id] = name`; name = `com.ComName` if non-empty else `componentInfo.Map[id]` |
| 234-243 | `objtype.LoadVarpTypes(dataPackDir)` | `varp.VarType[id] = scriptVarTypeName(varp.Type)`; `varp.Protect[id] = varp.Protect` |
| 245-253 | `objtype.LoadVarnTypes(dataPackDir)` | `varn.VarType[id] = scriptVarTypeName(varn.Type)` |
| 255-263 | `objtype.LoadVarsTypes(dataPackDir)` | `vars.VarType[id] = scriptVarTypeName(vars.Type)` |
| 265-273 | `objtype.LoadParamTypes(dataPackDir)` | `param.VarType[id] = param.GetType()` |

`scriptVarTypeName(t objtype.ScriptVarType) string` is a single unexported helper in `symbols.go` mirroring the same switch as `ParamType.GetType()` (which already exists at `pkg/objtype/paramtype.go:105`). The choice to inline this helper rather than add methods to `VarPlayerType`/`VarNpcType`/`VarSharedType` keeps the type-name lookup in one place; the alternative (three new instance methods + one existing on `ParamType`) would scatter identical switch statements across four files.

**Component fallback semantics** (TS Compiler.ts:225): if `com == null` (loader returned nil for that id), the TS loop `continue`s. Goscape: `objtype.ComponentTypeConfigs.Configs[id] == nil` triggers the same skip. The fallback `name = com.comName || componentInfo.map[id]` ports as `if com.ComName != "" { name = com.ComName } else { name = componentInfo.Map[id] }`.

**NAI-202-D-VARN-LOOP-GUARD**: TS Compiler.ts:247 reads `if (typeof varpInfo.map[id] === 'undefined')` (typo: should be `varnInfo`). The bug means: varn ids absent from varpInfo.map get skipped, even if present in varnInfo.map. Concretely — for any varn id N where N has no varp at the same id N, no varn vartype is recorded. Goscape fixes this; pin test below.

### 5.6 dbcolumn synthesis (TS Compiler.ts:275-297)

```go
// populateDbColumns synthesizes the dbcolumn TypeInfo from DbTableType
// column metadata. Mirrors TS Compiler.ts:275-297.
//
// Bitfield-encoded column ids:
//   - primary id  = (table.id & 0xffff) << 12 | (column & 0x7f) << 4
//   - tuple id    = primary | ((tuple + 1) & 0xf)     // only if len(types) > 1
//
// .Add is called with updateMax=false on all entries — dbcolumnInfo.Max
// stays at -1 (matching TS Compiler.ts:286,292 third arg).
func populateDbColumns(info *TypeInfo, tables *objtype.DbTableTypeConfigs)
```

Tuple entries are only emitted when `len(table.Types[column]) > 1`. The vartype string for the primary id is the comma-joined list of all type names; tuple-id vartype is the single tuple's type name.

Examples (verified vs TS):
- table id 1, column 0, types `[INT]`: primary id = `(1 << 12) | (0 << 4) = 0x1000` = 4096, vartype = `"int"`. No tuple entries.
- table id 1, column 0, types `[INT, OBJ]`: primary id = 4096, vartype = `"int,obj"`. Tuple entries: id = 4096 | 1 = 4097 with vartype "int"; id = 4096 | 2 = 4098 with vartype "obj".
- table id 2, column 5, types `[STRING]`: primary id = `(2 << 12) | (5 << 4) = 0x2050` = 8272, vartype = "string". No tuples.

### 5.7 Meta and static-array symbols (TS Compiler.ts:300-328)

```go
symbols["stat"]     = LoadMap(toStringIntMap(objtype.PlayerStatMap), true)
symbols["npc_stat"] = LoadMap(toStringIntMap(objtype.NpcStatMap), true)
symbols["npc_mode"] = LoadMap(toStringIntMap(objtype.NpcModeMap), true)
symbols["fontmetrics"] = LoadArray([]string{"p11", "p12", "b12", "q8"})
symbols["locshape"]    = LoadArray([]string{
    "wall_straight", "wall_diagonalcorner", "wall_l", "wall_squarecorner",
    "walldecor_straight_nooffset", "walldecor_straight_offset",
    "walldecor_diagonal_offset", "walldecor_diagonal_nooffset",
    "walldecor_diagonal_both", "wall_diagonal",
    "centrepiece_straight", "centrepiece_diagonal",
    "roof_straight", "roof_diagonal_with_roofedge", "roof_diagonal",
    "roof_l_concave", "roof_l_convex", "roof_flat",
    "roofedge_straight", "roofedge_diagonalcorner", "roofedge_l",
    "roofedge_squarecorner",
    "grounddecor",
})
```

`LoadMap` and `LoadArray` already exist in NAI-200's `typeinfo.go`. `toStringIntMap` is a one-line conversion if the registries are typed as `map[string]int` already (they are — see `pkg/objtype/npcstat.go:10`, `pkg/objtype/npcmode.go:14`, `pkg/objtype/playerstat.go:37`). No conversion shim needed; pass them directly.

`locshape` has 23 entries (verified vs TS Compiler.ts:304-328). `fontmetrics` has 4 entries (verified vs TS Compiler.ts:303).

### 5.8 Final symbol-map assembly (TS Compiler.ts:330-365)

The 32 keys, in the exact order TS hands them to `CompileServerScript`:

```
command, constant, npc, obj, inv, writeinv, seq, idk, spotanim, loc,
component, interface, overlayinterface, varp, varn, vars, param, struct,
enum, hunt, mesanim, synth, category, runescript, dbtable, dbcolumn,
dbrow, stat, npc_stat, npc_mode, fontmetrics, locshape
```

Map insertion order is unobservable to Go consumers, so the literal order in `symbols.go` matches TS for review purposes only.

`runescript` is loaded from `<srcDir>/pack/script.pack` (TS Compiler.ts:197 — note the file name is `script.pack` but the symbol key is `runescript`).

### 5.9 `PointerGroupFind` hardening

```go
// pkg/script/opcode_pointers.go

// pointerGroupFind is the 5-element find_* pointer-name list. Unexported
// to prevent caller mutation; access through PointerGroupFind() which
// returns a fresh slice copy. Internal callers (corruptExceptActive)
// slice the array directly.
var pointerGroupFind = [5]string{
    "find_player", "find_npc", "find_loc", "find_obj", "find_db",
}

// PointerGroupFind returns a fresh slice copy of the find_* pointer-name
// list. Returning a copy (not pointerGroupFind[:]) ensures callers cannot
// mutate the package-internal state.
func PointerGroupFind() []string {
    out := make([]string, len(pointerGroupFind))
    copy(out, pointerGroupFind[:])
    return out
}

func corruptExceptActive(extras ...string) []string {
    out := make([]string, 0, len(pointerGroupFind)+len(extras))
    out = append(out, pointerGroupFind[:]...)
    out = append(out, extras...)
    return out
}
```

Existing test references update from `PointerGroupFind` (var) to `PointerGroupFind()` (func) or to length-check via `len(pointerGroupFind)` inside the same package. The two extended-spread literal expansions at NPC_DELAY and NPC_ARRIVEDELAY (see NAI-201 §10) continue to spell out the five names verbatim — they don't reference the variable.

This is a breaking change to an exported name, but NAI-201 close confirmed zero external consumers (the registries are foundation, no production callers). Internal test consumers in `pkg/script/opcode_pointers_test.go` are the only callers to update.

### 5.10 Reverse-coverage test

```go
// pkg/script/opcode_map_test.go

// excludedOpcodes lists Op* constants intentionally absent from
// ScriptOpcodeMap (e.g., internal-only opcodes the script source can
// never reference by name). Empty at NAI-202 land; entries get added
// only with a justifying comment.
var excludedOpcodes = map[Opcode]string{}

// TestScriptOpcodeMap_ReverseCoverage pins: every Op* constant declared
// in pkg/script/opcode.go either appears as a value in ScriptOpcodeMap
// or is explicitly listed in excludedOpcodes with rationale. Catches the
// failure mode where a new Op* constant is added without the
// corresponding ScriptOpcodeMap entry.
func TestScriptOpcodeMap_ReverseCoverage(t *testing.T)
```

Implementation: enumerate the `Op*` values via the existing range-bounds anchor (`TestScriptOpcodePointers_KeysAreBoundedOpcodes` uses `OpTimeSpent` as the upper bound). Hand-list is acceptable here — Go has no built-in reflection over package-level constants. Alternative: iterate every value in the closed range `[OpPushConstantInt, OpTimeSpent]` and use `Opcode.String()` to detect "named" entries; opcodes whose String returns the generic `Opcode(N)` form aren't named in the source. Plan picks the implementation; spec only requires the test exists and is green.

## 6. Error handling

- `BuildSymbols` propagates errors from each sub-step with phase-tagged wrap (`fmt.Errorf("BuildSymbols: %s: %w", phase, err)`).
- `loadCompilerConstants`: an empty scripts dir → empty map, nil err (`pack.LoadDirExtFull` already silent-on-missing). A malformed line missing `=` → error with file path and offending line.
- Entity-type loaders: pass through. `LoadInvTypes` etc. each have their own error semantics (some return empty on missing file, some error). NAI-202 does not normalize these.
- `populateInterfaceOverlay`: nil entry in `Configs[id]` → skip (TS `if (!com) continue` parity). Out-of-range id (shouldn't happen given the `<= Max` loop, but defensive): skip.
- `populateDbColumns`: nil `Types[column]` → skip that column. Column index `> 0x7f` (bitfield overflow): error (the bitmask `& 0x7f` would silently corrupt the encoding). Cache data is bounded; this is a defensive guard.

## 7. Testing

### 7.1 End-to-end smoke (`TestBuildSymbols_EndToEnd`)

Seed a tmpdir with:
- `<srcDir>/scripts/foo.constant` containing `^MAX_HEALTH=99` and `THEME="dark"`.
- `<srcDir>/pack/<type>.pack` for all 22 types: each with one `id=name` entry.
- `<dataPackDir>/server/{inv,interface,varp,varn,vars,param,dbtable}.dat` + `.idx` where required: minimal binary fixtures with one entry per loader, plus matching client-side jagfile entries for varp/component.

Assert:
- `len(symbols) == 32`, every category key present.
- `symbols["command"]` has 393 entries (one per `ScriptOpcodeMap` entry).
- `symbols["constant"]` has 2 entries: `MAX_HEALTH=99`, `THEME=dark` (quote-stripped).
- `symbols["writeinv"].Protect[id]` matches the seed `inv.protect`.
- `symbols["interface"].Map[id]` populated; `symbols["overlayinterface"].Map[id]` populated only for the overlay-true component.
- `symbols["varp"].VarType[id]` matches the seed type name; `symbols["varp"].Protect[id]` matches.
- `symbols["dbcolumn"]` has at least one bitfield-encoded primary id (`>= 4096` for table id 1 column 0).
- `symbols["stat"].NameMap["0"] == "attack"`, etc. (LoadMap valueAsKey=true).
- `symbols["fontmetrics"].Map[0] == "p11"` and `symbols["locshape"].Map[0] == "wall_straight"`.

### 7.2 `populateCommandInfo` unit (`TestPopulateCommandInfo_Iteration`)

- Seed a fake-data variant: build `populateCommandInfo` against a synthetic Pointers map (using a local-scope override pattern is not possible since `ScriptOpcodePointers` is a package var; instead the test exercises against the real map and pins selected opcodes whose Pointers are byte-stable per NAI-201 spotchecks).
- Assert ascending opcode-value iteration order is observable in `info.Map` (Max == max Op + 1).
- Assert `info.Require[OpSomeOp]` matches the expected comma-joined string for an opcode with a known Require list.

### 7.3 Constant parser (`TestLoadCompilerConstants_*`)

- `_StripsLeadingCaret`: `^FOO=bar` → `m["FOO"] == "bar"`.
- `_StripsSurroundingQuotes`: `K="v"` → `m["K"] == "v"`; `K=v` → `m["K"] == "v"`; `K="v"x"` → unchanged middle quote (no strip, mismatched outer).
- `_LastWriterWins`: two files each with `K=a` and `K=b` → final map has `K=b` (file walk order is filesystem-dependent; test uses two entries in the same file to deterministically assert last-line-wins).
- `_SkipsComments`: `// K=a` line skipped.
- `_DiscardsPastSecondEquals`: `K=v=extra` → `m["K"] == "v"` (TS-faithful: split + take parts[0:2]).
- `_ErrorsOnMissingEquals`: a `K` line with no `=` returns wrapped error with file path.
- `_TrimsWhitespace`: `  K  =  v  ` → `m["K"] == "v"`.
- `_EmptyScriptsDir`: missing dir → empty map, nil err.

### 7.4 Interface/overlay synth (`TestPopulateInterfaceOverlay_*`)

- `_PrefersComName`: component with `ComName="myInterface"` → `interface.Map[id] = "myInterface"`, ignoring the `componentInfo.Map[id]` fallback.
- `_FallsBackToComponentInfoMap`: component with `ComName=""` → `interface.Map[id] = componentInfo.Map[id]`.
- `_OverlayOnlyOnTrue`: two components, one overlay=true one overlay=false → `overlay.Map` contains only the true id.
- `_SkipsNilConfig`: id present in `componentInfo.Map` but `Configs[id] == nil` → no `interface.Map` entry for that id.

### 7.5 dbcolumn synth (`TestPopulateDbColumns_*`)

- `_SingleTypeColumn`: table id 1, column 0, types `[INT]` → one entry at id 4096 with vartype "int", no tuples.
- `_MultiTypeColumn`: table id 1, column 0, types `[INT, OBJ]` → entry at 4096 with vartype "int,obj"; tuple entries at 4097 ("int") and 4098 ("obj").
- `_BitfieldEncoding`: pin specific id values for a table with id=2, column=5, types=`[STRING]` (expect 8272).
- `_MaxUnchanged`: `dbcolumn.Max == -1` after all adds (updateMax=false).
- `_DebugNameAndColumnNameFormat`: vartype-string-irrelevant; the name format is `"<debugname>:<columnname>"` for primary, `"<debugname>:<columnname>:<tuple>"` for tuple — assert via `info.Map[id]` content.

### 7.6 Enrichment loops cover-all (`TestBuildSymbols_EnrichmentParity`)

A single integration test that seeds the tmpdir with two entries per entity-type-loaded category and asserts the enrichment map size matches the source loader's count. Catches off-by-one in any `<= Max` boundary.

### 7.7 Deviation pin tests (`nai202_deviation_pins_test.go`)

- **`TestNAI202_CORRUPT2_FIELD_PinFix`**: pick an opcode with both `Corrupt` and `Corrupt2` non-empty (e.g., `OpProtect` — confirm at plan-write via grep). Assert `info.Corrupt[op] != info.Corrupt2[op]` AND `info.Corrupt2[op] == strings.Join(ptrs.Corrupt2, ",")`. Test name string contains `NAI-202-D-CORRUPT2-FIELD`.
- **`TestNAI202_VARN_LOOP_GUARD_PinFix`**: seed varn with an id N that has no corresponding varp at N. Assert `symbols["varn"].VarType[N]` is populated. Test name contains `NAI-202-D-VARN-LOOP-GUARD`.
- **`TestNAI202_CONSTANT_LOOSE_PARSER_PinDivergence`**: seed `<srcDir>/scripts/dups.constant` with two lines `K=a` and `K=b`. Assert `symbols["constant"].NameMap["K"] == "b"` (no error, last-writer-wins). Per `[[pin_test_self_trigger_production_doc]]`, the test docstring references the spec concept "loose parser" not the TS identifier `loadDirExtFull`.
- **`TestNAI202_POINTER_GROUP_FIND_HARDENED_PinAccessor`**: assert the accessor returns the expected 5-element slice; assert mutating the returned slice does not affect subsequent calls.

### 7.8 Reverse-coverage (`TestScriptOpcodeMap_ReverseCoverage`)

Lives in `pkg/script/opcode_map_test.go`. Iterates the `Op*` constants in the closed range `[0, OpTimeSpent]`. For each value, uses `Opcode(i).String()` to detect "named" entries (Go's stringer-style fallback for unnamed values returns `Opcode(N)`). Every named entry must either be a value in `ScriptOpcodeMap` or in `excludedOpcodes`. Expected starting allowlist: empty.

### 7.9 Existing-test adjustments

- `pkg/script/opcode_pointers_test.go` currently references `PointerGroupFind` as a var. Update to `PointerGroupFind()` (with parens) at every reference. The internal helper `corruptExceptActive` test (NAI-201 §7.10 pin) continues to work — it tests the helper's output, not the internal array variable.

### 7.10 Coverage targets

No coverage-percentage gate. The fixtures above exercise every numbered phase in `BuildSymbols` § 5.2 1-9 plus every deviation-fix path.

## 8. Open questions

None at spec-write. All four design decisions are recorded with rationale (see top-of-file Q&A).

## 9. Resolved risks

### R1: TS-bug-for-bug-vs-correct decision

Decision: fix both bugs with deviation tags + pin tests (design Q1). Rationale: TS intent is unambiguous in both cases (the typo on `corrupt[opcode] = corrupt2.join(',')` overwrites a value just-written one line above; the `varpInfo.map[id]` check inside a `varnInfo.max` loop is a copy-paste error). Goscape's port is "correct-by-intent" per `[[true_to_ts_gate]]`. Both deviations have pin tests anchoring the fixed behaviour.

### R2: Two `.constant` parsers in goscape

Decision: ship a new inline parser in `pkg/pack/compiler/` (design Q3). Rationale: TS has two semantically-different parsers across two files (`PackShared.ts:262-289` strict for substitution; `Compiler.ts:152-173` loose for compile-time symbols). Unifying them in goscape would force `LoadConstants` to gain optional knobs (quote-strip, dedup-policy, split-policy) for a single second caller. Two parsers mirror the two-parser TS reality and stay self-documenting.

### R3: `runescript` symbol-key vs `script.pack` filename

Verified at spec-write: TS Compiler.ts:197 loads `<srcDir>/pack/script.pack` into a variable named `runescriptInfo`, and Compiler.ts:356 maps it under the key `'runescript'` in the symbols dict. The file/key name asymmetry is intentional TS-side; goscape mirrors it. Plan must not "fix" the filename or the key.

### R4: Reverse `ScriptVarType` name lookup

`ParamType` already has `GetType() string` (`pkg/objtype/paramtype.go:105`). `VarPlayerType`/`VarNpcType`/`VarSharedType` do not. NAI-202 introduces an unexported package-local helper `scriptVarTypeName(t objtype.ScriptVarType) string` in `pkg/pack/compiler/symbols.go` rather than adding three new instance methods. Decision rationale: single switch in one place is easier to maintain than four near-identical switches (the existing `ParamType.GetType()` is its own switch but stays as-is for backwards compat).

### R5: `PointerGroupFind` API break

NAI-201 close confirmed zero production consumers. The only callers are `corruptExceptActive` (same file) and tests in `opcode_pointers_test.go` (same package). API break is contained. No external `pkg/script` consumers exist per grep at spec-write.

### R6: `excludedOpcodes` allowlist starting size

Expected size 0. NAI-201 §6 spotchecks confirmed all 393 `ScriptOpcodeMap` entries map to existing `Op*` constants. The reverse direction — `Op*` constants not in the map — depends on whether `pkg/script/opcode.go` defines opcodes beyond the 393 named TS-side. Plan must grep at plan-write to count actual `Op*` constants in `opcode.go` and confirm the test expectation. If `Op*` count > 393, the spec-prescribed allowlist starts empty; the plan documents which constants populate the initial allowlist with rationale.

### R7: TS line `Compiler.ts:204-212` `writeinvInfo` is re-loaded from `inv.pack`

TS Compiler.ts:204 reads `inv.pack` a SECOND time (line 179 already loaded it as `invInfo`). The result is a separate `writeinvInfo` `CompilerTypeInfo`. The TS comment at 202 is "load extra context for compiler". The two `CompilerTypeInfo`s are observably identical except `writeinvInfo` has `protect[id]` set. Goscape can either:
- Re-load `inv.pack` (TS-faithful, allocates two TypeInfo).
- Deep-copy `invInfo` to `writeinvInfo` then enrich.

Decision: re-load. TS-faithful, costs one extra file read of a small file. Avoids accidental aliasing if `invInfo` is mutated downstream. No deviation tag needed (behavioural parity preserved).

### R8: TS loop-bound off-by-one verification

TS pattern `for (let id = 0; id <= info.max; id++)` with `info.max = lastId + 1` means the loop visits `info.max` once with an intentionally-empty slot. Goscape's `TypeInfo.Add` (NAI-200 `typeinfo.go:96`) sets `Max = id + 1`. The loop `for id := 0; id <= info.Max; id++` thus visits one slot past the highest assigned id. The `_, ok := info.Map[id]; !ok { continue }` guard makes this safe. Verified against NAI-200 spec §9 R3.

## 10. Deviations enumerated

- **`NAI-202-D-CORRUPT2-FIELD`** — TS Compiler.ts:146-147 has a typo: the `corrupt2` arm assigns `commandInfo.corrupt[opcode] = pointers.corrupt2.join(',')`, overwriting the just-written `corrupt[opcode]` instead of populating `corrupt2[opcode]`. Goscape fixes by writing to `info.Corrupt2[op]` instead. Pin test in `nai202_deviation_pins_test.go` anchors the fix: an opcode with both `Corrupt` and `Corrupt2` non-empty produces distinct strings in the two maps.
- **`NAI-202-D-VARN-LOOP-GUARD`** — TS Compiler.ts:247 reads `if (typeof varpInfo.map[id] === 'undefined')` inside a loop bounded by `varnInfo.max` (copy-paste error from the varp loop two lines above). Effect: varn ids absent from `varpInfo.map` get skipped. Goscape fixes by reading `varnInfo.Map[id]`. Pin test anchors the fix.
- **`NAI-202-D-CONSTANT-LOOSE-PARSER`** — goscape introduces a second `.constant` parser (`loadCompilerConstants` in `pkg/pack/compiler/symbols.go`) distinct from `pkg/pack.LoadConstants`. The Compiler-side parser is TS-faithful: last-writer-wins, surrounding-quote strip on values, split-take-first-two-segments on `=`. Pin test asserts the dup-last-writer-wins behaviour and the quote-strip.
- **`NAI-202-D-POINTER-GROUP-FIND-HARDENED`** — `pkg/script.PointerGroupFind` migrates from an exported `[]string` var to an unexported `[5]string` array + exported accessor `PointerGroupFind() []string` returning a fresh copy. Carryforward from NAI-201 final review; defends against caller mutation of package state.

No bug-for-bug ports are taken.

## 11. Carry-forward (from prior NAI sub-specs)

Per `[[nai_followups]]` audit at spec-write:

- **NAI-191 #1 `LoadFileFull` `TrimLeft` Unicode narrowing** — not on NAI-202 hot path (`loadCompilerConstants` does its own line iteration via `LoadDirExtFull` callback). Leave deferred.
- **NAI-191 #3 `ShouldBuildFileAny` `ReadDir` failure returns false** — not used by NAI-202. Leave deferred.
- **NAI-198 #1 OPOBJ2 upstream reconciliation** — upstream-engagement track; orthogonal.
- **NAI-199 #1 `frame_del` `endsWith` suffix-match TS-parity** — orthogonal.
- **NAI-200 deviations** (`NAI-200-D-DUAL-MAP`) — consumed-as-is in NAI-202. No edit to `pkg/pack/compiler/typeinfo.go`. No count-drift risk in that file's doc-comments.
- **NAI-201 deviations** (`NAI-201-D-NPCMODE-QUEUE-TODO`, `NAI-201-D-POINTERS-SPREAD-HELPER`) — consumed-as-is. The `NAI-202-D-POINTER-GROUP-FIND-HARDENED` deviation modifies `corruptExceptActive`'s internal access pattern (slice the unexported array) but preserves the helper's behavioural semantics. NAI-201 §10 pin test still passes unchanged.

Two NAI-201 final-review follow-ups are pulled into NAI-202 (design Q4):
- Reverse-coverage test for `Op*` constants (§5.10 / §7.8).
- `PointerGroupFind` hardening (§5.9 / `NAI-202-D-POINTER-GROUP-FIND-HARDENED`).

One NAI-201 final-review item is **not** addressed: "The `OpTimeSpent` max-bound in `TestScriptOpcodePointers_KeysAreBoundedOpcodes` will need updating if NAI-202 adds higher-valued opcodes." NAI-202 does not add new opcodes — it consumes the existing `Op*` constants. The max-bound stays at `OpTimeSpent`.

No new NAI-202-bound carry-forwards introduced.

## 12. Arc next step

NAI-202 unblocks the external `@lostcityrs/runescript` `CompileServerScript` port, which is the bytecode lexer/parser/typechecker/bytecode-emitter. That work is multi-sub-spec on its own — the TS package is ~10K LOC. Likely arc:

- **NAI-203**: lexer + token stream (`@lostcityrs/runescript/src/lexer`).
- **NAI-204**: parser + AST (`@lostcityrs/runescript/src/parser`).
- **NAI-205**: type checker + symbol resolution (consumes NAI-202's symbol table).
- **NAI-206**: bytecode emitter (writes `script.dat` / `script.idx`).
- **NAI-207**: top-level `CompileServerScript` driver + `RunServerCompiler` wrapper that wires `BuildSymbols` to the compile pipeline.

NAI-203's spec-write should grep `pkg/pack/compiler.BuildSymbols` to confirm the symbol-table shape hasn't drifted from this slice.

## 13. Acceptance criteria

- `pkg/pack/compiler/symbols.go` exists with exported `BuildSymbols(srcDir, dataPackDir string) (map[string]*TypeInfo, error)`.
- `pkg/pack/compiler/symbols_test.go` exists with all tests enumerated in §7.1–7.6 passing.
- `pkg/pack/compiler/nai202_deviation_pins_test.go` exists with the four pin tests in §7.7 passing.
- `pkg/script/opcode_pointers.go` has `pointerGroupFind` unexported (array) and `PointerGroupFind()` accessor (returns fresh copy). `corruptExceptActive` updated to slice the unexported array.
- `pkg/script/opcode_map_test.go` has `TestScriptOpcodeMap_ReverseCoverage` passing with `excludedOpcodes` allowlist starting empty (or with documented entries justified at plan-write).
- `go test ./pkg/pack/compiler/... ./pkg/script/... ./pkg/objtype/...` passes (cleanly, `-race`).
- `go test ./...` passes (no regressions; `BuildSymbols` has no production consumers in this slice).
- Deviation pin grep returns ≥1 match for each of: `NAI-202-D-CORRUPT2-FIELD`, `NAI-202-D-VARN-LOOP-GUARD`, `NAI-202-D-CONSTANT-LOOSE-PARSER`, `NAI-202-D-POINTER-GROUP-FIND-HARDENED`.
- Code reviewer can read `BuildSymbols` body alongside `Compiler.ts:109-329` and verify phase-by-phase parity in one sitting.
