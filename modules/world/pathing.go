package world

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// pathingEntity is the dimensioned entity interface used by pathToTarget's
// type-switch and SMART/NAIVE branch dispatch. Mirrors TS PathingEntity's
// (width, length) inheritance from the Entity base. *Player and *Npc are
// the two concrete implementations.
type pathingEntity interface {
	entity
	Width() int
	Length() int
}

// pathfinderForTarget is the interface consumed by pathToTarget's
// SMART/NAIVE dispatch. Production: *routefinder.PathFinderAPI satisfies
// it via the four wrapper methods added in NAI-92 B1 T1.3. Tests inject
// a recorder via Server.testPathfinder.
type pathfinderForTarget interface {
	FindPathPlain(level, srcX, srcZ, destX, destZ int) routefinder.Route
	FindPathToEntity(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength int) routefinder.Route
	FindPathToLoc(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags int) routefinder.Route
	FindNaivePath(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag int, collisionType collision.Type) routefinder.Route
}

// pathfinder returns the pathfinder used by pathToTarget dispatch.
// Production: s.gamemap.Pathfinder. Tests: s.testPathfinder (injected).
// Returns nil if no pathfinder is available (gamemap uninitialized and
// no test seam); callers in pathToTargetSmart must guard the nil case.
func (s *Server) pathfinder() pathfinderForTarget {
	if s.testPathfinder != nil {
		return s.testPathfinder
	}
	if s.gamemap != nil {
		return s.gamemap.Pathfinder
	}
	return nil
}

// pathfinder returns the pathfinder for an Npc-anchored dispatch. Returns
// nil when n.server or the server's pathfinder (gamemap and test seam) are
// absent.
func (n *Npc) pathfinder() pathfinderForTarget {
	if n == nil || n.server == nil {
		return nil
	}
	return n.server.pathfinder()
}
