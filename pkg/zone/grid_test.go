package zone

import "testing"

func TestZoneGridFlagUnflag(t *testing.T) {
	g := NewZoneGrid()
	if g.IsFlagged(100, 200, 0) {
		t.Error("brand-new grid should not be flagged anywhere")
	}
	g.Flag(100, 200)
	if !g.IsFlagged(100, 200, 0) {
		t.Error("after Flag(100,200), IsFlagged(100,200,0) should be true")
	}
	g.Unflag(100, 200)
	if g.IsFlagged(100, 200, 0) {
		t.Error("after Unflag(100,200), IsFlagged(100,200,0) should be false")
	}
}

func TestZoneGridRadiusSearch(t *testing.T) {
	g := NewZoneGrid()
	g.Flag(100, 200)
	if !g.IsFlagged(105, 205, 6) {
		t.Error("(105,205) within radius 6 of flagged (100,200) should match")
	}
	if g.IsFlagged(120, 220, 6) {
		t.Error("(120,220) outside radius 6 of flagged (100,200) should not match")
	}
}
