package compiler

import (
	"testing"
)

// TestNewTypeInfo_ZeroValues pins spec §5.8: constructors return *TypeInfo
// with Max=-1 and all maps non-nil so callers can write immediately.
func TestNewTypeInfo_ZeroValues(t *testing.T) {
	p := newTypeInfo()
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if p.Map == nil {
		t.Fatal("Map: got nil, want non-nil empty map")
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got len %d, want 0", len(p.Map))
	}
	if p.NameMap == nil {
		t.Fatal("NameMap: got nil, want non-nil empty map")
	}
	if p.VarType == nil || p.Protect == nil || p.Require == nil || p.Require2 == nil ||
		p.Conditional == nil || p.Set == nil || p.Set2 == nil ||
		p.Corrupt == nil || p.Corrupt2 == nil {
		t.Fatal("auxiliary maps must be non-nil so NAI-201 populator can write without re-init")
	}
}

// TestAdd_UpdateMaxFalse pins spec §7.11: updateMax=false skips Max bump
// even when id > Max.
func TestAdd_UpdateMaxFalse(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", false)
	p.Add(2, "c", true)

	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3 (-1→1 via id=0; skip via updateMax=false; 1→3 via id=2)", p.Max)
	}
	if p.Map[0] != "a" || p.Map[5] != "b" || p.Map[2] != "c" {
		t.Fatalf("Map: got %v, want {0:a, 5:b, 2:c}", p.Map)
	}
}

// TestAdd_MaxMonotonic pins spec §7.12: Max is monotonic non-decreasing —
// a smaller id following a larger id does NOT shrink Max.
func TestAdd_MaxMonotonic(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", true)
	p.Add(2, "c", true)

	if p.Max != 6 {
		t.Fatalf("Max: got %d, want 6 (id=5 bumps to 6; id=2 does NOT re-bump since 6<2 is false)", p.Max)
	}
}
