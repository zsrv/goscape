package routefinder

import (
	"testing"
)

// lineRouteBenchSink prevents the compiler from eliminating ray-cast calls
// in benchmarks: assigning Coordinates here forces the slice to escape so
// allocation behavior of LineRouteFinder.RayCast is observed honestly.
var lineRouteBenchSink []RouteCoordinates

// benchLineRouteFinder loads the long-path benchmark dataset so the
// flag map has real cleared zones. Without flagged zones the ray-cast
// returns FlagNull -> early failure with empty coords (every other
// flag check is a hit) so the slice never grows.
func benchLineRouteFinder(b *testing.B) (LineRouteFinder, RouteFindingParameters) {
	b.Helper()
	params, err := RouteFindingParametersFromFile("testdata/benchmarks/long-path.json")
	if err != nil {
		b.Fatal(err)
	}
	return NewLineRouteFinder(params.ToCollisionFlags()), params
}

// BenchmarkLineRouteFinderShortRayCast measures a short successful ray-cast
// (~5 tiles) over the long-path collision map.
func BenchmarkLineRouteFinderShortRayCast(b *testing.B) {
	rf, p := benchLineRouteFinder(b)
	dstX, dstZ := p.SrcX+4, p.SrcZ+3
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		rc := rf.LineOfSight(p.Level, p.SrcX, p.SrcZ, dstX, dstZ, 1, 1, 1, 0)
		lineRouteBenchSink = rc.Coordinates
	}
}

// BenchmarkLineRouteFinderMediumRayCast measures a ~30-tile ray-cast.
func BenchmarkLineRouteFinderMediumRayCast(b *testing.B) {
	rf, p := benchLineRouteFinder(b)
	dstX, dstZ := p.SrcX+30, p.SrcZ+25
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		rc := rf.LineOfSight(p.Level, p.SrcX, p.SrcZ, dstX, dstZ, 1, 1, 1, 0)
		lineRouteBenchSink = rc.Coordinates
	}
}

// BenchmarkLineRouteFinderLongRayCast measures a long ray-cast across the
// map (the Coordinates slice can reach ~200 entries -- one or two per tile
// along the major axis -- so this stresses the pre-allocation / pool path).
func BenchmarkLineRouteFinderLongRayCast(b *testing.B) {
	rf, p := benchLineRouteFinder(b)
	dstX, dstZ := p.DestX, p.DestZ
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		rc := rf.LineOfSight(p.Level, p.SrcX, p.SrcZ, dstX, dstZ, 1, 1, 1, 0)
		lineRouteBenchSink = rc.Coordinates
	}
}
