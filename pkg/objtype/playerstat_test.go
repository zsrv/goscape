package objtype

import (
	"strings"
	"testing"
)

func TestGetExpByLevelKnownValues(t *testing.T) {
	cases := []struct {
		level, want int
	}{
		{1, 0},          // base case (TS returns undefined; we return 0)
		{2, 830},        // first table entry: 83 × 10
		{3, 1740},       // 174 × 10
		{10, 11540},     // 1154 × 10 — RS2 canonical level-10 XP
		{50, 1013330},   // 101333 × 10 — mid-curve sanity
		{99, 130344310}, // 13034431 × 10 — top of curve
	}
	for _, tc := range cases {
		if got := GetExpByLevel(tc.level); got != tc.want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestGetExpByLevelClampsLow(t *testing.T) {
	for _, lvl := range []int{0, -1, -100} {
		if got := GetExpByLevel(lvl); got != 0 {
			t.Errorf("GetExpByLevel(%d): got %d, want 0 (low-clamp)", lvl, got)
		}
	}
}

func TestGetExpByLevelClampsHigh(t *testing.T) {
	want := GetExpByLevel(99)
	for _, lvl := range []int{100, 200, 1000} {
		if got := GetExpByLevel(lvl); got != want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d (clamp to level-99)", lvl, got, want)
		}
	}
}

func TestPlayerStatCount(t *testing.T) {
	if PlayerStatCount != 21 {
		t.Errorf("PlayerStatCount: got %d, want 21 (matches TS PlayerStat enum)", PlayerStatCount)
	}
}

func TestPlayerStatHitpointsIsThree(t *testing.T) {
	if PlayerStatHitpoints != 3 {
		t.Errorf("PlayerStatHitpoints: got %d, want 3", PlayerStatHitpoints)
	}
}

func TestGetLevelByExpKnownValues(t *testing.T) {
	cases := []struct {
		xp, want int
	}{
		{0, 1},          // below any threshold
		{82, 1},         // just below level-2 (830) threshold
		{830, 2},        // exactly at level-2 threshold
		{831, 2},        // just above
		{11539, 9},      // just below level-10
		{11540, 10},     // exactly at level-10
		{1013329, 49},   // just below level-50
		{1013330, 50},   // exactly at level-50
		{130344309, 98}, // just below level-99
		{130344310, 99}, // exactly at level-99 (cap)
		{999999999, 99}, // way above cap → still 99
	}
	for _, tc := range cases {
		if got := GetLevelByExp(tc.xp); got != tc.want {
			t.Errorf("GetLevelByExp(%d): got %d, want %d", tc.xp, got, tc.want)
		}
	}
}

func TestGetLevelByExpNegativeClampsToOne(t *testing.T) {
	for _, xp := range []int{-1, -100, -999999} {
		if got := GetLevelByExp(xp); got != 1 {
			t.Errorf("GetLevelByExp(%d): got %d, want 1", xp, got)
		}
	}
}

func TestGetLevelByExpInverseOfGetExpByLevel(t *testing.T) {
	// Round-trip: for every valid level, GetLevelByExp(GetExpByLevel(level)) == level.
	for level := 2; level <= 99; level++ {
		xp := GetExpByLevel(level)
		if got := GetLevelByExp(xp); got != level {
			t.Errorf("roundtrip level=%d xp=%d GetLevelByExp=%d", level, xp, got)
		}
	}
}

func TestMaxSkillXP(t *testing.T) {
	if MaxSkillXP != 130344310 {
		t.Errorf("MaxSkillXP: got %d, want 130344310", MaxSkillXP)
	}
	if MaxSkillXP != GetExpByLevel(99) {
		t.Errorf("MaxSkillXP (%d) must equal GetExpByLevel(99) (%d)", MaxSkillXP, GetExpByLevel(99))
	}
}

func TestPlayerStatMap_AllNamesResolveToValidIndices(t *testing.T) {
	// Mirrors TS PlayerStat.ts:25-47. Every name in PlayerStatMap must
	// map to its corresponding PlayerStat* constant.
	cases := map[string]int{
		"ATTACK":      PlayerStatAttack,
		"DEFENCE":     PlayerStatDefence,
		"STRENGTH":    PlayerStatStrength,
		"HITPOINTS":   PlayerStatHitpoints,
		"RANGED":      PlayerStatRanged,
		"PRAYER":      PlayerStatPrayer,
		"MAGIC":       PlayerStatMagic,
		"COOKING":     PlayerStatCooking,
		"WOODCUTTING": PlayerStatWoodcutting,
		"FLETCHING":   PlayerStatFletching,
		"FISHING":     PlayerStatFishing,
		"FIREMAKING":  PlayerStatFiremaking,
		"CRAFTING":    PlayerStatCrafting,
		"SMITHING":    PlayerStatSmithing,
		"MINING":      PlayerStatMining,
		"HERBLORE":    PlayerStatHerblore,
		"AGILITY":     PlayerStatAgility,
		"THIEVING":    PlayerStatThieving,
		"STAT18":      PlayerStat18,
		"STAT19":      PlayerStat19,
		"RUNECRAFT":   PlayerStatRunecraft,
	}
	if len(PlayerStatMap) != len(cases) {
		t.Fatalf("PlayerStatMap len = %d, want %d", len(PlayerStatMap), len(cases))
	}
	for name, want := range cases {
		got, ok := PlayerStatMap[name]
		if !ok {
			t.Errorf("PlayerStatMap[%q] missing", name)
			continue
		}
		if got != want {
			t.Errorf("PlayerStatMap[%q] = %d, want %d", name, got, want)
		}
	}
}

func TestPlayerStatEnabled_MatchesTSPattern(t *testing.T) {
	// TS PlayerStat.ts:53. False only at STAT18, STAT19.
	want := [PlayerStatCount]bool{
		true, true, true, true, true, true, true, true, true, true,
		true, true, true, true, true, true, true, true, false, false, true,
	}
	if PlayerStatEnabled != want {
		t.Errorf("PlayerStatEnabled = %v, want %v", PlayerStatEnabled, want)
	}
}

func TestPlayerStatFree_MatchesTS(t *testing.T) {
	// TS PlayerStat.ts:55. False at Fletching (9), Herblore (15),
	// Agility (16), Thieving (17), STAT18 (18), STAT19 (19); true elsewhere.
	want := [PlayerStatCount]bool{
		true, true, true, true, true, true, true, true, true, false,
		true, true, true, true, true, false, false, false, false, false, true,
	}
	if PlayerStatFree != want {
		t.Errorf("PlayerStatFree = %v, want %v", PlayerStatFree, want)
	}
	// Sanity: f2p sum at all-99 = 15 × 99 = 1485. This is the sentinel
	// AddXP checks for the "you beat f2p!" Adventure log entry
	// (TS Player.ts:1800-1802).
	sum := 0
	for i := range PlayerStatCount {
		if PlayerStatFree[i] {
			sum += 99
		}
	}
	if sum != 1485 {
		t.Errorf("PlayerStatFree all-99 sum = %d, want 1485", sum)
	}
}

func TestPlayerStatNames_AllLowercaseAndMatchMap(t *testing.T) {
	// PlayerStatNames is pre-lowercased; every PlayerStatMap key (uppercase)
	// must lowercase to the array entry at the same index. Pins single
	// source of truth between the two tables.
	for upper, idx := range PlayerStatMap {
		lower := strings.ToLower(upper)
		if PlayerStatNames[idx] != lower {
			t.Errorf("PlayerStatNames[%d] = %q, want %q (lowercase of PlayerStatMap key %q)",
				idx, PlayerStatNames[idx], lower, upper)
		}
	}
}
