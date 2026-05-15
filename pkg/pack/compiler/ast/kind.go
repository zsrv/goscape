package ast

// NodeKind discriminates between concrete Node types. Mirrors
// RuneScriptTS src/parser/ast/NodeKind.ts. Diagnostic/serialization use
// only — type-switch on the concrete type is the primary dispatch.
type NodeKind int

const (
	KindScriptFile NodeKind = iota
	KindScript
	KindParameter
	KindToken
	KindBlockStatement
	KindReturnStatement
	KindIfStatement
	KindWhileStatement
	KindSwitchStatement
	KindSwitchCase
	KindDeclarationStatement
	KindArrayDeclarationStatement
	KindAssignmentStatement
	KindExpressionStatement
	KindEmptyStatement
	KindParenthesizedExpression
	KindIdentifier
	KindJoinedStringExpression
	KindBasicStringPart
	KindPTagStringPart
	KindExpressionStringPart
	KindIntegerLiteral
	KindCoordLiteral
	KindBooleanLiteral
	KindCharacterLiteral
	KindStringLiteral
	KindNullLiteral
	KindLocalVariableExpression
	KindGameVariableExpression
	KindConstantVariableExpression
	KindCommandCallExpression
	KindProcCallExpression
	KindJumpCallExpression
	KindClientScriptExpression
	KindArithmeticExpression
	KindCalcExpression
	KindConditionExpression
)

var kindName = [...]string{
	KindScriptFile:                 "ScriptFile",
	KindScript:                     "Script",
	KindParameter:                  "Parameter",
	KindToken:                      "Token",
	KindBlockStatement:             "BlockStatement",
	KindReturnStatement:            "ReturnStatement",
	KindIfStatement:                "IfStatement",
	KindWhileStatement:             "WhileStatement",
	KindSwitchStatement:            "SwitchStatement",
	KindSwitchCase:                 "SwitchCase",
	KindDeclarationStatement:       "DeclarationStatement",
	KindArrayDeclarationStatement:  "ArrayDeclarationStatement",
	KindAssignmentStatement:        "AssignmentStatement",
	KindExpressionStatement:        "ExpressionStatement",
	KindEmptyStatement:             "EmptyStatement",
	KindParenthesizedExpression:    "ParenthesizedExpression",
	KindIdentifier:                 "Identifier",
	KindJoinedStringExpression:     "JoinedStringExpression",
	KindBasicStringPart:            "BasicStringPart",
	KindPTagStringPart:             "PTagStringPart",
	KindExpressionStringPart:       "ExpressionStringPart",
	KindIntegerLiteral:             "IntegerLiteral",
	KindCoordLiteral:               "CoordLiteral",
	KindBooleanLiteral:             "BooleanLiteral",
	KindCharacterLiteral:           "CharacterLiteral",
	KindStringLiteral:              "StringLiteral",
	KindNullLiteral:                "NullLiteral",
	KindLocalVariableExpression:    "LocalVariableExpression",
	KindGameVariableExpression:     "GameVariableExpression",
	KindConstantVariableExpression: "ConstantVariableExpression",
	KindCommandCallExpression:      "CommandCallExpression",
	KindProcCallExpression:         "ProcCallExpression",
	KindJumpCallExpression:         "JumpCallExpression",
	KindClientScriptExpression:     "ClientScriptExpression",
	KindArithmeticExpression:       "ArithmeticExpression",
	KindCalcExpression:             "CalcExpression",
	KindConditionExpression:        "ConditionExpression",
}

// String returns the symbolic name of the kind (parity with TS NodeKind
// enum string values).
func (k NodeKind) String() string {
	if int(k) < 0 || int(k) >= len(kindName) {
		return "?"
	}
	return kindName[k]
}
