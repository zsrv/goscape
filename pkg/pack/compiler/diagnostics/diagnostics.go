package diagnostics

// Diagnostics is the accumulator for one compilation step.
// Mirrors TS src/compiler/diagnostics/Diagnostics.ts.
type Diagnostics struct {
	entries []Diagnostic
}

// Report appends a diagnostic to the list.
// Mirrors TS Diagnostics.report().
func (d *Diagnostics) Report(diag Diagnostic) {
	d.entries = append(d.entries, diag)
}

// List returns the accumulated diagnostics (no defensive copy).
// Mirrors TS getter Diagnostics.diagnostics.
func (d *Diagnostics) List() []Diagnostic {
	return d.entries
}

// HasErrors reports whether any reported diagnostic is an Error or SyntaxError.
// Mirrors TS Diagnostics.hasErrors() L33-38.
func (d *Diagnostics) HasErrors() bool {
	for _, e := range d.entries {
		if e.IsError() {
			return true
		}
	}
	return false
}
