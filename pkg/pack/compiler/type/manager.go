// pkg/pack/compiler/type/manager.go
package typ

import (
	"fmt"
	"strings"
)

// TypeChecker is a binary predicate over Types — does `right` flow into `left`?
// Mirrors TS TypeManager.ts L6.
type TypeChecker func(left, right Type) bool

// TypeBuilder mutates options as part of a registerNew/changeOptions call.
type TypeBuilder func(*TypeOptions)

// TypeManager owns the type registry plus a chain of assignability checkers.
// Mirrors TS class TypeManager (TypeManager.ts).
//
// NAI-205-D-TYPE-NO-INTERN: TS uses WeakMap interning + cache lookups for
// equality; goscape relies on singleton pointers for primitives/meta and
// Representation() comparison at the call sites that need it.
type TypeManager struct {
	nameToType map[string]Type
	checkers   []TypeChecker
}

func NewTypeManager() *TypeManager {
	return &TypeManager{
		nameToType: map[string]Type{},
	}
}

// Register inserts a Type under the given name. Errors on duplicate name.
// Mirrors TS register(name, type) overload at TypeManager.ts L23-30.
func (m *TypeManager) Register(name string, t Type) error {
	if _, exists := m.nameToType[name]; exists {
		return fmt.Errorf("type %q is already registered", name)
	}
	m.nameToType[name] = t
	return nil
}

// RegisterByRepresentation registers t under t.Representation().
// Mirrors TS register(type) overload at TypeManager.ts L31-37.
func (m *TypeManager) RegisterByRepresentation(t Type) error {
	return m.Register(t.Representation(), t)
}

// RegisterAll registers every type in the slice via RegisterByRepresentation.
// Mirrors TS registerAll(enumClass) at TypeManager.ts L66-70 — goscape passes
// the slice directly rather than reading a {ALL: readonly Type[]} static.
func (m *TypeManager) RegisterAll(types []Type) error {
	for _, t := range types {
		if err := m.RegisterByRepresentation(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisterNew creates a Type via the createType-equivalent fn-builder shape
// and registers it. Mirrors TS registerNew at TypeManager.ts L42-58.
// Returns the new Type so the caller can keep a reference for set-membership
// checks (e.g. the categoryType cache in ScriptRegistration).
func (m *TypeManager) RegisterNew(name, code string, base BaseVarType, defaultVal any, builders ...TypeBuilder) (Type, error) {
	fns := make([]func(*TypeOptions), len(builders))
	for i, b := range builders {
		fns[i] = b
	}
	t := newPrimitiveType(name, code, base, defaultVal, fns...)
	if err := m.RegisterByRepresentation(t); err != nil {
		return nil, err
	}
	return t, nil
}

// ChangeOptions mutates the options of a previously-registered Type via the
// builder fn. Errors if name is unknown OR the registered Type isn't a
// *PrimitiveType (only PrimitiveType has mutable options in the current
// type system; ArrayType/GameVarType/MetaType/TupleType all return
// computed/derived options).
//
// CAUTION: if t was registered by passing a package-level singleton pointer
// (e.g. RegisterByRepresentation(PrimitiveInt)), this mutates the singleton
// globally — every other TypeManager + every consumer that reads
// PrimitiveInt.options() across the process sees the mutation. Only call
// ChangeOptions on Types created via RegisterNew (which always allocates
// a fresh *PrimitiveType).
//
// Mirrors TS changeOptions L77-82 (which has the same hazard — TS callers
// rely on the convention that registry-owned types are local instances).
func (m *TypeManager) ChangeOptions(name string, build TypeBuilder) error {
	t, ok := m.nameToType[name]
	if !ok {
		return fmt.Errorf("type %q not found", name)
	}
	switch concrete := t.(type) {
	case *PrimitiveType:
		build(&concrete.options)
		return nil
	}
	return fmt.Errorf("type %q is not mutable", name)
}

// Find returns the named type or an error. AllowArray strips an "array"
// suffix and wraps base in ArrayType. Mirrors TS find L89-95.
func (m *TypeManager) Find(name string, allowArray bool) (Type, error) {
	if t := m.FindOrNil(name, allowArray); t != nil {
		return t, nil
	}
	return nil, fmt.Errorf("unable to find type %q", name)
}

// FindOrNil returns the named type or nil. Mirrors TS findOrNull L102-110.
func (m *TypeManager) FindOrNil(name string, allowArray bool) Type {
	if allowArray && strings.HasSuffix(name, "array") {
		baseName := name[:len(name)-5]
		base := m.FindOrNil(baseName, false)
		if base == nil || !base.Options().AllowArray {
			return nil
		}
		a, err := NewArrayType(base)
		if err != nil {
			return nil
		}
		return a
	}
	return m.nameToType[name]
}

// AddTypeChecker appends c to the chain. Mirrors TS L118.
func (m *TypeManager) AddTypeChecker(c TypeChecker) {
	m.checkers = append(m.checkers, c)
}

// Check returns true iff any registered checker accepts (left, right).
// Mirrors TS check L124-126.
func (m *TypeManager) Check(left, right Type) bool {
	for _, c := range m.checkers {
		if c(left, right) {
			return true
		}
	}
	return false
}
