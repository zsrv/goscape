# NAI-204 — RuneScript parser + AST (compiler slice 2 of 5)

## 0. Pre-context: where this slice sits in the arc

NAI-203 (closed at 2963935) shipped `pkg/pack/compiler/lexer/` — the full 58-token lexer plus a `TokenStream` with `LT/LA/Consume/Mark/Rewind` parity to antlr4ng's `CommonTokenStream`. After NAI-203, the next non-trivial step toward end-to-end `.rs2` → `script.dat/idx` compilation is the parser + AST: convert the lexer's token stream into an in-memory AST the type-checker (NAI-205) and bytecode emitter (NAI-206) can walk.

The slice arc, unchanged from NAI-202 §12 and NAI-203 §0:

- NAI-203 (closed): lexer + token stream — `src/antlr/RuneScriptLexer.g4`.
- **NAI-204 (this slice)**: parser + AST — `src/antlr/RuneScriptParser.g4` (238 lines) + `src/parser/ast/` + the dispatch logic from `src/parser/parser/AstBuilder.ts` (481 lines). Hand-written recursive descent over `lexer.TokenStream` producing AST nodes directly (no intermediate parse-tree).
- NAI-205: type checker + symbol resolution — `src/compiler/semantics/TypeChecking.ts` consumes NAI-202 symbols.
- NAI-206: bytecode emitter — `src/compiler/codegen/CodeGenerator.ts`.
- NAI-207: top-level `CompileServerScript` driver + `RunServerCompiler` wrapper.

NAI-204 is intentionally narrow: byte source → `*ast.ScriptFile` (or nil + reported errors). No semantic information populated on nodes — `Expression.Type`, `Identifier.Reference`, `Script.Symbol` and friends are NAI-205's job.

## 1. Goal

Hand-port `@lostcityrs/runescript`'s parser layer to goscape as a self-contained Go package pair. After this slice:

- `pkg/pack/compiler/ast/` exposes ~30 AST node types matching `src/parser/ast/` — sealed `Node` interface, Go-idiomatic struct types, exported `Kind()` discriminator.
- `pkg/pack/compiler/parser.NewScriptFileParser(input, sourceName)` returns a `*Parser`; calling `.ParseScriptFile()` produces `*ast.ScriptFile` (or `nil` on any reported syntax error).
- Sister entry points `NewScriptParser` (single `[trigger,name]` script) and `NewClientScriptParser` (one ClientScript reference) mirror `ScriptParser.createScript` / `ScriptParser.clientScript`.
- All 23 grammar productions from `RuneScriptParser.g4` parse correctly. Backtracking happens only at the two well-known ambiguous boundaries (`assignmentStatement` vs `expressionStatement`; `arrayDeclarationStatement` vs `declarationStatement`).
- `condition` and `arithmetic` left-recursive precedence rules are encoded via explicit precedence-climbing helpers — no general expression-parser framework.
- One real `.rs2` fixture (Neptune's `script.src` — same fixture as NAI-203 T14) round-trips through Lexer → TokenStream → Parser → AST with a smoke-shape assertion (script count + non-empty statements).

## 2. Scope

**In:**

- New package `pkg/pack/compiler/ast/` with files:
  - `node.go` — `Node` interface (sealed via unexported `isNode()`), `Children()` helper, `Source()` accessor.
  - `kind.go` — `NodeKind` iota constants (parity with `NodeKind.ts`) + `String()` method; diagnostic / serialization use only.
  - `scriptfile.go` — `ScriptFile`, `Script`, `Parameter`, `Token`.
  - `statements.go` — `BlockStatement`, `ReturnStatement`, `IfStatement`, `WhileStatement`, `SwitchStatement`, `SwitchCase`, `DeclarationStatement`, `ArrayDeclarationStatement`, `AssignmentStatement`, `ExpressionStatement`, `EmptyStatement`.
  - `expressions.go` — `ParenthesizedExpression`, `Identifier`, `JoinedStringExpression`, `BasicStringPart`, `PTagStringPart`, `ExpressionStringPart`.
  - `literals.go` — `IntegerLiteral`, `CoordLiteral`, `BooleanLiteral`, `CharacterLiteral`, `StringLiteral`, `NullLiteral` (no separate `HexLiteral`/`BinLiteral`; lexer keeps token-type distinct but AST collapses to `IntegerLiteral` per `AstBuilder.visitIntegerLiteral`).
  - `variables.go` — `LocalVariableExpression` (covers `$x` and `$arr(i)` via nullable Index field, parity with TS), `GameVariableExpression`, `ConstantVariableExpression`.
  - `calls.go` — `CommandCallExpression` (covers `name(...)` and `name*(...)(...)` via nullable Arguments2 field), `ProcCallExpression`, `JumpCallExpression`, `ClientScriptExpression`.
  - `arithmetic.go` — `ArithmeticExpression`, `CalcExpression`.
  - `condition.go` — `ConditionExpression`.
  - One `*_test.go` per file covering construction / Children / Kind round-trip.

- New package `pkg/pack/compiler/parser/` with files:
  - `parser.go` — `Parser` struct (token stream, listeners, error counter, source name), public constructors, top-level entry methods.
  - `errors.go` — `reportError`, `syncToStatement`, `expect(TokenType)` helper.
  - `script.go` — productions: `scriptFile`, `script`, `scriptName`, `parameterList`, `parameter`, `typeList`.
  - `statement.go` — `statement` dispatch + each statement production.
  - `expression.go` — `expression`, `call` (Command / Proc / Jump variants), `literal`, `joinedString`, `identifier`, `advancedIdentifier`, `localVariable`, `localArrayVariable`, `gameVariable`, `constantVariable`, `parenthesis`, `calc`, `assignableVariableList`.
  - `precedence.go` — `parseCondition` / `parseArithmetic` precedence climbers.
  - `parser_test.go`, `script_test.go`, `statement_test.go`, `expression_test.go`, `precedence_test.go`, `error_test.go`, `golden_test.go`.
  - `nai204_deviation_pins_test.go` — pins for `NAI-204-D-*` deviation tags actually used.
  - `testdata/golden_script.src` (copy of NAI-203 fixture).

**Out (deferred):**

- Semantic-analysis fields on AST nodes (`Expression.Type`, `Expression.TypeHint`, `Identifier.Reference`, `Script.Symbol`/`Block`/`ReturnType`/`TriggerType`/`SubjectReference`/`ParameterType`, `SwitchStatement.DefaultCase`/`Type`, `Declaration*.Symbol`, `CallExpression.Symbol`, `Literal.Reference`, `ConstantVariableExpression.SubExpression`, `StringLiteral.SubExpression`). NAI-205 adds these as it wires the type checker.
- `Node.Parent` back-pointer and the `findParentByType` helper. TS uses these for diagnostic walks; goscape consumers walk top-down and thread parent context explicitly when needed.
- `Node.Attributes` scratch map. TS uses it for compiler-stage side state; goscape consumers maintain side-tables keyed by node pointer.
- `Visitor` interface + per-node `Accept` methods. Consumers dispatch via Go type-switch.
- AST pretty-printer / source-text round-trip.
- Production wiring — nothing in `cmd/goscape` or `modules/` imports parser yet; NAI-207's `RunServerCompiler` wrapper does the wiring.

## 3. Tech stack

- Go 1.26+ (per [[go_version]] memory).
- No new external deps.
- Lexer dep: `pkg/pack/compiler/lexer` (consumed only by `parser/`, never by `ast/`).
- TS source-of-truth: `$HOME/Code/github.com/LostCityRS/RuneScriptTS` at HEAD `b8c338801fbb72d294ff9576a58925a8d3f6de47` (same pin as NAI-203). Specifically:
  - `src/antlr/RuneScriptParser.g4` (238 lines) — grammar.
  - `src/parser/ast/` (30 files) — AST node shapes.
  - `src/parser/parser/AstBuilder.ts` (481 lines) — visitor dispatch logic encoding AST construction from antlr parse-tree.
  - `src/parser/parser/ScriptParser.ts` (65 lines) — top-level driver / error-listener wiring.

## 4. Non-goals

- No semantic-analysis information on nodes (NAI-205+).
- No visitor pattern (Go type-switch dispatch — see §5.2, [[NAI-204-D-AST-NO-VISITOR]]).
- No parse-tree intermediate. Recursive descent emits AST nodes directly; the antlr `ParserRuleContext` → AST hop in `AstBuilder.ts` is folded into each production function.
- No general expression-parser framework (Pratt, etc.) — the grammar has a tiny operator set, precedence climbing is hand-rolled.
- No performance optimization beyond "single linear pass with O(1) lookahead". Real workload is one-shot at pack time.
- No source-map / preprocessor support.
- Unicode beyond ASCII — same constraint as the lexer.

## 5. Architecture

### 5.1 Package layout

```
pkg/pack/compiler/ast/                     (no parser dep)
  node.go            Node interface + Children/Source helpers
  kind.go            NodeKind iota
  scriptfile.go      ScriptFile, Script, Parameter, Token
  statements.go      Block/Return/If/While/Switch/SwitchCase/Decl/ArrayDecl/Assign/Expr/Empty
  expressions.go     Parenthesized, Identifier, JoinedString, *StringPart
  literals.go        Integer/Coord/Boolean/Character/String/Null
  variables.go       Local, Game, Constant
  calls.go           Command, Proc, Jump, ClientScript
  arithmetic.go      ArithmeticExpression, CalcExpression
  condition.go       ConditionExpression
  *_test.go          Construction + Children + Kind round-trip
pkg/pack/compiler/parser/                   (imports lexer + ast)
  parser.go          Parser struct, constructors, entry methods
  errors.go          reportError, syncToStatement, expect
  script.go          scriptFile / script / scriptName / parameterList / parameter / typeList
  statement.go       statement dispatch + each production
  expression.go      expression dispatch + call / literal / joinedString / variable / parenthesis / calc
  precedence.go      parseCondition + parseArithmetic precedence climbers
  *_test.go          Per-concern coverage
  testdata/          Neptune script.src (golden)
```

### 5.2 AST shape

The Go AST mirrors the TS class hierarchy but flattens it via a sealed `Node` interface plus concrete struct types. Consumers dispatch via Go type-switch.

```go
// pkg/pack/compiler/ast/node.go
package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// Node is the sealed root of the AST type hierarchy. Implementations
// live in this package only; the unexported isNode() method prevents
// out-of-package types from satisfying the interface.
type Node interface {
    Source() lexer.NodeSourceLocation
    Children() []Node
    Kind() NodeKind
    isNode()
}

// Expression is the marker for nodes that produce a value. Carried as
// a sub-interface so productions returning "any expression" can return
// Expression without losing the Node contract.
type Expression interface {
    Node
    isExpression()
}

// Statement is the marker for nodes that appear at statement position.
type Statement interface {
    Node
    isStatement()
}
```

Each concrete node embeds source location as a public field and implements `isNode()` (plus `isExpression()` / `isStatement()` where applicable). NodeKind is a parity discriminator (`String()` method names match TS NodeKind):

```go
// pkg/pack/compiler/ast/kind.go
type NodeKind int
const (
    KindScriptFile NodeKind = iota
    KindScript
    KindParameter
    KindToken
    KindBlockStatement
    // ... (all 35 kinds from NodeKind.ts)
)
```

Sample concrete node:

```go
// pkg/pack/compiler/ast/statements.go
type IfStatement struct {
    SrcLoc        lexer.NodeSourceLocation
    Condition     Expression
    ThenStatement Statement
    ElseStatement Statement // nil if no else clause
}

func (s *IfStatement) Source() lexer.NodeSourceLocation { return s.SrcLoc }
func (s *IfStatement) Kind() NodeKind                   { return KindIfStatement }
func (s *IfStatement) Children() []Node {
    out := []Node{s.Condition, s.ThenStatement}
    if s.ElseStatement != nil {
        out = append(out, s.ElseStatement)
    }
    return out
}
func (s *IfStatement) isNode()      {}
func (s *IfStatement) isStatement() {}
```

**Field naming:** match TS field names converted to Go-exported case. TS `thenStatement` → `ThenStatement`, `kind` → discriminator method (not a field, to keep struct size tight and avoid two sources of truth). TS `vars` → `Vars`, `expressions` → `Expressions`, `arguments` → `Arguments`, `arguments2` → `Arguments2` (CommandCallExpression's optional second arg list).

**Collapsing TS classes:** the TS hierarchy uses inheritance heavily (`Literal<T>` → `IntegerLiteral`/`StringLiteral`/...; `BinaryExpression` → `Arithmetic`/`Condition`; `CallExpression` → `Command`/`Proc`/`Jump`/`ClientScript`; `VariableExpression` → `Local`/`Game`/`Constant`). Go AST flattens this — each leaf type carries its full field set. Common fields (e.g. `BinaryExpression.left`/`operator`/`right`) appear identically on `ArithmeticExpression` and `ConditionExpression`. The total LOC overhead vs interface-embedding tricks is ~30 lines and the type-switch story is much cleaner.

**Token node:** TS `Token` wraps an antlr token with just `.text`. Goscape `ast.Token` carries source location plus text (same as TS plus location parity). Used by `SwitchStatement.TypeToken` and `Script.ReturnTokens`.

### 5.3 Parser shape

Single struct, single goroutine. The constructor wires a fresh `lexer.Lexer` + listener registration, but defers token-stream drain until `ParseScriptFile` (or sibling entry) is called — so listeners added between construction and parse see both lexer-stage and parser-stage errors. This mirrors TS `ScriptParser.invokeParser` where listeners are registered between lexer construction and `entry(parser)` call.

```go
// pkg/pack/compiler/parser/parser.go
type Parser struct {
    lx         *lexer.Lexer         // held until first parse call
    ts         *lexer.TokenStream   // nil until ensureStream() called
    sourceName string
    listeners  []lexer.ErrorListener
    numErrors  int
}

// NewScriptFileParser constructs a Parser positioned at the scriptFile
// entry rule. Token-stream drain is deferred — callers should add any
// listeners (lexer + parser stages share them) before invoking
// ParseScriptFile.
func NewScriptFileParser(input, sourceName string) *Parser {
    return &Parser{
        lx:         lexer.NewLexer(input, sourceName),
        sourceName: sourceName,
    }
}

// AddErrorListener registers l for both lexer-stage and parser-stage
// syntax errors. Must be called before the first Parse* invocation;
// adding a listener after parse-time is a no-op (lexer is already
// drained).
func (p *Parser) AddErrorListener(l lexer.ErrorListener) {
    p.listeners = append(p.listeners, l)
}

// RemoveErrorListeners drops all listeners.
func (p *Parser) RemoveErrorListeners() {
    p.listeners = nil
}

// ensureStream lazily drains the lexer into a TokenStream, wiring all
// registered listeners onto the lexer first so lexer-stage SyntaxError
// callbacks reach them.
func (p *Parser) ensureStream() {
    if p.ts != nil {
        return
    }
    for _, l := range p.listeners {
        p.lx.AddErrorListener(l)
    }
    p.ts = lexer.NewTokenStream(p.lx)
}

// ParseScriptFile returns nil if at least one syntax error was
// reported via any listener. Mirrors ScriptParser.invokeParser's
// "numberOfSyntaxErrors > 0 ⇒ return null".
func (p *Parser) ParseScriptFile() *ast.ScriptFile {
    p.ensureStream()
    sf := p.parseScriptFileBody()
    if p.numErrors > 0 {
        return nil
    }
    return sf
}
```

Sister constructors `NewScriptParser(input, sourceName)` and `NewClientScriptParser(input, sourceName)` produce parsers positioned at the corresponding entry rule. For test convenience the package provides a `newTestParserCollecting(t, src)` helper in `parser_test.go` that wires a `lexer.CollectingErrorListener` and returns both the parser and the listener.

### 5.4 Production-level mapping

Each grammar rule corresponds to one Go method on `*Parser`. Names use lower-case prefix `parseXxx`. The method consumes from `p.ts`, returns the corresponding `ast` node (or zero value on hard failure), and reports syntax errors via `p.reportError`.

Top-level methods that callers invoke directly (`parseScriptFileBody`, `parseScriptBody`, `parseClientScriptBody`) are package-private; the public surface is the three `Parse*` methods on `*Parser`.

Mapping (selected highlights):

| Grammar production | Go method | Notes |
|---|---|---|
| `scriptFile : script* EOF` | `parseScriptFileBody() *ast.ScriptFile` | Loop until EOF. |
| `script : LBRACK ... RBRACK ((LPAREN ...)?(LPAREN ...)?)? statement*` | `parseScript() *ast.Script` | LL(1) on second LPAREN for typeList. |
| `scriptName : identifier (identifier)*` | `parseScriptName() *ast.Identifier` | Concat with space when multiple. |
| `parameter : type DOLLAR advancedIdentifier` | `parseParameter() *ast.Parameter` | `type` is IDENTIFIER or TYPE_ARRAY. |
| `statement` | `parseStatement() ast.Statement` | Dispatch via LA(1) + select-LA(2/3/4) for assignment-vs-expr and array-vs-scalar decl. |
| `declarationStatement` vs `arrayDeclarationStatement` | `parseDeclOrArrayDecl() ast.Statement` | After `DEF_TYPE DOLLAR advancedIdentifier`, switch on LA(1): LPAREN → array, EQ → decl-with-init, SEMICOLON → decl-no-init. |
| `assignmentStatement` vs `expressionStatement` | `parseAssignOrExprStatement() ast.Statement` | Mark/Rewind: attempt LHS-list parse; if LHS landed on `EQ` it's assignment, else rewind and parse expression. |
| `switchStatement` | `parseSwitchStatement() *ast.SwitchStatement` | SWITCH_TYPE is the type token. |
| `switchCase : CASE (DEFAULT \| expressionList) COLON statement*` | `parseSwitchCase() *ast.SwitchCase` | `DEFAULT` → empty Keys slice (parity with TS `isDefault === (keys.length === 0)`). |
| `expression` | `parseExpression() ast.Expression` | Dispatch on LA(1): LPAREN → paren, CALC → calc, TILDE → proc, AT → jump, IDENTIFIER + LPAREN → command (with optional MUL for two-list form), DOLLAR → local, MOD/DOTMOD → game, CARET → const, literal tokens → literal, QUOTE_OPEN → joinedString (or pure StringLiteral if no interpolation/tag parts), identifier → bare Identifier. |
| `condition` | `parseCondition() ast.Expression` | Precedence climbing — see §5.5. |
| `arithmetic` (only inside `calc`) | `parseArithmetic() ast.Expression` | Precedence climbing — see §5.5. |
| `call` | `parseCall(name *ast.Identifier) ast.Expression` | Dispatched from expression-level after seeing `~`/`@`/IDENTIFIER+LPAREN. |
| `clientScript : identifier (LPAREN args? RPAREN)? (LBRACE triggers? RBRACE)? EOF` | `parseClientScriptBody() *ast.ClientScriptExpression` | Entry rule for `NewClientScriptParser`. |
| `localVariable : DOLLAR advancedIdentifier` | `parseLocalVariable() *ast.LocalVariableExpression` | Index = nil. |
| `localArrayVariable : DOLLAR advancedIdentifier parenthesis` | (folded into above by LA(LPAREN)) | Index = parenthesized expression. |
| `gameVariable : (MOD\|DOTMOD) advancedIdentifier` | `parseGameVariable() *ast.GameVariableExpression` | Dot = LA(1)==DOTMOD. |
| `constantVariable : CARET advancedIdentifier` | `parseConstantVariable() *ast.ConstantVariableExpression` | Straightforward. |

### 5.5 Expression precedence

The grammar encodes precedence via rule order in the left-recursive alternatives. RuneScriptTS relies on ANTLR's LL(*) to handle it. Goscape encodes the same precedence ladder explicitly:

`condition`:
- Level 1 (lowest): `OR`
- Level 2: `AND`
- Level 3: `EQ`, `EXCL`
- Level 4 (highest): `LT`, `GT`, `LTE`, `GTE`
- Atom: `LPAREN condition RPAREN` | `expression` (the `ConditionNormalExpression` alt — calls `parseExpression`)

`arithmetic`:
- Level 1 (lowest): `OR`
- Level 2: `AND`
- Level 3: `PLUS`, `MINUS`
- Level 4 (highest): `MUL`, `DIV`, `MOD`
- Atom: `LPAREN arithmetic RPAREN` | `expression`

All operators left-associative (parity with antlr's default for left-recursive alternatives).

```go
// pkg/pack/compiler/parser/precedence.go
func (p *Parser) parseCondition() ast.Expression {
    return p.parseConditionOr()
}
func (p *Parser) parseConditionOr() ast.Expression {
    left := p.parseConditionAnd()
    for p.ts.LA(1) == lexer.OR {
        op := p.consumeToken()
        right := p.parseConditionAnd()
        left = &ast.ConditionExpression{
            SrcLoc:   spanOf(left, right),
            Left:     left,
            Operator: op,
            Right:    right,
        }
    }
    return left
}
// parseConditionAnd / parseConditionEq / parseConditionRel similarly.
func (p *Parser) parseConditionAtom() ast.Expression {
    if p.ts.LA(1) == lexer.LPAREN {
        // ConditionParenthesizedExpression — wrap inner condition in
        // ParenthesizedExpression, parity with AstBuilder.visitConditionParenthesizedExpression.
        ...
    }
    return p.parseExpression() // ConditionNormalExpression
}
```

`spanOf(left, right)` synthesizes `NodeSourceLocation{Name, Line=left.Source().Line, Column=left.Source().Column, EndLine=right.Source().EndLine, EndColumn=right.Source().EndColumn}`. Note: NAI-203 lexer already hands out 1-based line/column on tokens — we never re-do `col+1` at the parser layer ([[plan_arithmetic_off_by_one_carry_forward]] guard).

### 5.6 Error reporting

The parser reports errors via the same `lexer.ErrorListener` interface as the lexer. Two error categories:

1. **Token-level**: lexer-side errors emitted during stream construction. NAI-203 already wires these through ErrorListener; goscape parser preserves them by registering listeners on the underlying Lexer before drain.
2. **Production-level**: unexpected-token, missing-token. Reported with the offending token's source location.

```go
// pkg/pack/compiler/parser/errors.go
func (p *Parser) reportError(tok *lexer.Token, format string, args ...any) {
    msg := fmt.Sprintf(format, args...)
    for _, l := range p.listeners {
        l.SyntaxError(p.sourceName, tok.Source.Line, tok.Source.Column, msg)
    }
    p.numErrors++
}

// expect consumes the current token if it matches; otherwise reports
// a "missing X" error using the current token's location and returns
// a zero-valued token (caller decides whether to sync).
func (p *Parser) expect(tt lexer.TokenType) lexer.Token {
    if p.ts.LA(1) != tt {
        cur := p.ts.LT(1)
        p.reportError(cur, "expected %s but found %s", tt, cur.Type)
        return lexer.Token{}
    }
    cur := *p.ts.LT(1)
    p.ts.Consume()
    return cur
}
```

**Recovery: panic-mode sync at statement boundaries.** On unexpected-token, `syncToStatement()` consumes until LA(1) is one of: `SEMICOLON`, `LBRACE`, `RBRACE`, `LBRACK`, `EOF`. If sync landed on `SEMICOLON`, consume it (assume the statement is done). Resume the enclosing parse loop. Pinned in `error_test.go`.

`ParseScriptFile` returns `nil` if `p.numErrors > 0` — parity with `ScriptParser.invokeParser`. Callers wanting partial AST can call `parseScriptFileBody()` directly (package-private; test-only).

### 5.7 Specific dispatch details

**Statement dispatch** (`parseStatement`):
```
LA(1):
  LBRACE        → parseBlockStatement
  RETURN        → parseReturnStatement
  IF            → parseIfStatement
  WHILE         → parseWhileStatement
  SWITCH_TYPE   → parseSwitchStatement
  DEF_TYPE      → parseDeclOrArrayDecl
  SEMICOLON     → parseEmptyStatement
  default       → parseAssignOrExprStatement
```

The default branch attempts to parse the LHS of an assignment; if it sees `EQ` after the LHS list, it's an assignment, else rewind and parse as `expressionStatement`. See [[plan_dispatch_order_self_inconsistency]] — the dispatch order above is final and non-overlapping (each TokenType case is unique).

**LiteralExpression dispatch**:
```
LA(1):
  INTEGER_LITERAL              → IntegerLiteral (parse text as decimal, parity with visitIntegerLiteral)
  HEX_LITERAL                  → IntegerLiteral (parse text[2:] as hex)
  BIN_LITERAL                  → IntegerLiteral (parse text[2:] as binary)
  COORD_LITERAL                → CoordLiteral (bit-pack per visitCoordLiteral)
  BOOLEAN_LITERAL              → BooleanLiteral (text == "true")
  CHAR_LITERAL                 → CharacterLiteral (unescape between quotes; require len 1)
  NULL_LITERAL                 → NullLiteral (value = -1, parity with TS NullLiteral)
  QUOTE_OPEN                   → joinedString (if any STRING_TAG/STRING_P_TAG/STRING_EXPR_START parts, return JoinedStringExpression;
                                                otherwise return StringLiteral with the unescaped concatenation)
```

**joinedString collapsing rule:** the TS path emits `JoinedStringExpression` for *all* string contexts, distinguishing in the grammar via `stringLiteral` vs `joinedString`. Inspecting `AstBuilder.visitStringLiteral` (line 330) vs `visitJoinedString` (line 338): TS keeps the two distinct. Goscape follows TS — `parseStringLiteralOrJoined` checks the body for interpolation parts (`STRING_TAG`, `STRING_P_TAG`, `STRING_EXPR_START`); if any present, returns `JoinedStringExpression`; if only `STRING_TEXT` parts, returns `StringLiteral` with concatenated unescaped text. (Confirms a TS-faithful semantic split — see [[true_to_ts_gate]].)

**`call` dispatch:**
```
LA(1):
  IDENTIFIER, LA(2)==LPAREN              → CommandCallExpression (args1 only)
  IDENTIFIER, LA(2)==MUL, LA(3)==LPAREN  → CommandCallExpression (args1 + args2, isStar)
  TILDE                                  → ProcCallExpression
  AT                                     → JumpCallExpression
```

Call-expression entry is reachable both from top-level expression dispatch (when the lookahead pattern matches) and from `expressionStatement`. Same parser method, no duplication.

**`unescape` helper:** port of `AstBuilder.unescape` (lines 463-480). Lives in `parser/expression.go` (or a tiny `string_part.go` sibling). Only `\\ \' \" \<` valid; anything else is an `ErrorListener.SyntaxError` (the TS path throws — goscape converts to syntax-error report since we collect rather than panic).

## 6. Data flow

```
source bytes
   │
   │  lexer.NewLexer(input, sourceName)
   ▼
lexer.Lexer
   │
   │  lexer.NewTokenStream(lx)   // pre-buffers all tokens
   ▼
lexer.TokenStream
   │
   │  parser.NewScriptFileParser(input, sourceName)
   │  parser.AddErrorListener(l)  // forwards to lexer + parser
   │  parser.ParseScriptFile()
   ▼
ast.ScriptFile  (or nil + reported errors via listener)
   │
   │  consumers (NAI-205 type checker, NAI-206 codegen)
   │  walk via type-switch on ast.Node
```

`NodeSourceLocation` flows through every layer with 1-based line/column at every public surface — never re-shifted.

## 7. Backward-compat / migrations

None. Both `pkg/pack/compiler/ast/` and `pkg/pack/compiler/parser/` are new packages. Existing callers continue to work — `pkg/pack/compiler/lexer/` API is unchanged.

The five anticipated deviation tags (see §10) are pinned with `nai204_deviation_pins_test.go` so removing the deviation (e.g. retro-adding a visitor pattern) is caught by failing pins.

## 8. Risks and trade-offs

| Risk | Mitigation |
|---|---|
| Expression precedence climber drifts from grammar order | `precedence_test.go` pins associativity + level cross-product (`1 + 2 * 3`, `1 \| 2 & 3`, `(1 \| 2) & 3`, etc.) |
| Type-switch dispatch becomes verbose in NAI-205/206 | Optional helper: package-level `Walk(node, fn)` for top-down traversal lands later if needed; YAGNI for this slice. Type-switch is mostly in one place per consumer. |
| Backtracking on assignment-vs-expression is broken / accidentally exponential | Single Mark/Rewind per statement attempt; if LHS-parse advances past EQ, commit; if not, rewind exactly once. Tested in `statement_test.go` with both shapes. |
| String literal vs joined string collapsing diverges from TS | TS keeps them distinct (`visitStringLiteral` vs `visitJoinedString`). Goscape parser uses presence-of-interpolation-parts as the discriminator. Pin: `"plain"` → `*StringLiteral`, `"hello <$x>"` → `*JoinedStringExpression`, `"<br>"` (tag, no interp) → `*JoinedStringExpression`. |
| Source-span synthesis (`spanOf(left, right)`) wrong on multi-line spans | Test in `precedence_test.go` covers a binary expression spanning two lines; assert `EndLine > Line`. |
| `Identifier` field collisions across `Script.Name` (multi-token concat) and bare identifier | Parser synthesizes a single `*ast.Identifier` with concatenated text when `scriptName` has > 1 sub-identifier (parity with `visitScriptName` lines 150-157). Source span covers both. |

## 9. Test strategy

Per [[plan_test_coverage_crosscheck]] — every plan task block lists its tests inline. Coverage:

- `ast/*_test.go` — for each AST node type: construct via field init, verify Source/Children/Kind round-trip. ~30 tests, mostly one-liners.
- `parser/parser_test.go` — top-level: empty file, single script, multiple scripts, header variants.
- `parser/script_test.go` — script header productions: trigger types, isStar, parameter list, return-type list, both lists, neither, scriptName multi-identifier.
- `parser/statement_test.go` — each statement kind, both decl shapes, both assign-vs-expr shapes, switch with default + multi-key case.
- `parser/expression_test.go` — each call shape (Command 1-list, Command 2-list, Proc, Jump, ClientScript), each variable shape, each literal type (including `null` → `NullLiteral.Value == -1`), joined string with tag + p-tag + interpolation.
- `parser/precedence_test.go` — operator precedence + associativity for condition + arithmetic. Spans pinned.
- `parser/error_test.go` — missing semicolon, unexpected token at statement-start, unterminated switch, unrecognized escape in string. Each pinned via collected `ErrorListener.SyntaxError` payload.
- `parser/golden_test.go` — Neptune `script.src` round-trip. Asserts `len(ScriptFile.Scripts) > 0`, every script has at least one statement, no errors reported. Shape pin only — no byte-level golden (AST printer deferred).
- `parser/nai204_deviation_pins_test.go` — pins for each `NAI-204-D-*` tag in §10 actually used.

TDD: each grammar production lands in red-then-green order in its task; reviewer verifies green via fresh `go test ./pkg/pack/compiler/...` after each task ([[stale_ide_diagnostic_during_tdd_red_phase]]).

## 10. Deviation tags

Pinned in `parser/nai204_deviation_pins_test.go`; each tag references a docstring on the canonical site in production.

- **`NAI-204-D-AST-NO-VISITOR`** — TS `AstVisitor<R>` + `accept(visitor)` mirror is dropped; consumers dispatch via Go type-switch. Pinned on `ast.Node` interface doc. Reason: idiomatic Go, no per-node `Accept` boilerplate, smaller dispatch surface in NAI-205/206. Reversible by adding `Accept(v Visitor) any` to each concrete node.
- **`NAI-204-D-AST-NO-PARENT`** — `Node.parent` back-pointer + `findParentByType` helper dropped. Pinned on `ast.Node` interface doc. Reason: goscape consumers walk top-down with explicit parent context. Memo: NAI-205+ may need to revive if TS diagnostic code paths require it.
- **`NAI-204-D-AST-NO-ATTRIBUTES`** — `Node.attributes` scratch map dropped. Pinned on `ast.Node` interface doc. Reason: side-tables keyed by node pointer are clearer than untyped attribute bags. Memo: NAI-205 may need an attribute-equivalent.
- **`NAI-204-D-AST-NO-TYPE-FIELDS`** — `Expression.Type`, `Expression.TypeHint`, `Identifier.Reference`, `Script.Symbol`/`Block`/`ReturnType`/`TriggerType`/`SubjectReference`/`ParameterType`, `SwitchStatement.DefaultCase`/`Type`, `DeclarationStatement.Symbol`, `ArrayDeclarationStatement.Symbol`, `CallExpression.Symbol`, `Literal.Reference`, `ConstantVariableExpression.SubExpression`, `StringLiteral.SubExpression`, `VariableExpression.Reference` — all NAI-205-owned fields skipped here. Pinned on `Expression` / `Script` / etc. interface docs. NAI-205 lifts the pin when it adds the fields.
- **`NAI-204-D-PARSER-PANIC-SYNC`** — Panic-mode sync at statement boundaries (`SEMICOLON`/`LBRACE`/`RBRACE`/`LBRACK`/`EOF`) instead of ANTLR's `DefaultErrorStrategy`. Pinned in `error_test.go`. Reason: hand-written parser, simpler recovery is correct enough for collect-all-errors mode.

If implementation surfaces additional divergences, plan-author adds them in the task that introduces them (per [[emergent_deviation_mid_impl]]).

## 11. Memory-note guards

Per the cadence brief and prior NAI experience:

- [[plan_arithmetic_off_by_one_carry_forward]] — every column/line computation in plan code blocks traces against the 1-based contract; lexer hands tokens already in 1-based, so the parser never adds 1 again.
- [[plan_dispatch_order_self_inconsistency]] — statement and expression dispatch use disjoint TokenType cases; final dispatch order documented in §5.7.
- [[plan_code_block_t_number_drift]] — plan author embeds "Authoritative task numbering: T1, T2, ..." line at top of plan; controller passes it in every dispatch prompt.
- [[stale_ide_diagnostic_during_tdd_red_phase]] — reviewer verifies green via fresh `go test`, not LSP snapshots.
- [[spec_ts_source_read]] — every plan code block traced against RuneScriptTS HEAD `b8c338801fbb72d294ff9576a58925a8d3f6de47`.
- [[plan_runnable_test_fixtures]] — plan-codified fixtures mentally compiled before dispatch.
- [[plan_var_name_collision]] — Go variable-name collisions audited per function body.
- [[true_to_ts_gate]] — every behavioral divergence pinned as `NAI-204-D-*` with rationale.

## 12. Task slicing (preview)

Final ordering and line-by-line code blocks are plan-author's job. Natural ~10-task shape:

- **T1** — `pkg/pack/compiler/ast/` skeleton: `node.go`, `kind.go`, `scriptfile.go` (ScriptFile, Script, Parameter, Token). Tests.
- **T2** — `pkg/pack/compiler/ast/statements.go` — all 11 statement node types + tests.
- **T3** — `pkg/pack/compiler/ast/expressions.go` + `literals.go` + `variables.go` + `calls.go` + `arithmetic.go` + `condition.go` + tests.
- **T4** — `pkg/pack/compiler/parser/` skeleton: `parser.go` (Parser struct, constructors, AddErrorListener forwarding) + `errors.go` (reportError, expect, syncToStatement). One smoke test on empty input.
- **T5** — `script.go`: scriptFile / script header / scriptName / parameterList / parameter / typeList. Tests pin all header shapes.
- **T6** — `statement.go` part 1: blockStatement, emptyStatement, returnStatement, ifStatement, whileStatement. Per-statement tests.
- **T7** — `statement.go` part 2: switchStatement, switchCase, parseDeclOrArrayDecl (LA-dispatch).
- **T8** — `statement.go` part 3 + `expression.go` part 1: parseAssignOrExprStatement (Mark/Rewind), assignableVariableList; basic expression dispatch + literal/identifier/variable subset.
- **T9** — `expression.go` part 2 + `precedence.go`: call shapes (Command/Proc/Jump), joinedString + string literal collapse + unescape, calc, parenthesis; condition + arithmetic precedence climbers.
- **T10** — `clientScript` entry rule (parseClientScriptBody, NewClientScriptParser); `error_test.go` recovery tests; `golden_test.go` Neptune round-trip.
- **T11** — `nai204_deviation_pins_test.go` + close commit (per [[close_commit_memory_trailer.md]]).

Each task is ~50–150 LOC + tests. Reviewer on Sonnet after each. Two-stage review per [[runescript_cadence]].
