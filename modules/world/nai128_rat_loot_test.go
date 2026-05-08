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
}
