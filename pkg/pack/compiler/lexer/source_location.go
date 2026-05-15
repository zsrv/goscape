package lexer

// NodeSourceLocation mirrors RuneScriptTS
// src/parser/ast/NodeSourceLocation.ts. All line/column values are
// 1-based at the API boundary (matches the TS AST). Lexer internals
// use 0-based column (antlr's charPositionInLine convention); the
// conversion happens at token-emit time so listener implementers and
// downstream consumers see 1-based throughout.
type NodeSourceLocation struct {
	Name      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
}
