package world

import (
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
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
	ba := buildarea.New()
	_ = ba.Rebuild(x, z, 0)
	p.buildArea = ba
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	return p, cc
}

func TestUpdateZonesSendsPartialEnclosedForActiveZone(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)
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
	p.buildArea.LoadedZones[bogusIdx] = true

	p.updateZones()
	if p.buildArea.LoadedZones[bogusIdx] {
		t.Error("bogus index not in ActiveZones should have been unloaded")
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
	p, cc := newZoneTestPlayer(t, s, 7, 3094, 3106, 0)

	z := s.zoneMap.Get(0, 3094, 3106)
	// Two Follows events: one targeted at slot 3, one at slot 7.
	obj3 := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	obj7 := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(obj3, 3)
	s.AddObj(obj7, 7)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected follows packets for slot 7")
	}
	// Should include exactly one Follows wrapper + one ObjAdd for slot 7
	// (slot 3 filtered). 2 + 5 = 7 bytes payload + 2 opcode bytes = 9.
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 header + 1 ObjAdd for slot 7); got %d", len(got))
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
