# NAI-200: bytecode compiler arc opener — `CompilerTypeInfo` foundation

**Date**: 2026-05-14
**Predecessor**: NAI-199 (`category.dat` + `frame_del.dat` writers; closed at `93ede42`; PackShared per-config + specials layer 18/18 complete).
**Arc opener**: First sub-spec of the bytecode arc that will eventually close `regenScriptPack` → `script.pack` → `script.dat` end-to-end. NAI-200 ports the **symbol-table data structure** that every downstream arc step writes into (NAI-201 populator) and reads from (NAI-202+ compiler).

## 0. Pre-context correction (scope-gate)

The original NAI-200 dispatch framed `tools/pack/Compiler.ts:1-368` as "lexer/parser/typechecker scaffolding" — "likely lexer + token types only". Audit at brainstorm-time reverses the framing:

- `Compiler.ts:21-107` is a small data type, `CompilerTypeInfo`, with five loaders.
- `Compiler.ts:109-367` is `runServerCompiler()` — a glue function that builds one `CompilerTypeInfo` per symbol category and hands them to `CompileServerScript({symbols: {...}})`.
- `CompileServerScript` is imported from the external `@lostcityrs/runescript` npm package (shipped in `Engine-TS/node_modules/@lostcityrs/runescript/dist/runescript.js` as compiled output). **That** package contains the actual lexer / parser / typechecker — Compiler.ts itself contains zero tokenizer or parser code.
- Pre-context also referenced `tools/pack/PackScript.ts` as a downstream-deferred file. That path **does not exist** in `Engine-TS/tools/pack/`. The script-pack call site flows via `PackAll.ts` → `runServerCompiler()` → `CompileServerScript`. Adjust spec §12 accordingly.

NAI-200 narrows scope to **`CompilerTypeInfo` only** (lines 21-107). The driver (lines 109-367) and the external `@lostcityrs/runescript` compiler port are routed to NAI-201 and NAI-202+ respectively. This is a `[[scope_gate_prerequisite_chain]]` decision: porting the driver requires `NpcModeMap` / `NpcStatMap` / `ScriptOpcodeMap` / `ScriptOpcodePointers` registries that are not yet on goscape side; porting the compiler requires the entire `runescript` package (a multi-sub-spec arc of its own).

## 1. Goal

Port `CompilerTypeInfo` from TS `tools/pack/Compiler.ts:21-107` to goscape as a new sub-package `pkg/pack/compiler/` containing:

- A `TypeInfo` value type with fields parallel to TS `CompilerTypeInfo` (with one shape deviation — see §10).
- Five constructor entry points: `Load` (from `.pack`-style file), `LoadArray` (from `[]string`), `LoadRecords` (from `map[string]string`), `LoadMap` (from `map[string]int`), and the in-method `(*TypeInfo).Add`.
- Per-loader semantic parity with TS (null sentinel filtering, lowercase normalization, `valueAsKey` flip, `updateMax` flag, `Max = id + 1` semantics).

After this slice:
- `pkg/pack/compiler/` exists with a tested, well-bounded data type ready for NAI-201's populator (`runServerCompiler` port).
- No production wiring lands yet — `pkg/pack/compiler/` has zero consumers outside its own test file. This is the foundational data shape, not a feature path.
- Deviation tag `NAI-200-D-DUAL-MAP` is pinned via a grep-based test.

## 2. Scope

**In**:

- New sub-package `pkg/pack/compiler/` with two files:
  - `typeinfo.go` — `TypeInfo` struct + `Add` method + five constructors.
  - `typeinfo_test.go` — unit tests covering each constructor + `Add` semantics.
- New pin test `pkg/pack/compiler/nai200_deviation_pins_test.go` for `NAI-200-D-DUAL-MAP`.
- TS-faithful semantics:
  - `Load`: skip empty lines, skip lines with no `=`, skip `name == "null"`, skip `name == "null:null"`. Return empty `TypeInfo` on missing file (no error).
  - `LoadArray`: `Add(i, lowercase(input[i]))` for each i; `Max = len(input)` (TS `add` with `updateMax=true` on the last bumps to `len-1 + 1 = len`).
  - `LoadRecords(input, valueAsKey)`: populate `NameMap` keyed per flag; `Max` unchanged (`updateMax` implicit-false — TS doesn't call `add`).
  - `LoadMap(input, valueAsKey)`: same shape as `LoadRecords` but for `map[string]int`; lowercase normalization preserved on the side that holds the string key.
  - `Add(id, name, updateMax)`: set `Map[id] = name`; if `updateMax && Max < id`, set `Max = id + 1`.

**Out**:

- `runServerCompiler` driver (TS Compiler.ts:109-367) → NAI-201.
- `NpcModeMap`, `NpcStatMap` ports (sources at `Engine-TS/src/engine/entity/NpcMode.ts:98` and `NpcStat.ts:10`) → NAI-201 prerequisite block.
- `ScriptOpcodeMap` (name→ID) and `ScriptOpcodePointers` (require/set/conditional/corrupt flag tables) → NAI-201 prerequisite block (`pkg/script/opcode.go` has the constants but neither the name-map nor the pointer registry).
- Port of `@lostcityrs/runescript` `CompileServerScript` (the actual lexer/parser/typechecker) → NAI-202+ arc (multi-sub-spec).
- `script.pack` / `script.dat` writers, `regenScriptPack`, `.rs2` output → blocked on the full bytecode arc.
- `loadDirExtFull` `.constant` walking — already ported as `pkg/pack/LoadDirExtFull` (verified at `pkg/pack/parse.go:162`). Out of NAI-200 scope.

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]`).
- TS source canon: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]`). Specifically:
  - `tools/pack/Compiler.ts:21-107` (full body of `CompilerTypeInfo` class).
- No external dependencies. Stdlib only.

## 4. Non-goals

- Cross-package wiring: NAI-200 lands `pkg/pack/compiler/` with zero production callers. The populator (NAI-201) is the first consumer.
- Generics over arbitrary map types: TS `loadMap` takes `Map<string, number>` only. Goscape ports the concrete signature `map[string]int`. Generalization is not a requirement until the second-shape consumer appears (none expected from `runServerCompiler` — every TS call site uses `Map<string, number>`).
- Performance: this is a one-off load at pack-CLI startup. No optimization called for.

## 5. Architecture

### 5.1 File layout

```
pkg/pack/compiler/                              (NEW sub-package)
├── typeinfo.go                                 (NEW — TypeInfo + 5 constructors + Add)
├── typeinfo_test.go                            (NEW — per-loader unit tests)
└── nai200_deviation_pins_test.go               (NEW — NAI-200-D-DUAL-MAP pin)
```

The sub-package keeps `pkg/pack/` flat-layout clean (which currently hosts only per-config writers + shared infra). Future bytecode-arc code (`compiler/`, possibly later `compiler/lexer/`, `compiler/parser/`, `compiler/typecheck/`) nests under this root.

### 5.2 `TypeInfo` struct shape

TS:

```ts
class CompilerTypeInfo {
    max: number = -1;
    map: Record<string, string> = {};   // mixed numeric AND string keys (sometimes both)
    vartype: Record<string, string> = {};
    protect: Record<string, boolean> = {};
    require, require2, set, set2, corrupt, corrupt2: Record<string, string> = {};
    conditional: Record<string, boolean> = {};
}
```

The TS `map: Record<string, string>` accepts both numeric IDs (`map[id] = name` in `add()`) and string keys (`map[key] = value` in `loadRecords` / `loadMap`). JS object-key coercion makes this work invisibly. Go cannot mix `int` and `string` keys in one map.

Goscape splits the field per call-site usage:

```go
type TypeInfo struct {
    Max     int                // sentinel -1 = empty
    Map     map[int]string     // int-keyed; populated by Add (Load + LoadArray paths)
    NameMap map[string]string  // string-keyed; populated by LoadRecords and LoadMap (both flag values)
    VarType map[int]string
    Protect map[int]bool
    Require, Require2 map[int]string
    Conditional map[int]bool
    Set, Set2 map[int]string
    Corrupt, Corrupt2 map[int]string
}
```

**Why the split**: every TS call site is statically one-shape or the other — `add(id, name)` writes int-keyed; `loadRecords` / `loadMap` write string-keyed. The split preserves all behaviour per-call-site without forcing `interface{}` keys or string-stringified IDs. See §10 — `NAI-200-D-DUAL-MAP`.

**Auxiliary maps** (`VarType`, `Protect`, `Require`, …): in TS, these are also `Record<string, string>` or `Record<string, boolean>`, but every write uses a numeric ID as the key. Goscape uses `map[int]<value-type>` consistently. NOT counted as a separate deviation — the choice is forced by the dual-map split and falls out cleanly.

### 5.3 Constructor: `Load(path string) (*TypeInfo, error)`

Maps TS `static load(file: string)` at lines 38-60.

Behaviour:
1. If the file does not exist → return empty `TypeInfo{Max: -1, Map: {}, ...}`, `nil` error. (TS `!fs.existsSync(file)` early-return.)
2. Read file as UTF-8, split by `/\r?\n/`.
3. For each line:
   - If line is empty OR contains no `=` → skip.
   - Split on first `=` into `(id, name)` strings.
   - If `name == "null"` OR `name == "null:null"` → skip.
   - Parse `id` as `int` (TS `parseInt(id)`). On parse-error, mirror TS `parseInt` behaviour: TS returns `NaN`, which when used as a key in `add(NaN, name)` writes `map["NaN"] = name` and the `updateMax` branch evaluates `this.max < NaN` as false (NaN comparisons are always false), so no `max` bump. Goscape can't follow that literally. **Treat parse-error as a skip** — equivalent to TS-with-a-NaN-key (the entry lands somewhere unreachable). This is a tiny correctness-not-divergence call: the `.pack` files this loads are mechanically generated by goscape's own `pkg/pack/` writers and contain only well-formed `id=name\n` lines, so the parse-error arm is unreachable in practice.
   - Otherwise call `(*TypeInfo).Add(id, name, true)`.

Signature returns `(*TypeInfo, error)` to leave room for genuine IO errors (`os.Open` / read failures other than `IsNotExist`). TS treats those as uncaught (`fs.readFileSync` throws). Goscape propagates per `[[true_to_ts_gate]]`.

### 5.4 Constructor: `LoadArray(input []string) *TypeInfo`

Maps TS `static loadArray(input: string[])` at lines 62-70.

For each `i, s` in `input`: `pack.Add(i, strings.ToLower(s), true)`. Returns the populated `TypeInfo`. `Max` ends at `len(input)` (the last `Add` bumps with `updateMax=true` when `i == len-1 > Max`, setting `Max = i + 1`).

No error return — input is in-memory.

### 5.5 Constructor: `LoadRecords(input map[string]string, valueAsKey bool) *TypeInfo`

Maps TS `static loadRecords(input: Record<string, string>, valueAsKey: boolean = false)` at lines 72-84.

For each `(k, v)` in `input`:
- If `valueAsKey`: `pack.NameMap[v] = strings.ToLower(k)`
- Else: `pack.NameMap[k] = strings.ToLower(v)`

`Max` is unchanged (TS doesn't call `add`). Lowercase normalization is applied to the value side per TS.

No error return.

### 5.6 Constructor: `LoadMap(input map[string]int, valueAsKey bool) *TypeInfo`

Maps TS `static loadMap(input: Map<string, number>, valueAsKey: boolean = false)` at lines 86-98.

For each `(k, v)` in `input`:
- If `valueAsKey`: `pack.NameMap[strconv.Itoa(v)] = strings.ToLower(k)`
- Else: `pack.NameMap[strings.ToLower(k)] = strconv.Itoa(v)`

`Max` is unchanged. The `strconv.Itoa(v)` mirrors TS `value.toString()`.

Note: TS `Map<string, number>` is insertion-ordered; Go `map[string]int` iteration order is randomized. Order-of-iteration only matters when two distinct keys collide on the same value-as-key target (i.e., `valueAsKey=true` and two `k`s map to the same `v`). In every Compiler.ts call site, the input is a static enum (`PlayerStatMap`, `NpcStatMap`, `NpcModeMap`) with unique-value-per-key invariant — collisions don't occur. Not a tracked deviation; inline-comment annotation in `LoadMap` flags the assumption for any future caller.

No error return.

### 5.7 Method: `(*TypeInfo).Add(id int, name string, updateMax bool)`

Maps TS `add(id: number, name: string, updateMax: boolean = true)` at lines 100-106.

Body:
```go
func (p *TypeInfo) Add(id int, name string, updateMax bool) {
    p.Map[id] = name
    if updateMax && p.Max < id {
        p.Max = id + 1
    }
}
```

Note the off-by-one: `Max` ends at `id + 1`, not `id`. TS comment on line 104 makes this explicit. The semantic is "Max is the upper-exclusive bound" — callers iterate `for i := 0; i <= max; i++` (TS Compiler.ts:205, 215, etc. all use `<=`), which means `i` runs from `0` to `Max` inclusive. With `Max = id + 1`, the highest id is reached one past — the iteration covers gaps. (This is a known TS quirk; goscape preserves it verbatim.)

### 5.8 Zero-value initialization

Constructors return `*TypeInfo` with all maps non-nil (initialized inline) and `Max = -1`. This matches TS `class` field defaults at lines 22-23. Callers can therefore call `Add` immediately on a freshly-constructed value without nil-map panic.

A small package-private helper `newTypeInfo() *TypeInfo` initializes the struct; all five constructors call it.

## 6. Error handling

- `Load`: returns wrapped error on IO failure (other than `IsNotExist`). `IsNotExist` → empty `TypeInfo`, nil error. Parse-error on `parseInt` → skip the line (per §5.3 rationale).
- `LoadArray` / `LoadRecords` / `LoadMap`: no error path. Pure in-memory transforms.
- `Add`: no error path. Direct map write.

Per `[[true_to_ts_gate]]`: TS doesn't validate inputs (no duplicate-id check, no name-collision check). Goscape mirrors. If a `.pack` file has `0=foo\n0=bar\n`, the second `Add(0, "bar")` silently overwrites. Pin behaviour at test §7.5.

## 7. Testing

All tests in `pkg/pack/compiler/typeinfo_test.go`. Standard `testing.T` table-driven where convenient. Per `[[plan_runnable_test_fixtures]]`, each fixture is mentally walked through TS code at spec-write.

### 7.1 `Load` happy-path

Fixture: temp file containing:
```
0=alpha
1=bravo
2=charlie
```

Expected:
- `Max == 3`
- `Map == {0: "alpha", 1: "bravo", 2: "charlie"}`
- `NameMap == {}` (empty, not nil)
- All auxiliary maps empty.

### 7.2 `Load` missing-file

Path that does not exist → `(*TypeInfo, error)` with `Max == -1`, all maps empty, nil error.

### 7.3 `Load` filter cases

Fixture (mixed valid/invalid lines):
```
0=valid_alpha

1=
2=null
3=null:null
not_an_equals_line
4=valid_beta
```

Expected:
- `Map == {0: "valid_alpha", 1: "", 4: "valid_beta"}` — note line `1=` produces empty-string name (TS does NOT filter empty names; only `null` / `null:null` / blank-line / no-`=` filters apply).
- `Max == 5` (last `Add(4, "valid_beta", true)` bumps to `4+1=5`).

The `1=` empty-name case is a real-data TS-faithful behaviour — TS `split('=')` on `"1="` yields `["1", ""]`, name is `""`, name is not `"null"` or `"null:null"`, so `add(1, "")` runs and `map[1] = ""`. Goscape matches.

### 7.4 `Load` IO-error

Pass a path that resolves to a directory rather than a regular file (`t.TempDir()` directly, no file inside). `os.ReadFile` on a directory returns a non-nil error that is not `os.IsNotExist` — exercises the genuine-IO-error arm. Assert: non-nil error returned, `*TypeInfo` return value is `nil`.

This is deterministic across Linux/macOS/Windows; no permission manipulation needed.

### 7.5 `Load` duplicate-id behaviour

Fixture:
```
0=first
0=second
```

Expected: `Map == {0: "second"}`, `Max == 1` (first `Add(0, "first")` bumps `-1 < 0` to `1`; second `Add(0, "second")` does NOT re-bump because `1 < 0` is false; the overwrite is silent).

Pins the no-validation TS-faithful behaviour.

### 7.6 `LoadArray` happy-path

Input: `[]string{"Alpha", "BRAVO", "Charlie"}`.
Expected:
- `Map == {0: "alpha", 1: "bravo", 2: "charlie"}` (lowercased)
- `Max == 3`

### 7.7 `LoadArray` empty input

Input: `[]string{}`.
Expected: `Max == -1`, all maps empty (no `Add` calls → no `Max` bump).

### 7.8 `LoadRecords` valueAsKey=false

Input: `{"foo": "BAR", "baz": "QUX"}`, `valueAsKey: false`.
Expected:
- `NameMap == {"foo": "bar", "baz": "qux"}` (value lowercased)
- `Map == {}`
- `Max == -1`

### 7.9 `LoadRecords` valueAsKey=true

Input: `{"FOO": "BAR", "BAZ": "QUX"}`, `valueAsKey: true`.
Expected:
- `NameMap == {"BAR": "foo", "QUX": "baz"}` — key UNCHANGED (TS preserves value's case as the new key), value lowercased.

This pins a subtle TS asymmetry: the value-as-key path lowercases the *KEY* (`key.toLowerCase()`) and stores it as the value, but writes the unchanged value as the map key. Re-read of TS Compiler.ts:77 (`pack.map[value] = key.toLowerCase()`) confirms.

### 7.10 `LoadMap` both flag values

Input: `map[string]int{"FOO": 7, "BAR": 9}`.

`valueAsKey: false`:
- `NameMap == {"foo": "7", "bar": "9"}` (key lowercased, value via `strconv.Itoa`)

`valueAsKey: true`:
- `NameMap == {"7": "foo", "9": "bar"}` (value-as-key via Itoa, original key lowercased into the new value)

### 7.11 `Add` updateMax=false

Sequence: `Add(0, "a", true); Add(5, "b", false); Add(2, "c", true)`.
Expected: `Max == 3` (first bumps -1→1; second skips bump; third bumps 1→3 since `1 < 2`).

### 7.12 `Add` Max non-monotonic

Sequence: `Add(0, "a", true); Add(5, "b", true); Add(2, "c", true)`.
Expected: `Max == 6` (first bumps -1→1; second bumps 1→6; third does NOT re-bump since `6 < 2` is false).

Pins the `if (updateMax && this.max < id)` semantics — `Max` is monotonic non-decreasing across `Add` calls.

### 7.13 Deviation tag pin

`nai200_deviation_pins_test.go`:
- `TestNAI200Deviation_DualMap_Pinned`: grep `pkg/pack/compiler/` for the string `NAI-200-D-DUAL-MAP`; assert ≥1 match (the doc-comment on `TypeInfo` explaining the int/string-key split).

Per `[[pin_test_self_trigger_production_doc]]`: pin matches the tag identifier only. The doc-comment may reference "TS `Record<string, string>`" or similar, but the pin grep keys on `NAI-200-D-DUAL-MAP` so the test itself cannot self-trigger.

## 8. Open questions

Resolved at spec-write — see §9.

## 9. Resolved risks

**R1 — `parseInt` NaN behaviour parity**
*Risk*: TS `parseInt('not_a_number')` returns `NaN`; `map[NaN]` and `max < NaN` produce reachable-but-quirky state.
*Resolution*: TS-faithful literal port is impossible (Go has no NaN-int). Skip parse-error lines silently (§5.3). The `.pack` files NAI-200 will eventually load are mechanically generated by goscape's own `pkg/pack/` writers (`PackFile.Save` writes only `id=name\n`), so the divergence is unreachable in practice. Not a formal deviation — code-comment annotation only.

**R2 — Are auxiliary maps (`VarType`, `Protect`, …) actually used by `Load` / `LoadArray` / `LoadRecords` / `LoadMap`?**
*Risk*: TS exposes them as instance fields; NAI-200 constructors don't populate them. Are they dead in NAI-200?
*Resolution*: Confirmed dead at NAI-200 scope. Inspection of TS Compiler.ts:38-98 shows none of the five constructors writes to `vartype`/`protect`/`require`/`set`/`conditional`/`corrupt` etc. — they are populated externally by `runServerCompiler` (lines 211, 242, 252, …, **NAI-201 territory**). Goscape initializes them as empty non-nil maps so NAI-201 can write into them without re-init. Exporting them in §5.2 is forward-compatibility, not dead-weight per `[[dead_api_polish]]` — the next slice's first action is writing into them.

**R3 — Does `Load`'s `parseInt(id)` failure differ from `parseInt("0")` succeeding?**
*Risk*: `id` strings like `" 0 "` (with whitespace) parse cleanly in TS but error in `strconv.Atoi`.
*Resolution*: `.pack` corpus is mechanically generated by `pkg/pack/PackFile.Save` (no whitespace padding). The TS code DOES tolerate `parseInt("  0  ")` (TS skips leading whitespace). Goscape uses `strconv.Atoi`, which rejects leading whitespace, but the input never contains it. If a hand-edited `.pack` ever contained whitespace-padded IDs, Atoi would fail and the line would be skipped (per R1). Inline-comment annotation; not a tracked deviation. (Reference `Engine-TS` `PackFile.save` for the canonical `id=name\n` format.)

**R4 — `Map`'s int-key choice: would `map[string]string` (TS-faithful) be simpler?**
*Risk*: keeping the TS `Record<string, string>` shape by stringifying IDs (`map[strconv.Itoa(id)] = name`) avoids the dual-map split.
*Resolution*: Rejected. Forcing every reader (NAI-201's iteration `for i := 0; i <= max; i++`) to `Itoa` on each lookup is performance noise and type-safety regression. The dual-map split is the right Go shape; document via `NAI-200-D-DUAL-MAP` and move on.

**R5 — Should we ship a `(*TypeInfo).Get(id int) (string, bool)` getter now?**
*Risk*: Dead-API per `[[dead_api_polish]]`. NAI-201 will be the first reader; we don't know its iteration shape yet.
*Resolution*: Skip. Callers can index `info.Map[id]` directly (returns zero-string on miss). If NAI-201 grows complex enough to want a getter, add at the same commit. NAI-200 ships data + writers only.

**R6 — Generics over `LoadMap`'s value type?**
*Risk*: Future caller wants `LoadMap(input map[string]uint32, ...)`. Forcing them to convert is friction.
*Resolution*: All TS call sites use `Map<string, number>` (no other shape exists in Compiler.ts). Ship `map[string]int` only. Generalize when a second-shape consumer appears.

**R7 — Field naming: `Map` vs `IDMap` vs `Entries`?**
*Risk*: `Map` collides with the builtin `map` keyword (not technically — `Map` is the field name, not the type). Conventional Go would prefer a more specific name.
*Resolution*: Use `Map` for parity with TS `map`. The field is unexported in TS (well, `public` is the default but used internally); goscape exports it because NAI-201's populator writes into it. The naming preserves grep-discoverability across the TS↔goscape boundary, which `[[flat_arg_signature_for_cross_lang_parity]]` and `[[ts_source_canonical_path]]` together support. (Same precedent: `pkg/pack/PackFile.Pack` mirrors TS `PackFile.pack`.)

**R8 — Should pin-test grep also cover `modules/`?**
*Risk*: Future consumer in `modules/world/` might write `NAI-200-D-DUAL-MAP` annotations. Pin would miss them.
*Resolution*: NAI-200 lands no `modules/` wiring. The grep root is `pkg/pack/compiler/` only. If a future slice extends usage to `modules/`, the pin scope expands at that commit per `[[retire_deviation_grep_all_comments]]`.

## 10. Deviations enumerated

- **`NAI-200-D-DUAL-MAP`**: TS `CompilerTypeInfo.map: Record<string, string>` is a single field that accepts both numeric-IDs-coerced-to-strings (`add(id, name)` path) and genuine string keys (`loadRecords` / `loadMap` paths). Goscape splits into `TypeInfo.Map map[int]string` (int-keyed; `Load` + `LoadArray` write) and `TypeInfo.NameMap map[string]string` (string-keyed; `LoadRecords` + `LoadMap` write). Rationale: Go's static type system rejects mixed-key maps; the split preserves per-call-site semantic parity. Auxiliary maps (`VarType`, `Protect`, `Require`, `Set`, `Conditional`, `Corrupt`, and their `2`-suffix siblings) are all `map[int]<T>` because every TS write to them uses a numeric ID key — falls out cleanly from the split.

(No second deviation. The `parseInt`-NaN-vs-`strconv.Atoi`-error and the whitespace-tolerance differences are unreachable-in-practice and inline-comment annotated, per R1/R3.)

## 11. Carry-forward (from prior NAI sub-specs)

Per `[[nai_followups]]` audit at spec-write:
- **NAI-191 #1 `LoadFileFull` `TrimLeft` Unicode narrowing** — not on NAI-200 hot path (compiler/`Load` uses `bufio.Scanner`-style line iteration, not `loadFileFull`). Leave deferred.
- **NAI-191 #3 `ShouldBuildFileAny` `ReadDir` failure returns false** — not used by NAI-200. Leave deferred.
- **NAI-198 #1 OPOBJ2 upstream reconciliation** — upstream-engagement track. Out of NAI-200 scope.
- **NAI-199 #1 `frame_del` `endsWith` suffix-match TS-parity** — orthogonal. Leave deferred to its existing tracker.

No new NAI-200-bound carry-forwards introduced.

## 12. Arc next step

NAI-200 unblocks **NAI-201: `runServerCompiler` driver port**. NAI-201 prerequisites (block before plan-author dispatch):

1. **`NpcModeMap`** name→ID port. Source: `Engine-TS/src/engine/entity/NpcMode.ts:98`. Likely lands as `pkg/objtype/npcmode.go` mirroring the existing `pkg/objtype/playerstat.go` shape.
2. **`NpcStatMap`** name→ID port. Source: `Engine-TS/src/engine/entity/NpcStat.ts:10`. Same shape — `pkg/objtype/npcstat.go`.
3. **`ScriptOpcodeMap`** name→ID port. Source: TS `ScriptOpcode.ts`. `pkg/script/opcode.go` (1286 LOC) has the Opcode constants but no name-map. Add as a `map[string]Opcode` populated from the existing constants (likely via a `go:generate` step or hand-maintained literal).
4. **`ScriptOpcodePointers`** require/set/conditional/corrupt flag tables. Source: TS `ScriptOpcodePointers.ts`. New file `pkg/script/opcode_pointers.go`.
5. After 1-4: port `runServerCompiler` body in a new file `pkg/pack/compiler/run_server.go` (or similar). Lands the wire-up of all ~25 symbol categories.

NAI-202+ then opens the **bytecode lexer/parser/typechecker** arc, porting `@lostcityrs/runescript` to a goscape sub-package (`pkg/pack/compiler/lexer/`, `pkg/pack/compiler/parser/`, `pkg/pack/compiler/typecheck/`, or similar — sub-spec decides). This is multi-sub-spec scope; NAI-202+ is its own arc opener.

Final close (NAI-2NN, far): `script.pack` writer + `regenScriptPack` wire-up + `script.dat` byte-pin against TS reference output.

## 13. Acceptance criteria

- `pkg/pack/compiler/typeinfo.go` exists with the documented type + 5 constructors + `Add` method.
- `go test ./pkg/pack/compiler/...` passes (cleanly, `-race`).
- `go test ./...` passes (no regressions elsewhere — the new sub-package has no consumers).
- Deviation pin grep returns ≥1 match for `NAI-200-D-DUAL-MAP`.
- TS-↔-goscape parity verified per-loader by reading TS Compiler.ts:21-107 against the goscape implementation alongside review.
