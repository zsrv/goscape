package routefinder

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

type RouteFindingParameters struct {
	Level int   `json:"level"`
	SrcX  int   `json:"srcX"`
	SrcZ  int   `json:"srcZ"`
	DestX int   `json:"destX"`
	DestZ int   `json:"destZ"`
	Flags []int `json:"flags"`
}

func (rfp RouteFindingParameters) ToCollisionFlags() collision.FlagMap {
	collisionFlags := collision.NewFlagMap()
	mapSearchSize := int(math.Sqrt(float64(len(rfp.Flags))))
	half := mapSearchSize / 2
	centerX := rfp.SrcX
	centerZ := rfp.SrcZ

	for z := centerZ - half; z < centerZ+half; z++ {
		for x := centerX - half; x < centerX+half; x++ {
			lx := x - (centerX - half)
			lz := z - (centerZ - half)
			index := (lz * mapSearchSize) + lx
			collisionFlags.Set(x, z, rfp.Level, rfp.Flags[index])
		}
	}

	return collisionFlags
}

func RouteFindingParametersFromFile(path string) (RouteFindingParameters, error) {
	var rfp RouteFindingParameters
	data, err := os.ReadFile(path)
	if err != nil {
		return RouteFindingParameters{}, err
	}
	err = json.Unmarshal(data, &rfp)
	if err != nil {
		return RouteFindingParameters{}, err
	}

	return rfp, nil
}

func BenchmarkRouteFinderShortRoute(b *testing.B) {
	// setup
	params, err := RouteFindingParametersFromFile("testdata/benchmarks/short-path.json")
	if err != nil {
		b.Fatal(err)
	}
	rf := NewRouteFinderDefault(params.ToCollisionFlags())

	for b.Loop() {
		rf.FindRouteDefault(params.Level, params.SrcX, params.SrcZ, params.DestX, params.DestZ)
	}
}

func BenchmarkRouteFinderMediumRoute(b *testing.B) {
	// setup
	params, err := RouteFindingParametersFromFile("testdata/benchmarks/med-path.json")
	if err != nil {
		b.Fatal(err)
	}
	rf := NewRouteFinderDefault(params.ToCollisionFlags())

	for b.Loop() {
		rf.FindRouteDefault(params.Level, params.SrcX, params.SrcZ, params.DestX, params.DestZ)
	}
}

func BenchmarkRouteFinderLongRoute(b *testing.B) {
	// setup
	params, err := RouteFindingParametersFromFile("testdata/benchmarks/long-path.json")
	if err != nil {
		b.Fatal(err)
	}
	rf := NewRouteFinderDefault(params.ToCollisionFlags())

	for b.Loop() {
		rf.FindRouteDefault(params.Level, params.SrcX, params.SrcZ, params.DestX, params.DestZ)
	}
}

func BenchmarkRouteFinderAlternateRoute(b *testing.B) {
	// setup
	params, err := RouteFindingParametersFromFile("testdata/benchmarks/outofbound-path.json")
	if err != nil {
		b.Fatal(err)
	}
	rf := NewRouteFinderDefault(params.ToCollisionFlags())

	for b.Loop() {
		rf.FindRouteDefault(params.Level, params.SrcX, params.SrcZ, params.DestX, params.DestZ)
	}
}
