package loc

import "fmt"

type Layer int

const (
	LayerWall Layer = iota
	LayerWallDecor
	LayerGround
	LayerGroundDecor
)

func LayerOf(shape Shape) Layer {
	switch shape {
	case ShapeWallStraight,
		ShapeWallDiagonalCorner,
		ShapeWallL,
		ShapeWallSquareCorner:
		return LayerWall
	case ShapeWallDecorStraightNoOffset,
		ShapeWallDecorStraightOffset,
		ShapeWallDecorDiagonalOffset,
		ShapeWallDecorDiagonalNoOffset,
		ShapeWallDecorDiagonalBoth:
		return LayerWallDecor
	case ShapeWallDiagonal,
		ShapeCentrepieceStraight,
		ShapeCentrepieceDiagonal,
		ShapeRoofStraight,
		ShapeRoofDiagonalWithRoofEdge,
		ShapeRoofDiagonal,
		ShapeRoofLConcave,
		ShapeRoofLConvex,
		ShapeRoofFlat,
		ShapeRoofEdgeStraight,
		ShapeRoofEdgeDiagonalCorner,
		ShapeRoofEdgeL,
		ShapeRoofEdgeSquareCorner:
		return LayerGround
	case ShapeGroundDecor:
		return LayerGroundDecor
	default:
		panic(fmt.Sprintf("unsupported shape: %d", shape))
	}
}
