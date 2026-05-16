// pkg/pack/compiler/runescript/symbol_mapper.go
package runescript

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// SymbolMapper maps compiler symbols to their numeric runtime IDs. Implements
// writer.IdProvider. Mirrors TS src/runescript/SymbolMapper.ts.
//
// Three internal tables:
//   - commands: indexed by stripped command name (".foo" → "foo")
//   - scripts:  indexed by full TS-style key "[trigger,name]"
//   - symbols:  any other symbol.Symbol (Basic/Local/Constant)
//
// NAI-209-D-SYMMAPPER-DIAG-CTOR: TS reads (symbol as any).context?.diagnostics
// on the fly; goscape symbols carry no context field, so diagnostics is
// constructor-injected. Diagnostics may be nil for tests that don't need to
// observe duplicate/missing reports.
type SymbolMapper struct {
	diags    *diagnostics.Diagnostics
	commands map[string]int
	scripts  map[string]int
	symbols  map[symbol.Symbol]int
}

// NewSymbolMapper returns a fresh SymbolMapper. diags may be nil.
func NewSymbolMapper(diags *diagnostics.Diagnostics) *SymbolMapper {
	return &SymbolMapper{
		diags:    diags,
		commands: map[string]int{},
		scripts:  map[string]int{},
		symbols:  map[symbol.Symbol]int{},
	}
}

// Compile-time assertion that SymbolMapper satisfies writer.IdProvider.
var _ writer.IdProvider = (*SymbolMapper)(nil)

// PutSymbol assigns id to s. If s is already mapped, reports a duplicate
// diagnostic (when diags is non-nil) and leaves the first mapping intact.
// Mirrors TS SymbolMapper.putSymbol L32-40.
func (m *SymbolMapper) PutSymbol(id int, s symbol.Symbol) {
	if _, dup := m.symbols[s]; dup {
		m.report(fmt.Sprintf("Duplicate symbol: %s.", s.SymbolName()))
		return
	}
	m.symbols[s] = id
}

// PutCommand maps name → id. Duplicate names are silently ignored
// (TS has no diagnostics-context for the bare-name path: SymbolMapper.ts L42-48).
func (m *SymbolMapper) PutCommand(id int, name string) {
	if _, dup := m.commands[name]; dup {
		return
	}
	m.commands[name] = id
}

// PutScript maps a "[ident,name]"-shaped key → id. Duplicates silently ignored
// (TS L50-56).
func (m *SymbolMapper) PutScript(id int, name string) {
	if _, dup := m.scripts[name]; dup {
		return
	}
	m.scripts[name] = id
}

// Get returns the runtime ID for s. For script symbols, branches on
// Trigger == CommandTrigger to choose between the commands and scripts
// tables. Returns -1 for missing script/command symbols (TS reports and
// returns -1). Panics for missing basic/local symbols (TS throws).
// Mirrors TS SymbolMapper.get L58-89.
func (m *SymbolMapper) Get(s symbol.Symbol) int {
	switch ss := s.(type) {
	case *symbol.ServerScriptSymbol:
		return m.getScript(ss.Trigger, ss.Name, s.SymbolName())
	case *symbol.ClientScriptSymbol:
		return m.getScript(ss.Trigger, ss.Name, s.SymbolName())
	}
	id, ok := m.symbols[s]
	if !ok {
		panic(fmt.Sprintf("SymbolMapper: unable to find id for %q.", s.SymbolName()))
	}
	return id
}

func (m *SymbolMapper) getScript(t *trigger.TriggerType, name, repr string) int {
	if t == trigger.CommandTrigger {
		// Trim everything up to and including the first dot (TS substring).
		key := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			key = name[i+1:]
		}
		id, ok := m.commands[key]
		if !ok {
			m.report(fmt.Sprintf("Unable to find id for '%s'.", repr))
			return -1
		}
		return id
	}
	key := "[" + t.Identifier + "," + name + "]"
	id, ok := m.scripts[key]
	if !ok {
		m.report(fmt.Sprintf("Unable to find id for '%s'.", repr))
		return -1
	}
	return id
}

func (m *SymbolMapper) report(msg string) {
	if m.diags == nil {
		return
	}
	m.diags.Report(diagnostics.NewDiagnostic(
		lexer.NodeSourceLocation{},
		diagnostics.DiagnosticError,
		diagnostics.MessageGenericUnresolvedSymbol,
		msg,
	))
}
