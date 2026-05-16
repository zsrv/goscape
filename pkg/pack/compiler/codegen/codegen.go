package codegen

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// CodeGenerator is the codegen pass walker. T3 ships only the struct stub;
// T4 fills in fields, ctor, and Visit dispatch.
type CodeGenerator struct {
	// Fields added in T4.
}

// T3 stubs — T4 replaces with real bodies.
func (g *CodeGenerator) Instruction(op Opcode, operand any, src lexer.NodeSourceLocation) {}
func (g *CodeGenerator) LineInstruction(n ast.Node)                                        {}
func (g *CodeGenerator) VisitNodeOrNull(n ast.Node)                                        {}
