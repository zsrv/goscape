// Package diagnostics provides the diagnostic-reporting surface for the
// RuneScript compiler: types, the accumulator, the handler interface, and
// message-template constants.
//
// Ported from TS src/compiler/diagnostics/ at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
package diagnostics

// DiagnosticType discriminates between info/hint/warning/error severities.
// Mirrors TS DiagnosticType.ts (string enum); goscape uses int + String().
type DiagnosticType int

const (
	DiagnosticInfo DiagnosticType = iota
	DiagnosticHint
	DiagnosticWarning
	DiagnosticError
	DiagnosticSyntaxError
)

func (t DiagnosticType) String() string {
	switch t {
	case DiagnosticInfo:
		return "INFO"
	case DiagnosticHint:
		return "HINT"
	case DiagnosticWarning:
		return "WARNING"
	case DiagnosticError:
		return "ERROR"
	case DiagnosticSyntaxError:
		return "SYNTAX_ERROR"
	}
	return "UNKNOWN"
}
