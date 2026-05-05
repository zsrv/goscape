package util

import "testing"

func TestFromBase37InvalidNameUpperBound(t *testing.T) {
	// 6582952005840035281 = 37**12 — sentinel TS line 38.
	if got := FromBase37(6582952005840035281); got != "invalid_name" {
		t.Errorf("upper-bound FromBase37: got %q, want %q", got, "invalid_name")
	}
}

func TestFromBase37InvalidNameMod37(t *testing.T) {
	// Any nonzero multiple of 37 must return "invalid_name" per TS
	// JString.ts:42-44. Pre-NAI-72 goscape returns the decoded string.
	cases := []uint64{37, 74, 1369, 37 * 12345}
	for _, v := range cases {
		if got := FromBase37(v); got != "invalid_name" {
			t.Errorf("FromBase37(%d): got %q, want %q", v, got, "invalid_name")
		}
	}
}

func TestFromBase37ValidNameDecodes(t *testing.T) {
	// Sanity check: a valid encoded name round-trips through ToBase37.
	name := "alice"
	encoded := ToBase37(name)
	if got := FromBase37(encoded); got != name {
		t.Errorf("FromBase37(ToBase37(%q)): got %q, want %q", name, got, name)
	}
}

func TestToDisplayName(t *testing.T) {
	// Trailing-underscore inputs round-trip post-NAI-105 divide-out-37
	// loop in ToBase37 (TS JString.ts:21-23). "alice_smith_jr" is 14
	// chars; the 12-char encode truncation (shared with TS, JString.ts:5)
	// drops the trailing "jr" before the divide-out runs, so the
	// post-fix round-trip is "Alice Smith", not "Alice Smith Jr".
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"alice", "Alice"},
		{"user_two", "User Two"},
		{"USER_TWO", "User Two"}, // case-insensitive via base37 round-trip
		{"player1", "Player1"},   // digits inside a token
		{"alice_", "Alice"},                 // single trailing '_'
		{"alice_smith_", "Alice Smith"},     // multi-word + trailing '_'
		{"alice_smith_jr", "Alice Smith"},   // 14-char input, truncated at 12
	}
	for _, c := range cases {
		if got := ToDisplayName(c.in); got != c.want {
			t.Errorf("ToDisplayName(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToBase37DividesOutTrailing37(t *testing.T) {
	// TS JString.ts:21-23 — after encoding, trailing factors of 37 are
	// divided out so the stored value is never divisible by 37 (except 0).
	// The base37 lookup maps '_' → 0; any trailing '_' makes the raw
	// encoding divisible by 37.
	base := ToBase37("alice")
	if base%37 == 0 {
		t.Fatalf("test fixture invariant: ToBase37(%q) = %d unexpectedly divisible by 37", "alice", base)
	}
	cases := []struct {
		in   string
		want uint64 // expected ToBase37 output post-divide-out
	}{
		{"alice", base},
		{"alice_", base},  // single trailing '_' divided out
		{"alice__", base}, // multiple trailing '_' divided out iteratively
	}
	for _, c := range cases {
		if got := ToBase37(c.in); got != c.want {
			t.Errorf("ToBase37(%q): got %d, want %d", c.in, got, c.want)
		}
	}
	// Post-divide-out invariant: nonzero outputs are never divisible by 37.
	for _, in := range []string{"alice_", "alice__", "alice_smith_"} {
		if v := ToBase37(in); v != 0 && v%37 == 0 {
			t.Errorf("ToBase37(%q) = %d post-divide-out invariant violated", in, v)
		}
	}
}
