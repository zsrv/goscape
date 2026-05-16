// pkg/pack/compiler/pointer/type_test.go
package pointer

import "testing"

// TestPointerType_AllHas22 pins the count of PointerType singletons. TS
// PointerType.ALL has 22 entries (PointerType.ts L2-L23). Adding or removing
// a pointer is a load-bearing change; this test fails when ALL drifts.
func TestPointerType_AllHas22(t *testing.T) {
	if got := len(All); got != 22 {
		t.Fatalf("len(All) = %d, want 22", got)
	}
}

// TestPointerType_AllSingletonsUniqueIdentity pins that the All slice entries
// are pointer-identity-unique. Pointer identity is the equality key for
// PointerSet and PointerChecker analysis arrays.
func TestPointerType_AllSingletonsUniqueIdentity(t *testing.T) {
	seen := map[*PointerType]struct{}{}
	for i, p := range All {
		if _, dup := seen[p]; dup {
			t.Errorf("All[%d] = %v is a duplicate pointer-identity", i, p.Representation)
		}
		seen[p] = struct{}{}
	}
}

// TestPointerType_IndexRoundTrip pins that Index(All[i]) == i for every i.
func TestPointerType_IndexRoundTrip(t *testing.T) {
	for i, p := range All {
		if got := Index(p); got != i {
			t.Errorf("Index(All[%d]) = %d, want %d", i, got, i)
		}
	}
}

// TestPointerType_ForNameKnown pins ForName resolves the canonical
// representation back to the singleton (case-insensitive).
func TestPointerType_ForNameKnown(t *testing.T) {
	cases := []struct {
		name string
		want *PointerType
	}{
		{"active_player", ActivePlayer},
		{"ACTIVE_PLAYER", ActivePlayer},
		{".active_player", ActivePlayer2},
		{"p_active_player", PActivePlayer},
		{"last_targetslot", LastTargetSlot},
	}
	for _, c := range cases {
		if got := ForName(c.name); got != c.want {
			t.Errorf("ForName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPointerType_ForNameMiss pins ForName returns nil for unknown names.
func TestPointerType_ForNameMiss(t *testing.T) {
	if got := ForName("nope"); got != nil {
		t.Errorf("ForName(\"nope\") = %v, want nil", got)
	}
}

// TestPointerType_RepresentationFromAll pins representation strings for the
// first three singletons (regression guard against literal drift).
func TestPointerType_RepresentationFromAll(t *testing.T) {
	cases := []struct {
		p    *PointerType
		want string
	}{
		{ActivePlayer, "active_player"},
		{ActivePlayer2, ".active_player"},
		{PActivePlayer, "p_active_player"},
	}
	for _, c := range cases {
		if c.p.Representation != c.want {
			t.Errorf("Representation = %q, want %q", c.p.Representation, c.want)
		}
	}
}
