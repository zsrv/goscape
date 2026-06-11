package clientinterface

import "testing"

func TestNameToType(t *testing.T) {
	cases := map[string]int{
		"layer": 0, "overlay": 0, "inv": 2, "rect": 3, "text": 4,
		"graphic": 5, "model": 6, "invtext": 7,
		"unknown": -1, "": -1,
	}
	for in, want := range cases {
		if got := nameToType(in); got != want {
			t.Errorf("nameToType(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNameToButtonType(t *testing.T) {
	cases := map[string]int{
		"normal": 1, "target": 2, "close": 3, "toggle": 4, "select": 5, "pause": 6,
		"unknown": 0,
	}
	for in, want := range cases {
		if got := nameToButtonType(in); got != want {
			t.Errorf("nameToButtonType(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNameToComparator(t *testing.T) {
	cases := map[string]int{
		"eq": 1, "lt": 2, "gt": 3, "neq": 4,
		"unknown": 0,
	}
	for in, want := range cases {
		if got := nameToComparator(in); got != want {
			t.Errorf("nameToComparator(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNameToScript(t *testing.T) {
	cases := map[string]int{
		"stat_level": 1, "stat_base_level": 2, "stat_xp": 3, "inv_count": 4,
		"pushvar": 5, "stat_xp_remaining": 6, "op7": 7, "op8": 8, "op9": 9,
		"inv_contains": 10, "runenergy": 11, "runweight": 12, "testbit": 13,
		// rev-254 (TS PackShared.ts:91-104 @ 2e3bcf43):
		"push_varbit": 14, "subtract": 15, "divide": 16, "multiply": 17,
		"coordx": 18, "coordz": 19, "push_constant": 20,
		"unknown": 0,
	}
	for in, want := range cases {
		if got := nameToScript(in); got != want {
			t.Errorf("nameToScript(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNameToStat(t *testing.T) {
	cases := map[string]int{
		"attack": 0, "defence": 1, "strength": 2, "hitpoints": 3, "ranged": 4,
		"prayer": 5, "magic": 6, "cooking": 7, "woodcutting": 8, "fletching": 9,
		"fishing": 10, "firemaking": 11, "crafting": 12, "smithing": 13,
		"mining": 14, "herblore": 15, "agility": 16, "thieving": 17, "runecraft": 20,
		"unknown": -1,
	}
	for in, want := range cases {
		if got := nameToStat(in); got != want {
			t.Errorf("nameToStat(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNameToFont(t *testing.T) {
	cases := map[string]int{
		"p11": 0, "p12": 1, "b12": 2, "q8": 3,
		"unknown": -1,
	}
	for in, want := range cases {
		if got := nameToFont(in); got != want {
			t.Errorf("nameToFont(%q) = %d, want %d", in, got, want)
		}
	}
}
