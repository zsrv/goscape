package objtype

// HitType wire values used by the client hitmark encoding and by
// RuneScript callers of NPC_DAMAGE / P_DAMAGE. Mirrors TS
// Engine-TS/src/engine/entity/HitType.ts:1-5.
//
// HitTypeCount is the exclusive upper bound consumed by the
// HitTypeValid range validator (TS ScriptValidators.ts:117 —
// ScriptInputRangeValidator(HitType.BLOCK, HitType.POISON), i.e. the
// inclusive range [0, 2]).
const (
	HitTypeBlock  = 0
	HitTypeDamage = 1
	HitTypePoison = 2

	HitTypeCount = 3
)
