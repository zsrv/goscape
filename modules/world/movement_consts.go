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

// MoveRestrict controls which surfaces an entity may walk on. The values are
// the canonical Engine-TS MoveRestrict.ts numbering (also mirrored in
// pkg/objtype/npctype.go) — npc.go casts the raw config byte straight across
// (MoveRestrict(typ.MoveRestrict)), so this ordering MUST stay 1:1 with the
// config decoder or every npc with moverestrict >= BLOCKED_NORMAL is misread.
type MoveRestrict int

const (
	MoveRestrictNormal MoveRestrict = iota
	MoveRestrictBlocked
	MoveRestrictBlockedNormal
	MoveRestrictIndoors
	MoveRestrictOutdoors
	MoveRestrictNoMove
	MoveRestrictPassthru
)

// MoveStrategy selects between SMART (pathfinder-routed), NAIVE (straight-line),
// and FLY (collision-bypassing) movement.
type MoveStrategy int

const (
	MoveStrategySmart MoveStrategy = iota
	MoveStrategyNaive
	MoveStrategyFly
)

// BlockWalk controls whether an entity blocks others from walking through its tile.
type BlockWalk int

const (
	BlockWalkNone BlockWalk = iota
	BlockWalkNpc
	BlockWalkAll
)

// entity is implemented by all targetable game objects.
// Sub-spec 2 only has *Player; sub-specs 3+ add Npc, Loc, Obj.
type entity interface {
	Slot() int
	Coords() (x, z, level int)
	IsValid() bool // NAI-11 — intrinsic validity; zone-membership is checked separately
}
