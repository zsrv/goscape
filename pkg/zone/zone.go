package zone

import "github.com/zsrv/goscape/pkg/entity"

// Zone is an 8×8 tile region of the world. See the file-level doc in
// future tasks for full semantics.
type Zone struct {
	Index       int
	X, Z, Level int

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int

	shared []byte
}

// New constructs a zone for the given packed index and (level, zoneX, zoneZ).
func New(index, level, x, z int) *Zone {
	return &Zone{
		Index:        index,
		X:            x,
		Z:            z,
		Level:        level,
		entityEvents: make(map[*entity.NonPathing][]int),
	}
}
