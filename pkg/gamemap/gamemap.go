package gamemap

import (
	"log/slog"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// GameMap holds collision data for the game world.
type GameMap struct {
	Pathfinder *routefinder.PathFinderAPI
	multimap   map[int]bool // packed zone coord -> multi combat
	freemap    map[int]bool // packed zone coord -> F2P
	log        *slog.Logger
}

func New(log *slog.Logger) *GameMap {
	pf := routefinder.NewPathFinderAPI()
	return &GameMap{
		Pathfinder: &pf,
		multimap:   make(map[int]bool),
		freemap:    make(map[int]bool),
		log:        log,
	}
}

// ChangeLandCollision marks or clears a floor tile as walkable.
func (gm *GameMap) ChangeLandCollision(x, z, level int, add bool) {
	gm.Pathfinder.ChangeFloor(x, z, level, add)
}

// ChangeLocCollision updates collision for a wall/scenery piece.
// Uses loc.LayerOf to route the call to the appropriate pathfinder method.
func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, x, z, level int, add bool) {
	layer := loc.LayerOf(loc.Shape(shape))
	switch layer {
	case loc.LayerWall:
		gm.Pathfinder.ChangeWall(x, z, level, angle, shape, blocksRange, false, add)
	case loc.LayerGround:
		gm.Pathfinder.ChangeLoc(x, z, level, width, length, blocksRange, false, add)
	}
	// LayerWallDecor and LayerGroundDecor do not affect collision.
}

// ChangeNPCCollision marks or clears an NPC's occupied tiles.
func (gm *GameMap) ChangeNPCCollision(size, x, z, level int, add bool) {
	gm.Pathfinder.ChangeNPC(x, z, level, size, add)
}

// ChangePlayerCollision marks or clears a player's occupied tile.
func (gm *GameMap) ChangePlayerCollision(size, x, z, level int, add bool) {
	gm.Pathfinder.ChangePlayer(x, z, level, size, add)
}

// ChangeRoofCollision marks or clears a roof.
func (gm *GameMap) ChangeRoofCollision(x, z, level int, add bool) {
	gm.Pathfinder.ChangeRoof(x, z, level, add)
}

// CanTravel tests whether moving from (x, z, level) with the given offset
// (offsetX, offsetZ) to an adjacent tile is allowed. offsetX/offsetZ must be in {-1, 0, 1}.
func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ int) bool {
	return gm.Pathfinder.StepValidator.CanTravel(
		level, x, z, offsetX, offsetZ, 1, 0, collision.TypeNormal,
	)
}
