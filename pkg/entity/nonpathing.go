package entity

// NonPathing is the shared concrete base for entities that don't walk —
// Locs and Objs. Exists to give zone code a single embedded base that
// future zone-event machinery can key against via interface satisfaction.
type NonPathing struct {
	Entity
}
