package flag

type BlockAccess int

const (
	BlockAccessNorth BlockAccess = 1 << iota
	BlockAccessEast
	BlockAccessSouth
	BlockAccessWest
)
