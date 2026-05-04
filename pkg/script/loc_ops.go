package script

// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
// LOC_DEL / LOC_ANIM. Implementations live in modules/world (see
// script_loc_ops.go). Decouples pkg/script from world-side entity
// types; handlers pass the script-side ActiveLoc interface, the
// adapter type-asserts to the concrete *entity.Loc.
//
// LocsAtCoord returns the slice of locs at (level, x, z) for the
// LOC_ADD same-layer search branch. Returning a slice (not iter.Seq)
// keeps the interface simple — the call site iterates synchronously
// and the slice is small (≤4 layers per tile in practice).
type LocOps interface {
	ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
	AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
	RemoveLoc(loc ActiveLoc, duration int) error
	AnimLoc(loc ActiveLoc, seq int) error
	LocsAtCoord(level, x, z int) []ActiveLoc
}
