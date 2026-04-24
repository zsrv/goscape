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
)

// NAI-11 aliases for the reserved *2 slot flags, named after TS's
// runNpcScript semantics.
const (
	// PtrOtherActiveNpc aliases PtrActiveNpc2. Used by NAI-11's
	// runNpcScript target type-dispatch for NPC→NPC AI targets.
	PtrOtherActiveNpc = PtrActiveNpc2
)
