package script

// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
// LOC_DEL / LOC_ANIM / LOC_FIND / MAP_LOCADDUNSAFE. Implementations live
// in modules/world (see script_loc_ops.go). Decouples pkg/script from
// world-side entity types; handlers pass the script-side ActiveLoc
// interface, the adapter type-asserts to the concrete *entity.Loc.
//
// LocsAtCoord returns the slice of locs at (level, x, z) for the
// LOC_ADD same-layer search branch. Returning a slice (not iter.Seq)
// keeps the interface simple — the call site iterates synchronously
// and the slice is small (≤4 layers per tile in practice).
//
// AllLocsInZone returns every loc in the zone owning (level, x, z),
// without any per-tile filtering. NAI-114 MAP_LOCADDUNSAFE consumes
// this for footprint-overlap probing; the handler does the per-loc
// (x, z, layer, footprint) checks itself per TS
// ServerOps.ts:212-252.
//
// GetLoc returns the loc at (level, x, z) whose type matches typ, or
// nil if no such loc exists. Mirrors TS World.getLoc → Zone.getLoc
// exact-tile + type-equality semantics (Zone.ts:259-266). Sole caller
// is LOC_FIND (LocOps.ts:79-94).
type LocOps interface {
	ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
	AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
	RemoveLoc(loc ActiveLoc, duration int) error
	AnimLoc(loc ActiveLoc, seq int) error
	LocsAtCoord(level, x, z int) []ActiveLoc
	AllLocsInZone(level, x, z int) []ActiveLoc
	GetLoc(level, x, z, typ int) ActiveLoc
}
