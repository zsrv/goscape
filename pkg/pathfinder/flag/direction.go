package flag

const (
	DirectionNorth int = 1 << iota
	DirectionEast
	DirectionSouth
	DirectionWest
)

const (
	DirectionSouthwest = DirectionWest | DirectionSouth
	DirectionNorthwest = DirectionWest | DirectionNorth
	DirectionSoutheast = DirectionEast | DirectionSouth
	DirectionNortheast = DirectionEast | DirectionNorth
)

type DirectionOffset struct {
	OffX int
	OffZ int
}

var CardinalDirectionToOffset = map[int]DirectionOffset{
	DirectionNorth: {OffX: 0, OffZ: 1},
	DirectionEast:  {OffX: 1, OffZ: 0},
	DirectionSouth: {OffX: 0, OffZ: -1},
	DirectionWest:  {OffX: -1, OffZ: 0},
}

var DirectionToOffset = map[int]DirectionOffset{
	DirectionNorth:     {OffX: 0, OffZ: 1},
	DirectionEast:      {OffX: 1, OffZ: 0},
	DirectionSouth:     {OffX: 0, OffZ: -1},
	DirectionWest:      {OffX: -1, OffZ: 0},
	DirectionSouthwest: {OffX: -1, OffZ: -1},
	DirectionNorthwest: {OffX: -1, OffZ: 1},
	DirectionSoutheast: {OffX: 1, OffZ: -1},
	DirectionNortheast: {OffX: 1, OffZ: 1},
}
