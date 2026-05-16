// pkg/pack/compiler/writer/base_context.go
package writer

import "github.com/zsrv/goscape/pkg/pack/compiler/codegen"

// BaseContext is the shared per-script writer context: tracks the current
// instruction pc plus the precomputed line-number and jump tables.
// Concrete writer contexts (binary/text) embed *BaseContext.
//
// Mirrors TS BaseScriptWriterContext (BaseScriptWriter.ts L289-299).
//
// NAI-209-D-LINENUMBER-ORDER-SLICE: LineNumberPCs holds the insertion-order
// pcs from the LineNumberTable so that Finish() can iterate deterministically.
type BaseContext struct {
	Script          *codegen.RuneScript
	CurIndex        int
	LineNumberTable map[int]int
	LineNumberPCs   []int
	JumpTable       map[*codegen.Label]int
}

// NewBaseContext populates both tables eagerly via GenerateLineNumberTable
// and GenerateJumpTable.
func NewBaseContext(script *codegen.RuneScript) *BaseContext {
	tbl, order := GenerateLineNumberTable(script)
	return &BaseContext{
		Script:          script,
		LineNumberTable: tbl,
		LineNumberPCs:   order,
		JumpTable:       GenerateJumpTable(script),
	}
}
