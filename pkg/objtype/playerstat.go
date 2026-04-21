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

// MaxSkillXP is the XP threshold to reach level 99 (the game's max level).
// Equal to levelExperience[98] = GetExpByLevel(99) = 130344310. XP is
// stored as fixed-point tenths (×10), so this represents 13,034,431 "real"
// XP — the canonical RS2 level-99 XP value.
//
// NOTE: this is the THRESHOLD for reaching level 99, NOT the XP accumulation
// ceiling. Players can accumulate XP beyond this up to MaxXP. Use MaxSkillXP
// for level-up derivation and combat-level inputs; use MaxXP for XP-cap
// clamps in AddXP and similar accumulation paths.
const MaxSkillXP = 130344310

// MaxXP is the engine-level XP accumulation ceiling — 200,000,000 "real" XP
// stored as 2,000,000,000 in ×10 fixed-point (fits in int32 with headroom).
// Matches TS Player.ts:1754-1757 comment: "cap to 200m, this is represented
// as '2 billion' because we use 32-bit signed integers and divide by 10 to
// give us a decimal point."
//
// Use this for AddXP's clamp, NOT MaxSkillXP. A level-99 player still
// accumulates XP up to MaxXP for prestige / XP-chase gameplay.
const MaxXP = 2_000_000_000

// GetLevelByExp returns the highest level whose XP threshold is <= xp, or 1
// if xp is below any threshold. Clamped at level 99. Matches TS
// Player.getLevelByExp (Player.ts:87-95). xp is the fixed-point tenths
// value (scaled ×10), consistent with GetExpByLevel.
//
// Negative xp returns 1 (defensive — no threshold is negative, so the loop
// falls through to the `return 1` tail).
func GetLevelByExp(xp int) int {
	for i := 98; i >= 0; i-- {
		if xp >= levelExperience[i] {
			level := i + 2
			if level > 99 {
				level = 99
			}
			return level
		}
	}
	return 1
}
