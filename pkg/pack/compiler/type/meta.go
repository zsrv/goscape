// pkg/pack/compiler/type/meta.go
package typ

// MetaType represents internal compiler types (Any/Nothing/Error/Unit) plus
// the parameterised wrapping types. Mirrors TS MetaType.ts.
//
// NAI-205-D-METATYPE-FLAT: TS nests MetaType.Type and MetaType.Script as
// static class properties extending MetaType. Goscape uses one base struct
// for the four named singletons + two distinct types (metaWrapping,
// metaScript) for the parameterised cases. Each implements Type.

type metaBase struct {
	rep     string
	options TypeOptions
}

func newMetaBase(name string) metaBase {
	return metaBase{
		rep: lowerASCII(name),
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = false
		}),
	}
}

// metaPrimitive is the concrete impl for the four named singletons.
type metaPrimitive struct {
	metaBase
}

func (m *metaPrimitive) Representation() string        { return m.rep }
func (m *metaPrimitive) Code() (string, bool)          { return "", false }
func (m *metaPrimitive) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (m *metaPrimitive) DefaultValue() any             { return -1 }
func (m *metaPrimitive) Options() TypeOptions          { return m.options }
func (m *metaPrimitive) AsTypeRef()                    {}

var (
	MetaAny     Type = &metaPrimitive{newMetaBase("any")}
	MetaNothing Type = &metaPrimitive{newMetaBase("nothing")}
	MetaError   Type = &metaPrimitive{newMetaBase("error")}
	MetaUnit    Type = &metaPrimitive{newMetaBase("unit")}
)

// metaWrapping is the TS MetaType.Type(inner) shape.
type metaWrapping struct {
	metaBase
	inner Type
}

func (m *metaWrapping) Representation() string        { return m.rep }
func (m *metaWrapping) Code() (string, bool)          { return "", false }
func (m *metaWrapping) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (m *metaWrapping) DefaultValue() any             { return -1 }
func (m *metaWrapping) Options() TypeOptions          { return m.options }
func (m *metaWrapping) AsTypeRef()                    {}
func (m *metaWrapping) Inner() Type                   { return m.inner }

// NewMetaWrapping returns the MetaType.Type(inner) shape.
// When inner == MetaAny, rep = "type"; otherwise rep = "type<inner>".
// Mirrors TS MetaType.ts L80-87.
func NewMetaWrapping(inner Type) Type {
	rep := "type"
	if inner != MetaAny {
		rep = "type<" + inner.Representation() + ">"
	}
	mb := newMetaBase("type")
	mb.rep = rep
	return &metaWrapping{metaBase: mb, inner: inner}
}

// metaScript is the TS MetaType.Script(trigger, params, returns) shape.
// Deferred surface — ScriptRegistration doesn't construct these. We ship
// the shape so TypeChecking (NAI-206) doesn't need a follow-up type to land.
type metaScript struct {
	metaBase
	params  Type
	returns Type
}

func (m *metaScript) Representation() string        { return m.rep }
func (m *metaScript) Code() (string, bool)          { return "", false }
func (m *metaScript) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (m *metaScript) DefaultValue() any             { return -1 }
func (m *metaScript) Options() TypeOptions          { return m.options }
func (m *metaScript) AsTypeRef()                    {}

// NewMetaScript constructs the TS MetaType.Script shape. NAI-205 doesn't
// consume it; ports the constructor only for symmetry with MetaType.ts.
//
// The first argument is the trigger's identifier string (TS reads
// `trigger.identifier` for `representation` — see MetaType.ts L94). Passing
// the identifier string directly avoids the type → trigger import cycle
// that storing a *trigger.TriggerType pointer would create. Consumers in
// NAI-206 may revisit this if metaScript starts carrying trigger semantics
// beyond just representation.
func NewMetaScript(triggerIdent string, params, returns Type) Type {
	mb := newMetaBase("script")
	mb.rep = triggerIdent
	mb.options.AllowParameter = true
	return &metaScript{metaBase: mb, params: params, returns: returns}
}

// metaHook is the TS MetaType.Hook(transmitListType) shape. Used by
// TypeChecking when a string literal's type hint is a hook (the
// literal is then re-parsed as a clientscript expression — see
// TypeChecking.ts L840-866 at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47).
//
// NAI-206-D-HOOK-REP: TS MetaType.Hook sets representation =
// `hook<${transmitListType.representation}>` (MetaType.ts L110), NOT the
// plain "hook" string that super('hook') initialises. Goscape mirrors this
// by overriding rep in NewMetaHook after newMetaBase.
type metaHook struct {
	metaBase
	transmitListType Type
}

func (m *metaHook) Representation() string        { return m.rep }
func (m *metaHook) Code() (string, bool)          { return "", false }
func (m *metaHook) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (m *metaHook) DefaultValue() any             { return -1 }
func (m *metaHook) Options() TypeOptions          { return m.options }
func (m *metaHook) AsTypeRef()                    {}

// NewMetaHook constructs a MetaType.Hook(transmitListType) instance.
// Mirrors TS MetaType.Hook constructor (MetaType.ts L103-112):
//   - representation = "hook<transmitListType.representation>"
//   - all four TypeOptions flags remain false (no override, unlike Script)
func NewMetaHook(transmitListType Type) Type {
	mb := newMetaBase("hook")
	mb.rep = "hook<" + transmitListType.Representation() + ">"
	return &metaHook{metaBase: mb, transmitListType: transmitListType}
}

// IsMetaHook returns (transmitListType, true) if t is a MetaType.Hook
// produced by NewMetaHook; otherwise (nil, false).
//
// TypeChecking (NAI-206) uses this discriminator at the visitStringLiteral
// dispatch and at visitClientScriptExpression's typeHint check (TS
// TypeChecking.ts L843, L852).
func IsMetaHook(t Type) (transmitListType Type, ok bool) {
	mh, ok := t.(*metaHook)
	if !ok {
		return nil, false
	}
	return mh.transmitListType, true
}

// IsMetaScript returns (params, returns, true) when t is a MetaType.Script
// instance produced by NewMetaScript; otherwise (nil, nil, false).
//
// NAI-206's TypeChecking port will need to discriminate MetaType.Script
// from other Type instances (TS uses `instanceof MetaType.Script` at
// TypeChecking.ts L1249) and read back its parameter/return components
// (TS reads `.params`/`.returns` directly). Goscape's `metaScript` struct
// + its fields are unexported to keep the API surface tight; this
// discriminator is the package-public boundary.
//
// The trigger field is NOT recoverable from metaScript by design (only
// the identifier string is stored, to avoid a type → trigger import
// cycle). NAI-206 callers reconstruct the full *trigger.TriggerType
// via the enclosing ServerScriptSymbol.Trigger when needed.
func IsMetaScript(t Type) (params, returns Type, ok bool) {
	ms, ok := t.(*metaScript)
	if !ok {
		return nil, nil, false
	}
	return ms.params, ms.returns, true
}
