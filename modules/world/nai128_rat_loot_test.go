package world

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// nai128CacheFixture loads the real data/pack cache + scriptProvider into a
// test Server. Mirrors the loader sequence in modules/world/server.go:175-260
// trimmed to what NAI-128 Stage 1 needs (locTypes, params, objTypes, npcTypes,
// varpTypes, scriptProvider, gamemap). Returns (server, skipReason); if
// skipReason != "" the test should t.Skipf to keep CI-portable.
func nai128CacheFixture(t *testing.T) (*Server, string) {
	t.Helper()
	cacheDir := ref244CacheDir(t)
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

	// NAI-128 Stage 3: enable NodeDebug so gateway probes fire during
	// the cascade. capturingHandler in CascadeDispatchTrace reads them
	// back as binding regression gates.
	s.cfg.NodeDebug = true

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

	// VarsTypes / VarnTypes — required to size s.vars and s.varsStrings
	// (mirrors NewServer at server.go:225-237). worldVarsView.VarsInt /
	// SetVarsInt key into these slices; with empty slices any cascade-side
	// varp read returns 0 silently rather than panicking on a nil-server
	// short-circuit.
	varsTypes, err := objtype.LoadVarsTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarsTypes: %v", err)
	}
	s.varsTypes = varsTypes
	varnTypes, err := objtype.LoadVarnTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarnTypes: %v", err)
	}
	s.varnTypes = varnTypes
	s.vars = make([]int32, len(varsTypes.Configs))
	s.varsStrings = make([]string, len(varsTypes.Configs))

	// Script-side adapter views — wire after cache types are set so the
	// inner-server pointer references a fixture in its final shape. Mirrors
	// NewServer at server.go:238, 250-252. Without these, ScriptState.World
	// is worldVarsView{s: nil} and LookupPlayerByUID short-circuits to nil
	// at server_varp.go:178-180 — silently breaks NPC_FINDHERO even though
	// s.players is populated.
	s.worldVars = worldVarsView{s: s}
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	// Real script provider — replaces the stub from newTestServer.
	provider := script.NewProvider()
	if _, err := provider.Load(filepath.Join(cacheDir, "server")); err != nil {
		t.Fatalf("provider.Load: %v", err)
	}
	s.scriptProvider = provider

	// zonesTracking — production NewServer initializes this at server.go:169
	// but newTestServer doesn't. Required for the cascade's obj_add → zone
	// AddObj → TrackZone chain (server.go:780).
	if s.zonesTracking == nil {
		s.zonesTracking = map[*zone.Zone]struct{}{}
	}

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
	// addPlayer assigns p.pid, p.uid=composeUID(username37, slot), and
	// adds to s.players with active=true. Required so
	// downstream NPC_FINDHERO -> LookupPlayerByUID(uid) resolves the
	// player at T5 (controller pre-flight: tut_giant_rat.rs2:6 gates
	// obj_add behind npc_findhero=^true).
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Install recording logger before the initial enqueue so the G3 gateway
	// captures both the test-side TriggerAiQueue2 enqueue and the
	// cascade-side TriggerAiQueue3 enqueue. Replaces the production
	// discardLogger with a capturingHandler for the duration of this test.
	rec := &capturingHandler{}
	s.log = slog.New(rec)

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
		// Drive multi-tick to completion: ai_queue3 gosubs into
		// [proc,npc_death] which contains NPC_ARRIVEDELAY (PC 4) and
		// NPC_DELAY (PC 30). Both suspend the script with delayedUntil
		// set on the NPC. n.turn() resumes when currentTick>=delayedUntil.
		// Without this loop, the cascade halts at NPC_ARRIVEDELAY before
		// reaching NPC_FINDHERO and the obj_add calls in ai_queue3.
		// Bound: 16 ticks (worst-case 2 suspends × ~8 ticks each).
		for tick := 0; tick < 16 && rat.activeScript != nil; tick++ {
			s.currentTick++
			rat.turn(s)
		}
		if rat.activeScript != nil {
			t.Errorf("cascade did not complete within 16 ticks; activeScript still suspended at PC=%d Script=%q",
				rat.activeScript.PC, rat.activeScript.Script.Name)
		}

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

	t.Run("CascadeDispatchTrace", func(t *testing.T) {
		// Static probe: is the rat-specific ai_queue3 script even
		// resolvable via the trigger lookup? processNpcQueue silently
		// `continue`s on nil at npc_script.go:518 — no log, no error.
		// nil → E0 bound (provider lookup gap).
		sf := s.scriptProvider.GetByTrigger(script.TriggerAiQueue3, ratTypeID, ratType.Category)
		if sf == nil {
			t.Errorf("scriptProvider.GetByTrigger(TriggerAiQueue3, %d, %d) = nil; binding candidate E0: provider lookup miss for [ai_queue3,newbiegiantrat]",
				ratTypeID, ratType.Category)
		} else {
			t.Logf("ai_queue3 script resolved: %q", sf.Name)
		}

		// Recorder readout: any warn-level "npc script execute error"
		// frames identify a script.Execute error during cascade. The err
		// attr names the failing opcode.
		records := rec.snapshot()
		var execErrors []slog.Record
		for _, r := range records {
			if r.Level == slog.LevelWarn && r.Message == "npc script execute error" {
				execErrors = append(execErrors, r)
			}
		}
		if len(execErrors) > 0 {
			for i, r := range execErrors {
				var scriptName, errStr string
				r.Attrs(func(a slog.Attr) bool {
					switch a.Key {
					case "script":
						scriptName = a.Value.String()
					case "err":
						errStr = a.Value.String()
					}
					return true
				})
				t.Errorf("warn[%d] script=%q err=%q; binding candidate E2b: cascade script errored",
					i, scriptName, errStr)
			}
		}

		// G1 — Npc.Damage gateway. ai_queue2 → ~npc_default_damage runs
		// NPC_DAMAGE during the cascade; assert at least one nai128.npc.damage
		// record fires for the rat.
		var damageRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.npc.damage" {
				damageRecs = append(damageRecs, r)
			}
		}
		if len(damageRecs) == 0 {
			t.Errorf("G1: expected at least one %q record during cascade; got 0", "nai128.npc.damage")
		}

		// G3 — Npc.EnqueueScriptForTrigger gateway. The test pre-enqueues
		// TriggerAiQueue2 manually; the cascade re-enters via NPC_QUEUE
		// inside ~npc_default_damage which enqueues TriggerAiQueue3.
		// Assert both fire (one per enqueue).
		var enqueueRecs []slog.Record
		var sawAiQueue2, sawAiQueue3 bool
		for _, r := range records {
			if r.Message == "nai128.npc.enqueue" {
				enqueueRecs = append(enqueueRecs, r)
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "trigger" {
						switch int(a.Value.Int64()) {
						case int(script.TriggerAiQueue2):
							sawAiQueue2 = true
						case int(script.TriggerAiQueue3):
							sawAiQueue3 = true
						}
					}
					return true
				})
			}
		}
		if !sawAiQueue2 {
			t.Errorf("G3: expected at least one %q record with trigger=TriggerAiQueue2 (%d); got %d enqueue records",
				"nai128.npc.enqueue", script.TriggerAiQueue2, len(enqueueRecs))
		}
		if !sawAiQueue3 {
			t.Errorf("G3: expected at least one %q record with trigger=TriggerAiQueue3 (%d); got %d enqueue records",
				"nai128.npc.enqueue", script.TriggerAiQueue3, len(enqueueRecs))
		}

		// G4 — processNpcQueue per-fire gateway. Both ai_queue2 and
		// ai_queue3 should fire during the cascade per spec §4.4
		// phase-collapse. Assert one queuefire record each by sf.Name
		// shape (rat-specific scripts).
		var queueFireRecs []slog.Record
		var sawAi2Fire, sawAi3Fire bool
		for _, r := range records {
			if r.Message == "nai128.npc.queuefire" {
				queueFireRecs = append(queueFireRecs, r)
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "sf" {
						name := a.Value.String()
						if name == "[ai_queue2,_]" || name == "[ai_queue2,newbiegiantrat]" {
							sawAi2Fire = true
						}
						if name == "[ai_queue3,_]" || name == "[ai_queue3,newbiegiantrat]" {
							sawAi3Fire = true
						}
					}
					return true
				})
			}
		}
		if !sawAi2Fire {
			t.Errorf("G4: expected one %q record for ai_queue2 (specific or generic); got %d queuefire records",
				"nai128.npc.queuefire", len(queueFireRecs))
		}
		if !sawAi3Fire {
			t.Errorf("G4: expected one %q record for ai_queue3 (specific or generic); got %d queuefire records",
				"nai128.npc.queuefire", len(queueFireRecs))
		}

		// G5 — handleNpcFindHero exit gateway. ai_queue3's npc_findhero
		// call should fire one record with pushed=1 (heroPoints credited
		// via test setup; player lookup resolves post-Phase-A).
		var findHeroRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.npc.findhero" {
				findHeroRecs = append(findHeroRecs, r)
			}
		}
		if len(findHeroRecs) == 0 {
			t.Errorf("G5: expected at least one %q record during cascade; got 0", "nai128.npc.findhero")
		} else {
			var pushed int64 = -1
			findHeroRecs[0].Attrs(func(a slog.Attr) bool {
				if a.Key == "pushed" {
					pushed = a.Value.Int64()
				}
				return true
			})
			if pushed != 1 {
				t.Errorf("G5: first record pushed=%d; want 1 (test setup credits heroPoints + Phase A wires LookupPlayerByUID)", pushed)
			}
		}

		// G6 — worldVarsView.AddObj gateway. ai_queue3's two obj_add
		// calls fire one record each (death_drop + raw_rat_meat).
		var addObjRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.obj.add" {
				addObjRecs = append(addObjRecs, r)
			}
		}
		if len(addObjRecs) < 2 {
			t.Errorf("G6: expected at least 2 %q records during cascade (death_drop + raw_rat_meat); got %d",
				"nai128.obj.add", len(addObjRecs))
		}

		// Diagnostic dump of all captured log frames — useful when the
		// binding is none of E0/E2a/E2b (e.g. an unexpected error path).
		if t.Failed() || testing.Verbose() {
			for i, r := range records {
				t.Logf("log[%d] level=%v msg=%q", i, r.Level, r.Message)
			}
		}

		// T6's two positive contracts (sf!=nil; execErrors empty) are
		// permanent regression gates: they catch (a) provider lookup
		// gaps for [ai_queue3,newbiegiantrat] and (b) any handler-level
		// error inside the cascade. The original E2a hypothesis (silent
		// NPC_FINDHERO=0) was a misdiagnosis — the actual root cause was
		// fixture-side: AiQueueCascade was a single processNpcQueue call
		// but [proc,npc_death] (gosub'd from ai_queue3) contains
		// NPC_ARRIVEDELAY + NPC_DELAY, so the cascade requires multi-tick
		// resumption via Npc.turn(). The fix is the tick-driver loop in
		// AiQueueCascade above.
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

// TestNAI128_G2_AddHeroPointsGateway pins the G2 gateway probe at
// (*Npc).AddHeroPoints. NAI-128 Stage 3.
func TestNAI128_G2_AddHeroPointsGateway(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeDebug = true
	rec := &capturingHandler{}
	s.log = slog.New(rec)

	npcType := &objtype.NpcType{}
	n := NewNpc(1, 100, 0, 0, 0, npcType)
	n.server = s

	n.AddHeroPoints(123, 5)

	records := rec.snapshot()
	var found *slog.Record
	for i := range records {
		if records[i].Message == "nai128.heropoints.add" {
			found = &records[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("G2: expected one %q record; got %d total records", "nai128.heropoints.add", len(records))
	}
	var npcUID, playerUID, amount int64
	found.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "npc":
			npcUID = a.Value.Int64()
		case "playerUID":
			playerUID = a.Value.Int64()
		case "amount":
			amount = a.Value.Int64()
		}
		return true
	})
	if npcUID != int64(n.uid) {
		t.Errorf("G2 npc attr = %d; want %d", npcUID, n.uid)
	}
	if playerUID != 123 {
		t.Errorf("G2 playerUID attr = %d; want 123", playerUID)
	}
	if amount != 5 {
		t.Errorf("G2 amount attr = %d; want 5", amount)
	}
}
