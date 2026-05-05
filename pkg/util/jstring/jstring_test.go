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
	// 3-word coverage (e.g. "alice_smith_jr") deferred — its ToBase37
	// encoding is divisible by 37, tripping goscape's pre-existing
	// missing divide-out-37 loop (TS JString.ts:21-23). Tracked as
	// separate follow-up; out of NAI-104 scope.
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"alice", "Alice"},
		{"user_two", "User Two"},
		{"USER_TWO", "User Two"}, // case-insensitive via base37 round-trip
		{"player1", "Player1"},   // digits inside a token
	}
	for _, c := range cases {
		if got := ToDisplayName(c.in); got != c.want {
			t.Errorf("ToDisplayName(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}
