// pkg/pack/compiler/symbol/symboltype.go
package symbol

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// SymbolKind enumerates the five categories of symbol storable in a SymbolTable.
// Mirrors TS SymbolType.ts kinds.
type SymbolKind int

const (
	SymbolKindServerScript SymbolKind = iota
	SymbolKindClientScript
	SymbolKindLocalVariable
	SymbolKindBasic
	SymbolKindConstant
)

// SymbolType is the (kind, optional-trigger-or-type) tuple used as a
// SymbolTable map key. Mirrors TS tagged-union SymbolType<T>.
//
// NAI-205-D-SYMBOLTYPE-STRING-KEY: TS interns via WeakMap+Map (identity
// equality on trigger/type instances). Goscape derives a string key from
// (Kind, Trigger.Identifier or Type.Representation) and uses that as the
// outer map key in SymbolTable. Behaviour-equivalent for the types
// ScriptRegistration actually consumes.
type SymbolType struct {
	Kind      SymbolKind
	Trigger   *trigger.TriggerType
	BasicType typ.Type
}

// Key returns the canonical string identifying this SymbolType, used as
// the outer map key in SymbolTable.
func (s SymbolType) Key() string {
	switch s.Kind {
	case SymbolKindServerScript:
		return "server:" + s.Trigger.Identifier
	case SymbolKindClientScript:
		return "client:" + s.Trigger.Identifier
	case SymbolKindLocalVariable:
		return "local"
	case SymbolKindBasic:
		return "basic:" + s.BasicType.Representation()
	case SymbolKindConstant:
		return "constant"
	}
	return "unknown"
}

// Factory functions matching TS SymbolType.serverScript(...)/etc. Each is a
// thin wrapper; goscape doesn't intern (TS WeakMap interning unnecessary
// since Key() produces the canonical string).
func SymbolTypeServerScript(t *trigger.TriggerType) SymbolType {
	return SymbolType{Kind: SymbolKindServerScript, Trigger: t}
}

func SymbolTypeClientScript(t *trigger.TriggerType) SymbolType {
	return SymbolType{Kind: SymbolKindClientScript, Trigger: t}
}

func SymbolTypeLocalVariable() SymbolType {
	return SymbolType{Kind: SymbolKindLocalVariable}
}

func SymbolTypeBasic(t typ.Type) SymbolType {
	return SymbolType{Kind: SymbolKindBasic, BasicType: t}
}

func SymbolTypeConstant() SymbolType {
	return SymbolType{Kind: SymbolKindConstant}
}
