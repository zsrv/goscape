package ast

// NAI-205-D-AST-REF-INTERFACES: TS allows AST nodes to directly typed-reference
// Symbol/Trigger/Type/SymbolTable instances. Go's no-cyclic-import rule would
// force pkg/pack/compiler/ast to import symbol/trigger/type — but those
// packages reference ast.Node (for the AST-field consumer side). Resolution:
// four marker interfaces here, each with one exported zero-arg method.
// Concrete impls in symbol/trigger/type implement the method; structural
// typing satisfies the interface from those packages without importing ast.
// Consumers (semantics, future codegen) type-assert to the concrete type
// at the read site, e.g. `s.Symbol.(*symbol.ServerScriptSymbol)`.

// SymbolRef is satisfied by every concrete symbol type
// (ServerScriptSymbol, ClientScriptSymbol, BasicSymbol, LocalVariableSymbol).
// Stored on ast.Script.Symbol, ast.Script.SubjectReference, ast.Parameter.Symbol.
type SymbolRef interface {
	AsSymbolRef()
}

// TriggerRef is satisfied by *trigger.TriggerType.
// Stored on ast.Script.TriggerType.
type TriggerRef interface {
	AsTriggerRef()
}

// TypeRef is satisfied by every concrete type implementation
// (PrimitiveType, MetaType variants, TupleType, ArrayType, GameVarType variants).
// Stored on ast.Script.ParameterType and ast.Script.ReturnType.
type TypeRef interface {
	AsTypeRef()
}

// SymbolTableRef is satisfied by *symbol.SymbolTable.
// Stored on ast.Script.Block.
type SymbolTableRef interface {
	AsSymbolTableRef()
}
