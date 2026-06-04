# NAI-201: bytecode compiler arc — foundation registries

**Date**: 2026-05-14
**Predecessor**: NAI-200 (`CompilerTypeInfo` foundation; closed at `0dd1a22`). NAI-200 §12 enumerated four "NAI-201 prerequisites" — this sub-spec lands those four as data registries with zero production consumers, deferring the `runServerCompiler` driver port that consumes them to NAI-202.
**Arc step**: Second sub-spec of the bytecode compiler arc. NAI-200 shipped the symbol-table data type; NAI-201 ships the input data the driver will feed into it.

## 0. Pre-context: scope-slice decision

The original NAI-201 framing in `NAI-200 §12` and the dispatch prompt was "port `runServerCompiler` driver (TS `tools/pack/Compiler.ts:109-367`)". Pre-flight against HEAD `0dd1a22`:

| Prerequisite | Status | TS source | Size |
|---|---|---|---|
| `NpcStatMap` | missing | `src/engine/entity/NpcStat.ts:10` | 6 entries |
| `NpcModeMap` | missing | `src/engine/entity/NpcMode.ts:98` | 48 entries (68 with TODO-commented) |
| `ScriptOpcodeMap` | missing (numeric `Op*` constants present in `pkg/script/opcode.go:1286`, no name→Opcode map) | `src/engine/script/ScriptOpcode.ts:445` | 393 entries |
| `ScriptOpcodePointers` | missing | `src/engine/script/ScriptOpcodePointers.ts` | 237 entries |
| `PlayerStatMap` | exists at `pkg/objtype/playerstat.go:37` | — | — |

A combined "prereqs + driver" NAI-201 would land ~1100 LOC across two packages with two distinct verification surfaces (data parity for the registries, orchestration for the driver). Per `[[scope_gate_prerequisite_chain]]` and the cadence norm visible in recent NAI sub-specs (NAI-198 ≈ 500 LOC / 7 commits; NAI-197 similar), this sub-spec splits the work:

- **NAI-201 (this)**: the four foundation registries as pure data + parity tests + cross-registry validity tests. ~900 LOC mostly mechanical data (393 + 237 + 48 + 6 entries plus tests).
- **NAI-202**: `runServerCompiler` driver in `pkg/pack/compiler/` orchestrating `Load`×22 + entity-type-loader enrichment + `LoadMap`×3 + `LoadArray`×2. ~200-300 LOC.
- **NAI-203+**: external `@lostcityrs/runescript` `CompileServerScript` port (multi-sub-spec arc — lexer/parser/typechecker).

Slicing rationale:
- Cohesive purpose: all four registries are "data the compiler reads".
- Tight coupling: `ScriptOpcodePointers` indexes by `Opcode`; landing it with `ScriptOpcodeMap` in the same sub-spec keeps opcode-id alignment verifiable in one place.
- `NpcStatMap` and `NpcModeMap` are too small to warrant standalone sub-specs (~10 and ~50 LOC respectively) and share a package (`pkg/objtype`).

Alternative considered: split per registry into 4 sub-specs. Rejected — `ScriptOpcodeMap` and `ScriptOpcodePointers` cross-reference and would force inter-spec validity tests that are cleaner local.

## 1. Goal

Port the four missing compiler-foundation registries from TS to goscape as pure-data declarations with zero production consumers:

- `NpcStatMap` in `pkg/objtype` — uppercase NPC-stat name → stat index (6 entries).
- `NpcModeMap` in `pkg/objtype` — uppercase NPC-mode name → mode index (48 entries including `NULL`; 20 `QUEUE*` TODO-commented entries omitted per TS).
- `ScriptOpcodeMap` in `pkg/script` — uppercase opcode name → `Opcode` value (393 entries).
- `ScriptOpcodePointers` in `pkg/script` — `Opcode` → `Pointers` struct holding `require`/`set`/`corrupt`/`require2`/`set2`/`corrupt2`/`conditional` flags (237 entries).
- Supporting type `Pointers` (Go struct mirroring the TS object literal shape) and constant `PointerGroupFind` (the 5-element find_* group used in spread positions).

After this slice:
- All four registries are present and tested. NAI-202's `runServerCompiler` driver port can consume them directly.
- No production wiring lands — registries have zero callers outside their own test files (mirrors NAI-200's "foundation, no consumers" pattern).
- Cross-registry validity: every key in `ScriptOpcodePointers` is a valid `Opcode` constant; `ScriptOpcodeMap` values exhaust the named subset of `Op*` constants without duplicates.

## 2. Scope

**In**:

- New file `pkg/objtype/npcstat.go` — `NpcStat*` index constants + `NpcStatMap` (uppercase → int).
- New file `pkg/objtype/npcmode.go` — `NpcMode*` index constants + `NpcModeMap` (uppercase → int).
- New file `pkg/script/opcode_map.go` — `ScriptOpcodeMap map[string]Opcode` (393 entries) populated from existing `Op*` constants in `pkg/script/opcode.go`.
- New file `pkg/script/opcode_pointers.go` — `Pointers` struct definition + `PointerGroupFind []string` constant + `ScriptOpcodePointers map[Opcode]Pointers` populated from TS source.
- Per-registry `*_test.go` files: length parity vs TS source, spot-check coverage, and the cross-registry validity tests described in §7.
- Pin test `pkg/script/nai201_deviation_pins_test.go` for each tracked deviation tag (see §10).

**Out** (deferred):

- `runServerCompiler` driver body (TS Compiler.ts:109-367) → NAI-202.
- `dbcolumnInfo` synthesis from `DbTableType` columns (lives inside `runServerCompiler`) → NAI-202.
- `interfaceInfo` / `overlayInfo` synthesis from `Component.comName` / `com.overlay` → NAI-202.
- `fontmetricsInfo` / `locshapeInfo` (static `LoadArray` inputs hard-coded inside `runServerCompiler`) → NAI-202.
- `CompileServerScript` (the external runescript package) → NAI-203+ arc.
- `NpcMode` / `NpcStat` enum behaviour beyond the index↔name mapping (e.g., NPC AI state machine consumers) — out of scope; only the name-map is in scope here.
- Renaming or restructuring existing `Op*` constants in `pkg/script/opcode.go` — they stay as-is; this sub-spec adds a name-keyed view.

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]`).
- TS source canon: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]`). Specifically:
  - `src/engine/entity/NpcStat.ts:1-17` (full file).
  - `src/engine/entity/NpcMode.ts:1-168` (full file; lines 147-167 are commented TODOs that must NOT be ported).
  - `src/engine/script/ScriptOpcode.ts:1-861` (enum body 1-443 with 393 named values; `ScriptOpcodeMap` body 445-857 with 393 entries).
  - `src/engine/script/ScriptOpcodePointers.ts:1-984` (full file).
- Stdlib only. No external dependencies.

## 4. Non-goals

- Generating `ScriptOpcodeMap` via `go:generate`: a hand-maintained literal preserves grep-parity with the TS source one-to-one (TS has 437 explicit `['NAME', ScriptOpcode.NAME]` lines; goscape will have 437 explicit `"NAME": OpName,` lines). go:generate would introduce a build-step dependency for a one-shot port — rejected per `[[dead_api_polish]]` (no second consumer pattern justifies the abstraction).
- Generating reverse maps (`ScriptOpcodeNameMap` — TS Compiler.ts:859-861 derives one). Not consumed by `runServerCompiler`. Skip until NAI-202 confirms need.
- Exposing the registries as a single "compiler symbol catalog" struct. The current TS shape is four independent maps; goscape mirrors that. Consolidation can happen later if NAI-202's driver wants it.
- Performance: these are one-shot loads at pack-CLI startup. No optimization warranted.

## 5. Architecture

### 5.1 File layout

```
pkg/objtype/
├── npcstat.go                                    (NEW)
├── npcstat_test.go                               (NEW)
├── npcmode.go                                    (NEW)
└── npcmode_test.go                               (NEW)

pkg/script/
├── opcode_map.go                                 (NEW)
├── opcode_map_test.go                            (NEW)
├── opcode_pointers.go                            (NEW)
├── opcode_pointers_test.go                       (NEW)
└── nai201_deviation_pins_test.go                 (NEW)
```

`pkg/script/opcode.go` is **not** edited — the existing `Op*` constants are the source of truth for `Opcode` values, and `opcode_map.go` references them by name.

### 5.2 `NpcStat*` constants + `NpcStatMap`

TS source (verbatim, NpcStat.ts:1-17):

```ts
export const enum NpcStat {
    ATTACK, DEFENCE, STRENGTH, HITPOINTS, RANGED, MAGIC
}
export const NpcStatMap: Map<string, number> = new Map([
    ['ATTACK', NpcStat.ATTACK], ...
]);
```

Goscape port:

```go
// pkg/objtype/npcstat.go
package objtype

// NpcStat* are indices into Npc.stats / Npc.baseLevels for NPC combat stats.
// Index values match TS NpcStat enum (NpcStat.ts:1-8).
const (
    NpcStatAttack    = 0
    NpcStatDefence   = 1
    NpcStatStrength  = 2
    NpcStatHitpoints = 3
    NpcStatRanged    = 4
    NpcStatMagic     = 5

    NpcStatCount = 6
)

// NpcStatMap maps uppercase NPC-stat name → stat index. Mirrors TS
// NpcStatMap (NpcStat.ts:10-17). Consumed by the bytecode compiler's
// LoadMap call in pkg/pack/compiler (NAI-202 runServerCompiler).
var NpcStatMap = map[string]int{
    "ATTACK":    NpcStatAttack,
    "DEFENCE":   NpcStatDefence,
    "STRENGTH":  NpcStatStrength,
    "HITPOINTS": NpcStatHitpoints,
    "RANGED":    NpcStatRanged,
    "MAGIC":     NpcStatMagic,
}
```

Mirrors the `PlayerStatMap` shape at `pkg/objtype/playerstat.go:37`.

### 5.3 `NpcMode*` constants + `NpcModeMap`

TS source enum (NpcMode.ts:1-96) defines 68 named values (`NULL = -1` through `QUEUE20 = 66`). TS `NpcModeMap` (NpcMode.ts:98-168) includes **48** active entries — `NULL` through `APNPC5` on lines 99-146. The 20 `QUEUE1`..`QUEUE20` entries on lines 147-167 are commented-out with the comment `// TODO: these are not used?`. Counts verified at spec-write: `awk '/^export const enum NpcMode/,/^}/' NpcMode.ts | grep -c '^\s\+[A-Z][A-Z0-9_]*\s*[=,]'` → `68`; `awk '/^export const NpcModeMap/,/^]\)/' NpcMode.ts | grep "^\s*\['" | wc -l` → `48`.

Goscape port:

```go
// pkg/objtype/npcmode.go
package objtype

// NpcMode* are NPC AI mode identifiers. Index values match TS NpcMode
// enum (NpcMode.ts:1-96). The full enum has 68 named values (NULL = -1
// through QUEUE20 = 66); goscape mirrors all 68.
const (
    NpcModeNull            = -1
    NpcModeNone            = 0
    NpcModeWander          = 1
    NpcModePatrol          = 2
    NpcModePlayerEscape    = 3
    NpcModePlayerFollow    = 4
    NpcModePlayerFace      = 5
    NpcModePlayerFaceClose = 6
    NpcModeOpPlayer1       = 7
    // ... (all 68 enum values, matching TS line numbers)
    NpcModeQueue20         = 66
)

// NpcModeMap maps uppercase NPC-mode name → mode index. Mirrors TS
// NpcModeMap (NpcMode.ts:98-168). The 20 QUEUE1..QUEUE20 entries are
// commented-out in TS (lines 147-167, `// TODO: these are not used?`)
// and are NOT included here — see NAI-201-D-NPCMODE-QUEUE-TODO.
//
// Consumed by the bytecode compiler's LoadMap call in pkg/pack/compiler
// (NAI-202 runServerCompiler).
var NpcModeMap = map[string]int{
    "NULL":            NpcModeNull,
    "NONE":            NpcModeNone,
    "WANDER":          NpcModeWander,
    // ... 48 entries total, matching TS NpcMode.ts:99-146 verbatim
    "APNPC5":          NpcModeApNpc5,
}
```

`Map` length = 48 (not 68). Tracked as deviation `NAI-201-D-NPCMODE-QUEUE-TODO` per §10.

### 5.4 `ScriptOpcodeMap`

TS source (ScriptOpcode.ts:445-857): a `Map<string, number>` literal with 393 `['NAME', ScriptOpcode.NAME]` entries in a specific TS-author-chosen ordering. (Confirmed at spec-write by `awk '/^export const ScriptOpcodeMap/,/^]\)/' ScriptOpcode.ts | grep -c '^\s*\['` → `393`. Reviewer must re-run this awk against TS HEAD at plan-author dispatch.)

Goscape port:

```go
// pkg/script/opcode_map.go
package script

// ScriptOpcodeMap maps uppercase opcode name → Opcode value. Mirrors TS
// ScriptOpcodeMap (ScriptOpcode.ts:445-857). Consumed by the bytecode
// compiler's allCommands construction in pkg/pack/compiler (NAI-202
// runServerCompiler).
//
// Naming convention: TS UPPER_SNAKE_CASE → goscape OpUpperCamel. E.g.
// PUSH_CONSTANT_INT → OpPushConstantInt. Reverse-mapping is one-to-one
// for the 437 entries here; verified by TestScriptOpcodeMap_LengthParity
// and TestScriptOpcodeMap_NoDuplicates.
//
// Insertion order: Go map iteration is randomized. TS Map<string, number>
// is insertion-ordered, and runServerCompiler sorts by opcode value
// before iteration (Compiler.ts:111). Goscape iteration order does not
// matter to consumers.
var ScriptOpcodeMap = map[string]Opcode{
    "PUSH_CONSTANT_INT":    OpPushConstantInt,
    "PUSH_VARP":            OpPushVarp,
    "POP_VARP":             OpPopVarp,
    // ... 393 entries verbatim from TS ScriptOpcode.ts:446-857
}
```

Each line is a one-to-one mechanical translation. The TS file groups entries with blank lines and inline comments (e.g., `// Player ops`) — goscape preserves these as Go comments where meaningful, for diff-against-TS reviewability per `[[flat_arg_signature_for_cross_lang_parity]]`.

### 5.5 `Pointers` struct + `PointerGroupFind` + `ScriptOpcodePointers`

TS source (ScriptOpcodePointers.ts:1-15) defines an inline object-literal type. Goscape ports it as a named struct:

```go
// pkg/script/opcode_pointers.go
package script

// PointerGroupFind is the 5-element list of find_* pointer names that
// many opcodes spread into their corrupt list. Mirrors TS
// POINTER_GROUP_FIND (ScriptOpcodePointers.ts:3).
//
// Used by ScriptOpcodePointers entries that mark "everything except
// active is assumed corrupted" — those entries spread this slice into
// their Corrupt field via the corruptExceptActive helper (see below).
var PointerGroupFind = []string{
    "find_player", "find_npc", "find_loc", "find_obj", "find_db",
}

// Pointers holds the pointer-gate flags for one script opcode. Mirrors
// the inline TS type at ScriptOpcodePointers.ts:5-14.
//
// Field semantics:
//   - Require / Require2: pointer names that MUST be set when the opcode
//     executes. Variant *2 applies in 2-active-entity contexts.
//   - Set / Set2: pointer names the opcode SETS on success.
//   - Corrupt / Corrupt2: pointer names the opcode invalidates.
//   - Conditional: true if Set takes effect only on a successful branch
//     (e.g., FINDUID conditional on lookup hit).
//
// Nil slice == "no entries" (matches TS optional-field omitted).
type Pointers struct {
    Require     []string
    Require2    []string
    Set         []string
    Set2        []string
    Corrupt     []string
    Corrupt2    []string
    Conditional bool
}

// corruptExceptActive returns PointerGroupFind ++ extras as a fresh
// slice. Mirrors TS spread pattern `[...POINTER_GROUP_FIND, ...extras]`
// used in 4 entries (P_ARRIVEDELAY ScriptOpcodePointers.ts:286,
// P_COUNTDIALOG :301, P_DELAY :314, P_PAUSEBUTTON :370 — verified via
// `grep -n POINTER_GROUP_FIND`).
//
// TWO additional sites (NPC_DELAY ScriptOpcodePointers.ts:569 and
// NPC_ARRIVEDELAY :711) use a longer prefix:
// `['p_active_player', 'p_active_player2', ...POINTER_GROUP_FIND,
// 'last_com', ...]`. Those two are ported as literal slice expansions
// (NOT via corruptExceptActive) because the prefix differs. The pin
// test at §7.10 anchors the helper-call count and the literal-spread
// count separately.
func corruptExceptActive(extras ...string) []string {
    out := make([]string, 0, len(PointerGroupFind)+len(extras))
    out = append(out, PointerGroupFind...)
    out = append(out, extras...)
    return out
}

// ScriptOpcodePointers maps Opcode → Pointers describing the
// pointer-gate flags consumed by the bytecode compiler's typechecker
// (NAI-203+ arc). Mirrors TS ScriptOpcodePointers
// (ScriptOpcodePointers.ts:1-984).
//
// Opcodes not listed here have an absent / empty Pointers (TS:
// `ScriptOpcodePointers[opcode]` returns undefined, treated as
// "no constraints"). Mirrored in goscape via map miss (zero-value
// Pointers{}).
//
// 237 entries; verified by TestScriptOpcodePointers_LengthParity.
var ScriptOpcodePointers = map[Opcode]Pointers{
    OpAllowDesign: {Require: []string{"active_player"}},
    OpAnim:        {Require: []string{"active_player"}, Require2: []string{"active_player2"}},
    // ... 237 entries verbatim from TS ScriptOpcodePointers.ts:17-981
    OpPArriveDelay: {
        Require: []string{"p_active_player"},
        Corrupt: corruptExceptActive(
            "last_com", "last_int", "last_item", "last_slot",
            "last_targetslot", "last_useitem", "last_useslot",
        ),
    },
    // ...
}
```

**Spread-syntax port (`...POINTER_GROUP_FIND`)**: 6 TS entries spread `POINTER_GROUP_FIND` into their `corrupt` slice. 4 follow the simple `[...POINTER_GROUP_FIND, ...extras]` pattern → goscape uses `corruptExceptActive("last_com", ...)`. 2 follow the extended `['p_active_player', 'p_active_player2', ...POINTER_GROUP_FIND, ...extras]` pattern (NPC_DELAY, NPC_ARRIVEDELAY) → goscape ports those as literal slice expansions. This is NOT a deviation — semantics are byte-equivalent in both shapes. Tracked at §7.10 as a sanity test that the helper is in fact called from the expected number of sites and the literal-spread count is fixed.

**Map keying**: TS uses numeric opcode values as keys (`[ScriptOpcode.ALLOWDESIGN]: {...}`). Goscape uses the same numeric values via the `Opcode` named type. No deviation.

**Missing entries**: TS treats `undefined` as "no constraints". Go map miss returns zero-value `Pointers{}`. Equivalent behaviour. No deviation.

### 5.6 Cross-registry validity

Every key in `ScriptOpcodePointers` must be a valid `Opcode` value (≤ max defined `Op*` constant). Validated in `pkg/script/opcode_pointers_test.go::TestScriptOpcodePointers_KeysAreValidOpcodes` (see §7.7).

Every key in `ScriptOpcodeMap` must be uppercase, and the value must round-trip with the goscape `Op*` constant naming convention (`PUSH_CONSTANT_INT` ↔ `OpPushConstantInt`). Validated by `TestScriptOpcodeMap_NoDuplicates` (no two names map to the same Opcode) and `TestScriptOpcodeMap_NamesUppercase`.

## 6. Error handling

None of the new declarations have an error path. They are package-level `var` declarations with literal values, evaluated at package init. If any value referenced (e.g., `OpAllowDesign`) does not exist in `pkg/script/opcode.go`, the build fails — exactly the desired behaviour.

Per `[[true_to_ts_gate]]`: TS doesn't validate inputs at load time (the map / object literal is what it is). Goscape mirrors. Tests in §7 substitute for runtime validation.

## 7. Testing

All tests in the new `*_test.go` files. Per `[[plan_runnable_test_fixtures]]`, each test is mentally walked through against the TS source before plan-author dispatch.

### 7.1 `TestNpcStatMap_Parity`

Build expected map literally from TS NpcStat.ts:10-17. Assert `reflect.DeepEqual(NpcStatMap, expected)`. Pins all 6 entries, name-cased ATTACK/DEFENCE/etc.

### 7.2 `TestNpcModeMap_Parity`

Build expected map literally from TS NpcMode.ts:99-146 (48 entries). Assert `reflect.DeepEqual(NpcModeMap, expected)`.

### 7.3 `TestNpcModeMap_QueueEntriesOmitted`

Pin deviation `NAI-201-D-NPCMODE-QUEUE-TODO`. Assert all 20 `QUEUE1`..`QUEUE20` keys are absent from `NpcModeMap`. (TS comments them out as `// TODO: these are not used?`.)

### 7.4 `TestScriptOpcodeMap_LengthParity`

Assert `len(ScriptOpcodeMap) == 393`. The exact count is brittle to future TS upstream additions; the test message references TS ScriptOpcode.ts:445 as the source of truth for the expected count. Confirmed at spec-write via `awk '/^export const ScriptOpcodeMap/,/^]\)/' ScriptOpcode.ts | grep -c '^\s*\['`.

### 7.5 `TestScriptOpcodeMap_SpotChecks`

Assert ~12-15 well-chosen entries map correctly:
- `PUSH_CONSTANT_INT` → `OpPushConstantInt` (opcode 0; first entry; smoke-tests indexing).
- `PUSH_CONSTANT_STRING` → `OpPushConstantString` (opcode 3; verifies non-contiguous numbering preserved).
- `RETURN` → `OpReturn` (opcode 21; verifies gap handling).
- `GETTIMESPENT` → `OpGetTimespent` (custom opcode; one of the highest values).
- A representative entry from each major group (Player ops, Npc ops, Inv ops, Loc ops, Obj ops, Db ops, Server ops).

Spot-checks are non-exhaustive by design — the length-parity test plus the no-duplicates test together pin the registry's overall shape, and individual entry typos surface during NAI-202 driver tests against real `.pack` data. Spot-checks here are a smoke-canary, not exhaustive verification.

### 7.6 `TestScriptOpcodeMap_NoDuplicates`

Build the reverse map (`map[Opcode]string`) by iterating `ScriptOpcodeMap`. Assert `len(reverse) == len(ScriptOpcodeMap)` (i.e., no two names collapse to the same `Opcode`). Catches copy-paste errors during the 393-line literal port.

### 7.7 `TestScriptOpcodePointers_LengthParity`

Assert `len(ScriptOpcodePointers) == 237`. Mirrors `grep -c '\[ScriptOpcode\.' ScriptOpcodePointers.ts` verified at spec-write.

### 7.8 `TestScriptOpcodePointers_SpotChecks`

Assert ~10-15 representative entries match TS exactly:
- `OpAllowDesign` → `{Require: ["active_player"]}` (smoke-tests the simplest shape).
- `OpFindUid` → `{Set: ["active_player"], Set2: ["active_player2"], Conditional: true}` (tests `conditional: true`).
- `OpPArriveDelay` → `{Require: ["p_active_player"], Corrupt: corruptExceptActive("last_com", "last_int", "last_item", "last_slot", "last_targetslot", "last_useitem", "last_useslot")}` (tests the `...POINTER_GROUP_FIND` spread).
- `OpHuntNext` → `{Require: ["find_player"], Require2: ["find_player"], Set: ["active_player"], Set2: ["active_player2"], Conditional: true}` (tests Require + Require2 + Conditional combination).
- One entry that has ONLY `Set` (e.g., `OpHuntAll`).
- One entry that has ONLY `Corrupt` (likely from the find_* family).
- The Db_Listall family (`OpDbListAll`, `OpDbListAllWithCount`) — verifies the last-defined entries are present.

### 7.9 `TestScriptOpcodePointers_KeysAreValidOpcodes`

For every key in `ScriptOpcodePointers`, assert the key is non-zero unless the key is intentionally `OpPushConstantInt` (the zero-value opcode — verify by membership rather than zero-check). The stronger check is: build a `set[Opcode]bool` of every `Op*` constant defined in `pkg/script/opcode.go` and assert every `ScriptOpcodePointers` key is in that set.

Since enumerating all 394 `Op*` constants in the test would be brittle, the test instead asserts a weaker property: every key value is ≤ `OpTimespent` (or whatever the maximum `Op*` constant is). The exact max is documented as a constant in the test, derived from `pkg/script/opcode.go` at spec-write.

### 7.10 `TestScriptOpcodePointers_CorruptExceptActiveCallSites`

Pin deviation `NAI-201-D-POINTERS-SPREAD-HELPER`. Two sub-assertions:

1. Grep `pkg/script/opcode_pointers.go` for the literal `corruptExceptActive(` and assert the count equals **4** (one per simple-spread TS entry: P_ARRIVEDELAY, P_COUNTDIALOG, P_DELAY, P_PAUSEBUTTON — TS lines 286, 301, 314, 370).
2. Assert that the `Corrupt` field on the NPC_DELAY and NPC_ARRIVEDELAY entries contains exactly the literal 12-element slice expected from the extended-spread TS pattern (`p_active_player`, `p_active_player2`, then all 5 PointerGroupFind names, then `last_com`, `last_int`, `last_item`, `last_slot`, `last_targetslot`, `last_useitem`, `last_useslot`). Pins the deliberate literal-expansion-not-helper choice.

If a future entry adds another spread site, the test fails and the author updates the count — preserves the audit trail per `[[retire_deviation_grep_all_comments]]`.

### 7.11 `TestScriptOpcodePointers_PointerGroupFindContent`

Assert `PointerGroupFind == []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}` in that exact order. Mirrors TS POINTER_GROUP_FIND order (ScriptOpcodePointers.ts:3) — order matters because the corrupt-slice content is concatenated in this order.

### 7.12 `TestNAI201Deviations_Pinned`

Grep `pkg/script/` and `pkg/objtype/` for each deviation tag — assert ≥1 match per tag. Tags pinned:
- `NAI-201-D-NPCMODE-QUEUE-TODO`
- `NAI-201-D-POINTERS-SPREAD-HELPER`

Per `[[pin_test_self_trigger_production_doc]]`: pins key on the tag identifier only, not on surrounding TS-source phrases.

## 8. Open questions

Resolved at spec-write — see §9.

## 9. Resolved risks

**R1 — Should `ScriptOpcodeMap` be auto-generated from `Op*` constants via `go:generate`?**
*Risk*: Hand-maintaining 437 lines invites typos. A generator would be authoritative.
*Resolution*: Hand-maintain. The TS source is hand-maintained at the same size; review proceeds line-by-line against TS for parity. `TestScriptOpcodeMap_NoDuplicates` + length-parity catches mass-typo classes. A generator would add a build-step dependency and obscure the TS-↔-goscape line-by-line correspondence that reviewers depend on. Per `[[flat_arg_signature_for_cross_lang_parity]]`, keep the surface flat and reviewable.

**R2 — Field naming: `Pointers` struct vs inline anonymous fields?**
*Risk*: TS uses an inline object-literal type. Goscape could go anonymous-struct for one-to-one shape parity.
*Resolution*: Named struct (`Pointers`). Anonymous structs in 237 map literals would force every entry to repeat the field-type declarations. Named struct lets each entry omit zero-value fields naturally (`{Require: ...}` doesn't need to write `Set: nil, Corrupt: nil, ...`).

**R3 — Should `NpcMode*` constants enumerate all 68 enum values, or only the 48 named in `NpcModeMap`?**
*Risk*: TS `enum NpcMode` declares 68 named values (NULL=-1 through QUEUE20=66); `NpcModeMap` enumerates 48 (NULL through APNPC5; QUEUE1..QUEUE20 are TODO-commented). Goscape needs the constants the NPC AI state machine consumes (modes -1 through 66) — but `NpcModeMap` only needs the 48 named-by-string subset.
*Resolution*: Constants enumerate all 68 (mirrors TS enum). `NpcModeMap` references 48. The 20 unmapped constants (`NpcModeQueue1`..`NpcModeQueue20`) compile cleanly without map entries — no dead-API concern because the NPC AI state machine (separate from NAI-201 scope) will reference these constants when ported. Per `[[true_to_ts_gate]]`, preserve TS enum completeness in the index space; only the name-map is the NAI-201 scope question.

**R4 — Should the `NpcMode*` constants be added now or deferred to whenever the AI state machine ports them?**
*Risk*: Dead-API per `[[dead_api_polish]]` — 68 constants with only 48 mapped.
*Resolution*: Add now. Reason: `NpcModeMap` references 48 of them; without the constants, the map entries can't compile. The remaining 20 (`NpcModeQueue1`..`NpcModeQueue20`) will be needed when the NPC AI state machine ports (a separate sub-spec arc), but they have no `NpcModeMap` entry. Defining unmapped constants is normal Go (unused constants are not flagged). Acceptable.

**R5 — Should `ScriptOpcodeNameMap` (reverse map; TS Compiler.ts:859-861) be added now?**
*Risk*: NAI-202's `runServerCompiler` may need it.
*Resolution*: Skip. Inspect of `runServerCompiler` (Compiler.ts:109-150) shows it iterates `ScriptOpcodeMap.entries()` not `ScriptOpcodeNameMap`. Defer until a NAI-202 consumer surfaces. Per `[[dead_api_polish]]`.

**R6 — Helper `corruptExceptActive` vs literal slice expansion in each call site?**
*Risk*: Helper hides the structure. Literal expansion is grep-direct but verbose.
*Resolution*: Hybrid. The helper covers the 4 simple-spread sites where the leading content IS exactly `POINTER_GROUP_FIND`. The 2 extended-spread sites (NPC_DELAY, NPC_ARRIVEDELAY, with a `p_active_player`/`p_active_player2` prefix before the spread) use literal expansion — the prefix breaks the symmetry and forcing the helper to take a `prefix` parameter would obscure rather than clarify. The pin test in §7.10 maintains the audit trail for both shapes.

**R7 — Order of entries in `ScriptOpcodePointers` Go literal — match TS line ordering, or sort by `Opcode`?**
*Risk*: Random reorder during port loses TS-↔-goscape line correspondence.
*Resolution*: Match TS line ordering verbatim. Reviewers can read TS and Go side-by-side. The Go map type doesn't preserve iteration order, but the literal source order is the only source-level signal of "this section is Player ops, this section is NPC ops" — preserve via inline comments mirroring TS (`// Player ops`, etc.).

**R8 — Do any `ScriptOpcodePointers` entries reference a `find_*` pointer not in `PointerGroupFind`?**
*Risk*: TS could have inconsistent grouping.
*Resolution*: Verified at spec-write via grep `find_` ScriptOpcodePointers.ts: only the 5 names in POINTER_GROUP_FIND appear. No inconsistency. Confirmed pre-flight at the prereq-pointer-coverage layer.

**R9 — `NpcMode*` constants: how do enum values for `NULL=-1` interact with goscape conventions?**
*Risk*: Negative-integer constants are unusual for index spaces in goscape (e.g., `PlayerStatCount = 21` is the convention).
*Resolution*: TS `enum NpcMode { NULL, ... }` starts unnamed at 0, but the explicit `NULL = -1` (TS line 2) shifts the base. Goscape ports the negative value verbatim — it's a sentinel for "no mode assigned", not an index. Compatible with Go's untyped-int constant rules.

**R10 — `NpcMode.QUEUE1`..`QUEUE20` constants: how do their values land if `NpcModeMap` omits them?**
*Risk*: Off-by-one in numbering. TS comments out the entries but the enum values still occupy 47-66.
*Resolution*: Enum values come from `enum NpcMode { ... QUEUE1 = 47, QUEUE2, ..., QUEUE20 = 66 }` (TS line 75-95). Goscape ports the constants explicitly named with their values (`NpcModeQueue1 = 47` through `NpcModeQueue20 = 66`). Map omits them. No off-by-one.

**R11 — Should `OpName` in goscape match TS `NAME` letter-by-letter?**
*Risk*: TS `NAME` is the entry-key value in the map. Goscape needs that exact string for grep parity with `.rs2` source files (which use `NAME(...)` syntax).
*Resolution*: Yes — map keys are TS-verbatim UPPER_SNAKE_CASE (`"NAME"`, not `"Name"`). The Go-side constant name (`OpName`) is goscape convention. The TS-source-faithful string is what `.rs2` parsers will look up.

## 10. Deviations enumerated

- **`NAI-201-D-NPCMODE-QUEUE-TODO`**: TS `NpcModeMap` (NpcMode.ts:98-168) has 20 `QUEUE1`..`QUEUE20` entries commented out with `// TODO: these are not used?`. Goscape's `NpcModeMap` omits these 20 entries (length 48, not 68). The corresponding `NpcMode*` constants (47-66) exist in `pkg/objtype/npcmode.go` so the AI state machine port (out of NAI-201 scope) can reference them, but they have no name-string mapping. Rationale: TS-faithful. If the upstream TS removes the comment, goscape adds the entries; tracked via grep on the tag.

- **`NAI-201-D-POINTERS-SPREAD-HELPER`**: TS `ScriptOpcodePointers` spreads `POINTER_GROUP_FIND` in 6 entries — 4 simple (`[...POINTER_GROUP_FIND, ...extras]`) and 2 extended (`['p_active_player', 'p_active_player2', ...POINTER_GROUP_FIND, ...extras]` at NPC_DELAY and NPC_ARRIVEDELAY). Goscape ports the 4 simple sites via a `corruptExceptActive(extras ...string) []string` helper that concatenates `PointerGroupFind` with the extras; the 2 extended sites use literal slice expansion (the asymmetric prefix breaks helper symmetry). Semantically equivalent (same resulting slice contents in the same order) on all 6 sites. The pin test §7.10 anchors both: helper-call count = 4 AND the two literal-expansion entries match expected 12-element slices.

(No other deviations. `ScriptOpcodeMap` insertion order is unobservable in Go but doesn't affect any consumer per §5.4. `Pointers` struct shape mirrors TS exactly modulo the named-struct decision in R2.)

## 11. Carry-forward (from prior NAI sub-specs)

Per `[[nai_followups]]` audit at spec-write:
- **NAI-191 #1 `LoadFileFull` `TrimLeft` Unicode narrowing** — not on NAI-201 hot path. Leave deferred.
- **NAI-191 #3 `ShouldBuildFileAny` `ReadDir` failure returns false** — not used by NAI-201. Leave deferred.
- **NAI-198 #1 OPOBJ2 upstream reconciliation** — upstream-engagement track. NAI-201 ports `OPOBJ2` as a `Pointers` entry only (data, not behaviour). Out of scope.
- **NAI-199 #1 `frame_del` `endsWith` suffix-match TS-parity** — orthogonal.
- **NAI-200 (just closed) deviations**: `NAI-200-D-DUAL-MAP` lives in `pkg/pack/compiler/typeinfo.go`. NAI-201 doesn't touch that package or that doc — no count-drift risk per `[[adjacent_doc_paragraph_count_drift]]`.

No new NAI-201-bound carry-forwards introduced.

## 12. Arc next step

NAI-201 unblocks **NAI-202: `runServerCompiler` driver port** in `pkg/pack/compiler/`:

1. New file `pkg/pack/compiler/run_server.go` ports `Compiler.ts:109-367`:
   - Build `commandInfo` from `ScriptOpcodeMap` + `ScriptOpcodePointers` (this sub-spec).
   - Load `.constant` files via `pkg/pack.LoadDirExtFull` into `LoadRecords`.
   - Load 22 `.pack` files via `compiler.Load`.
   - Enrich `writeinvInfo` from `LoadInvTypes` (`pkg/objtype.InvType.Protect`).
   - Synthesize `interfaceInfo` / `overlayInfo` from `LoadComponentTypes`.
   - Enrich `varpInfo` / `varnInfo` / `varsInfo` / `paramInfo` from respective `Load*Types`.
   - Synthesize `dbcolumnInfo` from `LoadDbTableTypes`.
   - Build `statInfo` / `npcStatInfo` / `npcModeInfo` via `compiler.LoadMap` (consumes `PlayerStatMap`, `NpcStatMap`, `NpcModeMap`).
   - Build `fontmetricsInfo` / `locshapeInfo` via `compiler.LoadArray` (hard-coded slice literals).
   - Return `map[string]*compiler.TypeInfo` (the symbol-table dict).
2. Stub the `CompileServerScript` call site with a TODO comment marker for NAI-203+; do NOT wire production callers.
3. Per `[[scope_gate_prerequisite_chain]]`: verify each `pkg/objtype.Load*Types` signature and field set at NAI-202 spec-write.

NAI-203+ then opens the **bytecode lexer/parser/typechecker** arc, porting `@lostcityrs/runescript` to a goscape sub-package.

## 13. Acceptance criteria

- `pkg/objtype/npcstat.go` exists with `NpcStat*` constants + `NpcStatMap` (6 entries).
- `pkg/objtype/npcmode.go` exists with `NpcMode*` constants (68 named values, `NpcModeNull = -1` through `NpcModeQueue20 = 66`) + `NpcModeMap` (48 entries, NULL through APNPC5).
- `pkg/script/opcode_map.go` exists with `ScriptOpcodeMap` (393 entries).
- `pkg/script/opcode_pointers.go` exists with `Pointers` struct, `PointerGroupFind`, `corruptExceptActive`, and `ScriptOpcodePointers` (237 entries).
- `go test ./pkg/objtype/... ./pkg/script/...` passes (cleanly, `-race`).
- `go test ./...` passes (no regressions elsewhere — the new declarations have no consumers).
- Deviation pin grep returns ≥1 match for each of: `NAI-201-D-NPCMODE-QUEUE-TODO`, `NAI-201-D-POINTERS-SPREAD-HELPER`.
- TS-↔-goscape parity verified per-registry by reading TS source against the goscape implementation alongside review.
