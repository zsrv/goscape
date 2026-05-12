package world

// MoveSpeed describes a player or NPC's movement mode.
type MoveSpeed int

const (
	MoveSpeedStationary MoveSpeed = iota
	MoveSpeedCrawl
	MoveSpeedWalk
	MoveSpeedRun
	MoveSpeedInstant
)

// MoveRestrict controls which surfaces an entity may walk on.
type MoveRestrict int

const (
	MoveRestrictNormal MoveRestrict = iota
	MoveRestrictBlocked
	MoveRestrictIndoors
	MoveRestrictOutdoors
	MoveRestrictNoMove
	MoveRestrictPassthru
)

// MoveStrategy selects between SMART (pathfinder-routed) and NAIVE (straight-line) movement.
type MoveStrategy int

const (
	MoveStrategySmart MoveStrategy = iota
	MoveStrategyNaive
)

// BlockWalk controls whether an entity blocks others from walking through its tile.
type BlockWalk int

const (
	BlockWalkNone BlockWalk = iota
	BlockWalkNpc
	BlockWalkAll
)

// stepStatus is the tri-state return classification from (*Npc).stepOnce
// and (*Player).stepOnce. Mirrors TS PathingEntity.takeStep's
// (number | null) return where the wrapper (validateAndAdvanceStep)
// dispatches on the value:
//
//	stepBlocked = TS null   → transient block; waypointIndex preserved (NAI-176 D2)
//	stepDone    = TS -1     → waypoint reached or no-move; wrapper decrements + recurses
//	stepMoved   = TS number → position applied inline; wrapper returns dir
//
// Mirrors PathingEntity.ts:617-683 (takeStep) + 202-232 (validateAndAdvanceStep).
type stepStatus int

const (
	stepMoved stepStatus = iota
	stepDone
	stepBlocked
)

// entity is implemented by all targetable game objects.
// Sub-spec 2 only has *Player; sub-specs 3+ add Npc, Loc, Obj.
type entity interface {
	Slot() int
	Coords() (x, z, level int)
	IsValid() bool // NAI-11 — intrinsic validity; zone-membership is checked separately
}
