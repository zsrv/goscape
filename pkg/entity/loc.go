package entity

// Loc is a scenery object: a door, a tree, a tile trap. Its type/shape/angle
// are packed into a single 32-bit Info word for wire-efficient comparison
// between the current state and the cache-loaded base state.
type Loc struct {
	NonPathing
	Info int
}

// NewLoc constructs a Loc at (level, x, z) with the given footprint and
// packed rendering fields. Returns a pointer so callers can mutate Info
// in place (shape changes, angle changes) without re-allocating.
func NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc {
	l := &Loc{Info: packLocInfo(typ, shape, angle)}
	l.Entity = NewEntity(level, x, z, width, length, lc)
	return l
}

// packLocInfo combines the three render fields into a single int using the
// bit layout shared with the TS reference: [type:14][shape:5][angle:2].
// Out-of-range inputs are silently masked.
func packLocInfo(typ, shape, angle int) int {
	return (typ & 0x3FFF) | (shape&0x1F)<<14 | (angle&0x3)<<19
}

// Type returns the ObjType id (bits 0..13).
func (l *Loc) Type() int { return l.Info & 0x3FFF }

// Shape returns the loc shape (bits 14..18).
func (l *Loc) Shape() int { return (l.Info >> 14) & 0x1F }

// Angle returns the loc rotation (bits 19..20).
func (l *Loc) Angle() int { return (l.Info >> 19) & 0x3 }
