package world

import (
	"fmt"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

// serverLocOps adapts *Server to the script.LocOps interface so script
// handlers can drive World mutations without leaking the *entity.Loc
// concrete type into pkg/script. Type-asserts script.ActiveLoc inputs
// to *entity.Loc; non-Loc inputs error out.
//
// Server initialises a single shared instance at startup (s.locOps);
// every script-state builder assigns state.LocOps = s.locOps.
type serverLocOps struct {
	s *Server
}

func (o *serverLocOps) ChangeLoc(loc script.ActiveLoc, typ, shape, angle, duration int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.ChangeLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.ChangeLoc(l, typ, shape, angle, duration)
	return nil
}

func (o *serverLocOps) AddLoc(level, x, z, typ, shape, angle, duration int) (script.ActiveLoc, error) {
	lt := o.s.locTypeOrNil(typ)
	if lt == nil {
		return nil, fmt.Errorf("LocOps.AddLoc: unknown loc id %d", typ)
	}
	width := lt.Width
	if width == 0 {
		width = 1
	}
	length := lt.Length
	if length == 0 {
		length = 1
	}
	created := entitypkg.NewLoc(level, x, z, width, length, entitypkg.LifecycleDespawn, typ, shape, angle)
	o.s.AddLoc(created, duration)
	return created, nil
}

func (o *serverLocOps) RemoveLoc(loc script.ActiveLoc, duration int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.RemoveLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.RemoveLoc(l, duration)
	return nil
}

func (o *serverLocOps) AnimLoc(loc script.ActiveLoc, seq int) error {
	l, ok := loc.(*entitypkg.Loc)
	if !ok {
		return fmt.Errorf("LocOps.AnimLoc: ActiveLoc is %T, not *entity.Loc", loc)
	}
	o.s.AnimLoc(l, seq)
	return nil
}

// LocsAtCoord returns the script-side ActiveLoc slice for every loc
// currently in the zone at (level, x, z). NAI-86 LOC_ADD same-layer
// search is the sole caller.
func (o *serverLocOps) LocsAtCoord(level, x, zc int) []script.ActiveLoc {
	z := o.s.zoneMap.Get(level, x, zc)
	out := make([]script.ActiveLoc, 0, len(z.Locs))
	for _, l := range z.Locs {
		if l.X == x && l.Z == zc && l.Level == level {
			out = append(out, l)
		}
	}
	return out
}

// AllLocsInZone returns the script-side ActiveLoc slice for every loc
// in the zone owning (level, x, zc), without any per-tile filter.
// MAP_LOCADDUNSAFE (NAI-114) consumes this for footprint-overlap
// probing; the handler does the per-loc (x, z, layer, footprint)
// checks itself.
func (o *serverLocOps) AllLocsInZone(level, x, zc int) []script.ActiveLoc {
	z := o.s.zoneMap.Get(level, x, zc)
	out := make([]script.ActiveLoc, 0, len(z.Locs))
	for _, l := range z.Locs {
		out = append(out, l)
	}
	return out
}
