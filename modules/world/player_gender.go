package world

// maleFemaleMap is the male-body-idkit → female-body-idkit lookup used by
// (*Player).SetGender when gender == 1 (target female). Mirrors TS
// Player.MALE_FEMALE_MAP at Engine-TS/src/engine/entity/Player.ts:110-148.
// Sparse: keys are real OSRS male idkit ids; unmapped keys lookup as -1
// (see lookupGenderIdkit).
//
// Lossiness is canonical OSRS: males {18..25} all collapse to female 56;
// males {27, 31} both → female 63; the makeover-mage is not fully
// reversible.
var maleFemaleMap = map[int]int{
	0: 45, 1: 47, 2: 48, 3: 49, 4: 50, 5: 51, 6: 52, 7: 53, 8: 54, 9: 55,
	18: 56, 19: 56, 20: 56, 21: 56, 22: 56, 23: 56, 24: 56, 25: 56,
	26: 61, 27: 63, 28: 62, 29: 65, 30: 64, 31: 63, 32: 66, 33: 67,
	34: 68, 35: 69, 36: 70, 37: 71, 38: 72, 39: 76, 40: 75, 41: 78,
	42: 79, 43: 80, 44: 81,
}

// femaleMaleMap mirrors TS Player.FEMALE_MALE_MAP (Player.ts:150-188).
// See maleFemaleMap doc comment for sparseness + lossiness notes; the
// female→male direction has its own collapse cases ({45, 46}→0;
// {73, 74, 77}→36).
var femaleMaleMap = map[int]int{
	45: 0, 46: 0, 47: 1, 48: 2, 49: 3, 50: 4, 51: 5, 52: 6, 53: 7, 54: 8, 55: 9,
	56: 18, 57: 18, 58: 18, 59: 18, 60: 18,
	61: 26, 62: 27, 63: 28, 64: 29, 65: 29, 66: 32, 67: 33, 68: 34, 69: 35,
	70: 36, 71: 37, 72: 38, 73: 36, 74: 36, 75: 40, 76: 39, 77: 36,
	78: 41, 79: 42, 80: 43, 81: 44,
}

// lookupGenderIdkit returns m[k] when present, -1 otherwise. Mirrors TS
// expression `Map.get(k) ?? -1` at PlayerOps.ts:1109,1115.
func lookupGenderIdkit(m map[int]int, k int) int {
	if v, ok := m[k]; ok {
		return v
	}
	return -1
}

// SetGender rewrites the player's 7-slot body[] idkit array via the
// gender lookup map and writes the gender field. Mirrors TS
// PlayerOps.ts:1104-1118 SETGENDER handler.
//
// Does NOT flip MaskAppearance — TS-faithful deferred-rebuild pattern:
// real content (LostCityRS/Content/scripts/areas/area_falador/scripts/
// makeover_mage.rs2:58-64) follows SETGENDER + SETSKINCOLOUR with
// BUILDAPPEARANCE, which is the explicit rebuild trigger. Mirrors the
// established SetBodyPart precedent.
//
// When gender == 1 (target female): every body slot is rewritten via
// maleFemaleMap; unmapped keys produce -1.
//
// When gender == 0 (target male): slot 1 is hardcoded to 14 (intentional
// TS canon for canonical male hair model, see PlayerOps.ts:1111-1113);
// all other slots are rewritten via femaleMaleMap; unmapped keys
// produce -1.
//
// Deviation NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED: when a body
// slot holds an idkit value not present in the relevant lookup map, the
// slot is overwritten with -1 (idkit "null"). This is TS-literal
// behavior (Map.get() ?? -1 at PlayerOps.ts:1109,1115). Real content
// scripts only invoke SETGENDER from controlled UI flows where the
// player's body already came from the same-direction lookup, so the
// -1 case is content-unreachable today; pinned for future TS sync.
// Pinned by TestPlayerSetGender_UnmappedKeysWriteMinusOne.
func (p *Player) SetGender(gender int) {
	for i := range 7 {
		if gender == 1 {
			p.body[i] = lookupGenderIdkit(maleFemaleMap, p.body[i])
		} else {
			if i == 1 {
				p.body[i] = 14
				continue
			}
			p.body[i] = lookupGenderIdkit(femaleMaleMap, p.body[i])
		}
	}
	p.gender = gender
}
