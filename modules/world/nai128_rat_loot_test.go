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
}
