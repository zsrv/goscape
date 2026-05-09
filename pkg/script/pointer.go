package script

// Pointer is a bitmask of active-entity slot flags. Handlers assert the required
// pointer is set before accessing the corresponding active entity.
type Pointer uint32

const (
	PtrActivePlayer  Pointer = 1 << 0
	PtrActivePlayer2 Pointer = 1 << 1
	PtrActiveNpc     Pointer = 1 << 2
	PtrActiveNpc2    Pointer = 1 << 3
	PtrActiveLoc     Pointer = 1 << 4
	PtrActiveLoc2    Pointer = 1 << 5
	PtrActiveObj     Pointer = 1 << 6
	PtrActiveObj2    Pointer = 1 << 7
	PtrFindDb        Pointer = 1 << 8 // S7g: DB_FIND* / DB_LISTALL* set; DB_FINDNEXT / DB_FIND_REFINE require.

	// PtrProtectedActivePlayer is the slot-0 protect flag — TS
	// ProtectedActivePlayer (ScriptPointer.ts:10). Set by `Init` when
	// `protect=true` and `self != nil`, by P_FINDUID success on
	// intOperand=0, and cleared by Player.CloseModal. NAI-133.
	PtrProtectedActivePlayer Pointer = 1 << 9

	// PtrProtectedActivePlayer2 is the slot-1 protect flag — TS
	// ProtectedActivePlayer2 (ScriptPointer.ts:11). Set ONLY by
	// P_FINDUID success on intOperand=1; TS never sets this from the
	// engine. NAI-133.
	PtrProtectedActivePlayer2 Pointer = 1 << 10
)

// NAI-11 aliases for the reserved *2 slot flags, named after TS's
// runNpcScript semantics.
const (
	// PtrOtherActiveNpc aliases PtrActiveNpc2. Used by NAI-11's
	// runNpcScript target type-dispatch for NPC→NPC AI targets.
	PtrOtherActiveNpc = PtrActiveNpc2
)
