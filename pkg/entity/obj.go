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
	// ReceiverID is UID-space — mirrors TS Engine-TS entity/Obj.ts receiver64.
	// PublicReceiver (-1) for public drops; else the owning player's UID per
	// modules/world.composeUID(username37, slot). Set by worldVarsView.AddObj
	// at modules/world/server_varp.go:169 for private drops.
	ReceiverID int
	Reveal     int // tick countdown until OBJ_REVEAL fires; -1 if already public
	LastChange int // last tick Count was modified; -1 if never

	// IsActive is true while the obj is present in its zone's Objs list.
	// Managed by pkg/zone Zone methods (AddStaticObj, AddObj, RemoveObj).
	// Mirrors TS Zone.ts isActive writes (Zone.ts:208,214,295) and
	// pkg/entity/loc.go:16. NAI-151.
	IsActive bool
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
	o.parent = o
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

// IsValid returns the obj's intrinsic validity. Same layering as Loc:
// zone-membership check lives in the world module.
func (o *Obj) IsValid() bool {
	return true
}

// ObjType returns the obj's type id. Method wrapper around the public
// Type field so *Obj satisfies script.ActiveObj (Go disallows same-name
// field + method, so the method is spelled ObjType).
func (o *Obj) ObjType() int { return o.Type }

// ObjCount returns the obj's current stack size. Method wrapper around
// the public Count field so *Obj satisfies script.ActiveObj. (Go
// disallows same-name field + method; same convention as ObjType().)
func (o *Obj) ObjCount() int { return o.Count }

// IsValidFor reports whether the obj is consumable by the given player
// UID. Mirrors TS Obj.ts:52-62 with goscape's UID-int receiver instead
// of TS bigint hash64. Reveal>-1 means private; non-receiver players
// see invalid. Count<1 means depleted.
//
// NAI-153-D2: TS uses hash64 (bigint username hash); goscape uses
// ReceiverID = composeUID(username37, slot) per
// modules/world/server_varp.go:169.
//
// Distinct from the no-arg IsValid() (intrinsic base, always true)
// which satisfies the polymorphic entity interface — Go disallows
// method overloading, so the player-aware variant gets its own name.
func (o *Obj) IsValidFor(playerUID int) bool {
	if o.Reveal > -1 && playerUID != o.ReceiverID {
		return false
	}
	if o.Count < 1 {
		return false
	}
	return true
}

// IsRespawnLifecycle reports whether o is engine-spawned RESPAWN
// lifecycle. Satisfies script.ActiveObj. NAI-178.
func (o *Obj) IsRespawnLifecycle() bool {
	return o.Lifecycle == LifecycleRespawn
}
