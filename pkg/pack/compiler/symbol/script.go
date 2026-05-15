// pkg/pack/compiler/symbol/script.go
package symbol

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ScriptSymbolFields is the shared field shape for ServerScriptSymbol +
// ClientScriptSymbol. TS uses subclass; goscape uses struct embedding.
//
// NAI-205-D-SCRIPTSYMBOL-NO-POINTERS: TS adds a `pointers(checker)` method
// that returns a PointerHolder. PointerChecker lives in codegen (NAI-207).
// Goscape omits the method; the field-shape carries forward.
type ScriptSymbolFields struct {
	Trigger    *trigger.TriggerType
	Name       string
	Parameters typ.Type
	Returns    typ.Type
}

// ServerScriptSymbol is a script defined with a server-side trigger (proc,
// label, opheld, etc.). Mirrors TS ServerScriptSymbol.
type ServerScriptSymbol struct {
	ScriptSymbolFields
}

func (s *ServerScriptSymbol) SymbolName() string { return s.Name }
func (*ServerScriptSymbol) AsSymbolRef()         {}
func (*ServerScriptSymbol) IsServerScript() bool { return true }

// ClientScriptSymbol is a script defined with a client-side trigger
// (only `clientscript`). Mirrors TS ClientScriptSymbol.
type ClientScriptSymbol struct {
	ScriptSymbolFields
}

func (s *ClientScriptSymbol) SymbolName() string { return s.Name }
func (*ClientScriptSymbol) AsSymbolRef()         {}
func (*ClientScriptSymbol) IsServerScript() bool { return false }
