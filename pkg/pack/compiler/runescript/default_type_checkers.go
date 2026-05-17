// pkg/pack/compiler/runescript/default_type_checkers.go
package runescript

import (
	"reflect"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// registerDefaultTypeCheckers installs the 8 default checkers on tm.
// Mirrors TS ScriptCompiler.setupDefaultTypeCheckers L121-204.
//
// Order matters: TypeManager.Check returns true at the first checker that
// accepts. More-specific shapes come BEFORE the representation-string
// fallback so the fallback only fires when no structural rule matches.
//
// The TS "allow Nothing on the right" checker is commented out in source
// (TS L130-131) — comment preserved here, code omitted.
func registerDefaultTypeCheckers(tm *typ.TypeManager) {
	// 1) Anything → MetaAny (TOP).
	tm.AddTypeChecker(func(left, _ typ.Type) bool {
		return left == typ.MetaAny
	})

	// 2) MetaNothing → anything (BOTTOM). The RuneScriptTS source has this
	// commented out at ScriptCompiler.ts:134, but the published
	// @lostcityrs/runescript@0.9.4 artifact (what Engine-TS bundles) ships
	// with it enabled. Real content relies on it: jump-call expressions
	// (e.g. `@head_wizard_nothing`) have call.type=MetaNothing, but get
	// passed where `label` is expected (e.g. `@multi2("...", @head_wizard_nothing, ...)`).
	// Without this axiom every dialogue-multi caller fails type-check.
	tm.AddTypeChecker(func(_, right typ.Type) bool {
		return right == typ.MetaNothing
	})

	// 3) Anything ↔ MetaError (propagation).
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left == typ.MetaError || right == typ.MetaError
	})

	// 4) Reflexive pointer-equality.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left == right
	})

	// 5) MetaScript: recursive on params + returns. (TS additionally compares
	// trigger identity; goscape's NewMetaScript bakes the trigger identifier
	// into the representation string, so trigger-identity differences fall to
	// the representation fallback below as a mismatch.)
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lp, lr, lOK := typ.IsMetaScript(left)
		rp, rr, rOK := typ.IsMetaScript(right)
		if !lOK || !rOK {
			return false
		}
		return tm.Check(lp, rp) && tm.Check(lr, rr)
	})

	// 6) MetaHook: recursive on transmit-list type.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lh, lOK := typ.IsMetaHook(left)
		rh, rOK := typ.IsMetaHook(right)
		if !lOK || !rOK {
			return false
		}
		return tm.Check(lh, rh)
	})

	// 7) WrappedType: same concrete Go type + recursive on Inner.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lw, lOK := left.(typ.WrappedType)
		rw, rOK := right.(typ.WrappedType)
		if !lOK || !rOK {
			return false
		}
		if reflect.TypeOf(left) != reflect.TypeOf(right) {
			return false
		}
		return tm.Check(lw.Inner(), rw.Inner())
	})

	// 8) TupleType: equal child counts + recursive on every child.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lt, lOK := left.(*typ.TupleType)
		rt, rOK := right.(*typ.TupleType)
		if !lOK || !rOK {
			return false
		}
		if len(lt.Children) != len(rt.Children) {
			return false
		}
		for i := range lt.Children {
			if !tm.Check(lt.Children[i], rt.Children[i]) {
				return false
			}
		}
		return true
	})

	// 9) Representation-string fallback (last-resort).
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left.Representation() == right.Representation()
	})
}
