package lexer

// Lexer scans RuneScript source bytes into a token stream. Mirrors
// antlr4ng's Lexer surface enough that the NAI-204 parser can plug
// in CommonTokenStream-equivalent consumption.
//
// One source = one Lexer = one goroutine. Lexer is NOT goroutine-safe.
// Callers that need parallel lex (e.g. compile farm) construct one
// Lexer per source goroutine.
type Lexer struct {
	input      string
	sourceName string

	pos  int // current byte offset (next unread)
	line int // 1-based line of pos
	col  int // 0-based column of pos (antlr convention)

	depth int // string-interpolation nesting (.g4 @members)
	modes []mode

	listeners  []ErrorListener
	tokenIndex int // 0-based; next-emitted token's Index
}

// mode is the lexer-mode enum. The stack is owned by Lexer and starts
// with modeDefault at the bottom.
type mode int

const (
	modeDefault mode = iota
	modeString
)

// NewLexer constructs a Lexer for the given source string. sourceName
// is attached to every emitted Token's Source.Name (and to
// ErrorListener.SyntaxError calls) — typically the file path. Pass
// "<source>" for in-memory inputs to mirror TS ScriptParser convention.
func NewLexer(input, sourceName string) *Lexer {
	return &Lexer{
		input:      input,
		sourceName: sourceName,
		line:       1,
		col:        0,
		modes:      []mode{modeDefault},
	}
}

// AddErrorListener registers l to receive SyntaxError callbacks. Order
// of registration is preserved across multiple listeners.
func (lx *Lexer) AddErrorListener(l ErrorListener) {
	lx.listeners = append(lx.listeners, l)
}

// RemoveErrorListeners drops all registered listeners. Mirrors TS
// `lexer.removeErrorListeners()` pattern from ScriptParser.invokeParser.
func (lx *Lexer) RemoveErrorListeners() {
	lx.listeners = nil
}

// currentMode returns the top of the mode stack.
func (lx *Lexer) currentMode() mode {
	return lx.modes[len(lx.modes)-1]
}

// pushMode pushes m on top of the mode stack.
func (lx *Lexer) pushMode(m mode) {
	lx.modes = append(lx.modes, m)
}

// popMode pops the top mode. The bottom modeDefault is never popped on
// well-formed inputs; error recovery may force-pop on unterminated
// strings (see NAI-203-D-LEXER-ERROR-RECOVERY).
func (lx *Lexer) popMode() {
	lx.modes = lx.modes[:len(lx.modes)-1]
}

// reportError fires SyntaxError on every registered listener. line and
// column are 1-based (callers convert from internal 0-based col).
func (lx *Lexer) reportError(line, column int, msg string) {
	for _, l := range lx.listeners {
		l.SyntaxError(lx.sourceName, line, column, msg)
	}
}

// NextToken returns the next token. At EOF, returns EOF repeatedly —
// callers don't need to test pos themselves.
//
// Dispatch chooses between modeDefault (lexer_default.go:nextDefault)
// and modeString (lexer_string.go:nextString) based on the mode stack
// top. The two dispatch functions are responsible for advancing pos
// and (where applicable) line/col.
//
// NAI-203-D-LEXER-ERROR-RECOVERY: EOF inside modeString fires
// SyntaxError once and resets the mode stack to modeDefault before
// emitting EOF.
func (lx *Lexer) NextToken() Token {
	if lx.pos >= len(lx.input) {
		if lx.currentMode() == modeString {
			// NAI-203-D-LEXER-ERROR-RECOVERY: EOF inside string. Fire
			// SyntaxError once, drop mode stack to modeDefault.
			lx.reportError(lx.line, lx.col+1, "unterminated string literal at EOF")
			lx.modes = []mode{modeDefault}
			lx.depth = 0
		}
		return lx.makeToken(EOF, lx.pos, lx.pos-1, "", lx.line, lx.col+1, lx.line, lx.col+1)
	}
	switch lx.currentMode() {
	case modeDefault:
		return lx.nextDefault()
	case modeString:
		return lx.nextString()
	default:
		panic("lexer: unknown mode")
	}
}

// makeToken constructs a Token with all bookkeeping fields filled and
// advances tokenIndex. start/stop are byte offsets into input; startLn,
// startCol are 1-based start position; endLn/endCol are 1-based end
// (inclusive — pointing at the LAST consumed character). For zero-text
// tokens (EOF), stop = start - 1.
//
// Internal callers convert from 0-based col (lexer state) to 1-based
// before calling makeToken.
func (lx *Lexer) makeToken(tt TokenType, start, stop int, text string, startLn, startCol, endLn, endCol int) Token {
	tok := Token{
		Type:    tt,
		Channel: ChannelDefault,
		Text:    text,
		Start:   start,
		Stop:    stop,
		Index:   lx.tokenIndex,
		Source: NodeSourceLocation{
			Name:      lx.sourceName,
			Line:      startLn,
			Column:    startCol,
			EndLine:   endLn,
			EndColumn: endCol,
		},
	}
	lx.tokenIndex++
	return tok
}

