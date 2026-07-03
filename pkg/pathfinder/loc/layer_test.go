package loc

import "testing"

// TestLayerOf pins every Shape → Layer mapping. LayerOf keys on iota-ordered
// Shape constants, so inserting or reordering a Shape silently reclassifies
// every later shape; this table is the regression net for that class of bug.
func TestLayerOf(t *testing.T) {
	cases := []struct {
		shape Shape
		want  Layer
	}{
		{ShapeWallStraight, LayerWall},
		{ShapeWallDiagonalCorner, LayerWall},
		{ShapeWallL, LayerWall},
		{ShapeWallSquareCorner, LayerWall},
		{ShapeWallDecorStraightNoOffset, LayerWallDecor},
		{ShapeWallDecorStraightOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalNoOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalBoth, LayerWallDecor},
		{ShapeWallDiagonal, LayerGround},
		{ShapeCentrepieceStraight, LayerGround},
		{ShapeCentrepieceDiagonal, LayerGround},
		{ShapeRoofStraight, LayerGround},
		{ShapeRoofDiagonalWithRoofEdge, LayerGround},
		{ShapeRoofDiagonal, LayerGround},
		{ShapeRoofLConcave, LayerGround},
		{ShapeRoofLConvex, LayerGround},
		{ShapeRoofFlat, LayerGround},
		{ShapeRoofEdgeStraight, LayerGround},
		{ShapeRoofEdgeDiagonalCorner, LayerGround},
		{ShapeRoofEdgeL, LayerGround},
		{ShapeRoofEdgeSquareCorner, LayerGround},
		{ShapeGroundDecor, LayerGroundDecor},
	}
	if len(cases) != int(ShapeGroundDecor)+1 {
		t.Fatalf("LayerOf test table has %d cases but there are %d Shape constants; a Shape was added without updating this test", len(cases), int(ShapeGroundDecor)+1)
	}
	for _, c := range cases {
		if got := LayerOf(c.shape); got != c.want {
			t.Errorf("LayerOf(%d) = %d, want %d", c.shape, got, c.want)
		}
	}
}

func TestLayerOfPanicsOnUnknownShape(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("LayerOf did not panic on an out-of-range Shape")
		}
	}()
	LayerOf(ShapeGroundDecor + 1)
}
