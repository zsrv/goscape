package routefinder

import "fmt"

type RouteCoordinates struct {
	packed int
}

func (rc RouteCoordinates) X() int {
	return (rc.packed >> 14) & 0x3FFF
}

func (rc RouteCoordinates) Z() int {
	return rc.packed & 0x3FFF
}

func (rc RouteCoordinates) Level() int {
	return (rc.packed >> 28) & 0x3
}

func (rc RouteCoordinates) Translate(xOffset int, zOffset int, levelOffset int) RouteCoordinates {
	return NewRouteCoordinates(rc.X()+xOffset, rc.Z()+zOffset, rc.Level()+levelOffset)
}

func (rc RouteCoordinates) String() string {
	return fmt.Sprintf("RouteCoordinates(x=%d, z=%d, level=%d)", rc.X(), rc.Z(), rc.Level())
}

func NewRouteCoordinates(x int, z int, level int) RouteCoordinates {
	return RouteCoordinates{
		packed: (z & 0x3FFF) | ((x & 0x3FFF) << 14) | ((level & 0x3) << 28),
	}
}
