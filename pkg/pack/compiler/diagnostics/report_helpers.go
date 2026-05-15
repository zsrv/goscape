package diagnostics

import "github.com/zsrv/goscape/pkg/pack/compiler/ast"

// NAI-205-D-NO-NODE-REPORT-ERROR: TS adds a reportError method to every Node
// instance directly. Goscape avoids the ast → diagnostics import cycle by
// routing through these helpers in the diagnostics package, which accept an
// ast.Node and pull its NodeSourceLocation via the Node.Source() method.
// Consumers call ReportAt / ReportErrorAt instead of node.reportError().

// ReportAt appends a Diagnostic with the given severity, using node.Source()
// for the source location. Mirrors TS Node.reportError + type-routing layer.
func ReportAt(d *Diagnostics, node ast.Node, t DiagnosticType, msg string, args ...any) {
	d.Report(NewDiagnostic(node.Source(), t, msg, args...))
}

// ReportErrorAt is shorthand for ReportAt with DiagnosticError. Most TS
// node.reportError(msg, args...) call sites map to this helper.
func ReportErrorAt(d *Diagnostics, node ast.Node, msg string, args ...any) {
	ReportAt(d, node, DiagnosticError, msg, args...)
}
