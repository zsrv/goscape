# NAI-208 — Pointer-flow checker (compiler slice 6a of 6)

**Date:** 2026-05-16
**Series:** Go rewrite of LostCityRS Engine-TS, compiler port (NAI-188 → NAI-210)
**TS pin:** LostCityRS/RuneScriptTS @ `b8c338801fbb72d294ff9576a58925a8d3f6de47`
**Tech stack:** Go 1.26+, `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`
**Commits:** `git commit --no-gpg-sign`

## 1. Context

NAI-207 closed compiler slice 5 (`codegen/`) on 2026-05-15: 47 abstract `Opcode`
singletons, `Instruction`/`Block`/`RuneScript`/`SwitchTable`/`LocalTable`, the
`CodeGenerator` type-switch dispatcher, and 12 dynamic-command handlers under
`pkg/pack/compiler/command/`. `TestPipeline_FullSlice` walks parse → register
→ typecheck → codegen on a 2-script source and asserts the codegen output.

The original "NAI-208" envelope (PointerChecker + writer cohort + driver) is
**~2812 LOC of TS** — far over the cadence's ~300-400-LOC-per-sub-spec target.
This spec decomposes the remainder into three slices and covers only the
first.

| Slice          | Scope                                                              | TS LOC |
| -------------- | ------------------------------------------------------------------ | ------ |
| **NAI-208 (6a, this spec)** | Pointer pkg + CFG + `PointerChecker` + `ServerPointerChecker` + pipeline wiring | ~1126  |
| **NAI-209 (6b)** | `ServerScriptOpcode` + `SymbolMapper` + `BaseScriptWriter` + `BinaryScriptWriter` + `BinaryScriptWriterContext` + `BytePacket` | ~1167  |
| **NAI-210 (6c)** | `BinaryFileScriptWriter` + `JagFileScriptWriter` + `Js5PackScriptWriter` + `ServerScriptCompiler` driver + retire `NAI-207-D-REGISTERALL-NO-FEATURES` | ~519   |

Boundaries: NAI-208 needs no numeric opcodes. NAI-209 produces script blobs
but writes nothing to disk. NAI-210 wires the file output and end-to-end
driver.

## 2. Goals (NAI-208 only)

1. Port TS `compiler/pointer/` (`PointerType` + `PointerHolder`) to a new
   `pkg/pack/compiler/pointer/` package.
2. Port TS `compiler/codegen/script/config/` (`InstructionNode`,
   `PointerInstructionNode`, `GraphGenerator`, `PointerChecker`) to a new
   `pkg/pack/compiler/cfg/` package.
3. Port TS `runescript/ServerPointerChecker` to a new
   `pkg/pack/compiler/runescript/` package (the runescript-driver pkg seed
   that grows in NAI-210).
4. Retire two NAI-205 deferral tags:
   - `NAI-205-D-TRIGGER-POINTERS-DEFERRED` — `TriggerType.Pointers` retypes
     from `any` to `*pointer.PointerSet`.
   - `NAI-205-D-SCRIPTSYMBOL-NO-POINTERS` — `ScriptSymbolFields.Pointers`
     accessor lands (TS `pointers(checker)` method).
5. Extend `TestPipeline_FullSlice` to run `PointerChecker.Run()` after
   codegen and assert zero new diagnostics on the existing 2-script source.

## 3. Out of scope (deferred to NAI-209 / NAI-210)

- Numeric `ServerScriptOpcode` table.
- `SymbolMapper`, `IdProvider`, any `*ScriptWriter`.
- `BinaryScriptWriterContext` byte layout.
- `ServerScriptCompiler` end-to-end driver.
- Feature-gating in `RegisterAllDynCommands` (retires
  `NAI-207-D-REGISTERALL-NO-FEATURES` in NAI-210).
- Macro processing (`compiler/preprocess/MacroProcessor.ts`).

## 4. Architecture

```
pkg/pack/compiler/
  pointer/                          (NEW — TS: src/compiler/pointer/)
    type.go                         — PointerType + 22 singletons + ALL + Index() + ForName()
    holder.go                       — PointerHolder{Required, Set, ConditionalSet, Corrupted}
                                      + PointerSet helpers (Add/Has/Equal/Clone)
  cfg/                              (NEW — TS: src/compiler/codegen/script/config/)
    instruction_node.go             — InstructionNode + PointerInstructionNode
    graph_generator.go              — Blocks → []*InstructionNode w/ pointer-inversion fixup
    pointer_checker.go              — getAnalysis + findEdgePath + calculate/validate
    pointer_checker_labels.go       — buildStaticLabelArgsByCall + getJumpParamNodes
    nai208_deviation_pins_test.go   — close-task deviation pin tests
  runescript/                       (NEW — TS: src/runescript/)
    server_pointer_checker.go       — extends cfg.PointerChecker for IF_BUTTON family
```

Imports:

- `cfg/` imports `codegen/`, `pointer/`, `symbol/`, `trigger/`, `diagnostics/`,
  `semantics/` (for `StrictFeatureLevel`), `type/`.
- `pointer/` imports nothing internal.
- `runescript/` imports `cfg/`, `pointer/`, `symbol/`, `trigger/`,
  `diagnostics/`, `semantics/`, `codegen/`.
- `codegen/`, `command/`, `semantics/`, `trigger/`, `symbol/` do **not** import
  `cfg/` or `runescript/`. No cycles.

## 5. Component-by-component design

### 5.1 `pkg/pack/compiler/pointer/`

```go
// type.go
type PointerType struct {
    Representation string
}

var (
    ActivePlayer    = &PointerType{"active_player"}
    ActivePlayer2   = &PointerType{".active_player"}
    PActivePlayer   = &PointerType{"p_active_player"}
    PActivePlayer2  = &PointerType{".p_active_player"}
    ActiveNpc       = &PointerType{"active_npc"}
    ActiveNpc2      = &PointerType{".active_npc"}
    ActiveLoc       = &PointerType{"active_loc"}
    ActiveLoc2      = &PointerType{".active_loc"}
    ActiveObj       = &PointerType{"active_obj"}
    ActiveObj2      = &PointerType{".active_obj"}
    FindPlayer      = &PointerType{"find_player"}
    FindNpc         = &PointerType{"find_npc"}
    FindLoc         = &PointerType{"find_loc"}
    FindObj         = &PointerType{"find_obj"}
    FindDb          = &PointerType{"find_db"}
    LastCom         = &PointerType{"last_com"}
    LastInt         = &PointerType{"last_int"}
    LastItem        = &PointerType{"last_item"}
    LastSlot        = &PointerType{"last_slot"}
    LastTargetSlot  = &PointerType{"last_targetslot"}
    LastUseItem     = &PointerType{"last_useitem"}
    LastUseSlot     = &PointerType{"last_useslot"}
)

var All = []*PointerType{ /* same order as above */ }

// Index returns the position of pt within All. Panics if absent.
func Index(pt *PointerType) int { ... }

// ForName resolves the lowercase representation to a *PointerType, or nil.
func ForName(name string) *PointerType { ... }
```

```go
// holder.go
type PointerSet struct {
    m map[*PointerType]struct{}
}

func NewPointerSet(items ...*PointerType) *PointerSet
func (s *PointerSet) Add(pt *PointerType)
func (s *PointerSet) Has(pt *PointerType) bool
func (s *PointerSet) Len() int
func (s *PointerSet) All() []*PointerType
func (s *PointerSet) Clone() *PointerSet

type PointerHolder struct {
    Required       *PointerSet
    Set            *PointerSet
    ConditionalSet bool
    Corrupted      *PointerSet
}
```

**Deviation tags:**

- `NAI-208-D-POINTERTYPE-PTR-SINGLETON` — TS uses a class with `private
  constructor`; goscape uses package-level `*PointerType` vars (mirrors
  NAI-207-D-OPCODE-UNTYPED pattern). Pointer identity is the equality key
  (analogous to TS instance identity).
- `NAI-208-D-POINTERSET-MAP-STRUCT` — Go has no built-in `Set<T>`; using
  `map[*PointerType]struct{}` wrapped in a `*PointerSet` helper.
- `NAI-208-D-POINTERHOLDER-PTRSET` — fields are `*PointerSet` (not bare
  `map`) so the helper's identity carries.

### 5.2 Retire NAI-205 deferrals

**`pkg/pack/compiler/trigger/triggertype.go`:**

```go
import "github.com/zsrv/goscape/pkg/pack/compiler/pointer"

type TriggerType struct {
    // ...
    Pointers *pointer.PointerSet  // was: any
}
```

Existing call sites in tests pass `nil` and remain valid.

**`pkg/pack/compiler/symbol/script.go`:**

`ScriptSymbolFields` gains no fields; only a doc note retracting
`NAI-205-D-SCRIPTSYMBOL-NO-POINTERS`. The actual `pointers(*cfg.PointerChecker)`
accessor lives on `cfg.PointerChecker.GetPointers(symbol.Symbol)` rather than
on the symbol type itself, to avoid a `symbol → cfg` import cycle. Updated tag:

- `NAI-208-D-SYMBOL-NO-METHOD-CYCLE-AVOID` — TS adds a `ScriptSymbol.pointers`
  method; goscape lifts it to `cfg.PointerChecker.GetPointers(symbol.Symbol)`
  to keep `symbol/` cycle-free (parallels NAI-207-D-CODEGENCONTEXT-MARKER).

### 5.3 `pkg/pack/compiler/cfg/instruction_node.go`

```go
type InstructionNode struct {
    Instruction *codegen.Instruction  // nil for synthetic start + PointerInstructionNode
    Next        []*InstructionNode
    Previous    []*InstructionNode
}

func (n *InstructionNode) AddNext(other *InstructionNode) {
    n.Next = append(n.Next, other)
    other.Previous = append(other.Previous, n)
}

type PointerInstructionNode struct {
    InstructionNode
    Set *pointer.PointerSet
}
```

Embedding (not subclassing) — Go-idiomatic; reviewers walk `PointerInstructionNode`
as an `InstructionNode` via field promotion.

### 5.4 `pkg/pack/compiler/cfg/graph_generator.go`

Literal port of TS `GraphGenerator.generate(blocks)`. Constructor takes:

```go
func NewGraphGenerator(
    commandPointers map[string]*pointer.PointerHolder,
    features semantics.StrictFeatureLevel,
) *GraphGenerator

func (g *GraphGenerator) Generate(blocks []*codegen.Block) []*InstructionNode
```

`allowPointerInversion` is `!features.DisablePointerInversion` (mirrors TS
`features.pointerInversion !== false`).

Terminal opcodes set: `Branch`, `Jump`, `Return`.
Branch opcodes set: the same 15 listed in TS `BRANCH_OPCODES`.

### 5.5 `pkg/pack/compiler/cfg/pointer_checker.go`

```go
type PointerChecker struct {
    diagnostics     *diagnostics.Diagnostics
    scripts         []*codegen.RuneScript
    commandPointers map[string]*pointer.PointerHolder
    features        semantics.StrictFeatureLevel

    scriptsBySymbol  map[symbol.Symbol]*codegen.RuneScript
    graphGenerator   *GraphGenerator

    scriptGraphs           map[symbol.Symbol][]*InstructionNode
    scriptPointers         map[symbol.Symbol]*pointer.PointerHolder
    scriptAnalyses         map[symbol.Symbol]*scriptPointerAnalysis
    jumpParamNodesByScript map[symbol.Symbol]map[int][]*InstructionNode

    pendingAnalyses map[symbol.Symbol]struct{}
    pendingScripts  map[symbol.Symbol]struct{}
}

func NewPointerChecker(
    d *diagnostics.Diagnostics,
    scripts []*codegen.RuneScript,
    commandPointers map[string]*pointer.PointerHolder,
    features semantics.StrictFeatureLevel,
) *PointerChecker

func (p *PointerChecker) Run()
func (p *PointerChecker) GetGraph(script *codegen.RuneScript) []*InstructionNode
func (p *PointerChecker) GetPointers(sym symbol.Symbol) *pointer.PointerHolder

// SetsPointerTrigger is the public entry-point; its body just delegates to
// the setsPointerTriggerFn field (see §5.7). The default fn is set in
// NewPointerChecker; NewServerPointerChecker overwrites it after embedding.
func (p *PointerChecker) SetsPointerTrigger(script *codegen.RuneScript, pt *pointer.PointerType) bool
```

**`scriptPointerAnalysis`** (unexported) mirrors TS:

```go
type scriptPointerAnalysis struct {
    graph              []*InstructionNode
    required           [][]*InstructionNode  // indexed by Index(pt)
    set                [][]*InstructionNode
    corrupted          [][]*InstructionNode
    setNodes           []map[*InstructionNode]struct{}
    corruptedNodes     []map[*InstructionNode]struct{}
    returns            []*InstructionNode
    staticLabelArgsByCall map[*codegen.Instruction]map[int]symbol.Symbol
}
```

**`findEdgePath`:** literal BFS port — pointer-identity for nodes, queue is
`[]*InstructionNode`, walks `Previous` edges, accumulates path on `end()` hit.

**Diagnostic-message audit-pin:** `MessagePointerUninitialized`,
`MessagePointerCorrupted`, `MessagePointerCorruptedLoc`,
`MessagePointerRequiredLoc` already exist in
`pkg/pack/compiler/diagnostics/messages.go` (lines 113-116, NAI-205-shipped).
T0 audit-pins their presence + format strings against TS HEAD; no add task.

### 5.6 `pkg/pack/compiler/cfg/pointer_checker_labels.go`

`buildStaticLabelArgsByCall`, `getJumpParamNodes`, `requiresPointerAtNodes`,
`addStaticLabelRequirements`. Pure port of TS lines 446-601 with the
`LABEL_JUMP_COMMANDS` set carrying `{"jump", ".jump"}` and the `ARG_PUSH_OPCODES`
set carrying the 7 push-shaped opcodes (`PushConstantInt`, `PushConstantString`,
`PushConstantLong`, `PushConstantSymbol`, `PushLocalVar`, `PushVar`, `PushVar2`).

### 5.7 `pkg/pack/compiler/runescript/server_pointer_checker.go`

```go
type ServerPointerChecker struct {
    *cfg.PointerChecker
    overlayInterfaces map[string]struct{}
}

func NewServerPointerChecker(
    d *diagnostics.Diagnostics,
    scripts []*codegen.RuneScript,
    commandPointers map[string]*pointer.PointerHolder,
    features semantics.StrictFeatureLevel,
    overlayInterfaces []string,
) *ServerPointerChecker

// Overrides SetsPointerTrigger by check-then-delegate.
func (s *ServerPointerChecker) SetsPointerTrigger(
    script *codegen.RuneScript, pt *pointer.PointerType,
) bool
```

The interface-button family (`IF_BUTTON`, `INV_BUTTON1..5`, `INV_BUTTOND`) is
identified by a triggertype-id lookup — `ServerTriggerType` lives in a sibling
file that **NAI-208 does not port in full**; it ports only the seven trigger
constants the override checks, behind a tag:

- `NAI-208-D-TRIGGER-PARTIAL-PORT` — `runescript.ServerTriggerType` ports only
  the 7 button triggers SetsPointerTrigger consults; full enum + `RegisterAll`
  hook lands in NAI-210.

Inversion-of-control: since Go has no virtual-method dispatch, the production
wiring constructs a `*ServerPointerChecker` and calls `s.Run()` on the embedded
`PointerChecker`. The embedded `Run` loop must call `SetsPointerTrigger`
through an indirection — we lift it to a function-pointer field on the embedded
struct that the `ServerPointerChecker` constructor overwrites:

```go
type PointerChecker struct {
    // ...
    setsPointerTriggerFn func(*codegen.RuneScript, *pointer.PointerType) bool
}
```

Default implementation set in `NewPointerChecker`; `NewServerPointerChecker`
overwrites it after embedding. Tag:

- `NAI-208-D-VIRTUAL-VIA-FNFIELD` — TS uses `protected override`; goscape uses
  a function-pointer field on the base struct overwritten by the subclass
  constructor (no virtual dispatch in Go).

### 5.8 Pipeline wiring

`codegen/smoke_test.go::TestPipeline_FullSlice` extends:

```go
cg := codegen.NewCodeGenerator(root, dyn, d)
cg.Visit(sf)
// NEW:
pc := cfg.NewPointerChecker(d, cg.Scripts(), map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
pc.Run()
if d.HasErrors() {
    t.Fatalf("pointer-check diagnostics: %+v", d.List())
}
```

With an empty `commandPointers` map and no `Pointers` field set on the `proc`
trigger, no script should set/require any pointer. The existing smoke source
(arithmetic + if/else + while + gosub) does not reference player/npc state, so
zero diagnostics is the expected outcome.

A second test, `TestPipeline_FullSlice_WithPointerRequirement`, constructs a
fixture where a command requires `active_player` and the proc does NOT have
`active_player` in its trigger.Pointers — asserts exactly one
`MessagePointerUninitialized` diagnostic.

## 6. Test strategy

**Per-package green:**

- `pkg/pack/compiler/pointer/`:
  - All 22 singletons have unique pointer identity.
  - `len(All) == 22` and `Index(p)` round-trips.
  - `ForName("active_player") == ActivePlayer`; `ForName("ACTIVE_PLAYER") == ActivePlayer`; `ForName("nope") == nil`.
  - `PointerSet.Add/Has/Clone/All` smoke.
- `pkg/pack/compiler/cfg/`:
  - `InstructionNode.AddNext` populates both endpoints.
  - `GraphGenerator.Generate` on a single-block straight-line: N+1 nodes (1 start + N instructions), edges chain head-to-tail.
  - `GraphGenerator.Generate` on a 2-block branch fixture: correct `Next` edges for both `Branch` opcodes and fallthrough.
  - `GraphGenerator` pointer-inversion: a fixture with `Command(cs-cond) + PushConstantInt(0) + BranchEquals` injects a `PointerInstructionNode` ahead of the branch when `commandPointers[cs-cond].conditionalSet == true`.
  - `PointerChecker.GetPointers` on a single-block return-only script: empty required/set/corrupted.
  - `PointerChecker.Run` on a script with a `Command` whose holder requires `active_player`, with empty trigger pointers: emits exactly one `MessagePointerUninitialized` diagnostic against the command's source.
  - `PointerChecker.Run` on a script whose trigger satisfies the required pointer: no diagnostics.
  - Recursive-gosub: A gosubs B gosubs A — neither `getAnalysis` nor `calculatePointers` recurses (pendingScripts/pendingAnalyses short-circuit returns empty holder).
  - `findEdgePath` empty-`starts` returns nil; blocked-only paths return nil; reachable end returns path with correct head + tail nodes.
- `pkg/pack/compiler/runescript/`:
  - `ServerPointerChecker.SetsPointerTrigger` for `P_ACTIVE_PLAYER` + `IF_BUTTON` trigger + non-overlay subject: returns true.
  - Same, with overlay subject (matched by lowercase-normalised name): returns false.
  - For non-`P_ACTIVE_PLAYER` pointers: delegates to embedded `PointerChecker.SetsPointerTrigger`.
- `pkg/pack/compiler/trigger/`:
  - Existing tests still green after `Pointers` field retype (nil literal still valid).

**Pipeline smoke (`codegen/smoke_test.go`):**

- `TestPipeline_FullSlice` extended with `PointerChecker.Run()` post-codegen; asserts zero diagnostics on the existing 2-script source.
- `TestPipeline_FullSlice_WithPointerRequirement` (new) asserts exactly one diagnostic when a required pointer is unset.

**Deviation-pin tests** (`pkg/pack/compiler/cfg/nai208_deviation_pins_test.go`):

Mirrors NAI-207's format. One `TestPin_NAI208_D_*` per tag, plus a grep walker that confirms every living tag appears in at least one `.go` file under the repo.

## 7. Deviation tag inventory (NAI-208)

| Tag | Reason |
| --- | --- |
| `NAI-208-D-POINTERTYPE-PTR-SINGLETON` | TS class with private ctor → Go `*PointerType` package vars (pointer identity). |
| `NAI-208-D-POINTERSET-MAP-STRUCT` | No built-in Set<T> → `map[*PointerType]struct{}` wrapper. |
| `NAI-208-D-POINTERHOLDER-PTRSET` | Holder fields are `*PointerSet` not bare maps. |
| `NAI-208-D-SYMBOL-NO-METHOD-CYCLE-AVOID` | TS `ScriptSymbol.pointers(checker)` method lifted to `cfg.PointerChecker.GetPointers(symbol)` to avoid symbol→cfg cycle. |
| `NAI-208-D-VIRTUAL-VIA-FNFIELD` | TS `protected override setsPointerTrigger` → function-pointer field on base struct overwritten by subclass ctor. |
| `NAI-208-D-TRIGGER-PARTIAL-PORT` | `runescript.ServerTriggerType` ports only the 7 button triggers `SetsPointerTrigger` consults; full enum lands in NAI-210. |

## 8. Retired tags (other-NAI deferrals NAI-208 closes)

| Tag | Origin | Closed by |
| --- | ------ | --------- |
| `NAI-205-D-TRIGGER-POINTERS-DEFERRED` | trigger/triggertype.go | T1 retype |
| `NAI-205-D-SCRIPTSYMBOL-NO-POINTERS`  | symbol/script.go       | T1 + T4 (GetPointers lives on PointerChecker) |

## 9. Task cohort (preview — plan doc is canonical)

| T | Scope | Approx LOC Go | Cohort |
| - | ----- | ------------- | ------ |
| 0 | Audit-pin: enumerate retired deferrals; verify diagnostic-template + trigger-field presence at HEAD; verify diagnostic-message format strings byte-match TS | n/a | foundation |
| 1 | `pointer/` package (type + holder + set) + retype `TriggerType.Pointers` + retract symbol/script.go tag | ~250 | foundation |
| 2 | `cfg.InstructionNode` + `cfg.PointerInstructionNode` | ~80 | A |
| 3 | `cfg.GraphGenerator` (incl. pointer-inversion + `StrictFeatureLevel.DisablePointerInversion`) | ~350 | A |
| 4 | `cfg.PointerChecker` core: `getAnalysis` + `findEdgePath` + `calculatePointers` + `GetPointers` | ~350 | B |
| 5 | `cfg.PointerChecker` validation: `validatePointer` + `logProcRequirement` + diagnostic emission | ~300 | B |
| 6 | `cfg.PointerChecker` static-label-args: `buildStaticLabelArgsByCall` + `getJumpParamNodes` + `requiresPointerAtNodes` + `addStaticLabelRequirements` | ~200 | B |
| 7 | `runescript.ServerPointerChecker` (IF_BUTTON family + overlayInterfaces + partial `ServerTriggerType` port) | ~150 | C |
| 8 | Pipeline wiring: extend `TestPipeline_FullSlice` + add `TestPipeline_FullSlice_WithPointerRequirement` | ~80 | close-prep |
| 9 | NAI-208 close: `nai208_deviation_pins_test.go` (per-tag pins + grep walker) + close commit with `Closes memory:` trailer | ~150 | close |

Total: ~1900 LOC Go for ~1126 LOC TS (1.7× expansion typical of port).

## 10. Open risks

1. **Recursive-gosub cycle in `getAnalysis`** — TS uses `pendingScripts` /
   `pendingAnalyses` sets; goscape must match exactly. T4 includes a fixture
   for A→B→A and asserts no infinite recursion + empty holder return.
2. **Pointer identity semantics** — using `*PointerType` as the equality key
   means callers must always reference the package-level singletons, never
   construct fresh ones. The grep walker in T9 enumerates any
   `pointer.PointerType{` struct-literals as evidence of misuse.
3. **`StrictFeatureLevel.DisablePointerInversion` is new** in goscape (already
   in `strict_feature.go` from NAI-205 as field index 11); T0 verifies field
   presence pre-flight.
4. **No `commandPointers` registry exists yet** in goscape — the
   `map[string]*pointer.PointerHolder` is an empty map at pipeline wiring
   time. Population belongs to NAI-210's driver-setup; for NAI-208 the smoke
   uses an empty map.
5. **`ServerTriggerType` partial port** — T7's seven-constant port satisfies
   the override but is a load-bearing subset. Reviewer must catch any future
   `ServerPointerChecker` code that references additional `ServerTriggerType`
   constants and either add them to the partial port or escalate to NAI-210.

## 11. Cadence

Per `[[runescript_cadence]]`:

1. ✅ Brainstorm (this session).
2. ✅ Spec (this doc) — commit as `docs(spec): NAI-208 PointerChecker (compiler slice 6a of 6)`.
3. ⏭ Plan doc — `docs/superpowers/plans/2026-05-16-nai-208-pointer-checker.md`, written via `superpowers:writing-plans`. Per-task checkbox TDD with full code blocks. Commit as `docs(plan): ...`.
4. ⏭ Execution — `superpowers:subagent-driven-development` with implementer + reviewer on Sonnet, controller stays on Opus. Controller pre-flight per `[[controller_preflight]]`. Two-stage review per task.
5. ⏭ NAI-208 close commit with `Closes memory: [[nai207_codegen_close]]` (partial — codegen surface consumed) trailer.
