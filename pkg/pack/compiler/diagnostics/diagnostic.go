package diagnostics

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// Diagnostic is one entry reported during a compilation step.
// Mirrors TS src/compiler/diagnostics/Diagnostic.ts.
type Diagnostic struct {
	Type           DiagnosticType
	SourceLocation lexer.NodeSourceLocation
	Message        string
	MessageArgs    []any
}

// NewDiagnostic constructs a Diagnostic from a known source location.
// Mirrors the NodeSourceLocation-based TS Diagnostic constructor overload.
func NewDiagnostic(loc lexer.NodeSourceLocation, t DiagnosticType, msg string, args ...any) Diagnostic {
	return Diagnostic{
		Type:           t,
		SourceLocation: loc,
		Message:        msg,
		MessageArgs:    args,
	}
}

// IsError reports whether this diagnostic is an Error or SyntaxError.
// Mirrors TS Diagnostic.isError().
func (d Diagnostic) IsError() bool {
	return d.Type == DiagnosticError || d.Type == DiagnosticSyntaxError
}
