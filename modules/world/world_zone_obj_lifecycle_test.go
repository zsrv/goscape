package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// TestServerAddObj_DurationSetsLifecycleTick pins that Server.AddObj with
// duration > 0 sets obj.LifecycleTick = s.currentTick + duration and registers
// the obj in s.locObjTracker. Mirrors TS World.addObj lifecycle-plumbing at
// Engine-TS/src/engine/World.ts:1467-1484. NAI-177 B0.
func TestServerAddObj_DurationSetsLifecycleTick(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(obj, zone.PublicReceiver, 50)

	if got, want := obj.LifecycleTick, s.currentTick+50; got != want {
		t.Errorf("obj.LifecycleTick: got %d, want %d", got, want)
	}

	// Verify tracker membership by iterating the locObjTracker.
	tracker := s.locObjTracker.(*locObjTracker)
	found := false
	for np := range tracker.All() {
		if np == &obj.NonPathing {
			found = true
			break
		}
	}
	if !found {
		t.Error("obj.NonPathing must be registered in s.locObjTracker after AddObj with duration>0")
	}
}

// TestServerAddObj_ZeroDurationLeavesLifecycleTickNegOne pins that
// Server.AddObj with duration == 0 leaves obj.LifecycleTick == -1 and does NOT
// register the obj in the tracker (preserves pre-NAI-177 behavior for all
// existing callers that pass 0). NAI-177 B0.
func TestServerAddObj_ZeroDurationLeavesLifecycleTickNegOne(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(obj, zone.PublicReceiver, 0)

	if got := obj.LifecycleTick; got != -1 {
		t.Errorf("obj.LifecycleTick: got %d, want -1 (untracked)", got)
	}

	tracker := s.locObjTracker.(*locObjTracker)
	for np := range tracker.All() {
		if np == &obj.NonPathing {
			t.Error("obj.NonPathing must NOT be in locObjTracker when duration == 0")
			break
		}
	}
}

// TestWorldVarsViewAddObj_ReceiverTargetedSetsReveal100 pins that a non-public
// receiver causes the constructed Obj.Reveal to be set to entity.ObjReveal (100).
// Mirrors TS World.addObj receiver64 branch (World.ts:1471-1474). NAI-177 B0.
func TestWorldVarsViewAddObj_ReceiverTargetedSetsReveal100(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	const receiverID = 12345
	got := w.AddObj(0, 3094, 3106, 995, 1, 50, receiverID)
	if got == nil {
		t.Fatal("AddObj returned nil")
	}
	obj, ok := got.(*entitypkg.Obj)
	if !ok {
		t.Fatalf("AddObj returned %T, want *entitypkg.Obj", got)
	}
	if obj.Reveal != entitypkg.ObjReveal {
		t.Errorf("obj.Reveal: got %d, want %d (ObjReveal)", obj.Reveal, entitypkg.ObjReveal)
	}
}

// TestWorldVarsViewAddObj_PublicReceiverLeavesRevealNegOne pins that a public
// receiver (zone.PublicReceiver == -1) leaves Obj.Reveal == -1 (already public
// from construction). Mirrors TS World.addObj else-branch (World.ts:1475-1477).
// NAI-177 B0.
func TestWorldVarsViewAddObj_PublicReceiverLeavesRevealNegOne(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	got := w.AddObj(0, 3094, 3106, 995, 1, 50, zone.PublicReceiver)
	if got == nil {
		t.Fatal("AddObj returned nil")
	}
	obj, ok := got.(*entitypkg.Obj)
	if !ok {
		t.Fatalf("AddObj returned %T, want *entitypkg.Obj", got)
	}
	if obj.Reveal != -1 {
		t.Errorf("obj.Reveal: got %d, want -1 (public drop)", obj.Reveal)
	}
}

// TestWorldVarsViewEnqueueObjDelayed_ReceiverTargetedSetsReveal100 pins that
// the INV_DROPITEM_DELAYED producer also initialises Reveal for non-public
// receivers — without it, combat loot routed via the delayed-drop queue would
// skip the 100-tick private window now that turnObj actively enforces it.
// Sibling of TestWorldVarsViewAddObj_ReceiverTargetedSetsReveal100. NAI-177 B1
// review fix.
func TestWorldVarsViewEnqueueObjDelayed_ReceiverTargetedSetsReveal100(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	const receiverID = 12345
	w.EnqueueObjDelayed(0, 3094, 3106, 995, 1, 50, 0, receiverID)
	if len(s.objDelayedQueue) != 1 {
		t.Fatalf("objDelayedQueue len: got %d, want 1", len(s.objDelayedQueue))
	}
	obj := s.objDelayedQueue[0].obj
	if obj.Reveal != entitypkg.ObjReveal {
		t.Errorf("obj.Reveal: got %d, want %d (ObjReveal)", obj.Reveal, entitypkg.ObjReveal)
	}
}

// TestWorldVarsViewEnqueueObjDelayed_PublicReceiverLeavesRevealNegOne pins
// the else-branch sibling for the EnqueueObjDelayed Reveal init. NAI-177 B1
// review fix.
func TestWorldVarsViewEnqueueObjDelayed_PublicReceiverLeavesRevealNegOne(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	w.EnqueueObjDelayed(0, 3094, 3106, 995, 1, 50, 0, zone.PublicReceiver)
	if len(s.objDelayedQueue) != 1 {
		t.Fatalf("objDelayedQueue len: got %d, want 1", len(s.objDelayedQueue))
	}
	obj := s.objDelayedQueue[0].obj
	if obj.Reveal != -1 {
		t.Errorf("obj.Reveal: got %d, want -1 (public drop)", obj.Reveal)
	}
}
