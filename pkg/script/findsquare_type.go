package script

import "fmt"

// MapFindSquareType selects the line-of-walk / line-of-sight gate for
// MAP_FINDSQUARE (opcode 1009). Mirrors TS MapFindSquareType enum at
// Engine-TS/src/engine/entity/MapFindSquareType.ts. NAI-35-T6.
type MapFindSquareType int

const (
	MapFindSquareNone        MapFindSquareType = 0
	MapFindSquareLineOfWalk  MapFindSquareType = 1
	MapFindSquareLineOfSight MapFindSquareType = 2
)

// checkFindSquareType validates that v is in {0, 1, 2}. Mirrors TS
// FindSquareValid (ScriptValidators.ts). NAI-35-T6.
func checkFindSquareType(v int, op string) error {
	switch MapFindSquareType(v) {
	case MapFindSquareNone, MapFindSquareLineOfWalk, MapFindSquareLineOfSight:
		return nil
	default:
		return fmt.Errorf("%s: invalid find-square type %d", op, v)
	}
}

// checkNumberPositive validates v > 0. Mirrors TS NumberPositive
// (ScriptValidators.ts). NAI-35-T6.
func checkNumberPositive(v int, op string) error {
	if v <= 0 {
		return fmt.Errorf("%s: expected positive number, got %d", op, v)
	}
	return nil
}
