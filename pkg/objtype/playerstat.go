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

// PlayerStatMap maps uppercase stat name → stat index. Mirrors TS
// PlayerStatMap (PlayerStat.ts:25-47). Used by ::setstat / ::advancestat
// cheat parsing in modules/world/handlers_game.go (NAI-184).
var PlayerStatMap = map[string]int{
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

// PlayerStatEnabled mirrors TS PlayerStat.ts:53. False entries (STAT18,
// STAT19) mark unused 2004-era reserved slots. The array is exported as
// a sentinel mainly for its length (PlayerStatEnabled.length = 21 in
// TS) — values are advisory only; ::minme iterates all 21 indices
// without filtering on this array (TS ClientCheatHandler.ts:432-440).
var PlayerStatEnabled = [PlayerStatCount]bool{
	true, true, true, true, true, true, true, true, true, true,
	true, true, true, true, true, true, true, true, false, false, true,
}

// PlayerStatFree mirrors TS PlayerStat.ts:55 — true for free-to-play
// skills, false for members-only. Used by AddXP's "you beat f2p!"
// sentinel: sum of baseLevels[i] across true entries at all-99 = 15×99
// = 1485 (TS Player.ts:1800-1802).
var PlayerStatFree = [PlayerStatCount]bool{
	true, true, true, true, true, true, true, true, true, false,
	true, true, true, true, true, false, false, false, false, false, true,
}

// PlayerStatNames maps stat index → pre-lowercased skill name. Used by
// AddXP's "Levelled up <skill> from N to M" ADVENTURE session-log entry
// (TS Player.ts:1775). TS stores uppercase names in PlayerStatNameMap
// and calls .toLowerCase() at use-site; goscape pre-lowercases the
// storage since the only consumer (AddXP) needs the lowercase form.
// PlayerStatMap remains the authoritative uppercase mapping for
// name→index lookups.
var PlayerStatNames = [PlayerStatCount]string{
	"attack", "defence", "strength", "hitpoints", "ranged",
	"prayer", "magic", "cooking", "woodcutting", "fletching",
	"fishing", "firemaking", "crafting", "smithing", "mining",
	"herblore", "agility", "thieving", "stat18", "stat19", "runecraft",
}

// levelExperience holds the XP threshold to reach level (i+2) at index i.
// Built once at package init from the canonical RS XP formula. Matches TS
// levelExperience (Player.ts:77-85). XP is stored as fixed-point tenths
// (×10) so increments can be fractional (e.g. 0.1 XP from a half-cooked food).
var levelExperience [99]int

func init() {
	acc := 0
	for i := range 99 {
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
