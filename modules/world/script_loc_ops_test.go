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

func TestServerLocOpsRejectsNonLocActiveLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	ops := &serverLocOps{s: s}
	other := &fakeNonLoc{}
	if err := ops.ChangeLoc(other, 100, 0, 0, 1); err == nil {
		t.Error("ChangeLoc with non-*Loc ActiveLoc must error")
	}
}

type fakeNonLoc struct{}

func (f *fakeNonLoc) LocType() int            { return 0 }
func (f *fakeNonLoc) Coords() (int, int, int) { return 0, 0, 0 }
func (f *fakeNonLoc) Angle() int              { return 0 }
func (f *fakeNonLoc) Shape() int              { return 0 }
func (f *fakeNonLoc) Layer() int              { return 0 }

// Suppress unused-script import warning if the file ends up not using script.
var _ script.ActiveLoc = &fakeNonLoc{}
