// Package ast holds the RuneScript AST node hierarchy ported from
// RuneScriptTS src/parser/ast/ (HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47).
//
// The hierarchy is a sealed Go interface (Node) with concrete struct
// implementations. Consumers dispatch via Go type-switch — there is no
// visitor pattern. See deviation tag NAI-204-D-AST-NO-VISITOR.
//
// Semantic-analysis fields on TS classes (Expression.type, Identifier.reference,
// Script.symbol, etc.) are intentionally absent here; NAI-205 partially lifts the
// NAI-204-D-AST-NO-TYPE-FIELDS deviation (Script/Parameter fields populated by
// ScriptRegistration); NAI-206 completes the lift by adding the
// TypeChecking-owned fields.
package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// Node is the sealed root of the AST type hierarchy.
//
// NAI-204-D-AST-NO-VISITOR: TS AstVisitor<R> + accept(visitor) is not
// modeled here; consumers use Go type-switch on Node.
//
// NAI-204-D-AST-NO-PARENT: TS Node.parent back-pointer + findParentByType
// are not modeled; goscape walks top-down with explicit parent context.
//
// NAI-204-D-AST-NO-ATTRIBUTES: TS Node.attributes scratch map is not
// modeled; goscape consumers use side-tables keyed by node pointer.
type Node interface {
	Source() lexer.NodeSourceLocation
	Children() []Node
	Kind() NodeKind
	isNode()
}

// Expression marks nodes that produce a value (mirrors TS Expression
// base class). NAI-204-D-AST-NO-TYPE-FIELDS: TS Expression.type and
// Expression.typeHint remain absent — NAI-206 (TypeChecking) adds them.
type Expression interface {
	Node
	isExpression()
}

// Statement marks nodes that appear at statement position (mirrors TS
// Statement base class).
type Statement interface {
	Node
	isStatement()
}

// CallExpressionNode marks the four call-shape expressions (Command,
// Proc, Jump, ClientScript). Parity with TS CallExpression abstract
// base class — consumers that need to walk "any call" without caring
// which kind use this interface.
type CallExpressionNode interface {
	Expression
	isCallExpression()
}

// VariableExpressionNode marks the three variable-reference shapes
// (Local, Game, Constant). Parity with TS VariableExpression.
type VariableExpressionNode interface {
	Expression
	isVariableExpression()
}
