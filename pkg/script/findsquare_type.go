package script

import "fmt"

// MapFindSquareType selects the line-of-walk / line-of-sight gate for
// MAP_FINDSQUARE (opcode 1009). Mirrors TS MapFindSquareType enum at
// Engine-TS/src/engine/entity/MapFindSquareType.ts. NAI-35-T6.
type MapFindSquareType int

const (
	MapFindSquareLineOfWalk  MapFindSquareType = 0
	MapFindSquareLineOfSight MapFindSquareType = 1
	MapFindSquareNone        MapFindSquareType = 2
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

// checkNumberPositive validates v >= 0. Mirrors TS NumberPositive
// (ScriptValidators.ts:43-48) which accepts zero despite the misleading
// name (it really validates non-negative).
func checkNumberPositive(v int, op string) error {
	if v < 0 {
		return fmt.Errorf("%s: input number was negative (%d)", op, v)
	}
	return nil
}
