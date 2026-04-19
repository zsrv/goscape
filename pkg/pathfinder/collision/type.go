package collision

type Type int

const (
	TypeNormal Type = iota
	TypeBlocked
	TypeIndoors
	TypeOutdoors
	TypeLineOfSight
)

// TODO: not in kt
