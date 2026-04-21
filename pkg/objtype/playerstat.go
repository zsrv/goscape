package objtype

import "math"

// PlayerStat* are indices into Player.levels, Player.baseLevels, and
// Player.stats[XP] for player-skill slots. Index values match TS PlayerStat
// enum (PlayerStat.ts).
const (
	PlayerStatAttack      = 0
	PlayerStatDefence     = 1
	PlayerStatStrength    = 2
	PlayerStatHitpoints   = 3
	PlayerStatRanged      = 4
	PlayerStatPrayer      = 5
	PlayerStatMagic       = 6
	PlayerStatCooking     = 7
	PlayerStatWoodcutting = 8
	PlayerStatFletching   = 9
	PlayerStatFishing     = 10
	PlayerStatFiremaking  = 11
	PlayerStatCrafting    = 12
	PlayerStatSmithing    = 13
	PlayerStatMining      = 14
	PlayerStatHerblore    = 15
	PlayerStatAgility     = 16
	PlayerStatThieving    = 17
	PlayerStat18          = 18 // unused in RS2-225 era; kept for index parity with TS
	PlayerStat19          = 19 // unused in RS2-225 era; kept for index parity with TS
	PlayerStatRunecraft   = 20

	PlayerStatCount = 21
)

// levelExperience holds the XP threshold to reach level (i+2) at index i.
// Built once at package init from the canonical RS XP formula. Matches TS
// levelExperience (Player.ts:77-85). XP is stored as fixed-point tenths
// (×10) so increments can be fractional (e.g. 0.1 XP from a half-cooked food).
var levelExperience [99]int

func init() {
	acc := 0
	for i := 0; i < 99; i++ {
		level := i + 1
		delta := int(math.Floor(float64(level) + math.Pow(2.0, float64(level)/7.0)*300.0))
		acc += delta
		levelExperience[i] = (acc / 4) * 10
	}
}

// GetExpByLevel returns the XP threshold required to reach `level`. Matches
// TS Player.getExpByLevel (Player.ts:97-99).
//
// Boundary handling diverges from TS for safety:
//   - level <= 1 returns 0 (TS returns undefined → NaN-cascade)
//   - level > 99 clamps to level-99 XP (TS returns undefined)
//
// These defensive clamps match the same convention as Player.Damage (S6e)
// and *Npc.Damage (S6c) negative-amount clamps.
func GetExpByLevel(level int) int {
	if level <= 1 {
		return 0
	}
	if level > 99 {
		level = 99
	}
	return levelExperience[level-2]
}
