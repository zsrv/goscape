package routefinder

type RayCast struct {
	Coordinates []RouteCoordinates
	Alternative bool
	Success     bool
}

func RayCastFailed() RayCast {
	return RayCast{
		Coordinates: []RouteCoordinates{},
		Alternative: false,
		Success:     false,
	}
}

func RayCastSuccessNoCoords() RayCast {
	return RayCast{
		Coordinates: []RouteCoordinates{},
		Alternative: false,
		Success:     true,
	}
}
