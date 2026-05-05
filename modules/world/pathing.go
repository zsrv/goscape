package world

// pathingEntity is the dimensioned entity interface used by pathToTarget's
// type-switch and SMART/NAIVE branch dispatch. Mirrors TS PathingEntity's
// (width, length) inheritance from the Entity base. *Player and *Npc are
// the two concrete implementations.
type pathingEntity interface {
	entity
	Width() int
	Length() int
}
