package world

import (
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

func newTestNpc(nid int) *Npc {
	typ := &objtype.NpcType{
		ID: 0, DebugName: "test",
		WanderRange: 0,
		RespawnRate: 50,
	}
	return NewNpc(nid, 0, 3094, 3106, 0, typ)
}

func TestNewNpc_OrientationXZ_DefaultMinusOne(t *testing.T) {
	n := newTestNpc(1)
	if n.OrientationX != -1 {
		t.Errorf("OrientationX default: got %d, want -1", n.OrientationX)
	}
	if n.OrientationZ != -1 {
		t.Errorf("OrientationZ default: got %d, want -1", n.OrientationZ)
	}
}

func TestNpcAnimateSetsMask(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(200)
	n := &Npc{server: s, animID: -1}
	n.Animate(123, 5)
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set")
	}
	if n.animID != 123 || n.animDelay != 5 {
		t.Errorf("animID/Delay: got (%d,%d), want (123,5)", n.animID, n.animDelay)
	}
}

// TestNpcResetMasks_ResetsAnimForCrossTickReplay is the NPC twin of
// TestResetMasks_ResetsAnimForCrossTickReplay: an NPC animation (combat
// attack/defend, scripted emote) must replay across ticks. ResetMasks must
// clear animID/animDelay to -1 so Animate's priority guard doesn't reject
// the equal-priority repeat. Mirrors TS PathingEntity.resetPathingEntity
// (PathingEntity.ts:598-601).
func TestNpcResetMasks_ResetsAnimForCrossTickReplay(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(200)
	n := &Npc{server: s, animID: -1, animDelay: -1, faceEntity: -1}

	n.Animate(123, 5)
	if n.animID != 123 || n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Fatalf("first Animate: animID=%d mask-set=%v, want 123/true", n.animID, n.masks&rsbuf.NpcMaskAnim != 0)
	}

	n.ResetMasks()
	if n.animID != -1 {
		t.Errorf("animID after ResetMasks: got %d, want -1", n.animID)
	}
	if n.animDelay != -1 {
		t.Errorf("animDelay after ResetMasks: got %d, want -1", n.animDelay)
	}

	n.Animate(123, 5)
	if n.animID != 123 {
		t.Errorf("replay animID: got %d, want 123", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("replay must re-flag NpcMaskAnim (otherwise NPC anim only plays once)")
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
	typ := &objtype.NpcType{WanderRange: 5, DefaultMode: objtype.NPCModeWander}
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
	n.animID = 123
	n.animDelay = 5
	n.masks |= rsbuf.NpcMaskAnim
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
	// animID/animDelay are per-tick: ResetMasks clears them to -1 (TS
	// PathingEntity.ts:598-601) so an NPC animation can replay on a later tick.
	if n.animID != -1 {
		t.Errorf("animID should reset to -1: got %d", n.animID)
	}
	if n.animDelay != -1 {
		t.Errorf("animDelay should reset to -1: got %d", n.animDelay)
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
	for i := range objtype.NpcStatCount {
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

	for i := range objtype.NpcStatCount {
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
	for i := range objtype.NpcStatCount {
		n.levels[i] = 100 + i
	}
	for i := range objtype.NpcStatCount {
		if got, want := n.NpcStat(i), 100+i; got != want {
			t.Errorf("NpcStat(%d): got %d, want %d", i, got, want)
		}
	}
}

// TestNpcBaseStatAllSlots verifies NpcBaseStat reads from
// n.baseLevels for all 6 slots after direct array writes.
func TestNpcBaseStatAllSlots(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	for i := range objtype.NpcStatCount {
		n.baseLevels[i] = 200 + i
	}
	for i := range objtype.NpcStatCount {
		if got, want := n.NpcBaseStat(i), 200+i; got != want {
			t.Errorf("NpcBaseStat(%d): got %d, want %d", i, got, want)
		}
	}
}

// TestChangeTypeResetsStatsWithBoostPreservation verifies the TS
// Npc.ts:436-443 boost/drain-preserving formula:
//
//	levels[i] = max(newBase - (baseLevels[i] - levels[i]), 0)
//	baseLevels[i] = newBase
//
// When the pre-morph NPC has stat boosts/drains, the morph preserves
// the SAME delta against the new type's base.
func TestChangeTypeResetsStatsWithBoostPreservation(t *testing.T) {
	s := newServerForScriptTest(t)
	baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
	newTyp := &objtype.NpcType{Stats: []uint16{20, 15, 25, 20, 12, 30}}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{baseTyp, newTyp}}

	n := NewNpc(1, 0, 100, 100, 0, baseTyp)
	n.server = s
	// Seed deltas: ATK drain=2 (levels=8), DEF boost=2 (levels=12),
	// STR level (no delta), HP drain=5 (levels=5), RNG boost=3 (levels=13),
	// MAG (no delta).
	n.levels[objtype.NpcStatAttack] = 8
	n.levels[objtype.NpcStatDefence] = 12
	n.levels[objtype.NpcStatStrength] = 10
	n.levels[objtype.NpcStatHitpoints] = 5
	n.levels[objtype.NpcStatRanged] = 13
	n.levels[objtype.NpcStatMagic] = 10

	n.ChangeType(1, 100) // morph to newTyp

	// Expected: newBase − drain  (drain positive = drained; negative = boosted)
	//   ATK: 20 − 2 = 18
	//   DEF: 15 − (−2) = 17
	//   STR: 25 − 0 = 25
	//   HP:  20 − 5 = 15
	//   RNG: 12 − (−3) = 15
	//   MAG: 30 − 0 = 30
	wantLevels := []int{18, 17, 25, 15, 15, 30}
	wantBase := []int{20, 15, 25, 20, 12, 30}
	for i := range objtype.NpcStatCount {
		if n.levels[i] != wantLevels[i] {
			t.Errorf("levels[%d]: got %d, want %d", i, n.levels[i], wantLevels[i])
		}
		if n.baseLevels[i] != wantBase[i] {
			t.Errorf("baseLevels[%d]: got %d, want %d", i, n.baseLevels[i], wantBase[i])
		}
	}
	if !n.resetOnRevert {
		t.Errorf("resetOnRevert: got false, want true (ChangeType default)")
	}
}

// TestChangeTypeResetsStatsClampedAtZero verifies that an oversize drain
// against a smaller new base clamps to zero via TS's Math.max(..., 0).
func TestChangeTypeResetsStatsClampedAtZero(t *testing.T) {
	s := newServerForScriptTest(t)
	baseTyp := &objtype.NpcType{Stats: []uint16{100, 10, 10, 10, 10, 10}}
	newTyp := &objtype.NpcType{Stats: []uint16{5, 10, 10, 10, 10, 10}}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{baseTyp, newTyp}}

	n := NewNpc(1, 0, 100, 100, 0, baseTyp)
	n.server = s
	// ATK drain=90 (base=100, level=10). New base=5. 5 − 90 = −85 → clamp 0.
	n.levels[objtype.NpcStatAttack] = 10

	n.ChangeType(1, 100)

	if got := n.levels[objtype.NpcStatAttack]; got != 0 {
		t.Errorf("levels[ATK]: got %d, want 0 (clamped from -85)", got)
	}
	if got := n.baseLevels[objtype.NpcStatAttack]; got != 5 {
		t.Errorf("baseLevels[ATK]: got %d, want 5", got)
	}
}

// TestChangeTypeKeepAllPreservesStats verifies that ChangeTypeKeepAll
// morphs typeId/uid/mask but leaves levels[]/baseLevels[] unchanged
// and writes resetOnRevert=false.
func TestChangeTypeKeepAllPreservesStats(t *testing.T) {
	s := newServerForScriptTest(t)
	baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
	newTyp := &objtype.NpcType{Stats: []uint16{99, 99, 99, 99, 99, 99}}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{baseTyp, newTyp}}

	n := NewNpc(1, 0, 100, 100, 0, baseTyp)
	n.server = s
	// Seed some deltas.
	n.levels[objtype.NpcStatAttack] = 5
	n.levels[objtype.NpcStatHitpoints] = 5
	n.levels[objtype.NpcStatDefence] = 15 // boosted

	n.ChangeTypeKeepAll(1, 100)

	// levels and baseLevels UNCHANGED.
	wantLevels := []int{5, 15, 10, 5, 10, 10}
	wantBase := []int{10, 10, 10, 10, 10, 10}
	for i := range objtype.NpcStatCount {
		if n.levels[i] != wantLevels[i] {
			t.Errorf("levels[%d]: got %d, want %d (KEEPALL preserves)", i, n.levels[i], wantLevels[i])
		}
		if n.baseLevels[i] != wantBase[i] {
			t.Errorf("baseLevels[%d]: got %d, want %d (KEEPALL preserves)", i, n.baseLevels[i], wantBase[i])
		}
	}
	// Morph state applied.
	if n.typeId != 1 {
		t.Errorf("typeId: got %d, want 1", n.typeId)
	}
	if n.uid != (1<<16)|n.nid {
		t.Errorf("uid: got %d, want %d", n.uid, (1<<16)|n.nid)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("mask: CHANGE_TYPE bit not set")
	}
	if n.resetOnRevert {
		t.Errorf("resetOnRevert: got true, want false (KEEPALL)")
	}
	if n.lifecycleTick != 100 {
		t.Errorf("lifecycleTick: got %d, want 100", n.lifecycleTick)
	}
}

// TestChangeTypeKeepAllDurationZeroNoOp verifies duration<1 guard.
func TestChangeTypeKeepAllDurationZeroNoOp(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.levels[objtype.NpcStatHitpoints] = 5
	origTypeId := n.typeId
	origResetOnRevert := n.resetOnRevert

	n.ChangeTypeKeepAll(42, 0)

	if n.typeId != origTypeId {
		t.Errorf("typeId: got %d, want %d (duration=0 no-op)", n.typeId, origTypeId)
	}
	if n.resetOnRevert != origResetOnRevert {
		t.Errorf("resetOnRevert: got %v, want %v (duration=0 no-op)",
			n.resetOnRevert, origResetOnRevert)
	}
}

// TestChangeTypeKeepAllDeadNoOp verifies dead-NPC guard.
func TestChangeTypeKeepAllDeadNoOp(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.dead = true
	origTypeId := n.typeId
	origResetOnRevert := n.resetOnRevert

	n.ChangeTypeKeepAll(42, 100)

	if n.typeId != origTypeId {
		t.Errorf("typeId: got %d, want %d (dead NPC no-op)", n.typeId, origTypeId)
	}
	if n.resetOnRevert != origResetOnRevert {
		t.Errorf("resetOnRevert: got %v, want %v (dead NPC no-op)",
			n.resetOnRevert, origResetOnRevert)
	}
}

// TestRevertTypeHonorsResetOnRevertFalse verifies the light path
// (TS Npc.ts:1086-1090): typeId + uid + CHANGE_TYPE mask only;
// stats/queue/waypoints/hunt fields unchanged.
func TestRevertTypeHonorsResetOnRevertFalse(t *testing.T) {
	s := newServerForScriptTest(t)
	baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{baseTyp}}

	n := NewNpc(1, 0, 100, 100, 0, baseTyp)
	n.server = s
	// Simulate post-KEEPALL state: typeId != baseType, resetOnRevert=false,
	// stats have survived a morph, queue/waypoints/hunt fields populated.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid
	n.resetOnRevert = false
	n.levels[objtype.NpcStatAttack] = 5 // drained
	n.levels[objtype.NpcStatHitpoints] = 7
	n.baseLevels[objtype.NpcStatAttack] = 20 // not from baseTyp
	n.baseLevels[objtype.NpcStatHitpoints] = 20
	n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, LastInt: 42}}
	n.waypointIndex = 3
	n.huntClock = 7
	n.huntRange = 99

	n.revertType()

	// Light path: typeId reverted, uid recomputed, mask raised.
	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want %d (baseType)", n.typeId, n.baseType)
	}
	if n.uid != (n.baseType<<16)|n.nid {
		t.Errorf("uid: got %d, want %d", n.uid, (n.baseType<<16)|n.nid)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("mask: CHANGE_TYPE bit not set")
	}
	// Light path: stats/queue/waypoints/hunt UNCHANGED.
	if n.levels[objtype.NpcStatAttack] != 5 {
		t.Errorf("levels[ATK]: got %d, want 5 (light path preserves)", n.levels[objtype.NpcStatAttack])
	}
	if n.baseLevels[objtype.NpcStatAttack] != 20 {
		t.Errorf("baseLevels[ATK]: got %d, want 20 (light path preserves)", n.baseLevels[objtype.NpcStatAttack])
	}
	if len(n.queue) != 1 {
		t.Errorf("queue: got len=%d, want 1 (light path preserves)", len(n.queue))
	}
	if n.waypointIndex != 3 {
		t.Errorf("waypointIndex: got %d, want 3 (light path preserves)", n.waypointIndex)
	}
	if n.huntClock != 7 {
		t.Errorf("huntClock: got %d, want 7 (light path preserves)", n.huntClock)
	}
	if n.huntRange != 99 {
		t.Errorf("huntRange: got %d, want 99 (light path preserves)", n.huntRange)
	}
	// Re-arm tail.
	if !n.resetOnRevert {
		t.Errorf("resetOnRevert: got false, want true (re-armed after revert)")
	}
}

// TestRevertTypeHonorsResetOnRevertTrue verifies the heavy path
// reseeds all 6 stats from n.typ.Stats (expands S6d's HP-only reseed).
func TestRevertTypeHonorsResetOnRevertTrue(t *testing.T) {
	s := newServerForScriptTest(t)
	baseTyp := &objtype.NpcType{
		Stats:     []uint16{7, 11, 13, 17, 19, 23},
		HuntRange: 8,
		HuntMode:  -1,
	}
	morphTyp := &objtype.NpcType{Stats: []uint16{50, 50, 50, 50, 50, 50}}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{baseTyp, morphTyp}}

	n := NewNpc(1, 0, 100, 100, 0, baseTyp)
	n.server = s
	// Simulate post-CHANGETYPE state: morphed + stats-reset to morphTyp.
	n.typeId = 1
	n.typ = morphTyp
	n.uid = (1 << 16) | n.nid
	n.resetOnRevert = true
	for i := range objtype.NpcStatCount {
		n.levels[i] = 50
		n.baseLevels[i] = 50
	}
	n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, LastInt: 42}}

	n.revertType()

	// Heavy path: stats reseeded to baseTyp, queue cleared.
	want := []int{7, 11, 13, 17, 19, 23}
	for i := range objtype.NpcStatCount {
		if n.levels[i] != want[i] {
			t.Errorf("levels[%d]: got %d, want %d (reseed from baseTyp)", i, n.levels[i], want[i])
		}
		if n.baseLevels[i] != want[i] {
			t.Errorf("baseLevels[%d]: got %d, want %d", i, n.baseLevels[i], want[i])
		}
	}
	if n.queue != nil {
		t.Errorf("queue: got %v, want nil (heavy path clears)", n.queue)
	}
	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want baseType=%d", n.typeId, n.baseType)
	}
	if !n.resetOnRevert {
		t.Errorf("resetOnRevert: got false, want true (re-armed)")
	}
}

// TestRevertTypeReArmsResetOnRevert is a dedicated assertion of the
// re-arm tail on the light path (the heavy-path test above also
// asserts this, but re-arm regression is worth pinning in a named test).
func TestRevertTypeReArmsResetOnRevert(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.resetOnRevert = false
	n.typeId = 42 // != baseType so the typeId write path runs

	n.revertType()

	if !n.resetOnRevert {
		t.Errorf("resetOnRevert: got false, want true (re-armed after revert)")
	}
}

// TestRevertTypeHeavyPathTeles pins that revertType's heavy path teles the
// NPC back to (startX, startZ) per TS Npc.ts:1083-1085 → World.addNpc:1264.
func TestRevertTypeHeavyPathTeles(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ID: 7, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.x = 150
	n.z = 150
	n.resetOnRevert = true

	n.revertType()

	if n.x != 100 || n.z != 100 {
		t.Errorf("n.(x,z): got (%d,%d), want (100,100) (startX/startZ)", n.x, n.z)
	}
}

// TestRevertTypeHeavyPathReseedsStats pins that revertType's heavy path
// reseeds all 6 stats from n.typ.Stats (via resetEntityForRespawn).
func TestRevertTypeHeavyPathReseedsStats(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	typ := &objtype.NpcType{
		ID:    7,
		Size:  1,
		Stats: []uint16{10, 20, 30, 40, 50, 60},
	}
	s.npcTypes.Configs[7] = typ
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	// Drain stats.
	for i := range objtype.NpcStatCount {
		n.levels[i] = 0
	}
	n.resetOnRevert = true

	n.revertType()

	want := []int{10, 20, 30, 40, 50, 60}
	for i := range objtype.NpcStatCount {
		if n.levels[i] != want[i] {
			t.Errorf("n.levels[%d]: got %d, want %d", i, n.levels[i], want[i])
		}
	}
}

// TestRevertTypeHeavyPathClearsQueueWaypoints pins that revertType's
// heavy path clears n.queue and n.waypointIndex.
func TestRevertTypeHeavyPathClearsQueueWaypoints(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	typ := &objtype.NpcType{ID: 7, Size: 1}
	s.npcTypes.Configs[7] = typ
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1}}
	n.waypointIndex = 5
	n.resetOnRevert = true

	n.revertType()

	if n.queue != nil {
		t.Errorf("n.queue: got %v, want nil", n.queue)
	}
	if n.waypointIndex != -1 {
		t.Errorf("n.waypointIndex: got %d, want -1", n.waypointIndex)
	}
}

// TestRevertTypeHeavyPathRunsCollisionToggles pins the dead-flag round-trip
// (removeNpc sets dead=true; addNpc clears it) as a proxy for the collision
// toggle cycle. The pre-condition n.dead = true forces revertType's heavy
// path to actually clear the flag via addNpc — without this, the assertion
// would trivially pass (addNpc runs in both pre- and post-revert states).
// Collision-flag observability directly would require Pathfinder fixture
// plumbing that this test doesn't set up.
func TestRevertTypeHeavyPathRunsCollisionToggles(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	s.gamemap = gamemap.New(discardLogger())
	typ := &objtype.NpcType{
		ID:        7,
		Size:      1,
		BlockWalk: objtype.BlockWalkNPC,
	}
	s.npcTypes.Configs[7] = typ
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.dead = true // pre-condition: force the round-trip to do real work
	n.resetOnRevert = true

	n.revertType()

	if n.dead {
		t.Error("n.dead post-revert: got true, want false (addNpc must clear)")
	}
}

// TestRevertTypeLightPathUnchanged pins that the !resetOnRevert (KEEPALL)
// branch is unchanged: typeId restored to baseType, uid recomputed,
// CHANGE_TYPE mask raised, resetOnRevert re-armed to true. No tele,
// no stats reseed, no queue clear.
func TestRevertTypeLightPathUnchanged(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ID: 7, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.server = s
	// Simulate KEEPALL changetype: typeId moved, resetOnRevert=false.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid
	n.resetOnRevert = false
	n.x = 150
	n.z = 150
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1}}

	n.revertType()

	if n.typeId != n.baseType {
		t.Errorf("n.typeId: got %d, want baseType=%d", n.typeId, n.baseType)
	}
	if n.uid != (n.baseType<<16)|n.nid {
		t.Errorf("n.uid: got %d, want recomputed for baseType", n.uid)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("NpcMaskChangeType bit not set")
	}
	if !n.resetOnRevert {
		t.Error("n.resetOnRevert: got false, want true (re-armed)")
	}
	if n.x != 150 || n.z != 150 {
		t.Errorf("n.(x,z): got (%d,%d), want (150,150) (light path must not tele)", n.x, n.z)
	}
	if len(n.queue) != 1 {
		t.Errorf("n.queue: light path must not clear; got len %d, want 1", len(n.queue))
	}
}

// TestChangeTypeDoesNotMutateBlockWalkOrSize pins NAI-20 Task 2:
// ChangeType updates n.typeId and n.typ but MUST NOT mutate the
// geometry snapshot fields. TS PathingEntity ctor-snapshot semantic.
func TestChangeTypeDoesNotMutateBlockWalkOrSize(t *testing.T) {
	s := newTestServer(t)
	baseTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	morphTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, baseTyp, morphTyp},
	}

	n := newRegisteredNpc(t, s, baseTyp, true)
	wantBlockWalk := n.blockWalk
	wantSize := n.size

	n.ChangeType(2, -1) // morph to typeId=2 (size=2); register=true so
	// changeTypeImpl's lookupType (via n.server) succeeds and n.typ
	// actually swaps to morphTyp — exercising the realistic post-morph
	// state rather than the lookupType-returns-nil short-circuit.

	if n.blockWalk != wantBlockWalk {
		t.Errorf("blockWalk after ChangeType: got %v, want %v",
			n.blockWalk, wantBlockWalk)
	}
	if n.size != wantSize {
		t.Errorf("size after ChangeType: got %d, want %d",
			n.size, wantSize)
	}
}

// TestNewNpcSnapshotsBlockWalkAndSize pins NAI-20 Task 2: NewNpc copies
// blockWalk + size from typ at construction time.
func TestNewNpcSnapshotsBlockWalkAndSize(t *testing.T) {
	typ := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	n := NewNpc(1, 1, 3200, 3200, 0, typ)
	if n.blockWalk != objtype.BlockWalkAll {
		t.Errorf("blockWalk: got %v, want BlockWalkAll", n.blockWalk)
	}
	if n.size != 2 {
		t.Errorf("size: got %d, want 2", n.size)
	}
}

// TestRevertTypeUsesScaledRespawnDuration pins that revertType's heavy
// path goes through removeNpc(n, -1) which would normally consult
// scaleByPlayerCount; -1 short-circuits the RESPAWN-branch lifecycleTick
// write so we expect lifecycleTick UNCHANGED post-revert (TS removeNpc
// 1316-1318: only writes lifecycleTick when duration > -1).
func TestRevertTypeUsesScaledRespawnDuration(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	typ := &objtype.NpcType{ID: 7, Size: 1}
	s.npcTypes.Configs[7] = typ
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.lifecycleTick = 99 // any prior value
	n.resetOnRevert = true

	n.revertType()

	if n.lifecycleTick != 99 {
		t.Errorf("n.lifecycleTick: got %d, want 99 (revertType's duration=-1 must not write)", n.lifecycleTick)
	}
}

func TestNpcAnimate_BoundsRejectAtCount(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(50)
	n := &Npc{server: s, animID: -1}
	n.Animate(50, 5)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (bounds-reject)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set on bounds-reject")
	}
}

func TestNpcAnimate_NilServerEarlyReturn(t *testing.T) {
	// Goscape-only nil-guard (test-fixture concession; no TS analogue).
	n := &Npc{server: nil, animID: -1}
	n.Animate(0, 5)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (nil server → no-op)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set when server is nil")
	}
}

func TestNpcAnimate_PriorityHigherOverwrites(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 3
	cfg.Configs[10].Priority = 7
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (higher priority overwrites)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set on overwrite")
	}
}

func TestNpcAnimate_PriorityLowerRejected(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 7
	cfg.Configs[10].Priority = 3
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5, animDelay: 99}
	n.Animate(10, 3)
	if n.animID != 5 {
		t.Errorf("animID: got %d, want 5 (lower priority rejected)", n.animID)
	}
	if n.animDelay != 99 {
		t.Errorf("animDelay: got %d, want 99 (preserved)", n.animDelay)
	}
	if n.masks&rsbuf.NpcMaskAnim != 0 {
		t.Error("NpcMaskAnim should not be set on rejection")
	}
}

func TestNpcAnimate_CurrentZeroPriorityOverwrites(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 0
	cfg.Configs[10].Priority = 5
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (current zero-priority overwrite)", n.animID)
	}
}

func TestNpcAnimate_FreshAnimIDMinusOneAlwaysOverwrites(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(20)
	n := &Npc{server: s, animID: -1}
	n.Animate(10, 3)
	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (fresh animID=-1 short-circuit)", n.animID)
	}
}

func TestNpcAnimate_ClearWithMinusOneSucceeds(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = buildSeqTypes(20)
	n := &Npc{server: s, animID: 5}
	n.Animate(-1, 0)
	if n.animID != -1 {
		t.Errorf("animID: got %d, want -1 (clear)", n.animID)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set on clear")
	}
}

// TestSetAnimEqualPriorityOverwrites_Npc pins the 244 gate change: when a
// new anim has the SAME nonzero priority as the current anim, the 244 rule
// (>=) overwrites, whereas the 225 rule (>) rejected. TS ref:
// Npc.ts:461 — `SeqType.get(anim).priority >= SeqType.get(this.animId).priority`
// at Engine-TS pin 9aadcec4. The discriminating case is equal nonzero priority.
func TestSetAnimEqualPriorityOverwrites_Npc(t *testing.T) {
	s := newTestServer(t)
	cfg := buildSeqTypes(20)
	// Both seq 5 and seq 10 have the default Priority=5. Seed with seq 5 active.
	s.seqTypes = cfg
	n := &Npc{server: s, animID: 5, animDelay: 99}
	n.masks = 0

	// Play seq 10 at equal priority (5 == 5). 244 gate must overwrite.
	n.Animate(10, 7)

	if n.animID != 10 {
		t.Errorf("animID: got %d, want 10 (equal priority must overwrite under 244 >=)", n.animID)
	}
	if n.animDelay != 7 {
		t.Errorf("animDelay: got %d, want 7", n.animDelay)
	}
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim must be set on equal-priority overwrite")
	}
}

func TestNpc_BlockWalkFlag_PerMoveRestrict(t *testing.T) {
	// Mirrors TS Npc.blockWalkFlag (Npc.ts:395-418 @1d25566c), which
	// Engine-TS 8139461a rewrote around two ORTHOGONAL opt-outs on top of an
	// always-present hard block:
	//
	//   blockwalk=none        -> no NPC_OCC    (walks through npcs)
	//   moverestrict=passthru -> no PLAYER_OCC (walks through players)
	//
	// Both dimensions are covered here; the pre-8139461a test only varied
	// moverestrict, which would leave the blockwalk opt-out unpinned.
	const hard = collision.FlagBlockNpcAndPlayers
	cases := []struct {
		mr        MoveRestrict
		blockWalk int
		want      int
	}{
		// blockwalk=none: npc occupancy opted out.
		{MoveRestrictNormal, objtype.BlockWalkNone, hard | collision.FlagPlayerOcc},
		{MoveRestrictBlockedNormal, objtype.BlockWalkNone, hard | collision.FlagPlayerOcc},
		{MoveRestrictIndoors, objtype.BlockWalkNone, hard | collision.FlagPlayerOcc},
		{MoveRestrictOutdoors, objtype.BlockWalkNone, hard | collision.FlagPlayerOcc},
		// blockwalk set: npc occupancy applies.
		{MoveRestrictNormal, objtype.BlockWalkNPC, hard | collision.FlagNpcOcc | collision.FlagPlayerOcc},
		{MoveRestrictBlockedNormal, objtype.BlockWalkAll, hard | collision.FlagNpcOcc | collision.FlagPlayerOcc},
		// passthru: player occupancy opted out, independently of blockwalk.
		{MoveRestrictPassthru, objtype.BlockWalkNone, hard},
		{MoveRestrictPassthru, objtype.BlockWalkNPC, hard | collision.FlagNpcOcc},
		// early-return arms are unaffected by either opt-out.
		{MoveRestrictBlocked, objtype.BlockWalkNPC, collision.FlagOpen},
		{MoveRestrictNoMove, objtype.BlockWalkNPC, collision.FlagNull},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("MR%d_BW%d", tc.mr, tc.blockWalk), func(t *testing.T) {
			n := &Npc{
				typ:       &objtype.NpcType{MoveRestrict: int(tc.mr)},
				blockWalk: tc.blockWalk,
			}
			if got := n.blockWalkFlag(); got != tc.want {
				t.Errorf("blockWalkFlag(mr=%v, blockwalk=%v) = %d, want %d", tc.mr, tc.blockWalk, got, tc.want)
			}
		})
	}
}

func TestNpc_GetCollisionStrategy_PerMoveRestrict(t *testing.T) {
	// Mirrors TS PathingEntity.getCollisionStrategy at the rev-254 pin
	// (PathingEntity.ts:567-587, 2787f1fb): the Npc branch reads
	// moverestrict LIVE from NpcType; an unknown value falls through to
	// NORMAL (pre-2787f1fb: nil). Full-fidelity enum incl.
	// BLOCKED_NORMAL → LINE_OF_SIGHT.
	cases := []struct {
		mr   MoveRestrict
		want *collision.Type
	}{
		{MoveRestrictNormal, ptrTypeNpc(collision.TypeNormal)},
		{MoveRestrictBlocked, ptrTypeNpc(collision.TypeBlocked)},
		{MoveRestrictBlockedNormal, ptrTypeNpc(collision.TypeLineOfSight)},
		{MoveRestrictIndoors, ptrTypeNpc(collision.TypeIndoors)},
		{MoveRestrictOutdoors, ptrTypeNpc(collision.TypeOutdoors)},
		{MoveRestrictNoMove, nil},
		{MoveRestrictPassthru, ptrTypeNpc(collision.TypeNormal)},
		{MoveRestrict(99), ptrTypeNpc(collision.TypeNormal)}, // unknown → NORMAL (TS L586)
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("MR%d", tc.mr), func(t *testing.T) {
			n := &Npc{typ: &objtype.NpcType{MoveRestrict: int(tc.mr)}}
			got := n.getCollisionStrategy()
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("getCollisionStrategy(%v) nil-mismatch: got %v want %v", tc.mr, got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Errorf("getCollisionStrategy(%v) = %v, want %v", tc.mr, *got, *tc.want)
			}
		})
	}
}

//go:fix inline
func ptrTypeNpc(t collision.Type) *collision.Type { return new(t) }

// TestNpcCleanup pins the (n *Npc) Cleanup() field-zeroing contract.
// Mirrors TS Npc.cleanup at Engine-TS/src/engine/entity/Npc.ts:187-193:
// nid=-1, uid=-1, activeScript=nil, huntTarget=nil, queue cleared.
//
// NAI-19: Cleanup is called from (*Server).removeNpc's DESPAWN-lifecycle
// arm after the registry slot has been nilled. Defensive nullification —
// any caller still holding the *Npc pointer post-DESPAWN reads -1
// sentinels rather than valid-looking state.
func TestNpcCleanup(t *testing.T) {
	n := &Npc{
		nid:          7,
		uid:          (42 << 16) | 7,
		activeScript: &script.ScriptState{},
		huntTarget:   &Npc{nid: 99},
		queue:        []script.NpcQueueRequest{{}, {}},
	}

	n.Cleanup()

	if n.nid != -1 {
		t.Errorf("nid: got %d, want -1", n.nid)
	}
	if n.uid != -1 {
		t.Errorf("uid: got %d, want -1", n.uid)
	}
	if n.activeScript != nil {
		t.Errorf("activeScript: got %p, want nil", n.activeScript)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil", n.huntTarget)
	}
	if n.queue != nil {
		t.Errorf("queue: got %v, want nil", n.queue)
	}
}

// TestNpcCleanupClearsDelayPair pins the fields Engine-TS 8139461a added to
// Npc.cleanup (Npc.ts:198-199 @1d25566c). The Npc struct is reused across
// respawn cycles, so an npc removed mid-delay must not come back still
// delayed until a tick number from its previous life.
func TestNpcCleanupClearsDelayPair(t *testing.T) {
	n := &Npc{
		nid:          5,
		uid:          9,
		delayed:      true,
		delayedUntil: 1234,
	}
	n.Cleanup()

	if n.delayed {
		t.Error("delayed: got true, want false after Cleanup")
	}
	if n.delayedUntil != -1 {
		t.Errorf("delayedUntil: got %d, want -1 after Cleanup", n.delayedUntil)
	}
	if n.activeScript != nil {
		t.Error("activeScript: got non-nil, want nil after Cleanup")
	}
}
