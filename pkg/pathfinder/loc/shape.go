package loc

type Shape int

const (
	ShapeWallStraight Shape = iota
	ShapeWallDiagonalCorner
	ShapeWallL
	ShapeWallSquareCorner
	ShapeWallDecorStraightNoOffset
	ShapeWallDecorStraightOffset
	ShapeWallDecorDiagonalOffset
	ShapeWallDecorDiagonalNoOffset
	ShapeWallDecorDiagonalBoth
	ShapeWallDiagonal
	ShapeCentrepieceStraight
	ShapeCentrepieceDiagonal
	ShapeRoofStraight
	ShapeRoofDiagonalWithRoofEdge
	ShapeRoofDiagonal
	ShapeRoofLConcave
	ShapeRoofLConvex
	ShapeRoofFlat
	ShapeRoofEdgeStraight
	ShapeRoofEdgeDiagonalCorner
	ShapeRoofEdgeL
	ShapeRoofEdgeSquareCorner
	ShapeGroundDecor
)
