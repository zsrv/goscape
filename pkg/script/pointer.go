package script

// Pointer is a bitmask of active-entity slot flags. Handlers assert the required
// pointer is set before accessing the corresponding active entity.
type Pointer uint32

// Bit positions mirror the TS ScriptPointer enum ordinals
// (ScriptPointer.ts:8-18) exactly, where TS stores the mask as
// `1 << ScriptPointer` (ScriptState.ts:165). The mask is internal-only
// (never serialized to wire or disk), but matching TS keeps the
// citations literally true and avoids confusion. PtrFindDb is a goscape
// extension — TS has no DB pointer in the enum — so it takes the first
// free bit after TS's _LAST sentinel (ordinal 10).
const (
	PtrActivePlayer  Pointer = 1 << 0 // TS ScriptPointer.ActivePlayer (ordinal 0)
	PtrActivePlayer2 Pointer = 1 << 1 // TS ActivePlayer2 (ordinal 1)

	// PtrProtectedActivePlayer is the slot-0 protect flag — TS
	// ProtectedActivePlayer (ordinal 2). Set by `Init` when
	// `protect=true` and `self != nil`, by P_FINDUID success on
	// intOperand=0, and cleared by Player.CloseModal. NAI-133.
	PtrProtectedActivePlayer Pointer = 1 << 2

	// PtrProtectedActivePlayer2 is the slot-1 protect flag — TS
	// ProtectedActivePlayer2 (ordinal 3). Set ONLY by P_FINDUID success
	// on intOperand=1; TS never sets this from the engine. NAI-133.
	PtrProtectedActivePlayer2 Pointer = 1 << 3

	PtrActiveNpc  Pointer = 1 << 4 // TS ActiveNpc (ordinal 4)
	PtrActiveNpc2 Pointer = 1 << 5 // TS ActiveNpc2 (ordinal 5)
	PtrActiveLoc  Pointer = 1 << 6 // TS ActiveLoc (ordinal 6)
	PtrActiveLoc2 Pointer = 1 << 7 // TS ActiveLoc2 (ordinal 7)
	PtrActiveObj  Pointer = 1 << 8 // TS ActiveObj (ordinal 8)
	PtrActiveObj2 Pointer = 1 << 9 // TS ActiveObj2 (ordinal 9)

	// PtrFindDb is goscape-only (no TS enum slot). S7g: DB_FIND* /
	// DB_LISTALL* set it; DB_FINDNEXT / DB_FIND_REFINE require it.
	PtrFindDb Pointer = 1 << 10
)

// NAI-11 aliases for the reserved *2 slot flags, named after TS's
// runNpcScript semantics.
const (
	// PtrOtherActiveNpc aliases PtrActiveNpc2. Used by NAI-11's
	// runNpcScript target type-dispatch for NPC→NPC AI targets.
	PtrOtherActiveNpc = PtrActiveNpc2
)
