package zone

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func TestNewZoneFields(t *testing.T) {
	z := New(42, 1, 100, 200)
	if z.Index != 42 {
		t.Errorf("Index: got %d, want 42", z.Index)
	}
	if z.Level != 1 || z.X != 100 || z.Z != 200 {
		t.Errorf("coords: got (L=%d, X=%d, Z=%d), want (1,100,200)", z.Level, z.X, z.Z)
	}
	if z.entityEvents == nil {
		t.Error("entityEvents map should be initialised")
	}
	if z.Shared() != nil {
		t.Error("fresh zone should have nil Shared()")
	}
	if len(z.Events()) != 0 {
		t.Error("fresh zone should have no events")
	}
}

func TestComputeSharedEmptyIsNil(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.ComputeShared()
	if z.Shared() != nil {
		t.Errorf("Shared after empty ComputeShared: got %v, want nil", z.Shared())
	}
}

func TestComputeSharedConcatsEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x01, 0x02}},
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x03, 0x04, 0x05}},
	}
	z.ComputeShared()
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(z.Shared(), want) {
		t.Errorf("Shared: got %v, want %v", z.Shared(), want)
	}
}

func TestComputeSharedSkipsFollows(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0xEE}},
		{Type: ZoneEventFollows, ReceiverID: 5, Bytes: []byte{0xFF}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0xEE}) {
		t.Errorf("Shared: got %v, want [0xEE]", z.Shared())
	}
}

func TestComputeSharedSkipsTombstones(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0x11}},
		{Type: ZoneEventEnclosed, Bytes: nil}, // tombstone
		{Type: ZoneEventEnclosed, Bytes: []byte{0x22}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0x11, 0x22}) {
		t.Errorf("Shared: got %v, want [0x11 0x22]", z.Shared())
	}
}

func TestResetClearsEverything(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if z.Shared() != nil {
		t.Error("Shared should be nil after Reset")
	}
	if len(z.Events()) != 0 {
		t.Error("events should be empty after Reset")
	}
	if len(z.entityEvents) != 0 {
		t.Error("entityEvents should be empty after Reset")
	}
}

// --- Loc mutations ---

func TestAddLocQueuesEnclosedLocAddChange(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 5, 2)
	z.AddLoc(loc)

	if len(z.Events()) != 1 {
		t.Fatalf("events len: got %d, want 1", len(z.Events()))
	}
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("Type: got %v, want Enclosed", e.Type)
	}
	if e.ReceiverID != PublicReceiver {
		t.Errorf("ReceiverID: got %d, want -1", e.ReceiverID)
	}
	if len(e.Bytes) == 0 || e.Bytes[0] != rsbuf.ZoneOpLocAddChange {
		t.Errorf("Bytes[0]: got %v, want ZoneOpLocAddChange=%d", e.Bytes, rsbuf.ZoneOpLocAddChange)
	}
}

func TestAddLocDespawnAppendsToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("Locs: got %v, want [loc]", z.Locs)
	}
}

func TestAddLocRespawnDoesNotAppendToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	z.AddLoc(loc)
	if len(z.Locs) != 0 {
		t.Errorf("Locs: got %d entries, want 0 (Respawn lifecycle)", len(z.Locs))
	}
	// But event still queued.
	if len(z.Events()) != 1 {
		t.Errorf("events: got %d, want 1", len(z.Events()))
	}
}

func TestChangeLocEmitsLocAddChange(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.ChangeLoc(loc)
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocAddChange {
		t.Errorf("opcode: got %d, want %d", z.Events()[0].Bytes[0], rsbuf.ZoneOpLocAddChange)
	}
}

func TestRemoveLocEmitsLocDelAndPurges(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc)    // queues LocAddChange
	z.RemoveLoc(loc) // tombstones LocAddChange + queues LocDel

	if len(z.Locs) != 0 {
		t.Errorf("Locs after remove: got %d, want 0", len(z.Locs))
	}
	z.ComputeShared()
	// After tombstoning the add, only the LocDel bytes should be in shared.
	if len(z.Shared()) == 0 {
		t.Fatal("Shared should include LocDel bytes")
	}
	if z.Shared()[0] != rsbuf.ZoneOpLocDel {
		t.Errorf("first shared opcode: got %d, want LocDel=%d", z.Shared()[0], rsbuf.ZoneOpLocDel)
	}
	// The original AddChange opcode should NOT appear in shared (tombstoned).
	// (Can't rely on byte equality of 59 in payload; check length).
	// LocDel payload is 2 bytes (coord + packed) + 1 opcode = 3 bytes.
	if len(z.Shared()) != 3 {
		t.Errorf("Shared len: got %d, want 3 (just the LocDel)", len(z.Shared()))
	}
}

func TestAnimLocDoesNotTouchLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AnimLoc(loc, 42)
	if len(z.Locs) != 0 {
		t.Errorf("AnimLoc should not append to Locs; got %d", len(z.Locs))
	}
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocAnim {
		t.Errorf("opcode: want LocAnim=%d", rsbuf.ZoneOpLocAnim)
	}
}

func TestMergeLocEmitsLocMerge(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 5, 5, 2, 2, entity.LifecycleDespawn, 100, 0, 0)
	z.MergeLoc(loc, 3, 10, 20, 6, 4, 4, 6)
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocMerge {
		t.Errorf("opcode: want LocMerge=%d", rsbuf.ZoneOpLocMerge)
	}
}

// --- Obj mutations ---

func TestAddObjPublicIsEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, PublicReceiver, 0)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("public drop should be Enclosed; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjAdd {
		t.Errorf("opcode: want ObjAdd=%d", rsbuf.ZoneOpObjAdd)
	}
	if len(z.Objs) != 1 {
		t.Errorf("Objs: got %d, want 1", len(z.Objs))
	}
}

func TestAddObjPrivateIsFollows(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, 5, 0)
	e := z.Events()[0]
	if e.Type != ZoneEventFollows {
		t.Errorf("private drop should be Follows; got %v", e.Type)
	}
	if e.ReceiverID != 5 {
		t.Errorf("ReceiverID: got %d, want 5", e.ReceiverID)
	}
}

func TestChangeObjEmitsFollowsObjCount(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 7
	z.ChangeObj(obj, 10, 25, 100)
	e := z.Events()[0]
	if e.Type != ZoneEventFollows {
		t.Errorf("ChangeObj should be Follows; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjCount {
		t.Errorf("opcode: want ObjCount=%d", rsbuf.ZoneOpObjCount)
	}
	if obj.Count != 25 {
		t.Errorf("Count after ChangeObj: got %d, want 25", obj.Count)
	}
	if obj.LastChange != 100 {
		t.Errorf("LastChange: got %d, want 100", obj.LastChange)
	}
}

func TestRemoveObjPurgesPendingAdd(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, PublicReceiver, 0)
	z.RemoveObj(obj, 100)

	if len(z.Objs) != 0 {
		t.Errorf("Objs after remove: got %d, want 0", len(z.Objs))
	}
	z.ComputeShared()
	// The add was tombstoned; only the del remains.
	if len(z.Shared()) == 0 {
		t.Fatal("Shared should include ObjDel bytes")
	}
	if z.Shared()[0] != rsbuf.ZoneOpObjDel {
		t.Errorf("first shared opcode: got %d, want ObjDel=%d", z.Shared()[0], rsbuf.ZoneOpObjDel)
	}
}

func TestRemoveObjSkipsEventIfLifecycleTransitionedThisTick(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.LastLifecycleTick = 100
	z.RemoveObj(obj, 100) // lastLifecycleTick == currentTick → skip queuing
	if len(z.Events()) != 0 {
		t.Errorf("events: got %d, want 0 (skip because lifecycle transition this tick)", len(z.Events()))
	}
}

func TestRevealObjEmitsEnclosedObjReveal(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 5
	obj.Reveal = 50
	z.RevealObj(obj, 5)

	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("RevealObj should be Enclosed; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjReveal {
		t.Errorf("opcode: want ObjReveal=%d", rsbuf.ZoneOpObjReveal)
	}
	if obj.ReceiverID != PublicReceiver {
		t.Errorf("ReceiverID after reveal: got %d, want -1", obj.ReceiverID)
	}
	if obj.Reveal != -1 {
		t.Errorf("Reveal after reveal: got %d, want -1", obj.Reveal)
	}
}

// --- Non-entity events ---

func TestAnimMapEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.AnimMap(3, 4, 200, 5, 50)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("AnimMap should be Enclosed")
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapAnim {
		t.Errorf("opcode: want MapAnim=%d", rsbuf.ZoneOpMapAnim)
	}
}

func TestMapProjAnimEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.MapProjAnim(3, 4, 5, 7, 0, 100, 10, 0, 0, 50, 40, 30)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("MapProjAnim should be Enclosed")
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapProjAnim {
		t.Errorf("opcode: want MapProjAnim=%d", rsbuf.ZoneOpMapProjAnim)
	}
}

func TestEventOrderPreserved(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AnimLoc(loc, 1) // event 0: LocAnim
	z.AddLoc(loc)     // event 1: LocAddChange
	z.ComputeShared()
	shared := z.Shared()
	if len(shared) == 0 || shared[0] != rsbuf.ZoneOpLocAnim {
		t.Errorf("first shared opcode: got %d, want LocAnim=%d", shared[0], rsbuf.ZoneOpLocAnim)
	}
}

func TestAddStaticLocAppendsToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 5, 2)
	z.AddStaticLoc(loc)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("Locs: got %v, want [loc]", z.Locs)
	}
	if len(z.Events()) != 0 {
		t.Errorf("AddStaticLoc should not queue events; got %d", len(z.Events()))
	}
}

func TestAddStaticLocNoEntityEvents(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 1, 0, 0)
	z.AddStaticLoc(loc)
	if len(z.entityEvents) != 0 {
		t.Errorf("AddStaticLoc should not register entityEvents; got %d entries", len(z.entityEvents))
	}
}

func TestResetPreservesStaticLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 1, 0, 0)
	z.AddStaticLoc(loc)
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if len(z.Locs) != 1 {
		t.Errorf("Locs should survive Reset; got %d", len(z.Locs))
	}
	if len(z.Events()) != 0 || z.Shared() != nil {
		t.Errorf("per-tick state should be cleared; events=%d shared=%v", len(z.Events()), z.Shared())
	}
}

// stubPlayer implements PlayerLike for Zone subscription tests.
type stubPlayer struct {
	slot  int
	valid bool
}

func (p *stubPlayer) IsValid() bool { return p.valid }
func (p *stubPlayer) Slot() int     { return p.slot }

// stubNpc implements NpcLike for Zone subscription tests.
type stubNpc struct {
	nid   int
	valid bool
}

func (n *stubNpc) IsValid() bool { return n.valid }
func (n *stubNpc) Nid() int      { return n.nid }

func TestZoneEnterPlayerFlagsGridOnFirstEntry(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	z.EnterPlayer(p, g)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("first EnterPlayer should flag the grid at (400,400)")
	}
}

func TestZoneEnterPlayerSecondPlayerDoesNotReFlag(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	// Manually unflag, then add a second player. If the second EnterPlayer
	// re-flags, that's incorrect — only the first should flag.
	g.Unflag(400, 400)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: true}, g)
	if g.IsFlagged(400, 400, 0) {
		t.Error("second EnterPlayer should NOT re-flag a previously-unflagged grid")
	}
}

func TestZoneLeaveLastPlayerUnflagsGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	e := z.EnterPlayer(p, g)
	z.LeavePlayer(p, e, g)
	if g.IsFlagged(400, 400, 0) {
		t.Error("LeavePlayer of last player should unflag grid")
	}
}

func TestZoneLeavePlayerNonLastDoesNotUnflag(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p1 := &stubPlayer{slot: 1, valid: true}
	p2 := &stubPlayer{slot: 2, valid: true}
	e1 := z.EnterPlayer(p1, g)
	z.EnterPlayer(p2, g)
	z.LeavePlayer(p1, e1, g)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("LeavePlayer when others remain should NOT unflag grid")
	}
}

func TestZoneEnterNpcDoesNotFlagGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	n := &stubNpc{nid: 1, valid: true}
	z.EnterNpc(n)
	// NPC enter must not touch the grid (TS Zone.enter only flags for Player).
	if g.IsFlagged(400, 400, 0) {
		t.Error("EnterNpc should NOT flag grid")
	}
}

func TestZoneLeaveNpcDoesNotUnflagGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	// Manually flag the grid (e.g., a player is in this zone).
	g.Flag(400, 400)
	n := &stubNpc{nid: 1, valid: true}
	e := z.EnterNpc(n)
	z.LeaveNpc(n, e)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("LeaveNpc should NOT unflag the grid (only LeavePlayer does)")
	}
}

func TestZoneEnterIncrementsCount(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: true}, g)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	if z.PlayersCount() != 2 {
		t.Errorf("PlayersCount: got %d, want 2", z.PlayersCount())
	}
	if z.NpcsCount() != 1 {
		t.Errorf("NpcsCount: got %d, want 1", z.NpcsCount())
	}
}

func TestZoneLeaveDecrementsCount(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	e := z.EnterPlayer(p, g)
	z.LeavePlayer(p, e, g)
	if z.PlayersCount() != 0 {
		t.Errorf("PlayersCount after Leave: got %d, want 0", z.PlayersCount())
	}
}

func TestZonePlayersSafeFiltersInvalid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: false}, g)
	z.EnterPlayer(&stubPlayer{slot: 3, valid: true}, g)
	got := []int{}
	for p := range z.PlayersSafe(false) {
		got = append(got, p.Slot())
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("PlayersSafe filter: got %v, want [1 3]", got)
	}
}

func TestZoneNpcsSafeFiltersInvalid(t *testing.T) {
	z := New(0, 0, 400, 400)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	z.EnterNpc(&stubNpc{nid: 2, valid: false})
	z.EnterNpc(&stubNpc{nid: 3, valid: true})
	got := []int{}
	for n := range z.NpcsSafe(false) {
		got = append(got, n.Nid())
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("NpcsSafe filter: got %v, want [1 3]", got)
	}
}

func TestZoneResetPreservesSubscription(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	z.Reset()
	// Zone.reset clears events/entityEvents/shared but NOT subscription
	// (mirrors TS Zone.reset at Zone.ts:197-201 which only clears the event-side state).
	if z.PlayersCount() != 1 {
		t.Errorf("Reset should preserve PlayersCount: got %d, want 1", z.PlayersCount())
	}
	if z.NpcsCount() != 1 {
		t.Errorf("Reset should preserve NpcsCount: got %d, want 1", z.NpcsCount())
	}
}

func TestAddStaticLocSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.AddStaticLoc(loc)
	if !loc.IsActive {
		t.Error("AddStaticLoc must set loc.IsActive=true (mirrors TS Zone.addStaticLoc Zone.ts:208)")
	}
}

func TestAddLocSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.AddLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.AddLoc must set loc.IsActive=true (mirrors TS Zone.addLoc Zone.ts:226)")
	}
}

func TestChangeLocSetsIsActiveWhenInactive(t *testing.T) {
	// Pins TS Zone.ts:231 comment: "If a loc is inactive, it should be
	// set to active when we call a change". This is the smoking-gun
	// branch for NAI-88's door-revert bug — a static map loc, never
	// touched by Server.AddLoc, has IsActive=false at script-time
	// change_loc; without this write the revert tick mis-dispatches.
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.ChangeLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.ChangeLoc on inactive loc must set IsActive=true (mirrors TS Zone.changeLoc Zone.ts:232)")
	}
}

func TestChangeLocPreservesActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	loc.IsActive = true
	z.ChangeLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.ChangeLoc on already-active loc must keep IsActive=true")
	}
}

func TestRemoveLocSetsIsActiveFalse(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc) // sets IsActive=true (Task 2)
	if !loc.IsActive {
		t.Fatal("setup: AddLoc should have set IsActive=true")
	}
	z.RemoveLoc(loc)
	if loc.IsActive {
		t.Error("Zone.RemoveLoc must set loc.IsActive=false (mirrors TS Zone.removeLoc Zone.ts:254)")
	}
}

func TestAddStaticObjAppendsToObjs(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 3094, 3106, entity.LifecycleRespawn, 1234, 5)
	z.AddStaticObj(obj)
	if len(z.Objs) != 1 || z.Objs[0] != obj {
		t.Errorf("Objs: got %v, want [obj]", z.Objs)
	}
	if len(z.Events()) != 0 {
		t.Errorf("AddStaticObj should not queue events; got %d", len(z.Events()))
	}
}

func TestAddStaticObjNoEntityEvents(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleRespawn, 1234, 1)
	z.AddStaticObj(obj)
	if len(z.entityEvents) != 0 {
		t.Errorf("AddStaticObj should not register entityEvents; got %d entries", len(z.entityEvents))
	}
}

func TestAddStaticObjSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 3094, 3106, entity.LifecycleRespawn, 1234, 1)
	if obj.IsActive {
		t.Fatal("setup: fresh obj must default IsActive=false")
	}
	z.AddStaticObj(obj)
	if !obj.IsActive {
		t.Error("AddStaticObj must set obj.IsActive=true (mirrors TS Zone.addStaticObj Zone.ts:214)")
	}
}
