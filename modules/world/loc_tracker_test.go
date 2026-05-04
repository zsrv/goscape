package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

func TestLocObjTrackerRegisterAddsToList(t *testing.T) {
	tr := newLocObjTracker()
	np := &entitypkg.NonPathing{Entity: entitypkg.NewEntity(0, 100, 200, 1, 1, entitypkg.LifecycleDespawn)}
	tr.Register(np)
	count := 0
	for range tr.All() {
		count++
	}
	if count != 1 {
		t.Errorf("All() count: got %d, want 1", count)
	}
}

func TestLocObjTrackerUnregisterRemoves(t *testing.T) {
	tr := newLocObjTracker()
	np := &entitypkg.NonPathing{Entity: entitypkg.NewEntity(0, 100, 200, 1, 1, entitypkg.LifecycleDespawn)}
	tr.Register(np)
	tr.Unregister(np)
	count := 0
	for range tr.All() {
		count++
	}
	if count != 0 {
		t.Errorf("All() count after Unregister: got %d, want 0", count)
	}
}

func TestLocObjTrackerReRegisterUnlinksOld(t *testing.T) {
	tr := newLocObjTracker()
	np := &entitypkg.NonPathing{Entity: entitypkg.NewEntity(0, 100, 200, 1, 1, entitypkg.LifecycleDespawn)}
	tr.Register(np)
	tr.Register(np) // second register should unlink-and-re-add, not duplicate
	count := 0
	for range tr.All() {
		count++
	}
	if count != 1 {
		t.Errorf("All() count after re-Register: got %d, want 1 (no duplicate)", count)
	}
}

func TestLocObjTrackerUnregisterUnknownIsNoOp(t *testing.T) {
	tr := newLocObjTracker()
	np := &entitypkg.NonPathing{Entity: entitypkg.NewEntity(0, 100, 200, 1, 1, entitypkg.LifecycleDespawn)}
	tr.Unregister(np) // no panic; no-op
}

func TestServerNewInitialisesLocObjTracker(t *testing.T) {
	s := newTestServer(t)
	if s.locObjTracker == nil {
		t.Error("Server.New must initialise locObjTracker")
	}
}
