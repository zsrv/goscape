package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func newTestNpc(nid int) *Npc {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	return NewNpc(nid, 0, 3094, 3106, 0, typ)
}

func TestNpcAnimateSetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.Animate(123, 5)
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set")
	}
	if n.animID != 123 || n.animDelay != 5 {
		t.Errorf("animID/Delay: got (%d,%d), want (123,5)", n.animID, n.animDelay)
	}
}

func TestNpcSaySetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.Say([]byte("hi"))
	if n.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay should be set")
	}
	if string(n.sayText) != "hi" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "hi")
	}
}

func TestNpcChangeTypeSetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.ChangeType(42, 100)
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("NpcMaskChangeType should be set")
	}
	if n.changeTypeID != 42 {
		t.Errorf("changeTypeID: got %d, want 42", n.changeTypeID)
	}
	if n.typeId != 42 {
		t.Errorf("typeId: got %d, want 42 (NAI-16 — ChangeType now writes typeId)", n.typeId)
	}
	wantUID := (42 << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d (recomputed from new typeId)", n.uid, wantUID)
	}
	if n.lifecycleTick != 100 {
		t.Errorf("lifecycleTick: got %d, want 100 (schedules revert)", n.lifecycleTick)
	}
}

func TestNpcChangeTypeDurationZeroNoOp(t *testing.T) {
	n := newTestNpc(1)
	// Seed known state so "no-op" is observable.
	origTypeID := n.typeId
	origUID := n.uid
	origLifecycleTick := n.lifecycleTick
	origMasks := n.masks

	n.ChangeType(42, 0) // TS guard: duration < 1 → total no-op

	if n.typeId != origTypeID {
		t.Errorf("typeId: got %d, want %d (duration=0 should not write)", n.typeId, origTypeID)
	}
	if n.uid != origUID {
		t.Errorf("uid: got %d, want %d (duration=0 should not recompute)", n.uid, origUID)
	}
	if n.lifecycleTick != origLifecycleTick {
		t.Errorf("lifecycleTick: got %d, want %d (duration=0 should not write)", n.lifecycleTick, origLifecycleTick)
	}
	if n.masks != origMasks {
		t.Errorf("masks: got %d, want %d (duration=0 should not raise mask)", n.masks, origMasks)
	}
}

func TestNpcChangeTypeDeadNoOp(t *testing.T) {
	n := newTestNpc(1)
	n.dead = true
	origTypeID := n.typeId
	origMasks := n.masks

	n.ChangeType(42, 100) // TS guard: !isActive → total no-op

	if n.typeId != origTypeID {
		t.Errorf("typeId: got %d, want %d (dead NPC should not morph)", n.typeId, origTypeID)
	}
	if n.masks != origMasks {
		t.Errorf("masks: got %d, want %d (dead NPC should not raise mask)", n.masks, origMasks)
	}
}

// TestNpcChangeTypeBaseTypeRespawnFastPath guards the TS:444-445
// fast-path: morphing a RESPAWN NPC to its own baseType must set
// lifecycleTick to -1 (never-fires), NOT duration. Without this
// fast-path, the Events block would fire revertType N ticks later,
// and revertType()'s unconditional tail (queue/waypoints/hunt/HP
// resets) would wipe state that the caller didn't ask to lose.
func TestNpcChangeTypeBaseTypeRespawnFastPath(t *testing.T) {
	n := newTestNpc(1)
	n.baseType = 7
	n.typeId = 42 // simulate a prior changetype to a non-base type
	n.lifecycle = NpcLifecycleRespawn

	n.ChangeType(7, 100) // morphing BACK to baseType

	if n.typeId != 7 {
		t.Errorf("typeId: got %d, want 7 (writes still happen)", n.typeId)
	}
	if n.lifecycleTick != -1 {
		t.Errorf("lifecycleTick: got %d, want -1 (fast-path must suppress revert schedule)", n.lifecycleTick)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("NpcMaskChangeType should still be set (fast-path only skips lifecycle schedule)")
	}
}

// TestNpcChangeTypeBaseTypeDespawnNoFastPath guards that the fast-path
// is gated on lifecycle == RESPAWN. A Despawn NPC morphing to baseType
// still gets the normal lifecycleTick = duration.
func TestNpcChangeTypeBaseTypeDespawnNoFastPath(t *testing.T) {
	n := newTestNpc(1)
	n.baseType = 7
	n.typeId = 42
	n.lifecycle = NpcLifecycleDespawn

	n.ChangeType(7, 100)

	if n.lifecycleTick != 100 {
		t.Errorf("lifecycleTick: got %d, want 100 (fast-path is RESPAWN-only)", n.lifecycleTick)
	}
}

func TestNpcFaceCoord(t *testing.T) {
	n := newTestNpc(1)
	n.FaceCoord(100, 200)
	if n.faceSquareX != 201 || n.faceSquareZ != 401 {
		t.Errorf("faceSquareX/Z: got (%d,%d), want (201,401)", n.faceSquareX, n.faceSquareZ)
	}
}

func TestNewNpcInitialisesInteractionFields(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10", n.apRange)
	}
	if n.apRangeCalled != false {
		t.Errorf("apRangeCalled: got %t, want false", n.apRangeCalled)
	}
	if n.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1", n.targetSubject.com)
	}
	if n.targetSubject.typ != -1 {
		t.Errorf("targetSubject.typ: got %d, want -1", n.targetSubject.typ)
	}
	if n.targetX != -1 {
		t.Errorf("targetX: got %d, want -1", n.targetX)
	}
	if n.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1", n.targetZ)
	}
	if n.faceAngleX != -1 {
		t.Errorf("faceAngleX: got %d, want -1", n.faceAngleX)
	}
	if n.faceAngleZ != -1 {
		t.Errorf("faceAngleZ: got %d, want -1", n.faceAngleZ)
	}
}

func TestNpcResetMasksClearsEphemerals(t *testing.T) {
	n := newTestNpc(1)
	n.Animate(123, 5)
	n.Say([]byte("hi"))
	n.Damage(10, 1)
	n.ResetMasks()

	if n.masks != 0 {
		t.Errorf("masks: got %d, want 0", n.masks)
	}
	if n.sayText != nil {
		t.Error("sayText should be nil after reset")
	}
	if n.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1", n.damageAmt)
	}
	if n.animID != 123 {
		t.Errorf("animID should persist: got %d, want 123", n.animID)
	}
}

func TestNpcIsValid(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	if !n.IsValid() {
		t.Error("fresh npc: IsValid = false, want true")
	}
	n.dead = true
	if n.IsValid() {
		t.Error("dead npc: IsValid = true, want false")
	}
}

// TestNewNpcSeedsStatsFromType verifies that NewNpc seeds both
// n.levels[] and n.baseLevels[] from typ.Stats for all 6 slots.
// Mirrors TS Npc.ts:90-94 ctor loop.
func TestNewNpcSeedsStatsFromType(t *testing.T) {
	typ := &objtype.NpcType{
		Stats: []uint16{7, 11, 13, 17, 19, 23}, // distinct per slot
	}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	want := []int{7, 11, 13, 17, 19, 23}
	for i := 0; i < objtype.NpcStatCount; i++ {
		if got := n.NpcStat(i); got != want[i] {
			t.Errorf("NpcStat(%d): got %d, want %d", i, got, want[i])
		}
		if got := n.NpcBaseStat(i); got != want[i] {
			t.Errorf("NpcBaseStat(%d): got %d, want %d", i, got, want[i])
		}
	}
	if !n.resetOnRevert {
		t.Errorf("resetOnRevert: got false, want true (default)")
	}
}

// TestNewNpcWithNilStatsStaysZero verifies that a zero-length Stats
// slice leaves both arrays zero-valued (no out-of-bounds panic).
func TestNewNpcWithNilStatsStaysZero(t *testing.T) {
	typ := &objtype.NpcType{Stats: nil}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	for i := 0; i < objtype.NpcStatCount; i++ {
		if got := n.NpcStat(i); got != 0 {
			t.Errorf("NpcStat(%d): got %d, want 0", i, got)
		}
		if got := n.NpcBaseStat(i); got != 0 {
			t.Errorf("NpcBaseStat(%d): got %d, want 0", i, got)
		}
	}
	if got := n.CurHP(); got != 0 {
		t.Errorf("CurHP: got %d, want 0 (nil Stats)", got)
	}
	if got := n.BaseHP(); got != 0 {
		t.Errorf("BaseHP: got %d, want 0 (nil Stats)", got)
	}
}

// TestNpcStatAllSlots verifies NpcStat reads from n.levels for all 6
// slots after direct array writes.
func TestNpcStatAllSlots(t *testing.T) {
	n := newNpcForLifecycleTest(t) // existing fixture
	for i := 0; i < objtype.NpcStatCount; i++ {
		n.levels[i] = 100 + i
	}
	for i := 0; i < objtype.NpcStatCount; i++ {
		if got, want := n.NpcStat(i), 100+i; got != want {
			t.Errorf("NpcStat(%d): got %d, want %d", i, got, want)
		}
	}
}

// TestNpcBaseStatAllSlots verifies NpcBaseStat reads from
// n.baseLevels for all 6 slots after direct array writes.
func TestNpcBaseStatAllSlots(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	for i := 0; i < objtype.NpcStatCount; i++ {
		n.baseLevels[i] = 200 + i
	}
	for i := 0; i < objtype.NpcStatCount; i++ {
		if got, want := n.NpcBaseStat(i), 200+i; got != want {
			t.Errorf("NpcBaseStat(%d): got %d, want %d", i, got, want)
		}
	}
}
