package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// setupLookupServer returns a Server with npcLookup bound and NpcTypes 7
// ("Hans", category 5) and 8 ("Other", category 9) registered. Mirrors
// the fixture patterns at player_npc_test.go:33 and script_test.go:939+.
func setupLookupServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.npcLookup = serverNpcLookup{s: s}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: make([]*objtype.NpcType, 100),
	}
	s.npcTypes.Configs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "hans"},
		Name:       "Hans",
		Category:   5,
	}
	s.npcTypes.Configs[8] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 8, DebugName: "other"},
		Name:       "Other",
		Category:   9,
	}
	return s
}

// TestServerNpcLookup_FindClosestByType places 3 NPCs (2 of target type,
// 1 other) and asserts that the closer of the two target-type NPCs is
// returned and the wrong-type NPC is never returned regardless of distance.
func TestServerNpcLookup_FindClosestByType(t *testing.T) {
	s := setupLookupServer(t)
	// near: target type at (50,50,0), far: target type at (60,50,0),
	// wrong: different type at (51,50,0) — closer than far but wrong type.
	near := setupNpc(t, s, 50, 50, 0)
	near.typeId = 7
	far := setupNpc(t, s, 60, 50, 0)
	far.typeId = 7
	wrong := setupNpc(t, s, 51, 50, 0)
	wrong.typeId = 8

	lookup := s.npcLookup
	got := lookup.FindClosestNpcByType(0, 50, 50, 30, 7, 0)
	if got == nil {
		t.Fatal("expected to find an NPC, got nil")
	}
	gotNpc, ok := got.(*Npc)
	if !ok {
		t.Fatalf("got type %T, want *Npc", got)
	}
	if gotNpc != near {
		t.Errorf("expected closest NPC (near), got %v", gotNpc)
	}
}

// TestServerNpcLookup_FindClosestByCategory places 2 NPCs with matching
// category (5) and 1 non-matching (category 9); asserts the closer
// category-match is returned and the non-matching NPC is not.
func TestServerNpcLookup_FindClosestByCategory(t *testing.T) {
	s := setupLookupServer(t)
	// catMatch: type 7 (category 5) at (50,50,0)
	// catFar: type 7 (category 5) at (60,50,0)
	// catMiss: type 8 (category 9) at (51,50,0) — closer than catFar but wrong category.
	catMatch := setupNpc(t, s, 50, 50, 0)
	catMatch.typeId = 7
	catFar := setupNpc(t, s, 60, 50, 0)
	catFar.typeId = 7
	catMiss := setupNpc(t, s, 51, 50, 0)
	catMiss.typeId = 8

	lookup := s.npcLookup
	got := lookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, 0)
	if got == nil {
		t.Fatal("expected to find an NPC with category 5, got nil")
	}
	gotNpc, ok := got.(*Npc)
	if !ok {
		t.Fatalf("got type %T, want *Npc", got)
	}
	if gotNpc != catMatch {
		t.Errorf("expected closer category-match NPC (catMatch), got %v", gotNpc)
	}
}

// TestServerNpcLookup_FindAtExactCoord places an NPC at (50,50,0) of type 7
// and verifies hit + four miss sub-cases (off-by-one x, off-by-one z, wrong
// level, wrong type). The "wrong type" sub-case closes spec §5.4 #3 coverage
// gap: a mock in handler tests cannot simulate the type filter world-side.
func TestServerNpcLookup_FindAtExactCoord(t *testing.T) {
	s := setupLookupServer(t)
	exact := setupNpc(t, s, 50, 50, 0)
	exact.typeId = 7

	lookup := s.npcLookup

	// Hit.
	got := lookup.FindNpcAtExactCoord(0, 50, 50, 7)
	if got == nil {
		t.Fatal("exact coord lookup should find the NPC")
	}

	// Compile-time assertion: serverNpcLookup satisfies script.NpcLookup.
	var _ script.NpcLookup = s.npcLookup

	// Miss sub-cases.
	for _, tc := range []struct {
		name    string
		l, x, z int
		typeID  int
	}{
		{"off by one x", 0, 51, 50, 7},
		{"off by one z", 0, 50, 51, 7},
		{"wrong level", 1, 50, 50, 7},
		{"wrong type", 0, 50, 50, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lookup.FindNpcAtExactCoord(tc.l, tc.x, tc.z, tc.typeID); got != nil {
				t.Errorf("%s: expected nil, got %v", tc.name, got)
			}
		})
	}
}

// --- NAI-33 Task 2: serverNpcLookup.ZoneNpcs tests ----------------------

func TestServerNpcLookup_ZoneNpcs_EmptyZone(t *testing.T) {
	s := setupLookupServer(t)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 0 {
		t.Errorf("empty zone: got len=%d, want 0", len(got))
	}
}

func TestServerNpcLookup_ZoneNpcs_SingleNpc(t *testing.T) {
	s := setupLookupServer(t)
	n := setupNpc(t, s, 3200, 3300, 0)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 {
		t.Fatalf("got len=%d, want 1", len(got))
	}
	if got[0] != script.ActiveNpc(n) {
		t.Errorf("got %v, want %v", got[0], n)
	}
}

func TestServerNpcLookup_ZoneNpcs_OnlyRequestedZone(t *testing.T) {
	s := setupLookupServer(t)
	nIn := setupNpc(t, s, 3200, 3300, 0)
	_ = setupNpc(t, s, 3300, 3400, 0) // different zone (zone-aligned 3296 ≠ 3200)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 {
		t.Fatalf("got len=%d, want 1 (only the requested zone's NPC)", len(got))
	}
	if got[0] != script.ActiveNpc(nIn) {
		t.Errorf("got %v, want %v", got[0], nIn)
	}
}

func TestServerNpcLookup_ZoneNpcs_OffGridReturnsEmpty(t *testing.T) {
	s := setupLookupServer(t)
	got := s.npcLookup.ZoneNpcs(0, -1000, -1000) // outside any allocated zone
	if len(got) != 0 {
		t.Errorf("off-grid: got len=%d, want 0", len(got))
	}
}
