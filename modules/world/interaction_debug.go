package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// chebDist returns the Chebyshev distance between two tile coords.
// Used by NAI-79 Stage 1 instrumentation (interaction tick frame).
func chebDist(ax, az, bx, bz int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dz := az - bz
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		return dx
	}
	return dz
}

// targetKindString returns a stable string label for an interaction
// target so the NAI-79 interaction tick frame can name the target type
// without relying on slog's reflection-based formatting. Returns
// "unknown" for nil or unrecognized types.
func targetKindString(t entity) string {
	switch t.(type) {
	case *entitypkg.Loc:
		return "Loc"
	case *entitypkg.Obj:
		return "Obj"
	case *Npc:
		return "Npc"
	case *Player:
		return "Player"
	default:
		return "unknown"
	}
}
