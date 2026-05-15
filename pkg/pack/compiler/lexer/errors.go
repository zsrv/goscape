package lexer

// ErrorListener receives lexer (and, in NAI-204, parser) syntax errors.
// Callbacks are invoked with 1-based line and column. Implementations
// MUST be non-blocking — the lexer calls SyntaxError from inside
// NextToken's dispatch loop.
//
// The four other antlr4ng ANTLRErrorListener methods (reportAmbiguity,
// reportAttemptingFullContext, reportContextSensitivity,
// reportConflict) are LL(*)-prediction artifacts and not modeled here —
// the NAI-204 parser is hand-written recursive-descent.
type ErrorListener interface {
	SyntaxError(sourceName string, line, column int, msg string)
}

// DiscardErrorListener implements ErrorListener as a no-op. Mirrors TS
// TypeChecking.DISCARD_ERROR_LISTENER — used when callers want silent
// best-effort lexing.
type DiscardErrorListener struct{}

func (DiscardErrorListener) SyntaxError(string, int, int, string) {}

// SyntaxError is one recorded lexer error — used by
// CollectingErrorListener for tests and any caller that needs a flat
// list of diagnostics.
type SyntaxError struct {
	SourceName string
	Line       int
	Column     int
	Msg        string
}

// CollectingErrorListener records every SyntaxError into a slice in the
// order received. Test-helper; production callers should plug their own
// listener (e.g. a Diagnostics sink in NAI-204+).
type CollectingErrorListener struct {
	Errors []SyntaxError
}

func (c *CollectingErrorListener) SyntaxError(sourceName string, line, column int, msg string) {
	c.Errors = append(c.Errors, SyntaxError{
		SourceName: sourceName,
		Line:       line,
		Column:     column,
		Msg:        msg,
	})
}
