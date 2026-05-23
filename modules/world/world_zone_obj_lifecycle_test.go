package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// ---------------------------------------------------------------------------
// B0 producer tests — Server.RemoveObj (NAI-178 T1)
// ---------------------------------------------------------------------------

// TestServerRemoveObj_InactiveObjEarlyReturns pins that RemoveObj is a no-op
// when obj.IsActive==false. LifecycleTick must remain at the constructor
// default (0 — Go zero value) and the locObjTracker must not gain a new entry.
// Mirrors TS World.removeObj isActive guard (World.ts:1505-1507). NAI-178 B0.
func TestServerRemoveObj_InactiveObjEarlyReturns(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 5

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = false // already inactive
	beforeTick := obj.LifecycleTick

	s.RemoveObj(obj, 50)

	if got := obj.LifecycleTick; got != beforeTick {
		t.Errorf("obj.LifecycleTick: got %d, want %d (early-return, unchanged from before)", got, beforeTick)
	}
	tracker := s.locObjTracker.(*locObjTracker)
	for np := range tracker.All() {
		if np == &obj.NonPathing {
			t.Error("obj.NonPathing must NOT be in locObjTracker after inactive early-return")
			break
		}
	}
}

// TestServerRemoveObj_RespawnLifecycleSetsAdjustedLifecycleTick pins that an
// active RESPAWN obj with duration>0 and an empty s.players (identity scale)
// gets obj.LifecycleTick == s.currentTick + duration.
// Mirrors TS World.removeObj RESPAWN branch (World.ts:1513-1515). NAI-178 B0.
func TestServerRemoveObj_RespawnLifecycleSetsAdjustedLifecycleTick(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = true

	s.RemoveObj(obj, 50)

	if got, want := obj.LifecycleTick, s.currentTick+50; got != want {
		t.Errorf("obj.LifecycleTick: got %d, want %d (currentTick+duration, identity scale)", got, want)
	}
}

// TestServerRemoveObj_RespawnLifecycleScalesByPlayerCount pins that the
// adjusted duration is derived via scaleByPlayerCount (halved at 2000 players).
// Asserts against s.scaleByPlayerCount(duration) directly so the test stays
// robust to formula changes. NAI-178 B0.
func TestServerRemoveObj_RespawnLifecycleScalesByPlayerCount(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 0
	const duration = 4000
	setPlayerCountForTest(t, s, 2000)

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = true

	s.RemoveObj(obj, duration)

	want := s.currentTick + s.scaleByPlayerCount(duration)
	if got := obj.LifecycleTick; got != want {
		t.Errorf("obj.LifecycleTick: got %d, want %d (scaled by player count)", got, want)
	}
}

// TestServerRemoveObj_DespawnLifecycleSetsLifecycleTickNegOne pins that a
// DESPAWN-lifecycle obj always gets LifecycleTick==-1 regardless of duration,
// because the RESPAWN gate denies. Mirrors TS World.removeObj else-branch
// (World.ts:1516-1518). NAI-178 B0.
func TestServerRemoveObj_DespawnLifecycleSetsLifecycleTickNegOne(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	obj.IsActive = true

	s.RemoveObj(obj, 50)

	if got := obj.LifecycleTick; got != -1 {
		t.Errorf("obj.LifecycleTick: got %d, want -1 (DESPAWN lifecycle denied by gate)", got)
	}
}

// TestServerRemoveObj_ZeroDurationSetsLifecycleTickNegOne pins that duration==0
// forces LifecycleTick==-1 even for RESPAWN-lifecycle, because the
// duration>0 gate denies. Mirrors TS World.removeObj (World.ts:1513-1518).
// NAI-178 B0.
func TestServerRemoveObj_ZeroDurationSetsLifecycleTickNegOne(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = true

	s.RemoveObj(obj, 0)

	if got := obj.LifecycleTick; got != -1 {
		t.Errorf("obj.LifecycleTick: got %d, want -1 (duration==0 gate denied)", got)
	}
}

// TestServerRemoveObj_RespawnLifecycleRegistersTracker pins that a RESPAWN obj
// with duration>0 is registered in s.locObjTracker after RemoveObj.
// Mirrors NonPathing.SetLifeCycle Register branch (nonpathing.go:50-55).
// NAI-178 B0.
func TestServerRemoveObj_RespawnLifecycleRegistersTracker(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 0

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = true

	s.RemoveObj(obj, 50)

	tracker := s.locObjTracker.(*locObjTracker)
	found := false
	for np := range tracker.All() {
		if np == &obj.NonPathing {
			found = true
			break
		}
	}
	if !found {
		t.Error("obj.NonPathing must be registered in s.locObjTracker after RemoveObj with RESPAWN+duration>0")
	}
}

// TestServerAddObj_DurationSetsLifecycleTick pins that Server.AddObj with
// duration > 0 sets obj.LifecycleTick = s.currentTick + duration and registers
// the obj in s.locObjTracker. Mirrors TS World.addObj lifecycle-plumbing at
// Engine-TS/src/engine/World.ts:1467-1484. NAI-177 B0.
func TestServerAddObj_DurationSetsLifecycleTick(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(obj, zone.PublicReceiver, 50, 0)

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
	s.AddObj(obj, zone.PublicReceiver, 0, 0)

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
	got := w.AddObj(0, 3094, 3106, 995, 1, 50, receiverID, 0)
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

	got := w.AddObj(0, 3094, 3106, 995, 1, 50, zone.PublicReceiver, 0)
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
	w.EnqueueObjDelayed(0, 3094, 3106, 995, 1, 50, 0, receiverID, 0)
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

	w.EnqueueObjDelayed(0, 3094, 3106, 995, 1, 50, 0, zone.PublicReceiver, 0)
	if len(s.objDelayedQueue) != 1 {
		t.Fatalf("objDelayedQueue len: got %d, want 1", len(s.objDelayedQueue))
	}
	obj := s.objDelayedQueue[0].obj
	if obj.Reveal != -1 {
		t.Errorf("obj.Reveal: got %d, want -1 (public drop)", obj.Reveal)
	}
}
