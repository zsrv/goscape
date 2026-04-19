package routefinder

type Route struct {
	Waypoints   []RouteCoordinates
	Alternative bool
	Success     bool
}
