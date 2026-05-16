// pkg/pack/compiler/diagnostics/parser_error_listener.go
package diagnostics

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// ParserErrorListener adapts lexer.ErrorListener to *Diagnostics. One
// SyntaxError callback produces one Diagnostic of type
// DiagnosticSyntaxError. The constructor's sourceName overrides whatever
// path the callback passes — mirrors TS ParserErrorListener
// (RuneScriptTS/src/compiler/parser/ParserErrorListener.ts) which
// captures the file at construction time.
//
// Implements lexer.ErrorListener structurally.
type ParserErrorListener struct {
	SourceName string
	Diag       *Diagnostics
}

// NewParserErrorListener constructs an adapter bound to sourceName + d.
func NewParserErrorListener(sourceName string, d *Diagnostics) *ParserErrorListener {
	return &ParserErrorListener{SourceName: sourceName, Diag: d}
}

// SyntaxError pushes one DiagnosticSyntaxError into Diag. The callback's
// sourceName arg is ignored — Diag uses the constructor's SourceName.
func (p *ParserErrorListener) SyntaxError(_ string, line, column int, msg string) {
	p.Diag.Report(Diagnostic{
		Type:           DiagnosticSyntaxError,
		SourceLocation: lexer.NodeSourceLocation{Name: p.SourceName, Line: line, Column: column},
		Message:        "%s",
		MessageArgs:    []any{msg},
	})
}
