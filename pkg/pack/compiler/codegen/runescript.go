package codegen

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// LocalTable carries every LocalVariableSymbol declared in a RuneScript.
// Parameters is a subset of All. Mirrors TS LocalTable (RuneScript.ts).
type LocalTable struct {
	Parameters []*symbol.LocalVariableSymbol
	All        []*symbol.LocalVariableSymbol
}

// RuneScript is the codegen output for one Script — the abstract-Opcode
// instruction stream organised into labelled Blocks, plus SwitchTables and
// the LocalTable. Mirrors TS RuneScript (RuneScript.ts).
type RuneScript struct {
	SourceName       string
	Symbol           symbol.Symbol
	SubjectReference ast.SymbolRef // nil-able
	Trigger          *trigger.TriggerType
	Name             string
	FullName         string // [triggerIdent,name]
	Locals           *LocalTable
	Blocks           []*Block
	SwitchTables     []*SwitchTable
}

// NewRuneScript constructs a RuneScript. Callers pass the script symbol plus
// its trigger and name explicitly (since symbol.Symbol marker interface does
// not expose Trigger/Name — those live on the concrete ScriptSymbolFields
// embedding). subjectReference may be nil.
func NewRuneScript(
	sourceName string,
	sym symbol.Symbol,
	tr *trigger.TriggerType,
	name string,
	subjectReference ast.SymbolRef,
) *RuneScript {
	return &RuneScript{
		SourceName:       sourceName,
		Symbol:           sym,
		SubjectReference: subjectReference,
		Trigger:          tr,
		Name:             name,
		FullName:         "[" + tr.Identifier + "," + name + "]",
		Locals:           &LocalTable{},
	}
}

// GenerateSwitchTable returns a fresh SwitchTable with ID = len(SwitchTables)
// and appends it to SwitchTables.
func (r *RuneScript) GenerateSwitchTable() *SwitchTable {
	st := NewSwitchTable(len(r.SwitchTables))
	r.SwitchTables = append(r.SwitchTables, st)
	return st
}
