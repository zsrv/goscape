package codegen

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
)

// CodeGeneratorContext carries the context of a CodeGenerator visit for use
// by DynamicCommandHandler.GenerateCode implementations. Mirrors TS class
// CodeGeneratorContext (CodeGeneratorContext.ts).
//
// NAI-207-D-CODEGENCONTEXT-MARKER: satisfies semantics.CodeGenContext by
// placing the marker interface in semantics to avoid a codegen→semantics
// import cycle. NAI-207-D-CODEGENCONTEXT-EXPORTEDMARKER: the marker method
// is exported (IsCodeGenContext()), not unexported, because Go's interface
// rules prevent a type in package codegen from satisfying an unexported method
// declared in package semantics.
type CodeGeneratorContext struct {
	// generator is the owning CodeGenerator; used for Instruction,
	// LineInstruction, and VisitNodeOrNull.
	generator *CodeGenerator

	// SymbolTable is the active scope's symbol table. Exported for handler use.
	SymbolTable *symbol.SymbolTable

	// Expression is the AST node being code-generated
	// (CommandCallExpression or Identifier).
	Expression ast.Expression

	// Diagnostics is the shared diagnostic collector. Exported for handler use.
	Diagnostics *diagnostics.Diagnostics
}

// NewCodeGeneratorContext constructs a CodeGeneratorContext.
func NewCodeGeneratorContext(
	gen *CodeGenerator,
	table *symbol.SymbolTable,
	expr ast.Expression,
	d *diagnostics.Diagnostics,
) *CodeGeneratorContext {
	return &CodeGeneratorContext{
		generator:   gen,
		SymbolTable: table,
		Expression:  expr,
		Diagnostics: d,
	}
}

// IsCodeGenContext satisfies semantics.CodeGenContext. Production code in
// codegen must NOT import semantics (cycle); the marker is declared there
// and satisfied here via this exported method.
// NAI-207-D-CODEGENCONTEXT-EXPORTEDMARKER: the method is exported because
// Go's interface rules prevent a type in package codegen from satisfying an
// unexported method declared in package semantics.
func (*CodeGeneratorContext) IsCodeGenContext() {}

// Instruction delegates to the owning CodeGenerator to emit one instruction.
// Mirrors TS CodeGeneratorContext.instruction().
func (c *CodeGeneratorContext) Instruction(op Opcode, operand any, src lexer.NodeSourceLocation) {
	c.generator.Instruction(op, operand, src)
}

// LineInstruction emits a LineNumber instruction for node n's source location.
// Mirrors TS CodeGeneratorContext.lineInstruction().
func (c *CodeGeneratorContext) LineInstruction(n ast.Node) {
	c.generator.LineInstruction(n)
}

// VisitNode passes node n through the code generator. Mirrors TS
// CodeGeneratorContext.visitNode().
func (c *CodeGeneratorContext) VisitNode(n ast.Node) {
	c.generator.VisitNodeOrNull(n)
}

// VisitExpression visits expr through the code generator. Mirrors TS
// CodeGeneratorContext.visitExpression().
func (c *CodeGeneratorContext) VisitExpression(expr ast.Expression) {
	c.generator.VisitNodeOrNull(expr)
}

// VisitNodes passes all nodes through the code generator. Mirrors TS
// CodeGeneratorContext.visitNodes().
func (c *CodeGeneratorContext) VisitNodes(nodes []ast.Node) {
	for _, n := range nodes {
		c.generator.VisitNodeOrNull(n)
	}
}

// Arguments returns the argument list if Expression is a CallExpressionNode,
// otherwise returns nil. Mirrors TS get arguments() in CodeGeneratorContext.
func (c *CodeGeneratorContext) Arguments() []ast.Expression {
	call, ok := c.Expression.(ast.CallExpressionNode)
	if !ok {
		return nil
	}
	switch cl := call.(type) {
	case *ast.CommandCallExpression:
		return cl.Arguments
	case *ast.ProcCallExpression:
		return cl.Arguments
	case *ast.JumpCallExpression:
		return cl.Arguments
	case *ast.ClientScriptExpression:
		return cl.Arguments
	}
	return nil
}

// Arguments2 returns the secondary argument list if Expression is a
// *ast.CommandCallExpression with a non-nil Arguments2, otherwise returns nil.
// Mirrors TS get arguments2() in CodeGeneratorContext.
func (c *CodeGeneratorContext) Arguments2() []ast.Expression {
	if cmd, ok := c.Expression.(*ast.CommandCallExpression); ok && cmd.Arguments2 != nil {
		return cmd.Arguments2
	}
	return nil
}

// Command emits the full Command instruction sequence for the expression: a
// LineInstruction followed by Command(sym). Panics if no symbol can be resolved.
// Mirrors TS CodeGeneratorContext.command().
func (c *CodeGeneratorContext) Command() {
	var sym symbol.Symbol
	switch v := c.Expression.(type) {
	case *ast.CommandCallExpression:
		if s, ok := v.Symbol.(symbol.Symbol); ok {
			sym = s
		}
	case *ast.Identifier:
		if s, ok := v.Reference.(symbol.Symbol); ok {
			sym = s
		}
	}
	if sym == nil {
		panic("CodeGeneratorContext.Command: symbol cannot be nil")
	}
	c.LineInstruction(c.Expression)
	c.Instruction(Command, sym, c.Expression.Source())
}
