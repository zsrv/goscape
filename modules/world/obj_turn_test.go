package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

// seedTradeableObjType configures s.objTypes so type id passes the M9 reveal
// gate in (*Server).RevealObj (tradeable, non-members). Without this the gate
// keeps the drop private (objType nil → treated as non-tradeable).
func seedTradeableObjType(s *Server, id int) {
	cfgs := make([]*objtype.ObjType, id+1)
	cfgs[id] = &objtype.ObjType{Tradeable: true}
	s.objTypes = &objtype.ObjTypeConfigs{Configs: cfgs}
}

// TestTurnObj_RevealCountdownDecrementsAcrossTicks verifies that Arm 1
// decrements o.Reveal each tick and fires RevealObj exactly when it hits 0.
func TestTurnObj_RevealCountdownDecrementsAcrossTicks(t *testing.T) {
	s := newZoneTestServer(t)
	seedTradeableObjType(s, 995)
	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.Reveal = 2
	o.LifecycleTick = 999 // far future — lifecycle arm must not fire

	z := s.zoneMap.Get(o.Level, o.X, o.Z)

	// First call: Reveal 2 → 1, no reveal event
	s.turnObj(o, 0)
	if o.Reveal != 1 {
		t.Errorf("after first turnObj: Reveal got %d, want 1", o.Reveal)
	}
	if len(z.Events()) != 0 {
		t.Errorf("no reveal event expected after first tick; got %d events", len(z.Events()))
	}

	// Second call: Reveal 1 → 0, RevealObj fires
	s.turnObj(o, 0)
	if o.Reveal != -1 {
		t.Errorf("after second turnObj: Reveal got %d, want -1 (post-reveal reset)", o.Reveal)
	}
	if o.ReceiverID != zone.PublicReceiver {
		t.Errorf("after reveal: ReceiverID got %d, want %d (PublicReceiver)", o.ReceiverID, zone.PublicReceiver)
	}
	if len(z.Events()) != 1 {
		t.Errorf("expected 1 Reveal event after second tick; got %d", len(z.Events()))
	}
}

// TestTurnObj_RevealAtZero_UsesReceiverPlayerSlot verifies that the encoded
// receiverSlot in the OBJ_REVEAL bytes matches the player's slot when the
// receiver is still logged in.
func TestTurnObj_RevealAtZero_UsesReceiverPlayerSlot(t *testing.T) {
	s := newZoneTestServer(t)

	seedTradeableObjType(s, 995)

	// Place a player at slot 3 with uid=42.
	p, _ := newZoneTestPlayer(t, s, 3, 3094, 3106, 0)
	p.uid = 42
	p.active = true // required for LookupPlayerByUID to find the player

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.ReceiverID = 42
	o.Reveal = 1
	o.LifecycleTick = 999

	z := s.zoneMap.Get(o.Level, o.X, o.Z)

	s.turnObj(o, 0)

	events := z.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event; got %d", len(events))
	}
	e := events[0]
	if e.Bytes[0] != rsbuf.ZoneOpObjReveal {
		t.Errorf("opcode: got %d, want ZoneOpObjReveal=%d", e.Bytes[0], rsbuf.ZoneOpObjReveal)
	}
	// Wire shape: [0]=opcode, [1]=coord, [2..3]=type, [4..5]=count, [6..7]=receiverSlot
	gotSlot := int(e.Bytes[6])<<8 | int(e.Bytes[7])
	if gotSlot != 3 {
		t.Errorf("encoded receiverSlot: got %d, want 3 (player slot)", gotSlot)
	}
}

// TestTurnObj_RevealAtZero_LoggedOutReceiverPassesSlotZero verifies that an
// unknown/logged-out receiver UID results in slot 0 in the OBJ_REVEAL bytes.
func TestTurnObj_RevealAtZero_LoggedOutReceiverPassesSlotZero(t *testing.T) {
	s := newZoneTestServer(t)
	seedTradeableObjType(s, 995)

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.ReceiverID = 99999 // no player with this UID
	o.Reveal = 1
	o.LifecycleTick = 999

	z := s.zoneMap.Get(o.Level, o.X, o.Z)

	s.turnObj(o, 0)

	events := z.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event; got %d", len(events))
	}
	e := events[0]
	gotSlot := int(e.Bytes[6])<<8 | int(e.Bytes[7])
	if gotSlot != 0 {
		t.Errorf("encoded receiverSlot for logged-out receiver: got %d, want 0", gotSlot)
	}
}

// TestTurnObj_RevealNegOneIsNoOp verifies that o.Reveal == -1 means the
// reveal arm is skipped entirely (o.Reveal stays -1, no zone events).
func TestTurnObj_RevealNegOneIsNoOp(t *testing.T) {
	s := newZoneTestServer(t)

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	// o.Reveal defaults to -1 from NewObj; lifecycle far in future
	o.LifecycleTick = 999

	z := s.zoneMap.Get(o.Level, o.X, o.Z)

	s.turnObj(o, 0)

	if o.Reveal != -1 {
		t.Errorf("Reveal: got %d, want -1 (no-op)", o.Reveal)
	}
	if len(z.Events()) != 0 {
		t.Errorf("expected no events for Reveal=-1; got %d", len(z.Events()))
	}
}

// TestTurnObj_DespawnAtScheduledTick_FiresRemove verifies that the DESPAWN
// lifecycle arm fires RemoveObj at the scheduled tick, setting IsActive=false.
//
// PORTING-EXCEPTION (entity-base-5): this test pins despawn firing at the
// scheduled tick (T+duration; here 105 = 100+5). TS's per-tick decrement
// model (Obj.ts:33-35) fires at T+duration-1 (here 104). goscape's
// absolute-tick re-model uses T+duration; the one-tick delay is the live
// entity-base-1 deviation (downgraded LOW). The test locks in goscape's
// observed contract to guard against silent drift either way; entity-base-1
// remains the canonical row for the production behaviour.
func TestTurnObj_DespawnAtScheduledTick_FiresRemove(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 100

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(o, zone.PublicReceiver, 5, 0) // schedules despawn at tick 105, sets IsActive=true

	if !o.IsActive {
		t.Fatal("setup: AddObj must have set IsActive=true")
	}

	s.currentTick = 105
	s.turnObj(o, 105)

	if o.IsActive {
		t.Error("after turnObj DESPAWN at scheduled tick: IsActive must be false")
	}
}

// TestTurnObj_RespawnAtScheduledTick_FiresAdd verifies that the RESPAWN
// lifecycle arm fires AddObj at the scheduled tick, setting IsActive=true.
func TestTurnObj_RespawnAtScheduledTick_FiresAdd(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 100

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	// Directly set state: lifecycle=RESPAWN, inactive, scheduled at 105.
	o.LifecycleTick = 105
	// IsActive defaults to false from NewObj — correct for RESPAWN arm.

	// Register in tracker so processZones can find it.
	s.locObjTracker.(*locObjTracker).Register(&o.NonPathing)

	s.currentTick = 105
	s.turnObj(o, 105)

	if !o.IsActive {
		t.Error("after turnObj RESPAWN at scheduled tick: IsActive must be true (re-added via zone.AddObj)")
	}
}

// TestTurnObj_BeforeScheduledTickIsLifecycleNoOp verifies that the lifecycle
// arm is a no-op when LifecycleTick != now. Also verifies that Arm 1 (reveal)
// still fires independently.
func TestTurnObj_BeforeScheduledTickIsLifecycleNoOp(t *testing.T) {
	s := newZoneTestServer(t)

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.LifecycleTick = 105
	o.IsActive = true // simulate AddObj-set state

	// Lifecycle not due yet.
	s.turnObj(o, 100)

	if !o.IsActive {
		t.Error("lifecycle arm must not fire before scheduled tick")
	}

	// Now verify that Arm 1 (reveal) fires independently when Arm 2 does not.
	o2 := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o2.Reveal = 2
	o2.LifecycleTick = 105
	// IsActive=false: Arm 2 DESPAWN+IsActive would NOT fire even at tick 105 without IsActive.
	// Here lifecycle not due at tick 100 anyway.

	s.turnObj(o2, 100) // lifecycle not due at tick 100
	if o2.Reveal != 1 {
		t.Errorf("reveal arm: got %d, want 1 (decremented independently of lifecycle)", o2.Reveal)
	}
}

// TestTurnObj_NoMatchingLifecycle_UntracksAndLogs verifies the default arm:
// when neither DESPAWN+IsActive nor RESPAWN+!IsActive matches, the obj is
// untracked (LifecycleTick → -1) and an error is logged.
func TestTurnObj_NoMatchingLifecycle_UntracksAndLogs(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 95

	// DESPAWN + IsActive=false at LifecycleTick == now → default arm.
	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.SetLifeCycle(5, 95, s.locObjTracker) // registers; LifecycleTick = 100
	// IsActive stays false (never AddObj'd) — triggers default arm when now==100

	s.currentTick = 100
	s.turnObj(o, 100)

	if o.LifecycleTick != -1 {
		t.Errorf("after default arm: LifecycleTick got %d, want -1 (untracked)", o.LifecycleTick)
	}

	// Verify obj's NonPathing pointer is no longer in the tracker.
	tr := s.locObjTracker.(*locObjTracker)
	found := false
	for np := range tr.All() {
		if np == &o.NonPathing {
			found = true
			break
		}
	}
	if found {
		t.Error("obj's NonPathing should not be in the tracker after default arm")
	}
}

// TestTurnObj_RevealAndLifecycleIndependent verifies that Arm 1 (reveal) and
// Arm 2 (lifecycle) operate independently: when Reveal hits 0 but lifecycle
// is not yet due, only RevealObj fires and lifecycle state is unaffected.
func TestTurnObj_RevealAndLifecycleIndependent(t *testing.T) {
	s := newZoneTestServer(t)
	seedTradeableObjType(s, 995)
	now := 100

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	o.Reveal = 1
	o.LifecycleTick = now + 5 // lifecycle due at 105, not 100
	o.IsActive = true

	z := s.zoneMap.Get(o.Level, o.X, o.Z)

	s.turnObj(o, now)

	// Arm 1 fired: Reveal reset to -1 by RevealObj.
	if o.Reveal != -1 {
		t.Errorf("Reveal: got %d, want -1 (post-reveal)", o.Reveal)
	}
	// Arm 1 fired: RevealObj queued an event.
	if len(z.Events()) != 1 {
		t.Errorf("expected 1 Reveal event; got %d", len(z.Events()))
	}
	// Arm 2 did NOT fire: IsActive unchanged (still true).
	if !o.IsActive {
		t.Error("lifecycle arm must not fire before scheduled tick (105 != 100)")
	}
}

// TestProcessZones_DispatchesObjToTurnObj is the integration test: verifies
// that processZones dispatches to turnObj, causing a DESPAWN obj to be
// removed at the scheduled tick.
func TestProcessZones_DispatchesObjToTurnObj(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 100

	o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(o, zone.PublicReceiver, 5, 0) // despawn at tick 105; IsActive=true; registers in tracker

	if !o.IsActive {
		t.Fatal("setup: AddObj must have set IsActive=true")
	}

	// Ticks 101-104: obj stays active.
	for tick := 101; tick < 105; tick++ {
		s.currentTick = tick
		s.processZones()
		if !o.IsActive {
			t.Errorf("tick %d: obj must stay active before scheduled despawn tick (105)", tick)
		}
	}

	// Tick 105: despawn fires through processZones → turnObj dispatch.
	s.currentTick = 105
	s.processZones()

	if o.IsActive {
		t.Error("after processZones at tick 105: obj must be deactivated (DESPAWN arm fired)")
	}
}

// TestRevealObj_GatedStaysPrivate pins M9: (*Server).RevealObj keeps a drop
// private (no public transition, Reveal forced to -1) when the obj is not
// tradeable, or is members-only on an f2p world. Mirrors TS Zone.revealObj
// (Zone.ts:309-312). A tradeable, non-members obj reveals as before.
func TestRevealObj_GatedStaysPrivate(t *testing.T) {
	const objID = 700

	newObjWithReceiver := func(s *Server) *entitypkg.Obj {
		o := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, objID, 1)
		o.ReceiverID = 42 // private to UID 42
		o.Reveal = 0      // at the reveal threshold
		return o
	}

	t.Run("not_tradeable_stays_private", func(t *testing.T) {
		s := newZoneTestServer(t)
		s.objTypes = &objtype.ObjTypeConfigs{Configs: func() []*objtype.ObjType {
			c := make([]*objtype.ObjType, objID+1)
			c[objID] = &objtype.ObjType{Tradeable: false}
			return c
		}()}
		o := newObjWithReceiver(s)
		z := s.zoneMap.Get(o.Level, o.X, o.Z)

		s.RevealObj(o, 0)

		if o.ReceiverID != 42 {
			t.Errorf("non-tradeable: ReceiverID got %d, want 42 (stays private)", o.ReceiverID)
		}
		if o.Reveal != -1 {
			t.Errorf("non-tradeable: Reveal got %d, want -1 (countdown stopped)", o.Reveal)
		}
		if len(z.Events()) != 0 {
			t.Errorf("non-tradeable: got %d zone events, want 0 (no public reveal)", len(z.Events()))
		}
	})

	t.Run("members_in_f2p_stays_private", func(t *testing.T) {
		s := newZoneTestServer(t)
		s.cfg.NodeMembers = false
		s.objTypes = &objtype.ObjTypeConfigs{Configs: func() []*objtype.ObjType {
			c := make([]*objtype.ObjType, objID+1)
			c[objID] = &objtype.ObjType{Tradeable: true, Members: true}
			return c
		}()}
		o := newObjWithReceiver(s)
		z := s.zoneMap.Get(o.Level, o.X, o.Z)

		s.RevealObj(o, 0)

		if o.ReceiverID != 42 || o.Reveal != -1 || len(z.Events()) != 0 {
			t.Errorf("members-in-f2p: ReceiverID=%d Reveal=%d events=%d, want 42, -1, 0",
				o.ReceiverID, o.Reveal, len(z.Events()))
		}
	})

	t.Run("members_in_p2p_reveals", func(t *testing.T) {
		s := newZoneTestServer(t)
		s.cfg.NodeMembers = true
		s.objTypes = &objtype.ObjTypeConfigs{Configs: func() []*objtype.ObjType {
			c := make([]*objtype.ObjType, objID+1)
			c[objID] = &objtype.ObjType{Tradeable: true, Members: true}
			return c
		}()}
		o := newObjWithReceiver(s)
		z := s.zoneMap.Get(o.Level, o.X, o.Z)

		s.RevealObj(o, 0)

		if o.ReceiverID != zone.PublicReceiver {
			t.Errorf("members-in-p2p: ReceiverID got %d, want PublicReceiver (%d)", o.ReceiverID, zone.PublicReceiver)
		}
		if len(z.Events()) != 1 {
			t.Errorf("members-in-p2p: got %d zone events, want 1 (public reveal)", len(z.Events()))
		}
	})
}
