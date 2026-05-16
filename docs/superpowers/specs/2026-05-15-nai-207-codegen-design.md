# NAI-207 — RuneScript Codegen

**Created:** 2026-05-15
**TS pin:** `LostCityRS/RuneScriptTS` @ `b8c338801fbb72d294ff9576a58925a8d3f6de47`
**TS source-of-truth:** `src/compiler/codegen/CodeGenerator.ts` (902 LOC) +
`src/compiler/codegen/{Opcode,Instruction}.ts` (358+22 LOC) +
`src/compiler/codegen/script/{Block,Label,LabelGenerator,RuneScript,SwitchTable}.ts` (177 LOC) +
`src/compiler/configuration/command/CodeGeneratorContext.ts` (120 LOC) +
`src/runescript/command/*.ts` + `src/runescript/command/debug/*.ts` (12 handlers, 595 LOC).
Combined: ~2174 TS LOC.
**Predecessor:** NAI-206 (close commit `47962bf`, on `main`)
**Successor:** NAI-208 (pointer/flow analysis + opcode-id mapping + binary writer)
**Tech Stack:** Go 1.26+

## 1. Purpose & Scope

Port the **codegen pass** — the third semantic pass and slice 5 of 6 of the
compiler port (lexer → parser → ScriptRegistration → **TypeChecking →
Codegen** → link/finalise). After this slice, every well-typed `ScriptFile`
lowers to a list of `RuneScript` records, each holding a typed-but-abstract
instruction stream organised into labelled `Block`s. NAI-208 then performs
pointer/flow analysis on these instructions, maps the abstract opcodes to
their numeric ids via `ServerScriptOpcode`/`SymbolMapper`, and writes binary
script files via the BinaryWriter.

### 1.1 In-scope

- `pkg/pack/compiler/codegen` (new package): `Opcode`, `Instruction`, `Label`,
  `LabelGenerator`, `Block`, `SwitchCase`, `SwitchTable`, `RuneScript`,
  `LocalTable`, `CodeGenerator`, `CodeGeneratorContext`.
- `pkg/pack/compiler/command` (new package): 12 concrete
  `DynamicCommandHandler` implementations + `RegisterAllDynCommands`.
- `pkg/pack/compiler/semantics/dynamic_command.go` surface gap closure:
  add `GenerateCode` to the interface (currently TypeCheck-only),
  add `TypeCheckingContext.CheckArgumentTypes` and `Arguments2` accessor.
- `pkg/pack/compiler/diagnostics/messages.go`: four new templates
  (`TYPE_HAS_NO_BASETYPE`, `INVALID_CONDITION`, `NULL_CONSTANT`,
  `EXPRESSION_NO_SUBEXPR`).

### 1.2 Out-of-scope

- `src/compiler/codegen/script/config/PointerChecker.ts` (756 LOC graph-based
  pointer/flow analysis) → NAI-208.
- `src/compiler/writer/BaseScriptWriter.ts` (313 LOC) +
  `src/runescript/SymbolMapper.ts` + `src/runescript/ServerScriptOpcode.ts` +
  `src/runescript/writer/Binary*.ts` (typed-Opcode → numeric bytecode +
  symbol-id resolution) → NAI-208.
- `src/compiler/preprocess/MacroProcessor.ts` (separate slice).
- End-to-end `ServerScriptCompiler` driver wiring → NAI-208. NAI-202's
  driver scaffold remains. The NAI-207 smoke test calls each pass directly.

## 2. Inputs from NAI-206

The following are in place at `main` (`47962bf`):

| Package | Surface |
|---|---|
| `pkg/pack/compiler/ast` | `ExpressionBase` mixin on 19 concrete Expression types carrying `Type`/`TypeHint`; `Symbol` fields on call/decl AST shapes; `Reference` fields on `Identifier`/`Literal`/`Variable` shapes; `SubExpression` on `StringLiteral`/`ConstantVariableExpression`; `DefaultCase`/`Type` on `SwitchStatement`. |
| `pkg/pack/compiler/type` | `MetaType.Hook` + `NewMetaHook` + `IsMetaHook` (per `[[nai206_metatype_hook_gap]]` resolution). All NAI-205 type surface. |
| `pkg/pack/compiler/symbol` | NAI-205 symbol table, `ServerScriptSymbol`. |
| `pkg/pack/compiler/trigger` | `CommandTrigger` singleton (codegen's `script.triggerType == CommandTrigger` test). |
| `pkg/pack/compiler/semantics` | `TypeChecker` populates Type/Reference/Symbol; `DynamicCommandHandler` interface with TypeCheck only; `TypeCheckingContext` with `Arguments`/`CheckArgument`/`VisitNode`/`VisitExpression`/`VisitNodeList`/`IsConstant`. |
| `pkg/pack/compiler/diagnostics` | ~76 templates; `ReportAt`/`ReportErrorAt`. |

NAI-207 reads these — never re-resolves a symbol, never re-types an expression.

## 3. Architecture

### 3.1 Package layout

```
pkg/pack/compiler/
├── codegen/                          ← NEW
│   ├── opcode.go                     ← Opcode struct + 54+ singletons (TS Opcode.ts)
│   ├── instruction.go                ← Instruction struct (TS Instruction.ts)
│   ├── label.go                      ← Label + LabelGenerator (TS Label.ts + LabelGenerator.ts)
│   ├── block.go                      ← Block (TS Block.ts)
│   ├── switch_table.go               ← SwitchCase + SwitchTable (TS SwitchTable.ts)
│   ├── runescript.go                 ← RuneScript + LocalTable (TS RuneScript.ts)
│   ├── code_generator_context.go     ← CodeGeneratorContext (TS CodeGeneratorContext.ts)
│   ├── codegen.go                    ← CodeGenerator struct + ctor + Visit dispatch + helpers
│   ├── codegen_stmt.go               ← visitReturn/If/While/Switch/Decl/ArrayDecl/Assign/Expr/Empty
│   ├── codegen_expr.go               ← visitLocal/Game/Constant/Paren/Arith/Calc/Identifier/literals
│   ├── codegen_call.go               ← visitCommandCall/ProcCall/JumpCall/ClientScript + emitDynamicCommand
│   ├── codegen_cond.go               ← generateCondition + branch mappings
│   ├── codegen_test.go               ← smoke + dispatch
│   ├── codegen_*_test.go             ← per-arm
│   └── nai207_deviation_pins_test.go ← one assertion per NAI-207-D-* tag
├── command/                          ← NEW
│   ├── enum_handler.go               ← EnumCommandHandler
│   ├── param_handler.go              ← ParamCommandHandler (lc_param/loc_param/nc_param/npc_param/oc_param/obj_param/struct_param)
│   ├── queue_handler.go              ← QueueCommandHandler + QueueVarArgCommandHandler
│   ├── longqueue_handler.go          ← LongQueueCommandHandler + LongQueueVarArgCommandHandler
│   ├── timer_handler.go              ← TimerCommandHandler
│   ├── db_find_handler.go            ← DbFindCommandHandler
│   ├── db_getfield_handler.go        ← DbGetFieldCommandHandler
│   ├── debug_dump_handler.go         ← DumpCommandHandler (debug/)
│   ├── debug_script_handler.go       ← ScriptCommandHandler (debug/)
│   ├── placeholder_handler.go        ← PlaceholderCommand
│   ├── register.go                   ← RegisterAllDynCommands(tm, features, register)
│   └── *_test.go
└── semantics/
    └── dynamic_command.go            ← add GenerateCode to interface; add CheckArgumentTypes + Arguments2 to TypeCheckingContext
```

### 3.2 Two-package split rationale (NAI-207-D-PACKAGE-SPLIT)

TS keeps codegen in one tree (`src/compiler/codegen/` + handlers in
`src/runescript/command/`). Go demands a tighter import-cycle story.

- `codegen` exposes `Opcode`, `CodeGenerator`, `CodeGeneratorContext`. It
  imports `ast`, `diagnostics`, `symbol`, `trigger`, `type`. **It does not
  import `semantics`.**
- `command` exposes the 12 handler structs + `RegisterAllDynCommands`. It
  imports `codegen` (for `CodeGeneratorContext`/`Opcode`) and `semantics`
  (for `TypeCheckingContext`/`DynamicCommandHandler`). **The handler types
  live above both `codegen` and `semantics` in the import graph.**
- `semantics → codegen` would create a cycle (codegen needs
  `DynamicCommandHandler` to call `generateCode`). The fix:
  `DynamicCommandHandler.GenerateCode` takes a `*codegen.CodeGeneratorContext`
  — but the interface itself lives in `semantics`. To avoid the cycle, the
  `GenerateCode` parameter type is a marker interface declared in `semantics`
  whose concrete implementation lives in `codegen`. See §3.5.

### 3.3 Opcode model (NAI-207-D-OPCODE-UNTYPED)

```go
type OperandKind int

const (
    OperandNone OperandKind = iota
    OperandInt
    OperandString
    OperandLong
    OperandLabel        // *Label
    OperandLocalVar     // *symbol.LocalVariableSymbol
    OperandBasicVar     // *symbol.BasicSymbol
    OperandScriptSym    // *symbol.ScriptSymbol
    OperandRuneScriptSym // symbol.RuneScriptSymbol (marker interface)
    OperandSwitchTable  // *SwitchTable
    OperandBaseVarType  // type.BaseVarType (Discard operand)
)

type Opcode struct {
    Name string
    Kind OperandKind
}

var (
    PushConstantInt    = Opcode{"PushConstantInt", OperandInt}
    PushConstantString = Opcode{"PushConstantString", OperandString}
    PushConstantLong   = Opcode{"PushConstantLong", OperandLong}
    PushConstantSymbol = Opcode{"PushConstantSymbol", OperandRuneScriptSym}
    // …54 total mirroring TS Opcode.ts
)

type Instruction struct {
    Opcode  Opcode
    Operand any
    Source  *lexer.SourceLocation // may be nil
}
```

`OperandKind` is bookkeeping consumers (writer pass, tests) use to switch on
operand shape. The codegen author and reviewers verify the right concrete
operand type at each emission site. `NAI-207-D-OPCODE-UNTYPED` documents
the divergence from TS `Opcode<T>` generics; the rationale is Go-idiomatic
heterogeneous instruction streams + the inability of Go generics to compose
with a `[]Instruction[any]`-style slice without `any`-erasure anyway.

### 3.4 Walker dispatch shape (carries NAI-204-D-AST-NO-VISITOR)

Mirrors NAI-205/206 precedent: no `AstVisitor` interface, no per-node
`accept`. Single `Visit(n ast.Node)` type-switch in `codegen.go` dispatching
to per-shape methods on `*CodeGenerator`. Exposes `VisitNodeOrNull` and
`VisitNodes` as exported helpers so `CodeGeneratorContext` can drive
sub-visits during dynamic-command codegen.

```go
func (g *CodeGenerator) Visit(n ast.Node) {
    switch v := n.(type) {
    case *ast.ScriptFile:               g.visitScriptFile(v)
    case *ast.Script:                   g.visitScript(v)
    case *ast.Parameter:                g.visitParameter(v)
    case *ast.BlockStatement:           g.visitBlockStatement(v)
    case *ast.ReturnStatement:          g.visitReturnStatement(v)
    case *ast.IfStatement:              g.visitIfStatement(v)
    case *ast.WhileStatement:           g.visitWhileStatement(v)
    case *ast.SwitchStatement:          g.visitSwitchStatement(v)
    case *ast.DeclarationStatement:     g.visitDeclaration(v)
    case *ast.ArrayDeclarationStatement: g.visitArrayDeclaration(v)
    case *ast.AssignmentStatement:      g.visitAssignment(v)
    case *ast.ExpressionStatement:      g.visitExpressionStatement(v)
    case *ast.EmptyStatement:           // no-op
    case *ast.LocalVariableExpression:  g.visitLocalVar(v)
    case *ast.GameVariableExpression:   g.visitGameVar(v)
    case *ast.ConstantVariableExpression: g.visitConstantVar(v)
    case *ast.ParenthesizedExpression:  g.visitParen(v)
    case *ast.ArithmeticExpression:     g.visitArith(v)
    case *ast.CalcExpression:           g.visitCalc(v)
    case *ast.CommandCallExpression:    g.visitCommandCall(v)
    case *ast.ProcCallExpression:       g.visitProcCall(v)
    case *ast.JumpCallExpression:       g.visitJumpCall(v)
    case *ast.ClientScriptExpression:   g.visitClientScript(v)
    case *ast.IntegerLiteral:           g.visitIntegerLiteral(v)
    case *ast.CoordLiteral:             g.visitCoordLiteral(v)
    case *ast.BooleanLiteral:           g.visitBooleanLiteral(v)
    case *ast.CharacterLiteral:         g.visitCharacterLiteral(v)
    case *ast.NullLiteral:              g.visitNullLiteral(v)
    case *ast.StringLiteral:            g.visitStringLiteral(v)
    case *ast.JoinedStringExpression:   g.visitJoinedString(v)
    case *ast.BasicStringPart:          g.visitJoinedStringPart(v)
    case *ast.ExpressionStringPart:     g.visitJoinedStringPart(v)
    case *ast.Identifier:               g.visitIdentifier(v)
    case nil:                           return
    }
}
```

Per `[[plan_dispatch_order_self_inconsistency]]`: each arm is verified for
char-class / type-class overlap before commit.

### 3.5 Dynamic-command interface across the package boundary

`pkg/pack/compiler/semantics/dynamic_command.go` declares:

```go
type DynamicCommandHandler interface {
    TypeCheck(ctx *TypeCheckingContext)
    // GenerateCode performs codegen for the dynamic command call. May be a
    // no-op (returning false from a HasGenerateCode probe) — in which case
    // the CodeGenerator falls back to emit-args-then-Command. The ctx
    // parameter is a marker; only *codegen.CodeGeneratorContext satisfies it
    // in production.
    GenerateCode(ctx CodeGenContext) bool // true if handler emitted code; false ⇒ caller does default
}

// CodeGenContext is the marker interface implemented by
// *codegen.CodeGeneratorContext. Declared here to avoid a semantics→codegen
// import cycle. Concrete consumers type-assert.
type CodeGenContext interface{ isCodeGenContext() }
```

`*codegen.CodeGeneratorContext` implements
`func (*CodeGeneratorContext) isCodeGenContext() {}`. Inside each handler's
`GenerateCode`, the body type-asserts: `cgc := ctx.(*codegen.CodeGeneratorContext)`
then operates on it.

TS achieves the optional-`generateCode` via the `?` operator
(`generateCode?(context: CodeGeneratorContext)`); the goscape equivalent
is `GenerateCode` returning `bool`: `true` ⇒ "handler emitted code; do
not fall back"; `false` ⇒ "I did nothing; caller should emit args + command".

NAI-207-D-DYNCOMMAND-BOOLRESULT documents this divergence.

### 3.6 Audit-pin surface gaps (per [[audit_pin_when_already_shipped]])

Pre-flight grep before plan-author dispatch will produce a "Plan name |
Actual goscape name" table per [[plan_author_surface_cascade]]. Already-
confirmed gaps to close in T0:

| Item | TS source | Status |
|---|---|---|
| `TypeCheckingContext.CheckArgumentTypes(expected, reportError, args2)` | `TypeCheckingContext.ts` L156 | **MISSING** in goscape `dynamic_command.go`; 12 handlers call it. Adds in T0. |
| `TypeCheckingContext.Arguments2()` exported accessor | `TypeCheckingContext.ts` (getter) | Underlying `argumentsList(call, args2=true)` exists; need public accessor. |
| `pkg/pack/compiler/type/DbColumnType` | `src/runescript/type/DbColumnType.ts` | Used by `DbFindCommandHandler`. Verify; add if missing. |
| `*ast.Identifier.Text` (or `.Name`) field | `Identifier.ts` `text` getter | Used in TS codegen visitIdentifier + emitDynamicCommand. Verify; add if missing. |
| `MetaType.Hook` discriminator at NullLiteral codegen | `CodeGenerator.ts` L725 | NAI-206 added `IsMetaHook` — verify. |
| `type.BaseVarType` operand on Discard | `Opcode.ts` L216 | Verify `BaseVarType` enum present in `pkg/pack/compiler/type`. |

T0's pre-flight grep also re-verifies the count of `OperandKind`s (no
omissions); the goal is to neutralise the recurring drift pattern, not to
re-discover it mid-impl.

### 3.7 Diagnostic message adds

Four new templates in `diagnostics/messages.go`:

| Template | TS counterpart | Used by |
|---|---|---|
| `MessageTypeHasNoBaseType` | `DiagnosticMessage.TYPE_HAS_NO_BASETYPE` | `generateCondition` + `visitExpressionStatement` |
| `MessageInvalidCondition` | `DiagnosticMessage.INVALID_CONDITION` | `generateCondition` fallback |
| `MessageNullConstant` | `DiagnosticMessage.NULL_CONSTANT` | `visitSwitchStatement` case-key |
| `MessageExpressionNoSubExpr` | `DiagnosticMessage.EXPRESSION_NO_SUBEXPR` | `visitConstantVariableExpression` |

`MessageSymbolIsNull` already exists from NAI-205/206.

## 4. Data flow

```
ScriptFile (post-TypeCheck)
   │
   ▼
CodeGenerator.Visit(scriptFile)
   │
   ├─ for each Script (skip if trigger=command):
   │     RuneScript created  ──┐
   │     visit parameters       │  fills LocalTable.parameters + .all
   │     bind 'entry' block     │
   │     visit statements ──────┤  each stmt visit emits Instructions into block
   │     generateDefaultReturns │  PushConstantInt(0|-1) / PushConstantString('') / PushConstantLong(-1) , then Return
   │     reset LabelGenerator   │
   │                            ▼
   └─ append to _scripts   RuneScript{Blocks[], SwitchTables[], LocalTable}
                                │
                                ▼
                          (consumed by NAI-208: PointerChecker → ScriptWriter → BinaryWriter)
```

### 4.1 Statement → instruction mapping

| AST shape | Emit shape |
|---|---|
| `ReturnStatement` | visit exprs ; `Return` |
| `IfStatement` | new labels (if_true / if_else? / if_end) ; `generateCondition` ; bind if_true block + body + Branch(if_end) ; optional bind if_else + body + Branch(if_end) ; bind if_end |
| `WhileStatement` | new labels (while_start / while_body / while_end) ; bind start + generateCondition ; bind body + body-stmts + Branch(while_start) ; bind end |
| `SwitchStatement` | `script.generateSwitchTable()` ; visit subject ; `Switch(table)` ; per case: `resolveConstantValue` → `SwitchCase` + bind case-label-block + body + Branch(switch_end) ; bind switch_end |
| `DeclarationStatement` | visit initializer OR push type default ; `PopLocalVar(symbol)` |
| `ArrayDeclarationStatement` | visit init ; `DefineArray(symbol)` |
| `AssignmentStatement` | (array-indexed first-var: visit index) ; visit RHS exprs ; reverse-iterate vars emitting `PopLocalVar`/`PopVar`/`PopVar2` |
| `ExpressionStatement` | visit expr ; per result-type `Discard(baseType)` |
| `EmptyStatement` | NO-OP |

### 4.2 Expression → instruction mapping

| AST shape | Emit |
|---|---|
| `LocalVariableExpression` | visit index if any ; `PushLocalVar(reference)` |
| `GameVariableExpression` | `PushVar`/`PushVar2` (per `.dot`) |
| `ConstantVariableExpression` | visit subExpression |
| `ParenthesizedExpression` | visit inner |
| `ArithmeticExpression` | visit L+R ; opcode from INT_OPERATIONS or LONG_OPERATIONS keyed by op |
| `CalcExpression` | visit inner |
| `CommandCallExpression` | `emitDynamicCommand` returns true ⇒ done ; else visit args ; `Command(symbol)` |
| `ProcCallExpression` | visit args ; `Gosub(symbol)` |
| `JumpCallExpression` | visit args ; `Jump(symbol)` |
| `ClientScriptExpression` | `PushConstantSymbol(symbol)` ; visit args ; if transmit_list non-empty: visit list + append 'Y' to typecodes + `PushConstantInt(len)` ; `PushConstantString(typecodes)` |
| `IntegerLiteral` | reference? `PushConstantSymbol` : Type==STRING? `PushConstantString(value.String())` : `PushConstantInt(value)` |
| `CoordLiteral` | `PushConstantInt(value)` |
| `BooleanLiteral` | Type==STRING? `PushConstantString(strconv)` : `PushConstantInt(0|1)` |
| `CharacterLiteral` | `PushConstantInt(rune)` |
| `NullLiteral` | baseType-keyed; STRING ⇒ `'null'`, LONG ⇒ `-1`, else `-1` ; `IsMetaHook(getType()) == true` ⇒ extra `PushConstantString('')` |
| `StringLiteral` | subExpression? visit : reference? `PushConstantSymbol` : `PushConstantString(value)` |
| `JoinedStringExpression` | visit parts ; len>1 ⇒ `JoinString(len)` |
| `Identifier` | ref==nil && Type==STRING ⇒ `PushConstantString(text)` ; ServerScriptSymbol+CommandTrigger ⇒ `emitDynamicCommand` OR `Command(reference)` ; else `PushConstantSymbol(reference)` |

### 4.3 Condition lowering (`generateCondition`, recursive)

- Logical `&` / `|`: chain via new `condition_and`/`condition_or` blocks;
  recurse with re-mapped true/false labels.
- Other Binary/Condition: `BRANCH_MAPPINGS[baseType][op]` → branch-true,
  then `Branch(false)`.
- `ParenthesizedExpression`: unwrap, recurse.
- Anything else: `MessageInvalidCondition` diagnostic.

## 5. Tasks (preview — full plan via writing-plans)

Tentative landing order; plan-author sets the final order. Sized per the
NAI-205/206 precedent (~10–15 commits per slice).

1. **T0 — audit-pin TypeCheckingContext gaps.** Per §3.6. Add
   `CheckArgumentTypes`, `Arguments2`, `Identifier.Text`/.Name (if missing),
   `DbColumnType` (if missing). Each item: grep first; convert to no-op if
   already present per `[[audit_pin_when_already_shipped]]`.
2. **T1 — `codegen` package skeleton.** `Opcode`/`Instruction`/`Label`/
   `LabelGenerator`/`Block`/`SwitchTable`/`RuneScript`/`LocalTable`. Pure
   value types, no logic. Shape-pin tests.
3. **T2 — DiagnosticMessage adds.** 4 new templates.
4. **T3 — `DynamicCommandHandler.GenerateCode` interface extension +
   `CodeGenContext` marker + `CodeGeneratorContext` struct.** Retires
   `NAI-206-D-DYNCOMMAND-NO-CODEGEN`.
5. **T4 — `CodeGenerator` ctor + Visit dispatch + visitScriptFile / Script /
   Parameter.** Empty-script smoke test (just default returns).
6. **T5 — control-flow stmts** (Return / If / While / Switch +
   `generateCondition` + `resolveConstantValue` + branch mappings). Bundle
   per `[[mutually_dependent_task_bundling]]` — generateCondition is
   mutually recursive with if/while.
7. **T6 — variable stmts** (Decl / ArrayDecl / Assign / ExpressionStatement /
   EmptyStatement).
8. **T7 — variable expressions** (Local / Game / Constant) + Paren + Calc.
9. **T8 — arithmetic expressions.**
10. **T9 — call expressions** (Command / Proc / Jump) + `emitDynamicCommand`
    plumbing. Registry still empty; default-fallback path tested via a fake
    handler.
11. **T10 — literals + JoinedString + StringPart + Identifier.**
12. **T11 — ClientScriptExpression** (transmit-list + 'Y'-code shape). The
    trickiest arm historically; isolated task.
13. **T12 — bundle: dyncommand cohort A** (no `generateCode` override) —
    `command` package + 5 handlers (`Enum` / `ParamCommandHandler` /
    `QueueCommandHandler` / `LongQueueCommandHandler` / `DbGetField`).
    Bundle per `[[mutually_dependent_task_bundling]]`: each handler's
    TypeCheck + (absent) GenerateCode are inseparable.
14. **T13 — bundle: dyncommand cohort B** (with `generateCode` override) —
    7 handlers (`DbFind` / `QueueVarArg` / `LongQueueVarArg` / `Timer` /
    `Dump` / `Script` / `Placeholder`). Each carries both TypeCheck +
    GenerateCode.
15. **T14 — `RegisterAllDynCommands` central registration + TypeChecker
    AND CodeGenerator construction integration.** Both ctors take the same
    `map[string]DynamicCommandHandler`. Mirrors NAI-205's `trigger.RegisterAll`.
    Retires `NAI-206-D-DYNCOMMAND-EMPTY`. Pipeline smoke test:
    parse → register → typecheck → codegen on a script exercising the
    major constructs (return / if/while / switch / arith / cond / proc-call
    / command-call (both direct + dyncommand) / local + game vars + arrays /
    literals + joined strings + clientscript expression).
16. **T15 — close commit + memory update.** Ship `nai207_codegen_close.md`;
    retire/update `nai206_dyncommand_deferrals.md`; add deviation-pin tests
    for every NAI-207-D-* tag. `Closes memory:` trailer per
    `[[close_commit_memory_trailer]]`.

## 6. Testing strategy

Three layers, matching NAI-206:

- **Per-arm tests** (`codegen_*_test.go`): build a single AST node (via
  `parser.ParseScriptFile` for fixture realism, or hand-assembled for
  unit-isolation), run TypeChecker → CodeGenerator, assert emitted
  instruction stream by `(Opcode.Name, Operand)` tuples. Mirrors
  NAI-206 per-arm pattern.
- **Smoke test** (`smoke_test.go`): full pipeline on a small representative
  script. Asserts non-empty RuneScript with expected block count and
  instruction summary.
- **Deviation-pin tests** (`nai207_deviation_pins_test.go`): one per
  NAI-207-D-* tag.

Per `[[plan_test_fixture_t17_dependency]]`: test fixtures that pre-type
literals bypass the TypeChecker — use only on tasks that exercise codegen
arms which would otherwise hit Visit fallback for missing Type. Full-pipeline
fixtures (T14 smoke) go through both passes.

Per `[[plan_red_phase_prediction_old_sut]]`: each TDD red-phase prediction
in the plan is traced against OLD-SUT-with-NEW-inputs at plan-write, not
"by analogy" with a neighbor arm.

## 7. Risks / known landmines

- **`Identifier.text` / `Identifier.Name` field** — verify exists in
  goscape AST. T0 audit-pin closes the gap if needed.
- **`MetaType.Hook` discriminator at codegen NullLiteral** — NAI-206 added
  `IsMetaHook` per [[nai206_metatype_hook_gap]]. Verify the discriminator
  shape matches TS `instanceof` site.
- **Switch-case key resolution via `resolveConstantValue`** — depends on
  `Literal.Reference` being populated by NAI-206 TypeChecker. Verify before
  T5.
- **`Block.Instructions` heterogeneous slice** — `[]Instruction` works
  since `Operand any`. Tests assert structural equality, not pointer
  equality.
- **Label uniqueness across nested if/while/switch** — `LabelGenerator`
  resets per-script. Cross-script collision impossible; intra-script
  collision protected by name-counter map.
- **`emitDynamicCommand` reaches commands via both `CommandCallExpression`
  and bare `Identifier`** — TS L584 + L791. Easy to miss the Identifier
  path; the per-arm `visitIdentifier` test must include a command-by-name
  case.
- **Per-Script LabelGenerator reset + `lastLineNumber = -1` reset** — codegen
  state. Tests must assert clean state across multiple scripts in the same
  ScriptFile.

## 8. Reviewer pre-flight checklist

Per `[[controller_preflight]]` and `[[plan_author_surface_cascade]]`,
before every implementer dispatch the controller MUST:

1. Build a "Plan name | Actual goscape name" table for every method, type,
   and field the task touches; correct deviations.
2. Re-confirm "Authoritative task numbering" matches the spec.
3. Grep TS source for any branch the plan codifies — never "by analogy."
4. For deletion-driven failure tests: verify error-leniency asymmetry
   per `[[loader_error_leniency_asymmetry]]` (does the target return
   `nil` or propagate? both happen at NAI-205).
5. For new fixtures: mentally execute them per
   `[[plan_runnable_test_fixtures]]`.

## 9. Cadence

Per `[[runescript_cadence]]`:

1. Brainstorm (this skill) → 2. Spec (this doc) → 3. Plan
   (`superpowers:writing-plans`) → 4. Subagent-driven execution
   (`superpowers:subagent-driven-development`) on Sonnet for implementer
   + reviewer; controller stays on Opus. Two-stage review per task per
   `[[runescript_cadence]]`.

`[[execution_mode_default]]`: always dispatch via subagent-driven; no
"which mode?" menu.

`[[true_to_ts_gate]]`: every behavioural divergence in production code
gets a NAI-207-D-* tag + deviation-pin test. Tags below are preliminary;
emergent tags via `[[emergent_deviation_mid_impl]]` are permitted and
expected.

## 10. Deviations (preliminary)

| Tag | What | Why |
|---|---|---|
| `NAI-207-D-OPCODE-UNTYPED` | `Opcode` = untyped struct + `Operand any` vs TS `Opcode<T>` generics | Go generics don't compose with heterogeneous `[]Instruction[any]`; untyped + `OperandKind` matches existing goscape singleton patterns. |
| `NAI-207-D-PACKAGE-SPLIT` | Two packages (`codegen` + `command`) vs TS single tree | Cyclic-import avoidance — `command` lives above both `codegen` and `semantics`. |
| `NAI-207-D-NO-VISITOR-INTERFACE` | Carries forward NAI-204-D-AST-NO-VISITOR; no `accept`/`AstVisitor`, type-switch dispatch | Established compiler-port-wide convention. |
| `NAI-207-D-NULLLITERAL-HOOK-DISC` | `IsMetaHook(t)` discriminator vs TS `t instanceof MetaType.Hook` | NAI-205 precedent — unexported struct + `IsMetaX` discriminator. |
| `NAI-207-D-DYNCOMMAND-BOOLRESULT` | `GenerateCode` returns `bool` (true=handled, false=default-fallback) vs TS optional `generateCode?` | Go lacks optional methods; bool return + default-fallback branch in `emitDynamicCommand` preserves semantics. |
| `NAI-207-D-CODEGENCONTEXT-MARKER` | `CodeGenContext` marker interface in `semantics` + `isCodeGenContext()` impl in `codegen.CodeGeneratorContext` | Avoids semantics→codegen import cycle. Handlers type-assert at use site. |

## 11. Memory references

- `[[nai206_typechecking_close]]` — NAI-206 inputs.
- `[[nai206_dyncommand_deferrals]]` — explicit cohort + interface gaps NAI-207
  closes.
- `[[nai206_metatype_hook_gap]]` — Hook discriminator surface.
- `[[plan_author_surface_cascade]]` — pre-flight name-divergence table.
- `[[mutually_dependent_task_bundling]]` — T5, T12, T13 rationale.
- `[[audit_pin_when_already_shipped]]` — T0 conversion-to-no-op.
- `[[plan_test_fixture_t17_dependency]]` — pre-typed fixture caveat.
- `[[stale_ide_diagnostic_during_tdd_red_phase]]` — red/green verification
  protocol.
- `[[verify_implementer_claims]]` — fresh `go test ./...` after each task.
- `[[plan_dispatch_order_self_inconsistency]]` — dispatch arm audit.
- `[[true_to_ts_gate]]` — deviation-tag discipline.
- `[[controller_preflight]]` — controller pre-flight.
- `[[plan_red_phase_prediction_old_sut]]` — red-phase prediction discipline.
- `[[runescript_cadence]]` — overall cadence.
- `[[execution_mode_default]]` — subagent-driven dispatch.
- `[[close_commit_memory_trailer]]` — close commit trailer format.
- `[[emergent_deviation_mid_impl]]` — emergent-tag policy.
- `[[plan_runnable_test_fixtures]]` — fixture runnability gate.
