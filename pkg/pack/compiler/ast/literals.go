package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// IntegerLiteral covers TS IntegerLiteral, HEX_LITERAL, BIN_LITERAL
// (lexer disambiguates the source-text shape, but AstBuilder collapses
// them into one IntegerLiteral with a parsed numeric Value — parity
// with AstBuilder.visitIntegerLiteral lines 296-303).
type IntegerLiteral struct {
	SrcLoc lexer.NodeSourceLocation
	Value  int32
}

func (l *IntegerLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *IntegerLiteral) Kind() NodeKind                   { return KindIntegerLiteral }
func (l *IntegerLiteral) Children() []Node                 { return nil }
func (l *IntegerLiteral) isNode()                          {}
func (l *IntegerLiteral) isExpression()                    {}

// CoordLiteral is the packed coord value computed from `N_N_N_N_N`
// source per AstBuilder.visitCoordLiteral lines 306-316:
//
//	parts = text.split('_').map(int)
//	x = (parts[1] << 6) | (parts[3] & 0x3fff)
//	z = (parts[2] << 6) | (parts[4] & 0x3fff)
//	y = parts[0] & 0x3
//	value = z | (x << 14) | (y << 28)
type CoordLiteral struct {
	SrcLoc lexer.NodeSourceLocation
	Value  int32
}

func (l *CoordLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *CoordLiteral) Kind() NodeKind                   { return KindCoordLiteral }
func (l *CoordLiteral) Children() []Node                 { return nil }
func (l *CoordLiteral) isNode()                          {}
func (l *CoordLiteral) isExpression()                    {}

// BooleanLiteral is `true` | `false`. Mirrors TS BooleanLiteral.
type BooleanLiteral struct {
	SrcLoc lexer.NodeSourceLocation
	Value  bool
}

func (l *BooleanLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *BooleanLiteral) Kind() NodeKind                   { return KindBooleanLiteral }
func (l *BooleanLiteral) Children() []Node                 { return nil }
func (l *BooleanLiteral) isNode()                          {}
func (l *BooleanLiteral) isExpression()                    {}

// CharacterLiteral is `'x'` (single character after unescape). Mirrors
// TS CharacterLiteral. Parser-side AstBuilder.visitCharacterLiteral
// validates len==1; in goscape that validation happens at parse time.
type CharacterLiteral struct {
	SrcLoc lexer.NodeSourceLocation
	Value  string // exactly 1 unicode codepoint when valid
}

func (l *CharacterLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *CharacterLiteral) Kind() NodeKind                   { return KindCharacterLiteral }
func (l *CharacterLiteral) Children() []Node                 { return nil }
func (l *CharacterLiteral) isNode()                          {}
func (l *CharacterLiteral) isExpression()                    {}

// StringLiteral is a quote-delimited string containing only STRING_TEXT
// parts (no tags, no interpolation). Strings with tags / interpolation
// parse to JoinedStringExpression. Mirrors TS StringLiteral.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS .subExpression is NAI-206-owned.
type StringLiteral struct {
	SrcLoc lexer.NodeSourceLocation
	Value  string // unescaped
}

func (l *StringLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *StringLiteral) Kind() NodeKind                   { return KindStringLiteral }
func (l *StringLiteral) Children() []Node                 { return nil }
func (l *StringLiteral) isNode()                          {}
func (l *StringLiteral) isExpression()                    {}

// NullLiteral is `null` — has the constant numeric value -1 (parity
// with TS NullLiteral which extends Literal<number> with value=-1).
type NullLiteral struct {
	SrcLoc lexer.NodeSourceLocation
}

func (l *NullLiteral) Source() lexer.NodeSourceLocation { return l.SrcLoc }
func (l *NullLiteral) Kind() NodeKind                   { return KindNullLiteral }
func (l *NullLiteral) Children() []Node                 { return nil }
func (l *NullLiteral) isNode()                          {}
func (l *NullLiteral) isExpression()                    {}

// Value returns the constant -1, parity with TS NullLiteral(value=-1).
func (l *NullLiteral) Value() int32 { return -1 }
