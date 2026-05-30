package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

func TestServerLocOpsChangeLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	if err := ops.ChangeLoc(loc, 100, loc.Shape(), loc.Angle(), 1); err != nil {
		t.Fatalf("ChangeLoc: %v", err)
	}
	if loc.Type() != 100 {
		t.Errorf("Type after Change: got %d", loc.Type())
	}
}

func TestServerLocOpsAddLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}

	created, err := ops.AddLoc(0, 3094, 3106, 100, 0, 0, 1)
	if err != nil {
		t.Fatalf("AddLoc: %v", err)
	}
	if created == nil {
		t.Fatal("AddLoc must return a non-nil ActiveLoc")
	}
	loc, ok := created.(*entitypkg.Loc)
	if !ok {
		t.Fatalf("AddLoc returned %T, want *entity.Loc", created)
	}
	if !loc.IsActive {
		t.Error("created loc must have IsActive=true")
	}
}

func TestServerLocOpsRemoveLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	if err := ops.RemoveLoc(loc, 1); err != nil {
		t.Fatalf("RemoveLoc: %v", err)
	}
	if loc.IsActive {
		t.Error("loc must be inactive after RemoveLoc")
	}
}

func TestServerLocOpsLocsAtCoord(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	at := ops.LocsAtCoord(0, 3094, 3106)
	if len(at) != 1 {
		t.Errorf("LocsAtCoord: got %d, want 1", len(at))
	}
}

func TestServerLocOpsAnimLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	ops := &serverLocOps{s: s}
	if err := ops.AnimLoc(loc, 42); err != nil {
		t.Errorf("AnimLoc: %v", err)
	}
}

func TestServerLocOpsRejectsNonLocActiveLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}
	other := &fakeNonLoc{}
	if err := ops.ChangeLoc(other, 100, 0, 0, 1); err == nil {
		t.Error("ChangeLoc with non-*Loc ActiveLoc must error")
	}
	if err := ops.RemoveLoc(other, 1); err == nil {
		t.Error("RemoveLoc with non-*Loc ActiveLoc must error")
	}
	if err := ops.AnimLoc(other, 42); err == nil {
		t.Error("AnimLoc with non-*Loc ActiveLoc must error")
	}
}

// TestServerLocOpsGetLoc pins the adapter's exact-tile + type-equality
// scan and the typed-nil → interface-nil wrap (returning *entity.Loc
// directly would produce a non-nil ActiveLoc holding a typed-nil pointer
// on miss; the explicit nil-check in (*serverLocOps).GetLoc avoids that).
func TestServerLocOpsGetLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 42, 0, 0)
	s.AddLoc(loc, 0)
	ops := &serverLocOps{s: s}

	t.Run("hit returns the loc", func(t *testing.T) {
		got := ops.GetLoc(0, 3094, 3106, 42)
		if got == nil {
			t.Fatal("GetLoc(hit): got nil, want non-nil ActiveLoc")
		}
		x, z, level := got.Coords()
		if x != 3094 || z != 3106 || level != 0 {
			t.Errorf("Coords: got (%d,%d,%d), want (3094,3106,0)", x, z, level)
		}
		if got.LocType() != 42 {
			t.Errorf("LocType: got %d, want 42", got.LocType())
		}
	})

	t.Run("wrong type returns nil", func(t *testing.T) {
		if got := ops.GetLoc(0, 3094, 3106, 99); got != nil {
			t.Errorf("GetLoc(wrong type): got %v, want nil", got)
		}
	})

	t.Run("wrong coord returns nil", func(t *testing.T) {
		if got := ops.GetLoc(0, 9999, 9999, 42); got != nil {
			t.Errorf("GetLoc(wrong coord): got %v, want nil", got)
		}
	})
}

// TestServerLocOps_AllLocsSafe_FiltersInactiveAndReverses pins TS
// Zone.getAllLocsSafe(reverse) semantics (Zone.ts:459-465): yields only
// IsActive locs, optionally in reverse zone order. Closes h-loc-4
// (LocIterator's source). The fake serverLocOps.AllLocsSafe lives in
// modules/world; this is the only place it can be exercised against a
// real *pkg/zone.Zone.
//
// Toggle-off proof for the filter: comment out the
// `if l == nil || !l.IsActive { continue }` line inside both branches
// of serverLocOps.AllLocsSafe → this test fails with "len: got 3,
// want 2" (inactive loc leaks through).
// Toggle-off proof for the reverse: replace the reverse loop with a
// forward loop in the reverse=true branch → this test fails with
// "yield[0].LocType: got 100, want 102".
func TestServerLocOps_AllLocsSafe_FiltersInactiveAndReverses(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}

	// Three locs at the same coord, in zone-append order [a, b, c].
	// a and c are active; b is intentionally inactive (raw-appended).
	locA := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleForever, 100, 0, 0)
	locB := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleForever, 101, 0, 0)
	locC := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleForever, 102, 0, 0)
	zn := s.zoneMap.Get(0, 3094, 3106)
	zn.Locs = append(zn.Locs, locA, locB, locC)
	locA.IsActive = true
	locC.IsActive = true
	// locB intentionally IsActive=false.

	t.Run("reverse=true yields active locs in reverse order", func(t *testing.T) {
		got := ops.AllLocsSafe(0, 3094, 3106, true)
		if len(got) != 2 {
			t.Fatalf("len: got %d, want 2 (active locs A and C; inactive B must be filtered)", len(got))
		}
		if got[0].LocType() != 102 {
			t.Errorf("yield[0].LocType: got %d, want 102 (reverse=true must yield C first)", got[0].LocType())
		}
		if got[1].LocType() != 100 {
			t.Errorf("yield[1].LocType: got %d, want 100 (reverse=true must yield A second)", got[1].LocType())
		}
	})

	t.Run("reverse=false yields active locs in forward order", func(t *testing.T) {
		got := ops.AllLocsSafe(0, 3094, 3106, false)
		if len(got) != 2 {
			t.Fatalf("len: got %d, want 2", len(got))
		}
		if got[0].LocType() != 100 {
			t.Errorf("yield[0].LocType: got %d, want 100 (reverse=false must yield A first)", got[0].LocType())
		}
		if got[1].LocType() != 102 {
			t.Errorf("yield[1].LocType: got %d, want 102", got[1].LocType())
		}
	})
}

type fakeNonLoc struct{}

func (f *fakeNonLoc) LocType() int            { return 0 }
func (f *fakeNonLoc) Coords() (int, int, int) { return 0, 0, 0 }
func (f *fakeNonLoc) Angle() int              { return 0 }
func (f *fakeNonLoc) Shape() int              { return 0 }
func (f *fakeNonLoc) Layer() int              { return 0 }
func (f *fakeNonLoc) Active() bool            { return false }

// Suppress unused-script import warning if the file ends up not using script.
var _ script.ActiveLoc = &fakeNonLoc{}
