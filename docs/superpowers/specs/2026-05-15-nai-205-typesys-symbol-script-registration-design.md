# NAI-205 — Type system + symbol table + ScriptRegistration (compiler slice 3 of 6)

## 0. Pre-context: where this slice sits in the arc

NAI-204 (closed at `28f8d53`, polished at `c3aa59c`) shipped `pkg/pack/compiler/ast/` + `pkg/pack/compiler/parser/` — byte-level RuneScript source becomes an `*ast.ScriptFile` (or `nil` + listener-reported syntax errors). All semantic-analysis fields on AST nodes (`Script.Symbol`, `Expression.Type`, `Identifier.Reference`, …) were deliberately left absent under `NAI-204-D-AST-NO-TYPE-FIELDS`. The TS compiler closes the loop with two `AstVisitor<void>` passes — `ScriptRegistration.ts` (registers script symbols + validates trigger/subject/parameters/returns) and `TypeChecking.ts` (the main 1546-LOC type-checker). Both consume a shared infrastructure layer: type/symbol/trigger/diagnostic packages totalling another ~1300 TS LOC. Until that infrastructure exists, neither pass can land.

**Slice arc update.** NAI-204 §0 advertised five compiler slices (NAI-203 lexer → NAI-204 parser → NAI-205 type checker → NAI-206 codegen → NAI-207 driver). Hand-porting all of TypeChecking.ts + its dependency closure as one slice projects to ~12k plan LOC and 12+ tasks — far past NAI-204's already-largest envelope (5569 plan LOC, 11 tasks). The arc is hereby renumbered:

| Slice | Original arc | New arc |
|---|---|---|
| 1 | NAI-203 lexer (closed) | unchanged |
| 2 | NAI-204 parser (closed) | unchanged |
| 3 | NAI-205 type checker + symbol resolution | **NAI-205: type/symbol/trigger/diagnostic foundations + ScriptRegistration first pass** |
| 4 | NAI-206 bytecode emitter | **NAI-206: TypeChecking second pass + retire NAI-204-D-AST-NO-TYPE-FIELDS** |
| 5 | NAI-207 driver | **NAI-207: bytecode emitter** |
| 6 | — | **NAI-208: driver wire-up + writer** |

The split is the recommended scope-control move; bundling NAI-205+NAI-206 was rejected because each half stays within the NAI-203/204 envelope and each half has a clear single integration point. NAI-205 lands the support packages and exercises them via `ScriptRegistration` (a 457-LOC, self-contained first pass that writes a known six-field subset of the deferred AST fields). NAI-206 then lands the much larger `TypeChecking` against an already-known-good support layer.

## 1. Goal

Hand-port the RuneScriptTS compiler infrastructure that `ScriptRegistration` and `TypeChecking` both depend on, plus `ScriptRegistration` itself. After this slice:

- `pkg/pack/compiler/diagnostics/` exposes `Diagnostic`, `Diagnostics`, `DiagnosticType`, `DiagnosticMessage` (template constants), and a `Handler` interface. `BaseDiagnosticsHandler` (file-reading, stdout-printing, `process.exit(1)` semantics) is deferred to NAI-208.
- `pkg/pack/compiler/type/` exposes the `Type` interface and its concrete implementations: `PrimitiveType`, `TupleType`, `MetaType` (Any/Nothing/Error/Unit + the parameterised Type/Script forms), `ArrayType`, `VarPlayerType`/`VarBitType`/`VarNpcType`/`VarSharedType` (the four `GameVarType` shapes), plus `TypeManager` with full name-lookup + `allowArray` suffix-stripping + assignability-checker registration.
- `pkg/pack/compiler/symbol/` exposes the symbol hierarchy (`Symbol` marker interface + `LocalVariableSymbol`, `BasicSymbol`, `ConstantSymbol`, `ServerScriptSymbol`, `ClientScriptSymbol`), the discriminated `SymbolType` (the `kind`-tagged union TS uses for symbol-table keying), and `SymbolTable` with insert/find/findAll plus parent-chain lookup and `createSubTable`.
- `pkg/pack/compiler/trigger/` exposes `TriggerType` (interface), `SubjectMode` (None/Name/Type/category/global discriminator), `TriggerManager` (name→trigger registry), and the `CommandTrigger` singleton. The full ~70-trigger registry seed is deferred to NAI-208 (or whichever slice owns driver wire-up); tests in NAI-205 build minimal `TriggerManager` fixtures per test.
- `pkg/pack/compiler/semantics/` exposes `ScriptRegistration` — a pure function/struct that walks an `*ast.ScriptFile`, registers each script's `ServerScriptSymbol` into a passed-in root `SymbolTable`, and writes six fields onto each `Script` node + one field onto each `Parameter`. Reports diagnostics for trigger/subject/parameter/return mismatches via the passed-in `Diagnostics` sink.
- `pkg/pack/compiler/ast/` gains a strict subset of the NAI-204-deferred fields — only the seven `ScriptRegistration` actually writes:
  - `Script.TriggerType` (`*trigger.TriggerType`)
  - `Script.Symbol` (`*symbol.ServerScriptSymbol`)
  - `Script.Block` (`*symbol.SymbolTable`)
  - `Script.ParameterType` (`type.Type`)
  - `Script.ReturnType` (`type.Type`)
  - `Script.SubjectReference` (`*symbol.BasicSymbol`)
  - `Parameter.Symbol` (`*symbol.LocalVariableSymbol`)
- The `NAI-204-D-AST-NO-TYPE-FIELDS` deviation tag is **narrowed**, not retired. Its doc-comment scope contracts from "all semantic fields" to "TypeChecking-owned semantic fields" (the nine remaining: `Expression.Type/TypeHint`, `Identifier.Reference`, `SwitchStatement.DefaultCase/Type`, `DeclarationStatement.Symbol`, `ArrayDeclarationStatement.Symbol`, `CallExpression.Symbol`, `Literal.Reference`, `ConstantVariableExpression.SubExpression`, `StringLiteral.SubExpression`, `VariableExpression.Reference`). NAI-206 retires it fully.
- One end-to-end smoke: parser → ScriptRegistration → assert that a small `[proc,foo]` + `[label,bar]` script-file fixture lands a `ServerScriptSymbol` per script in the root `SymbolTable` with no diagnostics.

## 2. Scope

**In:**

- New package `pkg/pack/compiler/diagnostics/`:
  - `type.go` — `DiagnosticType` enum (Info/Hint/Warning/Error/SyntaxError).
  - `diagnostic.go` — `Diagnostic` struct (`Type`, `SourceLocation`, `Message`, `MessageArgs []any`), `IsError()`. Plus `NewDiagnostic(loc lexer.NodeSourceLocation, t DiagnosticType, msg string, args ...any) Diagnostic` and `NewDiagnosticAt(node ast.Node, …) Diagnostic` (the node-overload sugar).
  - `diagnostics.go` — `Diagnostics` container with `Report`, `Diagnostics() []Diagnostic`, `HasErrors()`.
  - `handler.go` — `Handler` interface (HandleParse/HandleTypeChecking/HandleCodeGeneration/HandlePointerChecking, each optional → interface with default-noop concrete `NopHandler` since Go has no optional methods).
  - `messages.go` — typed constant set `MessageXxx string` for every `DiagnosticMessage` template literal (verbatim format-string ports, comments cite TS file:line).
  - `report_helpers.go` — `ReportAt(d *Diagnostics, node ast.Node, t DiagnosticType, msg string, args ...any)` + `ReportErrorAt`; replaces the TS `Node.reportError` method. Deviation `NAI-205-D-NO-NODE-REPORT-ERROR`.

- New package `pkg/pack/compiler/type/`:
  - `basevartype.go` — `BaseVarType` enum (Integer/Long/String, integer values matching TS 0/1/2).
  - `options.go` — `TypeOptions` struct (`AllowSwitch`, `AllowArray`, `AllowDeclaration`, `AllowParameter` — all bool, defaults all-true via constructor). TS exports both a readonly interface + a mutable class; goscape uses one mutable struct + a value-typed builder helper. Deviation `NAI-205-D-TYPEOPTIONS-FLAT`.
  - `type.go` — `Type` interface: `Representation() string`, `Code() (string, bool)` (TS `code?: string` ⇒ Go second-return-ok), `BaseType() (BaseVarType, bool)`, `DefaultValue() any`, `Options() TypeOptions`. The TS `createType(...)` plain-object factory is not modeled — concrete types implement the interface directly.
  - `primitive.go` — `PrimitiveType` struct + the seven singletons (`PrimitiveInt`, `PrimitiveBoolean`, `PrimitiveCoord`, `PrimitiveString`, `PrimitiveChar`, `PrimitiveLong`, `PrimitiveMapzone`). Plus `PrimitiveAll []*PrimitiveType` and `PrimitiveByRepresentation(rep string) *PrimitiveType`.
  - `tuple.go` — `TupleType{Children []Type}` + `NewTupleType(children ...Type) (Type, error)` (TS throws on `<2` children; Go returns error), `TupleTypeFromList(types []Type) Type` (returns `MetaUnit` for empty, first element for one, tuple for ≥2), `TupleToList(t Type) []Type`.
  - `meta.go` — `MetaType` struct + the four named singletons (`MetaAny`, `MetaNothing`, `MetaError`, `MetaUnit`). Plus two parameterised constructors: `NewMetaTypeWrapping(inner Type) Type` (TS `MetaType.Type`) and `NewMetaScript(trigger *trigger.TriggerType, params Type, returns Type) Type` (TS `MetaType.Script`). The TS static-nested-class pattern flattens to two factory functions returning a `*metaWrapped`/`*metaScript` struct each implementing `Type`. Deviation `NAI-205-D-METATYPE-FLAT`.
  - `wrapped.go` — `WrappedType` interface (`Inner() Type`) used by both ArrayType and the four GameVarTypes.
  - `array.go` — `ArrayType{inner Type}` + `NewArrayType(inner Type) (*ArrayType, error)` (TS throws on nested ArrayType; Go returns error).
  - `gamevar.go` — `VarPlayerType`, `VarBitType`, `VarNpcType`, `VarSharedType` — each wraps an inner Type, supplies its own representation prefix (`varp<…>`/`varbit<…>`/`varn<…>`/`vars<…>`).
  - `manager.go` — `TypeManager` (private `nameToType map[string]Type`, `checkers []TypeChecker`). Methods `Register(name string, t Type) error`, `RegisterByRepresentation(t Type) error`, `RegisterNew(name, code string, base BaseVarType, defaultVal any, build func(*TypeOptions)) (Type, error)`, `RegisterAll(types []Type) error`, `ChangeOptions(name string, build func(*TypeOptions)) error`, `Find(name string, allowArray bool) (Type, error)`, `FindOrNil(name string, allowArray bool) Type`, `AddTypeChecker(c TypeChecker)`, `Check(left, right Type) bool`. TS thrown errors → Go returned errors. `TypeChecker = func(left, right Type) bool`.

- New package `pkg/pack/compiler/symbol/`:
  - `symbol.go` — `Symbol` marker interface (`SymbolName() string`); `LocalVariableSymbol{Name; Type type.Type}`, `BasicSymbol{Name; Type type.Type; IsProtected bool}`, `ConstantSymbol{Name; Value string}`.
  - `script.go` — `ScriptSymbol{Trigger *trigger.TriggerType; Name string; Parameters type.Type; Returns type.Type}` + `ServerScriptSymbol` / `ClientScriptSymbol` wrappers (TS uses subclass; Go uses type alias of struct value with same fields and a discriminating method `IsServerScript() bool`). The TS `pointers(checker)` method is deferred. Deviation `NAI-205-D-SCRIPTSYMBOL-NO-POINTERS`.
  - `symboltype.go` — `SymbolType` struct (`Kind SymbolKind`, `TriggerType *trigger.TriggerType` (set when Kind in {ServerScript, ClientScript}), `BasicType type.Type` (set when Kind == Basic)) plus `SymbolKind` enum (ServerScript/ClientScript/LocalVariable/Basic/Constant). Factory functions `SymbolTypeServerScript(t *trigger.TriggerType)`, `SymbolTypeClientScript(t)`, `SymbolTypeLocalVariable()`, `SymbolTypeBasic(t type.Type)`, `SymbolTypeConstant()`. Equality is by `(Kind, Trigger, BasicType.Representation())` via a derived `key() string` method. Deviation `NAI-205-D-SYMBOLTYPE-STRING-KEY`.
  - `table.go` — `SymbolTable{parent *SymbolTable; symbols map[string]map[string]Symbol}` (outer key = `SymbolType.key()`, inner = normalised name). Methods `Insert(t SymbolType, s Symbol) bool`, `Find(t SymbolType, name string) Symbol`, `FindAll(name string) []Symbol`, `CreateSubTable() *SymbolTable`. Name normalisation: lowercase + whitespace → `_` only for `Kind == Basic` (matches TS `normalizeName`).

- New package `pkg/pack/compiler/trigger/`:
  - `subjectmode.go` — `SubjectMode` interface; package-level `ModeNone`, `ModeName` singletons; `NewModeType(t type.Type, category, global bool) SubjectMode` for the parameterised case. TS `static SubjectMode.Type(...)` factory flattens to a Go constructor. `IsTypeMode(SubjectMode) (TypeMode, bool)` helper.
  - `triggertype.go` — `TriggerType` is a struct (TS makes it an interface implemented by trigger objects; goscape makes it a struct with all fields exported, since every implementation is a frozen data record): `ID int; Identifier string; SubjectMode SubjectMode; AllowParameters bool; Parameters type.Type; AllowReturns bool; Returns type.Type; Pointers any` (Pointers stay `any` since pointer/PointerType is deferred to NAI-207-codegen — comments tag the field with `NAI-205-D-TRIGGER-POINTERS-DEFERRED`).
  - `manager.go` — `TriggerManager` with `Register(name string, t *TriggerType) error`, `RegisterTrigger(t *TriggerType) error`, `RegisterAll(triggers []*TriggerType) error`, `Find(name string) (*TriggerType, error)`, `FindOrNil(name string) *TriggerType`.
  - `command.go` — `CommandTrigger` package-level singleton (`*TriggerType` with the same field set as TS).

- New package `pkg/pack/compiler/semantics/`:
  - `strict_feature.go` — `StrictFeatureLevel` struct with explicit bool fields (`Procs`, `Enums`, `Structs`, `DBTables`, `Booleans` — each a `*bool` so unset is distinguishable from explicit-false, mirroring TS `{ procs?: boolean }` partial-record). Or, simpler: bool with documented default-zero = "permissive" (TS uses missing-key = true). Trade-off discussed in §6.5; spec picks `*bool` for true TS parity.
  - `script_registration.go` — `ScriptRegistration` struct + `NewScriptRegistration(typeManager *type.TypeManager, triggerManager *trigger.TriggerManager, rootTable *symbol.SymbolTable, diagnostics *diagnostics.Diagnostics, features StrictFeatureLevel) *ScriptRegistration`. One public method `Visit(file *ast.ScriptFile)` (mirrors `visitScriptFile`). All other walker logic is private methods.
  - `script_registration_test.go` + `parameter_test.go` + `subject_test.go` + `trigger_check_test.go` + `smoke_test.go`.
  - `nai205_deviation_pins_test.go` — pins for every `NAI-205-D-*` tag actually used.

- Modifications to `pkg/pack/compiler/ast/`:
  - Add the seven fields named in §1 onto `Script` and `Parameter` (file: `scriptfile.go`).
  - Narrow the `NAI-204-D-AST-NO-TYPE-FIELDS` doc-comment on `Script`, `Parameter`, `Expression`, `Identifier` to the NAI-206-owned residue.
  - Keep `TestPin_NAI204D_AstNoTypeFields` passing (tag string remains present, just narrower text after the colon).

**Out (deferred):**

- Everything `TypeChecking.ts` does — that's NAI-206. Specifically: every AST field not listed in §1's seven-field subset, all 1546 lines of TypeChecking.ts, and full retirement of `NAI-204-D-AST-NO-TYPE-FIELDS`.
- `BaseDiagnosticsHandler` — file-reading + stdout-printing + `process.exit(1)`. Deferred to NAI-208 driver. NAI-205 ships only the `Handler` interface + a `NopHandler` for test injection.
- The static trigger registry — ~70 OSRS trigger definitions (`proc`, `label`, `opheld`, `opheld1`, `ai_opplayer1`, …). NAI-205 builds them ad-hoc inside test functions; the production seed lands with NAI-208 driver wire-up.
- The static type registry — bridging NAI-200's `CompilerTypeInfo` (`namedobj`, `obj`, `npc`, `loc`, `seq`, …) into a `TypeManager`. Deferred to NAI-208 driver wire-up.
- `ScriptSymbol.pointers(checker)` and the `pointer/PointerChecker` import it pulls in. Deviation `NAI-205-D-SCRIPTSYMBOL-NO-POINTERS` lifts when NAI-207 codegen lands.
- AST `Visitor` interface + per-node `Accept` methods. Carries forward `NAI-204-D-AST-NO-VISITOR` — ScriptRegistration dispatches via Go type-switch.
- Macro-lookup / preprocessing diagnostics (`MacroLookup`, `MacroOrigin`). Not consumed by ScriptRegistration; deferred to whichever slice ports the preprocessor.
- Bytecode emission and the `Codegen` package — NAI-207.
- Production wiring — nothing in `cmd/goscape` imports `semantics/` yet; NAI-208 wires `ScriptRegistration` + `TypeChecking` into the driver.

## 3. Tech stack

- Go 1.26+ (per [[go_version]] memory).
- No new external deps.
- TS source-of-truth: `/home/owner/Code/github.com/LostCityRS/RuneScriptTS` at HEAD `b8c338801fbb72d294ff9576a58925a8d3f6de47` (same pin as NAI-203/204). In-scope files:
  - `src/compiler/diagnostics/` — all five files (Diagnostic, DiagnosticType, Diagnostics, DiagnosticMessage, DiagnosticsHandler; the BaseDiagnosticsHandler portion of the last file is deferred).
  - `src/compiler/type/` — all eight top-level files + `wrapped/{ArrayType, GameVarType, WrappedType}.ts`.
  - `src/compiler/symbol/` — all four files; `pointers(checker)` method deferred.
  - `src/compiler/trigger/` — all four files (`TriggerType.ts`, `TriggerManager.ts`, `SubjectMode.ts`, `CommandTrigger.ts`). `Pointers` field on TriggerType kept as `any` until pointer-package lands.
  - `src/compiler/StrictFeatureLevel.ts` — small (~30 LOC).
  - `src/compiler/semantics/ScriptRegistration.ts` — full port (457 LOC).
- Goscape deps used (existing): `pkg/pack/compiler/ast`, `pkg/pack/compiler/lexer` (`NodeSourceLocation`).

## 4. Non-goals

- **No** TypeChecking — that's NAI-206. Pin tests in `parser/nai204_deviation_pins_test.go` still pass post-NAI-205 because the deviation tag is narrowed, not removed.
- **No** code generation, bytecode emission, opcode tables. NAI-207.
- **No** pointer-tracking infrastructure (`PointerChecker`, `PointerHolder`). NAI-207 codegen owns it.
- **No** production registry of triggers or types. Test fixtures inject minimal `TypeManager`/`TriggerManager` instances ad-hoc.
- **No** driver-level diagnostic printing — `BaseDiagnosticsHandler` is NAI-208.
- **No** preprocessor / macro support. ScriptRegistration ignores macros (TS does too; macros are an earlier pipeline stage).
- **No** AST visitor pattern — continues the `NAI-204-D-AST-NO-VISITOR` deviation. ScriptRegistration uses type-switch.

## 5. Architecture

### 5.1. Package dependency DAG

```
lexer ←──── ast ←──────── diagnostics
              ↑                ↑
              │                │
              │      type ←────┤
              │       ↑        │
              │       │        │
              │      symbol    │
              │       ↑        │
              │       │        │
              │      trigger ──┘
              │       ↑
              │       │
              └─── semantics
```

No cycles. `ast` depends only on `lexer` (existing). `diagnostics` depends on `lexer` (for `NodeSourceLocation`) and `ast` (for the node-overload helper `NewDiagnosticAt`). `type` is leaf-most among the new packages. `symbol` depends on `type` (for `BasicSymbol.Type`, etc.). `trigger` depends on `type` (`Parameters`, `Returns`, `SubjectMode.Type(t)`). `semantics` depends on everything.

Note that **`ast` does not import `symbol`/`type`/`trigger`** even though `Script.Symbol` is `*symbol.ServerScriptSymbol`. This would be a cycle (`symbol → type → … but symbol fields on ast.Script ⇒ ast → symbol`). Resolution: see §5.2.

### 5.2. Resolving the ast → symbol back-reference

TS doesn't have this problem because ScriptSymbol fields on AST nodes are typed as a TS class which can freely reference back (TS modules form a free graph). Go's package-level cycle ban forces a choice. Options considered:

- **A. Side-table.** Don't put fields on AST nodes; semantics maintains `map[ast.Node]symbol.Symbol`. Preserves TS-faithful field naming in semantics code but loses the field-on-node API; consumers must lookup-by-node. Rejected: NAI-206 (TypeChecking) writes nine more fields and would compound the side-table approach.
- **B. Untyped fields (`any`).** `Script.Symbol any` + type-assert at read. Loses static typing. Rejected.
- **C. Interface placeholders in ast.** `ast.Script.Symbol scriptSymbolMarker` where `scriptSymbolMarker` is a minimal interface in `ast` that `*symbol.ServerScriptSymbol` satisfies structurally. This is the [[interface_at_cyclic_import_boundary]] pattern already in use elsewhere in goscape. **Chosen.**

Concretely, `ast/symbol_refs.go` (new file) declares four marker interfaces, each carrying a single **exported** zero-arg marker method. Exported method names are required: Go interface satisfaction requires the implementing package to spell the same method name, and unexported names are scoped to the declaring package. The interface methods stay zero-arg / no-return so the marker carries no semantic load — it exists only to gate which concrete types may flow into the field.

```go
// pkg/pack/compiler/ast/symbol_refs.go
package ast

// SymbolRef is the cyclic-import bridge for symbol pointers stored on
// AST nodes (Script.Symbol, Script.SubjectReference, Parameter.Symbol).
// Concrete impls live in pkg/pack/compiler/symbol; ast holds only the
// structural marker. See deviation NAI-205-D-AST-REF-INTERFACES.
type SymbolRef interface{ AsSymbolRef() }

// TriggerRef likewise for *trigger.TriggerType on Script.TriggerType.
type TriggerRef interface{ AsTriggerRef() }

// TypeRef likewise for type.Type on Script.ParameterType / .ReturnType.
type TypeRef interface{ AsTypeRef() }

// SymbolTableRef likewise for *symbol.SymbolTable on Script.Block.
type SymbolTableRef interface{ AsSymbolTableRef() }
```

```go
// in pkg/pack/compiler/symbol/script.go
func (*ServerScriptSymbol) AsSymbolRef() {}
func (*ClientScriptSymbol) AsSymbolRef() {}
func (*BasicSymbol) AsSymbolRef()        {}
func (*LocalVariableSymbol) AsSymbolRef() {}
func (*SymbolTable) AsSymbolTableRef()    {}
// in pkg/pack/compiler/trigger/triggertype.go
func (*TriggerType) AsTriggerRef() {}
// in pkg/pack/compiler/type/{primitive,meta,tuple,array,gamevar}.go
func (*PrimitiveType) AsTypeRef()    {}
func (*metaPrimitive) AsTypeRef()    {} // and one per Meta variant
func (*TupleType) AsTypeRef()        {}
func (*ArrayType) AsTypeRef()        {}
// (etc. for each GameVarType)
```

`ast` does not import `symbol`/`trigger`/`type`; consumers in `semantics` (and future `codegen`) type-assert to the concrete type at the read site (`s.Symbol.(*symbol.ServerScriptSymbol)`). Deviation tag `NAI-205-D-AST-REF-INTERFACES`.

### 5.3. ScriptRegistration dispatch shape

TS `ScriptRegistration extends AstVisitor<void>` overrides `visitScriptFile`, `visitScript`, `visitParameter`. Three nodes only. Goscape uses a type-switch in `Visit(file *ast.ScriptFile)`:

```go
func (sr *ScriptRegistration) Visit(file *ast.ScriptFile) {
    for _, s := range file.Scripts {
        sr.withScopedTable(func() { sr.visitScript(s) })
    }
}

func (sr *ScriptRegistration) visitScript(script *ast.Script) {
    // … parity port of TS visitScript …
    for _, p := range script.Parameters {
        sr.visitParameter(p)
    }
    // … rest …
}
```

No `accept(visitor)` dispatch needed — only three node types are visited, all reachable from `ScriptFile.Scripts → Script.Parameters`. The `Node.accept(this)` calls in TS that visit BlockStatement/SwitchStatement etc. are no-ops in TS for ScriptRegistration (it doesn't override them); goscape omits them entirely. Single deviation `NAI-205-D-NO-VISIT-BLOCK` (we don't walk Script.Statements — ScriptRegistration TS does the walk but every override falls through to base no-op).

### 5.4. Symbol-table normalisation

TS `SymbolTable.normalizeName` lowercases + collapses-spaces only when `type.kind === 'Basic'`. Goscape mirrors verbatim: `SymbolTable.normalize(t SymbolType, name string) string` returns `strings.ToLower(replaceAllWhitespaceWithUnderscore(name))` when `t.Kind == SymbolKindBasic`, else `name` unchanged.

### 5.5. StrictFeatureLevel field shape

TS uses partial record `{ procs?: boolean; enums?: boolean; … }`. The semantics: missing key = "feature enabled" (since checks are `features.procs === false`). Three options for goscape:

- Plain bool struct with documented zero-default = enabled. Simple, idiomatic Go. Loses ability to distinguish "explicitly off" from "unset" — but TS already treats missing as enabled, so this is parity.
- `*bool` per field. True parity (nil = unset, false = explicit off, true = explicit on). Idiomatic-Go-awkward but exact.
- Bitmask enum. Too clever.

**Spec chooses plain bool struct + inverted polarity**: `type StrictFeatureLevel struct { DisableProcs bool; DisableEnums bool; … }`. Zero value = nothing disabled = TS `{}` default. Reading `features.Procs === false` → `features.DisableProcs`. Test fixtures construct `StrictFeatureLevel{DisableProcs: true}` to assert ScriptRegistration emits `FEATURE_DISABLED_TRIGGER` for proc-triggered scripts. Deviation `NAI-205-D-STRICT-INVERTED-POLARITY`.

## 6. Package-by-package design notes

### 6.1. diagnostics

- `DiagnosticMessage` is the largest port — a single Go file with ~50 `MessageXxx` exported string constants matching the TS object literal verbatim. Each format string preserved char-for-char (the `%s` placeholders, the punctuation, the trailing periods).
- `Diagnostic.Message` is the **template** (the constant). `Diagnostic.MessageArgs` is the slice of formatting args. The actual formatted-string is produced by `Diagnostic.Format() string` at print time (TS `util.format` semantics ≈ Go `fmt.Sprintf`). Tests assert on `Message + MessageArgs`, not the formatted output, to keep them deterministic across `fmt` version drift.
- `Handler` interface has FOUR methods: `HandleParse(*Diagnostics)`, `HandleTypeChecking(*Diagnostics)`, `HandleCodeGeneration(*Diagnostics)`, `HandlePointerChecking(*Diagnostics)`. Concrete `NopHandler` implements all four as no-ops. TS's optional-method pattern (each is `?: (d) => void`) is collapsed; consumers always call the four methods, NopHandler eats them. Deviation `NAI-205-D-HANDLER-REQUIRED-METHODS`.
- `Diagnostics` is a pointer-receiver-only mutable container (`*Diagnostics`). `Diagnostics.Diagnostics() []Diagnostic` returns the internal slice unchanged (no defensive copy; documented as read-only). Tests assert on `len()` and value equality of indexed elements.

### 6.2. type

- **Singleton convention.** TS `static readonly INT = new PrimitiveType(...)`. Goscape: package-level `var PrimitiveInt *PrimitiveType = newPrimitiveInt()` initialised in `init()`. Constructors private (`newPrimitiveType`). Lower-case constructor + upper-case singleton.
- **Comparing types for equality.** TS uses reference-equality (`type === other`). Goscape uses **pointer-equality on the singletons** (`PrimitiveInt == found`) since constructors return shared pointers. For wrapped/derived types (`ArrayType{inner: PrimitiveInt}`), pointer-equality fails — TS handles this via the WeakMap interning in SymbolType + cache lookups in TypeManager. Goscape doesn't intern; semantics-level equality goes through `Type.Representation()` comparison. Deviation `NAI-205-D-TYPE-NO-INTERN` (the spec marks the few call-sites that compare types: `MetaUnit`, `MetaNothing` checks in `TupleType.fromList` / `TupleType.toList` and ScriptRegistration's `scriptReturns !== MetaType.Nothing`. These rely on singleton pointer-equality only — never on wrapped-type equality — so goscape's singleton-pointer comparison suffices in practice).
- **TupleType comparability**. TupleType has `Children []Type`. Goscape may not store TupleType inside `SymbolType.BasicType` because `SymbolType` must be comparable (it's a map key, see §6.3). The `SymbolType.key() string` method computes from `Representation()` so any-shape Type may flow in safely.
- **TypeManager error returns.** TS throws on collisions, missing types, missing args. Goscape returns wrapped errors via `fmt.Errorf("type %q already registered", name)`. `Find` returns `(Type, error)`; `FindOrNil` returns `Type` (nil on miss).

### 6.3. symbol

- See §5.2 for the cyclic-import treatment.
- `SymbolType` is a comparable struct (`Kind`, `Trigger *trigger.TriggerType` is comparable as a pointer, `BasicType type.Type` is an interface — comparable if the dynamic value is comparable; PrimitiveType+MetaType+ArrayType structs are all comparable since none have slice fields; TupleType has `Children []Type` and is **not** comparable. But TupleType never flows into `SymbolType.BasicType` per §6.2). Map keying uses the derived string key for portability; see deviation `NAI-205-D-SYMBOLTYPE-STRING-KEY`.
- `SymbolTable.Insert` walks parent chain checking for existing entries (matches TS lines 28-37 `current = current.parent` loop). Insertion always happens on `self`, not on any parent, even if the parent doesn't have the entry (matches TS).
- `SymbolTable.FindAll(name string) []Symbol` and the iterator-variant `FindAllIter` (returns `iter.Seq[Symbol]` in Go 1.23+). The TS `findAllIter<T>(name, type?)` accepts an optional kind filter — goscape ships only the unfiltered version since ScriptRegistration doesn't use the filter (TypeChecking might, deferred to NAI-206).

### 6.4. trigger

- `TriggerType` as a struct, not an interface (TS uses interface to allow CommandTrigger to be a const literal). Goscape: struct + package-level singletons / factories. `CommandTrigger` is `var CommandTrigger = &TriggerType{ID: -1, Identifier: "command", SubjectMode: ModeName, AllowParameters: true, AllowReturns: true}`.
- `SubjectMode` is an interface with one unexported method `subjectMode()` (sealed-type idiom — only types in `trigger` package may satisfy). Three concrete value-typed impls: `modeNoneT struct{}`, `modeNameT struct{}`, and `TypeMode struct{Type type.Type; Category, Global bool}`. Singletons `ModeNone SubjectMode = modeNoneT{}` and `ModeName SubjectMode = modeNameT{}` are package-level vars. `NewModeType(t type.Type, category, global bool) TypeMode` constructs a value-typed TypeMode (TS returns a fresh class instance each call; goscape returns a fresh struct value — no interning). `IsTypeMode(m SubjectMode) (TypeMode, bool)` does the type-assertion.
- `TriggerManager` parallels `TypeManager`: register/find/findOrNil; errors instead of throws.
- The `Pointers any` field on TriggerType. Until NAI-207 codegen ports `PointerType`, this field is `any` with package-level doc comment carrying `NAI-205-D-TRIGGER-POINTERS-DEFERRED`. ScriptRegistration never reads it.

### 6.5. semantics (ScriptRegistration)

Port `ScriptRegistration.ts` line-by-line:

| TS line range | TS member | Goscape equivalent |
|---|---|---|
| 36-50 | constructor + `tables: SymbolTable[]` + `categoryType` cache | `NewScriptRegistration(...)` + `tables []*symbol.SymbolTable` field + `categoryType type.Type` field |
| 52-62 | `isDisabledTypeName` | private method `isDisabledTypeName` |
| 64-68 | `isDisabledTrigger` | private method `isDisabledTrigger` |
| 70-72 | `isTypeMode` | inlined call to `trigger.IsTypeMode` |
| 78-86 | `createScopedTable` | private method `withScopedTable(block func())` |
| 88-94 | `visitScriptFile` | `Visit(file *ast.ScriptFile)` (public) |
| 96-180 | `visitScript` | private `visitScript(s *ast.Script)` |
| 184-208 | `checkScriptSubject` | private `checkScriptSubject` |
| 213-235 | `checkGlobalScriptSubject` | private |
| 240-269 | `checkCategoryScriptSubject` | private |
| 274-291 | `checkTypeScriptSubject` | private |
| 293-319 | `tryParseMapZone` | private; returns `(int32, bool)` — TS uses sentinel `-1`; Go uses second-return-ok for clarity |
| 321-352 | `tryParseZone` | private; same calling convention |
| 357-380 | `resolveSubjectSymbol` | private |
| 385-396 | `checkScriptParameters` | private |
| 400-410 | `checkScriptReturns` | private |
| 412-451 | `visitParameter` | private |
| 453-460 | `visit(nodes)` | not needed — `visitScript` does explicit `for _, p := range script.Parameters` loop |

**`tryParseMapZone` / `tryParseZone` return convention.** Straight TS port: both return `int32` (TS `number`); `-1` is a sentinel returned on parse failure (`parts.length` wrong, `levelInt != 0`, `lxInt % 8 != 0`, etc.). Caller in `resolveSubjectSymbol` always constructs a `BasicSymbol(strconv.Itoa(int(packed)), type, false)` and writes it to `script.SubjectReference` regardless — TS lines 357-368 do exactly this, including the `'-1'`-named BasicSymbol on failure. No deviation: behaviour-equivalent port.

**`reportError` calls.** TS `script.name.reportError(diagnostics, ...)` → goscape `diagnostics.ReportErrorAt(d, script.Name, MessageScriptSubjectNoSpaces, trigger.Identifier)`. Helper lives in `pkg/pack/compiler/diagnostics/report_helpers.go`. Both `Token.reportError` and `Identifier.reportError` map to the same helper since both implement `ast.Node`.

**`MessageArgs` ordering.** Format strings preserved verbatim from `DiagnosticMessage.ts`; positional `%s` arguments match TS order. Tests assert on both the template constant and the args slice.

### 6.6. ast field additions

In `pkg/pack/compiler/ast/scriptfile.go`:

```go
type Script struct {
    // ... existing fields ...
    Statements []Statement

    // NAI-205-populated fields. NAI-204-D-AST-NO-TYPE-FIELDS originally
    // listed these as deferred; NAI-205 lifts the subset that
    // ScriptRegistration writes. Remaining type-checker-owned fields
    // are still deferred (see narrowed deviation tag below).
    TriggerType      TriggerRef     // nil when trigger lookup failed
    Symbol           SymbolRef      // *symbol.ServerScriptSymbol; nil if redeclaration
    Block            SymbolTableRef // *symbol.SymbolTable; nil before ScriptRegistration
    ParameterType    TypeRef        // type.Type; computed even when params have errors
    ReturnType       TypeRef        // type.Type
    SubjectReference SymbolRef      // *symbol.BasicSymbol; nil for subjects that didn't resolve
}

type Parameter struct {
    // ... existing fields ...
    Name *Identifier

    Symbol SymbolRef // *symbol.LocalVariableSymbol; nil before ScriptRegistration
}
```

Doc-comment narrowing on `NAI-204-D-AST-NO-TYPE-FIELDS`:

```diff
- // NAI-204-D-AST-NO-TYPE-FIELDS: TS Script.symbol, .block, .returnType,
- // .triggerType, .subjectReference, .parameterType fields are NAI-205-owned
- // and not present here.
+ // NAI-204-D-AST-NO-TYPE-FIELDS: TS Script.symbol, .block, .returnType,
+ // .triggerType, .subjectReference, .parameterType landed in NAI-205.
+ // The remaining TypeChecking-owned fields (.defaultCase/.type on
+ // SwitchStatement, .symbol on Declaration*/CallExpression, .reference
+ // on Identifier/Literal/VariableExpression, .subExpression on
+ // ConstantVariableExpression/StringLiteral) are NAI-206-owned.
```

Pin test `TestPin_NAI204D_AstNoTypeFields` continues to find the tag string. Add a NEW pin in `nai205_deviation_pins_test.go` checking that the narrowed tag mentions NAI-206 (so a future careless full-removal regresses noticeably).

## 7. Deviation tag inventory (NAI-205-D-*)

Each tag below carries a one-line rationale; the full doc-comment in code follows the standard "TS does X; goscape does Y because Z" form.

| Tag | Where | Why |
|---|---|---|
| `NAI-205-D-NO-NODE-REPORT-ERROR` | `diagnostics/report_helpers.go` | TS adds a `reportError` method to every `Node`. Goscape avoids `ast → diagnostics` import by routing through `diagnostics.ReportErrorAt(d, node, ...)`. |
| `NAI-205-D-TYPEOPTIONS-FLAT` | `type/options.go` | TS exports `interface TypeOptions` + mutable subclass. Goscape uses one mutable struct + builder-fn convention. Semantically identical. |
| `NAI-205-D-METATYPE-FLAT` | `type/meta.go` | TS nests `MetaType.Type` and `MetaType.Script` as static class properties extending MetaType. Goscape uses two factory functions returning concrete types implementing the `Type` interface. |
| `NAI-205-D-TYPE-NO-INTERN` | `type/manager.go` | TS interns types via instance-identity caches. Goscape ships singletons for primitives/meta; wrapped types compare via `Representation()` at the call sites that need it. |
| `NAI-205-D-SCRIPTSYMBOL-NO-POINTERS` | `symbol/script.go` | TS `ScriptSymbol.pointers(checker)` pulls in codegen package (`PointerChecker`). Deferred to NAI-207 codegen. |
| `NAI-205-D-SYMBOLTYPE-STRING-KEY` | `symbol/symboltype.go` | TS WeakMap+Map interning for SymbolType. Goscape uses a derived `key() string` (`<kind>:<trigger-ident or representation>`) — semantically equivalent for the types ScriptRegistration uses. |
| `NAI-205-D-TRIGGER-POINTERS-DEFERRED` | `trigger/triggertype.go` | `TriggerType.Pointers` typed `any` until NAI-207's `PointerType` lands. |
| `NAI-205-D-STRICT-INVERTED-POLARITY` | `semantics/strict_feature.go` | TS `{ procs?: boolean }` (missing = enabled). Goscape `StrictFeatureLevel{DisableProcs bool}` (zero value = enabled). |
| `NAI-205-D-AST-REF-INTERFACES` | `ast/symbol_refs.go` | TS allows AST nodes to directly reference Symbol/Trigger/Type instances. Goscape avoids the import cycle via marker interfaces in `ast` with exported satisfaction methods. |
| `NAI-205-D-HANDLER-REQUIRED-METHODS` | `diagnostics/handler.go` | TS `DiagnosticsHandler` has four optional methods. Goscape interface requires all four; `NopHandler` satisfies. |
| `NAI-205-D-NO-VISIT-BLOCK` | `semantics/script_registration.go` | TS ScriptRegistration calls `accept(this)` on Script.Statements (block walk that no-ops via base class). Goscape skips the walk; matches observable behaviour. |

Pin tests for every tag live in `pkg/pack/compiler/semantics/nai205_deviation_pins_test.go` and `pkg/pack/compiler/ast/...` (for the cross-package tags).

## 8. Test strategy

Following [[runescript_cadence]] and the NAI-204 plan-test-coverage convention.

### 8.1. diagnostics

- `diagnostic_test.go` — `Diagnostic.IsError()` returns true for Error+SyntaxError, false for Info/Hint/Warning.
- `diagnostics_test.go` — `Report` + `Diagnostics()` round-trip; `HasErrors()` true iff any reported diagnostic is Error or SyntaxError.
- `report_helpers_test.go` — `ReportErrorAt(d, node, msg, args...)` produces a Diagnostic with Type=Error and SourceLocation matching `node.Source()`.

### 8.2. type

- `primitive_test.go` — singletons present, `PrimitiveByRepresentation("int") == PrimitiveInt`, all-list contains the seven expected types.
- `tuple_test.go` — `NewTupleType` rejects `<2` children; `fromList` returns Unit/single/tuple per arity; `toList` inverts.
- `meta_test.go` — singletons distinct from primitives; `NewMetaTypeWrapping(MetaAny).Representation() == "type"`; wrapping non-Any uses bracket form.
- `array_test.go` — wrapping ArrayType errors; `inner` accessor; `Representation()` adds `array` suffix.
- `gamevar_test.go` — each of the four shapes produces correct `varp<…>` / `varbit<…>` / `varn<…>` / `vars<…>` representation.
- `manager_test.go` — register-twice errors; `Find` returns error on miss, `FindOrNil` returns nil; `allowArray=true` strips `array` suffix and wraps in ArrayType; `RegisterNew` round-trips; `ChangeOptions` mutates in-place; `Check` runs registered checkers (positive + negative).

### 8.3. symbol

- `symbol_test.go` — each concrete `Symbol.SymbolName()` returns the constructor's name argument.
- `symboltype_test.go` — `SymbolType` equality via `key()`: two `SymbolTypeServerScript(t)` calls with the same trigger pointer produce equal keys; same `SymbolTypeBasic(PrimitiveInt)` calls produce equal keys; SymbolType across kinds compare unequal.
- `table_test.go` — `Insert` returns true the first time, false on conflict; `Find` returns nil on miss; child-table `Find` walks up to parent; parent `Find` does NOT walk down to child; `CreateSubTable.Insert` blocks parent-already-has duplicates (parity with TS L29-36).
- normalisation: `Insert(SymbolTypeBasic(PrimitiveObj), &BasicSymbol{Name: "Wooden Bowl"})` then `Find(SymbolTypeBasic(PrimitiveObj), "wooden bowl")` resolves (lowercase + space-to-underscore). Same lookup against `SymbolTypeServerScript(...)` does NOT normalise (case-sensitive).

### 8.4. trigger

- `subjectmode_test.go` — `ModeNone` and `ModeName` are distinct non-nil singletons; `NewModeType(PrimitiveInt, true, true)` returns a `TypeMode` value with Type=PrimitiveInt, Category=true, Global=true; `IsTypeMode` returns `(tm, true)` for type-modes and `(TypeMode{}, false)` for None/Name.
- `triggertype_test.go` — `CommandTrigger` has Identifier=`command`, SubjectMode=`ModeName`, AllowParameters=true, AllowReturns=true.
- `manager_test.go` — `Register`/`RegisterTrigger`/`RegisterAll`; double-register errors; `Find` returns error on miss; `FindOrNil` returns nil on miss.

### 8.5. semantics (ScriptRegistration)

Each test constructs a minimal `TypeManager` + `TriggerManager` + root `SymbolTable` + `Diagnostics`, runs `Visit` on a hand-built `*ast.ScriptFile`, asserts on diagnostics + AST field state + symbol-table membership.

- `script_registration_test.go` — happy paths:
  - Single `[proc,foo]` with no params, no returns → script.Symbol populated, root table contains the ServerScriptSymbol under `SymbolTypeServerScript(procTrigger)`, no diagnostics.
  - Script with parameters → parameter-symbols inserted into Script.Block; Script.ParameterType is a TupleType matching the param types; each Parameter.Symbol set.
  - Script with return tokens → Script.ReturnType is the parsed TupleType.
- `subject_test.go` — subject validation:
  - Global subject `_` accepted when trigger.SubjectMode is None/Type+global=true.
  - Global subject rejected (`SCRIPT_SUBJECT_NO_GLOBAL`) when SubjectMode.Type has global=false.
  - Category subject `_foo` resolves via root-table lookup of `SymbolTypeBasic(categoryType)`.
  - Type subject `obj_bowl` resolves via root-table lookup of `SymbolTypeBasic(mode.Type)`.
  - Mapzone subject `0_50_50` parses to packed int; subjectReference is a BasicSymbol with name=str(packed).
  - Subject with spaces rejected (`SCRIPT_SUBJECT_NO_SPACES`) when SubjectMode is not type-mode.
- `trigger_check_test.go` — trigger validation:
  - Unknown trigger emits `SCRIPT_TRIGGER_INVALID` and leaves Script.TriggerType=nil.
  - `*` after name on non-command trigger emits `SCRIPT_COMMAND_ONLY`.
  - Parameters on trigger with `AllowParameters=false` emit `SCRIPT_TRIGGER_NO_PARAMETERS`.
  - Returns on trigger with `AllowReturns=false` emit `SCRIPT_TRIGGER_NO_RETURNS`.
  - Parameters mismatching trigger.Parameters emit `SCRIPT_TRIGGER_EXPECTED_PARAMETERS`.
  - Returns mismatching trigger.Returns emit `SCRIPT_TRIGGER_EXPECTED_RETURNS`.
- `parameter_test.go` — parameter validation:
  - Invalid type-name emits `GENERIC_INVALID_TYPE`.
  - Disabled type (via StrictFeatureLevel) emits `FEATURE_DISABLED_TYPE`.
  - Duplicate parameter name emits `SCRIPT_LOCAL_REDECLARATION`.
  - Procs-disabled + non-command trigger emits `FEATURE_DISABLED_LOCAL`.
  - Parameter with type having `Options.AllowParameter=false` on non-command trigger emits `LOCAL_PARAMETER_INVALID_TYPE`.
- `smoke_test.go` — parser + ScriptRegistration end-to-end:
  - Parse `[proc,foo] return\n[label,bar]` → run ScriptRegistration with minimal trigger/type registries seeded with `proc` + `label`. Assert: zero diagnostics, root table has both `ServerScriptSymbol`s, each Script's `Symbol`/`Block`/`TriggerType`/`ReturnType`/`ParameterType` populated.
- `nai205_deviation_pins_test.go` — one Pin_* per deviation tag in §7 (11 pins).

### 8.6. ast

- `scriptfile_test.go` — add tests for the new fields (zero-value defaults; struct-literal construction with the new fields).
- `narrowed_deviation_pin_test.go` (new) — pins that the narrowed `NAI-204-D-AST-NO-TYPE-FIELDS` text mentions `NAI-206`. Doubles as a smoke for future careless removal.

## 9. Acceptance criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...` is clean.
3. All 11 deviation pins in §7 pass.
4. Smoke test in §8.5 passes: parser → ScriptRegistration → expected symbol-table state.
5. No new circular import warnings.
6. Existing NAI-204 pin tests (`TestPin_NAI204D_*`) all still pass — the narrowed tag still includes the string.

## 10. Risks and open questions

- **Cyclic import handled cleanly.** §5.2's marker-interface approach is unusual but uses Go's structural-typing escape valve. Worth verifying at plan-write time that `func (*symbol.ServerScriptSymbol) AsSymbolRef() {}` satisfies `ast.SymbolRef` from the other package — this is standard Go but the spec asserts it without a smoke compile yet. Plan T1 includes a 5-line interface-satisfaction compile check before any other code lands.
- **StrictFeatureLevel polarity inversion** ([[NAI-205-D-STRICT-INVERTED-POLARITY]]). If a later slice introduces a `features.<X> === true` check (as opposed to `=== false`), the polarity inversion silently breaks. Plan adds a comment to the struct: "If you add fields, name them `DisableX`, never `EnableX`."
- **Trigger registry seeding.** Tests build per-test registries. If the per-test setup gets unwieldy, consider extracting a `triggertest.NewFixtureManager()` helper. Defer the decision until T5/T6 reviews.
- **MetaType.Nothing vs MetaType.Unit pointer-equality.** ScriptRegistration line 162 does `scriptReturns !== MetaType.Nothing` and `triggerReturns !== scriptReturns`. Goscape's singleton pointers preserve identity comparison; verify in T8 review that `MetaNothing == MetaNothing` (both reads of the same package var) holds in Go (it does — package-level `var` initialised once).
- **Carry-forward from memory at plan-write:**
  - [[plan_arithmetic_off_by_one_carry_forward]] — `tryParseMapZone`/`tryParseZone` bit-arithmetic at TS L296-319: reuse exact TS expressions; do not re-derive in plan code blocks.
  - [[plan_dispatch_order_self_inconsistency]] — `checkScriptSubject` dispatches in TS order: empty-mode → name-mode → `_`-global → `_X`-category → other-type. Audit Case-overlap at plan-write.
  - [[plan_code_block_t_number_drift]] — dispatch task numbering re-checked in controller pre-flight before each implementer dispatch.
  - [[plan_helper_coverage]] — every test fixture in §8.5 must enumerate the minimum trigger/type registry it needs; grep all callers if a shared helper emerges mid-impl.
  - [[stale_ide_diagnostic_during_tdd_red_phase]] — verify red phases with fresh `go test`, not LSP snapshots.
  - [[true_to_ts_gate]] — every behavioural divergence from TS lands a `NAI-205-D-*` tag with rationale.
  - [[plan_constants_under_different_naming]] — grep case-insensitively for `PrimitiveInt`, `MetaAny`, etc. at plan-write to ensure no pre-existing constants collide.
  - [[interface_at_cyclic_import_boundary]] — the marker-interface idiom in §5.2 is the same pattern.
- **Plan size projection.** Conservative estimate: 12-14 tasks (T0 pre-flight + T1-T11 implementation + T12 end-to-end smoke + T13 final review + T14 close commit). Spec leaves plan-author to confirm during plan-write.

## 11. Outputs

| Package | Production files | Test files |
|---|---|---|
| `pkg/pack/compiler/diagnostics/` | 6 | 3 |
| `pkg/pack/compiler/type/` | 10 | 6 |
| `pkg/pack/compiler/symbol/` | 4 | 3 |
| `pkg/pack/compiler/trigger/` | 4 | 3 |
| `pkg/pack/compiler/semantics/` | 2 (`strict_feature.go`, `script_registration.go`) | 6 (5 behavioural + `nai205_deviation_pins_test.go`) |
| `pkg/pack/compiler/ast/` (modified) | `scriptfile.go` (new fields + narrowed deviation comment) + new `symbol_refs.go` (marker interfaces) | new `narrowed_deviation_pin_test.go` |

Final commits expected to follow the established NAI-* close-commit pattern: a per-task `feat`/`test`/`chore` series, a final `chore(close)` summary, and the `Closes memory:` trailer per [[close_commit_memory_trailer]].
