# NAI-203 — RuneScript lexer + token stream (compiler slice 1 of 5)

## 0. Pre-context: where this slice sits in the arc

NAI-202 (closed at 38de2a7) shipped `pkg/pack/compiler.BuildSymbols(srcDir, dataPackDir)` — the 32-key symbol table TS `Compiler.ts:330-365` hands to `CompileServerScript`. After NAI-202, the **only** thing blocking end-to-end `.rs2` → `script.dat/idx` compilation is the external `@lostcityrs/runescript` compiler itself.

That compiler is non-trivial — RuneScriptTS clone at `/home/owner/Code/github.com/LostCityRS/RuneScriptTS` is **130 TypeScript files** plus two ANTLR4 grammars (`src/antlr/RuneScriptLexer.g4` 105 lines, `RuneScriptParser.g4` 228 lines) compiled via `antlr-ng` to TypeScript. The arc decomposition from NAI-202 §12, lightly refined here:

- **NAI-203 (this slice)**: lexer + token stream — hand-port of `src/antlr/RuneScriptLexer.g4`.
- **NAI-204**: parser + AST — hand-port of `RuneScriptParser.g4` plus `src/parser/ast/` and `src/parser/parser/AstBuilder.ts`.
- **NAI-205**: type checker + symbol resolution — `src/compiler/semantics/TypeChecking.ts` consumes NAI-202 symbols.
- **NAI-206**: bytecode emitter — `src/compiler/codegen/CodeGenerator.ts`, writes `script.dat` / `script.idx`.
- **NAI-207**: top-level `CompileServerScript` driver + `RunServerCompiler` wrapper.

NAI-203 is intentionally narrow: source bytes → flat token slice + lookahead-capable `TokenStream`. No AST, no error recovery beyond what antlr's default-listener path emits, no production consumer in this slice.

## 1. Goal

Port `@lostcityrs/runescript` lexer to goscape as a self-contained hand-written Go package. After this:

- `pkg/pack/compiler/lexer.NewLexer(input, sourceName)` produces a `*Lexer` that emits tokens via `NextToken()` until EOF.
- `pkg/pack/compiler/lexer.NewTokenStream(lexer)` pre-buffers all tokens and exposes antlr-`CommonTokenStream`-parity API (`LT(k)`, `LA(k)`, `Consume`, `Index`, `Mark`, `Release`, `Rewind`) with channel-0 (default) filtering.
- All 58 token types from `RuneScriptLexer.g4` (plus `EOF`) are emitted with correct text, line/column, and channel.
- The two-mode state machine (`DEFAULT` + `String`) with `depth` counter and push/popMode transitions matches ANTLR semantics on grammar-fidelity tests.
- One real `.rs2` fixture (Neptune's `script.src` — interpolation-heavy) round-trips through Lexer → TokenStream with a hand-authored golden token sequence.
- Three start-sweep cleanups from NAI-202 review close: typeinfo.go forward-reference rot; `PointerGroupFind()` uses `slices.Clone`; `corruptExceptActive` uses `slices.Concat`.

## 2. Scope

**In:**

- New package `pkg/pack/compiler/lexer/` with files:
  - `tokens.go` — `TokenType` iota constants (59 types: EOF + 58 from .g4), `tokenName` table, channel constants.
  - `source_location.go` — `NodeSourceLocation` struct (parity with `src/parser/ast/NodeSourceLocation.ts`).
  - `errors.go` — `ErrorListener` interface, `DiscardErrorListener`, `CollectingErrorListener` (test-only).
  - `lexer.go` — `Lexer` struct, `NewLexer`, `NextToken`, `AddErrorListener`, `RemoveErrorListeners`.
  - `lexer_default.go` — DEFAULT-mode dispatch (symbols, keywords, literals, identifier, comments, whitespace, `"` → push String).
  - `lexer_string.go` — String-mode dispatch (text, tags, interpolation, `"` → pop, `<` → push DEFAULT).
  - `token_stream.go` — `TokenStream` with hidden-channel filtering and lookahead.
  - `lexer_test.go`, `lexer_modes_test.go`, `token_stream_test.go`, `golden_test.go` (one per concern).
  - `testdata/golden_script.src` (copy of Neptune `script.src`) + `testdata/golden_script.tokens` (hand-authored expected token sequence).
  - `nai203_deviation_pins_test.go` — pins for any `NAI-203-D-*` deviation tags actually used.
- Edit `pkg/pack/compiler/typeinfo.go` — scrub 7 "NAI-201 will port X" forward-references (lines per `package_doc_forward_references_rot` memory note).
- Edit `pkg/script/opcode_pointers.go` — `PointerGroupFind()` returns `slices.Clone(pointerGroupFind[:])`; `corruptExceptActive` returns `slices.Concat(pointerGroupFind[:], extras)`.

**Out (deferred):**

- Parser, AST nodes, AST builder — NAI-204.
- `ANTLRErrorListener`'s `reportAmbiguity` / `reportAttemptingFullContext` / `reportContextSensitivity` — LL(*)-prediction artifacts; hand-written recursive-descent parser in NAI-204 doesn't need them.
- Token-rewriting machinery (antlr's `TokenStreamRewriter`) — not consumed by RuneScriptTS pipeline.
- Production wiring — nothing in `cmd/goscape` or `modules/` imports lexer yet; NAI-207's `RunServerCompiler` wrapper does the wiring.

## 3. Tech stack

- Go 1.26+ (per [[go_version]] memory; uses `slices.Clone`, `slices.Concat`, `strings.SplitSeq` if helpful).
- No new external deps. No ANTLR4 runtime, no codegen toolchain.
- TS source-of-truth: `/home/owner/Code/github.com/LostCityRS/RuneScriptTS/src/antlr/RuneScriptLexer.g4` (105 lines).
- Cross-reference: `RuneScriptTS/src/parser/parser/ScriptParser.ts`, `RuneScriptTS/src/parser/ast/NodeSourceLocation.ts`, `RuneScriptTS/src/compiler/ParserErrorListener.ts`.

## 4. Non-goals

- Token-stream rewriting / synthetic-insert APIs.
- Source-map / preprocessor support.
- Hot-reload or incremental re-lex on edit.
- Performance optimization beyond "linear scan, no regex" — the realistic compile workload is one-shot at pack time.
- Unicode beyond ASCII — `.rs2` source is ASCII (the grammar's character classes are all ASCII ranges). Treat non-ASCII bytes as unrecognized.

## 5. Architecture

### 5.1 Package layout

```
pkg/pack/compiler/lexer/
  tokens.go
  source_location.go
  errors.go
  lexer.go
  lexer_default.go
  lexer_string.go
  token_stream.go
  testdata/
    golden_script.src
    golden_script.tokens
  *_test.go
```

The lexer is a sibling of `pkg/pack/compiler/{symbols,typeinfo}.go` (NAI-200/202 surface) — both feed the eventual `CompileServerScript` driver in NAI-207.

### 5.2 Token catalog

`type TokenType int` with `iota` constants in **declaration order from the .g4** so disambiguation by declaration order is naturally `TokenType` comparison. Final order:

```
EOF                                  // sentinel; not from .g4
LPAREN RPAREN COLON SEMICOLON COMMA
LBRACK RBRACK LBRACE RBRACE
PLUS MINUS MUL DIV DOTMOD MOD
AND OR EQ EXCL DOLLAR CARET TILDE AT
GT GTE LT LTE
IF ELSE WHILE CASE DEFAULT RETURN CALC
TYPE_ARRAY DEF_TYPE SWITCH_TYPE
INTEGER_LITERAL HEX_LITERAL BIN_LITERAL
COORD_LITERAL MAPZONE_LITERAL
BOOLEAN_LITERAL CHAR_LITERAL NULL_LITERAL
LINE_COMMENT BLOCK_COMMENT
QUOTE_OPEN IDENTIFIER WHITESPACE
// String mode:
QUOTE_CLOSE STRING_TEXT STRING_TAG STRING_CLOSE_TAG STRING_PARTIAL_TAG STRING_P_TAG STRING_EXPR_START STRING_EXPR_END
```

`tokenName [...]string` parallel-indexed table gives `(TokenType).String()`. Channel constants `ChannelDefault = 0`, `ChannelHidden = 1` mirror antlr.

### 5.3 Token struct

```go
type Token struct {
    Type    TokenType
    Channel int                // 0 = default, 1 = hidden
    Text    string             // matched text (slice of input)
    Source  NodeSourceLocation // line/col + endLine/endCol, 1-based
    Start   int                // byte offset into input (inclusive)
    Stop    int                // byte offset into input (inclusive)
    Index   int                // 0-based position in token slice
}
```

The TS `Token` AST wrapper (`src/parser/ast/Token.ts`) only exposes `{text}`; we ship more because the **parser** (NAI-204) will read antlr-style `start`/`stop`/`index` for error diagnostics, and NAI-205 / NAI-206 read `Source` for `NodeSourceLocation`. Pre-loading them now avoids a churning Token shape across slices.

### 5.4 Lexer state machine

```go
type Lexer struct {
    input      string
    sourceName string

    pos   int    // current byte offset (next unread)
    line  int    // 1-based line of pos
    col   int    // 0-based column of pos (antlr convention)

    depth int    // string-interpolation nesting (.g4 @members)
    modes []mode // mode stack; top = current. Init: []mode{modeDefault}

    listeners  []ErrorListener
    tokenIndex int // 0-based; next-emitted token's Index
}

type mode int
const (
    modeDefault mode = iota
    modeString
)
```

**Mode stack helpers:**

```go
func (l *Lexer) currentMode() mode      { return l.modes[len(l.modes)-1] }
func (l *Lexer) pushMode(m mode)        { l.modes = append(l.modes, m) }
func (l *Lexer) popMode()               { l.modes = l.modes[:len(l.modes)-1] }
```

`popMode()` on a stack of length 1 is a precondition violation; lexer never pops the bottom DEFAULT mode (matches antlr — `popMode` from default-only stack panics in antlr too; our grammar can't reach that state). Document as plan-author audit.

**NextToken:**

```go
func (l *Lexer) NextToken() Token {
    if l.pos >= len(l.input) {
        return l.makeToken(EOF, l.pos, l.pos-1, "")
    }
    switch l.currentMode() {
    case modeDefault:
        return l.nextDefault()
    case modeString:
        return l.nextString()
    default:
        panic("unreachable")
    }
}
```

### 5.5 DEFAULT-mode dispatch (`lexer_default.go`)

The dispatch is a hand-written longest-match scanner. ANTLR semantics: among all rules that can match starting at `l.pos`, the rule consuming the **most characters** wins; ties are broken by **declaration order** in the .g4 (earlier wins). The implementation below routes by first-char equivalence class to candidate rules, then materializes longest-match where the class admits multiple rules.

**Algorithm:**

1. **Whitespace + comments** (no ambiguity with anything else):
   - If `[ \t\n\r]` → consume WHITESPACE run, emit on hidden channel.
   - If `/` and `l.input[l.pos+1] == '/'` → consume to first `\n` (inclusive) or EOF, emit LINE_COMMENT on hidden channel.
   - If `/` and `l.input[l.pos+1] == '*'` → consume to first `*/` (or EOF + emit unterminated error), emit BLOCK_COMMENT on hidden channel.

2. **Single-quote and double-quote** (no ambiguity — `'` is only CHAR_LITERAL start, `"` is only QUOTE_OPEN):
   - If `'` → CHAR_LITERAL attempt (escape or single non-special, then `'`); error if unterminated.
   - If `"` → emit QUOTE_OPEN, `depth++`, `pushMode(modeString)`.

3. **Negative-INTEGER prefix**: if `-` followed by `[0-9]`, the only longest-match candidate that consumes both chars is INTEGER_LITERAL (`'-'? Digit+`). Other rules starting at `-`: MINUS (length 1). INTEGER wins (length ≥ 2 > 1). Route to numeric-literal dispatch (§5.5.1) consuming the `-`.

4. **Digit start (`[0-9]`)**: multiple rules can match — INTEGER, HEX (if `0x`/`0X`), BIN (if `0b`/`0B`), MAPZONE/COORD (if underscore-separated), and IDENTIFIER (digits are in the identifier class). Route to **§5.5.1 numeric-or-identifier dispatch** which tentatively matches each candidate and picks longest then declaration order.

5. **Letter / underscore start (`[a-zA-Z_]`)**: candidates are keywords, TYPE_ARRAY/DEF_TYPE/SWITCH_TYPE, BOOLEAN_LITERAL, NULL_LITERAL, IDENTIFIER. Route to **§5.5.2 identifier-or-keyword dispatch**.

6. **`+` start**: candidates are PLUS (length 1) and IDENTIFIER (length ≥ 1; `+` is in the id class). If `l.input[l.pos+1]` is also in id class, IDENTIFIER wins (longer). Else PLUS wins (tie at length 1; PLUS declared earlier at .g4 line 17 vs IDENTIFIER at line 74).

7. **`.` start**: candidates are DOTMOD (`.%`, length 2), IDENTIFIER (`.` is in id class, length ≥ 1). If next char is `%` → DOTMOD (length 2). Else if next char is in id class → IDENTIFIER. Else IDENTIFIER (single `.`, length 1; no other competitor).

8. **`:` start**: candidates are COLON (length 1) and IDENTIFIER (`:` is in id class). If next char is in id class → IDENTIFIER (longer). Else COLON (tie at length 1; COLON declared earlier).

9. **`>` start**: candidates are GT (length 1) and GTE (`>=`, length 2). If next char is `=` → GTE. Else → GT, **and apply the semantic action**: if `l.depth > 0`, retype to STRING_EXPR_END and `popMode()`. Otherwise emit GT.

10. **`<` start**: candidates are LT (length 1) and LTE (`<=`, length 2). If next char is `=` → LTE. Else → LT.

11. **All other single-char symbols** (`(` `)` `:` already handled `;` `,` `[` `]` `{` `}` `*` `%` `&` `|` `=` `!` `$` `^` `~` `@`): emit corresponding single-char token. None of these chars are in the identifier class except none — wait, **none** of these are in `[a-zA-Z0-9_+.:]`. ✓ No ambiguity.

12. If no rule matches (e.g. raw `?`, non-ASCII byte) → emit `SyntaxError` via listeners, advance one byte, continue (skip-on-error recovery — matches antlr's default `LexerNoViableAltException` handling).

#### 5.5.1 Numeric-or-identifier dispatch (digit-start, or `-` + digit)

When the first char is `[0-9]` (or `-` followed by `[0-9]`), **multiple rules can match**. The lexer materializes each candidate's length and picks the **longest**, breaking ties by **declaration order**. Candidates:

| Rule | Match | .g4 line |
|---|---|---|
| HEX_LITERAL | `0[xX][0-9a-fA-F]+` | 50 |
| BIN_LITERAL | `0[bB][01]+` | 51 |
| COORD_LITERAL | `Digit+_Digit+_Digit+_Digit+_Digit+` | 52 |
| MAPZONE_LITERAL | `Digit+_Digit+_Digit+` | 53 |
| INTEGER_LITERAL | `-?Digit+` | 49 |
| IDENTIFIER | `[a-zA-Z0-9_+.:]+` | 74 |

**Algorithm:**

1. **Tentatively consume the maximal IDENTIFIER run** starting at `l.pos` (greedy `[a-zA-Z0-9_+.:]+`). Call its end-offset `idEnd`. This bounds every numeric candidate, since none of them can extend past an identifier-class boundary (all numeric rules consume only chars also in the id class).
2. **Tentatively try each numeric candidate** within `[l.pos, idEnd)`, recording each candidate's match length (or 0 if it fails):
   - HEX: starts with `0[xX]` followed by `[0-9a-fA-F]+`.
   - BIN: starts with `0[bB]` followed by `[01]+`.
   - COORD: matches `Digit+_Digit+_Digit+_Digit+_Digit+` from `l.pos`.
   - MAPZONE: matches `Digit+_Digit+_Digit+` from `l.pos`.
   - INTEGER: matches `'-'? Digit+` from `l.pos`.
3. **Pick the winner**: max-length among all candidates including IDENTIFIER (which always has length `idEnd - l.pos`). If multiple candidates tie at max length, take the one declared earliest in the .g4 (HEX < BIN < COORD < MAPZONE < INTEGER < IDENTIFIER per line numbers above; note INTEGER is line 49, which is **before** HEX 50).
4. Emit the winning token.

**Worked examples** (in priority order — plan-author must pin each in tests):

| Input | IDENT len | INT len | HEX len | BIN len | COORD len | MAPZONE len | Winner |
|---|---|---|---|---|---|---|---|
| `5` | 1 | 1 | 0 | 0 | 0 | 0 | INTEGER (line 49 beats IDENT line 74 at length 1) |
| `5abc` | 4 | 1 | 0 | 0 | 0 | 0 | IDENTIFIER (length 4 > INTEGER 1) |
| `0x1F` | 4 | 1 | 4 | 0 | 0 | 0 | HEX (longest at 4; ties IDENT, HEX line 50 < IDENT line 74) |
| `0xZZZ` | 5 | 1 | 0 | 0 | 0 | 0 | IDENTIFIER (length 5; HEX fails — no hex digit after `0x`) |
| `0b101` | 5 | 1 | 0 | 5 | 0 | 0 | BIN (longer than INTEGER 1) |
| `0_50_50` | 7 | 1 | 0 | 0 | 0 | 7 | MAPZONE (tie at 7; MAPZONE line 53 beats IDENT line 74) |
| `0_50_50_50_50` | 13 | 1 | 0 | 0 | 13 | 7 | COORD (tie at 13; COORD line 52 beats IDENT line 74) |
| `0_50_50_50_50_50` | 16 | 1 | 0 | 0 | 13 | 7 | IDENTIFIER (length 16 > COORD 13) |
| `1_2_3_4` | 7 | 1 | 0 | 0 | 0 | 5 | IDENTIFIER (length 7 > MAPZONE 5) |
| `-5` | 0 (`-` not in id class) | 2 | 0 | 0 | 0 | 0 | INTEGER (only candidate at length 2) |
| `5 - 3` | (1, then WS+...) | 1 | — | — | — | — | INTEGER `5` (one-token); then WHITESPACE; then MINUS; then WHITESPACE; then INTEGER `3`. The `-` doesn't bond because the leading-`-`-INTEGER path requires `[0-9]` immediately after `-` (no whitespace). |

The `-5` vs `5 - 3` distinction is load-bearing — `MINUS` and the leading-`-` INTEGER prefix are ambiguous when `-` is preceded by a number. ANTLR's lexer is context-free (no parser feedback), so `-` immediately followed by a digit is **always** parsed as part of INTEGER. Consequence: `5-3` (no spaces) lexes as INTEGER `5` + INTEGER `-3` — **not** INTEGER `5` + MINUS + INTEGER `3`. The parser (NAI-204) will need a deviation tag if RuneScriptTS does context-sensitive disambiguation.

> **Plan-author pre-flight**: trace `5-3` through antlr4ng (or `bun test` a unit test) to confirm `INTEGER(5)` + `INTEGER(-3)` is what ANTLR actually emits. If it does, codify; if not, raise a deviation tag `NAI-203-D-MINUS-INTEGER-BONDING`.

#### 5.5.2 Identifier-or-keyword dispatch (letter/underscore start)

When the first char is `[a-zA-Z_]` (digit and `+`/`.`/`:` are routed elsewhere):

1. Consume the longest `[a-zA-Z0-9_+.:]+` run. Call this `text`, length `L`.
2. **Check exact-match keyword/literal table** (in .g4 declaration order):
   - `if` → IF; `else` → ELSE; `while` → WHILE; `case` → CASE; `default` → DEFAULT; `return` → RETURN; `calc` → CALC; `true`/`false` → BOOLEAN_LITERAL; `null` → NULL_LITERAL.
   - Each match is at length `L` (= full identifier run); only fires if no longer alternative exists. Since these keywords are exact-match-only (no suffix rule consumes more), an exact match wins at length `L` over IDENTIFIER at length `L` via declaration order (.g4 lines 37-56 vs IDENTIFIER line 74).
3. **Check suffix-pattern keywords** (TYPE_ARRAY line 44, DEF_TYPE line 45, SWITCH_TYPE line 46 — all before IDENTIFIER line 74):
   - DEF_TYPE: `text` starts with `def_` and has ≥1 char after → DEF_TYPE.
   - SWITCH_TYPE: `text` starts with `switch_` and has ≥1 char after → SWITCH_TYPE.
   - TYPE_ARRAY: `text` ends with `array` and has ≥1 char before → TYPE_ARRAY.
4. Else → IDENTIFIER.

**Edge cases:**

- **`default`**: keyword DEFAULT (line 41) matches length 7; DEF_TYPE (line 45) requires `def_` + non-empty tail; `default` has `default` not `def_...` (no underscore at position 3). DEFAULT wins.
- **bare `array`**: TYPE_ARRAY = `IDENTIFIER 'array'` requires ≥1 identifier-char before literal `array`. The IDENTIFIER sub-rule is greedy — it'd consume `array` itself, leaving nothing for the literal `array`. So TYPE_ARRAY does **not** match bare `array`; emit as IDENTIFIER.
- **`def_`** alone: requires non-empty tail; no DEF_TYPE; emit IDENTIFIER.
- **`true_foo`**: BOOLEAN_LITERAL would match exact `true`, but the full identifier run is `true_foo` length 8. IDENTIFIER wins (length 8 > 4). Emit IDENTIFIER `true_foo`.

### 5.6 String-mode dispatch (`lexer_string.go`)

When `currentMode() == modeString`:

1. If `l.input[l.pos] == '"'` → emit QUOTE_CLOSE, `depth--`, popMode.
2. If `l.input[l.pos] == '<'`:
   - Try STRING_P_TAG: `<p,` + at least one non-`<>` char + `>`. If match → emit.
   - Try STRING_CLOSE_TAG: `</` + Tag + `>`.
   - Try STRING_TAG: `<` + Tag + (`=` + non-`<>`+ )? + `>`.
   - Try STRING_PARTIAL_TAG: `<` + Tag + `=`.
   - Else → STRING_EXPR_START (1 char `<`), pushMode(modeDefault). **Important**: depth does **not** change; only `"` mutates depth.
3. Else → STRING_TEXT: consume longest `(StringEscapeSequence | ~('\\' | '"' | '<' | '\r' | '\n'))+`. Escape sequences: `\\`, `\"`, `\<`. Unterminated string (EOF before `"`) → emit `SyntaxError`, popMode, return EOF on next call.

Note `STRING_TEXT` does NOT include `\r` or `\n` — newlines inside a `"..."` string are an error per grammar. Treat `\n` / `\r` mid-string as end-of-STRING_TEXT + immediate SyntaxError if the string isn't closed before the newline.

**Tag fragment** (.g4 `fragment Tag`): exact-match one of `br`, `col`, `str`, `shad`, `u`, `img`, `gt`, `lt`. Implement as a small switch on the byte-after-`<` (or `</`).

### 5.7 Source-location tracking

ANTLR convention: `line` 1-based starting at 1, `column` (`charPositionInLine`) 0-based starting at 0. `NodeSourceLocation` (per TS) is 1-based on both; `ParserErrorListener.syntaxError` adds `+1` to column at the diagnostic boundary.

Goscape uses 0-based `col` **internally** in the `Lexer` struct (matches antlr's `charPositionInLine` convention to keep the implementation reading 1:1 against antlr4ng source). At token-emit time, the emitted `Token.Source` (type `NodeSourceLocation`) is **1-based on both line and column** (matches TS AST), with `endLine`/`endColumn` pointing at the **last** consumed char (inclusive — matches antlr's `Token.line` / `getCharPositionInLine` for the stop position via `Token.stop`).

The `ErrorListener.SyntaxError` callback also reports 1-based line + column (already adjusted from internal 0-based col), so listener implementers don't need to add `+1` themselves.

Line counter: advance on every `\n`. `\r\n` is two chars but only one line advance (col resets after the `\n`). Bare `\r` (no following `\n`) also advances line (matches antlr's default `\r|\n|\r\n` line-ending logic).

### 5.8 TokenStream

```go
type TokenStream struct {
    tokens []Token // pre-buffered (all channels)
    pos    int     // current raw index (any channel)
}

func NewTokenStream(l *Lexer) *TokenStream {
    var ts TokenStream
    for {
        t := l.NextToken()
        ts.tokens = append(ts.tokens, t)
        if t.Type == EOF { break }
    }
    return &ts
}

func (s *TokenStream) LT(k int) *Token {
    // 1-based; LT(1) returns next default-channel token from pos.
    // Negative k looks backward over already-consumed default-channel tokens.
    // Returns last EOF for k that overshoots.
    ...
}

func (s *TokenStream) LA(k int) TokenType { return s.LT(k).Type }
func (s *TokenStream) Consume()             // advance pos past current default-channel token
func (s *TokenStream) Index() int           // raw pos (any channel)
func (s *TokenStream) Mark() int            // returns current raw pos
func (s *TokenStream) Release(m int)        // no-op
func (s *TokenStream) Rewind(m int)         // pos = m
```

`Consume` semantics: advance `pos` past the current default-channel token, **including** any hidden-channel tokens immediately following (until the next default-channel token or EOF). This matches antlr's `BufferedTokenStream.consume` + `nextTokenOnChannel` pair.

### 5.9 Errors

```go
type ErrorListener interface {
    SyntaxError(sourceName string, line int, column int, msg string)
}

type DiscardErrorListener struct{}
func (DiscardErrorListener) SyntaxError(string, int, int, string) {}

// CollectingErrorListener for tests
type CollectingErrorListener struct{ Errors []SyntaxError }
type SyntaxError struct { SourceName string; Line, Column int; Msg string }
func (c *CollectingErrorListener) SyntaxError(...) { ... }
```

Lexer default: empty listener list (silent). Caller installs via `AddErrorListener` / `RemoveErrorListeners`. The four other antlr `ANTLRErrorListener` methods (`reportAmbiguity`, `reportAttemptingFullContext`, `reportContextSensitivity`) are LL(*)-prediction artifacts; not needed.

**Recovery contract:**

- Unrecognized character → emit `SyntaxError`, advance 1 byte, continue.
- Unterminated `"..."` (newline/EOF before closing `"`) → emit `SyntaxError`, force-pop to default mode, continue (or emit EOF if at end).
- Unterminated `/* ... */` (EOF before closing) → emit `SyntaxError`, emit BLOCK_COMMENT for the partial range on hidden channel, then EOF.
- Unterminated `'...'` (newline/EOF before closing) → emit `SyntaxError`, advance to newline-or-end, no CHAR_LITERAL token, continue.

These are **NAI-203-D-LEXER-ERROR-RECOVERY** territory — antlr4ng's default `DefaultErrorStrategy.recover` has slightly different mechanics. Tag and document.

## 6. Error handling (code-side)

- `NewLexer` accepts a `string` (not `io.Reader`) — TS uses `CharStream.fromString(source)` after `readFileSync`. Caller does the file I/O.
- All public methods are non-panicking; lexer errors flow through `ErrorListener`.
- `Token` is a value type; no pointers escape from `NextToken` (returned by value). `TokenStream` returns `*Token` for `LT(k)` so callers can compare pointer-equality if needed (matches antlr's `Token` reference semantics for diagnostics).
- Lexer is **not goroutine-safe**. One source = one lexer = one goroutine. Document on `Lexer` struct doc-comment.

## 7. Testing

Test files mirror code files. Each test name starts with `TestLex_` or `TestTokenStream_`.

### 7.1 Symbol tokens (`lexer_test.go`)

- `TestLex_Symbols_SingleChar`: table-driven over all 24 single-char symbol tokens. Input = exact char. Assert: single token of correct Type, correct Text, EOF after.
- `TestLex_Symbols_MultiChar`: `>=` → GTE; `<=` → LTE; `.%` → DOTMOD. Assert single-token emission.
- `TestLex_Symbols_LongestMatchPriority`: `>= ` → GTE WHITESPACE; `> =` → GT WHITESPACE EQ; `<<=` → LT LTE.
- `TestLex_Symbols_GtSemanticAction_OutsideString`: `>` outside any string → GT (not STRING_EXPR_END), `depth` stays 0.

### 7.2 Keyword tokens

- `TestLex_Keywords_Exact`: each of IF/ELSE/WHILE/CASE/DEFAULT/RETURN/CALC/BOOLEAN_LITERAL(true,false)/NULL_LITERAL → correct Type + Text.
- `TestLex_Keywords_PrefixIsIdentifier`: `elsex` → IDENTIFIER (text `elsex`), not ELSE + IDENTIFIER `x`.
- `TestLex_TypeArray`: `int_array` → TYPE_ARRAY; `array` alone → IDENTIFIER; `_array` → TYPE_ARRAY (`_` is identifier char, length 1 > 0).
- `TestLex_DefType`: `def_int` → DEF_TYPE; `def_` alone → IDENTIFIER (no identifier-tail).
- `TestLex_SwitchType`: `switch_int` → SWITCH_TYPE; `switch_` alone → IDENTIFIER.

### 7.3 Literal tokens

- `TestLex_IntegerLiteral`: `0`, `12345`, `-5` (single token), `9999999999` (no overflow at lex time — text-only).
- `TestLex_HexLiteral`: `0x1F`, `0X1f`, `0xDEADBEEF`. **`0x` alone (no hex digit) → IDENTIFIER `0x` length 2** (per §5.5.1 worked example: HEX fails, IDENTIFIER length 2 beats INTEGER length 1).
- `TestLex_BinLiteral`: `0b101`, `0B0`. `0b` alone → IDENTIFIER `0b` length 2 (same reasoning).
- `TestLex_CoordLiteral`: `0_50_50_50_50` → COORD (5 groups; ties IDENTIFIER at length 13, COORD wins by declaration order line 52 < line 74).
- `TestLex_MapzoneLiteral`: `1_50_50` → MAPZONE (3 groups; ties IDENTIFIER at length 7, MAPZONE wins by line 53 < line 74).
- `TestLex_FourGroupUnderscores_IDENTIFIER`: `1_2_3_4` → **IDENTIFIER** (length 7 > MAPZONE 5 — per §5.5.1 worked-example table). Pin this explicitly because intuition says "MAPZONE plus leftover" but ANTLR longest-match wins as IDENTIFIER.
- `TestLex_SixGroupUnderscores_IDENTIFIER`: `1_2_3_4_5_6` → **IDENTIFIER** (length 11 > COORD 9).
- `TestLex_DigitsThenLetters_IDENTIFIER`: `5abc` → IDENTIFIER (length 4 > INTEGER 1). Pin.
- `TestLex_CoordVsMapzonePriority_Tie`: explicit "5-group wins over 3-group truncation" test; `0_50_50_50_50` confirmed as COORD (single token), not MAPZONE + IDENT residual.
- `TestLex_BooleanLiteral`: `true`/`false` (each → BOOLEAN_LITERAL exact-match), `true_x` → IDENTIFIER (length 5).
- `TestLex_CharLiteral`: `'a'`, `'\\'` (escape `\\` → backslash char), `'\''` (escape `\'` → single quote). Unterminated `'a` → SyntaxError. `'ab'` → plan-author traces antlr4ng before pinning (see §8 open question).
- `TestLex_NullLiteral`: `null` → NULL_LITERAL; `nullx` → IDENTIFIER.

### 7.4 Hidden-channel tokens

- `TestLex_LineComment`: `// foo\n` → LINE_COMMENT (text includes the `\n` per `.g4`), channel = Hidden.
- `TestLex_LineComment_EOF`: `// foo` (no newline) → LINE_COMMENT to EOF.
- `TestLex_BlockComment`: `/* foo */` → BLOCK_COMMENT, channel = Hidden.
- `TestLex_BlockComment_MultiLine`: `/* a\nb */` → endLine > line.
- `TestLex_BlockComment_Unterminated`: `/* foo` (no `*/`) → SyntaxError + partial BLOCK_COMMENT + EOF.
- `TestLex_Whitespace`: ` \t\n\r` runs → single WHITESPACE token, channel = Hidden.

### 7.5 String-mode tokens (`lexer_modes_test.go`)

- `TestLex_PlainString`: `"hello"` → QUOTE_OPEN + STRING_TEXT(`hello`) + QUOTE_CLOSE; modes balance.
- `TestLex_StringEscapes`: `"a\\\\b\\\"c\\<d"` → STRING_TEXT covering each escape; verify raw text preserved.
- `TestLex_StringTags`: `"<br>"`, `"<col=ff0000>"`, `"</col>"`, `"<p,head>"`. Each tag = single STRING_TAG / STRING_CLOSE_TAG / STRING_P_TAG token.
- `TestLex_StringPartialTag`: `"<col="` → STRING_PARTIAL_TAG.
- `TestLex_StringInterpolation_Simple`: `"a<$x>b"` → QO TEXT(`a`) EXPR_START DOLLAR IDENT(`x`) EXPR_END TEXT(`b`) QC. Mode-stack returns to base.
- `TestLex_StringInterpolation_NestedString`: `"a<"b">c"` → depth reaches 2 inside; outer `>` retypes to STRING_EXPR_END.
- `TestLex_StringInterpolation_GtInsideExpr`: `"<calc(1 > 2)>"` — outside-string `>` would be GT, but `depth=1` so it retypes to STRING_EXPR_END. **This is the .g4 specified behavior** — note that `1 > 2` inside string interpolation is unparseable without `gte`/`<=` workarounds; document in deviation if it bites.
- `TestLex_String_Unterminated_Newline`: `"foo\n` → SyntaxError, popMode, no QUOTE_CLOSE.
- `TestLex_String_Unterminated_EOF`: `"foo` → SyntaxError + EOF.

### 7.6 Source-location tracking

- `TestLex_SourceLocation_OneLine`: `abc def` → IDENT(line=1,col=0,end=1,3) WS IDENT(line=1,col=4,end=1,7).
- `TestLex_SourceLocation_NewlineLf`: `a\nb` → first IDENT line=1, second line=2.
- `TestLex_SourceLocation_NewlineCrLf`: `a\r\nb` → same.
- `TestLex_SourceLocation_NewlineCr`: `a\rb` → second IDENT line=2 (antlr behavior).
- `TestLex_SourceLocation_TabsCountAsOne`: `\tfoo` → IDENT col=1 (antlr's `consume` advances col by 1 per char regardless of tab — matches the default `consume()` implementation).
- `TestLex_SourceLocation_BlockComment_EndPosition`: `/* a\nb */c` → BLOCK_COMMENT endLine=2,endCol=5; following IDENT line=2,col=5.

### 7.7 TokenStream (`token_stream_test.go`)

- `TestTokenStream_LT1_AfterInit`: `LT(1)` returns first default-channel token; skips leading whitespace + comments.
- `TestTokenStream_LT2_Lookahead`: `LT(2)` peeks the second default-channel token.
- `TestTokenStream_LT_BeyondEnd`: `LT(99)` past EOF returns the EOF token (not nil, not panic).
- `TestTokenStream_LTNegative`: after `Consume`, `LT(-1)` returns the just-consumed default-channel token.
- `TestTokenStream_Consume_SkipsHidden`: source `a /* c */ b`; after `Consume` past `a`, `LT(1)` returns `b` (not the comment).
- `TestTokenStream_MarkRewind_RoundTrip`: `Mark` → `Consume` × 3 → `Rewind` returns to same `LT(1)`.
- `TestTokenStream_Index`: raw index advances by 1 per token (hidden included).

### 7.8 Error listener

- `TestLex_ErrorListener_FiresOnce`: register `CollectingErrorListener`, feed `?` (unrecognized char), expect one `SyntaxError` with correct line/column.
- `TestLex_ErrorListener_DiscardListener`: `DiscardErrorListener` accepts errors silently.
- `TestLex_ErrorListener_RemoveListeners`: after `RemoveErrorListeners`, unrecognized char fires nothing.
- `TestLex_ErrorListener_MultipleListeners`: both registered listeners receive each error.

### 7.9 Golden round-trip (`golden_test.go`)

- `TestLex_Golden_NeptuneScript`: load `testdata/golden_script.src` (= Neptune `runescript-parser/.../script.src`, the `mes("TEST: <col=ffffff><escape($text)></col>");` interpolation fixture). Lex it; compare against hand-authored `testdata/golden_script.tokens` (one line per token: `<TypeName> <channel> <line>:<col>-<endLine>:<endCol> <text>`). On mismatch, dump actual + expected for diff.

A second golden test against a slice of `Content/scripts/engine.rs2` (first 30 lines) is **out of scope** for NAI-203 — it's repetitive `[command,X]` declarations covered by §7.1–7.4.

### 7.10 Deviation pin tests (`nai203_deviation_pins_test.go`)

For each `NAI-203-D-*` tag the implementer actually adds, pin its rationale comment via grep:

```go
func TestNAI203_DeviationTag_LexerErrorRecovery(t *testing.T) {
    out, err := exec.Command("rg", "-l", "NAI-203-D-LEXER-ERROR-RECOVERY", "../../../").CombinedOutput()
    require.NoError(t, err, "rg failure")
    require.Greater(t, len(strings.Split(strings.TrimSpace(string(out)), "\n")), 0)
}
```

Only landed if the deviation tag is actually used in code (matches NAI-202 deviation-pin pattern).

### 7.11 Race detector

All tests pass under `-race`. Lexer is not goroutine-safe per §6, but tests don't share Lexers across goroutines, so no failures expected.

## 8. Open questions

- **CHAR_LITERAL with multi-char body**: `'ab'` — does antlr4ng emit one error token, or CHAR_LITERAL `'a'` + IDENT `b` + error? Plan-author must trace via `cd /home/owner/Code/github.com/LostCityRS/RuneScriptTS && bun test` (or hand-trace `antlr4ng-runtime/src/Lexer.ts:nextToken`). If unclear, pick the simpler path and document under `NAI-203-D-CHAR-LIT-MULTICHAR`.
- **`5-3` (no whitespace)**: per §5.5.1 reasoning, ANTLR's context-free lexer should emit INTEGER `5` + INTEGER `-3`. Plan-author traces antlr4ng on this exact input to confirm. If divergent, raise `NAI-203-D-MINUS-INTEGER-BONDING`.
- **Tag charset in `STRING_TAG`**: the `.g4` allows `=` followed by any non-`<>` chars. Does `<col=red blue>` lex as one STRING_TAG (with space inside)? Per the `~('<' | '>')+` rule — yes. Confirm in golden fixture.

These are research items for plan-author pre-flight; not blockers for spec approval.

## 9. Resolved risks

- **No production consumer**: lexer is dead code post-NAI-203 until NAI-204 imports it. That's fine — symbol-table case (NAI-202 BuildSymbols) had no production consumer until NAI-207 will wire it. Each compiler-arc slice is independently testable.
- **Token shape evolution across slices**: pre-loaded `Source`/`Start`/`Stop`/`Index` per §5.3 avoids churn from NAI-204+. If a missed field surfaces in NAI-204, extend (don't reshape).
- **Mode-stack invariant**: only QUOTE_OPEN pushes modeString; only QUOTE_CLOSE pops it. STRING_EXPR_START pushes modeDefault; the matching `>` (when `depth > 0`) pops it. Stack underflow is unreachable on well-formed inputs; malformed inputs trigger error recovery (popMode-to-default, drop garbage).
- **Hand-port vs antlr4-go**: chosen hand-port per brainstorming. Project pattern: all hand-ports, zero ANTLR runtime dep, generated code would not match goscape's review style (cross-language side-by-side reading).
- **Source-of-truth drift**: RuneScriptTS pin at the local clone (currently at whatever HEAD is on disk). Plan-author records the commit hash in the plan doc for reproducibility.

## 10. Deviations enumerated

Anticipated `NAI-203-D-*` tags (final list determined at plan-write / impl):

- `NAI-203-D-LEXER-ERROR-RECOVERY` — hand-coded recovery (skip-1-byte, force-popMode on unterminated string) differs from antlr4ng's `DefaultErrorStrategy.recover` in detail. Documented in `lexer.go` near each recovery point.
- `NAI-203-D-CHAR-LIT-MULTICHAR` — only if §8 trace shows we diverge from antlr.
- `NAI-203-D-MINUS-INTEGER-BONDING` — only if §8 trace shows we diverge from antlr.
- `NAI-203-D-SOURCE-NAME-FROM-CONSTRUCTOR` — TS sets `stream.name = inputPath` after construction; goscape takes `sourceName` as `NewLexer` arg. Trivial divergence; tag if reviewer wants traceability.

Tags landed → pin-tested in `nai203_deviation_pins_test.go` (§7.10).

## 11. Carry-forward (from prior NAI sub-specs)

Three start-sweep cleanups land as **commit 1 (chore)** of NAI-203:

1. **`pkg/pack/compiler/typeinfo.go` forward-reference rot** (memory note [[package_doc_forward_references_rot]]): 5+ doc-comments still say "NAI-201 will port X" — the work landed in NAI-202. Rewrite to general intent ("the typed-symbol-table loader populates X") on lines 3, 5, 40, 85, 167, 184, 212 (re-grep at plan-write; line numbers may have shifted).
2. **`pkg/script/opcode_pointers.go` PointerGroupFind accessor** (memory note [[plan_grep_helper_patterns]] applies): replace `make([]string, len(pointerGroupFind))` + `copy(...)` with `slices.Clone(pointerGroupFind[:])`.
3. **`pkg/script/opcode_pointers.go` corruptExceptActive**: replace `make` + `for/append` with `slices.Concat(pointerGroupFind[:], extras)`.

No deviation tags for any of these — pure idiom upgrade. Verify NAI-201/NAI-202 tests still pass.

**Deferred to NAI-204**:

- AST Node hierarchy (`src/parser/ast/`).
- `AstBuilder` (the `antlr4ng.ParseTreeListener`-driven visitor that turns CST → AST).
- `ScriptParser.invokeParser` Go equivalent.

## 12. Arc next step

After NAI-203, NAI-204 begins the parser port:

- Hand-write recursive-descent parser against `RuneScriptParser.g4` (228 lines).
- Define AST nodes ports under `pkg/pack/compiler/ast/` mirroring `RuneScriptTS/src/parser/ast/`.
- Build the AST inline (no separate `AstBuilder` indirection — antlr4 needs the listener pattern; recursive-descent can build directly).
- Consume `lexer.TokenStream` via `LT(k)` / `Consume`.

NAI-204's spec-write should **re-grep `pkg/pack/compiler/lexer.TokenStream`** to confirm the API shape hasn't drifted from this slice. Per [[spec_followup_tracker_freshness]] memory: re-verify every assertion at spec-write.

## 13. Acceptance criteria

- `pkg/pack/compiler/lexer/` package exists with all files in §2 In-scope list.
- `go test ./pkg/pack/compiler/lexer/...` passes under `-race` with all tests from §7.1–7.11.
- `testdata/golden_script.src` and `testdata/golden_script.tokens` exist; golden test passes.
- Three start-sweep cleanups landed in their own commit; NAI-201 + NAI-202 deviation-pin tests still green; `go test ./...` clean.
- Each `NAI-203-D-*` deviation tag landed has ≥1 grep hit in `pkg/pack/compiler/lexer/`.
- Code reviewer can read `lexer.go` + `lexer_default.go` + `lexer_string.go` alongside `RuneScriptLexer.g4` (105 lines) and verify rule-by-rule parity in one sitting.
- No imports of antlr4-go or any ANTLR runtime; `go mod tidy` produces no new dependencies.
