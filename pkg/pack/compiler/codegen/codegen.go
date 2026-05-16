// pkg/pack/compiler/codegen/codegen.go — ports the dispatch + entry-point
// arms of TS src/compiler/codegen/CodeGenerator.ts.
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CodeGenerator lowers a type-checked ast.ScriptFile to a list of RuneScript
// records. Mirrors TS CodeGenerator (CodeGenerator.ts).
type CodeGenerator struct {
	rootTable       *symbol.SymbolTable
	dynamicCommands map[string]semantics.DynamicCommandHandler
	diagnostics     *diagnostics.Diagnostics

	labelGenerator *LabelGenerator
	scripts        []*RuneScript
	block          *Block // active block (set by bind)
	lastLineNumber int
}

// NewCodeGenerator constructs a CodeGenerator. dynamicCommands may be empty or
// nil — emitDynamicCommand will treat both as "no handler registered".
// Mirrors TS CodeGenerator constructor.
func NewCodeGenerator(
	rootTable *symbol.SymbolTable,
	dynamicCommands map[string]semantics.DynamicCommandHandler,
	d *diagnostics.Diagnostics,
) *CodeGenerator {
	if dynamicCommands == nil {
		dynamicCommands = map[string]semantics.DynamicCommandHandler{}
	}
	return &CodeGenerator{
		rootTable:       rootTable,
		dynamicCommands: dynamicCommands,
		diagnostics:     d,
		labelGenerator:  NewLabelGenerator(),
		lastLineNumber:  -1,
	}
}

// Scripts returns the codegen output — one RuneScript per non-command script
// in the input ScriptFile. Mirrors TS get scripts() getter.
func (g *CodeGenerator) Scripts() []*RuneScript { return g.scripts }

// activeScript returns the most-recently-pushed RuneScript.
func (g *CodeGenerator) activeScript() *RuneScript {
	if len(g.scripts) == 0 {
		return nil
	}
	return g.scripts[len(g.scripts)-1]
}

// bind sets b as the active block. Mirrors TS bind helper.
func (g *CodeGenerator) bind(b *Block) *Block {
	g.block = b
	return b
}

// generateBlock allocates a new Block and appends it to activeScript.Blocks.
// When generateUniqueName is true the label name is run through LabelGenerator;
// otherwise the literal name is used directly.
// Mirrors TS generateBlock(name, generateUniqueName=true).
func (g *CodeGenerator) generateBlock(name string, generateUniqueName bool) *Block {
	var lbl *Label
	if generateUniqueName {
		lbl = g.labelGenerator.Generate(name)
	} else {
		lbl = &Label{Name: name}
	}
	b := NewBlock(lbl)
	if s := g.activeScript(); s != nil {
		s.Blocks = append(s.Blocks, b)
	}
	return b
}

// generateBlockLabel constructs a Block from an existing Label (used when the
// caller has already generated the label). Mirrors TS generateBlockLabel.
func (g *CodeGenerator) generateBlockLabel(lbl *Label) *Block {
	b := NewBlock(lbl)
	if s := g.activeScript(); s != nil {
		s.Blocks = append(s.Blocks, b)
	}
	return b
}

// Instruction appends an instruction to the active block. Exported for use by
// CodeGeneratorContext (dynamic-command handlers). Mirrors TS instruction().
func (g *CodeGenerator) Instruction(op Opcode, operand any, src lexer.NodeSourceLocation) {
	if g.block == nil {
		return
	}
	g.block.Add(Instruction{Opcode: op, Operand: operand, Source: src})
}

// instructionUnit emits an opcode that takes no operand. Convenience.
func (g *CodeGenerator) instructionUnit(op Opcode, src lexer.NodeSourceLocation) {
	g.Instruction(op, nil, src)
}

// LineInstruction records the node's source line in lastLineNumber. Does NOT
// emit a LineNumber instruction — TS reserves the opcode but does not emit it
// at this stage either. Exposed for use by CodeGeneratorContext.
//
// NAI-207-D-LINENUMBER-NO-EMIT: TS CodeGenerator.lineInstruction records
// lastLineNumber but the LineNumber opcode emission is reserved for future
// use. Goscape preserves identical semantics: track lastLineNumber, no emit.
func (g *CodeGenerator) LineInstruction(n ast.Node) {
	if n == nil {
		return
	}
	line := n.Source().Line
	if line != g.lastLineNumber {
		g.lastLineNumber = line
	}
}

// VisitNodeOrNull is the public shortcut for Visit-with-nil-check. Exported
// for CodeGeneratorContext use. Mirrors TS visitNodeOrNull.
func (g *CodeGenerator) VisitNodeOrNull(n ast.Node) {
	if n == nil {
		return
	}
	g.Visit(n)
}

// Visit is the dispatch root. Mirrors TS visitor pattern, but via Go
// type-switch (NAI-204-D-AST-NO-VISITOR).
func (g *CodeGenerator) Visit(n ast.Node) {
	switch v := n.(type) {
	case *ast.ScriptFile:
		g.visitScriptFile(v)
	case *ast.Script:
		g.visitScript(v)
	case *ast.Parameter:
		g.visitParameter(v)
	case *ast.ReturnStatement:
		g.visitReturnStatement(v)
	case *ast.IfStatement:
		g.visitIfStatement(v)
	case *ast.WhileStatement:
		g.visitWhileStatement(v)
	case *ast.SwitchStatement:
		g.visitSwitchStatement(v)
	case *ast.BlockStatement:
		g.visitBlockStatement(v)
	case *ast.DeclarationStatement:
		g.visitDeclaration(v)
	case *ast.ArrayDeclarationStatement:
		g.visitArrayDeclaration(v)
	case *ast.AssignmentStatement:
		g.visitAssignment(v)
	case *ast.ExpressionStatement:
		g.visitExpressionStatement(v)
	case *ast.EmptyStatement:
		g.visitEmptyStatement(v)
	case *ast.IntegerLiteral:
		// NAI-207-D-INTLIT-T5-STUB: T5 needs IntegerLiteral emission for
		// condition and return-expression tests (generateConditionBinary and
		// visitReturnStatement call VisitNodeOrNull on their sub-expressions).
		// T10 will port the full visitIntegerLiteral (Reference + string-base
		// type promotion). This stub covers the no-reference int case only.
		g.LineInstruction(v)
		if v.Reference == nil {
			g.Instruction(PushConstantInt, v.Value, v.Source())
		} else {
			g.Instruction(PushConstantSymbol, v.Reference, v.Source())
		}
	// Expression arms added in T7–T11.
	case nil:
		return
	default:
		// Unhandled arms fall through silently until later tasks add them.
		_ = v
	}
}

// VisitNodes visits each Node in ns. Mirrors TS visitNodes.
func (g *CodeGenerator) VisitNodes(ns []ast.Node) {
	for _, n := range ns {
		g.VisitNodeOrNull(n)
	}
}

// visitExpressions visits each expression in es.
func (g *CodeGenerator) visitExpressions(es []ast.Expression) {
	for _, e := range es {
		g.VisitNodeOrNull(e)
	}
}

func (g *CodeGenerator) visitScriptFile(sf *ast.ScriptFile) {
	for _, s := range sf.Scripts {
		g.VisitNodeOrNull(s)
	}
}

func (g *CodeGenerator) visitScript(s *ast.Script) {
	// Skip command-trigger scripts. Mirrors TS CodeGenerator.ts L173-L176.
	tr, ok := s.TriggerType.(*trigger.TriggerType)
	if !ok || tr == nil {
		// NAI-207-D-UNRESOLVED-TRIGGER-NO-OP: unresolved trigger (nil TriggerType
		// or non-TriggerType TriggerRef) is silently skipped. ScriptRegistration
		// would have already emitted a diagnostic for the unresolved trigger.
		return
	}
	if tr == trigger.CommandTrigger {
		return
	}

	// Resolve the script symbol. Use the Symbol marker interface.
	sym, _ := s.Symbol.(symbol.Symbol)

	// Push a fresh RuneScript. Source name comes from the AST node's location
	// (populated by the lexer with the sourceName passed to NewScriptFileParser).
	rs := NewRuneScript(s.Source().Name, sym, tr, s.NameString(), s.SubjectReference)
	g.scripts = append(g.scripts, rs)

	// Visit parameters — populates LocalTable before the entry block is emitted.
	for _, p := range s.Parameters {
		g.visitParameter(p)
	}

	// Bind the entry block. TS uses the literal name "entry" (not unique-suffixed).
	g.bind(g.generateBlock("entry", false))

	// Record the script-header source line.
	g.LineInstruction(s)

	// Visit the body statements.
	for _, st := range s.Statements {
		g.VisitNodeOrNull(st)
	}

	// Default returns at the end of the script.
	g.generateDefaultReturns(s)

	// Reset per-script state.
	g.labelGenerator.Reset()
	g.lastLineNumber = -1
}

func (g *CodeGenerator) visitParameter(p *ast.Parameter) {
	sym, ok := p.Symbol.(*symbol.LocalVariableSymbol)
	if !ok || sym == nil {
		return
	}
	rs := g.activeScript()
	if rs == nil {
		return
	}
	rs.Locals.Parameters = append(rs.Locals.Parameters, sym)
	rs.Locals.All = append(rs.Locals.All, sym)
}

// generateDefaultReturns appends the default-return sequence for each type in
// the script's return-type tuple. Mirrors TS generateDefaultReturns
// (CodeGenerator.ts L212-L231).
func (g *CodeGenerator) generateDefaultReturns(s *ast.Script) {
	g.LineInstruction(s)
	rt, _ := s.ReturnType.(typ.Type)
	types := typ.TupleToList(rt)
	for _, t := range types {
		if t == nil {
			continue
		}
		// PrimitiveInt special case — push 0 not -1. Mirrors TS L218.
		if t == typ.PrimitiveInt {
			g.Instruction(PushConstantInt, 0, lexer.NodeSourceLocation{})
			continue
		}
		base, ok := t.BaseType()
		if !ok {
			panic(fmt.Sprintf("generateDefaultReturns: type %v has no base type", t))
		}
		switch base {
		case typ.BaseVarInteger:
			g.Instruction(PushConstantInt, -1, lexer.NodeSourceLocation{})
		case typ.BaseVarString:
			g.Instruction(PushConstantString, "", lexer.NodeSourceLocation{})
		case typ.BaseVarLong:
			g.Instruction(PushConstantLong, int64(-1), lexer.NodeSourceLocation{})
		default:
			panic(fmt.Sprintf("generateDefaultReturns: unsupported BaseVarType %v for type %v", base, t))
		}
	}
	g.instructionUnit(Return, lexer.NodeSourceLocation{})
}
