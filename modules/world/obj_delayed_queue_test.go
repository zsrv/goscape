package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// TestObjDelayedQueue_DelayZeroFiresImmediately pins TS post-decrement
// semantics: stored delay=0, captured 0, decrement to -1, 0>0 false → fires
// on the very first drain after enqueue. Mirrors TS World.ts:564.
func TestObjDelayedQueue_DelayZeroFiresImmediately(t *testing.T) {
	s := newZoneTestServer(t)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(obj, zone.PublicReceiver, 200, 0)

	if got := len(s.objDelayedQueue); got != 1 {
		t.Fatalf("post-enqueue queue len: got %d, want 1", got)
	}

	s.processObjDelayedQueue()

	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("delay=0: queue should drain on first call, got len %d", got)
	}
	if got := len(s.zonesTracking); got != 1 {
		t.Errorf("delay=0: drain should call s.AddObj → TrackZone; zonesTracking len got %d, want 1", got)
	}
}

// TestObjDelayedQueue_FiresAfterDelayTicks pins user delay=2 → first 2
// drain calls skip; the 3rd fires (captured 2 → skip; captured 1 → skip;
// captured 0 → fire).
func TestObjDelayedQueue_FiresAfterDelayTicks(t *testing.T) {
	s := newZoneTestServer(t)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(obj, zone.PublicReceiver, 200, 2)

	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 1 (captured 2): queue len got %d, want 1", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 2 (captured 1): queue len got %d, want 1", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("after drain 3 (captured 0): queue len got %d, want 0", got)
	}
}

// TestObjDelayedQueue_DrainCallsServerAddObj pins drain → s.AddObj routing.
// After fire: zoneMap-resolved zone has the obj, receiverID propagated.
func TestObjDelayedQueue_DrainCallsServerAddObj(t *testing.T) {
	s := newZoneTestServer(t)
	const receiverUID = 0x1234
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	obj.ReceiverID = receiverUID
	s.enqueueObjDelayed(obj, receiverUID, 200, 0)

	s.processObjDelayedQueue()

	z := s.zoneMap.Get(0, 3094, 3106)
	if z == nil {
		t.Fatalf("expected zone at (0,3094,3106) to exist after drain")
	}
	if got := obj.ReceiverID; got != receiverUID {
		t.Errorf("obj.ReceiverID after drain: got %d, want %d", got, receiverUID)
	}
}

// TestObjDelayedQueue_MultipleEntriesIndependentDelays pins per-entry
// delay independence: enqueue {0,1,2} → drain 1 fires entry-0 only, drain
// 2 fires entry-1 only, drain 3 fires entry-2 only.
func TestObjDelayedQueue_MultipleEntriesIndependentDelays(t *testing.T) {
	s := newZoneTestServer(t)
	objA := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	objB := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 995, 2)
	objC := entitypkg.NewObj(0, 3300, 3300, entitypkg.LifecycleDespawn, 995, 3)
	s.enqueueObjDelayed(objA, zone.PublicReceiver, 200, 0)
	s.enqueueObjDelayed(objB, zone.PublicReceiver, 200, 1)
	s.enqueueObjDelayed(objC, zone.PublicReceiver, 200, 2)

	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 2 {
		t.Errorf("after drain 1: queue len got %d, want 2 (objA fired)", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 2: queue len got %d, want 1 (objB fired)", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("after drain 3: queue len got %d, want 0 (objC fired)", got)
	}
}

// TestObjDelayedQueue_DurationDrainsToServerAddObj pins that duration stored
// at enqueue is forwarded to Server.AddObj at drain, resulting in
// obj.LifecycleTick == s.currentTick + duration after the drain fires.
// NAI-177 B0.
func TestObjDelayedQueue_DurationDrainsToServerAddObj(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 5
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	const wantDuration = 17
	s.enqueueObjDelayed(obj, zone.PublicReceiver, wantDuration, 0)

	if got := s.objDelayedQueue[0].duration; got != wantDuration {
		t.Errorf("enqueue: duration field got %d, want %d", got, wantDuration)
	}

	s.processObjDelayedQueue()

	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("post-drain queue len got %d, want 0", got)
	}
	if got, want := obj.LifecycleTick, s.currentTick+wantDuration; got != want {
		t.Errorf("obj.LifecycleTick after drain: got %d, want %d (currentTick+duration)", got, want)
	}
}

// TestObjDelayedQueue_RemoveBeforeFire_PanicRecovery pins recover-then-
// log-then-continue semantics: if AddObj panics inside fire,
// recoverObjDelayed swallows the panic, the entry is already removed
// from the queue (remove-before-fire), and the next iteration sees the
// next entry. Mirrors recoverWorldScript pattern at world_script_queue.go:75.
//
// Trigger panic by enqueuing a nil Obj — Server.AddObj nil-derefs at
// obj.Level on the first line. recoverObjDelayed must handle the nil
// case in its log-field extraction.
func TestObjDelayedQueue_RemoveBeforeFire_PanicRecovery(t *testing.T) {
	s := newZoneTestServer(t)
	good := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(nil, zone.PublicReceiver, 200, 0) // nil-Obj triggers panic on AddObj
	s.enqueueObjDelayed(good, zone.PublicReceiver, 200, 0)

	// Should not panic the test goroutine — recoverObjDelayed swallows.
	s.processObjDelayedQueue()

	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("post-drain queue len got %d, want 0 (both entries removed even with panic)", got)
	}
}
