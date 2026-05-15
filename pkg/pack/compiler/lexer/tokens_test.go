package lexer

import "testing"

// TestTokenType_String exercises the parallel-indexed tokenName table.
// Pins that every TokenType constant has a corresponding String() entry
// and the entries are in iota order.
func TestTokenType_String(t *testing.T) {
	cases := []struct {
		tt   TokenType
		want string
	}{
		{EOF, "EOF"},
		{LPAREN, "LPAREN"},
		{STRING_EXPR_END, "STRING_EXPR_END"},
		{IDENTIFIER, "IDENTIFIER"},
	}
	for _, c := range cases {
		if got := c.tt.String(); got != c.want {
			t.Errorf("TokenType(%d).String() = %q, want %q", c.tt, got, c.want)
		}
	}
}

// TestTokenType_Count pins the total catalog size: EOF (sentinel) + 58
// types from RuneScriptLexer.g4 = 59 entries.
func TestTokenType_Count(t *testing.T) {
	if got, want := len(tokenName), 59; got != want {
		t.Errorf("len(tokenName) = %d, want %d (EOF + 58 from .g4)", got, want)
	}
}

// TestTokenType_DeclarationOrder pins the load-bearing invariant from
// §5.5.1/§5.5.2 of the spec: TokenType values follow .g4 declaration
// order so disambiguation by declaration order is TokenType comparison.
// Critical: INTEGER_LITERAL must be < HEX_LITERAL < BIN_LITERAL <
// COORD_LITERAL < MAPZONE_LITERAL < BOOLEAN_LITERAL etc., and all
// keyword/literal types must be < IDENTIFIER.
func TestTokenType_DeclarationOrder(t *testing.T) {
	if !(INTEGER_LITERAL < HEX_LITERAL) {
		t.Errorf("INTEGER_LITERAL(%d) must be < HEX_LITERAL(%d)", INTEGER_LITERAL, HEX_LITERAL)
	}
	if !(HEX_LITERAL < BIN_LITERAL && BIN_LITERAL < COORD_LITERAL && COORD_LITERAL < MAPZONE_LITERAL) {
		t.Error("numeric literal declaration order broken")
	}
	if !(BOOLEAN_LITERAL < IDENTIFIER && NULL_LITERAL < IDENTIFIER) {
		t.Error("BOOLEAN_LITERAL / NULL_LITERAL must precede IDENTIFIER")
	}
	if !(IF < IDENTIFIER && DEF_TYPE < IDENTIFIER) {
		t.Error("keywords must precede IDENTIFIER")
	}
}

// TestChannel pins the channel constants — ChannelDefault = 0,
// ChannelHidden = 1, matching antlr4ng.
func TestChannel(t *testing.T) {
	if ChannelDefault != 0 {
		t.Errorf("ChannelDefault = %d, want 0", ChannelDefault)
	}
	if ChannelHidden != 1 {
		t.Errorf("ChannelHidden = %d, want 1", ChannelHidden)
	}
}

// TestNodeSourceLocation_Fields pins NodeSourceLocation shape parity
// with RuneScriptTS src/parser/ast/NodeSourceLocation.ts.
func TestNodeSourceLocation_Fields(t *testing.T) {
	loc := NodeSourceLocation{
		Name:      "x.rs2",
		Line:      1,
		Column:    1,
		EndLine:   1,
		EndColumn: 3,
	}
	if loc.Name != "x.rs2" || loc.Line != 1 || loc.Column != 1 || loc.EndLine != 1 || loc.EndColumn != 3 {
		t.Errorf("NodeSourceLocation literal failed: %+v", loc)
	}
}
