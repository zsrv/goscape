// pkg/pack/compiler/pointer/holder.go
package pointer

// PointerSet is the goscape port of TS `Set<PointerType>`. Backed by a
// map[*PointerType]struct{} since Go has no built-in Set<T>.
//
// NAI-208-D-POINTERSET-MAP-STRUCT: the wrapper exists so consumers don't
// scatter map-literal boilerplate; PointerChecker analysis arrays use the
// same shape via map[*InstructionNode]struct{}.
//
// All methods are nil-safe (zero-value reads return false/0) to simplify
// the cfg.PointerChecker code path where empty holders short-circuit.
type PointerSet struct {
	m map[*PointerType]struct{}
}

// NewPointerSet returns a fresh set containing items.
func NewPointerSet(items ...*PointerType) *PointerSet {
	s := &PointerSet{m: make(map[*PointerType]struct{}, len(items))}
	for _, p := range items {
		s.m[p] = struct{}{}
	}
	return s
}

// Add inserts pt. Idempotent.
func (s *PointerSet) Add(pt *PointerType) {
	if s.m == nil {
		s.m = map[*PointerType]struct{}{}
	}
	s.m[pt] = struct{}{}
}

// Has reports whether pt is present. nil-safe.
func (s *PointerSet) Has(pt *PointerType) bool {
	if s == nil || s.m == nil {
		return false
	}
	_, ok := s.m[pt]
	return ok
}

// Len returns the number of entries. nil-safe.
func (s *PointerSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.m)
}

// All returns the entries in declaration order from the package-level All
// slice (stable iteration regardless of map iteration order). nil-safe.
func (s *PointerSet) All() []*PointerType {
	if s == nil || len(s.m) == 0 {
		return nil
	}
	out := make([]*PointerType, 0, len(s.m))
	for _, p := range All {
		if _, ok := s.m[p]; ok {
			out = append(out, p)
		}
	}
	return out
}

// Clone returns a deep copy. nil-safe (nil → empty non-nil set).
func (s *PointerSet) Clone() *PointerSet {
	if s == nil {
		return NewPointerSet()
	}
	c := &PointerSet{m: make(map[*PointerType]struct{}, len(s.m))}
	for k := range s.m {
		c.m[k] = struct{}{}
	}
	return c
}

// PointerHolder describes the pointer state a command or script requires,
// sets, and/or corrupts. Mirrors TS PointerHolder interface
// (PointerHolder.ts).
//
// NAI-208-D-POINTERHOLDER-PTRSET: fields are *PointerSet rather than bare
// maps so the wrapper's nil-safety and ordered iteration carry through.
type PointerHolder struct {
	Required       *PointerSet
	Set            *PointerSet
	ConditionalSet bool
	Corrupted      *PointerSet
}
