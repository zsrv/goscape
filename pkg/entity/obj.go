package entity

// ObjReveal is the number of ticks a private drop stays private before
// automatically becoming visible to all players. Mirrors TS Obj.REVEAL = 100.
const ObjReveal = 100

// Obj is a ground item — an entry in the game-world ground-layer inventory.
type Obj struct {
	NonPathing

	// Construction properties.
	Type  int // ObjType id
	Count int // stack size

	// Runtime state.
	ReceiverID int // -1 = public; else the owning player's slot
	Reveal     int // tick countdown until OBJ_REVEAL fires; -1 if already public
	LastChange int // last tick Count was modified; -1 if never
}

// NewObj constructs a 1×1 ground item with public visibility by default
// (ReceiverID -1, Reveal -1). Callers that drop a private item must set
// ReceiverID and Reveal after construction.
func NewObj(level, x, z int, lc Lifecycle, typ, count int) *Obj {
	o := &Obj{
		Type: typ, Count: count,
		ReceiverID: -1, Reveal: -1, LastChange: -1,
	}
	o.Entity = NewEntity(level, x, z, 1, 1, lc)
	return o
}

// Slot returns -1 because objs are not slot-indexed (unlike Players
// and Npcs which live in server-wide slot registries). Mirrors
// *entity.Loc.Slot. Required for the world.entity interface so
// objs can be assigned to Npc.huntTarget.
func (o *Obj) Slot() int { return -1 }

// Coords returns the obj's tile position. Reads X/Z/Level from the
// embedded entity.Entity (see entity.go:6-12). Required for the
// world.entity interface.
func (o *Obj) Coords() (x, z, level int) {
	return o.X, o.Z, o.Level
}
