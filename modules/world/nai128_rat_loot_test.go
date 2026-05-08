package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// nai128CacheFixture loads the real data/pack cache + scriptProvider into a
// test Server. Mirrors the loader sequence in modules/world/server.go:175-260
// trimmed to what NAI-128 Stage 1 needs (locTypes, params, objTypes, npcTypes,
// varpTypes, scriptProvider, gamemap). Returns (server, skipReason); if
// skipReason != "" the test should t.Skipf to keep CI-portable.
func nai128CacheFixture(t *testing.T) (*Server, string) {
	t.Helper()
	cacheDir := filepath.Join("..", "..", "data", "pack")
	for _, p := range []string{
		filepath.Join(cacheDir, "server", "script.dat"),
		filepath.Join(cacheDir, "server", "npc.dat"),
		filepath.Join(cacheDir, "server", "obj.dat"),
	} {
		if _, err := os.Stat(p); err != nil {
			return nil, p + " unavailable: " + err.Error()
		}
	}

	s := newTestServer(t)

	// Locs + gamemap (death-side of cascade routes obj_add through zoneMap;
	// gamemap is required for s.zoneMap.Get to anchor on the rat coord).
	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes
	gm := gamemap.New(discardLogger())
	gm.SetLocTypes(locTypes)
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	s.gamemap = gm

	// ObjTypes + params for raw_rat_meat resolution and npc_param(death_drop).
	params, err := objtype.LoadParams(cacheDir)
	if err != nil {
		t.Fatalf("LoadParams: %v", err)
	}
	objTypes, err := objtype.LoadObjTypes(cacheDir, params)
	if err != nil {
		t.Fatalf("LoadObjTypes: %v", err)
	}
	s.paramTypes = params
	s.objTypes = objTypes

	// NpcTypes — defines newbiegiantrat with its death_drop param.
	npcTypes, err := objtype.LoadNPCTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadNPCTypes: %v", err)
	}
	s.npcTypes = npcTypes

	// VarpTypes — for any varp reads inside the cascade.
	varpTypes, err := objtype.LoadVarpTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarpTypes: %v", err)
	}
	s.varpTypes = varpTypes

	// Real script provider — replaces the stub from newTestServer.
	provider := script.NewProvider()
	if err := provider.Load(filepath.Join(cacheDir, "server")); err != nil {
		t.Fatalf("provider.Load: %v", err)
	}
	s.scriptProvider = provider

	return s, ""
}

// TestNAI128_RatLootCascade is the Stage-1 binding probe for NAI-128.
// See docs/superpowers/specs/2026-05-08-nai-128-rat-loot-cascade-investigation-design.md.
func TestNAI128_RatLootCascade(t *testing.T) {
	s, skip := nai128CacheFixture(t)
	if skip != "" {
		t.Skipf("cache unavailable: %s", skip)
	}
	t.Run("FixtureLoaded", func(t *testing.T) {
		if s.scriptProvider == nil {
			t.Fatal("scriptProvider nil after fixture load")
		}
		if s.npcTypes == nil || len(s.npcTypes.Configs) == 0 {
			t.Fatal("npcTypes empty after fixture load")
		}
		if s.objTypes == nil || len(s.objTypes.Configs) == 0 {
			t.Fatal("objTypes empty after fixture load")
		}
	})

	// Resolve newbiegiantrat type and spawn a rat at a benign coord.
	ratTypeID, ok := s.npcTypes.ConfigNames["newbiegiantrat"]
	if !ok {
		t.Fatalf("npc type 'newbiegiantrat' not in ConfigNames; check NPC_FINDHERO predecessor work")
	}
	ratType := s.npcTypes.Configs[ratTypeID]
	rat := NewNpc(1, ratTypeID, 3094, 3106, 0, ratType)
	rat.server = s

	// Spawn a player adjacent to the rat.
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3094, 3107, 0

	t.Run("BaselineState", func(t *testing.T) {
		if rat.uid == 0 {
			t.Errorf("rat.uid = 0; want non-zero (typeId<<16 | nid)")
		}
		if p.UID() == 0 {
			t.Errorf("player.UID() = 0; want non-zero")
		}
		hp := rat.levels[objtype.NpcStatHitpoints]
		if hp <= 0 {
			t.Errorf("rat HP = %d; want > 0 (seeded from typ.Stats[NpcStatHitpoints])", hp)
		}
		t.Logf("rat: uid=%d typeId=%d HP=%d coord=(%d,%d,%d)",
			rat.uid, rat.typeId, hp, rat.x, rat.z, rat.level)
		t.Logf("player: uid=%d coord=(%d,%d,%d)", p.UID(), p.x, p.z, p.level)
	})

	// Register the player with the server before crediting heroPoints.
	// addPlayer assigns p.slot, p.uid=composeUID(username37, slot), and
	// adds to s.players + s.playerLoop with active=true. Required so
	// downstream NPC_FINDHERO -> LookupPlayerByUID(uid) resolves the
	// player at T5 (controller pre-flight: tut_giant_rat.rs2:6 gates
	// obj_add behind npc_findhero=^true).
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Simulate the state player_melee_attack would leave: rat ledger
	// credited with the player's UID, and ai_queue2 enqueued with the
	// damage value. Per spec §4 plan-vs-spec divergence note, this
	// bypasses the RNG-bound hit-roll branch.
	const damage = 5
	rat.heroPoints.AddHero(p.UID(), damage)
	rat.EnqueueScriptForTrigger(script.TriggerAiQueue2, 0, damage)

	// Force rat HP to 1 so the next damage application kills (one-shot
	// simplifies cascade tracing).
	rat.levels[objtype.NpcStatHitpoints] = 1

	t.Run("Preconditions", func(t *testing.T) {
		top := rat.heroPoints.TopContributor()
		if top != p.UID() {
			t.Errorf("rat.heroPoints.TopContributor() = %d; want %d (player.UID)", top, p.UID())
		}
		if len(rat.queue) != 1 {
			t.Fatalf("rat.queue len = %d; want 1 (ai_queue2 enqueued)", len(rat.queue))
		}
		req := rat.queue[0]
		if req.Trigger != script.TriggerAiQueue2 {
			t.Errorf("rat.queue[0].Trigger = %v; want TriggerAiQueue2 (%d)", req.Trigger, script.TriggerAiQueue2)
		}
		if req.LastInt != damage {
			t.Errorf("rat.queue[0].LastInt = %d; want %d", req.LastInt, damage)
		}
		if req.Delay != 0 {
			t.Errorf("rat.queue[0].Delay = %d; want 0", req.Delay)
		}
		if hp := rat.levels[objtype.NpcStatHitpoints]; hp != 1 {
			t.Errorf("rat HP after force-set = %d; want 1", hp)
		}
		t.Logf("registered player: uid=%d slot=%d", p.UID(), p.Slot())
	})

	t.Run("AiQueueCascade", func(t *testing.T) {
		// One processNpcQueue call drains all entries with Delay<=0.
		// Per spec §4.4 phase-collapse pre-flight: ai_queue2 firing
		// runs npc_default_damage which calls NPC_QUEUE(3,0,0); the
		// re-entered enqueue lands at end of n.queue with Delay=0 and
		// fires within the SAME loop iteration via the for-len-grows
		// pattern (npc_script.go:497-526). After this call, BOTH
		// ai_queue2 AND ai_queue3 should have run.
		s.processNpcQueue(rat)

		// Cascade link 1: NPC_DAMAGE (called inside ~npc_default_damage)
		// must have decremented HP to 0.
		if hp := rat.levels[objtype.NpcStatHitpoints]; hp != 0 {
			t.Errorf("rat HP after one processNpcQueue = %d; want 0 (binding candidate B/C: NPC_DAMAGE handler bug or [ai_queue2,_] not dispatching)", hp)
		}

		// Cascade link 2: queue must be drained. If ai_queue3 enqueued
		// but didn't fire (phase-collapse hypothesis wrong), the queue
		// will still contain the ai_queue3 entry. If ai_queue2 didn't
		// dispatch at all, we'd have hit the HP assertion above.
		if remaining := len(rat.queue); remaining != 0 {
			t.Errorf("rat.queue len after cascade = %d; want 0 (binding candidate D-or-tick-order: ai_queue3 enqueued but not fired in same call)", remaining)
			for i, req := range rat.queue {
				t.Logf("  rat.queue[%d]: Trigger=%v Delay=%d LastInt=%d", i, req.Trigger, req.Delay, req.LastInt)
			}
		}
	})

	t.Run("GroundObjs", func(t *testing.T) {
		// Look up expected obj IDs.
		ratMeatID, ok := s.objTypes.ConfigNames["raw_rat_meat"]
		if !ok {
			t.Fatalf("obj type 'raw_rat_meat' not in ConfigNames; binding candidate: ObjType cache gap")
		}

		// Resolve the death_drop param ID, then read it off the rat type.
		// Pre-flight: NpcType.Params is map[uint32]any (paramtype.go:10);
		// no ParamInt method exists. Match handlers_inv.go:247 access shape.
		dropParamID, ok := s.paramTypes.ConfigNames["death_drop"]
		if !ok {
			t.Fatalf("param type 'death_drop' not in ConfigNames; binding candidate: ParamType cache gap")
		}
		v, ok := ratType.Params[uint32(dropParamID)]
		if !ok {
			t.Fatalf("ratType.Params[death_drop] missing; binding candidate D: npc_param(death_drop) returns null")
		}
		// Pre-flight stated "int" but actual cache decoder stores uint32
		// (npc_hunt.go:290 production pattern: v.(uint32) + int32 sign-extend).
		dropObjIDu, ok := v.(uint32)
		if !ok {
			t.Fatalf("ratType.Params[death_drop] = %v (type %T); want uint32", v, v)
		}
		dropObjID := int(int32(dropObjIDu))
		if dropObjID < 0 {
			t.Fatalf("ratType.Params[death_drop] = %d; want a valid obj ID (binding candidate D: npc_param(death_drop) returns -1)", dropObjID)
		}

		// Read the zone at the rat's coord.
		z := s.zoneMap.Get(rat.level, rat.x, rat.z)
		if z == nil {
			t.Fatal("zoneMap.Get returned nil for rat coord")
		}

		// Assert exactly the two obj_adds from [ai_queue3,newbiegiantrat]:
		// obj_add(npc_coord, npc_param(death_drop), 1, ^lootdrop_duration)
		// obj_add(npc_coord, raw_rat_meat,          1, ^lootdrop_duration)
		// Filter by rat coord (the zone may contain other test obj state).
		var atRat []int
		for _, o := range z.Objs {
			if o.X == rat.x && o.Z == rat.z && o.Level == rat.level {
				atRat = append(atRat, o.Type)
			}
		}
		if len(atRat) != 2 {
			t.Errorf("ground obj count at rat coord = %d; want 2 (binding candidate E: OBJ_ADD not registering OR npc_findhero=false skipping the if-block)", len(atRat))
			t.Logf("  observed types at rat coord: %v", atRat)
			t.Logf("  zone.Objs full: %d entries", len(z.Objs))
			for i, o := range z.Objs {
				t.Logf("    [%d] type=%d count=%d at (%d,%d,%d) lifecycle=%v",
					i, o.Type, o.Count, o.X, o.Z, o.Level, o.Lifecycle)
			}
			return
		}

		// Specific-match dispatch verification (spec §6 R3 mitigation):
		// raw_rat_meat is in the [ai_queue3,newbiegiantrat] specific match
		// but NOT in [ai_queue3,_] / [proc,npc_default_death]. Its presence
		// pins specific-trigger dispatch.
		hasMeat := false
		hasDrop := false
		for _, typ := range atRat {
			if typ == ratMeatID {
				hasMeat = true
			}
			if typ == dropObjID {
				hasDrop = true
			}
		}
		if !hasMeat {
			t.Errorf("raw_rat_meat (id=%d) not among ground objs at rat coord; binding candidate: [ai_queue3,newbiegiantrat] specific-match did not dispatch (fell through to [ai_queue3,_] generic)", ratMeatID)
		}
		if !hasDrop {
			t.Errorf("death_drop (id=%d) not among ground objs at rat coord; binding candidate D: npc_param(death_drop) returned a value but obj_add for it did not register", dropObjID)
		}
		t.Logf("ground objs at rat coord: %v (expected death_drop=%d + raw_rat_meat=%d)", atRat, dropObjID, ratMeatID)
	})
}
