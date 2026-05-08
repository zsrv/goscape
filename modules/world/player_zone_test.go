package world

import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func newZoneTestPlayer(t *testing.T, s *Server, slot, x, z, level int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{uint32(slot), 2, 3, 4})
	p.slot = slot
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	// Populate the 13x13 active-zone window centered on (x, z); equivalent
	// to the legacy `ba := buildarea.New(); ba.Rebuild(x, z, 0)` setup.
	_ = p.rebuildScenery(0)
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	return p, cc
}

// TestShouldRebuild_FiresOnFirstBuildEvenWithOriginSet pins the
// rebuiltOnce sentinel against silent regression. Background: tick.go's
// processLogins sets p.originX = p.x to anchor PlayerInfo zone-relative
// encoding (which runs in updatePlayers BEFORE updateMap each tick). An
// earlier draft of NAI-30 B4 T4.5 reused p.originX == -1 as the
// first-build sentinel, but tick.go consumed that field at login —
// shouldRebuild then returned false on the first updateMap call, never
// sending REBUILD_GETMAPS. The map silently failed to render. The fix
// adds a separate p.rebuiltOnce bool gated only by rebuildScenery's
// successful completion.
func TestShouldRebuild_FiresOnFirstBuildEvenWithOriginSet(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Mirror tick.go:processLogins setting originX/Z to a real coord at
	// login, before any updateMap call has run.
	p.x, p.z, p.level = 3094, 3106, 0
	p.originX, p.originZ = p.x, p.z

	if !p.shouldRebuild() {
		t.Fatal("shouldRebuild must return true on first build, even when p.originX is set to a real coord (REBUILD_GETMAPS regression)")
	}

	// After a rebuildScenery call, shouldRebuild should be quiescent for
	// a player who hasn't moved out of the 13x13 zone window.
	_ = p.rebuildScenery(0)
	if p.shouldRebuild() {
		t.Error("shouldRebuild must return false after first rebuild when player is still inside the rebuild window")
	}
}

func TestUpdateZonesSendsPartialEnclosedForActiveZone(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	for z := range s.zonesTracking {
		z.ComputeShared()
	}

	received := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected zone packets, got none")
	}
	// Expect: FullFollows (opcode 135, 2 payload) + PartialFollows wrapper
	// (opcode 7, 2 payload) + OpLocAddChange (59, 4 payload) for the replay
	// + PartialEnclosed (162, -2) with the current-tick shared bytes.
	// That's at minimum 4 packets → many bytes. Assert > 15 bytes as a smoke test.
	if len(got) < 15 {
		t.Errorf("got %d bytes, expected many (full+partial+replay+enclosed)", len(got))
	}
}

func TestUpdateZonesFullFollowsOnlyFirstLoad(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	// First call: writes FullFollows.
	received := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	first := <-received
	if len(first) == 0 {
		t.Fatal("first updateZones should emit FullFollows for each active zone")
	}

	// Second call: all zones now in LoadedZones → no FullFollows.
	received2 := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	second := <-received2
	if len(second) != 0 {
		t.Errorf("second updateZones with no new events should emit nothing; got %d bytes", len(second))
	}
}

func TestUpdateZonesUnloadsDroppedZones(t *testing.T) {
	s := newZoneTestServer(t)
	p, _ := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)
	// Populate LoadedZones with an index NOT in ActiveZones.
	bogusIdx := 999999
	p.loadedZones[bogusIdx] = true

	p.updateZones()
	if p.loadedZones[bogusIdx] {
		t.Error("bogus index not in activeZones should have been unloaded")
	}
}

func TestWriteFullFollowsReplaysActiveLocs(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	// Preload a dynamic Loc into the zone (bypassing Server.AddLoc so nothing
	// lives in zonesTracking — we're testing the replay path only).
	z := s.zoneMap.Get(0, 3094, 3106)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 5, 2)
	z.Locs = append(z.Locs, loc)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 1)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected FullFollows + PartialFollows + LocAddChange packets")
	}
	// First byte should be the encrypted OpUpdateZoneFullFollows opcode.
}

func TestWriteFullFollowsSkipsThisTickTransitions(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 100
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	z := s.zoneMap.Get(0, 3094, 3106)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	loc.LastLifecycleTick = 100 // transitioned this tick → skip
	z.Locs = append(z.Locs, loc)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 100)
	p.client.flushWrite()
	got := <-received
	// Only the FullFollows header (opcode 135 + 2 header bytes = 3 bytes).
	if len(got) != 3 {
		t.Errorf("want exactly 3 bytes (FullFollows header, no replay); got %d", len(got))
	}
}

func TestPartialFollowsFiltersByReceiverID(t *testing.T) {
	s := newZoneTestServer(t)
	// Player at slot 7 with a derived UID (username37=1 → uid = (1<<11)|7 = 2055).
	p, cc := newZoneTestPlayer(t, s, 7, 3094, 3106, 0)
	p.uid = composeUID(1, 7)
	otherUID := composeUID(2, 3) // username37=2, slot 3 → uid = (2<<11)|3 = 4099 (distinct, > 2047)

	z := s.zoneMap.Get(0, 3094, 3106)
	// Two Follows events: one for otherUID, one for p.uid.
	objOther := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	objMine := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(objOther, otherUID)
	s.AddObj(objMine, p.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected follows packets for p.uid")
	}
	// Should include exactly one Follows wrapper + one ObjAdd for p.uid
	// (otherUID filtered). 2 + 5 = 7 bytes payload + 2 opcode bytes = 9.
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 header + 1 ObjAdd for p.uid); got %d", len(got))
	}
}

func TestSendZoneNestedUnknownOpcodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on unknown opcode")
		}
	}()
	p, _ := newTestPlayer(t)
	sendZoneNested(p, []byte{255, 0, 0})
}

func TestPartialFollowsDeliversPrivateDropToOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	p.uid = composeUID(1, 5) // uid = (1<<11)|5 = 2053

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	s.AddObj(obj, p.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	// 1 Follows wrapper (opcode + 2-byte payload = 3 bytes) +
	// 1 ObjAdd nested (opcode + 4-byte payload = 6 bytes; rsbuf.EncodeObjAdd
	// writes 1-byte coord-pack stub + 2-byte type + 1-byte count … but the
	// existing slot=7 test pins this combined wire shape at 9 bytes).
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 Follows wrapper + 1 ObjAdd for p.uid); got %d", len(got))
	}
}


func TestPartialFollowsHidesPrivateDropFromNonOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	// Owner at slot 5 with uid = (1<<11)|5 = 2053.
	owner, _ := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	owner.uid = composeUID(1, 5)
	// Other player at slot 9 with uid = (2<<11)|9 = 4105 (distinct).
	other, otherCC := newZoneTestPlayer(t, s, 9, 3094, 3106, 0)
	other.uid = composeUID(2, 9)

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	s.AddObj(obj, owner.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, otherCC)
	other.writePartialFollows(z)
	other.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("non-owner must receive no bytes; got %d (%v)", len(got), got)
	}
}

func TestFullFollowsReplaysPrivateDropToOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	p.uid = composeUID(1, 5) // uid = 2053

	// Preload a dynamic Obj into the zone with ReceiverID == p.uid. Bypass
	// Server.AddObj so nothing lives in zonesTracking — we're testing the
	// replay path only. Set obj.ReceiverID directly to mirror what
	// worldVarsView.AddObj does at server_varp.go:169.
	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	obj.ReceiverID = p.uid
	obj.LifecycleTick = 100 // despawn at tick 100 → alive at tick 1 (CheckLifecycle: LifecycleTick > tick)
	z.Objs = append(z.Objs, obj)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 1) // currentTick=1, obj.LastLifecycleTick=0 → replay.
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected FullFollows + PartialFollows + ObjAdd packets")
	}
	// FullFollows header (3 bytes: opcode + 2 payload) + PartialFollows
	// wrapper (3 bytes) + ObjAdd (6 bytes: opcode + 5 payload) = 12 bytes.
	// The existing TestWriteFullFollowsSkipsThisTickTransitions pins the
	// header-only baseline at 3 bytes.
	if len(got) != 12 {
		t.Errorf("want 12 bytes (FullFollows header + PartialFollows wrapper + 1 ObjAdd); got %d", len(got))
	}
}

func TestFullFollowsHidesPrivateDropFromNonOwnerInReplay(t *testing.T) {
	s := newZoneTestServer(t)
	owner, _ := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	owner.uid = composeUID(1, 5) // uid = 2053
	other, otherCC := newZoneTestPlayer(t, s, 9, 3094, 3106, 0)
	other.uid = composeUID(2, 9) // uid = 4105

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	obj.ReceiverID = owner.uid
	obj.LifecycleTick = 100 // alive at tick 1 — proves filter is UID, not lifecycle
	z.Objs = append(z.Objs, obj)

	received := drainConn(t, otherCC)
	other.writeFullFollows(z, 1)
	other.client.flushWrite()
	got := <-received
	// Expect only the FullFollows header (3 bytes); no PartialFollows
	// wrapper, no ObjAdd. Mirrors TestWriteFullFollowsSkipsThisTickTransitions
	// header-only baseline.
	if len(got) != 3 {
		t.Errorf("non-owner replay must produce header-only (3 bytes); got %d (%v)", len(got), got)
	}
}
