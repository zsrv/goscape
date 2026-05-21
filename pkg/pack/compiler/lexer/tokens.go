// Package lexer ports @lostcityrs/runescript's lexer
// (src/antlr/RuneScriptLexer.g4, RuneScriptTS HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47) to idiomatic Go.
//
// The grammar declares 58 token types across two lexer modes
// (DEFAULT_MODE and String). This package emits all of them with
// channel + source-location parity to antlr4ng's CommonTokenStream
// output so the NAI-204 parser slice can consume tokens via LT(k) /
// Consume in the same shape RuneScriptTS's parser sees.
//
// One source = one Lexer = one goroutine. Lexer is not goroutine-safe.
package lexer

// TokenType identifies a RuneScript lexer token kind. Values are
// assigned in .g4 declaration order so longest-match tie-breaking by
// declaration order reduces to TokenType comparison.
type TokenType int

// Token-type constants in RuneScriptLexer.g4 declaration order.
// EOF is a sentinel emitted when the lexer reaches end-of-input.
const (
	EOF TokenType = iota

	// Symbols (RuneScriptLexer.g4:8-34)
	LPAREN    // (
	RPAREN    // )
	COLON     // :
	SEMICOLON // ;
	COMMA     // ,
	LBRACK    // [
	RBRACK    // ]
	LBRACE    // {
	RBRACE    // }
	PLUS      // +
	MINUS     // -
	MUL       // *
	DIV       // /
	DOTMOD    // .%
	MOD       // %
	AND       // &
	OR        // |
	EQ        // =
	EXCL      // !
	DOLLAR    // $
	CARET     // ^
	TILDE     // ~
	AT        // @
	GT        // >
	GTE       // >=
	LT        // <
	LTE       // <=

	// Keywords (RuneScriptLexer.g4:37-46)
	IF
	ELSE
	WHILE
	CASE
	DEFAULT
	RETURN
	CALC
	TYPE_ARRAY  // IDENTIFIER 'array'
	DEF_TYPE    // 'def_' IDENTIFIER
	SWITCH_TYPE // 'switch_' IDENTIFIER

	// Literals (RuneScriptLexer.g4:49-56)
	INTEGER_LITERAL // -? Digit+
	HEX_LITERAL     // 0[xX] [0-9a-fA-F]+
	BIN_LITERAL     // 0[bB] [01]+
	COORD_LITERAL   // 5-group N_N_N_N_N
	MAPZONE_LITERAL // 3-group N_N_N
	BOOLEAN_LITERAL // true | false
	CHAR_LITERAL    // 'X' with escape
	NULL_LITERAL    // null

	// Comments (RuneScriptLexer.g4:59-60) — hidden channel
	LINE_COMMENT
	BLOCK_COMMENT

	// Special — DEFAULT mode (RuneScriptLexer.g4:73-75)
	QUOTE_OPEN // " (pushes String mode, depth++)
	IDENTIFIER // [a-zA-Z0-9_+.:]+
	WHITESPACE // [ \t\n\r]+ — hidden channel

	// String mode (RuneScriptLexer.g4:80-87)
	QUOTE_CLOSE        // " (pops String mode, depth--)
	STRING_TEXT        // text run (with \\ \" \< escapes)
	STRING_TAG         // <br>, <col=red>, etc.
	STRING_CLOSE_TAG   // </col>
	STRING_PARTIAL_TAG // <col=
	STRING_P_TAG       // <p,head>
	STRING_EXPR_START  // < (pushes DEFAULT mode for interpolation)
	STRING_EXPR_END    // > inside interpolation (retyped GT, pops mode)
)

// tokenName is a parallel-indexed lookup for (TokenType).String().
// Index must match the iota order above exactly. Length: 59.
var tokenName = [...]string{
	"EOF",
	"LPAREN", "RPAREN", "COLON", "SEMICOLON", "COMMA",
	"LBRACK", "RBRACK", "LBRACE", "RBRACE",
	"PLUS", "MINUS", "MUL", "DIV", "DOTMOD", "MOD",
	"AND", "OR", "EQ", "EXCL", "DOLLAR", "CARET", "TILDE", "AT",
	"GT", "GTE", "LT", "LTE",
	"IF", "ELSE", "WHILE", "CASE", "DEFAULT", "RETURN", "CALC",
	"TYPE_ARRAY", "DEF_TYPE", "SWITCH_TYPE",
	"INTEGER_LITERAL", "HEX_LITERAL", "BIN_LITERAL",
	"COORD_LITERAL", "MAPZONE_LITERAL",
	"BOOLEAN_LITERAL", "CHAR_LITERAL", "NULL_LITERAL",
	"LINE_COMMENT", "BLOCK_COMMENT",
	"QUOTE_OPEN", "IDENTIFIER", "WHITESPACE",
	"QUOTE_CLOSE", "STRING_TEXT", "STRING_TAG", "STRING_CLOSE_TAG",
	"STRING_PARTIAL_TAG", "STRING_P_TAG", "STRING_EXPR_START", "STRING_EXPR_END",
}

// String returns the symbolic name of the token type (e.g. "LPAREN").
// Falls back to a numeric form for out-of-range values.
func (t TokenType) String() string {
	if int(t) < 0 || int(t) >= len(tokenName) {
		return "?"
	}
	return tokenName[t]
}

// Channel constants mirror antlr4ng: default channel = 0 (parser
// consumes), hidden = 1 (whitespace, comments).
const (
	ChannelDefault = 0
	ChannelHidden  = 1
)

// Token is the unit emitted by Lexer.NextToken. Fields are sized
// generously vs the TS Token wrapper (src/parser/ast/Token.ts only
// has {text}) so the NAI-204 parser + NAI-205 type-checker can read
// antlr-style start/stop/index without a Token reshape.
type Token struct {
	Type    TokenType
	Channel int
	Text    string
	Source  NodeSourceLocation
	Start   int // byte offset into input (inclusive)
	Stop    int // byte offset into input (inclusive); -1 for empty
	Index   int // 0-based position in the lexer's emit sequence
}
