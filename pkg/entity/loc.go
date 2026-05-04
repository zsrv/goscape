package entity

import (
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// Loc is a scenery object: a door, a tree, a tile trap. Its render fields
// are packed into two 32-bit Info words. BaseInfo is set at construction and
// never mutated; CurrentInfo starts equal to BaseInfo and is rewritten by
// Change. The split mirrors TS Loc.baseInfo / Loc.currentInfo
// (Engine-TS/src/engine/entity/Loc.ts:9-12) and gives IsChanged its meaning.
type Loc struct {
	NonPathing
	BaseInfo    int  // immutable: packed (type, shape, angle, layer)
	CurrentInfo int  // mutable: equals BaseInfo at construction; rewritten by Change
	IsActive    bool // true after Server.AddLoc, false after Server.RemoveLoc
}

// NewLoc constructs a Loc at (level, x, z) with the given footprint and
// packed render fields. Returns a pointer so callers can call Change()
// in place. Wires the NonPathing back-pointer so the lifecycle tracker
// can recover the *Loc from a *NonPathing handle (NAI-86 Bundle 2).
func NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc {
	info := packLocInfo(typ, shape, angle)
	l := &Loc{BaseInfo: info, CurrentInfo: info}
	l.Entity = NewEntity(level, x, z, width, length, lc)
	l.parent = l
	return l
}

// packLocInfo combines the four render fields into a single int using the
// bit layout shared with the TS reference (Loc.ts:20-24):
//
//	[type:14][shape:5][angle:2][layer:2]
//
// Layer is derived from shape via pkg/pathfinder/loc.LayerOf and stored in
// BaseInfo so Layer() never changes after construction (mirrors TS layer
// reading from baseInfo). Out-of-range inputs are silently masked.
func packLocInfo(typ, shape, angle int) int {
	maskedShape := shape & 0x1F
	layer := loc.LayerOf(loc.Shape(maskedShape))
	return (typ & 0x3FFF) |
		maskedShape<<14 |
		(angle&0x3)<<19 |
		(int(layer)&0x3)<<21
}

// Type returns the LocType id (bits 0..13 of CurrentInfo).
func (l *Loc) Type() int { return l.CurrentInfo & 0x3FFF }

// Shape returns the loc shape (bits 14..18 of CurrentInfo).
func (l *Loc) Shape() int { return (l.CurrentInfo >> 14) & 0x1F }

// Angle returns the loc rotation (bits 19..20 of CurrentInfo).
func (l *Loc) Angle() int { return (l.CurrentInfo >> 19) & 0x3 }

// Layer returns the loc shape's render layer (bits 21..22 of BaseInfo).
// Mirrors TS Loc.layer reading from baseInfo (Loc.ts:42-44).
func (l *Loc) Layer() int { return (l.BaseInfo >> 21) & 0x3 }

// IsChanged reports whether the loc's CurrentInfo has been mutated away
// from BaseInfo. Mirrors TS Loc.isChanged (Loc.ts:26-28).
func (l *Loc) IsChanged() bool { return l.CurrentInfo != l.BaseInfo }

// Change rewrites CurrentInfo to the packing of (typ, shape, angle).
// Mirrors TS Loc.change (Loc.ts:46-48). BaseInfo is not touched.
func (l *Loc) Change(typ, shape, angle int) {
	l.CurrentInfo = packLocInfo(typ, shape, angle)
}

// Revert restores CurrentInfo to BaseInfo. Mirrors TS Loc.revert (Loc.ts:50-52).
func (l *Loc) Revert() { l.CurrentInfo = l.BaseInfo }

// LocType returns the LocType ID for this loc. Satisfies the
// pkg/script.ActiveLoc interface. Alias for Type() with a less-ambiguous
// name when the loc is bound to script state.
func (l *Loc) LocType() int { return l.Type() }

// Slot returns -1 because locs are not slot-indexed (unlike Players and
// Npcs which live in server-wide slot registries). Required for the
// world.entity interface so locs can be assigned to Player.target.
func (l *Loc) Slot() int { return -1 }

// Coords returns the loc's tile position. Required for the world.entity
// interface. Reads X/Z/Level from the embedded entity.Entity (see
// entity.go:6-12 for the field layout); no allocation.
func (l *Loc) Coords() (x, z, level int) {
	return l.X, l.Z, l.Level
}

// IsValid returns the loc's intrinsic validity. Zone-membership
// (pointer still in zoneMap.Get(level,x,z).Locs) is checked separately
// by world-module helpers at the validateTarget call site, because
// pkg/entity cannot depend on modules/world. The "in world right now"
// check that gates Loc.Turn branches lives on IsActive.
func (l *Loc) IsValid() bool { return true }
