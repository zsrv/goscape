package asset

import "testing"

func TestIsValidMapName(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		// valid m{x}_{z} / l{x}_{z}
		{"m48_50", true},
		{"l48_50", true},
		{"m0_0", true},
		{"m100_100", true},
		{"l1_1", true},

		// invalid: wrong first char
		{"x48_50", false},
		{"", false},
		{"M48_50", false},

		// invalid: path-traversal attempts
		{"../etc/passwd", false},
		{"m../foo", false},
		{"m48_50/../bar", false},
		{"/m48_50", false},
		{"m48_50/", false},
		{"m48/50", false},

		// invalid: bad shapes
		{"m_50", false},   // empty x
		{"m48_", false},   // empty z
		{"m48", false},    // no underscore
		{"m48_50_60", false}, // two underscores
		{"m48a_50", false},   // non-digit
		{"m48_5a", false},    // non-digit
		{"mab_cd", false},

		// invalid: too short
		{"m", false},
		{"l", false},
		{"m_", false},
		{"m1_", false},
		{"l_1", false},
	}
	for _, c := range cases {
		if got := isValidMapName(c.s); got != c.want {
			t.Errorf("isValidMapName(%q) = %v; want %v", c.s, got, c.want)
		}
	}
}
