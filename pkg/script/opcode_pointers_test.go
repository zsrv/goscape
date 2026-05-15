package script

import (
	"reflect"
	"testing"
)

// TestPointerGroupFind_Content pins spec §7.11: 5 elements in TS order
// (find_player, find_npc, find_loc, find_obj, find_db). Order matters
// because corrupt-slice content is concatenated in this order.
func TestPointerGroupFind_Content(t *testing.T) {
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}
	if !reflect.DeepEqual(PointerGroupFind, want) {
		t.Fatalf("PointerGroupFind\n got = %v\nwant = %v", PointerGroupFind, want)
	}
}

// TestCorruptExceptActive_Behavior pins the helper's contract: returns
// PointerGroupFind ++ extras as a fresh slice (caller mutations must
// not alias PointerGroupFind).
func TestCorruptExceptActive_Behavior(t *testing.T) {
	got := corruptExceptActive("last_com", "last_int")
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db", "last_com", "last_int"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got = %v, want = %v", got, want)
	}

	// Independence: mutating the returned slice must not corrupt
	// PointerGroupFind.
	got[0] = "MUTATED"
	if PointerGroupFind[0] != "find_player" {
		t.Fatalf("PointerGroupFind[0] = %q after caller mutation; want %q (helper must return a fresh slice)", PointerGroupFind[0], "find_player")
	}
}

// TestPointers_ZeroValue pins that a missing entry returns a useful
// zero value (all-nil slices, Conditional=false) — this is what
// callers see on map miss. Matches TS `undefined` semantics via Go map miss.
func TestPointers_ZeroValue(t *testing.T) {
	var p Pointers
	if p.Require != nil || p.Require2 != nil || p.Set != nil || p.Set2 != nil ||
		p.Corrupt != nil || p.Corrupt2 != nil || p.Conditional {
		t.Fatalf("Pointers{} should be all-zero; got %+v", p)
	}
}
