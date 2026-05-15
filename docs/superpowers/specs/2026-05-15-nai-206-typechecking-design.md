# NAI-206 — RuneScript Type Checker

**Created:** 2026-05-15
**TS pin:** `LostCityRS/RuneScriptTS` @ `b8c338801fbb72d294ff9576a58925a8d3f6de47`
**TS source-of-truth:** `src/compiler/semantics/TypeChecking.ts` (1546 LOC) plus
`src/compiler/configuration/command/{DynamicCommandHandler,TypeCheckingContext}.ts`
**Predecessor:** NAI-205 (close commit `6aecea9`, on `main`)
**Successor:** NAI-207 (codegen, projected)
**Tech Stack:** Go 1.26+

## 1. Purpose

Port the second semantic pass — *type checking* — to goscape. NAI-205 shipped
the first pass (`ScriptRegistration`) which walks each script header to insert
symbols into the root `SymbolTable` and resolve trigger/subject/parameter/
return shapes. This slice picks up at the body: it walks every statement and
expression, propagates `typeHint` downward, sets `type` upward, resolves
identifiers and variables against the symbol table, and reports type
mismatches via the diagnostics handler.

After this slice lands, the compiler has full semantic information attached
to each AST node and is ready for codegen (NAI-207).

## 2. Inputs from NAI-205

The following are in place at `main` (`6aecea9`):

| Package | Surface |
|---|---|
| `pkg/pack/compiler/diagnostics` | `Diagnostic`, `Diagnostics`, `Handler`, ~76 `MessageXxx` templates, `ReportAt`/`ReportErrorAt` helpers (cyclic-import bridge replacing TS `Node.reportError`). |
| `pkg/pack/compiler/type` | `BaseVarType`, `TypeOptions`, `Type` interface, 7 `PrimitiveType` singletons, 4 `MetaType` singletons + `NewMetaWrapping` + `NewMetaScript` + `IsMetaScript`, `TupleType`, `ArrayType`, 4 `GameVarType` variants, `TypeManager`. |
| `pkg/pack/compiler/symbol` | `LocalVariableSymbol`/`BasicSymbol`/`ConstantSymbol`/`ServerScriptSymbol`/`ClientScriptSymbol`, `SymbolType` tagged-union, `SymbolTable` with parent-chain + `Find`/`FindAll`. |
| `pkg/pack/compiler/trigger` | `SubjectMode`, `TriggerType`, `TriggerManager`, `CommandTrigger` singleton. |
| `pkg/pack/compiler/ast` | `symbol_refs.go` marker interfaces (`SymbolRef`/`TriggerRef`/`TypeRef`/`SymbolTableRef`) + 6 NAI-205-owned fields on `Script` (`TriggerType`, `Symbol`, `Block`, `ParameterType`, `ReturnType`, `SubjectReference`) + 1 on `Parameter` (`Symbol`). |
| `pkg/pack/compiler/semantics` | `ScriptRegistration`, `StrictFeatureLevel` (inverted polarity, `DisableX bool`). |
| `pkg/pack/compiler/parser` | `Parser` with `ParseScriptFile`, `ParseScript`, `ParseClientScript`; internal `parseExpression`. |

NAI-204-D-AST-NO-TYPE-FIELDS was narrowed to a NAI-206-only scope at NAI-205
close — the AST is the largest pending shape change in this slice.

## 3. Out-of-Scope

- Code generation (NAI-207 owns lowering, opcode emission, switch-table layout).
- The `DynamicCommandHandler` registry — NAI-206 ports the *interface* + the
  `TypeCheckingContext`, but does NOT port any concrete commands; the
  `dynamicCommands` map is wired empty. (See deviation §10.)
- ANTLR `singleExpression()`-style fallback constant parsing — goscape uses a
  hand-written recursive-descent parser without ANTLR's “discard error
  listener.” See §10 / NAI-206-D-CONST-PARSE.
- Constant-expression caching across re-runs — single-pass equivalence is
  preserved; the on-instance cache is replicated, but the inter-pass
  performance optimisation is YAGNI for now.

## 4. Architecture

### 4.1 Package layout

```
pkg/pack/compiler/
├── ast/
│   ├── scriptfile.go         ← retire NAI-204-D-AST-NO-TYPE-FIELDS doc-comment
│   ├── expressions.go        ← add Type/TypeHint on Expression base
│   ├── statements.go         ← add Symbol on Decl/ArrayDecl; DefaultCase/Type on Switch
│   ├── calls.go              ← add Symbol on CallExpression
│   ├── literals.go           ← add Reference on Literal; SubExpression on StringLiteral
│   ├── variables.go          ← add Reference on VariableExpression; SubExpression on ConstantVariableExpression
│   └── narrowed_deviation_pin_test.go  ← DELETED at close
├── diagnostics/
│   └── messages.go           ← add ~10 type-checking templates
├── parser/
│   └── parser.go             ← add ParseSingleExpression entry rule
├── semantics/
│   ├── strict_feature.go     ← add Disable{Calc, LogicalAnd, RelationalEquals, TopLevelDefOnly}
│   ├── dynamic_command.go    ← NEW: DynamicCommandHandler interface + TypeCheckingContext
│   ├── type_checking.go      ← NEW: TypeChecker struct + ctor + helpers (~400 LOC)
│   ├── type_checking_stmt.go ← NEW: statement walker arms (~300 LOC)
│   ├── type_checking_expr.go ← NEW: expression walker arms (~600 LOC)
│   └── type_checking_test.go + per-arm tests
└── type/
    └── meta.go               ← add MetaType.Hook (carry-over from NAI-205)
```

File splits (`*_stmt.go`, `*_expr.go`) keep each unit under ~600 LOC; the
dispatch `Visit(node)` lives in `type_checking.go` and routes via Go
type-switch.

### 4.2 Walker shape (dispatch)

NAI-205's `ScriptRegistration` set the precedent: no `AstVisitor` interface
(see NAI-204-D-AST-NO-VISITOR), no per-node `accept` (see
NAI-205-D-NO-VISIT-BLOCK). Each "visit" is a method on `TypeChecker`
dispatched via a single `Visit(n ast.Node)` type-switch.

```go
func (tc *TypeChecker) Visit(n ast.Node) {
    switch v := n.(type) {
    case *ast.ScriptFile:        tc.visitScriptFile(v)
    case *ast.Script:            tc.visitScript(v)
    case *ast.BlockStatement:    tc.visitBlockStatement(v)
    case *ast.ReturnStatement:   tc.visitReturn(v)
    // …
    case *ast.Identifier:        tc.visitIdentifier(v)
    case nil:                    return
    default:                     tc.visitNodeFallback(n)
    }
}
```

### 4.3 Parent / scope context

TS `Node.parent` + `findParentByType` are forbidden by NAI-204-D-AST-NO-PARENT.
Three TS call sites need a replacement:

| TS site | Need | Resolution |
|---|---|---|
| `visitReturnStatement` | Enclosing `Script` for `returnType` | `tc.currentScript *ast.Script` field, set in `visitScript` enter/exit. |
| `visitJumpCallExpression` | Enclosing `Script` for `triggerType` (proc context detection) | Same `tc.currentScript`. |
| `visitSwitchCase` | Enclosing `SwitchStatement` for `switchType` | `tc.currentSwitch *ast.SwitchStatement`. |
| `visitDeclaration*` (`topLevelDefOnly`) | Is parent `Script` (not nested block)? | `tc.atScriptTopLevel bool`, toggled by `visitBlockStatement` enter/exit. |

This is tracked as **NAI-206-D-WALKER-OWNS-CONTEXT**: TS stores
back-pointers on the AST; goscape carries scope/script context on the walker
itself. Behaviour-equivalent for the sites listed.

### 4.4 Symbol-table scoping

TS uses `scoped(newTable, block)`: save current table, switch, run, restore.
Direct port:

```go
func (tc *TypeChecker) scoped(newTable *symbol.SymbolTable, fn func()) {
    old := tc.table
    tc.table = newTable
    fn()
    tc.table = old
}
```

`visitScript` runs the script body in the script's `Block` table (set by
ScriptRegistration). `visitBlockStatement` creates a `CreateSubTable()` for
each block. `visitSwitchCase` likewise scopes case statements.

### 4.5 Re-entrant parser for constants and hooks

Two TS sites re-invoke the parser on a string:

- `parseConstantExpression(value, source)` — feeds a constant's stored value
  back through the lexer + parser to `parser.singleExpression()`, returns
  `Expression`.
- `handleClientScriptExpression(stringLiteral, hint)` — feeds the literal's
  text back through `parser.clientScript()`, returns
  `ClientScriptExpression`.

Goscape already has `NewClientScriptParser` + `ParseClientScript`. Adds
**`ParseSingleExpression() ast.Expression`** and
**`NewSingleExpressionParser(input, sourceName) *Parser`** in
`pkg/pack/compiler/parser/parser.go`, exposing the existing
`parseExpression()` body. The re-entrant parsers use the same `Diagnostics`
sink the outer walker uses (see NAI-205 `Handler` shape).

For the constant-expression cache, `TypeChecker.constantExpressionCache
map[string]ast.Expression` mirrors the TS cache, but **caches the parsed AST
tree directly** (not the ANTLR parse tree); subsequent reads clone-via-`AstBuilder`
in TS, but goscape parses to AST in one pass so the cache value IS the AST.

NAI-206-D-CONST-PARSE: TS uses ANTLR's `DISCARD_ERROR_LISTENER` to silently
swallow syntax errors during constant parsing. Goscape's hand-written parser
accumulates errors via the listener chain; for the constant-parse path we
construct a parser with `RemoveErrorListeners()` + a no-op listener (or
inspect `numErrors > 0` and discard the result, mapping to TS's
"`syntax errors > 0 ⇒ return null`" semantics).

### 4.6 Dynamic-command surface

`DynamicCommandHandler` is an interface ported verbatim minus `generateCode`
(NAI-207's). Wiring:

```go
type DynamicCommandHandler interface {
    TypeCheck(ctx *TypeCheckingContext)
    // GenerateCode is deferred to NAI-207.
}
```

`TypeCheckingContext` has the helpers TS exposes: `Arguments` getter,
`Argument(index, hint, args2)`, `IsConstant`, etc. NAI-206 ports these so a
later cohort can register real handlers; for now `dynamicCommands` is wired
to an empty `map[string]DynamicCommandHandler{}` and the `checkDynamicCommand`
fast-path returns `false` immediately.

## 5. AST field expansion (T1)

The 13 deferred fields (TS-counted as "11 remaining"; goscape splits
`VariableExpression.reference` into two concrete sites — `Local` + `Game` —
to match its existing flat AST shape):

| Node | Field | Type (goscape) | Stored via |
|---|---|---|---|
| `Expression` (base) | `Type` | `ast.TypeRef` | direct |
| `Expression` (base) | `TypeHint` | `ast.TypeRef` | direct |
| `Identifier` | `Reference` | `ast.SymbolRef` | direct |
| `Literal` (base) | `Reference` | `ast.SymbolRef` | direct |
| `LocalVariableExpression` | `Reference` | `ast.SymbolRef` | direct |
| `GameVariableExpression` | `Reference` | `ast.SymbolRef` | direct |
| `ConstantVariableExpression` | `SubExpression` | `ast.Expression` | direct |
| `StringLiteral` | `SubExpression` | `ast.Expression` | direct |
| `SwitchStatement` | `DefaultCase` | `*ast.SwitchCase` | direct |
| `SwitchStatement` | `Type` | `ast.TypeRef` | direct |
| `DeclarationStatement` | `Symbol` | `ast.SymbolRef` | direct |
| `ArrayDeclarationStatement` | `Symbol` | `ast.SymbolRef` | direct |
| `CallExpression` (all 4 variants) | `Symbol` | `ast.SymbolRef` | direct |

`Expression.Type` and `Expression.TypeHint` are added to the **base struct**
embedded in each concrete expression type (TS pattern: fields on the abstract
`Expression`). Goscape's `expressions.go` currently has no shared base — Decision: add an
**`ExpressionBase` struct** that each concrete expression embeds, carrying
`Type` and `TypeHint`. Already-existing fields stay. Each concrete `*Foo`
type implementing `Expression` exposes `Type`/`TypeHint` by struct-embedding.

NAI-206-D-EXPR-BASE: TS's `Expression` is an abstract superclass with these
two fields; goscape adds the `ExpressionBase` mixin in NAI-206 instead of
back-fitting into NAI-204 (where the parser already produced concrete
literal/variable/call types without a shared base). Net behavioural effect:
none.

After this slice, the narrowed pin test
`pkg/pack/compiler/ast/narrowed_deviation_pin_test.go` is **deleted** (its
sole job was to keep the NAI-206-ownership claim alive in the
NAI-204-D-AST-NO-TYPE-FIELDS doc-comment until this slice landed). The
doc-comment itself loses its NAI-206 forward-reference (rephrased as
historical: "All fields landed in NAI-206 — see `Expression.Type` etc.").

## 6. Decomposition

Projected ~18 tasks. Each task includes red-phase pin, green-phase impl,
review. Counts derived from TS-LOC × goscape-density factor observed in
NAI-205 (~6× expansion vs TS).

| # | Task | TS lines | Notes |
|---|---|---|---|
| T1 | AST field expansion + `ExpressionBase` mixin | n/a | Adds 13 fields, no logic; pins via reflect-based existence test. |
| T2 | `MetaType.Hook` singleton + diagnostic-message expansion | n/a | Adds Hook to `type/meta.go`; ~15–20 new messages (FEATURE_DISABLED_*, CONSTANT_*, LOCAL_*, GAME_*, etc.); count finalised at plan-write via TS grep. |
| T3 | `DynamicCommandHandler` interface + `TypeCheckingContext` (no generateCode) | ~150 TS | Lives in `semantics/dynamic_command.go`. |
| T4 | `TypeChecker` struct + ctor + `scoped` + `isDisabledTypeName` + `isDisabledCommandName` + `checkTypeMatch` + `checkTypeMatchAny` + `getSafeType` + `typeHintExpressionList` + `visitNodeOrNull` + `visitNodes` | ~120 | Establishes the walker shell. |
| T5 | `StrictFeatureLevel` field expansion + sibling tests | n/a | Add Disable{Calc, LogicalAnd, RelationalEquals, TopLevelDefOnly, Macros, PointerInversion, QueueTyped}. |
| T6 | Statements: ScriptFile/Script/Block/Return/If/While/Empty | ~80 | Wires `currentScript`. |
| T7 | Switch + SwitchCase + `isConstantExpression` + `isConstantSymbol` | ~110 | Wires `currentSwitch`. |
| T8 | Declaration + ArrayDeclaration | ~100 | Wires `topLevelDefOnly` via `atScriptTopLevel`. |
| T9 | Assignment + ExpressionStatement + `expressionHasSideEffects` + `commandHasSideEffects` | ~110 | |
| T10 | Parenthesized + Condition + `checkBinaryConditionOperation` + `isConditionExpression` + `findInvalidConditionExpression` | ~140 | Largest single block; condition operator validation. |
| T11 | Arithmetic + Calc | ~60 | |
| T12 | `checkCallExpression` + `typeCheckArguments` + Command/Proc/Jump call dispatch | ~100 | |
| T13 | `checkDynamicCommand` + ClientScript expression + `MetaType.Hook`-aware transmit-list | ~120 | Hardest expression arm; depends on T2's Hook. |
| T14 | Variable expressions: Local + Game + Constant (incl. `constantsBeingEvaluated` cycle detection + cache + `parseConstantExpression`) | ~180 | Depends on parser `ParseSingleExpression`. |
| T15 | Literals: Integer/Coord/Boolean/Character/Null + StringLiteral + `handleClientScriptExpression` + JoinedString/JoinedStringPart | ~150 | StringLiteral has the heaviest dispatch (4 branches). |
| T16 | Identifier + `resolveSymbol` + `symbolToType` + `allowStringConversion` | ~120 | The largest single method; `findAllIter` consumer. |
| T17 | Parser entry: `NewSingleExpressionParser` + `ParseSingleExpression` + filtered `SymbolTable.FindAllByKind` if needed | ~30 | Parser-side enabling work. |
| T18 | End-to-end smoke + retire NAI-204-D-AST-NO-TYPE-FIELDS pin test + close commit | n/a | Mirrors NAI-205 close pattern. |

Task dependencies (writing-plans will codify):
- T1 → T6-T16 (every walker arm needs AST fields).
- T2 → T13 (Hook), T8/T14 (FEATURE_DISABLED_*), T14 (CONSTANT_*).
- T3 → T13 (real client-script flow uses TypeCheckingContext) and T12/T16.
- T4 → T6-T16.
- T5 → T6, T8 (procs/topLevel), T10 (logicalAnd/relationalEquals), T11 (calc), T15 (booleans).
- T17 → T14 (constants), T13 (string-literal hook re-parse).

## 7. Diagnostic templates added

The TS source emits messages whose Go templates aren't present yet. Audit
across the walker:

- `FEATURE_DISABLED_BOOLEAN`, `FEATURE_DISABLED_TYPE`, `FEATURE_DISABLED_LOCAL`,
  `FEATURE_DISABLED_OPERATOR`, `FEATURE_DISABLED_COMMAND`,
  `FEATURE_DISABLED_TRIGGER`, `FEATURE_DISABLED_CALC`
- `LOCAL_PARAMETER_INVALID_TYPE` already exists (NAI-205); add
  `LOCAL_DECLARATION_INVALID_TYPE`, `LOCAL_DECLARATION_NOT_TOPLEVEL`,
  `LOCAL_ARRAY_INVALID_TYPE`, `LOCAL_REFERENCE_UNRESOLVED`,
  `LOCAL_REFERENCE_NOT_ARRAY`, `LOCAL_ARRAY_REFERENCE_NOINDEX`
- `GAME_REFERENCE_UNRESOLVED`
- `CONSTANT_UNKNOWN_TYPE`, `CONSTANT_REFERENCE_UNRESOLVED`, `CONSTANT_CYCLIC_REF`,
  `CONSTANT_PARSE_ERROR`, `CONSTANT_NONCONSTANT`
- `CUSTOM_HANDLER_NOTYPE`, `CUSTOM_HANDLER_NOSYMBOL` (re-checked; may already
  exist as `MessageCustomHandlerNoType`/`MessageCustomHandlerNoSymbol` —
  T2 grep confirms).

T2 grep-confirms presence vs. TS, ports only the missing ones (count
expected ~15–20 new templates after dedup).

## 8. Deviations expected (NAI-206-D-*)

| Tag | Subject | Rationale |
|---|---|---|
| NAI-206-D-WALKER-OWNS-CONTEXT | TC walker carries `currentScript` / `currentSwitch` / `atScriptTopLevel` instead of relying on AST parent back-pointers. | TS uses `findParentByType`; goscape bans AST parent links (NAI-204-D-AST-NO-PARENT). Behaviour-equivalent. |
| NAI-206-D-EXPR-BASE | `Expression.Type`/`TypeHint` live on a new `ExpressionBase` mixin struct, embedded in each concrete expression type. | Goscape has no shared `Expression` base (TS abstract class); embedding mixin is the most Go-idiomatic equivalent. |
| NAI-206-D-CONST-PARSE | Constant re-parse uses goscape's hand-written parser with all error-listeners cleared (`RemoveErrorListeners()`); errors detected via `numErrors > 0` ⇒ return nil. | TS uses ANTLR's static `DISCARD_ERROR_LISTENER`; goscape replicates the same "ignore syntax errors" semantics through the listener API it already has. |
| NAI-206-D-CONST-CACHE-AST | `constantExpressionCache map[string]ast.Expression` caches AST nodes, not ANTLR parse trees. | TS caches the parse tree because `AstBuilder` runs per-read; goscape parses straight to AST in one pass. Behaviour-equivalent for cache *hit* semantics. |
| NAI-206-D-DYNCOMMAND-EMPTY | `dynamicCommands` wired empty; `TypeCheckingContext` ported but no concrete handlers registered. | Cohort decision: NAI-206 is type-check infrastructure; concrete dynamic commands (~12 in TS — enum, struct_param, db_find family) land in a follow-up. Tests cover the empty-fast-path. |
| NAI-206-D-SCRIPT-TYPE-IDENT-ONLY | `MetaType.Script` representation uses the trigger identifier string (no live trigger pointer). | Already-shipped NAI-205 invariant; called out here so the TypeChecking reader doesn't expect to recover `*trigger.TriggerType` from a `metaScript`. Resolution: `symbolToType` reads `symbol.Trigger` directly via `*symbol.ServerScriptSymbol`. |

## 9. Test strategy

- **Per-arm tests** (T6–T16): each walker arm gets a focused test file
  (`type_checking_<arm>_test.go`) with positive + negative cases mirroring
  TS unit-test fixtures where they exist; ours are constructed to bind the
  diagnostic-template + AST-field-mutation contract.
- **Deviation pin tests** (`pkg/pack/compiler/semantics/nai206_deviation_pins_test.go`):
  one per NAI-206-D-* tag, asserting the doc-comment is present at the
  declared site.
- **End-to-end smoke** (T18): a small valid `.rs2` script (e.g. one
  `[proc,demo]` with a local declaration, an arithmetic expression, a
  return) is parsed → registered → type-checked end-to-end, asserting (a)
  zero diagnostics and (b) the produced AST has `Symbol`/`Type` populated
  on every relevant node. A second variant introduces a deliberate type
  mismatch and pins the expected diagnostic.
- **AST field reflect-pin** (T1): reflective check that every field listed
  in §5 exists on its owning type, so future refactors can't silently drop
  them.

`go test ./...` is the canonical verification entry point. Apply
`stale_ide_diagnostic_during_tdd_red_phase` memory: each red phase verified
via fresh `go test`, never LSP snapshots.

## 10. Carry-forward memory checks performed at spec-write

- `metascript_handoff_pattern` — `MetaType.Script` discriminator usage
  codified in §4.3 (T16 `symbolToType` reads trigger from
  `ServerScriptSymbol.Trigger`, not from the metaScript instance).
- `nai206_metatype_hook_gap` — Hook landing isolated to T2; not interleaved
  with walker tasks. (T13 depends on T2 in §6.)
- `cyclic_import_marker_export_method` — Decision: no new marker interfaces
  needed; NAI-205's four marker interfaces (`SymbolRef`/`TriggerRef`/
  `TypeRef`/`SymbolTableRef`) cover every new field in §5. `Reference` and
  `Symbol` fields use `ast.SymbolRef`; `Type`/`TypeHint` use `ast.TypeRef`;
  `SubExpression` uses `ast.Expression` directly (no cross-package shape).
- `plan_arithmetic_off_by_one_carry_forward` — All TS line numbers in this
  spec were re-derived from the actual TS source at the pinned HEAD; no
  blind copy from NAI-205 spec.
- `plan_dispatch_order_self_inconsistency` — §4.2 type-switch dispatch order
  audited against §6 task list. No two arms claim the same concrete `ast.*`
  type. AST hierarchy is non-overlapping (no `LocalVariableExpression`
  also matching `Identifier`, etc., verified via `kind.go`).
- `plan_code_block_t_number_drift` — Plan-author must pre-flight grep each
  task's premise against HEAD before dispatch.
- `true_to_ts_gate` — Every behavioural divergence flagged §8.
- `stale_ide_diagnostic_during_tdd_red_phase` — Verification protocol §9.

## 11. Open questions resolved at spec-write

- **Q: Does goscape's `SymbolTable.FindAll` cover TS `findAllIter`'s
  kind-filtered shape?** A: `FindAll(name) []Symbol` returns all kinds;
  callers filter in code. TS `findAllIter<T>(name, kind?)` is satisfied by
  Go callers iterating + type-asserting. T17 may add a
  `FindAllByKind(name, SymbolKind) []Symbol` helper if call-sites become
  noisy.
- **Q: Does the parser support `singleExpression()`?** A: Not yet — T17
  adds `ParseSingleExpression`. Spec §4.5.
- **Q: How does the constant-parse path obtain its `Diagnostics` sink?**
  A: It uses the outer walker's `Diagnostics`. The re-entrant parser is
  constructed with `RemoveErrorListeners()` (per NAI-206-D-CONST-PARSE), so
  it does NOT route its lex/parse errors anywhere; `numErrors > 0 ⇒ nil`
  is the signal.
- **Q: `MetaType.Hook` shape?** A: Carries a `TransmitListType Type` field
  (used at TS L843, L852). Constructor `NewMetaHook(transmitListType Type)
  Type`; discriminator `IsMetaHook(t Type) (transmitListType Type, ok
  bool)`. Lives in `type/meta.go` alongside `IsMetaScript`.

## 12. Risks & follow-ups

- **Risk:** AST field count balloons (Q5 lists 13). Mitigation: T1 lands
  them all under one task, pinned via a reflect-existence test; consumers
  in T6–T16 work against stable surface.
- **Risk:** Constant re-parse cache key (TS uses the value string) may not
  be sound for distinct source-locations with same text. Mitigation: TS
  has the same behaviour (cache key IS the value string); we port verbatim.
- **Follow-up:** A future cohort registers concrete `DynamicCommandHandler`
  instances (enum, struct_param, db_find, db_find_refine, etc.); NAI-206
  leaves the registry empty and the fast-path measured by a deviation pin.
- **Follow-up:** `FindAllByKind` may emerge as a useful helper in T16; not
  forced into this slice.

## 13. Authoritative task numbering

Plan-author MUST include "Authoritative task numbering: T1…T18 per spec §6;
NEVER reuse a T-number once retired in this plan" in the controller dispatch
prompt (per `plan_code_block_t_number_drift`).

## 14. Close commit

Final commit message follows NAI-205 pattern:

```
chore(close): NAI-206 — TypeChecking walker (statements/expressions/calls)

Closes memory: <list of memory entries authored/touched in this slice>
```
