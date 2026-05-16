package writer

import "github.com/zsrv/goscape/pkg/pack/compiler/symbol"

// IdProvider maps a compiler-side symbol.Symbol to its runtime numeric ID.
// Concrete impl lives in pkg/pack/compiler/runescript/symbol_mapper.go.
//
// Returns -1 for missing script/command symbols (mirrors TS SymbolMapper
// behavior — TS reports a diagnostic and returns -1, never throws for
// those). For missing basic symbols TS throws; the concrete impl in goscape
// panics for parity.
//
// Mirrors TS src/compiler/writer/BaseScriptWriter.ts L304-313 (IdProvider).
type IdProvider interface {
	Get(s symbol.Symbol) int
}
