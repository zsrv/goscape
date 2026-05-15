package diagnostics

// Handler hooks into each compilation step's diagnostic stream.
// Mirrors the DiagnosticsHandler interface in
// TS src/compiler/diagnostics/DiagnosticsHandler.ts.
//
// NAI-205-D-HANDLER-REQUIRED-METHODS: TS optional methods (?:) collapse to
// goscape's interface-with-NopHandler. Every Handler must implement all four
// methods; NopHandler eats them silently. BaseDiagnosticsHandler (file-reading
// + stdout + process.exit) is deferred to NAI-208 driver.
type Handler interface {
	HandleParse(*Diagnostics)
	HandleTypeChecking(*Diagnostics)
	HandleCodeGeneration(*Diagnostics)
	HandlePointerChecking(*Diagnostics)
}

// NopHandler implements Handler with all four methods as no-ops. Test
// callers inject this when they don't care about diagnostic dispatch.
type NopHandler struct{}

func (NopHandler) HandleParse(*Diagnostics)           {}
func (NopHandler) HandleTypeChecking(*Diagnostics)    {}
func (NopHandler) HandleCodeGeneration(*Diagnostics)  {}
func (NopHandler) HandlePointerChecking(*Diagnostics) {}
