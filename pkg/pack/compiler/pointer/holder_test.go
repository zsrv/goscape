// pkg/pack/compiler/pointer/holder_test.go
package pointer

import "testing"

func TestPointerSet_Empty(t *testing.T) {
	s := NewPointerSet()
	if got := s.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if s.Has(ActivePlayer) {
		t.Error("Has(ActivePlayer) = true on empty set")
	}
}

func TestPointerSet_AddHasLen(t *testing.T) {
	s := NewPointerSet(ActivePlayer, ActiveNpc)
	if got := s.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
	if !s.Has(ActivePlayer) {
		t.Error("Has(ActivePlayer) = false")
	}
	s.Add(ActiveLoc)
	if got := s.Len(); got != 3 {
		t.Errorf("Len() after Add(ActiveLoc) = %d, want 3", got)
	}
	// Idempotent Add.
	s.Add(ActiveLoc)
	if got := s.Len(); got != 3 {
		t.Errorf("Len() after idempotent Add = %d, want 3", got)
	}
}

func TestPointerSet_Clone(t *testing.T) {
	s := NewPointerSet(ActivePlayer)
	c := s.Clone()
	c.Add(ActiveNpc)
	if s.Has(ActiveNpc) {
		t.Error("Clone mutation leaked back to source")
	}
	if !c.Has(ActivePlayer) {
		t.Error("Clone lost original entry")
	}
}

func TestPointerSet_NilSafe(t *testing.T) {
	var s *PointerSet
	if s.Has(ActivePlayer) {
		t.Error("nil set Has = true")
	}
	if s.Len() != 0 {
		t.Error("nil set Len != 0")
	}
}

func TestPointerHolder_ZeroValue(t *testing.T) {
	var h PointerHolder
	if h.Required != nil || h.Set != nil || h.Corrupted != nil {
		t.Error("PointerHolder zero value should leave set fields nil")
	}
	if h.ConditionalSet {
		t.Error("PointerHolder.ConditionalSet zero should be false")
	}
}
