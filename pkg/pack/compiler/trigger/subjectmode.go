// pkg/pack/compiler/trigger/subjectmode.go
package trigger

import (
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// SubjectMode is the sealed interface representing how a trigger's subject
// is validated. Mirrors TS SubjectMode.ts.
//
// Three concrete impls: ModeNone (global-only), ModeName (any name), and
// TypeMode (subject is a reference to a Type instance). The sealing is
// enforced via the unexported subjectMode() method.
type SubjectMode interface {
	subjectMode()
}

type modeNoneT struct{}
type modeNameT struct{}

func (modeNoneT) subjectMode() {}
func (modeNameT) subjectMode() {}

// ModeNone allows only the global subject `_`. Mirrors TS SubjectMode.None.
var ModeNone SubjectMode = modeNoneT{}

// ModeName allows any string as the subject. Mirrors TS SubjectMode.Name.
var ModeName SubjectMode = modeNameT{}

// TypeMode is a value-typed SubjectMode that carries the resolved Type and
// the category/global feature flags. Mirrors TS SubjectMode.Type(...).
type TypeMode struct {
	Type     typ.Type
	Category bool
	Global   bool
}

func (TypeMode) subjectMode() {}

// NewModeType is the goscape equivalent of TS `SubjectMode.Type(t, category, global)`.
// Returns a value-typed TypeMode (no interning; TS likewise returns a fresh
// class instance per call).
func NewModeType(t typ.Type, category, global bool) TypeMode {
	return TypeMode{Type: t, Category: category, Global: global}
}

// IsTypeMode returns (tm, true) when m is a TypeMode, otherwise (zero, false).
// Replaces TS `'type' in mode` discriminator check.
func IsTypeMode(m SubjectMode) (TypeMode, bool) {
	tm, ok := m.(TypeMode)
	return tm, ok
}
